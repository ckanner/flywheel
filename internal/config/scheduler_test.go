package config

import (
	"errors"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSchedulerExplicitPolicyAndBounds(t *testing.T) {
	for _, raw := range []string{`{}`, `{"scheduler":{}}`, `{"scheduler":{"mode":"parallel","max-parallel":2,"providers":{"deepseek-account":1},"_note":"isolated worktrees"}}`} {
		cfg, err := parseConfig([]byte(raw), "test")
		require.NoError(t, err)
		require.Equal(t, 2, cfg.Scheduler.MaxParallel)
	}
	for _, raw := range []string{`null`, `false`, `{"mode":""}`, `{"mode":"auto"}`, `{"max-parallel":0}`, `{"max-parallel":9}`, `{"max-parallel":1.5}`, `{"extra":1}`, `{"providers":{"":1}}`, `{"providers":{"deepseek-account":0}}`} {
		_, err := parseConfig([]byte(`{"scheduler":`+raw+`}`), "test")
		require.Error(t, err)
	}
	_, err := parseConfig([]byte(`{"scheduler":{"mode":"parallel"},"worktree":{"enabled":false}}`), "test")
	require.ErrorContains(t, err, "worktrees")
	cfg := Default()
	cfg.Scheduler.Mode = "unknown"
	require.Error(t, cfg.Validate())
	cfg = Default()
	cfg.Scheduler.Providers = map[string]int{"channel": 0}
	require.Error(t, cfg.Validate())
	old := schedulerSchema
	t.Cleanup(func() { schedulerSchema = old })
	schedulerSchema = func() (*jsonschema.Schema, error) { return nil, errors.New("schema failure") }
	require.Error(t, validateScheduler(nil))
}
