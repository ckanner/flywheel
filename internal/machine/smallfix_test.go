package machine

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/verify"
)

// ---- C4：execute 失败路径的权威空转签名证据 ----

func TestAuthoritativeChangedFiles(t *testing.T) {
	fx := newGitFixture(t)
	ctx := unitTestContext(t, fx)
	packet := ctx.PacketList[0]

	// 无 base-ref → nil（兜底链接管）。
	assert.Nil(t, ctx.authoritativeChangedFiles(packet))

	ps := ctx.State.Packets[packet.PacketID]
	ref, ok := verify.GetBaseRef(fx.root)
	require.True(t, ok)
	ps.BaseRef = &ref

	// worktree 未建 → 主 checkout；新增 untracked 文件必须出现在权威集里。
	writeFile(t, filepath.Join(fx.root, "wip.go"), "package wip\n")
	files := ctx.authoritativeChangedFiles(packet)
	require.NotNil(t, files)
	assert.Contains(t, files, "wip.go")
}

// ---- C5：plan 调用成功但未捕获 session id 的观测痕迹 ----

func TestPlanStepLogsMissingSessionID(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	ctx := unitTestContext(t, fx)
	var captured []string
	ctx.Log = func(msg string) { captured = append(captured, msg) }
	ctx.Drivers = sd.set()

	m := ctx.Spec.Milestones[0]
	_, err := ctx.planStep(m)
	require.NoError(t, err)

	found := false
	for _, l := range captured {
		if strings.Contains(l, "returned no session id") {
			found = true
		}
	}
	assert.True(t, found, "缺 session id 必须留日志，实际: %v", captured)
}

// TestJudgePanelFatalBetweenCalls 覆盖面板第二次调用前 fatal 已置位的上抛臂。
func TestJudgePanelFatalBetweenCalls(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	var ctx *Context
	calls := 0
	sd.judgeFn = func(drivers.CallArgs) drivers.Result {
		calls++
		if calls == 1 {
			return drivers.MockResult(func(r *drivers.Result) { r.Text = "" }) // 可重试空响应
		}
		ctx.fatal = errors.New("injected between calls") // 主判重试成功后、复核循环顶前置位
		return passVerdict()
	}
	ctx = unitTestContext(t, fx)
	ctx.Config.JudgePanel = panelBinding()
	ctx.Drivers = sd.set()

	_, err := ctx.judgeStep(milestoneOf(t, ctx))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected")
}

// TestAuthoritativeChangedFilesDiffUnavailable 覆盖 diff 不可用臂（非 git cwd）。
func TestAuthoritativeChangedFilesDiffUnavailable(t *testing.T) {
	fx := newGitFixture(t)
	ctx := unitTestContext(t, fx)
	packet := ctx.PacketList[0]
	ps := ctx.State.Packets[packet.PacketID]
	ref := verify.EmptyTree
	ps.BaseRef = &ref
	ctx.Cwd = t.TempDir() // 非 git checkout → GetChangedFiles ok=false

	assert.Nil(t, ctx.authoritativeChangedFiles(packet))
}
