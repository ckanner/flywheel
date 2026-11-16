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
	if preparation.Cwd != c.Cwd || preparation.Status != "supported" || preparation.Report == nil || preparation.Pending != nil || preparation.DecisionPending != nil ||
		preparation.Contract.Intent != "investigate-and-fix" && !c.planningRepairApproved(preparation) {
		return fmt.Errorf("investigation is not settled with repair authority and a supported report for this workspace")
	}
	return c.importPreparation(preparation)
}

// Imported investigation costs and identity survive planning, retries and later
// explicit repair authorization. Importing evidence never changes its intent.
func (c *Context) importPreparation(preparation *state.PreparationState) error {
	source := state.PreparationSource{Version: 1, ID: preparation.Contract.ID, RunID: preparation.RunID, ContractHash: preparation.ContractHash, ReportHash: state.PreparationHash(preparation.Report), ImplementationScope: preparation.Contract.ImplementationScope, ElapsedMS: preparation.ElapsedMS, Actions: preparation.ActionsUsed, Accounted: true}
	if old := c.State.PreparationSource; old != nil {
		if state.PreparationHash(old) != state.PreparationHash(source) {
			return fmt.Errorf("investigation provenance changed after implementation admission")
		}
		return nil
	}
	for _, receipt := range preparation.Decisions {
		if receipt.Status == "skipped" {
			continue
		}
		at := preparation.DecisionStartedAt[receipt.DecisionID]
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
		state.RecordUsage(c.State, usage, state.UsageOpts{Day: day, Provider: "jev-decision"})
		bucket := c.State.Budget.Daily[day]
		if receipt.Usage.Input == nil || receipt.Usage.Output == nil {
			c.State.Budget.UnknownUsageCalls++
			bucket.UnknownUsageCalls++
		}
		c.State.Budget.UnknownCostCalls++
		bucket.UnknownCostCalls++
		c.State.Budget.Daily[day] = bucket
	}
	c.State.PreparationSource = &source
	return nil
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
