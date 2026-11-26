package machine

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestSessionFullWorkflowAndCASRecoveryKeepOneInvocationPerStage(t *testing.T) {
	for _, fault := range []string{"none", "plan", "execute", "judge", "judge-panel", "collected-report"} {
		t.Run(fault, func(t *testing.T) {
			fx := newGitFixture(t)
			sd := newScripted(nil)
			injected := false
			bump := func(role string) {
				if !injected && role == fault {
					injected = true
					st := loadFixtureState(t, fx)
					_, err := state.Save(fx.taskDir, st, nil)
					require.NoError(t, err)
				}
			}
			sd.planFn = func(a drivers.CallArgs) drivers.Result { bump("plan"); return sd.mock.Plan(a) }
			sd.execFn = func(a drivers.CallArgs) drivers.Result {
				r := sd.mock.Execute(a)
				bump("execute")
				return r
			}
			sd.judgeFn = func(a drivers.CallArgs) drivers.Result {
				role := "judge"
				if a.Model == "panel-model" {
					role = "judge-panel"
				}
				bump(role)
				return sd.mock.Judge(a)
			}
			originalRemove := removeCollectedReport
			t.Cleanup(func() { removeCollectedReport = originalRemove })
			if fault == "collected-report" {
				removeCollectedReport = func(path string) error {
					err := originalRemove(path)
					if err == nil {
						bump("collected-report")
					}
					return err
				}
			}
			opts := sessionRunOpts(fx, sessionDriverSet(sd))
			opts.Config.JudgePanel = &config.LoopBinding{Harness: "dsh", Model: "panel-model", Channel: "opencode-go", Thinking: "high"}
			summary, err := RunTask(opts)
			if fault != "none" {
				var conflict *state.CasConflictError
				require.ErrorAs(t, err, &conflict)
				require.True(t, injected)
				removeCollectedReport = originalRemove
				summary, err = RunTask(opts)
			}
			require.NoError(t, err)
			require.Equal(t, "completed", summary.Status)
			require.Len(t, sd.log.plan, 1)
			require.Len(t, sd.log.exec, 2)
			require.Len(t, sd.log.review, 2)
			require.Len(t, sd.log.judge, 2)
			st := loadFixtureState(t, fx)
			require.Len(t, st.Sessions.Records, 7)
			histories := map[string]bool{}
			for key, record := range st.Sessions.Records {
				require.Equal(t, "consumed", record.Last.Status, key)
				require.True(t, record.Last.Accounted, key)
				require.NotEmpty(t, record.ThreadID, key)
				require.False(t, histories[record.ThreadID], "independent role scopes must not share history")
				histories[record.ThreadID] = true
			}
			for _, packet := range st.Packets {
				require.Equal(t, 1, packet.Attempts)
			}
			require.Equal(t, int64(7*180), st.Budget.Totals.Tokens)
		})
	}
}

func TestSessionQuickExecutorCASRecoveryKeepsAttemptsInScopedAndFreshModes(t *testing.T) {
	for _, mode := range []string{"scoped", "fresh"} {
		t.Run(mode, func(t *testing.T) {
			fx := newQuickFixture(t, false)
			sd := quickDrivers()
			sd.execFn = func(a drivers.CallArgs) drivers.Result {
				r := sd.mock.Execute(a)
				st := loadFixtureState(t, fx)
				_, err := state.Save(fx.taskDir, st, nil)
				require.NoError(t, err)
				return r
			}
			opts := sessionRunOpts(fx, sessionDriverSet(sd))
			opts.Config.Sessions.Mode = mode
			_, err := RunTask(opts)
			var conflict *state.CasConflictError
			require.ErrorAs(t, err, &conflict)
			res, err := RunTask(opts)
			require.NoError(t, err)
			require.Equal(t, "completed", res.Status)
			require.Len(t, sd.log.exec, 1)
			require.Len(t, sd.log.review, 1)
			st := loadFixtureState(t, fx)
			require.Equal(t, 1, st.Packets["M01-T01"].Attempts)
			require.Equal(t, int64(360), st.Budget.Totals.Tokens)
		})
	}
}

func TestSessionMalformedExecutorReportRepairsSameHistoryWithinBoundedAttempt(t *testing.T) {
	fx, c := sessionFixture(t)
	sd := newScripted(nil)
	calls := 0
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		calls++
		r := sd.mock.Execute(a)
		if calls == 1 {
			writeFile(t, filepath.Join(a.TaskDir, "reports", a.PacketID+".done.json"), "not JSON")
		} else {
			require.NotEmpty(t, a.SessionID)
			require.Contains(t, a.Prompt, "Verified context update")
			require.Contains(t, a.Prompt, "deliverable invalid")
		}
		return r
	}
	c.Drivers = sessionDriverSet(sd)
	out, err := c.executeStep(c.Packets["M01-T01"])
	require.NoError(t, err)
	require.True(t, out.Success)
	require.Equal(t, 2, calls)
	require.Equal(t, 1, loadFixtureState(t, fx).Packets["M01-T01"].Attempts)
	require.Equal(t, "history-"+sd.log.exec[0].RequestID, sd.log.exec[1].SessionID)
}

func TestSessionJudgePanelBudgetStopRetainsPrimaryReceiptAndOriginalGate(t *testing.T) {
	fx, c := sessionFixture(t)
	sd := newScripted(nil)
	c.Drivers = sessionDriverSet(sd)
	c.Config.JudgePanel = &config.LoopBinding{Harness: "dsh", Model: "panel", Channel: "opencode-go"}
	c.State.Budget.Enforce = "hard"
	c.Config.Budget.Enforce = "hard"
	limit := int64(100)
	c.State.Budget.DailyTokenLimit = &limit
	c.Config.Budget.DailyTokenLimit = &limit
	gateIdentity, err := c.candidateFingerprint(c.Cwd)
	require.NoError(t, err)
	originalGate := map[string]any{"pass": true, "identity": gateIdentity, "at": "original-evidence", "levels": map[string]any{}}
	ms := c.State.Milestones["M-01"]
	ms.Gate = originalGate
	c.State.Milestones["M-01"] = ms
	c.save()
	out, err := c.judgeStep(c.Spec.Milestones[0])
	require.NoError(t, err)
	require.True(t, out.Stopped)
	require.Len(t, sd.log.judge, 1)
	require.True(t, c.sessionReplay("judge", "M-01"))
	require.True(t, c.sessionRecord("judge", "M-01").Last.Accounted)
	c = unitTestContext(t, fx)
	enableSessionPolicy(c)
	c.Drivers = sd.set()
	c.Config.JudgePanel = &config.LoopBinding{Harness: "dsh", Model: "panel", Channel: "opencode-go"}
	c.Config.Budget.Enforce = "hard"
	c.Config.Budget.DailyTokenLimit = &limit
	require.NoError(t, c.reconcileSessionReceipts())
	gate, err := c.sessionMilestoneGate(c.Spec.Milestones[0])
	require.NoError(t, err)
	require.True(t, gate.Pass)
	require.Equal(t, "original-evidence", c.State.Milestones["M-01"].Gate.(map[string]any)["at"])
	out, err = c.judgeStep(c.Spec.Milestones[0])
	require.NoError(t, err)
	require.True(t, out.Stopped)
	require.Len(t, sd.log.judge, 1)
	more := int64(1000)
	c.State.Budget.DailyTokenLimit = &more
	c.Config.Budget.DailyTokenLimit = &more
	out, err = c.judgeStep(c.Spec.Milestones[0])
	require.NoError(t, err)
	require.True(t, out.Pass)
	require.Len(t, sd.log.judge, 2)
	require.Equal(t, int64(360), c.State.Budget.Totals.Tokens)
	for _, role := range []string{"judge", "judge-panel"} {
		require.Equal(t, "consumed", c.sessionRecord(role, "M-01").Last.Status)
	}
}

func TestSessionPendingJudgeCannotRegenerateMissingOrChangedGate(t *testing.T) {
	for _, change := range []string{"missing", "failed", "identity", "candidate"} {
		t.Run(change, func(t *testing.T) {
			fx, c := sessionFixture(t)
			a := sessionArgs(c)
			candidate, err := c.candidateFingerprint(fx.root)
			require.NoError(t, err)
			ms := c.State.Milestones["M-01"]
			ms.Gate = map[string]any{"pass": true, "identity": candidate, "levels": map[string]any{}}
			c.State.Milestones["M-01"] = ms
			_, err = c.invokeScoped("judge", "M-01", a, func(a drivers.CallArgs) drivers.Result { return sessionResult(a, drivers.MockResult(nil)) })
			require.NoError(t, err)
			gate := c.State.Milestones["M-01"].Gate.(map[string]any)
			switch change {
			case "missing":
				ms.Gate = nil
				c.State.Milestones["M-01"] = ms
			case "failed":
				gate["pass"] = false
			case "identity":
				delete(gate, "identity")
			case "candidate":
				writeFile(t, filepath.Join(fx.root, "foreign.go"), "package foreign\n")
			}
			_, err = c.sessionMilestoneGate(c.Spec.Milestones[0])
			require.ErrorContains(t, err, "original passing milestone gate")
			require.Equal(t, "escalated", c.State.Status)
		})
	}
}

func TestSessionLegacyPlanUnknownOutcomeBlocksFreshDifferentRole(t *testing.T) {
	fx, c := sessionFixture(t)
	c.Config.Sessions.Mode = "fresh"
	// The ledger refuses legacy uncertainty independently of selected backend.
	c.State.PlanSession = map[string]any{"session-id": "legacy", "outcome-unknown": true}
	c.save()
	c = unitTestContext(t, fx)
	enableSessionPolicy(c)
	c.Config.Sessions.Mode = "fresh"
	_, err := c.invokeScoped("execute", "M01-T01", sessionArgs(c), func(drivers.CallArgs) drivers.Result {
		t.Fatal("unknown prior effects cannot replay")
		return drivers.Result{}
	})
	require.ErrorContains(t, err, "previous resident Plan result remains unknown")
	require.Equal(t, "escalated", loadFixtureState(t, fx).Status)
}

func TestSessionCheckpointSlotIsBoundedPerScope(t *testing.T) {
	fx, c := sessionFixture(t)
	a := sessionArgs(c)
	for i := 0; i < 3; i++ {
		r, err := c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result { return sessionResult(a, drivers.MockResult(nil)) })
		require.NoError(t, err)
		applySessionResult(t, c, "execute", a.PacketID, r)
	}
	files, err := os.ReadDir(filepath.Join(fx.taskDir, ".sessions"))
	require.NoError(t, err)
	require.Len(t, files, 1, fmt.Sprint(files))
	_, _, err = state.ReadSessionReceipt(fx.taskDir, "execute/M01-T01", c.sessionRecord("execute", a.PacketID).Last.ID, "")
	require.NoError(t, err)
}

func TestSessionRunStopsBeforeExecuteWhenPlanCheckpointCASFails(t *testing.T) {
	fx := newGitFixture(t)
	sd := newScripted(nil)
	original := inspectPlanDirtyFiles
	t.Cleanup(func() { inspectPlanDirtyFiles = original })
	inspected := false
	inspectPlanDirtyFiles = func(cwd, base string) ([]string, bool) {
		files, ok := original(cwd, base)
		if !inspected {
			inspected = true
			st := loadFixtureState(t, fx)
			_, err := state.Save(fx.taskDir, st, nil)
			require.NoError(t, err)
		}
		return files, ok
	}
	_, err := RunTask(sessionRunOpts(fx, sessionDriverSet(sd)))
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	require.True(t, inspected)
	require.Len(t, sd.log.plan, 1)
	require.Empty(t, sd.log.exec)
	require.Empty(t, sd.log.judge)
}

func TestSessionRunCannotReplayJudgeAfterPassingGateIsRevoked(t *testing.T) {
	fx := newGitFixture(t)
	sd := newScripted(nil)
	sd.judgeFn = func(a drivers.CallArgs) drivers.Result {
		st := loadFixtureState(t, fx)
		_, err := state.Save(fx.taskDir, st, nil)
		require.NoError(t, err)
		return sd.mock.Judge(a)
	}
	opts := sessionRunOpts(fx, sessionDriverSet(sd))
	_, err := RunTask(opts)
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	st := loadFixtureState(t, fx)
	gate := st.Milestones["M-01"].Gate.(map[string]any)
	gate["pass"] = false
	_, err = state.Save(fx.taskDir, st, nil)
	require.NoError(t, err)
	_, err = RunTask(opts)
	require.ErrorContains(t, err, "original passing milestone gate")
	require.Len(t, sd.log.judge, 1)
	require.Len(t, sd.log.plan, 1)
	require.Len(t, sd.log.exec, 2)
	require.Equal(t, "escalated", loadFixtureState(t, fx).Status)
}
