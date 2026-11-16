package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func reviewedFingerprintFixture(t *testing.T) (fixture, *Context) {
	t.Helper()
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	design := planning.Ref{Revision: 1, Digest: strings.Repeat("a", 64)}
	c.State.Planning = &planning.State{CurrentDesign: &design}
	c.State.ContractFingerprints = nil
	require.NoError(t, c.reconcileContractRevisions())
	for _, p := range c.PacketList {
		ps := c.State.Packets[p.PacketID]
		ps.Status = "completed"
		ps.Evidence.L1 = map[string]any{"pass": true, "identity": "accepted-candidate"}
	}
	c.State.Milestones["M-01"] = state.MilestoneState{Status: "closed"}
	c.State.Status = "completed"
	return fx, c
}

func TestProgressiveMilestoneProjectionPreservesDeliveredPacketContracts(t *testing.T) {
	fx, c := reviewedFingerprintFixture(t)
	before := jsonClone(c.State.ContractFingerprints)
	spec := strings.Replace(fixtureSpec, "requirements:\n", "  - id: M-02\n    title: Later milestone\n    gate: Later artifacts verified\n    planning: deferred\nrequirements:\n", 1)
	spec = strings.Replace(spec, "---\n\n# Mini spec", "  - id: R-03\n    title: Third thing works\n    milestone: M-02\n    verification: test -f build/M02-T01.txt\n---\n\n# Mini spec", 1)
	writeFile(t, filepath.Join(fx.taskDir, "spec.md"), spec)
	require.NoError(t, c.reloadPackets())
	require.Equal(t, before, c.State.ContractFingerprints)
	require.Equal(t, "closed", c.State.Milestones["M-01"].Status)
	for _, p := range c.PacketList {
		require.Equal(t, "completed", c.State.Packets[p.PacketID].Status)
	}

	ref := planning.ActivationRef{Version: 1, Design: *c.State.Planning.CurrentDesign,
		Plans: []planning.Ref{{Revision: 1, Digest: strings.Repeat("b", 64)}, {Revision: 2, Digest: strings.Repeat("c", 64)}}}
	rawRef, err := json.Marshal(ref)
	require.NoError(t, err)
	spec = strings.Replace(spec, "planning: deferred", "planning: ready", 1)
	spec = strings.Replace(spec, "spec-version: 1\n", "spec-version: 1\nplanning-ref: "+string(rawRef)+"\n", 1)
	writeFile(t, filepath.Join(fx.taskDir, "spec.md"), spec)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M02-T01.md"), fixturePacket("M02-T01", "R-03"))
	require.NoError(t, c.reloadPackets())
	for _, id := range []string{"M01-T01", "M01-T02"} {
		require.Equal(t, before[id], c.State.ContractFingerprints[id])
		require.Equal(t, "completed", c.State.Packets[id].Status)
		require.Empty(t, c.State.Packets[id].EvidenceHistory)
		require.NotNil(t, c.State.Packets[id].Evidence.L1)
	}
	require.Equal(t, "closed", c.State.Milestones["M-01"].Status)
	require.Equal(t, "pending", c.State.Packets["M02-T01"].Status)
	require.NotEmpty(t, c.State.ContractFingerprints["M02-T01"])
}

func TestReviewedContractChangesStillReopenAffectedDeliveredPackets(t *testing.T) {
	for name, mutate := range map[string]func(*testing.T, fixture, *Context){
		"requirement": func(_ *testing.T, _ fixture, c *Context) {
			c.Spec.Requirements[0].Verification = "go test ./..."
		},
		"milestone gate": func(_ *testing.T, _ fixture, c *Context) {
			c.Spec.Milestones[0].Gate = "Verify retries and rollback"
		},
		"packet instructions": func(t *testing.T, fx fixture, _ *Context) {
			appendPacketRevision(t, fx, "M01-T01")
		},
		"root design": func(_ *testing.T, _ fixture, c *Context) {
			c.State.Planning.CurrentDesign = &planning.Ref{Revision: 2, Digest: strings.Repeat("d", 64)}
		},
		"verification policy": func(_ *testing.T, _ fixture, c *Context) {
			c.Config.Verify.CommandTimeoutMs++
		},
	} {
		t.Run(name, func(t *testing.T) {
			fx, c := reviewedFingerprintFixture(t)
			old := c.State.ContractFingerprints["M01-T01"]
			mutate(t, fx, c)
			require.NoError(t, c.reconcileContractRevisions())
			require.NotEqual(t, old, c.State.ContractFingerprints["M01-T01"])
			require.Equal(t, "pending", c.State.Packets["M01-T01"].Status)
			require.Equal(t, "pending", c.State.Milestones["M-01"].Status)
			require.Equal(t, "pending", c.State.Status)
			require.NotEmpty(t, c.State.Packets["M01-T01"].EvidenceHistory)
			if name == "requirement" || name == "packet instructions" {
				require.Equal(t, "completed", c.State.Packets["M01-T02"].Status)
			}
		})
	}
}

func TestReviewedExportsRemainGuardedInsideExecutionWorktree(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	c.State.Planning = &planning.State{}
	rel := "docs/minitask/planning/design-000001.md"
	writeFile(t, filepath.Join(fx.root, rel), "# Approved immutable design\n")
	gitCmd(t, fx.root, "add", "--", rel)
	gitCmd(t, fx.root, "commit", "-qm", "docs: approved design snapshot")
	worktree := filepath.Join(t.TempDir(), "execution")
	gitCmd(t, fx.root, "worktree", "add", "-b", "execution", worktree)
	before, err := c.candidateFingerprint(worktree)
	require.NoError(t, err)
	require.False(t, c.snapshotExcluded(rel))
	require.True(t, c.snapshotExcluded("docs/minitask/.control/queued-request.json"))
	writeFile(t, filepath.Join(worktree, rel), "# Forged design that grants extra scope\n")
	after, err := c.candidateFingerprint(worktree)
	require.NoError(t, err)
	require.NotEqual(t, before, after, "worker copies of planning exports are part of the candidate identity")
	require.Equal(t, "# Approved immutable design\n", string(mustReadFile(t, filepath.Join(fx.root, rel))))
	_, violations := c.partitionChangedFiles([]string{rel})
	require.Equal(t, []boundaryViolationLite{{Type: "contract-file", Path: rel}}, violations)
}

func mustReadFile(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(name)
	require.NoError(t, err)
	return raw
}

func TestReviewedRequirementOrderingDoesNotInvalidateUnchangedPacket(t *testing.T) {
	_, c := reviewedFingerprintFixture(t)
	p := *c.PacketList[0]
	p.Requirements = []string{"R-01", "R-02"}
	before, err := json.Marshal(c.packetSpecIdentity(&p, nil))
	require.NoError(t, err)
	c.Spec.Requirements[0], c.Spec.Requirements[1] = c.Spec.Requirements[1], c.Spec.Requirements[0]
	c.Spec.Milestones[0].Planning = "ready"
	c.Spec.Requirements = append(c.Spec.Requirements, contract.Requirement{ID: "R-99", Title: "Unrelated future requirement", Milestone: "M-99"})
	after, err := json.Marshal(c.packetSpecIdentity(&p, nil))
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
}
