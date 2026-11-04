package machine

import (
	"errors"
	"fmt"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
)

// An unconfirmed child lifetime is independent of the provider error code.
// Preserve the workspace and stop this entire run before retry, fallback,
// resident-session restart, or another role can overlap the unknown child.
func (c *Context) stopForUnconfirmedCleanup(role, packetID string, result drivers.Result) error {
	if result.CleanupError == "" {
		return nil
	}
	failure := fmt.Errorf("%s process cleanup is unconfirmed: %s; automatic execution stopped, confirm the owned processes have ended before resuming", role, result.CleanupError)
	at := c.nowISO()
	var packet any
	if packetID != "" {
		packet = packetID
		ps := c.State.Packets[packetID]
		ps.Status = "inbox"
		ps.UpdatedAt = &at
		ps.LastFeedback = map[string]any{"at": at, "role": role, "error-code": string(errs.TRANSPORT), "cleanup-error": result.CleanupError}
		c.recordRetry(ps, "lifecycle", string(errs.TRANSPORT), "cleanup-unconfirmed: no retry")
	}
	c.State.Status = "escalated"
	entry := map[string]any{"at": at, "packet-id": packet, "role": role,
		"reason": failure.Error(), "cleanup-error": result.CleanupError}
	c.State.Inbox = append(c.State.Inbox, entry)
	c.appendInbox(entry)
	c.Log(failure.Error())
	// Metrics may already have set fatal. Persist the escalation independently,
	// then retain both persistence errors alongside the lifecycle failure.
	previousFatal := c.fatal
	c.fatal = nil
	c.save()
	c.fatal = errors.Join(previousFatal, c.fatal, failure)
	return c.fatal
}
