package machine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/defectinjection"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
)

func (c *Context) quick() bool {
	return c.State.Workflow != nil && c.State.Workflow.Effective == "quick"
}

func (c *Context) initializeWorkflow(mode string) error {
	prior := c.State
	if c.Fresh {
		prior = nil
	}
	selection, err := state.ResolveWorkflow(prior, c.Spec, c.PacketList, mode)
	if err != nil {
		return err
	}
	if c.State.Workflow == nil {
		c.State.Workflow = &state.WorkflowState{WorkflowSelection: selection, SelectedAt: c.nowISO(), Upgrades: []state.WorkflowUpgrade{}}
	} else if c.quick() && selection.Effective == "full" {
		return c.upgradeWorkflow(selection.Reason, []string{"explicit workflow mode: " + mode})
	}
	return nil
}

// upgradeWorkflow writes the one-way checkpoint before Plan or any contract
// edit. A failed CAS prevents further work. No budget, attempts, workspace or
// history is replaced; invalid proof remains in the audit history.
func (c *Context) upgradeWorkflow(reason string, evidence []string) error {
	if !c.quick() {
		return nil
	}
	if err := c.err(); err != nil {
		return err
	}
	w := c.State.Workflow
	fingerprints := map[string]string{}
	for id, fingerprint := range c.State.ContractFingerprints {
		fingerprints[id] = fingerprint
	}
	w.Upgrades = append(w.Upgrades, state.WorkflowUpgrade{
		At: c.nowISO(), From: "quick", To: "full", Reason: reason,
		Evidence: append([]string{}, evidence...), ContractFingerprints: fingerprints,
	})
	w.Effective, w.Reason, w.Stages = "full", reason, contract.WorkflowStages("full")
	for _, p := range c.PacketList {
		ps := c.State.Packets[p.PacketID]
		invalidateEvidence(ps)
		if ps.Status == "completed" && ps.Worktree == nil {
			ps.BaseRef = nil
		}
		ps.Status = "pending"
		ps.LastFeedback = map[string]any{"workflow-upgrade": reason, "evidence": evidence, "previous-feedback": ps.LastFeedback}
		ms := c.State.Milestones[ps.Milestone]
		ms.Status, ms.Gate, ms.ClosedAt = "pending", nil, nil
		c.State.Milestones[ps.Milestone] = ms
	}
	c.State.Status = "running"
	c.save()
	if err := c.err(); err != nil {
		return err
	}
	c.appendRetro(fmt.Sprintf("- %s workflow Quick -> Full: %s", c.nowISO(), reason))
	return nil
}

// workflowEvidencePolicy keeps unchanged legacy Full proof compatible while
// binding Quick and upgraded Full to their distinct completion requirements.
func (c *Context) workflowEvidencePolicy() any {
	w := c.State.Workflow
	if w == nil || (w.Effective == "full" && len(w.Upgrades) == 0) {
		return nil
	}
	return map[string]any{"version": w.Version, "effective": w.Effective, "stages": w.Stages, "defect-injection": c.Config.DefectInjection}
}

// Keep this policy stable across a Quick -> Full checkpoint so an upgrade does
// not reset the existing retry counters through contract reconciliation.
func (c *Context) workflowDefectPolicy() any {
	w := c.State.Workflow
	if w != nil && (w.Effective == "quick" || len(w.Upgrades) != 0) {
		return c.Config.DefectInjection
	}
	return nil
}

// quickGateStep collects every applicable integration/user-path check against
// the very same packet worktree read by final Review, never the older task root.
func (c *Context) quickGateStep(packet *contract.Packet) bool {
	before, beforeErr := c.verificationIdentity(packet)
	pass := beforeErr == nil && c.currentL1Evidence(packet)
	levels := map[string]any{}
	for _, level := range []string{"L2", "L3"} {
		commands := packet.Acceptance.Level(level)
		if len(commands) == 0 {
			reason := c.Spec.Workflow.NotApplicable[level]
			levels[level] = map[string]any{"applicable": false, "reason": reason}
			if strings.TrimSpace(reason) == "" {
				pass = false
			}
			continue
		}
		if beforeErr != nil {
			levels[level] = map[string]any{"pass": false, "identity-error": beforeErr.Error()}
			continue
		}
		gate := verify.RunGateCommands(commands, verify.GateOpts{
			Context: c.RunContext, Cwd: c.unitCwd(packet),
			TimeoutMS: c.Config.Verify.CommandTimeoutMs, EnvAllowlist: c.Config.Verify.EnvAllowlist,
		})
		levels[level] = gate
		pass = pass && gate.Pass
	}
	after, afterErr := c.verificationIdentity(packet)
	identityError := afterErr
	if beforeErr != nil {
		identityError = beforeErr
	} else if afterErr == nil && after != before {
		identityError = fmt.Errorf("candidate, contract, report or workflow changed during Quick acceptance")
	}
	if afterErr != nil || after != before {
		pass = false
	}
	ps := c.State.Packets[packet.PacketID]
	ps.Evidence.FinalGate = map[string]any{
		"pass": pass, "levels": levels, "identity": before, "identity-after": after,
		"identity-error": jsonNilable(errorText(identityError)), "at": c.nowISO(), "cwd": c.unitCwd(packet),
	}
	c.save()
	return pass
}

func (c *Context) quickReviewInjection(packet *contract.Packet) (bool, error) {
	outcome, err := c.maybeDefectInjection(packet)
	if err != nil {
		return true, err
	}
	evidence, present := c.State.Packets[packet.PacketID].Evidence.DefectInjection.(map[string]any)
	if present && evidence["outcome"] != "pass" {
		// Full's historical diagnostic-only cases cannot count as Quick's
		// complete final acceptance. Preserve the actual outcome in evidence.
		if outcome == nil {
			outcome = &defectinjection.Outcome{Outcome: "inconclusive"}
		}
		return true, c.injectionStop(packet, outcome)
	}
	return false, nil
}

func (c *Context) currentFinalGate(packet *contract.Packet) bool {
	gate, ok := c.State.Packets[packet.PacketID].Evidence.FinalGate.(map[string]any)
	if !ok || gate["pass"] != true {
		return false
	}
	identity, err := c.verificationIdentity(packet)
	return err == nil && identity == evidenceIdentity(gate)
}

func (c *Context) validFinalReview(packet *contract.Packet) bool {
	review, ok := c.State.Packets[packet.PacketID].Evidence.Review.(map[string]any)
	if !ok || review["scope"] != "task-final" || review["verdict"] != "pass" {
		return false
	}
	raw, _ := json.Marshal(review)
	var verdict prompts.Verdict
	if err := json.Unmarshal(raw, &verdict); err != nil {
		return false
	}
	return prompts.ValidateFinalReviewVerdict(&verdict, c.Spec) == nil
}

func (c *Context) executorWorkflowRisks(packet *contract.Packet) []string {
	executor, _ := c.State.Packets[packet.PacketID].Evidence.Executor.(map[string]any)
	values, _ := executor["workflow-risks"].([]any)
	var risks []string
	for _, value := range values {
		risk, _ := value.(string) // done-report parser already enforces string arrays
		if strings.TrimSpace(risk) != "" {
			risks = append(risks, risk)
		}
	}
	return risks
}

// runQuick reuses the ordinary packet loop and persistence safeguards. "upgrade"
// hands control to the Full scheduler in this same run, without resetting state.
func (c *Context) runQuick() (bool, error) {
	if err := c.err(); err != nil {
		return false, err
	}
	selection, err := contract.SelectWorkflow(c.Spec, c.PacketList, "quick")
	if err != nil {
		return false, err
	}
	if selection.Effective != "quick" {
		return true, c.upgradeWorkflow(selection.Reason, selection.Evidence)
	}
	packet, milestone := c.PacketList[0], c.Spec.Milestones[0]
	ms := c.State.Milestones[milestone.ID]
	if ms.Status != "closed" || c.State.Packets[packet.PacketID].Status != "completed" {
		if err := c.persistPlanContracts(fmt.Sprintf("docs(%s): prepared Quick contracts", c.State.Slug)); err != nil {
			return false, err
		}
		c.State.CurrentMilestone = strPtrAlways(milestone.ID)
		ms.Status = "in-progress"
		c.State.Milestones[milestone.ID] = ms
		c.save()
		if c.State.Packets[packet.PacketID].Status != "completed" {
			outcome, err := c.processPacket(packet, processOpts{PauseBefore: c.pauseBefore})
			if err != nil {
				return false, err
			}
			if outcome == "upgrade" {
				return true, nil
			}
			if outcome == "stop" || outcome == "pause" {
				return false, c.err()
			}
		}
	}
	// Also check a closed milestone: a crash after close but before task finish
	// must not bless a subsequently changed, even fully committed, candidate.
	if !c.currentFinalGate(packet) || !c.currentL1Evidence(packet) || !c.validFinalReview(packet) ||
		evidenceIdentity(c.State.Packets[packet.PacketID].Evidence.Review) != evidenceIdentity(c.State.Packets[packet.PacketID].Evidence.FinalGate) {
		c.State.Status = "escalated"
		c.State.Packets[packet.PacketID].Status = "inbox"
		ms.Status, ms.ClosedAt = "in-progress", nil
		c.State.Milestones[milestone.ID] = ms
		entry := map[string]any{"at": c.nowISO(), "packet-id": packet.PacketID, "reason": "Quick completion proof is stale; preserve the delivered candidate and reconcile changes before retrying"}
		c.State.Inbox = append(c.State.Inbox, entry)
		c.appendInbox(entry)
		c.save()
		return false, fmt.Errorf("%w: Quick completion proof is stale", ErrCommitFailed)
	}
	if ms.Status != "closed" {
		ms.Gate = c.State.Packets[packet.PacketID].Evidence.FinalGate
		ms.Status, ms.ClosedAt = "closed", strPtrAlways(c.nowISO())
		c.State.Milestones[milestone.ID] = ms
		c.appendRetro(fmt.Sprintf("- %s milestone %s: closed (Quick final review; Plan/Judge not applicable)", c.nowISO(), milestone.ID))
		c.save()
	}
	return false, c.finishTask()
}
