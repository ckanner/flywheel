// Package defectinjection — 缺陷注入抽检（docs/design.md §4.3）：防假测试。
// 编排器在 L1 与 Review 之间确定性运行：先确立 GREEN 基线，注入可逆补丁，
// 跑测试期望变 RED，再 REVERSE-APPLY 还原。常开无开关（用户裁决 2026-08-20）。
//
// 结果分支（P0-2，用户核实）：
//
//	pass           基线绿 + 注入后测试红 + 还原干净
//	fake-tests     带缺陷测试仍绿（DC3 -> inbox + stop）
//	uninjectable   补丁不可读 / 触账本路径 / 无法 apply（陈旧补丁 -> inbox 请人刷新，
//	               不指控假测试，单元继续）
//	baseline-red   注入前套件已红（坏套件 -> inbox，单元继续）
//	restore-failed 反向 apply 失败（工作区可能仍带缺陷 -> inbox + stop）
//
// 还原用 `git apply -R`（P0-1，用户核实）：只移除注入的 hunk，保留执行者
// 在同文件上的其余未提交改动——`git checkout -- <files>` 会抹掉它们。反向
// apply 同样覆盖二进制/重命名/模式变更补丁；之后无需 workspaceClean 检查：
// 执行者在被触碰文件上的自有 diff 本来就应该在那里。
package defectinjection

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/ledger"
)

// maxBuffer 单流累计上限（Node execFile maxBuffer = 4MB；超限按 spawn 错误
// 处理）。包级变量供测试注入小上限覆盖溢出分支（真实 4MB 分支不可稳定复现）。
var maxBuffer = int64(4 * 1024 * 1024)

// RunResult 一次命令运行的可观察结果。
type RunResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// RunFunc 可注入的命令执行器（逻辑测试无需真实 git 仓库）。
type RunFunc func(cmd string, args []string, cwd string, timeoutMs int64, envAllowlist []string) RunResult

// DefaultRun 真实执行：白名单环境（P0-4）+ 超时 + 双流截断缓冲。
// Node execFile 的超时/maxBuffer 超限都走 catch 分支映射为 exitCode=1
// （typeof err.code === 'number' 为假）——此处等价：非正常退出（信号/超时/
// 启动失败）一律 exitCode=1。
func DefaultRun(cmdName string, args []string, cwd string, timeoutMs int64, envAllowlist []string) RunResult {
	ctx := context.Background()
	var cancel context.CancelFunc
	if timeoutMs > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cwd
	cmd.Env = childenv.EnvSlice(childenv.GateEnv(childenv.OSLookupEnv, envAllowlist))
	stdout := &limitedBuffer{limit: maxBuffer}
	stderr := &limitedBuffer{limit: maxBuffer}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if err == nil && !stdout.overflow && !stderr.overflow {
		return RunResult{ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String()}
	}
	// Node：maxBuffer 超限会杀子进程并以错误收场，catch 分支里
	// typeof err.code === 'number' 不成立 → exitCode 恒 1。
	code := 1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() >= 0 {
		code = exitErr.ExitCode()
	}
	return RunResult{ExitCode: code, Stdout: stdout.String(), Stderr: stderr.String()}
}

// ParsePatchFiles 解析 unified diff 触碰的文件路径（仅供账本路径守卫；
// 还原不再依赖该清单）。返回去重后的 created / modified 列表。
func ParsePatchFiles(patchContent string) (created, modified []string) {
	createdSeen := map[string]bool{}
	modifiedSeen := map[string]bool{}
	oldPath := ""
	for _, line := range strings.Split(patchContent, "\n") {
		switch {
		case strings.HasPrefix(line, "--- "):
			oldPath = strings.TrimSpace(line[4:])
		case strings.HasPrefix(line, "+++ "):
			newPath := strings.TrimSpace(line[4:])
			strip := func(p string) string {
				if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
					return p[2:]
				}
				return p
			}
			switch {
			case oldPath == "/dev/null":
				if s := strip(newPath); !createdSeen[s] {
					createdSeen[s] = true
					created = append(created, s)
				}
			case newPath != "/dev/null":
				if s := strip(newPath); !modifiedSeen[s] {
					modifiedSeen[s] = true
					modified = append(modified, s)
				}
			default: // 删除文件
				if s := strip(oldPath); !modifiedSeen[s] {
					modifiedSeen[s] = true
					modified = append(modified, s)
				}
			}
			oldPath = ""
		}
	}
	return created, modified
}

// ShouldSample 抽样决策（常开）：每 N 个单元抽一次（当前单元计为
// completedCount+1，故 N==1 时首个单元即抽样）；diff 以测试文件为主时更积极。
func ShouldSample(completedUnitCount, everyNUnits int, boost, testFileRatio float64) bool {
	if testFileRatio >= boost && completedUnitCount >= 1 {
		return true
	}
	return (completedUnitCount+1)%everyNUnits == 0
}

// Outcome 一次注入周期的结论（JSON 键与 Node 版逐字一致，直接落
// state.json evidence["defect-injection"]）。detail 仅在有话可说时出现。
type Outcome struct {
	Injected      bool    `json:"injected"`
	BaselineGreen *bool   `json:"baselineGreen"`
	TestsWentRed  bool    `json:"testsWentRed"`
	Restored      bool    `json:"restored"`
	Pass          bool    `json:"pass"`
	Outcome       string  `json:"outcome"`
	Detail        *string `json:"detail,omitempty"`
}

// Args RunDefectInjection 入参。
type Args struct {
	PatchFile    string // 可逆补丁路径
	TestCmd      string // 注入后应变红的测试命令（经 sh -c）
	Cwd          string // 工作区
	TimeoutMs    int64  // 每条命令的超时上限；<=0 不限时（Node 缺省 300s 由 config 层给值）
	EnvAllowlist []string
	ForbidPaths  []string // 兼容入口：repo 相对路径精确/前缀匹配（v0.18 C3）
	// LedgerTaskPrefix（v0.19，单一信源）：给出 docs/<slug>/ 前缀时，账本判定
	// 直接复用 internal/ledger.IsLedgerTaskPath——与边界判定/提交排除同一套
	// 语义，杜绝两套匹配规则漂移。设置后 ForbidPaths 被忽略。
	LedgerTaskPrefix string
	Run              RunFunc // nil → DefaultRun
}

// RunDefectInjection 运行一个注入周期。绝不 panic——坏补丁或任何 spawn
// 失败都映射为一种 outcome，而不是编排器崩溃（P0-2）。
func RunDefectInjection(a Args) Outcome {
	run := a.Run
	if run == nil {
		run = DefaultRun
	}
	outcomeOf := func(name, detail string) Outcome {
		return Outcome{Outcome: name, Detail: new(detail)}
	}

	patchAbs := a.PatchFile
	if !filepath.IsAbs(patchAbs) {
		patchAbs = filepath.Join(a.Cwd, patchAbs)
	}
	patchContent, err := os.ReadFile(patchAbs)
	if err != nil {
		return outcomeOf("uninjectable", fmt.Sprintf("patch unreadable: %v", err))
	}
	created, modified := ParsePatchFiles(string(patchContent))
	touched := append(append([]string{}, created...), modified...)
	var forbidden []string
	if a.LedgerTaskPrefix != "" {
		// 单一信源路径：与边界判定/提交排除共用 internal/ledger 分类。
		for _, f := range touched {
			if ledger.IsLedgerTaskPath(strings.TrimPrefix(f, a.LedgerTaskPrefix)) {
				forbidden = append(forbidden, f)
			}
		}
	} else {
		for _, f := range touched {
			for _, p := range a.ForbidPaths {
				if f == p || (strings.HasSuffix(p, "/") && strings.HasPrefix(f, p)) {
					forbidden = append(forbidden, f)
					break
				}
			}
		}
	}
	if len(forbidden) > 0 {
		return outcomeOf("uninjectable", fmt.Sprintf("patch touches forbidden (ledger) paths: %s", strings.Join(forbidden, ", ")))
	}

	// 先跑基线（P0-2 case 3）：套件本来就是红的时"注入后变红"证明不了任何事。
	baseline := run("/bin/sh", []string{"-c", a.TestCmd}, a.Cwd, a.TimeoutMs, a.EnvAllowlist)
	if baseline.ExitCode != 0 {
		return outcomeOf("baseline-red", "test suite red before injection (baseline broken)")
	}

	apply := run("git", []string{"apply", patchAbs}, a.Cwd, a.TimeoutMs, a.EnvAllowlist)
	if apply.ExitCode != 0 {
		o := outcomeOf("uninjectable", fmt.Sprintf("git apply failed: %s", strings.TrimSpace(apply.Stderr)))
		o.BaselineGreen = new(true)
		return o
	}

	tests := run("/bin/sh", []string{"-c", a.TestCmd}, a.Cwd, a.TimeoutMs, a.EnvAllowlist)
	testsWentRed := tests.ExitCode != 0

	// 反向 apply 还原（P0-1）。
	restore := run("git", []string{"apply", "-R", patchAbs}, a.Cwd, a.TimeoutMs, a.EnvAllowlist)
	if restore.ExitCode != 0 {
		o := Outcome{
			Injected:      true,
			BaselineGreen: new(true),
			TestsWentRed:  testsWentRed,
			Restored:      false,
			Pass:          false,
			Outcome:       "restore-failed",
		}
		o.Detail = new(fmt.Sprintf("git apply -R failed: %s", strings.TrimSpace(restore.Stderr)))
		return o
	}

	if !testsWentRed {
		o := Outcome{
			Injected:      true,
			BaselineGreen: new(true),
			TestsWentRed:  false,
			Restored:      true,
			Pass:          false,
			Outcome:       "fake-tests",
		}
		o.Detail = new("tests stayed green with the defect injected (fake/weak tests)")
		return o
	}
	return Outcome{
		Injected:      true,
		BaselineGreen: new(true),
		TestsWentRed:  true,
		Restored:      true,
		Pass:          true,
		Outcome:       "pass",
	}
}

// limitedBuffer 截断式缓冲：超过 limit 后丢弃后续字节并记 overflow
// （写入不报错，让进程跑完；溢出按 Node maxBuffer 语义映射 exitCode=1）。
type limitedBuffer struct {
	buf      bytes.Buffer
	limit    int64
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if int64(b.buf.Len())+int64(len(p)) > b.limit {
		if room := int(b.limit) - b.buf.Len(); room > 0 {
			b.buf.Write(p[:room])
		}
		b.overflow = true
		return len(p), nil
	}
	b.buf.Write(p)
	return len(p), nil
}

func (b *limitedBuffer) String() string { return b.buf.String() }
