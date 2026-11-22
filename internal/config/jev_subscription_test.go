package config

import (
	"encoding/json"
	"testing"

	"github.com/ckanner/flywheel/templates"
	"github.com/stretchr/testify/require"
)

func jevSubscriptionFixture(t *testing.T) Config {
	t.Helper()
	raw, err := templates.FS.ReadFile("flywheel.config.example.json")
	require.NoError(t, err)
	cfg, err := parseConfig(raw, "subscription-default")
	require.NoError(t, err)
	require.False(t, cfg.JEV.Enabled)
	require.Equal(t, "subscription", cfg.JEV.Backend)
	require.Equal(t, "hydrogen-local", cfg.JEV.Profile)
	cfg.JEV.Enabled = true
	return *cfg
}

func TestJEVSubscriptionResolvesExplicitNativeProfileWithoutAPIFields(t *testing.T) {
	cfg := jevSubscriptionFixture(t)
	require.NoError(t, cfg.Validate())
	b, err := cfg.ResolveJEVSubscription()
	require.NoError(t, err)
	require.Equal(t, "hydrogen-local", b.Profile)
	require.Equal(t, "antigravity", b.Harness)
	require.Equal(t, "gemini-4-hydrogen-medium", b.Model)
	require.Equal(t, "medium", b.Effort)
	require.Empty(t, cfg.JEV.Endpoint)
	require.Empty(t, cfg.JEV.Model)
	require.Empty(t, cfg.JEV.KeyEnv)
	require.EqualValues(t, 30000, cfg.JEV.TimeoutMS)
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	_, err = parseConfig(raw, "roundtrip")
	require.NoError(t, err)
	j := JEVConfig{Backend: "subscription"}
	j.normalize()
	require.Equal(t, "hydrogen-local", j.Profile)
	for _, field := range []string{"endpoint", "model", "key-env"} {
		var value map[string]any
		require.NoError(t, json.Unmarshal(raw, &value))
		value["jev"].(map[string]any)[field] = ""
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		_, err = parseConfig(encoded, "mixed")
		require.Error(t, err)
	}
}

func TestJEVSubscriptionRejectsUnverifiedOrChangingExecutionAuthority(t *testing.T) {
	for name, change := range map[string]func(*Config){
		"missing": func(c *Config) { c.JEV.Profile = "missing" },
		"other harness": func(c *Config) {
			p := c.Profiles[c.JEV.Profile]
			p.Harness = "claude-code-pro"
			c.Profiles[c.JEV.Profile] = p
		},
		"api provider": func(c *Config) {
			p := c.Profiles[c.JEV.Profile]
			p.Provider = "google-api"
			c.Profiles[c.JEV.Profile] = p
		},
		"native child": func(c *Config) {
			p := c.Profiles[c.JEV.Profile]
			p.NativeProfile = "child"
			c.Profiles[c.JEV.Profile] = p
		},
		"effort conflict": func(c *Config) { p := c.Profiles[c.JEV.Profile]; p.Thinking = "low"; c.Profiles[c.JEV.Profile] = p },
		"credential": func(c *Config) {
			p := c.Profiles[c.JEV.Profile]
			ch := c.Channels[p.Channel]
			ch.EnvKey = new("GOOGLE_API_KEY")
			c.Channels[p.Channel] = ch
		},
		"token": func(c *Config) { c.Profiles["bad/profile"] = c.Profiles[c.JEV.Profile]; c.JEV.Profile = "bad/profile" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := jevSubscriptionFixture(t)
			change(&cfg)
			_, err := cfg.ResolveJEVSubscription()
			require.Error(t, err)
		})
	}
	cfg := jevSubscriptionFixture(t)
	cfg.JEV.Profile = "missing"
	require.Error(t, cfg.Validate())
}
