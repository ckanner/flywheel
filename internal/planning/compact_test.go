package planning

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func compactState(t *testing.T) (*State, Review) {
	t.Helper()
	s := testState(t)
	d := addDesign(t, s)
	p := testPlan(t, s, d.Ref, "M-01")
	p.Preparation, p.PlannerInvocation = "compact-v1", d.PlannerInvocation
	var err error
	p, err = SealPlan(p)
	require.NoError(t, err)
	require.NoError(t, RecordPlan(s, p))
	r := designReview(s, d)
	r.Kind = "compact-v1"
	r.Plan = &p.Ref
	r.ScopeVerdict = "within-envelope"
	r.DesignVerdict = "valid"
	r.CoverageVerdict = "complete"
	return s, r
}

func TestCompactReviewIsOneIndependentAuthorityForBothExactArtifacts(t *testing.T) {
	s, r := compactState(t)
	require.NoError(t, RecordReview(s, r))
	require.NoError(t, Validate(s))
	require.Same(t, &s.Reviews[0], LatestReview(s, "design", r.Target))
	require.Same(t, &s.Reviews[0], LatestReview(s, "plan", *r.Plan))
	require.Len(t, ReviewHistory(s, "compact-v1", r.Target), 1)
	require.Len(t, ReviewHistory(s, "plan", *r.Plan), 1)
	require.NoError(t, Approve(s, Approval{Kind: "human", Design: r.Target, ReviewID: r.ID, RequestID: "human-approve", Source: "trusted-user", At: "now", Start: true}))
	s.Active = &Activation{Design: r.Target, Plans: []Ref{*r.Plan}, Files: map[string]string{"spec.md": DigestBytes([]byte("projected"))}, OldFiles: map[string]string{}, Stage: "active", Base: "base", Head: "head"}
	s.Phase = "ready"
	require.NoError(t, Admit(s, ActivationRefs(*s.Active)))
	require.Len(t, s.Reviews, 1)
	require.False(t, reviewTarget(r, "review", r.Target))
}

func TestCompactAdoptionIsAtomicAcrossBothArtifacts(t *testing.T) {
	fixture, _ := compactState(t)
	design, plan := fixture.Designs[0], fixture.Plans[0]
	s := testState(t)
	badDesign := design
	badDesign.Ref.Digest = DigestBytes([]byte("altered"))
	require.Error(t, RecordCompact(s, badDesign, plan))
	require.Empty(t, s.Designs)
	badPlan := plan
	badPlan.Ref.Revision++
	badPlan, err := SealPlan(badPlan)
	require.NoError(t, err)
	require.Error(t, RecordCompact(s, design, badPlan))
	require.Empty(t, s.Designs, "a rejected child cannot leave an adopted root behind")
	require.Empty(t, s.Plans)
	require.NoError(t, RecordCompact(s, design, plan))
	require.Equal(t, design.Ref, *s.CurrentDesign)
	require.Equal(t, plan.Ref, *s.CurrentPlan)
}

func TestCompactReviewCannotApprovePartialStaleOrSelfReviewedPlans(t *testing.T) {
	for name, edit := range map[string]func(*State, *Review){
		"missing plan":        func(s *State, r *Review) { r.Plan = nil },
		"unknown plan":        func(s *State, r *Review) { v := *r.Plan; v.Digest = DigestBytes([]byte("other")); r.Plan = &v },
		"no design verdict":   func(s *State, r *Review) { r.DesignVerdict = "" },
		"incomplete coverage": func(s *State, r *Review) { r.CoverageVerdict = "incomplete" },
		"scope change":        func(s *State, r *Review) { r.ScopeVerdict = "material-change" },
		"self review":         func(s *State, r *Review) { r.Invocation = s.Designs[0].PlannerInvocation },
		"ordinary design":     func(s *State, r *Review) { r.Kind = "design" },
		"stale projection":    func(s *State, r *Review) { s.CurrentPlan = nil },
	} {
		t.Run(name, func(t *testing.T) {
			s, r := compactState(t)
			edit(s, &r)
			require.Error(t, RecordReview(s, r))
			require.Empty(t, s.Reviews)
			require.False(t, Authorized(s, r.Target))
		})
	}
	s, r := compactState(t)
	p := s.Plans[0]
	p.Files = map[string]string{"spec.md": "no packet"}
	_, err := SealPlan(p)
	require.Error(t, err)
	ordinary := planReview(s.Plans[0])
	ordinary.Plan = r.Plan
	require.Error(t, RecordReview(s, ordinary))
}

func TestCompactGranularityRequiresEvidenceAndOnlyEscalates(t *testing.T) {
	b := testBrief()
	b.DesignGranularity = "compact"
	b.Mode = "auto"
	b.GranularityReason = "Localized existing pattern and executable checks"
	require.NoError(t, ValidateBrief(b))
	b.Evidence = nil
	require.Error(t, ValidateBrief(b))
	b.Evidence = []string{"source"}
	b.GranularityReason = ""
	require.Error(t, ValidateBrief(b))
	b.GranularityReason = "reason"
	b.Mode = "full"
	require.Error(t, ValidateBrief(b))
	b.Mode = "auto"
	s, err := New(b)
	require.NoError(t, err)
	r := Request{ID: "request", Scope: "design", InputRevision: 1}
	require.NoError(t, BeginRequest(s, r))
	require.Equal(t, "compact", DesignGranularity(s, r))
	r.Scope = "milestone"
	require.Equal(t, "system", DesignGranularity(s, r))
	before := Clone(s)
	s.Requests[0].DesignGranularity = "system"
	require.NoError(t, ValidateTransition(before, s))
	require.Equal(t, "system", DesignGranularity(s, s.Requests[0]))
	before = Clone(s)
	s.Requests[0].DesignGranularity = "compact"
	require.Error(t, ValidateTransition(before, s))
	s.Requests[0].DesignGranularity = ""
	require.Error(t, ValidateTransition(before, s))
}
