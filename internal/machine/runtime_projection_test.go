package machine

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestRuntimeProjectionKeepsDecisionEffectsWithoutExposingPrivateInputs(t *testing.T) {
	st := &state.State{Revision: 42, Status: "paused", Packets: map[string]*state.PacketState{}, Inbox: []any{},
		RuntimeDecisions: &state.RuntimeDecisions{Version: 1, Used: 14, ImportedCalls: 2, Status: "enabled",
			Config: &config.JEVConfig{Enabled: true, Endpoint: "https://private-target.invalid/v1/systemone", KeyEnv: "PRIVATE_KEY_NAME", MaxCalls: 20}}}
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("failure-M01-T01-0-%d-1", i)
		st.RuntimeDecisions.Records = append(st.RuntimeDecisions.Records, &state.RuntimeDecisionRecord{
			ID: id, Packet: "M01-T01", Attempt: i, Status: "feedback-completed", Action: "feedback-0", Accounted: true,
			Snapshot:  &decision.Snapshot{ID: id, Facts: []decision.Fact{{Category: decision.TaskSummary, Text: "private task context"}}},
			Selection: &decision.Selection{ActionID: "feedback-0", Adopted: true, Reason: "approved-diagnostic-choice"},
			Receipt:   &decision.Receipt{Status: "unknown", Reason: "transport-outcome-unknown", Model: "jev-1.14.0", SnapshotHash: "private-snapshot-hash", TargetHash: "private-target-hash", DurationMS: 17},
			Feedback:  map[string]any{"pass": false, "diagnostic": strings.Repeat("诊", 700)},
		})
	}
	before, err := json.Marshal(st)
	require.NoError(t, err)
	view, err := ConsoleProjection(st)
	require.NoError(t, err)
	ledger := view.RuntimeDecisions
	require.Nil(t, ledger.Config)
	require.Equal(t, 20, ledger.MaxCalls)
	require.Equal(t, 2, ledger.Omitted)
	require.Equal(t, 14, ledger.Used)
	require.Equal(t, 2, ledger.ImportedCalls)
	require.Len(t, ledger.Records, 10)
	require.Equal(t, "failure-M01-T01-0-2-1", ledger.Records[0].ID)
	for _, record := range ledger.Records {
		require.Nil(t, record.Snapshot)
		require.Equal(t, "feedback-completed", record.Status)
		require.True(t, record.Selection.Adopted)
		require.Equal(t, "approved-diagnostic-choice", record.Selection.Reason)
		require.Equal(t, "unknown", record.Receipt.Status)
		require.Equal(t, int64(17), record.Receipt.DurationMS)
		require.Nil(t, record.Receipt.Usage.Input, "missing usage must not turn into zero usage")
		require.Nil(t, record.Receipt.Usage.Output)
		require.Empty(t, record.Receipt.SnapshotHash)
		require.Empty(t, record.Receipt.TargetHash)
		feedback := record.Feedback.(map[string]any)
		require.Equal(t, false, feedback["pass"])
		require.Equal(t, strings.Repeat("诊", 512)+"… [truncated]", feedback["diagnostic"])
	}
	after, err := json.Marshal(st)
	require.NoError(t, err)
	require.Equal(t, before, after, "a display projection cannot modify the authoritative ledger")
	public, err := json.Marshal(view)
	require.NoError(t, err)
	for _, private := range []string{"private-target.invalid", "PRIVATE_KEY_NAME", "private task context", "private-snapshot-hash", "private-target-hash"} {
		require.NotContains(t, string(public), private)
	}
}

func TestRuntimeProjectionHandlesPendingRecordsAndRejectsNullEffects(t *testing.T) {
	st := &state.State{Packets: map[string]*state.PacketState{}, Inbox: []any{},
		RuntimeDecisions: &state.RuntimeDecisions{Version: 1, Used: 1, Status: "enabled", MaxCalls: 5,
			Records: []*state.RuntimeDecisionRecord{{ID: "pending", Status: "pending"}}}}
	view, err := ConsoleProjection(st)
	require.NoError(t, err)
	require.Equal(t, 5, view.RuntimeDecisions.MaxCalls)
	require.Zero(t, view.RuntimeDecisions.Omitted)
	require.Nil(t, view.RuntimeDecisions.Records[0].Receipt)
	require.Nil(t, view.RuntimeDecisions.Records[0].Feedback)
	require.Equal(t, "pending", view.RuntimeDecisions.Records[0].Status)
	st.RuntimeDecisions.Records = append(st.RuntimeDecisions.Records, nil)
	view, err = ConsoleProjection(st)
	require.ErrorContains(t, err, "null runtime decision")
	require.Nil(t, view)
}
