package modules

import (
	"errors"
	"path/filepath"
	"syscall"

	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
)

// Both budget accounting and cleanup escalation must respect the orchestrator's
// entire run lease, including the startup interval before its PID is published.
// Keep the lease through the read/modify/CAS-save sequence; a probe alone would
// leave another startup window between checking ownership and writing state.
func updateIdleTaskState(taskDir string, update func(*state.State)) (bool, error) {
	if !state.Exists(taskDir) {
		return false, nil
	}
	release, err := machine.AcquireRunLease(taskDir)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer release()
	// Check after acquisition: a pre-lock PID check would race a starting run.
	if exists(filepath.Join(taskDir, "orchestrator.pid")) {
		return false, nil
	}
	st, err := state.Load(taskDir)
	if err != nil {
		return false, err
	}
	update(st)
	_, err = state.Save(taskDir, st, nil)
	return err == nil, err
}
