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

func escalatedMilestoneCLIFixture(t *testing.T) (string, string, *state.State) {
	t.Helper()
	task := newCLISandbox(t)
	root := filepath.Dir(filepath.Dir(task))
	writeFileCli(t, state.StateFile(task), cliStateFixture)
	st, err := state.Load(task)
	require.NoError(t, err)
	st.Status = "escalated"
	st.Packets["M01-T01"].Status = "completed"
	ms := st.Milestones["M-01"]
	ms.Status = "in-progress"
	ms.Gate = map[string]any{"pass": false}
	st.Milestones["M-01"] = ms
	st, err = state.Save(task, st, nil)
	require.NoError(t, err)
	return task, root, st
}

func TestMilestoneCommandPrivatePayloadAndEnvProof(t *testing.T) {
	raw := `{"action":"retry","milestone-id":"M-01","revision":2,"reason":"rerun L2","guidance":"ignore synthetic edge cases"}`
	req, err := readMilestoneCommand(strings.NewReader(raw))
	require.NoError(t, err)
	assert.Equal(t, "retry", req.Action)
	for _, r := range []io.Reader{failingEditorReader{}, strings.NewReader("{} {}"), strings.NewReader(strings.Repeat(" ", 131073)), strings.NewReader(`{"bad":true}`), strings.NewReader(`{}`)} {
		_, err := readMilestoneCommand(r)
		require.Error(t, err)
	}

	// Env proof validation.
	t.Setenv("DSH_SESSION_ID", "host-1")
	proof := machine.MilestoneUserAction{
		Kind: "dsh", SessionID: "host-1", CommandID: "cmd-1", TaskDir: "/task",
		MilestoneID: req.MilestoneID, Revision: req.Revision, Action: req.Action, Reason: req.Reason, Guidance: req.Guidance,
	}
	proofRaw, _ := json.Marshal(proof)
	t.Setenv(milestoneUserActionEnv, string(proofRaw))
	require.NoError(t, milestoneActionProof(req, "/task", false))
	require.NotNil(t, req.UserAction)

	t.Setenv("DSH_SESSION_ID", "other")
	require.ErrorContains(t, milestoneActionProof(req, "/task", false), "does not match")
	t.Setenv("DSH_SESSION_ID", "host-1")
	t.Setenv(milestoneUserActionEnv, strings.Repeat("a", 8200))
	require.ErrorContains(t, milestoneActionProof(req, "/task", false), "malformed or oversized")
	require.NoError(t, os.Unsetenv(milestoneUserActionEnv))
	require.ErrorContains(t, milestoneActionProof(req, "/task", false), "verified host command")

	// TTY proof paths.
	origTTY := openPlanningTTY
	t.Cleanup(func() { openPlanningTTY = origTTY })
	openPlanningTTY = func() (io.ReadWriteCloser, error) { return nil, errors.New("no tty") }
	require.ErrorContains(t, milestoneActionProof(req, "/task", true), "controlling terminal")

	openPlanningTTY = func() (io.ReadWriteCloser, error) { return &failingSessionTTY{}, nil }
	require.ErrorContains(t, milestoneActionProof(req, "/task", true), "cannot present")

	openPlanningTTY = func() (io.ReadWriteCloser, error) { return &fakeSessionTTY{reader: strings.NewReader("nope\n")}, nil }
	require.ErrorContains(t, milestoneActionProof(req, "/task", true), "did not confirm")

	openPlanningTTY = func() (io.ReadWriteCloser, error) {
		return &fakeSessionTTY{reader: strings.NewReader("retry M-01\n")}, nil
	}
	require.NoError(t, milestoneActionProof(req, "/task", true))
	assert.Equal(t, "tty", req.UserAction.Kind)

	// Private input loading errors & happy path.
	task := t.TempDir()
	dir := filepath.Join(task, ".control")
	require.NoError(t, os.Mkdir(dir, 0o700))
	file := filepath.Join(dir, "milestone-request-1")
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))

	_, err = loadMilestoneInput("/no/such/path", file, digest)
	require.Error(t, err)
	_, err = loadMilestoneInput(task, "/no/such/dir/milestone-request-1", digest)
	require.Error(t, err)
	_, err = loadMilestoneInput(task, filepath.Join(dir, "wrong-name"), digest)
	require.Error(t, err)
	_, err = loadMilestoneInput(task, file, digest)
	require.Error(t, err)
	require.NoError(t, os.WriteFile(file, []byte(raw), 0o644))
	_, err = loadMilestoneInput(task, file, digest)
	require.Error(t, err)
	require.NoError(t, os.Chmod(file, 0o600))

	origOpen, origRead := sessionInputOpen, sessionInputRead
	t.Cleanup(func() { sessionInputOpen, sessionInputRead = origOpen, origRead })
	sessionInputOpen = func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("open fail") }
	_, err = loadMilestoneInput(task, file, digest)
	require.Error(t, err)
	sessionInputOpen = origOpen

	otherFile := filepath.Join(dir, "other-ms-file")
	require.NoError(t, os.WriteFile(otherFile, []byte(raw), 0o600))
	sessionInputOpen = func(string, int, os.FileMode) (*os.File, error) { return os.Open(otherFile) }
	_, err = loadMilestoneInput(task, file, digest)
	require.ErrorContains(t, err, "identity changed")
	sessionInputOpen = func(string, int, os.FileMode) (*os.File, error) {
		f, _ := os.Open(otherFile)
		_ = f.Close()
		return f, nil
	}
	_, err = loadMilestoneInput(task, file, digest)
	require.Error(t, err)
	sessionInputOpen = origOpen

	sessionInputRead = func(io.Reader) ([]byte, error) { return nil, errors.New("read fail") }
	_, err = loadMilestoneInput(task, file, digest)
	require.Error(t, err)
	sessionInputRead = origRead

	_, err = loadMilestoneInput(task, file, "bad-digest")
	require.ErrorContains(t, err, "content changed")

	loaded, err := loadMilestoneInput(task, file, digest)
	require.NoError(t, err)
	assert.Equal(t, "M-01", loaded.MilestoneID)
	_, statErr := os.Stat(file)
	require.True(t, os.IsNotExist(statErr))
}

func TestMilestoneControlReceiptTransitions(t *testing.T) {
	reqRetry := &machine.MilestoneCommand{Action: "retry", MilestoneID: "M-01", Revision: 2, Reason: "r"}
	reqAccept := &machine.MilestoneCommand{Action: "accept", MilestoneID: "M-01", Revision: 2, Reason: "ok"}

	assert.False(t, milestoneControlReceipt(nil, nil, reqRetry).Applied)
	assert.False(t, milestoneControlReceipt(&state.State{Milestones: map[string]state.MilestoneState{}}, nil, reqRetry).Applied)
	assert.False(t, milestoneControlReceipt(&state.State{Revision: 3, Status: "paused", Milestones: map[string]state.MilestoneState{"M-01": {Status: "in-progress"}}}, nil, reqRetry).Applied)
	assert.False(t, milestoneControlReceipt(&state.State{Revision: 3, Status: "paused", Milestones: map[string]state.MilestoneState{"M-01": {Status: "in-progress"}}}, &state.State{Revision: 2, Milestones: map[string]state.MilestoneState{}}, reqRetry).Applied)

	before := &state.State{Revision: 2, Status: "escalated", Milestones: map[string]state.MilestoneState{"M-01": {Status: "in-progress"}}}
	afterRetryOK := &state.State{Revision: 3, Status: "paused", Milestones: map[string]state.MilestoneState{"M-01": {Status: "in-progress"}}}
	assert.True(t, milestoneControlReceipt(afterRetryOK, before, reqRetry).Applied)
	assert.False(t, milestoneControlReceipt(before, before, reqRetry).Applied)

	afterAcceptOK := &state.State{Revision: 3, Status: "completed", Milestones: map[string]state.MilestoneState{"M-01": {Status: "closed", HumanSignoff: &state.PacketSignoff{Reason: "ok"}}}}
	assert.True(t, milestoneControlReceipt(afterAcceptOK, before, reqAccept).Applied)
	assert.False(t, milestoneControlReceipt(afterRetryOK, before, reqAccept).Applied)
}

func TestMilestoneControlAndCLIAndRunIntegration(t *testing.T) {
	task, root, st := escalatedMilestoneCLIFixture(t)
	chdirT(t, root)
	t.Setenv("FLYWHEEL_RUNS_DIR", t.TempDir())

	// `flywheel milestone` CLI validation errors.
	code, out := runCLI(t, "milestone", "/no/such/task", "--id", "M-01", "--retry", "--reason", "r")
	require.NotEqual(t, 0, code, out)
	code, out = runCLI(t, "milestone", task, "--id", "M-01", "--reason", "r")
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "exactly one of --retry or --accept")
	code, out = runCLI(t, "milestone", task, "--retry", "--reason", "r")
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "--id is required")
	code, out = runCLI(t, "milestone", task, "--id", "M-01", "--retry")
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "--reason is required")

	t.Setenv("FLYWHEEL_RUNS_DIR", "relative")
	code, _ = runCLI(t, "milestone", task, "--id", "M-01", "--retry", "--reason", "r")
	require.NotEqual(t, 0, code)
	t.Setenv("FLYWHEEL_RUNS_DIR", t.TempDir())

	code, _ = runCLI(t, "milestone", task, "--id", "M-01", "--retry", "--reason", strings.Repeat("x", 1001))
	require.NotEqual(t, 0, code)

	cfgPath := filepath.Join(task, "flywheel.config.json")
	require.NoError(t, os.WriteFile(cfgPath, []byte("{bad"), 0o644))
	code, _ = runCLI(t, "milestone", task, "--id", "M-01", "--retry", "--reason", "r")
	require.NotEqual(t, 0, code)
	require.NoError(t, os.Remove(cfgPath))

	emptyTask := t.TempDir()
	code, _ = runCLI(t, "milestone", emptyTask, "--id", "M-01", "--retry", "--reason", "r")
	require.NotEqual(t, 0, code)

	origTTY := openPlanningTTY
	t.Cleanup(func() { openPlanningTTY = origTTY })
	openPlanningTTY = func() (io.ReadWriteCloser, error) { return nil, errors.New("no tty") }
	code, _ = runCLI(t, "milestone", task, "--id", "M-01", "--retry", "--reason", "r")
	require.NotEqual(t, 0, code)

	// Accept without passing gate fails cleanly.
	openPlanningTTY = func() (io.ReadWriteCloser, error) {
		return &fakeSessionTTY{reader: strings.NewReader("accept M-01\n")}, nil
	}
	code, out = runCLI(t, "milestone", task, "--id", "M-01", "--accept", "--reason", "try accept")
	require.NotEqual(t, 0, code, out)

	// Happy path `flywheel milestone --retry`.
	openPlanningTTY = func() (io.ReadWriteCloser, error) {
		return &fakeSessionTTY{reader: strings.NewReader("retry M-01\n")}, nil
	}
	code, out = runCLI(t, "milestone", task, "--id", "M-01", "--retry", "--reason", "rerun gate", "--guidance", "scope ok")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "Milestone M-01 triage (retry) completed")

	// `flywheel run --only milestone` validation errors.
	code, out = runCLI(t, "run", task, "--only", "verify", "--milestone-request", "/tmp/x")
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "--milestone-request requires --only milestone")

	code, out = runCLI(t, "run", task, "--only", "milestone", "--milestone-request", "/no/file", "--milestone-request-digest", "abc", "--expect-revision", "1")
	require.NotEqual(t, 0, code, out)

	dir := filepath.Join(task, ".control")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	stNow, err := state.Load(task)
	require.NoError(t, err)
	rawReq, _ := json.Marshal(machine.MilestoneCommand{Action: "retry", MilestoneID: "M-01", Revision: stNow.Revision, Reason: "r"})
	reqFile := filepath.Join(dir, "milestone-request-rev")
	require.NoError(t, os.WriteFile(reqFile, rawReq, 0o600))
	digest := fmt.Sprintf("%x", sha256.Sum256(rawReq))

	code, out = runCLI(t, "run", task, "--only", "milestone", "--milestone-request", reqFile, "--milestone-request-digest", digest, "--expect-revision", fmt.Sprint(stNow.Revision+9))
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "revision does not match")

	require.NoError(t, os.WriteFile(reqFile, rawReq, 0o600))
	require.NoError(t, os.Unsetenv(milestoneUserActionEnv))
	code, out = runCLI(t, "run", task, "--only", "milestone", "--milestone-request", reqFile, "--milestone-request-digest", digest, "--expect-revision", fmt.Sprint(stNow.Revision))
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, "verified host command")

	require.NoError(t, os.WriteFile(reqFile, rawReq, 0o600))
	t.Setenv("DSH_SESSION_ID", "sess-1")
	proofJSON, _ := json.Marshal(machine.MilestoneUserAction{
		Kind: "dsh", SessionID: "sess-1", CommandID: "cmd-1", TaskDir: task,
		MilestoneID: "M-01", Revision: stNow.Revision, Action: "retry", Reason: "r",
	})
	t.Setenv(milestoneUserActionEnv, string(proofJSON))
	code, out = runCLI(t, "run", task, "--only", "milestone", "--milestone-request", reqFile, "--milestone-request-digest", digest, "--expect-revision", fmt.Sprint(stNow.Revision))
	require.Equal(t, 0, code, out)
	_ = st
}

func TestStartMilestoneControlPaths(t *testing.T) {
	controlFixture(t, "completed")
	task, root := t.TempDir(), t.TempDir()
	realTask, err := filepath.EvalSymlinks(task)
	require.NoError(t, err)
	t.Setenv("DSH_SESSION_ID", "host-1")

	setProof := func(action string) {
		p, _ := json.Marshal(machine.MilestoneUserAction{
			Kind: "dsh", SessionID: "host-1", CommandID: "cmd-1", TaskDir: realTask,
			MilestoneID: "M-01", Revision: 1, Action: action, Reason: "manual",
		})
		t.Setenv(milestoneUserActionEnv, string(p))
	}

	setProof("retry")
	cmd := controlCmd()
	cmd.SetArgs([]string{"milestone", "--task-dir", task, "--cwd", root})
	cmd.SetOut(io.Discard)
	cmd.SetIn(strings.NewReader(`{"action":"retry","milestone-id":"M-01","revision":1,"reason":"manual"}`))
	require.NoError(t, cmd.Execute())

	origSnap := controlSnapshotFn
	t.Cleanup(func() { controlSnapshotFn = origSnap })
	controlSnapshotFn = func(dir string) (controlResponse, error) {
		return controlResponse{
			Protocol: 1, TaskDir: dir, Running: true,
			State: &state.State{
				Revision:   1,
				Control:    &state.ControlResult{Message: "busy milestone control"},
				Milestones: map[string]state.MilestoneState{"M-01": {Status: "in-progress"}},
			},
		}, nil
	}
	cmd2 := controlCmd()
	cmd2.SetIn(strings.NewReader(`{"action":"retry","milestone-id":"M-01","revision":1,"reason":"manual"}`))
	reply2, err := startMilestoneControl(cmd2, realTask, root)
	require.Error(t, err)
	require.NotNil(t, reply2.MilestoneControl)
	assert.False(t, reply2.MilestoneControl.Applied)
	assert.Equal(t, "busy milestone control", reply2.MilestoneControl.Message)

	controlSnapshotFn = origSnap
	for _, scenario := range []string{"input", "proof", "directory", "stat", "symlink", "temp", "write", "sync"} {
		t.Run(scenario, func(t *testing.T) {
			d, r := t.TempDir(), t.TempDir()
			rd, _ := filepath.EvalSymlinks(d)
			p, _ := json.Marshal(machine.MilestoneUserAction{
				Kind: "dsh", SessionID: "host-1", CommandID: "cmd-1", TaskDir: rd,
				MilestoneID: "M-01", Revision: 1, Action: "retry", Reason: "manual",
			})
			t.Setenv(milestoneUserActionEnv, string(p))
			c := controlCmd()
			c.SetIn(strings.NewReader(`{"action":"retry","milestone-id":"M-01","revision":1,"reason":"manual"}`))
			oldStat, oldTemp, oldWrite, oldSync := sessionDirectoryStat, sessionInputTemp, sessionInputWrite, sessionInputSync
			t.Cleanup(func() {
				sessionDirectoryStat, sessionInputTemp, sessionInputWrite, sessionInputSync = oldStat, oldTemp, oldWrite, oldSync
			})
			switch scenario {
			case "input":
				c.SetIn(failingEditorReader{})
			case "proof":
				_ = os.Unsetenv(milestoneUserActionEnv)
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
			_, err := startMilestoneControl(c, rd, r)
			require.Error(t, err)
		})
	}
}
