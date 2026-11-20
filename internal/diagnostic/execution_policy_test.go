package diagnostic

import (
	"encoding/json"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/stretchr/testify/require"
)

func TestConfigurationInspectionExposesActualInitialAndKeepsDeclaration(t *testing.T) {
	cfg := config.Default()
	cfg.Routes = map[string]config.RouteBinding{"default": {Harness: "dsh", Provider: "deepseek-account", Channel: "account", Model: "flash", Thinking: "high"}}
	cfg.Channels = map[string]config.Channel{"account": {Harness: "dsh", Provider: "deepseek-account"}}
	cfg.ExecutionPolicy = &config.ExecutionPolicy{Version: 1, Bindings: map[string]config.ExecutionBinding{
		"trial-low": {Harness: "dsh", Provider: "deepseek-account", Channel: "account", Model: "flash", Thinking: "low", Permission: "workspace-write", SessionMode: "scoped"}},
		Routes: map[string]config.ExecutionRoute{"default": {Initial: "trial-low", Allowed: []string{"trial-low"}, QualityOrder: []string{}, AfterStalledAttempts: 1, MaxSwitches: 0}}}
	require.NoError(t, cfg.Validate())
	view := configurationView(&cfg, "/work", "")
	require.Equal(t, cfg.ExecutionPolicy, view.ExecutionPolicy)
	require.Len(t, view.Bindings, 1)
	require.Equal(t, "low", view.Bindings[0].Thinking)
	require.Equal(t, "trial-low", view.Bindings[0].CandidateID)
	require.Equal(t, "execution-policy.bindings.trial-low", view.Bindings[0].BindingSource)
	require.Equal(t, "scoped", view.Bindings[0].Session.Mode)
	require.Equal(t, "workspace-write", view.Bindings[0].PermissionMode)
	require.Equal(t, "high", cfg.Routes["default"].Thinking, "inspection must not edit the original declaration")
	raw, err := json.Marshal(view)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"execution-policy"`)
	require.Contains(t, string(raw), `"trial-low"`)
	cfg.ExecutionPolicy = nil
	raw, err = json.Marshal(configurationView(&cfg, "/work", ""))
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"execution-policy"`, "old inspection output gains no implicit policy")
}
