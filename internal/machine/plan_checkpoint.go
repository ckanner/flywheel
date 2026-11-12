package machine

import (
	"os/exec"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
)

// A Plan checkpoint is a confirmed planning result, not a cached conversation.
// Version changes invalidate older proofs when the planning policy changes.
const planCheckpointVersion = 1

func (c *Context) planInputIdentity(milestone string) string {
	return sessionHash([]any{
		"plan-checkpoint/v1", c.State.Slug, milestone,
		c.State.ContractFingerprints, c.Spec, c.PacketList,
		c.Config.Verify, c.Config.Scheduler, c.workflowEvidencePolicy(),
	})
}

// Confirmation is independent from persistence. A failed Git commit or worktree
// sync must leave Head empty, even if the model produced valid contracts.
func (c *Context) confirmedPlanCheckpoint(milestone string, result *drivers.Result) *state.PlanCheckpoint {
	if result == nil || !result.Settled || result.ExitCode == nil || *result.ExitCode != 0 ||
		result.TimedOut || result.Cancelled || result.Truncated || result.Signal != "" ||
		result.SpawnError != "" || result.CleanupError != "" || result.ErrorCode != "" ||
		(result.ErrorMessage != nil && *result.ErrorMessage != "") || len(c.State.ContractFingerprints) == 0 {
		return nil
	}
	if result.Session != nil && (result.Session.RequestID == "" || !safeSessionOutcome(*result, result.Session.RequestID) || result.Session.Outcome != "completed") {
		return nil
	}
	return &state.PlanCheckpoint{
		Version: planCheckpointVersion, Milestone: milestone,
		Inputs: c.planInputIdentity(milestone),
		Receipt: sessionHash([]any{result.Harness, result.Provider, result.Model, result.SessionID, result.Session,
			*result.ExitCode, result.Settled, result.DurationMS}),
	}
}

var planCheckpointAncestor = func(cwd, ancestor, head string) bool {
	cmd := exec.Command("git", "-C", cwd, "merge-base", "--is-ancestor", ancestor, head)
	cmd.Env = childenv.EnvSlice(childenv.GateEnv(childenv.OSLookupEnv, nil))
	return cmd.Run() == nil
}

// The current commit may be the Plan anchor or an orchestrator-confirmed packet
// delivery. An unrecognized external commit is a new baseline that needs Plan.
// Uncommitted Execute work deliberately is not part of this planning identity.
func (c *Context) knownPlanBaseline(checkpoint *state.PlanCheckpoint) bool {
	head, ok := verify.GetBaseRef(c.Cwd)
	if !ok || head == verify.EmptyTree {
		return false
	}
	if head == checkpoint.Head {
		return true
	}
	for _, packet := range c.State.Packets {
		commit, ok := packet.Commit.(map[string]any)
		if packet.Status == "completed" && ok && commit["sha"] == head {
			return planCheckpointAncestor(c.Cwd, checkpoint.Head, head)
		}
	}
	return false
}

// needsPlan is deliberately conservative for legacy, incomplete and changed
// proofs. Unknown invocations are reconciled before the run reaches this policy.
func (c *Context) needsPlan(milestone string) (bool, string) {
	progress := c.State.Milestones[milestone].Plan
	if progress == nil || progress.Checkpoint == nil {
		return true, "no trusted Plan checkpoint"
	}
	checkpoint := progress.Checkpoint
	if progress.Status != "passed" || progress.FinishedAt == "" ||
		checkpoint.Version != planCheckpointVersion || checkpoint.Milestone != milestone ||
		checkpoint.Receipt == "" || checkpoint.Head == "" {
		return true, "Plan checkpoint is incomplete or invalid"
	}
	if checkpoint.Inputs != c.planInputIdentity(milestone) {
		return true, "Plan contracts or verification policy changed"
	}
	if !c.knownPlanBaseline(checkpoint) {
		return true, "repository baseline is not a confirmed Plan or packet delivery"
	}
	return false, "confirmed Plan remains valid; continuing the current execution"
}

// Called only after all existing persistence/sync checks succeed. A plain legacy
// passed status cannot be promoted into a trusted checkpoint by this function.
func (c *Context) persistPlanCheckpoints(head string) {
	if head == "" || head == verify.EmptyTree {
		return
	}
	for id, milestone := range c.State.Milestones {
		progress := milestone.Plan
		if progress == nil || progress.Status != "passed" || progress.Checkpoint == nil {
			continue
		}
		checkpoint := progress.Checkpoint
		if checkpoint.Version == planCheckpointVersion && checkpoint.Milestone == id &&
			checkpoint.Receipt != "" && checkpoint.Head == "" && checkpoint.Inputs == c.planInputIdentity(id) {
			checkpoint.Head = head
		}
	}
}
