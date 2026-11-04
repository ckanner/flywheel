package watchdog

import (
	"context"
	"io"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An adapter must retain its own cleanup window without delaying a different
// stuck process. The slow child finishes only after the fast child is reaped.
func TestKillActiveChildrenPreservesEachChildCleanupGrace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	t.Cleanup(func() { cancel(); workers.Wait() })
	ready := []chan struct{}{make(chan struct{}), make(chan struct{})}
	results := []chan Result{make(chan Result, 1), make(chan Result, 1)}
	stdin := make(chan io.WriteCloser, 1)
	for i, script := range []string{
		`trap '' TERM; printf fast-ready; while :; do :; done`,
		`trap 'printf cleanup-start; read ack; printf cleanup-done; exit 0' TERM; printf slow-ready; while :; do :; done`,
	} {
		workers.Add(1)
		go func() {
			defer workers.Done()
			var once sync.Once
			opts := Opts{Context: ctx, IdleTimeoutMs: 20_000, WallTimeoutMs: 20_000, KillGraceMs: 100,
				OnActivity: func(chunk, stream string) {
					if stream == "stdout" && strings.TrimSpace(chunk) != "" {
						once.Do(func() { close(ready[i]) })
					}
				}}
			if i == 1 {
				opts.KillGraceMs = 1500
				opts.InteractStdin = func(pipe io.WriteCloser, _ func()) { stdin <- pipe }
			}
			results[i] <- Run("sh", []string{"-c", script}, opts)
		}()
	}
	for _, signal := range ready {
		select {
		case <-signal:
		case <-time.After(3 * time.Second):
			t.Fatal("child did not install its signal handler")
		}
	}
	killed := make(chan struct{})
	go func() { KillActiveChildren(20 * time.Millisecond); close(killed) }()
	var fast Result
	select {
	case fast = <-results[0]:
	case <-time.After(time.Second):
		t.Fatal("a short-grace child borrowed another adapter's longer cleanup window")
	}
	assert.True(t, fast.Cancelled)
	assert.Equal(t, syscall.SIGKILL.String(), fast.Signal)
	assert.Empty(t, fast.CleanupError)
	_, err := io.WriteString(<-stdin, "finish\n")
	require.NoError(t, err, "the adapter cleanup must still be alive after the caller's short grace")
	select {
	case slow := <-results[1]:
		assert.True(t, slow.Cancelled)
		assert.False(t, slow.TimedOut)
		require.NotNil(t, slow.ExitCode)
		assert.Equal(t, 0, *slow.ExitCode)
		assert.Empty(t, slow.Signal)
		assert.Empty(t, slow.CleanupError)
		assert.Contains(t, slow.Stdout, "cleanup-done")
	case <-time.After(3 * time.Second):
		t.Fatal("adapter did not finish its acknowledged cleanup")
	}
	select {
	case <-killed:
	case <-time.After(time.Second):
		t.Fatal("kill switch did not await and release the cancellation snapshot")
	}
}
