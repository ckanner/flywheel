package config

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

var modelFields = []string{"harness", "model", "channel", "provider", "thinking", "native-profile"}
var loopRoles = []string{"plan", "review", "judge", "intake", "garden"}
var configFields = []string{
	"native-profiles",
	"execution-policy",
	"jev", "jev-shadow", "version", "drivers", "loops", "routes", "channels", "profiles", "budget", "watchdog",
	"retry", "credentials", "verify", "defect-injection", "worktree", "scheduler", "permissions", "sessions", "judge-panel", "intake", "pricing", "mock-behaviors",
}
var channelFields = []string{"env-key", "fallback", "harness", "provider", "models", "cache-hit-rate-min"}

// configObject preserves raw scalar values so presence, null and false remain
// distinct until validation. Notes are accepted only in configuration objects,
// not recursively removed from arbitrary model names or mock behavior payloads.
func configObject(raw json.RawMessage, location string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("invalid JSON: %s must be a JSON object: %w", location, err)
	}
	if object == nil {
		return nil, fmt.Errorf("%s must be a JSON object, not null", location)
	}
	for key := range object {
		if key == "_note" || strings.HasPrefix(key, "_note_") {
			delete(object, key)
		}
	}
	return object, nil
}

// All values originate in successfully parsed JSON; re-encoding cannot contain
// unsupported Go values, cycles or malformed RawMessages.
func configJSON(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

func checkConfigFields(object map[string]json.RawMessage, location string, allowed []string) error {
	for _, key := range slices.Sorted(maps.Keys(object)) {
		if !slices.Contains(allowed, key) {
			return fmt.Errorf("%s: unknown field %q", location, key)
		}
	}
	return nil
}

func configString(raw json.RawMessage, location string) (string, error) {
	var value *string
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return "", fmt.Errorf("%s must be a string, not null", location)
	}
	return *value, nil
}

func validateModelFields(object map[string]json.RawMessage, location string, complete bool) error {
	for _, key := range modelFields {
		raw, present := object[key]
		required := complete && (key == "harness" || key == "model" || key == "channel")
		if !present {
			if required {
				return fmt.Errorf("%s.%s is required", location, key)
			}
			continue
		}
		value, err := configString(raw, location+"."+key)
		if err != nil {
			return err
		}
		if strings.TrimSpace(value) == "" && (required || value != "") {
			return fmt.Errorf("%s.%s must not be blank", location, key)
		}
	}
	return nil
}

func expandModelBinding(raw json.RawMessage, location string, profiles map[string]map[string]json.RawMessage, loop bool) (json.RawMessage, string, error) {
	object, err := configObject(raw, location)
	if err != nil {
		return nil, "", err
	}
	allowed := append(slices.Clone(modelFields), "profile")
	if loop {
		allowed = append(allowed, "resident-session")
	}
	if err := checkConfigFields(object, location, allowed); err != nil {
		return nil, "", err
	}
	profile := ""
	if raw, present := object["profile"]; present {
		profile, err = configString(raw, location+".profile")
		if err != nil {
			return nil, "", err
		}
		if strings.TrimSpace(profile) == "" {
			return nil, "", fmt.Errorf("%s.profile must not be blank", location)
		}
		base, found := profiles[profile]
		if !found {
			return nil, "", fmt.Errorf("%s.profile: unknown profile %q", location, profile)
		}
		merged := maps.Clone(base)
		for key, value := range object {
			if key != "profile" {
				merged[key] = value
			}
		}
		object = merged
	}
	if err := validateModelFields(object, location, profile != ""); err != nil {
		return nil, "", err
	}
	if raw, present := object["resident-session"]; present {
		var resident *bool
		if err := json.Unmarshal(raw, &resident); err != nil || resident == nil {
			return nil, "", fmt.Errorf("%s.resident-session must be a boolean, not null", location)
		}
	}
	return configJSON(object), profile, nil
}

// expandProfiles is the only reference interpreter. All callers receive actual
// bindings; neither the scheduler nor a driver needs to understand profile names.
func expandProfiles(raw []byte) ([]byte, map[string]string, error) {
	root, err := configObject(raw, "config")
	if err != nil {
		return nil, nil, err
	}
	// encoding/json matches struct field names case-insensitively. Reject aliases
	// here so e.g. "Loops" cannot bypass the reference and role checks below.
	if err := checkConfigFields(root, "config", configFields); err != nil {
		return nil, nil, err
	}
	profiles := map[string]map[string]json.RawMessage{}
	if raw, present := root["profiles"]; present {
		definitions, err := configObject(raw, "profiles")
		if err != nil {
			return nil, nil, err
		}
		for _, name := range slices.Sorted(maps.Keys(definitions)) {
			if strings.TrimSpace(name) == "" {
				return nil, nil, fmt.Errorf("profiles: name must not be blank")
			}
			location := "profiles." + name
			definition, err := configObject(definitions[name], location)
			if err != nil {
				return nil, nil, err
			}
			if err := checkConfigFields(definition, location, modelFields); err != nil {
				return nil, nil, err
			}
			if err := validateModelFields(definition, location, true); err != nil {
				return nil, nil, err
			}
			profiles[name] = definition
			definitions[name] = configJSON(definition)
		}
		root["profiles"] = configJSON(definitions)
	}
	refs := map[string]string{}
	for _, section := range []string{"loops", "routes"} {
		raw, present := root[section]
		if !present || string(raw) == "null" {
			continue // Legacy optional collections may be absent or null.
		}
		bindings, err := configObject(raw, section)
		if err != nil {
			return nil, nil, err
		}
		for _, name := range slices.Sorted(maps.Keys(bindings)) {
			location := section + "." + name
			if section == "loops" && !slices.Contains(loopRoles, name) {
				return nil, nil, fmt.Errorf("%s: unknown loop role", location)
			}
			expanded, profile, err := expandModelBinding(bindings[name], location, profiles, section == "loops")
			if err != nil {
				return nil, nil, err
			}
			bindings[name] = expanded
			if profile != "" {
				refs[location] = profile
			}
		}
		root[section] = configJSON(bindings)
	}
	if raw, present := root["judge-panel"]; present && string(raw) != "null" {
		expanded, profile, err := expandModelBinding(raw, "judge-panel", profiles, true)
		if err != nil {
			return nil, nil, err
		}
		root["judge-panel"] = expanded
		if profile != "" {
			refs["judge-panel"] = profile
		}
	}
	if err := prepareConfigSections(root); err != nil {
		return nil, nil, err
	}
	return configJSON(root), refs, nil
}

// Preserve supported notes without weakening strict typed decoding of fields.
func prepareConfigSections(root map[string]json.RawMessage) error {
	if err := prepareNativeProfiles(root); err != nil {
		return err
	}
	if err := prepareExecutionPolicy(root); err != nil {
		return err
	}
	if err := prepareSessions(root); err != nil {
		return err
	}
	if err := prepareJEV(root); err != nil {
		return err
	}
	if err := prepareScheduler(root); err != nil {
		return err
	}
	if err := preparePermissions(root); err != nil {
		return err
	}
	for _, section := range []string{"budget", "watchdog", "retry", "credentials", "verify", "defect-injection", "worktree", "intake"} {
		raw, present := root[section]
		if !present || string(raw) == "null" {
			continue
		}
		object, err := configObject(raw, section)
		if err != nil {
			return err
		}
		if section == "intake" {
			if raw, present := object["sources"]; present {
				var sources []json.RawMessage
				if err := json.Unmarshal(raw, &sources); err != nil {
					return fmt.Errorf("intake.sources must be an array: %w", err)
				}
				for i, raw := range sources {
					source, err := configObject(raw, fmt.Sprintf("intake.sources[%d]", i))
					if err != nil {
						return err
					}
					sources[i] = configJSON(source)
				}
				object["sources"] = configJSON(sources)
			}
		}
		root[section] = configJSON(object)
	}
	for _, section := range []string{"channels", "drivers"} {
		raw, present := root[section]
		if !present || string(raw) == "null" {
			continue
		}
		object, err := configObject(raw, section)
		if err != nil {
			return err
		}
		for _, name := range slices.Sorted(maps.Keys(object)) {
			location := section + "." + name
			if section == "channels" {
				channel, err := configObject(object[name], location)
				if err != nil {
					return err
				}
				if err := checkConfigFields(channel, location, channelFields); err != nil {
					return err
				}
				if raw, present := channel["provider"]; present {
					if _, err := configString(raw, location+".provider"); err != nil {
						return err
					}
				}
				object[name] = configJSON(channel)
			} else {
				if name != "execute" && !slices.Contains(loopRoles, name) {
					return fmt.Errorf("%s: unknown driver role", location)
				}
				value, err := configString(object[name], location)
				if err != nil {
					return err
				}
				if value != "real" && value != "mock" {
					return fmt.Errorf("%s must be real or mock", location)
				}
			}
		}
		root[section] = configJSON(object)
	}
	return nil
}

func (c *Config) validateBindingProvider(harness, channel, provider string) error {
	channelProvider := c.Channels[channel].Provider
	if key := c.Channels[channel].EnvKey; harness == "claude-code-pro" && key != nil && !ClaudeCodeProCredentialAllowed(*key) {
		return fmt.Errorf("claude-code-pro channel env-key is not supported by its public machine API")
	}
	if (provider != "" || channelProvider != "") && harness != "dsh" {
		return fmt.Errorf("provider is only supported by harness dsh")
	}
	if provider != "" && strings.TrimSpace(provider) == "" {
		return fmt.Errorf("provider must not be blank")
	}
	if provider != "" && channelProvider != "" && provider != channelProvider {
		return fmt.Errorf("provider %q conflicts with channels.%s.provider %q", provider, channel, channelProvider)
	}
	return nil
}
