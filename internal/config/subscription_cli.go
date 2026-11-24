package config

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// SubscriptionPolicy freezes the requested billing path. Native adapters must
// independently verify effective authentication and extra-usage controls.
// Merely loading this policy is not an account entitlement attestation.
type SubscriptionPolicy struct {
	Version int    `json:"version"`
	Mode    string `json:"mode"`
}

// DualDraftConfig configures blind dual-model drafting plus synthesis for Full root designs.
type DualDraftConfig struct {
	Enabled            bool   `json:"enabled"`
	SecondaryProfile   string `json:"secondary-profile,omitempty"`
	SynthesizerProfile string `json:"synthesizer-profile,omitempty"`
}

// Compact planning reuses named profiles without introducing another loop.
type PlanningConfig struct {
	Compact   map[string]string `json:"compact,omitempty"`
	DualDraft *DualDraftConfig  `json:"dual-draft,omitempty"`
}

var subscriptionCLISchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compilePolicySchema("subscription-cli.schema.json", templates.FS.ReadFile)
})

func validateSubscriptionSections(sections map[string]json.RawMessage) error {
	schema, err := subscriptionCLISchema()
	if err != nil {
		return err
	}
	var value any
	_ = json.Unmarshal(configJSON(sections), &value)
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("subscription/planning: %w", err)
	}
	return nil
}

func profileAsLoopBinding(p ModelProfile) LoopBinding {
	return LoopBinding{
		Harness:       p.Harness,
		Model:         p.Model,
		Channel:       p.Channel,
		Provider:      p.Provider,
		Thinking:      p.Thinking,
		NativeProfile: p.NativeProfile,
	}
}

func (c *Config) PlanningBindingForRole(role, granularity string) (LoopBinding, string, bool) {
	if granularity == "compact" {
		if name, ok := c.Planning.Compact[role]; ok {
			p, found := c.Profiles[name]
			return profileAsLoopBinding(p), "planning.compact." + role, found
		}
	}
	return c.LoopBindingForRole(role)
}

func (c *Config) validatePlanningConfig() error {
	if len(c.Planning.Compact) > 0 {
		if len(c.Planning.Compact) != 2 || c.Planning.Compact["design-plan"] == "" || c.Planning.Compact["design-review"] == "" {
			return fmt.Errorf("planning.compact requires exactly design-plan and design-review profile references")
		}
		for role, name := range c.Planning.Compact {
			if _, ok := c.Profiles[name]; !ok {
				return fmt.Errorf("planning.compact.%s references unknown profile %q", role, name)
			}
		}
	}
	if d := c.Planning.DualDraft; d != nil && d.Enabled {
		if _, ok := c.Profiles[d.SecondaryProfile]; !ok {
			return fmt.Errorf("planning.dual-draft.secondary-profile references unknown profile %q", d.SecondaryProfile)
		}
		if d.SynthesizerProfile != "" {
			if _, ok := c.Profiles[d.SynthesizerProfile]; !ok {
				return fmt.Errorf("planning.dual-draft.synthesizer-profile references unknown profile %q", d.SynthesizerProfile)
			}
		}
	}
	return nil
}

func prepareSubscriptionCLI(root map[string]json.RawMessage) error {
	sections := map[string]json.RawMessage{}
	for _, section := range []string{"planning", "subscription"} {
		raw, present := root[section]
		if !present {
			continue
		}
		object, err := configObject(raw, section)
		if err != nil {
			return err
		}
		fields := []string{"version", "mode"}
		if section == "planning" {
			fields = []string{"compact", "dual-draft"}
		}
		if err := checkConfigFields(object, section, fields); err != nil {
			return err
		}
		root[section] = configJSON(object)
		sections[section] = root[section]
	}
	return validateSubscriptionSections(sections)
}

// Validate every reachable binding, including configured fallbacks. Unused API
// profiles may coexist for other configurations; this graph cannot reach them.
func (c *Config) validateSubscriptionCLI() error {
	sections := map[string]json.RawMessage{"planning": configJSON(c.Planning)}
	if c.Subscription != nil {
		sections["subscription"] = configJSON(c.Subscription)
	}
	if err := validateSubscriptionSections(sections); err != nil {
		return err
	}
	if err := c.validatePlanningConfig(); err != nil {
		return err
	}
	if c.Subscription == nil {
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(c.Loops)) {
		if err := c.validateSubscriptionBinding(c.Loops[name], "loops."+name); err != nil {
			return err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(c.Routes)) {
		b := c.Routes[name]
		if err := c.validateSubscriptionBinding(LoopBinding{Harness: b.Harness, Model: b.Model, Channel: b.Channel, Provider: b.Provider}, "routes."+name); err != nil {
			return err
		}
	}
	for role := range c.Planning.Compact {
		b, source, _ := c.PlanningBindingForRole(role, "compact")
		if err := c.validateSubscriptionBinding(b, source); err != nil {
			return err
		}
	}
	if d := c.Planning.DualDraft; d != nil && d.Enabled {
		for _, pair := range [][2]string{
			{"planning.dual-draft.secondary-profile", d.SecondaryProfile},
			{"planning.dual-draft.synthesizer-profile", d.SynthesizerProfile},
		} {
			if pair[1] == "" {
				continue
			}
			p := c.Profiles[pair[1]]
			if err := c.validateSubscriptionBinding(profileAsLoopBinding(p), pair[0]); err != nil {
				return err
			}
		}
	}
	if c.FrontendUXReview.Enabled {
		if p, ok := c.Profiles[c.FrontendUXReview.Profile]; ok {
			if err := c.validateSubscriptionBinding(profileAsLoopBinding(p), "frontend-ux-review.profile"); err != nil {
				return err
			}
		}
	}
	if c.JudgePanel != nil {
		if err := c.validateSubscriptionBinding(*c.JudgePanel, "judge-panel"); err != nil {
			return err
		}
	}
	if c.ExecutionPolicy != nil {
		for name, b := range c.ExecutionPolicy.Bindings {
			if err := c.validateSubscriptionBinding(LoopBinding{Harness: b.Harness, Model: b.Model, Channel: b.Channel, Provider: b.Provider}, "execution-policy.bindings."+name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Config) validateSubscriptionBinding(b LoopBinding, location string) error {
	// The DSH ChatGPT provider is a legacy supported integration, but its
	// worker has no subscription-only billing admission/receipt contract.
	if !slices.Contains([]string{"antigravity", "kimi", "codex-pro", "claude-code-pro"}, b.Harness) || b.Provider != "" {
		return fmt.Errorf("%s is not a native harness with verified subscription admission", location)
	}
	seen := map[string]bool{}
	harness := b.Harness
	for channel := b.Channel; ; {
		if strings.TrimSpace(channel) == "" || seen[channel] {
			return fmt.Errorf("%s has an empty or cyclic subscription channel", location)
		}
		seen[channel] = true
		ch, ok := c.Channels[channel]
		if !ok || ch.EnvKey != nil && *ch.EnvKey != "" || ch.Provider != "" || channel == b.Channel && ch.Harness != "" && ch.Harness != b.Harness {
			return fmt.Errorf("%s channel %q conflicts with subscription-only access", location, channel)
		}
		if ch.Harness != "" {
			harness = ch.Harness
		}
		if !slices.Contains([]string{"antigravity", "kimi", "codex-pro", "claude-code-pro"}, harness) {
			return fmt.Errorf("%s fallback channel %q is not a declared subscription binding", location, channel)
		}
		if ch.Fallback == "" {
			return nil
		}
		channel = ch.Fallback
	}
}
