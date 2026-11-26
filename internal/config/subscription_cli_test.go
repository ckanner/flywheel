package config

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func subscriptionFixture() Config {
	c := Default()
	c.Subscription = &SubscriptionPolicy{Version: 1, Mode: "subscription-only"}
	c.Channels = map[string]Channel{"google": {Harness: "antigravity"}, "chatgpt": {Harness: "codex-pro"}, "claude": {Harness: "claude-code-pro"}}
	c.Loops = map[string]LoopBinding{"plan": {Harness: "claude-code-pro", Model: "claude-opus-5-5", Channel: "claude"}}
	c.Routes = map[string]RouteBinding{"default": {Harness: "antigravity", Model: "gemini-4-hydrogen-high", Thinking: "high", Channel: "google"}}
	c.Profiles = map[string]ModelProfile{
		"local":  {Harness: "antigravity", Model: "gemini-4-hydrogen-medium", Channel: "google", Thinking: "medium"},
		"review": {Harness: "codex-pro", Model: "gpt-6.2-sol", Channel: "chatgpt", Thinking: "medium"},
	}
	c.Planning.Compact = map[string]string{"design-plan": "local", "design-review": "review"}
	return c
}

func TestSubscriptionPlanningResolvesProfilesAndRetainsRole(t *testing.T) {
	c := subscriptionFixture()
	require.NoError(t, c.Validate())
	b, source, ok := c.PlanningBindingForRole("design-plan", "compact")
	require.True(t, ok)
	require.Equal(t, "planning.compact.design-plan", source)
	require.Equal(t, "gemini-4-hydrogen-medium", b.Model)
	b, source, ok = c.PlanningBindingForRole("design-plan", "system")
	require.True(t, ok)
	require.Equal(t, "plan", source)
	require.Equal(t, "claude-opus-5-5", b.Model)
	_, _, ok = c.PlanningBindingForRole("investigate", "compact")
	require.False(t, ok)
	c.Planning.Compact["design-plan"] = "absent"
	_, _, ok = c.PlanningBindingForRole("design-plan", "compact")
	require.False(t, ok)
	require.ErrorContains(t, c.Validate(), "unknown profile")
	c.Planning.Compact = map[string]string{"plan": "local"}
	require.Error(t, c.validatePlanningConfig())

	c = subscriptionFixture()
	c.Planning.DualDraft = &DualDraftConfig{Enabled: true, SecondaryProfile: "local", SynthesizerProfile: "review"}
	c.FrontendUXReview = FrontendUXReviewConfig{Enabled: true, Profile: "local"}
	require.NoError(t, c.Validate())

	c.Planning.DualDraft.SecondaryProfile = "absent"
	require.ErrorContains(t, c.Validate(), "planning.dual-draft.secondary-profile")
	c.Planning.DualDraft.SecondaryProfile = "local"
	c.Planning.DualDraft.SynthesizerProfile = "absent"
	require.ErrorContains(t, c.Validate(), "planning.dual-draft.synthesizer-profile")
	c.Planning.DualDraft.SynthesizerProfile = ""
	require.NoError(t, c.Validate())
}

func TestSubscriptionPolicyRejectsReachablePaidPaths(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"loop": func(c *Config) {
			c.Loops["plan"] = LoopBinding{Harness: "dsh", Provider: "deepseek-account", Channel: "paid"}
		},
		"route":   func(c *Config) { c.Routes["default"] = RouteBinding{Harness: "pi", Channel: "paid"} },
		"compact": func(c *Config) { p := c.Profiles["local"]; p.Harness = "pi"; c.Profiles["local"] = p },
		"dual-draft": func(c *Config) {
			c.Profiles["paid"] = ModelProfile{Harness: "pi", Model: "m", Channel: "paid"}
			c.Planning.DualDraft = &DualDraftConfig{Enabled: true, SecondaryProfile: "paid"}
		},
		"ux-review": func(c *Config) {
			c.Profiles["paid"] = ModelProfile{Harness: "pi", Model: "m", Channel: "paid"}
			c.FrontendUXReview = FrontendUXReviewConfig{Enabled: true, Profile: "paid"}
		},
		"panel": func(c *Config) { c.JudgePanel = &LoopBinding{Harness: "pi", Channel: "paid"} },
		"policy": func(c *Config) {
			c.ExecutionPolicy = &ExecutionPolicy{Bindings: map[string]ExecutionBinding{"paid": {Harness: "pi"}}}
		},
		"key":      func(c *Config) { c.Channels["google"] = Channel{EnvKey: new("GEMINI_API_KEY")} },
		"provider": func(c *Config) { c.Channels["google"] = Channel{Provider: "deepseek-account"} },
		"harness":  func(c *Config) { c.Channels["google"] = Channel{Harness: "dsh"} },
		"missing":  func(c *Config) { delete(c.Channels, "google") },
		"empty":    func(c *Config) { b := c.Routes["default"]; b.Channel = ""; c.Routes["default"] = b },
		"cycle":    func(c *Config) { c.Channels["google"] = Channel{Fallback: "google"} },
		"fallback": func(c *Config) {
			c.Channels["google"] = Channel{Fallback: "paid"}
			c.Channels["paid"] = Channel{EnvKey: new("API_KEY")}
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := subscriptionFixture()
			mutate(&c)
			require.Error(t, c.validateSubscriptionCLI())
		})
	}
	c := subscriptionFixture()
	c.JEV = JEVConfig{Enabled: true, Backend: "http", Endpoint: "https://merouter.play.one2x.ai/v1/systemone", Model: "jev", KeyEnv: "MEROUTER_API_KEY"}
	require.NoError(t, c.validateSubscriptionCLI(), "explicit HTTP JEV endpoint may coexist with subscription-only workflow bindings")
	c.JudgePanel = &LoopBinding{Harness: "codex-pro", Channel: "chatgpt"}
	c.ExecutionPolicy = &ExecutionPolicy{Bindings: map[string]ExecutionBinding{"allowed": {Harness: "codex-pro", Channel: "chatgpt"}}}
	c.Channels["google"] = Channel{Fallback: "other-google"}
	c.Channels["other-google"] = Channel{Harness: "antigravity"}
	c.Profiles["unused-paid"] = ModelProfile{Harness: "pi", Model: "unrelated", Channel: "paid"}
	require.NoError(t, c.validateSubscriptionCLI(), "unused paid profile is not a fallback")
	for _, b := range []LoopBinding{{Harness: "dsh", Provider: "chatgpt", Channel: "dsh"}, {Harness: "dsh", Channel: "dsh"}, {Harness: "dsh", Channel: "chatgpt"}} {
		c.Channels["dsh"] = Channel{Harness: "dsh", Provider: "chatgpt"}
		c.Channels["chatgpt"] = Channel{}
		require.ErrorContains(t, c.validateSubscriptionBinding(b, "test"), "verified subscription admission")
	}
	c.Subscription = nil
	c.Loops = map[string]LoopBinding{"plan": {Harness: "dsh", Provider: "chatgpt", Model: "gpt-6.2-sol", Channel: "dsh"}}
	require.NoError(t, c.validateSubscriptionCLI(), "legacy DSH ChatGPT remains supported outside strict subscription policy")
}

func TestSubscriptionSectionsAreVersionedAndStrict(t *testing.T) {
	for _, raw := range []string{
		`{"subscription":null}`, `{"subscription":[]}`, `{"subscription":{"version":1,"mode":"api"}}`,
		`{"subscription":{"version":1,"mode":"subscription-only","verified":true}}`,
		`{"planning":{"unknown":true}}`, `{"planning":{"compact":null}}`,
		`{"planning":{"compact":{"design-plan":"local"}}}`,
	} {
		_, err := parseConfig([]byte(raw), "fixture")
		require.Error(t, err, raw)
	}
	c := subscriptionFixture()
	b, err := json.Marshal(c)
	require.NoError(t, err)
	parsed, err := parseConfig(b, "fixture")
	require.NoError(t, err)
	require.Equal(t, c.Planning, parsed.Planning)
	c.Subscription.Version = 2
	require.Error(t, c.Validate())
	c.Subscription = nil
	c.Planning.Compact = nil
	require.NoError(t, c.validateSubscriptionCLI())
	previous := subscriptionCLISchema
	t.Cleanup(func() { subscriptionCLISchema = previous })
	subscriptionCLISchema = func() (*jsonschema.Schema, error) { return nil, errors.New("fixture schema failure") }
	require.ErrorContains(t, validateSubscriptionSections(nil), "fixture schema failure")
}

func TestAntigravityModelEffortIsExplicitAndConsistent(t *testing.T) {
	for _, family := range []string{"hydrogen", "neon", "carbon", "argon"} {
		require.NoError(t, ValidateAntigravityBinding("gemini-4-"+family+"-high", "high"))
	}
	require.NoError(t, ValidateAntigravityBinding("gemini-4-neon-xhigh", "xhigh"))
	for _, model := range []string{"auto", "gemini-3-carbon-high", "gemini-4-unknown-high", "gemini-4-carbon-xhigh", "gemini-4-hydrogen-max", "gemini-4-neon-medium"} {
		require.Error(t, ValidateAntigravityBinding(model, "high"), model)
	}
	for _, section := range []string{"loops", "routes", "profiles"} {
		raw := `{"` + section + `":{"plan":{"harness":"antigravity","model":"gemini-4-carbon-xhigh","channel":"google","thinking":"xhigh"}}}`
		_, err := parseConfig([]byte(raw), "fixture")
		require.ErrorContains(t, err, "supported thinking", section)
	}
}

func TestPlanningClarifyAndUIPrototypeConfigValidation(t *testing.T) {
	base := func() Config {
		c := subscriptionFixture()
		c.Profiles["clarifier"] = ModelProfile{Harness: "antigravity", Channel: "google", Model: "gemini-4-carbon-high", Thinking: "high"}
		c.Profiles["ux-proto"] = ModelProfile{Harness: "antigravity", Channel: "google", Model: "gemini-4-neon-high", Thinking: "high"}
		c.Profiles["paid-api"] = ModelProfile{Harness: "dsh", Channel: "paid", Model: "m"}
		return c
	}

	okCfg := base()
	okCfg.Planning.Clarify = &ClarifyConfig{Mode: "auto", Profile: "clarifier"}
	okCfg.Planning.UIPrototype = &UIPrototypeConfig{Mode: "auto", Profile: "ux-proto", ReviewProfile: "clarifier"}
	require.NoError(t, okCfg.Validate())

	badClarify := base()
	badClarify.Planning.Clarify = &ClarifyConfig{Mode: "auto", Profile: "missing"}
	require.ErrorContains(t, badClarify.Validate(), "planning.clarify.profile")

	badClarifySub := base()
	badClarifySub.Planning.Clarify = &ClarifyConfig{Mode: "auto", Profile: "paid-api"}
	require.ErrorContains(t, badClarifySub.Validate(), "planning.clarify.profile")

	badProto := base()
	badProto.Planning.UIPrototype = &UIPrototypeConfig{Mode: "always", Profile: "missing"}
	require.ErrorContains(t, badProto.Validate(), "planning.ui-prototype.profile")

	badProtoRev := base()
	badProtoRev.Planning.UIPrototype = &UIPrototypeConfig{Mode: "always", Profile: "ux-proto", ReviewProfile: "missing"}
	require.ErrorContains(t, badProtoRev.Validate(), "planning.ui-prototype.review-profile")

	badProtoSub := base()
	badProtoSub.Planning.UIPrototype = &UIPrototypeConfig{Mode: "always", Profile: "paid-api"}
	require.ErrorContains(t, badProtoSub.Validate(), "planning.ui-prototype.profile")

	// Empty review-profile path inside subscription validation loop
	okOnlyProto := base()
	okOnlyProto.Planning.UIPrototype = &UIPrototypeConfig{Mode: "always", Profile: "ux-proto"}
	require.NoError(t, okOnlyProto.Validate())
}
