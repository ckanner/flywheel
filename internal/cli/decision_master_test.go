package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/stretchr/testify/require"
)

func TestDecisionMasterOffBlocksOnlineButLeavesRecordedReplayAvailable(t *testing.T) {
	_, input, response := decisionCLIFixture(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	endpoint := server.URL + "/v1/systemone"
	writeDecisionCLIConfig(t, input, endpoint, false)
	t.Setenv("FLYWHEEL_DECISION_TEST_KEY", "")
	directory := filepath.Join(t.TempDir(), "evaluation")
	args := append(decisionCLIArgs("evaluate", input, endpoint), "--directory", directory, "--max-calls", "1")
	code, out := runCLI(t, args...)
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "JEV is disabled")
	_, err := os.Stat(directory)
	require.True(t, os.IsNotExist(err), "off cannot create an online decision journal")
	code, out = runCLI(t, append(decisionCLIArgs("replay", input, endpoint), "--response", response)...)
	require.Zero(t, code, out)
	require.Contains(t, out, "offline-recorded-response")
	require.Zero(t, calls.Load())
}

func TestDecisionEvaluationUsesConfiguredTargetAndCannotBypassScope(t *testing.T) {
	_, input, response := decisionCLIFixture(t)
	native, err := os.ReadFile(response)
	require.NoError(t, err)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write(native)
	}))
	defer server.Close()
	writeDecisionCLIConfig(t, input, server.URL+"/v1/systemone", true)
	t.Setenv("FLYWHEEL_DECISION_TEST_KEY", "fixture-key")
	args := []string{"decision", "evaluate", "--cwd", filepath.Dir(input), "--input", input, "--directory", filepath.Join(t.TempDir(), "evaluation"), "--max-calls", "1"}
	code, out := runCLI(t, args...)
	require.Zero(t, code, out)
	require.Contains(t, out, `"status": "ok"`)
	require.EqualValues(t, 1, calls.Load())
	for _, override := range [][]string{{"--endpoint", "https://other.invalid/v1/systemone"}, {"--model", "jev-9.9.9"}, {"--expected-model", "jev-9.9.9"}, {"--key-env", "OTHER_JEV_KEY"}} {
		code, out := runCLI(t, append(append([]string{}, args...), override...)...)
		require.Equal(t, 1, code, out)
		require.Contains(t, out, "must match the effective JEV configuration")
	}
	task := t.TempDir()
	writeDecisionCLIConfig(t, filepath.Join(task, "unused.json"), "", false)
	code, out = runCLI(t, append(args, "--task-dir", task)...)
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "JEV is disabled")
	require.EqualValues(t, 1, calls.Load())
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(input), "flywheel.config.json"), []byte(`{"jev":{"enabled":"yes"}}`), 0600))
	code, out = runCLI(t, args...)
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "effective configuration is invalid or unreadable")
}

func TestDecisionEvaluationNeverWidensConfiguredLimitsOrData(t *testing.T) {
	cfg := config.Default().JEV
	cfg.MaxCalls, cfg.MinApplicability, cfg.MinChoice = 2, .95, .9
	cfg.AllowedData = []decision.Category{decision.DiagnosticFacts}
	p := decision.Policy{Version: "fixture", Mode: decision.Adopt, MaxCalls: 20, MinApplicability: .8, MinChoice: .6,
		AllowedData: []decision.Category{decision.DiagnosticFacts, decision.Source}}
	effective := boundEvaluationPolicy(p, cfg, 12)
	require.Equal(t, decision.Advisory, effective.Mode)
	require.Equal(t, 2, effective.MaxCalls)
	require.Equal(t, .95, effective.MinApplicability)
	require.Equal(t, .9, effective.MinChoice)
	require.Equal(t, []decision.Category{decision.DiagnosticFacts}, effective.AllowedData)
	require.Len(t, p.AllowedData, 2, "captured input remains unchanged")

	_, input, _ := decisionCLIFixture(t)
	cfg.Enabled, cfg.Endpoint, cfg.TimeoutMS = true, "https://example.invalid/v1/systemone", 100
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(input), "flywheel.config.json"), []byte(mustJSONT(t, map[string]any{
		"jev": cfg, "budget": map[string]any{"enforce": "hard"},
	})), 0600))
	options := decision.Options{Timeout: 5 * time.Second}
	key := ""
	_, err := configureDecisionEvaluation(context.Background(), "", filepath.Dir(input), &key, &options)
	require.NoError(t, err)
	require.True(t, options.HardBudget)
	require.Equal(t, 100*time.Millisecond, options.Timeout)
}

func TestDecisionEvaluateCancellationKeepsUnknownReceiptAndDoesNotResend(t *testing.T) {
	_, input, _ := decisionCLIFixture(t)
	arrived, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(arrived)
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	writeDecisionCLIConfig(t, input, server.URL+"/v1/systemone", true)
	t.Setenv("FLYWHEEL_DECISION_TEST_KEY", "fixture-key")
	args := []string{"decision", "evaluate", "--cwd", filepath.Dir(input), "--input", input, "--directory", filepath.Join(t.TempDir(), "evaluation"), "--max-calls", "1"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := newRootCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs(args)
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()
	select {
	case <-arrived:
	case <-time.After(3 * time.Second):
		t.Fatal("online evaluation did not reach the loopback service")
	}
	cancel()
	select {
	case err := <-done:
		var exit *exitError
		require.ErrorAs(t, err, &exit)
		require.Equal(t, 143, exit.code)
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled evaluation did not stop")
	}
	require.Contains(t, output.String(), "cancelled-outcome-unknown")
	require.Contains(t, output.String(), `"input_tokens": null`)
	code, repeated := runCLI(t, args...)
	require.Equal(t, 3, code, repeated, "reading an old cancellation is an unknown result, not a new cancellation")
	require.Equal(t, output.String(), repeated)
	require.EqualValues(t, 1, calls.Load())
}
