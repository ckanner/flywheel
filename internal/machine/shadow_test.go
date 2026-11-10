package machine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOptionalShadowNeverChangesWorkflowAndFailsOpen(t *testing.T) {
	for _, scenario := range []string{"off", "resumed", "hard-budget", "no-key", "bad-summary", "no-directory", "bad-target", "unauthorized", "ok", "timeout"} {
		t.Run(scenario, func(t *testing.T) {
			fx := newQuickFixture(t, false)
			c := quickContext(t, fx, quickDrivers())
			c.Fresh = true
			c.RunContext = context.Background()
			c.Config.JEVShadow = config.Default().JEVShadow
			c.Config.JEVShadow.Enabled = scenario != "off"
			logs := []string{}
			c.Log = func(s string) { logs = append(logs, s) }
			lookups := 0
			c.EnvLookup = func(string) (string, bool) {
				lookups++
				if scenario == "no-key" {
					return "", false
				}
				return "observer-test-key-123456789", true
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				io.Copy(io.Discard, r.Body)
				if scenario == "timeout" {
					<-r.Context().Done()
					return
				}
				if scenario == "unauthorized" {
					w.WriteHeader(401)
					return
				}
				answers := map[string]any{}
				for _, key := range []string{"bounded_objective", "known_edit", "named_checks", "unresolved_design", "access_change", "durable_data_change", "concurrency_change", "public_contract_change"} {
					answers[key] = map[string]any{"type": "noul", "noul": 0.95}
				}
				json.NewEncoder(w).Encode(map[string]any{"model": "jev-1.14.0", "answers": answers})
			}))
			defer server.Close()
			c.Config.JEVShadow.Endpoint = server.URL + "/v1/systemone"
			home := t.TempDir()
			previous := shadowConfigPath
			t.Cleanup(func() { shadowConfigPath = previous })
			shadowConfigPath = func() (string, error) {
				if scenario == "no-directory" {
					return "", errors.New("missing")
				}
				return filepath.Join(home, "config.json"), nil
			}
			switch scenario {
			case "resumed":
				c.Fresh = false
			case "hard-budget":
				c.Config.Budget.Enforce = "hard"
			case "bad-summary":
				c.Spec.Goal = "api_key=sk-abcdefghijklmnopqrstuvwxyz0123456789"
			case "bad-target":
				c.Config.JEVShadow.Endpoint = "invalid"
			case "timeout":
				c.Config.JEVShadow.TimeoutMS = 10
			}
			before, _ := json.Marshal(c.State)
			c.observeShadow()
			after, _ := json.Marshal(c.State)
			require.Equal(t, string(before), string(after))
			require.NoError(t, c.err())
			if scenario == "ok" || scenario == "timeout" || scenario == "unauthorized" {
				require.EqualValues(t, 1, calls.Load())
				require.Contains(t, strings.Join(logs, "\n"), "workflow unchanged")
				files, err := filepath.Glob(filepath.Join(home, "shadow", "*", "*", "cases.json"))
				require.NoError(t, err)
				require.Len(t, files, 1)
				c.Fresh = false
				c.observeShadow()
				require.EqualValues(t, 1, calls.Load())
			} else {
				require.Zero(t, calls.Load())
			}
			if scenario == "off" || scenario == "resumed" {
				require.Empty(t, logs)
				require.Zero(t, lookups)
				entries, _ := os.ReadDir(home)
				require.Empty(t, entries)
			}
		})
	}
}

func TestAutomaticShadowStartupContinuesAfterFailure(t *testing.T) {
	fx := newQuickFixture(t, false)

	cfg := mustConfig(t, fx)
	cfg.JEVShadow.Enabled = true
	cfg.JEVShadow.Endpoint = "invalid"
	calls := 0
	opts := runOpts(fx, quickDrivers().set())
	opts.Config = cfg
	opts.PauseBefore = "review"
	opts.Env = func(string) (string, bool) { calls++; return "observer-key", true }
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "paused", summary.Status)
	require.Positive(t, calls)
	st, err := state.Load(fx.taskDir)
	require.NoError(t, err)
	require.Equal(t, "quick", st.Workflow.Effective)
	require.NotEmpty(t, st.Packets["M01-T01"].Evidence)
}

func TestAutomaticShadowAdviceCannotSelectModeAndCancellationStillStops(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelRequest), func(t *testing.T) {
			fx := newQuickFixture(t, false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				calls.Add(1)
				st, err := state.Load(fx.taskDir)
				require.NoError(t, err)
				require.Equal(t, "running", st.Status)
				require.Equal(t, "quick", st.Workflow.Effective)
				if cancelRequest {
					cancel()
					return
				}
				answers := map[string]any{}
				for _, key := range []string{"bounded_objective", "known_edit", "named_checks", "unresolved_design", "access_change", "durable_data_change", "concurrency_change", "public_contract_change"} {
					answers[key] = map[string]any{"type": "noul", "noul": 0.99}
				}
				json.NewEncoder(w).Encode(map[string]any{"model": "jev-1.14.0", "answers": answers})
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "config.json")
			old := shadowConfigPath
			t.Cleanup(func() { shadowConfigPath = old })
			shadowConfigPath = func() (string, error) { return path, nil }
			opts := runOpts(fx, quickDrivers().set())
			opts.Config.JEVShadow.Enabled = true
			opts.Config.JEVShadow.Endpoint = server.URL + "/v1/systemone"
			opts.Context = ctx
			opts.PauseBefore = "review"
			opts.Env = func(string) (string, bool) { return "test-observer-credential-123456789", true }
			summary, err := RunTask(opts)
			require.EqualValues(t, 1, calls.Load())
			if cancelRequest {
				require.ErrorIs(t, err, ErrInterrupted)
				require.NoFileExists(t, filepath.Join(fx.root, "build", "M01-T01.txt"))
			} else {
				require.NoError(t, err)
				require.Equal(t, "paused", summary.Status)
				st, err := state.Load(fx.taskDir)
				require.NoError(t, err)
				require.Equal(t, "quick", st.Workflow.Effective)
				_, err = RunTask(opts)
				require.NoError(t, err)
				require.EqualValues(t, 1, calls.Load())
			}
		})
	}
}
