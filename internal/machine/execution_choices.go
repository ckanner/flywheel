package machine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
)

// Selection does not grant capabilities. The same admission rule applies to
// configured initial bindings, optional decisions and deterministic upgrades.
func (c *Context) executionCandidateError(packet *contract.Packet, b config.ExecutionBinding) error {
	for _, condition := range c.executionCandidateConditions(packet, b) {
		if !condition.Passed {
			return fmt.Errorf("%s", condition.Message)
		}
	}
	return nil
}

// Execution and the console consume the same deterministic input checks. These
// checks do not probe account entitlement or imply that a model has run.
func (c *Context) executionCandidateConditions(packet *contract.Packet, b config.ExecutionBinding) []EligibilityCondition {
	checks := []EligibilityCondition{{Code: "image-input", Passed: packet.InputImages == nil || b.Harness == "claude-code-pro", Message: "No original image input is required, or the selected Claude transport can receive it."}}
	if !checks[0].Passed {
		checks[0].Message = fmt.Sprintf("packet %s requires a Claude image-capable execution binding", packet.PacketID)
	}
	mode := ""
	if b.NativeProfile != "" {
		profile, exists := c.Config.NativeProfiles[b.NativeProfile]
		var options struct {
			Mode string `json:"mode"`
		}
		valid := exists && profile.Harness == b.Harness && json.Unmarshal(profile.Options, &options) == nil
		checks = append(checks, EligibilityCondition{Code: "native-profile", Passed: valid, Message: "The selected native profile matches the execution backend."})
		if !valid {
			checks[len(checks)-1].Message = fmt.Sprintf("packet %s has an invalid native profile for execution", packet.PacketID)
			return checks
		}
		mode = options.Mode
	}
	if b.Harness == "antigravity" && mode == "swarm" {
		checks = append(checks,
			EligibilityCondition{Code: "full-workflow", Passed: !c.quick(), Message: "Antigravity Swarm requires Full and an approved collaboration contract without images"},
			EligibilityCondition{Code: "collaboration-declared", Passed: packet.Collaboration != nil, Message: "Antigravity Swarm requires Full and an approved collaboration contract without images"})
		if packet.Collaboration != nil {
			check := EligibilityCondition{Code: "independent-writes", Passed: true, Message: "The collaboration contains 2–8 independent assignments within the packet write scopes."}
			if err := contract.ValidateCollaboration(packet); err != nil {
				check.Passed, check.Message = false, err.Error()
			}
			checks = append(checks, check)
		}
	}
	return checks
}

// The public candidate explains the execution strategy without disclosing
// arbitrary native prompts, tool configuration or credentials. Complete native
// options remain part of the frozen execution-policy identity.
func (c *Context) executionChoiceDescription(id string, b config.ExecutionBinding) string {
	strategy := "Use the specified model and reasoning allocation for implementation."
	switch {
	case strings.HasPrefix(b.Model, "gemini-pro-orchestrator"):
		strategy = "Use native planner and Hydrogen implementation hand-offs in one conversation; reasoning is fixed by the native model."
	case strings.Contains(b.Model, "hydrogen"):
		strategy = "Configured for bounded implementation with frequent deterministic feedback."
	case strings.Contains(b.Model, "neon"):
		strategy = "Configured for coordinated changes across modules and more involved engineering."
	case strings.Contains(b.Model, "carbon"), strings.Contains(b.Model, "astra"):
		strategy = "Configured for difficult reasoning, complex defects and more demanding implementation tasks."
	case strings.Contains(b.Model, "sol"):
		strategy = "Configured for backend implementation, debugging and tool-driven code changes."
	case strings.Contains(b.Model, "opus"):
		strategy = "Configured for frontend and visual implementation; this Claude binding supports approved image input."
	case strings.Contains(b.Model, "kimi"):
		strategy = "Configured for repetitive implementation and coordinated batch edits within reviewed scope."
	}
	if b.NativeProfile != "" {
		var options struct {
			Mode string `json:"mode"`
		}
		_ = json.Unmarshal(c.Config.NativeProfiles[b.NativeProfile].Options, &options)
		switch options.Mode {
		case "swarm":
			strategy = "Use a native Swarm for the approved independent collaboration tasks, with shared coordination and verification."
		case "subagents":
			strategy += " Delegate bounded sub-tasks to the configured native subagents when useful."
		default:
			strategy += " Use only the explicitly configured native capabilities."
		}
		if b.Harness == "antigravity" {
			var native config.AntigravityNative
			_ = json.Unmarshal(c.Config.NativeProfiles[b.NativeProfile].Options, &native)
			public, _ := json.Marshal(native)
			strategy += " Native options: " + string(public) + ". Child lifecycle and usage may be incomplete; an observed-agent limit is not a hard concurrency cap."
		}
	}
	return boundedDecisionText(fmt.Sprintf("Approved binding %s: harness=%s provider=%s channel=%s model=%s effort=%s permission=%s session=%s native-profile=%s. %s Switching requires a cold start and adds no execution attempts or budget.", id, b.Harness, b.Provider, b.Channel, b.Model, b.Thinking, b.Permission, b.SessionMode, b.NativeProfile, strategy), 1024)
}

// Live optional decisions also freeze the referenced channel and native
// policies. Editing an unrelated native profile does not invalidate a choice.
func executionChoiceConfigurationHash(cfg *config.Config) string {
	channels := map[string]config.Channel{}
	native := map[string]config.NativeProfile{}
	if cfg.ExecutionPolicy != nil {
		for _, route := range cfg.ExecutionPolicy.Routes {
			for _, id := range route.Allowed {
				b := cfg.ExecutionPolicy.Bindings[id]
				channels[b.Channel] = cfg.Channels[b.Channel]
				if b.NativeProfile != "" {
					native[b.NativeProfile] = cfg.NativeProfiles[b.NativeProfile]
				}
			}
		}
	}
	return sessionHash([]any{cfg.ExecutionPolicy, channels, native})
}
