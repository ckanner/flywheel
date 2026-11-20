package machine

import (
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/stretchr/testify/require"
)

func TestWebUnknownCannotBeRetriedThroughFreshSessionOrOtherProvider(t *testing.T) {
	for _, code := range []errs.ErrorCode{errs.WEB_UNCERTAIN, errs.WEB_DELIVERY_CLAIMED, errs.WEB_CONNECTION_LOST, errs.WEB_PROTOCOL_ERROR} {
		t.Run(string(code), func(t *testing.T) {
			fx, c := sessionFixture(t)
			a := sessionArgs(c)
			a.Binding.Provider, a.Binding.Model, a.Model, a.Binding.Thinking = "chatgpt", "gpt-6-pro-web", "gpt-6-pro-web", "pro"
			_, err := c.invokeScoped("execute", "M01-T01", a, func(args drivers.CallArgs) drivers.Result {
				// Even a wrapper incorrectly marking the SDK error as a known
				// failure must not erase the provider's reconciliation requirement.
				r := sessionResult(args, drivers.Result{ErrorCode: code})
				r.Session.Outcome, r.Session.ResumeCandidate, r.Session.UsageStatus = "failed", false, "unknown"
				return r
			})
			require.ErrorContains(t, err, "requires reconciliation")
			require.Equal(t, "blocked", c.sessionRecord("execute", "M01-T01").Last.Status)
			require.False(t, errs.IsRetryable(code))
			require.False(t, errs.ShouldFallback(code))
			restarted, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config, Drivers: c.Drivers})
			require.NoError(t, err)
			restarted.Config.Sessions.Mode = "fresh"
			a.Binding.Harness, a.Binding.Provider, a.Binding.Model, a.Model = "kimi", "", "kimi-k3", "kimi-k3"
			calls := 0
			_, err = restarted.invokeScoped("execute", "M01-T01", a, func(drivers.CallArgs) drivers.Result { calls++; return drivers.Result{} })
			require.ErrorContains(t, err, "resume and fresh replay are blocked")
			require.Zero(t, calls)
		})
	}
}

func TestWebUnknownUsageStopsExplicitHardDailyBudget(t *testing.T) {
	_, c := sessionFixture(t)
	a := sessionArgs(c)
	a.Binding.Provider, a.Binding.Model, a.Model = "chatgpt", "gpt-6-pro-web", "gpt-6-pro-web"
	result, err := c.invokeScoped("execute", "M01-T01", a, func(args drivers.CallArgs) drivers.Result {
		r := sessionResult(args, drivers.Result{})
		r.Session.UsageStatus = "unknown"
		return r
	})
	require.NoError(t, err)
	applySessionResult(t, c, "execute", "M01-T01", result)
	require.Nil(t, result.Usage)
	require.EqualValues(t, 1, c.State.Budget.UnknownUsageCalls)
	c.State.Budget.Enforce, c.Config.Budget.Enforce = "hard", "hard"
	limit := int64(1000000)
	c.State.Budget.DailyTokenLimit, c.Config.Budget.DailyTokenLimit = &limit, &limit
	require.Equal(t, "stop", c.budgetGate(), "the next call stops despite a zero known lower bound")
}
