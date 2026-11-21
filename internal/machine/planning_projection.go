package machine

import (
	"encoding/json"
	"fmt"

	"github.com/ckanner/flywheel/internal/planning"
)

const omittedPlanningReview = "Review details omitted from this console excerpt; read the complete review in the task ledger (state.json)."

func planningProjectionSize(value any) int {
	raw, _ := json.Marshal(value) // concrete planning fields contain only JSON values
	return len(raw)
}

func consolePlanningProjection(source *planning.State, limit int) (*planning.State, error) {
	p := *source
	p.Projection = &planning.Projection{Kind: "console-excerpt", Ledger: "state.json", Omitted: map[string]int{}}
	omitted := p.Projection.Omitted
	designRefs, planRefs := map[planning.Ref]bool{}, map[planning.Ref]bool{}
	var currentReview *planning.Review
	if p.CurrentDesign != nil {
		designRefs[*p.CurrentDesign] = true
		currentReview = planning.LatestReview(source, "design", *p.CurrentDesign)
		for i := len(source.Designs) - 1; i >= 0; i-- {
			if source.Designs[i].Ref.Revision < p.CurrentDesign.Revision {
				designRefs[source.Designs[i].Ref] = true
				break
			}
		}
	}
	p.Projection.ApprovalComplete = p.CurrentDesign != nil && currentReview != nil
	// Keep the latest visible approval plus the exact current approval and the
	// independently recorded current-design Start grant, even if they differ.
	approvalIndices := map[int]bool{}
	foundApproval, foundStart := false, false
	for i := len(source.Approvals) - 1; i >= 0; i-- {
		a := source.Approvals[i]
		if i == len(source.Approvals)-1 {
			approvalIndices[i], designRefs[a.Design] = true, true
		}
		if p.CurrentDesign != nil && a.Design == *p.CurrentDesign {
			if !foundApproval && currentReview != nil && a.ReviewID == currentReview.ID && (p.Policy == "delegated" || a.Kind == "human") {
				approvalIndices[i], foundApproval = true, true
			}
			if !foundStart && a.Kind == "human" && a.Start {
				approvalIndices[i], foundStart = true, true
			}
		}
	}
	p.Approvals = []planning.Approval{}
	approvalReviews := map[string]bool{}
	for i, a := range source.Approvals {
		if approvalIndices[i] {
			p.Approvals = append(p.Approvals, a)
			approvalReviews[a.ReviewID] = true
		}
	}
	omitted["approval-history"] = len(source.Approvals) - len(p.Approvals)
	p.Designs = []planning.Design{}
	for _, d := range source.Designs {
		if designRefs[d.Ref] {
			p.Designs = append(p.Designs, d)
		}
	}
	omitted["design-history"] = len(source.Designs) - len(p.Designs)
	for _, ref := range p.CurrentPlans {
		planRefs[ref] = true
	}
	if p.CurrentPlan != nil {
		planRefs[*p.CurrentPlan] = true
	}
	for _, a := range []*planning.Activation{source.Active, source.PendingActivation} {
		if a != nil {
			omitted["activation-files"] += len(a.Files) + len(a.OldFiles)
			for _, ref := range a.Plans {
				planRefs[ref] = true
			}
		}
	}
	p.Active, p.PendingActivation = planningActivationProjection(source.Active), planningActivationProjection(source.PendingActivation)
	omitted["legacy-files"] = len(p.LegacyFiles)
	p.LegacyFiles = nil
	p.Plans = []planning.ExecutionPlan{}
	for _, plan := range source.Plans {
		if planRefs[plan.Ref] {
			if p.CurrentPlan == nil || plan.Ref != *p.CurrentPlan {
				omitted["plan-files"] += len(plan.Files)
				plan.Files = map[string]string{}
			}
			p.Plans = append(p.Plans, plan)
		}
	}
	omitted["plan-history"] = len(source.Plans) - len(p.Plans)
	latest := map[string]int{}
	for i, r := range source.Reviews {
		if (r.Kind == "design" || r.Kind == "compact-v1") && designRefs[r.Target] || r.Kind == "plan" && planRefs[r.Target] {
			latest[fmt.Sprintf("%s/%d/%s", r.Kind, r.Target.Revision, r.Target.Digest)] = i
		}
	}
	reviewIndices := map[int]bool{}
	for _, i := range latest {
		reviewIndices[i] = true
	}
	p.Reviews = []planning.Review{}
	for i, r := range source.Reviews {
		if reviewIndices[i] || approvalReviews[r.ID] {
			p.Reviews = append(p.Reviews, r)
		}
	}
	omitted["review-history"] = len(source.Reviews) - len(p.Reviews)
	omitted["requests"] = max(0, len(source.Requests)-10)
	omitted["feedback"] = max(0, len(source.Feedback)-10)
	p.Requests = append([]planning.Request{}, source.Requests[omitted["requests"]:]...)
	p.Feedback = append([]planning.Feedback{}, source.Feedback[omitted["feedback"]:]...)
	size := planningProjectionSize(&p)
	if size <= limit {
		return jsonClone(&p), nil
	}
	// Subsequent omission counters add fewer than 1 KiB. Track component byte
	// deltas instead of repeatedly encoding all large current-scope reviews.
	target := limit - 1024
	// Measure encoded bytes, including HTML escaping and multibyte text. Remove
	// whole secondary payloads before omitting any current approval material.
	for i := range p.Plans {
		if size > target {
			if currentReview != nil && currentReview.Kind == "compact-v1" && currentReview.Plan != nil && *currentReview.Plan == p.Plans[i].Ref {
				p.Projection.ApprovalComplete = false
			}
			before := planningProjectionSize(p.Plans[i])
			omitted["plan-files"] += len(p.Plans[i].Files)
			p.Plans[i].Files = map[string]string{}
			size += planningProjectionSize(p.Plans[i]) - before
		}
	}
	for i := range p.Designs {
		if (p.CurrentDesign == nil || p.Designs[i].Ref != *p.CurrentDesign) && size > target {
			before := planningProjectionSize(p.Designs[i])
			omitPlanningDesign(&p.Designs[i])
			omitted["design-details"]++
			size += planningProjectionSize(p.Designs[i]) - before
		}
	}
	for i := range p.Reviews {
		if (currentReview == nil || p.Reviews[i].ID != currentReview.ID) && size > target {
			before := planningProjectionSize(p.Reviews[i])
			omitPlanningReview(&p.Reviews[i])
			omitted["review-details"]++
			size += planningProjectionSize(p.Reviews[i]) - before
		}
	}
	if size > target {
		before := planningProjectionSize([]any{p.Brief, p.Feedback, p.Requests})
		p.Brief.OriginalRequest, p.Brief.Authorization = "", ""
		p.Brief.Constraints, p.Brief.Evidence = []string{}, []string{}
		omitted["brief-details"]++
		omitted["feedback"] += len(p.Feedback)
		p.Feedback = []planning.Feedback{}
		for i := range p.Requests {
			if p.Requests[i].Feedback != "" {
				p.Requests[i].Feedback = ""
				omitted["request-feedback"]++
			}
		}
		size += planningProjectionSize([]any{p.Brief, p.Feedback, p.Requests}) - before
	}
	if size > target {
		for i := range p.Designs {
			if p.CurrentDesign != nil && p.Designs[i].Ref == *p.CurrentDesign {
				before := planningProjectionSize(p.Designs[i])
				omitPlanningDesign(&p.Designs[i])
				omitted["design-details"]++
				p.Projection.ApprovalComplete = false
				size += planningProjectionSize(p.Designs[i]) - before
			}
		}
	}
	if size > target && currentReview != nil {
		for i := range p.Reviews {
			if p.Reviews[i].ID == currentReview.ID {
				omitPlanningReview(&p.Reviews[i])
				omitted["review-details"]++
				p.Projection.ApprovalComplete = false
			}
		}
	}
	if planningProjectionSize(&p) > limit {
		return nil, fmt.Errorf("planning identity metadata exceeds the console response budget; read the complete task ledger state.json or control planning status without --compact")
	}
	return jsonClone(&p), nil // no returned slice/map aliases authoritative state
}

func planningActivationProjection(source *planning.Activation) *planning.Activation {
	if source == nil {
		return nil
	}
	view := *source
	view.Files, view.OldFiles = map[string]string{}, map[string]string{}
	return &view
}

func omitPlanningDesign(d *planning.Design) {
	d.Document, d.Baseline = "", ""
	d.Requirements, d.Decisions, d.Milestones = []planning.Requirement{}, []planning.Decision{}, []planning.Milestone{}
	d.ImplementationScope = []string{}
}

func omitPlanningReview(r *planning.Review) {
	r.Summary, r.Findings = omittedPlanningReview, []planning.Finding{}
}
