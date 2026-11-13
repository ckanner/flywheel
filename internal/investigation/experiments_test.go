package investigation

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/require"
)

func commandAction(t *testing.T, script string) (string, contract.InvestigationAction, contract.InvestigationLimits) {
	t.Helper()
	cwd := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "reproduce.sh"), []byte(script), 0600))
	a := contract.InvestigationAction{ID: "reproduce", Kind: "run_command", Purpose: "Run the approved reproducer", Hypotheses: []string{"default"}, Command: &contract.InvestigationCommand{Argv: []string{"/bin/sh", "reproduce.sh"}, Inputs: []string{"reproduce.sh"}, Env: []string{}, TimeoutMS: 2000, ExpectExit: 0, ExecutionBoundary: "host-process"}}
	return cwd, a, contract.InvestigationLimits{MaxSeconds: 30, MaxFileBytes: 4096, MaxOutputBytes: 4096, MaxActions: 8}
}

func TestExperimentUsesCopiedInputsExplicitEnvironmentAndExactArgv(t *testing.T) {
	cwd, a, limits := commandAction(t, "printf '%s\\n' \"$APP_AUTH\" \"$SHOULD_NOT_INHERIT\" \"$HOME\"\nprintf changed > reproduce.sh\nprintf '%s' \"$1\"\n")
	a.Command.Argv = append(a.Command.Argv, "literal $(touch forbidden) ; value")
	a.Command.Env = []string{"APP_AUTH"}
	t.Setenv("SHOULD_NOT_INHERIT", "secret-from-parent")
	r := performAction(context.Background(), cwd, a, limits, 4096, Options{LookupEnv: func(k string) (string, bool) {
		if k == "APP_AUTH" {
			return "approved-secret", true
		}
		return os.LookupEnv(k)
	}})
	require.NoError(t, r.Err)
	require.False(t, r.Unknown)
	require.False(t, r.Truncated)
	require.Contains(t, r.Output, "[REDACTED]")
	require.NotContains(t, r.Output, "approved-secret")
	require.NotContains(t, r.Output, "secret-from-parent")
	require.Contains(t, r.Output, "literal $(touch forbidden) ; value")
	require.Contains(t, r.Output, r.Execution.Workspace)
	require.Equal(t, "removed", r.Execution.Cleanup)
	require.NotEmpty(t, r.Execution.InputHash)
	require.NotEmpty(t, r.ContentHash)
	_, err := os.Stat(r.Execution.Workspace)
	require.True(t, os.IsNotExist(err))
	raw, err := os.ReadFile(filepath.Join(cwd, "reproduce.sh"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "printf")
	_, err = os.Stat(filepath.Join(cwd, "forbidden"))
	require.True(t, os.IsNotExist(err))
}

func TestExperimentPhysicalFailuresAreNotSuccessfulEvidence(t *testing.T) {
	for _, scenario := range []string{"exit", "expected-exit", "timeout", "cancel", "output", "spawn", "missing-env", "missing-input", "directory", "changed-input", "copy-race"} {
		t.Run(scenario, func(t *testing.T) {
			cwd, a, limits := commandAction(t, "printf evidence\n")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := Options{}
			if scenario == "exit" || scenario == "expected-exit" {
				require.NoError(t, os.WriteFile(filepath.Join(cwd, "reproduce.sh"), []byte("printf reproduced; exit 1\n"), 0600))
			}
			if scenario == "expected-exit" {
				a.Command.ExpectExit = 1
			}
			if scenario == "timeout" {
				a.Command.TimeoutMS = 30
				require.NoError(t, os.WriteFile(filepath.Join(cwd, "reproduce.sh"), []byte("trap 'exit 0' TERM\nwhile :; do :; done\n"), 0600))
			}
			if scenario == "cancel" {
				cancel()
			}
			if scenario == "output" {
				require.NoError(t, os.WriteFile(filepath.Join(cwd, "reproduce.sh"), []byte("while :; do printf '0123456789'; done\n"), 0600))
			}
			if scenario == "spawn" {
				a.Command.Argv[0] = "/not/a/program"
			}
			if scenario == "missing-env" {
				a.Command.Env = []string{"APP_AUTH"}
				opts.LookupEnv = func(string) (string, bool) { return "", false }
			}
			if scenario == "missing-input" {
				a.Command.Inputs = []string{"absent"}
			}
			if scenario == "directory" {
				a.Command.Inputs = []string{"directory"}
				require.NoError(t, os.Mkdir(filepath.Join(cwd, "directory"), 0700))
			}
			if scenario == "changed-input" {
				old := runExperimentProcess
				t.Cleanup(func() { runExperimentProcess = old })
				runExperimentProcess = func(command string, args []string, o watchdog.Opts) watchdog.Result {
					require.NoError(t, os.WriteFile(filepath.Join(cwd, "reproduce.sh"), []byte("changed"), 0600))
					code := 0
					return watchdog.Result{ExitCode: &code, Stdout: "evidence"}
				}
			}
			if scenario == "copy-race" {
				old := experimentWrite
				t.Cleanup(func() { experimentWrite = old })
				experimentWrite = func(path string, b []byte, mode os.FileMode) error {
					require.NoError(t, os.WriteFile(filepath.Join(cwd, "reproduce.sh"), []byte("changed"), 0600))
					return old(path, b, mode)
				}
			}
			r := performAction(ctx, cwd, a, limits, 128, opts)
			if scenario == "expected-exit" {
				require.NoError(t, r.Err)
				require.Equal(t, 1, *r.Execution.ExitCode)
			} else {
				require.Error(t, r.Err)
			}
			require.Equal(t, "removed", r.Execution.Cleanup)
			if scenario == "timeout" {
				require.True(t, r.Execution.TimedOut)
			}
			if scenario == "output" {
				require.True(t, r.Truncated)
				require.LessOrEqual(t, len(r.Output), 128)
			}
		})
	}
}

func TestExperimentSetupAndCleanupFailuresRetainHonestOutcomes(t *testing.T) {
	for _, scenario := range []string{"temp", "mkdir", "write", "remove", "process-cleanup", "before-cancel", "after-cancel"} {
		t.Run(scenario, func(t *testing.T) {
			cwd, a, limits := commandAction(t, "printf evidence\n")
			originalTemp, originalMkdir, originalWrite, originalRemove, originalRun := experimentTempDir, experimentMkdir, experimentWrite, experimentRemove, runExperimentProcess
			t.Cleanup(func() {
				experimentTempDir, experimentMkdir, experimentWrite, experimentRemove, runExperimentProcess = originalTemp, originalMkdir, originalWrite, originalRemove, originalRun
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "temp" {
				experimentTempDir = func(string, string) (string, error) { return "", os.ErrPermission }
			}
			if scenario == "mkdir" {
				experimentMkdir = func(string, os.FileMode) error { return os.ErrPermission }
			}
			if scenario == "write" {
				experimentWrite = func(string, []byte, os.FileMode) error { return os.ErrPermission }
			}
			if scenario == "remove" {
				experimentRemove = func(string) error { return os.ErrPermission }
			}
			if scenario == "process-cleanup" {
				runExperimentProcess = func(string, []string, watchdog.Opts) watchdog.Result {
					code := 0
					return watchdog.Result{ExitCode: &code, Stdout: "evidence", CleanupError: "unknown group"}
				}
			}
			if scenario == "before-cancel" {
				experimentWrite = func(path string, b []byte, mode os.FileMode) error {
					err := originalWrite(path, b, mode)
					cancel()
					return err
				}
			}
			if scenario == "after-cancel" {
				runExperimentProcess = func(string, []string, watchdog.Opts) watchdog.Result {
					cancel()
					code := 0
					return watchdog.Result{ExitCode: &code, Stdout: "evidence"}
				}
			}
			r := runCommand(ctx, cwd, a, limits, 4096, Options{})
			require.Error(t, r.Err)
			if scenario == "remove" || scenario == "process-cleanup" {
				require.True(t, r.Unknown)
				require.Equal(t, "retained", r.Execution.Cleanup)
				require.DirExists(t, r.Execution.Workspace)
				require.NoError(t, os.RemoveAll(r.Execution.Workspace))
			} else {
				require.False(t, r.Unknown)
			}
		})
	}
	output, truncated := boundedOutput("中\xff文", 4)
	require.Equal(t, "中", output)
	require.True(t, truncated)
	require.Equal(t, "[REDACTED] [REDACTED]", redactExperiment("secret-long secret", []string{"secret", "", "secret-long"}))
	t.Setenv("EXPERIMENT_LOOKUP", "value")
	value, ok := (Options{}).lookup("EXPERIMENT_LOOKUP")
	require.True(t, ok)
	require.Equal(t, "value", value)
}

func TestUnknownExperimentCleanupBlocksWithoutRepeatingProcess(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	p.Actions = []contract.InvestigationAction{{ID: "experiment", Kind: "run_command", Purpose: "Inspect copied config", Hypotheses: []string{p.Hypotheses[0].ID}, Command: &contract.InvestigationCommand{Argv: []string{"/bin/cat", "settings.json"}, Inputs: []string{"settings.json"}, Env: []string{}, TimeoutMS: 1000, ExecutionBoundary: "host-process"}}}
	startFixture(t, dir, cwd, p)
	old := runExperimentProcess
	defer func() { runExperimentProcess = old }()
	calls := 0
	runExperimentProcess = func(string, []string, watchdog.Opts) watchdog.Result {
		calls++
		code := 0
		return watchdog.Result{ExitCode: &code, CleanupError: "cannot establish quiescence"}
	}
	st, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, "blocked", st.Status)
	require.NotNil(t, st.Pending)
	require.Empty(t, st.Observations)
	receipt, err := state.ReadPreparationReceipt(dir, *st.Pending)
	require.NoError(t, err)
	require.Equal(t, "unknown", receipt.Status)
	require.Equal(t, "retained", receipt.Execution.Cleanup)
	t.Cleanup(func() { _ = os.RemoveAll(receipt.Execution.Workspace) })
	st, err = Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, "blocked", st.Status)
	require.Equal(t, 1, calls)
	_, err = Cancel(dir, Options{})
	require.ErrorContains(t, err, "unresolved")
}

func TestCommandPhysicalRecordAndLogCategorySurviveRecovery(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	p.Actions = []contract.InvestigationAction{{ID: "experiment", Kind: "run_command", Purpose: "Inspect copied config", Hypotheses: []string{p.Hypotheses[0].ID}, Command: &contract.InvestigationCommand{Argv: []string{"/bin/cat", "settings.json"}, Inputs: []string{"settings.json"}, Env: []string{}, TimeoutMS: 1000, ExecutionBoundary: "host-process"}}}
	startFixture(t, dir, cwd, p)
	st, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, "awaiting-report", st.Status)
	require.Len(t, st.Observations, 1)
	require.Equal(t, "observed", st.Observations[0].Status)
	require.Equal(t, "removed", st.Observations[0].Execution.Cleanup)
	loaded, err := state.LoadPreparation(dir)
	require.NoError(t, err)
	require.Equal(t, st.Observations, loaded.Observations)
	s := snapshot(loaded, p.Actions, loaded.Observations[0].Invocation.CandidateHash)
	require.Equal(t, "diagnostic-facts", string(s.Facts[len(s.Facts)-1].Category))
	require.Contains(t, s.MissingFacts[0], "logs are not authorized")
	require.NotContains(t, s.Facts[len(s.Facts)-1].Text, `"timeout":0`)
	loaded.Contract.Decision = &contract.InvestigationDecision{Policy: decision.Policy{AllowedData: []decision.Category{decision.Logs}}}
	s = snapshot(loaded, p.Actions, loaded.Observations[0].Invocation.CandidateHash)
	require.Equal(t, "logs", string(s.Facts[len(s.Facts)-1].Category))
}

func TestExperimentPreservesExecutableInputsAndBindsTheirMode(t *testing.T) {
	cwd, a, limits := commandAction(t, "#!/bin/sh\nprintf executable\n")
	require.NoError(t, os.Chmod(filepath.Join(cwd, "reproduce.sh"), 0700))
	a.Command.Argv = []string{"./reproduce.sh"}
	r := runCommand(context.Background(), cwd, a, limits, 4096, Options{})
	require.NoError(t, r.Err)
	require.Equal(t, "executable", r.Output)
	st := &state.PreparationState{Cwd: cwd, Contract: contract.Preparation{ReadPaths: a.Command.Inputs, Actions: []contract.InvestigationAction{a}, Limits: limits}}
	before, err := candidate(context.Background(), st)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(filepath.Join(cwd, "reproduce.sh"), 0600))
	after, err := candidate(context.Background(), st)
	require.NoError(t, err)
	require.NotEqual(t, before, after)
}

func TestTruncatedCommandCredentialPrefixesAreNotPersisted(t *testing.T) {
	cwd, a, limits := commandAction(t, "printf unused")
	a.Command.Env = []string{"AUTH_TOKEN"}
	old := runExperimentProcess
	defer func() { runExperimentProcess = old }()
	runExperimentProcess = func(string, []string, watchdog.Opts) watchdog.Result {
		code := 0
		return watchdog.Result{ExitCode: &code, Stdout: "stdout prefix-secret", Stderr: "stderr prefix-sec", Truncated: true}
	}
	r := runCommand(context.Background(), cwd, a, limits, 4096, Options{LookupEnv: func(string) (string, bool) { return "prefix-secret-sensitive", true }})
	require.Error(t, r.Err)
	require.NotContains(t, r.Output, "prefix-sec")
	require.Contains(t, r.Output, "[REDACTED]")
}

func TestJEVAdoptsCommandAndHTTPChoicesWithoutRequiringRawLogs(t *testing.T) {
	for _, kind := range []string{"run_command", "http_read"} {
		t.Run(kind, func(t *testing.T) {
			dir, cwd, p := preparationFixture(t)
			calls, reads := 0, 0
			service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reads++; w.Write([]byte("private-runtime-detail")) }))
			defer service.Close()
			a := contract.InvestigationAction{ID: "runtime-evidence", Kind: kind, Purpose: "Gather runtime evidence", Hypotheses: []string{p.Hypotheses[0].ID, p.Hypotheses[1].ID}}
			if kind == "run_command" {
				a.Command = &contract.InvestigationCommand{Argv: []string{"/bin/cat", "settings.json"}, Inputs: []string{"settings.json"}, Env: []string{}, TimeoutMS: 1000, ExecutionBoundary: "host-process"}
			} else {
				a.HTTP = httpAction(service.URL).HTTP
			}
			p.Actions = append(p.Actions, a)
			jev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				raw, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NotContains(t, string(raw), "private-runtime-detail")
				var request struct {
					State struct {
						Facts []decision.Fact `json:"facts"`
					} `json:"state"`
				}
				require.NoError(t, json.Unmarshal(raw, &request))
				for _, fact := range request.State.Facts {
					require.NotEqual(t, decision.Logs, fact.Category)
					require.NotContains(t, fact.Text, `"timeout":0`)
				}
				if calls == 2 {
					require.Contains(t, string(raw), "logs are not authorized")
					require.Contains(t, string(raw), "status=observed")
				}
				r.Body = io.NopCloser(bytes.NewReader(raw))
				selectResponse(t, w, r, func(criteria map[string]string) string {
					if calls == 1 {
						return a.ID
					}
					return controlID(t, criteria, "jev-finish-")
				})
			}))
			defer jev.Close()
			withDecision(&p, jev.URL+"/v1/systemone")
			p.Decision.Policy.MaxCalls = 8
			p.Decision.Policy.AllowedData = []decision.Category{decision.TaskSummary, decision.DiagnosticFacts, decision.Source}
			startFixture(t, dir, cwd, p)
			st, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { return "key", true }})
			require.NoError(t, err)
			require.Equal(t, 2, calls)
			require.Len(t, st.Observations, 1)
			require.Equal(t, a.ID, st.Observations[0].Invocation.ActionID)
			require.True(t, st.Observations[0].Invocation.Adopted)
			require.Equal(t, "jev-evidence-ready-for-report", st.Reason)
			if kind == "http_read" {
				require.Equal(t, 1, reads)
			}
		})
	}
}

func TestExperimentChangedCandidateAndCancellationCannotBecomeEvidence(t *testing.T) {
	for _, scenario := range []string{"changed", "cancel", "failed"} {
		t.Run(scenario, func(t *testing.T) {
			dir, cwd, p := preparationFixture(t)
			p.Actions = []contract.InvestigationAction{{ID: "experiment", Kind: "run_command", Purpose: "Inspect copied config", Hypotheses: []string{p.Hypotheses[0].ID}, Command: &contract.InvestigationCommand{Argv: []string{"/bin/cat", "settings.json"}, Inputs: []string{"settings.json"}, Env: []string{}, TimeoutMS: 1000, ExecutionBoundary: "host-process"}}}
			startFixture(t, dir, cwd, p)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			old := runExperimentProcess
			t.Cleanup(func() { runExperimentProcess = old })
			runExperimentProcess = func(string, []string, watchdog.Opts) watchdog.Result {
				code := 0
				if scenario == "changed" {
					require.NoError(t, os.WriteFile(filepath.Join(cwd, "src", "loader.go"), []byte("changed"), 0600))
				}
				if scenario == "cancel" {
					cancel()
				}
				if scenario == "failed" {
					code = 1
				}
				return watchdog.Result{ExitCode: &code, Stdout: "evidence"}
			}
			st, err := Run(ctx, dir, Options{})
			require.NoError(t, err)
			require.Len(t, st.Observations, 1)
			require.NotEqual(t, "observed", st.Observations[0].Status)
		})
	}
}

// Keep otherwise unusual physical outcomes explicit without launching an
// unkillable real process to exercise cleanup/error defenses.
func TestExperimentSignalTruncationAndLateCancellationAreFailures(t *testing.T) {
	for _, scenario := range []string{"signal", "truncated", "late-cancel"} {
		t.Run(scenario, func(t *testing.T) {
			cwd, a, limits := commandAction(t, "printf evidence")
			old := runExperimentProcess
			t.Cleanup(func() { runExperimentProcess = old })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runExperimentProcess = func(string, []string, watchdog.Opts) watchdog.Result {
				code := 0
				r := watchdog.Result{ExitCode: &code, Stdout: "evidence"}
				if scenario == "signal" {
					r.Signal = "terminated"
				}
				if scenario == "truncated" {
					r.Truncated = true
				}
				if scenario == "late-cancel" {
					cancel()
				}
				return r
			}
			r := runCommand(ctx, cwd, a, limits, 4096, Options{})
			require.Error(t, r.Err)
		})
	}
}
