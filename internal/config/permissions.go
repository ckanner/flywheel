package config

import (
	"encoding/json"
	"errors"
)

// PermissionMode is shared by orchestration and diagnostics. Task boundaries,
// frozen acceptance, and evidence identity remain independent of host access.
func (c *Config) PermissionMode(role, harness string) string {
	if c.Permissions.Mode == "full-access" {
		return "danger-full-access"
	}
	switch role {
	case "review", "judge", "judge-panel":
		return "read-only"
	case "intake", "garden":
		if harness == "dsh" || harness == "codex" || harness == "codex-pro" {
			return "read-only"
		}
		return ""
	default:
		return "workspace-write"
	}
}

func preparePermissions(root map[string]json.RawMessage) error {
	raw, present := root["permissions"]
	if !present {
		return nil
	}
	object, err := configObject(raw, "permissions")
	if err != nil {
		return err
	}
	if err := checkConfigFields(object, "permissions", []string{"mode"}); err != nil {
		return err
	}
	if raw, present := object["mode"]; present {
		mode, err := configString(raw, "permissions.mode")
		if err != nil {
			return err
		}
		if mode != "role-based" && mode != "full-access" {
			return errors.New("permissions.mode must be role-based|full-access")
		}
	}
	root["permissions"] = configJSON(object)
	return nil
}
