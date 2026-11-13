package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func TestSchedulerExplicitPolicyAndBounds(t *testing.T) {
	for _, raw := range []string{`{}`, `{"scheduler":{}}`, `{"scheduler":{"mode":"parallel","max-parallel":2,"providers":{"deepseek-account":1},"_note":"isolated worktrees"}}`} {
		cfg, err := parseConfig([]byte(raw), "test")
		require.NoError(t, err)
		require.Equal(t, 2, cfg.Scheduler.MaxParallel)
	}
	for _, raw := range []string{`null`, `false`, `{"mode":""}`, `{"mode":"auto"}`, `{"max-parallel":0}`, `{"max-parallel":9}`, `{"max-parallel":1.5}`, `{"extra":1}`, `{"providers":{"":1}}`, `{"providers":{"deepseek-account":0}}`, `{"dispatch":""}`, `{"dispatch":"ready"}`, `{"dispatch":null}`, `{"dispatch":false}`, `{"prewarm":null}`, `{"prewarm":"true"}`, `{"prewarm":1}`} {
		_, err := parseConfig([]byte(`{"scheduler":`+raw+`}`), "test")
		require.Error(t, err)
	}
	off, err := parseConfig([]byte(`{"scheduler":{"mode":"parallel"},"worktree":{"enabled":false}}`), "test")
	require.NoError(t, err)
	require.Equal(t, "parallel", off.Scheduler.Mode)
	require.False(t, *off.Worktree.Enabled)
	cfg := Default()
	cfg.Scheduler.Mode = "unknown"
	require.Error(t, cfg.Validate())
	cfg = Default()
	cfg.Scheduler.Dispatch = "unknown"
	require.ErrorContains(t, cfg.Validate(), "dispatch")
	cfg = Default()
	cfg.Scheduler.Providers = map[string]int{"channel": 0}
	require.Error(t, cfg.Validate())
	old := schedulerSchema
	t.Cleanup(func() { schedulerSchema = old })
	schedulerSchema = func() (*jsonschema.Schema, error) { return nil, errors.New("schema failure") }
	require.Error(t, validateScheduler(nil))
}

func TestSchedulerCompatibilityAndExplicitOptimizations(t *testing.T) {
	defaults := Default()
	require.Equal(t, "parallel", defaults.Scheduler.Mode)
	require.Equal(t, "batch", defaults.Scheduler.Dispatch)
	require.False(t, defaults.Scheduler.Prewarm)
	for _, tc := range []struct {
		name, raw, mode, dispatch string
		prewarm                   bool
	}{
		{"legacy omitted", `{}`, "serial", "batch", false},
		{"legacy empty", `{"scheduler":{}}`, "serial", "batch", false},
		{"existing parallel", `{"scheduler":{"mode":"parallel"}}`, "parallel", "batch", false},
		{"ready queue only", `{"scheduler":{"mode":"parallel","dispatch":"ready-queue"}}`, "parallel", "ready-queue", false},
		{"prewarm only", `{"scheduler":{"mode":"serial","prewarm":true}}`, "serial", "batch", true},
		{"explicit both", `{"scheduler":{"mode":"parallel","dispatch":"ready-queue","prewarm":true}}`, "parallel", "ready-queue", true},
		{"explicit opt out", `{"scheduler":{"mode":"parallel","dispatch":"batch","prewarm":false}}`, "parallel", "batch", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseConfig([]byte(tc.raw), "test")
			require.NoError(t, err)
			require.Equal(t, tc.mode, cfg.Scheduler.Mode)
			require.Equal(t, tc.dispatch, cfg.Scheduler.Dispatch)
			require.Equal(t, tc.prewarm, cfg.Scheduler.Prewarm)
			// A saved, normalized configuration retains the explicit policy.
			roundTrip, err := parseConfig(configJSON(cfg), "saved")
			require.NoError(t, err)
			require.Equal(t, cfg.Scheduler, roundTrip.Scheduler)
		})
	}
}

func TestSchedulerEditorRetainsPolicyAndUneditedConfiguration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "flywheel.config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"_note":"preserve","scheduler":{"mode":"serial","max-parallel":2},"jev":{"enabled":false}}`), 0600))
	doc, err := ReadEditor(root, "project")
	require.NoError(t, err)
	var initial SchedulerConfig
	require.NoError(t, json.Unmarshal(doc.Values["scheduler"], &initial))
	require.Equal(t, "batch", initial.Dispatch)
	require.False(t, initial.Prewarm)
	policy := json.RawMessage(`{"mode":"parallel","max-parallel":3,"providers":{"deepseek-account":1},"dispatch":"ready-queue","prewarm":true}`)
	saved, err := SaveEditor(root, EditorRequest{Scope: "project", Revision: doc.Revision, Values: map[string]json.RawMessage{"scheduler": policy}})
	require.NoError(t, err)
	require.True(t, saved.Applied)
	loaded, err := LoadReadOnly("", root)
	require.NoError(t, err)
	require.Equal(t, SchedulerConfig{Mode: "parallel", MaxParallel: 3, Providers: map[string]int{"deepseek-account": 1}, Dispatch: "ready-queue", Prewarm: true}, loaded.Scheduler)
	require.False(t, loaded.JEV.Enabled, "scheduler changes do not enable JEV")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var source map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &source))
	require.JSONEq(t, `"preserve"`, string(source["_note"]))
	require.JSONEq(t, `{"enabled":false}`, string(source["jev"]))
}
