package config

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func executionPolicyConfig() Config {
	c := Default()
	c.Normalize()
	c.Routes = map[string]RouteBinding{"default": {Harness: "dsh", Provider: "deepseek-account", Channel: "account", Model: "flash", Thinking: "high"}}
	c.Channels = map[string]Channel{"account": {Harness: "dsh", Provider: "deepseek-account", Fallback: "backup"}, "backup": {Harness: "dsh", Provider: "deepseek-official", Models: map[string]string{"default": "flash"}}}
	low := ExecutionBinding{Harness: "dsh", Provider: "deepseek-account", Channel: "account", Model: "flash", Thinking: "low", Permission: "workspace-write", SessionMode: "scoped"}
	high, backup := low, low
	high.Thinking = "high"
	backup.Provider, backup.Channel = "deepseek-official", "backup"
	c.ExecutionPolicy = &ExecutionPolicy{Version: 1, Bindings: map[string]ExecutionBinding{"low": low, "high": high, "backup": backup},
		Routes: map[string]ExecutionRoute{"default": {Initial: "low", Allowed: []string{"low", "high", "backup"}, QualityOrder: []string{"low", "high"}, ChannelFallbacks: map[string]string{"low": "backup"}, AfterStalledAttempts: 1, MaxSwitches: 2}}}
	return c
}

func TestExecutionPolicyExplicitCompleteBindings(t *testing.T) {
	c := executionPolicyConfig()
	require.NoError(t, c.Validate())
	raw, err := json.Marshal(map[string]any{"routes": c.Routes, "channels": c.Channels, "execution-policy": c.ExecutionPolicy,
		"sessions": map[string]any{"mode": "scoped", "policy-version": 1, "context": "verified-incremental"}})
	require.NoError(t, err)
	parsed, err := parseConfig(raw, "experiment.json")
	require.NoError(t, err)
	require.Equal(t, c.ExecutionPolicy, parsed.ExecutionPolicy)
	require.Equal(t, "verified-incremental", parsed.Sessions.Context)
	require.Equal(t, "high", parsed.Routes["default"].Thinking, "the compatibility baseline is not rewritten")
	legacy, err := parseConfig([]byte(`{}`), "legacy.json")
	require.NoError(t, err)
	require.Nil(t, legacy.ExecutionPolicy)
	require.Empty(t, legacy.Sessions.Context)
}

func TestExecutionPolicyRejectsInvalidOrUnauthorizedBindings(t *testing.T) {
	cases := map[string]func(*Config){
		"schema version":  func(c *Config) { c.ExecutionPolicy.Version = 2 },
		"missing channel": func(c *Config) { delete(c.Channels, "account") },
		"provider on non DSH": func(c *Config) {
			b := c.ExecutionPolicy.Bindings["low"]
			b.Harness = "kimi"
			c.ExecutionPolicy.Bindings["low"] = b
		},
		"implicit provider": func(c *Config) {
			b := c.ExecutionPolicy.Bindings["low"]
			b.Provider = ""
			c.ExecutionPolicy.Bindings["low"] = b
		},
		"unsupported effort": func(c *Config) {
			b := c.ExecutionPolicy.Bindings["low"]
			b.Thinking = "medium"
			c.ExecutionPolicy.Bindings["low"] = b
		},
		"missing route": func(c *Config) { delete(c.Routes, "default") },
		"unknown ID":    func(c *Config) { delete(c.ExecutionPolicy.Bindings, "low") },
		"permission expansion": func(c *Config) {
			b := c.ExecutionPolicy.Bindings["low"]
			b.Permission = "danger-full-access"
			c.ExecutionPolicy.Bindings["low"] = b
		},
		"initial outside allowlist": func(c *Config) {
			r := c.ExecutionPolicy.Routes["default"]
			r.Initial = "other"
			c.ExecutionPolicy.Routes["default"] = r
		},
		"quality outside allowlist": func(c *Config) {
			r := c.ExecutionPolicy.Routes["default"]
			r.QualityOrder = []string{"other"}
			c.ExecutionPolicy.Routes["default"] = r
		},
		"fallback outside allowlist": func(c *Config) { r := c.ExecutionPolicy.Routes["default"]; r.ChannelFallbacks["low"] = "other" },
		"fallback self":              func(c *Config) { r := c.ExecutionPolicy.Routes["default"]; r.ChannelFallbacks["low"] = "low" },
		"fallback chain mismatch":    func(c *Config) { b := c.Channels["account"]; b.Fallback = "other"; c.Channels["account"] = b },
		"fallback model mismatch":    func(c *Config) { c.Channels["backup"].Models["default"] = "other" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := executionPolicyConfig()
			mutate(&c)
			require.ErrorContains(t, c.Validate(), "execution-policy")
		})
	}
	for _, raw := range []string{`null`, `[]`, `{}`, `{"version":1,"bindings":{},"routes":{}}`} {
		_, err := parseConfig([]byte(`{"execution-policy":`+raw+`}`), "invalid.json")
		require.ErrorContains(t, err, "execution-policy")
	}
	c := Default()
	c.Sessions.Context = "verified-incremental"
	c.Sessions.Mode, c.Sessions.PolicyVersion = "", 0
	c.Normalize()
	require.ErrorContains(t, c.Validate(), "sessions", "partial opt-in must not be silently normalized away")
}

func TestExecutionEffortVocabularyAndSchemaFailure(t *testing.T) {
	for _, tc := range []struct {
		harness, provider, effort string
		valid                     bool
	}{
		{"kimi", "", "max", true}, {"kimi", "", "xhigh", false},
		{"codex-pro", "", "medium", true}, {"codex-pro", "", "off", false},
		{"dsh", "chatgpt", "xhigh", true}, {"dsh", "deepseek-account", "off", true},
		{"dsh", "deepseek-official", "medium", false},
	} {
		require.Equal(t, tc.valid, executionEffortSupported(ExecutionBinding{Harness: tc.harness, Provider: tc.provider, Thinking: tc.effort}))
	}
	old := executionPolicySchema
	t.Cleanup(func() { executionPolicySchema = old })
	executionPolicySchema = func() (*jsonschema.Schema, error) { return nil, errors.New("unavailable embedded schema") }
	require.ErrorContains(t, validateExecutionPolicy(map[string]any{}), "execution-policy schema")
}
