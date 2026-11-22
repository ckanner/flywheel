package machine

import (
	"errors"
	"fmt"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/prompts"
)

var errCompactIneligible = errors.New("compact preparation does not qualify")

// Validate both artifacts against a staging copy before adopting either. A
// rejected compact proposal cannot leave a half-adopted root design behind.
func (c *Context) prepareCompactProjection(request planning.Request, design planning.Design, output *prompts.CompactOutput) (*planning.ExecutionPlan, error) {
	if output == nil {
		return nil, nil
	}
	if planning.DesignGranularity(c.State.Planning, request) != "compact" || output.Version != 1 {
		return nil, fmt.Errorf("unexpected compact preparation or unsupported compact protocol")
	}
	if len(design.Milestones) != 1 || len(output.Files) != 2 {
		return nil, errCompactIneligible
	}
	staged := *c.State
	staged.Planning = planning.Clone(c.State.Planning)
	if err := planning.RecordDesign(staged.Planning, design); err != nil {
		return nil, err
	}
	shadow := *c
	shadow.State = &staged
	child := planning.Request{Scope: "milestone", ScopeID: design.Milestones[0].ID}
	if err := shadow.validatePlanningFiles(output.Files, child); err != nil {
		return nil, err
	}
	// validatePlanningFiles already parsed and cross-validated every byte below.
	spec, _ := contract.ParseSpec([]byte(output.Files["spec.md"]), "spec.md")
	packets := []*contract.Packet{}
	for name, raw := range output.Files {
		if name != "spec.md" {
			packet, _ := contract.ParsePacket([]byte(raw), name)
			packets = append(packets, packet)
		}
	}
	mode := c.State.Planning.Brief.Mode
	if c.planningMode != "" {
		mode = c.planningMode
	}
	if mode == "" {
		mode = "auto"
	}
	selection, err := contract.SelectWorkflow(spec, packets, mode)
	if err != nil || selection.Effective != "quick" {
		return nil, fmt.Errorf("%w: %s", errCompactIneligible, selection.Reason)
	}
	dependencies, err := c.planScopeIdentity()
	if err != nil {
		return nil, err
	}
	result, err := planning.SealPlan(planning.ExecutionPlan{Preparation: "compact-v1", Ref: planning.Ref{Revision: len(staged.Planning.Plans) + 1}, Design: design.Ref,
		InputRevision: design.InputRevision, Scope: child.Scope, ScopeID: child.ScopeID, Files: output.Files, Dependencies: dependencies, ScopeVerdict: "within-envelope", PlannerInvocation: design.PlannerInvocation})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func escalateCompact(p *planning.State, r *planning.Request) {
	r.DesignGranularity, r.Stage, r.Role, r.Channel = "system", "", "", ""
	p.Reason = "局部紧凑准备未证明资格，保留已用预算并升级系统设计。"
}
