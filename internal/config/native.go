package config

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// NativeProfile selects an explicit, versioned harness contract. It never
// changes the model, account, permission, or loop acceptance contract.
type NativeProfile struct {
	Harness string          `json:"harness"`
	Options json.RawMessage `json:"options"`
}

var nativeProfilesSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compilePolicySchema("native-profiles.schema.json", templates.FS.ReadFile)
})

func validateNativeProfiles(value any) error {
	schema, err := nativeProfilesSchema()
	if err != nil {
		return err
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("native-profiles: %w", err)
	}
	return nil
}

func prepareNativeProfiles(root map[string]json.RawMessage) error {
	raw, exists := root["native-profiles"]
	if !exists {
		return nil
	}
	var value any
	_ = json.Unmarshal(raw, &value)
	return validateNativeProfiles(value)
}

func (c *Config) validateNativeProfiles() error {
	if c.NativeProfiles != nil {
		var value any
		if err := json.Unmarshal(configJSON(c.NativeProfiles), &value); err != nil {
			return fmt.Errorf("native-profiles: invalid JSON options: %w", err)
		}
		if err := validateNativeProfiles(value); err != nil {
			return err
		}
	}
	check := func(location, harness, name string) error {
		if name == "" {
			return nil
		}
		profile, exists := c.NativeProfiles[name]
		if !exists || profile.Harness != harness {
			return fmt.Errorf("%s.native-profile: %q must name a profile for harness %s", location, name, harness)
		}
		return nil
	}
	for name, b := range c.Loops {
		if err := check("loops."+name, b.Harness, b.NativeProfile); err != nil {
			return err
		}
	}
	for name, b := range c.Routes {
		if err := check("routes."+name, b.Harness, b.NativeProfile); err != nil {
			return err
		}
	}
	for name, b := range c.Profiles {
		if err := check("profiles."+name, b.Harness, b.NativeProfile); err != nil {
			return err
		}
	}
	if b := c.JudgePanel; b != nil {
		if err := check("judge-panel", b.Harness, b.NativeProfile); err != nil {
			return err
		}
	}
	if c.ExecutionPolicy != nil {
		for name, b := range c.ExecutionPolicy.Bindings {
			if err := check("execution-policy.bindings."+name, b.Harness, b.NativeProfile); err != nil {
				return err
			}
		}
	}
	return nil
}
