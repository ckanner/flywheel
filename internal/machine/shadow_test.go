package machine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestOptionalShadowQueuesLocallyWithoutCredentialsOrHTTP(t *testing.T) {
	for _, scenario := range []string{"off", "resumed", "cancelled", "hard-budget", "no-key", "bad-summary", "no-directory", "bad-target", "ok"} {
		t.Run(scenario, func(t *testing.T) {
			fx := newQuickFixture(t, false)
			c := quickContext(t, fx, quickDrivers())
			c.Fresh = true
			c.RunContext = context.Background()
			c.Config.JEVShadow = config.Default().JEVShadow
			c.Config.JEVShadow.Enabled = scenario != "off"
			var logs []string
			c.Log = func(s string) { logs = append(logs, s) }
			c.EnvLookup = func(string) (string, bool) { t.Fatal("capture looked up credentials"); return "", false }
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
			defer server.Close()
			c.Config.JEVShadow.Endpoint = server.URL + "/v1/systemone"
			directory := t.TempDir()
			previous := shadowConfigPath
			t.Cleanup(func() { shadowConfigPath = previous })
			shadowConfigPath = func() (string, error) {
				if scenario == "no-directory" {
					return "", errors.New("missing")
				}
				return filepath.Join(directory, "config.json"), nil
			}
			switch scenario {
			case "resumed":
				c.Fresh = false
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.RunContext = ctx
			case "hard-budget":
				c.Config.Budget.Enforce = "hard"
			case "no-key":
				c.RunContext = nil
			case "bad-summary":
				c.Spec.Goal = "api_key=sk-abcdefghijklmnopqrstuvwxyz0123456789"
			case "bad-target":
				c.Config.JEVShadow.Endpoint = "invalid"
			}
			before, _ := json.Marshal(c.State)
			c.observeShadow()
			after, _ := json.Marshal(c.State)
			require.Equal(t, string(before), string(after))
			if scenario == "cancelled" {
				require.ErrorIs(t, c.err(), context.Canceled)
			} else {
				require.NoError(t, c.err())
			}
			require.Zero(t, calls.Load())
			if scenario == "ok" || scenario == "no-key" || scenario == "hard-budget" {
				require.Contains(t, strings.Join(logs, "\n"), "snapshot queued")
				files, err := filepath.Glob(filepath.Join(directory, "shadow", "*", "*", "cases.json"))
				require.NoError(t, err)
				require.Len(t, files, 1)
				pending, err := filepath.Glob(filepath.Join(filepath.Dir(files[0]), "*.pending"))
				require.NoError(t, err)
				require.Empty(t, pending)
				c.Fresh = false
				c.observeShadow()
			} else if scenario == "off" || scenario == "resumed" || scenario == "cancelled" {
				require.Empty(t, logs)
				entries, _ := os.ReadDir(directory)
				require.Empty(t, entries)
			}
		})
	}
}

func TestAutomaticShadowStartupContinuesAfterCaptureFailure(t *testing.T) {
	fx := newQuickFixture(t, false)
	cfg := mustConfig(t, fx)
	cfg.JEVShadow.Enabled = true
	cfg.JEVShadow.Endpoint = "invalid"
	opts := runOpts(fx, quickDrivers().set())
	opts.Config = cfg
	opts.PauseBefore = "review"
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "paused", summary.Status)
	st, err := state.Load(fx.taskDir)
	require.NoError(t, err)
	require.Equal(t, "quick", st.Workflow.Effective)
	require.NotEmpty(t, st.Packets["M01-T01"].Evidence)
}

func TestAutomaticShadowIsLocalDuringEngineRunAndResume(t *testing.T) {
	fx := newQuickFixture(t, false)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "config.json")
	old := shadowConfigPath
	t.Cleanup(func() { shadowConfigPath = old })
	shadowConfigPath = func() (string, error) { return path, nil }
	opts := runOpts(fx, quickDrivers().set())
	opts.Config.JEVShadow.Enabled = true
	opts.Config.JEVShadow.Endpoint = server.URL + "/v1/systemone"
	opts.PauseBefore = "review"
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "paused", summary.Status)
	st, err := state.Load(fx.taskDir)
	require.NoError(t, err)
	require.Equal(t, "quick", st.Workflow.Effective)
	files, err := filepath.Glob(filepath.Join(filepath.Dir(path), "shadow", "*", "*", "cases.json"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	_, err = RunTask(opts)
	require.NoError(t, err)
	require.Zero(t, calls.Load())
}
