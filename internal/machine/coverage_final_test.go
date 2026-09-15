package machine

// 覆盖率收尾（2026-08-23）：internal/machine 94.2% → 100% 的最后一批零覆盖块。
//
// 手法备忘：
//   - 「保存失败」类分支统一用只读目录注入：chmod 0555 taskDir 让下一次
//     save() 失败并把错误记入 ctx.fatal（短路语义见 context.go save），
//     t.Cleanup 恢复权限。
//   - 升级/spin 各臂优先直测 processPacket / judgeStep / reviewStep（同包
//     直呼，与 coverage_steps_test.go 同风格）；需要完整编排的走公开 RunTask。
//   - 凭证审批双分支（超时升级 / 审批文件出现后重武装）由前任覆盖
//     （TestExecuteStepStopsWhenCredentialsNeverApproved、
//     TestApprovalFileAfterInboxReArmsPacket），此处不重复。

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- 只读目录注入 ----

// roDir 把 dir 变为不可写，测试结束后恢复。下一次 state.json 写入即失败，
// save() 将错误记入 ctx.fatal（此后全部保存短路）。
func roDir(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

// rewriteConfig 基于 fixture 配置做结构性改写后整体落盘（patchConfigFile 是
// 整文件替换语义）。
func rewriteConfig(t *testing.T, fx fixture, mutate func(m map[string]any)) {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(fixtureConfigJSON), &m))
	mutate(m)
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	patchConfigFile(t, fx, string(raw))
}

func dropMapKey(t *testing.T, m map[string]any, outer, key string) {
	t.Helper()
	inner, ok := m[outer].(map[string]any)
	require.True(t, ok, "%s 应为对象", outer)
	delete(inner, key)
}

// maxFilesPacket 把 packet 边界收紧到 max 个变更文件（executor 多写一个文件
// 即触发 max-files 违规——违规项只有 Count 没有 Path）。
func maxFilesPacket(id, requirement string, max int) string {
	return strings.Replace(fixturePacket(id, requirement),
		"max-files: 5", fmt.Sprintf("max-files: %d", max), 1)
}

// ---- CreateContext 缺省臂 ----

func TestCreateContextDefaultsNoLog(t *testing.T) {
	fx := newGitFixture(t)
	ctx, err := CreateContext(Opts{
		TaskDir: fx.taskDir, Cwd: fx.root, Config: configFor(fx),
		Drivers: happyDrivers().set(),
		Sleep:   func(time.Duration) {}, Now: func() time.Time { return time.Unix(0, 0) },
		// Log 留空 → 走缺省 no-op 臂。
	})
	require.NoError(t, err)
	require.NotNil(t, ctx.Log)
	ctx.Log("should not panic")
}

// ---- 入口：CreateContext 错误上抛 ----

func TestEntryPointsSurfaceCreateContextErrors(t *testing.T) {
	empty := t.TempDir() // 无 spec.md → LoadSpec 失败

	cfg := config.Default()
	_, err := RunTask(RunTaskOpts{RunOpts: RunOpts{TaskDir: empty, Config: &cfg}})
	require.Error(t, err)

	_, err = RunSingleStep(RunSingleStepOpts{RunOpts: RunOpts{TaskDir: empty, Config: &cfg}})
	require.Error(t, err)
}

// ---- run()：closed 里程碑跳过 ----

func TestRunTaskSkipsClosedMilestones(t *testing.T) {
	fx := newGitFixture(t)
	st := loadFixtureState(t, fx)
	ms := st.Milestones["M-01"]
	ms.Status = "closed"
	st.Milestones["M-01"] = ms
	saveStateObj(t, fx, st)

	sd := happyDrivers()
	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)
	sd.log.mu.Lock()
	defer sd.log.mu.Unlock()
	assert.Empty(t, sd.log.plan, "closed 里程碑绝不重跑 plan")
}

// ---- run()：planStep 错误 / Stopped / !OK 臂 ----

func TestRunTaskPlanBindingMissingSurfacesFromRun(t *testing.T) {
	fx := newGitFixture(t)
	rewriteConfig(t, fx, func(m map[string]any) { dropMapKey(t, m, "loops", "plan") })

	_, err := RunTask(runOpts(fx, happyDrivers().set()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no loop binding")
}

func TestRunTaskPlanStoppedBySeededBudget(t *testing.T) {
	fx := newGitFixture(t)
	rewriteConfig(t, fx, func(m map[string]any) {
		m["budget"] = map[string]any{"enforce": "hard", "daily-token-limit": 50}
	})
	st := loadFixtureState(t, fx)
	limit := int64(50)
	st.Budget.Enforce = "hard"
	st.Budget.DailyTokenLimit = &limit
	st.Budget.Daily[time.Now().UTC().Format("2006-01-02")] = state.DailyBucket{Tokens: 100}
	saveStateObj(t, fx, st)

	summary, err := RunTask(runOpts(fx, happyDrivers().set()))
	require.NoError(t, err, "预算停机不是错误")
	assert.Equal(t, "budget-stopped", summary.Status, "plan 循环顶的 budgetGate 直接停机")
}

func TestRunTaskPlanNotOKSaveFailureSurfaces(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	calls := 0
	sd.planFn = func(drivers.CallArgs) drivers.Result {
		calls++
		if calls == 3 { // maxTransport=2 → 第 3 次=最后一次尝试
			roDir(t, fx.taskDir) // 之后 run() 的 inbox 落盘必然失败
		}
		return drivers.MockResult(func(r *drivers.Result) {
			code := 1
			r.ExitCode = &code
			msg := "provider 500 exploded"
			r.ErrorMessage = &msg
			r.Settled = false
		})
	}

	_, err := RunTask(runOpts(fx, sd.set()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	assert.Equal(t, 3, calls)
}

func TestRunTaskFatalAfterPlanSessionSaveSurfacesAtLoopTop(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.planFn = func(drivers.CallArgs) drivers.Result {
		res := drivers.MockResult(func(r *drivers.Result) { r.SessionID = "sess-final" })
		roDir(t, fx.taskDir) // planStep 尾部的会话句柄保存失败 → fatal
		return res
	}

	_, err := RunTask(runOpts(fx, sd.set()))
	require.Error(t, err, "plan OK 但会话保存失败 → 包循环顶的 fatal 检查上抛")
	assert.Contains(t, err.Error(), "permission denied")
}

// ---- processPacket：execute 错误传播 ----

func TestRunTaskExecuteRouteMissingPropagatesThroughProcessPacket(t *testing.T) {
	fx := newGitFixture(t)
	rewriteConfig(t, fx, func(m map[string]any) { dropMapKey(t, m, "routes", "default") })

	_, err := RunTask(runOpts(fx, happyDrivers().set()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no route binding")
}

// ---- processPacket：inbox 重跑臂 + fatal ----

func TestProcessPacketInboxBranchShortCircuitsOnFatal(t *testing.T) {
	fx := newGitFixture(t)
	ctx := unitTestContext(t, fx)
	ctx.State.Packets["M01-T01"].Status = "inbox"
	ctx.State.Status = "escalated"
	ctx.fatal = errors.New("disk on fire")

	outcome, err := ctx.processPacket(ctx.Packets["M01-T01"], processOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk on fire")
	assert.Empty(t, outcome)
}

// ---- processPacket：循环顶升级两臂 ----

func TestProcessPacketLoopTopEscalationArms(t *testing.T) {
	t.Run("未知里程碑上抛", func(t *testing.T) {
		fx := newGitFixture(t)
		ctx := unitTestContext(t, fx)
		ctx.State.Packets["M01-T01"].FailedAttempts = 3 // max-retries=3
		ctx.State.Packets["M01-T01"].Milestone = "M-99"

		_, err := ctx.processPacket(ctx.Packets["M01-T01"], processOpts{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "references unknown milestone")
	})

	t.Run("planner 不可用直奔 inbox 停机", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		sd.planFn = func(drivers.CallArgs) drivers.Result {
			return drivers.MockResult(func(r *drivers.Result) {
				code := 1
				r.ExitCode = &code
				msg := "server down"
				r.ErrorMessage = &msg
				r.Settled = false
			})
		}
		ctx := verdictCtx(t, fx, sd)
		ctx.State.Packets["M01-T01"].FailedAttempts = 3

		outcome, err := ctx.processPacket(ctx.Packets["M01-T01"], processOpts{})
		require.NoError(t, err)
		assert.Equal(t, "stop", outcome)
		assert.Equal(t, "inbox", ctx.State.Packets["M01-T01"].Status)
	})
}

// TestMaxRetriesWithDistinctCodesReachesLoopTopEscalation
// 每轮三次尝试用互不相同族别的错误码（SERVER/RATE_LIMIT/TIMEOUT），签名恒异
// → spin 永不触发 → FailedAttempts 真正走到循环顶升级：首轮 return-to-planner
// 修订、次轮 revisions>=1 直奔 inbox 停机。
func TestMaxRetriesWithDistinctCodesReachesLoopTopEscalation(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	spawns := 0
	families := []string{
		"provider 500 exploded",       // SERVER
		"429 rate limit exceeded",     // RATE_LIMIT
		"operation deadline exceeded", // TIMEOUT
	}
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		spawns++
		msg := families[((spawns-1)/3)%3] // 每个 retryable attempt 恰好烧 3 次 spawn
		return drivers.MockResult(func(r *drivers.Result) {
			code := 1
			r.ExitCode = &code
			m := msg
			r.ErrorMessage = &m
			r.Settled = false
		})
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "escalated", summary.Status)
	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	assert.Equal(t, "inbox", ps.Status)
	assert.GreaterOrEqual(t, ps.PlanRevisions, 1)
	codes := map[string]bool{}
	for _, rh := range ps.RetryHistory {
		entry, _ := rh.(map[string]any)
		if ec, _ := entry["error-code"].(string); ec != "" && entry["scope"] == "attempt" {
			codes[ec] = true
		}
	}
	assert.GreaterOrEqual(t, len(codes), 3, "三个族别的码都进了审计史：%v", codes)
}

// ---- processPacket：execute 失败 spin → escalate 错误臂 ----

func TestProcessPacketExecuteSpinEscalateErrorArm(t *testing.T) {
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
	ctx := verdictCtx(t, fx, sd)
	ctx.State.Packets["M01-T01"].Milestone = "M-99" // spin 升级必炸

	_, err := ctx.processPacket(ctx.Packets["M01-T01"], processOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "references unknown milestone")
}

// TestL1BoundarySpinEscalationArms
// executor 恒多写一个杂散文件 + max-files=1 → L1 边界违规（只有 Count 没有
// Path 的格式化臂）且签名逐轮相同 → 第二次 L1 失败即 spin：首轮修订（重载
// 契约 + packet 重指），次轮 revisions>=1 → inbox 停机。
func TestL1BoundarySpinEscalationArms(t *testing.T) {
	fx := newGitFixture(t)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), maxFilesPacket("M01-T01", "R-01", 1))
	sd := happyDrivers()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		res := sd.mock.Execute(a)
		writeFile(t, filepath.Join(a.Cwd, "build", "zz-extra.txt"), "noise\n")
		return res
	}

	var logs []string
	summary, err := RunTask(runOptsWithLog(fx, sd.set(), &logs))
	require.NoError(t, err)
	assert.Equal(t, "escalated", summary.Status, "logs: %v", logs)
	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	assert.Equal(t, "inbox", ps.Status)
	assert.GreaterOrEqual(t, ps.PlanRevisions, 1, "spin 首轮先修订")
	l1, ok := ps.Evidence.L1.(map[string]any)
	require.True(t, ok)
	boundary, _ := l1["boundary"].(map[string]any)
	require.NotNil(t, boundary)
	violations, _ := boundary["violations"].([]any)
	require.NotEmpty(t, violations)
	found := false
	for _, v := range violations {
		vm, _ := v.(map[string]any)
		if vm["type"] == "max-files" {
			path, _ := vm["path"].(string)
			count, _ := vm["count"].(float64)
			found = count >= 2 && path == ""
		}
	}
	assert.True(t, found, "违规必须是 Count 臂（无 Path）：%v", violations)
}

// TestL1SpinEscalateErrorArm：spin 升级撞上未知里程碑 → 错误上抛。
func TestL1SpinEscalateErrorArm(t *testing.T) {
	fx := newGitFixture(t)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), maxFilesPacket("M01-T01", "R-01", 1))
	sd := happyDrivers()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		res := sd.mock.Execute(a)
		writeFile(t, filepath.Join(a.Cwd, "build", "zz-extra.txt"), "noise\n")
		return res
	}
	ctx := verdictCtx(t, fx, sd)
	ctx.State.Packets["M01-T01"].Milestone = "M-99"

	_, err := ctx.processPacket(ctx.Packets["M01-T01"], processOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "references unknown milestone")
}

// TestL1GateFailWithSkippedBoundary：非 git 工作区 diff 不可用 → 边界 skipped；
// executor 每次都抹掉交付物 → 闸门命令恒败。失败路径上的
// `Boundary.Skipped → 权威 changedFiles 置 nil`（兜底链接管）在此被真实执行。
func TestL1GateFailWithSkippedBoundary(t *testing.T) {
	fx := newFixture(t) // 不 init git → GetChangedFiles 不可用 → Skipped
	sd := happyDrivers()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		res := sd.mock.Execute(a)
		require.NoError(t, os.Remove(filepath.Join(a.Cwd, "build", a.PacketID+".txt")))
		return res
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "escalated", summary.Status)
	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	assert.Equal(t, "inbox", ps.Status)
	l1, ok := ps.Evidence.L1.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, l1["pass"], "闸门命令失败必须判 L1 失败")
	boundary, _ := l1["boundary"].(map[string]any)
	assert.Equal(t, true, boundary["skipped"])
}

// ---- 缺陷注入：test-file-patterns 喂采样 ----

func TestDefectInjectionTestFilePatternsFeedSampling(t *testing.T) {
	// 非 git fixture：L1 边界 skipped（放行），maybeDefectInjection 落到
	// executor 自报 changed-files 兜底——模式循环与命中臂在此被真实执行而
	// 不破坏 L1（git 场景下命中模式的文件会构成 test-file 违规）。
	fx := newFixture(t)
	sd := happyDrivers()
	o := runOpts(fx, sd.set())
	o.Config.DefectInjection.EveryNUnits = new(1)
	o.Config.Verify.TestFilePatterns = []string{`\.txt$`} // mock 自报交付物必命中

	summary, err := RunTask(o)
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)
	st := loadFixtureState(t, fx)
	diMap, ok := st.Packets["M01-T01"].Evidence.DefectInjection.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "missing-config", diMap["skipped"], "模式命中后照常抽样并进 missing-config 臂")
}

// ---- 缺陷注入：RunDefectInjection 之后的 fatal 臂 ----

func TestMaybeDefectInjectionSaveFailureAfterRun(t *testing.T) {
	fx := newGitFixture(t)
	ctx := unitTestContext(t, fx)
	ctx.Config.DefectInjection.EveryNUnits = new(1)
	ctx.Config.DefectInjection.Patch = writeDefectPatch(t, &fx)
	ctx.Config.DefectInjection.TestCmd = "true" // 注入后仍绿 → fake-tests
	ctx.fatal = errors.New("disk on fire")

	oi, err := ctx.maybeDefectInjection(ctx.Packets["M01-T01"])
	require.Error(t, err)
	assert.Nil(t, oi, "fatal 时结论作废")
	assert.Contains(t, err.Error(), "disk on fire")
	require.NotNil(t, ctx.State.Packets["M01-T01"].Evidence.DefectInjection)
}

// TestProcessPacketDefectInjectionErrAfterL1SaveFailure
// execFn 在交付物落盘后把任务目录变只读 → l1Step 保存失败记 fatal → 抽样命中
// missing-config 臂 → maybeDefectInjection 以 fatal 出口 → processPacket 上抛。
func TestProcessPacketDefectInjectionErrAfterL1SaveFailure(t *testing.T) {
	fx := newGitFixture(t)
	patchConfigFile(t, fx, diConfigJSON("", ""))
	sd := happyDrivers()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		res := sd.mock.Execute(a)
		roDir(t, fx.taskDir)
		return res
	}

	_, err := RunTask(runOpts(fx, sd.set()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

// TestPauseBeforeReviewSaveFailureSurfaces：默认抽样（every-n=5，首单元不中）
// → DI 放行 → pause 分支的落盘失败上抛。
func TestPauseBeforeReviewSaveFailureSurfaces(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		res := sd.mock.Execute(a)
		roDir(t, fx.taskDir)
		return res
	}
	o := runOpts(fx, sd.set())
	o.PauseBefore = "review"

	_, err := RunTask(o)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

// ---- resume 到 review：缺陷注入补采样两臂 ----

func pauseAtReview(t *testing.T) (fixture, *scriptedDrivers) {
	t.Helper()
	fx := newGitFixture(t)
	sd := happyDrivers()
	o := runOpts(fx, sd.set())
	o.PauseBefore = "review"
	_, err := RunTask(o)
	require.NoError(t, err)
	return fx, sd
}

func TestResumeAtReviewDefectInjectionArms(t *testing.T) {
	t.Run("fake-tests 注入停机", func(t *testing.T) {
		fx, sd := pauseAtReview(t)
		ctx := unitTestContext(t, fx)
		ctx.Drivers = sd.set()
		ctx.Config.DefectInjection.EveryNUnits = new(1)
		ctx.Config.DefectInjection.Patch = writeDefectPatch(t, &fx)
		ctx.Config.DefectInjection.TestCmd = "true"

		outcome, err := ctx.processPacket(ctx.Packets["M01-T01"], processOpts{})
		require.NoError(t, err)
		assert.Equal(t, "stop", outcome)
		assert.Equal(t, "escalated", ctx.State.Status)
		assert.Equal(t, "inbox", ctx.State.Packets["M01-T01"].Status)
	})

	t.Run("missing-config 携 fatal 上抛", func(t *testing.T) {
		fx, sd := pauseAtReview(t)
		ctx := unitTestContext(t, fx)
		ctx.Drivers = sd.set()
		ctx.Config.DefectInjection.EveryNUnits = new(1) // patch/test-cmd 留空
		ctx.fatal = errors.New("disk on fire")

		_, err := ctx.processPacket(ctx.Packets["M01-T01"], processOpts{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disk on fire")
	})
}

// ---- reviewStep 四臂直测 ----

func TestReviewStepDirectArms(t *testing.T) {
	t.Run("绑定缺失上抛", func(t *testing.T) {
		fx := newGitFixture(t)
		ctx := verdictCtx(t, fx, happyDrivers())
		delete(ctx.Config.Loops, "review")
		_, err := ctx.reviewStep(ctx.Packets["M01-T01"], nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no loop binding")
	})

	t.Run("fatal 短路", func(t *testing.T) {
		fx := newGitFixture(t)
		ctx := verdictCtx(t, fx, happyDrivers())
		ctx.fatal = errors.New("disk on fire")
		_, err := ctx.reviewStep(ctx.Packets["M01-T01"], nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disk on fire")
	})

	t.Run("budget 停机", func(t *testing.T) {
		fx := newGitFixture(t)
		ctx := verdictCtx(t, fx, happyDrivers())
		limit := int64(10)
		ctx.Config.Budget.Enforce = "hard"
		ctx.Config.Budget.DailyTokenLimit = &limit
		ctx.State.Budget.Daily[time.Now().UTC().Format("2006-01-02")] = state.DailyBucket{Tokens: 99}

		out, err := ctx.reviewStep(ctx.Packets["M01-T01"], nil)
		require.NoError(t, err)
		assert.True(t, out.Stopped)
		assert.False(t, out.Pass)
	})

	t.Run("流内 AUTH 不重试", func(t *testing.T) {
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
		out, err := ctx.reviewStep(ctx.Packets["M01-T01"], nil)
		require.NoError(t, err)
		assert.False(t, out.Pass)
		assert.False(t, out.Stopped)
		assert.Equal(t, errs.AUTH, out.ErrorCode)
	})
}

// ---- review 绑定缺失经 processPacket/run 上抛 ----

func TestRunTaskReviewBindingMissingSurfaces(t *testing.T) {
	fx := newGitFixture(t)
	rewriteConfig(t, fx, func(m map[string]any) { dropMapKey(t, m, "loops", "review") })

	_, err := RunTask(runOpts(fx, happyDrivers().set()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no loop binding")
}

// TestRunTaskReviewStoppedByMidFlightBudgetStop
// review 首 spawn 报巨额用量且不可解析 → 退避后循环顶 budgetGate 硬停 →
// reviewOutcome{Stopped} 经 processPacket 停机。
func TestRunTaskReviewStoppedByMidFlightBudgetStop(t *testing.T) {
	fx := newGitFixture(t)
	rewriteConfig(t, fx, func(m map[string]any) {
		m["budget"] = map[string]any{"enforce": "hard", "daily-token-limit": 500}
	})
	sd := happyDrivers()
	sd.reviewFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			r.Text = "garbage, no verdict"
			// 记账取 Input+Output：360(plan+exec)+1000 > 500 → 退避后硬停。
			r.Usage = &drivers.Usage{Input: 400, Output: 600}
		})
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "budget-stopped", summary.Status)
	sd.log.mu.Lock()
	defer sd.log.mu.Unlock()
	assert.Len(t, sd.log.review, 1, "硬停后绝不二次 spawn review")
	assert.Len(t, sd.log.exec, 1)
}

// TestRunTaskCompleteUnitSaveFailureSurfaces：评审通过但完成落盘失败 → fatal
// 从 completeUnit 一路上抛。
func TestRunTaskCompleteUnitSaveFailureSurfaces(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		res := sd.mock.Review(a) // pass verdict
		roDir(t, fx.taskDir)     // reviewStep 尾部保存失败 → completeUnit 短路
		return res
	}

	_, err := RunTask(runOpts(fx, sd.set()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

// ---- verification-gap 升级两臂 ----

func TestVerificationGapEscalateArms(t *testing.T) {
	t.Run("未知里程碑上抛", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		sd.reviewFn = func(drivers.CallArgs) drivers.Result { return gapVerdict() }
		ctx := verdictCtx(t, fx, sd)
		ctx.State.Packets["M01-T01"].Milestone = "M-99"

		_, err := ctx.processPacket(ctx.Packets["M01-T01"], processOpts{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "references unknown milestone")
	})

	t.Run("planner 不可用直奔 inbox", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		sd.reviewFn = func(drivers.CallArgs) drivers.Result { return gapVerdict() }
		sd.planFn = func(drivers.CallArgs) drivers.Result {
			return drivers.MockResult(func(r *drivers.Result) {
				code := 1
				r.ExitCode = &code
				msg := "server down"
				r.ErrorMessage = &msg
				r.Settled = false
			})
		}
		ctx := verdictCtx(t, fx, sd)

		outcome, err := ctx.processPacket(ctx.Packets["M01-T01"], processOpts{})
		require.NoError(t, err)
		assert.Equal(t, "stop", outcome)
		assert.Equal(t, "inbox", ctx.State.Packets["M01-T01"].Status)
	})
}

// ---- review 返退 spin 升级臂 ----

// TestReviewReturnSpinEscalationArms：评审恒返退 → 相同签名第二次即 spin：
// 首轮修订、次轮 inbox 停机。
func TestReviewReturnSpinEscalationArms(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.reviewFn = func(drivers.CallArgs) drivers.Result { return returnVerdict() }

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "escalated", summary.Status)
	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	assert.Equal(t, "inbox", ps.Status)
	assert.GreaterOrEqual(t, ps.PlanRevisions, 1)
}

func TestReviewReturnSpinEscalateErrorArm(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.reviewFn = func(drivers.CallArgs) drivers.Result { return returnVerdict() }
	ctx := verdictCtx(t, fx, sd)
	ctx.State.Packets["M01-T01"].Milestone = "M-99"

	_, err := ctx.processPacket(ctx.Packets["M01-T01"], processOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "references unknown milestone")
}

// TestReviewErrorCodeOverridesDefault：显式错误码（流内 AUTH）覆盖
// REVIEW_RETURNED 默认值进审计史。
func TestReviewErrorCodeOverridesDefault(t *testing.T) {
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

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "escalated", summary.Status)
	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	found := false
	for _, rh := range ps.RetryHistory {
		entry, _ := rh.(map[string]any)
		if ec, _ := entry["error-code"].(string); ec == "AUTH" && entry["scope"] == "attempt" {
			found = true
		}
	}
	assert.True(t, found, "AUTH 必须替代 REVIEW_RETURNED 进审计史：%v", ps.RetryHistory)
}

// ---- SIGTERM handler：子进程自证 exit 143 ----

func TestSigtermHandlerExits143(t *testing.T) {
	if os.Getenv("FLY_SIGTERM_CHILD") == "1" {
		ch := registerSigterm(func(string) {})
		defer unregisterSigterm(ch)
		time.Sleep(200 * time.Millisecond) // handler goroutine 就位
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		time.Sleep(10 * time.Second) // 到不了这里：handler 会以 143 退出
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSigtermHandlerExits143$", "-test.timeout=60s")
	cmd.Env = append(os.Environ(), "FLY_SIGTERM_CHILD=1")
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	require.ErrorAs(t, err, &ee, "output: %s", out)
	assert.Equal(t, 143, ee.ExitCode(), "output: %s", out)
}

// ---- judgeStep 直测臂 + run() 传播 ----

func TestJudgeStepDirectArms(t *testing.T) {
	t.Run("绑定缺失上抛", func(t *testing.T) {
		fx := newGitFixture(t)
		ctx := verdictCtx(t, fx, happyDrivers())
		delete(ctx.Config.Loops, "judge")
		_, err := ctx.judgeStep(contract.Milestone{ID: "M-01"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no loop binding")
	})

	t.Run("fatal 短路", func(t *testing.T) {
		fx := newGitFixture(t)
		ctx := verdictCtx(t, fx, happyDrivers())
		ctx.fatal = errors.New("disk on fire")
		_, err := ctx.judgeStep(contract.Milestone{ID: "M-01"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disk on fire")
	})
}

func TestRunTaskJudgeBindingMissingSurfaces(t *testing.T) {
	fx := newGitFixture(t)
	rewriteConfig(t, fx, func(m map[string]any) { dropMapKey(t, m, "loops", "judge") })

	_, err := RunTask(runOpts(fx, happyDrivers().set()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no loop binding")
}

// TestRunTaskJudgeStoppedByMidFlightBudgetStop：judge 首 spawn 巨额用量 +
// 不可解析 → 重试循环顶硬停 → judge.Stopped 经 run() 停机。
func TestRunTaskJudgeStoppedByMidFlightBudgetStop(t *testing.T) {
	fx := newGitFixture(t)
	rewriteConfig(t, fx, func(m map[string]any) {
		m["budget"] = map[string]any{"enforce": "hard", "daily-token-limit": 1000}
	})
	sd := happyDrivers()
	sd.judgeFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			r.Text = "garbage, no verdict"
			// 记账取 Input+Output：900(plan+2exec+2review)+1000 > 1000 → 硬停。
			r.Usage = &drivers.Usage{Input: 400, Output: 600}
		})
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "budget-stopped", summary.Status)
	sd.log.mu.Lock()
	defer sd.log.mu.Unlock()
	assert.Len(t, sd.log.judge, 1)
}

// TestRunTaskJudgeFailSaveFailureSurfaces：judge 判返退且其结论保存失败 →
// fatal 从 run() 的 inbox 落盘处上抛。
func TestRunTaskJudgeFailSaveFailureSurfaces(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.judgeFn = func(drivers.CallArgs) drivers.Result {
		res := drivers.MockResult(func(r *drivers.Result) {
			r.Text = `{"verdict":"return","score":0.4,"reasons":["gaps"]}`
		})
		roDir(t, fx.taskDir) // judgeStep 的 gate 保存失败 → fatal 已置位
		return res
	}

	_, err := RunTask(runOpts(fx, sd.set()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

// ---- 里程碑闸门失败 + 落盘失败 ----

// gatedChmodPacket L2 第一条命令把任务目录变只读（闸门命令本身就是注入点）、
// 第二条恒败 → 闸门失败且闸门报告的保存必然失败。
func gatedChmodPacket(id, requirement, taskDir string) string {
	base := fixturePacket(id, requirement)
	return strings.Replace(base,
		"acceptance:\n  L1:",
		fmt.Sprintf("acceptance:\n  L1:\n    - {cmd: \"test -f build/%s.txt\", expect: \"exit 0\"}\n"+
			"  L2:\n    - {cmd: \"chmod 555 '%s'\", expect: \"exit 0\"}\n"+
			"    - {cmd: \"exit 3\", expect: \"exit 0\"}", id, taskDir), 1)
}

func TestRunTaskGateFailSaveFailureSurfaces(t *testing.T) {
	fx := newGitFixture(t)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"),
		gatedChmodPacket("M01-T01", "R-01", fx.taskDir))
	t.Cleanup(func() { _ = os.Chmod(fx.taskDir, 0o755) }) // 闸门命令把目录变只读，先于 TempDir 清理恢复

	_, err := RunTask(runOpts(fx, happyDrivers().set()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

// ---- RunSingleStep 收尾臂 ----

func TestRunSingleStepCoverageArms(t *testing.T) {
	t.Run("中断恢复日志", func(t *testing.T) {
		fx := newGitFixture(t)
		st := loadFixtureState(t, fx)
		st.Packets["M01-T01"].Status = "executing" // 上次运行中断
		saveStateObj(t, fx, st)

		var logs []string
		opts := stepOpts(fx, happyDrivers().set())
		opts.Log = func(msg string) { logs = append(logs, msg) }
		opts.Only = "execute"
		opts.PacketID = "M01-T01"
		summary, err := RunSingleStep(opts)
		require.NoError(t, err)
		assert.True(t, summary.Step.OK)
		assert.Contains(t, strings.Join(logs, "\n"), "recovered interrupted units")
	})

	t.Run("only=plan 成功", func(t *testing.T) {
		fx := newGitFixture(t)
		opts := stepOpts(fx, happyDrivers().set())
		opts.Only = "plan"
		summary, err := RunSingleStep(opts)
		require.NoError(t, err)
		require.NotNil(t, summary.Step)
		assert.Equal(t, "plan", summary.Step.Name)
		assert.True(t, summary.Step.OK)
	})

	t.Run("only=plan 绑定缺失", func(t *testing.T) {
		fx := newGitFixture(t)
		opts := stepOpts(fx, happyDrivers().set())
		delete(opts.Config.Loops, "plan")
		opts.Only = "plan"
		_, err := RunSingleStep(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no loop binding")
	})

	t.Run("only=execute 路由缺失", func(t *testing.T) {
		fx := newGitFixture(t)
		opts := stepOpts(fx, happyDrivers().set())
		opts.Config.Routes = nil
		opts.Only = "execute"
		opts.PacketID = "M01-T01"
		_, err := RunSingleStep(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no route binding")
	})

	t.Run("only=review 未知单元", func(t *testing.T) {
		fx := newGitFixture(t)
		opts := stepOpts(fx, happyDrivers().set())
		opts.Only = "review"
		opts.PacketID = "NOPE-T99"
		_, err := RunSingleStep(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown packet/unit")
	})

	t.Run("only=review 绑定缺失", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		// 先 execute 再 verify 打底（铁律 4：评审前必须有绿灯 L1）。
		base := runOpts(fx, sd.set()).RunOpts
		_, err := RunSingleStep(RunSingleStepOpts{RunOpts: base, Only: "execute", PacketID: "M01-T01"})
		require.NoError(t, err)
		_, err = RunSingleStep(RunSingleStepOpts{RunOpts: base, Only: "verify", PacketID: "M01-T01"})
		require.NoError(t, err)

		opts := stepOpts(fx, sd.set())
		delete(opts.Config.Loops, "review")
		opts.Only = "review"
		opts.PacketID = "M01-T01"
		_, err = RunSingleStep(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no loop binding")
	})

	t.Run("verify 全量巡检闸门失败", func(t *testing.T) {
		fx := newGitFixture(t)
		writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), gatedPacket("M01-T01", "R-01"))
		opts := stepOpts(fx, happyDrivers().set())
		opts.Only = "verify" // 无 PacketID → 巡检全部未完成单元 + 里程碑闸门
		summary, err := RunSingleStep(opts)
		require.NoError(t, err)
		require.NotNil(t, summary.Step)
		assert.Equal(t, "verify", summary.Step.Name)
		assert.False(t, summary.Step.OK, "L2 恒败 → 闸门失败必须反映在巡检结论里")
	})
}

// ---- executeStep：fail-loud 守卫 ----

func TestExecuteStepFailLoudMissingEnvKey(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	ctx := unitTestContext(t, fx)
	ctx.Drivers = sd.set()
	ctx.Config.Drivers["execute"] = "pi" // 真实驱动才强制 key
	ctx.EnvLookup = func(string) (string, bool) { return "", false }

	out, err := ctx.executeStep(ctx.Packets["M01-T01"])
	require.NoError(t, err)
	assert.Equal(t, errs.AUTH, out.ErrorCode)
	assert.False(t, out.Success)
	sd.log.mu.Lock()
	defer sd.log.mu.Unlock()
	assert.Empty(t, sd.log.exec, "缺 key 绝不 spawn")
}

// ---- executeStep：渠道降级三臂 ----

func TestExecuteStepChannelFallbackSuccessSwitchesHarness(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		if a.Binding.Channel == "chan-a" {
			return drivers.MockResult(func(r *drivers.Result) {
				msg := "429 rate limit exceeded"
				r.ErrorMessage = &msg
			})
		}
		return sd.mock.Execute(a)
	}
	ctx := unitTestContext(t, fx)
	ctx.Drivers = sd.set()
	r := ctx.Config.Routes["default"]
	r.Channel = "chan-a"
	ctx.Config.Routes["default"] = r
	ctx.Config.Channels["chan-a"] = config.Channel{Fallback: "chan-b"}
	ctx.Config.Channels["chan-b"] = config.Channel{
		Harness: "pi", Models: map[string]string{"default": "m-b"},
	}

	out, err := ctx.executeStep(ctx.Packets["M01-T01"])
	require.NoError(t, err)
	assert.True(t, out.Success, "降级到 chan-b 后成功")
	sd.log.mu.Lock()
	defer sd.log.mu.Unlock()
	require.NotEmpty(t, sd.log.exec)
	last := sd.log.exec[len(sd.log.exec)-1]
	assert.Equal(t, "chan-b", last.Binding.Channel)
	assert.Equal(t, "pi", last.Binding.Harness, "降级携带渠道 harness 切换")
	assert.Equal(t, "m-b", last.Binding.Model)
}

func TestExecuteStepFallbackChainEndsWithoutModelMapping(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			msg := "429 rate limit exceeded"
			r.ErrorMessage = &msg
		})
	}
	ctx := unitTestContext(t, fx)
	ctx.Drivers = sd.set()
	var logs []string
	ctx.Log = func(msg string) { logs = append(logs, msg) }
	r := ctx.Config.Routes["default"]
	r.Channel = "chan-a"
	ctx.Config.Routes["default"] = r
	ctx.Config.Channels["chan-a"] = config.Channel{Fallback: "chan-b"}
	ctx.Config.Channels["chan-b"] = config.Channel{} // 无 models 映射 → 链终止

	out, err := ctx.executeStep(ctx.Packets["M01-T01"])
	require.NoError(t, err)
	assert.Equal(t, errs.RATE_LIMIT, out.ErrorCode)
	assert.Contains(t, strings.Join(logs, "\n"),
		"fallback chain ends: no model mapping for execute/default on chan-b")
}

// ---- l1Step：主 checkout 契约防篡改扫描 ----

func TestL1DetectsContractTamperInMainCheckout(t *testing.T) {
	fx := newGitFixture(t)
	specPath := filepath.Join(fx.taskDir, "spec.md")
	sd := happyDrivers()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		res := sd.mock.Execute(a)
		f, cerr := os.OpenFile(specPath, os.O_APPEND|os.O_WRONLY, 0o644)
		require.NoError(t, cerr)
		_, werr := f.WriteString("\ntampered line\n")
		require.NoError(t, werr)
		require.NoError(t, f.Close())
		return res
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "escalated", summary.Status, "executor 动契约层 → L1 恒败 → spin 升级")
	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	l1, ok := ps.Evidence.L1.(map[string]any)
	require.True(t, ok)
	boundary, _ := l1["boundary"].(map[string]any)
	violations, _ := boundary["violations"].([]any)
	found := false
	for _, v := range violations {
		vm, _ := v.(map[string]any)
		if vm["type"] == "contract-file" {
			if p, _ := vm["path"].(string); strings.HasSuffix(p, "spec.md") {
				found = true
			}
		}
	}
	assert.True(t, found, "主 checkout 的契约篡改必须进违规：%v", violations)
}

// ---- agentVerdictStep / planStep 分类兜底 ----

func TestAgentVerdictStepClassifiesBareNonZeroExit(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.reviewFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			code := 1
			r.ExitCode = &code // 无 ErrorMessage、无文本 → 文本分类兜底
			r.Settled = false
		})
	}
	ctx := verdictCtx(t, fx, sd)

	out, err := ctx.agentVerdictStep(verdictArgs{
		role: "review", binding: reviewBinding("opencode-go"), prompt: "P",
		packetID: "M01-T01", ps: ctx.State.Packets["M01-T01"],
	})
	require.NoError(t, err)
	assert.False(t, out.OK)
	assert.Equal(t, errs.TRANSPORT, out.ErrorCode, "裸非零退出码归类 TRANSPORT")
}

func TestAgentVerdictStepFallbackAppliesHarnessMapping(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		if a.Binding.Channel == "kimi-subscription" {
			return drivers.MockResult(func(r *drivers.Result) {
				msg := "429 slow down"
				r.ErrorMessage = &msg
			})
		}
		return sd.mock.Review(a)
	}
	ctx := verdictCtx(t, fx, sd)
	ch := ctx.Config.Channels["kimi-coding"]
	if ch.Models == nil {
		ch.Models = map[string]string{}
	}
	ch.Models["review"] = "kimi-coding/k3" // fixture 渠道本无 review 映射，补上
	ctx.Config.Channels["kimi-coding"] = ch

	out, err := ctx.agentVerdictStep(verdictArgs{
		role: "review", binding: reviewBinding("kimi-subscription"), prompt: "P",
		packetID: "M01-T01", ps: ctx.State.Packets["M01-T01"],
	})
	require.NoError(t, err)
	require.True(t, out.OK)
	sd.log.mu.Lock()
	defer sd.log.mu.Unlock()
	last := sd.log.review[len(sd.log.review)-1]
	assert.Equal(t, "kimi-coding", last.Binding.Channel)
	assert.Equal(t, "omp", last.Binding.Harness, "降级映射必须应用渠道 harness（fixture v0.19：pi 系角色统一 omp）")
}

func TestPlanStepClassifyAndFatalArms(t *testing.T) {
	t.Run("裸非零退出码归类", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		sd.planFn = func(drivers.CallArgs) drivers.Result {
			return drivers.MockResult(func(r *drivers.Result) {
				code := 1
				r.ExitCode = &code
				r.Settled = false
			})
		}
		ctx := verdictCtx(t, fx, sd)

		out, err := ctx.planStep(contract.Milestone{ID: "M-01"}, "")
		require.NoError(t, err)
		assert.False(t, out.OK)
		assert.Equal(t, errs.TRANSPORT, out.ErrorCode)
	})

	t.Run("fatal 短路", func(t *testing.T) {
		fx := newGitFixture(t)
		ctx := verdictCtx(t, fx, happyDrivers())
		ctx.fatal = errors.New("disk on fire")
		_, err := ctx.planStep(contract.Milestone{ID: "M-01"}, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disk on fire")
	})
}
