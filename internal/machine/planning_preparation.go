package machine

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
)

// planningResearchContext imports settled investigation cost before a planning
// budget gate and presents its exact report as evidence, not implementation
// authorization. The source marker and usage are persisted together once.
func (c *Context) planningResearchContext() (string, error) {
	preparation, err := planningPreparation(c.TaskDir, c.Cwd)
	if err != nil {
		return "", err
	}
	if preparation == nil {
		if c.State.PreparationSource != nil {
			return "", fmt.Errorf("the recorded investigation report is no longer available")
		}
		return "", nil
	}
	imported := c.State.PreparationSource == nil
	if err := c.importPreparation(preparation); err != nil {
		return "", err
	}
	if imported {
		c.save()
		if err := c.err(); err != nil {
			return "", err
		}
	}
	raw, _ := json.MarshalIndent(struct {
		Report *state.InvestigationReport `json:"report"`
		Source *state.PreparationSource   `json:"source"`
	}{preparation.Report, c.State.PreparationSource}, "", "  ")
	return "Verified read-only investigation evidence; retain its limitations and original authority:\n" + string(raw), nil
}

func planningPreparation(taskDir, cwd string) (*state.PreparationState, error) {
	preparation, err := state.LoadPreparation(taskDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if preparation.Cwd != cwd || preparation.Status != "supported" || preparation.Report == nil || preparation.Pending != nil || preparation.DecisionPending != nil {
		return nil, fmt.Errorf("planning handoff requires a supported, settled investigation report for this workspace")
	}
	return preparation, nil
}

func planningScopeWithin(scope, envelope []string) bool {
	for _, item := range scope {
		allowed := false
		for _, parent := range envelope {
			if parent == "." || item == parent || strings.HasPrefix(item, parent+"/") {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	return true
}

func planningRepairAuthority(opts RunOpts, st *state.State, r PlanningCommand) (string, error) {
	if !r.StartRequested(false) {
		return "", nil
	}
	preparation, err := planningPreparation(opts.TaskDir, st.Project.Cwd)
	if err != nil || preparation == nil {
		return "", err
	}
	design := planning.FindDesign(st.Planning, *r.Design)
	if design == nil || preparation.Contract.Intent == "investigate-and-fix" && !planningScopeWithin(design.ImplementationScope, preparation.Contract.ImplementationScope) {
		return "", fmt.Errorf("repair authorization must remain within the supported investigation scope")
	}
	hash := state.PreparationHash(preparation.Report)
	if old := st.PreparationSource; old != nil && (old.ReportHash != hash || old.ContractHash != preparation.ContractHash) {
		return "", fmt.Errorf("investigation report changed since the planning handoff")
	}
	return hash, nil
}

func (c *Context) planningRepairApproved(preparation *state.PreparationState) bool {
	p := c.State.Planning
	if p == nil || p.CurrentDesign == nil {
		return false
	}
	for _, approval := range p.Approvals {
		if approval.Kind == "human" && approval.Start && approval.Design == *p.CurrentDesign && approval.ReportHash == state.PreparationHash(preparation.Report) {
			design := planning.FindDesign(p, approval.Design)
			return design != nil && (preparation.Contract.Intent == "investigate" || planningScopeWithin(design.ImplementationScope, preparation.Contract.ImplementationScope))
		}
	}
	return false
}

func (c *Context) effectivePreparationScope() []string {
	scope := c.State.PreparationSource.ImplementationScope
	if c.State.Planning == nil {
		return scope
	}
	preparation, err := planningPreparation(c.TaskDir, c.Cwd)
	if err == nil && preparation != nil && preparation.Contract.Intent == "investigate" &&
		state.PreparationHash(preparation.Report) == c.State.PreparationSource.ReportHash && c.planningRepairApproved(preparation) {
		return planning.FindDesign(c.State.Planning, *c.State.Planning.CurrentDesign).ImplementationScope
	}
	return scope
}
