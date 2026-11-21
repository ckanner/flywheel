package planning

import (
	"fmt"
	"maps"
)

// RecordPlanForReview seals a new exact plan and enters an independent review
// round as one transaction. It does not activate files or grant Start. A failed
// request or round admission must not leave a partially adopted plan behind.
func RecordPlanForReview(s *State, plan ExecutionPlan, request Request) error {
	if !request.PlanningOnly || request.Scope != plan.Scope || request.ScopeID != plan.ScopeID || request.InputRevision != plan.InputRevision {
		return fmt.Errorf("plan review requires a planning-only request for the exact plan scope and input")
	}
	if plan.Ref.Revision != len(s.Plans)+1 {
		return fmt.Errorf("plan review requires a new immutable plan revision")
	}
	plan.Files = maps.Clone(plan.Files)
	plan, err := SealPlan(plan)
	if err != nil {
		return err
	}
	next := Clone(s)
	if err := RecordPlan(next, plan); err != nil {
		return err
	}
	if err := BeginRequest(next, request); err != nil {
		return err
	}
	if err := BeginRound(next, request.ID); err != nil {
		return err
	}
	for i := range next.Requests {
		if next.Requests[i].ID == request.ID {
			next.Requests[i].Stage = "reviewer"
			break
		}
	}
	*s = *next
	return nil
}
