package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/decision"
	"github.com/stretchr/testify/require"
)

func decisionCLIFixture(t *testing.T) (decisionCapture, string, string) {
	t.Helper()
	capture := decisionCapture{
		Snapshot: decision.Snapshot{Version: 1, ID: "choice-1", Identity: decision.Identity{TaskID: "incident", RunID: "diagnostic-run", Revision: 7, ContractHash: "contract", CandidateHash: "candidate", ObservationHash: "observations"}, Facts: []decision.Fact{{Category: decision.DiagnosticFacts, Text: "The isolated request timed out."}}, Actions: []decision.Action{{ID: "inspect-config", Kind: decision.Diagnostic, Description: "Read the approved config", BindingHash: "config-binding"}, {ID: "inspect-log", Kind: decision.Diagnostic, Description: "Read the approved log", BindingHash: "log-binding"}}, BaselineID: "inspect-log", SemanticUncertainty: true},
		Policy:   decision.Policy{Version: "experiment-1", Mode: decision.Adopt, AllowedData: []decision.Category{decision.DiagnosticFacts}, MinApplicability: .9, MinChoice: .9, MaxCalls: 2},
	}
	dir := t.TempDir()
	input, response := filepath.Join(dir, "snapshot.json"), filepath.Join(dir, "native-response.json")
	require.NoError(t, os.WriteFile(input, []byte(mustJSONT(t, capture)), 0600))
	require.NoError(t, os.WriteFile(response, []byte(`{"model":"jev-1.14.0","answers":{"action":{"type":"choice","choice":"inspect-config","confidence":0.83,"probabilities":{"inspect-config":0.96,"inspect-log":0.04}},"applicable":{"type":"noul","noul":0.98}},"usage":{"input_tokens":100,"output_tokens":20}}`), 0600))
	return capture, input, response
}

func decisionCLIArgs(action, input, endpoint string) []string {
	return []string{"decision", action, "--input", input, "--endpoint", endpoint, "--model", "jev-1.14.0", "--expected-model", "jev-1.14.0"}
}

func TestDecisionReplayCLIReadsRealCaptureAndBareSnapshotWithoutHTTP(t *testing.T) {
	capture, input, response := decisionCLIFixture(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	options := decision.Options{Endpoint: server.URL + "/v1/systemone", Model: "jev-1.14.0", ExpectedModel: "jev-1.14.0", Timeout: 5 * time.Second}
	capture.Policy.Mode = decision.Shadow
	journal := filepath.Join(t.TempDir(), "capture")
	receipt, err := decision.Evaluate(context.Background(), journal, capture.Snapshot, capture.Policy, options)
	require.NoError(t, err)
	require.Equal(t, "snapshot-captured", receipt.Reason)
	files, err := filepath.Glob(filepath.Join(journal, "*.snapshot.json"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	original, err := os.ReadFile(files[0])
	require.NoError(t, err)
	code, out := runCLI(t, append(decisionCLIArgs("replay", files[0], options.Endpoint), "--response", response)...)
	require.Zero(t, code, out)
	var report decisionCLIReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	require.Equal(t, decision.Offline, report.Mode)
	require.False(t, report.Adopted)
	require.Equal(t, "inspect-log", report.Baseline)
	require.Equal(t, "inspect-config", report.Receipt.ActionID)
	require.Equal(t, "offline-recorded-response", report.Receipt.Reason)
	require.NotEmpty(t, report.SourceTargetHash)
	unchanged, err := os.ReadFile(files[0])
	require.NoError(t, err)
	require.Equal(t, original, unchanged)
	entries, err := os.ReadDir(journal)
	require.NoError(t, err)
	require.Len(t, entries, 1, "offline replay cannot write intents, receipts or slots")
	policyFile := filepath.Join(t.TempDir(), "policy.json")
	capture.Policy.Mode = decision.Adopt
	require.NoError(t, os.WriteFile(input, []byte(mustJSONT(t, capture.Snapshot)), 0600))
	require.NoError(t, os.WriteFile(policyFile, []byte(mustJSONT(t, capture.Policy)), 0600))
	code, out = runCLI(t, append(decisionCLIArgs("replay", input, options.Endpoint), "--policy", policyFile, "--response", response)...)
	require.Zero(t, code, out)
	require.Contains(t, out, `"mode": "offline"`)
	require.Contains(t, out, `"adopted": false`)
	require.Zero(t, calls.Load())
}

func TestDecisionEvaluateCLIIsAdvisoryAndRespectsCumulativeCallBudget(t *testing.T) {
	capture, input, response := decisionCLIFixture(t)
	native, err := os.ReadFile(response)
	require.NoError(t, err)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "Bearer cli-decision-secret", r.Header.Get("Authorization"))
		_, _ = w.Write(native)
	}))
	defer server.Close()
	t.Setenv("FLYWHEEL_DECISION_TEST_KEY", "cli-decision-secret")
	directory := filepath.Join(t.TempDir(), "evaluation")
	args := append(decisionCLIArgs("evaluate", input, server.URL+"/v1/systemone"), "--directory", directory, "--key-env", "FLYWHEEL_DECISION_TEST_KEY", "--max-calls", "1")
	code, out := runCLI(t, args...)
	require.Zero(t, code, out)
	var report decisionCLIReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	require.Equal(t, decision.Advisory, report.Mode)
	require.Equal(t, decision.Advisory, report.Policy.Mode)
	require.False(t, report.Adopted)
	require.Equal(t, "ok", report.Receipt.Status)
	require.Equal(t, "inspect-config", report.Receipt.ActionID)
	code, repeated := runCLI(t, args...)
	require.Zero(t, code, repeated)
	require.Equal(t, out, repeated, "same snapshot reads the original durable receipt")
	capture.Snapshot.ID = "choice-2"
	require.NoError(t, os.WriteFile(input, []byte(mustJSONT(t, capture)), 0600))
	code, out = runCLI(t, args...)
	require.Zero(t, code, out)
	require.Contains(t, out, `"reason": "decision-call-budget"`)
	require.EqualValues(t, 1, calls.Load())
	info, err := os.Stat(directory)
	require.NoError(t, err)
	require.Zero(t, info.Mode().Perm()&0077)
	files, err := os.ReadDir(directory)
	require.NoError(t, err)
	for _, entry := range files {
		raw, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		require.NoError(t, err)
		require.NotContains(t, string(raw), "cli-decision-secret")
	}
	_, err = os.Stat(filepath.Join(filepath.Dir(input), "state.json"))
	require.True(t, os.IsNotExist(err), "evaluating a captured decision cannot create implementation state")
}

func TestDecisionEvaluateCLISkipsWithoutDataPermissionAndHardBound(t *testing.T) {
	capture, input, _ := decisionCLIFixture(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	t.Setenv("FLYWHEEL_DECISION_TEST_KEY", "cli-decision-secret")
	args := append(decisionCLIArgs("evaluate", input, server.URL+"/v1/systemone"), "--directory", filepath.Join(t.TempDir(), "evaluation"), "--key-env", "FLYWHEEL_DECISION_TEST_KEY", "--max-calls", "1")
	code, out := runCLI(t, append(append([]string{}, args...), "--hard-budget")...)
	require.Zero(t, code, out)
	require.Contains(t, out, "hard-budget-without-enforceable-bound")
	policy := filepath.Join(t.TempDir(), "policy.json")
	capture.Policy.AllowedData = nil
	require.NoError(t, os.WriteFile(policy, []byte(mustJSONT(t, capture.Policy)), 0600))
	code, out = runCLI(t, append(args, "--policy", policy)...)
	require.Zero(t, code, out)
	require.Contains(t, out, "data-not-authorized")
	require.Zero(t, calls.Load())
}

func TestDecisionEvaluateCLIReportsErrorUnknownAndCancellationWithoutReplay(t *testing.T) {
	_, input, _ := decisionCLIFixture(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(401) }))
	endpoint := server.URL + "/v1/systemone"
	t.Setenv("FLYWHEEL_DECISION_TEST_KEY", "cli-decision-secret")
	args := append(decisionCLIArgs("evaluate", input, endpoint), "--directory", filepath.Join(t.TempDir(), "evaluation"), "--key-env", "FLYWHEEL_DECISION_TEST_KEY", "--max-calls", "1")
	code, out := runCLI(t, args...)
	require.Equal(t, 3, code, out)
	require.Contains(t, out, `"reason": "http-error"`)
	require.Contains(t, out, `"input_tokens": null`)
	code, repeated := runCLI(t, args...)
	require.Equal(t, 3, code, repeated)
	require.Equal(t, out, repeated)
	require.EqualValues(t, 1, calls.Load())
	server.Close()
	unknownArgs := append(append([]string{}, args...), "--directory", filepath.Join(t.TempDir(), "unknown"))
	code, out = runCLI(t, unknownArgs...)
	require.Equal(t, 3, code, out)
	require.Contains(t, out, `"status": "unknown"`)
	code, repeated = runCLI(t, unknownArgs...)
	require.Equal(t, 3, code, repeated)
	require.Equal(t, out, repeated)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := newRootCmd()
	root.SetContext(ctx)
	root.SetArgs(append(args, "--directory", filepath.Join(t.TempDir(), "cancelled")))
	var buffer bytes.Buffer
	root.SetOut(&buffer)
	root.SetErr(&buffer)
	err := root.Execute()
	var exit *exitError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, 143, exit.code)
	require.Contains(t, buffer.String(), "cancelled-before-dispatch")
}

func TestDecisionCLIRejectsInvalidInputsAndMissingExplicitConfiguration(t *testing.T) {
	capture, input, response := decisionCLIFixture(t)
	endpoint := "https://example.invalid/v1/systemone"
	valid := decisionCLIArgs("replay", input, endpoint)
	for _, missing := range []string{"--input", "--endpoint", "--model", "--expected-model"} {
		args := append([]string{}, valid...)
		for i, arg := range args {
			if arg == missing {
				args = append(args[:i], args[i+2:]...)
				break
			}
		}
		code, out := runCLI(t, args...)
		require.Equal(t, 1, code, out)
		require.Contains(t, out, "requires --input")
	}
	live := decisionCLIArgs("evaluate", input, endpoint)
	for _, extra := range [][]string{
		nil,
		{"--directory", t.TempDir(), "--key-env", "BAD-KEY", "--max-calls", "1"},
		{"--directory", t.TempDir(), "--key-env", "FLYWHEEL_DECISION_TEST_KEY", "--max-calls", "101"},
	} {
		code, out := runCLI(t, append(append([]string{}, live...), extra...)...)
		require.Equal(t, 1, code, out)
		require.Contains(t, out, "explicit --max-calls")
	}
	t.Setenv("FLYWHEEL_DECISION_TEST_KEY", "")
	code, out := runCLI(t, append(live, "--directory", t.TempDir(), "--key-env", "FLYWHEEL_DECISION_TEST_KEY", "--max-calls", "1")...)
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "environment variable is empty")
	for _, extra := range [][]string{nil, {"--response", response, "--model", "jev"}, {"--response", response, "--policy", filepath.Join(t.TempDir(), "missing")}} {
		code, out := runCLI(t, append(append([]string{}, valid...), extra...)...)
		require.Equal(t, 1, code, out)
	}
	capture.Snapshot.Version = 0
	require.NoError(t, os.WriteFile(input, []byte(mustJSONT(t, capture)), 0600))
	code, out = runCLI(t, append(valid, "--response", response)...)
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "invalid decision identity")
	code, out = runCLI(t, "decision", "replay", "--input", filepath.Join(t.TempDir(), "missing"), "--endpoint", endpoint, "--model", "jev-1.14.0", "--expected-model", "jev-1.14.0")
	require.Equal(t, 1, code, out)
}

func TestDecisionDocumentsAreStrictBoundedRegularFilesAndPolicyReplacementIsComplete(t *testing.T) {
	capture, input, _ := decisionCLIFixture(t)
	for _, raw := range []string{"[", "[]", `{"snapshot":{},"unknown":true}`, `{"version":1,"unexpected":true}`, `{} {}`} {
		require.NoError(t, os.WriteFile(input, []byte(raw), 0600))
		_, err := loadDecisionInput(input, "")
		require.Error(t, err, raw)
	}
	require.NoError(t, os.WriteFile(input, []byte(mustJSONT(t, capture.Snapshot)), 0600))
	_, err := loadDecisionInput(input, "")
	require.ErrorContains(t, err, "requires --policy")
	policy := filepath.Join(t.TempDir(), "policy.json")
	require.NoError(t, os.WriteFile(input, []byte(mustJSONT(t, capture)), 0600))
	require.NoError(t, os.WriteFile(policy, []byte(`{"version":"replacement","mode":"offline","min-applicability":1,"min-choice":1,"max-calls":1}`), 0600))
	loaded, err := loadDecisionInput(input, policy)
	require.NoError(t, err)
	require.Nil(t, loaded.Policy.AllowedData, "omitting data permission in a replacement policy must not inherit capture permission")
	require.NoError(t, os.WriteFile(policy, []byte(`{"unexpected":true}`), 0600))
	_, err = loadDecisionInput(input, policy)
	require.Error(t, err)
	_, err = readDecisionDocument(t.TempDir())
	require.ErrorContains(t, err, "regular file")
	require.NoError(t, os.WriteFile(input, []byte(strings.Repeat("x", decisionDocumentLimit+1)), 0600))
	_, err = readDecisionDocument(input)
	require.ErrorContains(t, err, "128 KiB")
	fifo := filepath.Join(t.TempDir(), "fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0600))
	_, err = readDecisionDocument(fifo)
	require.ErrorContains(t, err, "regular file")
}

func TestDecisionDocumentIOFailuresAreVisible(t *testing.T) {
	_, input, _ := decisionCLIFixture(t)
	open, read := decisionOpenFile, decisionReadAll
	t.Cleanup(func() { decisionOpenFile, decisionReadAll = open, read })
	decisionOpenFile = func(string) (*os.File, error) {
		f, err := os.Open(input)
		require.NoError(t, err)
		require.NoError(t, f.Close())
		return f, nil
	}
	_, err := readDecisionDocument(input)
	require.Error(t, err)
	decisionOpenFile = open
	decisionReadAll = func(io.Reader) ([]byte, error) { return nil, errors.New("read failed") }
	_, err = readDecisionDocument(input)
	require.ErrorContains(t, err, "read failed")
	decisionReadAll = func(io.Reader) ([]byte, error) { return make([]byte, decisionDocumentLimit+1), nil }
	_, err = readDecisionDocument(input)
	require.ErrorContains(t, err, "grew beyond")
}
