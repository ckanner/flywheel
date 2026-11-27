package machine

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/state"
)

// PacketCommand carries a revision-bound human triage decision for an escalated
// inbox packet: retry (with optional authoritative guidance), replan (single
// packet contract revision), or accept (human sign-off when L1 is green).
type PacketCommand struct {
	Action     string            `json:"action"` // retry | replan | accept
	PacketID   string            `json:"packet-id"`
	Revision   int64             `json:"revision"`
	Reason     string            `json:"reason"`
	Guidance   string            `json:"guidance,omitempty"`
	UserAction *PacketUserAction `json:"-"`
}

// PacketUserAction is the non-serialized human proof supplied by the DSH host
// command boundary or an interactive controlling terminal. A model tool cannot
// mint it.
type PacketUserAction struct {
	Kind      string `json:"kind"` // dsh | tty
	SessionID string `json:"sessionId,omitempty"`
	CommandID string `json:"commandId,omitempty"`
	TaskDir   string `json:"taskDir"`
	PacketID  string `json:"packetId"`
	Revision  int64  `json:"revision"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
	Guidance  string `json:"guidance,omitempty"`
}

// Validate checks shape and bounds for a packet triage request.
func (p *PacketCommand) Validate() error {
	if p == nil {
		return fmt.Errorf("missing packet command")
	}
	switch p.Action {
	case "retry", "replan", "accept":
	default:
		return fmt.Errorf("packet triage action must be retry, replan, or accept")
	}
	if strings.TrimSpace(p.PacketID) == "" || len(p.PacketID) > 100 || strings.ContainsAny(p.PacketID, "\x00\r\n") {
		return fmt.Errorf("packet triage requires a bounded packet-id")
	}
	if p.Revision < 1 {
		return fmt.Errorf("packet triage requires a positive revision")
	}
	if strings.TrimSpace(p.Reason) == "" || len(p.Reason) > 1000 || strings.ContainsRune(p.Reason, 0) {
		return fmt.Errorf("packet triage requires a bounded reason")
	}
	if len(p.Guidance) > 2000 || strings.ContainsRune(p.Guidance, 0) {
		return fmt.Errorf("packet triage guidance must be at most 2000 bytes without NUL")
	}
	return nil
}

func (a *PacketUserAction) validate(request PacketCommand, taskDir string) (string, error) {
	if a == nil {
		return "", fmt.Errorf("packet triage requires a verified human command or controlling terminal")
	}
	if a.Kind != "dsh" && a.Kind != "tty" {
		return "", fmt.Errorf("packet triage proof has an invalid kind")
	}
	proofDir, err := filepath.EvalSymlinks(a.TaskDir)
	if err != nil || proofDir != taskDir || a.Action != request.Action || a.PacketID != request.PacketID ||
		a.Revision != request.Revision || a.Reason != request.Reason || a.Guidance != request.Guidance {
		return "", fmt.Errorf("packet triage proof does not match this exact command")
	}
	if a.Kind == "dsh" {
		if strings.TrimSpace(a.SessionID) == "" || len(a.SessionID) > 200 ||
			strings.TrimSpace(a.CommandID) == "" || len(a.CommandID) > 200 ||
			strings.ContainsAny(a.SessionID+a.CommandID, "\x00\r\n") {
			return "", fmt.Errorf("packet triage host proof is invalid")
		}
		return "dsh:" + a.SessionID + ":" + a.CommandID, nil
	}
	return "tty", nil
}

// triagePacket applies a verified human triage action to an escalated inbox
// packet without ever editing state.json by hand.
func (c *Context) triagePacket(request PacketCommand) (TaskSummary, error) {
	if err := request.Validate(); err != nil {
		return c.summary(), err
	}
	if c.State.Handoff != nil {
		return c.summary(), fmt.Errorf("the task is under human session control; return it before triaging a packet")
	}
	packet, ok := c.Packets[request.PacketID]
	ps := c.State.Packets[request.PacketID]
	if !ok || ps == nil {
		return c.summary(), fmt.Errorf("packet %s not found", request.PacketID)
	}
	if ps.Status != "inbox" {
		return c.summary(), fmt.Errorf("packet %s is %s, not inbox; only escalated inbox packets can be triaged", request.PacketID, ps.Status)
	}
	source, err := request.UserAction.validate(request, c.TaskDir)
	if err != nil {
		return c.summary(), err
	}

	guidance := strings.TrimSpace(request.Guidance)
	switch request.Action {
	case "retry":
		ps.HumanGuidance = guidance
		ps.HumanSignoff = nil
		ps.Status = "pending"
		ps.FailedAttempts = 0
		ps.LastFailureSignature = nil
		at := c.nowISO()
		ps.UpdatedAt = &at
		c.recordRetry(ps, "attempt", "HUMAN_TRIAGE", "human-triage")
		c.State.Status = "paused"
		entry := map[string]any{
			"at":        at,
			"action":    "packet-retry",
			"packet-id": request.PacketID,
			"reason":    request.Reason,
			"source":    source,
		}
		if guidance != "" {
			entry["guidance"] = guidance
		}
		c.State.Inbox = append(c.State.Inbox, entry)
		c.appendInbox(entry)
		c.appendRetro(fmt.Sprintf("- %s packet %s: human-triage retry (%s)", at, request.PacketID, request.Reason))
		c.Log(fmt.Sprintf("%s: human re-armed inbox packet for retry (%s)", request.PacketID, request.Reason))
		return c.finishTriage("packet-retry")

	case "replan":
		if c.quick() {
			return c.summary(), fmt.Errorf("Quick has no runtime Plan; request --mode full to upgrade before replanning")
		}
		milestone, found := c.milestoneByID(ps.Milestone)
		if !found {
			return c.summary(), fmt.Errorf("packet %s references unknown milestone %q", request.PacketID, ps.Milestone)
		}
		feedbackParts := []string{
			fmt.Sprintf("Human-authorized contract revision for packet %s: %s", request.PacketID, request.Reason),
		}
		if guidance != "" {
			feedbackParts = append(feedbackParts, "Human guidance: "+guidance)
		}
		if ps.LastFeedback != nil {
			raw, _ := json.Marshal(ps.LastFeedback)
			feedbackParts = append(feedbackParts, "Last feedback: "+string(raw))
		}
		plan, err := c.runtimePlan(packet, milestone, strings.Join(feedbackParts, "\n"))
		if err != nil {
			return c.summary(), err
		}
		if !plan.OK {
			return c.summary(), fmt.Errorf("planner could not produce a valid revision (%s)", plan.ErrorCode)
		}
		if err := c.persistPlanContracts(fmt.Sprintf("docs(%s): human-authorized plan revision %d", c.State.Slug, ps.PlanRevisions+1)); err != nil {
			return c.summary(), err
		}
		invalidateEvidence(ps)
		c.finishRuntimeReplan(packet)
		ps.PlanRevisions++
		ps.HumanGuidance = guidance
		ps.HumanSignoff = nil
		ps.FailedAttempts = 0
		ps.LastFailureSignature = nil
		ps.Status = "pending"
		at := c.nowISO()
		ps.UpdatedAt = &at
		c.recordRetry(ps, "attempt", "HUMAN_TRIAGE", "human-triage")
		c.State.Status = "paused"
		entry := map[string]any{
			"at":             at,
			"action":         "packet-replan",
			"packet-id":      request.PacketID,
			"plan-revisions": ps.PlanRevisions,
			"reason":         request.Reason,
			"source":         source,
		}
		if guidance != "" {
			entry["guidance"] = guidance
		}
		c.State.Inbox = append(c.State.Inbox, entry)
		c.appendInbox(entry)
		c.appendRetro(fmt.Sprintf("- %s packet %s: human-triage replan revision %d (%s)", at, request.PacketID, ps.PlanRevisions, request.Reason))
		c.Log(fmt.Sprintf("%s: human-authorized plan revision %d persisted; packet re-armed", request.PacketID, ps.PlanRevisions))
		return c.finishTriage("packet-replan")

	default: // accept
		if !c.currentL1Evidence(packet) || (c.quick() && !c.currentFinalGate(packet)) {
			return c.summary(), fmt.Errorf("packet %s does not have passing L1 evidence on the current candidate; retry or replan instead of accepting", request.PacketID)
		}
		candidate := evidenceIdentity(ps.Evidence.L1)
		at := c.nowISO()
		ps.HumanSignoff = &state.PacketSignoff{
			At:        at,
			Reason:    request.Reason,
			Authority: "human",
			Source:    source,
			Candidate: candidate,
		}
		if err := c.completeUnit(packet); err != nil {
			ps.HumanSignoff = nil
			ps.Status = "inbox"
			c.State.Status = "escalated"
			c.save()
			return c.summary(), err
		}
		c.State.Status = "paused"
		entry := map[string]any{
			"at":        at,
			"action":    "packet-accept",
			"packet-id": request.PacketID,
			"reason":    request.Reason,
			"source":    source,
			"candidate": candidate,
		}
		c.State.Inbox = append(c.State.Inbox, entry)
		c.appendInbox(entry)
		c.appendRetro(fmt.Sprintf("- %s packet %s: human-triage accepted with L1 green (%s)", at, request.PacketID, request.Reason))
		c.Log(fmt.Sprintf("%s: human signed off and completed inbox packet (%s)", request.PacketID, request.Reason))
		return c.finishTriage("packet-accept")
	}
}

func (c *Context) finishTriage(stepName string) (TaskSummary, error) {
	c.save()
	if err := c.err(); err != nil {
		return c.summary(), err
	}
	summary := c.summary()
	summary.Step = &StepSum{Name: stepName, OK: true}
	return summary, nil
}
