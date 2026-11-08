package machine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionQualityBoundedRebuildPreservesTaskAccounting(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy config.SessionQualityPolicy
		reason string
	}{
		{"confirmed turns", config.SessionQualityPolicy{MaxConfirmedTurns: 1}, "confirmed turn limit"},
		{"unchanged candidate", config.SessionQualityPolicy{MaxUnproductiveTurns: 1}, "unchanged candidate limit"},
		{"measured run input", config.SessionQualityPolicy{MaxRunInputTokens: 1}, "measured run input limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, c := sessionFixture(t)
			c.Config.Sessions.Quality = tc.policy
			args := sessionArgs(c)
			args.Prompt = "Read the current authoritative packet and candidate."
			c.State.Packets[args.PacketID].Attempts = 2
			first, err := c.invokeScoped("execute", args.PacketID, args, func(a drivers.CallArgs) drivers.Result {
				return sessionResult(a, drivers.MockResult(nil))
			})
			require.NoError(t, err)
			applySessionResult(t, c, "execute", args.PacketID, first)
			prior := c.sessionRecord("execute", args.PacketID)
			second, err := c.invokeScoped("execute", args.PacketID, args, func(a drivers.CallArgs) drivers.Result {
				require.Empty(t, a.SessionID, "the new generation cannot silently resume retired history")
				require.Contains(t, a.Prompt, tc.reason)
				require.Contains(t, a.Prompt, prior.Last.ID)
				require.True(t, strings.HasSuffix(a.Prompt, args.Prompt))
				return sessionResult(a, drivers.MockResult(nil))
			})
			require.NoError(t, err)
			applySessionResult(t, c, "execute", args.PacketID, second)
			r := c.sessionRecord("execute", args.PacketID)
			assert.Equal(t, "rebuild", r.Action)
			assert.Contains(t, r.Reason, tc.reason)
			assert.Equal(t, 2, r.Generation)
			assert.NotEqual(t, prior.ThreadID, r.ThreadID)
			assert.Equal(t, 1, r.Quality.Rebuilds)
			assert.Equal(t, 1, r.Quality.ConfirmedTurns)
			assert.Equal(t, 2, c.State.Packets[args.PacketID].Attempts)
			assert.Equal(t, int64(360), c.State.Budget.Totals.Tokens)
		})
	}
}

func TestSessionRoleOverridesSeparateExperimentalHistories(t *testing.T) {
	_, c := sessionFixture(t)
	c.Config.Sessions.Mode = "fresh"
	c.Config.Sessions.Roles = map[string]string{"execute": "scoped", "review": "fresh", "plan": "fresh"}
	args := sessionArgs(c)
	for _, role := range []string{"execute", "review"} {
		for i := 0; i < 2; i++ {
			result, err := c.invokeScoped(role, args.PacketID, args, func(a drivers.CallArgs) drivers.Result {
				assert.Equal(t, role == "execute", a.PersistentSession)
				assert.Equal(t, role == "execute" && i > 0, a.SessionID != "")
				return sessionResult(a, drivers.MockResult(nil))
			})
			require.NoError(t, err)
			applySessionResult(t, c, role, args.PacketID, result)
		}
	}
	// The new explicit role override wins over the legacy Plan reuse shortcut.
	args.SessionID = "legacy-private-plan-history"
	result, err := c.invokeScoped("plan", "M-01", args, func(a drivers.CallArgs) drivers.Result {
		assert.Empty(t, a.SessionID)
		assert.False(t, a.PersistentSession)
		return sessionResult(a, drivers.MockResult(nil))
	})
	require.NoError(t, err)
	applySessionResult(t, c, "plan", "M-01", result)

	c.Config.Sessions.Roles["plan"] = "scoped"
	p := c.Config.Loops["plan"]
	p.ResidentSessionSet, p.ResidentSession = true, false
	c.Config.Loops["plan"] = p
	assert.False(t, c.scopedSessions("plan", "dsh"))
	assert.Equal(t, "fresh", sessionQualityMode("unconfigured", c.Config.Sessions))
}

func TestSessionQualityReceiptRecoveryIsIdempotent(t *testing.T) {
	fx, c := sessionFixture(t)
	args := sessionArgs(c)
	_, err := c.invokeScoped("execute", args.PacketID, args, func(a drivers.CallArgs) drivers.Result {
		onDisk := loadFixtureState(t, fx)
		_, err := state.Save(fx.taskDir, onDisk, nil)
		require.NoError(t, err)
		return sessionResult(a, drivers.MockResult(nil))
	})
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	for i := 0; i < 2; i++ {
		c = unitTestContext(t, fx)
		enableSessionPolicy(c)
		result, err := c.invokeScoped("execute", args.PacketID, sessionArgs(c), func(drivers.CallArgs) drivers.Result {
			t.Fatal("a durable terminal result must never be redispatched")
			return drivers.Result{}
		})
		require.NoError(t, err)
		assert.Equal(t, 1, c.sessionRecord("execute", args.PacketID).Quality.ConfirmedTurns)
		if i == 1 {
			applySessionResult(t, c, "execute", args.PacketID, result)
		}
	}
	assert.Equal(t, int64(180), c.State.Budget.Totals.Tokens)
}

func TestSessionQualityOnlyUsesSafeConfirmedEvidence(t *testing.T) {
	_, c := sessionFixture(t)
	args := sessionArgs(c)
	res, err := c.invokeScoped("execute", args.PacketID, args, func(a drivers.CallArgs) drivers.Result {
		return sessionResult(a, drivers.MockResult(nil))
	})
	require.NoError(t, err)
	applySessionResult(t, c, "execute", args.PacketID, res)
	r := c.sessionRecord("execute", args.PacketID)
	policy := config.SessionQualityPolicy{MaxConfirmedTurns: 1, MaxUnproductiveTurns: 1, MaxRunInputTokens: 1}
	assert.Empty(t, sessionQualityReason("execute", nil, policy))
	assert.Empty(t, sessionQualityReason("execute", &state.SessionRecord{}, policy))
	for _, mutation := range []func(*state.SessionRecord){
		func(r *state.SessionRecord) { r.Quality = nil },
		func(r *state.SessionRecord) { r.Last = nil },
		func(r *state.SessionRecord) { r.ResumeCandidate = false },
		func(r *state.SessionRecord) { r.Last.Status = "received" },
		func(r *state.SessionRecord) { r.Last.Status = "blocked" },
		func(r *state.SessionRecord) { r.Last.Accounted = false },
		func(r *state.SessionRecord) { r.Last.CleanupConfirmed = false },
		func(r *state.SessionRecord) { r.Last.Dispatch = "unknown" },
		func(r *state.SessionRecord) { r.Last.Outcome = "unknown" },
	} {
		copy := *r
		last := *r.Last
		copy.Last = &last
		mutation(&copy)
		assert.Empty(t, sessionQualityReason("execute", &copy, policy))
	}
	assert.Empty(t, sessionQualityReason("execute", r, config.SessionQualityPolicy{}))
	assert.Empty(t, sessionQualityReason("review", r, config.SessionQualityPolicy{MaxUnproductiveTurns: 1}))
	assert.Empty(t, sessionRebuildHandoff(nil, "none"))

	// Candidate progress resets only the bounded quality counter, not attempts.
	writeFile(t, filepath.Join(c.Cwd, "progress.txt"), "new implementation\n")
	res, err = c.invokeScoped("execute", args.PacketID, args, func(a drivers.CallArgs) drivers.Result {
		writeFile(t, filepath.Join(c.Cwd, "progress.txt"), "next implementation\n")
		return sessionResult(a, drivers.MockResult(nil))
	})
	require.NoError(t, err)
	applySessionResult(t, c, "execute", args.PacketID, res)
	r = c.sessionRecord("execute", args.PacketID)
	assert.Equal(t, 2, r.Quality.ConfirmedTurns)
	assert.Zero(t, r.Quality.UnproductiveTurns)

	// Partial usage cannot become a fabricated zero or a context-window claim.
	copy := *r
	last := *r.Last
	copy.Last = &last
	copy.Quality = nil
	partial := res
	s := *res.Session
	s.UsageStatus = "partial"
	partial.Session = &s
	observeSessionQuality(&copy, partial)
	assert.Nil(t, copy.Quality.LastInputTokens)
	assert.Equal(t, "partial", copy.Quality.LastUsageStatus)
	assert.Empty(t, sessionQualityReason("execute", &copy, config.SessionQualityPolicy{MaxRunInputTokens: 1}))
	observeSessionQuality(&copy, partial)
	assert.Equal(t, 1, copy.Quality.ConfirmedTurns)

	last.ID = "not-dispatched"
	s.RequestID, s.Dispatch = last.ID, "not-dispatched"
	observeSessionQuality(&copy, partial)
	assert.Equal(t, 1, copy.Quality.ConfirmedTurns)
	copy.Last = nil
	observeSessionQuality(&copy, partial)
	assert.Equal(t, 1, copy.Quality.ConfirmedTurns)
	assert.Equal(t, 1, nextSessionQuality(&state.SessionRecord{}, "rebuild", "unavailable").Rebuilds)
}
