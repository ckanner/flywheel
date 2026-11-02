package watchdog

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKillActiveChildrenIncludesProcessStillReturningFromStart(t *testing.T) {
	originalStart := startCommand
	t.Cleanup(func() { startCommand = originalStart })
	started, ready := make(chan struct{}), make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	ctx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		cancel()
		workers.Wait()
	})
	startCommand = func(cmd *exec.Cmd) error {
		if err := originalStart(cmd); err != nil {
			return err
		}
		close(started)
		<-release // The real child is running, but Run has not registered it yet.
		return nil
	}
	done := make(chan Result, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		done <- Run("sh", []string{"-c", `trap 'exit 0' TERM; printf ready; while :; do :; done`}, Opts{
			Context: ctx, IdleTimeoutMs: 30_000, WallTimeoutMs: 30_000, KillGraceMs: 100,
			OnActivity: func(_, _ string) {
				select {
				case ready <- struct{}{}:
				default:
				}
			},
		})
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("the actual subprocess did not start")
	}
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("the subprocess did not install its TERM trap and emit readiness")
	}
	killing, killed := make(chan struct{}), make(chan struct{})
	workers.Add(1)
	go func() {
		defer workers.Done()
		close(killing)
		KillActiveChildren(50 * time.Millisecond)
		close(killed)
	}()
	<-killing
	select {
	case <-killed:
		t.Error("kill switch returned while an already running subprocess was not yet registered")
	case <-time.After(50 * time.Millisecond):
		// The snapshot must wait for startup to publish its child entry.
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case r := <-done:
		assert.True(t, r.Cancelled, "the one kill switch snapshot must include the started child")
		assert.False(t, r.TimedOut)
		require.NotNil(t, r.ExitCode)
		assert.Equal(t, 0, *r.ExitCode, "trap exit 0 cannot erase cancellation")
		assert.Empty(t, r.CleanupError)
	case <-time.After(3 * time.Second):
		t.Fatal("a subprocess active before the kill switch snapshot was left running")
	}
	select {
	case <-killed:
	case <-time.After(3 * time.Second):
		t.Fatal("kill switch did not return after the subprocess was reaped")
	}
}

func TestFailedStartDoesNotBlockLaterSnapshotsOrProcesses(t *testing.T) {
	done := make(chan Result, 1)
	go func() {
		failed := Run("/no/such/flywheel-startup-test-command", nil, Opts{})
		if failed.SpawnError == "" {
			done <- failed
			return
		}
		KillActiveChildren(time.Millisecond)
		done <- Run("sh", []string{"-c", "printf ready"}, Opts{})
	}()
	select {
	case r := <-done:
		require.NotNil(t, r.ExitCode)
		assert.Equal(t, 0, *r.ExitCode)
		assert.Empty(t, r.SpawnError)
		assert.Equal(t, "ready", r.Stdout)
	case <-time.After(3 * time.Second):
		t.Fatal("failed process startup left the registry locked")
	}
}
