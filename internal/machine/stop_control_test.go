package machine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunControlTokensAndAtomicStopRequests(t *testing.T) {
	dir := t.TempDir()
	c := &Context{TaskDir: dir, Now: time.Now}
	require.NoError(t, c.writePid())
	first, err := ReadRunControl(dir)
	require.NoError(t, err)
	assert.Equal(t, os.Getpid(), first.PID)
	assert.NotEmpty(t, first.ControlToken)
	require.NoError(t, c.writePid())
	second, err := ReadRunControl(dir)
	require.NoError(t, err)
	assert.NotEqual(t, first.ControlToken, second.ControlToken)
	require.NoError(t, WriteStopRequest(dir, second.ControlToken))
	raw, err := os.ReadFile(StopRequestFile(dir))
	require.NoError(t, err)
	var request stopRequest
	require.NoError(t, json.Unmarshal(raw, &request))
	assert.Equal(t, 1, request.Protocol)
	assert.Equal(t, second.ControlToken, request.ControlToken)
	assert.Error(t, WriteStopRequest(dir, " "))
	file := filepath.Join(dir, "not-a-directory")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	assert.Error(t, WriteStopRequest(file, "token"))
	oldCreate := createStopTemp
	createStopTemp = func(string, string) (*os.File, error) { return nil, os.ErrPermission }
	assert.ErrorIs(t, WriteStopRequest(dir, "replacement"), os.ErrPermission)
	createStopTemp = oldCreate
	oldWrite := writeStopData
	writeStopData = func(string, []byte, os.FileMode) error { return os.ErrPermission }
	t.Cleanup(func() { writeStopData = oldWrite })
	assert.ErrorIs(t, WriteStopRequest(dir, "replacement"), os.ErrPermission)
	after, err := os.ReadFile(StopRequestFile(dir))
	require.NoError(t, err)
	assert.Equal(t, raw, after, "a failed write cannot replace the previous atomic request")
	writeStopData = oldWrite
	require.NoError(t, os.Remove(StopRequestFile(dir)))
	require.NoError(t, os.Mkdir(StopRequestFile(dir), 0o755))
	assert.Error(t, WriteStopRequest(dir, "replacement"))

	clearPid(dir)
	_, err = ReadRunControl(dir)
	assert.Error(t, err)
	for _, raw := range []string{`{broken`, `{"pid":42}`, `{"pid":0,"control-token":"x"}`} {
		require.NoError(t, os.WriteFile(PidFile(dir), []byte(raw), 0o600))
		_, err := ReadRunControl(dir)
		assert.Error(t, err)
	}
}

func TestRunLeaseHeldIsReadOnly(t *testing.T) {
	dir := t.TempDir()
	held, err := RunLeaseHeld(dir)
	require.NoError(t, err)
	assert.False(t, held)
	assert.NoFileExists(t, filepath.Join(dir, ".run.lock"))
	release, err := AcquireRunLease(dir)
	require.NoError(t, err)
	held, err = RunLeaseHeld(dir)
	require.NoError(t, err)
	assert.True(t, held)
	release()
	held, err = RunLeaseHeld(dir)
	require.NoError(t, err)
	assert.False(t, held)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	_, err = RunLeaseHeld(file)
	assert.Error(t, err)
	original := controlFileFlock
	controlFileFlock = func(int, int) error { return syscall.EPERM }
	t.Cleanup(func() { controlFileFlock = original })
	_, err = RunLeaseHeld(dir)
	assert.ErrorIs(t, err, syscall.EPERM)
}

func TestStopWatcherIgnoresStaleAndMalformedRequests(t *testing.T) {
	old := stopRequestPollInterval
	stopRequestPollInterval = 2 * time.Millisecond
	t.Cleanup(func() { stopRequestPollInterval = old })
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := watchStopRequests(ctx, dir, "current", cancel)
	defer stop()
	time.Sleep(8 * time.Millisecond) // absent request is normal
	for _, raw := range []string{`{broken`, `{"protocol":1,"control-token":"stale"}`, `{"protocol":2,"control-token":"current"}`} {
		require.NoError(t, os.WriteFile(StopRequestFile(dir), []byte(raw), 0o600))
		time.Sleep(8 * time.Millisecond)
		assert.NoError(t, ctx.Err())
	}
	require.NoError(t, WriteStopRequest(dir, "current"))
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("matching stop request did not cancel run")
	}
	other, otherCancel := context.WithCancel(context.Background())
	defer otherCancel()
	stopOther := watchStopRequests(other, dir, "other", otherCancel)
	stopOther()
	assert.NoError(t, other.Err(), "stopping a watcher must not cancel a completed run")
}

func TestRunEntriesOwnStopProtocolThroughCleanup(t *testing.T) {
	for _, single := range []bool{false, true} {
		t.Run(map[bool]string{false: "task", true: "single-step"}[single], func(t *testing.T) {
			fx := newGitFixture(t)
			require.NoError(t, WriteStopRequest(fx.taskDir, "old-run"))
			sd := happyDrivers()
			sd.planFn = func(a drivers.CallArgs) drivers.Result {
				control, err := ReadRunControl(fx.taskDir)
				require.NoError(t, err)
				require.NotNil(t, a.Watchdog.Context)
				assert.NoError(t, a.Watchdog.Context.Err(), "stale request from previous run was cleared")
				require.NoError(t, WriteStopRequest(fx.taskDir, control.ControlToken))
				select {
				case <-a.Watchdog.Context.Done():
				case <-time.After(time.Second):
					t.Fatal("run context did not receive stop request")
				}
				return drivers.MockResult(func(r *drivers.Result) { r.Cancelled = true; r.Settled = false })
			}
			var err error
			if single {
				opts := stepOpts(fx, sd.set())
				opts.Only = "plan"
				_, err = RunSingleStep(opts)
			} else {
				_, err = RunTask(runOpts(fx, sd.set()))
			}
			assert.True(t, errors.Is(err, ErrInterrupted), "%v", err)
			assert.NoFileExists(t, PidFile(fx.taskDir))
			assert.NoFileExists(t, StopRequestFile(fx.taskDir))
			release, err := AcquireRunLease(fx.taskDir)
			require.NoError(t, err)
			release()
		})
	}
}

func TestControlSidecarsAreOutsideCandidateAndCommit(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	before, err := c.candidateFingerprint(fx.root)
	require.NoError(t, err)
	require.NoError(t, c.writePid())
	require.NoError(t, WriteStopRequest(fx.taskDir, c.controlToken))
	writeFile(t, filepath.Join(fx.taskDir, ".control", "stop-request-leftover"), "old atomic write")
	writeFile(t, filepath.Join(fx.taskDir, ".control", "orchestrator-pid-leftover"), "old PID write")
	after, err := c.candidateFingerprint(fx.root)
	require.NoError(t, err)
	assert.Equal(t, before, after, "stop publication and interrupted atomic writes do not change verified code")
	writeFile(t, filepath.Join(fx.taskDir, "note.md"), "contract-side note")
	out := verify.CommitTaskDirChanges(fx.root, c.taskDirPrefix(), "docs: persist note", c.commitExcludes(), nil)
	require.Empty(t, out.Error)
	require.NotEmpty(t, out.SHA)
	for _, path := range []string{".control", ".stop-request", "orchestrator.pid"} {
		assert.Empty(t, gitCmd(t, fx.root, "ls-files", "--", filepath.Join(c.taskDirPrefix(), path)))
	}
}
