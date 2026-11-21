package planning

import "fmt"

func validateActivation(s *State, activation Activation) error {
	// The containing state schema has already validated both activation objects.
	if FindDesign(s, activation.Design) == nil {
		return fmt.Errorf("activation refers to an unknown design")
	}
	seen := map[Ref]bool{}
	for _, ref := range activation.Plans {
		plan := FindPlan(s, ref)
		if plan == nil || plan.Design != activation.Design || seen[ref] {
			return fmt.Errorf("activation plan set must reference distinct plans of the same design")
		}
		seen[ref] = true
	}
	if activation.Files["spec.md"] == "" || (activation.Stage == "committed" || activation.Stage == "synced" || activation.Stage == "active") && activation.Head == "" {
		return fmt.Errorf("activation requires its complete Spec projection and committed HEAD")
	}
	return nil
}

func ActivationRefs(activation Activation) ActivationRef {
	return ActivationRef{Version: 1, Design: activation.Design, Plans: append([]Ref{}, activation.Plans...)}
}

// StartAllowed checks the implementation grant for the exact current design.
// Review/approval validity remains an independent Authorized check. Starting an
// earlier design never authorizes implementation of a later approved-only one.
func StartAllowed(s *State, design Ref) bool {
	if !s.StartAuthorized || s.CurrentDesign == nil || *s.CurrentDesign != design {
		return false
	}
	if s.Brief.ImplementationAuthorized {
		return true
	}
	for _, approval := range s.Approvals {
		if approval.Design == design && approval.Kind == "human" && approval.Start {
			return true
		}
	}
	return false
}

// Admit checks persisted authorization and active plan identities. Filesystem
// bytes, dependency snapshots and native lifecycle are additionally checked by
// the engine against this exact transaction before every implementation call.
func Admit(s *State, ref ActivationRef) error {
	return admitActivePlan(s, ref, true)
}

// AdmitPlanRevision permits a bounded change to an already reviewed active
// plan under its approved design. It does not grant implementation Start.
func AdmitPlanRevision(s *State, ref ActivationRef) error {
	return admitActivePlan(s, ref, false)
}

func admitActivePlan(s *State, ref ActivationRef, requireStart bool) error {
	if err := Validate(s); err != nil {
		return err
	}
	if err := shape("activation-ref", ref); err != nil {
		return err
	}
	if s.PendingActivation != nil || s.Phase != "ready" || s.Active == nil || s.CurrentDesign == nil || *s.CurrentDesign != ref.Design || s.Active.Design != ref.Design || !Authorized(s, ref.Design) {
		return fmt.Errorf("design approval or completed activation is required before implementation")
	}
	if requireStart && !StartAllowed(s, ref.Design) {
		return fmt.Errorf("design approval does not by itself authorize starting implementation")
	}
	if len(ref.Plans) != len(s.Active.Plans) {
		return fmt.Errorf("projected plan set differs from the active transaction")
	}
	active := map[Ref]bool{}
	for _, p := range s.Active.Plans {
		active[p] = true
	}
	for _, p := range ref.Plans {
		plan := FindPlan(s, p)
		if !active[p] || plan == nil || plan.Design != ref.Design || s.CurrentPlans[PlanKey(plan.Scope, plan.ScopeID)] != p {
			return fmt.Errorf("implementation plan is stale or absent from the active transaction")
		}
		delete(active, p)
		review := LatestReview(s, "plan", p)
		if review == nil || review.Verdict != "pass" || review.ScopeVerdict != "within-envelope" {
			return fmt.Errorf("current execution plan requires an independent in-scope review")
		}
	}
	return nil
}
