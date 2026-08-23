package machine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/drivers"
)

// defectMarkerPatch 创建 marker 文件的补丁（注入后 test 语义可翻转：marker
// 存在 = RED，还原后不存在 = GREEN）。
const defectMarkerPatch = `diff --git a/defect-marker b/defect-marker
new file mode 100644
index 0000000..7898192
--- /dev/null
+++ b/defect-marker
@@ -0,0 +1 @@
+injected
`

// TestDefectInjectionSampledMissingConfigInboxButContinues
// every-n-units=1 → 首个单元 L1 绿即采样；patch/test-cmd 未配置 →
// missing-config 进 inbox 提示人工配置、单元继续（P0-2 分支语义）。
func TestDefectInjectionSampledMissingConfigInboxButContinues(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	o := runOpts(fx, sd.set())
	o.Config.DefectInjection.EveryNUnits = 1
	// Patch/TestCmd 留空。

	summary, err := RunTask(o)
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)

	st := loadFixtureState(t, fx)
	ps := st.Packets["M01-T01"]
	require.NotNil(t, ps.Evidence.DefectInjection)
	diMap, ok := ps.Evidence.DefectInjection.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "missing-config", diMap["skipped"])
	found := false
	for _, e := range st.Inbox {
		entry, _ := e.(map[string]any)
		if r, _ := entry["reason"].(string); strings.Contains(r, "defect injection sampled") {
			found = true
		}
	}
	assert.True(t, found, "missing-config 必须进 inbox 提示人")
}

func writeDefectPatch(t *testing.T, fx *fixture) string {
	t.Helper()
	patchFile := filepath.Join(fx.root, "defect.patch")
	writeFile(t, patchFile, defectMarkerPatch)
	return patchFile
}

// TestDefectInjectionFakeTestsEscalates：注入后 test-cmd 仍绿 = 假测试实证 →
// escalated + stop（DC3）。
func TestDefectInjectionFakeTestsEscalates(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	o := runOpts(fx, sd.set())
	o.Config.DefectInjection.EveryNUnits = 1
	o.Config.DefectInjection.Patch = writeDefectPatch(t, &fx)
	o.Config.DefectInjection.TestCmd = "true" // 注入后仍成功 → fake

	summary, err := RunTask(o)
	require.NoError(t, err)
	assert.Equal(t, "escalated", summary.Status)
	st := loadFixtureState(t, fx)
	diMap, ok := st.Packets["M01-T01"].Evidence.DefectInjection.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "fake-tests", diMap["outcome"])
	assert.Equal(t, "inbox", st.Packets["M01-T01"].Status)
}

// TestDefectInjectionPassContinuesToComplete：注入使测试变红、还原干净 →
// pass → 流程继续到完成。
func TestDefectInjectionPassContinuesToComplete(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	o := runOpts(fx, sd.set())
	o.Config.DefectInjection.EveryNUnits = 1
	o.Config.DefectInjection.Patch = writeDefectPatch(t, &fx)
	o.Config.DefectInjection.TestCmd = "test ! -f defect-marker" // 注入后存在 → RED ✓

	summary, err := RunTask(o)
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)
	st := loadFixtureState(t, fx)
	diMap, ok := st.Packets["M01-T01"].Evidence.DefectInjection.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "pass", diMap["outcome"])
}

func gapVerdict() drivers.Result {
	return drivers.MockResult(func(r *drivers.Result) {
		r.Text = `{"verdict":"verification-gap","score":0.5,"reasons":["acceptance too weak"]}`
	})
}

func passVerdict() drivers.Result {
	return drivers.MockResult(func(r *drivers.Result) {
		r.Text = `{"verdict":"pass","score":1,"reasons":["strengthened"]}`
	})
}

func returnVerdict() drivers.Result {
	return drivers.MockResult(func(r *drivers.Result) {
		r.Text = `{"verdict":"return","score":0.6,"reasons":["naming"]}`
	})
}

// TestVerificationGapReturnsToPlannerThenCompletes
// review 返 verification-gap → 退回 Plan 加强验收（PlanRevisions+1）→
// 复验 L1 + 复评审通过 → completed（DC2：单元绝不在缺口上完成）。
func TestVerificationGapReturnsToPlannerThenCompletes(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	reviewCalls := 0
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		reviewCalls++
		if reviewCalls == 1 {
			return gapVerdict()
		}
		return passVerdict()
	}
	sd.planFn = func(a drivers.CallArgs) drivers.Result {
		return drivers.MockResult(nil) // planner OK：确认 packet 序列（不重写契约）
	}
	var logs []string
	o := runOptsWithLog(fx, sd.set(), &logs)

	summary, err := RunTask(o)
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status, "logs: %v", logs)
	st := loadFixtureState(t, fx)
	assert.Equal(t, 1, st.Packets["M01-T01"].PlanRevisions, "gap 必须消耗一次修订预算")
	assert.GreaterOrEqual(t, reviewCalls, 2)
}

// TestReviewReturnRetriesThenCompletes：普通 return（非 gap）→
// review-failed → failAttempt(REVIEW_RETURNED) → 重试后通过。
func TestReviewReturnRetriesThenCompletes(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	reviewCalls := 0
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		reviewCalls++
		if reviewCalls == 1 {
			return returnVerdict()
		}
		return passVerdict()
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)
	st := loadFixtureState(t, fx)
	historyHasReviewReturned := false
	for _, rh := range st.Packets["M01-T01"].RetryHistory {
		entry, _ := rh.(map[string]any)
		if ec, _ := entry["error-code"].(string); ec == "REVIEW_RETURNED" {
			historyHasReviewReturned = true
		}
	}
	assert.True(t, historyHasReviewReturned)
}

// TestMaxRetriesExhaustedInboxWithRevision：恒败且错误码交替（避开同签名
// spin）→ 每轮 max-retries 耗尽 → return-to-planner 修订一轮 → 再败 →
// revisions>=1 → inbox 叫人并停。
func TestMaxRetriesExhaustedInboxWithRevision(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	execCalls := 0
	codes := []string{"500 upstream A", "503 upstream B"} // SERVER 家族、签名不同
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		execCalls++
		msg := codes[(execCalls-1)%len(codes)]
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
	assert.GreaterOrEqual(t, ps.PlanRevisions, 1, "return-to-planner 应先修订一轮")
}

// TestL1FailureRetryLoopWithAuthoritativeDiff：L1 命令首轮失败 →
// failAttempt(L1_FAILED, 权威 changedFiles) → execute 重试写产物 → L1 过 →
// completed。
func TestL1FailureRetryLoopWithAuthoritativeDiff(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	execCalls := 0
	flagFile := filepath.Join(fx.root, "l1-flag")
	packetText := strings.Replace(
		fixturePacket("M01-T01", "R-01"),
		`    - {cmd: "test -f build/M01-T01.txt", expect: "exit 0"}`,
		"    - {cmd: \"test -f build/M01-T01.txt\", expect: \"exit 0\"}\n"+
			"    - {cmd: \"test -f "+filepath.Join(fx.root, "l1-flag")+"\", expect: \"exit 0\"}", 1)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), packetText)
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		execCalls++
		if execCalls >= 2 {
			writeFile(t, flagFile, "go")
		}
		return sd.mock.Execute(a)
	}

	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", summary.Status)
	st := loadFixtureState(t, fx)
	assert.Equal(t, 2, st.Packets["M01-T01"].Attempts, "L1 失败重试计一次 attempt")
}
