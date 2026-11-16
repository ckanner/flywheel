package fallback

import (
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanningRoleBindingResolutionKeepsRoleOverrides(t *testing.T) {
	cfg := testConfig()
	cfg.Loops["review"] = config.LoopBinding{Harness: "dsh", Model: "review", Channel: "account", NativeProfile: "team", Thinking: "high"}
	cfg.Channels["account"] = config.Channel{Harness: "dsh", Provider: "chatgpt"}
	for _, role := range []string{"design-review", "plan-review"} {
		got, err := ResolveBinding(cfg, role, "")
		require.NoError(t, err)
		assert.Equal(t, Binding{Harness: "dsh", Provider: "chatgpt", Model: "review", Channel: "account", Thinking: "high", NativeProfile: "team"}, got)
	}
	got, err := ResolveBinding(cfg, "design-plan", "")
	require.NoError(t, err)
	assert.Equal(t, "kimi-code/k3", got.Model)
	cfg.Loops["design-review"] = config.LoopBinding{Harness: "codex-pro", Model: "independent", Channel: "subscription"}
	got, err = ResolveBinding(cfg, "design-review", "")
	require.NoError(t, err)
	assert.Equal(t, "independent", got.Model)
	assert.Empty(t, got.NativeProfile)
	assert.Empty(t, got.Provider)
	delete(cfg.Loops, "plan")
	_, err = ResolveBinding(cfg, "design-plan", "")
	require.ErrorContains(t, err, `no loop binding for "design-plan"`)
}

func TestPlanningBackendInheritanceDoesNotAuthorizeUnmappedChannelFallback(t *testing.T) {
	cfg := testConfig()
	cfg.Channels["paid"] = config.Channel{Harness: "dsh", Provider: "paid-adapter", Models: map[string]string{"plan": "large", "review": "large"}}
	for _, role := range []string{"design-plan", "design-review", "plan-review"} {
		assert.Nil(t, ModelForChannel(cfg, "paid", role, ""), "sharing a backend does not authorize another role's paid channel")
		cfg.Channels["paid"].Models[role] = "explicit-" + role
		assert.Equal(t, &ChannelModel{Harness: "dsh", Model: "explicit-" + role}, ModelForChannel(cfg, "paid", role, ""))
	}
}
