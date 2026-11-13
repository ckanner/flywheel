package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDSHPrewarmProcessFixture(t *testing.T) {
	scenario := os.Getenv("FLYWHEEL_TEST_DSH_WARM")
	if scenario == "" {
		return
	}
	if scenario == "before-prepare" {
		// Test-only startup barrier: the process exists, but has not read the
		// prepare request or installed any receipt-producing shutdown handler.
		if os.WriteFile(os.Getenv("FLYWHEEL_TEST_DSH_WARM_BOOT"), []byte(fmt.Sprint(os.Getpid())), 0o600) != nil {
			os.Exit(2)
		}
		time.Sleep(10 * time.Second)
		os.Exit(2)
	}
	decoder := json.NewDecoder(os.Stdin)
	var raw json.RawMessage
	if decoder.Decode(&raw) != nil {
		os.Exit(2)
	}
	var prepared dshPrepare
	_ = json.Unmarshal(raw, &prepared)
	encode := json.NewEncoder(os.Stdout)
	if prepared.Type == "start" {
		var request dshStart
		_ = json.Unmarshal(raw, &request)
		first, last := dshFixtureFrames(request)
		if request.WorkspaceCwd != "" {
			first["workspaceCwd"] = request.WorkspaceCwd
		}
		_ = os.WriteFile(filepath.Join(request.Cwd, "formal-request.json"), raw, 0o600)
		_ = os.WriteFile(filepath.Join(request.Cwd, "cold-model-call"), []byte(request.RequestID), 0o600)
		if scenario == "admitted-no-ready" {
			calls, err := os.OpenFile(filepath.Join(request.Cwd, "fixture-cold-calls.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
			if err != nil || json.NewEncoder(calls).Encode(request) != nil || calls.Close() != nil {
				os.Exit(2)
			}
		}
		_ = encode.Encode(first)
		if scenario == "expired-cold-missing" {
			os.Exit(1)
		}
		if scenario == "expired-cold-hang" {
			time.Sleep(3 * time.Second)
		}
		_ = encode.Encode(last)
		os.Exit(0)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	closeWarm := func() {
		no := false
		receipt := dshWarmReceipt{Protocol: 1, WarmID: prepared.WarmID, Binding: prepared.Binding, WorkerPID: os.Getpid(),
			Dispatched: &no, CleanupConfirmed: scenario != "unclean"}
		if scenario != "missing-receipt" {
			raw, _ := json.Marshal(receipt)
			_ = os.WriteFile(filepath.Join(prepared.ReceiptDirectory, "terminal.json"), raw, 0o600)
		}
		if scenario == "failed-initialize-no-output" {
			return // receipt survived, but no terminal frame reached stdout
		}
		frame := map[string]any{"protocol": 1, "type": "warm-result", "warmId": prepared.WarmID,
			"binding": prepared.Binding, "dispatched": false, "cleanupConfirmed": receipt.CleanupConfirmed}
		if strings.HasPrefix(scenario, "expired") {
			frame["error"] = map[string]string{"code": "TIMEOUT"}
		} else if strings.HasPrefix(scenario, "failed-initialize") {
			code := "SERVER"
			if selected, exists := os.LookupEnv("FLYWHEEL_TEST_DSH_WARM_CODE"); exists {
				code = selected
			}
			if scenario == "failed-initialize-empty-code" {
				code = ""
			}
			frame["error"] = map[string]string{"code": code, "message": "initialization refused"}
		}
		_ = encode.Encode(frame)
	}
	if scenario == "admitted-no-ready" {
		// This is fixture admission, not SDK readiness. Signal handling and
		// closeWarm are installed before the parent may shorten the deadline.
		admitted, _ := json.Marshal(map[string]any{"warmId": prepared.WarmID, "binding": prepared.Binding, "workerPid": os.Getpid()})
		if os.WriteFile(filepath.Join(prepared.Cwd, "fixture-prepare-admitted.json"), admitted, 0o600) != nil {
			os.Exit(2)
		}
	}
	ready := map[string]any{"protocol": 1, "type": "warm-ready", "warmId": prepared.WarmID,
		"binding": prepared.Binding, "level": "sdk-initialized", "dispatched": false}
	if strings.HasPrefix(scenario, "expired") || scenario == "closed-unused" || strings.HasPrefix(scenario, "failed-initialize") {
		if scenario == "failed-initialize-delayed" {
			var release struct{ Type string }
			if decoder.Decode(&release) != nil || release.Type != "close" {
				os.Exit(2)
			}
		}
		closeWarm()
		if scenario == "failed-initialize-extra-frame" {
			_ = encode.Encode(ready)
		}
		if scenario == "failed-initialize-bad-exit" {
			os.Exit(0)
		}
		if strings.HasSuffix(scenario, "-held") {
			// The real worker emits its terminal frame before its outer process
			// exits. Keep that window open until the test releases owned stdin.
			var release struct{ Type string }
			if decoder.Decode(&release) != nil || release.Type != "close" {
				os.Exit(2)
			}
		}
		if strings.HasPrefix(scenario, "failed-initialize") {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if scenario == "bad-ready" {
		ready["binding"] = "different"
	}
	if scenario == "malformed" {
		fmt.Fprintln(os.Stdout, "{broken-json}")
	} else if scenario == "early-output" {
		_ = encode.Encode(map[string]any{"type": "ready"})
	} else if scenario != "no-ready" && scenario != "admitted-no-ready" {
		_ = encode.Encode(ready)
	}
	if scenario == "duplicate-ready" {
		_ = encode.Encode(ready)
	}
	var control struct {
		Type    string    `json:"type"`
		Request *dshStart `json:"request"`
	}
	read := make(chan error, 1)
	go func() { read <- decoder.Decode(&control) }()
	select {
	case <-signals:
		closeWarm()
		os.Exit(143)
	case err := <-read:
		if err != nil || control.Type == "close" {
			if scenario == "ignore-close" {
				for {
					time.Sleep(time.Second)
				}
			}
			closeWarm()
			os.Exit(0)
		}
	}
	if control.Request == nil {
		os.Exit(2)
	}
	r := *control.Request
	first, last := dshFixtureFrames(r)
	if r.WorkspaceCwd != "" {
		first["workspaceCwd"] = r.WorkspaceCwd
	}
	_ = os.WriteFile(filepath.Join(prepared.Cwd, "formal-request.json"), []byte(dshJSONL(map[string]any{"request": r, "pid": os.Getpid()})), 0o600)
	_ = encode.Encode(first)
	if scenario == "hang" {
		<-signals
		last["status"], last["text"] = "cancelled", ""
		_ = encode.Encode(last)
		os.Exit(143)
	}
	if scenario == "missing-terminal" {
		os.Exit(1)
	}
	_ = encode.Encode(last)
	if scenario == "late-warm-result" {
		closeWarm()
	}
	os.Exit(0)
}

func warmArgs(t *testing.T, scenario string) DSHArgs {
	t.Helper()
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	directory := filepath.Join(cwd, "warm")
	require.NoError(t, os.Mkdir(directory, 0o700))
	return DSHArgs{Command: kimiScript(t, "GORACE=atexit_sleep_ms=0 exec "+shQuote(os.Args[0])+" -test.run '^TestDSHPrewarmProcessFixture$'\n"),
		Cwd: cwd, Provider: "deepseek-account", Model: "deepseek-flash", Prompt: "formal Review only", Thinking: "high", PermissionMode: "read-only",
		PrewarmID: "warm-owned-123", PrewarmDirectory: directory,
		APIKeys: map[string]string{"FLYWHEEL_TEST_DSH_WARM": scenario}, Watchdog: watchdog.Opts{IdleTimeoutMs: 3000, WallTimeoutMs: 5000, KillGraceMs: 1000}}
}

func awaitWarm(t *testing.T, h *DSHPrewarm) {
	t.Helper()
	select {
	case <-h.ready:
	case <-h.done:
		t.Fatalf("warm worker terminated before ready: %+v", h.result)
	case <-time.After(3 * time.Second):
		t.Fatal("warm worker did not report readiness")
	}
}

func TestDSHPrewarmReusesOwnedProcessAndSeparatesFormalTiming(t *testing.T) {
	a := warmArgs(t, "ok")
	a.PersistentSession, a.RequestID, a.SessionID = true, "formal-review", "previous-review"
	a.WorkspaceCwd = a.Cwd
	binding, err := DSHPrewarmBinding(a)
	require.NoError(t, err)
	h := PrepareDSH(a)
	awaitWarm(t, h)
	assert.Equal(t, a.PrewarmID, h.ID())
	assert.True(t, h.Matches(a))
	assert.NoFileExists(t, filepath.Join(a.Cwd, "formal-request.json"))
	started := time.Now()
	time.Sleep(60 * time.Millisecond)
	var events []string
	a.OnEvent = func(line string) { events = append(events, line) }
	a.Prewarm = h
	result := RunDSH(a)
	require.True(t, result.Settled, "%+v", result)
	assert.Equal(t, "formal-review", result.Session.RequestID)
	assert.True(t, result.Session.ResumeAccepted)
	assert.Equal(t, "previous-review", result.SessionID)
	assert.Less(t, result.DurationMS, time.Since(started).Milliseconds()-30)
	assert.Len(t, events, 2, "warm transport is separate from formal invocation events")
	require.NoError(t, h.Close())
	require.NoError(t, h.Close())
	assert.Error(t, RecoverDSHPrewarm(h.ID(), a.PrewarmDirectory, binding), "formal runs do not leave no-dispatch proof")
	assert.False(t, h.Run(a).Settled, "consumed instances cannot run twice")
}

func TestDSHPrewarmDiscardAndRecover(t *testing.T) {
	for _, scenario := range []string{"ok", "no-ready", "unclean", "missing-receipt", "bad-ready", "malformed", "early-output", "duplicate-ready"} {
		t.Run(scenario, func(t *testing.T) {
			a := warmArgs(t, scenario)
			h := PrepareDSH(a)
			if scenario == "ok" {
				awaitWarm(t, h)
			}
			err := h.Close()
			if scenario == "ok" || scenario == "no-ready" {
				require.NoError(t, err)
				require.NoError(t, h.Close())
				require.NoError(t, RecoverDSHPrewarm(h.ID(), a.PrewarmDirectory, h.prepare.Binding))
			} else {
				require.Error(t, err)
			}
			assert.NoFileExists(t, filepath.Join(a.Cwd, "formal-request.json"))
		})
	}
}

func TestDSHPrewarmBindingChangesRejectBeforeModelDispatch(t *testing.T) {
	for _, change := range []func(*DSHArgs){
		func(a *DSHArgs) { a.Model = "different" }, func(a *DSHArgs) { a.Provider = "different" },
		func(a *DSHArgs) { a.PermissionMode = "danger-full-access" }, func(a *DSHArgs) { a.Thinking = "max" },
		func(a *DSHArgs) { a.APIKeys["CREDENTIAL"] = "changed" }, func(a *DSHArgs) { a.Command = "/different" },
		func(a *DSHArgs) { a.Watchdog.WallTimeoutMs++ }, func(a *DSHArgs) { a.WorkspaceCwd = t.TempDir() },
	} {
		a := warmArgs(t, "ok")
		h := PrepareDSH(a)
		awaitWarm(t, h)
		change(&a)
		assert.False(t, h.Matches(a))
		result := h.Run(a)
		assert.False(t, result.Settled)
		assert.Empty(t, result.CleanupError)
		assert.Equal(t, "not-dispatched", result.Session.Dispatch)
		assert.NoFileExists(t, filepath.Join(a.Cwd, "formal-request.json"))
	}
	a := warmArgs(t, "unclean")
	h := PrepareDSH(a)
	a.Model = "different"
	result := h.Run(a)
	assert.NotEmpty(t, result.CleanupError)
	assert.False(t, result.Session.CleanupConfirmed)
}

func TestDSHPrewarmCancelledExpiredAndUnknownResults(t *testing.T) {
	for _, persistent := range []bool{false, true} {
		a := warmArgs(t, "expired")
		a.PersistentSession, a.RequestID = persistent, "formal-review"
		h := PrepareDSH(a)
		<-h.done
		result := h.Run(a)
		require.True(t, result.Settled, "%+v", result)
		assert.Equal(t, "dispatched", result.Session.Dispatch)
		assert.Equal(t, "formal-review", result.Session.RequestID)
		assert.True(t, result.Session.CleanupConfirmed)
		raw, err := os.ReadFile(filepath.Join(a.Cwd, "cold-model-call"))
		require.NoError(t, err)
		assert.Equal(t, "formal-review", string(raw), "safe expiration uses exactly the original invocation")
		require.NoError(t, h.Close())
	}
	for _, scenario := range []string{"missing-terminal", "late-warm-result"} {
		a := warmArgs(t, scenario)
		h := PrepareDSH(a)
		result := h.Run(a)
		assert.False(t, result.Settled)
		assert.NotEmpty(t, result.CleanupError)
		assert.Equal(t, "unknown", result.Session.Outcome)
		assert.Error(t, h.Close())
	}
	for _, idle := range []bool{false, true} {
		a := warmArgs(t, "hang")
		if idle {
			a.Watchdog.IdleTimeoutMs = 100
		} else {
			a.Watchdog.WallTimeoutMs = 100
		}
		h := PrepareDSH(a)
		awaitWarm(t, h)
		result := h.Run(a)
		assert.True(t, result.TimedOut)
		assert.Equal(t, idle, result.IdleTimedOut)
		assert.Equal(t, !idle, result.WallTimedOut)
		require.NoError(t, h.Close())
	}
	a := warmArgs(t, "hang")
	h := PrepareDSH(a)
	awaitWarm(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	a.Watchdog.Context = ctx
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	assert.True(t, h.Run(a).Cancelled)
	require.NoError(t, h.Close())
}

func TestDSHPrewarmNoProcessProofAndInvalidPreparation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		a := warmArgs(t, "ok")
		if cancelled {
			a.Watchdog.IdleTimeoutMs, a.Watchdog.WallTimeoutMs = 0, 0
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			a.Watchdog.Context = ctx
		} else {
			a.Command = filepath.Join(t.TempDir(), "missing-worker")
		}
		h := PrepareDSH(a)
		require.NoError(t, h.Close())
		receipt, err := readDSHWarmReceipt(h.ID(), a.PrewarmDirectory, h.prepare.Binding)
		require.NoError(t, err)
		assert.True(t, receipt.NoProcess)
		assert.Zero(t, receipt.WorkerPID)
	}
	for _, change := range []func(*DSHArgs){func(a *DSHArgs) { a.Doctor = true }, func(a *DSHArgs) { a.Cwd = "/missing-cwd" },
		func(a *DSHArgs) { a.PrewarmID = "" }, func(a *DSHArgs) { a.PrewarmDirectory = "/missing-directory" }} {
		a := warmArgs(t, "ok")
		change(&a)
		h := PrepareDSH(a)
		assert.Error(t, h.Close())
		assert.False(t, h.Run(a).Settled)
	}
}

func TestDSHPrewarmStrictReceiptAndPublication(t *testing.T) {
	a := warmArgs(t, "ok")
	binding, err := DSHPrewarmBinding(a)
	require.NoError(t, err)
	require.NoError(t, RecordUnstartedDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
	require.NoError(t, RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
	assert.Error(t, RecordUnstartedDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding), "evidence cannot be replaced")
	path := filepath.Join(a.PrewarmDirectory, "terminal.json")
	valid, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, data := range []string{"{broken", string(valid) + "{}", strings.Replace(string(valid), `"dispatched":false`, `"dispatched":true`, 1),
		strings.Replace(string(valid), `"cleanupConfirmed":true`, `"cleanupConfirmed":false`, 1),
		strings.Replace(string(valid), `"protocol":1`, `"protocol":2`, 1),
		strings.Replace(string(valid), `"noProcess":true`, `"noProcess":false`, 1),
		strings.Replace(string(valid), `"workerPid":0`, fmt.Sprintf(`"workerPid":%d`, os.Getpid()), 1),
		strings.Replace(string(valid), `"dispatched":false,`, "", 1),
		strings.Replace(string(valid), `"protocol":1`, `"extra":1,"protocol":1`, 1)} {
		require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
		assert.Error(t, RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding), data)
	}
	var receipt map[string]any
	require.NoError(t, json.Unmarshal(valid, &receipt))
	receipt["noProcess"], receipt["workerPid"] = false, os.Getpid()
	raw, _ := json.Marshal(receipt)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	assert.ErrorContains(t, RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding), "physically exited")
	require.NoError(t, os.WriteFile(path, valid, 0o600))
	assert.Error(t, RecoverDSHPrewarm("bad", a.PrewarmDirectory, binding))
	assert.Error(t, RecordUnstartedDSHPrewarm("bad", a.PrewarmDirectory, binding))
	assert.Error(t, RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, "bad"))
	assert.Error(t, RecordUnstartedDSHPrewarm(a.PrewarmID, "/missing-dir", binding))
	require.NoError(t, os.Chmod(path, 0o644))
	assert.Error(t, RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
	require.NoError(t, os.Chmod(path, 0o600))
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", 8193)), 0o600))
	assert.Error(t, RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Symlink(filepath.Join(a.Cwd, "outside"), path))
	assert.Error(t, RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
	require.NoError(t, os.Remove(path))
	require.NoError(t, syscall.Mkfifo(path, 0o600))
	assert.Error(t, RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
	require.NoError(t, os.Chmod(a.PrewarmDirectory, 0o755))
	assert.Error(t, RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
}

type failingWarmWriter struct{}

func (failingWarmWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (failingWarmWriter) Close() error              { return nil }

func TestDSHPrewarmWriteFailuresAndIOEvidenceFailures(t *testing.T) {
	a := warmArgs(t, "ok")
	h := PrepareDSH(a)
	awaitWarm(t, h)
	stopped := false
	h.writeFailure(io.ErrClosedPipe, func() { stopped = true })
	assert.True(t, stopped)
	assert.Error(t, h.Close())
	binding, err := DSHPrewarmBinding(a)
	require.NoError(t, err)
	originalStat, originalFileStat, originalCreate, originalSync := dshWarmStat, dshWarmFileStat, dshWarmCreate, dshWarmSync
	t.Cleanup(func() {
		dshWarmStat, dshWarmFileStat, dshWarmCreate, dshWarmSync = originalStat, originalFileStat, originalCreate, originalSync
	})
	dshWarmStat = func(string) (os.FileInfo, error) { return nil, os.ErrPermission }
	assert.Error(t, RecordUnstartedDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
	dshWarmStat = originalStat
	dshWarmFileStat = func(*os.File) (os.FileInfo, error) { return nil, os.ErrPermission }
	assert.Error(t, RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
	dshWarmFileStat = originalFileStat
	dshWarmCreate = func(string, string) (*os.File, error) { return nil, os.ErrPermission }
	assert.Error(t, RecordUnstartedDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
	dshWarmCreate = func(dir, pattern string) (*os.File, error) {
		file, err := originalCreate(dir, pattern)
		require.NoError(t, err)
		require.NoError(t, file.Close())
		return file, nil
	}
	assert.Error(t, RecordUnstartedDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
	dshWarmCreate = originalCreate
	dshWarmSync = func(*os.File) error { return os.ErrInvalid }
	assert.Error(t, RecordUnstartedDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
}

type controlledWarmWriter struct {
	writes int
	failAt int
	closed bool
}

func (w *controlledWarmWriter) Write(data []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, io.ErrClosedPipe
	}
	return len(data), nil
}
func (w *controlledWarmWriter) Close() error { w.closed = true; return nil }

func TestDSHPrewarmTransportCancellationBeforeAndAfterAdmission(t *testing.T) {
	for _, stage := range []string{"before-context", "before-exit", "ready-context", "ready-exit", "ready-expired", "first-write", "second-write"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			h := &DSHPrewarm{ctx: ctx, cancel: cancel, done: make(chan struct{}), processDone: make(chan struct{}), inputDone: make(chan struct{}), ready: make(chan struct{}), controls: make(chan *dshStart)}
			h.prepareTimer = time.NewTimer(time.Hour)
			defer h.prepareTimer.Stop()
			writer := &controlledWarmWriter{}
			if stage == "first-write" {
				writer.failAt = 1
			}
			if stage == "before-context" {
				cancel()
			} else if stage == "before-exit" {
				close(h.processDone)
			}
			stopped := false
			finished := make(chan struct{})
			go func() { h.interact(writer, func() { stopped = true }); close(finished) }()
			if strings.HasPrefix(stage, "ready-") || stage == "second-write" {
				h.controls <- &dshStart{Type: "start"}
				switch stage {
				case "ready-context":
					cancel()
				case "ready-exit":
					close(h.processDone)
				case "ready-expired":
					h.mu.Lock()
					h.prepareExpired = true
					h.mu.Unlock()
					close(h.ready)
				case "second-write":
					writer.failAt = 2
					close(h.ready)
				}
			}
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("transport did not stop at the cancelled admission boundary")
			}
			assert.True(t, writer.closed)
			assert.Equal(t, stage == "first-write" || stage == "second-write", stopped)
			if stopped {
				assert.ErrorIs(t, h.writeError, io.ErrClosedPipe)
			}
		})
	}
}

func TestDSHPrewarmQueuedTTLDoesNotCancelFormalInvocation(t *testing.T) {
	a := warmArgs(t, "hang")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.Watchdog.Context = ctx
	h := PrepareDSH(a)
	awaitWarm(t, h)
	finished := make(chan Result, 1)
	go func() { finished <- h.Run(a) }()
	require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(a.Cwd, "formal-request.json")); return err == nil }, time.Second, 5*time.Millisecond)
	h.expirePrepare() // A timer callback may already be queued when Stop wins ownership.
	h.mu.Lock()
	expired := h.prepareExpired
	h.mu.Unlock()
	assert.False(t, expired)
	assert.NoError(t, h.ctx.Err(), "the formal call uses its own deadlines after handoff")
	cancel()
	assert.True(t, (<-finished).Cancelled)
	require.NoError(t, h.Close())
}

func TestDSHPrewarmQueuedIdleExpiryDoesNotCancelRecentActivity(t *testing.T) {
	a := warmArgs(t, "hang")
	a.Watchdog.IdleTimeoutMs = 300
	h := PrepareDSH(a)
	t.Cleanup(func() { _ = h.Close() })
	awaitWarm(t, h)
	finished := make(chan Result, 1)
	go func() { finished <- h.Run(a) }()
	require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(a.Cwd, "formal-request.json")); return err == nil }, time.Second, 5*time.Millisecond)
	// Reset can win the mutex after an old AfterFunc callback was queued. That
	// callback still runs, but must use the deadline established by new output.
	h.activity("new worker progress", "stderr")
	h.expire(true)
	h.mu.Lock()
	timedOut := h.idleTimedOut
	h.mu.Unlock()
	assert.False(t, timedOut, "a queued callback cannot expire the refreshed idle window")
	assert.NoError(t, h.ctx.Err(), "recent activity keeps the formal Review alive")
	select {
	case result := <-finished:
		assert.True(t, result.TimedOut, "the genuine new idle deadline remains enforced")
		assert.True(t, result.IdleTimedOut)
		assert.False(t, result.WallTimedOut)
	case <-time.After(3 * time.Second):
		t.Fatal("the refreshed idle deadline did not stop an actually idle Review")
	}
	require.NoError(t, h.Close())
}

func TestDSHPrewarmQueuedTTLDoesNotReclassifyFailedPreparation(t *testing.T) {
	for _, scenario := range []string{"failed-initialize", "failed-initialize-no-output"} {
		t.Run(scenario, func(t *testing.T) {
			a := warmArgs(t, scenario)
			a.RequestID = "failed-initialization-review"
			h := PrepareDSH(a)
			t.Cleanup(func() { _ = h.Close() })
			<-h.done
			// Stop cannot retract a callback already queued before process exit.
			// Neither a known initialization failure nor a missing terminal
			// frame can supply permission for a cold call after physical exit.
			assert.Equal(t, scenario == "failed-initialize", h.warmTerminal)
			h.expirePrepare()
			h.mu.Lock()
			expired := h.prepareExpired
			h.mu.Unlock()
			assert.False(t, expired)
			result := h.Run(a)
			assert.False(t, result.Settled)
			assert.Equal(t, "not-dispatched", result.Session.Dispatch)
			assert.True(t, result.Session.CleanupConfirmed)
			assert.NoFileExists(t, filepath.Join(a.Cwd, "cold-model-call"))
			require.NoError(t, h.Close())
		})
	}
}

func TestDSHPrewarmInitializationFailureRetainsStructuredRetryPolicy(t *testing.T) {
	for _, tc := range []struct {
		code      errs.ErrorCode
		retryable bool
		fallback  bool
	}{
		{errs.INVALID_REQUEST, false, false},
		{errs.AUTH, false, false},
		{errs.QUOTA, false, true},
		{errs.SERVER, true, false},
		{errs.RATE_LIMIT, true, true},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			a := warmArgs(t, "failed-initialize")
			a.RequestID = "failed-initialization-review"
			a.APIKeys["FLYWHEEL_TEST_DSH_WARM_CODE"] = string(tc.code)
			h := PrepareDSH(a)
			t.Cleanup(func() { _ = h.Close() })
			<-h.done
			result := h.Run(a)
			assert.False(t, result.Settled)
			assert.Equal(t, tc.code, result.ErrorCode)
			require.NotNil(t, result.ErrorMessage)
			assert.Equal(t, string(tc.code)+": initialization refused", *result.ErrorMessage)
			code := errs.ClassifyError(errs.SpawnResult{ExitCode: *result.ExitCode, ErrorCode: result.ErrorCode})
			assert.Equal(t, tc.retryable, errs.IsRetryable(code))
			assert.Equal(t, tc.fallback, errs.ShouldFallback(code))
			require.NotNil(t, result.Session)
			assert.Equal(t, "not-dispatched", result.Session.Dispatch)
			assert.True(t, result.Session.CleanupConfirmed)
			assert.Equal(t, "complete", result.Session.UsageStatus)
			assert.Equal(t, &Usage{}, result.Usage)
			assert.NoFileExists(t, filepath.Join(a.Cwd, "cold-model-call"))
			require.NoError(t, h.Close())
		})
	}
}

func TestDSHPrewarmInitializationFailureAfterLocalTTLDoesNotAuthorizeColdCall(t *testing.T) {
	for _, code := range []errs.ErrorCode{errs.AUTH, errs.SERVER} {
		t.Run(string(code), func(t *testing.T) {
			a := warmArgs(t, "failed-initialize-delayed")
			a.RequestID = "failure-after-local-expiry"
			a.APIKeys["FLYWHEEL_TEST_DSH_WARM_CODE"] = string(code)
			h := PrepareDSH(a)
			t.Cleanup(func() { _ = h.Close() })
			// Initialization can fail before the worker's own deadline, with its
			// terminal frame arriving after Go's TTL while SDK shutdown awaits.
			h.expirePrepare()
			h.controls <- nil
			<-h.done
			result := h.Run(a)
			assert.False(t, result.Settled)
			assert.Equal(t, code, result.ErrorCode)
			require.NotNil(t, result.Session)
			assert.Equal(t, "not-dispatched", result.Session.Dispatch)
			assert.True(t, result.Session.CleanupConfirmed)
			assert.NoFileExists(t, filepath.Join(a.Cwd, "cold-model-call"))
			require.NoError(t, h.Close())
		})
	}
}

func TestDSHPrewarmInitializationFailureRequiresIntactErrorProof(t *testing.T) {
	for _, tc := range []struct {
		name, scenario, code string
		cleanup              bool
	}{
		{"unknown-code", "failed-initialize", "UNRECOGNIZED", false},
		{"empty-code", "failed-initialize-empty-code", "", false},
		{"extra-frame", "failed-initialize-extra-frame", "AUTH", false},
		{"wrong-exit", "failed-initialize-bad-exit", "AUTH", true},
		{"missing-frame", "failed-initialize-no-output", "AUTH", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := warmArgs(t, tc.scenario)
			a.RequestID = "untrusted-initialization-review"
			a.APIKeys["FLYWHEEL_TEST_DSH_WARM_CODE"] = tc.code
			h := PrepareDSH(a)
			t.Cleanup(func() { _ = h.Close() })
			<-h.done
			result := h.Run(a)
			assert.False(t, result.Settled)
			assert.Equal(t, errs.TRANSPORT, result.ErrorCode)
			require.NotNil(t, result.Session)
			assert.Equal(t, tc.cleanup, result.Session.CleanupConfirmed)
			if tc.cleanup {
				assert.Equal(t, "not-dispatched", result.Session.Dispatch)
				assert.Empty(t, result.CleanupError)
				require.NoError(t, h.Close())
			} else {
				assert.Equal(t, "unknown", result.Session.Dispatch)
				assert.NotEmpty(t, result.CleanupError)
				assert.Error(t, h.Close())
			}
			assert.NoFileExists(t, filepath.Join(a.Cwd, "cold-model-call"))
		})
	}
}

func TestDSHPrewarmInitializationFailureDoesNotOverrideCancellationOrTimeout(t *testing.T) {
	for _, cancelCall := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancelled=%t", cancelCall), func(t *testing.T) {
			a := warmArgs(t, "failed-initialize-held")
			a.RequestID = "cancelled-initialization-review"
			a.APIKeys["FLYWHEEL_TEST_DSH_WARM_CODE"] = string(errs.AUTH)
			a.Watchdog.IdleTimeoutMs = 30
			a.Watchdog.KillGraceMs = 30
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a.Watchdog.Context = ctx
			h := PrepareDSH(a)
			t.Cleanup(func() { _ = h.Close() })
			require.Eventually(t, func() bool {
				h.mu.Lock()
				defer h.mu.Unlock()
				return h.warmTerminal
			}, time.Second, 5*time.Millisecond)
			if cancelCall {
				cancel()
			}
			result := h.Run(a)
			assert.False(t, result.Settled)
			assert.Equal(t, cancelCall, result.Cancelled)
			assert.Equal(t, !cancelCall, result.TimedOut)
			assert.NotEqual(t, errs.AUTH, result.ErrorCode)
			require.NotNil(t, result.Session)
			assert.Equal(t, "not-dispatched", result.Session.Dispatch)
			assert.True(t, result.Session.CleanupConfirmed)
			assert.NoFileExists(t, filepath.Join(a.Cwd, "cold-model-call"))
			require.NoError(t, h.Close())
		})
	}
}

func TestDSHPrewarmQueuedTTLRespectsTerminalBeforeProcessExit(t *testing.T) {
	for _, tc := range []struct {
		scenario string
		expired  bool
	}{
		{scenario: "failed-initialize-held", expired: false},
		{scenario: "expired-held", expired: true},
	} {
		t.Run(tc.scenario, func(t *testing.T) {
			a := warmArgs(t, tc.scenario)
			a.RequestID = "terminal-before-exit-review"
			h := PrepareDSH(a)
			t.Cleanup(func() { _ = h.Close() })
			require.Eventually(t, func() bool {
				h.mu.Lock()
				defer h.mu.Unlock()
				return h.warmTerminal
			}, time.Second, 5*time.Millisecond)
			select {
			case <-h.processDone:
				t.Fatal("fixture must retain the terminal-to-exit window")
			default:
			}

			// A queued Go TTL cannot replace a terminal SERVER result while
			// physical exit is still pending. A real TIMEOUT remains expired.
			h.expirePrepare()
			h.mu.Lock()
			expired := h.prepareExpired
			h.mu.Unlock()
			assert.Equal(t, tc.expired, expired)
			h.controls <- nil // release the fixture without dispatching a prompt
			<-h.done
			result := h.Run(a)
			if tc.expired {
				require.True(t, result.Settled, "%+v", result)
				require.Equal(t, a.RequestID, result.Session.RequestID)
				raw, err := os.ReadFile(filepath.Join(a.Cwd, "cold-model-call"))
				require.NoError(t, err)
				assert.Equal(t, a.RequestID, string(raw))
			} else {
				assert.False(t, result.Settled, "a known initialization failure cannot become cold model work")
				require.NotNil(t, result.Session)
				assert.Equal(t, "not-dispatched", result.Session.Dispatch)
				assert.True(t, result.Session.CleanupConfirmed)
				assert.NoFileExists(t, filepath.Join(a.Cwd, "cold-model-call"))
			}
			require.NoError(t, h.Close())
		})
	}
}

func TestDSHPrewarmTerminalBeforeExitStillHasBoundedCleanup(t *testing.T) {
	originalGrace := dshWarmCloseGrace
	dshWarmCloseGrace = 25 * time.Millisecond
	t.Cleanup(func() { dshWarmCloseGrace = originalGrace })
	a := warmArgs(t, "failed-initialize-held")
	a.RequestID = "terminal-cleanup-review"
	a.Watchdog.KillGraceMs = 25
	h := PrepareDSH(a)
	t.Cleanup(func() { _ = h.Close() })
	require.Eventually(t, func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.warmTerminal
	}, time.Second, 5*time.Millisecond)
	select {
	case <-h.processDone:
		t.Fatal("fixture must still own a live process after its terminal frame")
	default:
	}

	// Knowing the terminal outcome retires classification, not ownership of
	// a worker that never exits. Do not release its stdin: TTL must clean up.
	h.expirePrepare()
	select {
	case <-h.done:
	case <-time.After(time.Second):
		t.Fatal("terminal output must not disable bounded prewarm process cleanup")
	}
	h.mu.Lock()
	expired := h.prepareExpired
	h.mu.Unlock()
	assert.False(t, expired, "forced cleanup cannot turn SERVER into safe expiration")
	result := h.Run(a)
	assert.False(t, result.Settled)
	require.NotNil(t, result.Session)
	assert.Equal(t, "not-dispatched", result.Session.Dispatch)
	assert.True(t, result.Session.CleanupConfirmed)
	assert.NoFileExists(t, filepath.Join(a.Cwd, "cold-model-call"))
	require.NoError(t, h.Close())
}

func TestDSHPrewarmQueuedInvocationTimeoutsIgnoreExitedProcess(t *testing.T) {
	for _, idle := range []bool{false, true} {
		t.Run(fmt.Sprintf("idle=%t", idle), func(t *testing.T) {
			a := warmArgs(t, "ok")
			h := PrepareDSH(a)
			t.Cleanup(func() { _ = h.Close() })
			result := h.Run(a)
			require.True(t, result.Settled, "%+v", result)
			// A callback queued before Stop may run after physical exit, even
			// before Run extracts the result. Neither deadline owns it anymore.
			h.mu.Lock()
			h.lastActivity = time.Now().Add(-time.Hour)
			h.mu.Unlock()
			h.expire(idle)
			h.mu.Lock()
			idleTimedOut, wallTimedOut := h.idleTimedOut, h.wallTimedOut
			h.mu.Unlock()
			assert.False(t, idleTimedOut)
			assert.False(t, wallTimedOut)
			assert.NoError(t, h.ctx.Err(), "an exited process cannot become timed out")
			require.NoError(t, h.Close())
		})
	}
}

func TestDSHPrewarmLocalTTLAndSafeColdFallbackBoundaries(t *testing.T) {
	originalClose := dshWarmCloseGrace
	t.Cleanup(func() { dshWarmCloseGrace = originalClose })
	dshWarmCloseGrace = 25 * time.Millisecond
	a := warmArgs(t, "admitted-no-ready")
	a.RequestID = "same-formal-invocation"
	h := PrepareDSH(a)
	t.Cleanup(func() { _ = h.Close() })
	var admitted struct {
		WarmID    string `json:"warmId"`
		Binding   string `json:"binding"`
		WorkerPID int    `json:"workerPid"`
	}
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(filepath.Join(a.Cwd, "fixture-prepare-admitted.json"))
		return err == nil && json.Unmarshal(raw, &admitted) == nil
	}, 5*time.Second, 5*time.Millisecond, "fixture must accept prepare and install cleanup before expiry")
	assert.Equal(t, h.ID(), admitted.WarmID)
	assert.Equal(t, h.prepare.Binding, admitted.Binding)
	assert.Positive(t, admitted.WorkerPID)
	select {
	case <-h.ready:
		t.Fatal("fixture admission must not be reported as SDK readiness")
	default:
	}
	finished := make(chan Result, 1)
	go func() { finished <- h.Run(a) }()
	require.Eventually(t, func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.formal != nil
	}, 5*time.Second, 5*time.Millisecond)
	// Exercise the real timer callback, but only after startup and admission.
	// The formal invocation has already started, so expiry/cleanup must spend
	// its original time budget rather than giving the cold call a fresh one.
	h.mu.Lock()
	stopped := h.prepareTimer.Stop()
	if stopped {
		h.prepareTimer.Reset(75 * time.Millisecond)
	}
	h.mu.Unlock()
	require.True(t, stopped, "the original preparation deadline must not race the fixture barrier")
	var result Result
	select {
	case result = <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("expired preparation and its cold replacement must finish within the invocation budget")
	}
	assert.True(t, h.prepareExpired, "Go independently bounds an unresponsive prepare")
	assert.False(t, h.readySeen, "cleanup after fixture admission never authorizes a warm start")
	receipt, err := readDSHWarmReceipt(h.ID(), a.PrewarmDirectory, h.prepare.Binding)
	require.NoError(t, err)
	assert.Equal(t, admitted.WorkerPID, receipt.WorkerPID)
	assert.False(t, receipt.NoProcess)
	assert.False(t, *receipt.Dispatched)
	assert.True(t, receipt.CleanupConfirmed)
	require.NoError(t, RecoverDSHPrewarm(h.ID(), a.PrewarmDirectory, h.prepare.Binding))
	require.True(t, result.Settled, "%+v", result)
	require.NotNil(t, result.Session)
	assert.Equal(t, a.RequestID, result.Session.RequestID)
	assert.False(t, h.Run(a).Settled, "the same preparation cannot authorize another cold invocation")
	raw, err := os.ReadFile(filepath.Join(a.Cwd, "fixture-cold-calls.jsonl"))
	require.NoError(t, err)
	calls := strings.Split(strings.TrimSpace(string(raw)), "\n")
	require.Len(t, calls, 1, "safe expiry authorizes exactly one cold call")
	var request dshStart
	require.NoError(t, json.Unmarshal([]byte(calls[0]), &request))
	assert.Equal(t, a.RequestID, request.RequestID)
	assert.Positive(t, request.TimeoutMS)
	assert.Less(t, request.TimeoutMS, a.Watchdog.WallTimeoutMs, "cold fallback receives only the remaining formal deadline")
	require.NoError(t, h.Close())
	dshWarmCloseGrace = originalClose

	a = warmArgs(t, "closed-unused")
	h = PrepareDSH(a)
	<-h.done
	result = h.Run(a)
	assert.False(t, result.Settled, "an initialization failure is not silently replaced by cold work")
	assert.Equal(t, "not-dispatched", result.Session.Dispatch)
	assert.NoFileExists(t, filepath.Join(a.Cwd, "cold-model-call"))
	require.NoError(t, h.Close())

	a = warmArgs(t, "expired")
	ctx, cancel := context.WithCancel(context.Background())
	a.Watchdog.Context = ctx
	h = PrepareDSH(a)
	<-h.done
	cancel()
	result = h.Run(a)
	assert.True(t, result.Cancelled)
	assert.NoFileExists(t, filepath.Join(a.Cwd, "cold-model-call"))
	require.NoError(t, h.Close())

	a = warmArgs(t, "expired")
	h = PrepareDSH(a)
	<-h.done
	result = h.coldAfterExpiry(a, time.Now().Add(-time.Hour), Result{})
	assert.True(t, result.TimedOut)
	assert.True(t, result.WallTimedOut)
	assert.NoFileExists(t, filepath.Join(a.Cwd, "cold-model-call"))
	require.NoError(t, h.Close())

	a = warmArgs(t, "expired-cold-missing")
	h = PrepareDSH(a)
	<-h.done
	result = h.Run(a)
	assert.False(t, result.Settled)
	assert.NotEmpty(t, result.CleanupError)
	assert.ErrorContains(t, h.Close(), "cold invocation")

	dshWarmCloseGrace = 25 * time.Millisecond
	a = warmArgs(t, "ignore-close")
	a.Watchdog.WallTimeoutMs, a.Watchdog.KillGraceMs = 600000, 50
	h = PrepareDSH(a)
	awaitWarm(t, h)
	started := time.Now()
	assert.Error(t, h.Close())
	assert.Less(t, time.Since(started), time.Second, "discard cannot wait for the model's wall-time allowance")
}

func TestDSHPrewarmDeadlineBeforeAdmissionDoesNotAuthorizeColdFallback(t *testing.T) {
	originalClose := dshWarmCloseGrace
	dshWarmCloseGrace = 25 * time.Millisecond
	t.Cleanup(func() { dshWarmCloseGrace = originalClose })
	a := warmArgs(t, "before-prepare")
	a.RequestID = "unadmitted-formal-invocation"
	boot := filepath.Join(a.Cwd, "fixture-before-prepare")
	a.APIKeys["FLYWHEEL_TEST_DSH_WARM_BOOT"] = boot
	h := PrepareDSH(a)
	t.Cleanup(func() { _ = h.Close() })
	var pid int
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(boot)
		if err != nil {
			return false
		}
		_, err = fmt.Sscan(string(raw), &pid)
		return err == nil && pid > 0
	}, 5*time.Second, 5*time.Millisecond, "must observe a live child before testing unknown admission")
	require.NoError(t, syscall.Kill(pid, 0))
	h.mu.Lock()
	stopped := h.prepareTimer.Stop()
	if stopped {
		h.prepareTimer.Reset(75 * time.Millisecond)
	}
	h.mu.Unlock()
	require.True(t, stopped)
	select {
	case <-h.done:
	case <-time.After(3 * time.Second):
		t.Fatal("a real preparation deadline must stop a child that never accepts prepare")
	}
	assert.True(t, h.prepareExpired)
	assert.False(t, h.readySeen)
	assert.ErrorIs(t, syscall.Kill(pid, 0), syscall.ESRCH)
	assert.ErrorIs(t, syscall.Kill(-pid, 0), syscall.ESRCH)
	assert.NoFileExists(t, filepath.Join(a.PrewarmDirectory, "terminal.json"), "a started child is not no-process proof")
	assert.Error(t, RecoverDSHPrewarm(h.ID(), a.PrewarmDirectory, h.prepare.Binding))
	result := h.Run(a)
	assert.False(t, result.Settled)
	assert.NotEmpty(t, result.CleanupError)
	require.NotNil(t, result.Session)
	assert.False(t, result.Session.CleanupConfirmed)
	assert.Equal(t, "unknown", result.Session.Outcome)
	assert.NoFileExists(t, filepath.Join(a.Cwd, "cold-model-call"))
	assert.NoFileExists(t, filepath.Join(a.Cwd, "formal-request.json"))
	assert.Error(t, h.Close(), "physical exit alone cannot replace the missing SDK cleanup receipt")
}

func TestDSHPrewarmCloseWaitsForColdFallbackCleanup(t *testing.T) {
	a := warmArgs(t, "expired-cold-hang")
	a.Watchdog.KillGraceMs = 100
	h := PrepareDSH(a)
	<-h.done
	finished := make(chan Result, 1)
	go func() { finished <- h.Run(a) }()
	require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(a.Cwd, "cold-model-call")); return err == nil }, time.Second, 5*time.Millisecond)
	started := time.Now()
	assert.Error(t, h.Close(), "a killed cold invocation cannot borrow an old unused receipt")
	result := <-finished
	assert.True(t, result.Cancelled)
	assert.NotEmpty(t, result.CleanupError)
	assert.Less(t, time.Since(started), time.Second)
}

func TestDSHPrewarmEvidenceDurabilityAndGroupRecovery(t *testing.T) {
	a := warmArgs(t, "ok")
	binding, err := DSHPrewarmBinding(a)
	require.NoError(t, err)
	require.NoError(t, RecordUnstartedDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
	assert.Error(t, PrepareDSH(a).Close(), "an existing receipt directory cannot be reused")
	originalSync, originalOpen, originalProbe := dshWarmSync, dshWarmOpenDirectory, dshWarmProbe
	t.Cleanup(func() { dshWarmSync, dshWarmOpenDirectory, dshWarmProbe = originalSync, originalOpen, originalProbe })
	dshWarmSync = func(*os.File) error { return os.ErrPermission }
	assert.Error(t, RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding), "readable evidence still requires durability confirmation")
	dshWarmSync = func(file *os.File) error {
		if file.Name() == a.PrewarmDirectory {
			return os.ErrPermission
		}
		return originalSync(file)
	}
	assert.Error(t, RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
	dshWarmSync = originalSync
	dshWarmOpenDirectory = func(string) (*os.File, error) { return nil, os.ErrNotExist }
	assert.Error(t, RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding))
	dshWarmOpenDirectory = originalOpen
	path := filepath.Join(a.PrewarmDirectory, "terminal.json")
	no := false
	receipt := dshWarmReceipt{Protocol: 1, WarmID: a.PrewarmID, Binding: binding, WorkerPID: 999999,
		Dispatched: &no, CleanupConfirmed: true}
	raw, err := json.Marshal(receipt)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	for _, groupStatus := range []error{nil, syscall.EPERM, syscall.EIO, syscall.ESRCH} {
		var probes []int
		dshWarmProbe = func(pid int, signal syscall.Signal) error {
			probes = append(probes, pid)
			assert.Zero(t, signal, "recovery only probes, never kills")
			if pid < 0 {
				return groupStatus
			}
			return syscall.ESRCH
		}
		err := RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding)
		assert.Equal(t, groupStatus != syscall.ESRCH, err != nil)
		assert.Equal(t, []int{999999, -999999}, probes, "a dead leader alone cannot prove group quiescence")
	}
}

func TestDSHPrewarmRecoveryDoesNotIgnoreSurvivingProcessGroup(t *testing.T) {
	a := warmArgs(t, "ok")
	binding, err := DSHPrewarmBinding(a)
	require.NoError(t, err)
	child := exec.Command("/bin/sh", "-c", "sleep 4 >/dev/null 2>&1 &")
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, child.Start())
	pid := child.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
	require.NoError(t, child.Wait())
	require.ErrorIs(t, syscall.Kill(pid, 0), syscall.ESRCH)
	require.NoError(t, syscall.Kill(-pid, 0), "the detached-stdio descendant still belongs to the original group")
	no := false
	receipt := dshWarmReceipt{Protocol: 1, WarmID: a.PrewarmID, Binding: binding, WorkerPID: pid, Dispatched: &no, CleanupConfirmed: true}
	raw, err := json.Marshal(receipt)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(a.PrewarmDirectory, "terminal.json"), raw, 0o600))
	assert.ErrorContains(t, RecoverDSHPrewarm(a.PrewarmID, a.PrewarmDirectory, binding), "process group")
	require.NoError(t, syscall.Kill(-pid, 0), "read-only recovery must not kill a potentially reused group")
}
