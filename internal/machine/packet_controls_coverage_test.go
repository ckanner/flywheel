package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPacketControlRejectsDifferentPacketIdentityBeforeExposingControls(t *testing.T) {
	fx, c := collaborationProjectionFixture(t, true)
	path := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
	raw := strings.ReplaceAll(string(mustReadFile(t, path)), "M01-T01", "M01-T02")
	writeFile(t, path, raw)
	before := jsonClone(c.State)
	ctx, packet, fingerprint, err := packetControlContext(c.State, "M01-T01", []string{fx.taskDir, fx.root})
	require.ErrorContains(t, err, "identity differs")
	require.Nil(t, ctx)
	require.Nil(t, packet)
	require.Empty(t, fingerprint)
	view, editor := packetControlViews(c.State, "M01-T01", []string{fx.taskDir, fx.root})
	require.Equal(t, "unknown", view.Status)
	require.Empty(t, view.Candidates)
	require.False(t, editor.Editable)
	require.Equal(t, before, c.State)
}

func TestPacketControlPreviewRejectsDamagedPlanningEvidenceWithoutRepairingIt(t *testing.T) {
	for _, scenario := range []string{"other-packet-changed", "missing-export-directory", "changed-export"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c := collaborationProjectionFixture(t, true)
			exports := filepath.Join(fx.taskDir, "planning")
			switch scenario {
			case "other-packet-changed":
				path := filepath.Join(fx.taskDir, "task-packet-M01-T02.md")
				writeFile(t, path, string(mustReadFile(t, path))+"\nUnreviewed change to another active packet.\n")
			case "missing-export-directory":
				require.NoError(t, os.RemoveAll(exports))
			case "changed-export":
				writeFile(t, filepath.Join(exports, "plan-000002.json"), "unreviewed export")
			}
			before := jsonClone(c.State)
			view, _ := packetControlViews(c.State, "M01-T01", []string{fx.taskDir, fx.root})
			require.Equal(t, "unknown", view.Status)
			require.NotEmpty(t, view.Reason)
			require.Empty(t, view.Candidates)
			require.Equal(t, before, c.State)
			if scenario == "missing-export-directory" {
				_, err := os.Stat(exports)
				require.True(t, os.IsNotExist(err), "a read must not recreate missing planning exports")
			}
		})
	}
}

func TestPacketControlMissingActivationReferenceDoesNotInferReviewedAuthority(t *testing.T) {
	fx, c := collaborationProjectionFixture(t, true)
	path := filepath.Join(fx.taskDir, "spec.md")
	data, body, err := contract.ParseFrontmatter(mustReadFile(t, path), path)
	require.NoError(t, err)
	delete(data, "planning-ref")
	raw, err := yaml.Marshal(data)
	require.NoError(t, err)
	writeFile(t, path, "---\n"+string(raw)+"---\n"+body)
	before := jsonClone(c.State)
	view, editor := packetControlViews(c.State, "M01-T01", []string{fx.taskDir, fx.root})
	require.Equal(t, "unknown", view.Status)
	require.Equal(t, "reviewed planning reference is missing", view.Reason)
	require.Empty(t, view.Candidates)
	require.False(t, editor.Editable)
	require.Equal(t, before, c.State)
}

func TestPacketControlLegacyFallbackKeepsUnknownRoutesUnavailable(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	require.Nil(t, c.State.Planning)
	view, editor := packetControlViews(c.State, "M01-T01", []string{fx.taskDir, fx.root})
	require.Equal(t, "verified", view.Status, view.Reason)
	require.False(t, editor.Editable, "legacy execution cannot invent reviewed planning authority")
	path := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
	raw := []byte(strings.Replace(string(mustReadFile(t, path)), "route: default", "route: complex", 1))
	writeFile(t, path, string(raw))
	packet, err := contract.ParsePacket(raw, path)
	require.NoError(t, err)
	c.Config, err = config.LoadReadOnly(fx.taskDir, fx.root)
	require.NoError(t, err)
	c.State.ContractFingerprints[packet.PacketID] = c.packetContractFingerprint(packet, mustReadFile(t, filepath.Join(fx.taskDir, "spec.md")), raw)
	before := jsonClone(c.State)
	view, _ = packetControlViews(c.State, packet.PacketID, []string{fx.taskDir, fx.root})
	require.Equal(t, "unknown", view.Status)
	require.Contains(t, view.Reason, "complex")
	require.Empty(t, view.Candidates)
	require.Equal(t, before, c.State)
}

func TestPacketEditorCanPrepareReviewWithoutImplementationStartAndDoesNotMutateAuthority(t *testing.T) {
	_, c := collaborationProjectionFixture(t, true)
	c.State.Planning.StartAuthorized = false
	for i := range c.State.Planning.Approvals {
		c.State.Planning.Approvals[i].Start = false
	}
	require.NoError(t, planning.AdmitPlanRevision(c.State.Planning, *c.Spec.PlanningRef))
	require.ErrorContains(t, planning.Admit(c.State.Planning, *c.Spec.PlanningRef), "authorize starting implementation")
	before := jsonClone(c.State)
	view := c.collaborationEditor(c.Packets["M01-T01"], c.State.ContractFingerprints["M01-T01"])
	require.True(t, view.Editable, view.Reason)
	require.Equal(t, before, c.State)
	require.False(t, c.State.Planning.StartAuthorized)
	// A live invocation still prevents edits even though no Run lease is held.
	c.State.WorkerPrewarm = &state.WorkerPrewarm{}
	view = c.collaborationEditor(c.Packets["M01-T01"], c.State.ContractFingerprints["M01-T01"])
	require.False(t, view.Editable)
	require.Contains(t, view.Reason, "outstanding invocations")
}

func TestPacketControlOmitsOversizedCandidatesAndEditorWithoutShorteningAuthority(t *testing.T) {
	t.Run("candidate", func(t *testing.T) {
		fx := newGitFixture(t)
		c := unitTestContext(t, fx)
		path := filepath.Join(fx.taskDir, "flywheel.config.json")
		var cfg map[string]any
		require.NoError(t, json.Unmarshal(mustReadFile(t, path), &cfg))
		cfg["routes"].(map[string]any)["default"].(map[string]any)["model"] = strings.Repeat("long-model-id-", 4096)
		raw, err := json.Marshal(cfg)
		require.NoError(t, err)
		writeFile(t, path, string(raw))
		before := jsonClone(c.State)
		view, _ := packetControlViews(c.State, "M01-T01", []string{fx.taskDir, fx.root})
		require.Equal(t, "unknown", view.Status)
		require.Contains(t, view.Reason, "display limit")
		require.Empty(t, view.Candidates, "a shortened model ID must not be presented as a selectable binding")
		require.Equal(t, before, c.State)
	})
	t.Run("collaboration", func(t *testing.T) {
		fx, c := collaborationProjectionFixture(t, true, func(packet string) string {
			packet = strings.Replace(packet, "writes: [build/M01-T01.txt, build/M01-T01-extra.txt]", "writes: [build/]", 1)
			packet = strings.ReplaceAll(packet, "build/M01-T01.txt", "build/"+strings.Repeat("scope/", 4000)+"primary.txt")
			return strings.ReplaceAll(packet, "build/M01-T01-extra.txt", "build/"+strings.Repeat("scope/", 4000)+"secondary.txt")
		})
		before := jsonClone(c.State)
		_, editor := packetControlViews(c.State, "M01-T01", []string{fx.taskDir, fx.root})
		require.False(t, editor.Editable)
		require.Contains(t, editor.Reason, "editor limit")
		require.Nil(t, editor.Value, "a shortened write scope must never become editable authority")
		require.Empty(t, editor.Writes)
		raw, err := json.Marshal(editor)
		require.NoError(t, err)
		require.Less(t, len(raw), 32<<10)
		require.Equal(t, before, c.State)
	})
}
