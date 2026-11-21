package state

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func executionLinkFixture(t *testing.T) (*State, map[string]*RuntimeDecisionRecord) {
	t.Helper()
	st := initialLedgerFixture(t)
	settleRuntimeRecord(t, st, "model-0", "baseline", false)
	r := st.RuntimeDecisions.Records[0]
	r.ModelBindings = map[string]BindingSnapshot{"model-0": {BindingID: "frozen", Binding: SessionConfiguration{Harness: "antigravity", Model: "gemini-4-neon-high"}}}
	r.Execution = &RuntimeDecisionExecution{ContractFingerprint: strings.Repeat("a", 64), Invocation: &RuntimeDecisionInvocation{ID: "call-bound1", StartedAt: "2026-11-13T00:00:01Z", Attempt: 1, Status: "consumed", Dispatch: "dispatched", Outcome: "completed", CleanupConfirmed: true}}
	st.Packets[r.Packet].Attempts = 1
	st.Sessions = &SessionLedger{Records: map[string]*SessionRecord{"execute/" + r.Packet: {Role: "execute", Scope: r.Packet,
		Last: &SessionInvocation{ID: "call-bound1", DecisionID: r.ID, Attempt: 1}}}}
	return st, map[string]*RuntimeDecisionRecord{r.ID: r}
}

func TestRuntimeExecutionMetadataRoundTripsAndLinksOnlyToItsModelDecision(t *testing.T) {
	st, records := executionLinkFixture(t)
	require.NoError(t, validateRuntimeDecisions(st), "optional display metadata is accepted by the real state schema")
	require.NoError(t, validateRuntimeExecutionLinks(st, records))
	r := st.RuntimeDecisions.Records[0]
	for name, change := range map[string]func(*State, *RuntimeDecisionRecord){
		"invalid time":          func(_ *State, r *RuntimeDecisionRecord) { r.Execution.Invocation.StartedAt = "invalid" },
		"wrong attempt":         func(_ *State, r *RuntimeDecisionRecord) { r.Execution.Invocation.Attempt = 2 },
		"pending decision":      func(_ *State, r *RuntimeDecisionRecord) { r.Status = "pending" },
		"nonmodel selection":    func(_ *State, r *RuntimeDecisionRecord) { r.Action = "repair" },
		"missing binding":       func(_ *State, r *RuntimeDecisionRecord) { r.ModelBindings = nil },
		"wrong scope":           func(st *State, _ *RuntimeDecisionRecord) { st.Sessions.Records["execute/M01-T01"].Scope = "M01-T02" },
		"wrong role":            func(st *State, _ *RuntimeDecisionRecord) { st.Sessions.Records["execute/M01-T01"].Role = "review" },
		"wrong session attempt": func(st *State, _ *RuntimeDecisionRecord) { st.Sessions.Records["execute/M01-T01"].Last.Attempt = 2 },
		"unknown decision": func(st *State, _ *RuntimeDecisionRecord) {
			st.Sessions.Records["execute/M01-T01"].Last.DecisionID = "missing"
		},
		"legacy decision":      func(_ *State, r *RuntimeDecisionRecord) { r.Execution = nil },
		"missing live binding": func(_ *State, r *RuntimeDecisionRecord) { r.Execution.Invocation = nil; r.ModelBindings = nil },
	} {
		t.Run(name, func(t *testing.T) {
			st, records := executionLinkFixture(t)
			change(st, st.RuntimeDecisions.Records[0])
			require.Error(t, validateRuntimeExecutionLinks(st, records))
			require.Error(t, validateRuntimeDecisions(st), "load-time validation must reject a corrupt decision/invocation association")
		})
	}
	// A physical invocation cannot be copied into another decision's history.
	other := *r
	other.ID = "another-decision"
	records[other.ID] = &other
	require.ErrorContains(t, validateRuntimeExecutionLinks(st, records), "summary")
}

func TestLegacyRuntimeRecordsNeedNoSyntheticExecutionLink(t *testing.T) {
	st, records := executionLinkFixture(t)
	r := st.RuntimeDecisions.Records[0]
	r.Execution.Invocation = nil
	st.Sessions = nil
	require.NoError(t, validateRuntimeExecutionLinks(st, records))
	st.Sessions = &SessionLedger{Records: map[string]*SessionRecord{"nil": nil, "empty": {}, "legacy": {Last: &SessionInvocation{}}}}
	r.Execution = nil
	require.NoError(t, validateRuntimeExecutionLinks(st, records))
	st.RuntimeDecisions = nil
	st.Packets = map[string]*PacketState{}
	require.NoError(t, validateRuntimeDecisions(st))
	st.Sessions.Records["linked"] = &SessionRecord{Last: &SessionInvocation{DecisionID: "missing"}}
	require.ErrorContains(t, validateRuntimeDecisions(st), "scope")
}

func TestRuntimeExecutionSchemaRejectsInventedWireFacts(t *testing.T) {
	for _, change := range []func(*RuntimeDecisionRecord){
		func(r *RuntimeDecisionRecord) { r.Execution.ContractFingerprint = "not-a-fingerprint" },
		func(r *RuntimeDecisionRecord) { r.Execution.Invocation.Dispatch = "accepted" },
		func(r *RuntimeDecisionRecord) { r.Execution.Invocation.ID = "model-0" },
		func(r *RuntimeDecisionRecord) { r.Execution.Invocation.Attempt = 0 },
	} {
		st, _ := executionLinkFixture(t)
		change(st.RuntimeDecisions.Records[0])
		require.ErrorContains(t, validateRuntimeDecisions(st), "runtime decisions")
	}
}
