package machine

import (
	"fmt"
	"strings"

	"github.com/ckanner/flywheel/internal/state"
)

// SessionUserAction is the non-serialized proof supplied by the host command
// boundary or the controlling terminal. A model tool cannot mint it, and the
// engine requires it for the irreversible abandonment of an unknown call.
type SessionUserAction struct {
	Kind      string `json:"kind"` // dsh | tty
	SessionID string `json:"sessionId,omitempty"`
	CommandID string `json:"commandId,omitempty"`
	Key       string `json:"key"`
	Revision  int64  `json:"revision"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
}

func (a *SessionUserAction) validate(request SessionCommand) (string, error) {
	if a == nil {
		return "", fmt.Errorf("abandoning an unknown invocation requires a verified human command or controlling terminal")
	}
	if a.Kind != "dsh" && a.Kind != "tty" {
		return "", fmt.Errorf("session abandonment proof has an invalid kind")
	}
	if a.Action != "abandon" || a.Key != request.Key || a.Revision != request.Revision || a.Reason != request.Reason {
		return "", fmt.Errorf("session abandonment proof does not match this exact command")
	}
	if strings.TrimSpace(a.Reason) == "" || len(a.Reason) > 1000 || strings.ContainsRune(a.Reason, 0) {
		return "", fmt.Errorf("session abandonment requires a bounded reason")
	}
	if a.Kind == "dsh" {
		if strings.TrimSpace(a.SessionID) == "" || len(a.SessionID) > 200 ||
			strings.TrimSpace(a.CommandID) == "" || len(a.CommandID) > 200 ||
			strings.ContainsAny(a.SessionID+a.CommandID, "\x00\r\n") {
			return "", fmt.Errorf("session abandonment host proof is invalid")
		}
		return "dsh:" + a.SessionID + ":" + a.CommandID, nil
	}
	return "tty", nil
}

func isSessionAbandon(request *SessionCommand) bool {
	return request != nil && request.Action == "abandon"
}

func planningAbandonRole(role string) bool {
	return role == "design-plan" || role == "design-review" || role == "plan" || role == "plan-review"
}

// abandonSession retires an unresolved planning invocation after an explicit
// human decision. It never applies a stage result, never resumes the old
// thread, and keeps the unknown dispatch/outcome facts in Last. The next role
// invocation rebuilds a fresh generation under the normal binding checks.
func (c *Context) abandonSession(request SessionCommand) (TaskSummary, error) {
	if c.State.Handoff != nil {
		return c.summary(), fmt.Errorf("the task is under human session control; return it before abandoning an invocation")
	}
	if c.State.Sessions == nil {
		return c.summary(), fmt.Errorf("session key not found")
	}
	r := c.State.Sessions.Records[request.Key]
	if r == nil || r.Last == nil {
		return c.summary(), fmt.Errorf("session key not found")
	}
	if request.Key != sessionKey(r.Role, r.Scope) {
		return c.summary(), fmt.Errorf("session key does not match its recorded role and scope")
	}
	if !planningAbandonRole(r.Role) {
		return c.summary(), fmt.Errorf("unknown invocation abandonment is limited to planning roles; use the packet reconciliation path for %s", r.Role)
	}
	if r.Last.Status != "blocked" {
		return c.summary(), fmt.Errorf("session %s is %s, not blocked; only an unresolved invocation can be abandoned", request.Key, r.Last.Status)
	}
	if !r.Last.Accounted {
		return c.summary(), fmt.Errorf("session %s has not accounted its original receipt; account it before abandoning", request.Key)
	}
	if !r.Last.CleanupConfirmed {
		return c.summary(), fmt.Errorf("session %s cleanup is unconfirmed; confirm the process group has stopped before abandoning", request.Key)
	}
	if r.Last.Dispatch != "unknown" && r.Last.Outcome != "unknown" {
		return c.summary(), fmt.Errorf("session %s has known dispatch and outcome; apply or reconcile the receipt instead of abandoning", request.Key)
	}
	source, err := request.UserAction.validate(request)
	if err != nil {
		return c.summary(), err
	}

	requestID := ""
	if strings.HasPrefix(r.Scope, "planning:") {
		requestID = strings.TrimPrefix(r.Scope, "planning:")
	}
	oldThread := r.ThreadID
	r.Last.Abandon = &state.SessionAbandon{
		At: c.nowISO(), Reason: request.Reason, Authority: "human",
		RequestID: requestID, ThreadID: oldThread, Source: source,
	}
	r.Last.Status = "abandoned"
	r.Action = "abandon"
	r.Reason = "human abandoned unknown invocation: " + request.Reason
	r.ResumeCandidate = false
	r.ThreadID = ""
	if c.State.Planning != nil && requestID != "" {
		for i := range c.State.Planning.Requests {
			if c.State.Planning.Requests[i].ID == requestID {
				// The old request has no result to apply. A later explicit Plan
				// creates a new bounded request instead of resuming this one.
				c.State.Planning.Requests[i].Status = "cancelled"
			}
		}
	}
	c.State.Status = "paused"
	entry := map[string]any{
		"at": c.nowISO(), "action": "session-abandon", "session": request.Key,
		"invocation": r.Last.ID, "reason": request.Reason, "source": source,
	}
	c.State.Inbox = append(c.State.Inbox, entry)
	c.appendInbox(entry)
	c.Log(fmt.Sprintf("session %s: human abandoned unknown invocation %s", request.Key, r.Last.ID))
	c.save()
	if err := c.err(); err != nil {
		return c.summary(), err
	}
	summary := c.summary()
	summary.Step = &StepSum{Name: "session-abandon", OK: true}
	return summary, nil
}
