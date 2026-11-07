package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControlWorkflowModeValidationHasNoSideEffects(t *testing.T) {
	for _, tc := range []struct{ action, mode string }{
		{"launch", "fast"}, {"start", "Quick"}, {"launch", ""},
		{"status", "quick"}, {"stop", "full"}, {"config", "auto"}, {"doctor", "auto"},
	} {
		t.Run(tc.action+"-"+tc.mode, func(t *testing.T) {
			taskDir, cwd := t.TempDir(), t.TempDir()
			code, result := runControlTest(t, tc.action, taskDir, cwd, "--mode", tc.mode)
			require.Equal(t, 1, code)
			assert.False(t, result.OK)
			assert.Contains(t, result.Error, "--mode")
			files, err := os.ReadDir(taskDir)
			require.NoError(t, err)
			assert.Empty(t, files, "invalid mode cannot create a run, log, or state")
		})
	}
	for _, mode := range []string{"", "fast", "quick; touch /tmp/unexpected"} {
		code, out := runCLI(t, "run", t.TempDir(), "--mode", mode)
		assert.Equal(t, 1, code)
		assert.Contains(t, out, "--mode only supports")
	}
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), `{"revision":0,"workflow":{"version":99}}`)
	code, result := runControlTest(t, "launch", taskDir, t.TempDir())
	assert.Equal(t, 1, code)
	assert.False(t, result.OK)
	assert.Contains(t, result.Error, "workflow")
	assert.NoFileExists(t, filepath.Join(taskDir, "orchestrator.log"), "corrupt durable state cannot admit a new run")
}

func TestControlLaunchPassesValidatedModeWithoutShell(t *testing.T) {
	for _, tc := range []struct {
		action string
		flags  []string
		want   string
	}{
		{"launch", nil, "auto"},
		{"launch", []string{"--mode", "quick"}, "quick"},
		{"launch", []string{"--mode", "full"}, "full"},
		{"start", []string{"--mode", "auto"}, "auto"},
		{"start", nil, ""},
	} {
		t.Run(tc.action+"-"+tc.want, func(t *testing.T) {
			controlFixture(t, "completed")
			taskDir, cwd := t.TempDir(), t.TempDir()
			code, result := runControlTest(t, tc.action, taskDir, cwd, tc.flags...)
			require.Zero(t, code, "%+v", result)
			physicalCwd, err := filepath.EvalSymlinks(cwd)
			require.NoError(t, err)
			assert.Equal(t, physicalCwd, result.Cwd)
			raw, err := os.ReadFile(filepath.Join(taskDir, "fixture-args.json"))
			require.NoError(t, err)
			var actual []string
			require.NoError(t, json.Unmarshal(raw, &actual))
			want := []string{"run", taskDir}
			if tc.want != "" {
				want = append(want, "--mode", tc.want)
			}
			assert.Equal(t, want, actual)
		})
	}
}

func TestControlDoesNotChangeRunningWorkflow(t *testing.T) {
	taskDir, cwd := t.TempDir(), t.TempDir()
	release, err := machine.AcquireRunLease(taskDir)
	require.NoError(t, err)
	defer release()
	code, result := runControlTest(t, "launch", taskDir, cwd, "--mode", "quick")
	require.Equal(t, 1, code)
	assert.True(t, result.Running)
	assert.Contains(t, result.Error, "already starting")
	writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
	code, result = runControlTest(t, "start", taskDir, cwd, "--mode", "quick")
	require.Equal(t, 1, code)
	assert.Contains(t, result.Error, "already running with full")
	assert.Equal(t, int64(3), result.State.Revision)
	code, result = runControlTest(t, "start", taskDir, cwd, "--mode", "full")
	require.Zero(t, code)
	assert.True(t, result.Running)
	code, result = runControlTest(t, "launch", taskDir, cwd)
	require.Zero(t, code)
	assert.Equal(t, int64(3), result.State.Revision, "idempotent resume does not edit state")
	assert.NoFileExists(t, filepath.Join(taskDir, "orchestrator.log"))
	st, err := state.Load(taskDir)
	require.NoError(t, err)
	st.Workflow = &state.WorkflowState{
		WorkflowSelection: contract.WorkflowSelection{Version: 1, Requested: "auto", Effective: "quick",
			Reason: "Localized verified repair", Evidence: []string{"Existing regression test"}, Risks: []string{}, Stages: contract.WorkflowStages("quick")},
		SelectedAt: state.NowISO(), Upgrades: []state.WorkflowUpgrade{},
	}
	_, err = state.Save(taskDir, st, nil)
	require.NoError(t, err)
	code, result = runControlTest(t, "launch", taskDir, cwd, "--mode", "quick")
	require.Zero(t, code)
	assert.Equal(t, "quick", result.State.Workflow.Effective)
	code, result = runControlTest(t, "launch", taskDir, cwd, "--mode", "full")
	assert.Equal(t, 1, code)
	assert.Contains(t, result.Error, "already running with quick")
}

func TestStatusExplainsQuickNonApplicableStagesAndUpgrade(t *testing.T) {
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
	st, err := state.Load(taskDir)
	require.NoError(t, err)
	st.Workflow = &state.WorkflowState{
		WorkflowSelection: contract.WorkflowSelection{Version: 1, Requested: "auto", Effective: "quick",
			Reason: "Localized verified repair", Evidence: []string{"Existing regression test"}, Risks: []string{}, Stages: contract.WorkflowStages("quick")},
		SelectedAt: state.NowISO(), Upgrades: []state.WorkflowUpgrade{},
	}
	st, err = state.Save(taskDir, st, nil)
	require.NoError(t, err)
	code, out := runCLI(t, "status", taskDir)
	require.Zero(t, code)
	assert.Contains(t, out, "auto → quick")
	assert.Contains(t, out, "plan 不适用")
	assert.Contains(t, out, "judge 不适用")
	st.Workflow.Effective = "full"
	st.Workflow.Stages = contract.WorkflowStages("full")
	st.Workflow.Upgrades = []state.WorkflowUpgrade{{At: state.NowISO(), From: "quick", To: "full", Reason: "Integration coverage gap", Evidence: []string{}, ContractFingerprints: map[string]string{}}}
	_, err = state.Save(taskDir, st, nil)
	require.NoError(t, err)
	code, out = runCLI(t, "status", taskDir)
	require.Zero(t, code)
	assert.Contains(t, out, "auto → full")
	assert.Contains(t, out, "流程升级 quick → full: Integration coverage gap")
	assert.NotContains(t, out, "不适用")
}

func TestControlLaunchAutoCompletesWithConservativeFullPolicy(t *testing.T) {
	controlFixture(t, "real-run")
	taskDir := newCLISandbox(t)
	cwd := filepath.Dir(filepath.Dir(taskDir))
	code, started := runControlTest(t, "launch", taskDir, cwd)
	require.Zero(t, code, "%+v", started)
	require.Eventually(t, func() bool {
		r, err := controlSnapshot(taskDir)
		return err == nil && !r.Running && r.State != nil && r.State.Status == "completed"
	}, 10*time.Second, 10*time.Millisecond)
	st, err := state.Load(taskDir)
	require.NoError(t, err)
	require.NotNil(t, st.Workflow)
	assert.Equal(t, "auto", st.Workflow.Requested)
	assert.Equal(t, "full", st.Workflow.Effective, "no qualification evidence cannot select Quick")
	assert.NotEmpty(t, st.Workflow.Reason)
}
