package diagnostic

import (
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/stretchr/testify/require"
)

func TestFixedEffortAndSubscriptionEvaluatorHaveResolvedDiagnosticBindings(t *testing.T) {
	root, _ := diagnosticFixture(t, `{"jev":{"enabled":false,"backend":"subscription","profile":"local"},"profiles":{"local":{"harness":"antigravity","channel":"google","model":"gemini-4-hydrogen-medium","thinking":"medium"}},"channels":{"google":{"harness":"antigravity","env-key":null}},"routes":{"default":{"harness":"antigravity","channel":"google","model":"gemini-pro-orchestrator-neon","thinking":""}}}`)
	view, err := Inspect(root, "")
	require.NoError(t, err)
	require.Empty(t, bindingByID(t, view, "routes.default").Thinking)
	require.NotNil(t, view.JEVBinding)
	require.Equal(t, "local", view.JEVBinding.Profile)
	require.Equal(t, "profiles.local", view.JEVBinding.BindingSource)
	require.Equal(t, "gemini-4-hydrogen-medium", view.JEVBinding.Model)
	require.Equal(t, "medium", view.JEVBinding.Thinking)
	require.Equal(t, "read-only", view.JEVBinding.PermissionMode)
	require.Equal(t, "fresh", view.JEVBinding.Session.Mode)
	require.False(t, view.JEVBinding.InScope, "a resolved evaluator does not mean its switch is enabled")
	cfg := config.Default()
	cfg.JEV.Backend, cfg.JEV.Profile = "subscription", "missing"
	require.Nil(t, configurationView(&cfg, root, "").JEVBinding, "disabled invalid targets stay unresolved")
}

func TestExecutionCandidateDoesNotFabricateUnsupportedSessionAdmission(t *testing.T) {
	cfg := config.Default()
	cfg.Routes = map[string]config.RouteBinding{"default": {Harness: "pi", Channel: "legacy", Model: "legacy", Thinking: "high"}}
	cfg.ExecutionPolicy = &config.ExecutionPolicy{Routes: map[string]config.ExecutionRoute{"default": {Initial: "choice"}},
		Bindings: map[string]config.ExecutionBinding{"choice": {Harness: "pi", Channel: "legacy", Model: "different", Thinking: "low", Permission: "workspace-write", SessionMode: "scoped"}}}
	b := bindingByID(t, configurationView(&cfg, "/work", ""), "routes.default")
	require.Equal(t, "different", b.Model)
	require.Equal(t, "scoped", b.Session.Mode)
	require.False(t, b.Session.Supported)
}
