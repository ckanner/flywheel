package diagnostic

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func qualifyDiagnosticTask(t *testing.T, task string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(task, "spec.md"))
	require.NoError(t, err)
	text := strings.Replace(string(raw), "spec-version: 1", `spec-version: 1
workflow:
  version: 1
  mode: auto
  reason: Existing local behavior has an executable regression check.
  scope-clear: true
  implementation-known: true
  verification-complete: true
  review-sufficient: true
  evidence: ["Existing implementation and its regression test were inspected."]
  risks: []
  not-applicable: {L2: No integration change., L3: No user interface change.}`, 1)
	text = strings.Replace(text, "milestone: M-01}", "milestone: M-01, verification: L1 regression}", 1)
	writeFixture(t, filepath.Join(task, "spec.md"), text)
}

func TestDoctorUsesWorkflowPolicyWithoutWritingOrExecuting(t *testing.T) {
	root, runtime := diagnosticFixture(t, dshConfiguration)
	task := taskFixture(t, root, "frontend")
	qualifyDiagnosticTask(t, task)
	report := Doctor(context.Background(), root, task, true)
	require.True(t, report.OK, "%+v", report.Checks)
	assert.Contains(t, checkByID(t, report, "workflow").Summary, "auto → quick")
	for _, id := range []string{"loops.plan", "loops.judge", "judge-panel"} {
		assert.Equal(t, "skipped", checkByID(t, report, id).Status)
	}
	assert.NoFileExists(t, filepath.Join(task, "state.json"))
	assert.NoFileExists(t, filepath.Join(root, "acceptance-must-not-run"))
	for _, probe := range runtime.probes {
		assert.True(t, probe.Doctor)
		assert.Empty(t, probe.Prompt)
	}

	spec, err := contract.LoadSpec(task)
	require.NoError(t, err)
	packets, err := contract.LoadPackets(task, spec)
	require.NoError(t, err)
	cfg := config.Default()
	st, err := state.Init("task", spec, packets, &cfg)
	require.NoError(t, err)
	_, err = state.Save(task, st, nil)
	require.NoError(t, err)
	before, err := os.ReadFile(state.StateFile(task))
	require.NoError(t, err)
	report = Doctor(context.Background(), root, task, false)
	require.True(t, report.OK, "%+v", report.Checks)
	assert.Contains(t, checkByID(t, report, "workflow").Summary, "full → full", "legacy state must not be reclassified by a newer Spec")
	after, err := os.ReadFile(state.StateFile(task))
	require.NoError(t, err)
	assert.Equal(t, before, after, "doctor is read-only")

	var corrupted map[string]any
	require.NoError(t, json.Unmarshal(before, &corrupted))
	corrupted["workflow"] = map[string]any{"version": 999}
	raw, err := json.Marshal(corrupted)
	require.NoError(t, err)
	writeFixture(t, state.StateFile(task), string(raw))
	runtime.probes = nil
	report = Doctor(context.Background(), root, task, true)
	assert.False(t, report.OK)
	assert.Equal(t, "fail", checkByID(t, report, "workflow").Status)
	assert.Empty(t, runtime.probes, "unknown policy cannot reach SDK probes")
	after, err = os.ReadFile(state.StateFile(task))
	require.NoError(t, err)
	assert.Equal(t, raw, after, "invalid policy must be retained, not repaired by inspection")
}
