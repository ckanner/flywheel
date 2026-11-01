package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/drivers"
)

// happyDrivers 全绿脚本化驱动：mock 基底（execute 写交付物、review/judge 返
// pass verdict），plan 用 mock 默认 issue 动作。
func happyDrivers() *scriptedDrivers {
	return newScripted(nil)
}

func TestRunTaskHappyPathEndToEnd(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)

	st := loadFixtureState(t, fx)
	assert.Equal(t, "completed", st.Status, "inbox: %#v", st.Inbox)
	assert.Nil(t, st.CurrentMilestone)
	assert.Equal(t, "closed", st.Milestones["M-01"].Status)
	require.NotNil(t, st.Milestones["M-01"].ClosedAt)
	for _, id := range []string{"M01-T01", "M01-T02"} {
		ps := st.Packets[id]
		assert.Equal(t, "completed", ps.Status, id)
		assert.NotZero(t, ps.Attempts, id)
		require.NotNil(t, ps.Commit, "%s 应有单元提交记录（P2：先 commit 再 completed）", id)
		// Node 语义：完成时删除 worktree 目录但保留引用，下次使用时才发现
		// 已删并回退主 checkout（machine.js:395）。断言目录确已清理。
		if ps.Worktree != nil {
			assert.NoDirExists(t, ps.Worktree.Path, "%s 的 worktree 目录应已删除", id)
		}
	}
	// 摘要字段。
	assert.Equal(t, "minitask", summary.Slug)
	assert.Equal(t, "completed", summary.Status)
	assert.Equal(t, "2/2", summary.PacketsCompleted)
	assert.Greater(t, summary.Tokens, int64(0), "mock usage 记账进预算")
	// 账本产物。
	assert.FileExists(t, filepath.Join(fx.taskDir, "metrics.jsonl"))
	retro, rerr := os.ReadFile(filepath.Join(fx.taskDir, "retro.md"))
	require.NoError(t, rerr)
	assert.Contains(t, string(retro), "milestone M-01: closed")
	// 单元代码经 worktree ff-merge 回主 checkout。
	assert.FileExists(t, filepath.Join(fx.root, "build", "M01-T01.txt"))

	// plan/execute/review/judge 都被调用过。
	sd.log.mu.Lock()
	defer sd.log.mu.Unlock()
	assert.NotEmpty(t, sd.log.plan)
	assert.Len(t, sd.log.exec, 2)
	assert.Len(t, sd.log.review, 2)
	assert.NotEmpty(t, sd.log.judge)
}

func TestRunTaskExecuteRetryThenSuccess(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	calls := 0
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		calls++
		if calls == 1 {
			return drivers.MockResult(func(r *drivers.Result) {
				code := 1
				r.ExitCode = &code
				msg := "provider 500 exploded"
				r.ErrorMessage = &msg
				r.Settled = false
			})
		}
		return sd.mock.Execute(a) // 成功路径必须走真 mock（写交付物）
	}

	_, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)

	st := loadFixtureState(t, fx)
	assert.Equal(t, "completed", st.Status)
	first := st.Packets["M01-T01"]
	t.Logf("retry-history: %#v", first.RetryHistory)
	// transport 重试发生在 executeStep 内部（同一次 attempt），attempts 计
	// executeStep 次数而非 spawn 次数。
	assert.Equal(t, 1, first.Attempts)
	require.NotNil(t, first.RetryHistory)
	found := false
	for _, rh := range first.RetryHistory {
		entry, _ := rh.(map[string]any)
		if entry != nil && entry["scope"] == "transport" && entry["policy"] == "backoff" {
			found = true // 错误码由 errs 分类决定（此处实测 SERVER）；scope/policy 是 machine 层契约
		}
	}
	assert.True(t, found, "transport 失败必须留审计痕迹")
}
func TestRunTaskSpinEscalatesToInbox(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			code := 1
			r.ExitCode = &code
			msg := "same failure every time"
			r.ErrorMessage = &msg
			r.Settled = false
		})
	}

	var logs []string
	summary, err := RunTask(runOptsWithLog(fx, sd.set(), &logs))
	require.NoError(t, err)

	st := loadFixtureState(t, fx)
	assert.Equal(t, "escalated", st.Status)
	assert.Equal(t, "escalated", summary.Status)
	ps := st.Packets["M01-T01"]
	assert.Equal(t, "inbox", ps.Status)
	assert.LessOrEqual(t, ps.FailedAttempts, 3, "spin 检测应在 max-retries 内提前升级")
	found := false
	for _, e := range st.Inbox {
		entry, _ := e.(map[string]any)
		if pid, _ := entry["packet-id"].(string); pid == "M01-T01" {
			found = true
		}
	}
	assert.True(t, found, "spin 升级必须落 inbox")
}

func TestRunTaskBudgetHardStop(t *testing.T) {
	fx := newGitFixture(t)
	patchConfigFile(t, fx, `{
		"version": 1,
		"drivers": {"plan": "mock", "execute": "mock", "review": "mock", "judge": "mock", "intake": "mock", "garden": "mock"},
		"loops": {
			"plan": { "harness": "kimi", "model": "kimi-code/k3", "channel": "kimi-subscription", "thinking": "max" },
			"review": { "harness": "omp", "model": "opencode-go/deepseek-v4-pro", "channel": "opencode-go", "thinking": "max" },
			"judge": { "harness": "omp", "model": "kimi-coding/k3", "channel": "kimi-coding", "thinking": "max" }
		},
		"routes": { "default": { "harness": "omp", "model": "opencode-go/deepseek-v4-flash", "channel": "opencode-go", "thinking": "max" } },
		"channels": {
			"opencode-go": { "env-key": "OPENCODE_API_KEY", "fallback": "opencode" },
			"kimi-subscription": { "env-key": null, "fallback": "kimi-coding" },
			"kimi-coding": { "env-key": "KIMI_API_KEY", "fallback": "opencode" }
		},
		"budget": {"enforce": "hard", "daily-token-limit": 50},
		"watchdog": {"idle-timeout-ms": 5000, "wall-timeout-ms": 30000, "kill-grace-ms": 500},
		"retry": {"base-delay-ms": 5, "max-delay-ms": 10, "jitter": 0},
		"defect-injection": {"every-n-units": 1}
	}`)

	summary, err := RunTask(runOpts(fx, happyDrivers().set()))
	require.NoError(t, err, "hard stop 是正常停机不是错误")

	st := loadFixtureState(t, fx)
	assert.Equal(t, "budget-stopped", st.Status)
	assert.Equal(t, "budget-stopped", summary.Status)
}

func TestRunTaskPlanFailureEscalates(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.planFn = func(a drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			code := 1
			r.ExitCode = &code
			msg := "kimi hung"
			r.ErrorMessage = &msg
			r.Settled = false
		})
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)

	st := loadFixtureState(t, fx)
	assert.Equal(t, "escalated", st.Status)
	assert.Equal(t, "escalated", summary.Status)
	require.NotEmpty(t, st.Inbox)
	entry, _ := st.Inbox[0].(map[string]any)
	assert.Contains(t, entry["reason"], "plan step failed on every channel")
}

func TestRunTaskJudgeReturnEscalates(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.judgeFn = func(a drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			r.Text = `{"verdict":"return","score":0.4,"reasons":["coverage matrix has gaps"]}`
		})
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)

	st := loadFixtureState(t, fx)
	assert.Equal(t, "escalated", st.Status)
	assert.Equal(t, "escalated", summary.Status)
	last, _ := st.Inbox[len(st.Inbox)-1].(map[string]any)
	assert.Contains(t, last["reason"], "judge verdict: return")
	assert.Contains(t, last["reason"], "coverage matrix has gaps")
}

func TestRunSingleStepOnlyExecuteAndVerify(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	opts := stepOpts(fx, sd.set())
	opts.Only = "execute"
	opts.PacketID = "M01-T01"
	summary, err := RunSingleStep(opts)
	require.NoError(t, err)
	assert.True(t, summary.Step.OK, "mock execute 应成功")
	assert.Equal(t, "execute", summary.Step.Name)

	st := loadFixtureState(t, fx)
	// v0.24：--only execute = 子环闭环（execute+L1 到绿），不再是单步 executed。
	assert.Equal(t, "l1-passed", st.Packets["M01-T01"].Status)

	// verify 步：编排器亲自跑 L1（幂等复跑）。
	opts.Only = "verify"
	summary, err = RunSingleStep(opts)
	require.NoError(t, err)
	assert.Equal(t, "verify", summary.Step.Name)
	st = loadFixtureState(t, fx)
	assert.Equal(t, "l1-passed", st.Packets["M01-T01"].Status)
	l1, ok := st.Packets["M01-T01"].Evidence.L1.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, l1["pass"])
}

func TestAssertNoRunningOrchestratorCases(t *testing.T) {
	dir := t.TempDir()
	pidFile := PidFile(dir)
	assert.Equal(t, filepath.Join(dir, "orchestrator.pid"), pidFile)

	// 无文件 → ok。
	assert.NoError(t, AssertNoRunningOrchestrator(dir))

	// 坏 JSON / 非整数 pid → 陈旧，放行。
	require.NoError(t, os.WriteFile(pidFile, []byte("not json"), 0o644))
	assert.NoError(t, AssertNoRunningOrchestrator(dir))
	require.NoError(t, os.WriteFile(pidFile, []byte(`{"pid":"x"}`), 0o644))
	assert.NoError(t, AssertNoRunningOrchestrator(dir))

	// 死 pid → 陈旧，放行。
	require.NoError(t, os.WriteFile(pidFile, []byte(`{"pid":999999999}`), 0o644))
	assert.NoError(t, AssertNoRunningOrchestrator(dir))

	// 活 pid（自己）→ 拒绝启动。
	require.NoError(t, os.WriteFile(pidFile, []byte(`{"pid":`+strconv.Itoa(os.Getpid())+`}`), 0o644))
	err := AssertNoRunningOrchestrator(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "another orchestrator is running this task")
}

func TestWarnIfForeignDirtyLogs(t *testing.T) {
	fx := newGitFixture(t)
	writeFile(t, filepath.Join(fx.root, "stray.txt"), "pre-existing WIP")

	var logs []string
	ctx, err := CreateContext(Opts{
		TaskDir: fx.taskDir, Cwd: fx.root, Config: configFor(fx),
		Drivers: happyDrivers().set(),
		Log:     func(msg string) { logs = append(logs, msg) },
	})
	require.NoError(t, err)
	ctx.WarnIfForeignDirty()
	joined := strings.Join(logs, "\n")
	assert.Contains(t, joined, "WARNING:")
	assert.Contains(t, joined, "dirty file(s) outside the task dir")

	// 干净工作区 → 无警告。
	logs = nil
	gitCmd(t, fx.root, "add", "-A")
	gitCmd(t, fx.root, "commit", "-q", "-m", "clean")
	ctx.WarnIfForeignDirty()
	assert.Empty(t, logs)
}

func TestRunSingleStepOnlyReviewAfterL1Green(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	// 先 execute 再 verify 打底。
	_, err := RunSingleStep(RunSingleStepOpts{RunOpts: runOpts(fx, sd.set()).RunOpts, Only: "execute", PacketID: "M01-T01"})
	require.NoError(t, err)
	_, err = RunSingleStep(RunSingleStepOpts{RunOpts: runOpts(fx, sd.set()).RunOpts, Only: "verify", PacketID: "M01-T01"})
	require.NoError(t, err)

	summary, err := RunSingleStep(RunSingleStepOpts{RunOpts: runOpts(fx, sd.set()).RunOpts, Only: "review", PacketID: "M01-T01"})
	require.NoError(t, err)
	assert.Equal(t, "review", summary.Step.Name)
	require.NotNil(t, summary.Step.Verdict)
	assert.Equal(t, "pass", summary.Step.Verdict.Verdict)

	st := loadFixtureState(t, fx)
	review, ok := st.Packets["M01-T01"].Evidence.Review.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "pass", review["verdict"])
}

func TestPauseBeforeReviewThenResume(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()

	// 第一跑：--pause-before review → L1 绿后冻结。
	firstOpts := runOpts(fx, sd.set())
	firstOpts.PauseBefore = "review"
	_, err := RunTask(firstOpts)
	st := loadFixtureState(t, fx)
	assert.Equal(t, "l1-passed", st.Packets["M01-T01"].Status)
	assert.Equal(t, "paused", st.Status)

	// 第二跑：从 review resume 直达完成（P3：补缺陷注入采样路径）。
	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)
	st = loadFixtureState(t, fx)
	assert.Equal(t, "completed", st.Packets["M01-T01"].Status)
}

func TestInboxedPacketRerunStopsWithoutRespawn(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	// 手工把 packet 置为 inbox 升级态（模拟上次运行升级后人工未 re-arm）。
	st := loadFixtureState(t, fx)
	inboxAt := "2026-08-22T00:00:00.000Z"
	st.Packets["M01-T01"].Status = "inbox"
	st.Packets["M01-T01"].UpdatedAt = &inboxAt
	st.Status = "escalated"
	raw, merr := json.Marshal(st)
	require.NoError(t, merr)
	writeFile(t, filepath.Join(fx.taskDir, "state.json"), string(raw))

	before := len(sd.log.exec)
	summary, err := RunTask(runOptsWithLog(fx, sd.set(), &[]string{}))
	require.NoError(t, err)
	assert.Equal(t, "escalated", summary.Status, "升级态重跑绝不翻回 running")
	sd.log.mu.Lock()
	defer sd.log.mu.Unlock()
	assert.Equal(t, before, len(sd.log.exec), "inbox packet 绝不重新 spawn")
}
