package machine

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func nativeBindingProjectionFixture(t *testing.T) (fixture, *Context) {
	t.Helper()
	fx, c := sessionFixture(t)
	definitions := map[string]any{}
	for i := range 8 {
		definitions[fmt.Sprintf("research-%d", i)] = map[string]any{"description": "Read one independently assigned component", "prompt": strings.Repeat("x", 4096)}
	}
	options, err := json.Marshal(map[string]any{"version": 1, "agents": map[string]any{"enabled": true, "definitions": definitions}})
	require.NoError(t, err)
	c.Config.NativeProfiles = map[string]config.NativeProfile{"reviewed-eight-component-parallel-research-definitions": {Harness: "claude-code-pro", Options: options}}
	c.Config.Channels["claude-native"] = config.Channel{Harness: "claude-code-pro"}
	c.Config.Routes["default"] = config.RouteBinding{Harness: "claude-code-pro", Model: "claude-opus-5-5", Channel: "claude-native", Thinking: "high", NativeProfile: "reviewed-eight-component-parallel-research-definitions"}
	bindings := map[string]config.ExecutionBinding{}
	route := config.ExecutionRoute{Initial: "candidate-00", Allowed: []string{}, QualityOrder: []string{}, MaxSwitches: 2, AfterStalledAttempts: 1}
	for i := range 16 {
		id := fmt.Sprintf("candidate-%02d", i)
		bindings[id] = config.ExecutionBinding{Harness: "claude-code-pro", Model: "claude-opus-5-5", Channel: "claude-native", Thinking: "high", NativeProfile: "reviewed-eight-component-parallel-research-definitions",
			Permission: c.Config.PermissionMode("execute", "claude-code-pro", "routes.default"), SessionMode: "scoped"}
		route.Allowed = append(route.Allowed, id)
	}
	c.Config.ExecutionPolicy = &config.ExecutionPolicy{Version: 1, Bindings: bindings, Routes: map[string]config.ExecutionRoute{"default": route}}
	c.Config.File = filepath.Join(fx.taskDir, "flywheel.config.json")
	raw, err := json.Marshal(c.Config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(c.Config.File, raw, 0o600))
	c.Config, err = config.LoadReadOnly(fx.taskDir, fx.root)
	require.NoError(t, err, "large native definitions are a legal, loadable configuration")
	return fx, c
}

func TestConsoleProjectionBoundsRepeatedNativeBindingsWithoutMutatingFrozenFacts(t *testing.T) {
	_, c := nativeBindingProjectionFixture(t)
	initial := c.initialRunBindings()
	c.State.Run = &state.RunIdentity{InitialBindings: initial, ID: "display-run", Kind: "task", Conditions: strings.Repeat("a", 64), StartedAt: c.nowISO()}
	actions := []decision.Action{}
	for i := range 16 {
		actions = append(actions, decision.Action{ID: fmt.Sprintf("model-%d", i), Kind: decision.Model, Description: "Use this frozen candidate", BindingHash: strings.Repeat("b", 64)})
	}
	frozen := c.runtimeModelBindings(c.Packets["M01-T01"], actions)
	c.State.RuntimeDecisions = &state.RuntimeDecisions{Version: 1, Used: 10, Records: []*state.RuntimeDecisionRecord{}}
	for i := range 10 {
		c.State.RuntimeDecisions.Records = append(c.State.RuntimeDecisions.Records, &state.RuntimeDecisionRecord{ID: fmt.Sprintf("failure-%d", i), Packet: "M01-T01", ModelBindings: frozen,
			Snapshot: &decision.Snapshot{Version: 2, BaselineID: "model-0", Actions: actions}, Status: "baseline", Action: "model-0"})
	}
	before, err := json.Marshal(c.State)
	require.NoError(t, err)
	require.Greater(t, len(before), 1<<20, "complete candidate copies would overflow the real control transport")
	view, err := ConsoleProjection(c.State)
	require.NoError(t, err)
	raw, err := json.Marshal(map[string]any{"protocol": 1, "ok": true, "state": view})
	require.NoError(t, err)
	require.Less(t, len(raw), 1<<20)
	actual := view.Run.InitialBindings["routes.default"]
	require.True(t, actual.NativeOptionsOmitted)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(initial["routes.default"].Binding.NativeOptions)), actual.NativeOptionsHash)
	require.Nil(t, actual.Binding.NativeOptions)
	require.Equal(t, initial["routes.default"].Binding.Permission, actual.Binding.Permission)
	require.Equal(t, initial["routes.default"].Binding.SessionMode, actual.Binding.SessionMode)
	require.Equal(t, "reviewed-eight-component-parallel-research-definitions", actual.Binding.NativeProfile)
	omitted := 0
	for _, record := range view.RuntimeDecisions.Records {
		omitted += record.ModelBindingsOmitted
		for _, binding := range record.ModelBindings {
			require.True(t, binding.NativeOptionsOmitted)
			require.Equal(t, actual.NativeOptionsHash, binding.NativeOptionsHash)
		}
	}
	require.Greater(t, omitted, 0, "all displayed maps share one budget, including complete binding identity")
	after, err := json.Marshal(c.State)
	require.NoError(t, err)
	require.Equal(t, before, after, "display omission must not affect recovery or the original frozen choices")
}

func TestRunCatalogSharesProjectionBudgetAcrossOneHundredFullFrozenRecords(t *testing.T) {
	fx, c := nativeBindingProjectionFixture(t)
	directory := t.TempDir()
	initial := c.initialRunBindings()
	var total int
	for i := range 100 {
		record := &RunRecord{Version: 1, ID: fmt.Sprintf("run-%03d", i), TaskDir: fx.taskDir, Cwd: fx.root, StartedAt: fmt.Sprintf("2026-11-20T12:%02d:00Z", i),
			InitialBindings: initial, Bindings: c.Config.Loops, Routes: c.Config.Routes, ConfigurationSource: c.Config.File, ConfigurationScope: "task"}
		require.NoError(t, writeRunRecord(directory, record))
		raw, err := json.Marshal(record)
		require.NoError(t, err)
		total += len(raw)
	}
	require.Greater(t, total, 1<<20)
	original := mustReadFile(t, filepath.Join(directory, "run-099.json"))
	rows, err := ListRuns(directory)
	require.NoError(t, err)
	require.Len(t, rows, 100, "bounding details must not hide registered tasks")
	raw, err := json.Marshal(map[string]any{"protocol": 1, "ok": true, "runs": rows})
	require.NoError(t, err)
	require.Less(t, len(raw), 1<<20)
	require.True(t, rows[0].InitialBindings["routes.default"].NativeOptionsOmitted)
	omitted := 0
	for _, row := range rows {
		omitted += row.InitialBindingsOmitted
	}
	require.Greater(t, omitted, 0)
	require.Equal(t, original, mustReadFile(t, filepath.Join(directory, "run-099.json")), "catalog reads cannot rewrite registered full snapshots")
	var persisted RunRecord
	require.NoError(t, json.Unmarshal(original, &persisted))
	require.Equal(t, initial, persisted.InitialBindings)
	require.Zero(t, persisted.InitialBindingsOmitted)
}

func TestBindingProjectionRetainsSmallNativeOptionsAndNeverTruncatesIdentity(t *testing.T) {
	baseline := state.BindingSnapshot{BindingID: "native", Binding: state.SessionConfiguration{Harness: "antigravity", Model: "gemini-pro-orchestrator-neon", Thinking: "",
		NativeProfile: "agy", NativeOptions: json.RawMessage(`{"version":1,"mode":"swarm","maxObservedAgents":8}`), Permission: "danger-full-access", SessionMode: "scoped"}}
	bindings := map[string]state.BindingSnapshot{"routes.default": baseline}
	budget := displayBindingBudget
	view, omitted := compactBindingSnapshots(bindings, &budget)
	require.Zero(t, omitted)
	require.Equal(t, baseline, view["routes.default"])
	large := baseline
	large.Binding.Model = strings.Repeat("x", displayBindingBudget)
	bindings["routes.large"] = large
	budget = displayBindingBudget
	view, omitted = compactBindingSnapshots(bindings, &budget)
	require.Equal(t, 1, omitted)
	require.NotContains(t, view, "routes.large")
	require.Equal(t, baseline, view["routes.default"])
}

func TestLegacyNativeInitialBindingKeepsUnknownModelAcrossStateAndCatalog(t *testing.T) {
	for _, harness := range []string{"pi", "omp"} {
		t.Run(harness, func(t *testing.T) {
			fx, c := sessionFixture(t)
			c.Config.ExecutionPolicy = nil
			c.Config.Routes["default"] = config.RouteBinding{Harness: harness, Channel: "test"}
			directory := t.TempDir()
			finish, err := c.beginRunRecord(directory)
			require.NoError(t, err)
			var outcome error
			finish(&outcome)
			require.NoError(t, outcome)
			loaded := loadFixtureState(t, fx)
			view, err := ConsoleProjection(loaded)
			require.NoError(t, err)
			binding := view.Run.InitialBindings["routes.default"]
			require.Equal(t, harness, binding.Binding.Harness)
			require.Empty(t, binding.Binding.Model, "an unspecified legacy model must remain unknown")
			rows, err := ListRuns(directory)
			require.NoError(t, err)
			require.Equal(t, binding, rows[0].InitialBindings["routes.default"])
		})
	}
}
