package shadow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func contractFixture(t *testing.T) string {
	t.Helper()
	dir := privateDir(t)
	spec := map[string]any{
		"spec-version": 1, "title": "Empty list", "goal": "Fix the known empty-list branch",
		"milestones": []any{map[string]any{"id": "M-01", "title": "One fix"}},
		"requirements": []any{
			map[string]any{"id": "R-01", "title": "Empty arrays show the empty state", "milestone": "M-01", "verification": "Component regression"},
			map[string]any{"id": "R-02", "title": "Nonempty arrays remain unchanged", "milestone": "M-01", "verification": "Component regression"},
		},
		"workflow": map[string]any{"version": 1, "mode": "auto", "reason": "Known existing pattern", "scope-clear": true,
			"implementation-known": true, "verification-complete": true, "review-sufficient": true,
			"evidence": []string{"/Users/private-person/project/list.ts reviewed", "https://private.example/review inspected"}, "risks": []string{},
			"not-applicable": map[string]string{"L2": "Component only", "L3": "Covered by component test"}},
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "spec.md"), []byte("---\n"+string(jsonBytes(spec))+"\n---\nprivate body not transmitted"), 0o600))
	packet, err := templates.FS.ReadFile("task-packet.example.md")
	require.NoError(t, err)
	packet = []byte(strings.Replace(string(packet), "Implement the auth token parser so that `parseToken` returns claims for valid JWTs and null otherwise.", "Fix the empty-list branch using the existing component.", 1))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "task-packet-M01-T01.md"), packet, 0o600))
	return dir
}

func TestPrepareUsesRealSelectorWithoutChangingTask(t *testing.T) {
	dir := contractFixture(t)
	before, err := os.ReadFile(filepath.Join(dir, "spec.md"))
	require.NoError(t, err)
	dataset, err := Prepare(dir, "auto")
	require.NoError(t, err)
	assert.Equal(t, "quick", dataset.Cases[0].Baseline.Mode)
	assert.Equal(t, "prepared", dataset.Cases[0].Source.Kind)
	assert.Empty(t, dataset.Cases[0].HardBlockers)
	assert.Nil(t, dataset.Cases[0].Label)
	assert.NotContains(t, string(jsonBytes(dataset)), "private-person")
	assert.NotContains(t, string(jsonBytes(dataset)), "private.example")
	assert.NotContains(t, string(jsonBytes(dataset)), "private body")
	after, err := os.ReadFile(filepath.Join(dir, "spec.md"))
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.NoFileExists(t, state.StateFile(dir))
	alias := filepath.Join(privateDir(t), "alias")
	require.NoError(t, os.Symlink(dir, alias))
	fromAlias, err := Prepare(alias, "auto")
	require.NoError(t, err)
	assert.Equal(t, dataset, fromAlias)
	full, err := Prepare(dir, "full")
	require.NoError(t, err)
	assert.Equal(t, "full", full.Cases[0].Baseline.Mode)
	assert.NotEmpty(t, full.Cases[0].HardBlockers)
	_, err = Prepare(dir, "fast")
	require.Error(t, err)

	spec, err := contract.LoadSpec(dir)
	require.NoError(t, err)
	packets, err := contract.LoadPackets(dir, spec)
	require.NoError(t, err)
	st, err := state.Init("fixture", spec, packets, nil)
	require.NoError(t, err)
	st.Status = "paused"
	st, err = state.Save(dir, st, nil)
	require.NoError(t, err)
	stateBefore, err := os.ReadFile(state.StateFile(dir))
	require.NoError(t, err)
	legacy, err := Prepare(dir, "")
	require.NoError(t, err)
	assert.Equal(t, "replay", legacy.Cases[0].Source.Kind)
	assert.Equal(t, "full", legacy.Cases[0].Baseline.Mode)
	assert.Contains(t, legacy.Cases[0].HardBlockers[0], "Persisted")
	_, err = Prepare(dir, "full")
	require.ErrorContains(t, err, "persisted mode")
	stateAfter, err := os.ReadFile(state.StateFile(dir))
	require.NoError(t, err)
	assert.Equal(t, stateBefore, stateAfter)
	selection, err := contract.SelectWorkflow(spec, packets, "auto")
	require.NoError(t, err)
	st.Workflow = &state.WorkflowState{WorkflowSelection: selection, SelectedAt: time.Now().UTC().Format(time.RFC3339), Upgrades: []state.WorkflowUpgrade{}}
	st, err = state.Save(dir, st, nil)
	require.NoError(t, err)
	replay, err := Prepare(dir, "")
	require.NoError(t, err)
	assert.Equal(t, "quick", replay.Cases[0].Baseline.Mode)
	assert.Empty(t, replay.Cases[0].HardBlockers)
	st.Status = "running"
	_, err = state.Save(dir, st, nil)
	require.NoError(t, err)
	_, err = Prepare(dir, "")
	require.ErrorContains(t, err, "quiescent")
}

func TestCaptureFailuresAndSnapshotDrift(t *testing.T) {
	_, err := Prepare("/missing-shadow-task", "auto")
	require.Error(t, err)
	_, err = Prepare(privateDir(t), "auto")
	require.Error(t, err)
	dir := contractFixture(t)
	require.NoError(t, os.Remove(filepath.Join(dir, "task-packet-M01-T01.md")))
	_, err = Prepare(dir, "auto")
	require.Error(t, err)
	dir = contractFixture(t)
	require.NoError(t, os.WriteFile(state.StateFile(dir), []byte("{"), 0o600))
	_, err = Prepare(dir, "auto")
	require.Error(t, err)
	require.NoError(t, os.Remove(state.StateFile(dir)))
	require.NoError(t, os.Symlink("state.json", state.StateFile(dir)))
	_, err = Prepare(dir, "auto")
	require.Error(t, err, "metadata errors must not imply an absent state")
	dir = contractFixture(t)
	base, err := readSnapshot(dir)
	require.NoError(t, err)
	original := snapshotRead
	t.Cleanup(func() { snapshotRead = original })
	calls := 0
	snapshotRead = func(string) (snapshot, error) {
		calls++
		if calls == 2 {
			return snapshot{}, errors.New("second read failed")
		}
		return base, nil
	}
	_, err = Prepare(dir, "auto")
	require.ErrorContains(t, err, "second read")
	calls = 0
	snapshotRead = func(string) (snapshot, error) {
		copy := base
		copy.Revision = int64(calls)
		calls++
		return copy, nil
	}
	_, err = Prepare(dir, "auto")
	require.ErrorContains(t, err, "changed")
	snapshotRead = original
	abs := absolutePath
	absolutePath = func(string) (string, error) { return "", errors.New("cwd unavailable") }
	t.Cleanup(func() { absolutePath = abs })
	_, err = Prepare(dir, "auto")
	require.ErrorContains(t, err, "cwd unavailable")
}

func TestMissingAssessmentStaysFullAndSecretCaptureFails(t *testing.T) {
	dir := contractFixture(t)
	s, err := readSnapshot(dir)
	require.NoError(t, err)
	s.Spec.Workflow = nil
	assert.NotEmpty(t, structuralBlockers(s, "auto", "full"))
	raw, err := os.ReadFile(filepath.Join(dir, "spec.md"))
	require.NoError(t, err)
	raw = []byte(strings.Replace(string(raw), "Fix the known empty-list branch", "Bearer a-secret-looking-value-0123456789", 1))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "spec.md"), raw, 0o600))
	_, err = Prepare(dir, "auto")
	require.ErrorContains(t, err, "possible credential")
}
