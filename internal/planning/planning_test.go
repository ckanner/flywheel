package planning

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func testBrief() Brief {
	return Brief{Version: 1, ID: "orders", Goal: "Add idempotent order creation", OriginalRequest: "Design orders before implementing",
		Constraints: []string{"Preserve public API"}, Evidence: []string{"src/orders.go"}, Policy: "review-first", Mode: "full"}
}

func TestRequestFallbackBindingSurvivesValidatedTransitions(t *testing.T) {
	s := testState(t)
	require.NoError(t, BeginRequest(s, Request{ID: "scope-1", Scope: "design", ScopeID: "root", InputRevision: 1}))
	before := Clone(s)
	s.Requests[0].Role, s.Requests[0].Channel = "design-plan", "fallback-provider"
	require.NoError(t, Validate(s))
	require.NoError(t, ValidateTransition(before, s))
	s.Requests[0].Role = "execute"
	require.Error(t, Validate(s))
	s.Requests[0].Role, s.Requests[0].Channel = "plan-review", " "
	require.Error(t, Validate(s))
}

func TestRootPlanningRequestMayOmitScopeIDButChildRequestsCannot(t *testing.T) {
	for _, scope := range []string{"design", "replan", "milestone", "packet"} {
		for _, id := range []string{"", "M-01"} {
			s := testState(t)
			err := BeginRequest(s, Request{ID: "request", Scope: scope, ScopeID: id, InputRevision: 1})
			if id == "" && (scope == "milestone" || scope == "packet") {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NoError(t, Validate(s))
			}
		}
	}
}

func TestMigratedContractOwnershipIsBoundedAndImmutable(t *testing.T) {
	s := testState(t)
	before := Clone(s)
	s.LegacyFiles = map[string]string{"spec.md": DigestBytes([]byte("original spec")), "task-packet-M01-T01.md": DigestBytes([]byte("original packet"))}
	require.NoError(t, Validate(s))
	require.NoError(t, ValidateTransition(before, s))
	require.NoError(t, ValidateTransition(s, Clone(s)))
	changed := Clone(s)
	changed.LegacyFiles["spec.md"] = DigestBytes([]byte("external edits"))
	require.ErrorContains(t, ValidateTransition(s, changed), "ownership")
	changed.LegacyFiles = nil
	require.ErrorContains(t, ValidateTransition(s, changed), "ownership")
	s.LegacyFiles["../../elsewhere.md"] = DigestBytes(nil)
	require.Error(t, Validate(s))
	delete(s.LegacyFiles, "../../elsewhere.md")
	s.LegacyFiles["spec.md"] = "unverified"
	require.Error(t, Validate(s))
}

func TestAuthorityCannotRestoreOlderSealedVersionsOrPreFeedbackApproval(t *testing.T) {
	s, ref := readyState(t)
	oldPlan := *s.CurrentPlan
	plan := testPlan(t, s, ref.Design, "M-01")
	require.NoError(t, RecordPlan(s, plan))
	require.NoError(t, Validate(s))
	s.CurrentPlan = &oldPlan
	s.CurrentPlans["milestone/M-01"] = oldPlan
	require.ErrorContains(t, Validate(s), "latest sealed")
	s.CurrentPlan = &plan.Ref
	s.CurrentPlans["milestone/M-01"] = plan.Ref
	require.NoError(t, ApplyFeedback(s, Feedback{RequestID: "root-feedback", Source: "verified-user", Text: "Include tenant isolation", At: "now"}))
	require.False(t, Authorized(s, ref.Design), "feedback invalidates design authority independently of phase")
	s.Phase = "ready"
	require.Error(t, Admit(s, ref))
	d := addDesign(t, s)
	require.Empty(t, s.CurrentPlans)
	require.Nil(t, s.CurrentPlan)
	require.NoError(t, Validate(s))
	s.CurrentDesign = &ref.Design
	require.ErrorContains(t, Validate(s), "latest sealed")
	require.False(t, Authorized(s, d.Ref))
	s.CurrentDesign = nil
	require.ErrorContains(t, Validate(s), "latest sealed")
	require.False(t, Authorized(s, ref.Design))
	require.False(t, Authorized(s, Ref{Revision: 9, Digest: strings.Repeat("f", 64)}))
}

func testState(t *testing.T) *State {
	t.Helper()
	s, err := New(testBrief())
	require.NoError(t, err)
	return s
}

func testDesign(t *testing.T, s *State) Design {
	t.Helper()
	d, err := SealDesign(Design{Ref: Ref{Revision: len(s.Designs) + 1}, InputRevision: s.InputRevision,
		Document:     "# Orders\nUse one atomic write with an idempotency key; retries return the same result.",
		Requirements: []Requirement{{ID: "R-01", Title: "Idempotent requests", Milestone: "M-01", Verification: "Concurrent duplicate requests return the same order"}},
		Decisions:    []Decision{{ID: "D-01", Kind: "fixed", Statement: "Reuse transactional storage", Rationale: "Avoid partial writes"}},
		Milestones:   []Milestone{{ID: "M-01", Title: "Order service", Gate: "Duplicate request and rollback tests pass"}},
		Baseline:     "repo-head", PlannerInvocation: "planner-one", ImplementationScope: []string{"src/orders"}})
	require.NoError(t, err)
	return d
}

func addDesign(t *testing.T, s *State) Design {
	t.Helper()
	d := testDesign(t, s)
	require.NoError(t, RecordDesign(s, d))
	return d
}

func testPlan(t *testing.T, s *State, design Ref, scopeID string) ExecutionPlan {
	t.Helper()
	p, err := SealPlan(ExecutionPlan{Ref: Ref{Revision: len(s.Plans) + 1}, Design: design, InputRevision: s.InputRevision,
		Scope: "milestone", ScopeID: scopeID, Files: map[string]string{"spec.md": "immutable source spec", "task-packet-M01-T01.md": "immutable source packet"},
		Dependencies: "repo-head:verified", ScopeVerdict: "within-envelope", PlannerInvocation: "packet-planner"})
	require.NoError(t, err)
	return p
}

func designReview(s *State, d Design) Review {
	return Review{ID: "design-review-one", Kind: "design", Target: d.Ref, InputRevision: d.InputRevision,
		Invocation: "independent-review-call", Role: "design-review", Binding: "review-binding", Verdict: "pass", Summary: "Requirements and failure recovery are covered", Findings: []Finding{}}
}

func planReview(p ExecutionPlan) Review {
	return Review{ID: "plan-review-" + p.ScopeID, Kind: "plan", Target: p.Ref, Design: &p.Design, InputRevision: p.InputRevision,
		Invocation: "independent-plan-review", Role: "plan-review", Binding: "review-binding", Verdict: "pass", ScopeVerdict: "within-envelope", Summary: "Current plan remains within the approved architecture", Findings: []Finding{}}
}

func approveDesign(t *testing.T, s *State, d Design, start bool) {
	t.Helper()
	r := designReview(s, d)
	require.NoError(t, RecordReview(s, r))
	require.NoError(t, Approve(s, Approval{Kind: "human", Design: d.Ref, ReviewID: r.ID, RequestID: "approve-one", Source: "trusted-ui-action", At: "2026-11-16T00:00:00Z", Start: start}))
}

func readyState(t *testing.T) (*State, ActivationRef) {
	t.Helper()
	s := testState(t)
	d := addDesign(t, s)
	approveDesign(t, s, d, true)
	p := testPlan(t, s, d.Ref, "M-01")
	require.NoError(t, RecordPlan(s, p))
	require.NoError(t, RecordReview(s, planReview(p)))
	s.Active = &Activation{Design: d.Ref, Plans: []Ref{p.Ref}, Files: map[string]string{"spec.md": DigestBytes([]byte("projected spec"))}, OldFiles: map[string]string{}, Stage: "active", Base: "base", Head: "committed"}
	s.Phase = "ready"
	ref := ActivationRefs(*s.Active)
	require.NoError(t, Admit(s, ref))
	return s, ref
}

func TestReviewedPlanningStopsBeforeApprovalAndAllowsRootFirst(t *testing.T) {
	s := testState(t)
	require.NoError(t, Validate(s))
	d := addDesign(t, s)
	require.Equal(t, "reviewing", s.Phase)
	require.NoError(t, RecordDesign(s, d), "recovery adopts the exact result once")
	approveDesign(t, s, d, false)
	require.True(t, Authorized(s, d.Ref))
	require.Empty(t, s.Plans, "root approval must not require fabricated child packets")
	require.False(t, s.StartAuthorized)
	require.NoError(t, Validate(s))
	require.Error(t, Admit(s, ActivationRef{Version: 1, Design: d.Ref, Plans: []Ref{d.Ref}}))
	p := testPlan(t, s, d.Ref, "M-01")
	require.NoError(t, RecordPlan(s, p))
	require.NoError(t, RecordPlan(s, p))
	require.NoError(t, RecordReview(s, planReview(p)))
	require.True(t, Authorized(s, d.Ref), "child review does not replace root approval")
	require.NoError(t, Validate(s))
}

func TestSealContractsRejectsTamperingAndInvalidScope(t *testing.T) {
	s := testState(t)
	d := testDesign(t, s)
	for _, edit := range []func(*Design){
		func(d *Design) { d.Document += " changed after review" },
		func(d *Design) { d.Ref.Revision = 0 },
	} {
		candidate := d
		edit(&candidate)
		require.Error(t, validateDesign(candidate))
	}
	for _, edit := range []func(*Design){
		func(d *Design) { d.Milestones = append(d.Milestones, d.Milestones[0]) },
		func(d *Design) { d.Requirements = append(d.Requirements, d.Requirements[0]) },
		func(d *Design) {
			d.Requirements = []Requirement{{ID: "R-01", Title: "Missing milestone", Milestone: "M-99", Verification: "test"}}
		},
		func(d *Design) { d.Decisions = append(d.Decisions, d.Decisions[0]) },
		func(d *Design) { d.ImplementationScope = []string{"../outside"} },
		func(d *Design) { d.ImplementationScope = []string{"/absolute"} },
		func(d *Design) { d.ImplementationScope = []string{"a/../outside"} },
	} {
		candidate := d
		edit(&candidate)
		_, err := SealDesign(candidate)
		require.Error(t, err)
	}
	d.Decisions = nil
	d, err := SealDesign(d)
	require.NoError(t, err)
	require.NotNil(t, d.Decisions)
	p := testPlan(t, s, d.Ref, "M-01")
	p.Files["spec.md"] += "tampered"
	require.Error(t, validatePlan(p))
	p.Files["../state.json"] = "bad"
	_, err = SealPlan(p)
	require.Error(t, err, "planner output cannot address arbitrary paths")
	_, err = New(Brief{})
	require.Error(t, err)
	b := testBrief()
	b.Policy = "delegated"
	require.Error(t, ValidateBrief(b))
	b.Authorization = "explicit-user-request"
	require.NoError(t, ValidateBrief(b))
}

func TestReviewRequiresIndependentExactInputAndEvidence(t *testing.T) {
	s := testState(t)
	d := addDesign(t, s)
	r := designReview(s, d)
	for _, edit := range []func(*Review){
		func(r *Review) { r.Role = "plan-review" },
		func(r *Review) { r.Target.Digest = strings.Repeat("b", 64) },
		func(r *Review) { r.Invocation = d.PlannerInvocation },
		func(r *Review) { r.InputRevision++ },
		func(r *Review) { r.Verdict = "unknown" },
		func(r *Review) {
			r.Findings = []Finding{{ID: "F-1", Severity: "blocker", Target: "D-01", Evidence: "duplicate requests create two rows", Impact: "duplicates", Status: "open"}}
		},
		func(r *Review) {
			r.Findings = []Finding{{ID: "F-1", Severity: "major", Target: "D-01", Evidence: "counterexample", Impact: "duplicates", Status: "resolved"}}
		},
	} {
		candidate := r
		edit(&candidate)
		require.Error(t, RecordReview(s, candidate))
	}
	r.Findings = []Finding{{ID: "F-1", Severity: "major", Target: "D-01", Evidence: "counterexample", Impact: "duplicates", Status: "resolved", Resolution: "Unique index and retry test"}}
	r.Findings = append(r.Findings, r.Findings[0])
	require.Error(t, RecordReview(s, r))
	r.Findings = r.Findings[:1]
	require.NoError(t, RecordReview(s, r))
	require.NoError(t, RecordReview(s, r))
	r.Summary += " different"
	require.Error(t, RecordReview(s, r), "review IDs cannot be reused for other results")
	newDesign := testDesign(t, s)
	newDesign.Decisions[0].Kind = "unresolved"
	newDesign, err := SealDesign(newDesign)
	require.NoError(t, err)
	require.NoError(t, RecordDesign(s, newDesign))
	r = designReview(s, newDesign)
	r.ID = "new-review"
	require.Error(t, RecordReview(s, r), "unresolved product decisions cannot become an automatic pass")
	for _, verdict := range []string{"revise", "needs-user-decision", "blocked"} {
		r.ID, r.Verdict = verdict, verdict
		require.NoError(t, RecordReview(s, r))
	}
	old := designReview(s, d)
	old.ID = "stale-review"
	require.Error(t, RecordReview(s, old))
}

func TestPlanReviewAndVersionsRetainIndependentScopes(t *testing.T) {
	s := testState(t)
	d := addDesign(t, s)
	approveDesign(t, s, d, true)
	p := testPlan(t, s, d.Ref, "M-01")
	require.NoError(t, RecordPlan(s, p))
	r := planReview(p)
	for _, edit := range []func(*Review){
		func(r *Review) { r.Role = "design-review" },
		func(r *Review) { r.Design = nil },
		func(r *Review) { r.ScopeVerdict = "material-change" },
		func(r *Review) { r.Target.Digest = strings.Repeat("b", 64) },
	} {
		candidate := r
		edit(&candidate)
		require.Error(t, RecordReview(s, candidate))
	}
	require.NoError(t, RecordReview(s, r))
	p2 := testPlan(t, s, d.Ref, "M-02")
	require.NoError(t, RecordPlan(s, p2))
	require.NoError(t, RecordReview(s, planReview(p2)))
	require.Equal(t, p.Ref, s.CurrentPlans["milestone/M-01"])
	require.Equal(t, p2.Ref, s.CurrentPlans["milestone/M-02"])
	require.NoError(t, Validate(s))
	p3 := testPlan(t, s, d.Ref, "M-01")
	require.NoError(t, RecordPlan(s, p3))
	r.ID = "stale-plan-review"
	require.Error(t, RecordReview(s, r))
	delete(s.CurrentPlans, "milestone/M-01")
	r = planReview(p3)
	r.ID = "absent-current-plan"
	require.Error(t, RecordReview(s, r))
	p4 := testPlan(t, s, d.Ref, "M-03")
	p4.Ref.Revision++
	p4, err := SealPlan(p4)
	require.NoError(t, err)
	require.Error(t, RecordPlan(s, p4))
	p4.Ref.Digest = "invalid"
	require.Error(t, RecordPlan(s, p4))
	d2 := testDesign(t, s)
	d2.Ref.Revision++
	d2, err = SealDesign(d2)
	require.NoError(t, err)
	require.Error(t, RecordDesign(s, d2))
	d2.Ref.Digest = "invalid"
	require.Error(t, RecordDesign(s, d2))
}

func TestApprovalAndFeedbackAreExactIdempotentControlEvents(t *testing.T) {
	s := testState(t)
	d := addDesign(t, s)
	a := Approval{Kind: "human", Design: d.Ref, ReviewID: "design-review-one", RequestID: "approve-one", Source: "trusted-control", At: "now"}
	require.False(t, Authorized(s, d.Ref))
	require.Error(t, Approve(s, a))
	require.NoError(t, RecordReview(s, designReview(s, d)))
	require.NoError(t, Approve(s, a))
	require.NoError(t, Approve(s, a))
	a.Start = true
	require.Error(t, Approve(s, a), "same request ID cannot silently add execution permission")
	a.RequestID = "approve-and-start"
	require.NoError(t, Approve(s, a))
	require.True(t, s.StartAuthorized)
	for _, edit := range []func(*Approval){
		func(a *Approval) { a.Source = "" },
		func(a *Approval) { a.Design.Digest = strings.Repeat("b", 64) },
		func(a *Approval) { a.Kind = "delegated" },
		func(a *Approval) { a.ReviewID = "missing" },
	} {
		candidate := a
		candidate.RequestID = "different"
		edit(&candidate)
		require.Error(t, Approve(s, candidate))
	}
	f := Feedback{RequestID: "feedback-one", Target: &d.Ref, Source: "trusted-control", Text: "Keep all tenant keys isolated", At: "now"}
	require.NoError(t, ApplyFeedback(s, f))
	require.NoError(t, ApplyFeedback(s, f))
	require.Equal(t, 2, s.InputRevision)
	require.Equal(t, "drafting", s.Phase)
	a.RequestID = "stale-approve"
	require.Error(t, Approve(s, a))
	f.Text = "changed"
	require.Error(t, ApplyFeedback(s, f))
	f.RequestID = "next"
	f.Policy = "delegated"
	require.Error(t, ApplyFeedback(s, f))
	require.Len(t, s.Feedback, 1, "rejected policy changes are not applied")
	f.Policy = "review-first"
	require.NoError(t, ApplyFeedback(s, f))
	f.RequestID = "stale"
	missing := Ref{Revision: 22, Digest: strings.Repeat("a", 64)}
	f.Target = &missing
	require.Error(t, ApplyFeedback(s, f))
	f.Source = ""
	require.Error(t, ApplyFeedback(s, f))
	require.NoError(t, Validate(s))
}

func TestDelegationIsNotHumanApprovalAndCannotOverrideReviewFirst(t *testing.T) {
	b := testBrief()
	b.Policy, b.Authorization, b.ImplementationAuthorized = "delegated", "explicit-user-message", true
	s, err := New(b)
	require.NoError(t, err)
	d := addDesign(t, s)
	r := designReview(s, d)
	require.NoError(t, RecordReview(s, r))
	a := Approval{Kind: "delegated", Design: d.Ref, ReviewID: r.ID, RequestID: "delegate", Source: b.Authorization, At: "now"}
	require.NoError(t, Approve(s, a))
	require.True(t, Authorized(s, d.Ref))
	s.Policy = "review-first"
	a.RequestID = "again"
	require.Error(t, Approve(s, a))
	require.False(t, Authorized(s, d.Ref))
	require.NoError(t, Validate(s), "past delegation remains historical truth after policy changes")
}

func TestRoundsAreDurableAndRequestIdentityCannotResetCounters(t *testing.T) {
	s := testState(t)
	r := Request{ID: "initial-design", Scope: "design", ScopeID: "root", InputRevision: 1, PlanningOnly: true}
	require.NoError(t, BeginRequest(s, r))
	for i := 0; i < 3; i++ {
		require.NoError(t, BeginRound(s, r.ID))
		require.Equal(t, "planner", s.Requests[0].Stage)
		require.NoError(t, BeginRound(s, r.ID), "crash resume does not count the same round twice")
		s.Requests[0].Stage = "reviewer"
		require.Error(t, BeginRound(s, r.ID))
		s.Requests[0].Stage = ""
		require.NoError(t, BeginRequest(s, r))
	}
	require.Error(t, BeginRound(s, r.ID))
	require.Equal(t, 3, s.Rounds)
	require.Equal(t, "budget-stopped", s.Phase)
	require.Error(t, BeginRound(s, r.ID))
	require.Error(t, BeginRound(s, "unknown"))
	r.ScopeID = "different"
	require.Error(t, BeginRequest(s, r))
	r.ID, r.InputRevision = "new-but-stale", 2
	require.Error(t, BeginRequest(s, r))
	r.Scope = "unknown"
	require.Error(t, BeginRequest(s, r))
	require.NoError(t, Validate(s))
	copy := Clone(s)
	copy.Requests[0].Rounds--
	require.Error(t, ValidateTransition(s, copy))
	require.NoError(t, ApplyFeedback(s, Feedback{RequestID: "feedback", Text: "Need a different API", Source: "user", At: "now"}))
	require.Equal(t, 3, s.Rounds, "new feedback does not erase spent rounds")
	copy = Clone(s)
	require.NoError(t, Validate(copy))
	require.Nil(t, Clone(nil))
}

func TestScopeRequestFeedbackIsImmutableBoundedAndDoesNotInvalidateRootApproval(t *testing.T) {
	s, ref := readyState(t)
	request := Request{ID: "refine-current-milestone", Scope: "milestone", ScopeID: "M-01", InputRevision: s.InputRevision,
		Feedback: "Keep the approved API and explain retry behavior in each affected packet.", PlanningOnly: true}
	require.NoError(t, BeginRequest(s, request))
	require.Equal(t, request.Feedback, s.Requests[0].Feedback)
	require.NoError(t, BeginRound(s, request.ID))
	s.Requests[0].Stage = "reviewer"
	s.Reason = "A transport error was recovered."
	require.NoError(t, BeginRequest(s, request), "resuming the exact scope request keeps its original feedback")
	require.Equal(t, request.Feedback, s.Requests[0].Feedback)
	require.True(t, Authorized(s, ref.Design))
	require.Empty(t, s.Feedback, "scope-local refinement is not a new root requirement")
	require.Equal(t, 1, s.InputRevision)
	changed := request
	changed.Feedback = "Replace the approved public API."
	require.ErrorContains(t, BeginRequest(s, changed), "identity was reused")
	next := Clone(s)
	next.Requests[0].Feedback = changed.Feedback
	require.ErrorContains(t, ValidateTransition(s, next), "identity")
	request.ID = "oversized-feedback"
	request.Feedback = strings.Repeat("x", 8193)
	require.Error(t, BeginRequest(s, request))
	request.Feedback = strings.Repeat("x", 8192)
	require.NoError(t, BeginRequest(s, request))
	require.NoError(t, ValidateTransition(s, Clone(s)))
}

func TestAdmissionRequiresCurrentApprovedRootAndReviewedPlans(t *testing.T) {
	ready, ref := readyState(t)
	for _, edit := range []func(*State){
		func(s *State) { s.Version = 99 },
		func(s *State) { s.Phase = "awaiting-design-approval" },
		func(s *State) {
			s.PendingActivation = &Activation{Design: s.Active.Design, Plans: s.Active.Plans, Files: s.Active.Files, OldFiles: map[string]string{}, Stage: "intent", Base: "base"}
		},
		func(s *State) { s.StartAuthorized = false },
		func(s *State) { s.Approvals[0].Start = false },
		func(s *State) { delete(s.CurrentPlans, "milestone/M-01") },
		func(s *State) { s.Reviews = s.Reviews[:1] },
		func(s *State) { s.Reviews[1].Verdict = "revise" },
	} {
		candidate := Clone(ready)
		edit(candidate)
		require.Error(t, Admit(candidate, ref))
	}
	bad := ref
	bad.Version = 2
	require.Error(t, Admit(ready, bad))
	bad = ref
	bad.Plans = append(append([]Ref{}, ref.Plans...), ref.Plans[0])
	require.Error(t, Admit(ready, bad))
	bad.Plans = []Ref{{Revision: 2, Digest: strings.Repeat("c", 64)}}
	require.Error(t, Admit(ready, bad))
	ready.Brief.ImplementationAuthorized = true
	ready.Brief.Authorization = "earlier-explicit-authority"
	ready.Approvals[0].Start = false
	require.NoError(t, Admit(ready, ref), "separately recorded earlier implementation authority remains usable")
}

func TestStartGrantBelongsToTheExactCurrentDesignOrOriginalTaskAuthority(t *testing.T) {
	s, ref := readyState(t)
	require.True(t, StartAllowed(s, ref.Design))
	s.StartAuthorized = false
	require.False(t, StartAllowed(s, ref.Design))
	s.StartAuthorized = true
	s.CurrentDesign = nil
	require.False(t, StartAllowed(s, ref.Design))
	s.CurrentDesign = &ref.Design
	require.False(t, StartAllowed(s, Ref{Revision: 99, Digest: strings.Repeat("c", 64)}))
	require.NoError(t, ApplyFeedback(s, Feedback{RequestID: "redesign", Text: "Change the approved implementation boundary.", Source: "user", At: "now"}))
	design := addDesign(t, s)
	review := designReview(s, design)
	review.ID, review.Invocation = "new-root-review", "new-review-call"
	require.NoError(t, RecordReview(s, review))
	require.NoError(t, Approve(s, Approval{Kind: "human", Design: design.Ref, ReviewID: review.ID,
		RequestID: "approve-only-new-root", Source: "user", At: "now"}))
	require.True(t, Authorized(s, design.Ref))
	require.True(t, s.StartAuthorized, "historical start events remain recorded")
	require.False(t, StartAllowed(s, design.Ref), "approve-only cannot inherit the earlier root's Start")
	require.False(t, StartAllowed(s, ref.Design), "the previous design is no longer current")

	// A separately authorized original implementation task remains authorized
	// when its design is revised and reviewed; this does not create an approval.
	original := Clone(s)
	original.Brief.ImplementationAuthorized = true
	require.True(t, StartAllowed(original, design.Ref))
	approval := Approval{Kind: "human", Design: design.Ref, ReviewID: review.ID, RequestID: "start-new-root", Source: "user", At: "now", Start: true}
	require.NoError(t, Approve(s, approval))
	require.True(t, StartAllowed(s, design.Ref))
}

func TestValidateRejectsCorruptHistoricalReferences(t *testing.T) {
	ready, _ := readyState(t)
	for name, edit := range map[string]func(*State){
		"brief authorization": func(s *State) { s.Brief.ImplementationAuthorized = true },
		"input counter":       func(s *State) { s.InputRevision++ },
		"design bytes":        func(s *State) { s.Designs[0].Document += "tamper" },
		"design sequence":     func(s *State) { s.Designs[0].Ref.Revision = 2; s.Designs[0], _ = SealDesign(s.Designs[0]) },
		"plan bytes":          func(s *State) { s.Plans[0].Files["spec.md"] = "tamper" },
		"plan sequence":       func(s *State) { s.Plans[0].Ref.Revision = 2; s.Plans[0], _ = SealPlan(s.Plans[0]) },
		"review role":         func(s *State) { s.Reviews[0].Role = "plan-review" },
		"review duplicate":    func(s *State) { s.Reviews = append(s.Reviews, s.Reviews[0]) },
		"approval reference":  func(s *State) { s.Approvals[0].ReviewID = "missing" },
		"approval duplicate":  func(s *State) { s.Approvals = append(s.Approvals, s.Approvals[0]) },
		"feedback reference": func(s *State) {
			target := Ref{Revision: 3, Digest: strings.Repeat("a", 64)}
			s.Feedback = append(s.Feedback, Feedback{RequestID: "unknown", Target: &target, Text: "x", Source: "user", At: "now"})
			s.InputRevision++
		},
		"feedback duplicate": func(s *State) {
			f := Feedback{RequestID: "same", Text: "x", Source: "user", At: "now"}
			s.Feedback = []Feedback{f, f}
			s.InputRevision = 3
		},
		"request duplicate": func(s *State) {
			r := Request{ID: "same", Scope: "design", ScopeID: "root", InputRevision: 1, MaxRounds: 3, Status: "pending"}
			s.Requests = []Request{r, r}
		},
		"request counters": func(s *State) {
			s.Requests = []Request{{ID: "x", Scope: "design", ScopeID: "root", InputRevision: 1, MaxRounds: 3, Rounds: 4, Status: "running"}}
		},
		"total rounds":               func(s *State) { s.Rounds = 1 },
		"current design":             func(s *State) { s.CurrentDesign = &Ref{Revision: 9, Digest: strings.Repeat("a", 64)} },
		"current plan key":           func(s *State) { s.CurrentPlans["wrong/key"] = *s.CurrentPlan },
		"current plan missing":       func(s *State) { s.CurrentPlans["wrong/key"] = Ref{Revision: 9, Digest: strings.Repeat("a", 64)} },
		"activation stage ownership": func(s *State) { s.Active.Stage = "intent" },
		"pending stage ownership":    func(s *State) { v := *s.Active; s.PendingActivation = &v },
		"activation no source":       func(s *State) { s.Active.Base = "" },
		"activation design":          func(s *State) { s.Active.Design.Digest = strings.Repeat("a", 64) },
		"activation duplicate plan":  func(s *State) { s.Active.Plans = append(s.Active.Plans, s.Active.Plans[0]) },
		"activation absent plan":     func(s *State) { s.Active.Plans[0].Digest = strings.Repeat("b", 64) },
		"activation incomplete projection": func(s *State) {
			delete(s.Active.Files, "spec.md")
			s.Active.Files["task-packet-M01-T01.md"] = strings.Repeat("a", 64)
		},
		"activation missing head": func(s *State) { s.Active.Head = "" },
	} {
		t.Run(name, func(t *testing.T) { s := Clone(ready); edit(s); require.Error(t, Validate(s)) })
	}
	require.NoError(t, Validate(ready))
	value := jsonValue(ready).(map[string]any)
	value["trust-me"] = true
	require.Error(t, ValidateValue(value), "unknown state fields must not be discarded at load")
}

func TestPlanningHistoryCannotBeRewrittenDuringCAS(t *testing.T) {
	ready, _ := readyState(t)
	require.NoError(t, ValidateTransition(nil, ready))
	require.NoError(t, ValidateTransition(ready, Clone(ready)))
	require.Error(t, ValidateTransition(ready, nil))
	for _, edit := range []func(*State){
		func(s *State) { s.Brief.Goal = "another task" },
		func(s *State) { s.Designs = []Design{} },
		func(s *State) { s.Designs[0].Document = "replace history" },
	} {
		next := Clone(ready)
		edit(next)
		require.Error(t, ValidateTransition(ready, next))
	}
	require.NoError(t, BeginRequest(ready, Request{ID: "initial", Scope: "design", ScopeID: "root", InputRevision: 1}))
	next := Clone(ready)
	next.Requests = []Request{}
	require.Error(t, ValidateTransition(ready, next))
	next = Clone(ready)
	next.Requests[0].MaxRounds = 100
	require.Error(t, ValidateTransition(ready, next))
	next = Clone(ready)
	require.NoError(t, BeginRound(next, "initial"))
	require.NoError(t, ValidateTransition(ready, next))
}

func TestSchemaLoadingErrorsRemainVisible(t *testing.T) {
	_, err := compile(templates.FS.ReadFile)
	require.NoError(t, err)
	want := errors.New("schema unavailable")
	_, err = compile(func(string) ([]byte, error) { return nil, want })
	require.ErrorIs(t, err, want)
	_, err = compile(func(string) ([]byte, error) { return []byte("{"), nil })
	require.Error(t, err)
	_, err = compile(func(string) ([]byte, error) { return []byte(`{"type":"not-a-type"}`), nil })
	require.Error(t, err)
	_, err = compile(func(string) ([]byte, error) { return []byte(`{"type":"object"}`), nil })
	require.Error(t, err)
	old := compiled
	t.Cleanup(func() { compiled = old })
	compiled = func() (map[string]*jsonschema.Schema, error) { return nil, want }
	require.ErrorIs(t, ValidateBrief(testBrief()), want)
}

// Keep imported concrete JSON and schema shapes checked by an independent
// round trip, rather than accepting a typed decoder that discards new fields.
func TestPublicSchemaIsEmbeddedAndPlanningCloneDoesNotAlias(t *testing.T) {
	raw, err := os.ReadFile("../../templates/planning.schema.json")
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(raw, &document))
	require.Equal(t, schemaID, document["$id"])
	s, _ := readyState(t)
	copy := Clone(s)
	require.True(t, reflect.DeepEqual(s, copy))
	copy.Plans[0].Files["spec.md"] = "different"
	require.NotEqual(t, s.Plans[0].Files, copy.Plans[0].Files)
}
