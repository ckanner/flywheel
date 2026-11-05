package config

import (
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
