package machine

import (
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestSessionTitlesDescribeTaskAndRoleWithoutChangingOpaqueIdentity(t *testing.T) {
	c := &Context{State: &state.State{Slug: "alert-repair"}}
	require.Equal(t, "Flywheel · execute/M01-T01 · alert-repair", c.sessionTitle("execute", "M01-T01"))
	c.Spec = &contract.Spec{Title: "修复告警重复发送"}
	require.Equal(t, "Flywheel · review/M01-T01 · 修复告警重复发送", c.sessionTitle("review", "M01-T01"))
	c.Spec = nil
	c.State.Planning = &planning.State{Brief: planning.Brief{Goal: "排查告警\n发送失败\u202e"}}
	require.Equal(t, "Flywheel · design-plan · 排查告警 发送失败", c.sessionTitle("design-plan", "planning:opaque-random-id"))
	require.NotContains(t, c.sessionTitle("design-plan", ""), "opaque")
	c.State.Planning.Brief.Goal = strings.Repeat("告警", 150)
	title := c.sessionTitle("plan-review", "planning:another-id")
	require.LessOrEqual(t, len([]rune(title)), 200)
	require.True(t, strings.HasSuffix(title, "…"))
	require.Equal(t, "alert-repair", c.State.Slug)
	c.State.Planning.Brief.Goal = strings.Repeat("🔧", 100)
	title = c.sessionTitle("review", strings.Repeat("修", 100))
	require.LessOrEqual(t, len(title), 512)
	require.True(t, strings.HasSuffix(title, "…"))
}
