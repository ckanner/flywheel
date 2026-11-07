package machine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func faultRecordedSession(t *testing.T, role, scope string) (fixture, *Context, drivers.CallArgs, drivers.Result) {
	t.Helper()
	fx, c := sessionFixture(t)
	a := sessionArgs(c)
	result, err := c.invokeScoped(role, scope, a, func(a drivers.CallArgs) drivers.Result {
		return sessionResult(a, drivers.Result{Settled: true, ExitCode: new(0), Text: "confirmed invocation"})
	})
	require.NoError(t, err)
	require.True(t, c.sessionReplay(role, scope))
	return fx, c, a, result
}

func changeFaultReceipt(t *testing.T, c *Context, role, scope string, edit func(*state.SessionReceipt)) {
	t.Helper()
	key, record := sessionKey(role, scope), c.sessionRecord(role, scope)
	receipt, _, err := state.ReadSessionReceipt(c.TaskDir, key, record.Last.ID, record.Last.ReceiptDigest)
	require.NoError(t, err)
	edit(receipt)
	digest, err := state.WriteSessionReceipt(c.TaskDir, key, *receipt)
	require.NoError(t, err)
	record.Last.ReceiptDigest = digest
}

func bumpFaultRevision(t *testing.T, fx fixture) {
	t.Helper()
	current := loadFixtureState(t, fx)
	_, err := state.Save(fx.taskDir, current, nil)
	require.NoError(t, err)
}

func TestSessionFaultStoredReceiptMustRetainBindingAndKnownFacts(t *testing.T) {
	for _, scenario := range []string{"missing", "corrupt", "cwd", "binding", "typed-result", "no-facts", "wrong-request", "unknown-dispatch", "unknown-outcome", "cleanup", "cleanup-error"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, a, _ := faultRecordedSession(t, "execute", "M01-T01")
			r := c.sessionRecord("execute", "M01-T01")
			if scenario == "missing" || scenario == "corrupt" {
				files, err := filepath.Glob(filepath.Join(fx.taskDir, ".sessions", "*.json"))
				require.NoError(t, err)
				require.Len(t, files, 1)
				if scenario == "missing" {
					require.NoError(t, os.Remove(files[0]))
				} else {
					require.NoError(t, os.WriteFile(files[0], []byte("{\"truncated\":"), 0o600))
				}
			} else {
				changeFaultReceipt(t, c, "execute", "M01-T01", func(receipt *state.SessionReceipt) {
					switch scenario {
					case "cwd":
						receipt.Cwd = t.TempDir()
					case "binding":
						receipt.Binding = "different-binding"
					case "typed-result":
						receipt.Result = json.RawMessage(`{"settled":"not-a-bool"}`)
					default:
						var result drivers.Result
						require.NoError(t, json.Unmarshal(receipt.Result, &result))
						switch scenario {
						case "no-facts":
							result.Session = nil
						case "wrong-request":
							result.Session.RequestID = "another-invocation"
						case "unknown-dispatch":
							result.Session.Dispatch = "unknown"
						case "unknown-outcome":
							result.Session.Outcome = "unknown"
						case "cleanup":
							result.Session.CleanupConfirmed = false
						case "cleanup-error":
							result.CleanupError = "process group still unconfirmed"
						}
						receipt.Result = mustJSON(result)
					}
				})
			}
			calls := 0
			_, err := c.invokeScoped("execute", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
				calls++
				return drivers.Result{}
			})
			require.Error(t, err)
			assert.Zero(t, calls, "damaged receipt must not authorize a new model call")
			assert.Equal(t, "wait", r.Action)
			assert.Equal(t, "escalated", c.State.Status)
		})
	}
}

func TestSessionFaultIntentRecoveryNeedsTerminalCandidateAndCAS(t *testing.T) {
	for _, scenario := range []string{"missing-candidate", "cas"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, _, _ := faultRecordedSession(t, "execute", "M01-T01")
			r := c.sessionRecord("execute", "M01-T01")
			if scenario == "missing-candidate" {
				changeFaultReceipt(t, c, "execute", "M01-T01", func(receipt *state.SessionReceipt) { receipt.CandidateAfter = "" })
			}
			r.Last.Status = "intent"
			c.save()
			require.NoError(t, c.err())
			if scenario == "cas" {
				bumpFaultRevision(t, fx)
			}
			err := c.reconcileSessionReceipts()
			require.Error(t, err)
			if scenario == "cas" {
				var conflict *state.CasConflictError
				assert.ErrorAs(t, err, &conflict)
			} else {
				assert.Contains(t, err.Error(), "terminal candidate checkpoint is missing")
				assert.Equal(t, "blocked", r.Last.Status)
			}
		})
	}
}

func TestSessionFaultBeforeDispatchNeverCallsTheBackend(t *testing.T) {
	for _, scenario := range []string{"cancelled", "cwd", "snapshot", "intent-save"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c := sessionFixture(t)
			a := sessionArgs(c)
			switch scenario {
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.RunContext = ctx
			case "cwd":
				a.Cwd = filepath.Join(fx.root, "missing")
			case "snapshot":
				previous := snapshotOpen
				t.Cleanup(func() { snapshotOpen = previous })
				snapshotOpen = func(string) (io.ReadCloser, error) { return nil, os.ErrPermission }
			case "intent-save":
				bumpFaultRevision(t, fx)
			}
			calls := 0
			_, err := c.invokeScoped("execute", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
				calls++
				return drivers.Result{}
			})
			require.Error(t, err)
			assert.Zero(t, calls)
			assert.Nil(t, loadFixtureState(t, fx).Sessions, "a failed admission must not become a durable invocation")
		})
	}
}

func TestSessionFaultAfterDispatchCannotAuthorizeAnotherInvocation(t *testing.T) {
	for _, scenario := range []string{"serialization", "receipt-write", "candidate-snapshot", "state-cas"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c := sessionFixture(t)
			a := sessionArgs(c)
			previousSnapshot, previousPersist := snapshotOpen, persistSessionReceipt
			t.Cleanup(func() { snapshotOpen, persistSessionReceipt = previousSnapshot, previousPersist })
			calls := 0
			_, err := c.invokeScoped("execute", "M01-T01", a, func(a drivers.CallArgs) drivers.Result {
				calls++
				result := sessionResult(a, drivers.Result{Settled: true, ExitCode: new(0), Text: "completed before the storage fault"})
				switch scenario {
				case "serialization":
					cost := math.NaN()
					result.CostUSD = &cost
				case "receipt-write":
					persistSessionReceipt = func(string, string, state.SessionReceipt) (string, error) { return "", os.ErrPermission }
				case "candidate-snapshot":
					snapshotOpen = func(string) (io.ReadCloser, error) { return nil, os.ErrPermission }
				case "state-cas":
					bumpFaultRevision(t, fx)
				}
				return result
			})
			require.Error(t, err)
			require.Equal(t, 1, calls)
			snapshotOpen, persistSessionReceipt = previousSnapshot, previousPersist
			_, again := c.invokeScoped("execute", "M01-T01", a, func(drivers.CallArgs) drivers.Result { calls++; return drivers.Result{} })
			require.Error(t, again)
			assert.Equal(t, 1, calls)
			if scenario == "state-cas" {
				var conflict *state.CasConflictError
				require.ErrorAs(t, err, &conflict)
				restarted, createErr := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config, Drivers: c.Drivers})
				require.NoError(t, createErr)
				recovered, recoverErr := restarted.invokeScoped("execute", "M01-T01", sessionArgs(restarted), func(drivers.CallArgs) drivers.Result {
					calls++
					return drivers.Result{}
				})
				require.NoError(t, recoverErr)
				assert.Equal(t, "completed before the storage fault", recovered.Text)
				assert.Equal(t, 1, calls, "a persisted terminal receipt survives the state CAS conflict without another turn")
			}
		})
	}
}

func TestSessionFaultConsumedHistoryRetiresOnUnsupportedBackend(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "retire", true: "retirement-cas"}[conflict], func(t *testing.T) {
			fx, c, a, result := faultRecordedSession(t, "execute", "M01-T01")
			applySessionResult(t, c, "execute", "M01-T01", result)
			a.Binding.Harness = "omp"
			if conflict {
				bumpFaultRevision(t, fx)
			}
			calls := 0
			_, err := c.invokeScoped("execute", "M01-T01", a, func(a drivers.CallArgs) drivers.Result {
				calls++
				assert.Empty(t, a.RequestID)
				assert.False(t, a.PersistentSession)
				assert.Empty(t, a.SessionID)
				return drivers.Result{}
			})
			assert.Equal(t, !conflict, err == nil)
			assert.Equal(t, map[bool]int{false: 1, true: 0}[conflict], calls)
			assert.False(t, c.sessionRecord("execute", "M01-T01").ResumeCandidate)
			assert.Equal(t, "rebuild", c.sessionRecord("execute", "M01-T01").Action)
		})
	}
}

func TestSessionFaultUnknownInvocationBlocksChangedModeAndBackend(t *testing.T) {
	for _, change := range []string{"fresh", "unsupported", "mock"} {
		t.Run(change, func(t *testing.T) {
			fx, c := sessionFixture(t)
			a := sessionArgs(c)
			_, err := c.invokeScoped("execute", "M01-T01", a, func(a drivers.CallArgs) drivers.Result {
				r := sessionResult(a, drivers.Result{})
				r.Session.Outcome, r.Session.Dispatch, r.Session.ResumeCandidate = "unknown", "unknown", false
				return r
			})
			require.Error(t, err)
			restarted, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config, Drivers: c.Drivers})
			require.NoError(t, err)
			switch change {
			case "fresh":
				restarted.Config.Sessions.Mode = "fresh"
			case "unsupported":
				a.Binding.Harness = "omp"
			case "mock":
				restarted.Config.Drivers["execute"] = "mock"
			}
			calls := 0
			_, err = restarted.invokeScoped("execute", "M01-T01", a, func(drivers.CallArgs) drivers.Result { calls++; return drivers.Result{} })
			require.ErrorContains(t, err, "resume and fresh replay are blocked")
			assert.Zero(t, calls)
		})
	}
}

func TestSessionFaultPendingResultCannotBeReboundOrAppliedToChangedCandidate(t *testing.T) {
	for _, scenario := range []string{"binding", "goal", "candidate"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, a, _ := faultRecordedSession(t, "execute", "M01-T01")
			switch scenario {
			case "binding":
				a.Binding.Model = "another-model"
			case "goal":
				c.Spec.Goal = "a newly requested objective"
			case "candidate":
				writeFile(t, filepath.Join(fx.root, "changed-after-terminal.txt"), "new candidate")
			}
			calls := 0
			_, err := c.invokeScoped("execute", "M01-T01", a, func(drivers.CallArgs) drivers.Result { calls++; return drivers.Result{} })
			require.Error(t, err)
			assert.Zero(t, calls)
			assert.False(t, c.sessionRecord("execute", "M01-T01").ResumeCandidate)
		})
	}
}

func TestSessionFaultApplyOrderingPrecedesOtherRoleDispatch(t *testing.T) {
	_, c, a, _ := faultRecordedSession(t, "execute", "M01-T01")
	calls := 0
	_, err := c.invokeScoped("review", "M01-T01", a, func(drivers.CallArgs) drivers.Result { calls++; return drivers.Result{} })
	require.ErrorContains(t, err, "apply the recorded role result")
	assert.Zero(t, calls)
	assert.Equal(t, "received", c.sessionRecord("execute", "M01-T01").Last.Status)

	for _, pair := range [][2]string{{"judge", "judge-panel"}, {"judge-panel", "judge"}} {
		t.Run(pair[0]+" then "+pair[1], func(t *testing.T) {
			_, c, a, _ := faultRecordedSession(t, pair[0], "M-01")
			calls := 0
			_, err := c.invokeScoped(pair[1], "M-01", a, func(a drivers.CallArgs) drivers.Result {
				calls++
				r := sessionResult(a, drivers.Result{Settled: true, ExitCode: new(0)})
				r.Session.SessionID = "independent-" + pair[1]
				return r
			})
			require.NoError(t, err)
			assert.Equal(t, 1, calls)
			assert.True(t, c.sessionReplay(pair[0], "M-01"))
			assert.True(t, c.sessionReplay(pair[1], "M-01"))
		})
	}
}

func TestSessionFaultTransportRetryRechecksMeasuredBudget(t *testing.T) {
	for _, role := range []string{"execute", "plan", "review", "judge"} {
		t.Run(role, func(t *testing.T) {
			_, c := sessionFixture(t)
			c.Config.Budget.Enforce, c.Config.Budget.DailyTokenLimit = "hard", new(int64(5))
			c.State.Budget.Enforce, c.State.Budget.DailyTokenLimit = "hard", new(int64(5))
			c.Sleep = func(time.Duration) {}
			calls := 0
			failed := func(a drivers.CallArgs) drivers.Result {
				calls++
				message := "a confirmed native turn failed with a transient transport error"
				r := sessionResult(a, drivers.Result{ExitCode: new(1), ErrorCode: errs.TRANSPORT, ErrorMessage: &message,
					Usage: &drivers.Usage{Input: 8, Output: 2, TotalTokens: 10}})
				r.Session.Outcome, r.Session.ResumeCandidate = "failed", false
				return r
			}
			sd := newScripted(nil)
			sd.execFn, sd.planFn, sd.reviewFn, sd.judgeFn = failed, failed, failed, failed
			c.Drivers = sd.set()
			var stopped bool
			var err error
			switch role {
			case "execute":
				var outcome execOutcome
				outcome, err = c.executeStep(c.PacketList[0])
				stopped = outcome.Stopped
			case "plan":
				var outcome planOutcome
				outcome, err = c.planStep(c.Spec.Milestones[0], "")
				stopped = outcome.Stopped
			default:
				args := verdictArgs{role: role, binding: loopToBinding(c.Config.Loops[role]), prompt: "current evidence", packetID: "M01-T01"}
				if role == "judge" {
					args.packetID, args.sessionScope = "", "M-01"
				}
				var outcome verdictOutcome
				outcome, err = c.agentVerdictStep(args)
				stopped = outcome.Stopped
			}
			require.NoError(t, err)
			assert.True(t, stopped)
			assert.Equal(t, 1, calls, "a transport retry is a new paid call and must pass the budget again")
			assert.EqualValues(t, 10, c.State.Budget.Totals.Tokens)
		})
	}
}

func TestSessionFaultReceiptLoadFailureIsPreserved(t *testing.T) {
	_, c, _, _ := faultRecordedSession(t, "execute", "M01-T01")
	previous := loadSessionReceipt
	t.Cleanup(func() { loadSessionReceipt = previous })
	want := errors.New("receipt storage unavailable")
	loadSessionReceipt = func(string, string, string, string) (*state.SessionReceipt, string, error) { return nil, "", want }
	_, _, _, err := c.readSessionResult("execute/M01-T01", c.sessionRecord("execute", "M01-T01"))
	assert.ErrorIs(t, err, want)
}

func TestSessionFaultReceivedStepsApplyWithoutCurrentChannelSecret(t *testing.T) {
	for _, role := range []string{"execute", "plan", "review", "judge"} {
		t.Run(role, func(t *testing.T) {
			fx, c := sessionFixture(t)
			c.Config.Worktree.Enabled = new(false)
			for name, channel := range c.Config.Channels {
				channel.EnvKey = new("REPLAY_CHANNEL_KEY")
				c.Config.Channels[name] = channel
			}
			c.EnvLookup = func(name string) (string, bool) { return "available-before-dispatch", name == "REPLAY_CHANNEL_KEY" }
			calls := 0
			sd := newScripted(nil)
			run := func(c *Context) (bool, error) {
				switch role {
				case "execute":
					r, err := c.executeStep(c.PacketList[0])
					return r.Success, err
				case "plan":
					r, err := c.planStep(c.Spec.Milestones[0], "")
					return r.OK, err
				default:
					a := verdictArgs{role: role, binding: loopToBinding(c.Config.Loops[role]), prompt: "current evidence", packetID: "M01-T01"}
					if role == "judge" {
						a.packetID, a.sessionScope = "", "M-01"
					}
					r, err := c.agentVerdictStep(a)
					return r.OK, err
				}
			}
			complete := func(a drivers.CallArgs) drivers.Result {
				calls++
				require.Equal(t, "available-before-dispatch", a.APIKeys["REPLAY_CHANNEL_KEY"])
				var r drivers.Result
				switch role {
				case "execute":
					r = sd.mock.Execute(a)
				case "plan":
					r = sd.mock.Plan(a)
				case "review":
					r = sd.mock.Review(a)
				default:
					r = sd.mock.Judge(a)
				}
				bumpFaultRevision(t, fx) // terminal is durable; stage state cannot be saved
				return sessionResult(a, r)
			}
			sd.execFn, sd.planFn, sd.reviewFn, sd.judgeFn = complete, complete, complete, complete
			c.Drivers = sd.set()
			_, err := run(c)
			var conflict *state.CasConflictError
			require.ErrorAs(t, err, &conflict)
			require.Equal(t, 1, calls)
			restarted, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config, Drivers: c.Drivers,
				Env: func(string) (string, bool) { return "", false }})
			require.NoError(t, err)
			require.NoError(t, restarted.reconcileSessionReceipts())
			ok, err := run(restarted)
			require.NoError(t, err)
			assert.True(t, ok, "applying a persisted result does not require sending another authenticated request")
			assert.Equal(t, 1, calls)
			assert.EqualValues(t, 180, restarted.State.Budget.Totals.Tokens)
		})
	}
}

func TestSessionFaultCredentialReferencesStayBoundWithoutCopyingValues(t *testing.T) {
	_, c := sessionFixture(t)
	a := sessionArgs(c)
	channel := c.Config.Channels[a.Binding.Channel]
	channel.EnvKey = new("CHANNEL_KEY")
	c.Config.Channels[a.Binding.Channel] = channel
	p := c.Packets[a.PacketID]
	p.NeedsLiveCredentials = true
	c.State.Packets[a.PacketID].Evidence.Credentials = map[string]any{"injected-env": []any{"APPROVED_LIVE_KEY"}}
	a.APIKeys = map[string]string{"CHANNEL_KEY": "private-channel-value", "APPROVED_LIVE_KEY": "private-live-value"}
	before, _, _ := c.sessionIdentities(a.PacketID, a)
	assert.Equal(t, "private-live-value", a.APIKeys["APPROVED_LIVE_KEY"], "identity normalization must not mutate the actual child env")
	a.APIKeys = nil
	after, _, _ := c.sessionIdentities(a.PacketID, a)
	assert.Equal(t, before, after, "declared references survive unavailable secret values")
	c.State.Packets[a.PacketID].Evidence.Credentials = map[string]any{"injected-env": []any{"DIFFERENT_LIVE_KEY"}}
	changed, _, _ := c.sessionIdentities(a.PacketID, a)
	assert.NotEqual(t, before, changed, "changing approved injection names changes binding identity")
	p.NeedsLiveCredentials = false
	removed, _, _ := c.sessionIdentities(a.PacketID, a)
	assert.NotEqual(t, changed, removed, "removing credential injection cannot silently reuse the old binding")
}

func TestSessionFaultUncorrelatedOrForeignHandlesBlockBeforeAccounting(t *testing.T) {
	for _, scenario := range []string{"no-facts", "wrong-request", "wrong-resumed-session", "unaccepted-resume", "another-role-history"} {
		t.Run(scenario, func(t *testing.T) {
			_, c, a, first := faultRecordedSession(t, "execute", "M01-T01")
			applySessionResult(t, c, "execute", "M01-T01", first)
			role, scope := "execute", "M01-T01"
			if scenario == "another-role-history" {
				role = "review"
			}
			result, err := c.invokeScoped(role, scope, a, func(a drivers.CallArgs) drivers.Result {
				r := sessionResult(a, drivers.MockResult(nil))
				r.CostUSD = new(1.0)
				switch scenario {
				case "no-facts":
					r.Session = nil
				case "wrong-request":
					r.Session.RequestID = "different-invocation"
				case "wrong-resumed-session":
					r.Session.SessionID = "different-history"
				case "unaccepted-resume":
					r.Session.ResumeAccepted = false
				case "another-role-history":
					r.Session.SessionID = first.Session.SessionID
				}
				return r
			})
			require.ErrorContains(t, err, "resume and fresh replay are blocked")
			require.NotNil(t, result.Session)
			assert.Equal(t, "unknown", result.Session.Dispatch)
			assert.Equal(t, "unknown", result.Session.Outcome)
			assert.False(t, result.Session.ResumeCandidate)
			assert.Nil(t, result.Usage, "unattributable usage cannot be charged as this invocation")
			assert.Nil(t, result.CostUSD)
			assert.Equal(t, "blocked", c.sessionRecord(role, scope).Last.Status)
		})
	}
}

func TestSessionFaultLegacyPlanResumeStillTracksFreshModeInvocation(t *testing.T) {
	_, c := sessionFixture(t)
	c.Config.Sessions.Mode = "fresh"
	a := sessionArgs(c)
	a.Binding.Harness, a.SessionID = "kimi", "explicit-legacy-plan-history"
	r, err := c.invokeScoped("plan", "M-01", a, func(a drivers.CallArgs) drivers.Result {
		assert.False(t, a.PersistentSession)
		assert.NotEmpty(t, a.RequestID)
		assert.Equal(t, "explicit-legacy-plan-history", a.SessionID)
		return sessionResult(a, drivers.MockResult(nil))
	})
	require.NoError(t, err)
	record := c.sessionRecord("plan", "M-01")
	assert.Equal(t, "resume", record.Action)
	assert.Equal(t, "explicit-legacy-plan-history", record.Last.RequestedThread)
	assert.False(t, record.Last.Persistent)
	assert.Empty(t, record.ThreadID)
	assert.False(t, record.ResumeCandidate)
	applySessionResult(t, c, "plan", "M-01", r)
}
