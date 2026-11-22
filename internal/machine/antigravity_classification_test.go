package machine

import (
	"encoding/json"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/stretchr/testify/require"
)

func antigravityClassificationResult() drivers.Result {
	// Model the already validated driver boundary; parser and real worker
	// verification are covered separately by the receipt and bridge suites.
	return drivers.Result{
		Harness: "antigravity", Model: "gemini-4-neon-high", ExitCode: new(0), Settled: true,
		Stdout:             `{"receipt":{"ok":true,"subscription":{"extraBilling":"unknown"},"output":[{"text":"Investigate a quota warning."}]}}`,
		Text:               "Investigate a quota warning.",
		AntigravityReceipt: json.RawMessage(`{"ok":true,"subscription":{"extraBilling":"unknown"}}`),
		Session:            &drivers.SessionOutcome{RequestID: "classification-one", Dispatch: "dispatched", Outcome: "completed", CleanupConfirmed: true, UsageStatus: "unknown"},
	}
}

func TestAntigravityConfirmedSuccessClassification(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*drivers.Result)
		code   errs.ErrorCode
		omit   bool
	}{
		{name: "success metadata and answer", omit: true},
		{name: "empty error message", change: func(r *drivers.Result) { r.ErrorMessage = new("") }, omit: true},
		{name: "stderr auth remains authoritative", change: func(r *drivers.Result) { r.Stderr = "401 unauthorized" }, code: errs.AUTH, omit: true},
		{name: "stderr quota remains authoritative", change: func(r *drivers.Result) { r.Stderr = "quota exhausted" }, code: errs.QUOTA, omit: true},
		{name: "typed auth failure", change: func(r *drivers.Result) {
			r.ExitCode, r.Settled, r.ErrorCode, r.Session.Outcome = new(1), false, errs.AUTH, "failed"
		}, code: errs.AUTH},
		{name: "typed quota failure", change: func(r *drivers.Result) {
			r.ExitCode, r.Settled, r.ErrorCode, r.Session.Outcome = new(1), false, errs.QUOTA, "failed"
		}, code: errs.QUOTA},
		{name: "invalid typed failure exit", change: func(r *drivers.Result) { r.ErrorCode = errs.AUTH }, code: errs.TRANSPORT},
		{name: "missing exit", change: func(r *drivers.Result) { r.ExitCode = nil }, code: errs.TRANSPORT},
		{name: "nonzero exit retains text classification", change: func(r *drivers.Result) { r.ExitCode = new(1) }, code: errs.QUOTA},
		{name: "cancelled", change: func(r *drivers.Result) { r.Cancelled = true }, code: errs.QUOTA},
		{name: "timed out", change: func(r *drivers.Result) { r.TimedOut = true }, code: errs.TIMEOUT},
		{name: "truncated", change: func(r *drivers.Result) { r.Truncated = true }, code: errs.TRANSPORT},
		{name: "signal", change: func(r *drivers.Result) { r.Signal = "SIGTERM" }, code: errs.QUOTA},
		{name: "spawn failed", change: func(r *drivers.Result) { r.SpawnError = "process launch failed" }, code: errs.TRANSPORT},
		{name: "cleanup failed", change: func(r *drivers.Result) { r.CleanupError = "process group still running" }, code: errs.TRANSPORT},
		{name: "unsettled", change: func(r *drivers.Result) { r.Settled = false }, code: errs.TRANSPORT},
		{name: "reported error", change: func(r *drivers.Result) { r.ErrorMessage = new("provider failure") }, code: errs.QUOTA},
		{name: "missing session", change: func(r *drivers.Result) { r.Session = nil }, code: errs.QUOTA},
		{name: "unknown dispatch", change: func(r *drivers.Result) { r.Session.Dispatch = "unknown" }, code: errs.QUOTA},
		{name: "unknown outcome", change: func(r *drivers.Result) { r.Session.Outcome = "unknown" }, code: errs.QUOTA},
		{name: "unconfirmed cleanup", change: func(r *drivers.Result) { r.Session.CleanupConfirmed = false }, code: errs.QUOTA},
		{name: "no native receipt", change: func(r *drivers.Result) { r.AntigravityReceipt = nil }, code: errs.QUOTA},
		{name: "different harness", change: func(r *drivers.Result) { r.Harness = "pi" }, code: errs.QUOTA},
		{name: "mock remains text classified", change: func(r *drivers.Result) { r.Harness = "mock" }, code: errs.QUOTA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := antigravityClassificationResult()
			if tc.change != nil {
				tc.change(&r)
			}
			original, err := json.Marshal(r)
			require.NoError(t, err)
			classified := toSpawnResult(r)
			require.Equal(t, tc.code, errs.ClassifyError(classified))
			if tc.omit {
				require.Empty(t, classified.Stdout)
			} else {
				require.Equal(t, r.Stdout, classified.Stdout)
			}
			require.Equal(t, r.Stderr, classified.Stderr)
			after, err := json.Marshal(r)
			require.NoError(t, err)
			require.Equal(t, original, after, "classification must preserve the original wire evidence")
		})
	}
}

func TestAntigravityInvalidVerdictDoesNotBecomeQuotaFailure(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	calls := 0
	sd.reviewFn = func(drivers.CallArgs) drivers.Result {
		calls++
		return antigravityClassificationResult()
	}
	c := verdictCtx(t, fx, sd)
	c.Config.Retry.MaxTransportRetries = 0
	c.Config.Channels["google-ultra"] = config.Channel{Harness: "antigravity"}
	out, err := c.agentVerdictStep(verdictArgs{
		role: "review", binding: fallback.Binding{Harness: "antigravity", Model: "gemini-4-neon-high", Channel: "google-ultra", Thinking: "high"},
		prompt: "Review the candidate", packetID: "M01-T01", ps: c.State.Packets["M01-T01"],
	})
	require.NoError(t, err)
	require.False(t, out.OK)
	require.Equal(t, errs.EMPTY_RESPONSE, out.ErrorCode)
	require.Equal(t, 1, calls)
}
