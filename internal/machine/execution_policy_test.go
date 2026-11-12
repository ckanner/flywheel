package machine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/circuitbreaker"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func enableExecutionPolicy(c *Context) {
	c.Config.Channels["opencode-go"] = config.Channel{Harness: "dsh", Provider: "deepseek-account", Fallback: "opencode"}
	c.Config.Channels["opencode"] = config.Channel{Harness: "dsh", Provider: "deepseek-official", Models: map[string]string{"default": "flash"}}
	low := config.ExecutionBinding{Harness: "dsh", Provider: "deepseek-account", Model: "flash", Channel: "opencode-go", Thinking: "low", Permission: "workspace-write", SessionMode: "scoped"}
	high, backup := low, low
	high.Thinking = "high"
	backup.Channel, backup.Provider = "opencode", "deepseek-official"
	c.Config.ExecutionPolicy = &config.ExecutionPolicy{Version: 1, Bindings: map[string]config.ExecutionBinding{"low": low, "high": high, "backup": backup},
		Routes: map[string]config.ExecutionRoute{"default": {Initial: "low", Allowed: []string{"low", "high", "backup"}, QualityOrder: []string{"low", "high"}, ChannelFallbacks: map[string]string{"low": "backup"}, MaxSwitches: 2, AfterStalledAttempts: 1}}}
	c.Config.Worktree.Enabled = new(false)
}

func TestExecutionPolicyUpgradesRealRepeatedGateFailureWithinOriginalBudget(t *testing.T) {
	for _, repaired := range []bool{true, false} {
		t.Run(map[bool]string{true: "repair", false: "bounded"}[repaired], func(t *testing.T) {
			fx, c := sessionFixture(t)
			enableExecutionPolicy(c)
			c.Config.Sessions.Context = "verified-incremental"
			p := c.Packets["M01-T01"]
			sd := happyDrivers()
			calls := 0
			sd.execFn = func(a drivers.CallArgs) drivers.Result {
				calls++
				result := sd.mock.Execute(a)
				if calls < 3 || !repaired {
					require.NoError(t, os.Remove(filepath.Join(a.Cwd, "build", a.PacketID+".txt")))
				}
				return result
			}
			c.Drivers = sessionDriverSet(sd)
			out, err := c.runExecuteLoop(p)
			require.NoError(t, err)
			require.Equal(t, repaired, out.Green)
			require.Equal(t, !repaired, out.Bounded || out.Spun)
			require.Equal(t, 3, calls)
			require.Equal(t, []string{"low", "low", "high"}, []string{sd.log.exec[0].Binding.Thinking, sd.log.exec[1].Binding.Thinking, sd.log.exec[2].Binding.Thinking})
			require.NotEmpty(t, sd.log.exec[1].SessionID)
			require.Empty(t, sd.log.exec[2].SessionID, "effort switch must cold start")
			require.Contains(t, sd.log.exec[1].Prompt, "Verified context update")
			require.Contains(t, sd.log.exec[2].Prompt, "## Spec (stable contract content)")
			ps := c.State.Packets[p.PacketID]
			require.Equal(t, 3, ps.Attempts)
			require.Equal(t, map[bool]int{true: 2, false: 3}[repaired], ps.FailedAttempts)
			require.Len(t, ps.ExecutionPolicy.History, 1)
			require.Equal(t, "quality-upgrade", ps.ExecutionPolicy.History[0].Kind)
			require.Equal(t, "low", ps.ExecutionPolicy.History[0].Before.Thinking)
			require.Equal(t, "high", ps.ExecutionPolicy.History[0].After.Thinking)
			reloaded := loadFixtureState(t, fx)
			require.Equal(t, ps.ExecutionPolicy, reloaded.Packets[p.PacketID].ExecutionPolicy)
		})
	}
}

func TestExecutionPolicyChannelFallbackRemainsDistinctAndBounded(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		t.Run(map[bool]string{true: "allowed", false: "no switch budget"}[allowed], func(t *testing.T) {
			_, c := sessionFixture(t)
			enableExecutionPolicy(c)
			if !allowed {
				route := c.Config.ExecutionPolicy.Routes["default"]
				route.MaxSwitches = 0
				c.Config.ExecutionPolicy.Routes["default"] = route
			}
			sd := happyDrivers()
			calls := 0
			sd.execFn = func(a drivers.CallArgs) drivers.Result {
				calls++
				if calls == 1 {
					return drivers.MockResult(func(r *drivers.Result) {
						r.ExitCode = new(1)
						r.ErrorCode = errs.QUOTA
						r.ErrorMessage = new("quota exhausted")
					})
				}
				return sd.mock.Execute(a)
			}
			c.Drivers = sessionDriverSet(sd)
			out, err := c.executeStep(c.Packets["M01-T01"])
			require.NoError(t, err)
			require.Equal(t, allowed, out.Success)
			ps := c.State.Packets["M01-T01"]
			if allowed {
				require.Equal(t, 2, calls)
				require.Equal(t, "channel-fallback", ps.ExecutionPolicy.History[0].Kind)
				require.Equal(t, "deepseek-official", sd.log.exec[1].Binding.Provider)
				require.Equal(t, "opencode", sd.log.exec[1].Binding.Channel)
				require.Empty(t, sd.log.exec[1].SessionID)
			} else {
				require.Equal(t, 1, calls)
				require.Equal(t, errs.QUOTA, out.ErrorCode)
				require.Empty(t, ps.ExecutionPolicy.History)
			}
			require.Zero(t, ps.FailedAttempts, "channel fallback is not a code quality failure")
		})
	}
}

func TestExecutionPolicyRecoveryAndAdmission(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	p := c.Packets["M01-T01"]
	record, route, err := c.executionPolicy(p)
	require.NoError(t, err)
	ps := c.State.Packets[p.PacketID]
	ps.Attempts, ps.FailedAttempts = 2, 2
	stalled := circuitbreaker.ProgressAssessment{Status: "stalled", Reason: "unchanged verified candidate and failure"}
	require.False(t, c.upgradeExecutionPolicy(p, errs.AUTH, stalled))
	require.False(t, c.upgradeExecutionPolicy(p, errL1Failed, circuitbreaker.ProgressAssessment{Status: "unknown"}))
	ps.FailedAttempts = p.Escalation.MaxRetries
	require.False(t, c.upgradeExecutionPolicy(p, errL1Failed, stalled))
	ps.FailedAttempts = 2
	record.LastObservedAttempt = ps.Attempts
	require.False(t, c.upgradeExecutionPolicy(p, errL1Failed, stalled))
	record.LastObservedAttempt = 0
	for _, next := range []string{"", "low", "outside"} {
		require.False(t, c.switchExecutionPolicy(p, record, route, next, "quality-upgrade", "not eligible"))
	}
	c.State.Sessions = &state.SessionLedger{Version: 2, Records: map[string]*state.SessionRecord{"execute/M01-T01": {Last: &state.SessionInvocation{Status: "blocked"}}}}
	require.False(t, c.switchExecutionPolicy(p, record, route, "high", "quality-upgrade", "unconfirmed"))
	c.State.Sessions = nil
	allowed, err := c.executionChannelFallback(p, "other", "QUOTA")
	require.NoError(t, err)
	require.False(t, allowed)
	record.Current = "high"
	allowed, err = c.executionChannelFallback(p, "opencode", "QUOTA")
	require.NoError(t, err)
	require.False(t, allowed)
	require.False(t, c.upgradeExecutionPolicy(p, errL1Failed, stalled), "the final quality binding cannot advance")
	record.Current, record.LastObservedAttempt = "backup", 0
	require.False(t, c.upgradeExecutionPolicy(p, errL1Failed, stalled), "a fallback has no implicit quality rank")
	record.Identity = "changed"
	_, _, _, err = c.executionBinding(p)
	require.ErrorContains(t, err, "identity changed")
	_, err = c.executionChannelFallback(p, "opencode", "QUOTA")
	require.ErrorContains(t, err, "identity changed")
	require.False(t, c.upgradeExecutionPolicy(p, errL1Failed, stalled))
	require.ErrorContains(t, c.err(), "identity changed")
	c.Config.ExecutionPolicy = nil
	_, _, _, err = c.executionBinding(p)
	require.ErrorContains(t, err, "no longer configured")
}

func TestExecutionPolicyDoesNotDispatchUnknownOrChangedPolicy(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	p := c.Packets["M01-T01"]
	sd := happyDrivers()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		result := sessionResult(a, drivers.MockResult(nil))
		result.Session.Outcome = "unknown"
		return result
	}
	c.Drivers = sd.set()
	_, err := c.executeStep(p)
	require.ErrorContains(t, err, "requires reconciliation")
	require.Len(t, sd.log.exec, 1)
	require.Empty(t, c.State.Packets[p.PacketID].ExecutionPolicy.History)
	c.Config.ExecutionPolicy = nil
	_, err = c.executeStep(p)
	require.ErrorContains(t, err, "requires reconciliation", "disabling policy cannot bypass an unknown call")
	require.Len(t, sd.log.exec, 1)

	_, c = sessionFixture(t)
	enableExecutionPolicy(c)
	p = c.Packets["M01-T01"]
	record, _, err := c.executionPolicy(p)
	require.NoError(t, err)
	record.Identity = strings.Repeat("b", 64)
	_, err = c.executeStep(p)
	require.ErrorContains(t, err, "identity changed")
}

func TestExecutionPolicyRejectsChangedFallbackAndExhaustedTokenBudget(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	p := c.Packets["M01-T01"]
	sd := happyDrivers()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		route := c.Config.ExecutionPolicy.Routes["default"]
		route.MaxSwitches++
		c.Config.ExecutionPolicy.Routes["default"] = route
		return drivers.MockResult(func(r *drivers.Result) {
			r.ExitCode = new(1)
			r.ErrorCode = errs.QUOTA
			r.ErrorMessage = new("quota exhausted")
		})
	}
	c.Drivers = sessionDriverSet(sd)
	_, err := c.executeStep(p)
	require.ErrorContains(t, err, "identity changed")
	require.Len(t, sd.log.exec, 1)

	_, c = sessionFixture(t)
	enableExecutionPolicy(c)
	p = c.Packets["M01-T01"]
	ps := c.State.Packets[p.PacketID]
	ps.Attempts, ps.FailedAttempts = 2, 2
	c.Config.Budget.Enforce, c.Config.Budget.DailyTokenLimit = "hard", new(int64(1))
	c.State.Budget.Daily[c.Now().UTC().Format("2006-01-02")] = state.DailyBucket{Tokens: 2}
	require.False(t, c.upgradeExecutionPolicy(p, errL1Failed, circuitbreaker.ProgressAssessment{Status: "stalled", Reason: "unchanged failure"}))
	require.Equal(t, "budget-stopped", c.State.Status)
	require.Empty(t, ps.ExecutionPolicy.History)
}
