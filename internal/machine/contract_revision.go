package machine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/state"
)

// Contract fingerprints survive restarts independently of candidate identities.
// Later legitimate units change the repository; that must not invalidate an
// already delivered unit. A changed Spec/packet/check policy must reopen it.
var readContractRevision = os.ReadFile

// A dynamic scheduler may revisit a closed milestone after a later Plan revises
// its contracts. Bound that feedback loop: one initial visit + two reopenings.
const maxMilestoneVisits = 3

func (c *Context) reconcileContractRevisions() error {
	spec, err := readContractRevision(filepath.Join(c.TaskDir, "spec.md"))
	if err != nil {
		return err
	}
	fresh := make(map[string]string, len(c.PacketList))
	for _, p := range c.PacketList {
		packet, err := readContractRevision(filepath.Join(c.TaskDir, "task-packet-"+p.PacketID+".md"))
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(struct {
			Spec, Packet string
			Verify       any
		}{string(spec), string(packet), c.Config.Verify})
		h := sha256.Sum256(raw)
		fresh[p.PacketID] = hex.EncodeToString(h[:])
	}
	for _, m := range c.Spec.Milestones {
		if _, ok := c.State.Milestones[m.ID]; !ok {
			c.State.Milestones[m.ID] = state.MilestoneState{Status: "pending"}
		}
	}
	previousSet, _ := json.Marshal(c.State.ContractFingerprints)
	freshSet, _ := json.Marshal(fresh)
	setChanged := len(c.State.ContractFingerprints) > 0 && string(previousSet) != string(freshSet)
	for _, p := range c.PacketList {
		ps := c.State.Packets[p.PacketID]
		if setChanged && ps.Status != "completed" && ps.BaseRef != nil {
			// Every paused candidate contains the whole contract directory. A
			// revision of a different packet must reach this worktree too; otherwise
			// its obsolete contract copy is mistaken for executor tampering.
			ps.ContractSyncPending = true
			invalidateEvidence(ps)
		}
		if c.State.ContractFingerprints[p.PacketID] == fresh[p.PacketID] {
			continue
		}
		if c.State.ContractFingerprints[p.PacketID] == "" && ps.Status != "completed" && c.State.Milestones[ps.Milestone].Status != "closed" {
			// Legacy unfinished state retains retry/interruption/approval history.
			// It has no reusable completed verdict; normal execute/review identity
			// checks still reject legacy proof that lacks a candidate identity.
			continue
		}
		wasCompleted := ps.Status == "completed"
		if ps.Status == "completed" && ps.Worktree == nil {
			// A delivered unit has no unfinished branch to preserve. Reopen it
			// from the current main HEAD, not from its obsolete delivery base.
			ps.BaseRef = nil
		}
		ps.ContractSyncPending = ps.BaseRef != nil
		invalidateEvidence(ps)
		ps.Status = "pending"
		if wasCompleted || c.State.ContractFingerprints[p.PacketID] != "" {
			ps.FailedAttempts = 0
			ps.LastFailureSignature = nil
		}
		ps.Route = p.Route
		ps.NeedsLiveCredentials = p.NeedsLiveCredentials
		ms := c.State.Milestones[ps.Milestone]
		ms.Status, ms.Gate, ms.ClosedAt = "pending", nil, nil
		c.State.Milestones[ps.Milestone] = ms
		if c.State.Status == "completed" {
			c.State.Status = "pending"
		}
	}
	c.State.ContractFingerprints = fresh
	return nil
}

func (c *Context) isPlannerContract(rel string) bool {
	prefix := filepath.ToSlash(c.taskDirPrefix())
	if !strings.HasPrefix(rel, prefix) {
		return false
	}
	inner := strings.TrimPrefix(rel, prefix)
	return inner != "flywheel.config.json" && contractFileRe.MatchString(inner)
}

// Plan may revise the task's frozen documents. It must not quietly edit code,
// external acceptance oracles, or live model/permission configuration. Such
// changes stay on disk for recovery and fail before any automatic commit.
func (c *Context) planScopeIdentity() (string, error) {
	return c.candidateFingerprint(c.Cwd, c.isPlannerContract)
}

var planRevisionFiles = func(cwd, from, to string) ([]byte, error) {
	cmd := exec.Command("git", "-C", cwd, "diff", "--name-only", "-z", from, to, "--")
	cmd.Env = childenv.EnvSlice(childenv.GateEnv(childenv.OSLookupEnv, nil))
	return cmd.Output()
}

func (c *Context) checkPlanReanchor(from, to string) error {
	files, err := planRevisionFiles(c.Cwd, from, to)
	if err != nil {
		return fmt.Errorf("cannot inspect plan checkpoint: %w", err)
	}
	for _, rel := range strings.Split(string(files), "\x00") {
		if rel != "" && !c.isPlannerContract(rel) && rel != c.taskDirPrefix()+"flywheel.config.json" && !c.snapshotExcluded(rel) {
			return fmt.Errorf("checkpoint also contains implementation change %q; refusing to erase it from the unit boundary", rel)
		}
	}
	return nil
}

// All current contract units, not retired historical state entries, must be
// complete before the task is complete. Reopened earlier milestones are visited
// by run's dynamic scheduler instead of being skipped by a stale for-loop index.
func (c *Context) contractsComplete() bool {
	for _, m := range c.Spec.Milestones {
		if c.State.Milestones[m.ID].Status != "closed" {
			return false
		}
	}
	for _, p := range c.PacketList {
		if c.State.Packets[p.PacketID].Status != "completed" {
			return false
		}
	}
	return true
}
