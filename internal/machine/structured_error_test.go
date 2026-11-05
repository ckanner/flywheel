package machine

import (
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type structuredStepObservation struct {
	ok       bool
	code     errs.ErrorCode
	err      error
	context  *Context
	channels []string
}

// Exercise the real step entry, including fallback and verdict/report handling.
// A backup succeeds so an accidental AUTH -> QUOTA conversion is observable.
func observeStructuredStep(t *testing.T, role string, failure drivers.Result) structuredStepObservation {
	t.Helper()
	fx := newGitFixture(t)
	sd := happyDrivers()
	observed := structuredStepObservation{}
	call := func(a drivers.CallArgs) drivers.Result {
		observed.channels = append(observed.channels, a.Binding.Channel)
		if a.Binding.Channel == "primary" {
			return failure
		}
		switch role {
		case "plan":
			return sd.mock.Plan(a)
		case "execute":
			return sd.mock.Execute(a)
		case "review":
			return sd.mock.Review(a)
		default:
			return sd.mock.Judge(a)
		}
	}
	sd.planFn, sd.execFn, sd.reviewFn, sd.judgeFn = call, call, call, call
	c := verdictCtx(t, fx, sd)
	observed.context = c
	c.Config.Retry.MaxTransportRetries = 1
	c.Config.Channels = map[string]config.Channel{
		"primary": {Harness: "dsh", Fallback: "backup"},
		"backup": {Harness: "dsh", Models: map[string]string{
			"plan": "backup-model", "default": "backup-model", "review": "backup-model", "judge": "backup-model",
		}},
	}
	// Neither account has a key-based AUTH fallback; QUOTA may use the explicit chain.
	binding := config.LoopBinding{Harness: "dsh", Provider: "deepseek-account", Channel: "primary", Model: "primary-model"}
	c.Config.Loops[role] = binding
	c.Config.Routes["default"] = config.RouteBinding{Harness: binding.Harness, Provider: binding.Provider, Channel: binding.Channel, Model: binding.Model}
	switch role {
	case "plan":
		out, err := c.planStep(milestoneOf(t, c), "")
		observed.ok, observed.code, observed.err = out.OK, out.ErrorCode, err
	case "execute":
		out, err := c.executeStep(c.Packets["M01-T01"])
		observed.ok, observed.code, observed.err = out.Success, out.ErrorCode, err
	case "review":
		seedVerification(t, c, "M01-T01")
		out, err := c.reviewStep(c.Packets["M01-T01"], nil)
		observed.ok, observed.code, observed.err = out.Pass, out.ErrorCode, err
	case "judge":
		out, err := c.judgeStep(milestoneOf(t, c))
		observed.ok, observed.code, observed.err = out.Pass, out.ErrorCode, err
	default:
		t.Fatalf("unsupported test role %q", role)
	}
	return observed
}

func TestStructuredErrorCodesReachEveryMachineStep(t *testing.T) {
	for _, role := range []string{"plan", "execute", "review", "judge"} {
		t.Run(role, func(t *testing.T) {
			for _, tc := range []struct {
				name     string
				result   drivers.Result
				want     errs.ErrorCode
				channels []string
			}{
				{"auth despite quota wording", drivers.Result{ExitCode: new(1), ErrorCode: errs.AUTH,
					ErrorMessage: strPtr("Sign in again to inspect quota; native detail retained.")}, errs.AUTH, []string{"primary"}},
				{"quota without English keywords", drivers.Result{ExitCode: new(1), ErrorCode: errs.QUOTA,
					ErrorMessage: strPtr("账户额度暂不可用。")}, "", []string{"primary", "backup"}},
				{"code without message", drivers.Result{ExitCode: new(1), ErrorCode: errs.AUTH}, errs.AUTH, []string{"primary"}},
				{"typed error contradicts exit zero", drivers.Result{ExitCode: new(0), Settled: true, ErrorCode: errs.AUTH,
					ErrorMessage: strPtr("quota exhausted")}, errs.TRANSPORT, []string{"primary", "primary"}},
				{"typed error contradicts exit 42", drivers.Result{ExitCode: new(42), ErrorCode: errs.AUTH,
					ErrorMessage: strPtr("quota exhausted")}, errs.TRANSPORT, []string{"primary", "primary"}},
				{"untrusted native code is transport", drivers.Result{ExitCode: new(1), ErrorCode: "ACCOUNT_TOKEN_INVALID",
					ErrorMessage: strPtr("401 unauthorized; quota exhausted")}, errs.TRANSPORT, []string{"primary", "primary"}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					got := observeStructuredStep(t, role, tc.result)
					require.NoError(t, got.err)
					assert.Equal(t, tc.want == "", got.ok)
					assert.Equal(t, tc.want, got.code)
					assert.Equal(t, tc.channels, got.channels, "typed code must control retry/fallback, not diagnostic wording")
				})
			}
		})
	}
}

func TestStructuredErrorsCannotOverrideMachineProcessFailures(t *testing.T) {
	for _, tc := range []struct {
		name, role string
		change     func(*drivers.Result)
		want       errs.ErrorCode
	}{
		{"timeout", "plan", func(r *drivers.Result) { r.TimedOut = true }, errs.TIMEOUT},
		{"cancelled", "execute", func(r *drivers.Result) { r.Cancelled = true }, errs.TRANSPORT},
		{"spawn", "review", func(r *drivers.Result) { r.SpawnError = "executable unavailable" }, errs.TRANSPORT},
		{"cleanup", "judge", func(r *drivers.Result) { r.CleanupError = "owned process not quiescent" }, errs.TRANSPORT},
		{"signal", "plan", func(r *drivers.Result) { r.Signal = "terminated" }, errs.TRANSPORT},
		{"truncated", "review", func(r *drivers.Result) { r.Truncated = true }, errs.TRANSPORT},
		{"no physical exit", "execute", func(r *drivers.Result) { r.ExitCode = nil }, errs.TRANSPORT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := drivers.Result{ExitCode: new(1), ErrorCode: errs.QUOTA, ErrorMessage: strPtr("401 unauthorized")}
			tc.change(&result)
			got := observeStructuredStep(t, tc.role, result)
			assert.False(t, got.ok)
			if tc.name == "cleanup" {
				require.ErrorContains(t, got.err, "cleanup is unconfirmed")
				assert.Equal(t, "escalated", got.context.State.Status)
				assert.Equal(t, []string{"primary"}, got.channels, "uncertain cleanup must not retry or fall back")
			} else {
				require.NoError(t, got.err)
				assert.Equal(t, tc.want, got.code)
				assert.NotContains(t, got.channels, "backup", "physical failure must not follow the provider QUOTA fallback")
			}
		})
	}
}
