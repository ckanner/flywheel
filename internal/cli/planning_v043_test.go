package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControlPlanningCancelPlanAndContinueExecutionCLIContract(t *testing.T) {
	// readPlanningCommand accepts cancel-plan and plan with continue-execution, and rejects cross-action leakage.
	cmd, err := readPlanningCommand(strings.NewReader(`{"action":"cancel-plan","revision":2,"request-id":"cancel-1","target-request-id":"req-bad","start":false}`))
	require.NoError(t, err)
	assert.Equal(t, "cancel-plan", cmd.Action)
	assert.Equal(t, "req-bad", cmd.TargetRequestID)

	cmd, err = readPlanningCommand(strings.NewReader(`{"action":"plan","revision":2,"request-id":"m02-1","scope":"milestone","scope-id":"M-02","continue-execution":true,"start":true}`))
	require.NoError(t, err)
	assert.Equal(t, "plan", cmd.Action)
	assert.True(t, cmd.ContinueExecution)

	_, err = readPlanningCommand(strings.NewReader(`{"action":"cancel-plan","revision":2,"request-id":"c2","continue-execution":true}`))
	require.ErrorContains(t, err, "is not valid")

	// End-to-end control planning for cancel-plan and continue-execution omits --planning-only on startControlledRun.
	taskDir, repo, st := planningCLIFixture(t, true)
	approveReq := planningCLIApproval(st, "approve-v043", true)
	planningCLIProof(t, approveReq)
	argsFile := filepath.Join(t.TempDir(), "captured-args.txt")
	fakeBin := filepath.Join(t.TempDir(), "fake-flywheel.sh")
	require.NoError(t, os.WriteFile(fakeBin, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \""+argsFile+"\"\n"), 0o755))
	origExec := controlExecutable
	t.Cleanup(func() { controlExecutable = origExec })
	controlExecutable = func() (string, error) { return fakeBin, nil }

	_, err = planningCLIControl(t, taskDir, repo, approveReq)
	require.NoError(t, err)

	st, err = state.Load(taskDir)
	require.NoError(t, err)
	st.Planning.Phase = "drafting"
	st.Planning.Requests = append(st.Planning.Requests, planning.Request{
		ID: "req-mistake", Scope: "design", InputRevision: st.Planning.InputRevision, Rounds: 1, MaxRounds: 3, Status: "running", PlanningOnly: true,
	})
	st.Planning.Rounds++
	st.Milestones = map[string]state.MilestoneState{"M-01": {Status: "in-progress"}}
	st, err = state.Save(taskDir, st, &st.Revision)
	require.NoError(t, err)

	// 1. cancel-plan via CLI control planning succeeds without starting a run.
	resp, err := planningCLIControl(t, taskDir, repo, machine.PlanningCommand{
		Action:          "cancel-plan",
		Revision:        st.Revision,
		RequestID:       "cancel-cli-1",
		TargetRequestID: "req-mistake",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.PlanningControl)
	assert.Equal(t, "applied", resp.PlanningControl.Status)

	reloaded, err := state.Load(taskDir)
	require.NoError(t, err)
	assert.Equal(t, "ready", reloaded.Planning.Phase)

	// 2. plan with continue-execution: true omits --planning-only when invoking startControlledRun.
	_ = os.Remove(argsFile)
	resp, err = planningCLIControl(t, taskDir, repo, machine.PlanningCommand{
		Action:            "plan",
		Revision:          reloaded.Revision,
		RequestID:         "m01-continue-1",
		Scope:             "milestone",
		ScopeID:           "M-01",
		ContinueExecution: true,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.PlanningControl)
	assert.Equal(t, "applied", resp.PlanningControl.Status)

	require.Eventually(t, func() bool {
		raw, readErr := os.ReadFile(argsFile)
		return readErr == nil && strings.Contains(string(raw), "run")
	}, 5*time.Second, 20*time.Millisecond)
	rawArgs, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	assert.Contains(t, string(rawArgs), "run")
	assert.NotContains(t, string(rawArgs), "--planning-only")
}
