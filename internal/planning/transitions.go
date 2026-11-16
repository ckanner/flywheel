package planning

import (
	"encoding/json"
	"fmt"
	"reflect"
)

func New(brief Brief) (*State, error) {
	if err := ValidateBrief(brief); err != nil {
		return nil, err
	}
	return &State{Version: 1, Phase: "drafting", Brief: brief, Policy: brief.Policy,
		InputRevision: 1, MaxRounds: 3, Requests: []Request{}, Designs: []Design{}, Plans: []ExecutionPlan{},
		Reviews: []Review{}, Approvals: []Approval{}, Feedback: []Feedback{}, CurrentPlans: map[string]Ref{},
		StartAuthorized: brief.ImplementationAuthorized}, nil
}

func RecordDesign(s *State, design Design) error {
	if err := validateDesign(design); err != nil {
		return err
	}
	if old := FindDesign(s, design.Ref); old != nil {
		return nil // exact immutable result already adopted after recovery
	}
	if design.Ref.Revision != len(s.Designs)+1 || design.InputRevision != s.InputRevision || s.PendingActivation != nil {
		return fmt.Errorf("design revision is stale or an activation is pending")
	}
	s.Designs = append(s.Designs, design)
	s.CurrentDesign = &s.Designs[len(s.Designs)-1].Ref
	s.CurrentPlan = nil
	s.CurrentPlans = map[string]Ref{}
	s.Phase, s.Reason = "reviewing", ""
	return nil
}

func RecordPlan(s *State, plan ExecutionPlan) error {
	if err := validatePlan(plan); err != nil {
		return err
	}
	if old := FindPlan(s, plan.Ref); old != nil {
		return nil
	}
	if plan.Ref.Revision != len(s.Plans)+1 || plan.InputRevision != s.InputRevision || s.CurrentDesign == nil || *s.CurrentDesign != plan.Design || s.PendingActivation != nil {
		return fmt.Errorf("execution plan revision or approved design input is stale")
	}
	s.Plans = append(s.Plans, plan)
	s.CurrentPlan = &s.Plans[len(s.Plans)-1].Ref
	s.CurrentPlans[PlanKey(plan.Scope, plan.ScopeID)] = plan.Ref
	s.Phase, s.Reason = "reviewing", ""
	return nil
}

func RecordReview(s *State, review Review) error {
	if err := validateReview(s, review); err != nil {
		return err
	}
	for _, old := range s.Reviews {
		if old.ID == review.ID {
			if reflect.DeepEqual(old, review) {
				return nil
			}
			return fmt.Errorf("review identity was already used for different content")
		}
	}
	current := s.CurrentDesign
	if review.Kind == "plan" {
		p := FindPlan(s, review.Target)
		ref, ok := s.CurrentPlans[PlanKey(p.Scope, p.ScopeID)]
		current = nil
		if ok {
			current = &ref
		}
	}
	if current == nil || *current != review.Target || review.InputRevision != s.InputRevision {
		return fmt.Errorf("review result belongs to stale planning input")
	}
	if err := validateFindingClosure(s, review); err != nil {
		return err
	}
	s.Reviews = append(s.Reviews, review)
	s.Reason = review.Summary
	switch review.Verdict {
	case "pass":
		if review.Kind == "design" {
			s.Phase = "awaiting-design-approval"
		} else {
			s.Phase = "approved"
		}
	case "revise":
		s.Phase = "drafting"
	case "needs-user-decision":
		s.Phase = "awaiting-input"
	case "blocked":
		s.Phase = "blocked"
	}
	return nil
}

func validateApproval(s *State, approval Approval, current bool) error {
	if err := shape("approval", approval); err != nil {
		return err
	}
	design := FindDesign(s, approval.Design)
	if design == nil {
		return fmt.Errorf("approval refers to an unknown design")
	}
	found := false
	for _, r := range s.Reviews {
		if r.ID == approval.ReviewID && r.Kind == "design" && r.Target == approval.Design && r.Verdict == "pass" {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("approval requires a passing independent review of the exact design")
	}
	if approval.Kind == "delegated" && (s.Brief.Authorization == "" || approval.Source != s.Brief.Authorization) {
		return fmt.Errorf("delegated approval must retain the original authorization source")
	}
	if current {
		r := LatestReview(s, "design", approval.Design)
		if s.CurrentDesign == nil || *s.CurrentDesign != approval.Design || r.ID != approval.ReviewID || design.InputRevision != s.InputRevision || s.PendingActivation != nil {
			return fmt.Errorf("approval request is stale or activation is pending")
		}
		if approval.Kind == "delegated" && s.Policy != "delegated" {
			return fmt.Errorf("this task requires a human design approval")
		}
	}
	return nil
}

func Approve(s *State, approval Approval) error {
	for _, old := range s.Approvals {
		if old.RequestID == approval.RequestID {
			if reflect.DeepEqual(old, approval) {
				return nil
			}
			return fmt.Errorf("approval request identity was already used for different content")
		}
	}
	if err := validateApproval(s, approval, true); err != nil {
		return err
	}
	s.Approvals = append(s.Approvals, approval)
	s.StartAuthorized = s.StartAuthorized || approval.Start
	s.Phase, s.Reason = "approved", ""
	return nil
}

func Authorized(s *State, design Ref) bool {
	d := FindDesign(s, design)
	if d == nil || s.CurrentDesign == nil || *s.CurrentDesign != design || d.InputRevision != s.InputRevision {
		return false
	}
	r := LatestReview(s, "design", design)
	if r == nil || r.Verdict != "pass" {
		return false
	}
	for _, approval := range s.Approvals {
		if approval.Design == design && approval.ReviewID == r.ID && (s.Policy == "delegated" || approval.Kind == "human") {
			return true
		}
	}
	return false
}

func ApplyFeedback(s *State, feedback Feedback) error {
	if err := shape("feedback", feedback); err != nil {
		return err
	}
	for _, old := range s.Feedback {
		if old.RequestID == feedback.RequestID {
			if reflect.DeepEqual(old, feedback) {
				return nil
			}
			return fmt.Errorf("feedback request identity was already used for different content")
		}
	}
	if feedback.Target != nil && (s.CurrentDesign == nil || *feedback.Target != *s.CurrentDesign) || s.PendingActivation != nil {
		return fmt.Errorf("feedback target is stale or activation is pending")
	}
	s.Feedback = append(s.Feedback, feedback)
	s.InputRevision++
	if feedback.Policy != "" {
		if feedback.Policy == "delegated" && s.Brief.Authorization == "" {
			// Do not adopt a new authorization from unstructured feedback.
			s.Feedback = s.Feedback[:len(s.Feedback)-1]
			s.InputRevision--
			return fmt.Errorf("delegated policy requires an existing authorization source")
		}
		s.Policy = feedback.Policy
	}
	s.Phase, s.Reason = "drafting", feedback.Text
	return nil
}

func BeginRequest(s *State, request Request) error {
	if request.MaxRounds == 0 {
		request.MaxRounds = s.MaxRounds
	}
	if request.Status == "" {
		request.Status = "pending"
	}
	if err := shape("request", request); err != nil {
		return err
	}
	for _, old := range s.Requests {
		if old.ID == request.ID {
			if old.Scope == request.Scope && old.ScopeID == request.ScopeID && old.InputRevision == request.InputRevision && old.PlanningOnly == request.PlanningOnly && old.Feedback == request.Feedback {
				return nil // resume preserves the recorded limits and used rounds
			}
			return fmt.Errorf("planning request identity was reused for different input")
		}
	}
	if request.InputRevision != s.InputRevision || request.Rounds != 0 || request.Retries != 0 || request.Status != "pending" || request.MaxRounds > s.MaxRounds {
		return fmt.Errorf("new planning request must preserve current input and round limits")
	}
	s.Requests = append(s.Requests, request)
	return nil
}

func BeginRound(s *State, requestID string) error {
	for i := range s.Requests {
		r := &s.Requests[i]
		if r.ID != requestID {
			continue
		}
		if r.InputRevision != s.InputRevision || (r.Status != "pending" && r.Status != "running") {
			return fmt.Errorf("planning request is stale or no longer runnable")
		}
		if r.Stage == "planner" {
			return nil // resume the already-counted round after a crash
		}
		if r.Stage != "" {
			return fmt.Errorf("planning round cannot restart while its review is pending")
		}
		if r.Rounds >= r.MaxRounds {
			r.Status, s.Phase = "budget-stopped", "budget-stopped"
			return fmt.Errorf("planning round limit reached; unresolved work is not approved")
		}
		r.Rounds++
		s.Rounds++
		r.Status = "running"
		r.Stage = "planner"
		return nil
	}
	return fmt.Errorf("unknown planning request")
}

// ValidateTransition prevents a stale worker/state clone from deleting new
// authority or rewriting historical evidence, even with a matching CAS number.
func ValidateTransition(previous, next *State) error {
	if next == nil {
		return fmt.Errorf("planning state cannot be removed")
	}
	if previous == nil {
		return nil
	}
	if !reflect.DeepEqual(previous.Brief, next.Brief) || next.InputRevision < previous.InputRevision || next.Rounds < previous.Rounds {
		return fmt.Errorf("original brief or cumulative planning accounting cannot be rewritten")
	}
	if len(previous.LegacyFiles) > 0 && !reflect.DeepEqual(previous.LegacyFiles, next.LegacyFiles) {
		return fmt.Errorf("migrated contract ownership cannot be rewritten")
	}
	for _, pair := range [][2]any{{previous.Designs, next.Designs}, {previous.Plans, next.Plans}, {previous.Reviews, next.Reviews}, {previous.Approvals, next.Approvals}, {previous.Feedback, next.Feedback}} {
		old, current := reflect.ValueOf(pair[0]), reflect.ValueOf(pair[1])
		if current.Len() < old.Len() || !reflect.DeepEqual(old.Interface(), current.Slice(0, old.Len()).Interface()) {
			return fmt.Errorf("planning history is append-only")
		}
	}
	for i, old := range previous.Requests {
		if i >= len(next.Requests) {
			return fmt.Errorf("planning request ledger cannot shrink")
		}
		r := next.Requests[i]
		if old.ID != r.ID || old.Scope != r.Scope || old.ScopeID != r.ScopeID || old.InputRevision != r.InputRevision || old.MaxRounds != r.MaxRounds || old.PlanningOnly != r.PlanningOnly || old.Feedback != r.Feedback || r.Rounds < old.Rounds || r.Retries < old.Retries {
			return fmt.Errorf("planning request identity and spent rounds are immutable")
		}
	}
	return nil
}

// Clone isolates callers that stage a transaction before the outer state CAS.
func Clone(s *State) *State {
	raw, _ := json.Marshal(s)
	var result *State
	_ = json.Unmarshal(raw, &result)
	return result
}
