package machine

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestConsoleProjectionBoundsHundredsOfNodesWithoutChangingTruth(t *testing.T) {
	st := &state.State{Revision: 42, Status: "paused", Packets: map[string]*state.PacketState{}, ContractFingerprints: map[string]string{}, Inbox: []any{}}
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("M01-T%03d", i)
		st.ContractFingerprints[id] = "fingerprint"
		history := []any{}
		for j := 0; j < 30; j++ {
			history = append(history, map[string]any{"output": strings.Repeat("大", 400)})
		}
		st.Packets[id] = &state.PacketState{Status: "l1-failed", Attempts: 4, FailedAttempts: 2, EvidenceHistory: history, RetryHistory: history,
			Evidence: state.Evidence{L1: map[string]any{"pass": false, "identity": "candidate", "commands": history}, Review: map[string]any{"verdict": "return", "identity": "candidate", "reasons": history}}}
	}
	st.Packets["retired"] = &state.PacketState{}
	before, err := json.Marshal(st)
	require.NoError(t, err)
	require.Greater(t, len(before), 1<<20)
	compact, err := ConsoleProjection(st)
	require.NoError(t, err)
	after, err := json.Marshal(compact)
	require.NoError(t, err)
	require.Less(t, len(after), 1<<20)
	require.Equal(t, int64(42), compact.Revision)
	require.Len(t, compact.Packets, 200)
	packet := compact.Packets["M01-T001"]
	require.Equal(t, 4, packet.Attempts)
	require.False(t, packet.Evidence.L1.(map[string]any)["pass"].(bool))
	require.Len(t, st.Packets["M01-T001"].EvidenceHistory, 30)
	node, err := ReadNode(st, "M01-T001")
	require.NoError(t, err)
	require.Len(t, node.Packet.EvidenceHistory, 30)
	_, err = ReadNode(st, "missing")
	require.Error(t, err)
	_, err = ReadNode(nil, "missing")
	require.Error(t, err)
	value, err := ConsoleProjection(nil)
	require.NoError(t, err)
	require.Nil(t, value)
	st.Packets["null"] = nil
	_, err = ConsoleProjection(st)
	require.Error(t, err)
}

func TestDiagnosticExcerptsPreserveScalarsAndMarkOmissions(t *testing.T) {
	fields := map[string]any{}
	for i := 0; i < 40; i++ {
		fields[fmt.Sprintf("field%02d", i)] = strings.Repeat("中文", 400)
	}
	excerpt := compactDiagnostic(fields).(map[string]any)
	require.Equal(t, 8, excerpt["omitted-fields"])
	require.Contains(t, excerpt["field00"], "[truncated]")
	list := make([]any, 12)
	for i := range list {
		list[i] = "short"
	}
	result := compactDiagnostic(list).([]any)
	require.Len(t, result, 11)
	require.Equal(t, 2, result[0].(map[string]any)["omitted-items"])
	require.Equal(t, true, compactDiagnostic(true))
	require.Equal(t, "short", evidenceSummary("short", "unused"))
	require.Empty(t, historySummary(nil))
}
