package machine

// 覆盖率冲刺（2026-08）：run.go/processPacket 全流程场景（升级修订、缺口回退、
// 缺陷注入、in-place 回退、单步模式错误路径等）。直测见 coverage_steps_test.go。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- 本文件专用设施 ----

// gatedPacket 带 L2/L3 里程碑闸门的 packet（L2 恒失败、L3 恒通过）。
func gatedPacket(id, requirement string) string {
	base := fixturePacket(id, requirement)
	return strings.Replace(base,
		"acceptance:\n  L1:",
		"acceptance:\n  L1:\n    - {cmd: \"test -f build/"+id+".txt\", expect: \"exit 0\"}\n  L2:\n    - {cmd: \"exit 3\", expect: \"exit 0\"}\n  L3:\n    - {cmd: \"true\", expect: \"exit 0\"}", 1)
}

// makeDefectPatch 用真实 git diff 生成可逆补丁（手写 hunk 头易损坏）。
func makeDefectPatch(t *testing.T, fx fixture) string {
	t.Helper()
	writeFile(t, filepath.Join(fx.root, "notes.txt"), "// notes.txt with injected defect\n")
	body := gitCmd(t, fx.root, "diff", "--", "notes.txt")
	gitCmd(t, fx.root, "checkout", "--", "notes.txt")
	require.NotEmpty(t, body, "git diff 应产出补丁")
	return body
}

// diConfigJSON 缺陷注入场景的整份配置（其余键照抄 fixture）。
func diConfigJSON(patch, testCmd string) string {
	return fmt.Sprintf(`{
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
  "budget": { "enforce": "soft", "daily-token-limit": 10000000 },
  "watchdog": { "idle-timeout-ms": 5000, "wall-timeout-ms": 30000, "kill-grace-ms": 500 },
  "retry": { "base-delay-ms": 10, "max-delay-ms": 40, "jitter": 0, "max-transport-retries": 2 },
  "credentials": { "poll-interval-ms": 10, "timeout-ms": 200 },
  "defect-injection": { "every-n-units": 1, "test-file-ratio-boost": 0.5, "patch": %s, "test-cmd": %s },
  "verify": { "command-timeout-ms": 10000 }
}`, strconv.Quote(patch), strconv.Quote(testCmd))
}

func saveStateObj(t *testing.T, fx fixture, st *state.State) {
	t.Helper()
	raw, err := json.Marshal(st)
	require.NoError(t, err)
	writeFile(t, filepath.Join(fx.taskDir, "state.json"), string(raw))
}

// ---- 升级修订（return-to-planner）----

func TestRunTaskEscalationRevisionReanchorsBaseRef(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	calls := 0
	codes := []string{"401 unauthorized", "400 bad request", "context window exceeded"}
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		calls++
		if calls <= 3 {
			msg := codes[(calls-1)%len(codes)]
			return drivers.MockResult(func(r *drivers.Result) {
				code := 0
				r.ExitCode = &code
				r.ErrorMessage = &msg
			})
		}
		return sd.mock.Execute(a)
	}
	sd.planFn = func(drivers.CallArgs) drivers.Result {
		// planner 常规修订：追加注释行（合法契约变更 → 触发契约提交 + 重锚）。
		content, err := os.ReadFile(filepath.Join(fx.taskDir, "spec.md"))
		require.NoError(t, err)
		writeFile(t, filepath.Join(fx.taskDir, "spec.md"), string(content)+"\n<!-- planner revision -->\n")
		return sd.mock.Plan(drivers.CallArgs{})
	}

	originalHead := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
	var logs []string
	summary, err := RunTask(runOptsWithLog(fx, sd.set(), &logs))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)

	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	assert.Equal(t, "completed", ps.Status)
	assert.Equal(t, 1, ps.PlanRevisions)
	assert.NotEqual(t, originalHead, *ps.BaseRef, "base-ref 重锚到修订检查点")
	policySeen := false
	for _, rh := range ps.RetryHistory {
		entry, _ := rh.(map[string]any)
		if entry != nil && entry["policy"] == "return-to-planner" {
			policySeen = true
		}
	}
	assert.True(t, policySeen)
	retro, err := os.ReadFile(filepath.Join(fx.taskDir, "retro.md"))
	require.NoError(t, err)
	assert.Contains(t, string(retro), "escalated to planner")
	sd.log.mu.Lock()
	defer sd.log.mu.Unlock()
	assert.Equal(t, 5, calls, "三次轮换失败 + 修订后 T01 成功 + T02 成功")
}

// ---- verification-gap（DC2）----

func TestRunTaskVerificationGapStrengthensAcceptance(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	reviews := 0
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		reviews++
		if reviews == 1 {
			return drivers.MockResult(func(r *drivers.Result) {
				r.Text = `{"verdict":"verification-gap","score":0.5,"reasons":["acceptance too weak"]}`
			})
		}
		return sd.mock.Review(a)
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)

	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	assert.Equal(t, 1, ps.PlanRevisions, "缺口消耗一次修订预算")
	sd.log.mu.Lock()
	defer sd.log.mu.Unlock()
	assert.Len(t, sd.log.exec, 2, "复验走 L1 不重执行（T01+T02 各一次）")
	assert.Len(t, sd.log.review, 3)
}

func TestRunTaskGapRecheckFailureReturnsToExecute(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	reviews := 0
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		reviews++
		if reviews == 1 {
			_, wtPath := verify.WorktreeNames(fx.root, fx.slug, "M01-T01")
			require.NoError(t, os.Remove(filepath.Join(wtPath, "build", "M01-T01.txt")))
			return drivers.MockResult(func(r *drivers.Result) {
				r.Text = `{"verdict":"verification-gap","score":0.5,"reasons":["weak"]}`
			})
		}
		return sd.mock.Review(a)
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)

	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	assert.Equal(t, "completed", ps.Status)
	assert.Equal(t, 1, ps.FailedAttempts, "评审改动候选，旧裁决失效")
	l1FailedSeen := false
	for _, rh := range ps.RetryHistory {
		entry, _ := rh.(map[string]any)
		if entry != nil && entry["error-code"] == "INVALID_REQUEST" {
			l1FailedSeen = true
		}
	}
	assert.True(t, l1FailedSeen)
}

// ---- L1 失败 + 契约篡改违规 ----

func TestRunTaskL1FailRecordsContractTamperViolation(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	execs := 0
	specPathInWt := func(cwd string) string {
		return filepath.Join(cwd, "docs", "minitask", "spec.md")
	}
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		execs++
		res := sd.mock.Execute(a) // 正常交付物
		if execs == 1 {
			f, err := os.OpenFile(specPathInWt(a.Cwd), os.O_APPEND|os.O_WRONLY, 0o644)
			require.NoError(t, err)
			_, err = f.WriteString("TAMPERED\n")
			require.NoError(t, err)
			require.NoError(t, f.Close())
		} else {
			// 还原：重写为与已提交版本逐字节一致。
			writeFile(t, specPathInWt(a.Cwd), mustReadCommittedSpec(t, fx))
		}
		return res
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)

	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	assert.Equal(t, 1, ps.FailedAttempts)
	l1FailedSeen := false
	for _, rh := range ps.RetryHistory {
		entry, _ := rh.(map[string]any)
		if entry != nil && entry["error-code"] == "L1_FAILED" {
			l1FailedSeen = true
		}
	}
	assert.True(t, l1FailedSeen, "契约篡改必须以 L1_FAILED 记账")
}

func mustReadCommittedSpec(t *testing.T, fx fixture) string {
	t.Helper()
	out := gitCmd(t, fx.root, "show", "HEAD:docs/minitask/spec.md")
	return out
}

// ---- 非 git 仓库：全程 in-place ----

func TestRunTaskNonGitPreservesWorkWithoutClaimingCompletion(t *testing.T) {
	fx := newFixture(t) // 不 init git

	summary, err := RunTask(runOpts(fx, happyDrivers().set()))
	require.NoError(t, err)
	assert.Equal(t, "escalated", summary.Status)

	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	assert.Equal(t, "inbox", ps.Status)
	require.NotNil(t, ps.Commit)
	commit, _ := ps.Commit.(map[string]any)
	require.NotEmpty(t, commit["error"], "没有持久 Git tree 不能把跳过提交视为交付成功")
	require.FileExists(t, filepath.Join(fx.root, "build", "M01-T01.txt"))

	l1, ok := ps.Evidence.L1.(map[string]any)
	require.True(t, ok)
	boundary, _ := l1["boundary"].(map[string]any)
	assert.Equal(t, true, boundary["skipped"], "diff 不可用 → 边界判定 skip 放行")

	review, ok := ps.Evidence.Review.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "pass", review["verdict"])
}

// ---- worktree 不可用 → in-place 回退 ----

func TestRunTaskWorktreeUnavailableFallsBackInPlace(t *testing.T) {
	fx := newGitFixture(t)
	parent := filepath.Dir(fx.root)
	require.NoError(t, os.Chmod(parent, 0o555)) // 无法创建兄弟 worktree 目录
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	var logs []string
	summary, err := RunTask(runOptsWithLog(fx, happyDrivers().set(), &logs))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)
	assert.Contains(t, strings.Join(logs, "\n"), "running in-place")

	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	require.NotNil(t, ps.Worktree)
	assert.Equal(t, "in-place", ps.Worktree.Mode)
	commit, _ := ps.Commit.(map[string]any)
	require.NotNil(t, commit["sha"], "scoped 单元提交成功")
}

// ---- 陈旧 worktree 重建 ----

func TestRunSingleStepRecreatesStaleWorktree(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()

	// 完整跑完：completeUnit 删除 worktree 目录但保留引用（Node 语义）。
	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)
	st := loadFixtureState(t, fx)
	require.Nil(t, st.Packets["M01-T01"].Worktree, "成功清理后不留悬空引用")
	branch, wtPath := verify.WorktreeNames(fx.root, fx.slug, "M01-T01")
	st.Packets["M01-T01"].Worktree = &state.WorktreeInfo{Mode: "worktree", Branch: branch, Path: wtPath}
	_, err = state.Save(fx.taskDir, st, nil)
	require.NoError(t, err)
	assert.NoDirExists(t, wtPath, "目录已删 → 引用陈旧")

	// 手工 --only execute：发现陈旧引用 → 重置并按需重建。
	opts := stepOpts(fx, sd.set())
	opts.Only = "execute"
	opts.PacketID = "M01-T01"
	_, err = RunSingleStep(opts)
	require.NoError(t, err)
	assert.DirExists(t, wtPath, "按需重建")
}

// ---- done-report 完好性守卫（pause/resume #6）----

func TestResumeWithoutValidDoneReportReexecutes(t *testing.T) {
	t.Run("报告缺失", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		first := runOpts(fx, sd.set())
		first.PauseBefore = "review"
		_, err := RunTask(first)
		require.NoError(t, err)
		report := filepath.Join(fx.taskDir, "reports", "M01-T01.done.json")
		require.NoError(t, os.Remove(report))

		summary, err := RunTask(runOpts(fx, sd.set()))
		require.NoError(t, err)
		assert.Equal(t, "completed", summary.Status)
		sd.log.mu.Lock()
		defer sd.log.mu.Unlock()
		assert.GreaterOrEqual(t, len(sd.log.exec), 2, "无有效报告 → 重执行而非直达评审")
	})

	t.Run("报告损坏", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		first := runOpts(fx, sd.set())
		first.PauseBefore = "review"
		_, err := RunTask(first)
		require.NoError(t, err)
		report := filepath.Join(fx.taskDir, "reports", "M01-T01.done.json")
		writeFile(t, report, "{not a report")

		summary, err := RunTask(runOpts(fx, sd.set()))
		require.NoError(t, err)
		assert.Equal(t, "completed", summary.Status)
	})
}

// ---- 中断恢复 ----

func TestRunTaskRecoversInterruptedUnits(t *testing.T) {
	fx := newGitFixture(t)
	st := loadFixtureState(t, fx)
	st.Packets["M01-T01"].Status = "executing" // 上次运行中断
	saveStateObj(t, fx, st)

	var logs []string
	summary, err := RunTask(runOptsWithLog(fx, happyDrivers().set(), &logs))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)
	joined := strings.Join(logs, "\n")
	assert.Contains(t, joined, "recovered interrupted units: M01-T01")
	assert.Contains(t, joined, "resuming interrupted unit")
}

// ---- 凭证审批文件出现在 inbox 升级之后 → 重新武装 ----

func TestApprovalFileAfterInboxReArmsPacket(t *testing.T) {
	fx := newGitFixture(t)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), credsPacket("M01-T01", "R-01"))
	st := loadFixtureState(t, fx)
	st.Packets["M01-T01"].Status = "inbox"
	st.Status = "escalated"
	saveStateObj(t, fx, st)
	writeApproval(t, fx, "M01-T01", `{"approved-env": ["FLY_TEST_KEY_A"]}`)

	var logs []string
	opts := runOptsWithLog(fx, happyDrivers().set(), &logs)
	opts.Env = testEnvKeys
	summary, err := RunTask(opts)
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)
	assert.Contains(t, strings.Join(logs, "\n"), "approval file appeared after inbox escalation — re-arming")

	st = loadFixtureState(t, fx)
	cred, ok := st.Packets["M01-T01"].Evidence.Credentials.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, cred["injected-env"], "FLY_TEST_KEY_A")
}

// ---- 入口冲突 / 单步错误路径 ----

func TestRunEntriesRejectRunningOrchestrator(t *testing.T) {
	fx := newGitFixture(t)
	pidFile := PidFile(fx.taskDir)
	require.NoError(t, os.WriteFile(pidFile, []byte(fmt.Sprintf(`{"pid":%d}`, os.Getpid())), 0o644))
	release, err := AcquireRunLease(fx.taskDir)
	require.NoError(t, err)
	defer release()

	summary, err := RunTask(runOpts(fx, happyDrivers().set()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "another orchestrator holds the task lease")
	assert.Empty(t, summary.Slug, "lease rejection precedes context/state loading")

	sSummary, err := RunSingleStep(stepOpts(fx, happyDrivers().set()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "another orchestrator")
	assert.Empty(t, sSummary.Slug)
}

func TestRunSingleStepErrorPaths(t *testing.T) {
	t.Run("自动选包执行", func(t *testing.T) {
		fx := newGitFixture(t)
		opts := stepOpts(fx, happyDrivers().set())
		opts.Only = "execute" // 无 PacketID → currentMilestone + nextActionablePacket
		summary, err := RunSingleStep(opts)
		require.NoError(t, err)
		assert.True(t, summary.Step.OK)
		st := loadFixtureState(t, fx)
		// v0.24 闭环语义：execute 环到 L1 绿（同上测试）。
		assert.Equal(t, "l1-passed", st.Packets["M01-T01"].Status)
	})

	t.Run("未知 --only 步骤", func(t *testing.T) {
		fx := newGitFixture(t)
		opts := stepOpts(fx, happyDrivers().set())
		opts.Only = "bogus"
		_, err := RunSingleStep(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown --only step")
	})

	t.Run("未知单元", func(t *testing.T) {
		fx := newGitFixture(t)
		opts := stepOpts(fx, happyDrivers().set())
		opts.Only = "verify"
		opts.PacketID = "NOPE-T99"
		_, err := RunSingleStep(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown packet/unit")
	})

	t.Run("评审前必须有绿灯 L1", func(t *testing.T) {
		fx := newGitFixture(t)
		opts := stepOpts(fx, happyDrivers().set())
		opts.Only = "review"
		opts.PacketID = "M01-T01"
		_, err := RunSingleStep(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no PASSING L1 evidence")
	})

	t.Run("无开启里程碑", func(t *testing.T) {
		fx := newGitFixture(t)
		st := loadFixtureState(t, fx)
		ms := st.Milestones["M-01"]
		ms.Status = "closed"
		st.Milestones["M-01"] = ms
		st.ContractFingerprints = unitTestContext(t, fx).State.ContractFingerprints
		saveStateObj(t, fx, st)

		opts := stepOpts(fx, happyDrivers().set())
		opts.Only = "plan"
		_, err := RunSingleStep(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no open milestone")

		opts.Only = "execute"
		_, err = RunSingleStep(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no open milestone")
	})

	t.Run("无可推进单元", func(t *testing.T) {
		fx := newGitFixture(t)
		st := loadFixtureState(t, fx)
		for id := range st.Packets {
			st.Packets[id].Status = "completed"
		}
		st.ContractFingerprints = unitTestContext(t, fx).State.ContractFingerprints
		saveStateObj(t, fx, st)

		opts := stepOpts(fx, happyDrivers().set())
		opts.Only = "execute"
		_, err := RunSingleStep(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no actionable packet")
	})

	t.Run("verify 全量巡检", func(t *testing.T) {
		fx := newGitFixture(t)
		opts := stepOpts(fx, happyDrivers().set())
		opts.Only = "verify" // 无 PacketID → 遍历未完成单元 + 里程碑闸门
		summary, err := RunSingleStep(opts)
		require.NoError(t, err)
		require.NotNil(t, summary.Step)
		assert.Equal(t, "verify", summary.Step.Name)
		assert.False(t, summary.Step.OK, "产物缺失 → 巡检失败")
	})
}

// ---- planner 破坏契约 → reloadPackets 错误上抛 ----

// TestPlanLoopInvalidContractFeedsBackThenInbox（v0.24 闭环化改写）：planner
// 持续写坏契约 → plan 子环反馈重试（界 = 2）→ 界尽 inbox 叫人（不是 fail-loud
// error——契约质量问题是结构性的，闭环出口是升级给人）。
func TestPlanLoopInvalidContractFeedsBackThenInbox(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	calls := 0
	sd.planFn = func(drivers.CallArgs) drivers.Result {
		calls++
		writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T03.md"), "::: broken [frontmatter")
		return drivers.MockResult(nil)
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err, "闭环出口是 inbox 升级，不是错误上抛")
	assert.Equal(t, planValidationRetries+1, calls, "初次 + 2 轮反馈重试")
	assert.Equal(t, "escalated", summary.Status)
	inboxRaw, ierr := os.ReadFile(filepath.Join(fx.taskDir, "inbox.md"))
	require.NoError(t, ierr)
	assert.Contains(t, string(inboxRaw), "invalid contracts", "界尽必须 inbox 留痕")
}

// ---- 里程碑闸门失败 ----

func TestMilestoneGateFailureEscalates(t *testing.T) {
	fx := newGitFixture(t)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), gatedPacket("M01-T01", "R-01"))
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T02.md"), gatedPacket("M01-T02", "R-02"))

	summary, err := RunTask(runOpts(fx, happyDrivers().set()))
	require.NoError(t, err)
	assert.Equal(t, "escalated", summary.Status)

	st := loadFixtureState(t, fx)
	gate, ok := st.Milestones["M-01"].Gate.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, gate["pass"])
	levels, _ := gate["levels"].(map[string]any)
	l2, _ := levels["L2"].(map[string]any)
	assert.Equal(t, false, l2["pass"])
	l3, _ := levels["L3"].(map[string]any)
	assert.Equal(t, true, l3["pass"], "L3 独立执行")
	last, _ := st.Inbox[len(st.Inbox)-1].(map[string]any)
	assert.Contains(t, last["reason"], "gate (L2/L3) failed")
}

// ---- judge 错误码升级（无 verdict 细节 → see metrics）----

func TestJudgeErrorCodeEscalatesWithSeeMetrics(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.judgeFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) { r.Text = "garbage, no verdict" })
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "escalated", summary.Status)

	st := loadFixtureState(t, fx)
	last, _ := st.Inbox[len(st.Inbox)-1].(map[string]any)
	assert.Contains(t, last["reason"], "judge verdict: EMPTY_RESPONSE")
	assert.Contains(t, last["reason"], "see metrics")
}

// ---- 合并失败：保留 worktree 待人工处理 ----

func TestMergeConflictRoutesToInboxWithoutCompleted(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	reviews := 0
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		reviews++
		if reviews == 1 {
			// 单元在飞期间主 checkout 被推进 → ff-only 失败。
			writeFile(t, filepath.Join(fx.root, "stray-commit.txt"), "user WIP landed mid-unit")
			gitCmd(t, fx.root, "add", "-A")
			gitCmd(t, fx.root, "commit", "-q", "-m", "chore: user commit mid-unit")
		}
		return sd.mock.Review(a)
	}

	var logs []string
	summary, err := RunTask(runOptsWithLog(fx, sd.set(), &logs))
	require.NoError(t, err, "合并冲突走 inbox 路由（与其他升级同款），不是进程错误")
	assert.Equal(t, "escalated", summary.Status)

	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	assert.Equal(t, "inbox", ps.Status, "§5.2：合并失败绝不标 completed")
	assert.Equal(t, "escalated", st.Status)
	commit, _ := ps.Commit.(map[string]any)
	require.NotNil(t, commit, "commit 记录必须落账")
	require.NotNil(t, commit["merge-error"], "合并错误落账")
	require.NotNil(t, commit["ledger-sha"], "账本照常落库：合并失败后 ledger 提交仍执行")
	found := false
	for _, e := range st.Inbox {
		entry, _ := e.(map[string]any)
		if r, _ := entry["reason"].(string); strings.Contains(r, "commit/merge failed") {
			found = true
		}
	}
	assert.True(t, found, "合并失败必须升级进 inbox")
	_, wtPath := verify.WorktreeNames(fx.root, fx.slug, "M01-T01")
	assert.DirExists(t, wtPath, "合并失败的 worktree 必须保留")
}

// TestCompleteUnitAheadCountFailure：base-ref 指向不存在的对象 →
// rev-list 失败 → ErrMergeConflict（review #1 的 aerr 分支）。
func TestCompleteUnitAheadCountFailure(t *testing.T) {
	fx := newGitFixture(t)
	id := "M01-T01"
	head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
	branch, wtPath := verify.WorktreeNames(fx.root, fx.slug, id)
	gitCmd(t, fx.root, "worktree", "add", "-f", "-b", branch, wtPath, head)
	writeFile(t, filepath.Join(wtPath, "w.txt"), "w")
	gitCmd(t, wtPath, "add", "-A")
	gitCmd(t, wtPath, "commit", "-q", "-m", "unit work")

	ctx := unitTestContext(t, fx)
	ps := ctx.State.Packets[id]
	bad := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	ps.BaseRef = &bad
	ps.Worktree = &state.WorktreeInfo{Mode: "worktree", Path: wtPath, Branch: branch}

	seedVerification(t, ctx, id)
	err := ctx.completeUnit(ctx.Packets[id])
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMergeConflict)
}

// ---- completeUnit：零变更 → commit skipped ----

func TestCompleteUnitCommitSkippedWhenNothingChanged(t *testing.T) {
	fx := newGitFixture(t)
	id := "M01-T01"
	// 预提交与 mock 产出逐字节一致的交付物。
	writeFile(t, filepath.Join(fx.root, "build", id+".txt"), "ok "+id+"\n")
	payload, err := json.MarshalIndent(map[string]any{
		"requirements-covered": []string{},
		"changed-files":        []string{"build/" + id + ".txt"},
		"commands-run":         []string{},
		"failures":             []string{},
	}, "", "  ")
	require.NoError(t, err)
	writeFile(t, filepath.Join(fx.taskDir, "reports", id+".done.json"), string(payload)+"\n")
	gitCmd(t, fx.root, "add", "-A")
	gitCmd(t, fx.root, "commit", "-q", "-m", "chore: seed deliverables")

	ctx := unitTestContext(t, fx)
	head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
	branch, wtPath := verify.WorktreeNames(fx.root, fx.slug, id)
	gitCmd(t, fx.root, "worktree", "add", "-f", "-b", branch, wtPath, head)

	ps := ctx.State.Packets[id]
	ps.Status = "l1-passed"
	ps.BaseRef = &head
	ps.Worktree = &state.WorktreeInfo{Mode: "worktree", Path: wtPath, Branch: branch}

	seedVerification(t, ctx, id)
	require.NoError(t, ctx.completeUnit(ctx.Packets[id]))
	commit, _ := ps.Commit.(map[string]any)
	assert.Equal(t, head, commit["sha"], "零变更仍记录已验证的持久化 HEAD")
	// ahead==0（分支与 base 同点）：无合并发生 → 无 merge-error 记录
	// （review 修正后的语义：merge-error 仅在真正尝试合并失败时落账）。
	assert.Nil(t, commit["merge-error"])
	retro, rerr := os.ReadFile(filepath.Join(fx.taskDir, "retro.md"))
	require.NoError(t, rerr)
	assert.Contains(t, string(retro), "commit "+shortSHA(head))
}

// ---- 缺陷注入 ----

func TestDefectInjectionMissingConfigContinues(t *testing.T) {
	fx := newGitFixture(t)
	patchConfigFile(t, fx, diConfigJSON("", ""))

	summary, err := RunTask(runOpts(fx, happyDrivers().set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status, "缺配置进 inbox 叫人但单元继续")

	st := loadFixtureState(t, fx)
	assert.NotEmpty(t, st.Inbox)
	ps := st.Packets["M01-T01"]
	di, ok := ps.Evidence.DefectInjection.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "missing-config", di["skipped"])
}

func TestDefectInjectionUninjectableAndBaselineRed(t *testing.T) {
	t.Run("补丁不可读", func(t *testing.T) {
		fx := newGitFixture(t)
		patchConfigFile(t, fx, diConfigJSON("/nonexistent/defect.patch", "true"))

		summary, err := RunTask(runOpts(fx, happyDrivers().set()))
		require.NoError(t, err)
		assert.Equal(t, "completed", summary.Status)
		st := loadFixtureState(t, fx)
		di, _ := st.Packets["M01-T01"].Evidence.DefectInjection.(map[string]any)
		assert.Equal(t, "uninjectable", di["outcome"])
	})

	t.Run("基线即红", func(t *testing.T) {
		fx := newGitFixture(t, "notes.txt")
		patch := filepath.Join(t.TempDir(), "defect.patch")
		writeFile(t, patch, makeDefectPatch(t, fx))
		patchConfigFile(t, fx, diConfigJSON(patch, "exit 1"))

		summary, err := RunTask(runOpts(fx, happyDrivers().set()))
		require.NoError(t, err)
		assert.Equal(t, "completed", summary.Status)
		st := loadFixtureState(t, fx)
		di, _ := st.Packets["M01-T01"].Evidence.DefectInjection.(map[string]any)
		assert.Equal(t, "baseline-red", di["outcome"])
	})
}

func TestDefectInjectionFakeTestsStopsUnit(t *testing.T) {
	fx := newGitFixture(t, "notes.txt")
	patch := filepath.Join(t.TempDir(), "defect.patch")
	writeFile(t, patch, makeDefectPatch(t, fx))
	patchConfigFile(t, fx, diConfigJSON(patch, "true"))

	var logs []string
	summary, err := RunTask(runOptsWithLog(fx, happyDrivers().set(), &logs))
	require.NoError(t, err)
	assert.Equal(t, "escalated", summary.Status)

	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	assert.Equal(t, "inbox", ps.Status)
	di, _ := ps.Evidence.DefectInjection.(map[string]any)
	assert.Equal(t, "fake-tests", di["outcome"])
	last, _ := st.Inbox[len(st.Inbox)-1].(map[string]any)
	assert.Contains(t, last["reason"], "fake/weak tests proven")

	// 注入后工作区已还原：补丁反向应用成功。
	content, err := os.ReadFile(filepath.Join(fx.root, "notes.txt"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), "injected defect")
}

// ---- retro 里程碑摘要（§5.3）----

func TestSummaryLineInterventionsAndHistory(t *testing.T) {
	fx := newGitFixture(t)
	st := loadFixtureState(t, fx)
	st.Inbox = append(st.Inbox,
		map[string]any{"milestone": "M-01", "reason": "m-level intervention"},
		map[string]any{"packet-id": "M01-T02", "reason": "packet-level intervention"},
		map[string]any{"packet-id": "BOGUS-ID", "reason": "无法解析归属"},
		"junk-non-map",
	)
	st.Packets["M01-T02"].RetryHistory = append(st.Packets["M01-T02"].RetryHistory,
		map[string]any{"error-code": "REVIEW_RETURNED", "policy": "requeue"},
		map[string]any{"policy": "return-to-planner"},
		"junk-entry",
	)
	st.Packets["M01-T02"].PlanRevisions = 1
	saveStateObj(t, fx, st)

	summary, err := RunTask(runOpts(fx, happyDrivers().set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)

	retro, err := os.ReadFile(filepath.Join(fx.taskDir, "retro.md"))
	require.NoError(t, err)
	text := string(retro)
	assert.Contains(t, text, "human-interventions=2", "里程碑条目 + 可解析 packet 条目")
	assert.Contains(t, text, "defects(review-failed)=1")
	assert.Contains(t, text, "scope-changes=1")
	assert.Contains(t, text, "reopens=1")
}

// TestExecuteLoopRetriesUntilGreen（v0.24）：execute 子环闭环——首轮 spawn 失败
// （传输级）→ 反馈重试 → 第二轮成功 → L1 绿 → 环达标。旧单步语义会在首轮失败
// 后直接返回。
func TestExecuteLoopRetriesUntilGreen(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	calls := 0
	md := drivers.NewMockDriver(nil) // 成功轮委托真 mock：交付物（artifact+done-report）齐全
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		calls++
		if calls == 1 {
			return drivers.MockResult(func(r *drivers.Result) {
				code := 7
				r.ExitCode, r.Settled = &code, false
				msg := "transient boom"
				r.ErrorMessage = &msg
			})
		}
		return md.Execute(a)
	}
	opts := stepOpts(fx, sd.set())
	opts.Only = "execute"
	opts.PacketID = "M01-T01"
	summary, err := RunSingleStep(opts)
	require.NoError(t, err)
	assert.True(t, summary.Step.OK, "环必须转到 L1 绿（第二轮成功）")
	assert.Equal(t, 2, calls)
	st := loadFixtureState(t, fx)
	assert.Equal(t, "l1-passed", st.Packets["M01-T01"].Status)
}

// TestExecuteLoopBoundedStop（v0.24）：持续失败 → FailedAttempts 耗尽 → 有界
// 停止（单环模式不自动升级，人裁决）。Step.OK=false + Detail 报告界。
func TestExecuteLoopBoundedStop(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.execFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			code := 7
			r.ExitCode, r.Settled = &code, false
			msg := "always fails"
			r.ErrorMessage = &msg
		})
	}
	opts := stepOpts(fx, sd.set())
	opts.Only = "execute"
	opts.PacketID = "M01-T01"
	summary, err := RunSingleStep(opts)
	require.NoError(t, err)
	assert.False(t, summary.Step.OK)
	require.NotNil(t, summary.Step.Detail)
	// An unsettled model failure has no trustworthy candidate/check observation.
	// It must stop at the original attempt bound, without inventing early spin.
	assert.Contains(t, *summary.Step.Detail, "bounded stop")
	st := loadFixtureState(t, fx)
	assert.Equal(t, 3, st.Packets["M01-T01"].FailedAttempts, "unknown progress does not reset or bypass the attempt bound")
	assert.NotEqual(t, "escalated", st.Status, "单环模式不自动升级（人在驾驶座）")
}

// TestExecuteLoopBoundedVaryingSignatures（v0.24）：失败签名各不相同（空转
// 检测不触发）→ max-retries 界先耗尽 → Bounded 出口（区别于 Spun 出口）。
func TestExecuteLoopBoundedVaryingSignatures(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	// 每轮一个互不相同的不可重试错误码（按 state.json 的 FailedAttempts 选）：
	// 非重试码让 executeStep 内部传输环单发即停，轮级码互异 → 空转签名不同。
	msgs := []string{"401 unauthorized", "403 quota forbidden", "400 invalid malformed request"}
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		msg := msgs[0]
		if raw, rerr := os.ReadFile(filepath.Join(fx.taskDir, "state.json")); rerr == nil {
			var st struct {
				Packets map[string]struct {
					FailedAttempts int `json:"failed-attempts"`
				} `json:"packets"`
			}
			if json.Unmarshal(raw, &st) == nil {
				if ps, ok := st.Packets[a.PacketID]; ok && ps.FailedAttempts < len(msgs) {
					msg = msgs[ps.FailedAttempts]
				}
			}
		}
		return drivers.MockResult(func(r *drivers.Result) {
			code := 7
			r.ExitCode, r.Settled = &code, false
			r.ErrorMessage = &msg
		})
	}
	opts := stepOpts(fx, sd.set())
	opts.Only = "execute"
	opts.PacketID = "M01-T01"
	summary, err := RunSingleStep(opts)
	require.NoError(t, err)
	assert.False(t, summary.Step.OK)
	require.NotNil(t, summary.Step.Detail)
	assert.Contains(t, *summary.Step.Detail, "bounded stop", "不同签名失败走 max-retries 界（非空转）")
	st := loadFixtureState(t, fx)
	assert.Equal(t, 3, st.Packets["M01-T01"].FailedAttempts)
}

// TestExecuteLoopBudgetStop（v0.24）：预算硬闸 → 子环 Stopped 出口。
func TestExecuteLoopBudgetStop(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	opts := stepOpts(fx, sd.set())
	var limit int64 = 1
	opts.Config.Budget.Enforce = "hard"
	opts.Config.Budget.DailyTokenLimit = &limit
	// 预置超限：mock driver 零 usage，靠既有账目触发硬闸。
	sp := filepath.Join(fx.taskDir, "state.json")
	raw, rerr := os.ReadFile(sp)
	require.NoError(t, rerr)
	var st map[string]any
	require.NoError(t, json.Unmarshal(raw, &st))
	bud := st["budget"].(map[string]any)
	// CheckBudget 看当日桶（budget.daily[<day>].tokens），不是 totals。
	daily, _ := bud["daily"].(map[string]any)
	if daily == nil {
		daily = map[string]any{}
		bud["daily"] = daily
	}
	day := time.Now().UTC().Format("2006-01-02")
	daily[day] = map[string]any{"tokens": float64(999), "cost-usd": float64(0)}
	require.NoError(t, os.WriteFile(sp, mustJSON(st), 0o644))
	opts.Only = "execute"
	opts.PacketID = "M01-T01"
	summary, err := RunSingleStep(opts)
	require.NoError(t, err)
	assert.False(t, summary.Step.OK)
	require.NotNil(t, summary.Step.Detail)
	assert.Contains(t, *summary.Step.Detail, "stopped")
}

// TestRunExecuteLoopFatalError（v0.24）：ctx.fatal → 子环 Stopped + 错误上抛。
func TestRunExecuteLoopFatalError(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	ctx := verdictCtx(t, fx, sd)
	ctx.fatal = fmt.Errorf("disk gone")
	_, err := ctx.runExecuteLoop(ctx.Packets["M01-T01"])
	require.ErrorContains(t, err, "disk gone")
}
