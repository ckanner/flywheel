package machine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestQuickManualFinalReviewPersistsUpgradeBeforeRestart(t *testing.T) {
	for _, verdict := range []string{"upgrade", "verification-gap", "return"} {
		t.Run(verdict, func(t *testing.T) {
			fx := newQuickFixture(t, false)
			sd := quickDrivers()
			opts := runOpts(fx, sd.set())
			opts.PauseBefore = "review"
			_, err := RunTask(opts)
			require.NoError(t, err)
			before := loadFixtureState(t, fx)
			sd.reviewFn = func(a drivers.CallArgs) drivers.Result { return quickVerdict(verdict, verdict == "verification-gap") }
			manual := stepOpts(fx, sd.set())
			manual.Only = "review"
			summary, err := RunSingleStep(manual)
			require.NoError(t, err)
			require.False(t, summary.Step.OK)
			require.Empty(t, sd.log.plan, "single-step review must not secretly invoke Plan")
			after := loadFixtureState(t, fx)
			require.Equal(t, "full", after.Workflow.Effective)
			require.Len(t, after.Workflow.Upgrades, 1)
			require.Equal(t, before.Packets["M01-T01"].Attempts, after.Packets["M01-T01"].Attempts)
			require.Equal(t, before.Packets["M01-T01"].FailedAttempts, after.Packets["M01-T01"].FailedAttempts)
			require.NotEmpty(t, after.Packets["M01-T01"].EvidenceHistory)
			require.Greater(t, after.Budget.Totals.Tokens, before.Budget.Totals.Tokens)
			// An optimistic later reviewer cannot undo the recorded risk.
			sd.reviewFn = sd.mock.Review
			opts.PauseBefore, opts.Mode = "", "auto"
			summary, err = RunTask(opts)
			require.NoError(t, err)
			require.Equal(t, "completed", summary.Status)
			require.NotEmpty(t, sd.log.plan)
			require.Len(t, sd.log.judge, 1)
			require.Len(t, loadFixtureState(t, fx).Workflow.Upgrades, 1)
		})
	}
}

func TestQuickDeliveredRecoveryChecksProofEvenAfterMilestoneClosed(t *testing.T) {
	for _, checkpoint := range []string{"unit-delivered", "milestone-closed", "task-completed"} {
		for _, changed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/changed=%v", checkpoint, changed), func(t *testing.T) {
				fx := newQuickFixture(t, true)
				sd := quickDrivers()
				_, err := RunTask(runOpts(fx, sd.set()))
				require.NoError(t, err)
				st := loadFixtureState(t, fx)
				if checkpoint != "task-completed" {
					st.Status = "running"
				}
				if checkpoint == "unit-delivered" {
					ms := st.Milestones["M-01"]
					ms.Status, ms.ClosedAt, ms.Gate = "in-progress", nil, nil
					st.Milestones["M-01"] = ms
				}
				_, err = state.Save(fx.taskDir, st, nil)
				require.NoError(t, err)
				if changed {
					writeFile(t, filepath.Join(fx.root, "build", "M01-T01.txt"), "unreviewed external committed change\n")
					gitCmd(t, fx.root, "add", "--", "build/M01-T01.txt")
					gitCmd(t, fx.root, "commit", "-m", "fix: external change after checkpoint")
				}
				summary, err := RunTask(runOpts(fx, sd.set()))
				if changed {
					require.ErrorIs(t, err, ErrCommitFailed)
					require.Equal(t, "escalated", summary.Status)
					st = loadFixtureState(t, fx)
					require.Equal(t, "inbox", st.Packets["M01-T01"].Status)
					require.NotEqual(t, "closed", st.Milestones["M-01"].Status)
					require.Contains(t, st.Inbox[len(st.Inbox)-1].(map[string]any)["reason"], "stale")
				} else {
					require.NoError(t, err)
					require.Equal(t, "completed", summary.Status)
				}
				require.Len(t, sd.log.exec, 1)
				require.Len(t, sd.log.review, 1)
				require.Empty(t, sd.log.plan)
				require.Empty(t, sd.log.judge)
			})
		}
	}
}

func TestQuickDefectPolicyChangeReopensDeliveredProof(t *testing.T) {
	fx := newQuickFixture(t, false)
	sd := quickDrivers()
	opts := runOpts(fx, sd.set())
	_, err := RunTask(opts)
	require.NoError(t, err)
	before := loadFixtureState(t, fx)
	opts.Config.DefectInjection.EveryNUnits = new(1)
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "escalated", summary.Status)
	after := loadFixtureState(t, fx)
	require.NotEqual(t, before.ContractFingerprints["M01-T01"], after.ContractFingerprints["M01-T01"])
	require.NotEmpty(t, after.Packets["M01-T01"].EvidenceHistory)
	require.Len(t, sd.log.exec, 2, "new completion policy requires fresh candidate proof")
	require.Len(t, sd.log.review, 1, "missing sampled defect checks stop before final review")
	require.Equal(t, "missing-config", after.Packets["M01-T01"].Evidence.DefectInjection.(map[string]any)["skipped"])
}

func TestQuickSampledDefectInjectionCannotBeSkippedByManualReview(t *testing.T) {
	for _, outcome := range []string{"missing-config", "fake-tests", "pass"} {
		t.Run(outcome, func(t *testing.T) {
			fx := newQuickFixture(t, false)
			sd := quickDrivers()
			cfg := configFor(fx)
			cfg.DefectInjection.EveryNUnits = new(1)
			if outcome != "missing-config" {
				cfg.DefectInjection.Patch = writeDefectPatch(t, &fx)
				cfg.DefectInjection.TestCmd = "true"
				if outcome == "pass" {
					cfg.DefectInjection.TestCmd = "test ! -f defect-marker"
				}
			}
			c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: sd.set()})
			require.NoError(t, err)
			p := c.PacketList[0]
			r, err := c.runExecuteLoop(p)
			require.NoError(t, err)
			require.True(t, r.Green)
			summary, err := c.runSingle("review", p.PacketID)
			require.NoError(t, err)
			require.Equal(t, outcome == "pass", summary.Step.OK)
			require.NoFileExists(t, filepath.Join(c.unitCwd(p), "defect-marker"), "all injections must restore the actual candidate")
			if outcome == "pass" {
				require.Len(t, sd.log.review, 1)
				require.Contains(t, sd.log.review[0].Prompt, `"outcome": "pass"`)
				require.True(t, c.currentFinalGate(p))
			} else {
				require.Empty(t, sd.log.review)
				require.Equal(t, "escalated", summary.Status)
				reason := c.State.Inbox[len(c.State.Inbox)-1].(map[string]any)["reason"].(string)
				if outcome == "fake-tests" {
					require.Contains(t, reason, "fake/weak tests proven")
				} else {
					require.Contains(t, reason, "not proven")
				}
			}
		})
	}
}

func TestQuickChangedQualificationUpgradesOnRunAndManualResume(t *testing.T) {
	for _, manual := range []bool{false, true} {
		t.Run(fmt.Sprint(manual), func(t *testing.T) {
			fx := newQuickFixture(t, false)
			sd := quickDrivers()
			c := quickContext(t, fx, sd)
			c.save()
			path := filepath.Join(fx.taskDir, "spec.md")
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			writeFile(t, path, strings.Replace(string(raw), "scope-clear: true", "scope-clear: false", 1))
			if manual {
				opts := stepOpts(fx, sd.set())
				opts.Only = "plan"
				summary, err := RunSingleStep(opts)
				require.NoError(t, err)
				require.True(t, summary.Step.OK)
			} else {
				summary, err := RunTask(runOpts(fx, sd.set()))
				require.NoError(t, err)
				require.Equal(t, "completed", summary.Status)
			}
			require.Equal(t, "full", loadFixtureState(t, fx).Workflow.Effective)
			require.Len(t, loadFixtureState(t, fx).Workflow.Upgrades, 1)
			require.NotEmpty(t, sd.log.plan)
		})
	}
}
