// Package verify — L1 确定性验收（docs/design.md §4.4）：编排器亲自跑契约内的
// 验收命令——它们是 ground-truth oracle：便宜、瞬时、无法被执行者辩驳。
// coverage gate 只是 packet 里又一条 L1 命令，此处无任何特判。
//
// 测试/边界完整性由 git diff 事后判定：单元与回归测试可修改，冻结的验收
// 由 must-not-touch 或显式保护模式约束。另含 per-unit worktree 隔离与编排器侧
// 单元提交（§4.2 v0.16）。
package verify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// DefaultTestFilePatterns 测试文件识别模式（缺陷抽样等用途）；不隐式禁止修改。
var DefaultTestFilePatterns = []string{
	`(^|/)(tests?|__tests__|spec)/`,
	`\.(test|spec)\.[cm]?[jt]sx?$`,
}

// EmptyTree 零提交仓库的 diff 基准（git 众所周知空树 sha）。
const EmptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// DiffTextCap 评审用全文 diff 截断上限（~100KB）。
const DiffTextCap = 100_000

// ---- 环境传递 helpers ----

// envLookup 把环境来源归一为 childenv.LookupEnv；m 为 nil 时读进程环境。
func envLookup(m map[string]string) childenv.LookupEnv {
	if m == nil {
		m = map[string]string{}
		for _, kv := range os.Environ() {
			if i := strings.IndexByte(kv, '='); i >= 0 {
				m[kv[:i]] = kv[i+1:]
			}
		}
	}
	return func(key string) (string, bool) { v, ok := m[key]; return v, ok }
}

func envSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// ---- L1 gate 执行 ----

// GateResult 单条验收命令的结果。
type GateResult struct {
	CheckID      string `json:"checkId,omitempty"`
	Cmd          string `json:"cmd"`
	Expect       string `json:"expect"`
	ExitCode     int    `json:"exitCode"`
	Pass         bool   `json:"pass"`
	DurationMS   int64  `json:"durationMs"`
	Output       string `json:"output,omitempty"`
	OutputDigest string `json:"outputDigest,omitempty"`
	// OutputTruncated clips the displayed summary only; Truncated below means
	// the watchdog lost process output and must invalidate the gate outcome.
	OutputTruncated bool `json:"outputTruncated,omitempty"`
	// ObservationComplete describes the command process outcome, not discovery
	// of a complete test-case inventory inside an arbitrary shell command.
	ObservationComplete bool   `json:"observationComplete"`
	TimedOut            bool   `json:"timedOut"`
	Cancelled           bool   `json:"cancelled"`
	Truncated           bool   `json:"truncated"`
	Signal              string `json:"signal,omitempty"`
	SpawnError          string `json:"spawnError,omitempty"`
	CleanupError        string `json:"cleanupError,omitempty"`
}

// GateReport 一个 gate 层的完整结果。普通失败继续收集结果；取消后的命令
// 记录 cancelled 而不再启动，避免 stop 与下一条命令的 spawn 竞态。
type GateReport struct {
	Pass          bool         `json:"pass"`
	Results       []GateResult `json:"results"`
	EnvironmentID string       `json:"environmentId,omitempty"`
}

// GateOpts RunGateCommands 选项。Env 为 nil 时用进程环境（经白名单过滤）。
type GateOpts struct {
	Context      context.Context
	Cwd          string
	Env          map[string]string
	TimeoutMS    int64
	KillGraceMS  int64
	EnvAllowlist []string
}

// RunGateCommands 跑一个 gate 层的命令序列。每条命令拿白名单环境（P0-4）：
// 它们执行 executor 写的代码，不得看见编排器的密钥；config verify.env-allowlist
// 是唯一显式出口。
func RunGateCommands(commands []contract.GateEntry, opts GateOpts) GateReport {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// One filtered environment snapshot makes every command in this gate use
	// the same supplied variables; only its digest is retained in the report.
	env := childenv.GateEnv(envLookup(opts.Env), opts.EnvAllowlist)
	childEnv := envSlice(env)
	sort.Strings(childEnv)
	report := GateReport{EnvironmentID: gateEnvironmentID(env)}
	for index, entry := range commands {
		command := entry.Command()
		checkID := gateCheckID(index, command, entry.Expect)
		expected, perr := contract.ParseExpect(entry.Expect)
		if perr != nil {
			// 加载期已 fail-fast 校验过 expect；运行期再遇坏值按失败处理。
			output, clipped := summarizeOutput(perr.Error())
			report.Results = append(report.Results, GateResult{
				CheckID: checkID, Cmd: command, Expect: entry.Expect,
				ExitCode: 1, Pass: false, Output: output, OutputTruncated: clipped,
			})
			report.Pass = false
			continue
		}
		timeoutMS := opts.TimeoutMS
		if timeoutMS <= 0 {
			timeoutMS = 300_000
		}
		res := watchdog.Run("/bin/sh", []string{"-c", command}, watchdog.Opts{
			Context: ctx, Cwd: opts.Cwd,
			Env:           childEnv,
			IdleTimeoutMs: timeoutMS, WallTimeoutMs: timeoutMS, KillGraceMs: opts.KillGraceMS,
		})
		if res.Cancelled {
			cancel() // SIGTERM kill switch 取消也阻止后续命令启动。
		}
		exitCode := -1 // 无正常退出码；绝不能把超时/信号/启动失败伪装成 expect exit 1。
		if res.ExitCode != nil {
			exitCode = *res.ExitCode
		}
		interrupted := res.TimedOut || res.Cancelled || res.Truncated || res.Signal != "" || res.SpawnError != "" || res.CleanupError != ""
		complete := !interrupted && res.ExitCode != nil
		pass := complete && exitCode == expected
		digest := ""
		if complete && !pass {
			digest = gateOutputDigest(res.Stdout, res.Stderr)
		}
		output := ""
		if !pass || exitCode != 0 {
			output = res.Stdout + res.Stderr
			if res.SpawnError != "" {
				output += res.SpawnError
			}
			if res.CleanupError != "" {
				output += res.CleanupError
			}
			if res.Truncated {
				output += "\ncommand output truncated"
			}
		}
		output, clipped := summarizeOutput(output)
		report.Results = append(report.Results, GateResult{
			CheckID:             checkID,
			Cmd:                 command,
			Expect:              entry.Expect,
			ExitCode:            exitCode,
			Pass:                pass,
			DurationMS:          res.DurationMS,
			Output:              output,
			OutputDigest:        digest,
			OutputTruncated:     clipped,
			ObservationComplete: complete,
			TimedOut:            res.TimedOut,
			Cancelled:           res.Cancelled,
			Truncated:           res.Truncated,
			Signal:              res.Signal,
			SpawnError:          res.SpawnError,
			CleanupError:        res.CleanupError,
		})
	}
	report.Pass = true
	for _, r := range report.Results {
		if !r.Pass {
			report.Pass = false
		}
	}
	return report
}

// ---- git 只读检查 ----

// gitOut / gitRun 可替换注入点（git 故障分支在真实环境不可稳定复现，
// 测试注入覆盖错误传播）。
var gitOut = func(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	cmd.Env = envSlice(childenv.GateEnv(childenv.OSLookupEnv, nil))
	out, err := cmd.Output()
	return string(out), err
}

var gitRun = func(cwd string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	cmd.Env = envSlice(childenv.GateEnv(childenv.OSLookupEnv, nil))
	return cmd.Run()
}

func identityArgs() []string {
	return []string{"-c", "user.name=flywheel-orchestrator", "-c", "user.email=flywheel-orchestrator@localhost"}
}

// ForeignDirtyFiles 任务目录之外的脏工作区路径（repo-relative）。任务目录是
// 编排器领地、预期会变（契约+账本）；运行起点上其他任何脏文件都是既有 WIP，
// 会被 scoped unit commit 卷入（P1-1 caveat——diff 是"自 base ref 以来的一切"，
// 不是"executor 摸过的一切"）。ok=false 表示不是 git checkout。
// C10（v0.18）：porcelain 的 rename 条目（`R  old -> new`）拆出新旧两路径；
// 含特殊字符的引号路径剥引号并还原常见转义。
func ForeignDirtyFiles(cwd, taskDirPrefix string) (paths []string, ok bool) {
	stdout, err := gitOut(cwd, "status", "--porcelain")
	if err != nil {
		return nil, false
	}
	add := func(f string) {
		if f != "" && !strings.HasPrefix(f, taskDirPrefix) {
			paths = append(paths, f)
		}
	}
	for _, line := range strings.Split(stdout, "\n") {
		if len(line) < 4 {
			continue // XY + 空格 + 至少 1 字符路径
		}
		for _, seg := range strings.SplitN(line[3:], " -> ", 2) {
			add(unescapePorcelain(strings.TrimSpace(seg)))
		}
	}
	return paths, true
}

// unquotePorcelain 剥掉 git 对特殊路径的外层双引号并还原其转义
// （C10：\" \\ \t \n 四种；其余 \x 序列罕见，保留原样不误伤）。
func unescapePorcelain(p string) string {
	if len(p) < 2 || p[0] != '"' || p[len(p)-1] != '"' {
		return p
	}
	inner := p[1 : len(p)-1]
	r := strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\t`, "\t", `\n`, "\n")
	return r.Replace(inner)
}

// ---- per-unit worktree 隔离（§4.2 v0.16）----

// WorktreeNames 单元 worktree 的确定性命名：分支 + checkout 路径。checkout
// 在仓库外的兄弟目录——放进 `<repo>/.git/` 会与 git 自己的 per-worktree 管理
// 目录冲突。
func WorktreeNames(cwd, slug, packetID string) (branch, path string) {
	repoName := filepath.Base(cwd)
	if repoName == "" || repoName == "." || repoName == "/" {
		repoName = "repo"
	}
	branch = fmt.Sprintf("flywheel/%s/%s", slug, packetID)
	path = filepath.Join(filepath.Dir(cwd), ".flywheel-worktrees",
		fmt.Sprintf("%s-%s-%s", repoName, slug, packetID))
	return branch, path
}

// EnsureUnitWorktree 保证单元 worktree 存在于确定性路径，从单元 base ref 创建。
// 已存在（崩溃恢复）则复用——内容保留。非 git checkout 或 worktree 不可用时
// 返回 mode=in-place 与原因。
func EnsureUnitWorktree(cwd, slug, packetID, baseRef string) state.WorktreeInfo {
	branch, path := WorktreeNames(cwd, slug, packetID)
	if _, err := gitOut(path, "rev-parse", "--git-dir"); err == nil {
		return state.WorktreeInfo{Path: path, Branch: branch, Mode: "worktree"} // 已存在：resume 复用
	}
	if err := gitRun(cwd, "worktree", "add", "-f", "-B", branch, path, baseRef); err != nil {
		return state.WorktreeInfo{Mode: "in-place", Path: path, Branch: branch,
			Reason: fmt.Sprintf("git worktree add failed: %v", err)}
	}
	return state.WorktreeInfo{Path: path, Branch: branch, Mode: "worktree"}
}

// MergeUnitWorktree 把单元分支 fast-forward 进主 checkout。绝不 force：
// 主 HEAD 被移动（单元期间用户提交）是上报错误，不是覆盖。
func MergeUnitWorktree(cwd, branch string) (string, error) {
	out, err := gitOut(cwd, "merge", "--ff-only", branch)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			out += string(exitErr.Stderr)
		}
		return "", fmt.Errorf("fast-forward merge %s: %s: %w", branch, strings.TrimSpace(out), err)
	}
	return strings.TrimSpace(out), nil
}

// RemoveUnitWorktree 清理已合并且干净的单元 worktree 与分支。拒绝 force，
// 任何错误都保留尚未清理的资源并返回；已清理的资源可幂等重试。
func RemoveUnitWorktree(cwd, branch, path string) error {
	refs, err := gitOut(cwd, "for-each-ref", "--format=%(refname)", "refs/heads/"+branch)
	if err != nil {
		return fmt.Errorf("inspect unit branch: %w", err)
	}
	branchExists := false
	for _, ref := range strings.Fields(refs) {
		if ref == "refs/heads/"+branch {
			branchExists = true
		}
	}
	worktrees, err := gitOut(cwd, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return fmt.Errorf("inspect unit worktree: %w", err)
	}
	canonicalPath := path
	if parent, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		canonicalPath = filepath.Join(parent, filepath.Base(path))
	}
	registered := false
	for _, record := range strings.Split(worktrees, "\x00\x00") {
		if strings.HasPrefix(record, "worktree "+canonicalPath+"\x00") {
			registered = true
			if !strings.Contains(record+"\x00", "\x00branch refs/heads/"+branch+"\x00") {
				return fmt.Errorf("refusing to remove worktree %s: no longer bound to unit branch %s", path, branch)
			}
		}
	}
	if registered && !branchExists {
		return fmt.Errorf("refusing to remove worktree %s: unit branch %s is missing", path, branch)
	}
	if branchExists {
		if _, err := gitOut(cwd, "merge-base", "--is-ancestor", branch, "HEAD"); err != nil {
			return fmt.Errorf("unit branch %s is not confirmed merged: %w", branch, err)
		}
	}
	if registered {
		if _, err := gitOut(cwd, "worktree", "remove", path); err != nil {
			return fmt.Errorf("remove unit worktree %s: %w", path, err)
		}
	} else if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return fmt.Errorf("refusing to clean unregistered worktree path %s", path)
	}
	if branchExists {
		if _, err := gitOut(cwd, "branch", "-d", branch); err != nil {
			return fmt.Errorf("delete unit branch %s: %w", branch, err)
		}
	}
	return nil
}

// GetBaseRef 当前单元的 base ref：git checkout 用 HEAD（零提交仓库用空树）；
// 非 git repo 返回 ok=false。编排器在 executor 运行前记录它，边界判定一律对
// 它 diff——executor 中途自行提交也无法静默抽走 diff（§4.2/§4.4）。
func GetBaseRef(cwd string) (ref string, ok bool) {
	if _, err := gitOut(cwd, "rev-parse", "--git-dir"); err != nil {
		return "", false // 不是 git checkout
	}
	head, err := gitOut(cwd, "rev-parse", "HEAD")
	if err != nil {
		return EmptyTree, true
	}
	return strings.TrimSpace(head), true
}

// PersistedHead 确认真实 commit 的 tree 与当前候选一致；提交成功和工作区身份
// 不变各自都不足够：pre-commit 可以只篡改临时 index 而不改工作区。
// 账本由调用方显式排除。任何扫描错误或未入库源码都不能用于 completed。
func PersistedHead(cwd string, exclude []string) (string, error) {
	head, err := gitOut(cwd, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", fmt.Errorf("read persisted HEAD commit: %w", err)
	}
	pathspec := []string{"--", "."}
	for _, path := range exclude {
		pathspec = append(pathspec, ":(exclude,literal)"+path)
	}
	if err := checkGitSemantics(cwd, pathspec); err != nil {
		return "", err
	}
	// User Git settings may hide executable-bit or symlink-type differences.
	// Override them only for this read-only comparison: the persisted tree must
	// match the actual candidate tested, without rewriting repository settings.
	args := append([]string{"-c", "core.filemode=true", "-c", "core.symlinks=true", "diff", "--no-ext-diff", "--no-textconv", "--ignore-submodules=none", "--name-only", "-z", "HEAD"}, pathspec...)
	changed, err := gitOut(cwd, args...)
	if err != nil {
		return "", fmt.Errorf("compare persisted HEAD with candidate: %w", err)
	}
	if changed != "" {
		return "", fmt.Errorf("persisted HEAD differs from verified candidate: %q", strings.Split(strings.TrimSuffix(changed, "\x00"), "\x00"))
	}
	args = append([]string{"ls-files", "--others", "--exclude-standard", "-z"}, pathspec...)
	untracked, err := gitOut(cwd, args...)
	if err != nil {
		return "", fmt.Errorf("inspect uncommitted candidate files: %w", err)
	}
	if untracked != "" {
		return "", fmt.Errorf("candidate files were not persisted: %q", strings.Split(strings.TrimSuffix(untracked, "\x00"), "\x00"))
	}
	return strings.TrimSpace(head), nil
}

// AheadCount 分支 head 相对 base 领先的提交数（base..head）。编排器据此判断
// "单元代码是否已落分支"——而不是"本次调用是否产生了新提交"：executor 自提交
// 或崩溃恢复重跑 completeUnit 时，scoped 提交可能为空但分支仍然领先，合并门控
// 必须以领先关系为准。
func AheadCount(repoDir, base, head string) (int, error) {
	out, err := gitOut(repoDir, "rev-list", "--count", base+".."+head)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(out))
}

func GetChangedFiles(cwd, baseRef string) (files []string, ok bool) {
	base := baseRef
	if base == "" {
		base = "HEAD"
	}
	diffOut, err := gitOut(cwd, "diff", "--name-only", base)
	if err != nil {
		return nil, false
	}
	untracked, ok := listUntracked(cwd, nil)
	if !ok {
		return nil, false
	}
	files = []string{} // a successful empty diff is known, unlike an unavailable scan
	set := map[string]bool{}
	for _, line := range strings.Split(diffOut, "\n") {
		if f := strings.TrimSpace(line); f != "" {
			set[f] = true
		}
	}
	for _, f := range untracked {
		set[f] = true
	}
	for f := range set {
		files = append(files, f)
	}
	sort.Strings(files)
	return files, true
}

// listUntracked untracked 文件清单（-z NUL 分隔：文件名含空格/引号/非 ASCII
// 时不会被 core.quotePath 的引号化破坏——PR#3 review must-fix 4）。
// exclude 为 repo-relative pathspec（调用方拼好 :(exclude) 前缀）。
func listUntracked(cwd string, exclude []string) ([]string, bool) {
	args := []string{"ls-files", "--others", "--exclude-standard", "-z", "--", "."}
	for _, p := range exclude {
		args = append(args, ":(exclude)"+p)
	}
	out, err := gitOut(cwd, args...)
	if err != nil {
		return nil, false
	}
	var files []string
	for _, f := range strings.Split(out, "\x00") {
		if f = strings.TrimSpace(f); f != "" {
			files = append(files, f)
		}
	}
	sort.Strings(files)
	return files, true
}

// GetDiffText 对 baseRef 的完整 unified diff 文本（§4.3 向评审者承诺的是
// git diff，不是文件清单）。~100KB 截断加标记。exclude pathspec（repo-relative）
// 把编排器账本文件从 diff 里剔除；契约文件（spec/packets）保留在内。
// v0.18（C2）：git diff 天然不含 untracked 文件——逐个追加合成 new-file 段
// （全文），否则 greenfield 单元（新建文件为主）评审者只看到文件名看不到内容。
// ok=false = 不可用的 git checkout。
func GetDiffText(cwd, baseRef string, exclude []string) (text string, ok bool) {
	base := baseRef
	if base == "" {
		base = "HEAD"
	}
	args := []string{"diff", base, "--", "."}
	for _, p := range exclude {
		args = append(args, ":(exclude)"+p)
	}
	out, err := gitOut(cwd, args...)
	if err != nil {
		return "", false
	}
	var b strings.Builder
	b.WriteString(out)
	if files, ok2 := listUntracked(cwd, exclude); ok2 {
		appendUntrackedSections(&b, cwd, files)
	}
	text = b.String()
	if len(text) > DiffTextCap {
		return text[:DiffTextCap] + fmt.Sprintf("\n... [diff truncated at %d chars]", DiffTextCap), true
	}
	return text, true
}

// perFileContentCap 单个 untracked 文件正文进入评审 diff 的上限（PR#3 review
// must-fix 4：无上限时 GB 级文件会造成内存尖峰）。总量仍受 DiffTextCap 约束。
const perFileContentCap = 256 << 10 // 256KB

// appendUntrackedSections 把 untracked 文件清单逐个渲染为合成 new-file 段
// （v0.18 C2，v0.19 加固）：
//   - Lstat 先行：符号链接只渲染 target 路径、绝不读内容——防 /dev/zero 挂死
//     与凭证文件外泄（executor 在 worktree 里 ln -s 的攻击面）；
//   - 非 regular 文件占位；空文件零行段；
//   - 二进制以占位标记代替内容；总量截断由调用方统一处理。
//
// readDiffFile 允许确定性验证 Lstat 后文件被删除/变得不可读的竞态。
var readDiffFile = os.ReadFile

func appendUntrackedSections(b *strings.Builder, cwd string, files []string) {
	for _, f := range files {
		if f == "" {
			continue
		}
		full := filepath.Join(cwd, filepath.FromSlash(f))
		fi, err := os.Lstat(full) // Lstat：不跟随符号链接
		if err != nil {
			continue // 竞态：文件刚被删 → 跳过
		}
		fmt.Fprintf(b, "diff --git a/%s b/%s\nnew file mode %o\n--- /dev/null\n+++ b/%s\n",
			f, f, fi.Mode().Perm(), f)
		if !fi.Mode().IsRegular() {
			if linkTarget, lerr := os.Readlink(full); lerr == nil {
				fmt.Fprintf(b, "[symlink -> %s]\n", linkTarget)
			} else {
				b.WriteString("[non-regular file]\n")
			}
			continue
		}
		if fi.Size() == 0 {
			b.WriteString("@@ -0,0 +0,0 @@\n")
			continue
		}
		if fi.Size() > perFileContentCap {
			fmt.Fprintf(b, "@@ -0,0 +1,1 @@\n+[file too large: %d bytes]\n", fi.Size())
			continue
		}
		raw, rerr := readDiffFile(full)
		if rerr != nil {
			continue
		}
		head := raw
		if len(head) > 8000 {
			head = head[:8000]
		}
		if bytes.IndexByte(head, 0) >= 0 {
			b.WriteString("[binary file]\n")
			continue
		}
		content := string(raw)
		noNL := !strings.HasSuffix(content, "\n")
		if noNL {
			content += "\n"
		}
		fmt.Fprintf(b, "@@ -0,0 +1,%d @@\n", strings.Count(content, "\n"))
		for _, line := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
			b.WriteByte('+')
			b.WriteString(line)
			b.WriteByte('\n')
		}
		if noNL {
			b.WriteString("\\ No newline at end of file\n")
		}
	}
}

// ---- 编排器侧提交（§4.2：提交权在编排器，永不在 executor）----

// CommitOutcome 提交结果（bookkeeping 失败上报而不抛出——不能毁掉已验证的工作）。
type CommitOutcome struct {
	SHA     string `json:"sha,omitempty"`
	Skipped string `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	At      string `json:"at"`
}

// CommitUnitChanges 单元粒度提交：只提交当前 HEAD 尚未持久化的工作区变更，
// 减去 excludePaths（易变运行时文件）；restrictToPrefix 再收窄到单目录。
// baseRef 保留调用兼容；它用于上层的单元边界/合并判断，不用于重复提交
// executor 已自提交的文件。--only 保留其它路径的 staged 状态，永不 add -A。
func CommitUnitChanges(cwd string, packet *contract.Packet, excludePaths []string, baseRef, restrictToPrefix string) CommitOutcome {
	id := packet.PacketID
	milestone, mErr := contract.MilestoneOfPacket(id)
	goal := strings.Join(strings.Fields(packet.Goal), " ")
	if len(goal) > 50 {
		goal = goal[:50]
	}
	message := strings.TrimSpace(fmt.Sprintf("feat(%s): %s %s", milestone, id, goal))
	out := CommitOutcome{Message: message, At: state.NowISO()}
	if mErr != nil {
		out.Error = mErr.Error()
		return out
	}
	base, ok := GetBaseRef(cwd)
	if !ok {
		out.Skipped = "not a git checkout"
		return out
	}
	changed, ok := GetChangedFiles(cwd, base)
	if !ok {
		out.Error = "changed-file scan failed"
		return out
	}
	var addList []string
	for _, f := range changed {
		if excludedCommitPath(f, excludePaths) {
			continue
		}
		if restrictToPrefix != "" && !strings.HasPrefix(f, restrictToPrefix) {
			continue
		}
		addList = append(addList, f)
	}
	if len(addList) == 0 {
		out.Skipped = "no changes"
		return out
	}
	// intent-to-add 让 --only 识别新文件，同时不重写已有文件的 partial staging。
	addArgs := append(identityArgs(), "add", "--intent-to-add", "--")
	if err := gitRun(cwd, append(addArgs, addList...)...); err != nil {
		out.Error = err.Error()
		return out
	}
	commitArgs := append(identityArgs(), "commit", "--only", "-m", message, "--")
	if err := gitRun(cwd, append(commitArgs, addList...)...); err != nil {
		out.Error = err.Error()
		return out
	}
	sha, err := gitOut(cwd, "rev-parse", "HEAD")
	if err != nil {
		out.Error = fmt.Sprintf("read committed HEAD: %v", err)
		return out
	}
	out.SHA = strings.TrimSpace(sha)
	return out
}

// CommitTaskDirChanges 以调用方给定的 message 提交任务目录当前变更。用于
// planner 修订之后——契约层必须保持已提交状态（硬规则：worktree 只见已提交
// 契约；防篡改扫描 diff 的基准必须认识合法修订）。onlyFiles（正则，匹配
// repo-relative 路径）把提交收窄到如仅契约文件——planner 修订提交不得卷入
// 脏账本文件，否则移动 HEAD 会破坏 ff-merge。
func CommitTaskDirChanges(cwd, prefix, message string, excludePaths []string, onlyFiles *regexp.Regexp) CommitOutcome {
	out := CommitOutcome{At: state.NowISO()}
	base, ok := GetBaseRef(cwd)
	if !ok {
		out.Skipped = "not a git checkout"
		return out
	}
	changed, ok := GetChangedFiles(cwd, base)
	if !ok {
		out.Error = "changed-file scan failed"
		return out
	}
	var addList []string
	for _, f := range changed {
		if !strings.HasPrefix(f, prefix) || excludedCommitPath(f, excludePaths) {
			continue
		}
		if onlyFiles != nil {
			inner := strings.TrimPrefix(f, prefix)
			if !onlyFiles.MatchString(inner) && !onlyFiles.MatchString(f) {
				continue
			}
		}
		addList = append(addList, f)
	}
	if len(addList) == 0 {
		out.Skipped = "no changes"
		return out
	}
	addArgs := append(identityArgs(), "add", "--intent-to-add", "--")
	if err := gitRun(cwd, append(addArgs, addList...)...); err != nil {
		out.Error = err.Error()
		return out
	}
	commitArgs := append(identityArgs(), "commit", "--only", "-m", message, "--")
	if err := gitRun(cwd, append(commitArgs, addList...)...); err != nil {
		out.Error = err.Error()
		return out
	}
	sha, err := gitOut(cwd, "rev-parse", "HEAD")
	if err != nil {
		out.Error = fmt.Sprintf("read committed HEAD: %v", err)
		return out
	}
	out.SHA = strings.TrimSpace(sha)
	return out
}

// excludedCommitPath 以 / 结尾的条目保护整个目录，其余条目精确匹配文件。
// worktree 账本副本（reports/ 等）由主 checkout 收集，不能随实现分支提交。
func excludedCommitPath(path string, excludes []string) bool {
	for _, entry := range excludes {
		if path == entry || strings.HasSuffix(entry, "/") && strings.HasPrefix(path, entry) {
			return true
		}
	}
	return false
}

// ---- 边界判定（纯函数）----

// BoundaryViolation 一条边界违规。
type BoundaryViolation struct {
	Type  string `json:"type"` // must-not-touch | test-file | max-files
	Path  string `json:"path,omitempty"`
	Count int    `json:"count,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// BoundaryReport 边界检查结论。
type BoundaryReport struct {
	Pass         bool                `json:"pass"`
	Skipped      bool                `json:"skipped"`
	Reason       string              `json:"reason,omitempty"`
	Violations   []BoundaryViolation `json:"violations"`
	ChangedFiles []string            `json:"changedFiles"`
}

// globToRegExp 只有 * 为通配符，其余正则元字符按路径字面量处理。
func globToRegExp(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range glob {
		switch r {
		case '.', '+', '?', '^', '$', '{', '}', '(', ')', '|', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '*':
			b.WriteString(".*")
		default:
			b.WriteRune(r)
		}
	}
	return regexp.MustCompile(b.String())
}

// CheckBoundaries 对变更文件列表做纯边界检查。protectedTestPatterns 只保护
// 显式指定的验收 oracle；nil/空不限制普通测试。changedFiles 为 nil（diff
// 不可用）时 skipped=true 且放行——skip 记录进证据，不冒充通过理由。
func CheckBoundaries(changedFiles []string, boundaries contract.Boundaries, protectedTestPatterns []string) BoundaryReport {
	testRes := make([]*regexp.Regexp, 0, len(protectedTestPatterns))
	for _, p := range protectedTestPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return BoundaryReport{Pass: false, Reason: "invalid protected test pattern: " + err.Error(),
				Violations: []BoundaryViolation{{Type: "invalid-protection-pattern", Path: p}}, ChangedFiles: changedFiles}
		}
		testRes = append(testRes, re)
	}
	if changedFiles == nil {
		return BoundaryReport{Pass: true, Skipped: true,
			Reason:       "git diff unavailable (not a git checkout)",
			Violations:   []BoundaryViolation{},
			ChangedFiles: []string{}}
	}
	type matcher struct {
		re     *regexp.Regexp
		exact  string
		prefix string
		isRe   bool
	}
	var matchers []matcher
	for _, entry := range boundaries.MustNotTouch {
		if strings.Contains(entry, "*") {
			matchers = append(matchers, matcher{re: globToRegExp(entry), isRe: true})
		} else {
			prefix := entry
			if !strings.HasSuffix(entry, "/") {
				prefix = entry + "/"
			}
			matchers = append(matchers, matcher{exact: entry, prefix: prefix})
		}
	}
	violations := []BoundaryViolation{}
	for _, file := range changedFiles {
		for _, m := range matchers {
			hit := m.isRe && m.re.MatchString(file) ||
				!m.isRe && (file == m.exact || strings.HasPrefix(file, m.prefix))
			if hit {
				violations = append(violations, BoundaryViolation{Type: "must-not-touch", Path: file})
			}
		}
		for _, re := range testRes {
			if re.MatchString(file) {
				violations = append(violations, BoundaryViolation{Type: "test-file", Path: file})
				break // Node some() 语义：每文件至多一条 test-file 违规
			}
		}
	}
	if boundaries.MaxFiles != nil && len(changedFiles) > *boundaries.MaxFiles {
		violations = append(violations, BoundaryViolation{
			Type: "max-files", Count: len(changedFiles), Limit: *boundaries.MaxFiles})
	}
	return BoundaryReport{Pass: len(violations) == 0, Violations: violations, ChangedFiles: changedFiles}
}
