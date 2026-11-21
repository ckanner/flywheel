package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectionResolvesInitialWithoutChangingRouteAuthority(t *testing.T) {
	c := Default()
	c.Routes = map[string]RouteBinding{"default": {Harness: "antigravity", Model: "gemini-4-hydrogen-high", Thinking: "high", Channel: "google"}}
	c.Permissions.Mode = "full-access"
	initial := ExecutionBinding{Harness: "antigravity", Model: "gemini-pro-orchestrator-neon", Channel: "google", Permission: "danger-full-access", SessionMode: "scoped"}
	c.ExecutionPolicy = &ExecutionPolicy{Bindings: map[string]ExecutionBinding{"native": initial}, Routes: map[string]ExecutionRoute{"default": {Initial: "native"}}}
	binding, id := c.InitialExecutionBinding("default")
	require.Equal(t, "native", id)
	require.Equal(t, initial, binding)
	require.Equal(t, "gemini-4-hydrogen-high", c.Routes["default"].Model)
	delete(c.ExecutionPolicy.Routes, "default")
	binding, id = c.InitialExecutionBinding("default")
	require.Empty(t, id)
	require.Equal(t, "high", binding.Thinking)
	require.Equal(t, "danger-full-access", binding.Permission)
	require.Equal(t, c.Sessions.ModeForRole("execute"), binding.SessionMode)
	c.ExecutionPolicy = nil
	withoutPolicy, id := c.InitialExecutionBinding("default")
	require.Empty(t, id)
	require.Equal(t, binding, withoutPolicy)
}

func TestConfigurationSourceScopeIsTheSelectedSingleFile(t *testing.T) {
	root, task := t.TempDir(), t.TempDir()
	for scope, file := range map[string]string{"embedded": "(embedded legal table)", "task": filepath.Join(task, "flywheel.config.json"), "project": filepath.Join(root, "flywheel.config.json"), "global": filepath.Join(t.TempDir(), "settings.json")} {
		c := Config{File: file}
		require.Equal(t, scope, c.ConfigurationScope(root, task))
	}
}

func TestAntigravitySwarmIsRejectedAtNonExecuteReferences(t *testing.T) {
	makeConfig := func(mode string) Config {
		c := Default()
		c.NativeProfiles = map[string]NativeProfile{"agy": {Harness: "antigravity", Options: json.RawMessage(`{"version":1,"mode":"` + mode + `"}`)}}
		c.Profiles = map[string]ModelProfile{"native": {Harness: "antigravity", NativeProfile: "agy"}}
		return c
	}
	for _, role := range []string{"plan", "design-plan", "design-review", "plan-review", "review", "judge", "investigate", "intake", "garden"} {
		c := makeConfig("swarm")
		c.Loops = map[string]LoopBinding{role: {Harness: "antigravity", NativeProfile: "agy"}}
		require.ErrorContains(t, c.validateNativeProfiles(), "approved Execute", role)
	}
	c := makeConfig("swarm")
	c.JudgePanel = &LoopBinding{Harness: "antigravity", NativeProfile: "agy"}
	require.ErrorContains(t, c.validateNativeProfiles(), "judge-panel")
	c.JudgePanel = nil
	c.Planning.Compact = map[string]string{"design-plan": "native"}
	require.ErrorContains(t, c.validateNativeProfiles(), "planning.compact.design-plan")
	c.Planning.Compact = map[string]string{"design-plan": "missing"}
	require.NoError(t, c.validateNativeProfiles(), "the planning validator owns unknown model-profile references")
	c.Planning.Compact = nil
	c.Loops = map[string]LoopBinding{"execute": {Harness: "antigravity", NativeProfile: "agy"}}
	c.Routes = map[string]RouteBinding{"default": {Harness: "antigravity", NativeProfile: "agy"}}
	c.ExecutionPolicy = &ExecutionPolicy{Bindings: map[string]ExecutionBinding{"swarm": {Harness: "antigravity", NativeProfile: "agy"}}}
	require.NoError(t, c.validateNativeProfiles(), "Execute eligibility remains the packet/driver contract")
	for _, mode := range []string{"native", "subagents"} {
		c := makeConfig(mode)
		c.Planning.Compact = map[string]string{"design-plan": "native"}
		c.Loops = map[string]LoopBinding{"plan": {Harness: "antigravity", NativeProfile: "agy"}}
		require.NoError(t, c.validateNativeProfiles())
	}
}

func TestEditorRejectsNonExecuteSwarmBeforePublishingTheFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "flywheel.config.json")
	original := []byte(`{"native-profiles":{"swarm":{"harness":"antigravity","options":{"version":1,"mode":"swarm"}}},"channels":{"google":{"harness":"antigravity","env-key":null}},"routes":{"default":{"harness":"antigravity","channel":"google","model":"gemini-4-neon-high","thinking":"high"}}}`)
	require.NoError(t, os.WriteFile(file, original, 0600))
	doc, err := ReadEditor(root, "project")
	require.NoError(t, err)
	_, err = SaveEditor(root, EditorRequest{Scope: "project", Revision: doc.Revision, Values: map[string]json.RawMessage{
		"loops": json.RawMessage(`{"review":{"harness":"antigravity","channel":"google","model":"gemini-4-neon-high","thinking":"high","native-profile":"swarm"}}`),
	}})
	require.ErrorContains(t, err, "approved Execute")
	stored, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, original, stored, "an invalid role selection must not be published or broaden permissions")
}
