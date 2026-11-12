package machine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/require"
)

func TestExecutionDirectoryProposalAdmission(t *testing.T) {
	proposal, err := parseExecutionDirectory("ordinary completion")
	require.NoError(t, err)
	require.Nil(t, proposal)
	for _, ordinary := range []string{`Documented "flywheelExecutionDirectory" in the guide.`, `{"example":{"flywheelExecutionDirectory":{}}}`} {
		proposal, err := parseExecutionDirectory(ordinary)
		require.NoError(t, err)
		require.Nil(t, proposal)
	}
	for _, text := range []string{
		`{"flywheelExecutionDirectory":`,
		`{"flywheelExecutionDirectory":{}}`,
		`{"flywheelExecutionDirectory":{"cwd":"relative","branch":"b"}}`,
		`{"flywheelExecutionDirectory":{"cwd":"/x","branch":"b","extra":true}}`,
		`{"flywheelExecutionDirectory":{}}` + strings.Repeat(" ", 16385),
	} {
		_, err := parseExecutionDirectory(text)
		require.Error(t, err)
	}
	proposal, err = parseExecutionDirectory(`{"flywheelExecutionDirectory":{"cwd":"/x","branch":"b"}}`)
	require.NoError(t, err)
	fx, c := sessionFixture(t)
	packet := c.PacketList[0]
	result := drivers.MockResult(nil)
	require.ErrorContains(t, c.adoptExecutionDirectory(packet, proposal, result), "caller-managed")
	off := false
	c.Config.Worktree.Enabled = &off
	require.ErrorContains(t, c.adoptExecutionDirectory(packet, proposal, drivers.Result{}), "quiescent")
	ps := c.State.Packets[packet.PacketID]
	ps.Worktree = &state.WorktreeInfo{Mode: "worktree"}
	require.ErrorContains(t, c.adoptExecutionDirectory(packet, proposal, result), "already has")
	ps.Worktree = nil
	require.Error(t, c.adoptExecutionDirectory(packet, proposal, result))
	proposal.Directory.Cwd = fx.root
	require.ErrorContains(t, c.adoptExecutionDirectory(packet, proposal, result), "independent")
	other := newGitFixture(t)
	proposal.Directory.Cwd = other.root
	require.ErrorContains(t, c.adoptExecutionDirectory(packet, proposal, result), "independent")
	unit := filepath.Join(t.TempDir(), "caller-unit")
	gitCmd(t, fx.root, "worktree", "add", "-b", "caller", unit)
	proposal.Directory.Cwd = unit
	require.ErrorContains(t, c.adoptExecutionDirectory(packet, proposal, result), "baseline")
	base, ok := verify.GetBaseRef(fx.root)
	require.True(t, ok)
	ps.BaseRef = &base
	require.ErrorContains(t, c.adoptExecutionDirectory(packet, proposal, result), "branch")
	proposal.Directory.Branch = "caller"
	writeFile(t, filepath.Join(unit, "unreviewed"), "preserve")
	require.ErrorContains(t, c.adoptExecutionDirectory(packet, proposal, result), "clean")
	require.NoError(t, os.Remove(filepath.Join(unit, "unreviewed")))
	physical, err := filepath.EvalSymlinks(unit)
	require.NoError(t, err)
	c.State.Packets["M01-T02"].Worktree = &state.WorktreeInfo{Path: physical, Mode: "worktree"}
	require.ErrorContains(t, c.adoptExecutionDirectory(packet, proposal, result), "belongs")
	c.State.Packets["M01-T02"].Worktree = nil
	old := deliveryRun
	t.Cleanup(func() { deliveryRun = old })
	deliveryRun = func(context.Context, string, string, ...string) (string, error) {
		return "", errors.New("inspection unavailable")
	}
	require.ErrorContains(t, c.adoptExecutionDirectory(packet, proposal, result), "branch")
	deliveryRun = func(_ context.Context, _ string, _ string, args ...string) (string, error) {
		if args[0] == "symbolic-ref" {
			return "caller", nil
		}
		return "", errors.New("status unavailable")
	}
	require.ErrorContains(t, c.adoptExecutionDirectory(packet, proposal, result), "clean")
	deliveryRun = old
	ps.Worktree = &state.WorktreeInfo{Mode: "in-place", Reason: "previous automatic setup was unavailable"}
	require.NoError(t, c.adoptExecutionDirectory(packet, proposal, result))
	require.True(t, ps.Worktree.External, "a failed in-place fallback is not an owned Worktree")
}

func TestMalformedExecutionProposalCannotBeAnImplementationSuccess(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	off := false
	c.Config.Worktree.Enabled = &off
	d := happyDrivers()
	d.execFn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) { r.Text = `{"flywheelExecutionDirectory":` })
	}
	c.Drivers = d.set()
	outcome, err := c.executeStep(c.PacketList[0])
	require.NoError(t, err)
	require.Equal(t, "INVALID_REQUEST", string(outcome.ErrorCode))
	require.Contains(t, c.State.Packets[c.PacketList[0].PacketID].LastFeedback.(map[string]any)["details"], "invalid or oversized execution directory proposal")
	require.False(t, outcome.Success)
	require.False(t, outcome.Relocated)
	require.Nil(t, c.State.Packets[c.PacketList[0].PacketID].Worktree)
}

func TestBadDirectoryProposalIsConsumedAndBoundedRepairResumesTheSameSession(t *testing.T) {
	_, c := sessionFixture(t)
	off := false
	c.Config.Worktree.Enabled = &off
	base := drivers.NewMockDriver(nil)
	d := happyDrivers()
	calls, thread := 0, ""
	d.execFn = func(a drivers.CallArgs) drivers.Result {
		calls++
		if calls == 1 {
			result := sessionResult(a, drivers.MockResult(func(r *drivers.Result) { r.Text = `{"flywheelExecutionDirectory":` }))
			thread = result.SessionID
			return result
		}
		require.Equal(t, thread, a.SessionID)
		require.Contains(t, a.Prompt, "invalid or oversized execution directory proposal")
		return sessionResult(a, base.Execute(a))
	}
	c.Drivers = d.set()
	result, err := c.runExecuteLoop(c.PacketList[0])
	require.NoError(t, err)
	require.True(t, result.Green)
	require.Equal(t, 2, calls)
	require.Equal(t, 1, c.State.Packets[c.PacketList[0].PacketID].FailedAttempts)
	require.Equal(t, "consumed", c.State.Sessions.Records["execute/M01-T01"].Last.Status)
}

func TestExecutionDirectoryProposalCannotHideCleanupOrPersistenceFailure(t *testing.T) {
	for _, cleanup := range []bool{true, false} {
		t.Run(map[bool]string{true: "cleanup", false: "CAS"}[cleanup], func(t *testing.T) {
			fx := newGitFixture(t)
			c := unitTestContext(t, fx)
			off := false
			c.Config.Worktree.Enabled = &off
			d := happyDrivers()
			d.execFn = func(drivers.CallArgs) drivers.Result {
				if !cleanup {
					c.revision--
				}
				return drivers.MockResult(func(r *drivers.Result) {
					r.Text = `{"flywheelExecutionDirectory":`
					if cleanup {
						r.CleanupError = "unconfirmed child"
					}
				})
			}
			c.Drivers = d.set()
			result, err := c.executeStep(c.PacketList[0])
			require.Error(t, err)
			require.False(t, result.Relocated)
			require.Nil(t, c.State.Packets[c.PacketList[0].PacketID].Worktree)
		})
	}
}

func TestExecutionDirectoryBindingCASFailureDoesNotPublishSuccess(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	off := false
	c.Config.Worktree.Enabled = &off
	unit := filepath.Join(t.TempDir(), "candidate")
	gitCmd(t, fx.root, "worktree", "add", "-b", "binding-cas", unit)
	raw, err := json.Marshal(map[string]any{"flywheelExecutionDirectory": map[string]string{"cwd": unit, "branch": "binding-cas"}})
	require.NoError(t, err)
	d := happyDrivers()
	d.execFn = func(drivers.CallArgs) drivers.Result {
		c.revision--
		return drivers.MockResult(func(r *drivers.Result) { r.Text = string(raw) })
	}
	c.Drivers = d.set()
	result, err := c.executeStep(c.PacketList[0])
	require.Error(t, err)
	require.False(t, result.Relocated)
	require.Nil(t, loadFixtureState(t, fx).Packets[c.PacketList[0].PacketID].Worktree)
	require.DirExists(t, unit, "failed publication cannot destroy the caller's checkout")
}

func TestExecutionDirectoryReservationsPreventParallelAndSequentialSharing(t *testing.T) {
	_, c := sessionFixture(t)
	c.State.Packets["M01-T02"].Worktree = &state.WorktreeInfo{Path: "/owned", Mode: "worktree"}
	require.Error(t, c.reserveExecutionDirectory("M01-T01", "/owned"))
	group := &parallelGroup{owner: c}
	worker := *c
	worker.parallel = &parallelLane{group: group, id: "M01-T01"}
	require.NoError(t, worker.reserveExecutionDirectory("M01-T01", "/new"))
	require.NoError(t, worker.reserveExecutionDirectory("M01-T01", "/new"))
	require.Error(t, worker.reserveExecutionDirectory("M01-T02", "/new"))
	require.Error(t, worker.reserveExecutionDirectory("M01-T01", "/owned"))
}

func TestSharedParallelIntegrationRechecksAfterQuiescence(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	off := false
	c.Config.Worktree.Enabled = &off
	packet := c.PacketList[0]
	ps := c.State.Packets[packet.PacketID]
	ps.Parallel = true
	require.ErrorContains(t, c.prepareParallelIntegration(packet), "failed acceptance")
	d := drivers.NewMockDriver(nil)
	d.Execute(drivers.CallArgs{Cwd: fx.root, TaskDir: fx.taskDir, PacketID: packet.PacketID})
	// Exercise the normal execute path to retain the report identity L1 requires.
	_, err := c.executeStep(packet)
	require.NoError(t, err)
	require.NoError(t, c.prepareParallelIntegration(packet))
	require.False(t, ps.Parallel)
}

func TestExternalExecutionRejectsOneBranchCheckedOutInMultipleDirectories(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	off := false
	c.Config.Worktree.Enabled = &off
	base, ok := verify.GetBaseRef(fx.root)
	require.True(t, ok)
	packet := c.PacketList[0]
	c.State.Packets[packet.PacketID].BaseRef = &base
	first, second := filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")
	gitCmd(t, fx.root, "worktree", "add", "-b", "shared-branch", first)
	gitCmd(t, fx.root, "worktree", "add", "--force", second, "shared-branch")
	proposal := &executionDirectoryProposal{}
	proposal.Directory.Cwd, proposal.Directory.Branch = first, "shared-branch"
	require.ErrorContains(t, c.adoptExecutionDirectory(packet, proposal, drivers.MockResult(nil)), "branch is shared")
	require.Nil(t, c.State.Packets[packet.PacketID].Worktree)
}
