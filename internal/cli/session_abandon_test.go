package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSessionTTY struct {
	reader *strings.Reader
	output bytes.Buffer
}

func (f *fakeSessionTTY) Read(p []byte) (int, error)  { return f.reader.Read(p) }
func (f *fakeSessionTTY) Write(p []byte) (int, error) { return f.output.Write(p) }
func (f *fakeSessionTTY) Close() error                { return nil }

func blockedSessionFixture(t *testing.T) (string, string, *state.State, string) {
	t.Helper()
	taskDir, cwd, st := planningCLIFixture(t, false)
	key := "design-plan/planning:engine-1-1"
	hash := strings.Repeat("a", 64)
	st.Planning.Rounds = 1
	st.Planning.Requests = []planning.Request{{
		ID: "engine-1-1", Scope: "design", ScopeID: "", InputRevision: 1, Rounds: 1,
		MaxRounds: 3, Status: "running", PlanningOnly: false, Stage: "planner",
		Role: "design-plan", Channel: "kimi-subscription",
	}}
	st.Status = "escalated"
	st.Sessions.Records[key] = &state.SessionRecord{
		Task: taskDir, Role: "design-plan", Scope: "planning:engine-1-1", Cwd: cwd,
		Binding: hash, Goal: hash, Contract: hash, Generation: 1,
		Action: "wait", Reason: "dispatch, outcome or cleanup remains unknown",
		Last: &state.SessionInvocation{
			ID: "call-abandon1", Status: "blocked", StartedAt: "2026-11-16T05:24:37Z",
			Attempt: 1, Persistent: true, CandidateBefore: hash, GuardBefore: hash,
			Accounted: true, Dispatch: "unknown", Outcome: "unknown",
			CleanupConfirmed: true, UsageStatus: "unknown",
		},
	}
	st, err := state.Save(taskDir, st, nil)
	require.NoError(t, err)
	return taskDir, cwd, st, key
}

func TestSessionAbandonEnvironmentProofMustBindTheExactCommand(t *testing.T) {
	request := &machine.SessionCommand{Action: "abandon", Key: "design-plan/planning:engine-1-1", Revision: 7, Reason: "retire this call", Invocation: "call-abandon1"}
	t.Setenv("DSH_SESSION_ID", "host-session")
	proof := machine.SessionUserAction{Kind: "dsh", SessionID: "host-session", CommandID: "command-1", TaskDir: "/repo",
		Key: request.Key, Invocation: request.Invocation, Revision: request.Revision, Action: "abandon", Reason: request.Reason}
	raw, err := json.Marshal(proof)
	require.NoError(t, err)
	t.Setenv(sessionUserActionEnv, string(raw))
	require.NoError(t, sessionActionProof(request, nil, "/repo", false))
	require.NotNil(t, request.UserAction)
	assert.Equal(t, "dsh", request.UserAction.Kind)

	request.Reason = "changed after proof"
	require.ErrorContains(t, sessionActionProof(request, nil, "/repo", false), "does not match")
	t.Setenv(sessionUserActionEnv, "{")
	require.Error(t, sessionActionProof(request, nil, "/repo", false))
	t.Setenv(sessionUserActionEnv, "placeholder")
	require.NoError(t, os.Unsetenv(sessionUserActionEnv))
	request.Reason = "retire this call"
	require.ErrorContains(t, sessionActionProof(request, nil, "/repo", false), "verified host command")
}

type failingSessionTTY struct{}

func (f *failingSessionTTY) Read([]byte) (int, error)  { return 0, io.EOF }
func (f *failingSessionTTY) Write([]byte) (int, error) { return 0, fmt.Errorf("terminal write failed") }
func (f *failingSessionTTY) Close() error              { return nil }

func TestSessionAbandonCLIRejectsMissingSessionAndLease(t *testing.T) {
	task := newCLISandbox(t)
	t.Chdir(task)
	writeFileCli(t, state.StateFile(task), cliStateFixture)
	st, err := state.Load(task)
	require.NoError(t, err)
	st.Sessions = nil
	_, err = state.Save(task, st, nil)
	require.NoError(t, err)
	code, out := runCLI(t, "session", task, "--key", "missing/scope", "--abandon", "--reason", "retire")
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "session key not found")

	taskDir, cwd, st2, key := blockedSessionFixture(t)
	t.Chdir(cwd)
	st2.Sessions.Records["missing/scope"] = nil
	delete(st2.Sessions.Records, "missing/scope")
	code, out = runCLI(t, "session", taskDir, "--key", "missing/scope", "--abandon", "--reason", "retire")
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "session key not found")

	original := openPlanningTTY
	t.Cleanup(func() { openPlanningTTY = original })
	openPlanningTTY = func() (io.ReadWriteCloser, error) {
		return &fakeSessionTTY{reader: strings.NewReader("abandon call-abandon1\n")}, nil
	}
	release, err := machine.AcquireRunLease(taskDir)
	require.NoError(t, err)
	code, out = runCLI(t, "session", taskDir, "--key", key, "--abandon", "--reason", "retire while busy")
	release()
	require.NotEqual(t, 0, code, out)
}

func TestRunSessionAbandonRejectsMissingHostProof(t *testing.T) {
	taskDir, cwd, st, key := blockedSessionFixture(t)
	t.Chdir(cwd)
	directory := filepath.Join(taskDir, ".control")
	require.NoError(t, os.MkdirAll(directory, 0o700))
	raw, err := json.Marshal(machine.SessionCommand{Action: "abandon", Key: key, Revision: st.Revision, Reason: "retire", Invocation: "call-abandon1"})
	require.NoError(t, err)
	file := filepath.Join(directory, "session-request-abandon")
	require.NoError(t, os.WriteFile(file, raw, 0o600))
	require.NoError(t, os.Unsetenv("FLYWHEEL_SESSION_USER_ACTION"))
	code, out := runCLI(t, "run", taskDir, "--only", "session", "--session-request", file, "--session-request-digest", fmt.Sprintf("%x", sha256.Sum256(raw)), "--expect-revision", fmt.Sprint(st.Revision))
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "verified host command")
}

func TestSessionAbandonTTYProofRejectsMissingOrFailedTerminal(t *testing.T) {
	request := &machine.SessionCommand{Action: "abandon", Key: "design-plan/planning:engine-1-1", Revision: 7, Reason: "retire", Invocation: "call-abandon1"}
	history := &machine.SessionHistory{Key: request.Key, Invocation: "call-abandon1"}
	original := openPlanningTTY
	t.Cleanup(func() { openPlanningTTY = original })
	openPlanningTTY = func() (io.ReadWriteCloser, error) { return nil, fmt.Errorf("no terminal") }
	_, err := sessionUserActionFromTTY(request, history, "/repo")
	require.ErrorContains(t, err, "verified host command")

	openPlanningTTY = func() (io.ReadWriteCloser, error) { return &failingSessionTTY{}, nil }
	_, err = sessionUserActionFromTTY(request, history, "/repo")
	require.ErrorContains(t, err, "cannot present")

	history.Invocation = ""
	openPlanningTTY = func() (io.ReadWriteCloser, error) { return &fakeSessionTTY{reader: strings.NewReader("")}, nil }
	_, err = sessionUserActionFromTTY(request, history, "/repo")
	require.ErrorContains(t, err, "no matching recorded invocation")

	t.Setenv("DSH_SESSION_ID", "host")
	t.Setenv(sessionUserActionEnv, "{")
	openPlanningTTY = func() (io.ReadWriteCloser, error) {
		return &fakeSessionTTY{reader: strings.NewReader("abandon call-abandon1\n")}, nil
	}
	history.Invocation = "call-abandon1"
	require.NoError(t, sessionActionProof(request, history, "/repo", true), "an invalid env proof may fall back to the controlling terminal")
}

func TestSessionAbandonControlRequiresHostProofBeforeSpawn(t *testing.T) {
	taskDir, cwd, st, key := blockedSessionFixture(t)
	raw, err := json.Marshal(machine.SessionCommand{Action: "abandon", Key: key, Revision: st.Revision, Reason: "retire", Invocation: "call-abandon1"})
	require.NoError(t, err)
	require.NoError(t, os.Unsetenv(sessionUserActionEnv))
	cmd := controlCmd()
	cmd.SetIn(bytes.NewReader(raw))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	_, err = startSessionControl(cmd, taskDir, cwd)
	require.ErrorContains(t, err, "verified host command")
}

func TestSessionAbandonCLIRequiresConfirmedInvocation(t *testing.T) {
	taskDir, cwd, _, key := blockedSessionFixture(t)
	t.Chdir(cwd)
	original := openPlanningTTY
	t.Cleanup(func() { openPlanningTTY = original })
	openPlanningTTY = func() (io.ReadWriteCloser, error) {
		return &fakeSessionTTY{reader: strings.NewReader("abandon call-abandon1\n")}, nil
	}
	code, out := runCLI(t, "session", taskDir, "--key", key, "--abandon", "--reason", "The watchdog killed a live call; retire it.")
	require.Equal(t, 0, code, out)
	loaded, err := state.Load(taskDir)
	require.NoError(t, err)
	assert.Equal(t, "abandoned", loaded.Sessions.Records[key].Last.Status)
	assert.Equal(t, "cancelled", loaded.Planning.Requests[0].Status)
	assert.Equal(t, "paused", loaded.Status)

	taskDir2, cwd2, st2, key2 := blockedSessionFixture(t)
	t.Chdir(cwd2)
	openPlanningTTY = func() (io.ReadWriteCloser, error) {
		return &fakeSessionTTY{reader: strings.NewReader("abandon wrong-call\n")}, nil
	}
	code, out = runCLI(t, "session", taskDir2, "--key", key2, "--abandon", "--reason", "retire")
	require.NotEqual(t, 0, code, out)
	loaded, err = state.Load(taskDir2)
	require.NoError(t, err)
	assert.Equal(t, "blocked", loaded.Sessions.Records[key2].Last.Status)
	assert.Equal(t, "escalated", st2.Status)

	code, out = runCLI(t, "session", taskDir, "--key", key, "--abandon")
	require.NotEqual(t, 0, code, out)
	code, out = runCLI(t, "session", taskDir, "--key", key, "--reason", "no abandon flag")
	require.NotEqual(t, 0, code, out)
}

func TestSessionAbandonControlEnvelopeCarriesSemanticReceipt(t *testing.T) {
	taskDir, cwd, st, key := blockedSessionFixture(t)
	request := machine.SessionCommand{Action: "abandon", Key: key, Revision: st.Revision, Reason: "retire the unknown call", Invocation: "call-abandon1"}
	raw, err := json.Marshal(request)
	require.NoError(t, err)
	proof := machine.SessionUserAction{Kind: "dsh", SessionID: "desktop-session", CommandID: "command-1", TaskDir: taskDir,
		Key: key, Invocation: request.Invocation, Revision: st.Revision, Action: "abandon", Reason: request.Reason}
	proofRaw, err := json.Marshal(proof)
	require.NoError(t, err)
	t.Setenv("DSH_SESSION_ID", "desktop-session")
	t.Setenv(sessionUserActionEnv, string(proofRaw))
	original := controlExecutable
	t.Cleanup(func() { controlExecutable = original })
	controlExecutable = func() (string, error) { return "/usr/bin/true", nil }
	cmd := controlCmd()
	cmd.SetIn(bytes.NewReader(raw))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	reply, err := startSessionControl(cmd, taskDir, cwd)
	require.NoError(t, err)
	require.NotNil(t, reply.SessionControl)
	assert.Equal(t, key, reply.SessionControl.Key)
	assert.Equal(t, "abandon", reply.SessionControl.Action)
	assert.False(t, reply.SessionControl.Applied, "a child that exits without retiring the record must not be reported as applied")
	assert.Contains(t, reply.SessionControl.Message, "dispatch, outcome or cleanup remains unknown")

	controlExecutable = func() (string, error) { return "/usr/bin/false", nil }
	cmd = controlCmd()
	cmd.SetIn(bytes.NewReader(raw))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	reply, err = startSessionControl(cmd, taskDir, cwd)
	require.Error(t, err)
	require.NotNil(t, reply.SessionControl)
	assert.False(t, reply.SessionControl.Applied)
	assert.Contains(t, reply.SessionControl.Message, "orchestrator exited before readiness")
}

func TestQuickSessionControlWaitsForTheChildToExit(t *testing.T) {
	taskDir := t.TempDir()
	script := filepath.Join(taskDir, "fake-orchestrator.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho $$ > \"$2/orchestrator.pid\"\nsleep 0.1\n"), 0o755))
	originalExec, originalSnapshot := controlExecutable, controlSnapshotFn
	originalStart, originalPoll := controlStartWait, controlPollInterval
	t.Cleanup(func() {
		controlExecutable, controlSnapshotFn = originalExec, originalSnapshot
		controlStartWait, controlPollInterval = originalStart, originalPoll
	})
	controlExecutable = func() (string, error) { return script, nil }
	controlStartWait, controlPollInterval = time.Second, 5*time.Millisecond
	controlSnapshotFn = func(dir string) (controlResponse, error) {
		reply := controlResponse{Protocol: 1, TaskDir: dir}
		raw, err := os.ReadFile(filepath.Join(dir, "orchestrator.pid"))
		if err == nil {
			if pid, conv := strconv.Atoi(strings.TrimSpace(string(raw))); conv == nil {
				reply.Running, reply.PID = true, pid
			}
		}
		return reply, nil
	}
	reply, err := startControlledRunWait(taskDir, taskDir, "", []string{"--only", "session"}, true)
	require.NoError(t, err)
	assert.True(t, reply.Running)
}

func TestQuickSessionControlTimesOutWhenTheChildDoesNotExit(t *testing.T) {
	taskDir := t.TempDir()
	script := filepath.Join(taskDir, "slow-orchestrator.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho $$ > \"$2/orchestrator.pid\"\nsleep 0.3\n"), 0o755))
	originalExec, originalSnapshot := controlExecutable, controlSnapshotFn
	originalStart, originalPoll := controlStartWait, controlPollInterval
	t.Cleanup(func() {
		controlExecutable, controlSnapshotFn = originalExec, originalSnapshot
		controlStartWait, controlPollInterval = originalStart, originalPoll
	})
	controlExecutable = func() (string, error) { return script, nil }
	controlStartWait, controlPollInterval = 20*time.Millisecond, 5*time.Millisecond
	controlSnapshotFn = func(dir string) (controlResponse, error) {
		reply := controlResponse{Protocol: 1, TaskDir: dir}
		raw, err := os.ReadFile(filepath.Join(dir, "orchestrator.pid"))
		if err == nil {
			if pid, conv := strconv.Atoi(strings.TrimSpace(string(raw))); conv == nil {
				reply.Running, reply.PID = true, pid
			}
		}
		return reply, nil
	}
	_, err := startControlledRunWait(taskDir, taskDir, "", []string{"--only", "session"}, true)
	require.ErrorContains(t, err, "did not finish the session command")
}

func receiptState(key, id, status string, handoff string) *state.State {
	st := &state.State{Sessions: &state.SessionLedger{Records: map[string]*state.SessionRecord{
		key: {Reason: "dispatch, outcome or cleanup remains unknown", Last: &state.SessionInvocation{ID: id, Status: status, CleanupConfirmed: true, Accounted: true}},
	}}}
	if handoff != "" {
		st.Handoff = &state.Handoff{Key: handoff}
	}
	return st
}

func TestSessionControlReceiptMakesSemanticOutcomeExplicit(t *testing.T) {
	key := "design-plan/planning:engine-1-1"
	request := &machine.SessionCommand{Action: "reconcile", Key: key, Revision: 7}
	blocked := receiptState(key, "call-one", "blocked", "")
	receipt := sessionControlReceipt(blocked, blocked, request)
	require.NotNil(t, receipt)
	assert.False(t, receipt.Applied)
	assert.Equal(t, "blocked", receipt.Status)
	assert.Contains(t, receipt.Message, "dispatch, outcome or cleanup remains unknown")

	consumed := receiptState(key, "call-one", "consumed", "")
	assert.True(t, sessionControlReceipt(consumed, blocked, request).Applied)

	request.Action = "abandon"
	request.Reason = "retire"
	abandoned := receiptState(key, "call-one", "abandoned", "")
	abandoned.Sessions.Records[key].Last.Abandon = &state.SessionAbandon{At: "now", Reason: request.Reason, Authority: "human"}
	receipt = sessionControlReceipt(abandoned, blocked, request)
	assert.True(t, receipt.Applied)
	assert.Equal(t, "abandoned", receipt.Status)
	assert.Equal(t, "call-one", receipt.Invocation)
	noReason := receiptState(key, "call-one", "blocked", "")
	noReason.Sessions.Records[key].Reason = ""
	assert.Equal(t, "the unresolved invocation was not retired", sessionControlReceipt(noReason, blocked, request).Message)

	assert.Equal(t, "task state is unavailable", sessionControlReceipt(nil, nil, request).Message)
	assert.Equal(t, "session key not found", sessionControlReceipt(&state.State{}, nil, request).Message)

	request.Action = "claim"
	assert.False(t, sessionControlReceipt(receiptState(key, "call-one", "consumed", key), receiptState(key, "call-one", "consumed", key), request).Applied)
	assert.True(t, sessionControlReceipt(receiptState(key, "call-one", "consumed", key), receiptState(key, "call-one", "consumed", ""), request).Applied)
	request.Action = "release"
	assert.False(t, sessionControlReceipt(receiptState(key, "call-one", "consumed", ""), nil, request).Applied)
	assert.True(t, sessionControlReceipt(receiptState(key, "call-one", "consumed", ""), receiptState(key, "call-one", "consumed", key), request).Applied)
	request.Action = "send"
	assert.False(t, sessionControlReceipt(receiptState(key, "call-one", "consumed", ""), receiptState(key, "call-one", "consumed", ""), request).Applied)
	assert.True(t, sessionControlReceipt(receiptState(key, "call-two", "consumed", ""), receiptState(key, "call-one", "consumed", ""), request).Applied)
	request.Action = "return"
	assert.True(t, sessionControlReceipt(receiptState(key, "call-one", "consumed", ""), blocked, request).Applied)
	request.Action = "unknown"
	assert.False(t, sessionControlReceipt(blocked, blocked, request).Applied)
}
