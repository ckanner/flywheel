package machine

import (
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanSessionIdentityIncludesPermissionMode(t *testing.T) {
	for _, tc := range []struct {
		name, configured, stored string
		resume                   bool
	}{
		{"unchanged full access", "full-access", "danger-full-access", true},
		{"upgraded access", "full-access", "workspace-write", false},
		{"restricted access", "role-based", "danger-full-access", false},
		{"legacy missing permission", "role-based", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newGitFixture(t)
			sd := happyDrivers()
			c := verdictCtx(t, fx, sd)
			c.Config.Permissions.Mode = tc.configured
			b := c.Config.Loops["plan"]
			c.State.PlanSession = map[string]any{"session-id": "old-session", "harness": b.Harness,
				"model": b.Model, "channel": b.Channel, "cwd": c.Cwd, "thinking": b.Thinking,
				"permission-mode": tc.stored}
			sd.planFn = func(a drivers.CallArgs) drivers.Result {
				if tc.resume {
					assert.Equal(t, "old-session", a.SessionID)
				} else {
					assert.Empty(t, a.SessionID)
				}
				return drivers.MockResult(func(r *drivers.Result) { r.SessionID = "current-session" })
			}
			c.Drivers = sd.set()
			out, err := c.planStep(milestoneOf(t, c), "")
			require.NoError(t, err)
			require.True(t, out.OK)
			stored := c.State.PlanSession.(map[string]any)
			want := "workspace-write"
			if tc.configured == "full-access" {
				want = "danger-full-access"
			}
			assert.Equal(t, want, stored["permission-mode"])
		})
	}
}

func TestFullAccessStillRejectsReviewerCandidateMutation(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	c := verdictCtx(t, fx, sd)
	c.Config.Permissions.Mode = "full-access"
	p := c.Packets["M01-T01"]
	seedVerification(t, c, p.PacketID)
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		assert.Equal(t, "danger-full-access", a.PermissionMode)
		writeFile(t, filepath.Join(a.Cwd, "reviewer-change.txt"), "outside reviewer role\n")
		return passVerdict()
	}
	c.Drivers = sd.set()
	out, err := c.reviewStep(p, nil)
	require.NoError(t, err)
	assert.False(t, out.Pass)
	assert.Equal(t, errs.INVALID_REQUEST, out.ErrorCode)
	assert.FileExists(t, filepath.Join(c.unitCwd(p), "reviewer-change.txt"), "preserve the rejected candidate")
}

func TestFullAccessJudgePanelKeepsCandidateIdentityGuard(t *testing.T) {
	fx := newGitFixture(t, "source.txt")
	c := unitTestContext(t, fx)
	c.Config.Permissions.Mode = "full-access"
	c.Config.JudgePanel = panelBinding()
	milestone := milestoneOf(t, c)
	require.True(t, c.milestoneGateStep(milestone).Pass)
	sd := happyDrivers()
	calls := 0
	sd.judgeFn = func(a drivers.CallArgs) drivers.Result {
		assert.Equal(t, "danger-full-access", a.PermissionMode)
		calls++
		if calls == 2 {
			writeFile(t, filepath.Join(a.Cwd, "source.txt"), "changed by second opinion\n")
		}
		return passVerdict()
	}
	c.Drivers = sd.set()
	out, err := c.judgeStep(milestone)
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	assert.False(t, out.Pass)
	assert.Equal(t, errs.INVALID_REQUEST, out.ErrorCode)
}
