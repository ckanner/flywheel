package machine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/require"
)

func sharedFixture(t *testing.T, accepted bool) (fixture, *Context) {
	t.Helper()
	fx, cfg := parallelFixture(t)
	off := false
	cfg.Worktree.Enabled = &off
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set()})
	require.NoError(t, err)
	head, ok := verify.GetBaseRef(fx.root)
	require.True(t, ok)
	ids := []string{"M01-T01", "M01-T02"}
	c.State.SharedExecution = &state.SharedExecution{Cwd: c.Cwd, BaseRef: head, Packets: ids}
	for _, id := range ids {
		c.State.Packets[id].BaseRef = &head
		c.State.Packets[id].SharedPeers = append([]string{}, ids...)
	}
	c.save()
	require.NoError(t, c.err())
	if accepted {
		for _, p := range c.PacketList {
			out, err := c.executeStep(p)
			require.NoError(t, err)
			require.True(t, out.Success)
		}
		for _, p := range c.PacketList {
			require.True(t, c.l1Step(p).Pass)
			out, err := c.reviewStep(p, nil)
			require.NoError(t, err)
			require.True(t, out.Pass)
		}
	}
	return fx, c
}

func TestSharedCommitFaultsNeverPublishPartialCompletion(t *testing.T) {
	for _, fault := range []string{"unaccepted", "cancel", "read", "commit", "index", "candidate", "report", "CAS"} {
		t.Run(fault, func(t *testing.T) {
			fx, c := sharedFixture(t, true)
			switch fault {
			case "unaccepted":
				c.State.Packets["M01-T02"].Evidence.Review = nil
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.RunContext = ctx
			case "read":
				old := snapshotGitFiles
				t.Cleanup(func() { snapshotGitFiles = old })
				calls := 0
				snapshotGitFiles = func(cwd string) ([]byte, error) {
					calls++
					if calls == 3 {
						return nil, errors.New("late snapshot read failure")
					}
					return old(cwd)
				}
			case "commit":
				installCommitHook(t, fx, "exit 1")
			case "index":
				installCommitHook(t, fx, "blob=$(printf 'unverified index\\n' | git hash-object -w --stdin)\ngit update-index --add --cacheinfo 100644 \"$blob\" build/M01-T01.txt")
			case "candidate":
				installCommitHook(t, fx, "printf 'changed by hook\\n' > build/M01-T01.txt\ngit add -- build/M01-T01.txt")
			case "report":
				rel, err := filepath.Rel(fx.root, filepath.Join(fx.taskDir, "reports", "M01-T01.done.json"))
				require.NoError(t, err)
				installCommitHook(t, fx, "printf '{}' > '"+rel+"'\ngit add -- '"+rel+"'")
			case "CAS":
				c.revision--
			}
			require.Error(t, c.commitSharedExecution(c.PacketList))
			loaded := loadFixtureState(t, fx)
			require.NotNil(t, loaded.SharedExecution)
			for _, id := range []string{"M01-T01", "M01-T02"} {
				require.NotEqual(t, "completed", loaded.Packets[id].Status)
				require.NotEqual(t, "completed", c.State.Packets[id].Status)
				require.FileExists(t, filepath.Join(fx.root, "build", id+".txt"))
			}
		})
	}
}

func TestSharedCoordinatorRejectsChangedContractsAndUnknownCalls(t *testing.T) {
	for _, fault := range []string{"cwd", "missing", "scope", "unknown", "CAS", "inbox", "commit"} {
		t.Run(fault, func(t *testing.T) {
			fx, c := sharedFixture(t, fault == "commit")
			switch fault {
			case "cwd":
				c.State.SharedExecution.Cwd = "/wrong"
			case "missing":
				delete(c.Packets, "M01-T02")
			case "scope":
				c.Packets["M01-T02"].Schedule = nil
			case "unknown":
				c.State.PlanSession = map[string]any{"outcome-unknown": true}
			case "CAS":
				c.State.Packets["M01-T01"].SharedPeers = nil
				c.revision--
			case "inbox":
				c.State.Packets["M01-T01"].Status = "inbox"
			case "commit":
				installCommitHook(t, fx, "exit 1")
			}
			stopped, err := c.processSharedExecution("")
			if fault == "inbox" {
				require.True(t, stopped)
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.NotNil(t, c.State.SharedExecution)
			if err != nil && c.fatal == nil {
				require.NotEmpty(t, c.State.SharedExecution.LastError)
			}
		})
	}
}

func TestSharedScopeProjectionPreservesWholeBatchBoundaries(t *testing.T) {
	fx, c := sharedFixture(t, false)
	p := c.PacketList[0]
	for _, id := range []string{"M01-T01", "M01-T02"} {
		writeFile(t, filepath.Join(fx.root, "build", id+".txt"), id)
	}
	selected, violations := c.sharedBoundaryFiles(p, nil)
	require.Equal(t, []string{"build/M01-T01.txt"}, selected)
	require.Empty(t, violations)
	writeFile(t, filepath.Join(fx.root, "outside.txt"), "not declared")
	_, violations = c.sharedBoundaryFiles(p, nil)
	require.NotEmpty(t, violations)
	require.NoError(t, os.Remove(filepath.Join(fx.root, "outside.txt")))
	p.Boundaries.MustNotTouch = []string{"build/M01-T02.txt"}
	_, violations = c.sharedBoundaryFiles(p, nil)
	require.NotEmpty(t, violations)
	p.Boundaries.MustNotTouch = nil
	c.State.SharedExecution.BaseRef = "missing-reference"
	_, violations = c.sharedBoundaryFiles(p, nil)
	require.Equal(t, "shared-diff-unavailable", violations[0].Type)
	c.Packets["M01-T02"].Schedule = nil
	_, violations = c.sharedBoundaryFiles(p, nil)
	require.Equal(t, "shared-scope-unavailable", violations[0].Type)
	_, err := c.sharedInvocationScope("review", p.PacketID)
	require.Error(t, err)
	_, err = c.invokeScoped("execute", p.PacketID, sessionArgs(c), func(drivers.CallArgs) drivers.Result {
		t.Fatal("a missing peer scope must not dispatch a model")
		return drivers.Result{}
	})
	require.Error(t, err)
	c.Packets["M01-T02"].Schedule = &contract.PacketSchedule{Writes: []string{"build/M01-T02.txt"}}
	c.State.Packets["M01-T02"].Worktree = &state.WorktreeInfo{Mode: "worktree", Path: "/external"}
	_, _ = c.sharedBoundaryFiles(p, nil)
	_, err = c.sharedInvocationScope("review", p.PacketID)
	require.NoError(t, err)
}

func TestSharedL1RejectsAnUndeclaredChangeBeforeReviewOrDelivery(t *testing.T) {
	fx, c := sharedFixture(t, false)
	for _, packet := range c.PacketList {
		result, err := c.executeStep(packet)
		require.NoError(t, err)
		require.True(t, result.Success)
	}
	writeFile(t, filepath.Join(fx.root, "unclaimed.txt"), "outside every declared task scope")
	p := c.PacketList[0]
	gate := c.l1Step(p)
	require.False(t, gate.Pass)
	require.False(t, gate.Boundary.Pass)
	require.Contains(t, gate.Boundary.Violations, verify.BoundaryViolation{Type: "shared-write-scope", Path: "unclaimed.txt"})
	require.False(t, c.unitAccepted(p))
	require.Error(t, c.commitSharedExecution(c.PacketList))
	st := loadFixtureState(t, fx)
	require.Equal(t, "l1-failed", st.Packets[p.PacketID].Status)
	require.NotEqual(t, "completed", st.Packets["M01-T02"].Status)
	require.FileExists(t, filepath.Join(fx.root, "unclaimed.txt"), "rejection preserves the candidate for inspection")
}

func TestSharedDirectoryRegistrationCannotAbandonChangesInTheSourceCheckout(t *testing.T) {
	fx, c := sharedFixture(t, false)
	unit := filepath.Join(t.TempDir(), "target")
	gitCmd(t, fx.root, "worktree", "add", "-b", "adopt-guard", unit)
	proposal := &executionDirectoryProposal{}
	proposal.Directory.Cwd, proposal.Directory.Branch = unit, "adopt-guard"
	p := c.PacketList[0]
	changed := filepath.Join(fx.root, "build", "M01-T01.txt")
	writeFile(t, changed, "retain this implementation")
	require.ErrorContains(t, c.adoptExecutionDirectory(p, proposal, drivers.MockResult(nil)), "before modifying")
	require.Nil(t, c.State.Packets[p.PacketID].Worktree)
	require.FileExists(t, changed)
	require.NoError(t, os.Remove(changed))
	c.State.SharedExecution.BaseRef = "missing-base"
	require.ErrorContains(t, c.adoptExecutionDirectory(p, proposal, drivers.MockResult(nil)), "cannot inspect")
	require.Nil(t, c.State.Packets[p.PacketID].Worktree)
}

func TestSharedReceiptScopeIgnoresPeersButDetectsOwnCandidateChanges(t *testing.T) {
	fx, c := sharedFixture(t, false)
	enableSessionPolicy(c)
	packet := c.PacketList[0]
	args := sessionArgs(c)
	writeFile(t, filepath.Join(fx.root, "build", "M01-T01.txt"), "first")
	result, err := c.invokeScoped("execute", packet.PacketID, args, func(a drivers.CallArgs) drivers.Result {
		require.Equal(t, packet.Schedule.Writes, a.CandidateScope)
		return sessionResult(a, drivers.MockResult(nil))
	})
	require.NoError(t, err)
	writeFile(t, filepath.Join(fx.root, "build", "M01-T02.txt"), "peer changed after receipt")
	_, err = c.invokeScoped("execute", packet.PacketID, args, func(drivers.CallArgs) drivers.Result {
		t.Fatal("known receipt must not redispatch")
		return drivers.Result{}
	})
	require.NoError(t, err)
	writeFile(t, filepath.Join(fx.root, "build", "M01-T01.txt"), "own scope changed")
	_, err = c.invokeScoped("execute", packet.PacketID, args, func(drivers.CallArgs) drivers.Result {
		t.Fatal("changed scope must not redispatch")
		return drivers.Result{}
	})
	require.ErrorContains(t, err, "candidate changed")
	require.True(t, result.Settled)
}

func TestSharedBatchKeepsSamplingAndManualDeliveryBoundaries(t *testing.T) {
	_, c := sharedFixture(t, false)
	n := 2
	c.Config.DefectInjection.EveryNUnits = &n
	_, err := c.maybeDefectInjection(c.PacketList[1])
	require.NoError(t, err)
	require.Contains(t, c.State.Packets["M01-T02"].Evidence.DefectInjection.(map[string]any)["detail"], "sampled")
	require.ErrorContains(t, c.admitConsoleStep("integrate", "M01-T01"), "shared execution batch")
	require.ErrorContains(t, c.completeUnit(c.PacketList[0]), "whole-batch")
	c.State.PlanSession = map[string]any{"outcome-unknown": true}
	require.Error(t, c.admitConsoleStep("execute", "M01-T01"))
	require.True(t, strings.Contains(c.sharedPrompt("M01-T01"), "M01-T02"))
}

func TestSharedHandoffChecksCurrentScopeBeforeResumingTheOriginalSession(t *testing.T) {
	_, c := sharedFixture(t, false)
	enableSessionPolicy(c)
	binding := c.Config.Routes["default"]
	binding.Provider = "offline"
	c.Config.Routes["default"] = binding
	c.Drivers = sessionDriverSet(happyDrivers())
	result, err := c.executeStep(c.PacketList[0])
	require.NoError(t, err)
	require.True(t, result.Success)
	r := c.State.Sessions.Records["execute/M01-T01"]
	_, err = c.handoffArgs(r)
	require.NoError(t, err)
	c.PacketList[0].Schedule.Writes = []string{"different/"}
	_, err = c.handoffArgs(r)
	require.ErrorContains(t, err, "binding")
	c.PacketList[0].Schedule = nil
	_, err = c.handoffArgs(r)
	require.ErrorContains(t, err, "scope")
}
