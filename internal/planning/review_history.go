package planning

import "fmt"

// ReviewHistory returns public review evidence for one design lineage or child
// scope, up to the exact known target revision. Child histories never cross the
// root design, milestone or packet boundary. Callers must not mutate evidence.
func ReviewHistory(s *State, kind string, target Ref) []Review {
	if kind == "compact-v1" {
		kind = "design"
	}
	result := []Review{}
	var targetPlan *ExecutionPlan
	switch kind {
	case "design":
		if FindDesign(s, target) == nil {
			return result
		}
	case "plan":
		targetPlan = FindPlan(s, target)
		if targetPlan == nil {
			return result
		}
	default:
		return result
	}
	plans := map[Ref]*ExecutionPlan{}
	if targetPlan != nil {
		for i := range s.Plans {
			plans[s.Plans[i].Ref] = &s.Plans[i]
		}
	}
	for _, review := range s.Reviews {
		ref := review.Target
		if kind == "plan" && review.Kind == "compact-v1" && review.Plan != nil {
			ref = *review.Plan
		}
		if !reviewTarget(review, kind, ref) || ref.Revision > target.Revision {
			continue
		}
		if targetPlan != nil {
			p := plans[ref]
			if p == nil || p.Design != targetPlan.Design || PlanKey(p.Scope, p.ScopeID) != PlanKey(targetPlan.Scope, targetPlan.ScopeID) {
				continue
			}
		}
		result = append(result, review)
	}
	return result
}

// Omission or a severity downgrade is not closure of a previous blocker. A
// passing review must give the same stable ID and concrete resolution evidence;
// intermediate revise reviews may close findings before the final full review.
func validateFindingClosure(s *State, review Review) error {
	if review.Verdict != "pass" {
		return nil
	}
	open := map[string]bool{}
	for _, previous := range ReviewHistory(s, review.Kind, review.Target) {
		for _, finding := range previous.Findings {
			if finding.Status == "resolved" {
				delete(open, finding.ID)
			} else if finding.Severity == "major" || finding.Severity == "blocker" {
				open[finding.ID] = true
			}
		}
	}
	for _, finding := range review.Findings {
		if finding.Status == "resolved" && finding.Resolution != "" {
			delete(open, finding.ID)
		}
	}
	if len(open) > 0 {
		return fmt.Errorf("passing review must explicitly resolve every previous blocking finding in this scope")
	}
	return nil
}
