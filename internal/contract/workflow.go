package contract

import (
	"fmt"
	"strings"
)

// Workflow is the host's declared assessment, not an executable verdict. The
// engine checks these facts and structural constraints before selecting Quick.
type Workflow struct {
	Version              int               `json:"version"`
	Mode                 string            `json:"mode"`
	Reason               string            `json:"reason,omitempty"`
	ScopeClear           bool              `json:"scope-clear,omitempty"`
	ImplementationKnown  bool              `json:"implementation-known,omitempty"`
	VerificationComplete bool              `json:"verification-complete,omitempty"`
	ReviewSufficient     bool              `json:"review-sufficient,omitempty"`
	Evidence             []string          `json:"evidence,omitempty"`
	Risks                []string          `json:"risks,omitempty"`
	NotApplicable        map[string]string `json:"not-applicable,omitempty"`
}

type StageApplicability struct {
	Applicable bool   `json:"applicable"`
	Reason     string `json:"reason"`
}

// WorkflowSelection is immutable across ordinary resume. One-way upgrades are
// persisted by machine/state; SelectWorkflow itself is read-only.
type WorkflowSelection struct {
	Version   int                           `json:"version"`
	Requested string                        `json:"requested"`
	Effective string                        `json:"effective"`
	Reason    string                        `json:"reason"`
	Evidence  []string                      `json:"evidence"`
	Risks     []string                      `json:"risks"`
	Stages    map[string]StageApplicability `json:"stages"`
}

// ValidWorkflowMode accepts only public mode names, with no aliases or case
// folding. Empty is handled by callers as the absence of an override.
func ValidWorkflowMode(mode string) bool {
	return mode == "auto" || mode == "quick" || mode == "full"
}

// SelectWorkflow resolves a new task's requested policy. Legacy contracts use
// Full; a user asking for Quick cannot override missing qualification evidence.
func SelectWorkflow(spec *Spec, packets []*Packet, requested string) (WorkflowSelection, error) {
	w := spec.Workflow
	if w != nil && (w.Version != 1 || !ValidWorkflowMode(w.Mode)) {
		return WorkflowSelection{}, fmt.Errorf("unsupported workflow version or mode")
	}
	if requested == "" {
		requested = "full"
		if w != nil {
			requested = w.Mode
		}
	}
	if !ValidWorkflowMode(requested) {
		return WorkflowSelection{}, fmt.Errorf("unsupported workflow mode %q", requested)
	}
	s := WorkflowSelection{Version: 1, Requested: requested, Effective: "full",
		Reason: "Full requested or legacy task without workflow assessment", Evidence: []string{}, Risks: []string{}}
	if w != nil {
		s.Evidence = append(s.Evidence, w.Evidence...)
		s.Risks = append(s.Risks, w.Risks...)
	}
	if requested != "full" {
		reasons := quickIneligibility(spec, packets)
		if len(reasons) == 0 {
			s.Effective, s.Reason = "quick", w.Reason
		} else {
			s.Reason = strings.Join(reasons, "; ")
		}
	}
	s.Stages = WorkflowStages(s.Effective)
	return s, nil
}

// WorkflowStages records non-applicability explicitly; skipped stages never
// pretend to have a passing model verdict.
func WorkflowStages(effective string) map[string]StageApplicability {
	if effective == "quick" {
		return map[string]StageApplicability{
			"plan":  {Reason: "Prepared compact contract is qualified; runtime Plan is not applicable"},
			"judge": {Reason: "Independent final Review covers all requirements and validation; separate Judge is not applicable"},
		}
	}
	return map[string]StageApplicability{
		"plan":  {Applicable: true, Reason: "Full workflow requires runtime planning"},
		"judge": {Applicable: true, Reason: "Full workflow requires milestone final verification"},
	}
}

func quickIneligibility(spec *Spec, packets []*Packet) []string {
	w := spec.Workflow
	if w == nil {
		return []string{"Quick assessment is missing"}
	}
	if w.Mode == "full" {
		return []string{"Spec explicitly requires Full"}
	}
	var reasons []string
	if !w.ScopeClear || !w.ImplementationKnown || !w.VerificationComplete || !w.ReviewSufficient ||
		strings.TrimSpace(w.Reason) == "" || len(w.Evidence) == 0 {
		reasons = append(reasons, "Quick requires a clear scope, known implementation, complete checks and sufficient independent review, with a reason and code evidence")
	}
	if len(w.Risks) > 0 {
		reasons = append(reasons, "Declared risks require Full: "+strings.Join(w.Risks, "; "))
	}
	if len(spec.Milestones) != 1 || len(packets) != 1 {
		return append(reasons, "Quick requires exactly one milestone and one independently verifiable packet")
	}
	p := packets[0]
	milestone, err := MilestoneOfPacket(p.PacketID)
	if err != nil || milestone != spec.Milestones[0].ID {
		reasons = append(reasons, "Quick packet must belong to the sole milestone")
	}
	covered := map[string]bool{}
	for _, id := range p.Requirements {
		covered[id] = true
	}
	for _, r := range spec.Requirements {
		if !covered[r.ID] || r.Milestone != milestone || strings.TrimSpace(r.Verification) == "" {
			reasons = append(reasons, "Quick requirement "+r.ID+" needs packet coverage and an explicit verification")
		}
	}
	if len(p.Acceptance.L1) == 0 {
		reasons = append(reasons, "Quick requires executable L1 checks")
	}
	for _, level := range []string{"L2", "L3"} {
		if len(p.Acceptance.Level(level)) == 0 && strings.TrimSpace(w.NotApplicable[level]) == "" {
			reasons = append(reasons, "Quick requires "+level+" checks or an explicit non-applicability reason")
		}
	}
	return reasons
}
