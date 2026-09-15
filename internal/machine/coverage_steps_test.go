package machine

// 覆盖率冲刺（2026-08）：steps.go 各步骤的分支级直测（凭证审批、裁决循环、
// plan 会话纪律、升级路径）。任务级场景见 coverage_flow_test.go。

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/defectinjection"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- 共用设施 ----

// steppingClock 每次调用前进 step，首返回返回 start。
func steppingClock(start time.Time, step time.Duration) func() time.Time {
	n := -1
	return func() time.Time {
		n++
		return start.Add(time.Duration(n) * step)
	}
}

// credsPacket 与 fixturePacket 同形，但声明需要 live 凭证。
func credsPacket(id, requirement string) string {
	base := fixturePacket(id, requirement)
	return strings.Replace(base, "needs-live-credentials: false", "needs-live-credentials: true", 1)
}

func writeApproval(t *testing.T, fx fixture, packetID, body string) string {
	t.Helper()
	path := filepath.Join(fx.taskDir, "approvals", packetID+".json")
	writeFile(t, path, body)
	return path
}

var testEnvKeys = func(key string) (string, bool) {
	switch key {
	case "FLY_TEST_KEY_A":
		return "value-a", true
	case "FLY_TEST_KEY_B":
		return "value-b", true
	}
	return "", false
}

// ---- awaitCredentialsApproval ----

func TestAwaitCredentialsApprovalTimeoutAnnouncesAndEscalates(t *testing.T) {
	fx := newFixture(t)
	var logs []string
	ctx := unitTestContext(t, fx)
	ctx.Log = func(msg string) { logs = append(logs, msg) }
	ctx.Now = steppingClock(time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC), 50*time.Millisecond)

	packet := ctx.Packets["M01-T01"]
	ps := ctx.State.Packets["M01-T01"]

	approval := ctx.awaitCredentialsApproval(packet)
	assert.False(t, approval.OK)
	assert.Empty(t, approval.Injected)
	assert.Equal(t, "inbox", ps.Status)
	assert.Equal(t, "escalated", ctx.State.Status)
	require.Len(t, ctx.State.Inbox, 2, "公告 + 超时各一条")
	first, _ := ctx.State.Inbox[0].(map[string]any)
	assert.Contains(t, first["reason"], "needs-live-credentials")
	last, _ := ctx.State.Inbox[1].(map[string]any)
	assert.Contains(t, last["reason"], "timed out after 200ms")
	assert.FileExists(t, filepath.Join(fx.taskDir, "inbox.md"))
	assert.Contains(t, strings.Join(logs, "\n"), "awaiting live-credentials approval")
}

func TestAwaitCredentialsApprovalAlreadyAnnouncedSkipsDuplicate(t *testing.T) {
	fx := newFixture(t)
	ctx := unitTestContext(t, fx)
	ctx.Now = steppingClock(time.Now(), 50*time.Millisecond)
	ctx.State.Packets["M01-T01"].Status = "awaiting-credentials"

	approval := ctx.awaitCredentialsApproval(ctx.Packets["M01-T01"])
	assert.False(t, approval.OK)
	require.Len(t, ctx.State.Inbox, 1, "已公告过 → 只有超时条目")
}

func TestAwaitCredentialsApprovalArrivesMidPoll(t *testing.T) {
	fx := newFixture(t)
	var logs []string
	ctx := unitTestContext(t, fx)
	ctx.Log = func(msg string) { logs = append(logs, msg) }
	ctx.EnvLookup = testEnvKeys
	ctx.Now = steppingClock(time.Now(), 10*time.Millisecond)
	approvalFile := filepath.Join(fx.taskDir, "approvals", "M01-T01.json")
	ctx.Sleep = func(time.Duration) { // 首次轮询后人工写下审批文件
		writeApproval(t, fx, "M01-T01", `{"approved-env": ["FLY_TEST_KEY_A"]}`)
		_ = approvalFile
	}

	approval := ctx.awaitCredentialsApproval(ctx.Packets["M01-T01"])
	require.True(t, approval.OK)
	assert.Equal(t, map[string]string{"FLY_TEST_KEY_A": "value-a"}, approval.Injected)
	ps := ctx.State.Packets["M01-T01"]
	assert.Equal(t, "pending", ps.Status)
	cred, ok := ps.Evidence.Credentials.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"FLY_TEST_KEY_A"}, cred["approved-env"])
	assert.Equal(t, []any{"FLY_TEST_KEY_A"}, cred["injected-env"])
	assert.Equal(t, []any{}, cred["missing-env"])
	assert.Contains(t, strings.Join(logs, "\n"), "credentials approved by human (1 keys)")
}

func TestAwaitCredentialsApprovalPartialMissingKeys(t *testing.T) {
	fx := newFixture(t)
	var logs []string
	ctx := unitTestContext(t, fx)
	ctx.Log = func(msg string) { logs = append(logs, msg) }
	ctx.EnvLookup = testEnvKeys
	writeApproval(t, fx, "M01-T01", `{"approved-env": ["FLY_TEST_KEY_A", "FLY_TEST_ABSENT"]}`)

	approval := ctx.awaitCredentialsApproval(ctx.Packets["M01-T01"])
	require.True(t, approval.OK)
	assert.Equal(t, map[string]string{"FLY_TEST_KEY_A": "value-a"}, approval.Injected)
	cred, _ := ctx.State.Packets["M01-T01"].Evidence.Credentials.(map[string]any)
	assert.Equal(t, []any{"FLY_TEST_ABSENT"}, cred["missing-env"])
	assert.Contains(t, strings.Join(logs, "\n"), ", missing in env: FLY_TEST_ABSENT")
}

func TestAwaitCredentialsApprovalRejectsBadPayloads(t *testing.T) {
	for name, body := range map[string]string{
		"非 JSON": `{oops`,
		"非字符串键":  `{"approved-env": [1, 2]}`,
	} {
		t.Run(name, func(t *testing.T) {
			fx := newFixture(t)
			ctx := unitTestContext(t, fx)
			ctx.EnvLookup = testEnvKeys
			writeApproval(t, fx, "M01-T01", body)
			approval := ctx.awaitCredentialsApproval(ctx.Packets["M01-T01"])
			assert.False(t, approval.OK)
			assert.Nil(t, ctx.State.Packets["M01-T01"].Evidence.Credentials)
		})
	}
}

// ---- executeStep 凭证闸门 ----

func TestExecuteStepReinjectsPreviouslyApprovedKeys(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	ctx := unitTestContext(t, fx)
	ctx.Drivers = sd.set()
	ctx.EnvLookup = testEnvKeys

	packet := ctx.Packets["M01-T01"]
	packet.NeedsLiveCredentials = true // 契约已批准过的 resume 场景
	ps := ctx.State.Packets["M01-T01"]
	ps.Evidence.Credentials = map[string]any{"injected-env": []any{"FLY_TEST_KEY_A", "FLY_TEST_ABSENT"}}

	out, err := ctx.executeStep(packet)
	require.NoError(t, err)
	assert.True(t, out.Success)
	sd.log.mu.Lock()
	defer sd.log.mu.Unlock()
	require.Len(t, sd.log.exec, 1)
	assert.Equal(t, map[string]string{"FLY_TEST_KEY_A": "value-a"}, sd.log.exec[0].APIKeys)
}

func TestExecuteStepStopsWhenCredentialsNeverApproved(t *testing.T) {
	fx := newGitFixture(t)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), credsPacket("M01-T01", "R-01"))
	sd := happyDrivers()

	opts := runOpts(fx, sd.set())
	opts.Env = func(string) (string, bool) { return "", false }
	opts.Now = steppingClock(time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC), 50*time.Millisecond)

	summary, err := RunTask(opts)
	require.NoError(t, err)
	assert.Equal(t, "escalated", summary.Status)

	st := loadFixtureState(t, fx)
	assert.Equal(t, "inbox", st.Packets["M01-T01"].Status)
	assert.Equal(t, "escalated", st.Status)
	sd.log.mu.Lock()
	defer sd.log.mu.Unlock()
	assert.Empty(t, sd.log.exec, "等人批凭证绝不 spawn")
}

// ---- executeStep 杂项分支 ----

func TestExecuteStepUnknownRouteFailsFast(t *testing.T) {
	fx := newGitFixture(t)
	ctx := unitTestContext(t, fx)
	ctx.Config.Routes = nil // packet schema 只允许三值路由；绑定缺失用配置缺失触发

	_, err := ctx.executeStep(ctx.Packets["M01-T01"])
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no route binding")
}

func TestExecuteStepFatalShortCircuits(t *testing.T) {
	fx := newGitFixture(t)
	ctx := unitTestContext(t, fx)
	ctx.fatal = errors.New("disk on fire")
	_, err := ctx.executeStep(ctx.Packets["M01-T01"])
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk on fire")
}

func TestExecuteStepMissingDoneReportIsRetryableExhaustion(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	calls := 0
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		calls++
		// 写产物但不写 done-report：出口契约判交付物缺失（EMPTY_RESPONSE）。
		require.NoError(t, os.MkdirAll(filepath.Join(a.Cwd, "build"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(a.Cwd, "build", a.PacketID+".txt"), []byte("x"), 0o644))
		return drivers.MockResult(nil)
	}
	ctx := unitTestContext(t, fx)
	ctx.Drivers = sd.set()

	out, err := ctx.executeStep(ctx.Packets["M01-T01"])
	require.NoError(t, err)
	assert.False(t, out.Success)
	assert.Equal(t, errs.EMPTY_RESPONSE, out.ErrorCode)
	assert.Equal(t, 3, calls, "max-transport-retries=2 → 共 3 次 spawn")
}

// ---- agentVerdictStep 直测 ----

func verdictCtx(t *testing.T, fx fixture, sd *scriptedDrivers) *Context {
	ctx := unitTestContext(t, fx)
	ctx.Drivers = sd.set()
	return ctx
}

func reviewBinding(channel string) fallback.Binding {
	return fallback.Binding{Harness: "pi", Model: "test-model", Channel: channel}
}

func TestAgentVerdictStepParsesVerdictAndLabelsLogs(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	var logs []string
	ctx := verdictCtx(t, fx, sd)
	ctx.Log = func(msg string) { logs = append(logs, msg) }

	attempt := 1
	out, err := ctx.agentVerdictStep(verdictArgs{
		role: "review", binding: reviewBinding("opencode-go"), prompt: "P",
		packetID: "M01-T01", attemptNo: &attempt, ps: ctx.State.Packets["M01-T01"],
	})
	require.NoError(t, err)
	require.True(t, out.OK)
	assert.Equal(t, "pass", out.Verdict.Verdict)
	assert.Empty(t, logs, "成功路径无失败日志")
}

func TestAgentVerdictStepEmptyResponseExhaustsRetries(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.reviewFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) { r.Text = "no json here" })
	}
	ctx := verdictCtx(t, fx, sd)
	ps := ctx.State.Packets["M01-T01"]

	out, err := ctx.agentVerdictStep(verdictArgs{
		role: "review", binding: reviewBinding("opencode-go"), prompt: "P",
		packetID: "M01-T01", ps: ps,
	})
	require.NoError(t, err)
	assert.False(t, out.OK)
	assert.Equal(t, errs.EMPTY_RESPONSE, out.ErrorCode)
	require.NotNil(t, ps.RetryHistory)
	assert.Len(t, ps.RetryHistory, 3, "初试 + 两次退避")
}

func TestAgentVerdictStepStreamErrorAuthNotRetried(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.reviewFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			code := 0
			r.ExitCode = &code
			msg := "401 unauthorized"
			r.ErrorMessage = &msg
		})
	}
	ctx := verdictCtx(t, fx, sd)
	ps := ctx.State.Packets["M01-T01"]

	out, err := ctx.agentVerdictStep(verdictArgs{
		role: "review", binding: reviewBinding("opencode-go"), prompt: "P",
		packetID: "M01-T01", ps: ps,
	})
	require.NoError(t, err)
	assert.Equal(t, errs.AUTH, out.ErrorCode)
	found := false
	for _, rh := range ps.RetryHistory {
		entry, _ := rh.(map[string]any)
		if entry != nil && entry["policy"] == "no-retry" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestAgentVerdictStepFallsBackToNextChannel(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	var logs []string
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		if a.Binding.Channel == "kimi-coding" {
			return drivers.MockResult(func(r *drivers.Result) {
				msg := "429 rate limit exceeded"
				r.ErrorMessage = &msg
			})
		}
		return sd.mock.Review(a) // opencode 渠道成功
	}
	ctx := verdictCtx(t, fx, sd)
	ctx.Log = func(msg string) { logs = append(logs, msg) }

	out, err := ctx.agentVerdictStep(verdictArgs{
		role: "review", binding: reviewBinding("kimi-coding"), prompt: "P",
		packetID: "M01-T01", ps: ctx.State.Packets["M01-T01"],
	})
	require.NoError(t, err)
	require.True(t, out.OK)
	sd.log.mu.Lock()
	defer sd.log.mu.Unlock()
	found := false
	for _, rh := range ctx.State.Packets["M01-T01"].RetryHistory {
		entry, _ := rh.(map[string]any)
		if entry != nil && entry["policy"] == "channel kimi-coding -> opencode" {
			found = true
		}
	}
	assert.True(t, found, "降级必须留审计痕迹")
}

// ---- 裁决解析门（§6 出口契约）：终态正常 + 无信号 + stderr 无错误才解析 ----

// TestAgentVerdictStepSignalTrappedExitZeroNeverGreen 被 kill 后 trap 成
// exit 0 的假绿灯：signal 在场 → 绝不解析裁决文本。
func TestAgentVerdictStepSignalTrappedExitZeroNeverGreen(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.reviewFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			code := 0
			r.ExitCode = &code
			r.Signal = "SIGKILL"
		})
	}
	ctx := verdictCtx(t, fx, sd)
	out, err := ctx.agentVerdictStep(verdictArgs{
		role: "review", binding: reviewBinding("opencode-go"), prompt: "P",
		packetID: "M01-T01", ps: ctx.State.Packets["M01-T01"],
	})
	require.NoError(t, err)
	assert.False(t, out.OK)
	assert.Equal(t, errs.TRANSPORT, out.ErrorCode, "signal → TRANSPORT，文本再绿也不算")
}

// TestAgentVerdictStepStderrErrorBlocksVerdict stderr 带错误模式 →
// 文本里的裁决不可信（出口契约"stderr 无错误"第三条腿）。
func TestAgentVerdictStepStderrErrorBlocksVerdict(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.reviewFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			code := 0
			r.ExitCode = &code
			r.Stderr = "401 unauthorized"
		})
	}
	ctx := verdictCtx(t, fx, sd)
	out, err := ctx.agentVerdictStep(verdictArgs{
		role: "review", binding: reviewBinding("opencode-go"), prompt: "P",
		packetID: "M01-T01", ps: ctx.State.Packets["M01-T01"],
	})
	require.NoError(t, err)
	assert.False(t, out.OK)
	assert.Equal(t, errs.AUTH, out.ErrorCode)
}

// TestAgentVerdictStepParsesVerdictFromStdoutFallback Text 空 →
// Stdout 兜底（Node: result.text ?? result.stdout）。
func TestAgentVerdictStepParsesVerdictFromStdoutFallback(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.reviewFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			code := 0
			r.ExitCode = &code
			r.Text = ""
			r.Stdout = "prose\n{\"verdict\":\"pass\",\"score\":0.9,\"reasons\":[\"ok\"]}"
		})
	}
	ctx := verdictCtx(t, fx, sd)
	out, err := ctx.agentVerdictStep(verdictArgs{
		role: "review", binding: reviewBinding("opencode-go"), prompt: "P",
		packetID: "M01-T01", ps: ctx.State.Packets["M01-T01"],
	})
	require.NoError(t, err)
	require.True(t, out.OK)
	assert.Equal(t, "pass", out.Verdict.Verdict)
}

func TestAgentVerdictStepChainEndsWithoutModelMapping(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	var logs []string
	sd.reviewFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			msg := "429 slow down"
			r.ErrorMessage = &msg
		})
	}
	ctx := verdictCtx(t, fx, sd)
	ctx.Log = func(msg string) { logs = append(logs, msg) }

	// kimi-subscription → kimi-coding 没有 review 模型映射（P1-12①）→ 链终止。
	out, err := ctx.agentVerdictStep(verdictArgs{
		role: "review", binding: reviewBinding("kimi-subscription"), prompt: "P",
		packetID: "M01-T01", ps: ctx.State.Packets["M01-T01"],
	})
	require.NoError(t, err)
	assert.Equal(t, errs.RATE_LIMIT, out.ErrorCode)
	assert.Contains(t, strings.Join(logs, "\n"), "fallback chain ends")
}

func TestAgentVerdictStepBudgetStopFatalAndFailLoud(t *testing.T) {
	fx := newGitFixture(t)

	// budget hard stop。
	sd := happyDrivers()
	ctx := verdictCtx(t, fx, sd)
	limit := int64(100)
	ctx.Config.Budget.Enforce = "hard"
	ctx.Config.Budget.DailyTokenLimit = &limit
	day := time.Now().UTC().Format("2006-01-02")
	ctx.State.Budget.Daily[day] = state.DailyBucket{Tokens: limit}
	out, err := ctx.agentVerdictStep(verdictArgs{role: "review", binding: reviewBinding("opencode-go"), prompt: "P"})
	require.NoError(t, err)
	assert.True(t, out.Stopped)

	// fatal 短路。
	ctx.fatal = errors.New("boom")
	_, err = ctx.agentVerdictStep(verdictArgs{role: "review", binding: reviewBinding("opencode-go"), prompt: "P"})
	require.Error(t, err)
	ctx.fatal = nil

	// fail-loud：真实驱动 + 缺 key。
	sd2 := happyDrivers()
	ctx2 := verdictCtx(t, fx, sd2)
	ctx2.Config.Drivers["review"] = "pi"
	ctx2.EnvLookup = func(string) (string, bool) { return "", false }
	var logs []string
	ctx2.Log = func(msg string) { logs = append(logs, msg) }
	out, err = ctx2.agentVerdictStep(verdictArgs{
		role: "review", binding: reviewBinding("opencode-go"), prompt: "P", ps: ctx2.State.Packets["M01-T01"],
	})
	require.NoError(t, err)
	assert.Equal(t, errs.AUTH, out.ErrorCode)
	assert.Contains(t, strings.Join(logs, "\n"), "fail-loud")
	sd2.log.mu.Lock()
	defer sd2.log.mu.Unlock()
	assert.Empty(t, sd2.log.review)
}

func TestAgentVerdictStepJudgeRoleDispatchWithoutPacketState(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	ctx := verdictCtx(t, fx, sd)

	out, err := ctx.agentVerdictStep(verdictArgs{
		role: "judge", binding: reviewBinding("kimi-coding"), prompt: "JUDGE",
	})
	require.NoError(t, err)
	assert.True(t, out.OK)
}

// ---- judgeStep 直测 ----

func TestJudgeStepInitializesNilGateAndRecordsError(t *testing.T) {
	fx := newGitFixture(t)
	ms := m01GateState(t, fx)
	ms.Gate = nil
	saveMilestoneState(t, fx, "M-01", ms)

	sd := happyDrivers()
	sd.judgeFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) { r.Text = "unparseable" })
	}
	ctx := verdictCtx(t, fx, sd)

	out, err := ctx.judgeStep(contract.Milestone{ID: "M-01"})
	require.NoError(t, err)
	assert.False(t, out.Pass)
	assert.Equal(t, errs.EMPTY_RESPONSE, out.ErrorCode)
	gate, _ := ctx.State.Milestones["M-01"].Gate.(map[string]any)
	judge, _ := gate["judge"].(map[string]any)
	assert.Equal(t, "error", judge["verdict"])
}

func TestJudgeStepCoercesNonMapGate(t *testing.T) {
	fx := newGitFixture(t)
	ms := m01GateState(t, fx)
	ms.Gate = "not-a-map"
	saveMilestoneState(t, fx, "M-01", ms)

	ctx := verdictCtx(t, fx, happyDrivers())
	out, err := ctx.judgeStep(contract.Milestone{ID: "M-01"})
	require.NoError(t, err)
	assert.True(t, out.Pass)
	// C11（v0.18）：非对象 gate 替换为对象并写回 state——judge 结论绝不落进
	// 孤立内存 map。
	gate, ok := ctx.State.Milestones["M-01"].Gate.(map[string]any)
	require.True(t, ok, "gate 必须被持久化为对象")
	assert.Contains(t, gate, "judge")
}

func TestJudgeStepExtractsGateLevelsAndStopsOnBudget(t *testing.T) {
	fx := newGitFixture(t)
	ms := m01GateState(t, fx)
	ms.Gate = map[string]any{"levels": map[string]any{"L2": map[string]any{"pass": true}}}
	saveMilestoneState(t, fx, "M-01", ms)

	sd := happyDrivers()
	var prompts []string
	sd.judgeFn = func(a drivers.CallArgs) drivers.Result {
		prompts = append(prompts, a.Prompt)
		return sd.mock.Judge(a)
	}
	ctx := verdictCtx(t, fx, sd)
	out, err := ctx.judgeStep(contract.Milestone{ID: "M-01"})
	require.NoError(t, err)
	assert.True(t, out.Pass)
	require.Len(t, prompts, 1)
	assert.Contains(t, prompts[0], `"L2"`, "gate levels 注入 judge 提示词")

	// budget hard stop。
	limit := int64(50)
	ctx.Config.Budget.Enforce = "hard"
	ctx.Config.Budget.DailyTokenLimit = &limit
	ctx.State.Budget.Daily[time.Now().UTC().Format("2006-01-02")] = state.DailyBucket{Tokens: 80}
	out, err = ctx.judgeStep(contract.Milestone{ID: "M-01"})
	require.NoError(t, err)
	assert.True(t, out.Stopped)
}

// ---- planStep 直测 ----

const planLoopChannel = "kimi-subscription"

func TestPlanStepErrorsWithoutPacketsOrBinding(t *testing.T) {
	fx := newGitFixture(t)
	ctx := verdictCtx(t, fx, happyDrivers())

	_, err := ctx.planStep(contract.Milestone{ID: "M-09"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no packets")

	delete(ctx.Config.Loops, "plan")
	_, err = ctx.planStep(contract.Milestone{ID: "M-01"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no loop binding")
}

func TestPlanStepColdStartTrimsRetroTail(t *testing.T) {
	fx := newGitFixture(t)
	var retro strings.Builder
	for i := range 45 {
		fmt.Fprintf(&retro, "RETRO-L%02d\n", i+1)
	}
	writeFile(t, filepath.Join(fx.taskDir, "retro.md"), retro.String())

	sd := happyDrivers()
	ctx := verdictCtx(t, fx, sd)
	out, err := ctx.planStep(contract.Milestone{ID: "M-01"}, "")
	require.NoError(t, err)
	assert.True(t, out.OK)

	sd.log.mu.Lock()
	defer sd.log.mu.Unlock()
	prompt := sd.log.plan[0].Prompt
	assert.Contains(t, prompt, "RETRO-L07", "保留最后 40 行（Split 含尾空元素）")
	assert.NotContains(t, prompt, "RETRO-L06")
}

func TestPlanStepResumesPinnedSessionAndCountsResumes(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.planFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) { r.SessionID = "sess-1" })
	}
	ctx := verdictCtx(t, fx, sd)
	ctx.State.PlanSession = map[string]any{
		"session-id": "sess-1", "channel": planLoopChannel, "model": "kimi-code/k3",
		"created-at": "2026-08-01T00:00:00.000Z", "resumes": float64(2),
	}

	out, err := ctx.planStep(contract.Milestone{ID: "M-01"}, "")
	require.NoError(t, err)
	assert.True(t, out.OK)
	session, _ := ctx.State.PlanSession.(map[string]any)
	assert.Equal(t, "sess-1", session["session-id"])
	assert.Equal(t, 3, session["resumes"], "resume 计数递增")
	assert.Equal(t, "2026-08-01T00:00:00.000Z", session["created-at"], "created-at 沿用")
}

func TestPlanStepMismatchedStoredSessionColdStarts(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	var logs []string
	sd.planFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) { r.SessionID = "sess-new" })
	}
	ctx := verdictCtx(t, fx, sd)
	ctx.Log = func(msg string) { logs = append(logs, msg) }
	ctx.State.PlanSession = map[string]any{
		"session-id": "old", "channel": "other-channel", "model": "other-model", "resumes": float64(0),
	}

	out, err := ctx.planStep(contract.Milestone{ID: "M-01"}, "")
	require.NoError(t, err)
	assert.True(t, out.OK)
	assert.Contains(t, strings.Join(logs, "\n"), "mismatches current binding")
	session, _ := ctx.State.PlanSession.(map[string]any)
	assert.Equal(t, "sess-new", session["session-id"])
}

func TestPlanStepDeadSessionRestartsCold(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	var logs []string
	calls := 0
	sd.planFn = func(drivers.CallArgs) drivers.Result {
		calls++
		if calls == 1 {
			return drivers.MockResult(func(r *drivers.Result) {
				code := 1
				r.ExitCode = &code
				msg := "provider 500 exploded"
				r.ErrorMessage = &msg
			})
		}
		return drivers.MockResult(func(r *drivers.Result) { r.SessionID = "sess-2" })
	}
	ctx := verdictCtx(t, fx, sd)
	ctx.Log = func(msg string) { logs = append(logs, msg) }
	ctx.State.PlanSession = map[string]any{
		"session-id": "sess-dead", "channel": planLoopChannel, "model": "kimi-code/k3", "resumes": float64(0),
	}

	out, err := ctx.planStep(contract.Milestone{ID: "M-01"}, "")
	require.NoError(t, err)
	assert.True(t, out.OK)
	assert.Contains(t, strings.Join(logs, "\n"), "is dead")
	session, _ := ctx.State.PlanSession.(map[string]any)
	assert.Equal(t, "sess-2", session["session-id"])
	assert.Equal(t, 0, session["resumes"])
	assert.Equal(t, 2, calls)
}

func TestPlanStepTransportRetryExhaustion(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.planFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			code := 1
			r.ExitCode = &code
			msg := "server error"
			r.ErrorMessage = &msg
		})
	}
	ctx := verdictCtx(t, fx, sd)

	out, err := ctx.planStep(contract.Milestone{ID: "M-01"}, "")
	require.NoError(t, err)
	assert.False(t, out.OK)
	assert.Equal(t, errs.SERVER, out.ErrorCode)
}

func TestPlanStepFallsBackAcrossChannels(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	var logs []string
	sd.planFn = func(a drivers.CallArgs) drivers.Result {
		if a.Binding.Channel == planLoopChannel {
			return drivers.MockResult(func(r *drivers.Result) {
				msg := "quota exceeded (403)"
				r.ErrorMessage = &msg
			})
		}
		return sd.mock.Plan(a)
	}
	ctx := verdictCtx(t, fx, sd)
	ctx.Log = func(msg string) { logs = append(logs, msg) }

	out, err := ctx.planStep(contract.Milestone{ID: "M-01"}, "")
	require.NoError(t, err)
	assert.True(t, out.OK, "降级到 kimi-coding 后成功")
	assert.Contains(t, strings.Join(logs, "\n"), "plan fallback: kimi-subscription -> kimi-coding")
	sd.log.mu.Lock()
	assert.Equal(t, "kimi-coding", sd.log.plan[len(sd.log.plan)-1].Binding.Channel, "降级后换渠道 spawn")
	sd.log.mu.Unlock()

	// 链终止：opencode-go 的下一渠道 opencode 没有 plan 映射。
	sd2 := happyDrivers()
	sd2.planFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			msg := "rate limited"
			r.ErrorMessage = &msg
		})
	}
	fx2 := newGitFixture(t)
	ctx2 := verdictCtx(t, fx2, sd2)
	var logs2 []string
	ctx2.Log = func(msg string) { logs2 = append(logs2, msg) }
	ctx2.Config.Loops["plan"] = config.LoopBinding{Harness: "pi", Model: "m", Channel: "opencode-go"}
	out, err = ctx2.planStep(contract.Milestone{ID: "M-01"}, "")
	require.NoError(t, err)
	assert.Equal(t, errs.RATE_LIMIT, out.ErrorCode)
	assert.Contains(t, strings.Join(logs2, "\n"), "plan fallback chain ends")
}

func TestPlanStepBudgetStopAndFailLoud(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	ctx := verdictCtx(t, fx, sd)
	limit := int64(10)
	ctx.Config.Budget.Enforce = "hard"
	ctx.Config.Budget.DailyTokenLimit = &limit
	ctx.State.Budget.Daily[time.Now().UTC().Format("2006-01-02")] = state.DailyBucket{Tokens: 99}

	out, err := ctx.planStep(contract.Milestone{ID: "M-01"}, "")
	require.NoError(t, err)
	assert.True(t, out.Stopped)
	sd.log.mu.Lock()
	assert.Empty(t, sd.log.plan)
	sd.log.mu.Unlock()

	// fail-loud：真实 plan 驱动 + 缺 KIMI_API_KEY。
	fx2 := newGitFixture(t)
	cfg := configFor(fx2)
	cfg.Drivers["plan"] = "pi"
	cfg.Loops["plan"] = config.LoopBinding{Harness: "pi", Model: "m", Channel: "kimi-coding"}
	ctx2, err := CreateContext(Opts{TaskDir: fx2.taskDir, Cwd: fx2.root, Config: cfg,
		Drivers: happyDrivers().set(), Log: func(string) {}, Sleep: func(time.Duration) {},
		Env: func(string) (string, bool) { return "", false }})
	require.NoError(t, err)
	out, err = ctx2.planStep(contract.Milestone{ID: "M-01"}, "")
	require.NoError(t, err)
	assert.False(t, out.OK)
	assert.Equal(t, errs.AUTH, out.ErrorCode)
}

// ---- escalatePacket 直测 ----

func primeEscalation(t *testing.T, ctx *Context) *contract.Packet {
	t.Helper()
	ps := ctx.State.Packets["M01-T01"]
	ps.FailedAttempts = 3 // max-retries=3 → 触发升级
	return ctx.Packets["M01-T01"]
}

func TestEscalatePacketDirectArms(t *testing.T) {
	t.Run("未知里程碑", func(t *testing.T) {
		fx := newGitFixture(t)
		ctx := verdictCtx(t, fx, happyDrivers())
		packet := primeEscalation(t, ctx)
		ctx.State.Packets["M01-T01"].Milestone = "M-99"
		_, err := ctx.escalatePacket(packet)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "references unknown milestone")
	})

	t.Run("planner 不可用直奔 inbox", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		sd.planFn = func(drivers.CallArgs) drivers.Result {
			return drivers.MockResult(func(r *drivers.Result) {
				code := 1
				r.ExitCode = &code
				msg := "server down"
				r.ErrorMessage = &msg
			})
		}
		var logs []string
		ctx := verdictCtx(t, fx, sd)
		ctx.Log = func(msg string) { logs = append(logs, msg) }
		outcome, err := ctx.escalatePacket(primeEscalation(t, ctx))
		require.NoError(t, err)
		assert.Equal(t, "inbox", outcome)
		assert.Contains(t, strings.Join(logs, "\n"), "planner unavailable")
		assert.Equal(t, "inbox", ctx.State.Packets["M01-T01"].Status)
	})

	t.Run("重载契约失败上抛", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		sd.planFn = func(drivers.CallArgs) drivers.Result {
			writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T02.md"), "::: broken frontmatter [")
			return drivers.MockResult(nil)
		}
		ctx := verdictCtx(t, fx, sd)
		_, err := ctx.escalatePacket(primeEscalation(t, ctx))
		require.Error(t, err)
	})

	t.Run("plan 绑定缺失上抛", func(t *testing.T) {
		fx := newGitFixture(t)
		ctx := verdictCtx(t, fx, happyDrivers())
		delete(ctx.Config.Loops, "plan")
		_, err := ctx.escalatePacket(primeEscalation(t, ctx))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no loop binding")
	})
}

// ---- injectionStop 双臂 ----

func TestInjectionStopBothReasonArms(t *testing.T) {
	fx := newGitFixture(t)
	ctx := verdictCtx(t, fx, happyDrivers())
	packet := ctx.Packets["M01-T01"]

	require.NoError(t, ctx.injectionStop(packet, &defectinjection.Outcome{Outcome: "fake-tests"}))
	entry, _ := ctx.State.Inbox[len(ctx.State.Inbox)-1].(map[string]any)
	assert.Contains(t, entry["reason"], "fake/weak tests proven")

	require.NoError(t, ctx.injectionStop(ctx.Packets["M01-T02"], &defectinjection.Outcome{Outcome: "restore-failed"}))
	entry, _ = ctx.State.Inbox[len(ctx.State.Inbox)-1].(map[string]any)
	assert.Contains(t, entry["reason"], "restore via git apply -R FAILED")
	assert.Equal(t, "escalated", ctx.State.Status)
}

// ---- registerSigterm 生命周期 ----

func TestRegisterSigtermLifecycle(t *testing.T) {
	var logs []string
	ch := registerSigterm(func(msg string) { logs = append(logs, msg) })
	require.NotNil(t, ch)
	unregisterSigterm(ch)
	time.Sleep(50 * time.Millisecond) // handler goroutine 收到 close 即退出（os.Exit 臂进程内不可达）
	assert.Empty(t, logs)
}

// ---- 测试专用小工具 ----

func m01GateState(t *testing.T, fx fixture) state.MilestoneState {
	t.Helper()
	st := loadFixtureState(t, fx)
	return st.Milestones["M-01"]
}

func saveMilestoneState(t *testing.T, fx fixture, id string, ms state.MilestoneState) {
	t.Helper()
	st := loadFixtureState(t, fx)
	st.Milestones[id] = ms
	raw, err := json.Marshal(st)
	require.NoError(t, err)
	writeFile(t, filepath.Join(fx.taskDir, "state.json"), string(raw))
}
