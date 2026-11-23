package config

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// PermissionMode is shared by orchestration and diagnostics. Task boundaries,
// frozen acceptance, and evidence identity remain independent of host access.
func (c *Config) PermissionMode(role, harness string, bindingIDs ...string) string {
	id := "loops." + role
	if role == "judge-panel" || role == "frontend-ux-review" {
		id = role
	}
	if len(bindingIDs) > 0 {
		id = bindingIDs[0]
	}
	if mode, ok := c.Permissions.Bindings[id]; ok {
		return mode
	}
	if c.Permissions.Mode == "full-access" {
		return "danger-full-access"
	}
	switch role {
	case "review", "judge", "judge-panel", "design-review", "plan-review", "investigate", "frontend-ux-review":
		return "read-only"
	case "intake", "garden":
		if harness == "dsh" || harness == "codex" || harness == "codex-pro" || harness == "claude-code-pro" {
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
	var value any
	_ = json.Unmarshal(configJSON(object), &value)
	if err := validatePermissions(value); err != nil {
		return err
	}
	root["permissions"] = configJSON(object)
	return nil
}

var permissionSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compilePolicySchema("permission-policy.schema.json", templates.FS.ReadFile)
})

func validatePermissions(value any) error {
	schema, err := permissionSchema()
	if err != nil {
		return fmt.Errorf("permissions schema: %w", err)
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("permissions.mode or bindings: %w", err)
	}
	return nil
}
