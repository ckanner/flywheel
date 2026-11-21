package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/stretchr/testify/require"
)

func TestPlanningCLIRejectedCollaborationNeverStartsAProcess(t *testing.T) {
	taskDir, cwd, st := planningCLIFixture(t, true)
	previous := controlExecutable
	t.Cleanup(func() { controlExecutable = previous })
	started := false
	controlExecutable = func() (string, error) { started = true; return "", errors.New("unexpected process start") }
	request := machine.PlanningCommand{Action: "collaboration", Revision: st.Revision + 1, RequestID: "stale-collaboration", Start: new(true),
		Collaboration: &machine.CollaborationEdit{Packet: "M01-T01", ContractFingerprint: strings.Repeat("a", 64), Plan: planning.Ref{Revision: 1, Digest: strings.Repeat("b", 64)},
			Value: contract.PacketCollaboration{Version: 1, Tasks: []contract.CollaborationTask{{Goal: "First task", Writes: []string{"src/a"}}, {Goal: "Second task", Writes: []string{"src/b"}}}}}}
	reply, err := planningCLIControl(t, taskDir, cwd, request)
	require.NoError(t, err, reply.Error)
	require.True(t, reply.OK)
	require.False(t, started)
	require.False(t, reply.Running)
	require.Equal(t, "rejected", reply.PlanningControl.Status)
	require.Equal(t, st.Planning, reply.State.Planning)
	read, err := planningCLIControl(t, taskDir, cwd, machine.PlanningCommand{Action: "status", RequestID: request.RequestID})
	require.NoError(t, err)
	require.Equal(t, reply.PlanningControl, read.PlanningControl)
	require.False(t, started)
}
