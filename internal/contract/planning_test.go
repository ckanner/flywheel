package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func TestPlanningSessionVersionFencesActualPreviousLoader(t *testing.T) {
	raw, err := os.ReadFile("testdata/session-ledger-c5cc993.schema.json")
	require.NoError(t, err)
	var document any
	require.NoError(t, json.Unmarshal(raw, &document))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("https://flywheel.local/old-planning-ledger.json", document))
	old, err := compiler.Compile("https://flywheel.local/old-planning-ledger.json")
	require.NoError(t, err)
	ledger := map[string]any{"version": 3, "records": map[string]any{}}
	require.NoError(t, old.Validate(ledger))
	ledger["version"] = 4
	require.Error(t, old.Validate(ledger), "old state.Load rejects even an empty v4 ledger before returning a rewritable State")
	require.NoError(t, ValidateSessionStateValue(ledger))
	for _, role := range []string{"design-plan", "design-review", "plan-review"} {
		value := sessionSchemaValue()
		value["version"] = 4
		record := value["records"].(map[string]any)["execute/M01-T01"].(map[string]any)
		record["role"] = role
		require.NoError(t, ValidateSessionStateValue(value))
		require.Error(t, old.Validate(value))
	}
}

func TestSpecPlanningReferenceAndDeferredMilestones(t *testing.T) {
	raw, err := templates.FS.ReadFile("spec.example.md")
	require.NoError(t, err)
	ref := "planning-ref:\n  version: 1\n  design: {revision: 1, digest: \"" + strings.Repeat("a", 64) + "\"}\n  plans:\n    - {revision: 1, digest: \"" + strings.Repeat("b", 64) + "\"}\n"
	raw = []byte(strings.Replace(string(raw), "---\n", "---\n"+ref, 1))
	spec, err := ParseSpec(raw, "spec.md")
	require.NoError(t, err)
	require.Equal(t, 1, spec.PlanningRef.Version)
	require.Len(t, spec.PlanningRef.Plans, 1)
	bad := strings.Replace(string(raw), "version: 1\n  design", "version: 2\n  design", 1)
	_, err = ParseSpec([]byte(bad), "spec.md")
	require.Error(t, err)
	packet, err := templates.FS.ReadFile("task-packet.example.md")
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "task-packet-M01-T01.md"), packet, 0o600))
	parsed, err := ParsePacket(packet, "task-packet-M01-T01.md")
	require.NoError(t, err)
	milestone, err := MilestoneOfPacket(parsed.PacketID)
	require.NoError(t, err)
	spec.Milestones = append(spec.Milestones, Milestone{ID: "M-99", Title: "Future work", Planning: "deferred"})
	_, err = LoadPackets(dir, spec)
	require.NoError(t, err, "future deferred milestones need no placeholder packet")
	for i := range spec.Milestones {
		if spec.Milestones[i].ID == milestone {
			spec.Milestones[i].Planning = "deferred"
		}
	}
	_, err = LoadPackets(dir, spec)
	require.ErrorContains(t, err, "deferred")
}
