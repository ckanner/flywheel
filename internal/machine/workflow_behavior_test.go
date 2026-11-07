package machine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const quickAssessment = `workflow:
  version: 1
  mode: auto
  reason: Localized change follows the existing artifact implementation.
  scope-clear: true
  implementation-known: true
  verification-complete: true
  review-sufficient: true
  evidence: ["build/M01-T01.txt has a focused executable regression check"]
  risks: []
  not-applicable: {L2: "No additional service integration", L3: "No separate user interface"}
`

func newQuickFixture(t *testing.T, levels bool) fixture {
	t.Helper()
	fx := newGitFixture(t)
	spec := strings.Replace(fixtureSpec, "spec-version: 1\n", "spec-version: 1\n"+quickAssessment, 1)
	start := strings.Index(spec, "  - id: R-02")
	end := strings.Index(spec[start:], "---\n") + start
	spec = spec[:start] + spec[end:]
	writeFile(t, filepath.Join(fx.taskDir, "spec.md"), spec)
	require.NoError(t, os.Remove(filepath.Join(fx.taskDir, "task-packet-M01-T02.md")))
	require.NoError(t, os.Remove(state.StateFile(fx.taskDir)))
	if levels {
		packet := fixturePacket("M01-T01", "R-01")
		packet = strings.Replace(packet, "review-rubric:", "  L2:\n    - {cmd: \"test -f build/M01-T01.txt\", expect: \"exit 0\"}\n  L3:\n    - {cmd: \"test -f build/M01-T01.txt\", expect: \"exit 0\"}\nreview-rubric:", 1)
		writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), packet)
	}
	gitCmd(t, fx.root, "add", "-A")
	gitCmd(t, fx.root, "commit", "-q", "-m", "docs: prepare qualified compact contract")
	return fx
}

func quickVerdict(verdict string, eligible bool) drivers.Result {
	return drivers.MockResult(func(r *drivers.Result) {
		p := map[string]any{
			"verdict": verdict, "score": 1.0, "reasons": []string{"build/M01-T01.txt:1 and acceptance command exit 0"},
			"quick-eligible": eligible,
			"requirements":   []map[string]any{{"id": "R-01", "pass": verdict == "pass", "evidence": []string{"test -f build/M01-T01.txt exit 0"}}},
		}
		raw, _ := json.Marshal(p)
		r.Text = string(raw)
	})
}

func quickDrivers() *scriptedDrivers {
	sd := newScripted(nil)
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		if strings.Contains(a.Prompt, "task FINAL Review") {
			return quickVerdict("pass", true)
		}
		return sd.mock.Review(a)
	}
	return sd
}

func quickContext(t *testing.T, fx fixture, sd *scriptedDrivers) *Context {
	t.Helper()
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: configFor(fx), Drivers: sd.set()})
	require.NoError(t, err)
	require.True(t, c.quick())
	return c
}

func TestQuickCompletesWithTwoAgentCallsAndCandidateIntegrationEvidence(t *testing.T) {
	for _, levels := range []bool{false, true} {
		t.Run(fmt.Sprint(levels), func(t *testing.T) {
			fx := newQuickFixture(t, levels)
			sd := quickDrivers()
			sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
				require.NotEqual(t, fx.root, a.Cwd)
				require.NoFileExists(t, filepath.Join(fx.root, "build", "M01-T01.txt"), "integration checkout still lacks the candidate before final review")
				require.Contains(t, a.Prompt, "task FINAL Review")
				require.Contains(t, a.Prompt, "R-01")
				require.Contains(t, a.Prompt, a.Cwd)
				return quickVerdict("pass", true)
			}
			summary, err := RunTask(runOpts(fx, sd.set()))
			require.NoError(t, err)
			require.Equal(t, "completed", summary.Status)
			require.Empty(t, sd.log.plan)
			require.Len(t, sd.log.exec, 1)
			require.Len(t, sd.log.review, 1)
			require.Empty(t, sd.log.judge)
			st := loadFixtureState(t, fx)
			require.Equal(t, "quick", st.Workflow.Effective)
			require.False(t, st.Workflow.Stages["plan"].Applicable)
			require.False(t, st.Workflow.Stages["judge"].Applicable)
			ps := st.Packets["M01-T01"]
			review := ps.Evidence.Review.(map[string]any)
			require.Equal(t, "task-final", review["scope"])
			require.Equal(t, evidenceIdentity(ps.Evidence.L1), evidenceIdentity(ps.Evidence.FinalGate))
			require.Equal(t, evidenceIdentity(ps.Evidence.L1), evidenceIdentity(review))
			require.FileExists(t, filepath.Join(fx.root, "build", "M01-T01.txt"))
			require.Equal(t, strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD")), ps.Commit.(map[string]any)["sha"])
			before := len(sd.log.exec) + len(sd.log.review)
			_, err = RunTask(runOpts(fx, sd.set()))
			require.NoError(t, err)
			require.Equal(t, before, len(sd.log.exec)+len(sd.log.review), "completed resume does not rerun valid delivered work")
		})
	}
}

func TestQuickPauseResumeAndManualStagesPreserveFinalContract(t *testing.T) {
	fx := newQuickFixture(t, true)
	sd := quickDrivers()
	opts := runOpts(fx, sd.set())
	opts.PauseBefore = "review"
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "paused", summary.Status)
	require.Len(t, sd.log.exec, 1)
	require.Empty(t, sd.log.review)
	manual := stepOpts(fx, sd.set())
	manual.Only = "plan"
	_, err = RunSingleStep(manual)
	require.ErrorContains(t, err, "--mode full")
	require.Empty(t, sd.log.plan)
	for _, id := range []string{"M01-T01", ""} {
		manual.Only, manual.PacketID = "verify", id
		summary, err = RunSingleStep(manual)
		require.NoError(t, err)
		require.True(t, summary.Step.OK)
	}
	manual.Only, manual.PacketID = "review", "M01-T01"
	summary, err = RunSingleStep(manual)
	require.NoError(t, err)
	require.True(t, summary.Step.OK)
	opts.PauseBefore, opts.Mode = "", "auto"
	summary, err = RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	require.Len(t, sd.log.exec, 1)
	require.Empty(t, sd.log.judge)
}

func TestQuickReviewReturnsThenRepairsWithoutSeparatePlannerOrJudge(t *testing.T) {
	fx := newQuickFixture(t, false)
	sd := quickDrivers()
	reviews := 0
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		reviews++
		if reviews == 1 {
			return quickVerdict("return", true)
		}
		return quickVerdict("pass", true)
	}
	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	require.Len(t, sd.log.exec, 2)
	require.Len(t, sd.log.review, 2)
	require.Empty(t, sd.log.plan)
	require.Empty(t, sd.log.judge)
	require.Contains(t, sd.log.exec[1].Prompt, "Previous attempt feedback")
}

func TestQuickFinalReviewUpgradesKeepTaskBudgetAndHistory(t *testing.T) {
	for _, verdict := range []string{"upgrade", "verification-gap"} {
		t.Run(verdict, func(t *testing.T) {
			fx := newQuickFixture(t, false)
			sd := quickDrivers()
			sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
				if strings.Contains(a.Prompt, "task FINAL Review") {
					return quickVerdict(verdict, verdict != "upgrade")
				}
				return sd.mock.Review(a)
			}
			summary, err := RunTask(runOpts(fx, sd.set()))
			require.NoError(t, err)
			require.Equal(t, "completed", summary.Status)
			st := loadFixtureState(t, fx)
			require.Equal(t, "full", st.Workflow.Effective)
			require.Equal(t, "auto", st.Workflow.Requested)
			require.Len(t, st.Workflow.Upgrades, 1)
			require.NotEmpty(t, st.Workflow.Upgrades[0].ContractFingerprints)
			require.NotEmpty(t, st.Packets["M01-T01"].EvidenceHistory)
			require.GreaterOrEqual(t, st.Packets["M01-T01"].Attempts, 2)
			require.Greater(t, st.Budget.Totals.Tokens, int64(0))
			require.NotEmpty(t, sd.log.plan)
			require.Len(t, sd.log.judge, 1)
			before := st.Budget.Totals.Tokens
			opts := runOpts(fx, sd.set())
			opts.Mode = "auto"
			_, err = RunTask(opts)
			require.NoError(t, err)
			st = loadFixtureState(t, fx)
			require.Equal(t, before, st.Budget.Totals.Tokens)
			require.Len(t, st.Workflow.Upgrades, 1)
			opts.Mode = "quick"
			_, err = RunTask(opts)
			require.ErrorContains(t, err, "downgraded")
		})
	}
}

func TestQuickExecutorRiskUpgradesBeforeAnyFinalReview(t *testing.T) {
	fx := newQuickFixture(t, false)
	packetPath := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
	packet, err := os.ReadFile(packetPath)
	require.NoError(t, err)
	writeFile(t, packetPath, strings.Replace(string(packet), "  failures: []", "  failures: []\n  workflow-risks: []", 1))
	gitCmd(t, fx.root, "add", "-A")
	gitCmd(t, fx.root, "commit", "-m", "docs: declare executor risk reporting")
	sd := quickDrivers()
	calls := 0
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		calls++
		r := sd.mock.Execute(a)
		raw, err := os.ReadFile(filepath.Join(a.TaskDir, "reports", a.PacketID+".done.json"))
		require.NoError(t, err)
		var report map[string]any
		require.NoError(t, json.Unmarshal(raw, &report))
		report["workflow-risks"] = []string{}
		if calls == 1 {
			report["workflow-risks"] = []string{" ", "src/transaction.go:1 requires concurrent writer protection"}
		}
		writeFile(t, filepath.Join(a.TaskDir, "reports", a.PacketID+".done.json"), string(mustJSON(report)))
		return r
	}
	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	require.Len(t, sd.log.review, 1)
	require.NotContains(t, sd.log.review[0].Prompt, "task FINAL Review")
	st := loadFixtureState(t, fx)
	require.Contains(t, st.Workflow.Reason, "Executor")
	require.Len(t, st.Workflow.Upgrades[0].Evidence, 1)
}

func TestQuickReviewCannotMutateCandidateOrAcceptWeakUnitVerdict(t *testing.T) {
	fx := newQuickFixture(t, true)
	sd := quickDrivers()
	c := quickContext(t, fx, sd)
	p := c.PacketList[0]
	r, err := c.runExecuteLoop(p)
	require.NoError(t, err)
	require.True(t, r.Green)
	weak := newScripted(nil)
	c.Drivers = weak.set()
	review, err := c.reviewStep(p, nil)
	require.NoError(t, err)
	require.False(t, review.Pass)
	require.Equal(t, errs.EMPTY_RESPONSE, review.ErrorCode)
	require.Nil(t, c.State.Packets[p.PacketID].Evidence.Review)
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		writeFile(t, filepath.Join(a.Cwd, "build", "M01-T01.txt"), "unreviewed mutation\n")
		return quickVerdict("pass", true)
	}
	c.Drivers = sd.set()
	review, err = c.reviewStep(p, nil)
	require.NoError(t, err)
	require.False(t, review.Pass)
	require.Equal(t, errs.INVALID_REQUEST, review.ErrorCode)
	require.False(t, c.currentFinalGate(p))
	require.ErrorIs(t, c.completeUnit(p), ErrCommitFailed)
}

func TestQuickUpgradeCheckpointCASFailureNeverRunsPlanner(t *testing.T) {
	fx := newQuickFixture(t, false)
	sd := quickDrivers()
	c := quickContext(t, fx, sd)
	c.save()
	st := loadFixtureState(t, fx)
	_, err := state.Save(fx.taskDir, st, nil)
	require.NoError(t, err)
	err = c.upgradeWorkflow("risk requires planning", []string{"evidence"})
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	require.Empty(t, sd.log.plan)
	require.Equal(t, "quick", loadFixtureState(t, fx).Workflow.Effective)
	_, err = c.run()
	require.Error(t, err)
	require.Empty(t, sd.log.exec)
	// A confirmed pre-existing failure also prevents a new upgrade checkpoint.
	c.State.Workflow.Effective = "quick"
	c.fatal = errors.New("disk unavailable")
	require.ErrorContains(t, c.upgradeWorkflow("risk", nil), "disk unavailable")
	c.State.Workflow.Effective = "full"
	require.NoError(t, c.upgradeWorkflow("already Full", nil))
}

func TestQuickStageAndEvidencePoliciesCannotBeReusedAfterUpgrade(t *testing.T) {
	fx := newQuickFixture(t, false)
	sd := quickDrivers()
	c := quickContext(t, fx, sd)
	p := c.PacketList[0]
	r, err := c.runExecuteLoop(p)
	require.NoError(t, err)
	require.True(t, r.Green)
	review, err := c.reviewStep(p, nil)
	require.NoError(t, err)
	require.True(t, review.Pass)
	old := evidenceIdentity(c.State.Packets[p.PacketID].Evidence.L1)
	require.NoError(t, c.upgradeWorkflow("require full plan", []string{"new requirement evidence"}))
	fresh, err := c.verificationIdentity(p)
	require.NoError(t, err)
	require.NotEqual(t, old, fresh)
	require.Nil(t, c.State.Packets[p.PacketID].Evidence.L1)
	require.False(t, c.currentFinalGate(p))
	require.NotNil(t, c.State.Packets[p.PacketID].Worktree)
	require.NotEmpty(t, c.State.Packets[p.PacketID].EvidenceHistory)
}

func TestQuickExplicitFullUpgradeResumesThroughManualPlanner(t *testing.T) {
	fx := newQuickFixture(t, false)
	sd := quickDrivers()
	c := quickContext(t, fx, sd)
	c.save()
	manual := stepOpts(fx, sd.set())
	manual.Only = "plan"
	manual.Mode = "full"
	summary, err := RunSingleStep(manual)
	require.NoError(t, err)
	require.True(t, summary.Step.OK)
	require.Len(t, sd.log.plan, 1)
	require.Equal(t, "full", loadFixtureState(t, fx).Workflow.Effective)
	require.Len(t, loadFixtureState(t, fx).Workflow.Upgrades, 1)
}

func TestQuickHelpersRejectIncompleteFinalProof(t *testing.T) {
	fx := newQuickFixture(t, false)
	c := quickContext(t, fx, quickDrivers())
	p := c.PacketList[0]
	ps := c.State.Packets[p.PacketID]
	require.False(t, c.validFinalReview(p))
	ps.Evidence.Review = map[string]any{"scope": "task-final", "verdict": "pass", "score": "bad"}
	require.False(t, c.validFinalReview(p))
	ps.Evidence.Review = map[string]any{"scope": "task-final", "verdict": "pass", "score": 1}
	require.False(t, c.validFinalReview(p))
	assert.Nil(t, c.executorWorkflowRisks(p))
	c.Spec.Workflow.Version = 77
	_, err := c.runQuick()
	require.Error(t, err)
	_, err = c.runSingle("verify", "")
	require.Error(t, err)
}
