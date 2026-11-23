package config

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestSubscriptionCLIMigrationPreviewPreservesEngineeringAndRequiresExactSnapshot(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "flywheel.config.json")
	raw := []byte(`{"loops":{"plan":{"harness":"kimi","model":"kimi-code/k3","channel":"old","_note":"keep me"}},"routes":{"default":{"harness":"pi","model":"old","channel":"old"}},"channels":{"old":{"env-key":null}},"profiles":{"custom":{"harness":"kimi","model":"custom","channel":"old"}},"permissions":{"mode":"role-based"},"scheduler":{"mode":"serial","max-parallel":1},"verify":{"command-timeout-ms":12345},"jev":{"enabled":false}}`)
	require.NoError(t, os.WriteFile(file, raw, 0o600))
	t.Setenv(GlobalConfigEnv, filepath.Join(root, "absent-global"))
	preview, err := Migrate(MigrationOptions{Preset: "subscription-cli", Cwd: root})
	require.NoError(t, err)
	require.True(t, preview.Changed)
	require.False(t, preview.Applied)
	require.Equal(t, "role-based", preview.Candidate.Permissions.Mode)
	require.Equal(t, "serial", preview.Candidate.Scheduler.Mode)
	require.EqualValues(t, 12345, preview.Candidate.Verify.CommandTimeoutMs)
	require.Equal(t, "antigravity", preview.Candidate.Routes["default"].Harness)
	require.Equal(t, "custom", preview.Candidate.Profiles["custom"].Model)
	current, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, raw, current)
	applied, err := Migrate(MigrationOptions{Preset: "subscription-cli", Cwd: root, Apply: true, ExpectSHA256: preview.SHA256})
	require.NoError(t, err)
	require.True(t, applied.Applied)
	current, err = os.ReadFile(file)
	require.NoError(t, err)
	require.Contains(t, string(current), "keep me")
	backup, err := os.ReadFile(applied.Backup)
	require.NoError(t, err)
	require.Equal(t, raw, backup)
	again, err := Migrate(MigrationOptions{Preset: "subscription-cli", Cwd: root})
	require.NoError(t, err)
	require.False(t, again.Changed)
	_, err = Migrate(MigrationOptions{Preset: "subscription-cli", Cwd: root, Apply: true, ExpectSHA256: preview.SHA256})
	require.ErrorContains(t, err, "changed since preview")
}
func TestSubscriptionCLIMigrationRejectsMalformedSourceAndEmbeddedContract(t *testing.T) {
	original, err := EmbeddedTable()
	require.NoError(t, err)
	fresh := func() map[string]json.RawMessage {
		root, err := migrationObject(original)
		require.NoError(t, err)
		return root
	}
	for _, tc := range []struct {
		name string
		edit func(map[string]json.RawMessage, map[string]json.RawMessage)
	}{
		{"missing definition", func(_, p map[string]json.RawMessage) { delete(p, "loops") }},
		{"bad section", func(r, _ map[string]json.RawMessage) { r["profiles"] = json.RawMessage(`[]`) }},
		{"bad entry", func(_, p map[string]json.RawMessage) { p["loops"] = json.RawMessage(`{"plan":3}`) }},
		{"bad original entry", func(r, _ map[string]json.RawMessage) { r["loops"] = json.RawMessage(`{"plan":3}`) }},
		{"omitted policy", func(_, p map[string]json.RawMessage) { delete(p, "subscription") }},
		{"bad permissions", func(r, _ map[string]json.RawMessage) { r["permissions"] = json.RawMessage(`[]`) }},
		{"bad permission bindings", func(r, _ map[string]json.RawMessage) {
			r["permissions"] = json.RawMessage(`{"bindings":[]}`)
		}},
		{"enabled HTTP JEV", func(r, _ map[string]json.RawMessage) { r["jev"] = json.RawMessage(`{"enabled":true,"backend":"http"}`) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, preset := fresh(), fresh()
			tc.edit(root, preset)
			changes := []MigrationChange{}
			require.Error(t, migrateSubscriptionCLI(root, preset, &changes))
		})
	}
	root, preset := fresh(), fresh()
	delete(root, "jev")
	delete(root, "sessions")
	changes := []MigrationChange{}
	require.NoError(t, migrateSubscriptionCLI(root, preset, &changes))
	require.NotEmpty(t, root["jev"])
	require.NotEmpty(t, root["sessions"])
	// An embedded explanatory note never becomes a model binding.
	preset["loops"] = json.RawMessage(`{"_note":"description","plan":{"profile":"opus-plan"}}`)
	require.NoError(t, migrateSubscriptionCLI(root, preset, &changes))
}
func TestSubscriptionFallbackCanSelectAnotherDeclaredSubscription(t *testing.T) {
	c := subscriptionFixture()
	c.Channels["google"] = Channel{Harness: "antigravity", Fallback: "claude"}
	require.NoError(t, c.validateSubscriptionCLI())
	delete(c.Loops, "plan") // Exercise the fallback, independently of the primary Claude binding.
	c.Channels["claude"] = Channel{Harness: "pi"}
	require.ErrorContains(t, c.validateSubscriptionCLI(), "subscription")
	c.Channels["claude"] = Channel{Harness: "dsh"}
	require.ErrorContains(t, c.validateSubscriptionCLI(), "subscription")
	c.Channels["claude"] = Channel{Harness: "dsh", Provider: "chatgpt"}
	require.ErrorContains(t, c.validateSubscriptionCLI(), "subscription")
	c.Channels["claude"] = Channel{Harness: "claude-code-pro"}
	require.NoError(t, c.validateSubscriptionCLI())
}

func TestSubscriptionCLIMigrationDoesNotWriteWhenPresetDefinitionsAreInvalid(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "flywheel.config.json")
	raw := []byte(`{"loops":{"plan":{"harness":"kimi","model":"kimi-code/k3","channel":"native"}},"channels":{"native":{"env-key":null}}}`)
	require.NoError(t, os.WriteFile(file, raw, 0o600))
	t.Setenv(GlobalConfigEnv, filepath.Join(root, "absent-global"))
	original := embeddedTable
	t.Cleanup(func() { embeddedTable = original })
	embeddedTable = func() ([]byte, error) { return []byte(`{"loops":{}}`), nil }
	_, err := Migrate(MigrationOptions{Preset: "subscription-cli", Cwd: root, Apply: true})
	require.Error(t, err)
	current, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, raw, current)
	matches, err := filepath.Glob(file + ".backup*")
	require.NoError(t, err)
	require.Empty(t, matches)
}

func TestSubscriptionCLIMigrationConvertsDisabledHTTPJEVWithoutChangingLimits(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "flywheel.config.json")
	raw := []byte(`{"jev":{"enabled":false,"backend":"http","endpoint":"https://example.test/v1/systemone","model":"jev-1.14.0","key-env":"CUSTOM_JEV_KEY","timeout-ms":9000,"max-calls":3,"min-applicability":0.91,"min-choice":0.72,"allowed-data":["task-summary"]}}`)
	require.NoError(t, os.WriteFile(file, raw, 0o600))
	t.Setenv(GlobalConfigEnv, filepath.Join(root, "absent-global"))
	preview, err := Migrate(MigrationOptions{Preset: "subscription-cli", Cwd: root})
	require.NoError(t, err)
	require.Empty(t, preview.Blockers)
	require.Equal(t, "subscription", preview.Candidate.JEV.Backend)
	require.Equal(t, "hydrogen-summary", preview.Candidate.JEV.Profile)
	require.False(t, preview.Candidate.JEV.Enabled)
	before, after := preview.Original.JEV, preview.Candidate.JEV
	require.Equal(t, before.TimeoutMS, after.TimeoutMS)
	require.Equal(t, before.MaxCalls, after.MaxCalls)
	require.Equal(t, before.MinApplicability, after.MinApplicability)
	require.Equal(t, before.MinChoice, after.MinChoice)
	require.Equal(t, before.AllowedData, after.AllowedData)
	require.Empty(t, after.Endpoint)
	require.Empty(t, after.Model)
	require.Empty(t, after.KeyEnv)
	for _, change := range []MigrationChange{
		{Path: "jev.backend", Action: "set"}, {Path: "jev.profile", Action: "set"},
		{Path: "jev.endpoint", Action: "remove"}, {Path: "jev.model", Action: "remove"}, {Path: "jev.key-env", Action: "remove"},
	} {
		require.Contains(t, preview.Changes, change)
	}
	_, err = Migrate(MigrationOptions{Preset: "subscription-cli", Cwd: root, Apply: true, ExpectSHA256: preview.SHA256})
	require.NoError(t, err)
	again, err := Migrate(MigrationOptions{Preset: "subscription-cli", Cwd: root})
	require.NoError(t, err)
	require.False(t, again.Changed)
	// Enabling the migrated policy needs no API fields or further target edit.
	preview.Candidate.JEV.Enabled = true
	require.NoError(t, preview.Candidate.Validate())
}

func TestSubscriptionCLIMigrationBlocksEnabledHTTPJEVBeforeAnyWrite(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "flywheel.config.json")
	raw := []byte(`{"jev":{"enabled":true,"endpoint":"https://example.test/v1/systemone","model":"jev-1.14.0"}}`)
	require.NoError(t, os.WriteFile(file, raw, 0o600))
	t.Setenv(GlobalConfigEnv, filepath.Join(root, "absent-global"))
	report, err := Migrate(MigrationOptions{Preset: "subscription-cli", Cwd: root, Apply: true})
	require.NoError(t, err)
	require.Len(t, report.Blockers, 1)
	require.Contains(t, report.Blockers[0], "enabled HTTP backend")
	require.False(t, report.Applied)
	require.Empty(t, report.Changes)
	require.Empty(t, report.Backup)
	current, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, raw, current)
}

func TestSubscriptionCLIMigrationAddsOnlyMissingAnalystPermission(t *testing.T) {
	for _, tc := range []struct {
		name, mode, override string
	}{
		{"full access", "full-access", ""},
		{"role based", "role-based", ""},
		{"explicit full access", "full-access", "danger-full-access"},
		{"explicit read only", "full-access", "read-only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bindings := map[string]string{"loops.review": "read-only", "routes.hard": "workspace-write"}
			if tc.override != "" {
				bindings["loops.investigate"] = tc.override
			}
			raw := configJSON(map[string]any{"permissions": map[string]any{"mode": tc.mode, "bindings": bindings}})
			require.NoError(t, os.WriteFile(filepath.Join(root, "flywheel.config.json"), raw, 0o600))
			t.Setenv(GlobalConfigEnv, filepath.Join(root, "absent-global"))
			report, err := Migrate(MigrationOptions{Preset: "subscription-cli", Cwd: root})
			require.NoError(t, err)
			require.Empty(t, report.Blockers)
			require.Equal(t, tc.mode, report.Candidate.Permissions.Mode)
			require.Equal(t, "read-only", report.Candidate.PermissionMode("review", "codex-pro"))
			require.Equal(t, "workspace-write", report.Candidate.PermissionMode("execute", "codex-pro", "routes.hard"))
			change := MigrationChange{Path: "permissions.bindings.loops.investigate", Action: "set"}
			if tc.override == "" {
				require.Equal(t, "read-only", report.Candidate.PermissionMode("investigate", "antigravity"))
				require.Contains(t, report.Changes, change)
			} else {
				require.Equal(t, tc.override, report.Candidate.PermissionMode("investigate", "antigravity"))
				require.NotContains(t, report.Changes, change)
			}
			if tc.override == "danger-full-access" {
				require.Contains(t, report.Notes, "permissions.bindings.loops.investigate was preserved; evidence-only Antigravity analysis requires read-only, so review and change this explicit override before running investigation analysis")
			}
		})
	}
}

func TestSubscriptionCLIJEVRejectsMalformedInputsAndActiveTarget(t *testing.T) {
	for _, tc := range []struct {
		name, current, preset, message string
	}{
		{"invalid preset", `{}`, `[]`, "invalid jev target"},
		{"enabled preset", `{}`, `{"enabled":true,"backend":"subscription","profile":"hydrogen-local"}`, "invalid jev target"},
		{"invalid object", `[]`, `{"enabled":false,"backend":"subscription","profile":"hydrogen-local"}`, "JSON objects"},
		{"invalid field", `{"enabled":"false"}`, `{"enabled":false,"backend":"subscription","profile":"hydrogen-local"}`, "invalid existing jev policy"},
		{"enabled target", `{"enabled":true}`, `{"enabled":false,"backend":"subscription","profile":"hydrogen-local"}`, "disable the enabled HTTP backend"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := map[string]json.RawMessage{"jev": json.RawMessage(tc.current)}
			preset := map[string]json.RawMessage{"jev": json.RawMessage(tc.preset)}
			changes := []MigrationChange{}
			require.ErrorContains(t, migrateSubscriptionCLIJEV(root, preset, &changes), tc.message)
			require.Empty(t, changes)
		})
	}
}

func TestAntigravityPanelAndExecutionPolicyValidateEffortAtLoad(t *testing.T) {
	c := subscriptionFixture()
	c.JudgePanel = &LoopBinding{Harness: "antigravity", Model: "gemini-4-hydrogen-high", Thinking: "low", Channel: "google"}
	require.ErrorContains(t, c.Validate(), "judge-panel")
	c.JudgePanel.Thinking = "high"
	require.NoError(t, c.Validate())
	c = executionPolicyConfig()
	b := c.ExecutionPolicy.Bindings["low"]
	b.Harness = "antigravity"
	b.Provider = ""
	b.Model = "gemini-4-hydrogen-high"
	b.Thinking = "low"
	c.ExecutionPolicy.Bindings["low"] = b
	require.ErrorContains(t, c.validateExecutionPolicy(), "thinking level must agree")
	b.Thinking = "high"
	c.ExecutionPolicy.Bindings["low"] = b
	require.ErrorContains(t, c.validateExecutionPolicy(), "provider")
}
