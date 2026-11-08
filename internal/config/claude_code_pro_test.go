package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeCodeProOptionalProfileWorksInEveryRole(t *testing.T) {
	cfg, err := parseConfig([]byte(`{
 "permissions":{"mode":"full-access"},"sessions":{"mode":"scoped","policy-version":1},
 "profiles":{"claude":{"harness":"claude-code-pro","model":"sonnet","channel":"claude-subscription","thinking":"max"}},
 "channels":{"claude-subscription":{"env-key":null}},
 "loops":{"plan":{"profile":"claude"},"review":{"profile":"claude"},"judge":{"profile":"claude"},"intake":{"profile":"claude"},"garden":{"profile":"claude"}},
 "routes":{"default":{"profile":"claude"},"hard":{"profile":"claude","model":"opus"}},
 "judge-panel":{"profile":"claude","model":"haiku"}
}`), "test")
	require.NoError(t, err)
	for _, role := range []string{"plan", "review", "judge", "intake", "garden"} {
		binding := cfg.Loops[role]
		assert.Equal(t, "claude-code-pro", binding.Harness)
		assert.Equal(t, "sonnet", binding.Model)
		assert.Equal(t, "max", binding.Thinking)
		assert.Equal(t, "claude-subscription", binding.Channel)
		assert.Empty(t, binding.Provider)
		assert.Equal(t, "danger-full-access", cfg.PermissionMode(role, binding.Harness))
		assert.False(t, binding.ResidentSession)
	}
	assert.Equal(t, "sonnet", cfg.Routes["default"].Model)
	assert.Equal(t, "opus", cfg.Routes["hard"].Model)
	assert.Equal(t, "haiku", cfg.JudgePanel.Model)
	assert.Equal(t, "danger-full-access", cfg.PermissionMode("execute", "claude-code-pro"))
	assert.True(t, SupportsScopedSessions("claude-code-pro"))
	assert.Equal(t, "claude", cfg.ProfileRefs["routes.hard"])

	// A profile is available for an explicit choice; defining one does not
	// choose a stage, route, model or credential source on the user's behalf.
	unused, err := parseConfig([]byte(`{"profiles":{"claude":{"harness":"claude-code-pro","model":"custom-cli-alias","channel":"subscription"}}}`), "test")
	require.NoError(t, err)
	baseline, err := parseConfig([]byte(`{}`), "test")
	require.NoError(t, err)
	assert.Equal(t, baseline.Loops, unused.Loops)
	assert.Equal(t, baseline.Routes, unused.Routes)
	assert.Equal(t, baseline.JudgePanel, unused.JudgePanel)
	for role, want := range map[string]string{"plan": "workspace-write", "execute": "workspace-write", "review": "read-only", "judge": "read-only", "judge-panel": "read-only", "intake": "read-only", "garden": "read-only"} {
		assert.Equal(t, want, unused.PermissionMode(role, "claude-code-pro"), "a native approval mode is not an OS sandbox")
	}
}

func TestClaudeCodeProConfigurationRejectsAmbiguousBindings(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`{"loops":{"plan":{"harness":"claude-code-pro","resident-session":true}}}`, "resident-session is unsupported for claude-code-pro"},
		{`{"loops":{"review":{"harness":"claude-code-pro","channel":"c"}}}`, "claude-code-pro requires explicit model and channel"},
		{`{"routes":{"hard":{"harness":"claude-code-pro","model":"opus"}}}`, "claude-code-pro requires explicit model and channel"},
		{`{"judge-panel":{"harness":"claude-code-pro","model":"haiku","channel":"c","resident-session":true}}`, "resident-session is unsupported for claude-code-pro"},
		{`{"routes":{"hard":{"harness":"claude-code-pro","model":"opus","channel":"c","provider":"anthropic"}}}`, "provider is only supported by harness dsh"},
		{`{"channels":{"c":{"harness":"claude-code-pro","env-key":"UNSUPPORTED_API_KEY"}}}`, "env-key is not supported"},
		{`{"loops":{"review":{"harness":"claude-code-pro","model":"sonnet","channel":"c"}},"channels":{"c":{"env-key":"UNSUPPORTED_API_KEY"}}}`, "env-key is not supported"},
		{`{"profiles":{"claude":{"harness":"claude-code-pro","model":"sonnet","channel":"c"}},"channels":{"c":{"env-key":"UNSUPPORTED_API_KEY"}}}`, "env-key is not supported"},
	} {
		_, err := parseConfig([]byte(tc.raw), "test")
		require.ErrorContains(t, err, tc.want)
	}
}

func TestClaudeCodeProDeclaredCredentialsMatchThePublicContract(t *testing.T) {
	for _, key := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN"} {
		assert.True(t, ClaudeCodeProCredentialAllowed(key))
		cfg, err := parseConfig([]byte(fmt.Sprintf(`{
 "loops":{"review":{"harness":"claude-code-pro","model":"sonnet","channel":"c"}},
 "channels":{"c":{"harness":"claude-code-pro","env-key":%q}}
}`, key)), "test")
		require.NoError(t, err)
		assert.Equal(t, key, *cfg.Channels["c"].EnvKey)
	}
	for _, key := range []string{"", "API_KEY", "OPENAI_API_KEY", "CLAUDE_CONFIG_DIR", "anthropic_api_key", " ANTHROPIC_API_KEY "} {
		assert.False(t, ClaudeCodeProCredentialAllowed(key))
	}
}
