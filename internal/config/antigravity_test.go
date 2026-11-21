package config

import (
	"encoding/json"
	"testing"

	"github.com/ckanner/flywheel/templates"
	"github.com/stretchr/testify/require"
)

func TestAntigravityOrchestratorPinsNativeEffort(t *testing.T) {
	for _, slug := range []string{"gemini-pro-orchestrator-neon", "gemini-pro-orchestrator", "gemini-pro-orchestrator-argon"} {
		require.True(t, AntigravityOrchestrator(slug))
		require.NoError(t, ValidateAntigravityBinding(slug, ""))
		require.ErrorContains(t, ValidateAntigravityBinding(slug, "high"), "fixed")
		require.True(t, executionEffortSupported(ExecutionBinding{Harness: "antigravity", Model: slug}))
	}
	for _, slug := range []string{"gemini-pro-orchestrator-carbon", "gemini-pro-orchestrator-neon-high", "gemini-4-neon-high", ""} {
		require.False(t, AntigravityOrchestrator(slug))
		require.Error(t, ValidateAntigravityBinding(slug, ""))
	}
	c := jevSubscriptionFixture(t)
	p := c.Profiles[c.JEV.Profile]
	p.Model, p.Thinking = "gemini-pro-orchestrator-neon", ""
	c.Profiles[c.JEV.Profile] = p
	_, err := c.ResolveJEVSubscription()
	require.ErrorContains(t, err, "tool-free")
}

func TestAntigravityNativeSchemaRejectsUnsupportedControls(t *testing.T) {
	require.NoError(t, ValidateAntigravityNative(nil))
	for _, mode := range []string{"native", "subagents", "swarm"} {
		for _, suffix := range []string{"", `,"maxObservedAgents":1`, `,"maxObservedAgents":8`} {
			raw := json.RawMessage(`{"version":1,"mode":"` + mode + `"` + suffix + `}`)
			require.NoError(t, ValidateAntigravityNative(raw))
			c := Default()
			c.NativeProfiles = map[string]NativeProfile{"native": {Harness: "antigravity", Options: raw}}
			c.Routes = map[string]RouteBinding{"default": {Harness: "antigravity", NativeProfile: "native"}}
			require.NoError(t, c.validateNativeProfiles())
		}
	}
	for _, raw := range []string{`{`, `null`, `{}`, `{"version":2,"mode":"native"}`, `{"version":1,"mode":"solo"}`, `{"version":1,"mode":"swarm","maxObservedAgents":0}`, `{"version":1,"mode":"swarm","maxObservedAgents":9}`, `{"version":1,"mode":"swarm","maxConcurrent":4}`, `{"version":1,"mode":"swarm","maxObservedAgents":1.5}`} {
		require.Error(t, ValidateAntigravityNative(json.RawMessage(raw)), raw)
	}
}

func TestSubscriptionDefaultCandidatesPreserveFixedBaselines(t *testing.T) {
	for _, file := range []string{"flywheel.config.example.json", "flywheel.profiles.example.json"} {
		raw, err := templates.FS.ReadFile(file)
		require.NoError(t, err)
		cfg, err := parseConfig(raw, file)
		require.NoError(t, err)
		require.False(t, cfg.JEV.Enabled)
		require.Len(t, cfg.ExecutionPolicy.Routes, 7)
		for route, p := range cfg.ExecutionPolicy.Routes {
			b := cfg.ExecutionPolicy.Bindings[p.Initial]
			baseline := cfg.Routes[route]
			require.Equal(t, baseline.Model, b.Model, route)
			require.Equal(t, baseline.Thinking, b.Thinking, route)
			require.Equal(t, baseline.NativeProfile, b.NativeProfile, route)
			require.Empty(t, p.QualityOrder, "JEV off must retain the fixed baseline")
			require.Equal(t, 2, p.MaxSwitches)
			require.Len(t, p.Allowed, map[bool]int{true: 2, false: 3}[route == "frontend" || route == "multimodal"])
		}
		require.Equal(t, "gemini-pro-orchestrator-neon", cfg.ExecutionPolicy.Bindings["orchestrator-neon"].Model)
		require.Empty(t, cfg.ExecutionPolicy.Bindings["orchestrator-neon"].Thinking)
		require.JSONEq(t, `{"version":1,"mode":"swarm","maxObservedAgents":4}`, string(cfg.NativeProfiles["antigravity-swarm"].Options))
	}
}
