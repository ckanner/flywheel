package machine

import (
	"errors"
	"fmt"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
)

// Console steps are deliberately bounded by the same serial milestone order
// as an unattended run. The ordinary CLI's explicit manual API stays compatible.
func (c *Context) admitConsoleStep(only, packetID string) error {
	if c.State.SharedExecution != nil {
		if err := c.reconcileSessionReceipts(); err != nil {
			return err
		}
		if only == "integrate" || only == "final" {
			return fmt.Errorf("resume the shared execution batch to verify and deliver all peers together")
		}
	}
	if packet := c.State.Packets[packetID]; packet != nil && packet.Status == "completed" && only == "execute" && c.consoleRetryReason != "" {
		if err := c.reopenConsolePacket(packetID); err != nil {
			return err
		}
	}
	if only == "final" {
		if packetID != "" {
			return fmt.Errorf("final acceptance operates on a milestone")
		}
		return nil
	}
	milestone := c.currentMilestone()
	if milestone == nil {
		return fmt.Errorf("task has no open milestone")
	}
	if only == "plan" {
		if packetID != "" {
			return fmt.Errorf("Plan operates on the current milestone, not a packet")
		}
		return nil
	}
	packet := c.nextActionablePacket(milestone.ID)
	if packet == nil || packet.PacketID != packetID {
		selected := c.Packets[packetID]
		if selected == nil || selected.Schedule == nil || c.State.Packets[packetID].Milestone != milestone.ID || c.State.Packets[packetID].Status == "completed" {
			return fmt.Errorf("packet must be next or explicitly independent in the current milestone")
		}
		for _, dependency := range selected.Schedule.After {
			if c.State.Packets[dependency].Status != "completed" {
				return fmt.Errorf("packet dependency %s is not delivered", dependency)
			}
		}
		packet = selected
	}
	if only == "execute" && packet.Schedule != nil && (c.Config.Worktree.Enabled == nil || *c.Config.Worktree.Enabled) {
		ps := c.State.Packets[packetID]
		if ps.Worktree == nil {
			head, ok := verify.GetBaseRef(c.Cwd)
			if !ok {
				return fmt.Errorf("independent execution requires Git")
			}
			wt := verify.EnsureUnitWorktree(c.Cwd, c.State.Slug, packetID, head)
			if wt.Mode != "worktree" {
				return fmt.Errorf("independent worktree unavailable: %s", wt.Reason)
			}
			ps.Worktree, ps.BaseRef = &wt, &head
		}
		ps.Parallel = true
		c.save()
		if err := c.err(); err != nil {
			return err
		}
	}
	if only == "verify" && c.State.Packets[packetID].Attempts == 0 {
		return fmt.Errorf("execute the packet before verifying its candidate")
	}
	return nil
}

// Reopening retains delivered Git history and all billing/failure counters. With
// candidate-wide acceptance, later units are conservatively downstream even
// when their write scopes are independent.
func (c *Context) reopenConsolePacket(packetID string) error {
	if c.Packets[packetID] == nil {
		return fmt.Errorf("unknown packet %s", packetID)
	}
	target := c.State.Packets[packetID].Milestone
	for _, milestone := range c.Spec.Milestones {
		if milestone.ID == target {
			break
		}
		if c.State.Milestones[milestone.ID].Status != "closed" {
			return fmt.Errorf("close earlier milestones before reopening this node")
		}
	}

	if c.State.Sessions != nil {
		for _, record := range c.State.Sessions.Records {
			if !state.TerminalSessionStatus(record.Last.Status) || !record.Last.CleanupConfirmed {
				return fmt.Errorf("reconcile all pending session receipts before reopening a delivered node")
			}
		}
	}
	downstream := false
	for _, packet := range c.PacketList {
		downstream = downstream || packet.PacketID == packetID
		if !downstream {
			if c.State.Packets[packet.PacketID].Status != "completed" {
				return fmt.Errorf("finish earlier units before reopening this node")
			}
			continue
		}
		ps := c.State.Packets[packet.PacketID]
		if ps.Status == "completed" && ps.Worktree != nil && ps.Worktree.Mode == "worktree" && fileExists(ps.Worktree.Path) {
			return fmt.Errorf("delivered worktree %s is still retained; reconcile its integration before reopening", ps.Worktree.Path)
		}
	}
	// PacketList and Packets are assembled together from validated contracts.
	downstream = false
	affected := []string{}
	for _, packet := range c.PacketList {
		downstream = downstream || packet.PacketID == packetID
		if !downstream {
			continue
		}
		ps := c.State.Packets[packet.PacketID]
		affected = append(affected, packet.PacketID)
		ps.EvidenceHistory = append(ps.EvidenceHistory, map[string]any{"at": c.nowISO(), "reason": c.consoleRetryReason,
			"action": "reopen", "commit": ps.Commit, "milestone": c.State.Milestones[ps.Milestone]})
		invalidateEvidence(ps)
		if ps.Status == "completed" {
			ps.BaseRef, ps.Worktree = nil, nil
			ps.SharedPeers = nil
		}
		ps.ContractSyncPending = ps.BaseRef != nil
		ps.Status, ps.Commit = "pending", nil
		ms := c.State.Milestones[ps.Milestone]
		ms.Status, ms.Gate, ms.ClosedAt = "pending", nil, nil
		c.State.Milestones[ps.Milestone] = ms
	}
	c.State.Inbox = append(c.State.Inbox, map[string]any{"at": c.nowISO(), "action": "reopen", "packet-id": packetID,
		"affected": affected, "reason": c.consoleRetryReason})
	c.State.Status = "paused"
	c.save()
	return c.err()
}

// A manual final stage uses the same candidate-bound gate and independent Judge
// as the workflow, without calling Plan or starting unfinished Execute units.
func (c *Context) runFinalStage() (bool, error) {
	milestone := c.currentMilestone()
	if milestone == nil {
		milestone = &c.Spec.Milestones[len(c.Spec.Milestones)-1]
	}
	var last *contract.Packet
	for _, packet := range c.PacketList {
		ps := c.State.Packets[packet.PacketID]
		if ps.Milestone != milestone.ID {
			continue
		}
		if ps.Status != "completed" {
			return false, fmt.Errorf("integrate all milestone packets before final acceptance")
		}
		if c.currentL1Evidence(packet) {
			last = packet
		}
	}
	if last == nil || !c.currentL1Evidence(last) {
		return false, fmt.Errorf("current integration candidate lacks its delivered unit evidence")
	}
	if c.quick() {
		_, err := c.runQuick()
		return err == nil, err
	}
	gate, err := c.sessionMilestoneGate(*milestone)
	if err != nil || !gate.Pass {
		return false, err
	}
	judge, err := c.judgeStep(*milestone)
	if err != nil || !judge.Pass {
		return false, err
	}
	ms := c.State.Milestones[milestone.ID]
	ms.Status, ms.ClosedAt = "closed", strPtrAlways(c.nowISO())
	c.State.Milestones[milestone.ID] = ms
	c.touchProgress()
	c.save()
	if c.contractsComplete() {
		return true, c.finishTask()
	}
	return true, c.err()
}

func (c *Context) finishControl(summary TaskSummary, runErr *error) {
	result := c.State.Control
	result.Status, result.FinishedAt = "passed", c.nowISO()
	if *runErr != nil {
		result.Status, result.Message = "failed", (*runErr).Error()
	} else if summary.Step != nil && !summary.Step.OK {
		result.Status = "failed"
		result.Message = "stage did not pass; inspect node evidence"
		if summary.Step.Detail != nil {
			result.Message = *summary.Step.Detail
		}
	}
	if errors.Is(*runErr, ErrInterrupted) {
		result.Status = "cancelled"
	}
	if message := []rune(result.Message); len(message) > 2000 {
		result.Message = string(message[:2000])
	}
	c.save()
	*runErr = errors.Join(*runErr, c.err())
}
