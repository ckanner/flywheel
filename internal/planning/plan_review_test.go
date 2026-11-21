package planning

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func manualPlanReviewFixture(t *testing.T) (*State, ExecutionPlan, Request) {
	t.Helper()
	s, ref := readyState(t)
	s.StartAuthorized, s.Approvals[0].Start = false, false
	plan := testPlan(t, s, ref.Design, "M01-T01")
	plan.Scope = "packet"
	plan.PlannerInvocation = "control:collaboration:reviewed-user-edit"
	plan.Files["task-packet-M01-T01.md"] = "manually revised split within the approved packet"
	request := Request{ID: "manual-split", Origin: "collaboration-edit", Scope: "packet", ScopeID: "M01-T01", InputRevision: s.InputRevision,
		PlanningOnly: true, Feedback: "Manual collaboration revision exact-input-digest"}
	return s, plan, request
}

func TestRecordPlanForReviewPublishesOneIndependentReviewWithoutGrantingStart(t *testing.T) {
	s, plan, request := manualPlanReviewFixture(t)
	before := Clone(s)
	require.NoError(t, RecordPlanForReview(s, plan, request))
	require.NoError(t, Validate(s))
	require.NoError(t, ValidateTransition(before, s))
	require.Equal(t, before.Plans, s.Plans[:len(before.Plans)])
	require.Equal(t, before.Active, s.Active)
	require.Equal(t, before.Reviews, s.Reviews)
	require.Equal(t, before.Approvals, s.Approvals)
	require.Equal(t, before.Brief, s.Brief)
	require.False(t, s.StartAuthorized)
	require.False(t, StartAllowed(s, *s.CurrentDesign))
	require.Equal(t, "reviewing", s.Phase)
	require.Equal(t, 1, s.Rounds)
	require.Len(t, s.Requests, 1)
	require.Equal(t, "collaboration-edit", s.Requests[0].Origin)
	require.Equal(t, "reviewer", s.Requests[0].Stage)
	require.Equal(t, "running", s.Requests[0].Status)
	require.Equal(t, 1, s.Requests[0].Rounds)
	require.True(t, s.Requests[0].PlanningOnly)
	current := *s.CurrentPlan
	require.Nil(t, LatestReview(s, "plan", current), "manual edit cannot manufacture a passing review")
	plan.Files["task-packet-M01-T01.md"] = "later caller mutation"
	require.Equal(t, "manually revised split within the approved packet", FindPlan(s, current).Files["task-packet-M01-T01.md"])
	require.NoError(t, Validate(s), "caller-owned maps cannot rewrite the sealed plan")
}

func TestRecordPlanForReviewResumesTheMatchingRequestWithoutResettingItsRound(t *testing.T) {
	s, plan, request := manualPlanReviewFixture(t)
	require.NoError(t, BeginRequest(s, request))
	require.NoError(t, BeginRound(s, request.ID))
	require.NoError(t, BeginRequest(s, Request{ID: "other-scope", Scope: "milestone", ScopeID: "M-01", InputRevision: s.InputRevision, PlanningOnly: true}))
	before := Clone(s)
	require.NoError(t, RecordPlanForReview(s, plan, request))
	require.Equal(t, 1, s.Rounds)
	require.Equal(t, 1, s.Requests[0].Rounds)
	require.Equal(t, "reviewer", s.Requests[0].Stage)
	require.Equal(t, before.Requests[1], s.Requests[1], "the final ledger row is not necessarily the edited request")
	require.NoError(t, Validate(s))
	require.NoError(t, ValidateTransition(before, s))
}

func TestRecordPlanForReviewRollsBackEveryFailedAdmission(t *testing.T) {
	for _, scenario := range []string{"implementation-request", "different-scope", "different-scope-id", "different-input", "historical-plan", "invalid-plan", "stale-plan-input", "different-design", "pending-activation", "invalid-request", "invalid-origin", "forged-resume-stage", "request-budget-increase", "request-origin-reuse", "completed-request", "pending-review", "exhausted-rounds"} {
		t.Run(scenario, func(t *testing.T) {
			s, plan, request := manualPlanReviewFixture(t)
			switch scenario {
			case "implementation-request":
				request.PlanningOnly = false
			case "different-scope":
				request.Scope = "milestone"
			case "different-scope-id":
				request.ScopeID = "M01-T02"
			case "different-input":
				request.InputRevision++
			case "historical-plan":
				plan = s.Plans[0]
				request.Scope, request.ScopeID = plan.Scope, plan.ScopeID
			case "invalid-plan":
				plan.Files["spec.md"] = strings.Repeat("x", 131073)
			case "stale-plan-input":
				plan.InputRevision++
				request.InputRevision++
			case "different-design":
				plan.Design.Digest = strings.Repeat("a", 64)
			case "pending-activation":
				s.PendingActivation = &Activation{Design: s.Active.Design, Plans: s.Active.Plans, Files: s.Active.Files, OldFiles: map[string]string{}, Stage: "intent", Base: "base"}
			case "invalid-request":
				request.ID = "invalid/request"
			case "invalid-origin":
				request.Origin = "unverified-source"
			case "forged-resume-stage":
				request.Stage = "planner"
			case "request-budget-increase":
				request.MaxRounds = s.MaxRounds + 1
			case "request-origin-reuse":
				old := request
				old.Origin = ""
				require.NoError(t, BeginRequest(s, old))
			case "completed-request":
				require.NoError(t, BeginRequest(s, request))
				s.Requests[0].Status, s.Requests[0].Stage = "completed", "finished"
			case "pending-review":
				require.NoError(t, BeginRequest(s, request))
				require.NoError(t, BeginRound(s, request.ID))
				s.Requests[0].Stage = "reviewer"
			case "exhausted-rounds":
				require.NoError(t, BeginRequest(s, request))
				for i := 0; i < s.MaxRounds; i++ {
					require.NoError(t, BeginRound(s, request.ID))
					s.Requests[0].Stage = ""
				}
			}
			require.NoError(t, Validate(s), "failed admission must be exercised from valid durable state")
			before := Clone(s)
			require.Error(t, RecordPlanForReview(s, plan, request))
			require.Equal(t, before, s, "a rejected plan, request or round must not leak plans, counters or authority")
			require.NoError(t, Validate(s))
		})
	}
}

func TestRequestOriginIsOptionalButCannotBeReusedOrRewritten(t *testing.T) {
	for _, original := range []string{"", "collaboration-edit", "input-images-edit"} {
		t.Run("origin-"+original, func(t *testing.T) {
			s := testState(t)
			request := Request{ID: "same-text", Origin: original, Scope: "packet", ScopeID: "M01-T01", InputRevision: s.InputRevision, PlanningOnly: true,
				Feedback: "Manual collaboration revision exact-input-digest"}
			require.NoError(t, BeginRequest(s, request))
			require.NoError(t, BeginRequest(s, request))
			require.NoError(t, Validate(s))
			changed := request
			changed.Origin = "input-images-edit"
			if original == "input-images-edit" {
				changed.Origin = "collaboration-edit"
			}
			before := Clone(s)
			require.ErrorContains(t, BeginRequest(s, changed), "identity was reused")
			require.Equal(t, before, s)
			next := Clone(s)
			next.Requests[0].Origin = changed.Origin
			require.NoError(t, Validate(next), "both origins are valid shapes; changing recorded provenance is the forbidden operation")
			require.ErrorContains(t, ValidateTransition(s, next), "identity")
			if original != "" {
				next.Requests[0].Origin = ""
				require.ErrorContains(t, ValidateTransition(s, next), "identity", "recorded edit provenance cannot be erased")
			}
			next.Requests[0].Origin = "unknown"
			require.Error(t, Validate(next))
		})
	}
}

func TestAdmitPlanRevisionRetainsExactReviewAndApprovalWithoutGrantingStart(t *testing.T) {
	s, ref := readyState(t)
	s.StartAuthorized, s.Approvals[0].Start = false, false
	before := Clone(s)
	require.NoError(t, AdmitPlanRevision(s, ref))
	require.ErrorContains(t, Admit(s, ref), "authorize starting implementation")
	require.Equal(t, before, s, "review-only admission is read-only")
	for _, scenario := range []string{"unapproved-design", "missing-plan-review", "failed-plan-review", "stale-plan-reference", "pending-activation"} {
		t.Run(scenario, func(t *testing.T) {
			next := Clone(s)
			candidate := ref
			switch scenario {
			case "unapproved-design":
				next.Approvals = []Approval{}
			case "missing-plan-review":
				next.Reviews = next.Reviews[:1]
			case "failed-plan-review":
				next.Reviews[1].Verdict = "revise"
			case "stale-plan-reference":
				candidate.Plans = []Ref{{Revision: 99, Digest: strings.Repeat("a", 64)}}
			case "pending-activation":
				next.PendingActivation = &Activation{Design: next.Active.Design, Plans: next.Active.Plans, Files: next.Active.Files, OldFiles: map[string]string{}, Stage: "intent", Base: "base"}
			}
			require.Error(t, AdmitPlanRevision(next, candidate))
		})
	}
}
