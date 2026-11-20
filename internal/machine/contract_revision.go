package machine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/planning"
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
	scheduler := c.Config.Scheduler
	c.State.Scheduler = &scheduler
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
		fresh[p.PacketID] = c.packetContractFingerprint(p, spec, packet)
	}
	for _, m := range c.Spec.Milestones {
		if _, ok := c.State.Milestones[m.ID]; !ok {
			c.State.Milestones[m.ID] = state.MilestoneState{Status: "pending"}
		}
	}
	previousSet, _ := json.Marshal(c.State.ContractFingerprints)
	freshSet, _ := json.Marshal(fresh)
	setChanged := len(c.State.ContractFingerprints) > 0 && string(previousSet) != string(freshSet)
	if setChanged {
		for id, ms := range c.State.Milestones {
			if ms.Plan != nil {
				ms.Plan.Status = "stale"
				c.State.Milestones[id] = ms
			}
		}
	}
	for _, p := range c.PacketList {
		ps := c.State.Packets[p.PacketID]
		ps.Schedule = p.Schedule
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

// Read-only displays use the same identity as execution, including its exact
// verification policy. A packet-only digest cannot establish reviewed scope.
func (c *Context) packetContractFingerprint(packet *contract.Packet, spec, rawPacket []byte) string {
	raw, _ := json.Marshal(struct {
		Spec            any
		Packet          string
		Verify          any
		DefectInjection any `json:"DefectInjection,omitempty"`
	}{c.packetSpecIdentity(packet, spec), string(rawPacket), c.Config.Verify, c.workflowDefectPolicy()})
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

// A progressive child plan changes the Spec projection without revising an
// earlier packet's approved contract. Bind that packet to the immutable root,
// its own requirements and milestone gate, not another milestone's expansion
// or the aggregate planning-ref. Legacy tasks retain byte-for-byte Spec checks.
func (c *Context) packetSpecIdentity(packet *contract.Packet, raw []byte) any {
	if c.State.Planning == nil {
		return string(raw)
	}
	requirements := map[string]contract.Requirement{}
	for _, requirement := range c.Spec.Requirements {
		if slices.Contains(packet.Requirements, requirement.ID) {
			requirements[requirement.ID] = requirement
		}
	}
	milestoneID, _ := contract.MilestoneOfPacket(packet.PacketID) // schema-validated packet ID
	var milestone contract.Milestone
	for _, m := range c.Spec.Milestones {
		if m.ID == milestoneID {
			milestone = m
			milestone.Planning = "" // deferred/ready describes projection availability
			break
		}
	}
	return struct {
		Version      int
		Design       *planning.Ref
		Requirements map[string]contract.Requirement
		Milestone    contract.Milestone
	}{c.Spec.SpecVersion, c.State.Planning.CurrentDesign, requirements, milestone}
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
