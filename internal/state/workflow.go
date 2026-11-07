package state

import (
	"encoding/json"
	"fmt"

	"github.com/ckanner/flywheel/internal/contract"
)

// ResolveWorkflow is a read-only resolver shared by runtime and diagnostics.
// nil state denotes a new task; an existing state without policy is legacy Full.
// Resume never reclassifies. The caller must persist an explicit Full upgrade
// before launching any model or changing contracts.
func ResolveWorkflow(st *State, spec *contract.Spec, packets []*contract.Packet, mode string) (contract.WorkflowSelection, error) {
	if mode != "" && !contract.ValidWorkflowMode(mode) {
		return contract.WorkflowSelection{}, fmt.Errorf("unsupported workflow mode %q", mode)
	}
	if st == nil {
		return contract.SelectWorkflow(spec, packets, mode)
	}
	if st.Workflow == nil {
		if mode == "quick" {
			return contract.WorkflowSelection{}, fmt.Errorf("legacy Full task cannot be downgraded to Quick")
		}
		legacy := *spec
		legacy.Workflow = nil
		return contract.SelectWorkflow(&legacy, packets, "full")
	}
	raw, _ := json.Marshal(st.Workflow)
	var value any
	_ = json.Unmarshal(raw, &value)
	if err := contract.ValidateWorkflowStateValue(value); err != nil {
		return contract.WorkflowSelection{}, err
	}
	selection := st.Workflow.WorkflowSelection
	if mode == "quick" && selection.Effective != "quick" {
		return contract.WorkflowSelection{}, fmt.Errorf("persisted Full workflow cannot be downgraded to Quick")
	}
	if mode == "full" && selection.Effective == "quick" {
		selection.Effective, selection.Reason = "full", "User explicitly requested Full"
		selection.Stages = contract.WorkflowStages("full")
	}
	return selection, nil
}
