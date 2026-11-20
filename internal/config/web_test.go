package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWebBindingsRequireExactProviderAndEffort(t *testing.T) {
	for _, thinking := range []string{"", "pro"} {
		c := Default()
		c.Loops = map[string]LoopBinding{"review": {Harness: "dsh", Provider: "chatgpt", Model: "gpt-6-pro-web", Channel: "chatgpt-subscription", Thinking: thinking}}
		require.NoError(t, c.Validate())
	}
	for _, raw := range []string{
		`{"loops":{"review":{"harness":"dsh","provider":"chatgpt","model":"gpt-6-pro-web","channel":"chatgpt-subscription","thinking":"high"}}}`,
		`{"routes":{"default":{"harness":"dsh","provider":"deepseek-account","model":"gpt-6-pro-web","channel":"account","thinking":"pro"}}}`,
		`{"profiles":{"web":{"harness":"codex-pro","model":"gpt-6-pro-web","channel":"chatgpt-subscription","thinking":"pro"}}}`,
		`{"judge-panel":{"harness":"dsh","provider":"chatgpt","model":"gpt-6-pro-web","channel":"chatgpt-subscription","thinking":"max"}}`,
	} {
		_, err := parseConfig([]byte(raw), "web.json")
		require.ErrorContains(t, err, "gpt-6-pro-web requires")
	}
	c := executionPolicyConfig()
	b := c.ExecutionPolicy.Bindings["low"]
	b.Provider, b.Model, b.Thinking = "chatgpt", "gpt-6-pro-web", "pro"
	c.Channels["account"] = Channel{Harness: "dsh", Provider: "chatgpt"}
	c.ExecutionPolicy.Bindings = map[string]ExecutionBinding{"web": b}
	c.ExecutionPolicy.Routes = map[string]ExecutionRoute{"default": {Initial: "web", Allowed: []string{"web"}, QualityOrder: []string{"web"}, AfterStalledAttempts: 1, MaxSwitches: 0}}
	c.Routes["default"] = RouteBinding{Harness: "dsh", Provider: "chatgpt", Channel: "account", Model: "gpt-6-pro-web", Thinking: "pro"}
	require.NoError(t, c.Validate(), "the execution policy schema accepts the exact Web effort")
	b.Thinking = "high"
	require.False(t, executionEffortSupported(b))
	b.Thinking, b.Model = "pro", "gpt-6.2-sol"
	require.False(t, executionEffortSupported(b), "pro is not added to unrelated adapter vocabularies")
}
