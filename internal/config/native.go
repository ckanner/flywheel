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
	check := func(location, harness, name string, execute bool) error {
		if name == "" {
			return nil
		}
		profile, exists := c.NativeProfiles[name]
		if !exists || profile.Harness != harness {
			return fmt.Errorf("%s.native-profile: %q must name a profile for harness %s", location, name, harness)
		}
		if harness == "antigravity" && !execute {
			var native AntigravityNative
			_ = json.Unmarshal(profile.Options, &native) // options were schema-validated above
			if native.Mode == "swarm" {
				return fmt.Errorf("%s.native-profile: Antigravity Swarm is restricted to approved Execute collaboration", location)
			}
		}
		return nil
	}
	for name, b := range c.Loops {
		if err := check("loops."+name, b.Harness, b.NativeProfile, name == "execute"); err != nil {
			return err
		}
	}
	for name, b := range c.Routes {
		if err := check("routes."+name, b.Harness, b.NativeProfile, true); err != nil {
			return err
		}
	}
	for name, b := range c.Profiles {
		if err := check("profiles."+name, b.Harness, b.NativeProfile, true); err != nil {
			return err
		}
	}
	if b := c.JudgePanel; b != nil {
		if err := check("judge-panel", b.Harness, b.NativeProfile, false); err != nil {
			return err
		}
	}
	if c.ExecutionPolicy != nil {
		for name, b := range c.ExecutionPolicy.Bindings {
			if err := check("execution-policy.bindings."+name, b.Harness, b.NativeProfile, true); err != nil {
				return err
			}
		}
	}
	for role, name := range c.Planning.Compact {
		if b, exists := c.Profiles[name]; exists {
			if err := check("planning.compact."+role, b.Harness, b.NativeProfile, false); err != nil {
				return err
			}
		}
	}
	if d := c.Planning.DualDraft; d != nil && d.Enabled {
		for _, pair := range [][2]string{
			{"planning.dual-draft.secondary-profile", d.SecondaryProfile},
			{"planning.dual-draft.synthesizer-profile", d.SynthesizerProfile},
		} {
			if b, exists := c.Profiles[pair[1]]; pair[1] != "" && exists {
				if err := check(pair[0], b.Harness, b.NativeProfile, false); err != nil {
					return err
				}
			}
		}
	}
	if c.FrontendUXReview.Enabled {
		if b, exists := c.Profiles[c.FrontendUXReview.Profile]; exists {
			if err := check("frontend-ux-review.profile", b.Harness, b.NativeProfile, false); err != nil {
				return err
			}
		}
	}
	return nil
}
