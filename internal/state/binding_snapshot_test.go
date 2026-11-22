package state

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitialBindingSnapshotAndConfigurationSourceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st := sessionTestState(t, dir)
	initial := BindingSnapshot{BindingID: "orchestrator", Binding: SessionConfiguration{Harness: "antigravity", Model: "gemini-pro-orchestrator-neon", Channel: "google", Permission: "danger-full-access", SessionMode: "scoped"}}
	st.Run = &RunIdentity{ID: "run-initial", Kind: "run", Conditions: strings.Repeat("a", 64), StartedAt: "2026-11-20T00:00:00Z",
		ConfigurationSource: "/project/flywheel.config.json", ConfigurationScope: "project", InitialBindings: map[string]BindingSnapshot{"routes.default": initial}}
	_, err := Save(dir, st, nil)
	require.NoError(t, err)
	loaded, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, initial, loaded.Run.InitialBindings["routes.default"])
	require.Equal(t, "project", loaded.Run.ConfigurationScope)
	require.Equal(t, st.Run.ConfigurationSource, loaded.Run.ConfigurationSource)
	require.Nil(t, loaded.Run.Bindings, "new initial facts do not manufacture a legacy declaration")
}

func TestRuntimeModelBindingsAreOptionalAndLimitedToFrozenModelActions(t *testing.T) {
	st := initialLedgerFixture(t)
	r := st.RuntimeDecisions.Records[0]
	before := PreparationHash(r.Snapshot)
	initial := BindingSnapshot{BindingID: "native", Binding: SessionConfiguration{Harness: "antigravity", Channel: "google", Model: "gemini-pro-orchestrator-neon", Permission: "danger-full-access", SessionMode: "scoped"}}
	r.ModelBindings = map[string]BindingSnapshot{"model-0": initial}
	require.NoError(t, validateRuntimeDecisions(st))
	require.Equal(t, before, PreparationHash(r.Snapshot), "display metadata cannot alter original request/recovery hashes")
	r.ModelBindings["model-9"] = initial
	require.ErrorContains(t, validateRuntimeDecisions(st), "absent from its frozen actions")
	delete(r.ModelBindings, "model-9")
	r.ModelBindings["model-0"] = BindingSnapshot{Binding: initial.Binding}
	require.ErrorContains(t, validateRuntimeDecisions(st), "bindingId")
	r.ModelBindings = nil
	require.NoError(t, validateRuntimeDecisions(st), "historical records remain valid without newly frozen display data")
}
