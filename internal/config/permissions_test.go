package config

import (
	"errors"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPermissionPolicyIsExplicitAndCompatible(t *testing.T) {
	for _, raw := range []string{`{}`, `{"permissions":{}}`, `{"permissions":{"mode":"role-based","_note":"compatibility"}}`} {
		cfg, err := parseConfig([]byte(raw), "test")
		require.NoError(t, err)
		require.Equal(t, "role-based", cfg.Permissions.Mode)
		for _, harness := range []string{"dsh", "codex-pro", "codex", "kimi"} {
			for _, role := range []string{"plan", "execute", "review", "judge", "judge-panel", "intake", "garden"} {
				want := "read-only"
				if role == "plan" || role == "execute" {
					want = "workspace-write"
				} else if (role == "intake" || role == "garden") && harness == "kimi" {
					want = ""
				}
				require.Equal(t, want, cfg.PermissionMode(role, harness), "%s/%s", role, harness)
			}
		}
	}
	cfg, err := parseConfig([]byte(`{"permissions":{"mode":"full-access"}}`), "test")
	require.NoError(t, err)
	for _, harness := range []string{"dsh", "codex-pro", "codex", "kimi"} {
		for _, role := range []string{"plan", "execute", "review", "judge", "judge-panel", "intake", "garden"} {
			require.Equal(t, "danger-full-access", cfg.PermissionMode(role, harness))
		}
	}
}

func TestPermissionPolicyRejectsAmbiguousConfiguration(t *testing.T) {
	for _, raw := range []string{
		`{"permissions":null}`, `{"permissions":true}`, `{"permissions":{"Mode":"full-access"}}`,
		`{"permissions":{"mode":null}}`, `{"permissions":{"mode":true}}`,
		`{"permissions":{"mode":""}}`, `{"permissions":{"mode":" full-access "}}`,
		`{"permissions":{"mode":"full-acess"}}`,
	} {
		_, err := parseConfig([]byte(raw), "test")
		require.Error(t, err, raw)
	}
	cfg := Default()
	cfg.Permissions.Mode = "unknown"
	require.ErrorContains(t, cfg.Validate(), "permissions.mode")
}

func TestPermissionBindingsOverrideWithoutChangingOtherStages(t *testing.T) {
	cfg, err := parseConfig([]byte(`{"permissions":{"bindings":{"loops.review":"read-only","routes.hard":"workspace-write","judge-panel":"read-only"},"mode":"full-access"}}`), "test")
	require.NoError(t, err)
	require.Equal(t, "read-only", cfg.PermissionMode("review", "dsh"))
	require.Equal(t, "read-only", cfg.PermissionMode("judge-panel", "dsh"))
	require.Equal(t, "workspace-write", cfg.PermissionMode("execute", "codex-pro", "routes.hard"))
	require.Equal(t, "danger-full-access", cfg.PermissionMode("execute", "dsh", "routes.default"))
	cfg.Permissions.Mode = ""
	cfg.Normalize()
	require.Equal(t, "workspace-write", cfg.PermissionMode("execute", "codex-pro", "routes.hard"))
	for _, raw := range []string{`{"bindings":{"execute":"read-only"}}`, `{"bindings":{"loops.plan":"full-access"}}`, `{"bindings":null}`} {
		_, err := parseConfig([]byte(`{"permissions":`+raw+`}`), "test")
		require.Error(t, err)
	}
	old := permissionSchema
	permissionSchema = func() (*jsonschema.Schema, error) { return nil, errors.New("schema unavailable") }
	t.Cleanup(func() { permissionSchema = old })
	require.ErrorContains(t, validatePermissions(nil), "schema unavailable")
}
