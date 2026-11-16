package machine

import (
	"context"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestManualCancellationBeforeDispatchLeavesConsumedPendingHandoff(t *testing.T) {
	fx, c := handoffFixture(t)
	_, err := c.runSessionCommand(sessionRequest("claim"))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.RunContext = ctx
	c.Log = func(message string) {
		if strings.HasPrefix(message, "session execute/M01-T01: ") {
			cancel() // The session intent is durable; the adapter has not been called.
		}
	}
	consoleInvoke = func(drivers.CallArgs, string) drivers.Result {
		t.Fatal("a locally cancelled manual invocation must not dispatch")
		return drivers.Result{}
	}
	_, err = c.runSessionCommand(sessionRequest("send"))
	require.Error(t, err)
	before, err := state.Load(fx.taskDir)
	require.NoError(t, err)
	key := "execute/M01-T01"
	require.Equal(t, "consumed", before.Sessions.Records[key].Last.Status)
	require.Equal(t, "not-dispatched", before.Sessions.Records[key].Last.Dispatch)
	require.True(t, before.Handoff.Pending)

	request := sessionRequest("reconcile")
	request.Revision = before.Revision
	summary, err := RunSingleStep(RunSingleStepOpts{
		RunOpts: RunOpts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config, Drivers: happyDrivers().set()},
		Only:    "session", Session: &request, ExpectedRevision: &request.Revision,
	})
	require.NoError(t, err)
	require.Equal(t, "session-reconcile", summary.Step.Name)
	after, err := state.Load(fx.taskDir)
	require.NoError(t, err)
	require.Equal(t, "consumed", after.Sessions.Records[key].Last.Status)
	require.Equal(t, before.Sessions.Records[key].Last.ID, after.Sessions.Records[key].Last.ID)
	require.False(t, after.Handoff.Pending)
	require.Equal(t, before.Budget.Totals, after.Budget.Totals)
}
