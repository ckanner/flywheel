package cli

import (
	"bytes"
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
	"github.com/stretchr/testify/require"
)

func TestConsoleReadCommandsExposeRegisteredRunsAndMetrics(t *testing.T) {
	root, task := t.TempDir(), t.TempDir()
	t.Setenv("FLYWHEEL_RUNS_DIR", t.TempDir())
	code, reply := runControlTest(t, "runs", "", root)
	require.Zero(t, code)
	require.Empty(t, reply.Runs)
	code, reply = runControlTest(t, "metrics", task, root)
	require.Zero(t, code)
	require.NotNil(t, reply.Metrics)
	code, _ = runControlTest(t, "history", task, root, "--session-key", "missing")
	require.Equal(t, 1, code)
	code, _ = runControlTest(t, "status", task, root, "--session-key", "wrong")
	require.Equal(t, 1, code)
	t.Setenv("FLYWHEEL_RUNS_DIR", "relative")
	code, _ = runControlTest(t, "runs", "", root)
	require.Equal(t, 1, code)
	task = newCLISandbox(t)
	chdirT(t, task)
	code, _ = runCLI(t, "run", task)
	require.Equal(t, 1, code)
}

func TestSessionCommandPrivatePayloadValidation(t *testing.T) {
	raw := `{"action":"claim","key":"execute/M01-T01","revision":1,"reason":"manual"}`
	request, err := readSessionCommand(strings.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, "claim", request.Action)
	for _, reader := range []io.Reader{failingEditorReader{}, strings.NewReader("{} {}"), strings.NewReader(strings.Repeat(" ", 131073)), strings.NewReader(`{"bad":true}`), strings.NewReader(`{}`)} {
		_, err := readSessionCommand(reader)
		require.Error(t, err)
	}
	task := t.TempDir()
	directory := filepath.Join(task, ".control")
	require.NoError(t, os.Mkdir(directory, 0700))
	file := filepath.Join(directory, "session-request-one")
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
	_, err = loadSessionInput(task, "/unrelated", digest)
	require.Error(t, err)
	_, err = loadSessionInput(task, file, digest)
	require.Error(t, err)
	require.NoError(t, os.WriteFile(file, []byte(raw), 0644))
	_, err = loadSessionInput(task, file, digest)
	require.Error(t, err)
	require.NoError(t, os.Chmod(file, 0600))
	_, err = loadSessionInput(task, file, "changed")
	require.ErrorContains(t, err, "content changed")
	request, err = loadSessionInput(task, file, digest)
	require.NoError(t, err)
	require.Equal(t, "claim", request.Action)
	_, err = os.Stat(file)
	require.True(t, os.IsNotExist(err))
	require.NoError(t, os.Symlink("missing", file))
	_, err = loadSessionInput(task, file, digest)
	require.Error(t, err)
	require.NoError(t, os.Remove(file))
	require.NoError(t, os.WriteFile(file, []byte("{}"), 0600))
	_, err = loadSessionInput(task, file, fmt.Sprintf("%x", sha256.Sum256([]byte("{}"))))
	require.Error(t, err)
}

func TestSessionControlTransportsPromptFileAndHashWithoutArgvContent(t *testing.T) {
	controlFixture(t, "completed")
	task, root := t.TempDir(), t.TempDir()
	cmd := controlCmd()
	cmd.SetArgs([]string{"session", "--task-dir", task, "--cwd", root})
	cmd.SetOut(io.Discard)
	cmd.SetIn(strings.NewReader(`{"action":"send","key":"execute/M01-T01","revision":1,"prompt":"a private prompt"}`))
	require.NoError(t, cmd.Execute())
	args, err := os.ReadFile(filepath.Join(task, "fixture-args.json"))
	require.NoError(t, err)
	require.Contains(t, string(args), "--session-request-digest")
	require.NotContains(t, string(args), "a private prompt")
	var argv []string
	require.NoError(t, json.Unmarshal(args, &argv))
	var file, digest string
	for i, arg := range argv {
		if arg == "--session-request" {
			file = argv[i+1]
		}
		if arg == "--session-request-digest" {
			digest = argv[i+1]
		}
	}
	payload, err := loadSessionInput(task, file, digest)
	require.NoError(t, err)
	require.Equal(t, "a private prompt", payload.Prompt)
}

func TestSessionControlInputAndFilesystemFailures(t *testing.T) {
	for _, scenario := range []string{"input", "directory", "symlink", "temp", "write", "sync"} {
		t.Run(scenario, func(t *testing.T) {
			task, root := t.TempDir(), t.TempDir()
			cmd := controlCmd()
			cmd.SetIn(strings.NewReader(`{"action":"claim","key":"execute/M01-T01","revision":1,"reason":"manual"}`))
			oldTemp, oldWrite, oldSync := sessionInputTemp, sessionInputWrite, sessionInputSync
			t.Cleanup(func() { sessionInputTemp, sessionInputWrite, sessionInputSync = oldTemp, oldWrite, oldSync })
			switch scenario {
			case "input":
				cmd.SetIn(failingEditorReader{})
			case "directory":
				require.NoError(t, os.WriteFile(filepath.Join(task, ".control"), nil, 0600))
			case "symlink":
				require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(task, ".control")))
			case "temp":
				sessionInputTemp = func(string, string) (*os.File, error) { return nil, errors.New("temp fail") }
			case "write":
				sessionInputWrite = func(*os.File, []byte) (int, error) { return 0, errors.New("write fail") }
			case "sync":
				sessionInputSync = func(*os.File) error { return errors.New("sync fail") }
			}
			_, err := startSessionControl(cmd, task, root)
			require.Error(t, err)
		})
	}
}

func interactiveFixture(t *testing.T) (string, *state.State) {
	task := newCLISandbox(t)
	root := filepath.Dir(filepath.Dir(task))
	chdirT(t, root)
	writeFileCli(t, state.StateFile(task), cliStateFixture)
	st, err := state.Load(task)
	require.NoError(t, err)
	digest := strings.Repeat("a", 64)
	r := &state.SessionRecord{Task: task, Role: "execute", Scope: "M01-T01", Cwd: root, Binding: digest, Goal: digest, Contract: digest, Generation: 1, Action: "resume", Reason: "test", ResumeCandidate: true, ThreadID: "thread", Protocol: "test/session", Last: &state.SessionInvocation{ID: "call-human1", Status: "consumed", StartedAt: state.NowISO(), CandidateBefore: digest, CandidateAfter: digest, Persistent: true, Dispatch: "dispatched", Outcome: "completed", CleanupConfirmed: true, UsageStatus: "unknown", Accounted: true}}
	receipt := state.SessionReceipt{Version: 1, InvocationID: r.Last.ID, Key: "execute/M01-T01", Task: task, Cwd: root, Binding: digest, CandidateAfter: digest, Result: json.RawMessage(`{"text":"last response"}`)}
	r.Last.ReceiptDigest, err = state.WriteSessionReceipt(task, "execute/M01-T01", receipt)
	require.NoError(t, err)
	st.Sessions = &state.SessionLedger{Version: 1, Records: map[string]*state.SessionRecord{"execute/M01-T01": r}}
	st.Handoff = &state.Handoff{Key: "execute/M01-T01", Thread: "thread", Cwd: root, Packet: "M01-T01", Candidate: digest, StartedAt: state.NowISO(), Reason: "test"}
	st, err = state.Save(task, st, nil)
	require.NoError(t, err)
	return task, st
}

func TestInteractiveSessionKeepsReservationAcrossHistoryQuitAndEOF(t *testing.T) {
	for _, input := range []string{"\n/history\n/quit\n", "/history\n", "/return\n", "fix it\n"} {
		t.Run(input, func(t *testing.T) {
			task, _ := interactiveFixture(t)
			t.Setenv("FLYWHEEL_RUNS_DIR", t.TempDir())
			cmd := sessionCmd()
			cmd.SetArgs([]string{task, "--key", "execute/M01-T01"})
			cmd.SetIn(strings.NewReader(input))
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(io.Discard)
			err := cmd.Execute()
			if input == "fix it\n" {
				require.ErrorContains(t, err, "binding")
			} else {
				require.NoError(t, err)
			}
			st, err := state.Load(task)
			require.NoError(t, err)
			if input == "/return\n" {
				require.Nil(t, st.Handoff)
			} else {
				require.NotNil(t, st.Handoff)
			}
			if strings.Contains(input, "/history") {
				require.Contains(t, out.String(), "last response")
			}
		})
	}
}

func TestSessionStageAdmissionRequiresMatchingPayloadRevision(t *testing.T) {
	task := newCLISandbox(t)
	chdirT(t, task)
	for _, args := range [][]string{{"session", task}, {"session", task, "--key", "missing"}, {"run", task, "--session-request", "nope"}, {"run", task, "--only", "session"}} {
		code, _ := runCLI(t, args...)
		require.Equal(t, 1, code)
	}
	_, err := machine.RunSingleStep(machine.RunSingleStepOpts{Only: "session"})
	require.Error(t, err)
}

type sessionMutationReader struct {
	before func()
	input  io.Reader
}

func (r *sessionMutationReader) Read(p []byte) (int, error) {
	if r.before != nil {
		r.before()
		r.before = nil
	}
	return r.input.Read(p)
}

func TestPrivateSessionInputRejectsReplacementAndIOFailures(t *testing.T) {
	for _, scenario := range []string{"root", "parent", "directory-stat", "open", "stat", "replaced", "read"} {
		t.Run(scenario, func(t *testing.T) {
			task := t.TempDir()
			dir := filepath.Join(task, ".control")
			require.NoError(t, os.Mkdir(dir, 0700))
			file := filepath.Join(dir, "session-request-test")
			raw := []byte(`{"action":"return","key":"execute/M01-T01","revision":1}`)
			require.NoError(t, os.WriteFile(file, raw, 0600))
			digest := fmt.Sprintf("%x", sha256.Sum256(raw))
			oldStat, oldOpen, oldRead := sessionDirectoryStat, sessionInputOpen, sessionInputRead
			t.Cleanup(func() { sessionDirectoryStat, sessionInputOpen, sessionInputRead = oldStat, oldOpen, oldRead })
			switch scenario {
			case "root":
				task = filepath.Join(task, "missing")
			case "parent":
				file = filepath.Join(task, "missing", "session-request-test")
			case "directory-stat":
				sessionDirectoryStat = func(string) (os.FileInfo, error) { return nil, errors.New("directory replaced") }
				cmd := controlCmd()
				cmd.SetIn(bytes.NewReader(raw))
				_, err := startSessionControl(cmd, task, task)
				require.Error(t, err)
				return
			case "open":
				sessionInputOpen = func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("open failed") }
			case "stat":
				sessionInputOpen = func(path string, flags int, mode os.FileMode) (*os.File, error) {
					f, err := oldOpen(path, flags, mode)
					require.NoError(t, err)
					require.NoError(t, f.Close())
					return f, nil
				}
			case "replaced":
				sessionInputOpen = func(path string, flags int, mode os.FileMode) (*os.File, error) {
					require.NoError(t, os.Rename(path, path+".old"))
					require.NoError(t, os.WriteFile(path, raw, 0600))
					return oldOpen(path, flags, mode)
				}
			case "read":
				sessionInputRead = func(io.Reader) ([]byte, error) { return nil, errors.New("read failed") }
			}
			_, err := loadSessionInput(task, file, digest)
			require.Error(t, err)
		})
	}
}

func TestInteractiveSessionRejectsMissingStateConfigAndForeignReservation(t *testing.T) {
	for _, scenario := range []string{"path", "config", "index", "claim", "foreign", "state-race", "reconcile-race", "history-race", "reconcile"} {
		t.Run(scenario, func(t *testing.T) {
			task, st := interactiveFixture(t)
			t.Setenv("FLYWHEEL_RUNS_DIR", t.TempDir())
			input := io.Reader(strings.NewReader("/quit\n"))
			switch scenario {
			case "path":
				task = filepath.Join(task, "missing")
			case "config":
				writeFileCli(t, filepath.Join(task, "flywheel.config.json"), "{")
			case "index":
				t.Setenv("FLYWHEEL_RUNS_DIR", "relative")
			case "claim":
				st.Handoff = nil
				_, err := state.Save(task, st, nil)
				require.NoError(t, err)
			case "foreign":
				st.Handoff.Key = "review/M01-T01"
				_, err := state.Save(task, st, nil)
				require.NoError(t, err)
			case "state-race", "reconcile-race":
				line := "/return\n"
				if scenario == "reconcile-race" {
					line = "/reconcile\n"
				}
				input = &sessionMutationReader{before: func() { require.NoError(t, os.Remove(state.StateFile(task))) }, input: strings.NewReader(line)}
			case "history-race":
				input = &sessionMutationReader{before: func() { require.NoError(t, os.RemoveAll(filepath.Join(task, ".sessions"))) }, input: strings.NewReader("/history\n")}
			case "reconcile":
				input = strings.NewReader("/reconcile\n/quit\n")
			}
			cmd := sessionCmd()
			cmd.SetArgs([]string{task, "--key", "execute/M01-T01"})
			cmd.SetIn(input)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			err := cmd.Execute()
			if scenario == "reconcile" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestRunSessionFileRevisionIsRecheckedInTheEngine(t *testing.T) {
	task, st := interactiveFixture(t)
	t.Setenv("FLYWHEEL_RUNS_DIR", t.TempDir())
	for _, revision := range []int64{st.Revision, st.Revision + 1} {
		directory := filepath.Join(task, ".control")
		require.NoError(t, os.MkdirAll(directory, 0700))
		file := filepath.Join(directory, "session-request-return")
		raw, _ := json.Marshal(machine.SessionCommand{Action: "reconcile", Key: "execute/M01-T01", Revision: st.Revision})
		require.NoError(t, os.WriteFile(file, raw, 0600))
		code, out := runCLI(t, "run", task, "--only", "session", "--session-request", file, "--session-request-digest", fmt.Sprintf("%x", sha256.Sum256(raw)), "--expect-revision", fmt.Sprint(revision))
		if revision == st.Revision {
			require.Zero(t, code, out)
		} else {
			require.Equal(t, 1, code)
		}
	}
}

func TestControlConsoleQueriesAndWriteAdmissionHaveOneJSONBoundary(t *testing.T) {
	task := newCLISandbox(t)
	root := filepath.Dir(filepath.Dir(task))
	writeFileCli(t, state.StateFile(task), cliStateFixture)
	code, reply := runControlTest(t, "node", task, root, "--packet", "M01-T01", "--compact")
	require.Zero(t, code)
	require.NotNil(t, reply.Node)
	require.Equal(t, reply.State.Revision, reply.Node.Revision)
	require.Equal(t, "console-excerpt", reply.Projection)
	code, _ = runControlTest(t, "status", task, root, "--run-id", "wrong")
	require.Equal(t, 1, code)
	for _, tc := range []struct {
		action string
		input  io.Reader
	}{
		{"delivery", failingEditorReader{}}, {"delivery", strings.NewReader("{} {}")}, {"delivery", strings.NewReader(`{"unknown":true}`)},
		{"delivery", strings.NewReader(`{"action":"create"}`)}, {"deliver", strings.NewReader(`{"action":"inspect"}`)},
		{"delivery", strings.NewReader(`{"action":"inspect"}`)}, {"deliver", strings.NewReader(`{"action":"create"}`)},
	} {
		cmd := controlCmd()
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetArgs([]string{tc.action, "--task-dir", task, "--cwd", root})
		cmd.SetIn(tc.input)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(io.Discard)
		require.Error(t, cmd.Execute())
		var response controlResponse
		require.NoError(t, json.Unmarshal(out.Bytes(), &response))
		require.False(t, response.OK)
	}
	writeFileCli(t, filepath.Join(task, "flywheel.config.json"), "invalid")
	cmd := controlCmd()
	cmd.SetArgs([]string{"deliver", "--task-dir", task, "--cwd", root})
	cmd.SetIn(strings.NewReader(`{"action":"create"}`))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	require.Error(t, cmd.Execute())
}

func TestInteractiveReleaseLeavesTheTaskPaused(t *testing.T) {
	task, _ := interactiveFixture(t)
	cmd := sessionCmd()
	cmd.SetArgs([]string{task, "--key", "execute/M01-T01"})
	cmd.SetIn(strings.NewReader("/release\n"))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	require.NoError(t, cmd.Execute())
	st, err := state.Load(task)
	require.NoError(t, err)
	require.Nil(t, st.Handoff)
	require.Equal(t, "paused", st.Status)
}

func TestTerminalSingleStageRegistersItsHostConversation(t *testing.T) {
	task := newCLISandbox(t)
	chdirT(t, task)
	t.Setenv("FLYWHEEL_RUNS_DIR", t.TempDir())
	t.Setenv("DSH_SESSION_ID", "desktop-terminal-session")
	code, out := runCLI(t, "run", task, "--only", "verify", "--packet", "M01-T01")
	require.Zero(t, code, out)
	root := filepath.Dir(filepath.Dir(task))
	code, reply := runControlTest(t, "runs", "", root)
	require.Zero(t, code)
	require.Len(t, reply.Runs, 1)
	require.Equal(t, "desktop-terminal-session", reply.Runs[0].HostSessionID)
	require.Equal(t, "verify", reply.Runs[0].Kind)
	require.NotEmpty(t, reply.Runs[0].FinishedAt)
	physical, err := filepath.EvalSymlinks(task)
	require.NoError(t, err)
	require.Equal(t, physical, reply.Runs[0].TaskDir)
}
