package config

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// This explicit preset replaces model bindings, preserving engineering and
// existing permission choices. The new analyst role gets a restrictive default.
// Unused custom channels and profiles retain their identity.
func migrateSubscriptionCLI(root, preset map[string]json.RawMessage, changes *[]MigrationChange) error {
	for _, section := range []string{"loops", "routes", "profiles", "channels", "native-profiles"} {
		definitions, err := migrationObject(preset[section])
		if err != nil || len(definitions) == 0 {
			return fmt.Errorf("embedded subscription-cli preset has invalid %s", section)
		}
		existing, err := migrationObject(root[section])
		if err != nil {
			return err
		}
		for _, name := range slices.Sorted(maps.Keys(definitions)) {
			if migrationNote(name) {
				continue
			}
			// Notes can survive replacement; native/model overrides must not.
			replacement, err := migrationObject(definitions[name])
			if err != nil {
				return err
			}
			old, err := migrationObject(existing[name])
			if err != nil {
				return err
			}
			for key, value := range old {
				if migrationNote(key) {
					replacement[key] = value
				}
			}
			migrationSet(existing, name, section+"."+name, configJSON(replacement), changes)
		}
		root[section] = configJSON(existing)
	}
	for _, section := range []string{"subscription", "planning"} {
		if len(preset[section]) == 0 {
			return fmt.Errorf("embedded subscription-cli preset omitted %s", section)
		}
		migrationSet(root, section, section, preset[section], changes)
	}
	// Engineering policies retain their existing values. Disabled legacy JEV
	// receives the new target without replacing its limits or data consent.
	if _, present := root["sessions"]; !present {
		migrationSet(root, "sessions", "sessions", preset["sessions"], changes)
	}
	if err := migrateSubscriptionCLIAnalystPermission(root, changes); err != nil {
		return err
	}
	return migrateSubscriptionCLIJEV(root, preset, changes)
}

func migrateSubscriptionCLIAnalystPermission(root map[string]json.RawMessage, changes *[]MigrationChange) error {
	permissions, err := migrationObject(root["permissions"])
	if err != nil {
		return err
	}
	bindings, err := migrationObject(permissions["bindings"])
	if err != nil {
		return err
	}
	if _, present := bindings["loops.investigate"]; present {
		return nil
	}
	migrationSet(bindings, "loops.investigate", "permissions.bindings.loops.investigate", configJSON("read-only"), changes)
	permissions["bindings"] = configJSON(bindings)
	root["permissions"] = configJSON(permissions)
	return nil
}

func migrateSubscriptionCLIJEV(root, preset map[string]json.RawMessage, changes *[]MigrationChange) error {
	defaults, err := migrationObject(preset["jev"])
	if err != nil || string(defaults["enabled"]) != "false" || string(defaults["backend"]) != `"subscription"` || len(defaults["profile"]) == 0 {
		return fmt.Errorf("embedded subscription-cli preset has invalid jev target")
	}
	raw, present := root["jev"]
	if !present {
		migrationSet(root, "jev", "jev", preset["jev"], changes)
		return nil
	}
	existing, err := migrationObject(raw)
	if err != nil {
		return err
	}
	var policy JEVConfig
	if err := json.Unmarshal(raw, &policy); err != nil {
		return fmt.Errorf("invalid existing jev policy: %w", err)
	}
	if policy.Backend == "subscription" {
		return nil
	}
	if policy.Enabled {
		return fmt.Errorf("jev: disable the enabled HTTP backend before subscription-cli migration; its active decision target was preserved")
	}
	for _, field := range []string{"enabled", "backend", "profile"} {
		migrationSet(existing, field, "jev."+field, defaults[field], changes)
	}
	for _, field := range []string{"endpoint", "key-env", "model"} {
		migrationSet(existing, field, "jev."+field, nil, changes)
	}
	root["jev"] = configJSON(existing)
	return nil
}
