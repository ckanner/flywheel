package modules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModuleRequiresConfirmedTerminalAndPhysicalSuccess(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*drivers.Result)
		code   errs.ErrorCode
	}{
		{"not settled", func(r *drivers.Result) { r.Settled = false }, errs.TRANSPORT},
		{"cancelled", func(r *drivers.Result) { r.Cancelled = true }, errs.TRANSPORT},
		{"cleanup failed", func(r *drivers.Result) { r.CleanupError = "child group alive" }, errs.TRANSPORT},
		{"truncated", func(r *drivers.Result) { r.Truncated = true }, errs.TRANSPORT},
		{"signal", func(r *drivers.Result) { r.Signal = "terminated" }, errs.TRANSPORT},
		{"timeout beats authentication text", func(r *drivers.Result) { r.TimedOut = true; s := "401 unauthorized"; r.ErrorMessage = &s }, errs.TIMEOUT},
		{"nonzero after complete", func(r *drivers.Result) { code := 4; r.ExitCode = &code }, errs.TRANSPORT},
		{"stderr error after complete", func(r *drivers.Result) { r.Stderr = "429 rate limit" }, errs.RATE_LIMIT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			taskDir := t.TempDir()
			cfg := testConfigT(t, taskDir)
			cfg.Retry.MaxTransportRetries = 1
			driver := &countingDriver{onCall: func(int) drivers.Result {
				return drivers.MockResult(tc.change)
			}}
			m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: &drivers.Drivers{Intake: driver}, Sleep: func(int64) {}})
			r := CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "p"})
			assert.False(t, r.OK)
			assert.Equal(t, tc.code, r.ErrorCode)
			if tc.name == "cancelled" {
				assert.Equal(t, 1, driver.n, "cancellation must not spawn a retry")
				CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "next candidate"})
				assert.Equal(t, 1, driver.n, "cancelled module must not continue with later candidates")
			}
		})
	}
}

type scopedModuleDriver struct {
	drivers.Driver
	args drivers.CallArgs
}

func (d *scopedModuleDriver) Intake(a drivers.CallArgs) drivers.Result {
	d.args = a
	return drivers.MockResult(nil)
}

func TestAdviceModulesUseReadOnlyOnCapableHarnesses(t *testing.T) {
	for _, harness := range []string{"dsh", "codex", "codex-pro", "kimi"} {
		t.Run(harness, func(t *testing.T) {
			taskDir := t.TempDir()
			cfg := testConfigT(t, taskDir)
			driver := &scopedModuleDriver{}
			m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: &drivers.Drivers{Intake: driver}})
			binding := bindingFor(t, cfg, "intake")
			binding.Harness = harness
			r := CallAgent(m, CallOptions{Role: "intake", Binding: binding, Prompt: "p"})
			assert.True(t, r.OK)
			if harness == "kimi" {
				assert.Empty(t, driver.args.PermissionMode)
			} else {
				assert.Equal(t, "read-only", driver.args.PermissionMode)
			}
		})
	}
}

func TestModuleUnknownUsageRemainsObservableInState(t *testing.T) {
	taskDir := t.TempDir()
	writeFileT(t, filepath.Join(taskDir, "state.json"), `{"revision":0,"budget":{"daily":null}}`)
	cfg := testConfigT(t, taskDir)
	// No price exists for this model, so missing monetary usage remains unknown.
	m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg})
	require.NoError(t, recordModuleMetrics(m, "garden", "unpriced-model", "subscription", "p", drivers.Result{}, ""))
	require.NoError(t, recordModuleMetrics(m, "garden", "unpriced-model", "subscription", "p", drivers.Result{}, ""))
	st, err := state.Load(taskDir)
	require.NoError(t, err)
	assert.Equal(t, int64(2), st.Budget.UnknownUsageCalls)
	assert.Equal(t, int64(2), st.Budget.UnknownCostCalls)
	assert.Zero(t, st.Budget.Totals.Tokens)
	assert.Zero(t, st.Budget.Totals.CostUSD, "zero is a known lower bound accompanied by explicit unknown counters")
	require.Len(t, st.Budget.Warnings, 1, "do not repeat the same warning every call")
	var unknownUsage, unknownCost int64
	for _, bucket := range st.Budget.Daily {
		unknownUsage += bucket.UnknownUsageCalls
		unknownCost += bucket.UnknownCostCalls
	}
	assert.Equal(t, int64(2), unknownUsage)
	assert.Equal(t, int64(2), unknownCost)
}

func TestModulePreservesCodexProReceiptAndUnknownBudget(t *testing.T) {
	taskDir := t.TempDir()
	writeFileT(t, filepath.Join(taskDir, "state.json"), `{"revision":0,"budget":{"daily":null}}`)
	cfg := testConfigT(t, taskDir)
	m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg})
	receipt := json.RawMessage(`{"protocol":1,"requestId":"advice","costUSD":null,"costStatus":"unknown","outcome":{"usage":{"status":"partial","run":{"inputTokens":42}},"cleanup":{"confirmed":true}}}`)
	r := drivers.Result{Harness: "codex-pro", CodexProReceipt: receipt}
	require.NoError(t, recordModuleMetrics(m, "garden", "model", "subscription", "prompt", r, ""))
	entries := readMetricsLinesT(t, m.MetricsFile)
	require.Len(t, entries, 1)
	encoded, err := json.Marshal(entries[0]["codex-pro-receipt"])
	require.NoError(t, err)
	assert.JSONEq(t, string(receipt), string(encoded))
	assert.Nil(t, entries[0]["provider"])
	assert.Nil(t, entries[0]["usage"])
	assert.Nil(t, entries[0]["cost-usd"])
	st, err := state.Load(taskDir)
	require.NoError(t, err)
	assert.Equal(t, int64(1), st.Budget.UnknownUsageCalls)
	assert.Equal(t, int64(1), st.Budget.UnknownCostCalls)
	assert.Zero(t, st.Budget.Totals.Tokens)
}

func TestModuleMetricWriteFailureStopsFurtherCalls(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	driver := &countingDriver{onCall: func(int) drivers.Result { return drivers.MockResult(nil) }}
	var logs []string
	m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: &drivers.Drivers{Intake: driver}, Log: func(s string) { logs = append(logs, s) }})
	require.NoError(t, os.Mkdir(m.MetricsFile, 0o755))
	for i := 0; i < 2; i++ {
		r := CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "p"})
		assert.False(t, r.OK)
		assert.Equal(t, errs.TRANSPORT, r.ErrorCode)
	}
	assert.Equal(t, 1, driver.n)
	assert.Contains(t, strings.Join(logs, "\n"), "metrics recording failed")
}
