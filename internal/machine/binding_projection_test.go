package machine

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestRunInitialBindingsSeparateCandidateFromHistoricalRouteDeclaration(t *testing.T) {
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	baseline := c.Config.Routes["default"]
	selected := config.ExecutionBinding{Harness: "antigravity", Model: "gemini-pro-orchestrator-neon", Channel: "google", NativeProfile: "native", Permission: "danger-full-access", SessionMode: "scoped"}
	c.Config.ExecutionPolicy.Bindings["low"] = selected
	c.Config.NativeProfiles = map[string]config.NativeProfile{"native": {Harness: "antigravity", Options: json.RawMessage(`{"version":1,"mode":"native"}`)}}
	c.Config.File = filepath.Join(fx.taskDir, "flywheel.config.json")
	directory := t.TempDir()
	finish, err := c.beginRunRecord(directory)
	require.NoError(t, err)
	initial := c.State.Run.InitialBindings["routes.default"]
	require.Equal(t, "low", initial.BindingID)
	require.Equal(t, selected.Model, initial.Binding.Model)
	require.Empty(t, initial.Binding.Thinking)
	require.Equal(t, selected.Permission, initial.Binding.Permission)
	require.Equal(t, selected.SessionMode, initial.Binding.SessionMode)
	require.JSONEq(t, string(c.Config.NativeProfiles["native"].Options), string(initial.Binding.NativeOptions))
	require.Equal(t, baseline.Model, c.State.Run.Bindings["routes.default"].Model)
	require.Equal(t, "task", c.State.Run.ConfigurationScope)
	require.Equal(t, c.Config.File, c.State.Run.ConfigurationSource)
	var runErr error
	finish(&runErr)
	require.NoError(t, runErr)
	records, err := ListRuns(directory)
	require.NoError(t, err)
	require.Equal(t, initial, records[0].InitialBindings["routes.default"])
	require.Equal(t, baseline.Model, records[0].Routes["default"].Model)
	loaded, err := json.Marshal(loadFixtureState(t, fx).Run.InitialBindings)
	require.NoError(t, err)
	original, err := json.Marshal(c.State.Run.InitialBindings)
	require.NoError(t, err)
	require.JSONEq(t, string(original), string(loaded))
}

func TestRuntimeProjectionKeepsOriginalProposalAndFrozenCandidateIdentity(t *testing.T) {
	_, c := sessionFixture(t)
	p := c.Packets["M01-T01"]
	require.Empty(t, c.runtimeModelBindings(p, nil))
	enableExecutionPolicy(c)
	actions := []decision.Action{{ID: "model-1", Kind: decision.Model, Description: "Original approved high effort"}, {ID: "repair", Kind: decision.Repair, Description: "Continue repair"}}
	frozen := c.runtimeModelBindings(p, actions)
	require.Len(t, frozen, 1)
	require.Equal(t, "high", frozen["model-1"].BindingID)
	changed := c.Config.ExecutionPolicy.Bindings["high"]
	changed.Thinking = "max"
	c.Config.ExecutionPolicy.Bindings["high"] = changed
	require.Equal(t, "high", frozen["model-1"].Binding.Thinking, "historical choices cannot be remapped through today's config")
	r := &state.RuntimeDecisionRecord{ID: "initial-M01-T01", Packet: p.PacketID, Action: "repair", Status: "baseline", ModelBindings: frozen,
		Snapshot: &decision.Snapshot{Version: 3, BaselineID: "repair", Actions: actions, Facts: []decision.Fact{{Text: "private task evidence"}}},
		Receipt:  &decision.Receipt{ActionID: "model-1", Status: "ok"}, Selection: &decision.Selection{ActionID: "repair", Reason: "not-adopted"}}
	c.State.RuntimeDecisions = &state.RuntimeDecisions{Records: []*state.RuntimeDecisionRecord{r}}
	view, err := ConsoleProjection(c.State)
	require.NoError(t, err)
	display := view.RuntimeDecisions.Records[0]
	require.Equal(t, "initial-execution", display.Phase)
	require.Equal(t, "repair", display.BaselineActionID)
	require.Equal(t, "model-1", display.Receipt.ActionID)
	require.Equal(t, "repair", display.Action)
	require.Equal(t, frozen, display.ModelBindings)
	require.Equal(t, actions, display.Actions)
	require.Nil(t, display.Snapshot)
	require.Empty(t, r.Phase, "the display cannot write back projection-only fields")
	require.NotNil(t, r.Snapshot)
}
