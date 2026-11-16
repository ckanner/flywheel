package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanningBindingsInheritWithoutMutatingConfiguration(t *testing.T) {
	cfg, err := parseConfig([]byte(`{
	  "native-profiles": {"research":{"harness":"kimi","options":{"version":1,"mode":"swarm"}}},
	  "profiles": {"planner":{"harness":"kimi","model":"kimi-code/k3.1-pro","thinking":"high","channel":"subscription","native-profile":"research"}},
	  "loops": {"plan":{"profile":"planner","resident-session":true},"review":{"harness":"dsh","model":"review-model","provider":"chatgpt","channel":"account","thinking":"high"}},
	  "drivers": {"plan":"mock","review":"real"}
	}`), "planning.json")
	require.NoError(t, err)
	before, err := json.Marshal(cfg)
	require.NoError(t, err)
	for _, tc := range []struct{ role, source, kind string }{
		{"design-plan", "plan", "mock"}, {"design-review", "review", "real"}, {"plan-review", "review", "real"},
		{"plan", "plan", "mock"}, {"review", "review", "real"},
	} {
		binding, source, ok := cfg.LoopBindingForRole(tc.role)
		require.True(t, ok, tc.role)
		assert.Equal(t, tc.source, source)
		assert.Equal(t, cfg.Loops[tc.source], binding)
		assert.Equal(t, tc.kind, cfg.DriverKind(tc.role))
	}
	_, source, ok := cfg.LoopBindingForRole("unknown")
	assert.False(t, ok)
	assert.Equal(t, "unknown", source)
	assert.Empty(t, cfg.DriverKind("unknown"))
	after, err := json.Marshal(cfg)
	require.NoError(t, err)
	assert.Equal(t, before, after, "resolving a role cannot materialize inherited settings")
	assert.NotContains(t, cfg.Loops, "design-plan")
	assert.NotContains(t, cfg.Loops, "design-review")

	empty := Default()
	_, _, ok = empty.LoopBindingForRole("design-plan")
	assert.False(t, ok, "an absent base binding is not a working planner")
}

func TestPlanningExplicitOverridesPreserveNativeAndHistoryBoundaries(t *testing.T) {
	cfg, err := parseConfig([]byte(`{
	  "native-profiles": {"team":{"harness":"dsh","options":{"version":1,"preset":"heavy","agents":{"enabled":true},"team":{"enabled":true}}}},
	  "profiles": {"research":{"harness":"dsh","model":"research-model","provider":"chatgpt","channel":"account","thinking":"high","native-profile":"team"}},
	  "loops": {"plan":{"profile":"research"},"review":{"profile":"research"},"design-plan":{"profile":"research","model":"design-model","native-profile":""},"design-review":{"profile":"research","thinking":""},"plan-review":{"profile":"research"}},
	  "drivers": {"plan":"mock","review":"mock","design-plan":"real","design-review":"mock","plan-review":"real"},
	  "permissions": {"mode":"role-based","bindings":{"loops.design-review":"danger-full-access","loops.plan-review":"read-only"}},
	  "sessions": {"mode":"scoped","policy-version":1,"roles":{"plan":"fresh","design-plan":"scoped","design-review":"fresh","plan-review":"scoped"}}
	}`), "planning.json")
	require.NoError(t, err)
	for _, role := range []string{"design-plan", "design-review", "plan-review"} {
		binding, source, ok := cfg.LoopBindingForRole(role)
		require.True(t, ok)
		assert.Equal(t, role, source, "explicit binding wins even when native-profile or thinking is empty")
		assert.Equal(t, cfg.Loops[role], binding)
	}
	assert.Equal(t, "design-model", cfg.Loops["design-plan"].Model)
	assert.Empty(t, cfg.Loops["design-plan"].NativeProfile)
	assert.Empty(t, cfg.Loops["design-review"].Thinking)
	assert.Equal(t, "team", cfg.Loops["design-review"].NativeProfile)
	assert.Equal(t, "team", cfg.Loops["plan"].NativeProfile)
	assert.Equal(t, "high", cfg.Loops["review"].Thinking)
	assert.Equal(t, "real", cfg.DriverKind("design-plan"))
	assert.Equal(t, "mock", cfg.DriverKind("design-review"))
	assert.Equal(t, "real", cfg.DriverKind("plan-review"))
	assert.Equal(t, "fresh", cfg.Sessions.ModeForRole("design-review"))
	assert.Equal(t, "scoped", cfg.Sessions.ModeForRole("review"), "new role override cannot change code-review history")
	assert.Equal(t, "scoped", cfg.Sessions.ModeForRole("design-plan"))
	assert.Equal(t, "fresh", cfg.Sessions.ModeForRole("plan"))
	assert.Equal(t, "danger-full-access", cfg.PermissionMode("design-review", "dsh"))
	assert.Equal(t, "read-only", cfg.PermissionMode("plan-review", "dsh"))
	assert.Equal(t, "workspace-write", cfg.PermissionMode("design-plan", "dsh"))
	assert.Equal(t, "read-only", cfg.PermissionMode("review", "dsh"))

	cfg.Permissions.Bindings = map[string]string{"loops.review": "danger-full-access"}
	assert.Equal(t, "read-only", cfg.PermissionMode("design-review", "dsh"), "backend inheritance must not widen a new role's permission")
	assert.Equal(t, "read-only", cfg.PermissionMode("plan-review", "dsh"))
	cfg.Permissions.Mode = "full-access"
	assert.Equal(t, "danger-full-access", cfg.PermissionMode("design-review", "claude-code-pro"))
	assert.Equal(t, "danger-full-access", cfg.PermissionMode("design-plan", "kimi"))

	encoded, err := json.Marshal(cfg)
	require.NoError(t, err)
	reloaded, err := parseConfig(encoded, "snapshot.json")
	require.NoError(t, err)
	assert.Equal(t, cfg.Loops, reloaded.Loops)
	assert.Equal(t, cfg.Sessions, reloaded.Sessions)
}

func TestPlanningRoleConfigurationRejectsTyposAndMismatchedNative(t *testing.T) {
	for _, raw := range []string{
		`{"loops":{"design-reveiw":{}}}`,
		`{"drivers":{"design-reveiw":"mock"}}`,
		`{"permissions":{"bindings":{"loops.design-reveiw":"read-only"}}}`,
		`{"sessions":{"mode":"scoped","policy-version":1,"roles":{"design-reveiw":"fresh"}}}`,
		`{"native-profiles":{"codex":{"harness":"codex-pro","options":{"version":1,"agents":{"enabled":true}}}},"loops":{"design-review":{"harness":"dsh","provider":"chatgpt","model":"gpt","channel":"account","native-profile":"codex"}}}`,
	} {
		_, err := parseConfig([]byte(raw), "bad-planning.json")
		require.Error(t, err, raw)
	}
}
