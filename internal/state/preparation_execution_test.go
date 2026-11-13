package state

import (
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/stretchr/testify/require"
)

func executionObservation(st *PreparationState, kind string) InvestigationObservation {
	o := preparationObservation(st, 0)
	zero := 0
	o.Execution = &InvestigationExecution{Kind: kind, ExitCode: &zero, Workspace: "/tmp/approved-copy", Cleanup: "removed", InputHash: PreparationHash("inputs")}
	if kind == "http_read" {
		o.Execution = &InvestigationExecution{Kind: kind, HTTPStatus: 200, Cleanup: "not-created"}
	}
	return o
}

func TestExecutionObservationPhysicalInvariants(t *testing.T) {
	st := preparationFixture(t)
	for _, kind := range []string{"run_command", "http_read"} {
		require.NoError(t, validatePreparationObservation(executionObservation(st, kind)))
	}
	for name, mutate := range map[string]func(*InvestigationObservation){
		"HTTP workspace": func(o *InvestigationObservation) {
			o.Execution = executionObservation(st, "http_read").Execution
			o.Execution.Workspace = "/tmp/leak"
		},
		"HTTP exit": func(o *InvestigationObservation) {
			o.Execution = executionObservation(st, "http_read").Execution
			v := 0
			o.Execution.ExitCode = &v
		},
		"HTTP cleanup": func(o *InvestigationObservation) {
			o.Execution = executionObservation(st, "http_read").Execution
			o.Execution.Cleanup = "removed"
		},
		"HTTP unknown": func(o *InvestigationObservation) {
			o.Execution = executionObservation(st, "http_read").Execution
			o.Status, o.Error = "unknown", "unknown"
		},
		"HTTP unexpected status": func(o *InvestigationObservation) {
			o.Execution = executionObservation(st, "http_read").Execution
			o.Execution.HTTPStatus = 503
		},
		"unknown kind":       func(o *InvestigationObservation) { o.Execution.Kind = "unknown" },
		"relative workspace": func(o *InvestigationObservation) { o.Execution.Workspace = "relative" },
		"timed out":          func(o *InvestigationObservation) { o.Execution.TimedOut = true },
		"cancelled":          func(o *InvestigationObservation) { o.Execution.Cancelled = true },
		"signal":             func(o *InvestigationObservation) { o.Execution.Signal = "terminated" },
		"unfinished cleanup": func(o *InvestigationObservation) { o.Execution.Cleanup = "retained" },
		"no exit":            func(o *InvestigationObservation) { o.Execution.ExitCode = nil },
		"no input identity":  func(o *InvestigationObservation) { o.Execution.InputHash = "" },
		"unknown removed":    func(o *InvestigationObservation) { o.Status, o.Error = "unknown", "unknown" },
		"unknown no workspace": func(o *InvestigationObservation) {
			o.Status, o.Error = "unknown", "unknown"
			o.Execution.Cleanup = "retained"
			o.Execution.Workspace = ""
		},
		"unknown no execution": func(o *InvestigationObservation) { o.Status, o.Error = "unknown", "unknown"; o.Execution = nil },
	} {
		t.Run(name, func(t *testing.T) {
			o := executionObservation(st, "run_command")
			mutate(&o)
			require.Error(t, validatePreparationObservation(o))
		})
	}
	o := executionObservation(st, "run_command")
	o.Status, o.Error = "unknown", "cleanup cannot be established"
	o.Execution.Cleanup = "retained"
	require.NoError(t, validatePreparationObservation(o))
	dir := t.TempDir()
	require.NoError(t, SavePreparationReceipt(dir, o))
	loaded, err := ReadPreparationReceipt(dir, o.Invocation)
	require.NoError(t, err)
	require.Equal(t, o, *loaded)
	o.Invocation.ID = PreparationHash("other")
	o.Execution.Cleanup = "removed"
	require.Error(t, SavePreparationReceipt(dir, o))
	o.Status = "not-a-status"
	require.Error(t, SavePreparationReceipt(dir, o))
}

func TestStoredExecutionMustMatchItsApprovedAction(t *testing.T) {
	makeState := func(kind string) *PreparationState {
		st := observedPreparation(t)
		a := &st.Contract.Actions[0]
		a.Kind, a.Path, a.Query = kind, "", ""
		if kind == "run_command" {
			a.Command = &contract.InvestigationCommand{Argv: []string{"/bin/cat", "settings.json"}, Inputs: []string{"settings.json"}, Env: []string{}, TimeoutMS: 1000, ExecutionBoundary: "host-process"}
		} else {
			a.HTTP = &contract.InvestigationHTTP{URL: "https://example.invalid/health", Method: "GET", HeadersEnv: map[string]string{}, TimeoutMS: 1000, MaxBytes: 128, ExpectStatus: 200}
		}
		st.ContractHash = PreparationHash(st.Contract)
		st.Observations[0] = executionObservation(st, kind)
		st.Observations[0].Invocation.BindingHash = PreparationHash(*a)
		return st
	}
	for _, kind := range []string{"run_command", "http_read"} {
		st := makeState(kind)
		require.NoError(t, validatePreparationState(st))
		st.Observations[0].Execution = nil
		require.ErrorContains(t, validatePreparationState(st), "does not match")
		st = makeState(kind)
		if kind == "run_command" {
			v := 1
			st.Observations[0].Execution.ExitCode = &v
		} else {
			st.Observations[0].Execution.HTTPStatus = 201
		}
		require.ErrorContains(t, validatePreparationState(st), "does not match")
	}
	st := observedPreparation(t)
	st.Observations[0].Execution = executionObservation(st, "run_command").Execution
	require.ErrorContains(t, validatePreparationState(st), "file observation")
	st = makeState("run_command")
	st.Observations[0].Status, st.Observations[0].Error = "unknown", "unknown"
	st.Observations[0].Execution.Cleanup = "retained"
	require.ErrorContains(t, validatePreparationState(st), "must remain pending")
}

func TestExperimentFailuresCannotAuthorizeCollectionShortcuts(t *testing.T) {
	for _, scenario := range []string{"nonzero", "timeout", "cancel", "signal", "nil-exit", "retained", "bad-http"} {
		t.Run(scenario, func(t *testing.T) {
			st := observedPreparation(t)
			// This selector also rejects malformed physical records defensively;
			// normal Load already rejects impossible successful observations.
			e := executionObservation(st, "run_command").Execution
			if scenario == "nonzero" {
				v := 1
				e.ExitCode = &v
			}
			if scenario == "timeout" {
				e.TimedOut = true
			}
			if scenario == "cancel" {
				e.Cancelled = true
			}
			if scenario == "signal" {
				e.Signal = "terminated"
			}
			if scenario == "nil-exit" {
				e.ExitCode = nil
			}
			if scenario == "retained" {
				e.Cleanup = "retained"
			}
			if scenario == "bad-http" {
				e = executionObservation(st, "http_read").Execution
				e.HTTPStatus = 500
			}
			st.Observations[0].Execution = e
			require.Empty(t, InvestigationControls(st, st.Observations[0].Invocation.CandidateHash))
		})
	}
}
