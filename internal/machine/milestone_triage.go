package machine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
)

// MilestoneCommand carries a revision-bound human triage decision for a
// milestone whose L2/L3 gate or Judge escalated: retry (with optional
// authoritative Judge guidance) or accept (human sign-off when all milestone
// units are completed and L2/L3 deterministic gate passed on current HEAD).
type MilestoneCommand struct {
	Action      string               `json:"action"` // retry | accept
	MilestoneID string               `json:"milestone-id"`
	Revision    int64                `json:"revision"`
	Reason      string               `json:"reason"`
	Guidance    string               `json:"guidance,omitempty"`
	UserAction  *MilestoneUserAction `json:"-"`
}

// MilestoneUserAction is the non-serialized human proof supplied by the DSH
// host command boundary or an interactive controlling terminal. A model tool
// cannot mint it.
type MilestoneUserAction struct {
	Kind        string `json:"kind"` // dsh | tty
	SessionID   string `json:"sessionId,omitempty"`
	CommandID   string `json:"commandId,omitempty"`
	TaskDir     string `json:"taskDir"`
	MilestoneID string `json:"milestoneId"`
	Revision    int64  `json:"revision"`
	Action      string `json:"action"`
	Reason      string `json:"reason"`
	Guidance    string `json:"guidance,omitempty"`
}

// Validate checks shape and bounds for a milestone triage request.
func (m *MilestoneCommand) Validate() error {
	if m == nil {
		return fmt.Errorf("missing milestone command")
	}
	switch m.Action {
	case "retry", "accept":
	default:
		return fmt.Errorf("milestone triage action must be retry or accept")
	}
	if strings.TrimSpace(m.MilestoneID) == "" || len(m.MilestoneID) > 100 || strings.ContainsAny(m.MilestoneID, "\x00\r\n") {
		return fmt.Errorf("milestone triage requires a bounded milestone-id")
	}
	if m.Revision < 1 {
		return fmt.Errorf("milestone triage requires a positive revision")
	}
	if strings.TrimSpace(m.Reason) == "" || len(m.Reason) > 1000 || strings.ContainsRune(m.Reason, 0) {
		return fmt.Errorf("milestone triage requires a bounded reason")
	}
	if len(m.Guidance) > 2000 || strings.ContainsRune(m.Guidance, 0) {
		return fmt.Errorf("milestone triage guidance must be at most 2000 bytes without NUL")
	}
	return nil
}

func (a *MilestoneUserAction) validate(request MilestoneCommand, taskDir string) (string, error) {
	if a == nil {
		return "", fmt.Errorf("milestone triage requires a verified human command or controlling terminal")
	}
	if a.Kind != "dsh" && a.Kind != "tty" {
		return "", fmt.Errorf("milestone triage proof has an invalid kind")
	}
	proofDir, err := filepath.EvalSymlinks(a.TaskDir)
	if err != nil || proofDir != taskDir || a.Action != request.Action || a.MilestoneID != request.MilestoneID ||
		a.Revision != request.Revision || a.Reason != request.Reason || a.Guidance != request.Guidance {
		return "", fmt.Errorf("milestone triage proof does not match this exact command")
	}
	if a.Kind == "dsh" {
		if strings.TrimSpace(a.SessionID) == "" || len(a.SessionID) > 200 ||
			strings.TrimSpace(a.CommandID) == "" || len(a.CommandID) > 200 ||
			strings.ContainsAny(a.SessionID+a.CommandID, "\x00\r\n") {
			return "", fmt.Errorf("milestone triage host proof is invalid")
		}
		return "dsh:" + a.SessionID + ":" + a.CommandID, nil
	}
	return "tty", nil
}

// triageMilestone applies a verified human triage action to an open milestone
// without ever editing state.json by hand.
func (c *Context) triageMilestone(request MilestoneCommand) (TaskSummary, error) {
	if err := request.Validate(); err != nil {
		return c.summary(), err
	}
	if c.State.Handoff != nil {
		return c.summary(), fmt.Errorf("the task is under human session control; return it before triaging a milestone")
	}
	milestone, found := c.milestoneByID(request.MilestoneID)
	ms, exists := c.State.Milestones[request.MilestoneID]
	if !found || !exists {
		return c.summary(), fmt.Errorf("milestone %s not found", request.MilestoneID)
	}
	if ms.Status == "closed" {
		return c.summary(), fmt.Errorf("milestone %s is already closed", request.MilestoneID)
	}
	source, err := request.UserAction.validate(request, c.TaskDir)
	if err != nil {
		return c.summary(), err
	}

	guidance := strings.TrimSpace(request.Guidance)
	at := c.nowISO()
	switch request.Action {
	case "retry":
		ms.HumanGuidance = guidance
		ms.HumanSignoff = nil
		c.State.Milestones[request.MilestoneID] = ms
		c.State.Status = "paused"
		entry := map[string]any{
			"at":        at,
			"action":    "milestone-retry",
			"milestone": request.MilestoneID,
			"reason":    request.Reason,
			"source":    source,
		}
		if guidance != "" {
			entry["guidance"] = guidance
		}
		c.State.Inbox = append(c.State.Inbox, entry)
		c.appendInbox(entry)
		c.appendRetro(fmt.Sprintf("- %s milestone %s: human-triage retry (%s)", at, request.MilestoneID, request.Reason))
		c.Log(fmt.Sprintf("%s: human re-armed milestone for gate/judge re-verification (%s)", request.MilestoneID, request.Reason))
		return c.finishTriage("milestone-retry")

	default: // accept
		var milestonePackets int
		for _, p := range c.PacketList {
			if m, err := contract.MilestoneOfPacket(p.PacketID); err == nil && m == request.MilestoneID {
				milestonePackets++
				ps := c.State.Packets[p.PacketID]
				if ps == nil || ps.Status != "completed" {
					return c.summary(), fmt.Errorf("milestone %s cannot be accepted while packet %s is not completed", request.MilestoneID, p.PacketID)
				}
			}
		}
		if milestonePackets == 0 {
			return c.summary(), fmt.Errorf("milestone %s has no completed packets to accept", request.MilestoneID)
		}
		gmap, ok := ms.Gate.(map[string]any)
		gateIdentity := evidenceIdentity(ms.Gate)
		if !ok || gmap["pass"] != true || gateIdentity == "" {
			return c.summary(), fmt.Errorf("milestone %s does not have a passing deterministic L2/L3 gate; retry instead of accepting", request.MilestoneID)
		}
		candidate, candErr := c.candidateFingerprint(c.Cwd)
		if candErr != nil || candidate != gateIdentity {
			return c.summary(), fmt.Errorf("milestone %s gate evidence does not match the current repository candidate; re-verify before accepting", request.MilestoneID)
		}

		prev := ms
		ms.HumanGuidance = guidance
		ms.HumanSignoff = &state.PacketSignoff{
			At:        at,
			Reason:    request.Reason,
			Authority: "human",
			Source:    source,
			Candidate: candidate,
		}
		ms.Status = "closed"
		ms.ClosedAt = &at
		c.State.Milestones[request.MilestoneID] = ms
		if c.currentMilestone() == nil {
			if err := c.finishTask(); err != nil {
				c.State.Milestones[request.MilestoneID] = prev
				c.State.Status = "escalated"
				c.save()
				return c.summary(), err
			}
		} else {
			c.State.Status = "paused"
		}
		c.touchProgress()
		entry := map[string]any{
			"at":        at,
			"action":    "milestone-accept",
			"milestone": request.MilestoneID,
			"reason":    request.Reason,
			"source":    source,
			"candidate": candidate,
		}
		if guidance != "" {
			entry["guidance"] = guidance
		}
		c.State.Inbox = append(c.State.Inbox, entry)
		c.appendInbox(entry)
		c.appendRetro(fmt.Sprintf("- %s milestone %s: human-triage accepted with L2/L3 gate green (%s)", at, request.MilestoneID, request.Reason))
		c.appendDeliveredMilestoneMemory(milestone, fmt.Sprintf("gate pass, human-accepted (%s)", request.Reason))
		c.Log(fmt.Sprintf("%s: human signed off and closed milestone (%s)", request.MilestoneID, request.Reason))
		return c.finishTriage("milestone-accept")
	}
}

// appendDeliveredMilestoneMemory records an objective, idempotent delivery line
// under "## Delivered Milestones" in project-context.md whenever a milestone
// closes (Judge pass or human milestone accept). Written deterministically by
// Go — never by an LLM, and never caching volatile execution state.
func (c *Context) appendDeliveredMilestoneMemory(milestone contract.Milestone, outcome string) {
	path := filepath.Join(c.TaskDir, "project-context.md")
	existing := ""
	if raw, err := os.ReadFile(path); err == nil {
		existing = string(raw)
	}
	marker := "- " + milestone.ID + " ("
	if strings.Contains(existing, marker) {
		return
	}
	packetIDs := []string{}
	for _, p := range c.PacketList {
		if m, err := contract.MilestoneOfPacket(p.PacketID); err == nil && m == milestone.ID {
			if ps := c.State.Packets[p.PacketID]; ps != nil && ps.Status == "completed" {
				packetIDs = append(packetIDs, p.PacketID)
			}
		}
	}
	sort.Strings(packetIDs)
	head := shortHeadSHA(c.Cwd)
	line := fmt.Sprintf("- %s (%s): packets=[%s], head=%s, outcome=%s\n",
		milestone.ID, milestone.Title, strings.Join(packetIDs, ", "), head, outcome)

	trimmed := strings.TrimRight(existing, "\n")
	var next string
	switch {
	case trimmed == "":
		next = "## Delivered Milestones\n" + line
	case strings.Contains(trimmed, "## Delivered Milestones"):
		next = trimmed + "\n" + line
	default:
		next = trimmed + "\n\n## Delivered Milestones\n" + line
	}
	_ = os.WriteFile(path, []byte(next), 0o644)
}

func shortHeadSHA(cwd string) string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return "n/a"
	}
	return strings.TrimSpace(string(out))
}
