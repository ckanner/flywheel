package modules

import (
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type structuredModuleDriver struct {
	drivers.Driver
	failure drivers.Result
	calls   []drivers.CallArgs
}

func (d *structuredModuleDriver) call(a drivers.CallArgs) drivers.Result {
	d.calls = append(d.calls, a)
	if a.Binding.Channel == "backup" {
		return drivers.MockResult(nil)
	}
	return d.failure
}

func (d *structuredModuleDriver) Intake(a drivers.CallArgs) drivers.Result { return d.call(a) }
func (d *structuredModuleDriver) Garden(a drivers.CallArgs) drivers.Result { return d.call(a) }

func structuredModuleContext(t *testing.T, result drivers.Result) (*Ctx, *structuredModuleDriver) {
	t.Helper()
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	cfg.Retry.MaxTransportRetries = 1
	cfg.Channels = map[string]config.Channel{
		"primary": {Harness: "dsh", Fallback: "backup"},
		"backup":  {Harness: "dsh", Models: map[string]string{"intake": "backup-model", "garden": "backup-model"}},
	}
	driver := &structuredModuleDriver{failure: result}
	m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: &drivers.Drivers{Intake: driver, Garden: driver}, Sleep: func(int64) {}})
	return m, driver
}

func structuredModuleOptions(role string) CallOptions {
	return CallOptions{Role: role, Prompt: "advice only", Binding: fallback.Binding{
		Harness: "dsh", Provider: "deepseek-account", Channel: "primary", Model: "primary-model",
	}}
}

func TestStructuredErrorCodesControlAdviceRetryAndFallback(t *testing.T) {
	for _, role := range []string{"intake", "garden"} {
		for _, tc := range []struct {
			name     string
			result   drivers.Result
			want     errs.ErrorCode
			channels []string
		}{
			{"auth despite quota wording", drivers.Result{ExitCode: new(1), ErrorCode: errs.AUTH,
				ErrorMessage: strPtrT("Sign in again to inspect quota.")}, errs.AUTH, []string{"primary"}},
			{"quota without English keywords", drivers.Result{ExitCode: new(1), ErrorCode: errs.QUOTA,
				ErrorMessage: strPtrT("账户额度暂不可用。")}, "", []string{"primary", "backup"}},
			{"code without message", drivers.Result{ExitCode: new(1), ErrorCode: errs.AUTH}, errs.AUTH, []string{"primary"}},
			{"typed error contradicts exit zero", drivers.Result{ExitCode: new(0), Settled: true, ErrorCode: errs.AUTH,
				ErrorMessage: strPtrT("quota exhausted")}, errs.TRANSPORT, []string{"primary", "primary"}},
			{"typed error contradicts exit 42", drivers.Result{ExitCode: new(42), ErrorCode: errs.AUTH,
				ErrorMessage: strPtrT("quota exhausted")}, errs.TRANSPORT, []string{"primary", "primary"}},
			{"untrusted native code is transport", drivers.Result{ExitCode: new(1), ErrorCode: "ACCOUNT_TOKEN_INVALID",
				ErrorMessage: strPtrT("401 unauthorized; quota exhausted")}, errs.TRANSPORT, []string{"primary", "primary"}},
		} {
			t.Run(role+"/"+tc.name, func(t *testing.T) {
				m, driver := structuredModuleContext(t, tc.result)
				got := CallAgent(m, structuredModuleOptions(role))
				assert.Equal(t, tc.want == "", got.OK)
				assert.Equal(t, tc.want, got.ErrorCode)
				var channels []string
				for _, call := range driver.calls {
					channels = append(channels, call.Binding.Channel)
				}
				assert.Equal(t, tc.channels, channels)
				entries := readMetricsLinesT(t, m.MetricsFile)
				require.Len(t, entries, len(tc.channels))
				wantFirst := tc.want
				if wantFirst == "" {
					wantFirst = errs.QUOTA
				}
				assert.Equal(t, string(wantFirst), entries[0]["error-code"])
			})
		}
	}
}

func TestStructuredErrorsCannotOverrideModuleProcessFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*drivers.Result)
		want   errs.ErrorCode
		stops  bool
	}{
		{"cancelled", func(r *drivers.Result) { r.Cancelled = true }, errs.TRANSPORT, true},
		{"cancelled timeout", func(r *drivers.Result) { r.Cancelled, r.TimedOut = true, true }, errs.TRANSPORT, true},
		{"timeout", func(r *drivers.Result) { r.TimedOut = true }, errs.TIMEOUT, false},
		{"spawn", func(r *drivers.Result) { r.SpawnError = "executable unavailable" }, errs.TRANSPORT, false},
		{"cleanup", func(r *drivers.Result) { r.CleanupError = "owned process not quiescent" }, errs.TRANSPORT, true},
		{"signal", func(r *drivers.Result) { r.Signal = "terminated" }, errs.TRANSPORT, false},
		{"truncated", func(r *drivers.Result) { r.Truncated = true }, errs.TRANSPORT, false},
		{"no physical exit", func(r *drivers.Result) { r.ExitCode = nil }, errs.TRANSPORT, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := drivers.Result{ExitCode: new(1), ErrorCode: errs.QUOTA, ErrorMessage: strPtrT("401 unauthorized")}
			tc.change(&result)
			m, driver := structuredModuleContext(t, result)
			options := structuredModuleOptions("intake")
			got := CallAgent(m, options)
			assert.False(t, got.OK)
			assert.Equal(t, tc.want, got.ErrorCode)
			for _, call := range driver.calls {
				assert.Equal(t, "primary", call.Binding.Channel, "physical failure must not follow a QUOTA fallback")
			}
			if tc.stops {
				require.Error(t, m.fatalErr)
				assert.False(t, CallAgent(m, options).OK)
				assert.Len(t, driver.calls, 1, "cancel or cleanup failure must block later candidates")
			}
		})
	}
}
