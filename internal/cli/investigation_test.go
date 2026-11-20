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
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/templates"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func investigationCLIFixture(t *testing.T) (string, string, contract.Preparation) {
	t.Helper()
	taskDir := newCLISandbox(t)
	cwd := filepath.Dir(filepath.Dir(taskDir))
	writeFileCli(t, filepath.Join(cwd, "settings.json"), `{"timeout":0}`)
	writeFileCli(t, filepath.Join(cwd, "src", "loader.go"), "package loader\nvar timeout = 0\n")
	gitCliCommitAll(t, cwd, "test: local diagnostic evidence")
	raw, err := templates.FS.ReadFile("preparation.example.json")
	require.NoError(t, err)
	p, err := contract.ValidatePreparation(raw)
	require.NoError(t, err)
	return taskDir, cwd, *p
}

func investigationControl(t *testing.T, taskDir, cwd string, request investigationRequest) (controlResponse, error) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetIn(strings.NewReader(mustJSONT(t, request)))
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"control", "investigate", "--task-dir", taskDir, "--cwd", cwd})
	err := cmd.Execute()
	require.Empty(t, stderr.String())
	var reply controlResponse
	require.NoError(t, json.Unmarshal(out.Bytes(), &reply), out.String())
	require.Equal(t, 1, reply.Protocol)
	require.Equal(t, 1, strings.Count(out.String(), "\n"), "control keeps its single JSON boundary")
	require.Equal(t, err == nil, reply.OK, reply.Error)
	return reply, err
}

func startInvestigationCLI(t *testing.T, taskDir, cwd string, p contract.Preparation) *state.PreparationState {
	t.Helper()
	reply, err := investigationControl(t, taskDir, cwd, investigationRequest{Action: "start", Contract: json.RawMessage(mustJSONT(t, p))})
	require.NoError(t, err, reply.Error)
	require.NotNil(t, reply.Preparation)
	require.Equal(t, "ready", reply.Preparation.Status)
	require.Nil(t, reply.State)
	return reply.Preparation
}

func investigationCLIReport(st *state.PreparationState) state.InvestigationReport {
	findings := []state.InvestigationFinding{}
	for i, h := range st.Contract.Hypotheses {
		finding := state.InvestigationFinding{Hypothesis: h.ID, Status: "unknown", Evidence: []string{}, Reason: "Local evidence does not settle every alternative."}
		if i == 0 {
			finding.Status, finding.Evidence = "supported", []string{st.Observations[0].Invocation.ID}
		}
		findings = append(findings, finding)
	}
	return state.InvestigationReport{Status: "supported", Summary: "The saved local evidence supports the documented hypothesis.", Findings: findings,
		CounterEvidence: "Runtime overrides are still outside this investigation.", Limitations: []string{"Local evidence only."},
		Recommendation: "Request repair authority separately.", Regression: "Reproduce the timeout default before a change.", Rollback: "Restore any later change if its regression fails."}
}

func TestInvestigationControlRejectsInvalidOrUnownedRequests(t *testing.T) {
	taskDir, cwd, p := investigationCLIFixture(t)
	for _, input := range []io.Reader{
		failingEditorReader{},
		strings.NewReader(""),
		strings.NewReader(strings.Repeat(" ", 131073)),
		strings.NewReader(`{"action":"status","unexpected":true}`),
		strings.NewReader(`{"action":"status"} {}`),
		strings.NewReader(`{"action":"cancel"}`),
		strings.NewReader(`{"action":"start"}`),
		strings.NewReader(`{"action":"status","contract":{}}`),
		strings.NewReader(`{"action":"report","revision":1}`),
		strings.NewReader(`{"action":"cancel","revision":1,"report":{}}`),
		strings.NewReader(`{"action":"unsupported","revision":1}`),
	} {
		cmd := &cobra.Command{}
		cmd.SetIn(input)
		_, err := controlInvestigation(cmd, taskDir, cwd)
		require.Error(t, err)
		require.NoFileExists(t, state.PreparationFile(taskDir))
		require.False(t, state.Exists(taskDir))
	}
	release, err := machine.AcquireRunLease(taskDir)
	require.NoError(t, err)
	defer release()
	reply, err := investigationControl(t, taskDir, cwd, investigationRequest{Action: "start", Contract: json.RawMessage(mustJSONT(t, p))})
	require.Error(t, err)
	require.NotEmpty(t, reply.Error)
	require.NoFileExists(t, state.PreparationFile(taskDir), "another lifetime owner prevents preparation mutation")
}

func TestInvestigationControlRunsReportsAndNeverImplementsReportOnlyTask(t *testing.T) {
	controlFixture(t, "real-run")
	// Wait for the actual short-lived child, so the returned result is its
	// persisted awaiting-report state rather than a transient ready snapshot.
	controlPollInterval = time.Hour
	taskDir, cwd, p := investigationCLIFixture(t)
	chdirT(t, cwd)
	st := startInvestigationCLI(t, taskDir, cwd, p)
	before, err := os.ReadFile(state.PreparationFile(taskDir))
	require.NoError(t, err)
	reply, err := investigationControl(t, taskDir, cwd, investigationRequest{Action: "status"})
	require.NoError(t, err)
	require.Equal(t, st.Revision, reply.Preparation.Revision)
	code, human := runCLI(t, "status", taskDir)
	require.Zero(t, code, human)
	require.Contains(t, human, p.Goal)
	after, err := os.ReadFile(state.PreparationFile(taskDir))
	require.NoError(t, err)
	require.Equal(t, before, after, "both status surfaces are read-only")

	reply, err = investigationControl(t, taskDir, cwd, investigationRequest{Action: "run", Revision: st.Revision})
	require.NoError(t, err, reply.Error)
	require.Equal(t, "awaiting-report", reply.Preparation.Status)
	require.False(t, reply.Running)
	require.Len(t, reply.Preparation.Observations, 3)
	require.False(t, state.Exists(taskDir), "bounded investigation is not implementation completion")
	report := investigationCLIReport(reply.Preparation)
	_, err = investigationControl(t, taskDir, cwd, investigationRequest{Action: "report", Revision: st.Revision, Report: &report})
	require.Error(t, err, "a stale observer cannot close a newer investigation")
	reply, err = investigationControl(t, taskDir, cwd, investigationRequest{Action: "report", Revision: reply.Preparation.Revision, Report: &report})
	require.NoError(t, err, reply.Error)
	require.Equal(t, "supported", reply.Preparation.Status)
	code, human = runCLI(t, "status", taskDir)
	require.Zero(t, code, human)
	require.Contains(t, human, report.Summary)
	code, human = runCLI(t, "run", taskDir)
	require.Equal(t, 1, code, human)
	require.Contains(t, human, "repair authority")
	require.False(t, state.Exists(taskDir), "even existing Spec and packets do not expand investigate intent")
	code, stopped := runControlTest(t, "stop", taskDir, cwd)
	require.Zero(t, code, stopped.Error)
	require.Equal(t, "supported", stopped.Preparation.Status, "stopping an already closed report preserves its conclusion")

	taskDir, cwd, p = investigationCLIFixture(t)
	st = startInvestigationCLI(t, taskDir, cwd, p)
	reply, err = investigationControl(t, taskDir, cwd, investigationRequest{Action: "cancel", Revision: st.Revision})
	require.NoError(t, err, reply.Error)
	require.Equal(t, "cancelled", reply.Preparation.Status)
	require.Zero(t, reply.Preparation.ActionsUsed)
}

func TestInvestigationRunExitCodesAndUnknownOwnership(t *testing.T) {
	controlFixture(t, "real-run")
	controlPollInterval = time.Hour
	taskDir, cwd, p := investigationCLIFixture(t)
	st := startInvestigationCLI(t, taskDir, cwd, p)
	code, output := runCLI(t, "run", taskDir, "--investigate", "--expect-revision", strconv.FormatInt(st.Revision, 10))
	require.Zero(t, code, output)
	require.Contains(t, output, `"status": "awaiting-report"`)

	for _, extra := range [][]string{{}, {"--expect-revision", "0"}, {"--expect-revision", "1", "--mode", "quick"}, {"--expect-revision", "1", "--only", "execute"}} {
		args := append([]string{"run", taskDir, "--investigate"}, extra...)
		code, output = runCLI(t, args...)
		require.Equal(t, 1, code, output)
		require.Contains(t, output, "requires only an observed positive")
	}
	code, output = runCLI(t, "run", filepath.Join(taskDir, "missing"), "--investigate", "--expect-revision", "1")
	require.Equal(t, 1, code, output)

	taskDir, cwd, p = investigationCLIFixture(t)
	st = startInvestigationCLI(t, taskDir, cwd, p)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := newRootCmd()
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"run", taskDir, "--investigate", "--expect-revision", strconv.FormatInt(st.Revision, 10)})
	err := cmd.Execute()
	var exit *exitError
	require.True(t, errors.As(err, &exit))
	require.Equal(t, 143, exit.code)
	st, err = state.LoadPreparation(taskDir)
	require.NoError(t, err)
	code, output = runCLI(t, "run", taskDir, "--investigate", "--expect-revision", strconv.FormatInt(st.Revision, 10))
	require.Equal(t, 143, code, output)
	require.Contains(t, output, `"status": "cancelled"`)

	taskDir, cwd, p = investigationCLIFixture(t)
	st = startInvestigationCLI(t, taskDir, cwd, p)
	st.Pending = &state.PreparationInvocation{ID: state.PreparationHash("lost-cli-action"), ActionID: p.Actions[0].ID,
		BindingHash: state.PreparationHash(p.Actions[0]), CandidateHash: state.PreparationHash("source"), StartedAt: st.StartedAt, Reason: "process exited without a trustworthy receipt"}
	st.Status, st.ActionsUsed = "running", 1
	st, err = state.SavePreparation(taskDir, st)
	require.NoError(t, err)
	code, output = runCLI(t, "run", taskDir, "--investigate", "--expect-revision", strconv.FormatInt(st.Revision, 10))
	require.Equal(t, 3, code, output)
	require.Contains(t, output, "action-outcome-unknown")
	st, err = state.LoadPreparation(taskDir)
	require.NoError(t, err)
	reply, err := investigationControl(t, taskDir, cwd, investigationRequest{Action: "run", Revision: st.Revision})
	require.NoError(t, err, reply.Error)
	require.Equal(t, "blocked", reply.Preparation.Status, "a child exit 3 is a valid preparation result")
	require.Equal(t, st.Pending, reply.Preparation.Pending)
	require.Equal(t, 1, reply.Preparation.ActionsUsed)
	require.Empty(t, reply.Preparation.Observations)
	require.False(t, state.Exists(taskDir))
}

func TestInvestigationBackgroundStopCancelsOwnedDecisionWithoutImplementation(t *testing.T) {
	controlFixture(t, "real-run")
	stopPollFast(t)
	stopPollAttempts = 300
	taskDir, cwd, p := investigationCLIFixture(t)
	arrived, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(arrived)
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	t.Setenv("PREPARATION_CLI_FIXTURE_KEY", "fixture-key")
	cfg, err := config.LoadReadOnly(taskDir, cwd)
	require.NoError(t, err)
	cfg.JEV.Enabled, cfg.JEV.Endpoint, cfg.JEV.KeyEnv, cfg.JEV.TimeoutMS = true, server.URL+"/v1/systemone", "PREPARATION_CLI_FIXTURE_KEY", 10000
	writeFileCli(t, filepath.Join(taskDir, "flywheel.config.json"), mustJSONT(t, cfg))
	st := startInvestigationCLI(t, taskDir, cwd, p)
	reply, err := investigationControl(t, taskDir, cwd, investigationRequest{Action: "run", Revision: st.Revision})
	require.NoError(t, err, reply.Error)
	require.True(t, reply.Running)
	require.Positive(t, reply.PID)
	t.Cleanup(func() { _ = syscall.Kill(reply.PID, syscall.SIGKILL) })
	select {
	case <-arrived:
	case <-time.After(3 * time.Second):
		t.Fatal("real investigation child did not dispatch the local fixture decision")
	}
	code, stopped := runControlTest(t, "stop", taskDir, cwd)
	require.Zero(t, code, stopped.Error)
	require.False(t, stopped.Running)
	require.Equal(t, "cancelled", stopped.Preparation.Status)
	require.Zero(t, stopped.Preparation.ActionsUsed, "cancelled advice cannot launch the next diagnostic action")
	require.Nil(t, stopped.Preparation.DecisionPending)
	require.Nil(t, stopped.Preparation.Pending)
	require.Len(t, stopped.Preparation.Decisions, 1)
	require.Equal(t, "unknown", stopped.Preparation.Decisions[0].Status)
	require.Nil(t, stopped.Preparation.Decisions[0].Usage.Input)
	require.Nil(t, stopped.Preparation.Decisions[0].Usage.Output)
	require.False(t, state.Exists(taskDir))
	require.NoFileExists(t, machine.PidFile(taskDir))
	held, err := machine.RunLeaseHeld(taskDir)
	require.NoError(t, err)
	require.False(t, held)
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("the owned HTTP request survived the run's cancellation")
	}
}

func TestInvestigationStatusAndStopRejectCorruptPreparation(t *testing.T) {
	taskDir, cwd, p := investigationCLIFixture(t)
	startInvestigationCLI(t, taskDir, cwd, p)
	writeFileCli(t, state.PreparationFile(taskDir), "invalid preparation JSON")
	code, output := runCLI(t, "status", taskDir)
	require.Equal(t, 1, code, output)
	code, reply := runControlTest(t, "status", taskDir, cwd)
	require.Equal(t, 1, code)
	require.NotEmpty(t, reply.Error)
	code, reply = runControlTest(t, "stop", taskDir, cwd)
	require.Equal(t, 1, code)
	require.NotEmpty(t, reply.Error)
	raw, err := os.ReadFile(state.PreparationFile(taskDir))
	require.NoError(t, err)
	require.Equal(t, "invalid preparation JSON", string(raw), "stop cannot replace untrusted provenance with a cancelled claim")
}

func TestInvestigationContinueControlKeepsOneTaskAndRejectsAStaleRevision(t *testing.T) {
	taskDir, cwd, p := investigationCLIFixture(t)
	st := startInvestigationCLI(t, taskDir, cwd, p)
	_, err := investigationControl(t, taskDir, cwd, investigationRequest{Action: "continue", Revision: st.Revision, Contract: json.RawMessage(mustJSONT(t, p))})
	require.Error(t, err, "a round must first settle, even with correct revision")
	_, err = investigationControl(t, taskDir, cwd, investigationRequest{Action: "continue", Revision: st.Revision})
	require.Error(t, err, "continue requires an explicit evidence-linked contract")
	require.False(t, state.Exists(taskDir), "a request for another diagnostic round never starts implementation")
}
