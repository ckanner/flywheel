package diagnostic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/watchdog"
)

type Check struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Code    string `json:"code,omitempty"`
	Remedy  string `json:"remedy,omitempty"`
}

type Report struct {
	Kind            string         `json:"kind"`
	OK              bool           `json:"ok"`
	Cwd             string         `json:"cwd"`
	TaskDir         string         `json:"taskDir"`
	ProbeRequested  bool           `json:"probeRequested"`
	InferenceTested bool           `json:"inferenceTested"`
	Configuration   *Configuration `json:"configuration,omitempty"`
	Checks          []Check        `json:"checks"`
}

var (
	lookPath       = exec.LookPath
	runCommand     = watchdog.Run
	probeDSH       = drivers.RunDSH
	readDirectory  = os.ReadDir
	doctorBudget   = 40 * time.Second
	commandTimeout = int64(10_000)
	versionPattern = regexp.MustCompile(`\d{1,6}\.\d{1,6}(?:\.\d{1,6})?(?:-[A-Za-z0-9.]{1,48})?`)
)

func (r *Report) add(id, status, summary, code, remedy string) {
	r.Checks = append(r.Checks, Check{ID: id, Status: status, Summary: summary, Code: code, Remedy: remedy})
	if status == "fail" {
		r.OK = false
	}
}

// Doctor performs local checks, and optionally SDK initialization/sandbox checks.
// It never calls a model, runs an acceptance command, or creates task state.
func Doctor(parent context.Context, cwd, taskDir string, probe bool) Report {
	ctx, cancel := context.WithTimeout(parent, doctorBudget)
	defer cancel()
	r := Report{Kind: "doctor", OK: true, Cwd: cwd, TaskDir: taskDir, ProbeRequested: probe, Checks: []Check{}}
	view, cfg, err := inspect(ctx, cwd, taskDir)
	if err != nil {
		if ctx.Err() != nil {
			r.add("deadline", "fail", "预检已取消或超过整体时间预算", "TIMEOUT", "检查命令是否挂起，修复后重新运行")
		} else {
			r.add("configuration", "fail", "无法解析有效配置或目录", "INVALID_REQUEST", "检查所选配置的 JSON 字段、profile 引用与目录；不会切换到低优先级配置")
		}
		return r
	}
	r.Cwd, r.TaskDir, r.Configuration = view.Cwd, view.TaskDir, view
	r.add("configuration", "pass", "已解析配置："+view.Source, "", "")
	required := r.taskScope(cfg)
	for i := range view.Bindings {
		view.Bindings[i].InScope = required == nil || required[view.Bindings[i].ID]
	}
	commands := map[string]bool{}
	if r.command(ctx, "git", "git", commands, true) {
		result := runCommand("git", []string{"-C", r.Cwd, "rev-parse", "--is-inside-work-tree"}, commandOptions(ctx, r.Cwd))
		if cleanCommand(result) && strings.TrimSpace(result.Stdout) == "true" {
			r.add("workspace.git", "pass", "工作目录属于 Git 仓库", "", "")
		} else {
			status := "warn"
			if r.TaskDir != "" {
				status = "fail"
			}
			r.add("workspace.git", status, "工作目录不是可用 Git 仓库，无法完成代码提交交付", "INVALID_REQUEST", "在目标 Git 仓库内运行；预检不会自动初始化或提交")
		}
	}
	// An invalid task cannot establish which agents may run. Complete local
	// diagnostics, but do not initialize any SDK until its prerequisites hold.
	probeAllowed := r.OK
	probes := map[string]bool{}
	for i := range view.Bindings {
		b := &view.Bindings[i]
		if !b.InScope {
			r.add(b.ID, "skipped", "此绑定不属于当前任务的必需阶段或路由", "", "")
			continue
		}
		if ctx.Err() != nil {
			break
		}
		role := b.Role
		if role == "judge-panel" {
			role = "judge"
		}
		if cfg.Drivers[role] == "mock" {
			r.add(b.ID, "skipped", "使用 mock driver；未检查真实模型通道", "", "")
			continue
		}
		if b.PermissionMode == "read-only" && b.Harness != "dsh" && b.Harness != "codex" && b.Harness != "codex-pro" {
			r.add(b.ID+".permission", "fail", "所选 harness 无法实施本阶段要求的只读权限", "INVALID_REQUEST", "为评审选择支持 read-only 的 DSH 或 Codex 路径")
			continue
		}
		ready := r.credentials(cfg, *b)
		if b.Harness == "dsh" {
			ready = r.command(ctx, "dsh-worker", drivers.DSHCommand(), commands, false) && ready
			ready = r.command(ctx, "node", "node", commands, true) && ready
			binary := os.Getenv("FLYWHEEL_DSH_BIN")
			if binary == "" {
				binary = "dsh"
			}
			ready = r.command(ctx, "dsh", binary, commands, true) && ready
			if b.Provider == "" || b.Model == "" {
				r.add(b.ID+".binding", "fail", "DSH 必须明确 provider 和 model", "INVALID_REQUEST", "修正 profile 或阶段绑定后重新检查")
				ready = false
			}
			if probe && ready && probeAllowed {
				r.probe(ctx, cfg, *b, probes)
			} else {
				r.add(b.ID+".sdk", "skipped", "未运行 SDK 探测；必须显式请求且本地配置、契约和工作目录检查通过", "", "修复本地问题后使用 --probe；探测不会发送推理请求")
			}
		} else if b.Harness == "codex-pro" {
			r.codexPro(ctx, *b, commands)
		} else {
			r.harnessCommand(ctx, *b, commands)
			if b.Harness != "codex" {
				r.add(b.ID+".sandbox", "warn", "此 CLI 路径没有可由 Flywheel 证明的 OS 工作区沙箱", "", "使用可信项目；worktree 只隔离变更，不是安全沙箱")
			}
		}
		r.add(b.ID+".authentication", "unknown", "配置渠道名称不证明实际账号；授权、额度和模型服务能力未验证", "", "按工具正常登录流程配置，再运行经确认的小型真实任务")
	}
	if ctx.Err() != nil {
		r.add("deadline", "fail", "预检已取消或超过整体时间预算", "TIMEOUT", "检查命令是否挂起，修复后重新运行")
	}
	return r
}

// taskScope checks contracts without executing their commands. No packets is a
// valid pre-planning state: check the default route until Plan creates packets.
func (r *Report) taskScope(cfg *config.Config) map[string]bool {
	if r.TaskDir == "" {
		r.add("contracts", "skipped", "未指定任务；检查全部配置绑定", "", "")
		return nil
	}
	required := map[string]bool{"loops.plan": true, "loops.review": true, "loops.judge": true}
	if cfg.JudgePanel != nil {
		required["judge-panel"] = true
	}
	spec, err := contract.LoadSpec(r.TaskDir)
	if err == nil {
		entries, readErr := readDirectory(r.TaskDir)
		err = readErr
		hasPackets := false
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "task-packet-") && strings.HasSuffix(entry.Name(), ".md") {
				hasPackets = true
			}
		}
		if err == nil && hasPackets {
			packets, packetErr := contract.LoadPackets(r.TaskDir, spec)
			err = packetErr
			for _, packet := range packets {
				required["routes."+packet.Route] = true
			}
		} else if err == nil {
			required["routes.default"] = true
			r.add("contracts.packets", "warn", "尚无 packet；预检默认执行路由，规划后再次检查", "", "")
		}
	}
	if err != nil {
		r.add("contracts", "fail", "任务契约无法加载或未通过 schema 校验", "INVALID_REQUEST", "检查 spec.md 与 task-packet 文件；预检不会运行其中的命令")
	} else {
		r.add("contracts", "pass", "任务契约结构已验证", "", "")
	}
	present := map[string]bool{}
	for _, b := range r.Configuration.Bindings {
		present[b.ID] = true
	}
	ids := make([]string, 0, len(required))
	for id := range required {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if !present[id] {
			r.add(id, "fail", "缺少任务所需的模型绑定", "INVALID_REQUEST", "在 loops 或 routes 中补齐此绑定")
		}
	}
	return required
}

func (r *Report) credentials(cfg *config.Config, b Binding) bool {
	key := fallback.EnvKeyForChannel(cfg, b.Channel)
	if key == "" {
		return true
	}
	if fallback.MissingChannelEnv(cfg, b.Channel, childenv.OSLookupEnv) != "" {
		r.add(b.ID+".credential", "fail", "缺少声明的环境变量："+key, "AUTH", "在启动引擎的环境中配置该变量；不会使用其他账号补位")
		return false
	}
	r.add(b.ID+".credential", "pass", "已提供声明的环境变量："+key+"（未验证有效性）", "", "")
	return true
}

func commandOptions(ctx context.Context, cwd string) watchdog.Opts {
	return watchdog.Opts{Context: ctx, Cwd: cwd, Env: childenv.EnvSlice(childenv.GateEnv(childenv.OSLookupEnv, nil)),
		IdleTimeoutMs: commandTimeout, WallTimeoutMs: commandTimeout, KillGraceMs: 5_000}
}

func cleanCommand(r watchdog.Result) bool {
	return r.ExitCode != nil && *r.ExitCode == 0 && !r.Cancelled && !r.TimedOut && !r.Truncated &&
		r.Signal == "" && r.SpawnError == "" && r.CleanupError == ""
}

func (r *Report) command(ctx context.Context, id, binary string, cache map[string]bool, version bool, prefix ...string) bool {
	if ctx.Err() != nil {
		return false
	}
	// Existence alone cannot satisfy a version/capability check, and `node
	// runtime.cjs --version` is a different command from `node --version`.
	key := fmt.Sprintf("%q|%q|%t|%q", binary, prefix, version, id)
	if ok, seen := cache[key]; seen {
		return ok
	}
	cache[key] = false
	path, err := lookPath(binary)
	if err != nil {
		r.add("binary."+id, "fail", "找不到可执行文件："+binary, "INVALID_REQUEST", "安装对应工具，或修正桌面启动环境的 PATH／显式可执行路径")
		return false
	}
	versionSummary := ""
	if version {
		result := runCommand(path, append(append([]string{}, prefix...), "--version"), commandOptions(ctx, r.Cwd))
		versionText := versionPattern.FindString(result.Stdout)
		if !cleanCommand(result) || versionText == "" {
			r.add("binary."+id, "fail", "命令无法可靠返回版本："+path, "TRANSPORT", "单独检查 --version、执行权限和工具安装；取消或清理失败不会判通过")
			return false
		}
		if id == "node" {
			parts := strings.Split(strings.SplitN(versionText, "-", 2)[0], ".")
			major, _ := strconv.Atoi(parts[0]) // regexp admits bounded decimal components
			minor, _ := strconv.Atoi(parts[1])
			if major < 22 || (major == 22 && minor < 18) {
				r.add("binary.node", "fail", "Node.js 版本低于 worker 要求的 22.18.0", "INVALID_REQUEST", "安装 Node.js 22.18.0 或更新版本，并更新桌面启动环境的 PATH")
				return false
			}
		}
		// Only numeric version components are safe to repeat. Arbitrary
		// prerelease labels and other raw tool output may contain credentials.
		versionSummary = "（版本 " + strings.SplitN(versionText, "-", 2)[0] + "）"
	}
	cache[key] = true
	r.add("binary."+id, "pass", "本地命令可用："+path+versionSummary, "", "")
	return true
}

// Codex Pro's public machine API has no inference-free same-turn configuration
// probe. Only import/version availability is checked here; the real invocation
// must validate its own receipt, including the complete post-run policy.
func (r *Report) codexPro(ctx context.Context, b Binding, cache map[string]bool) {
	worker := drivers.CodexProCommand()
	ready := r.command(ctx, "codex-pro-worker", worker, cache, false)
	ready = r.command(ctx, "node", "node", cache, true) && ready
	r.command(ctx, "codex", drivers.CodexProBinary(), cache, true)
	if ready && ctx.Err() == nil {
		key := "codex-pro-import:" + worker
		if _, checked := cache[key]; !checked {
			result := runCommand(worker, []string{"--doctor"}, commandOptions(ctx, r.Cwd))
			var probe struct {
				Protocol        int    `json:"protocol"`
				Type            string `json:"type"`
				Package         string `json:"package"`
				Version         string `json:"version"`
				MachineProtocol int    `json:"machineProtocol"`
				Available       bool   `json:"available"`
				InferenceTested *bool  `json:"inferenceTested"`
			}
			ok := cleanCommand(result) && json.Unmarshal([]byte(result.Stdout), &probe) == nil &&
				probe.Protocol == 1 && probe.Type == "doctor" && probe.Package == "dsh-subagent-codex-pro" &&
				probe.Version == drivers.CodexProPackageVersion && probe.MachineProtocol == 1 &&
				probe.Available && probe.InferenceTested != nil && !*probe.InferenceTested
			cache[key] = ok
			if ok {
				r.add("codex-pro.api", "pass", "已导入 dsh-subagent-codex-pro/machine "+drivers.CodexProPackageVersion+"（协议 1，未推理）", "", "")
			} else {
				r.add("codex-pro.api", "fail", "Codex Pro 公开入口、精确版本或无推理检查未通过", "INVALID_REQUEST", "重新安装当前 Flywheel npm 包并检查 worker 入口；不会自动改走旧 Codex CLI")
			}
		}
	}
	r.add(b.ID+".sandbox", "warn", "插件运行前核对沙箱模式；完整策略仅在结果返回后核对，Codex 配置仍可能扩展目录或网络权限", "", "核对本机 Codex 配置；需要运行前完整策略限制的环境须等待上游公开 guard，不要将事后拒绝视为访问防护")
	r.add(b.ID+".configuration", "unknown", "实际模型、完整权限和 effort 接受回执由正式调用验证；本次未启动 Codex 模型回合", "", "")
}

// Match the real driver's entry point, including the legacy ZCode app runtime.
// Version checks never open a desktop workspace or start an app-server turn.
func (r *Report) harnessCommand(ctx context.Context, b Binding, cache map[string]bool) {
	binary := b.Harness
	if b.Harness == "kimi" {
		if configured := os.Getenv(drivers.KimiCommandEnv); configured != "" {
			binary = configured
		}
	}
	if b.Harness == "zcode" {
		if configured := os.Getenv(drivers.ZcodeServerBinEnv); configured != "" {
			r.command(ctx, "zcode", configured, cache, true)
		} else if r.command(ctx, "node", "node", cache, true) {
			r.command(ctx, "zcode-runtime", "node", cache, true, "/Applications/ZCode.app/Contents/Resources/glm/zcode.cjs")
		}
		return
	}
	r.command(ctx, b.Harness, binary, cache, true)
}

func (r *Report) probe(ctx context.Context, cfg *config.Config, b Binding, cache map[string]bool) {
	if ctx.Err() != nil {
		return
	}
	effort := b.Thinking
	if effort == "" {
		effort = "high"
	}
	key := fmt.Sprintf("%q|%q|%q|%q|%q|%q", r.Cwd, b.Provider, b.Channel, b.Model, effort, b.PermissionMode)
	if _, checked := cache[key]; checked {
		r.add(b.ID+".sdk", "skipped", "相同配置渠道、provider、模型、effort、目录与权限已检查；不证明实际账号身份", "", "")
		return
	}
	cache[key] = true
	keys := map[string]string{}
	if key := fallback.EnvKeyForChannel(cfg, b.Channel); key != "" {
		keys[key] = os.Getenv(key)
	}
	result := probeDSH(drivers.DSHArgs{Doctor: true, Provider: b.Provider, Model: b.Model, Thinking: effort,
		PermissionMode: b.PermissionMode, Cwd: r.Cwd, APIKeys: keys, Watchdog: commandOptions(ctx, r.Cwd)})
	if result.Settled && result.ExitCode != nil && *result.ExitCode == 0 && !result.Cancelled && !result.TimedOut &&
		!result.Truncated && result.Signal == "" && result.SpawnError == "" && result.CleanupError == "" && result.ErrorMessage == nil &&
		result.Usage == nil && result.CostUSD == nil {
		r.add(b.ID+".sdk", "pass", "adapter 接受配置；目录、"+b.PermissionMode+" 沙箱与 SDK 正常关闭已确认", "", "")
	} else {
		code := errs.INVALID_REQUEST
		switch {
		case result.TimedOut:
			code = errs.TIMEOUT
		case result.Cancelled || result.Truncated || result.Signal != "" || result.SpawnError != "" || result.CleanupError != "":
			code = errs.TRANSPORT
		case result.ErrorMessage != nil:
			code = errs.ClassifyError(errs.SpawnResult{ExitCode: 1, Stderr: *result.ErrorMessage})
		}
		r.add(b.ID+".sdk", "fail", "SDK 配置、完整权限或进程收尾未得到确认", string(code), "核对 SDK profile、provider/model/effort、DSH 版本和本机沙箱；不要通过 full access 绕过")
	}
}
