package machine

import (
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- A3 双 judge 面板（DESIGN §4.3 v0.18）----
// 注意：scriptedDrivers.set() 在构造时捕获 fn 字段——脚本函数必须先赋值再
// verdictCtx()。

func milestoneOf(t *testing.T, ctx *Context) contract.Milestone {
	t.Helper()
	require.NotEmpty(t, ctx.Spec.Milestones)
	return ctx.Spec.Milestones[0]
}

func loadGateMap(t *testing.T, ctx *Context) map[string]any {
	t.Helper()
	g, ok := ctx.State.Milestones["M-01"].Gate.(map[string]any)
	require.True(t, ok, "gate 必须是对象")
	return g
}

func panelBinding() *config.LoopBinding {
	return &config.LoopBinding{Harness: "pi", Model: "prov/second-opinion", Channel: "panel-ch", Thinking: "max"}
}

func TestJudgePanelAgreePass(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	judgeCalls := 0
	sd.judgeFn = func(drivers.CallArgs) drivers.Result {
		judgeCalls++
		return passVerdict()
	}
	ctx := verdictCtx(t, fx, sd)
	panelModel := "opencode-go/deepseek-v4-pro"
	ctx.Config.JudgePanel = &config.LoopBinding{Harness: "pi", Model: panelModel, Channel: "opencode-go", Thinking: "max"}

	out, err := ctx.judgeStep(milestoneOf(t, ctx))
	require.NoError(t, err)
	assert.True(t, out.Pass)
	assert.False(t, out.Split)
	assert.Equal(t, 2, judgeCalls, "主判 + 复核各一次")

	gate := loadGateMap(t, ctx)
	judge := gate["judge"].(map[string]any)
	panel := judge["panel"].(map[string]any)
	assert.Equal(t, "agree", panel["agreement"])
	assert.Equal(t, panelModel, panel["binding-model"])
}

func TestJudgePanelSplitEscalates(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	calls := 0
	sd.judgeFn = func(drivers.CallArgs) drivers.Result {
		calls++
		if calls == 1 {
			return passVerdict()
		}
		return returnVerdict() // 复核退回 → 分歧
	}
	ctx := verdictCtx(t, fx, sd)
	ctx.Config.JudgePanel = panelBinding()

	out, err := ctx.judgeStep(milestoneOf(t, ctx))
	require.NoError(t, err)
	assert.False(t, out.Pass)
	assert.True(t, out.Split, "pass vs return → split")
	assert.Equal(t, "split", out.Verdict.Verdict)

	gate := loadGateMap(t, ctx)
	judge := gate["judge"].(map[string]any)
	panel := judge["panel"].(map[string]any)
	assert.Equal(t, "split", panel["agreement"])
	reasons := judge["reasons"].([]string)
	require.Len(t, reasons, 2, "双方理由合并供人裁决")
	sec := panel["secondary"].(map[string]any)
	assert.Equal(t, "return", sec["verdict"])
}

func TestJudgePanelSecondaryErrorFailsLoud(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	calls := 0
	sd.judgeFn = func(drivers.CallArgs) drivers.Result {
		calls++
		if calls == 1 {
			return passVerdict()
		}
		// 复核通道流内 AUTH 错误：不可重试、fail-loud，绝不静默降级单 judge。
		return drivers.MockResult(func(r *drivers.Result) {
			r.ErrorMessage = strPtr("401: {\"message\":\"Invalid API key\"}")
		})
	}
	ctx := verdictCtx(t, fx, sd)
	ctx.Config.JudgePanel = panelBinding()

	out, err := ctx.judgeStep(milestoneOf(t, ctx))
	require.NoError(t, err)
	assert.False(t, out.Pass)
	assert.Equal(t, "AUTH", string(out.ErrorCode))

	gate := loadGateMap(t, ctx)
	judge := gate["judge"].(map[string]any)
	panel := judge["panel"].(map[string]any)
	assert.Equal(t, "secondary-error", panel["agreement"])
}

// TestJudgeStepNonObjectGatePersisted（C11 回归）：手工把 milestone.gate 写成
// 非对象时，替换为对象并写回 state——judge 结论绝不落进孤立内存 map。
func TestJudgeStepNonObjectGatePersisted(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.judgeFn = func(drivers.CallArgs) drivers.Result { return passVerdict() }
	ctx := verdictCtx(t, fx, sd)
	ms := ctx.State.Milestones["M-01"]
	ms.Gate = "not-a-map"
	ctx.State.Milestones["M-01"] = ms

	out, err := ctx.judgeStep(milestoneOf(t, ctx))
	require.NoError(t, err)
	assert.True(t, out.Pass)

	persisted, ok := ctx.State.Milestones["M-01"].Gate.(map[string]any)
	require.True(t, ok, "gate 必须以对象形态持久化")
	require.Contains(t, persisted, "judge")
}
