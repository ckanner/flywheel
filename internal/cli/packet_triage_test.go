package cli

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func inboxPacketCLIFixture(t *testing.T) (string, string, *state.State) {
	t.Helper()
	task := newCLISandbox(t)
	root := filepath.Dir(filepath.Dir(task))
	writeFileCli(t, state.StateFile(task), cliStateFixture)
	st, err := state.Load(task)
	require.NoError(t, err)
	st.Status = "escalated"
	st.Packets["M01-T01"].Status = "inbox"
	st.Packets["M01-T01"].FailedAttempts = 2
	st, err = state.Save(task, st, nil)
	require.NoError(t, err)
	return task, root, st
}

func TestPacketCommandPrivatePayloadAndEnvProof(t *testing.T) {
	raw := `{"action":"retry","packet-id":"M01-T01","revision":2,"reason":"retry it","guidance":"keep R-01 tight"}`
	req, err := readPacketCommand(strings.NewReader(raw))
	require.NoError(t, err)
	assert.Equal(t, "retry", req.Action)
	for _, r := range []io.Reader{failingEditorReader{}, strings.NewReader("{} {}"), strings.NewReader(strings.Repeat(" ", 131073)), strings.NewReader(`{"bad":true}`), strings.NewReader(`{}`)} {
		_, err := readPacketCommand(r)
		require.Error(t, err)
	}

	// Env proof validation.
	t.Setenv("DSH_SESSION_ID", "host-1")
	proof := machine.PacketUserAction{
		Kind: "dsh", SessionID: "host-1", CommandID: "cmd-1", TaskDir: "/task",
		PacketID: req.PacketID, Revision: req.Revision, Action: req.Action, Reason: req.Reason, Guidance: req.Guidance,
	}
	proofRaw, _ := json.Marshal(proof)
	t.Setenv(packetUserActionEnv, string(proofRaw))
	require.NoError(t, packetActionProof(req, "/task", false))
	require.NotNil(t, req.UserAction)

	t.Setenv("DSH_SESSION_ID", "other")
	require.ErrorContains(t, packetActionProof(req, "/task", false), "does not match")
	t.Setenv("DSH_SESSION_ID", "host-1")
	t.Setenv(packetUserActionEnv, strings.Repeat("a", 8200))
	require.ErrorContains(t, packetActionProof(req, "/task", false), "malformed or oversized")
	require.NoError(t, os.Unsetenv(packetUserActionEnv))
	require.ErrorContains(t, packetActionProof(req, "/task", false), "verified host command")

	// TTY proof paths.
	origTTY := openPlanningTTY
	t.Cleanup(func() { openPlanningTTY = origTTY })
	openPlanningTTY = func() (io.ReadWriteCloser, error) { return nil, errors.New("no tty") }
	require.ErrorContains(t, packetActionProof(req, "/task", true), "controlling terminal")

	openPlanningTTY = func() (io.ReadWriteCloser, error) { return &failingSessionTTY{}, nil }
	require.ErrorContains(t, packetActionProof(req, "/task", true), "cannot present")

	openPlanningTTY = func() (io.ReadWriteCloser, error) { return &fakeSessionTTY{reader: strings.NewReader("nope\n")}, nil }
	require.ErrorContains(t, packetActionProof(req, "/task", true), "did not confirm")

	openPlanningTTY = func() (io.ReadWriteCloser, error) {
		return &fakeSessionTTY{reader: strings.NewReader("retry M01-T01\n")}, nil
	}
	require.NoError(t, packetActionProof(req, "/task", true))
	assert.Equal(t, "tty", req.UserAction.Kind)

	// Private input loading errors & happy path.
	task := t.TempDir()
	dir := filepath.Join(task, ".control")
	require.NoError(t, os.Mkdir(dir, 0o700))
	file := filepath.Join(dir, "packet-request-1")
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))

	_, err = loadPacketInput("/no/such/path", file, digest)
	require.Error(t, err)
	_, err = loadPacketInput(task, "/no/such/dir/packet-request-1", digest)
	require.Error(t, err)
	_, err = loadPacketInput(task, filepath.Join(dir, "wrong-name"), digest)
	require.Error(t, err)
	_, err = loadPacketInput(task, file, digest)
	require.Error(t, err)
	require.NoError(t, os.WriteFile(file, []byte(raw), 0o644))
	_, err = loadPacketInput(task, file, digest)
	require.Error(t, err)
	require.NoError(t, os.Chmod(file, 0o600))

	// Open / read injection errors.
	origOpen, origRead := sessionInputOpen, sessionInputRead
	t.Cleanup(func() { sessionInputOpen, sessionInputRead = origOpen, origRead })
	sessionInputOpen = func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("open fail") }
	_, err = loadPacketInput(task, file, digest)
	require.Error(t, err)
	sessionInputOpen = origOpen

	otherFile := filepath.Join(dir, "other-file")
	require.NoError(t, os.WriteFile(otherFile, []byte(raw), 0o600))
	sessionInputOpen = func(string, int, os.FileMode) (*os.File, error) { return os.Open(otherFile) }
	_, err = loadPacketInput(task, file, digest)
	require.ErrorContains(t, err, "identity changed")
	// Closed file fails Stat()
	sessionInputOpen = func(string, int, os.FileMode) (*os.File, error) {
		f, _ := os.Open(otherFile)
		_ = f.Close()
		return f, nil
	}
	_, err = loadPacketInput(task, file, digest)
	require.Error(t, err)
	sessionInputOpen = origOpen

	sessionInputRead = func(io.Reader) ([]byte, error) { return nil, errors.New("read fail") }
	_, err = loadPacketInput(task, file, digest)
	require.Error(t, err)
	sessionInputRead = origRead

	_, err = loadPacketInput(task, file, "bad-digest")
	require.ErrorContains(t, err, "content changed")

	loaded, err := loadPacketInput(task, file, digest)
	require.NoError(t, err)
	assert.Equal(t, "M01-T01", loaded.PacketID)
	_, statErr := os.Stat(file)
	require.True(t, os.IsNotExist(statErr))
}

func TestPacketControlReceiptTransitions(t *testing.T) {
	reqRetry := &machine.PacketCommand{Action: "retry", PacketID: "M01-T01", Revision: 2, Reason: "r"}
	reqReplan := &machine.PacketCommand{Action: "replan", PacketID: "M01-T01", Revision: 2, Reason: "r"}
	reqAccept := &machine.PacketCommand{Action: "accept", PacketID: "M01-T01", Revision: 2, Reason: "ok"}

	assert.False(t, packetControlReceipt(nil, nil, reqRetry).Applied)
	assert.False(t, packetControlReceipt(&state.State{Packets: map[string]*state.PacketState{}}, nil, reqRetry).Applied)
	assert.False(t, packetControlReceipt(&state.State{Revision: 3, Packets: map[string]*state.PacketState{"M01-T01": {Status: "pending"}}}, nil, reqRetry).Applied)

	before := &state.State{Revision: 2, Packets: map[string]*state.PacketState{"M01-T01": {Status: "inbox", PlanRevisions: 0}}}
	afterRetryOK := &state.State{Revision: 3, Packets: map[string]*state.PacketState{"M01-T01": {Status: "pending"}}}
	assert.True(t, packetControlReceipt(afterRetryOK, before, reqRetry).Applied)
	assert.False(t, packetControlReceipt(before, before, reqRetry).Applied)

	afterReplanOK := &state.State{Revision: 3, Packets: map[string]*state.PacketState{"M01-T01": {Status: "pending", PlanRevisions: 1}}}
	assert.True(t, packetControlReceipt(afterReplanOK, before, reqReplan).Applied)
	assert.False(t, packetControlReceipt(afterRetryOK, before, reqReplan).Applied)

	afterAcceptOK := &state.State{Revision: 3, Packets: map[string]*state.PacketState{"M01-T01": {Status: "completed", HumanSignoff: &state.PacketSignoff{Reason: "ok"}}}}
	assert.True(t, packetControlReceipt(afterAcceptOK, before, reqAccept).Applied)
	assert.False(t, packetControlReceipt(afterRetryOK, before, reqAccept).Applied)
}

func TestPacketControlAndCLIAndRunIntegration(t *testing.T) {
	task, root, st := inboxPacketCLIFixture(t)
	chdirT(t, root)
	t.Setenv("FLYWHEEL_RUNS_DIR", t.TempDir())

	// `flywheel packet` CLI validation errors.
	code, out := runCLI(t, "packet", "/no/such/task", "--id", "M01-T01", "--retry", "--reason", "r")
	require.NotEqual(t, 0, code, out)
	code, out = runCLI(t, "packet", task, "--id", "M01-T01", "--reason", "r")
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "exactly one of --retry, --replan, or --accept")
	code, out = runCLI(t, "packet", task, "--retry", "--reason", "r")
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "--id is required")
	code, out = runCLI(t, "packet", task, "--id", "M01-T01", "--retry")
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "--reason is required")

	// Config / runs-dir / state / validate / proof errors on `flywheel packet`.
	t.Setenv("FLYWHEEL_RUNS_DIR", "relative")
	code, _ = runCLI(t, "packet", task, "--id", "M01-T01", "--retry", "--reason", "r")
	require.NotEqual(t, 0, code)
	t.Setenv("FLYWHEEL_RUNS_DIR", t.TempDir())

	code, _ = runCLI(t, "packet", task, "--id", "M01-T01", "--retry", "--reason", strings.Repeat("x", 1001))
	require.NotEqual(t, 0, code)

	// Invalid config.json error.
	cfgPath := filepath.Join(task, "flywheel.config.json")
	require.NoError(t, os.WriteFile(cfgPath, []byte("{bad"), 0o644))
	code, _ = runCLI(t, "packet", task, "--id", "M01-T01", "--retry", "--reason", "r")
	require.NotEqual(t, 0, code)
	require.NoError(t, os.Remove(cfgPath))

	// Missing state.json error.
	emptyTask := t.TempDir()
	code, _ = runCLI(t, "packet", emptyTask, "--id", "M01-T01", "--retry", "--reason", "r")
	require.NotEqual(t, 0, code)

	// TTY proof rejected.
	origTTY := openPlanningTTY
	t.Cleanup(func() { openPlanningTTY = origTTY })
	openPlanningTTY = func() (io.ReadWriteCloser, error) { return nil, errors.New("no tty") }
	code, _ = runCLI(t, "packet", task, "--id", "M01-T01", "--retry", "--reason", "r")
	require.NotEqual(t, 0, code)

	// RunSingleStep failure (accept without L1 green).
	openPlanningTTY = func() (io.ReadWriteCloser, error) {
		return &fakeSessionTTY{reader: strings.NewReader("accept M01-T01\n")}, nil
	}
	code, out = runCLI(t, "packet", task, "--id", "M01-T01", "--accept", "--reason", "try accept")
	require.NotEqual(t, 0, code, out)

	// Replan flag path (will fail proof or planner cleanly; test flag parsing by rejecting proof).
	openPlanningTTY = func() (io.ReadWriteCloser, error) { return nil, errors.New("no tty") }
	code, _ = runCLI(t, "packet", task, "--id", "M01-T01", "--replan", "--reason", "try replan")
	require.NotEqual(t, 0, code)

	// Happy path `flywheel packet --retry`.
	openPlanningTTY = func() (io.ReadWriteCloser, error) {
		return &fakeSessionTTY{reader: strings.NewReader("retry M01-T01\n")}, nil
	}
	code, out = runCLI(t, "packet", task, "--id", "M01-T01", "--retry", "--reason", "retry now", "--guidance", "skip fluff")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "triage (retry) completed")

	// `flywheel run --only packet` validation errors.
	code, out = runCLI(t, "run", task, "--only", "verify", "--packet-request", "/tmp/x")
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "--packet-request requires --only packet")

	code, out = runCLI(t, "run", task, "--only", "packet", "--packet-request", "/no/file", "--packet-request-digest", "abc", "--expect-revision", "1")
	require.NotEqual(t, 0, code, out)

	dir := filepath.Join(task, ".control")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	stNow, err := state.Load(task)
	require.NoError(t, err)
	rawReq, _ := json.Marshal(machine.PacketCommand{Action: "retry", PacketID: "M01-T01", Revision: stNow.Revision, Reason: "r"})
	reqFile := filepath.Join(dir, "packet-request-rev")
	require.NoError(t, os.WriteFile(reqFile, rawReq, 0o600))
	digest := fmt.Sprintf("%x", sha256.Sum256(rawReq))

	// Revision mismatch.
	code, out = runCLI(t, "run", task, "--only", "packet", "--packet-request", reqFile, "--packet-request-digest", digest, "--expect-revision", fmt.Sprint(stNow.Revision+9))
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "revision does not match")

	// Missing host proof.
	require.NoError(t, os.WriteFile(reqFile, rawReq, 0o600))
	require.NoError(t, os.Unsetenv(packetUserActionEnv))
	code, out = runCLI(t, "run", task, "--only", "packet", "--packet-request", reqFile, "--packet-request-digest", digest, "--expect-revision", fmt.Sprint(stNow.Revision))
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "verified host command")

	// Valid host proof reaches RunSingleStep (fails because M01-T01 is now pending, not inbox).
	require.NoError(t, os.WriteFile(reqFile, rawReq, 0o600))
	t.Setenv("DSH_SESSION_ID", "sess-1")
	proofJSON, _ := json.Marshal(machine.PacketUserAction{
		Kind: "dsh", SessionID: "sess-1", CommandID: "cmd-1", TaskDir: task,
		PacketID: "M01-T01", Revision: stNow.Revision, Action: "retry", Reason: "r",
	})
	t.Setenv(packetUserActionEnv, string(proofJSON))
	code, out = runCLI(t, "run", task, "--only", "packet", "--packet-request", reqFile, "--packet-request-digest", digest, "--expect-revision", fmt.Sprint(stNow.Revision))
	require.NotEqual(t, 0, code, out)
	_ = st
}

func TestStartPacketControlPaths(t *testing.T) {
	controlFixture(t, "completed")
	task, root := t.TempDir(), t.TempDir()
	realTask, err := filepath.EvalSymlinks(task)
	require.NoError(t, err)
	t.Setenv("DSH_SESSION_ID", "host-1")

	setProof := func(action string) {
		p, _ := json.Marshal(machine.PacketUserAction{
			Kind: "dsh", SessionID: "host-1", CommandID: "cmd-1", TaskDir: realTask,
			PacketID: "M01-T01", Revision: 1, Action: action, Reason: "manual",
		})
		t.Setenv(packetUserActionEnv, string(p))
	}

	// Control packet happy path via controlCmd (retry).
	setProof("retry")
	cmd := controlCmd()
	cmd.SetArgs([]string{"packet", "--task-dir", task, "--cwd", root})
	cmd.SetOut(io.Discard)
	cmd.SetIn(strings.NewReader(`{"action":"retry","packet-id":"M01-T01","revision":1,"reason":"manual"}`))
	require.NoError(t, cmd.Execute())

	// Async replan path with reply.Running == true.
	setProof("replan")
	origSnap := controlSnapshotFn
	t.Cleanup(func() { controlSnapshotFn = origSnap })
	calls := 0
	controlSnapshotFn = func(dir string) (controlResponse, error) {
		calls++
		if calls <= 2 {
			return controlResponse{Protocol: 1, TaskDir: dir, Running: false, State: &state.State{Revision: 1}}, nil
		}
		return controlResponse{Protocol: 1, TaskDir: dir, Running: true, State: &state.State{Revision: 2}}, nil
	}
	cmd2 := controlCmd()
	cmd2.SetIn(strings.NewReader(`{"action":"replan","packet-id":"M01-T01","revision":1,"reason":"manual"}`))
	reply, err := startPacketControl(cmd2, realTask, root)
	require.NoError(t, err)
	require.NotNil(t, reply.PacketControl)
	assert.True(t, reply.PacketControl.Applied)
	assert.Equal(t, "running", reply.PacketControl.Status)

	// Error propagation sets PacketControl.Applied = false and pulls State.Control.Message.
	setProof("retry")
	controlSnapshotFn = func(dir string) (controlResponse, error) {
		return controlResponse{
			Protocol: 1, TaskDir: dir, Running: true,
			State: &state.State{
				Revision: 1,
				Control:  &state.ControlResult{Message: "busy control"},
				Packets:  map[string]*state.PacketState{"M01-T01": {Status: "inbox"}},
			},
		}, nil
	}
	cmd3 := controlCmd()
	cmd3.SetIn(strings.NewReader(`{"action":"retry","packet-id":"M01-T01","revision":1,"reason":"manual"}`))
	reply3, err := startPacketControl(cmd3, realTask, root)
	require.Error(t, err)
	require.NotNil(t, reply3.PacketControl)
	assert.False(t, reply3.PacketControl.Applied)
	assert.Equal(t, "busy control", reply3.PacketControl.Message)

	// Input/proof/filesystem failures.
	controlSnapshotFn = origSnap
	for _, scenario := range []string{"input", "proof", "directory", "stat", "symlink", "temp", "write", "sync"} {
		t.Run(scenario, func(t *testing.T) {
			d, r := t.TempDir(), t.TempDir()
			rd, _ := filepath.EvalSymlinks(d)
			p, _ := json.Marshal(machine.PacketUserAction{
				Kind: "dsh", SessionID: "host-1", CommandID: "cmd-1", TaskDir: rd,
				PacketID: "M01-T01", Revision: 1, Action: "retry", Reason: "manual",
			})
			t.Setenv(packetUserActionEnv, string(p))
			c := controlCmd()
			c.SetIn(strings.NewReader(`{"action":"retry","packet-id":"M01-T01","revision":1,"reason":"manual"}`))
			oldStat, oldTemp, oldWrite, oldSync := sessionDirectoryStat, sessionInputTemp, sessionInputWrite, sessionInputSync
			t.Cleanup(func() {
				sessionDirectoryStat, sessionInputTemp, sessionInputWrite, sessionInputSync = oldStat, oldTemp, oldWrite, oldSync
			})
			switch scenario {
			case "input":
				c.SetIn(failingEditorReader{})
			case "proof":
				_ = os.Unsetenv(packetUserActionEnv)
			case "directory":
				require.NoError(t, os.WriteFile(filepath.Join(rd, ".control"), nil, 0o600))
			case "stat":
				sessionDirectoryStat = func(string) (os.FileInfo, error) { return nil, errors.New("stat fail") }
			case "symlink":
				require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(rd, ".control")))
			case "temp":
				sessionInputTemp = func(string, string) (*os.File, error) { return nil, errors.New("temp fail") }
			case "write":
				sessionInputWrite = func(*os.File, []byte) (int, error) { return 0, errors.New("write fail") }
			case "sync":
				sessionInputSync = func(*os.File) error { return errors.New("sync fail") }
			}
			_, err := startPacketControl(c, rd, r)
			require.Error(t, err)
		})
	}
}
