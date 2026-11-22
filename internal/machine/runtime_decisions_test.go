package machine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func runtimeServer(t *testing.T, choose func(map[string]string) string, hook func()) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	count := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		var request struct {
			Questions map[string]struct {
				Criteria map[string]string `json:"criteria"`
			} `json:"questions"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		choices := request.Questions["action"].Criteria
		selected := choose(choices)
		require.Contains(t, choices, selected)
		probabilities := map[string]float64{}
		for id := range choices {
			probabilities[id] = 0
		}
		probabilities[selected] = 1
		if hook != nil {
			hook()
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "jev-1.14.0", "answers": map[string]any{
			"action":     map[string]any{"type": "choice", "choice": selected, "confidence": 1, "probabilities": probabilities},
			"applicable": map[string]any{"type": "noul", "noul": 1}}, "usage": map[string]any{"input_tokens": 17, "output_tokens": 3}})
	}))
	t.Cleanup(server.Close)
	return server, count
}

func persistRuntimeConfig(t *testing.T, c *Context) {
	t.Helper()
	raw, err := json.Marshal(c.Config)
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(raw, &document))
	for name, binding := range c.Config.Loops {
		document["loops"].(map[string]any)[name].(map[string]any)["provider"] = binding.Provider
	}
	for name, binding := range c.Config.Routes {
		document["routes"].(map[string]any)[name].(map[string]any)["provider"] = binding.Provider
	}
	raw, err = json.Marshal(document)
	require.NoError(t, err)
	writeFile(t, filepath.Join(c.TaskDir, "flywheel.config.json"), string(raw))
	_, err = config.Load(c.TaskDir, c.Cwd)
	require.NoError(t, err)
}

func enableRuntimeJEV(t *testing.T, c *Context, endpoint string) {
	t.Helper()
	c.Config.Subscription = nil // Explicit API fixture; production subscription policy remains strict.
	for name, binding := range c.Config.Loops {
		if c.Config.Channels[binding.Channel].Provider != "" {
			binding.Harness = "dsh"
			c.Config.Loops[name] = binding
		}
	}
	c.Config.JEV = config.JEVConfig{Backend: "http", Enabled: true, Endpoint: endpoint + "/v1/systemone", Model: "jev-1.14.0", KeyEnv: "TEST_JEV_KEY", TimeoutMS: 2000, MaxCalls: 8, MinApplicability: .8, MinChoice: .6, AllowedData: []decision.Category{decision.TaskSummary, decision.DiagnosticFacts}}
	c.EnvLookup = func(key string) (string, bool) {
		if key == "TEST_JEV_KEY" {
			return "runtime-fixture-secret", true
		}
		return os.LookupEnv(key)
	}
	persistRuntimeConfig(t, c)
	gitCmd(t, c.Cwd, "add", "--", filepath.Join(c.TaskDir, "flywheel.config.json"))
	gitCmd(t, c.Cwd, "commit", "-q", "-m", "test: configure JEV runtime fixture")
}

func failingFirstDriver(t *testing.T) *scriptedDrivers {
	t.Helper()
	sd := happyDrivers()
	calls := 0
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		calls++
		r := sd.mock.Execute(a)
		if calls == 1 {
			require.NoError(t, os.Remove(filepath.Join(a.Cwd, "build", a.PacketID+".txt")))
		}
		return r
	}
	return sd
}

func TestRuntimeJEVActuallySelectsRepairFeedbackModelAndReplan(t *testing.T) {
	for _, choice := range []string{"repair", "feedback-0", "recheck-0", "model-1", "replan"} {
		t.Run(choice, func(t *testing.T) {
			fx, c := sessionFixture(t)
			enableExecutionPolicy(c)
			c.Packets["M01-T01"].FeedbackChecks = []contract.GateEntry{{Purpose: "Check whether the generated build artifact exists", Cmd: "test -f build/M01-T01.txt", Expect: "exit 0"}}
			server, count := runtimeServer(t, func(choices map[string]string) string {
				if _, failed := choices["repair"]; !failed {
					return "model-0" // First choose the configured binding, then handle the real failure.
				}
				return choice
			}, nil)
			enableRuntimeJEV(t, c, server.URL)
			sd := failingFirstDriver(t)
			c.Drivers = sessionDriverSet(sd)
			packet := c.Packets["M01-T01"]
			out, err := c.runExecuteLoop(packet)
			require.NoError(t, err)
			if choice == "replan" {
				require.True(t, out.Spun)
				require.NotEmpty(t, c.State.Packets[packet.PacketID].PendingStrategy)
				out, err = c.runExecuteLoop(packet)
				require.NoError(t, err)
				require.True(t, out.Spun, "a saved replan request survives another entry without executing")
				status, err := c.escalatePacket(packet)
				require.NoError(t, err)
				require.Equal(t, "revised", status)
				require.Empty(t, c.State.Packets[packet.PacketID].PendingStrategy)
				out, err = c.runExecuteLoop(c.Packets[packet.PacketID])
				require.NoError(t, err)
			}
			require.True(t, out.Green, "out=%+v feedback=%+v", out, c.State.Packets["M01-T01"].LastFeedback)
			require.Equal(t, int32(2), count.Load())
			ledger := c.State.RuntimeDecisions
			require.NotNil(t, ledger)
			require.Equal(t, 2, ledger.Used)
			require.Len(t, ledger.Records, 2)
			require.Equal(t, 3, ledger.Records[0].Snapshot.Version)
			require.Equal(t, "model-0", ledger.Records[0].Action)
			r := ledger.Records[1]
			require.Equal(t, choice, r.Action)
			require.True(t, r.Selection.Adopted)
			require.True(t, r.Accounted)
			require.Equal(t, int64(17), *r.Receipt.Usage.Input)
			if choice == "model-1" {
				require.Equal(t, "high", sd.log.exec[1].Binding.Thinking)
				require.Empty(t, sd.log.exec[1].SessionID)
				require.Equal(t, "jev-model-selection", c.State.Packets[packet.PacketID].ExecutionPolicy.History[0].Kind)
			}
			if choice == "recheck-0" || choice == "feedback-0" {
				require.Equal(t, "feedback-completed", r.Status)
				require.NotNil(t, r.Feedback)
				require.Contains(t, sd.log.exec[1].Prompt, "jev-feedback")
			}
			loaded := loadFixtureState(t, fx)
			require.Equal(t, ledger.Used, loaded.RuntimeDecisions.Used)
			require.Equal(t, choice, loaded.RuntimeDecisions.Records[1].Action)
		})
	}
}

func TestRuntimeJEVOffAndLateDisableKeepBaseline(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "late"}[late], func(t *testing.T) {
			_, c := sessionFixture(t)
			enableExecutionPolicy(c)
			server, count := runtimeServer(t, func(map[string]string) string { return "model-1" }, func() {
				copy := *c
				copy.Config = jsonClone(c.Config)
				copy.Config.JEV.Enabled = false
				persistRuntimeConfig(t, &copy)
				gitCmd(t, c.Cwd, "add", "--", filepath.Join(c.TaskDir, "flywheel.config.json"))
				gitCmd(t, c.Cwd, "commit", "-q", "-m", "test: disable optional decisions")
			})
			enableRuntimeJEV(t, c, server.URL)
			if !late {
				c.Config.JEV.Enabled = false
				persistRuntimeConfig(t, c)
				gitCmd(t, c.Cwd, "add", "--", filepath.Join(c.TaskDir, "flywheel.config.json"))
				gitCmd(t, c.Cwd, "commit", "-q", "-m", "test: keep JEV disabled")
			}
			sd := failingFirstDriver(t)
			c.Drivers = sessionDriverSet(sd)
			out, err := c.runExecuteLoop(c.Packets["M01-T01"])
			if !late {
				require.NoError(t, err)
				require.True(t, out.Green)
				require.Equal(t, "low", sd.log.exec[1].Binding.Thinking)
			} else {
				require.ErrorContains(t, err, "initial execution inputs changed")
				require.Empty(t, sd.log.exec, "a changed repository must be reloaded before executing")
			}
			if late {
				require.Equal(t, int32(1), count.Load())
				require.False(t, c.State.RuntimeDecisions.Records[0].Selection.Adopted)
				require.Equal(t, "disabled", c.State.RuntimeDecisions.Status)
			} else {
				require.Zero(t, count.Load())
				require.Nil(t, c.State.RuntimeDecisions)
			}
		})
	}
}

func TestRuntimeJEVInterruptedIntentRecoversWithoutResending(t *testing.T) {
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	server, count := runtimeServer(t, func(map[string]string) string { return "model-1" }, func() {
		st, err := state.Load(c.TaskDir)
		require.NoError(t, err)
		_, err = state.Save(c.TaskDir, st, nil)
		require.NoError(t, err)
	})
	enableRuntimeJEV(t, c, server.URL)
	p := c.Packets["M01-T01"]
	facts := runtimeFailureFacts(t, c, p)
	c.failureDecision(p, errL1Failed, facts, false)
	require.Error(t, c.err())
	loaded := loadFixtureState(t, fx)
	require.Equal(t, "pending", loaded.RuntimeDecisions.Records[0].Status)
	c.State, c.revision, c.fatal = loaded, loaded.Revision, nil
	out, err := c.runExecuteLoop(c.Packets["M01-T01"])
	require.NoError(t, err)
	require.True(t, out.Green, "out=%+v feedback=%+v", out, c.State.Packets["M01-T01"].LastFeedback)
	require.Equal(t, int32(1), count.Load())
	require.False(t, c.State.RuntimeDecisions.Records[0].Selection.Adopted)
	require.Equal(t, "low", c.State.Packets[p.PacketID].ExecutionPolicy.Current)
	require.Equal(t, 1, c.State.RuntimeDecisions.Used)
	before := c.State.Budget.Totals.Tokens
	require.Equal(t, "", c.resumeRuntimeDecision(c.Packets["M01-T01"]))
	require.Equal(t, before, c.State.Budget.Totals.Tokens)
}

func TestRuntimeJEVDoesNotRetryCancelledRequest(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	ctx, cancel := context.WithCancel(context.Background())
	c.RunContext = ctx
	server, count := runtimeServer(t, func(map[string]string) string { return "model-1" }, cancel)
	enableRuntimeJEV(t, c, server.URL)
	c.Drivers = sessionDriverSet(failingFirstDriver(t))
	_, err := c.runExecuteLoop(c.Packets["M01-T01"])
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, int32(1), count.Load())
	require.Empty(t, c.State.Packets["M01-T01"].ExecutionPolicy.History)
}
