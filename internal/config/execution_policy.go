package config

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// ExecutionPolicy is explicitly enabled per route. Its switch budget is
// additional to, never a replacement for, the task and packet budgets.
type ExecutionPolicy struct {
	Version  int                         `json:"version"`
	Bindings map[string]ExecutionBinding `json:"bindings"`
	Routes   map[string]ExecutionRoute   `json:"routes"`
}

type ExecutionBinding struct {
	NativeProfile string `json:"native-profile,omitempty"`
	Harness       string `json:"harness"`
	Provider      string `json:"provider"`
	Channel       string `json:"channel"`
	Model         string `json:"model"`
	Thinking      string `json:"thinking"`
	Permission    string `json:"permission"`
	SessionMode   string `json:"session-mode"`
}

type ExecutionRoute struct {
	Initial              string            `json:"initial"`
	Allowed              []string          `json:"allowed"`
	QualityOrder         []string          `json:"quality-order"`
	ChannelFallbacks     map[string]string `json:"channel-fallbacks,omitempty"`
	AfterStalledAttempts int               `json:"after-stalled-attempts"`
	MaxSwitches          int               `json:"max-switches"`
}

var executionPolicySchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compilePolicySchema("execution-policy.schema.json", templates.FS.ReadFile)
})

func validateExecutionPolicy(value any) error {
	schema, err := executionPolicySchema()
	if err != nil {
		return fmt.Errorf("execution-policy schema: %w", err)
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("execution-policy: %w", err)
	}
	return nil
}

func prepareExecutionPolicy(root map[string]json.RawMessage) error {
	raw, present := root["execution-policy"]
	if !present {
		return nil
	}
	object, err := configObject(raw, "execution-policy")
	if err != nil {
		return err
	}
	var value any
	_ = json.Unmarshal(configJSON(object), &value)
	if err := validateExecutionPolicy(value); err != nil {
		return err
	}
	root["execution-policy"] = configJSON(object)
	return nil
}

func (c *Config) validateExecutionPolicy() error {
	p := c.ExecutionPolicy
	if p == nil {
		return nil
	}
	var value any
	_ = json.Unmarshal(configJSON(p), &value)
	if err := validateExecutionPolicy(value); err != nil {
		return err
	}
	for id, binding := range p.Bindings {
		if _, found := c.Channels[binding.Channel]; !found {
			return fmt.Errorf("execution-policy.bindings.%s: channel is not configured", id)
		}
		if err := c.validateBindingProvider(binding.Harness, binding.Channel, binding.Provider); err != nil {
			return fmt.Errorf("execution-policy.bindings.%s: %w", id, err)
		}
		if binding.Harness == "dsh" && strings.TrimSpace(binding.Provider) == "" {
			return fmt.Errorf("execution-policy.bindings.%s: DSH requires an explicit provider", id)
		}
		if !executionEffortSupported(binding) {
			return fmt.Errorf("execution-policy.bindings.%s: unsupported thinking value for this integration", id)
		}
	}
	for name, route := range p.Routes {
		baseline, found := c.Routes[name]
		if !found {
			return fmt.Errorf("execution-policy.routes.%s: route is not configured", name)
		}
		for _, id := range route.Allowed {
			binding, found := p.Bindings[id]
			if !found {
				return fmt.Errorf("execution-policy.routes.%s: unknown binding %q", name, id)
			}
			if binding.Permission != c.PermissionMode("execute", baseline.Harness, "routes."+name) {
				return fmt.Errorf("execution-policy.routes.%s: binding %s changes the route's authorized permission", name, id)
			}
		}
		if !slices.Contains(route.Allowed, route.Initial) {
			return fmt.Errorf("execution-policy.routes.%s: initial binding is not allowed", name)
		}
		for _, id := range route.QualityOrder {
			if !slices.Contains(route.Allowed, id) {
				return fmt.Errorf("execution-policy.routes.%s: quality-order binding %s is not allowed", name, id)
			}
		}
		for from, to := range route.ChannelFallbacks {
			if from == to || !slices.Contains(route.Allowed, from) || !slices.Contains(route.Allowed, to) {
				return fmt.Errorf("execution-policy.routes.%s: fallback endpoints must be distinct allowed bindings", name)
			}
			a, b := p.Bindings[from], p.Bindings[to]
			channel := c.Channels[b.Channel]
			if c.Channels[a.Channel].Fallback != b.Channel || channel.Models[name] != b.Model || channel.Harness != b.Harness {
				return fmt.Errorf("execution-policy.routes.%s: fallback %s -> %s must match the configured channel chain and model mapping", name, from, to)
			}
		}
	}
	return nil
}

// These are the documented adapter request vocabularies. Backends still verify
// model availability and actual admission; these values are not quality ranks.
func executionEffortSupported(b ExecutionBinding) bool {
	values := []string{"low", "medium", "high", "xhigh", "max"}
	if b.Harness == "kimi" {
		values = []string{"low", "high", "max"}
	} else if b.Harness == "dsh" && strings.HasPrefix(b.Provider, "deepseek") {
		values = []string{"off", "low", "high", "max"}
	}
	return slices.Contains(values, b.Thinking)
}
