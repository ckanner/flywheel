package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubscriptionMigrationAddsOnlyCandidatesWithPreservedAuthority(t *testing.T) {
	raw, err := EmbeddedTable()
	require.NoError(t, err)
	preset, err := migrationObject(raw)
	require.NoError(t, err)
	root := map[string]json.RawMessage{"permissions": json.RawMessage(`{"mode":"role-based","bindings":{"routes.default":"danger-full-access"}}`)}
	changes := []MigrationChange{}
	require.NoError(t, migrateSubscriptionCLICandidates(root, preset, &changes))
	var policy ExecutionPolicy
	require.NoError(t, json.Unmarshal(root["execution-policy"], &policy))
	require.Len(t, policy.Routes, 1)
	require.Contains(t, policy.Routes, "default")
	require.Len(t, policy.Bindings, 3)
	require.JSONEq(t, `{"mode":"role-based","bindings":{"routes.default":"danger-full-access"}}`, string(root["permissions"]))
	before := string(root["execution-policy"])
	require.NoError(t, migrateSubscriptionCLICandidates(root, preset, &changes))
	require.Equal(t, before, string(root["execution-policy"]))
	for name, edit := range map[string]func(map[string]json.RawMessage, map[string]json.RawMessage){
		"missing embedded policy":     func(_, p map[string]json.RawMessage) { delete(p, "execution-policy") },
		"invalid embedded policy":     func(_, p map[string]json.RawMessage) { p["execution-policy"] = json.RawMessage(`{"version":2}`) },
		"invalid preserved authority": func(r, _ map[string]json.RawMessage) { r["permissions"] = json.RawMessage(`{"mode":false}`) },
	} {
		t.Run(name, func(t *testing.T) {
			p, err := migrationObject(raw)
			require.NoError(t, err)
			r := map[string]json.RawMessage{}
			edit(r, p)
			require.Error(t, migrateSubscriptionCLICandidates(r, p, &changes))
		})
	}
	root = map[string]json.RawMessage{}
	require.NoError(t, migrateSubscriptionCLICandidates(root, preset, &changes))
	require.NotContains(t, root, "execution-policy")
	dir := t.TempDir()
	t.Setenv(GlobalConfigEnv, filepath.Join(dir, "missing-global"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "flywheel.config.json"), []byte(`{"permissions":{"mode":"role-based","bindings":{"routes.default":"danger-full-access"}}}`), 0600))
	report, err := Migrate(MigrationOptions{Preset: "subscription-cli", Cwd: dir})
	require.NoError(t, err)
	require.Len(t, report.Candidate.ExecutionPolicy.Routes, 1)
	require.Contains(t, report.Notes, "execution-policy candidates were not added for routes backend, batch, complex, frontend, hard, multimodal: preserved permission is not danger-full-access; fixed route bindings remain unchanged")
}

func TestEditorPreservesFullExecutionCandidatesIncludingFixedEffort(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(GlobalConfigEnv, filepath.Join(dir, "global.json"))
	doc, err := ReadEditor(dir, "global")
	require.NoError(t, err)
	require.Contains(t, doc.Values, "execution-policy")
	var policy ExecutionPolicy
	require.NoError(t, json.Unmarshal(doc.Values["execution-policy"], &policy))
	route := policy.Routes["default"]
	route.Initial = "orchestrator-neon"
	policy.Routes["default"] = route
	saved, err := SaveEditor(dir, EditorRequest{Scope: "global", Revision: doc.Revision, Values: map[string]json.RawMessage{"execution-policy": configJSON(policy)}})
	require.NoError(t, err)
	require.True(t, saved.Applied)
	cfg, err := LoadReadOnly("", dir)
	require.NoError(t, err)
	require.Equal(t, "orchestrator-neon", cfg.ExecutionPolicy.Routes["default"].Initial)
	require.Empty(t, cfg.ExecutionPolicy.Bindings["orchestrator-neon"].Thinking)
	require.Equal(t, "antigravity-native", cfg.ExecutionPolicy.Bindings["orchestrator-neon"].NativeProfile)
	// An explicit migration must retain the user's reviewed candidate policy.
	report, err := Migrate(MigrationOptions{Preset: "subscription-cli", Cwd: dir})
	require.NoError(t, err)
	require.Equal(t, cfg.ExecutionPolicy, report.Candidate.ExecutionPolicy)
	_, err = SaveEditor(dir, EditorRequest{Scope: "global", Revision: saved.Revision, Values: map[string]json.RawMessage{"execution-policy": json.RawMessage(`{"version":1}`)}})
	require.Error(t, err)
}
