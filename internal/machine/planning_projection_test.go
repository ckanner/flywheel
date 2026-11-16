package machine

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func planningProjectionFixture(t *testing.T, document string, findings []planning.Finding) *state.State {
	t.Helper()
	st, err := state.InitPlanning(planning.Brief{Version: 1, ID: "bounded-console", Goal: "Inspect the complete reviewed design before approval.",
		OriginalRequest: "Design a verified implementation", Constraints: []string{}, Evidence: []string{}, Policy: "review-first"}, nil)
	require.NoError(t, err)
	st.Revision = 1
	output := engineDesignOutput(t)
	d, err := planning.SealDesign(planning.Design{Ref: planning.Ref{Revision: 1}, InputRevision: 1, Document: document,
		Requirements: output.Requirements, Decisions: output.Decisions, Milestones: output.Milestones, ImplementationScope: output.ImplementationScope,
		Baseline: "baseline", PlannerInvocation: "design-call-1"})
	require.NoError(t, err)
	require.NoError(t, planning.RecordDesign(st.Planning, d))
	if findings == nil {
		findings = []planning.Finding{}
	}
	require.NoError(t, planning.RecordReview(st.Planning, planning.Review{ID: "root-review-1", Kind: "design", Target: d.Ref, InputRevision: 1,
		Invocation: "independent-call-1", Role: "design-review", Binding: strings.Repeat("b", 64), Verdict: "pass", Summary: "The current design preserves its verified acceptance floor.", Findings: findings}))
	require.NoError(t, planning.Approve(st.Planning, planning.Approval{Kind: "human", Design: d.Ref, ReviewID: "root-review-1",
		RequestID: "start-first-design", Source: "tty:human", At: "now", Start: true}))
	return st
}

func projectionChild(t *testing.T, p *planning.State, id, dependency string, files map[string]string) planning.ExecutionPlan {
	t.Helper()
	plan, err := planning.SealPlan(planning.ExecutionPlan{Ref: planning.Ref{Revision: len(p.Plans) + 1}, Design: *p.CurrentDesign, InputRevision: p.InputRevision,
		Scope: "packet", ScopeID: id, Files: files, Dependencies: dependency, ScopeVerdict: "within-envelope", PlannerInvocation: fmt.Sprintf("plan-call-%d", len(p.Plans)+1)})
	require.NoError(t, err)
	require.NoError(t, planning.RecordPlan(p, plan))
	require.NoError(t, planning.RecordReview(p, planning.Review{ID: fmt.Sprintf("plan-review-%d", plan.Ref.Revision), Kind: "plan", Target: plan.Ref, Design: p.CurrentDesign,
		InputRevision: p.InputRevision, Invocation: fmt.Sprintf("plan-review-call-%d", plan.Ref.Revision), Role: "plan-review", Binding: strings.Repeat("b", 64), Verdict: "pass",
		ScopeVerdict: "within-envelope", Summary: "The child scope is independently reviewed.", Findings: []planning.Finding{}}))
	return plan
}

func TestPlanningConsoleProjectionBoundsRealHistoryWithoutChangingAuthority(t *testing.T) {
	st := planningProjectionFixture(t, strings.Repeat("Reviewed document.\n", 256), nil)
	p := st.Planning
	files := enginePlanFiles()
	files["spec.md"] += "\n" + strings.Repeat("<html-sensitive-evidence>&\n", 512)
	first := projectionChild(t, p, "M01-T01", "dependency", files)
	p.Active = &planning.Activation{Design: *p.CurrentDesign, Plans: []planning.Ref{first.Ref}, Files: map[string]string{"spec.md": planning.DigestBytes([]byte("projection"))},
		OldFiles: map[string]string{"spec.md": planning.DigestBytes([]byte("original"))}, Stage: "active", Base: "base", Head: "head"}
	p.LegacyFiles = map[string]string{"spec.md": planning.DigestBytes([]byte("legacy"))}
	for i := 2; i <= 256; i++ {
		d := p.Designs[0]
		d.Ref.Revision, d.PlannerInvocation = i, fmt.Sprintf("design-call-%d", i)
		d, err := planning.SealDesign(d)
		require.NoError(t, err)
		require.NoError(t, planning.RecordDesign(p, d))
		require.NoError(t, planning.RecordReview(p, planning.Review{ID: fmt.Sprintf("root-review-%d", i), Kind: "design", Target: d.Ref, InputRevision: 1,
			Invocation: fmt.Sprintf("independent-call-%d", i), Role: "design-review", Binding: strings.Repeat("b", 64), Verdict: "pass", Summary: "Review this exact revision.", Findings: []planning.Finding{}}))
		projectionChild(t, p, "M01-T01", "dependency", files)
	}
	projectionChild(t, p, "M01-T02", "dependency", files)
	for i := 0; i < 15; i++ {
		require.NoError(t, planning.BeginRequest(p, planning.Request{ID: fmt.Sprintf("scope-request-%d", i), Scope: "packet", ScopeID: "M01-T02", InputRevision: p.InputRevision, Feedback: "Keep the approved boundary."}))
	}
	require.NoError(t, planning.Validate(p))
	before, err := json.Marshal(st)
	require.NoError(t, err)
	require.Greater(t, len(before), 1<<20)
	view, err := ConsoleProjection(st)
	require.NoError(t, err)
	raw, err := json.Marshal(view)
	require.NoError(t, err)
	require.Less(t, len(raw), 1<<20)
	require.LessOrEqual(t, planningProjectionSize(view.Planning), 512<<10)
	require.Equal(t, st.Revision, view.Revision)
	require.Len(t, view.Planning.Designs, 3, "current, previous, and most recently approved root remain inspectable")
	require.Equal(t, []int{1, 255, 256}, []int{view.Planning.Designs[0].Ref.Revision, view.Planning.Designs[1].Ref.Revision, view.Planning.Designs[2].Ref.Revision})
	require.Equal(t, p.CurrentPlans, view.Planning.CurrentPlans)
	require.Equal(t, p.Active.Plans, view.Planning.Active.Plans)
	require.Empty(t, view.Planning.Active.Files)
	require.Empty(t, view.Planning.Active.OldFiles)
	require.Empty(t, view.Planning.LegacyFiles)
	require.Equal(t, "console-excerpt", view.Planning.Projection.Kind)
	require.True(t, view.Planning.Projection.ApprovalComplete)
	require.Equal(t, 253, view.Planning.Projection.Omitted["design-history"])
	require.Positive(t, view.Planning.Projection.Omitted["plan-history"])
	require.Equal(t, 5, view.Planning.Projection.Omitted["requests"])
	for _, ref := range p.CurrentPlans {
		expected := planning.LatestReview(p, "plan", ref)
		actual := planning.LatestReview(view.Planning, "plan", ref)
		require.Equal(t, expected, actual)
	}
	require.Equal(t, planning.StartAllowed(p, *p.CurrentDesign), planning.StartAllowed(view.Planning, *p.CurrentDesign), "an older approved root cannot authorize the current one")
	_, err = state.Save(t.TempDir(), view, nil)
	require.ErrorContains(t, err, "projection", "the authoritative schema must reject display-only projection metadata")
	// The returned view is detached even for retained maps, slices and refs.
	view.Planning.CurrentPlans["foreign"] = first.Ref
	view.Planning.Designs[0].Requirements[0].Title = "changed only in display copy"
	view.Planning.Active.Plans[0].Digest = "display-only mutation"
	after, err := json.Marshal(st)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestPlanningConsoleProjectionRetainsExactCurrentReviewApprovalAndSeparateStart(t *testing.T) {
	st := planningProjectionFixture(t, "# Reviewable current design", nil)
	p := st.Planning
	r := p.Reviews[0]
	r.ID, r.Invocation = "second-review", "second-independent-call"
	require.NoError(t, planning.RecordReview(p, r))
	for i := 0; i < 12; i++ {
		require.NoError(t, planning.Approve(p, planning.Approval{Kind: "human", Design: *p.CurrentDesign, ReviewID: r.ID,
			RequestID: fmt.Sprintf("approve-only-%d", i), Source: "tty:human", At: "now"}))
	}
	view, err := ConsoleProjection(st)
	require.NoError(t, err)
	require.Len(t, view.Planning.Approvals, 2)
	require.Len(t, view.Planning.Reviews, 2, "the Start event's review remains attributable but does not replace the latest independent review")
	require.Equal(t, r.ID, planning.LatestReview(view.Planning, "design", *p.CurrentDesign).ID)
	require.True(t, planning.Authorized(view.Planning, *p.CurrentDesign))
	require.True(t, planning.StartAllowed(view.Planning, *p.CurrentDesign))
	for i := 0; i < 15; i++ {
		require.NoError(t, planning.ApplyFeedback(p, planning.Feedback{RequestID: fmt.Sprintf("feedback-%d", i), Text: "Revise the original requirements.", Source: "user", At: "now"}))
	}
	view, err = ConsoleProjection(st)
	require.NoError(t, err)
	require.Equal(t, p.InputRevision, view.Planning.InputRevision)
	require.Len(t, view.Planning.Feedback, 10)
	require.Equal(t, 5, view.Planning.Projection.Omitted["feedback"])
	require.False(t, planning.Authorized(view.Planning, *p.CurrentDesign), "input revision invalidation survives the shorter feedback history")
}

func TestPlanningConsoleProjectionKeepsThinBriefExplicit(t *testing.T) {
	st := planningProjectionFixture(t, "draft", nil)
	p, err := planning.New(st.Planning.Brief)
	require.NoError(t, err)
	st.Planning = p
	view, err := ConsoleProjection(st)
	require.NoError(t, err)
	require.Empty(t, view.Planning.Designs)
	require.Empty(t, view.Planning.Approvals)
	require.False(t, view.Planning.Projection.ApprovalComplete)
	require.Contains(t, string(mustJSON(view.Planning.Projection)), `"approval-complete":false`)
	require.Equal(t, "state.json", view.Planning.Projection.Ledger)
}

func TestPlanningConsoleProjectionDropsSecondaryPayloadsBeforeCurrentApprovalMaterial(t *testing.T) {
	st := planningProjectionFixture(t, "# Complete current design", nil)
	p := st.Planning
	files := enginePlanFiles()
	files["spec.md"] += strings.Repeat("<", 100000)
	plan := projectionChild(t, p, "M01-T01", "dependency", files)
	p.PendingActivation = &planning.Activation{Design: *p.CurrentDesign, Plans: []planning.Ref{plan.Ref}, Files: map[string]string{"spec.md": planning.DigestBytes(nil)}, OldFiles: map[string]string{}, Stage: "intent", Base: "base"}
	view, err := ConsoleProjection(st)
	require.NoError(t, err)
	require.Empty(t, view.Planning.Plans[0].Files)
	require.Equal(t, p.PendingActivation.Plans, view.Planning.PendingActivation.Plans)
	require.True(t, view.Planning.Projection.ApprovalComplete)
	require.Equal(t, p.Designs[0], view.Planning.Designs[0])
	// Previous-version and child-review text are also secondary to the full
	// currently displayed design and its independent review.
	p.PendingActivation = nil
	d := p.Designs[0]
	d.Ref.Revision, d.PlannerInvocation = 2, "current-design-call"
	d, err = planning.SealDesign(d)
	require.NoError(t, err)
	require.NoError(t, planning.RecordDesign(p, d))
	require.NoError(t, planning.RecordReview(p, planning.Review{ID: "current-root-review", Kind: "design", Target: d.Ref, InputRevision: 1, Invocation: "current-review-call",
		Role: "design-review", Binding: "independent", Verdict: "pass", Summary: "Current review is complete.", Findings: []planning.Finding{}}))
	p.Designs[0].Document = strings.Repeat("<", 100000)
	p.Reviews[0].Summary = strings.Repeat("<", 32768)
	planningView, err := consolePlanningProjection(p, 10000)
	require.NoError(t, err)
	require.Empty(t, planningView.Designs[0].Document)
	require.Equal(t, omittedPlanningReview, planningView.Reviews[0].Summary)
	require.Equal(t, d.Document, planningView.Designs[1].Document)
	require.True(t, planningView.Projection.ApprovalComplete)
}

func TestPlanningConsoleProjectionUsesEncodedByteBudgetAndDisablesIncompleteApproval(t *testing.T) {
	// '<' is one input byte but six bytes in Go's JSON encoding.
	document := strings.Repeat("<中文>", 40000)
	require.Less(t, len(document), 512<<10)
	st := planningProjectionFixture(t, document, nil)
	view, err := ConsoleProjection(st)
	require.NoError(t, err)
	require.LessOrEqual(t, planningProjectionSize(view.Planning), 512<<10)
	require.Empty(t, view.Planning.Designs[0].Document)
	require.Empty(t, view.Planning.Designs[0].Requirements)
	require.False(t, view.Planning.Projection.ApprovalComplete)
	require.Positive(t, view.Planning.Projection.Omitted["design-details"])
	require.Equal(t, document, st.Planning.Designs[0].Document)
	findings := []planning.Finding{}
	for i := 0; i < 16; i++ {
		findings = append(findings, planning.Finding{ID: fmt.Sprintf("minor-%d", i), Severity: "minor", Target: "documentation", Evidence: strings.Repeat("<", 8192), Impact: "Documented limitation", Status: "accepted"})
	}
	st = planningProjectionFixture(t, "# Current design must remain paired with its complete review", findings)
	view, err = ConsoleProjection(st)
	require.NoError(t, err)
	require.False(t, view.Planning.Projection.ApprovalComplete, "omitted findings must never leave an enabled approval button")
	require.Equal(t, omittedPlanningReview, view.Planning.Reviews[0].Summary)
	require.Empty(t, view.Planning.Reviews[0].Findings)
	require.Equal(t, st.Planning.Reviews[0].ID, view.Planning.Reviews[0].ID)
	require.Equal(t, st.Planning.Reviews[0].Verdict, view.Planning.Reviews[0].Verdict)
	require.Len(t, st.Planning.Reviews[0].Findings, 16)
}

func TestPlanningConsoleProjectionOmitsLargeBriefAndScopeFeedbackExplicitly(t *testing.T) {
	st := planningProjectionFixture(t, "# Complete design", nil)
	p := st.Planning
	p.Brief.Constraints = []string{strings.Repeat("<", 8192), strings.Repeat("<", 8192)}
	for i := 0; i < 12; i++ {
		require.NoError(t, planning.BeginRequest(p, planning.Request{ID: fmt.Sprintf("request-%d", i), Scope: "design", InputRevision: 1, Feedback: strings.Repeat("<", 8192)}))
	}
	view, err := consolePlanningProjection(p, 15000)
	require.NoError(t, err)
	require.True(t, view.Projection.ApprovalComplete)
	require.Empty(t, view.Brief.Constraints)
	require.Positive(t, view.Projection.Omitted["brief-details"])
	require.Equal(t, 10, view.Projection.Omitted["request-feedback"])
	require.Equal(t, p.Requests[len(p.Requests)-1].ID, view.Requests[len(view.Requests)-1].ID)
	require.Equal(t, p.Requests[len(p.Requests)-1].Status, view.Requests[len(view.Requests)-1].Status)
	require.NotEmpty(t, p.Requests[0].Feedback)
}

func TestPlanningConsoleProjectionReportsIrreducibleIdentityOverflow(t *testing.T) {
	st := planningProjectionFixture(t, "# Complete design", nil)
	for i := 1; i <= 16; i++ {
		projectionChild(t, st.Planning, fmt.Sprintf("M01-T%02d", i), strings.Repeat("<", 8192), enginePlanFiles())
	}
	before, err := json.Marshal(st)
	require.NoError(t, err)
	view, err := ConsoleProjection(st)
	require.Nil(t, view)
	require.ErrorContains(t, err, "identity metadata exceeds")
	require.ErrorContains(t, err, "without --compact")
	after, err := json.Marshal(st)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
