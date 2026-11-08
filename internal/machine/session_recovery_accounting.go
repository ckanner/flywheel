package machine

import (
	"fmt"
	"reflect"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/metrics"
	"github.com/ckanner/flywheel/internal/state"
)

func sessionAccountedUsage(usage *drivers.Usage) *state.SessionUsage {
	if usage == nil {
		return nil
	}
	total := usage.TotalTokens
	if total == 0 {
		total = usage.Input + usage.Output
	}
	return &state.SessionUsage{Input: usage.Input, Output: usage.Output, Reasoning: usage.Reasoning,
		CacheRead: usage.CacheRead, CacheWrite: usage.CacheWrite, TotalTokens: total}
}

// A late public receipt refines the original invocation, never creates another
// model call. Append a distinguishable audit correction, then apply only newly
// known increments in the same state CAS as its updated accounting snapshot.
func (c *Context) reconcileSessionAccounting(r *state.SessionRecord, result drivers.Result) error {
	old := r.Last.Accounting
	if old.ReceiptDigest == r.Last.ReceiptDigest {
		return nil
	}
	next := *old
	next.ReceiptDigest = r.Last.ReceiptDigest
	usage := sessionAccountedUsage(result.Usage)
	if old.Usage != nil && usage != nil && !reflect.DeepEqual(old.Usage, usage) {
		return fmt.Errorf("recovered usage contradicts the already-accounted invocation")
	}
	if old.CostUSD != nil && result.CostUSD != nil && *old.CostUSD != *result.CostUSD {
		return fmt.Errorf("recovered cost contradicts the already-accounted invocation")
	}
	if next.Usage == nil {
		next.Usage = usage
	}
	if next.CostUSD == nil {
		next.CostUSD = result.CostUSD
		if next.CostUSD == nil && usage != nil {
			next.CostUSD = metrics.EstimateCostUSD(&metrics.Usage{Input: usage.Input, Output: usage.Output,
				CacheRead: usage.CacheRead, CacheWrite: usage.CacheWrite, InputIncludesCache: metrics.InputIncludesCache(result.Harness)}, metrics.PriceForModel(c.Config.Pricing, result.Model))
		}
	}
	usageAdded, costAdded := old.Usage == nil && next.Usage != nil, old.CostUSD == nil && next.CostUSD != nil
	if (usageAdded && c.State.Budget.UnknownUsageCalls <= 0) || (costAdded && c.State.Budget.UnknownCostCalls <= 0) {
		return fmt.Errorf("recovery accounting is missing the original unknown counter")
	}
	var metricUsage any
	if u := next.Usage; u != nil {
		metricUsage = drivers.Usage{Input: u.Input, Output: u.Output, Reasoning: u.Reasoning,
			CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, TotalTokens: u.TotalTokens}
	}
	if err := metrics.AppendOnce(c.metricsFile, r.Last.ID+":reconcile:"+next.ReceiptDigest, next.ReceiptDigest, map[string]any{
		"record-kind": "reconciliation", "reconciles-invocation": r.Last.ID, "supersedes-receipt": old.ReceiptDigest,
		"role": r.Role, "model": result.Model, "harness": result.Harness, "usage": metricUsage, "cost-usd": next.CostUSD,
		"usage-added": usageAdded, "cost-added": costAdded, "session": r,
		"result": map[string]any{"settled": result.Settled, "cancelled": result.Cancelled, "truncated": result.Truncated,
			"signal": result.Signal, "cleanup-error": result.CleanupError, "session-id": result.SessionID, "recovered": true},
	}); err != nil {
		return fmt.Errorf("recovery accounting could not be persisted: %w", err)
	}
	bucket, dayRetained := c.State.Budget.Daily[old.Day]
	if usageAdded {
		c.State.Budget.UnknownUsageCalls--
		if dayRetained && bucket.UnknownUsageCalls > 0 {
			bucket.UnknownUsageCalls--
		}
	}
	if costAdded {
		c.State.Budget.UnknownCostCalls--
		if dayRetained && bucket.UnknownCostCalls > 0 {
			bucket.UnknownCostCalls--
		}
	}
	if dayRetained {
		c.State.Budget.Daily[old.Day] = bucket
	}
	if usageAdded || costAdded {
		u := state.Usage{}
		if usageAdded {
			u = state.Usage{Input: next.Usage.Input, Output: next.Usage.Output, Reasoning: next.Usage.Reasoning,
				CacheRead: next.Usage.CacheRead, CacheWrite: next.Usage.CacheWrite, TotalTokens: &next.Usage.TotalTokens}
		}
		var cost *float64
		if costAdded {
			cost = next.CostUSD
		}
		retainedDays := c.State.Budget.Daily
		if !dayRetained {
			// Refining an expired invocation updates lifetime/provider totals,
			// not a misleading partial day bucket or the retained day set.
			c.State.Budget.Daily = map[string]state.DailyBucket{}
		}
		state.RecordUsage(c.State, u, state.UsageOpts{Provider: old.Provider, Day: old.Day, CostUSD: cost})
		c.State.Budget.Daily = retainedDays
	}
	r.Last.Accounting = &next
	return nil
}
