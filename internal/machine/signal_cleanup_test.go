package machine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise the real OS signal handler, including its separate global kill
// switch. Context cancellation alone would miss an early global SIGKILL.
func TestSigtermHonorsAdapterCleanupGrace(t *testing.T) {
	const helper = "FLYWHEEL_SIGTERM_GRACE_HELPER"
	switch os.Getenv(helper) {
	case "worker":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM)
		fmt.Println("worker-ready")
		<-signals
		// The machine handler's generic grace is 5 seconds; an adapter can
		// require longer to await its nested supervisor and final receipt.
		time.Sleep(5500 * time.Millisecond)
		fmt.Println(`{"cleanupConfirmed":true}`)
		os.Exit(0)
	case "owner":
		ctx, cancel := context.WithCancel(context.Background())
		ch := registerSigterm(func(string) {}, cancel)
		defer unregisterSigterm(ch)
		var ready sync.Once
		result := watchdog.Run(os.Args[0], []string{"-test.run=^TestSigtermHonorsAdapterCleanupGrace$"}, watchdog.Opts{
			Context: ctx, Env: append(os.Environ(), helper+"=worker"),
			IdleTimeoutMs: 20_000, WallTimeoutMs: 20_000, KillGraceMs: 8000,
			OnActivity: func(chunk, stream string) {
				if stream == "stdout" && strings.Contains(chunk, "worker-ready") {
					ready.Do(func() { _ = os.WriteFile(os.Getenv("FLYWHEEL_SIGTERM_GRACE_READY"), []byte("ready"), 0o600) })
				}
			},
		})
		_ = json.NewEncoder(os.Stdout).Encode(result)
		os.Exit(143)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	readyFile := filepath.Join(t.TempDir(), "ready")
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSigtermHonorsAdapterCleanupGrace$")
	cmd.Env = append(os.Environ(), helper+"=owner", "FLYWHEEL_SIGTERM_GRACE_READY="+readyFile)
	done := make(chan struct{})
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	require.NoError(t, cmd.Start())
	var runError error
	go func() { runError = cmd.Wait(); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	require.Eventually(t, func() bool {
		_, err := os.Stat(readyFile)
		return err == nil
	}, 5*time.Second, 10*time.Millisecond, "worker did not install its TERM handler")
	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("SIGTERM cleanup did not finish within its bounded budget")
	}
	var exitError *exec.ExitError
	require.ErrorAs(t, runError, &exitError, "%s", output.String())
	assert.Equal(t, 143, exitError.ExitCode(), "%s", output.String())
	var result watchdog.Result
	require.NoError(t, json.Unmarshal(output.Bytes(), &result), "%s", output.String())
	assert.True(t, result.Cancelled)
	assert.False(t, result.TimedOut)
	assert.False(t, result.Truncated)
	assert.Empty(t, result.Signal)
	assert.Empty(t, result.CleanupError)
	require.NotNil(t, result.ExitCode)
	assert.Equal(t, 0, *result.ExitCode, "global cancellation must allow the adapter's complete cleanup receipt")
	assert.Contains(t, result.Stdout, `{"cleanupConfirmed":true}`)
}
