package machine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/state"
)

// SessionUserAction is the non-serialized proof supplied by the host command
// boundary or the controlling terminal. A model tool cannot mint it, and the
// engine requires it for the irreversible abandonment of an unknown call.
type SessionUserAction struct {
	Kind       string `json:"kind"` // dsh | tty
	SessionID  string `json:"sessionId,omitempty"`
	CommandID  string `json:"commandId,omitempty"`
	TaskDir    string `json:"taskDir"`
	Key        string `json:"key"`
	Invocation string `json:"invocation"`
	Revision   int64  `json:"revision"`
	Action     string `json:"action"`
	Reason     string `json:"reason"`
}

func (a *SessionUserAction) validate(request SessionCommand, taskDir, invocation string) (string, error) {
	if a == nil {
		return "", fmt.Errorf("abandoning an unknown invocation requires a verified human command or controlling terminal")
	}
	if a.Kind != "dsh" && a.Kind != "tty" {
		return "", fmt.Errorf("session abandonment proof has an invalid kind")
	}
	proofDir, err := filepath.EvalSymlinks(a.TaskDir)
	if err != nil || proofDir != taskDir || a.Action != "abandon" || a.Key != request.Key ||
		a.Revision != request.Revision || a.Reason != request.Reason ||
		a.Invocation != invocation || request.Invocation != invocation {
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
	if request.Invocation == "" {
		return c.summary(), fmt.Errorf("session abandonment requires the exact invocation ID")
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
	if request.Invocation != r.Last.ID {
		return c.summary(), fmt.Errorf("session invocation %s does not match the recorded invocation %s", request.Invocation, r.Last.ID)
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
	source, err := request.UserAction.validate(request, c.TaskDir, r.Last.ID)
	if err != nil {
		return c.summary(), err
	}

	requestID := ""
	if strings.HasPrefix(r.Scope, "planning:") {
		requestID = strings.TrimPrefix(r.Scope, "planning:")
	}
	if requestID != "" {
		if c.State.Planning == nil {
			return c.summary(), fmt.Errorf("session %s names a planning request but the task has no planning authority", request.Key)
		}
		if c.State.Planning.PendingActivation != nil {
			return c.summary(), fmt.Errorf("a planning activation transaction is pending; reconcile it before abandoning a session")
		}
		index := -1
		for i := range c.State.Planning.Requests {
			if c.State.Planning.Requests[i].ID == requestID {
				index = i
				break
			}
		}
		if index < 0 {
			return c.summary(), fmt.Errorf("planning request %s was not found for session %s", requestID, request.Key)
		}
		planningReq := &c.State.Planning.Requests[index]
		if planningReq.Status != "pending" && planningReq.Status != "running" {
			return c.summary(), fmt.Errorf("planning request %s is %s, not pending or running", requestID, planningReq.Status)
		}
		if planningReq.InputRevision != c.State.Planning.InputRevision {
			return c.summary(), fmt.Errorf("planning request %s belongs to input revision %d, not the current revision", requestID, planningReq.InputRevision)
		}
		if planningReq.Role != "" && planningReq.Role != r.Role {
			return c.summary(), fmt.Errorf("planning request %s belongs to role %s, not %s", requestID, planningReq.Role, r.Role)
		}
		planningReq.Status = "cancelled"
	}
	oldThread := r.ThreadID
	if oldThread == "" {
		oldThread = r.Last.RequestedThread
	}
	// The legacy resident Plan cache is a second owner of the same CLI history.
	// Leaving its unknown marker blocks every retry; leaving its history ID can
	// resume the retired thread. Retire only the cache tied to this exact call,
	// in the same CAS as Last, while preserving the thread in the audit record.
	if legacy, ok := c.State.PlanSession.(map[string]any); ok && requestID == "" && r.Role == "plan" &&
		r.Protocol == "kimi-print/v1" && legacy["harness"] == "kimi" && legacy["cwd"] == r.Cwd {
		thread, _ := legacy["session-id"].(string)
		if thread != "" && (thread == r.Last.RequestedThread || thread == r.ThreadID) {
			c.State.PlanSession = nil
		}
	}
	r.Last.Abandon = &state.SessionAbandon{
		At: c.nowISO(), Reason: request.Reason, Authority: "human",
		RequestID: requestID, Invocation: r.Last.ID, ThreadID: oldThread, Source: source,
	}
	r.Last.Status = "abandoned"
	r.Action = "abandon"
	r.Reason = "human abandoned unknown invocation: " + request.Reason
	r.ResumeCandidate = false
	r.ThreadID = ""
	c.State.Status = "paused"
	entry := map[string]any{
		"at": c.nowISO(), "action": "session-abandon", "session": request.Key,
		"invocation": r.Last.ID, "reason": request.Reason, "source": source,
		"thread-id": oldThread, "dispatch": r.Last.Dispatch, "outcome": r.Last.Outcome,
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
