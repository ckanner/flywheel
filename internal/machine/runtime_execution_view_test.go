package machine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func executionViewFixture() (*Context, *state.RuntimeDecisionRecord, *state.SessionRecord) {
	fingerprint := strings.Repeat("a", 64)
	binding := state.SessionConfiguration{Harness: "antigravity", Channel: "google-subscription", Model: "gemini-pro-orchestrator-neon", Thinking: "", Permission: "danger-full-access", SessionMode: "scoped"}
	r := &state.RuntimeDecisionRecord{ID: "initial-M01-T01", Packet: "M01-T01", Action: "model-0", Status: "applied", ModelBindings: map[string]state.BindingSnapshot{"model-0": {BindingID: "native", Binding: binding}}, Execution: &state.RuntimeDecisionExecution{ContractFingerprint: fingerprint}}
	s := &state.SessionRecord{Role: "execute", Scope: r.Packet, Configuration: &binding, Last: &state.SessionInvocation{ID: "call-bound1", Attempt: 1, StartedAt: "2026-11-21T06:00:00Z", Status: "intent", Dispatch: "unknown", Outcome: "unknown"}}
	cfg := config.Default()
	c := &Context{Config: &cfg, State: &state.State{ContractFingerprints: map[string]string{r.Packet: fingerprint}, Packets: map[string]*state.PacketState{r.Packet: {ExecutionPolicy: &state.ExecutionPolicyRecord{Current: "native"}}},
		RuntimeDecisions: &state.RuntimeDecisions{Records: []*state.RuntimeDecisionRecord{r}}, Sessions: &state.SessionLedger{Records: map[string]*state.SessionRecord{sessionKey("execute", r.Packet): s}}}}
	return c, r, s
}

func TestExecutionDecisionLinkRequiresExactFrozenInputAndNextAttempt(t *testing.T) {
	c, r, s := executionViewFixture()
	require.Equal(t, r.Execution, runtimeExecutionCheckpoint(c.State, r.Packet))
	require.Nil(t, runtimeExecutionCheckpoint(c.State, "missing"))
	require.Equal(t, r.ID, c.executionDecisionForInvocation(s))
	for name, mutate := range map[string]func(*Context, *state.RuntimeDecisionRecord, *state.SessionRecord){
		"no ledger": func(c *Context, _ *state.RuntimeDecisionRecord, _ *state.SessionRecord) {
			c.State.RuntimeDecisions = nil
		},
		"review role": func(_ *Context, _ *state.RuntimeDecisionRecord, s *state.SessionRecord) { s.Role = "review" },
		"missing policy": func(c *Context, r *state.RuntimeDecisionRecord, _ *state.SessionRecord) {
			c.State.Packets[r.Packet].ExecutionPolicy = nil
		},
		"same configuration other candidate": func(c *Context, r *state.RuntimeDecisionRecord, _ *state.SessionRecord) {
			c.State.Packets[r.Packet].ExecutionPolicy.Current = "other-id"
		},
		"missing configuration":  func(_ *Context, _ *state.RuntimeDecisionRecord, s *state.SessionRecord) { s.Configuration = nil },
		"different packet":       func(_ *Context, _ *state.RuntimeDecisionRecord, s *state.SessionRecord) { s.Scope = "M01-T02" },
		"different next attempt": func(_ *Context, _ *state.RuntimeDecisionRecord, s *state.SessionRecord) { s.Last.Attempt = 2 },
		"different plan": func(c *Context, r *state.RuntimeDecisionRecord, _ *state.SessionRecord) {
			c.State.Packets[r.Packet].PlanRevisions++
		},
		"legacy decision": func(_ *Context, r *state.RuntimeDecisionRecord, _ *state.SessionRecord) { r.Execution = nil },
		"changed contract": func(c *Context, r *state.RuntimeDecisionRecord, _ *state.SessionRecord) {
			c.State.ContractFingerprints[r.Packet] = strings.Repeat("b", 64)
		},
		"pending choice": func(_ *Context, r *state.RuntimeDecisionRecord, _ *state.SessionRecord) { r.Status = "pending" },
		"different model": func(_ *Context, _ *state.RuntimeDecisionRecord, s *state.SessionRecord) {
			s.Configuration.Model = "another-model"
		},
		"different permission": func(_ *Context, _ *state.RuntimeDecisionRecord, s *state.SessionRecord) {
			s.Configuration.Permission = "read-only"
		},
		"different native mode": func(_ *Context, _ *state.RuntimeDecisionRecord, s *state.SessionRecord) {
			s.Configuration.NativeOptions = json.RawMessage(`{"version":1,"mode":"swarm"}`)
		},
		"different history": func(_ *Context, _ *state.RuntimeDecisionRecord, s *state.SessionRecord) {
			s.Configuration.SessionMode = "fresh"
		},
		"unbound action": func(_ *Context, r *state.RuntimeDecisionRecord, _ *state.SessionRecord) { r.Action = "model-1" },
		"nonmodel action": func(_ *Context, r *state.RuntimeDecisionRecord, _ *state.SessionRecord) {
			r.Action = "repair"
			r.ModelBindings["repair"] = r.ModelBindings["model-0"]
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, r, s := executionViewFixture()
			mutate(c, r, s)
			require.Empty(t, c.executionDecisionForInvocation(s))
		})
	}
}

func TestRuntimeExecutionViewKeepsExplicitCallEvidenceWithoutInventingHistory(t *testing.T) {
	c, r, s := executionViewFixture()
	require.Equal(t, "selected", runtimeDecisionExecutionView(c.State, r).Status)
	s.Last.DecisionID = r.ID
	for _, example := range []struct {
		dispatch, outcome, status string
		observed, cleanup         bool
		want                      string
	}{
		{"unknown", "unknown", "intent", false, false, "unknown"},
		{"unknown", "unknown", "intent", true, false, "started"},
		{"not-dispatched", "failed", "received", false, true, "not-started"},
		{"not-dispatched", "failed", "blocked", true, false, "unknown"},
		{"dispatched", "unknown", "blocked", true, false, "started"},
		{"dispatched", "completed", "received", true, false, "started"},
		{"dispatched", "completed", "received", true, true, "completed"},
		{"dispatched", "failed", "consumed", true, true, "failed"},
		{"dispatched", "cancelled", "consumed", true, true, "cancelled"},
	} {
		s.Last.Dispatch, s.Last.Outcome, s.Last.Status = example.dispatch, example.outcome, example.status
		s.Last.DispatchObserved, s.Last.CleanupConfirmed = example.observed, example.cleanup
		syncRuntimeDecisionExecutions(c.State)
		view := runtimeDecisionExecutionView(c.State, r)
		require.Equal(t, example.want, view.Status)
		require.Equal(t, s.Last.ID, view.Invocation.ID)
		view.Invocation.ID = "changed-display"
		require.Equal(t, "call-bound1", r.Execution.Invocation.ID)
	}
	// A newer call in the role slot does not replace this decision's evidence.
	c.State.Sessions.Records[sessionKey("execute", r.Packet)] = &state.SessionRecord{Last: &state.SessionInvocation{ID: "call-other", DecisionID: "other-decision"}}
	syncRuntimeDecisionExecutions(c.State)
	require.Equal(t, "cancelled", runtimeDecisionExecutionView(c.State, r).Status)
	before, err := json.Marshal(c.State)
	require.NoError(t, err)
	c.State.Inbox = []any{}
	view, err := ConsoleProjection(c.State)
	require.NoError(t, err)
	require.Nil(t, view.RuntimeDecisions.Records[0].Execution)
	require.Equal(t, "cancelled", view.RuntimeDecisions.Records[0].ExecutionView.Status)
	c.State.Inbox = nil
	after, err := json.Marshal(c.State)
	require.NoError(t, err)
	require.Equal(t, before, after)
	r.Execution = nil
	require.Equal(t, "unknown", runtimeDecisionExecutionView(c.State, r).Status, "same model or packet is not a historical association")
	r.Status = "pending"
	require.Equal(t, "not-selected", runtimeDecisionExecutionView(c.State, r).Status)
	r.Action = "repair"
	require.Nil(t, runtimeDecisionExecutionView(c.State, r))
}

func TestRuntimeExecutionWithoutLinkedInvocationDoesNotGuessAfterInputChanges(t *testing.T) {
	c, r, _ := executionViewFixture()
	c.State.RuntimeDecisions.Records = append(c.State.RuntimeDecisions.Records, nil, &state.RuntimeDecisionRecord{})
	syncRuntimeDecisionExecutions(c.State)
	require.Nil(t, r.Execution.Invocation)
	c.State.Sessions = nil
	syncRuntimeDecisionExecutions(c.State)
	require.Equal(t, "selected", runtimeDecisionExecutionView(c.State, r).Status)
	c.State.Packets[r.Packet].Attempts++
	require.Equal(t, "unknown", runtimeDecisionExecutionView(c.State, r).Status)
	c.State.Packets[r.Packet].Attempts = 0
	c.State.ContractFingerprints[r.Packet] = strings.Repeat("b", 64)
	require.Equal(t, "unknown", runtimeDecisionExecutionView(c.State, r).Status)
	c.State.RuntimeDecisions = nil
	syncRuntimeDecisionExecutions(c.State)
}
