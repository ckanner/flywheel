package config

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"sync"

	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// JEVConfig is the single authority for optional live JEV decisions. Enabling it
// authorizes the declared bounded data categories, not new executor permissions.
type JEVConfig struct {
	Backend          string              `json:"backend,omitempty"`
	Profile          string              `json:"profile,omitempty"`
	Enabled          bool                `json:"enabled"`
	Endpoint         string              `json:"endpoint,omitempty"`
	Model            string              `json:"model,omitempty"`
	KeyEnv           string              `json:"key-env,omitempty"`
	TimeoutMS        int64               `json:"timeout-ms"`
	MaxCalls         int                 `json:"max-calls"`
	MinApplicability float64             `json:"min-applicability"`
	MinChoice        float64             `json:"min-choice"`
	AllowedData      []decision.Category `json:"allowed-data"`
}

var fixedJEVModel = regexp.MustCompile(`^jev-[0-9]+\.[0-9]+\.[0-9]+$`)

var jevSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compilePolicySchema("jev-config.schema.json", templates.FS.ReadFile)
})

var legacyJEVSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compilePolicySchema("jev-policy.schema.json", templates.FS.ReadFile)
})

func validateJEV(value any) error {
	schema, err := jevSchema()
	if err != nil {
		return err
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("jev: %w", err)
	}
	return nil
}

// Old shadow consent covered local capture only. Preserve connection hints but
// never reinterpret an old enabled flag as authority to send or adopt decisions.
func migrateLegacyJEV(root map[string]json.RawMessage) error {
	raw, present := root["jev-shadow"]
	if !present {
		return nil
	}
	object, err := configObject(raw, "jev-shadow")
	if err != nil {
		return err
	}
	var value any
	_ = json.Unmarshal(configJSON(object), &value)
	schema, err := legacyJEVSchema()
	if err != nil {
		return err
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("jev-shadow: %w", err)
	}
	if _, explicit := root["jev"]; !explicit {
		migrated := map[string]json.RawMessage{"enabled": json.RawMessage("false")}
		for _, key := range []string{"endpoint", "key-env", "timeout-ms"} {
			if val, exists := object[key]; exists {
				migrated[key] = val
			}
		}
		var expected string
		_ = json.Unmarshal(object["expected-model"], &expected)
		if fixedJEVModel.MatchString(expected) {
			migrated["model"] = configJSON(expected)
		}
		root["jev"] = configJSON(migrated)
	}
	delete(root, "jev-shadow")
	return nil
}

func prepareJEV(root map[string]json.RawMessage) error {
	if err := migrateLegacyJEV(root); err != nil {
		return err
	}
	raw, present := root["jev"]
	if !present {
		return nil
	}
	object, err := configObject(raw, "jev")
	if err != nil {
		return err
	}
	var value any
	_ = json.Unmarshal(configJSON(object), &value)
	if err := validateJEV(value); err != nil {
		return err
	}
	root["jev"] = configJSON(object)
	return nil
}

func (j *JEVConfig) normalize() {
	if j.Backend == "" {
		j.Backend = "http"
	} // Existing explicit files retain their HTTP contract.
	if j.Backend == "subscription" && j.Profile == "" {
		j.Profile = "hydrogen-local"
	}
	if j.Model == "" && j.Backend != "subscription" {
		j.Model = "jev-1.14.0"
	}
	if j.KeyEnv == "" && j.Backend != "subscription" {
		j.KeyEnv = "MEROUTER_API_KEY"
	}
	if j.TimeoutMS == 0 {
		j.TimeoutMS = 5000
		if j.Backend == "subscription" {
			j.TimeoutMS = 30000
		}
	}
	if j.MaxCalls == 0 {
		j.MaxCalls = 8
	}
	if j.MinApplicability == 0 {
		j.MinApplicability = 0.8
	}
	if j.MinChoice == 0 {
		j.MinChoice = 0.6
	}
	if j.AllowedData == nil {
		j.AllowedData = []decision.Category{decision.TaskSummary, decision.DiagnosticFacts, decision.Source}
	}
}

func (j JEVConfig) validateTarget() error {
	if !j.Enabled {
		return nil
	}
	if j.Backend == "subscription" {
		return nil
	}
	u, err := url.Parse(j.Endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/v1/systemone" {
		return fmt.Errorf("jev.enabled requires a credential-free System One endpoint ending in /v1/systemone")
	}
	local := u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return fmt.Errorf("jev.endpoint requires HTTPS; HTTP is allowed only for loopback testing")
	}
	return nil
}

// ResolveJEVSubscription freezes only the explicitly named, tool-free native
// profile. No provider fallback or model-selected command is available here.
func (c *Config) ResolveJEVSubscription() (*decision.SubscriptionBinding, error) {
	p, ok := c.Profiles[c.JEV.Profile]
	if !ok || p.Harness != "antigravity" || p.Provider != "" || p.NativeProfile != "" || AntigravityOrchestrator(p.Model) {
		return nil, fmt.Errorf("jev subscription profile must name a tool-free Antigravity binding without native children")
	}
	if err := ValidateAntigravityBinding(p.Model, p.Thinking); err != nil {
		return nil, err
	}
	if err := c.validateSubscriptionBinding(LoopBinding{Harness: p.Harness, Channel: p.Channel, Model: p.Model}, "jev.profile"); err != nil {
		return nil, err
	}
	b := &decision.SubscriptionBinding{Profile: c.JEV.Profile, Harness: p.Harness, Channel: p.Channel, Model: p.Model, Effort: p.Thinking}
	return b, decision.ValidateSubscriptionBinding(b)
}

// DecisionPolicy freezes one effective policy for every supported decision
// point. A copied allowlist prevents later configuration edits mutating it.
func (j JEVConfig) DecisionPolicy() decision.Policy {
	mode := decision.Off
	if j.Enabled {
		mode = decision.Adopt
	}
	return decision.Policy{Version: "jev-v1", Mode: mode, AllowedData: slices.Clone(j.AllowedData),
		MinApplicability: j.MinApplicability, MinChoice: j.MinChoice, MaxCalls: j.MaxCalls}
}
