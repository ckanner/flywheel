package diagnostic

import (
	"context"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredErrorCodesReachSDKDiagnostic(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result drivers.Result
		want   errs.ErrorCode
	}{
		{"auth despite quota wording", drivers.Result{ExitCode: new(1), ErrorCode: errs.AUTH,
			ErrorMessage: new("Sign in again to inspect quota.")}, errs.AUTH},
		{"quota without English keywords", drivers.Result{ExitCode: new(1), ErrorCode: errs.QUOTA,
			ErrorMessage: new("账户额度暂不可用。")}, errs.QUOTA},
		{"code without message", drivers.Result{ExitCode: new(1), ErrorCode: errs.AUTH}, errs.AUTH},
		{"typed error contradicts exit zero", drivers.Result{ExitCode: new(0), Settled: true, ErrorCode: errs.AUTH,
			ErrorMessage: new("quota exhausted")}, errs.TRANSPORT},
		{"typed error contradicts exit 42", drivers.Result{ExitCode: new(42), ErrorCode: errs.AUTH,
			ErrorMessage: new("quota exhausted")}, errs.TRANSPORT},
		{"untrusted native code is transport", drivers.Result{ExitCode: new(1), ErrorCode: "ACCOUNT_TOKEN_INVALID",
			ErrorMessage: new("401 unauthorized; quota exhausted")}, errs.TRANSPORT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkStructuredDiagnostic(t, tc.result, tc.want)
		})
	}
}

func TestStructuredErrorsCannotOverrideDiagnosticProcessFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*drivers.Result)
		want   errs.ErrorCode
	}{
		{"timeout", func(r *drivers.Result) { r.TimedOut = true }, errs.TIMEOUT},
		{"cancelled", func(r *drivers.Result) { r.Cancelled = true }, errs.TRANSPORT},
		{"spawn", func(r *drivers.Result) { r.SpawnError = "executable unavailable" }, errs.TRANSPORT},
		{"cleanup", func(r *drivers.Result) { r.CleanupError = "owned process not quiescent" }, errs.TRANSPORT},
		{"signal", func(r *drivers.Result) { r.Signal = "terminated" }, errs.TRANSPORT},
		{"truncated", func(r *drivers.Result) { r.Truncated = true }, errs.TRANSPORT},
		{"no physical exit", func(r *drivers.Result) { r.ExitCode = nil }, errs.TRANSPORT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := drivers.Result{ExitCode: new(1), ErrorCode: errs.AUTH, ErrorMessage: new("quota exhausted")}
			tc.change(&result)
			checkStructuredDiagnostic(t, result, tc.want)
		})
	}
}

func checkStructuredDiagnostic(t *testing.T, result drivers.Result, want errs.ErrorCode) {
	t.Helper()
	root, runtime := diagnosticFixture(t, `{}`)
	runtime.probe = func(drivers.DSHArgs) drivers.Result { return result }
	cfg := config.Default()
	r := Report{OK: true, Cwd: root}
	r.probe(context.Background(), &cfg, Binding{ID: "binding", Provider: "deepseek-account", Channel: "account",
		Model: "deepseek-flash", PermissionMode: "read-only"}, map[string]bool{})
	assert.False(t, r.OK)
	check := checkByID(t, r, "binding.sdk")
	assert.Equal(t, "fail", check.Status)
	assert.Equal(t, string(want), check.Code)
	require.Len(t, runtime.probes, 1, "diagnosis must not retry or choose a different account")
	assert.True(t, runtime.probes[0].Doctor)
	assert.Equal(t, "deepseek-account", runtime.probes[0].Provider)
}
