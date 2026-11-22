package machine

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/state"
)

// attachPreparation runs before implementation admission. A report-only goal
// cannot silently become an implementation task. Existing sources are immutable
// and usage is imported once in the same state CAS as the provenance marker.
func (c *Context) attachPreparation() error {
	preparation, err := state.LoadPreparation(c.TaskDir)
	if os.IsNotExist(err) && c.State.PreparationSource == nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load investigation provenance: %w", err)
	}
	if preparation.Cwd != c.Cwd || preparation.Status != "supported" || preparation.Report == nil || preparation.Pending != nil || preparation.DecisionPending != nil || preparation.AnalysisPending != nil ||
		preparation.Contract.Intent != "investigate-and-fix" && !c.planningRepairApproved(preparation) {
		return fmt.Errorf("investigation is not settled with repair authority and a supported report for this workspace")
	}
	return c.importPreparation(preparation)
}

// Imported investigation costs and identity survive planning, retries and later
// explicit repair authorization. Importing evidence never changes its intent.
func (c *Context) importPreparation(preparation *state.PreparationState) error {
	actions, _, elapsed := state.PreparationTotals(preparation)
	source := state.PreparationSource{Version: 1, ID: preparation.Contract.ID, RunID: preparation.RunID, ContractHash: preparation.ContractHash, ReportHash: state.PreparationHash(preparation.Report), ImplementationScope: preparation.Contract.ImplementationScope, ElapsedMS: elapsed, Actions: actions, Accounted: true}
	if len(preparation.History) > 0 {
		source.Round = len(preparation.History) + 1
	}
	if old := c.State.PreparationSource; old != nil {
		if state.PreparationHash(old) != state.PreparationHash(source) {
			return fmt.Errorf("investigation provenance changed after implementation admission")
		}
		return nil
	}
	accountPreparationUsage(c.State, preparation)
	c.State.PreparationSource = &source
	return nil
}

// The same receipt accounting is used by analysis admission and by the one-time
// implementation handoff; immutable round history is never charged twice.
func accountPreparationUsage(target *state.State, preparation *state.PreparationState) {
	for _, round := range state.PreparationRounds(preparation) {
		for _, receipt := range round.Decisions {
			if receipt.Status == "skipped" {
				continue
			}
			at := round.DecisionStartedAt[receipt.DecisionID]
			// LoadPreparation validates every original dispatch time and its receipt
			// identity. Do not reclassify that checked value using today's clock.
			when, _ := time.Parse(time.RFC3339Nano, at)
			day := when.UTC().Format("2006-01-02")
			usage := state.Usage{}
			if receipt.Usage.Input != nil {
				usage.Input = *receipt.Usage.Input
			}
			if receipt.Usage.Output != nil {
				usage.Output = *receipt.Usage.Output
			}
			state.RecordUsage(target, usage, state.UsageOpts{Day: day, Provider: "jev-decision"})
			bucket := target.Budget.Daily[day]
			if receipt.Usage.Input == nil || receipt.Usage.Output == nil {
				target.Budget.UnknownUsageCalls++
				bucket.UnknownUsageCalls++
			}
			target.Budget.UnknownCostCalls++
			bucket.UnknownCostCalls++
			target.Budget.Daily[day] = bucket
		}
	}
	for _, round := range state.PreparationRounds(preparation) {
		a := round.Analysis
		if a == nil || a.Dispatch == "not-dispatched" {
			continue
		}
		when, _ := time.Parse(time.RFC3339Nano, a.Request.StartedAt)
		day := when.UTC().Format("2006-01-02")
		usage := state.Usage{}
		if a.Usage != nil {
			usage = *a.Usage
		}
		state.RecordUsage(target, usage, state.UsageOpts{Day: day, Provider: a.Request.Binding.Harness, CostUSD: a.CostUSD})
		bucket := target.Budget.Daily[day]
		if a.Usage == nil || a.UsageStatus != "complete" {
			target.Budget.UnknownUsageCalls++
			bucket.UnknownUsageCalls++
		}
		if a.CostUSD == nil {
			target.Budget.UnknownCostCalls++
			bucket.UnknownCostCalls++
		}
		target.Budget.Daily[day] = bucket
	}
}

func (c *Context) preparationScopeViolations(paths []string) []boundaryViolationLite {
	if c.State.PreparationSource == nil {
		return nil
	}
	violations := []boundaryViolationLite{}
	scopeEnvelope := c.effectivePreparationScope()
	for _, path := range paths {
		allowed := false
		for _, scope := range scopeEnvelope {
			if scope == "." || path == scope || strings.HasPrefix(path, scope+"/") {
				allowed = true
				break
			}
		}
		if !allowed {
			violations = append(violations, boundaryViolationLite{Type: "investigation-scope", Path: path})
		}
	}
	return violations
}
