package machine

import (
	"encoding/json"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeCodeProMetricsPreserveUnknownUsageAndReportedCost(t *testing.T) {
	c, dir := spawnMetricsCtx(t)
	receipt := json.RawMessage(`{"protocol":1,"requestId":"owned-call","reportedCostUSD":123.45,"costUSD":null,"costStatus":"unknown","usage":{"status":"partial","inputTokens":42,"outputTokens":null},"configuration":{"source":"cli-init","matched":true}}`)
	r := drivers.Result{Harness: "claude-code-pro", ClaudeCodeProReceipt: receipt}
	c.recordSpawnMetrics(spawnMetricsArgs{role: "execute", model: "sonnet", channel: "claude-subscription", result: &r})
	entry := lastMetricsLine(t, dir)
	encoded, err := json.Marshal(entry["claude-code-pro-receipt"])
	require.NoError(t, err)
	assert.JSONEq(t, string(receipt), string(encoded))
	assert.Nil(t, entry["usage"])
	assert.Nil(t, entry["cost-usd"], "API-equivalent reported cost is not the subscription charge")
	assert.Nil(t, entry["provider"])
	assert.Equal(t, "claude-code-pro", entry["harness"])
	assert.Equal(t, "claude-subscription", entry["channel"])
	assert.Equal(t, int64(1), c.State.Budget.UnknownUsageCalls)
	assert.Equal(t, int64(1), c.State.Budget.UnknownCostCalls)
	assert.Zero(t, c.State.Budget.Totals.Tokens)
}

func TestMachinePricesInclusiveUsageWithoutCountingCachesTwice(t *testing.T) {
	for _, harness := range []string{"claude-code-pro", "codex-pro", "dsh", "pi"} {
		t.Run(harness, func(t *testing.T) {
			c, dir := spawnMetricsCtx(t)
			c.Config.Pricing = map[string]config.Price{"priced": {Input: 2, Output: 3, CacheRead: 0.5}}
			r := drivers.Result{Harness: harness, Usage: &drivers.Usage{Input: 17, Output: 5, CacheRead: 4, CacheWrite: 3, TotalTokens: 22}}
			if harness == "pi" {
				r.Usage.Input = 10 // the legacy driver provides disjoint input and cache buckets
			}
			c.recordSpawnMetrics(spawnMetricsArgs{role: "execute", model: "priced", result: &r})
			entry := lastMetricsLine(t, dir)
			// 10 input * $2 + 5 output * $3 + 4 cache reads * $0.5 +
			// 3 cache writes * $2, divided by one million tokens.
			assert.InDelta(t, 0.000043, entry["cost-usd"], 1e-10)
			assert.Equal(t, int64(22), c.State.Budget.Totals.Tokens)
			assert.InDelta(t, 0.000043, c.State.Budget.Totals.CostUSD, 1e-10)
			assert.Zero(t, c.State.Budget.UnknownCostCalls)
		})
	}
}

func TestClaudeCodeProHistoryIdentityTracksRuntimeButNeverSecretValues(t *testing.T) {
	a := drivers.CallArgs{Binding: config.LoopBinding{Harness: "claude-code-pro"}, APIKeys: map[string]string{"ANTHROPIC_API_KEY": "first-secret"}}
	for _, name := range []string{"CLAUDE_CONFIG_DIR", drivers.ClaudeCodeProBinaryEnv, drivers.ClaudeCodeProWorkerEnv} {
		t.Setenv(name, "/initial/runtime")
		before := sessionRuntimeIdentity(a)
		t.Setenv(name, "/another/runtime")
		assert.NotEqual(t, before, sessionRuntimeIdentity(a), name)
	}
	before := sessionRuntimeIdentity(a)
	a.APIKeys["ANTHROPIC_API_KEY"] = "rotated-secret"
	t.Setenv("ANTHROPIC_API_KEY", "irrelevant-ambient-secret")
	assert.Equal(t, before, sessionRuntimeIdentity(a))
	a.APIKeys["ANTHROPIC_AUTH_TOKEN"] = "another-explicit-key"
	assert.NotEqual(t, before, sessionRuntimeIdentity(a), "credential references remain part of the binding")
}
