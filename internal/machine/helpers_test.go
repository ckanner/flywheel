package machine

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

const fixtureSpec = `---
spec-version: 1
title: Mini fixture task
goal: Prove the orchestrator loop end to end.
non-goals: [anything real]
milestones:
  - id: M-01
    title: The only milestone
    gate: L1 green for all packets.
requirements:
  - id: R-01
    title: First thing works
    milestone: M-01
    verification: test -f build/M01-T01.txt
  - id: R-02
    title: Second thing works
    milestone: M-01
    verification: test -f build/M01-T02.txt
---

# Mini spec

R-01 and R-02 map to M-01.
`

func fixturePacket(id, requirement string) string {
	return fmt.Sprintf(`---
packet-id: %s
contract-version: 1
goal: Make %s produce its artifact.
requirements: [%s]
route: default
needs-live-credentials: false
context-files: []
cache-scope: []
boundaries:
  must-not-touch: [forbidden/]
  max-files: 5
acceptance:
  L1:
    - {cmd: "test -f build/%s.txt", expect: "exit 0"}
review-rubric: |
  Pass line: score >= 0.8. Artifact exists and is honest.
escalation: {max-retries: 3, on-fail: return-to-planner}
done-report-format:
  requirements-covered: []
  changed-files: []
  commands-run: []
  failures: []
---

Work instructions for %s.
`, id, id, requirement, id, id)
}

const fixtureConfigJSON = `{
  "version": 1,
  "drivers": {"plan": "mock", "execute": "mock", "review": "mock", "judge": "mock", "intake": "mock", "garden": "mock"},
  "loops": {
    "plan": { "harness": "kimi", "model": "kimi-code/k3", "channel": "kimi-subscription", "thinking": "max", "resident-session": true },
    "review": { "harness": "omp", "model": "opencode-go/deepseek-v4-pro", "channel": "opencode-go", "thinking": "max" },
    "judge": { "harness": "omp", "model": "kimi-coding/k3", "channel": "kimi-coding", "thinking": "max" },
    "intake": { "harness": "omp", "model": "opencode-go/deepseek-v4-flash", "channel": "opencode-go", "thinking": "max" },
    "garden": { "harness": "kimi", "model": "kimi-code/k3", "channel": "kimi-subscription", "thinking": "max" }
  },
  "routes": {
    "frontend": { "harness": "kimi", "model": "kimi-code/k3", "channel": "kimi-subscription", "thinking": "max" },
    "default": { "harness": "omp", "model": "opencode-go/deepseek-v4-flash", "channel": "opencode-go", "thinking": "max" },
    "hard": { "harness": "omp", "model": "kimi-coding/k3", "channel": "kimi-coding", "thinking": "max" }
  },
  "channels": {
    "opencode-go": { "env-key": "OPENCODE_API_KEY", "fallback": "opencode" },
    "opencode": {
      "env-key": "OPENCODE_API_KEY",
      "models": { "default": "opencode/deepseek-v4-flash", "hard": "opencode/deepseek-v4-pro", "review": "opencode/deepseek-v4-pro", "judge": "opencode/deepseek-v4-pro" }
    },
    "kimi-subscription": { "env-key": null, "fallback": "kimi-coding" },
    "kimi-coding": {
      "env-key": "KIMI_API_KEY",
      "harness": "omp",
      "models": { "plan": "kimi-coding/k3", "frontend": "kimi-coding/k3", "hard": "kimi-coding/k3", "judge": "kimi-coding/k3" },
      "fallback": "opencode"
    }
  },
  "budget": { "enforce": "soft", "daily-token-limit": 10000000 },
  "watchdog": { "idle-timeout-ms": 5000, "wall-timeout-ms": 30000, "kill-grace-ms": 500 },
  "retry": { "base-delay-ms": 10, "max-delay-ms": 40, "jitter": 0, "max-transport-retries": 2 },
  "credentials": { "poll-interval-ms": 10, "timeout-ms": 200 },
  "defect-injection": { "every-n-units": 5, "test-file-ratio-boost": 0.5 },
  "verify": { "command-timeout-ms": 10000 }
}`

func fixturePacketState() string {
	return `{
  "milestone": "M-01",
  "status": "pending",
  "route": "default",
  "attempts": 0,
  "failed-attempts": 0,
  "plan-revisions": 0,
  "interrupted": false,
  "needs-live-credentials": false,
  "artifacts": [],
  "base-ref": null,
  "commit": null,
  "worktree": null,
  "evidence": { "executor": null, "l1": null, "review": null, "defect-injection": null, "credentials": null },
  "last-failure-signature": null,
  "retry-history": [],
  "updated-at": null
}`
}

func fixtureState(slug string) string {
	return fmt.Sprintf(`{
  "schema-version": 1,
  "slug": %q,
  "revision": 0,
  "status": "pending",
  "current-milestone": "M-01",
  "plan-session": null,
  "milestones": { "M-01": { "status": "pending", "gate": null, "closed-at": null } },
  "packets": {
    "M01-T01": %s,
    "M01-T02": %s
  },
  "budget": {
    "enforce": "soft",
    "daily-token-limit": 10000000,
    "daily": {},
    "totals": { "input": 0, "output": 0, "reasoning": 0, "cache-read": 0, "cache-write": 0, "tokens": 0, "cost-usd": 0 },
    "cache": { "hit-rate": 0, "by-provider": {} },
    "warnings": []
  },
  "last-progress-at": null,
  "inbox": []
}`,
		slug, fixturePacketState(), fixturePacketState())
}

type fixture struct {
	root    string // 假仓库根（cwd）
	taskDir string // docs/<slug>
	slug    string
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// patchConfigFile 用 JSON 合并语义改写 fixture 配置的顶层键（测试局部覆写）。
func patchConfigFile(t *testing.T, fx fixture, override string) {
	t.Helper()
	writeFile(t, filepath.Join(fx.taskDir, "flywheel.config.json"), override)
}

func mustConfig(t *testing.T, fx fixture) *config.Config {
	t.Helper()
	cfg, err := config.Load(fx.taskDir, fx.root)
	require.NoError(t, err)
	return cfg
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	fx := fixture{root: root, taskDir: filepath.Join(root, "docs", "minitask"), slug: "minitask"}
	writeFile(t, filepath.Join(fx.taskDir, "spec.md"), fixtureSpec)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), fixturePacket("M01-T01", "R-01"))
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T02.md"), fixturePacket("M01-T02", "R-02"))
	writeFile(t, filepath.Join(fx.taskDir, "flywheel.config.json"), fixtureConfigJSON)
	writeFile(t, filepath.Join(fx.taskDir, "state.json"), fixtureState(fx.slug))
	return fx
}

func gitCmd(t *testing.T, root string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", root, "-c", "user.name=test", "-c", "user.email=test@localhost"}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return string(out)
}

// newGitFixture 整个 fixture（契约在内）已提交——模拟 $flywheel-new 之后的目标仓库。
func newGitFixture(t *testing.T, extraFiles ...string) fixture {
	t.Helper()
	fx := newFixture(t)
	for _, rel := range extraFiles {
		writeFile(t, filepath.Join(fx.root, rel), "// "+rel+"\n")
	}
	gitCmd(t, fx.root, "init", "-q")
	gitCmd(t, fx.root, "add", "-A")
	gitCmd(t, fx.root, "commit", "-q", "-m", "chore: initial contracts")
	return fx
}

func loadFixtureState(t *testing.T, fx fixture) *state.State {
	t.Helper()
	st, err := state.Load(fx.taskDir)
	require.NoError(t, err)
	return st
}

// ---- 可脚本化的驱动集合 ----

type scriptLog struct {
	mu     sync.Mutex
	plan   []drivers.CallArgs
	exec   []drivers.CallArgs
	review []drivers.CallArgs
	judge  []drivers.CallArgs
}

func (l *scriptLog) record(role string, a drivers.CallArgs) {
	switch role {
	case "plan":
		l.plan = append(l.plan, a)
	case "execute":
		l.exec = append(l.exec, a)
	case "review":
		l.review = append(l.review, a)
	case "judge":
		l.judge = append(l.judge, a)
	}
}

// configFor 无 t 版（供 opts 装配器使用；配置损坏直接炸测试）。
func configFor(fx fixture) *config.Config {
	cfg, err := config.Load(fx.taskDir, fx.root)
	if err != nil {
		panic(err)
	}
	return cfg
}

type fnDriver struct {
	role string
	base drivers.Driver
	fn   func(drivers.CallArgs) drivers.Result
	log  *scriptLog
}

func (d *fnDriver) Name() string { return d.role }

func (d *fnDriver) dispatch(a drivers.CallArgs) drivers.Result {
	d.log.record(d.role, a)
	if d.fn != nil {
		return d.fn(a)
	}
	switch d.role {
	case "plan":
		return d.base.Plan(a)
	case "execute":
		return d.base.Execute(a)
	case "review":
		return d.base.Review(a)
	default:
		return d.base.Judge(a)
	}
}

func (d *fnDriver) Plan(a drivers.CallArgs) drivers.Result    { return d.dispatch(a) }
func (d *fnDriver) Execute(a drivers.CallArgs) drivers.Result { return d.dispatch(a) }
func (d *fnDriver) Review(a drivers.CallArgs) drivers.Result  { return d.dispatch(a) }
func (d *fnDriver) Judge(a drivers.CallArgs) drivers.Result   { return d.dispatch(a) }
func (d *fnDriver) Intake(a drivers.CallArgs) drivers.Result  { return d.base.Intake(a) }
func (d *fnDriver) Garden(a drivers.CallArgs) drivers.Result  { return d.base.Garden(a) }

type scriptedDrivers struct {
	mock *drivers.MockDriver
	log  *scriptLog

	planFn   func(drivers.CallArgs) drivers.Result
	execFn   func(drivers.CallArgs) drivers.Result
	reviewFn func(drivers.CallArgs) drivers.Result
	judgeFn  func(drivers.CallArgs) drivers.Result
}

func newScripted(behaviors any) *scriptedDrivers {
	return &scriptedDrivers{
		mock: drivers.NewMockDriver(behaviors),
		log:  &scriptLog{},
	}
}

func (s *scriptedDrivers) set() *drivers.Drivers {
	return &drivers.Drivers{
		Plan:    &fnDriver{role: "plan", base: s.mock, fn: s.planFn, log: s.log},
		Execute: &fnDriver{role: "execute", base: s.mock, fn: s.execFn, log: s.log},
		Review:  &fnDriver{role: "review", base: s.mock, fn: s.reviewFn, log: s.log},
		Judge:   &fnDriver{role: "judge", base: s.mock, fn: s.judgeFn, log: s.log},
	}
}

// runOpts 标准运行入参：noLog + noSleep（退避延迟全部吞掉，测试瞬时完成）。
func runOpts(fx fixture, drv *drivers.Drivers) RunTaskOpts {
	return RunTaskOpts{
		TaskDir: fx.taskDir,
		Cwd:     fx.root,
		Config:  configFor(fx),
		Drivers: drv,
		Log:     func(string) {},
		Sleep:   func(time.Duration) {},
	}
}

// stepOpts RunSingleStepOpts 版装配（同 runOpts 的 noLog/noSleep 语义）。
func stepOpts(fx fixture, drv *drivers.Drivers) RunSingleStepOpts {
	return RunSingleStepOpts{RunOpts: runOpts(fx, drv).RunOpts}
}

// runOptsWithLog 同 runOpts，但收集日志供断言。
func runOptsWithLog(fx fixture, drv *drivers.Drivers, sink *[]string) RunTaskOpts {
	o := runOpts(fx, drv)
	o.Log = func(msg string) { *sink = append(*sink, msg) }
	return o
}

// mustJSON 测试便捷：map → JSON 字节。
func mustJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}
