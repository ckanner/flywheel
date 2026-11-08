package modules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdviceModulesStopForAnUnknownInvocationAfterKnownCleanup(t *testing.T) {
	for _, role := range []string{"intake", "garden"} {
		for _, missing := range []string{"dispatch", "outcome", "cleanup", "empty"} {
			t.Run(role+"/"+missing, func(t *testing.T) {
				taskDir := t.TempDir()
				cfg := testConfigT(t, taskDir)
				writeFileT(t, filepath.Join(taskDir, "state.json"), `{"revision":0,"status":"pending","packets":{},"inbox":[],"budget":{"daily":{},"cache":{"by-provider":{}}}}`)
				s := &drivers.SessionOutcome{RequestID: "uncertain-advice", Protocol: "claude-code-pro/0.4.0/worker-v1", Dispatch: "dispatched", Outcome: "completed", CleanupConfirmed: true, UsageStatus: "complete"}
				switch missing {
				case "dispatch":
					s.Dispatch = "unknown"
				case "outcome":
					s.Outcome = "unknown"
				case "cleanup":
					s.CleanupConfirmed = false
				case "empty":
					s.Dispatch, s.Outcome = "", ""
				}
				r := drivers.MockResult(func(r *drivers.Result) {
					r.Harness, r.Session, r.Settled = "claude-code-pro", s, false
					r.Usage = &drivers.Usage{Input: 17, Output: 5, CacheRead: 4, CacheWrite: 3, TotalTokens: 22}
					r.CostUSD = nil
					message := "429 rate limit"
					r.ErrorMessage = &message
				})
				stub := newStub(map[string]drivers.Result{role: r})
				m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: &drivers.Drivers{Intake: stub, Garden: stub},
					Sleep: func(int64) { t.Error("an unconfirmed invocation must not enter retry backoff") }})
				o := CallOptions{Role: role, Binding: bindingFor(t, cfg, role), Prompt: "advice"}
				result := CallAgent(m, o)
				assert.False(t, result.OK)
				assert.Equal(t, errs.TRANSPORT, result.ErrorCode)
				require.ErrorContains(t, m.fatalErr, "invocation outcome is unconfirmed")
				assert.False(t, CallAgent(m, o).OK)
				assert.Equal(t, []string{role}, stub.calls, "later candidates and fallback channels remain blocked")
				st, err := state.Load(taskDir)
				require.NoError(t, err)
				assert.Equal(t, "escalated", st.Status)
				require.Len(t, st.Inbox, 1)
				assert.Equal(t, int64(22), st.Budget.Totals.Tokens)
				inbox, err := os.ReadFile(filepath.Join(taskDir, "inbox.md"))
				require.NoError(t, err)
				assert.Contains(t, string(inbox), role+" invocation")
				assert.NotContains(t, string(inbox), "cleanup is unconfirmed", "unknown terminal facts do not invent a cleanup failure")
				entries := readMetricsLinesT(t, m.MetricsFile)
				require.Len(t, entries, 1)
				assert.Nil(t, entries[0]["cleanup-error"])
				assert.NotNil(t, entries[0]["usage"], "keep trustworthy consumption even when application is unsafe")
				assert.Equal(t, "uncertain-advice", entries[0]["invocation"].(map[string]any)["requestId"])
			})
		}
	}
}

func TestModuleKnownInvocationDoesNotBlockSubsequentAdvice(t *testing.T) {
	for _, outcome := range []string{"completed", "failed", "cancelled"} {
		cfg := config.Default()
		m := NewContext(CtxOpts{TaskDir: t.TempDir(), Config: &cfg})
		for _, dispatch := range []string{"dispatched", "not-dispatched"} {
			assert.False(t, m.stopForUnknownInvocation("garden", drivers.Result{Session: &drivers.SessionOutcome{
				Dispatch: dispatch, Outcome: outcome, CleanupConfirmed: true,
			}}))
		}
		assert.NoError(t, m.fatalErr)
	}
}

func TestModuleClaudeMetricsKeepReportedCostSeparateAndPriceCachesOnce(t *testing.T) {
	for _, harness := range []string{"claude-code-pro", "codex-pro", "dsh", "pi"} {
		t.Run(harness, func(t *testing.T) {
			taskDir := t.TempDir()
			writeFileT(t, filepath.Join(taskDir, "state.json"), `{"revision":0,"budget":{"daily":{},"cache":{"by-provider":{}}}}`)
			cfg := testConfigT(t, taskDir)
			m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg})
			receipt := json.RawMessage(`{"protocol":1,"requestId":"advice","reportedCostUSD":99,"costUSD":null,"costStatus":"unknown","usage":{"status":"partial","inputTokens":42,"outputTokens":null}}`)
			r := drivers.Result{Harness: harness, ClaudeCodeProReceipt: receipt}
			require.NoError(t, recordModuleMetrics(m, "garden", "priced", "subscription", "prompt", r, ""))
			entry := readMetricsLinesT(t, m.MetricsFile)[0]
			encoded, err := json.Marshal(entry["claude-code-pro-receipt"])
			require.NoError(t, err)
			assert.JSONEq(t, string(receipt), string(encoded))
			assert.Nil(t, entry["usage"])
			assert.Nil(t, entry["cost-usd"])
			assert.Nil(t, entry["provider"])
			cfg.Pricing = map[string]config.Price{"priced": {Input: 2, Output: 3, CacheRead: 0.5}}
			r.Usage = &drivers.Usage{Input: 17, Output: 5, CacheRead: 4, CacheWrite: 3, TotalTokens: 22}
			if harness == "pi" {
				r.Usage.Input = 10
			}
			require.NoError(t, recordModuleMetrics(m, "garden", "priced", "subscription", "prompt", r, ""))
			entries := readMetricsLinesT(t, m.MetricsFile)
			require.Len(t, entries, 2)
			assert.InDelta(t, 0.000043, entries[1]["cost-usd"], 1e-10)
			st, err := state.Load(taskDir)
			require.NoError(t, err)
			assert.Equal(t, int64(22), st.Budget.Totals.Tokens)
			assert.InDelta(t, 0.000043, st.Budget.Totals.CostUSD, 1e-10)
			assert.Equal(t, int64(1), st.Budget.UnknownUsageCalls)
			assert.Equal(t, int64(1), st.Budget.UnknownCostCalls)
		})
	}
}
