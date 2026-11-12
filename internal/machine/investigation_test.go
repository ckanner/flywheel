package machine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/investigation"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestManagedInvestigationOwnsLifetimeAndCollectsOnlyReadOnlyEvidence(t *testing.T) {
	fx, p := managedPreparationFixture(t, false)
	start := startManagedPreparation(t, fx, p, investigation.Options{})
	before, err := os.ReadFile(filepath.Join(fx.root, "settings.json"))
	require.NoError(t, err)
	st, err := RunInvestigation(nil, fx.taskDir, start.Revision)
	require.NoError(t, err)
	require.Equal(t, "awaiting-report", st.Status)
	require.Len(t, st.Observations, 3)
	require.False(t, state.Exists(fx.taskDir))
	after, err := os.ReadFile(filepath.Join(fx.root, "settings.json"))
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoFileExists(t, PidFile(fx.taskDir))
	held, err := RunLeaseHeld(fx.taskDir)
	require.NoError(t, err)
	require.False(t, held)
	_, err = RunInvestigation(context.Background(), fx.taskDir, start.Revision)
	require.ErrorContains(t, err, "stale preparation revision")
	stored, err := state.LoadPreparation(fx.taskDir)
	require.NoError(t, err)
	require.Equal(t, st.ActionsUsed, stored.ActionsUsed)
}

func TestManagedInvestigationCancelsAndPreservesUncertainAction(t *testing.T) {
	fx, p := managedPreparationFixture(t, false)
	st := startManagedPreparation(t, fx, p, investigation.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := RunInvestigation(ctx, fx.taskDir, st.Revision)
	require.NoError(t, err)
	require.Equal(t, "cancelled", result.Status)
	require.Zero(t, result.ActionsUsed)

	fx, p = managedPreparationFixture(t, false)
	st = startManagedPreparation(t, fx, p, investigation.Options{})
	action := p.Actions[0]
	st.Pending = &state.PreparationInvocation{ID: state.PreparationHash("lost-observation"), ActionID: action.ID,
		BindingHash: state.PreparationHash(action), CandidateHash: state.PreparationHash("candidate"), StartedAt: st.StartedAt, Reason: "dispatch had no trustworthy receipt"}
	st.Status, st.ActionsUsed = "running", 1
	st, err = state.SavePreparation(fx.taskDir, st)
	require.NoError(t, err)
	result, err = RunInvestigation(context.Background(), fx.taskDir, st.Revision)
	require.NoError(t, err)
	require.Equal(t, "blocked", result.Status)
	require.Equal(t, st.Pending, result.Pending)
	require.Equal(t, 1, result.ActionsUsed)
	require.Empty(t, result.Observations, "unknown dispatch must not execute a fresh copy")
	require.Contains(t, result.Reason, "action-outcome-unknown")
	require.False(t, state.Exists(fx.taskDir))
}

func TestManagedInvestigationRefusesUnownedOrUnrecordableRun(t *testing.T) {
	for _, fault := range []string{"lease", "load", "phase", "pid"} {
		t.Run(fault, func(t *testing.T) {
			fx, p := managedPreparationFixture(t, false)
			st := startManagedPreparation(t, fx, p, investigation.Options{})
			switch fault {
			case "lease":
				release, err := AcquireRunLease(fx.taskDir)
				require.NoError(t, err)
				defer release()
			case "load":
				writeFile(t, state.PreparationFile(fx.taskDir), "invalid JSON")
			case "phase":
				require.NoError(t, os.Mkdir(filepath.Join(fx.taskDir, "metrics.jsonl"), 0o700))
			case "pid":
				writeFile(t, filepath.Join(fx.taskDir, ".control"), "not a runtime directory")
			}
			_, err := RunInvestigation(context.Background(), fx.taskDir, st.Revision)
			require.Error(t, err)
			require.False(t, state.Exists(fx.taskDir))
			if fault != "load" {
				current, err := state.LoadPreparation(fx.taskDir)
				require.NoError(t, err)
				require.Zero(t, current.ActionsUsed, "admission failure cannot spend an action")
			}
		})
	}
}
