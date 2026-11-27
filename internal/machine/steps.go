package machine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// ---- 小工具 ----

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// toSpawnResult driver 结果 → 错误分类输入（Node 直接传 result 对象等价）。
func toSpawnResult(r drivers.Result) errs.SpawnResult {
	sr := errs.SpawnResult{Signal: r.Signal, TimedOut: r.TimedOut, Stderr: r.Stderr, Stdout: r.Stdout, ErrorCode: r.ErrorCode}
	if r.ExitCode != nil {
		sr.ExitCode = *r.ExitCode
	}
	if r.SpawnError != "" {
		sr.SpawnError = errors.New(r.SpawnError)
	}
	if r.CleanupError != "" {
		sr.SpawnError = errors.New(r.CleanupError)
	}
	if r.Cancelled {
		sr.Signal = "cancelled"
	}
	if sr.SpawnError == nil && (r.ExitCode == nil || r.Truncated || (!r.Settled && r.ErrorCode == "") ||
		(r.ErrorCode != "" && *r.ExitCode != 1)) {
		sr.SpawnError = errors.New("harness did not reach a confirmed terminal result")
	}
	// Antigravity's validated success receipt contains diagnostic metadata such
	// as extraBilling:unknown and may discuss quota errors in its answer. Keep
	// that wire evidence in Result, but do not classify it as process stderr.
	if r.Harness == "antigravity" && len(r.AntigravityReceipt) != 0 && r.Settled &&
		sr.SpawnError == nil && sr.ExitCode == 0 && sr.Signal == "" && !sr.TimedOut && sr.ErrorCode == "" &&
		(r.ErrorMessage == nil || *r.ErrorMessage == "") && r.Session != nil &&
		r.Session.Dispatch == "dispatched" && r.Session.Outcome == "completed" && r.Session.CleanupConfirmed {
		sr.Stdout = ""
	}
	return sr
}

func (c *Context) wdogFor(role string) watchdog.Opts {
	idle, wall := c.Config.Watchdog.IdleTimeoutMs, c.Config.Watchdog.WallTimeoutMs
	if override, ok := c.Config.Watchdog.Roles[role]; ok && role != "" {
		if override.IdleTimeoutMs > 0 {
			idle = override.IdleTimeoutMs
		}
		if override.WallTimeoutMs > 0 {
			wall = override.WallTimeoutMs
		}
	}
	return watchdog.Opts{
		Context:       c.RunContext,
		IdleTimeoutMs: idle,
		WallTimeoutMs: wall,
		KillGraceMs:   c.Config.Watchdog.KillGraceMs,
	}
}

// appendFile Node appendFileSync 等价（不存在则创建）。
func appendFile(path, content string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(content)
}

func (c *Context) appendInbox(entry map[string]any) {
	who := entry["packet-id"]
	if who == nil {
		who = "task"
	}
	appendFile(filepath.Join(c.TaskDir, "inbox.md"),
		fmt.Sprintf("\n## %s — %s\n\n%s\n", entry["at"], who, entry["reason"]))
}

func (c *Context) appendRetro(line string) {
	appendFile(filepath.Join(c.TaskDir, "retro.md"), line+"\n")
}

func (c *Context) baseRefOf(ps *state.PacketState) string {
	if ps.BaseRef == nil {
		return ""
	}
	return *ps.BaseRef
}

// orHEAD Node `ps['base-ref'] ?? 'HEAD'` 的展示回退。
func orHEAD(ref string) string {
	if ref == "" {
		return "HEAD"
	}
	return ref
}

// ---- 凭证人工确认（§5.1 needs-live-credentials / §9）----

type credsApproval struct {
	OK       bool
	Injected map[string]string
}

// ---- Execute 步骤（传输重试 + 渠道降级 §6）----

type execOutcome struct {
	Relocated bool
	Success   bool
	Stopped   bool
	Awaiting  string // "credentials"：等人批凭证，非失败
	ErrorCode errs.ErrorCode
}

// ---- L1 步骤（§4.4）----

type l1Outcome struct {
	Pass     bool
	Gate     verify.GateReport
	Boundary verify.BoundaryReport
}

// ---- LLM 裁决循环（review / judge 共用）----

type verdictArgs struct {
	sessionRole  string
	sessionScope string
	role         string // "review" | "judge"
	cwd          string // review 必须读取候选 worktree；judge 使用主 checkout
	binding      fallback.Binding
	prompt       string
	packetID     string
	attemptNo    *int
	ps           *state.PacketState // 可空——里程碑级 judge 时重试只进 metrics
	finalSpec    *contract.Spec     // Quick's stronger task-final verdict schema
}

type verdictOutcome struct {
	OK        bool
	Verdict   *prompts.Verdict
	Stopped   bool
	ErrorCode errs.ErrorCode
}

// ---- Review 步骤（§4.3）----

type reviewOutcome struct {
	Pass         bool
	Stopped      bool
	Converging   bool
	OpenFindings int
	Verdict      *prompts.Verdict
	ErrorCode    errs.ErrorCode
}

// ---- Judge 步骤（§7 里程碑终验）----

type judgeOutcome struct {
	Pass      bool
	Stopped   bool
	Split     bool // v0.18 A3：面板分歧 → 升级 inbox 由人裁决
	Verdict   *prompts.Verdict
	ErrorCode errs.ErrorCode
}

// ---- 里程碑闸门（§4.4）----

type gateOutcome struct {
	Pass   bool
	Levels map[string]any
}

// ---- Plan 步骤（§4.1）----

type planOutcome struct {
	OK        bool
	Stopped   bool
	ErrorCode errs.ErrorCode
	Packets   []*contract.Packet
	Result    *drivers.Result
}
