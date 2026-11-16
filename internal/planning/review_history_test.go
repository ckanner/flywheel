package planning

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func blockingFinding(id string) Finding {
	return Finding{ID: id, Severity: "major", Target: "recovery", Evidence: "An interrupted write loses the durable result", Impact: "Duplicate work or data loss", Status: "open"}
}

func TestRootReviewMustResolveHistoricalBlockersByStableID(t *testing.T) {
	s := testState(t)
	d1 := addDesign(t, s)
	r1 := designReview(s, d1)
	r1.ID, r1.Verdict = "review-first-draft", "revise"
	r1.Findings = []Finding{blockingFinding("F-1"), blockingFinding("F-2")}
	require.NoError(t, RecordReview(s, r1))
	d2 := addDesign(t, s)
	r2 := designReview(s, d2)
	r2.ID, r2.Verdict = "review-second-draft", "revise"
	r2.Findings = []Finding{blockingFinding("F-1"), blockingFinding("F-2")}
	r2.Findings[0].Severity = "minor" // a label change does not establish closure
	r2.Findings[1].Status, r2.Findings[1].Resolution = "resolved", "The rollback transaction and restart counterexample now pass"
	require.NoError(t, RecordReview(s, r2))
	d3 := addDesign(t, s)
	r3 := designReview(s, d3)
	r3.ID = "review-final-draft"
	require.ErrorContains(t, RecordReview(s, r3), "previous blocking finding")
	r3.Findings = []Finding{blockingFinding("F-1")}
	r3.Findings[0].Severity = "minor"
	require.ErrorContains(t, RecordReview(s, r3), "previous blocking finding")
	r3.Findings[0].Status, r3.Findings[0].Resolution = "resolved", "Crash replay now retains the original invocation receipt"
	require.NoError(t, RecordReview(s, r3))
	require.NoError(t, Validate(s))
	require.Equal(t, []Review{r1}, ReviewHistory(s, "design", d1.Ref), "later design reviews are not retroactive evidence")
	require.Equal(t, []Review{r1, r2, r3}, ReviewHistory(s, "design", d3.Ref))
	require.NoError(t, RecordReview(s, r1), "idempotent adoption does not reinterpret an old revision")
	corrupt := Clone(s)
	corrupt.Reviews[2].Findings = []Finding{}
	require.ErrorContains(t, Validate(corrupt), "previous blocking finding", "load must reject history which silently erases an unresolved blocker")
}

func TestChildReviewHistoryDoesNotCrossRootOrScope(t *testing.T) {
	s := testState(t)
	d := addDesign(t, s)
	approveDesign(t, s, d, true)
	p1 := testPlan(t, s, d.Ref, "M-01")
	require.NoError(t, RecordPlan(s, p1))
	r1 := planReview(p1)
	r1.ID, r1.Verdict, r1.Findings = "first-child-review", "revise", []Finding{blockingFinding("F-1")}
	require.NoError(t, RecordReview(s, r1))
	p2 := testPlan(t, s, d.Ref, "M-02")
	require.NoError(t, RecordPlan(s, p2))
	r2 := planReview(p2)
	require.NoError(t, RecordReview(s, r2), "unrelated milestone review has its own findings")
	p3 := testPlan(t, s, d.Ref, "M-01")
	require.NoError(t, RecordPlan(s, p3))
	r3 := planReview(p3)
	r3.ID = "revised-child-review"
	require.ErrorContains(t, RecordReview(s, r3), "previous blocking finding")
	require.Equal(t, []Review{r1}, ReviewHistory(s, "plan", p3.Ref))
	require.Equal(t, []Review{r1}, ReviewHistory(s, "plan", p1.Ref))
	require.Equal(t, []Review{r2}, ReviewHistory(s, "plan", p2.Ref))
	d2 := addDesign(t, s)
	p4 := testPlan(t, s, d2.Ref, "M-01")
	require.NoError(t, RecordPlan(s, p4))
	r4 := planReview(p4)
	r4.ID = "new-root-child-review"
	require.NoError(t, RecordReview(s, r4), "a different approved design has a distinct child review lineage")
	require.Equal(t, []Review{r4}, ReviewHistory(s, "plan", p4.Ref))
	require.NoError(t, Validate(s))
	unknown := Ref{Revision: 1, Digest: strings.Repeat("f", 64)}
	require.Empty(t, ReviewHistory(s, "plan", unknown))
	require.Empty(t, ReviewHistory(s, "design", unknown))
	require.Empty(t, ReviewHistory(s, "execute", p4.Ref))
	s.Reviews = append(s.Reviews, Review{Kind: "plan", Target: unknown})
	require.Equal(t, []Review{r4}, ReviewHistory(s, "plan", p4.Ref), "unknown targets never become review evidence")
}

func TestIntermediateReviewClosureRemainsValidForLaterFullReview(t *testing.T) {
	s := testState(t)
	d := addDesign(t, s)
	r := designReview(s, d)
	r.ID, r.Verdict, r.Findings = "first", "revise", []Finding{blockingFinding("F-1")}
	require.NoError(t, RecordReview(s, r))
	r.ID = "resolved-but-revise"
	r.Findings[0].Status, r.Findings[0].Resolution = "resolved", "Verified recovery proof"
	require.NoError(t, RecordReview(s, r))
	pass := designReview(s, d)
	pass.ID = "whole-design-pass"
	require.NoError(t, RecordReview(s, pass))
	late := designReview(s, d)
	late.ID, late.Verdict, late.Findings = "new-counterexample", "revise", []Finding{blockingFinding("F-2")}
	require.NoError(t, RecordReview(s, late))
	require.NoError(t, Validate(s), "future findings do not retroactively falsify earlier independent reviews")
	pass.ID = "unproven-second-pass"
	require.ErrorContains(t, RecordReview(s, pass), "previous blocking finding")
}

func TestReplanAliasCannotEscapeItsExistingFindingScope(t *testing.T) {
	for _, item := range []struct{ scope, id, wantScope, wantID string }{
		{"design", "root", "design", ""}, {"replan", "", "design", ""}, {"replan", "root", "design", ""},
		{"replan", "M-01", "milestone", "M-01"}, {"replan", "M01-T01", "packet", "M01-T01"},
		{"milestone", "M-02", "milestone", "M-02"}, {"replan", "unrecognized", "replan", "unrecognized"},
	} {
		scope, id := CanonicalScope(item.scope, item.id)
		require.Equal(t, item.wantScope, scope)
		require.Equal(t, item.wantID, id)
	}
	s := testState(t)
	d := addDesign(t, s)
	p1 := testPlan(t, s, d.Ref, "M-01")
	require.NoError(t, RecordPlan(s, p1))
	r := planReview(p1)
	r.ID, r.Verdict, r.Findings = "blocking-child", "revise", []Finding{blockingFinding("F-1")}
	require.NoError(t, RecordReview(s, r))
	p2 := testPlan(t, s, d.Ref, "M-01")
	p2.Scope = "replan"
	var err error
	p2, err = SealPlan(p2)
	require.NoError(t, err)
	require.NoError(t, RecordPlan(s, p2))
	require.Equal(t, p2.Ref, s.CurrentPlans["milestone/M-01"])
	require.Len(t, s.CurrentPlans, 1)
	pass := planReview(p2)
	pass.ID = "attempted-alias-pass"
	require.ErrorContains(t, RecordReview(s, pass), "previous blocking finding")
	pass.Findings = []Finding{blockingFinding("F-1")}
	pass.Findings[0].Status, pass.Findings[0].Resolution = "resolved", "Counterexample now has a verified recovery case"
	require.NoError(t, RecordReview(s, pass))
	require.NoError(t, Validate(s))
}
