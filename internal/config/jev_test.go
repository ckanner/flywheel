package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/decision"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func TestJEVOptionalDefaultsAndSingleAdoptionPolicy(t *testing.T) {
	for _, raw := range []string{`{}`, `{"jev":{}}`, `{"jev":{"enabled":false,"endpoint":"unavailable","_note":"off"}}`} {
		cfg, err := parseConfig([]byte(raw), "test")
		require.NoError(t, err)
		require.False(t, cfg.JEV.Enabled)
		require.Equal(t, int64(5000), cfg.JEV.TimeoutMS)
		require.Equal(t, "MEROUTER_API_KEY", cfg.JEV.KeyEnv)
		require.Equal(t, "jev-1.14.0", cfg.JEV.Model)
		require.Equal(t, decision.Off, cfg.JEV.DecisionPolicy().Mode)
		require.Equal(t, 8, cfg.JEV.MaxCalls)
		require.Equal(t, 0.8, cfg.JEV.MinApplicability)
		require.Equal(t, 0.6, cfg.JEV.MinChoice)
	}
	cfg, err := parseConfig([]byte(`{"jev":{"enabled":true,"endpoint":"https://example.org/v1/systemone","model":"jev-1.15.0","key-env":"TEST_KEY","timeout-ms":50,"max-calls":3,"min-applicability":0.9,"min-choice":0.7,"allowed-data":["task-summary","diagnostic-facts","source"]}}`), "test")
	require.NoError(t, err)
	policy := cfg.JEV.DecisionPolicy()
	require.Equal(t, decision.Adopt, policy.Mode)
	require.Equal(t, "jev-v1", policy.Version)
	require.Equal(t, 3, policy.MaxCalls)
	require.Equal(t, 0.9, policy.MinApplicability)
	require.Equal(t, 0.7, policy.MinChoice)
	require.Equal(t, int64(50), cfg.JEV.TimeoutMS)
	require.Equal(t, "jev-1.15.0", cfg.JEV.Model)
	cfg.JEV.AllowedData[0] = decision.Diff
	require.Equal(t, decision.TaskSummary, policy.AllowedData[0], "frozen policy must not share mutable config data")
	require.Contains(t, editorValues(cfg), "jev")
	require.NotContains(t, editorValues(cfg), "jev-shadow")
	cfg, err = parseConfig([]byte(`{"jev":{"enabled":true,"endpoint":"https://example.org/v1/systemone","model":"jev","key-env":"TEST_KEY"}}`), "test")
	require.NoError(t, err)
	require.Equal(t, "jev", cfg.JEV.Model, "gateway route alias jev must be accepted for HTTP JEV")
	cfg, err = parseConfig([]byte(`{"jev":{"allowed-data":[]}}`), "test")
	require.NoError(t, err)
	require.Empty(t, cfg.JEV.AllowedData, "explicitly restricted data must not be widened by normalization")
}

func TestJEVStrictConfigurationAndEnabledTarget(t *testing.T) {
	for _, raw := range []string{`null`, `false`, `{"enabled":"false"}`, `{"api-key":"secret"}`, `{"mode":"shadow"}`, `{"expected-model":"jev-1.14.0"}`, `{"timeout-ms":0}`, `{"timeout-ms":30001}`, `{"model":"jev-latest"}`, `{"key-env":"a=b"}`, `{"max-calls":0}`, `{"max-calls":101}`, `{"min-choice":0}`, `{"min-applicability":1.1}`, `{"allowed-data":["secret"]}`, `{"allowed-data":["source","source"]}`} {
		t.Run(raw, func(t *testing.T) {
			_, err := parseConfig([]byte(`{"jev":`+raw+`}`), "test")
			require.Error(t, err)
		})
	}
	for _, endpoint := range []string{"", "://", "https://name:secret@example.org/v1/systemone", "https://example.org/v1/systemone?q=1", "https://example.org/v1/systemone#fragment", "https://example.org/v1/chat/completions", "http://example.org/v1/systemone", "file:///v1/systemone"} {
		cfg := Default()
		cfg.JEV.Enabled, cfg.JEV.Endpoint = true, endpoint
		require.Error(t, cfg.Validate(), endpoint)
	}
	for _, endpoint := range []string{"https://example.org/v1/systemone", "http://localhost:1234/v1/systemone", "http://127.0.0.1:1234/v1/systemone", "http://[::1]:1234/v1/systemone"} {
		cfg := Default()
		cfg.JEV.Enabled, cfg.JEV.Endpoint = true, endpoint
		require.NoError(t, cfg.Validate(), endpoint)
	}
	cfg := Default()
	cfg.JEV.TimeoutMS = 30001
	require.Error(t, cfg.Validate())
	old := jevSchema
	t.Cleanup(func() { jevSchema = old })
	jevSchema = func() (*jsonschema.Schema, error) { return nil, errors.New("schema missing") }
	require.ErrorContains(t, validateJEV(nil), "schema missing")
}

func TestLegacyJEVOnlyMigratesDisabledConnectionHints(t *testing.T) {
	for _, raw := range []string{`{}`, `{"enabled":true}`, `{"enabled":false,"endpoint":"unavailable"}`, `{"model":"jev","expected-model":"mutable"}`} {
		cfg, err := parseConfig([]byte(`{"jev-shadow":`+raw+`}`), "test")
		require.NoError(t, err)
		require.False(t, cfg.JEV.Enabled)
		require.Equal(t, "jev-1.14.0", cfg.JEV.Model)
		require.Equal(t, decision.Off, cfg.JEV.DecisionPolicy().Mode)
	}
	old := `{"enabled":true,"endpoint":"https://example.org/v1/systemone","model":"jev","expected-model":"jev-1.13.0","key-env":"OLD_KEY","timeout-ms":50}`
	cfg, err := parseConfig([]byte(`{"jev-shadow":`+old+`}`), "test")
	require.NoError(t, err)
	require.False(t, cfg.JEV.Enabled)
	require.Equal(t, "https://example.org/v1/systemone", cfg.JEV.Endpoint)
	require.Equal(t, "jev-1.13.0", cfg.JEV.Model)
	require.Equal(t, "OLD_KEY", cfg.JEV.KeyEnv)
	require.Equal(t, int64(50), cfg.JEV.TimeoutMS)
	for _, enabled := range []bool{true, false} {
		newJEV := configJSON(map[string]any{"enabled": enabled, "endpoint": "https://new.example.org/v1/systemone", "model": "jev-2.0.0"})
		cfg, err := parseConfig(configJSON(map[string]json.RawMessage{"jev-shadow": json.RawMessage(old), "jev": newJEV}), "test")
		require.NoError(t, err)
		require.Equal(t, enabled, cfg.JEV.Enabled)
		require.Equal(t, "https://new.example.org/v1/systemone", cfg.JEV.Endpoint)
		require.Equal(t, "jev-2.0.0", cfg.JEV.Model)
		require.Equal(t, "MEROUTER_API_KEY", cfg.JEV.KeyEnv, "explicit new object must not silently inherit old hints")
	}
	for _, raw := range []string{`null`, `false`, `{"enabled":"false"}`, `{"api-key":"secret"}`, `{"timeout-ms":5001}`} {
		_, err := parseConfig([]byte(`{"jev-shadow":`+raw+`}`), "test")
		require.Error(t, err)
	}
	saved := legacyJEVSchema
	t.Cleanup(func() { legacyJEVSchema = saved })
	legacyJEVSchema = func() (*jsonschema.Schema, error) { return nil, errors.New("legacy schema missing") }
	_, err = parseConfig([]byte(`{"jev-shadow":{}}`), "test")
	require.ErrorContains(t, err, "legacy schema missing")
}

func TestEditorRetiresLegacyKeyWithoutEnablingJEV(t *testing.T) {
	for _, explicit := range []bool{true, false} {
		t.Run(map[bool]string{true: "new-settings", false: "legacy-hints"}[explicit], func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "flywheel.config.json")
			require.NoError(t, os.WriteFile(path, []byte(`{"jev-shadow":{"enabled":true,"endpoint":"https://old.example.org/v1/systemone","key-env":"OLD_KEY"},"_note":"preserved"}`), 0600))
			doc, err := ReadEditor(root, "project")
			require.NoError(t, err)
			require.NotContains(t, doc.Values, "jev-shadow")
			values := map[string]json.RawMessage{}
			if explicit {
				values["jev"] = configJSON(map[string]any{"enabled": true, "endpoint": "https://new.example.org/v1/systemone"})
			}
			saved, err := SaveEditor(root, EditorRequest{Scope: "project", Revision: doc.Revision, Values: values})
			require.NoError(t, err)
			require.True(t, saved.Applied)
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			var actual map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &actual))
			require.NotContains(t, actual, "jev-shadow")
			require.Contains(t, actual, "jev")
			require.JSONEq(t, `"preserved"`, string(actual["_note"]))
			cfg, err := LoadReadOnly("", root)
			require.NoError(t, err)
			require.Equal(t, explicit, cfg.JEV.Enabled)
		})
	}
}

func TestJEVUsesExistingWholeFileConfigurationPrecedence(t *testing.T) {
	root := t.TempDir()
	global := filepath.Join(root, "global.json")
	project := filepath.Join(root, "project")
	task := filepath.Join(project, "docs", "task")
	require.NoError(t, os.MkdirAll(task, 0700))
	t.Setenv(GlobalConfigEnv, global)
	require.NoError(t, os.WriteFile(global, []byte(`{"jev":{"enabled":true,"endpoint":"https://global.example.org/v1/systemone","key-env":"GLOBAL_KEY"}}`), 0600))
	cfg, err := LoadReadOnly(task, project)
	require.NoError(t, err)
	require.True(t, cfg.JEV.Enabled)
	require.NoError(t, os.WriteFile(filepath.Join(project, "flywheel.config.json"), []byte(`{"jev":{"enabled":false}}`), 0600))
	cfg, err = LoadReadOnly(task, project)
	require.NoError(t, err)
	require.False(t, cfg.JEV.Enabled)
	require.Empty(t, cfg.JEV.Endpoint, "project file is not a deep merge with global credentials or target")
	require.Equal(t, "MEROUTER_API_KEY", cfg.JEV.KeyEnv)
	require.NoError(t, os.WriteFile(filepath.Join(task, "flywheel.config.json"), []byte(`{"jev":{"enabled":true,"endpoint":"https://task.example.org/v1/systemone"}}`), 0600))
	cfg, err = LoadReadOnly(task, project)
	require.NoError(t, err)
	require.True(t, cfg.JEV.Enabled)
	require.Equal(t, "https://task.example.org/v1/systemone", cfg.JEV.Endpoint)
}
