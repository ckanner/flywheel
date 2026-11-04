package modules

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
)

// Advice modules also own their model calls. A potentially live child prevents
// every later candidate from running, regardless of transport retry policy.
func (m *Ctx) stopForUnconfirmedCleanup(role string, result drivers.Result) bool {
	if result.CleanupError == "" {
		return false
	}
	failure := fmt.Errorf("%s process cleanup is unconfirmed: %s; automatic execution stopped, confirm the owned processes have ended before resuming", role, result.CleanupError)
	entry := map[string]any{"at": state.NowISO(), "packet-id": nil, "role": role,
		"reason": failure.Error(), "cleanup-error": result.CleanupError}
	inboxErr := appendModuleCleanupInbox(m.TaskDir, entry)
	stateErr := saveModuleCleanupEscalation(m.TaskDir, entry)
	m.fatalErr = errors.Join(m.fatalErr, failure, inboxErr, stateErr)
	m.Log(m.fatalErr.Error())
	return true
}

func appendModuleCleanupInbox(taskDir string, entry map[string]any) error {
	f, err := os.OpenFile(filepath.Join(taskDir, "inbox.md"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n## %s — %s cleanup\n\n%s\n", entry["at"], entry["role"], entry["reason"])
	return err
}

// An active orchestrator owns its revision. Preserve that boundary, and keep
// the module's independent inbox/metrics record when no idle task is available.
func saveModuleCleanupEscalation(taskDir string, entry map[string]any) error {
	_, err := updateIdleTaskState(taskDir, func(st *state.State) {
		st.Status = "escalated"
		st.Inbox = append(st.Inbox, entry)
	})
	return err
}
