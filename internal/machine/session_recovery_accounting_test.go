package machine

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func recoveryAccountingFixture(t *testing.T) (*Context, *state.SessionRecord, drivers.Result) {
	t.Helper()
	_, c := sessionFixture(t)
	day := c.Now().UTC().Format("2006-01-02")
	c.State.Budget.UnknownUsageCalls, c.State.Budget.UnknownCostCalls = 1, 1
	c.State.Budget.Daily[day] = state.DailyBucket{UnknownUsageCalls: 1, UnknownCostCalls: 1}
	r := &state.SessionRecord{Role: "execute", Scope: "M01-T01", Last: &state.SessionInvocation{ID: "call-accounting", Accounted: true,
		ReceiptDigest: strings.Repeat("b", 64), Accounting: &state.SessionAccounting{Day: day, Provider: "subscription", ReceiptDigest: strings.Repeat("a", 64)}}}
	result := drivers.Result{Harness: "codex-pro", Model: "test", Recovered: true, Usage: &drivers.Usage{Input: 10, Output: 2, CacheRead: 4, TotalTokens: 12}, CostUSD: new(0.125)}
	return c, r, result
}

func TestRecoveryAccountingRefinesUnknownExactlyOnceAcrossCASRetry(t *testing.T) {
	c, r, result := recoveryAccountingFixture(t)
	require.NoError(t, c.reconcileSessionAccounting(r, result))
	assert.Equal(t, int64(12), c.State.Budget.Totals.Tokens)
	assert.Equal(t, 0.125, c.State.Budget.Totals.CostUSD)
	assert.Zero(t, c.State.Budget.UnknownUsageCalls)
	assert.Zero(t, c.State.Budget.UnknownCostCalls)
	require.NoError(t, c.reconcileSessionAccounting(r, result))
	assert.Equal(t, int64(12), c.State.Budget.Totals.Tokens)
	raw, err := os.ReadFile(c.metricsFile)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(raw), "reconciles-invocation"))
	assert.Contains(t, string(raw), `"record-kind":"reconciliation"`)
	assert.Contains(t, string(raw), `"cacheRead":4`)
	// The audit row can already exist when the state CAS fails. A reloaded old
	// accounting snapshot must apply its increment once without another row.
	c.State.Budget.Totals = state.BudgetTotals{}
	c.State.Budget.UnknownUsageCalls, c.State.Budget.UnknownCostCalls = 1, 1
	r.Last.Accounting.Usage, r.Last.Accounting.CostUSD, r.Last.Accounting.ReceiptDigest = nil, nil, strings.Repeat("a", 64)
	require.NoError(t, c.reconcileSessionAccounting(r, result))
	assert.Equal(t, int64(12), c.State.Budget.Totals.Tokens)
	raw, err = os.ReadFile(c.metricsFile)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(raw), "reconciles-invocation"))
}

func TestRecoveryAccountingPreservesKnownValuesAndUnknownCost(t *testing.T) {
	for _, scenario := range []string{"cost-only", "unknown-cost", "priced-cost", "still-unknown", "pruned-day", "known-both"} {
		t.Run(scenario, func(t *testing.T) {
			c, r, result := recoveryAccountingFixture(t)
			switch scenario {
			case "cost-only":
				r.Last.Accounting.Usage = sessionAccountedUsage(result.Usage)
				result.Usage = nil
			case "unknown-cost":
				result.CostUSD = nil
			case "priced-cost":
				result.CostUSD = nil
				c.Config.Pricing = map[string]config.Price{"test": {Input: 2, Output: 3, CacheRead: 1}}
			case "still-unknown":
				result.Usage, result.CostUSD = nil, nil
			case "pruned-day":
				r.Last.Accounting.Day = "2026-09-01"
				c.State.Budget.Daily = map[string]state.DailyBucket{}
				for day := 1; day <= 31; day++ {
					c.State.Budget.Daily[fmt.Sprintf("2026-10-%02d", day)] = state.DailyBucket{Tokens: int64(day)}
				}
			case "known-both":
				r.Last.Accounting.Usage = sessionAccountedUsage(result.Usage)
				r.Last.Accounting.CostUSD = result.CostUSD
			}
			require.NoError(t, c.reconcileSessionAccounting(r, result))
			if scenario == "unknown-cost" || scenario == "still-unknown" {
				assert.Equal(t, int64(1), c.State.Budget.UnknownCostCalls)
			}
			if scenario == "known-both" || scenario == "still-unknown" {
				assert.Zero(t, c.State.Budget.Totals.Tokens)
			}
			if scenario == "priced-cost" {
				assert.Equal(t, 0.000022, c.State.Budget.Totals.CostUSD)
			}
			if scenario == "pruned-day" {
				assert.Len(t, c.State.Budget.Daily, 31)
				assert.NotContains(t, c.State.Budget.Daily, "2026-09-01")
				for day := 1; day <= 31; day++ {
					assert.Equal(t, int64(day), c.State.Budget.Daily[fmt.Sprintf("2026-10-%02d", day)].Tokens)
				}
				assert.Equal(t, int64(12), c.State.Budget.Totals.Tokens)
			}
		})
	}
	assert.Nil(t, sessionAccountedUsage(nil))
	assert.Equal(t, int64(12), sessionAccountedUsage(&drivers.Usage{Input: 10, Output: 2}).TotalTokens)
}

func TestRecoveryAccountingRefusesContradictionsAndWriteFailures(t *testing.T) {
	for _, scenario := range []string{"usage", "cost", "missing-unknown", "metrics"} {
		c, r, result := recoveryAccountingFixture(t)
		switch scenario {
		case "usage":
			r.Last.Accounting.Usage = &state.SessionUsage{Input: 99}
		case "cost":
			r.Last.Accounting.CostUSD = new(0.5)
		case "missing-unknown":
			c.State.Budget.UnknownUsageCalls = 0
		case "metrics":
			c.metricsFile = t.TempDir()
		}
		require.Error(t, c.reconcileSessionAccounting(r, result))
		assert.Zero(t, c.State.Budget.Totals.Tokens)
		c.accountSessionReceipt(r, result)
		require.Error(t, c.err())
	}
}
