package machine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestRuntimeAdmissionCASConflictDoesNotDispatchOrPublishIntent(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	server, calls := runtimeServer(t, func(map[string]string) string { return "repair" }, nil)
	enableRuntimeJEV(t, c, server.URL)
	packet := c.Packets["M01-T01"]
	progress := runtimeFailureFacts(t, c, packet)
	before, err := state.Load(c.TaskDir)
	require.NoError(t, err)
	before.Status = "paused"
	newer, err := state.Save(c.TaskDir, before, &before.Revision)
	require.NoError(t, err)
	written, err := os.ReadFile(state.StateFile(c.TaskDir))
	require.NoError(t, err)

	require.Empty(t, c.failureDecision(packet, errL1Failed, progress, false))
	var conflict *state.CasConflictError
	require.ErrorAs(t, c.err(), &conflict)
	require.Zero(t, calls.Load(), "a lost task reservation must never dispatch JEV")
	after, err := state.Load(c.TaskDir)
	require.NoError(t, err)
	require.Equal(t, newer.Revision, after.Revision)
	require.Equal(t, "paused", after.Status)
	require.Nil(t, after.RuntimeDecisions)
	require.Empty(t, after.Packets[packet.PacketID].PendingStrategy)
	current, err := os.ReadFile(state.StateFile(c.TaskDir))
	require.NoError(t, err)
	require.Equal(t, written, current, "failed CAS cannot replace another writer's state")
	require.NoDirExists(t, filepath.Join(c.TaskDir, ".investigation", "runtime-decisions"))
}

func TestRuntimeDecisionCopyInitializesMissingWorkerSessionLedger(t *testing.T) {
	_, owner, packet, _ := runtimeSafetyFixture(t)
	owner.retireSession("execute", packet.PacketID, "selected binding requires a cold start")
	worker := *owner
	worker.State = jsonClone(owner.State)
	worker.State.Sessions = nil
	worker.copyRuntimeDecisionState(packet, owner)
	require.NotNil(t, worker.State.Sessions)
	require.Equal(t, owner.State.Sessions.Version, worker.State.Sessions.Version)
	actual := worker.sessionRecord("execute", packet.PacketID)
	require.NotNil(t, actual)
	require.Equal(t, owner.sessionRecord("execute", packet.PacketID), actual)
	require.False(t, actual.ResumeCandidate)
	require.Equal(t, "selected binding requires a cold start", actual.Reason)
	actual.Reason = "worker-local edit"
	require.Equal(t, "selected binding requires a cold start", owner.sessionRecord("execute", packet.PacketID).Reason,
		"worker recovery copies cannot alias the authoritative session record")
}
