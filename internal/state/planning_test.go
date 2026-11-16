package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/stretchr/testify/require"
)

func planningBrief() planning.Brief {
	return planning.Brief{Version: 1, ID: "design-task", Goal: "Design durable execution", OriginalRequest: "Review the design before implementation", Constraints: []string{}, Evidence: []string{}, Policy: "review-first"}
}

func savePlanningFixture(t *testing.T) (string, *State) {
	t.Helper()
	dir := t.TempDir()
	s, err := InitPlanning(planningBrief(), nil)
	require.NoError(t, err)
	s, err = Save(dir, s, nil)
	require.NoError(t, err)
	return dir, s
}

func TestPlanningInitHasNoFabricatedImplementationContracts(t *testing.T) {
	defaults := config.Default()
	for _, cfg := range []*config.Config{nil, {}, &defaults} {
		s, err := InitPlanning(planningBrief(), cfg)
		require.NoError(t, err)
		require.Equal(t, 2, s.SchemaVersion)
		require.Equal(t, 4, s.Sessions.Version)
		require.NotNil(t, s.Sessions.Records)
		require.Empty(t, s.Packets)
		require.Empty(t, s.Milestones)
		require.Nil(t, s.Workflow)
		require.Nil(t, s.CurrentMilestone)
		raw, err := json.Marshal(s)
		require.NoError(t, err)
		var value any
		require.NoError(t, json.Unmarshal(raw, &value))
		require.NoError(t, contract.ValidateStateValue(value))
	}
	_, err := InitPlanning(planning.Brief{}, nil)
	require.Error(t, err)
}

func TestPlanningStateRoundTripAndCASRetainAuthority(t *testing.T) {
	dir, saved := savePlanningFixture(t)
	loaded, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, saved.Planning, loaded.Planning)
	_, err = os.Stat(filepath.Join(dir, "spec.md"))
	require.True(t, os.IsNotExist(err), "brief-only state must load before Spec exists")
	old := *loaded
	old.Planning = planning.Clone(loaded.Planning)
	require.NoError(t, planning.ApplyFeedback(loaded.Planning, planning.Feedback{RequestID: "feedback-1", Text: "Preserve the existing API", Source: "verified-ui-request", At: "now"}))
	loaded, err = Save(dir, loaded, nil)
	require.NoError(t, err)
	require.Equal(t, 2, loaded.Planning.InputRevision)
	_, err = Save(dir, &old, nil)
	require.IsType(t, &CasConflictError{}, err)
	old.Revision = loaded.Revision
	_, err = Save(dir, &old, nil)
	require.ErrorContains(t, err, "cannot be rewritten")
	final, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, 2, final.Planning.InputRevision)
	final.Planning.Brief.Goal = "Replace past user intent"
	_, err = Save(dir, final, nil)
	require.ErrorContains(t, err, "cannot be rewritten")
}

func TestPlanningLoadAndSaveRejectMissingDowngradeFence(t *testing.T) {
	for name, edit := range map[string]func(*State){
		"missing ledger":     func(s *State) { s.Sessions = nil },
		"old ledger":         func(s *State) { s.Sessions.Version = 3 },
		"different identity": func(s *State) { s.Slug = "another-task" },
		"planning removed":   func(s *State) { s.Planning = nil },
		"malformed planning": func(s *State) { s.Planning.Phase = "invented-pass" },
	} {
		t.Run(name, func(t *testing.T) {
			dir, s := savePlanningFixture(t)
			edit(s)
			_, err := Save(dir, s, nil)
			require.Error(t, err)
			raw, err := json.Marshal(s)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(StateFile(dir), raw, 0o600))
			_, err = Load(dir)
			require.Error(t, err)
		})
	}
}

func TestPlanningSaveCannotEraseNewPartitionFromOldStateClone(t *testing.T) {
	dir, s := savePlanningFixture(t)
	s.Planning, s.Sessions = nil, nil
	_, err := Save(dir, s, nil)
	require.ErrorContains(t, err, "cannot be removed")
	loaded, err := Load(dir)
	require.NoError(t, err)
	require.NotNil(t, loaded.Planning)
	require.Equal(t, int64(1), loaded.Revision)
}

func TestPlanningUnknownFieldsAreRejectedBeforeTypedConversion(t *testing.T) {
	dir, saved := savePlanningFixture(t)
	raw, err := os.ReadFile(StateFile(dir))
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, json.Unmarshal(raw, &value))
	value["planning"].(map[string]any)["pretend-approved"] = true
	raw, err = json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(StateFile(dir), raw, 0o600))
	_, err = Load(dir)
	require.Error(t, err)
	_, err = Save(dir, saved, nil)
	require.Error(t, err, "even a matching CAS must not discard a newer unknown authority")
}

func TestPlanningPrewarmLoadAndCASKeepNewerLedgerFence(t *testing.T) {
	dir, s := savePlanningFixture(t)
	packet, err := EmptyPacketState(&contract.Packet{PacketID: "M01-T01", Route: "default"})
	require.NoError(t, err)
	s.Packets["M01-T01"] = packet
	s.WorkerPrewarm = &WorkerPrewarm{Version: 1, ID: "warm-0001", Scope: "M01-T01", Binding: strings.Repeat("a", 64), StartedAt: "now"}
	s, err = Save(dir, s, nil)
	require.NoError(t, err)
	reloaded, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, s.WorkerPrewarm, reloaded.WorkerPrewarm)
	require.Equal(t, 4, reloaded.Sessions.Version)
	reloaded.WorkerPrewarm = nil
	_, err = Save(dir, reloaded, nil)
	require.NoError(t, err)
	reloaded, err = Load(dir)
	require.NoError(t, err)
	require.Nil(t, reloaded.WorkerPrewarm)
	require.Equal(t, 4, reloaded.Sessions.Version)
	require.NotNil(t, reloaded.Planning)
}

func TestReviewedPlanningMayImportReportOnlyProvenanceWithoutGrantingSourceScope(t *testing.T) {
	dir, s := savePlanningFixture(t)
	s.PreparationSource = &PreparationSource{Version: 1, ID: "investigation", RunID: "investigation-run", ContractHash: "original-intent",
		ReportHash: "supported-report", ImplementationScope: []string{}, Accounted: true}
	_, err := Save(dir, s, nil)
	require.NoError(t, err)
	loaded, err := Load(dir)
	require.NoError(t, err)
	require.Empty(t, loaded.PreparationSource.ImplementationScope)
	require.Empty(t, loaded.Planning.Approvals)
	require.False(t, loaded.Planning.StartAuthorized)
	raw, err := json.Marshal(loaded)
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, json.Unmarshal(raw, &value))
	require.NoError(t, contract.ValidateStateValue(value))
	delete(value, "planning")
	require.Error(t, contract.ValidateStateValue(value), "the full public schema also rejects legacy empty source scope")

	// Removing planning cannot reinterpret an investigation-only source as
	// legacy repair authority, even if its old ledger fence is also removed.
	loaded.Planning, loaded.Sessions = nil, nil
	_, err = Save(t.TempDir(), loaded, nil)
	require.ErrorContains(t, err, "without implementation scope")
	raw, err = json.Marshal(loaded)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(StateFile(dir), raw, 0600))
	_, err = Load(dir)
	require.ErrorContains(t, err, "without implementation scope")
}

func TestReviewedBriefQualifiesWorkflowOnceAndMigratedLegacyRemainsFull(t *testing.T) {
	spec, packets, s := workflowFixture(t)
	brief := planningBrief()
	brief.Mode = "quick"
	p, err := planning.New(brief)
	require.NoError(t, err)
	s.Planning = p
	for _, mode := range []string{"", "auto", "quick"} {
		selection, err := ResolveWorkflow(s, spec, packets, mode)
		require.NoError(t, err)
		require.Equal(t, "quick", selection.Effective)
		require.Nil(t, s.Workflow, "selection still requires the owning engine's CAS")
	}
	s.Planning.Brief.Mode = "full"
	selection, err := ResolveWorkflow(s, spec, packets, "")
	require.NoError(t, err)
	require.Equal(t, "full", selection.Effective)
	selection, err = ResolveWorkflow(s, spec, packets, "quick")
	require.NoError(t, err)
	require.Equal(t, "quick", selection.Effective, "an explicit mode selects a still-unclassified new task")
	s.Planning.Brief.Mode = "quick"
	s.Planning.LegacyFiles = map[string]string{"spec.md": planning.DigestBytes([]byte("original legacy contract"))}
	selection, err = ResolveWorkflow(s, spec, packets, "")
	require.NoError(t, err)
	require.Equal(t, "full", selection.Effective)
	_, err = ResolveWorkflow(s, spec, packets, "quick")
	require.ErrorContains(t, err, "legacy Full")
}
