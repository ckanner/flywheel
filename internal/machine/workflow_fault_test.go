package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func executedQuick(t *testing.T, levels bool) (fixture, *Context, *scriptedDrivers, *contract.Packet) {
	t.Helper()
	fx := newQuickFixture(t, levels)
	sd := quickDrivers()
	c := quickContext(t, fx, sd)
	p := c.PacketList[0]
	r, err := c.runExecuteLoop(p)
	require.NoError(t, err)
	require.True(t, r.Green)
	return fx, c, sd, p
}

func TestQuickAcceptanceFailsClosedOnUnreadableOrChangedCandidate(t *testing.T) {
	for _, scenario := range []string{"before-read", "after-read", "command-change", "missing-reason", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			_, c, sd, p := executedQuick(t, true)
			original := snapshotGitFiles
			t.Cleanup(func() { snapshotGitFiles = original })
			switch scenario {
			case "before-read", "after-read":
				reads := 0
				snapshotGitFiles = func(cwd string) ([]byte, error) {
					reads++
					if (scenario == "before-read" && reads == 1) || (scenario == "after-read" && reads == 3) {
						return nil, os.ErrPermission
					}
					return original(cwd)
				}
			case "command-change":
				p.Acceptance.L2[0].Cmd = "printf 'unchecked mutation\\n' > build/M01-T01.txt"
				require.True(t, c.l1Step(p).Pass, "L1 initially checks the old candidate against this exact contract")
			case "missing-reason":
				p.Acceptance.L2 = nil
				delete(c.Spec.Workflow.NotApplicable, "L2")
				require.True(t, c.l1Step(p).Pass)
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.RunContext = ctx
			}
			require.False(t, c.quickGateStep(p))
			require.False(t, c.currentFinalGate(p))
			require.Empty(t, sd.log.review)
			gate := c.State.Packets[p.PacketID].Evidence.FinalGate.(map[string]any)
			require.Equal(t, false, gate["pass"])
			if scenario == "before-read" || scenario == "after-read" || scenario == "command-change" {
				require.NotEmpty(t, gate["identity-error"])
			}
		})
	}
}

func TestQuickFinalReviewRequiresAuthoritativeGitReads(t *testing.T) {
	for _, part := range []string{"inventory", "diff"} {
		t.Run(part, func(t *testing.T) {
			_, c, sd, p := executedQuick(t, false)
			oldFiles, oldDiff := reviewChangedFiles, reviewDiffText
			t.Cleanup(func() { reviewChangedFiles, reviewDiffText = oldFiles, oldDiff })
			if part == "inventory" {
				reviewChangedFiles = func(string, string) ([]string, bool) { return nil, false }
			} else {
				reviewDiffText = func(string, string, []string) (string, bool) { return "", false }
			}
			_, err := c.reviewStep(p, nil)
			require.ErrorContains(t, err, "authoritative")
			require.Empty(t, sd.log.review)
			require.Nil(t, c.State.Packets[p.PacketID].Evidence.Review)
		})
	}
}

func TestQuickFinalReviewStopsWhenAcceptanceCannotBeSaved(t *testing.T) {
	fx, c, sd, p := executedQuick(t, false)
	st := loadFixtureState(t, fx)
	_, err := state.Save(fx.taskDir, st, nil)
	require.NoError(t, err)
	_, err = c.reviewStep(p, nil)
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	require.Empty(t, sd.log.review)
	require.Nil(t, loadFixtureState(t, fx).Packets[p.PacketID].Evidence.FinalGate)
}

func TestQuickFinalReviewStopsOnRedIntegrationBeforeCallingModel(t *testing.T) {
	_, c, sd, p := executedQuick(t, true)
	p.Acceptance.L2[0].Cmd = "exit 1"
	require.True(t, c.l1Step(p).Pass)
	r, err := c.reviewStep(p, nil)
	require.NoError(t, err)
	require.False(t, r.Pass)
	require.Empty(t, sd.log.review)
	require.Nil(t, c.State.Packets[p.PacketID].Evidence.Review)
}

func TestQuickDefectSaveFailureStopsBeforeReview(t *testing.T) {
	fx, c, sd, p := executedQuick(t, false)
	c.Config.DefectInjection.EveryNUnits = new(1)
	require.True(t, c.l1Step(p).Pass)
	st := loadFixtureState(t, fx)
	_, err := state.Save(fx.taskDir, st, nil)
	require.NoError(t, err)
	r, err := c.reviewStep(p, nil)
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	require.True(t, r.Stopped)
	require.Empty(t, sd.log.review)
}

func TestQuickPreparedContractFailureAndCancellationNeverExecute(t *testing.T) {
	for _, scenario := range []string{"contract-hook", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			fx := newQuickFixture(t, false)
			sd := quickDrivers()
			c := quickContext(t, fx, sd)
			if scenario == "contract-hook" {
				appendPacketRevision(t, fx, "M01-T01")
				require.NoError(t, c.reloadPackets())
				installCommitHook(t, fx, "exit 1")
			} else {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.RunContext = ctx
			}
			_, err := c.run()
			require.Error(t, err)
			require.Empty(t, sd.log.exec)
			require.Empty(t, sd.log.review)
			require.Empty(t, sd.log.plan)
			require.NotEqual(t, "completed", loadFixtureState(t, fx).Status)
		})
	}
}

func TestQuickBoundedRepairAndRepeatedReturnUpgradeWithoutReset(t *testing.T) {
	for _, scenario := range []string{"bounded-execution", "repeated-review"} {
		t.Run(scenario, func(t *testing.T) {
			fx := newQuickFixture(t, false)
			sd := quickDrivers()
			c := quickContext(t, fx, sd)
			p := c.PacketList[0]
			ps := c.State.Packets[p.PacketID]
			if scenario == "bounded-execution" {
				ps.FailedAttempts = p.Escalation.MaxRetries
			} else {
				sd.reviewFn = func(drivers.CallArgs) drivers.Result { return quickVerdict("return", true) }
				c.Drivers = sd.set()
			}
			outcome, err := c.processPacket(p, processOpts{})
			require.NoError(t, err)
			require.Equal(t, "upgrade", outcome)
			require.Equal(t, "full", c.State.Workflow.Effective)
			require.Len(t, c.State.Workflow.Upgrades, 1)
			require.Greater(t, ps.FailedAttempts, 0)
			require.Empty(t, sd.log.plan, "only the scheduler invokes the next Full stage after the durable transition")
			c.save()
			reloaded, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: configFor(fx), Drivers: sd.set(), Mode: "auto"})
			require.NoError(t, err)
			require.Equal(t, ps.FailedAttempts, reloaded.State.Packets[p.PacketID].FailedAttempts)
			require.Equal(t, ps.Attempts, reloaded.State.Packets[p.PacketID].Attempts)
			require.Equal(t, c.State.Budget.Totals.Tokens, reloaded.State.Budget.Totals.Tokens)
			require.False(t, reloaded.quick())
		})
	}
}

func TestQuickGitFailurePreservesReviewedCandidate(t *testing.T) {
	fx, c, sd, p := executedQuick(t, false)
	r, err := c.reviewStep(p, nil)
	require.NoError(t, err)
	require.True(t, r.Pass)
	installCommitHook(t, fx, "exit 1")
	require.ErrorIs(t, c.completeUnit(p), ErrCommitFailed)
	require.NotEqual(t, "completed", c.State.Packets[p.PacketID].Status)
	require.FileExists(t, filepath.Join(c.unitCwd(p), "build", p.PacketID+".txt"))
	require.True(t, c.currentFinalGate(p))
	require.Len(t, sd.log.review, 1)
}

func TestQuickUpgradeAfterDeliveredCandidateKeepsHistory(t *testing.T) {
	fx := newQuickFixture(t, false)
	sd := quickDrivers()
	_, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	before := loadFixtureState(t, fx)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: configFor(fx), Drivers: sd.set(), Mode: "full"})
	require.NoError(t, err)
	ps := c.State.Packets["M01-T01"]
	require.Nil(t, ps.BaseRef, "a delivered candidate restarts Full from the current integration head")
	require.Equal(t, before.Packets["M01-T01"].Attempts, ps.Attempts)
	require.NotEmpty(t, ps.EvidenceHistory)
	require.Equal(t, before.Budget.Totals.Tokens, c.State.Budget.Totals.Tokens)
	require.Contains(t, fmt.Sprint(c.State.Workflow.Upgrades[0].ContractFingerprints), before.ContractFingerprints["M01-T01"])
}

func TestQuickManualExecutorRiskPersistsUpgrade(t *testing.T) {
	fx := newQuickFixture(t, false)
	path := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	writeFile(t, path, strings.Replace(string(raw), "  failures: []", "  failures: []\n  workflow-risks: []", 1))
	sd := quickDrivers()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		r := sd.mock.Execute(a)
		reportPath := filepath.Join(a.TaskDir, "reports", a.PacketID+".done.json")
		raw, err := os.ReadFile(reportPath)
		require.NoError(t, err)
		var report map[string]any
		require.NoError(t, json.Unmarshal(raw, &report))
		report["workflow-risks"] = []string{"existing authentication change requires Full review"}
		writeFile(t, reportPath, string(mustJSON(report)))
		return r
	}
	opts := stepOpts(fx, sd.set())
	opts.Only = "execute"
	summary, err := RunSingleStep(opts)
	require.NoError(t, err)
	require.False(t, summary.Step.OK)
	require.Contains(t, *summary.Step.Detail, "upgraded to Full")
	require.Equal(t, "full", loadFixtureState(t, fx).Workflow.Effective)
	require.Empty(t, sd.log.plan)
	require.Empty(t, sd.log.review)
}

func TestQuickManualUpgradeCannotProceedAfterConcurrentStateWrite(t *testing.T) {
	for _, scenario := range []string{"qualification", "final-review"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, sd, p := executedQuick(t, false)
			advance := func() {
				st := loadFixtureState(t, fx)
				_, err := state.Save(fx.taskDir, st, nil)
				require.NoError(t, err)
			}
			if scenario == "qualification" {
				c.Spec.Workflow.ScopeClear = false
				advance()
			} else {
				sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
					advance()
					return quickVerdict("upgrade", false)
				}
				c.Drivers = sd.set()
			}
			_, err := c.runSingle("review", p.PacketID)
			var conflict *state.CasConflictError
			require.ErrorAs(t, err, &conflict)
			require.Equal(t, "quick", loadFixtureState(t, fx).Workflow.Effective)
			require.Empty(t, sd.log.plan)
			require.Len(t, sd.log.exec, 1)
		})
	}
}

func TestQuickStartupCASConflictCannotCommitPreparedContracts(t *testing.T) {
	fx := newQuickFixture(t, false)
	sd := quickDrivers()
	c := quickContext(t, fx, sd)
	c.save()
	appendPacketRevision(t, fx, "M01-T01")
	require.NoError(t, c.reloadPackets())
	head := gitCmd(t, fx.root, "rev-parse", "HEAD")
	st := loadFixtureState(t, fx)
	_, err := state.Save(fx.taskDir, st, nil)
	require.NoError(t, err)
	_, err = c.run()
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, head, gitCmd(t, fx.root, "rev-parse", "HEAD"), "a stale writer must not create a contract checkpoint")
	require.Contains(t, gitCmd(t, fx.root, "diff", "--name-only"), "task-packet-M01-T01.md")
	require.Empty(t, sd.log.exec)
}

func TestQuickCancellationAfterExecutionPreservesCandidateAndStops(t *testing.T) {
	fx := newQuickFixture(t, false)
	sd := quickDrivers()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		r := sd.mock.Execute(a)
		cancel()
		return r
	}
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: configFor(fx), Drivers: sd.set(), Context: ctx})
	require.NoError(t, err)
	_, err = c.run()
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, sd.log.exec, 1)
	require.Empty(t, sd.log.review)
	require.NotEqual(t, "completed", c.State.Status)
	require.FileExists(t, filepath.Join(c.unitCwd(c.PacketList[0]), "build", "M01-T01.txt"))
}
