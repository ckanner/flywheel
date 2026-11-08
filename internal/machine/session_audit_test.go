package machine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestSessionAuditCancelledInvocationStillAccountsBeforeReturning(t *testing.T) {
	for _, mode := range []string{"scoped", "fresh"} {
		for _, terminal := range []bool{false, true} {
			for _, measured := range []bool{false, true} {
				name := mode
				if terminal {
					name += "/terminal"
				} else {
					name += "/unknown"
				}
				if measured {
					name += "/measured"
				} else {
					name += "/missing-usage"
				}
				t.Run(name, func(t *testing.T) {
					fx, c := sessionFixture(t)
					c.Config.Sessions.Mode = mode
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					c.RunContext = ctx
					a := sessionArgs(c)
					_, err := c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
						r := sessionResult(a, drivers.MockResult(nil))
						r.CostUSD = nil
						if !terminal {
							r.Cancelled, r.Settled = true, false
							r.Session.Outcome, r.Session.ResumeCandidate = "unknown", false
						}
						if !measured {
							r.Usage, r.Session.UsageStatus = nil, "unknown"
						}
						cancel()
						return r
					})
					require.Error(t, err)
					st := loadFixtureState(t, fx)
					record := st.Sessions.Records["execute/"+a.PacketID]
					require.True(t, record.Last.Accounted, "cancellation must not drop the performed invocation from accounting")
					if measured {
						require.EqualValues(t, 180, st.Budget.Totals.Tokens)
						require.Zero(t, st.Budget.UnknownUsageCalls)
					} else {
						require.Zero(t, st.Budget.Totals.Tokens)
						require.EqualValues(t, 1, st.Budget.UnknownUsageCalls)
					}
					require.EqualValues(t, 1, st.Budget.UnknownCostCalls)
					raw, err := os.ReadFile(filepath.Join(fx.taskDir, "metrics.jsonl"))
					require.NoError(t, err)
					require.Len(t, strings.Split(strings.TrimSpace(string(raw)), "\n"), 1)
					restarted := unitTestContext(t, fx)
					enableSessionPolicy(restarted)
					restarted.Config.Sessions.Mode = mode
					if terminal {
						r, err := restarted.invokeScoped("execute", a.PacketID, sessionArgs(restarted), func(drivers.CallArgs) drivers.Result {
							t.Fatal("a terminal receipt must be applied without dispatch")
							return drivers.Result{}
						})
						require.NoError(t, err)
						applySessionResult(t, restarted, "execute", a.PacketID, r)
					} else {
						require.Error(t, restarted.reconcileSessionReceipts(), "billing a cancelled call must not unblock unknown effects")
					}
					require.Equal(t, st.Budget.Totals, restarted.State.Budget.Totals)
					require.Equal(t, st.Budget.UnknownUsageCalls, restarted.State.Budget.UnknownUsageCalls)
				})
			}
		}
	}
}

func TestSessionAuditCancellationAfterIntentIsKnownNotDispatched(t *testing.T) {
	for _, resumed := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "resume"}[resumed], func(t *testing.T) {
			fx, c := sessionFixture(t)
			a := sessionArgs(c)
			thread, protocol := "", ""
			if resumed {
				r, err := c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result { return sessionResult(a, drivers.MockResult(nil)) })
				require.NoError(t, err)
				applySessionResult(t, c, "execute", a.PacketID, r)
				thread, protocol = r.Session.SessionID, r.Session.Protocol
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.RunContext = ctx
			previousNow := c.Now
			c.Now = func() time.Time { cancel(); return previousNow() }
			_, err := c.invokeScoped("execute", a.PacketID, a, func(drivers.CallArgs) drivers.Result {
				t.Fatal("the invocation was cancelled before dispatch")
				return drivers.Result{}
			})
			require.Error(t, err)
			st := loadFixtureState(t, fx)
			record := st.Sessions.Records["execute/"+a.PacketID]
			require.Equal(t, "not-dispatched", record.Last.Dispatch)
			require.True(t, record.Last.CleanupConfirmed)
			require.True(t, record.Last.Accounted)
			require.Equal(t, "consumed", record.Last.Status, "no stage output exists to replay after a locally cancelled admission")
			receipt, _, err := state.ReadSessionReceipt(fx.taskDir, "execute/"+a.PacketID, record.Last.ID, record.Last.ReceiptDigest)
			require.NoError(t, err)
			var admitted drivers.Result
			require.NoError(t, json.Unmarshal(receipt.Result, &admitted))
			require.Nil(t, admitted.ExitCode, "a local cancellation has no physical process exit")
			require.Equal(t, &drivers.Usage{}, admitted.Usage)
			require.Equal(t, new(float64), admitted.CostUSD)
			require.Zero(t, st.Budget.UnknownUsageCalls, "a proven local non-call must not block a future hard budget")
			require.Zero(t, st.Budget.UnknownCostCalls)
			require.EqualValues(t, map[bool]int{false: 0, true: 180}[resumed], st.Budget.Totals.Tokens)
			if resumed {
				require.Equal(t, protocol, record.Protocol)
			}
			restarted := unitTestContext(t, fx)
			enableSessionPolicy(restarted)
			calls := 0
			_, err = restarted.invokeScoped("execute", a.PacketID, sessionArgs(restarted), func(a drivers.CallArgs) drivers.Result {
				calls++
				require.Equal(t, thread, a.SessionID)
				return sessionResult(a, drivers.MockResult(nil))
			})
			require.NoError(t, err)
			require.Equal(t, 1, calls)
		})
	}
}

func TestSessionAuditUnknownReceiptCASRecoveryAccountsOnce(t *testing.T) {
	for _, mode := range []string{"scoped", "fresh"} {
		for _, scenario := range []string{"outcome", "cleanup", "dispatch", "plan", "accounting-cas"} {
			t.Run(mode+"/"+scenario, func(t *testing.T) {
				fx, c := sessionFixture(t)
				c.Config.Sessions.Mode = mode
				a := sessionArgs(c)
				role, scope := "execute", a.PacketID
				if scenario == "plan" {
					role, scope = "plan", "M-01"
				}
				_, err := c.invokeScoped(role, scope, a, func(a drivers.CallArgs) drivers.Result {
					r := sessionResult(a, drivers.MockResult(nil))
					r.Model, r.CostUSD, r.Session.ResumeCandidate = a.Model, nil, false
					switch scenario {
					case "cleanup":
						r.Session.CleanupConfirmed = false
						r.CleanupError = "owned process cleanup unconfirmed"
					case "dispatch":
						r.Session.Dispatch = "unknown"
					default:
						r.Session.Outcome = "unknown"
					}
					bumpFaultRevision(t, fx)
					return r
				})
				var conflict *state.CasConflictError
				require.ErrorAs(t, err, &conflict)
				st := loadFixtureState(t, fx)
				require.Equal(t, "intent", st.Sessions.Records[sessionKey(role, scope)].Last.Status)
				require.Zero(t, st.Budget.Totals.Tokens)
				if scenario == "accounting-cas" {
					restarted := unitTestContext(t, fx)
					enableSessionPolicy(restarted)
					restarted.Config.Sessions.Mode = mode
					before := restarted.Now
					injected := false
					restarted.Now = func() time.Time {
						if !injected {
							injected = true
							bumpFaultRevision(t, fx)
						}
						return before()
					}
					require.ErrorAs(t, restarted.reconcileSessionReceipts(), &conflict)
					require.True(t, injected)
					require.False(t, loadFixtureState(t, fx).Sessions.Records[sessionKey(role, scope)].Last.Accounted)
				}
				for i := 0; i < 2; i++ {
					restarted := unitTestContext(t, fx)
					enableSessionPolicy(restarted)
					restarted.Config.Sessions.Mode = mode
					_, err := restarted.invokeScoped(role, scope, sessionArgs(restarted), func(drivers.CallArgs) drivers.Result {
						t.Fatal("accounted unknown effects must still block every new dispatch")
						return drivers.Result{}
					})
					require.Error(t, err)
					st = loadFixtureState(t, fx)
					record := st.Sessions.Records[sessionKey(role, scope)]
					require.Equal(t, "blocked", record.Last.Status)
					require.True(t, record.Last.Accounted)
					require.False(t, record.ResumeCandidate)
					require.EqualValues(t, 180, st.Budget.Totals.Tokens)
					require.EqualValues(t, 1, st.Budget.UnknownCostCalls)
					require.Zero(t, st.Budget.UnknownUsageCalls)
				}
				raw, err := os.ReadFile(filepath.Join(fx.taskDir, "metrics.jsonl"))
				require.NoError(t, err)
				require.Len(t, strings.Split(strings.TrimSpace(string(raw)), "\n"), 1, "receipt/state CAS retries must not duplicate metrics")
			})
		}
	}
}

func TestSessionAuditCorruptOrForeignReceiptNeverAccounts(t *testing.T) {
	for _, scenario := range []string{"request", "protocol", "dispatch", "outcome", "usage-status", "usage", "cost"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, _, _ := faultRecordedSession(t, "execute", "M01-T01")
			changeFaultReceipt(t, c, "execute", "M01-T01", func(receipt *state.SessionReceipt) {
				var r drivers.Result
				require.NoError(t, json.Unmarshal(receipt.Result, &r))
				r.Usage = drivers.MockResult(nil).Usage
				r.Session.Outcome = "unknown"
				switch scenario {
				case "request":
					r.Session.RequestID = "call-unrelated"
				case "protocol":
					r.Session.Protocol = ""
				case "dispatch":
					r.Session.Dispatch = "invented"
				case "outcome":
					r.Session.Outcome = "invented"
				case "usage-status":
					r.Session.UsageStatus = "invented"
				case "usage":
					r.Usage.Input = -1
				case "cost":
					r.CostUSD = new(-1.0)
				}
				receipt.Result = mustJSON(r)
			})
			c.sessionRecord("execute", "M01-T01").Last.Status = "intent"
			c.save()
			require.NoError(t, c.err())
			require.Error(t, c.reconcileSessionReceipts())
			st := loadFixtureState(t, fx)
			require.False(t, st.Sessions.Records["execute/M01-T01"].Last.Accounted)
			require.Zero(t, st.Budget.Totals.Tokens)
			require.Zero(t, st.Budget.UnknownUsageCalls)
			_, err := os.Stat(filepath.Join(fx.taskDir, "metrics.jsonl"))
			require.True(t, os.IsNotExist(err))
		})
	}
}

func TestSessionAuditReceivedUsageAccountsBeforeReplayPreconditions(t *testing.T) {
	for _, scenario := range []string{"binding", "goal", "contract", "candidate", "review-evidence"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c := sessionFixture(t)
			a := sessionArgs(c)
			role := "execute"
			if scenario == "review-evidence" {
				role = "review"
			}
			_, err := c.invokeScoped(role, a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
				return sessionResult(a, drivers.MockResult(nil))
			})
			require.NoError(t, err)
			require.False(t, loadFixtureState(t, fx).Sessions.Records[sessionKey(role, a.PacketID)].Last.Accounted)
			switch scenario {
			case "binding":
				a.Binding.Model = "different-model"
			case "goal":
				c.Spec.Goal = "a different approved objective"
			case "contract":
				c.Spec.SpecVersion++
			case "candidate":
				writeFile(t, filepath.Join(fx.root, "changed-after-receipt.txt"), "different candidate")
			case "review-evidence":
				c.State.Packets[a.PacketID].Evidence.L1 = map[string]any{"identity": "a later verification"}
			}
			_, err = c.invokeScoped(role, a.PacketID, a, func(drivers.CallArgs) drivers.Result {
				t.Fatal("changed replay preconditions must never authorize another invocation")
				return drivers.Result{}
			})
			require.Error(t, err)
			st := loadFixtureState(t, fx)
			r := st.Sessions.Records[sessionKey(role, a.PacketID)]
			require.True(t, r.Last.Accounted)
			require.Equal(t, "received", r.Last.Status, "accounting is not stage application")
			require.False(t, r.ResumeCandidate)
			require.EqualValues(t, 180, st.Budget.Totals.Tokens)
			raw, err := os.ReadFile(filepath.Join(fx.taskDir, "metrics.jsonl"))
			require.NoError(t, err)
			require.Len(t, strings.Split(strings.TrimSpace(string(raw)), "\n"), 1)
		})
	}
}

func TestSessionAuditCancelledAccountingFailureRemainsRecoverable(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown", true: "terminal"}[terminal], func(t *testing.T) {
			fx, c := sessionFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.RunContext, c.metricsFile = ctx, t.TempDir()
			a := sessionArgs(c)
			_, err := c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
				r := sessionResult(a, drivers.MockResult(nil))
				if !terminal {
					r.Session.Outcome, r.Session.ResumeCandidate = "unknown", false
				}
				cancel()
				return r
			})
			require.ErrorContains(t, err, "metrics could not be persisted")
			require.False(t, loadFixtureState(t, fx).Sessions.Records["execute/"+a.PacketID].Last.Accounted)
			restarted := unitTestContext(t, fx)
			enableSessionPolicy(restarted)
			if terminal {
				r, err := restarted.invokeScoped("execute", a.PacketID, sessionArgs(restarted), func(drivers.CallArgs) drivers.Result {
					t.Fatal("a metrics write failure must not redispatch the recorded invocation")
					return drivers.Result{}
				})
				require.NoError(t, err)
				applySessionResult(t, restarted, "execute", a.PacketID, r)
			} else {
				require.Error(t, restarted.reconcileSessionReceipts())
			}
			st := loadFixtureState(t, fx)
			require.True(t, st.Sessions.Records["execute/"+a.PacketID].Last.Accounted)
			require.EqualValues(t, 180, st.Budget.Totals.Tokens)
		})
	}
}

func TestSessionAuditReviewCASRecoveryPreservesOriginalEvidence(t *testing.T) {
	for _, quick := range []bool{false, true} {
		t.Run(map[bool]string{false: "full", true: "quick"}[quick], func(t *testing.T) {
			var fx fixture
			var sd *scriptedDrivers
			if quick {
				fx, sd = newQuickFixture(t, false), quickDrivers()
			} else {
				fx, sd = newGitFixture(t), newScripted(nil)
			}
			injected := false
			sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
				if !injected {
					injected = true
					bumpFaultRevision(t, fx)
				}
				if quick {
					return quickVerdict("pass", true)
				}
				return sd.mock.Review(a)
			}
			opts := sessionRunOpts(fx, sessionDriverSet(sd))
			_, err := RunTask(opts)
			var conflict *state.CasConflictError
			require.ErrorAs(t, err, &conflict)
			result, err := RunTask(opts)
			require.NoError(t, err)
			require.Equal(t, "completed", result.Status)
			calls, stages := 2, 6
			if quick {
				calls, stages = 1, 2
			}
			require.Len(t, sd.log.exec, calls)
			require.Len(t, sd.log.review, calls, "recovery must not request a new verdict")
			require.EqualValues(t, stages*180, loadFixtureState(t, fx).Budget.Totals.Tokens)
		})
	}
}
