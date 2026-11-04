package modules

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdviceModulesStopWhenCleanupIsUnknown(t *testing.T) {
	for _, role := range []string{"intake", "garden"} {
		t.Run(role, func(t *testing.T) {
			taskDir := t.TempDir()
			cfg := testConfigT(t, taskDir)
			writeFileT(t, filepath.Join(taskDir, "state.json"), `{"revision":0,"status":"pending","packets":{},"inbox":[],"budget":{"daily":{},"cache":{"by-provider":{}}}}`)
			stub := newStub(map[string]drivers.Result{role: drivers.MockResult(func(r *drivers.Result) {
				r.CleanupError = "owned process group disappearance was not confirmed"
				r.Settled = false
				r.TimedOut = role == "garden"
				message := "429 rate limit"
				r.ErrorMessage = &message
			})})
			m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: &drivers.Drivers{Intake: stub, Garden: stub},
				Sleep: func(int64) { t.Error("unconfirmed cleanup must not enter retry backoff") }})
			o := CallOptions{Role: role, Binding: bindingFor(t, cfg, role), Prompt: "advice"}
			r := CallAgent(m, o)
			assert.False(t, r.OK)
			if role == "garden" {
				assert.Equal(t, errs.TIMEOUT, r.ErrorCode)
			} else {
				assert.Equal(t, errs.TRANSPORT, r.ErrorCode)
			}
			require.ErrorContains(t, m.fatalErr, "cleanup is unconfirmed")
			assert.False(t, CallAgent(m, o).OK, "fatal cleanup must block subsequent candidates")
			assert.Equal(t, []string{role}, stub.calls)
			st, err := state.Load(taskDir)
			require.NoError(t, err)
			assert.Equal(t, "escalated", st.Status)
			require.Len(t, st.Inbox, 1)
			assert.Equal(t, role, st.Inbox[0].(map[string]any)["role"])
			inbox, err := os.ReadFile(filepath.Join(taskDir, "inbox.md"))
			require.NoError(t, err)
			assert.Contains(t, string(inbox), "confirm the owned processes have ended")
			lines := readMetricsLinesT(t, m.MetricsFile)
			require.Len(t, lines, 1)
			assert.Equal(t, role, lines[0]["role"])
			assert.Equal(t, "owned process group disappearance was not confirmed", lines[0]["cleanup-error"])
		})
	}
}

func TestModuleCleanupEscalationHonorsStateOwnershipAndPersistence(t *testing.T) {
	for _, condition := range []string{"absent-state", "active-orchestrator", "lease-without-pid", "run-lease-failure", "invalid-state", "state-save-failure", "inbox-failure", "metrics-failure"} {
		t.Run(condition, func(t *testing.T) {
			taskDir := t.TempDir()
			cfg := testConfigT(t, taskDir)
			statePath := filepath.Join(taskDir, "state.json")
			if condition != "absent-state" {
				writeFileT(t, statePath, `{"revision":7,"status":"running","packets":{},"inbox":[],"budget":{"daily":{},"cache":{"by-provider":{}}}}`)
			}
			switch condition {
			case "active-orchestrator":
				writeFileT(t, filepath.Join(taskDir, "orchestrator.pid"), `{"pid":123}`)
			case "lease-without-pid":
				release, err := machine.AcquireRunLease(taskDir)
				require.NoError(t, err)
				t.Cleanup(release)
				assert.NoFileExists(t, filepath.Join(taskDir, "orchestrator.pid"))
			case "run-lease-failure":
				require.NoError(t, os.Mkdir(filepath.Join(taskDir, ".run.lock"), 0o700))
			case "invalid-state":
				writeFileT(t, statePath, "broken JSON")
			case "state-save-failure":
				require.NoError(t, os.Mkdir(filepath.Join(taskDir, ".state.lock"), 0o755))
			case "inbox-failure":
				require.NoError(t, os.Mkdir(filepath.Join(taskDir, "inbox.md"), 0o755))
			case "metrics-failure":
				require.NoError(t, os.Mkdir(filepath.Join(taskDir, "metrics.jsonl"), 0o755))
			}
			before, _ := os.ReadFile(statePath)
			stub := newStub(map[string]drivers.Result{"intake": drivers.MockResult(func(r *drivers.Result) {
				r.CleanupError = "guardian ownership unconfirmed"
			})})
			var logs []string
			m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: &drivers.Drivers{Intake: stub},
				Log: func(s string) { logs = append(logs, s) }, Sleep: func(int64) { t.Error("retry after cleanup failure") }})
			r := CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "advice"})
			assert.False(t, r.OK)
			require.ErrorContains(t, m.fatalErr, "guardian ownership unconfirmed")
			assert.Len(t, stub.calls, 1)
			switch condition {
			case "absent-state":
				assert.NoFileExists(t, statePath, "advice modules must not invent a main task state")
			case "active-orchestrator", "lease-without-pid":
				after, err := os.ReadFile(statePath)
				require.NoError(t, err)
				assert.Equal(t, before, after, "neither budget nor cleanup may change state owned by an active orchestrator")
				assert.Contains(t, strings.Join(logs, "\n"), "state.json budget write skipped")
			case "run-lease-failure":
				require.ErrorContains(t, m.fatalErr, ".run.lock")
				assert.Contains(t, strings.Join(logs, "\n"), "budget recording skipped:")
			case "invalid-state":
				require.ErrorContains(t, m.fatalErr, "state.json")
			case "state-save-failure":
				require.ErrorContains(t, m.fatalErr, ".state.lock")
			case "inbox-failure", "metrics-failure":
				if condition == "inbox-failure" {
					require.ErrorContains(t, m.fatalErr, "inbox.md")
				} else {
					require.ErrorContains(t, m.fatalErr, "metrics.jsonl")
				}
				st, err := state.Load(taskDir)
				require.NoError(t, err)
				assert.Equal(t, "escalated", st.Status)
			}
			if condition != "inbox-failure" {
				inbox, err := os.ReadFile(filepath.Join(taskDir, "inbox.md"))
				require.NoError(t, err)
				assert.Contains(t, string(inbox), "guardian ownership unconfirmed")
			}
			if condition != "metrics-failure" {
				assert.Len(t, readMetricsLinesT(t, m.MetricsFile), 1)
			}
		})
	}
}

func TestModuleStateUpdateHoldsRunLeaseThroughMutation(t *testing.T) {
	taskDir := t.TempDir()
	writeFileT(t, filepath.Join(taskDir, "state.json"), `{"revision":0,"status":"pending","packets":{},"inbox":[]}`)
	saved, err := updateIdleTaskState(taskDir, func(st *state.State) {
		// A starting orchestrator must remain excluded while this state revision
		// is being changed; releasing a lease immediately after probing is unsafe.
		_, err := machine.AcquireRunLease(taskDir)
		require.ErrorIs(t, err, syscall.EWOULDBLOCK)
		st.Status = "escalated"
	})
	require.NoError(t, err)
	assert.True(t, saved)
	st, err := state.Load(taskDir)
	require.NoError(t, err)
	assert.Equal(t, "escalated", st.Status)
	assert.Equal(t, int64(1), st.Revision)
	release, err := machine.AcquireRunLease(taskDir)
	require.NoError(t, err, "the lease must be released after the CAS save")
	release()
}

func TestIntakeStopsBeforeNextCandidateOnUnknownCleanup(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	cfg.Intake.Sources = []config.Source{{Name: "source", Type: "command", Cmd: "emit"}}
	stub := newStub(map[string]drivers.Result{"intake": drivers.MockResult(func(r *drivers.Result) {
		r.CleanupError = "guardian ownership unconfirmed"
	})})
	summary, err := RunIntake(CtxOpts{TaskDir: taskDir, Cwd: taskDir, Config: cfg,
		Drivers: &drivers.Drivers{Intake: stub}, Sleep: func(int64) { t.Error("retry after cleanup failure") }}, "",
		func(string, []string, string) (string, error) { return "First candidate\nSecond candidate", nil })
	require.ErrorContains(t, err, "cleanup is unconfirmed")
	assert.False(t, summary.OK)
	assert.Equal(t, 2, summary.New)
	assert.Equal(t, 1, summary.Triaged["failed"])
	assert.Equal(t, 1, summary.Triaged["human"])
	assert.Equal(t, []string{"intake"}, stub.calls)
	inbox, err := os.ReadFile(summary.InboxFile)
	require.NoError(t, err)
	assert.Contains(t, string(inbox), "First candidate")
	assert.Contains(t, string(inbox), "Second candidate")
	assert.Equal(t, 1, strings.Count(string(inbox), "- triage: pending"), "untouched candidates retain their pending status")
}

func TestGardenReturnsCleanupEscalationWithoutProposals(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	stub := newStub(map[string]drivers.Result{"garden": drivers.MockResult(func(r *drivers.Result) {
		r.CleanupError = "guardian ownership unconfirmed"
	})})
	summary, err := RunGarden(CtxOpts{TaskDir: taskDir, Cwd: taskDir, Config: cfg,
		Drivers: &drivers.Drivers{Garden: stub}, Sleep: func(int64) { t.Error("retry after cleanup failure") }})
	require.ErrorContains(t, err, "cleanup is unconfirmed")
	assert.False(t, summary.OK)
	assert.Nil(t, summary.File)
	assert.Zero(t, summary.Proposals)
	assert.Equal(t, []string{"garden"}, stub.calls)
	assert.FileExists(t, filepath.Join(taskDir, "inbox.md"))
	assert.NoFileExists(t, filepath.Join(taskDir, "garden-proposals.md"))
}
