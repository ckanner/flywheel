package machine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// RunControl identifies one task run. PID is display-only; a controller never
// signals it. A fresh random token prevents an old stop file cancelling a new run.
type RunControl struct {
	PID          int    `json:"pid"`
	StartedAt    string `json:"startedAt"`
	ControlToken string `json:"control-token"`
}

type stopRequest struct {
	Protocol     int    `json:"protocol"`
	ControlToken string `json:"control-token"`
}

var (
	stopRequestPollInterval = 50 * time.Millisecond
	controlFileFlock        = syscall.Flock
	createStopTemp          = os.CreateTemp
	writeStopData           = os.WriteFile
)

func StopRequestFile(taskDir string) string { return filepath.Join(taskDir, ".stop-request") }

// RunLeaseHeld probes an existing inode without creating/modifying any file.
// It is safe for read-only status; AcquireRunLease is the mutation-side authority.
func RunLeaseHeld(taskDir string) (bool, error) {
	f, err := os.Open(filepath.Join(taskDir, ".run.lock"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	err = controlFileFlock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		_ = controlFileFlock(int(f.Fd()), syscall.LOCK_UN)
		return false, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return true, nil
	}
	return false, err
}

func ReadRunControl(taskDir string) (RunControl, error) {
	var control RunControl
	raw, err := os.ReadFile(PidFile(taskDir))
	if err != nil {
		return control, err
	}
	if err := json.Unmarshal(raw, &control); err != nil {
		return control, fmt.Errorf("invalid orchestrator.pid: %w", err)
	}
	if control.PID <= 0 || strings.TrimSpace(control.ControlToken) == "" {
		return control, fmt.Errorf("running task has no control token; automatic PID signalling is not supported")
	}
	return control, nil
}

// WriteStopRequest atomically publishes a request for one exact run. The engine
// validates the token; malformed or stale requests cannot cancel its context.
func WriteStopRequest(taskDir, token string) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("stop control token is empty")
	}
	raw, _ := json.Marshal(stopRequest{Protocol: 1, ControlToken: token})
	runtimeDir := filepath.Join(taskDir, ".control")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return err
	}
	tmp, err := createStopTemp(runtimeDir, "stop-request-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(name)
	if err := writeStopData(name, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(name, StopRequestFile(taskDir))
}

// watchStopRequests lives inside the run lease. Its returned stop function joins
// the poller before sidecar cleanup/release, so no stale watcher can cancel a new run.
func watchStopRequests(parent context.Context, taskDir, token string, cancelRun context.CancelFunc) func() {
	ctx, stop := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(stopRequestPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				raw, err := os.ReadFile(StopRequestFile(taskDir))
				if err != nil {
					continue
				}
				var request stopRequest
				if json.Unmarshal(raw, &request) == nil && request.Protocol == 1 && request.ControlToken == token {
					cancelRun()
					return
				}
			}
		}
	}()
	return func() { stop(); <-done }
}
