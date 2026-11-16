package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Match the engine's atomic publication: polling must see either no file or
// the complete fixture bytes, including intentionally malformed JSON. Keeping
// the bytes unchanged also preserves the fixture's initial revision.
func writeControlFixtureFile(path string, data []byte) {
	tmp := fmt.Sprintf("%s.fixture-%d", path, os.Getpid())
	defer os.Remove(tmp)
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		panic(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		panic(err)
	}
}

// A real subprocess of this test binary implements lifecycle cases. It never
// contacts a model. The real-run scenario exercises the actual mock engine too.
func controlChildHelperMain() int {
	if len(os.Args) < 3 || os.Args[1] != "run" {
		return 2
	}
	taskDir := os.Args[2]
	scenario := os.Getenv("FLYWHEEL_CONTROL_SCENARIO")
	if scenario == "real-run" {
		return Execute()
	}
	argv, _ := json.Marshal(os.Args[1:])
	writeControlFixtureFile(filepath.Join(taskDir, "fixture-args.json"), argv)
	writeControlFixtureFile(filepath.Join(taskDir, "fixture.pid"), []byte(strconv.Itoa(os.Getpid())))
	fmt.Println("fixture stdout")
	fmt.Fprintln(os.Stderr, "fixture stderr")
	switch scenario {
	case "never-ready":
		time.Sleep(500 * time.Millisecond)
		return 0
	case "start-fail":
		return 2
	case "bad-state":
		writeControlFixtureFile(filepath.Join(taskDir, "state.json"), []byte("{broken"))
		return 0
	case "bad-pid":
		release, err := machine.AcquireRunLease(taskDir)
		if err != nil {
			return 2
		}
		defer release()
		writeControlFixtureFile(machine.PidFile(taskDir), []byte("{broken"))
		time.Sleep(150 * time.Millisecond)
		return 0
	case "completed", "escalated":
		status := scenario
		writeControlFixtureFile(filepath.Join(taskDir, "state.json"), []byte(strings.Replace(cliStateFixture, `"status": "running"`, `"status": "`+status+`"`, 1)))
		if scenario == "escalated" {
			return 3
		}
		return 0
	}
	release, err := machine.AcquireRunLease(taskDir)
	if err != nil {
		return 2
	}
	defer release()
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	defer signal.Stop(term)
	writeControlFixtureFile(filepath.Join(taskDir, "state.json"), []byte(cliStateFixture))
	token := "fixture-" + strconv.Itoa(os.Getpid())
	if scenario == "other-owner" {
		payload, _ := json.Marshal(machine.RunControl{PID: os.Getpid() + 1000000, StartedAt: time.Now().UTC().Format(time.RFC3339Nano), ControlToken: token})
		writeControlFixtureFile(machine.PidFile(taskDir), payload)
		time.Sleep(50 * time.Millisecond)
		return 2
	}
	payload, _ := json.Marshal(machine.RunControl{PID: os.Getpid(), StartedAt: time.Now().UTC().Format(time.RFC3339Nano), ControlToken: token})
	writeControlFixtureFile(machine.PidFile(taskDir), payload)
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
waitForRequest:
	for {
		select {
		case <-term:
			break waitForRequest
		case <-tick.C:
			var request struct {
				Protocol int    `json:"protocol"`
				Token    string `json:"control-token"`
			}
			raw, _ := os.ReadFile(machine.StopRequestFile(taskDir))
			if scenario != "ignore-stop" && json.Unmarshal(raw, &request) == nil && request.Protocol == 1 && request.Token == token {
				break waitForRequest
			}
		}
	}
	// Hold both PID and lease through simulated managed-child cleanup.
	time.Sleep(100 * time.Millisecond)
	writeControlFixtureFile(filepath.Join(taskDir, "cleanup-complete"), []byte("yes"))
	_ = os.Remove(machine.PidFile(taskDir))
	_ = os.Remove(machine.StopRequestFile(taskDir))
	return 143
}

func runControlTest(t *testing.T, action, taskDir, cwd string, extra ...string) (int, controlResponse) {
	t.Helper()
	args := append([]string{"control", action, "--task-dir", taskDir, "--cwd", cwd}, extra...)
	var out, stderr bytes.Buffer
	code := runRoot(args, &out, &stderr)
	assert.Empty(t, stderr.String(), "control must keep even errors in its single stdout JSON")
	var result controlResponse
	require.NoError(t, json.Unmarshal(out.Bytes(), &result), out.String())
	assert.Equal(t, 1, result.Protocol)
	assert.Equal(t, 1, strings.Count(out.String(), "\n"), "one response, no log lines")
	return code, result
}

func controlFixture(t *testing.T, scenario string) {
	t.Setenv("FLYWHEEL_RUNS_DIR", t.TempDir())
	t.Helper()
	t.Setenv("FLYWHEEL_CLI_EXEC_HELPER", "control-child")
	t.Setenv("FLYWHEEL_CONTROL_SCENARIO", scenario)
	oldWait, oldPoll := controlStartWait, controlPollInterval
	controlStartWait, controlPollInterval = 2*time.Second, 5*time.Millisecond
	t.Cleanup(func() { controlStartWait, controlPollInterval = oldWait, oldPoll })
}

func TestControlStartStatusStopRealProcess(t *testing.T) {
	controlFixture(t, "hold")
	stopPollFast(t)
	taskDir, cwd := t.TempDir(), t.TempDir()
	code, started := runControlTest(t, "start", taskDir, cwd)
	require.Zero(t, code, "%+v", started)
	require.True(t, started.OK)
	require.True(t, started.Running)
	require.Positive(t, started.PID)
	t.Cleanup(func() { _ = syscall.Kill(started.PID, syscall.SIGKILL) })
	pgid, err := syscall.Getpgid(started.PID)
	require.NoError(t, err)
	assert.Equal(t, started.PID, pgid, "engine is detached from the control/UI process group")

	code, again := runControlTest(t, "start", taskDir, cwd)
	assert.Zero(t, code)
	assert.Equal(t, started.PID, again.PID, "start is idempotent for an existing run")
	rawBefore, err := os.ReadFile(filepath.Join(taskDir, "state.json"))
	require.NoError(t, err)
	code, snapshot := runControlTest(t, "status", taskDir, cwd)
	assert.Zero(t, code)
	assert.True(t, snapshot.Running)
	require.NotNil(t, snapshot.State)
	assert.Equal(t, int64(3), snapshot.State.Revision)
	rawAfter, err := os.ReadFile(filepath.Join(taskDir, "state.json"))
	require.NoError(t, err)
	assert.Equal(t, rawBefore, rawAfter, "status cannot mutate engine state")

	before := time.Now()
	code, stopped := runControlTest(t, "stop", taskDir, cwd)
	require.Zero(t, code, "%+v", stopped)
	assert.GreaterOrEqual(t, time.Since(before), 100*time.Millisecond)
	assert.True(t, stopped.OK)
	assert.False(t, stopped.Running)
	require.NotNil(t, stopped.State)
	assert.Equal(t, "stopped", stopped.State.Status)
	assert.Equal(t, int64(4), stopped.State.Revision, "stopped is a CAS update after lease release")
	assert.FileExists(t, filepath.Join(taskDir, "cleanup-complete"))
	log, err := os.ReadFile(filepath.Join(taskDir, "orchestrator.log"))
	require.NoError(t, err)
	assert.Contains(t, string(log), "fixture stdout")
	assert.Contains(t, string(log), "fixture stderr")
}

func TestControlStartTerminalAndStartupFailures(t *testing.T) {
	for _, scenario := range []string{"completed", "escalated", "start-fail", "bad-state", "bad-pid", "never-ready"} {
		t.Run(scenario, func(t *testing.T) {
			controlFixture(t, scenario)
			if scenario == "never-ready" {
				controlStartWait = 25 * time.Millisecond
			}
			if scenario == "bad-state" {
				controlPollInterval = time.Second // observe malformed final state after child exit
			}
			taskDir, cwd := t.TempDir(), t.TempDir()
			code, response := runControlTest(t, "start", taskDir, cwd)
			if scenario == "completed" || scenario == "escalated" {
				assert.Zero(t, code, "%+v", response)
				assert.True(t, response.OK)
				assert.False(t, response.Running)
				require.NotNil(t, response.State)
				assert.Equal(t, scenario, response.State.Status)
			} else {
				assert.Equal(t, 1, code)
				assert.False(t, response.OK)
				assert.NotEmpty(t, response.Error)
			}
			if scenario == "never-ready" {
				assert.True(t, response.Running, "spawned-but-not-ready must not masquerade as stopped")
				assert.Positive(t, response.PID)
			}
			// Reap through startControlledTask's Wait goroutine before temp cleanup.
			if raw, err := os.ReadFile(filepath.Join(taskDir, "fixture.pid")); err == nil {
				pid, err := strconv.Atoi(string(raw))
				require.NoError(t, err)
				_ = syscall.Kill(pid, syscall.SIGTERM)
				require.Eventually(t, func() bool { return signalZero(pid) == syscall.ESRCH }, time.Second, 5*time.Millisecond)
			}
		})
	}
}

func TestControlStatusIsReadOnlyForUnstartedTask(t *testing.T) {
	taskDir := t.TempDir()
	code, response := runControlTest(t, "status", taskDir, t.TempDir())
	assert.Zero(t, code)
	assert.True(t, response.OK)
	assert.False(t, response.Running)
	assert.Nil(t, response.State)
	files, err := os.ReadDir(taskDir)
	require.NoError(t, err)
	assert.Empty(t, files, "read-only status must not even create a run lock")
}

func TestControlErrorsUseSingleJSON(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	writeFileCli(t, file, "not a directory")
	for _, tc := range []struct{ action, task, cwd string }{
		{"status", "relative/task", t.TempDir()}, {"status", t.TempDir(), "relative/cwd"},
		{"status", filepath.Join(t.TempDir(), "missing"), t.TempDir()},
		{"status", file, t.TempDir()}, {"restart", t.TempDir(), t.TempDir()},
	} {
		code, response := runControlTest(t, tc.action, tc.task, tc.cwd)
		assert.Equal(t, 1, code)
		assert.False(t, response.OK)
		assert.NotEmpty(t, response.Error)
	}
	for _, args := range [][]string{
		{"control"},
		{"control", "status", "--bogus", "--task-dir", "/task"},
		{"control", "status", "--task-dir=/task", "--bogus"},
		{"control", "status", "--task-dir"},
	} {
		var out, stderr bytes.Buffer
		code := runRoot(args, &out, &stderr)
		assert.Equal(t, 1, code)
		assert.Empty(t, stderr.String())
		var response controlResponse
		require.NoError(t, json.Unmarshal(out.Bytes(), &response), out.String())
		assert.Equal(t, 1, response.Protocol)
		assert.NotEmpty(t, response.Error)
	}
}

func TestControlFilesystemAndOSFailureBoundaries(t *testing.T) {
	t.Run("directory removed after resolution", func(t *testing.T) {
		taskDir := t.TempDir()
		original := controlStat
		controlStat = func(path string) (os.FileInfo, error) { _ = os.Remove(path); return os.Stat(path) }
		t.Cleanup(func() { controlStat = original })
		code, response := runControlTest(t, "status", taskDir, t.TempDir())
		assert.Equal(t, 1, code)
		assert.NotEmpty(t, response.Error)
	})
	t.Run("executable lookup fails", func(t *testing.T) {
		original := controlExecutable
		controlExecutable = func() (string, error) { return "", os.ErrNotExist }
		t.Cleanup(func() { controlExecutable = original })
		code, response := runControlTest(t, "start", t.TempDir(), t.TempDir())
		assert.Equal(t, 1, code)
		assert.NotEmpty(t, response.Error)
	})
	t.Run("spawn fails", func(t *testing.T) {
		original := controlExecutable
		controlExecutable = func() (string, error) { return filepath.Join(t.TempDir(), "missing"), nil }
		t.Cleanup(func() { controlExecutable = original })
		code, response := runControlTest(t, "start", t.TempDir(), t.TempDir())
		assert.Equal(t, 1, code)
		assert.NotEmpty(t, response.Error)
	})
	t.Run("log cannot open", func(t *testing.T) {
		taskDir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(taskDir, "orchestrator.log"), 0o755))
		code, response := runControlTest(t, "start", taskDir, t.TempDir())
		assert.Equal(t, 1, code)
		assert.NotEmpty(t, response.Error)
	})
	t.Run("pid cannot read", func(t *testing.T) {
		taskDir := t.TempDir()
		release, err := machine.AcquireRunLease(taskDir)
		require.NoError(t, err)
		defer release()
		require.NoError(t, os.Mkdir(machine.PidFile(taskDir), 0o755))
		for _, action := range []string{"status", "stop"} {
			code, response := runControlTest(t, action, taskDir, t.TempDir())
			assert.Equal(t, 1, code)
			assert.NotEmpty(t, response.Error)
		}
	})
	t.Run("lease prevents premature stopped", func(t *testing.T) {
		taskDir := t.TempDir()
		writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
		release, err := machine.AcquireRunLease(taskDir)
		require.NoError(t, err)
		defer release()
		code, response := runControlTest(t, "stop", taskDir, t.TempDir())
		assert.Equal(t, 1, code)
		require.NotNil(t, response.State)
		assert.Equal(t, "running", response.State.Status)
		assert.Equal(t, int64(3), response.State.Revision)
	})
	t.Run("legacy run cannot be signalled from its pid file", func(t *testing.T) {
		taskDir := t.TempDir()
		release, err := machine.AcquireRunLease(taskDir)
		require.NoError(t, err)
		defer release()
		writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
		writePidFile(t, taskDir, fmt.Sprintf(`{"pid":%d}`, os.Getpid()))
		original := processSignal
		processSignal = func(int, syscall.Signal) error { panic("control must not signal a PID from disk") }
		t.Cleanup(func() { processSignal = original })
		code, response := runControlTest(t, "status", taskDir, t.TempDir())
		assert.Equal(t, 1, code)
		assert.True(t, response.Running)
		code, response = runControlTest(t, "stop", taskDir, t.TempDir())
		assert.Equal(t, 1, code)
		assert.True(t, response.Running)
		assert.Contains(t, response.Error, "no control token")
		st, err := state.Load(taskDir)
		require.NoError(t, err)
		assert.Equal(t, "running", st.Status)
	})
	t.Run("request publication fails", func(t *testing.T) {
		taskDir := t.TempDir()
		release, err := machine.AcquireRunLease(taskDir)
		require.NoError(t, err)
		defer release()
		writePidFile(t, taskDir, mustJSONT(t, machine.RunControl{PID: os.Getpid(), ControlToken: "held"}))
		require.NoError(t, os.Mkdir(machine.StopRequestFile(taskDir), 0o700))
		code, response := runControlTest(t, "stop", taskDir, t.TempDir())
		assert.Equal(t, 1, code)
		assert.True(t, response.Running)
		assert.NotEmpty(t, response.Error)
	})
	t.Run("lease cannot open", func(t *testing.T) {
		taskDir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(taskDir, ".run.lock"), 0o700))
		code, response := runControlTest(t, "stop", taskDir, t.TempDir())
		assert.Equal(t, 1, code)
		assert.Contains(t, response.Error, "无法确认运行锁")
	})
	t.Run("lease cannot be observed", func(t *testing.T) {
		taskDir := t.TempDir()
		require.NoError(t, os.Symlink(".run.lock", filepath.Join(taskDir, ".run.lock")))
		code, response := runControlTest(t, "status", taskDir, t.TempDir())
		assert.Equal(t, 1, code)
		assert.NotEmpty(t, response.Error)
	})
	t.Run("sidecar cannot be cleared", func(t *testing.T) {
		taskDir := t.TempDir()
		writeFileCli(t, filepath.Join(machine.StopRequestFile(taskDir), "file"), "not empty")
		code, response := runControlTest(t, "stop", taskDir, t.TempDir())
		assert.Equal(t, 1, code)
		assert.NotEmpty(t, response.Error)
	})
}

func TestStalePIDCannotTargetAnUnrelatedLiveProcess(t *testing.T) {
	child := exec.Command("sleep", "30")
	require.NoError(t, child.Start())
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
	writePidFile(t, taskDir, mustJSONT(t, machine.RunControl{PID: child.Process.Pid, ControlToken: "old-run"}))
	code, snapshot := runControlTest(t, "status", taskDir, t.TempDir())
	require.Zero(t, code)
	assert.False(t, snapshot.Running, "a reused PID is not a running task without its lease")
	code, stopped := runControlTest(t, "stop", taskDir, t.TempDir())
	require.Zero(t, code, "%+v", stopped)
	assert.False(t, stopped.Running)
	assert.Equal(t, "stopped", stopped.State.Status)
	assert.NoError(t, child.Process.Signal(syscall.Signal(0)), "stop must not signal the unrelated live child")
	assert.NoFileExists(t, machine.PidFile(taskDir))
}

func TestStopReissuesRequestForANewRunToken(t *testing.T) {
	stopPollFast(t)
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
	release, err := machine.AcquireRunLease(taskDir)
	require.NoError(t, err)
	defer release()
	writePidFile(t, taskDir, mustJSONT(t, machine.RunControl{PID: os.Getpid(), ControlToken: "first-run"}))
	result := make(chan error, 1)
	go func() { _, err := stopTask(taskDir); result <- err }()
	requestIs := func(token string) bool {
		raw, _ := os.ReadFile(machine.StopRequestFile(taskDir))
		var request map[string]any
		return json.Unmarshal(raw, &request) == nil && request["control-token"] == token
	}
	require.Eventually(t, func() bool { return requestIs("first-run") }, time.Second, time.Millisecond)
	// Atomic replacement models a subsequent run winning admission between polls.
	tmp := filepath.Join(taskDir, "pid-next")
	writeFileCli(t, tmp, mustJSONT(t, machine.RunControl{PID: os.Getpid(), ControlToken: "second-run"}))
	require.NoError(t, os.Rename(tmp, machine.PidFile(taskDir)))
	require.Eventually(t, func() bool { return requestIs("second-run") }, time.Second, time.Millisecond)
	require.NoError(t, os.Remove(machine.PidFile(taskDir)))
	time.Sleep(3 * stopPollInterval)
	select {
	case err := <-result:
		t.Fatalf("stop returned while cleanup still held the lease: %v", err)
	default:
	}
	release()
	require.NoError(t, <-result)
	st, err := state.Load(taskDir)
	require.NoError(t, err)
	assert.Equal(t, "stopped", st.Status)
	assert.NoFileExists(t, machine.StopRequestFile(taskDir))
}

func TestControlRunsActualMockEngineWithoutShell(t *testing.T) {
	controlFixture(t, "real-run")
	taskDir := newCLISandbox(t)
	cwd := filepath.Dir(filepath.Dir(taskDir))
	code, started := runControlTest(t, "start", taskDir, cwd)
	require.Zero(t, code, "%+v", started)
	require.Eventually(t, func() bool {
		r, err := controlSnapshot(taskDir)
		return err == nil && !r.Running && r.State != nil && r.State.Status == "completed"
	}, 10*time.Second, 10*time.Millisecond)
	st, err := state.Load(taskDir)
	require.NoError(t, err)
	assert.Equal(t, "completed", st.Status)
}

func TestStatusReportsUnknownUsageAndCost(t *testing.T) {
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
	st, err := state.Load(taskDir)
	require.NoError(t, err)
	st.Budget.UnknownUsageCalls = 2
	st.Budget.UnknownCostCalls = 3
	_, err = state.Save(taskDir, st, nil)
	require.NoError(t, err)
	code, out := runCLI(t, "status", taskDir)
	assert.Zero(t, code)
	assert.Contains(t, out, "2 次调用未报告 usage，3 次调用成本未知")
}

// The CLI cancellation tests exercise the real named-print handoff. This owned
// subprocess implements only the no-inference preparation protocol; the later
// print fixture, not this short-lived preparer, is the process being cancelled.
func controlKimiPrepareMain() int {
	var request struct {
		Type            string `json:"type"`
		Protocol        int    `json:"protocol"`
		RequestID       string `json:"requestId"`
		Cwd             string `json:"cwd"`
		Model           string `json:"model"`
		ReasoningEffort string `json:"reasoningEffort"`
		SessionTitle    string `json:"sessionTitle"`
		TimeoutMS       int64  `json:"timeoutMs"`
	}
	decoder := json.NewDecoder(os.Stdin)
	decoder.DisallowUnknownFields()
	if len(os.Args) != 2 || decoder.Decode(&request) != nil || request.Type != "prepare-print" || request.Protocol != 1 ||
		request.RequestID == "" || !filepath.IsAbs(request.Cwd) || request.Model == "" || request.ReasoningEffort == "" ||
		request.SessionTitle == "" || request.TimeoutMS <= 0 {
		return 2
	}
	record, _ := json.Marshal(map[string]any{"request": request, "workerPid": os.Getpid()})
	writeControlFixtureFile(os.Args[1], record)
	if json.NewEncoder(os.Stdout).Encode(map[string]any{
		"protocol": 1, "type": "prepared-print", "requestId": request.RequestID, "status": "ok",
		"sessionId": "control-kimi-prepared", "sessionTitle": request.SessionTitle,
		"cleanupConfirmed": true, "inferenceDispatched": false, "error": nil,
	}) != nil {
		return 2
	}
	return 0
}

func installControlKimiPreparation(t *testing.T, bin, markers string) string {
	t.Helper()
	record := filepath.Join(markers, "prepare-request.json")
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	worker := filepath.Join(bin, "prepare-kimi-print")
	script := "#!/bin/sh\nFLYWHEEL_CLI_EXEC_HELPER=kimi-print-prepare GORACE=atexit_sleep_ms=0 exec " + quote(os.Args[0]) + " " + quote(record) + "\n"
	require.NoError(t, os.WriteFile(worker, []byte(script), 0o755))
	t.Setenv("FLYWHEEL_KIMI_NATIVE_WORKER", worker)
	return record
}

func assertControlKimiPreparation(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var record struct {
		Request   map[string]any `json:"request"`
		WorkerPID int            `json:"workerPid"`
	}
	require.NoError(t, json.Unmarshal(data, &record))
	assert.Equal(t, "prepare-print", record.Request["type"])
	assert.NotEmpty(t, record.Request["sessionTitle"])
	assert.NotContains(t, record.Request, "prompt", "preparation must not execute the planning task")
	require.Positive(t, record.WorkerPID)
	assert.ErrorIs(t, syscall.Kill(record.WorkerPID, 0), syscall.ESRCH, "the preparation worker is reaped before the actual print process is cancelled")
}

func TestRunCommandContextCancellationReturns143AfterCleanup(t *testing.T) {
	taskDir := newCLISandbox(t)
	chdirT(t, filepath.Dir(filepath.Dir(taskDir)))
	configPath := filepath.Join(taskDir, "flywheel.config.json")
	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(configPath)), &cfg))
	cfg["drivers"].(map[string]any)["plan"] = "real"
	writeFileCli(t, configPath, mustJSONT(t, cfg))
	bin, markers := t.TempDir(), t.TempDir()
	marker, cleaned := filepath.Join(markers, "started"), filepath.Join(markers, "cleaned")
	prepared := installControlKimiPreparation(t, bin, markers)
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	script := "#!/bin/sh\n[ \"$1\" = --session ] && [ \"$2\" = control-kimi-prepared ] && [ -s " + quote(prepared) + " ] || exit 2\n" +
		"trap \"printf done > " + quote(cleaned) + "; exit 0\" TERM\nprintf ready > " + quote(marker) + "\nwhile :; do sleep 0.01; done\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "kimi"), []byte(script), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FLYWHEEL_KIMI_BIN", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan bool, 1)
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		seen := false
		for time.Now().Before(deadline) {
			if _, err := os.Stat(marker); err == nil {
				seen = true
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
		started <- seen
	}()
	root := newRootCmd()
	root.SetContext(ctx)
	root.SetArgs([]string{"run", taskDir, "--only", "plan"})
	var out, stderr bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&stderr)
	err := root.Execute()
	assert.True(t, <-started, "offline Kimi fixture must be the process being cancelled")
	assertControlKimiPreparation(t, prepared)
	var exit *exitError
	require.ErrorAs(t, err, &exit, out.String()+stderr.String())
	assert.Equal(t, 143, exit.code)
	assert.FileExists(t, cleaned, "context cancellation waits for the actual print process to handle termination")
	assert.NoFileExists(t, machine.PidFile(taskDir))
	release, leaseErr := machine.AcquireRunLease(taskDir)
	require.NoError(t, leaseErr, "CLI return must follow run cleanup")
	release()
}

func TestControlStopsActualEngineAndManagedCLI(t *testing.T) {
	controlFixture(t, "real-run")
	stopPollFast(t)
	taskDir := newCLISandbox(t)
	cwd := filepath.Dir(filepath.Dir(taskDir))
	configPath := filepath.Join(taskDir, "flywheel.config.json")
	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(configPath)), &cfg))
	cfg["drivers"].(map[string]any)["plan"] = "real"
	writeFileCli(t, configPath, mustJSONT(t, cfg))
	bin, markers := t.TempDir(), t.TempDir()
	ready, cleaned := filepath.Join(markers, "ready"), filepath.Join(markers, "cleaned")
	prepared := installControlKimiPreparation(t, bin, markers)
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	script := "#!/bin/sh\n[ \"$1\" = --session ] && [ \"$2\" = control-kimi-prepared ] && [ -s " + quote(prepared) + " ] || exit 2\n" +
		"trap \"printf done > " + quote(cleaned) + "; exit 0\" TERM\nprintf ready > " + quote(ready) + "\nwhile :; do sleep 0.01; done\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "kimi"), []byte(script), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FLYWHEEL_KIMI_BIN", "")
	code, started := runControlTest(t, "start", taskDir, cwd)
	require.Zero(t, code, "%+v", started)
	require.True(t, started.Running)
	t.Cleanup(func() { _ = syscall.Kill(started.PID, syscall.SIGKILL) })
	require.Eventually(t, func() bool { _, err := os.Stat(ready); return err == nil }, 10*time.Second, 5*time.Millisecond)
	assertControlKimiPreparation(t, prepared)
	code, stopped := runControlTest(t, "stop", taskDir, cwd)
	require.Zero(t, code, "%+v", stopped)
	assert.False(t, stopped.Running)
	assert.Equal(t, "stopped", stopped.State.Status)
	assert.FileExists(t, cleaned, "control stop returns after the offline CLI processed cancellation")
	assert.NoFileExists(t, machine.PidFile(taskDir))
	assert.NoFileExists(t, machine.StopRequestFile(taskDir))
}

func TestControlSnapshotTaskTitleIsValidatedDisplayMetadata(t *testing.T) {
	dir := t.TempDir()
	writeFileCli(t, filepath.Join(dir, "spec.md"), cliE2ESpec)
	r, err := controlSnapshot(dir)
	require.NoError(t, err)
	assert.Equal(t, "CLI e2e", r.TaskTitle)
	assert.Nil(t, r.State, "reading display metadata must not create task state")
	for _, spec := range []string{"broken", strings.Replace(cliE2ESpec, "title: CLI e2e", "title: []", 1)} {
		writeFileCli(t, filepath.Join(dir, "spec.md"), spec)
		r, err = controlSnapshot(dir)
		require.NoError(t, err, "invalid display metadata must not prevent status/stop")
		assert.Empty(t, r.TaskTitle)
	}
	require.NoError(t, os.Remove(filepath.Join(dir, "spec.md")))
	r, err = controlSnapshot(dir)
	require.NoError(t, err)
	assert.Empty(t, r.TaskTitle)
}
