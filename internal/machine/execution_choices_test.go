package machine

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/imageinput"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func collaborationFixture(packet *contract.Packet) {
	packet.Schedule = &contract.PacketSchedule{After: []string{}, Writes: []string{"build/"}}
	packet.Collaboration = &contract.PacketCollaboration{Version: 1, Tasks: []contract.CollaborationTask{
		{Goal: "Implement the first independent unit", Writes: []string{"build/first/"}},
		{Goal: "Implement the second independent unit", Writes: []string{"build/second/"}},
	}}
}

func addSwarmChoice(c *Context) {
	if c.Config.NativeProfiles == nil {
		c.Config.NativeProfiles = map[string]config.NativeProfile{}
	}
	c.Config.NativeProfiles["swarm"] = config.NativeProfile{Harness: "antigravity", Options: json.RawMessage(`{"version":1,"mode":"swarm","maxObservedAgents":6}`)}
	c.Config.Channels["google-native"] = config.Channel{Harness: "antigravity"}
	c.Config.ExecutionPolicy.Bindings["swarm"] = config.ExecutionBinding{NativeProfile: "swarm", Harness: "antigravity", Channel: "google-native", Model: "gemini-4-neon-high", Thinking: "high", Permission: "workspace-write", SessionMode: "scoped"}
	route := c.Config.ExecutionPolicy.Routes["default"]
	route.Allowed = append(route.Allowed, "swarm")
	c.Config.ExecutionPolicy.Routes["default"] = route
}

func TestExecutionCandidatesRequireReviewedSwarmScopeAndImageCapability(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	addSwarmChoice(c)
	p := c.Packets["M01-T01"]
	swarm := c.Config.ExecutionPolicy.Bindings["swarm"]
	require.ErrorContains(t, c.executionCandidateError(p, swarm), "approved collaboration")
	collaborationFixture(p)
	require.NoError(t, c.executionCandidateError(p, swarm))
	p.Collaboration.Tasks[1].Writes = []string{"build/first/nested"}
	require.ErrorContains(t, c.executionCandidateError(p, swarm), "overlaps")
	collaborationFixture(p)
	c.State.Workflow = &state.WorkflowState{WorkflowSelection: contract.WorkflowSelection{Effective: "quick"}}
	require.ErrorContains(t, c.executionCandidateError(p, swarm), "Full")
	c.State.Workflow = nil
	p.InputImages = &imageinput.References{}
	require.ErrorContains(t, c.executionCandidateError(p, swarm), "image-capable")
	require.NoError(t, c.executionCandidateError(p, config.ExecutionBinding{Harness: "claude-code-pro"}))
	p.InputImages = nil
	for _, raw := range []string{`{`, `{"version":1,"mode":"subagents"}`} {
		c.Config.NativeProfiles["swarm"] = config.NativeProfile{Harness: "antigravity", Options: json.RawMessage(raw)}
		err := c.executionCandidateError(p, swarm)
		if raw == `{` {
			require.ErrorContains(t, err, "invalid native profile")
		} else {
			p.Collaboration = nil
			require.NoError(t, c.executionCandidateError(p, swarm), "ordinary delegation does not require a write decomposition")
		}
	}
	delete(c.Config.NativeProfiles, "swarm")
	require.Error(t, c.executionCandidateError(p, swarm))
	c.Config.NativeProfiles["swarm"] = config.NativeProfile{Harness: "codex-pro", Options: json.RawMessage(`{}`)}
	require.Error(t, c.executionCandidateError(p, swarm))
}

func TestInitialAndFailureCandidateSetsApplyTheSameCapabilities(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	addSwarmChoice(c)
	p := c.Packets["M01-T01"]
	initial, baseline, err := c.initialExecutionActions(p)
	require.NoError(t, err)
	require.Len(t, initial, 3)
	require.Equal(t, "model-0", baseline)
	failed, _ := c.failureActions(p, errL1Failed, false)
	for _, action := range failed {
		require.NotEqual(t, "model-3", action.ID)
	}
	policy, route, err := c.executionPolicy(p)
	require.NoError(t, err)
	require.False(t, c.switchExecutionPolicy(p, policy, route, "swarm", "jev-model-selection", "ineligible"))
	collaborationFixture(p)
	initial, _, err = c.initialExecutionActions(p)
	require.NoError(t, err)
	require.Len(t, initial, 4)
	require.Contains(t, initial[3].Description, "approved independent collaboration tasks")
	failed, _ = c.failureActions(p, errL1Failed, false)
	found := false
	for _, action := range failed {
		if action.ID == "model-3" {
			found = true
		}
	}
	require.True(t, found)
	policy.Switches = route.MaxSwitches
	policy.History = make([]state.ExecutionSwitch, route.MaxSwitches)
	initial, _, err = c.initialExecutionActions(p)
	require.NoError(t, err)
	require.Len(t, initial, 1, "an exhausted switch cap leaves only the current binding")
}

func TestStaticExecutionCannotBypassSwarmAdmission(t *testing.T) {
	for _, policyEnabled := range []bool{true, false} {
		t.Run(fmt.Sprint(policyEnabled), func(t *testing.T) {
			_, c := sessionFixture(t)
			enableExecutionPolicy(c)
			addSwarmChoice(c)
			p := c.Packets["M01-T01"]
			if policyEnabled {
				route := c.Config.ExecutionPolicy.Routes["default"]
				route.Initial = "swarm"
				c.Config.ExecutionPolicy.Routes["default"] = route
			} else {
				c.Config.ExecutionPolicy = nil
				c.Config.Routes["default"] = config.RouteBinding{Harness: "antigravity", Channel: "google-native", Model: "gemini-4-neon-high", Thinking: "high", NativeProfile: "swarm"}
			}
			sd := happyDrivers()
			c.Drivers = sessionDriverSet(sd)
			_, err := c.executeStep(p)
			require.ErrorContains(t, err, "approved collaboration")
			require.Empty(t, sd.log.exec)
			collaborationFixture(p)
			binding, _, _, err := c.executionBinding(p)
			require.NoError(t, err)
			require.Equal(t, "swarm", binding.NativeProfile)
		})
	}
}

func TestInitialActionsStayBoundedAndRetainAnyConfiguredBaseline(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	p := c.Packets["M01-T01"]
	route := c.Config.ExecutionPolicy.Routes["default"]
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("extra-%d", i)
		c.Config.ExecutionPolicy.Bindings[id] = c.Config.ExecutionPolicy.Bindings["low"]
		route.Allowed = append(route.Allowed, id)
	}
	route.Initial = "extra-19"
	c.Config.ExecutionPolicy.Routes["default"] = route
	actions, baseline, err := c.initialExecutionActions(p)
	require.NoError(t, err)
	require.Len(t, actions, 16)
	require.Equal(t, "model-22", baseline)
	require.Equal(t, baseline, actions[0].ID)
	c.Config.ExecutionPolicy = nil
	_, _, err = c.initialExecutionActions(p)
	require.ErrorContains(t, err, "no longer configured")
	c.State.Packets[p.PacketID].ExecutionPolicy = nil
	actions, baseline, err = c.initialExecutionActions(p)
	require.NoError(t, err)
	require.Empty(t, actions)
	require.Empty(t, baseline)
}

func TestExecutionChoiceDescriptionsExposeStrategyWithoutPrivatePrompts(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	addSwarmChoice(c)
	for model, expected := range map[string]string{
		"gemini-pro-orchestrator-neon": "Hydrogen implementation hand-offs", "gemini-4-hydrogen-high": "frequent deterministic feedback",
		"gemini-4-neon-high": "across modules", "gemini-4-carbon-high": "difficult reasoning", "gpt-6.2-sol": "backend implementation",
		"gpt-6.2-astra": "difficult reasoning", "claude-opus-5-5": "frontend and visual", "kimi-code/k3.1-pro": "batch edits", "custom": "specified model",
	} {
		b := c.Config.ExecutionPolicy.Bindings["low"]
		b.Model = model
		require.Contains(t, c.executionChoiceDescription("candidate", b), expected)
	}
	b := c.Config.ExecutionPolicy.Bindings["swarm"]
	for mode, expected := range map[string]string{"swarm": "native Swarm", "subagents": "bounded sub-tasks", "native": "explicitly configured native capabilities"} {
		c.Config.NativeProfiles["swarm"] = config.NativeProfile{Harness: "antigravity", Options: json.RawMessage(fmt.Sprintf(`{"version":1,"mode":%q,"maxObservedAgents":6,"private-prompt":"do-not-forward"}`, mode))}
		description := c.executionChoiceDescription("candidate", b)
		require.Contains(t, description, expected)
		require.Contains(t, description, `"maxObservedAgents":6`)
		require.Contains(t, description, "not a hard concurrency cap")
		require.NotContains(t, description, "do-not-forward")
		require.LessOrEqual(t, len([]rune(description)), 1024)
	}
	b.Harness = "kimi-code"
	c.Config.NativeProfiles["swarm"] = config.NativeProfile{Harness: b.Harness, Options: json.RawMessage(`{"system-prompt":"do-not-forward"}`)}
	require.NotContains(t, c.executionChoiceDescription("candidate", b), "do-not-forward")
	before := executionChoiceConfigurationHash(c.Config)
	c.Config.NativeProfiles["unused"] = config.NativeProfile{Harness: "antigravity", Options: json.RawMessage(`{"version":1,"mode":"native"}`)}
	require.Equal(t, before, executionChoiceConfigurationHash(c.Config))
	profile := c.Config.NativeProfiles["swarm"]
	profile.Options = json.RawMessage(strings.ReplaceAll(string(profile.Options), "do-not-forward", "changed"))
	c.Config.NativeProfiles["swarm"] = profile
	require.NotEqual(t, before, executionChoiceConfigurationHash(c.Config), "complete private options remain frozen even though the summary excludes them")
}

func TestExecutionNativeAdmissionRetainsHarnessSpecificHistoryValidation(t *testing.T) {
	_, c, args := nativeKimiHandoffFixture(t, "agent")
	before := c.sessionRecord("execute", args.PacketID).Last.ID
	profile := c.Config.NativeProfiles["native"]
	profile.Options = json.RawMessage(`{"version":2,"mode":"agent"}`)
	c.Config.NativeProfiles["native"] = profile
	_, err := c.runSessionCommand(sessionRequest("claim"))
	require.ErrorContains(t, err, "native Kimi history requires a valid version 1")
	require.Equal(t, before, c.sessionRecord("execute", args.PacketID).Last.ID)
	require.Nil(t, c.State.Handoff)
}
