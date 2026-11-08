package machine

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func restartSessionGuardContext(t *testing.T, fx fixture, before *Context) *Context {
	t.Helper()
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: before.Config, Drivers: before.Drivers,
		Log: before.Log, Sleep: before.Sleep})
	require.NoError(t, err)
	return c
}

func TestSessionPlanGuardSurvivesReceiptCASInterruption(t *testing.T) {
	fx, c := sessionFixture(t)
	sd := newScripted(nil)
	sd.planFn = func(a drivers.CallArgs) drivers.Result {
		writeFile(t, filepath.Join(a.Cwd, "implementation.txt"), "planner changed implementation\n")
		bumpFaultRevision(t, fx)
		return sd.mock.Plan(a)
	}
	c.Drivers = sessionDriverSet(sd)
	_, err := c.planUntilValid(c.Spec.Milestones[0])
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, "intent", loadFixtureState(t, fx).Sessions.Records["plan/M-01"].Last.Status)

	c = restartSessionGuardContext(t, fx, c)
	out, err := c.planUntilValid(c.Spec.Milestones[0])
	require.NoError(t, err)
	require.False(t, out.OK, "restarting must not make the planner's illegal edit the new guard baseline")
	require.Equal(t, errs.INVALID_REQUEST, out.ErrorCode)
	require.Len(t, sd.log.plan, 1)
	r := c.sessionRecord("plan", "M-01")
	require.NotEmpty(t, r.Last.GuardBefore)
	require.NotEqual(t, r.Last.GuardBefore, r.Last.GuardAfter)
	require.False(t, r.ResumeCandidate)
	require.Equal(t, "consumed", r.Last.Status)
	require.True(t, r.Last.Accounted)
	require.Equal(t, int64(180), c.State.Budget.Totals.Tokens)
	receipt, _, err := state.ReadSessionReceipt(c.TaskDir, "plan/M-01", r.Last.ID, r.Last.ReceiptDigest)
	require.NoError(t, err)
	var physical drivers.Result
	require.NoError(t, json.Unmarshal(receipt.Result, &physical))
	require.Equal(t, 0, *physical.ExitCode, "role rejection must not fabricate a failed physical exit")
	require.Equal(t, errs.INVALID_REQUEST, physical.ErrorCode)
}

func TestSessionPlanMayReplayItsOwnPermittedContractRevision(t *testing.T) {
	fx, c := sessionFixture(t)
	sd := newScripted(nil)
	sd.planFn = func(a drivers.CallArgs) drivers.Result {
		packet := strings.Replace(fixturePacket("M01-T01", "R-01"), "contract-version: 1", "contract-version: 2", 1)
		packet = strings.Replace(packet, "Artifact exists and is honest.", "Artifact exists and is honest; inspect the current implementation.", 1)
		writeFile(t, filepath.Join(a.TaskDir, "task-packet-M01-T01.md"), packet)
		bumpFaultRevision(t, fx)
		return sd.mock.Plan(a)
	}
	c.Drivers = sessionDriverSet(sd)
	_, err := c.planUntilValid(c.Spec.Milestones[0])
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	c = restartSessionGuardContext(t, fx, c)
	require.Equal(t, 2, c.Packets["M01-T01"].ContractVersion)
	out, err := c.planUntilValid(c.Spec.Milestones[0])
	require.NoError(t, err)
	require.True(t, out.OK)
	require.Len(t, sd.log.plan, 1, "the permitted contract outputs are part of the original terminal candidate")
	require.True(t, c.sessionRecord("plan", "M-01").ResumeCandidate)
	require.Equal(t, "consumed", c.sessionRecord("plan", "M-01").Last.Status)
	require.Equal(t, int64(180), c.State.Budget.Totals.Tokens)
}

func TestSessionReviewGuardSurvivesCrashAndReverification(t *testing.T) {
	fx, c := sessionFixture(t)
	sd := newScripted(nil)
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		writeFile(t, filepath.Join(a.Cwd, "build", "M01-T01.txt"), "reviewer changed this candidate\n")
		bumpFaultRevision(t, fx)
		return sd.mock.Review(a)
	}
	c.Drivers = sessionDriverSet(sd)
	packet := c.Packets["M01-T01"]
	executed, err := c.runExecuteLoop(packet)
	require.NoError(t, err)
	require.True(t, executed.Green)
	_, err = c.reviewStep(packet, nil)
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	c = restartSessionGuardContext(t, fx, c)
	require.NoError(t, c.reconcileSessionReceipts())
	packet = c.Packets["M01-T01"]
	require.True(t, c.l1Step(packet).Pass, "the edited artifact still satisfies the mechanical command")
	_, err = c.reviewStep(packet, nil)
	require.ErrorContains(t, err, "original verification evidence changed")
	require.Len(t, sd.log.review, 1)
	require.False(t, c.sessionRecord("review", packet.PacketID).ResumeCandidate)
	require.True(t, c.sessionGuardViolated("review", packet.PacketID))
	require.Nil(t, c.State.Packets[packet.PacketID].Evidence.Review, "the earlier pass cannot be applied to a newly verified candidate")
}

func TestSessionMalformedReviewCannotRetryItsMutatedCandidate(t *testing.T) {
	_, c := sessionFixture(t)
	sd := newScripted(nil)
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		writeFile(t, filepath.Join(a.Cwd, "build", "M01-T01.txt"), "read-only role violated its boundary\n")
		return drivers.MockResult(func(r *drivers.Result) { r.Text = "not a verdict" })
	}
	c.Drivers = sessionDriverSet(sd)
	packet := c.Packets["M01-T01"]
	executed, err := c.runExecuteLoop(packet)
	require.NoError(t, err)
	require.True(t, executed.Green)
	out, err := c.reviewStep(packet, nil)
	require.NoError(t, err)
	require.Equal(t, errs.INVALID_REQUEST, out.ErrorCode)
	require.False(t, out.Pass)
	require.Len(t, sd.log.review, 1, "malformed output must not turn a role violation into a transport retry")
	r := c.sessionRecord("review", packet.PacketID)
	require.False(t, r.ResumeCandidate)
	require.True(t, r.Last.Accounted)
	require.Equal(t, "consumed", r.Last.Status)
	require.Equal(t, int64(360), c.State.Budget.Totals.Tokens)
}

func TestSessionPendingReceiptRequiresExactContract(t *testing.T) {
	for _, field := range []string{"spec-version", "packet-version", "acceptance"} {
		t.Run(field, func(t *testing.T) {
			_, c, a, _ := faultRecordedSession(t, "execute", "M01-T01")
			switch field {
			case "spec-version":
				c.Spec.SpecVersion++
			case "packet-version":
				c.Packets["M01-T01"].ContractVersion++
			case "acceptance":
				c.Packets["M01-T01"].Acceptance.L1[0].Cmd = "test -s build/M01-T01.txt"
			}
			_, err := c.invokeScoped("execute", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
				t.Fatal("a pending result cannot silently become a fresh call for changed contracts")
				return drivers.Result{}
			})
			require.ErrorContains(t, err, "pending result belongs to different task, binding, goal or contract")
			require.False(t, c.sessionRecord("execute", "M01-T01").ResumeCandidate)
		})
	}
}

func TestSessionMilestoneHistoryRetainsOnlyCompatibleDeclaredGoals(t *testing.T) {
	for _, role := range []string{"plan", "judge", "judge-panel"} {
		for _, change := range []string{"goal", "body", "boundaries", "requirements", "milestone-title", "acceptance"} {
			t.Run(role+"/"+change, func(t *testing.T) {
				_, c, a, result := faultRecordedSession(t, role, "M-01")
				applySessionResult(t, c, role, "M-01", result)
				prior := c.sessionRecord(role, "M-01").ThreadID
				packet := c.Packets["M01-T01"]
				switch change {
				case "goal":
					packet.Goal = "Replace the earlier feature with a different observable capability."
				case "body":
					packet.Body += "\nThe previous implementation approach is outside the newly approved scope.\n"
				case "boundaries":
					packet.Boundaries.MustNotTouch = append(packet.Boundaries.MustNotTouch, "additional-oracle/")
				case "requirements":
					packet.Requirements = []string{"R-01", "R-02"}
				case "milestone-title":
					c.Spec.Milestones[0].Title = "Changed milestone objective"
				case "acceptance":
					packet.ContractVersion++
					packet.Acceptance.L1[0].Cmd = "test -s build/M01-T01.txt"
					c.Spec.Milestones[0].Gate = "Existing requirements must satisfy the strengthened acceptance."
				}
				var requested string
				_, err := c.invokeScoped(role, "M-01", a, func(a drivers.CallArgs) drivers.Result {
					requested = a.SessionID
					return sessionResult(a, passVerdict())
				})
				require.NoError(t, err)
				if change == "acceptance" {
					require.Equal(t, prior, requested, "same-goal acceptance revisions can continue the role's consumed history")
					require.Equal(t, 1, c.sessionRecord(role, "M-01").Generation)
				} else {
					require.Empty(t, requested, "milestone roles must not resume a history with changed declared constituent goals")
					require.Equal(t, 2, c.sessionRecord(role, "M-01").Generation)
				}
			})
		}
	}
}

func TestSessionPendingReviewBindsItsOriginalVerificationEvidence(t *testing.T) {
	for _, field := range []string{"l1", "final-gate", "defect-injection"} {
		t.Run(field, func(t *testing.T) {
			_, c, a, _ := faultRecordedSession(t, "review", "M01-T01")
			proof := map[string]any{"pass": true, "at": "new evidence for the same candidate"}
			evidence := &c.State.Packets["M01-T01"].Evidence
			switch field {
			case "l1":
				evidence.L1 = proof
			case "final-gate":
				evidence.FinalGate = proof
			case "defect-injection":
				evidence.DefectInjection = proof
			}
			_, err := c.invokeScoped("review", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
				t.Fatal("changed proof cannot authorize a second reviewer or rebase its stored verdict")
				return drivers.Result{}
			})
			require.ErrorContains(t, err, "original verification evidence changed")
			require.False(t, c.sessionRecord("review", "M01-T01").ResumeCandidate)
		})
	}
}

func TestSessionJudgeGuardCannotBeRebasedByNewGateEvidence(t *testing.T) {
	fx, c := sessionFixture(t)
	require.True(t, c.milestoneGateStep(c.Spec.Milestones[0]).Pass)
	a := sessionArgs(c)
	_, err := c.invokeScoped("judge", "M-01", a, func(a drivers.CallArgs) drivers.Result {
		writeFile(t, filepath.Join(a.Cwd, "judge-edited.txt"), "unreviewed code\n")
		bumpFaultRevision(t, fx)
		return sessionResult(a, passVerdict())
	})
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	c = restartSessionGuardContext(t, fx, c)
	require.NoError(t, c.reconcileSessionReceipts())
	require.True(t, c.milestoneGateStep(c.Spec.Milestones[0]).Pass)
	_, err = c.invokeScoped("judge", "M-01", sessionArgs(c), func(drivers.CallArgs) drivers.Result {
		t.Fatal("new milestone gate evidence cannot validate the old judge's mutation")
		return drivers.Result{}
	})
	require.ErrorContains(t, err, "original verification evidence changed")
	require.True(t, c.sessionGuardViolated("judge", "M-01"))
	require.False(t, c.sessionRecord("judge", "M-01").ResumeCandidate)
}

func TestSessionQuickReviewReceiptPreservesItsOriginalFinalGate(t *testing.T) {
	fx := newQuickFixture(t, true)
	sd := quickDrivers()
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		bumpFaultRevision(t, fx)
		return quickVerdict("pass", true)
	}
	opts := sessionRunOpts(fx, sessionDriverSet(sd))
	_, err := RunTask(opts)
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	before := loadFixtureState(t, fx)
	originalGate := sessionHash(before.Packets["M01-T01"].Evidence.FinalGate)
	require.NotNil(t, before.Packets["M01-T01"].Evidence.FinalGate)
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	require.Len(t, sd.log.review, 1)
	require.Len(t, sd.log.exec, 1)
	after := loadFixtureState(t, fx)
	require.Equal(t, originalGate, sessionHash(after.Packets["M01-T01"].Evidence.FinalGate))
	require.Equal(t, int64(360), after.Budget.Totals.Tokens)
}

func TestSessionFullSampledReviewReceiptPreservesItsOriginalDefectProof(t *testing.T) {
	fx := newGitFixture(t)
	sd := newScripted(nil)
	injected := false
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		if !injected {
			injected = true
			bumpFaultRevision(t, fx)
		}
		return sd.mock.Review(a)
	}
	opts := sessionRunOpts(fx, sessionDriverSet(sd))
	opts.Config.DefectInjection.EveryNUnits = new(1)
	opts.Config.DefectInjection.Patch = writeDefectPatch(t, &fx)
	checks := filepath.Join(t.TempDir(), "checks")
	probe := filepath.Join(t.TempDir(), "check.sh")
	writeFile(t, probe, fmt.Sprintf("#!/bin/sh\nprintf x >> %q\ntest ! -f defect-marker\n", checks))
	opts.Config.DefectInjection.TestCmd = fmt.Sprintf("sh %q", probe)
	_, err := RunTask(opts)
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	before := loadFixtureState(t, fx)
	originalProof := sessionHash(before.Packets["M01-T01"].Evidence.DefectInjection)
	require.Equal(t, "pass", before.Packets["M01-T01"].Evidence.DefectInjection.(map[string]any)["outcome"])
	runs, err := os.ReadFile(checks)
	require.NoError(t, err)
	require.Len(t, runs, 2, "one original baseline check and one injected check")

	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	require.Len(t, sd.log.exec, 2)
	require.Len(t, sd.log.review, 2, "the first unit's stored Review must be consumed without another reviewer")
	after := loadFixtureState(t, fx)
	require.Equal(t, originalProof, sessionHash(after.Packets["M01-T01"].Evidence.DefectInjection))
	runs, err = os.ReadFile(checks)
	require.NoError(t, err)
	require.Len(t, runs, 4, "recovery must not rerun the first unit's sampled checks; only the second unit gets new checks")
	require.Equal(t, int64(6*180), after.Budget.Totals.Tokens)
}

func TestSessionPendingQuickReviewDoesNotReplaceStaleFinalGate(t *testing.T) {
	fx := newQuickFixture(t, true)
	sd := quickDrivers()
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		bumpFaultRevision(t, fx)
		return quickVerdict("pass", true)
	}
	opts := sessionRunOpts(fx, sessionDriverSet(sd))
	_, err := RunTask(opts)
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	st := loadFixtureState(t, fx)
	st.Packets["M01-T01"].Evidence.FinalGate.(map[string]any)["pass"] = false
	_, err = state.Save(fx.taskDir, st, nil)
	require.NoError(t, err)
	summary, err := RunTask(opts)
	require.ErrorContains(t, err, "no longer has its original passing gate")
	require.Equal(t, "escalated", summary.Status)
	require.Len(t, sd.log.review, 1)
	require.Len(t, sd.log.exec, 1)
	after := loadFixtureState(t, fx)
	require.False(t, after.Packets["M01-T01"].Evidence.FinalGate.(map[string]any)["pass"].(bool))
	require.False(t, after.Sessions.Records["review/final:M01-T01"].ResumeCandidate)
}

func TestSessionMissingRoleGuardsFailBeforeDispatchOrReplay(t *testing.T) {
	for _, scenario := range []string{"missing-packet", "review-contract", "judge-snapshot"} {
		t.Run("before/"+scenario, func(t *testing.T) {
			fx, c := sessionFixture(t)
			a := sessionArgs(c)
			role, scope := "review", "M01-T01"
			switch scenario {
			case "missing-packet":
				scope = "missing"
			case "review-contract":
				require.NoError(t, os.Remove(filepath.Join(c.TaskDir, "spec.md")))
			case "judge-snapshot":
				role, scope, a.Cwd = "judge", "M-01", t.TempDir()
				old := snapshotOpen
				t.Cleanup(func() { snapshotOpen = old })
				snapshotOpen = func(string) (io.ReadCloser, error) { return nil, os.ErrPermission }
			}
			_, err := c.invokeScoped(role, scope, a, func(drivers.CallArgs) drivers.Result {
				t.Fatal("the original role guard must exist before a model can be dispatched")
				return drivers.Result{}
			})
			require.Error(t, err)
			require.Nil(t, loadFixtureState(t, fx).Sessions)
		})
	}
	for _, scenario := range []string{"state-before", "receipt-before", "different-before", "receipt-after"} {
		t.Run("receipt/"+scenario, func(t *testing.T) {
			_, c, a, _ := faultRecordedSession(t, "review", "M01-T01")
			if scenario == "state-before" {
				c.sessionRecord("review", "M01-T01").Last.GuardBefore = ""
			} else {
				changeFaultReceipt(t, c, "review", "M01-T01", func(receipt *state.SessionReceipt) {
					switch scenario {
					case "receipt-before":
						receipt.GuardBefore = ""
					case "different-before":
						receipt.GuardBefore = strings.Repeat("f", 64)
					case "receipt-after":
						receipt.GuardAfter = ""
					}
				})
			}
			_, err := c.invokeScoped("review", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
				t.Fatal("a damaged guard cannot authorize repeating the invocation")
				return drivers.Result{}
			})
			if scenario == "receipt-after" {
				require.ErrorContains(t, err, "terminal role guard checkpoint is missing")
			} else {
				require.ErrorContains(t, err, "original role guard checkpoint is missing or mismatched")
			}
			require.False(t, c.sessionRecord("review", "M01-T01").ResumeCandidate)
		})
	}
	t.Run("lost-after-dispatch", func(t *testing.T) {
		fx, c := sessionFixture(t)
		a := sessionArgs(c)
		_, err := c.invokeScoped("review", "M01-T01", a, func(a drivers.CallArgs) drivers.Result {
			require.NoError(t, os.Remove(filepath.Join(c.TaskDir, "spec.md")))
			return sessionResult(a, passVerdict())
		})
		require.ErrorContains(t, err, "terminal candidate is unconfirmed")
		require.False(t, c.sessionRecord("review", "M01-T01").ResumeCandidate)
		writeFile(t, filepath.Join(fx.taskDir, "spec.md"), fixtureSpec)
		c = restartSessionGuardContext(t, fx, c)
		_, err = c.invokeScoped("review", "M01-T01", sessionArgs(c), func(drivers.CallArgs) drivers.Result {
			t.Fatal("restoring the file does not manufacture the missing post-call role guard")
			return drivers.Result{}
		})
		require.ErrorContains(t, err, "terminal role guard checkpoint is missing")
	})
}

func TestSessionRetiringGuardPreservesUnaccountedResultUntilConsumption(t *testing.T) {
	_, c, _, result := faultRecordedSession(t, "review", "M01-T01")
	c.retireSession("missing", "scope", "no record")
	c.retireSession("review", "M01-T01", "role permission boundary was violated")
	r := c.sessionRecord("review", "M01-T01")
	require.False(t, r.ResumeCandidate)
	require.Equal(t, "received", r.Last.Status, "retirement cannot silently lose attributable usage")
	require.False(t, r.Last.Accounted)
	c.recordSpawnMetrics(spawnMetricsArgs{role: "review", packetID: "M01-T01", result: &result})
	c.retireSession("review", "M01-T01", "role permission boundary was violated")
	require.Equal(t, "consumed", r.Last.Status)
	c.save()
	require.NoError(t, c.err())
}

func TestSessionKnownTerminalMissingProofStillAccountsAfterReceiptCAS(t *testing.T) {
	for _, role := range []string{"execute", "review"} {
		t.Run(role, func(t *testing.T) {
			fx, c := sessionFixture(t)
			a := sessionArgs(c)
			previousSnapshot := snapshotOpen
			t.Cleanup(func() { snapshotOpen = previousSnapshot })
			_, err := c.invokeScoped(role, "M01-T01", a, func(a drivers.CallArgs) drivers.Result {
				bumpFaultRevision(t, fx)
				if role == "execute" {
					snapshotOpen = func(string) (io.ReadCloser, error) { return nil, os.ErrPermission }
				} else {
					// The original guard still matches the intent. Only the after-call
					// authority read fails; the backend's usage identity remains known.
					require.NoError(t, os.Remove(filepath.Join(fx.taskDir, "spec.md")))
				}
				return sessionResult(a, drivers.MockResult(nil))
			})
			var conflict *state.CasConflictError
			require.ErrorAs(t, err, &conflict)
			require.Equal(t, "intent", loadFixtureState(t, fx).Sessions.Records[role+"/M01-T01"].Last.Status)
			snapshotOpen = previousSnapshot
			if role == "review" {
				writeFile(t, filepath.Join(fx.taskDir, "spec.md"), fixtureSpec)
			}
			for recovery := 0; recovery < 2; recovery++ {
				c = restartSessionGuardContext(t, fx, c)
				_, err = c.invokeScoped(role, "M01-T01", sessionArgs(c), func(drivers.CallArgs) drivers.Result {
					t.Fatal("missing terminal proof must never authorize replay or another model call")
					return drivers.Result{}
				})
				require.Error(t, err)
				record := c.sessionRecord(role, "M01-T01")
				require.True(t, record.Last.Accounted, "a missing post-call candidate or guard prevents applying the result, not accounting its measured usage")
				require.False(t, record.ResumeCandidate)
				require.Equal(t, int64(180), c.State.Budget.Totals.Tokens)
				raw, err := os.ReadFile(filepath.Join(fx.taskDir, "metrics.jsonl"))
				require.NoError(t, err)
				require.Len(t, strings.Split(strings.TrimSpace(string(raw)), "\n"), 1, "repeated blocked recovery must not double bill")
			}
		})
	}
}
