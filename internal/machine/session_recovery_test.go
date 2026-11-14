package machine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func p3Invocation(t *testing.T, role, scope, lastEvent string, loseReceipt bool) (fixture, *Context, drivers.CallArgs, drivers.Result) {
	t.Helper()
	fx, c := sessionFixture(t)
	a := sessionArgs(c)
	a.Binding.Harness = "codex-pro"
	a.Prompt = "current authoritative input"
	var completed drivers.Result
	previous := persistSessionReceipt
	if loseReceipt {
		persistSessionReceipt = func(task, key string, receipt state.SessionReceipt) (string, error) {
			if !strings.HasSuffix(key, "/lifecycle") {
				return "", errors.New("Go interrupted before terminal receipt")
			}
			return previous(task, key, receipt)
		}
	}
	t.Cleanup(func() { persistSessionReceipt = previous })
	_, err := c.invokeScoped(role, scope, a, func(call drivers.CallArgs) drivers.Result {
		completed = sessionResult(call, drivers.Result{Harness: "codex-pro", Model: call.Model, Settled: true, ExitCode: new(0), Text: "one completed model invocation", Usage: &drivers.Usage{Input: 10, Output: 2, TotalTokens: 12}})
		completed.Session.Protocol = "codex-pro/" + drivers.CodexProPackageVersion + "/worker-v3"
		e := drivers.MachineLifecycle{LifecycleVersion: 1, Type: "lifecycle", RequestID: call.RequestID, RunID: completed.Session.RunID, ThreadID: &completed.Session.SessionID,
			Persistent: call.PersistentSession, Configuration: json.RawMessage(`{}`), Dispatch: "not-dispatched", Outcome: "unknown"}
		for i, stage := range []string{"thread-ready", "dispatch-intent", "dispatch-possible", "turn-accepted", "terminal", "cleanup-confirmed"} {
			e.Sequence, e.Event = i+1, stage
			if stage == "dispatch-possible" {
				e.Dispatch = "possible"
			}
			if stage == "turn-accepted" {
				e.Dispatch, e.TurnID = "accepted", &completed.Session.TurnID
			}
			if stage == "terminal" {
				e.Outcome, e.CleanupConfirmed = "completed", true
			}
			require.NoError(t, call.OnLifecycle(e))
			if stage == lastEvent {
				break
			}
		}
		if loseReceipt {
			return completed
		}
		lost := completed
		facts := *completed.Session
		lost.Session = &facts
		lost.Session.Outcome, lost.Session.Dispatch, lost.Session.UsageStatus = "unknown", "unknown", "unknown"
		lost.Session.CleanupConfirmed, lost.Session.ResumeCandidate, lost.Settled = false, false, false
		lost.Usage = nil
		return lost
	})
	require.Error(t, err)
	persistSessionReceipt = previous
	return fx, c, a, completed
}

func injectP3Recovery(t *testing.T, completed drivers.Result) *int {
	t.Helper()
	old := recoverSessionMachine
	t.Cleanup(func() { recoverSessionMachine = old })
	queries := new(int)
	recoverSessionMachine = func(a drivers.MachineRecoveryArgs) (drivers.Result, error) {
		*queries++
		require.Equal(t, completed.Session.RequestID, a.RequestID)
		require.Equal(t, completed.Session.RunID, a.RunID)
		require.Equal(t, completed.Model, a.Model)
		require.NotEmpty(t, a.RecoveryDirectory)
		return completed, nil
	}
	return queries
}

func TestSessionLifecycleRecoversReceiptAfterCleanupAcknowledgementWithoutReplay(t *testing.T) {
	for _, loseReceipt := range []bool{false, true} {
		fx, c, a, completed := p3Invocation(t, "execute", "M01-T01", "cleanup-confirmed", loseReceipt)
		queries := injectP3Recovery(t, completed)
		before := loadFixtureState(t, fx)
		r := before.Sessions.Records["execute/M01-T01"]
		assert.Equal(t, 6, r.Last.Recovery.Sequence)
		assert.Equal(t, "cleanup-confirmed", r.Last.Recovery.Event)
		c = restartSessionGuardContext(t, fx, c)
		out, err := c.invokeScoped("execute", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
			t.Fatal("recovery must not dispatch a model")
			return drivers.Result{}
		})
		require.NoError(t, err)
		assert.True(t, out.Recovered)
		assert.Equal(t, completed.Text, out.Text)
		assert.Equal(t, 1, *queries)
		assert.Equal(t, int64(12), c.State.Budget.Totals.Tokens)
		assert.Zero(t, c.State.Budget.UnknownUsageCalls)
		assert.Equal(t, int64(1), c.State.Budget.UnknownCostCalls)
		applySessionResult(t, c, "execute", "M01-T01", out)
		c = restartSessionGuardContext(t, fx, c)
		require.NoError(t, c.reconcileSessionReceipts())
		assert.Equal(t, 1, *queries)
		assert.Equal(t, int64(12), c.State.Budget.Totals.Tokens)
		assert.Equal(t, 1, c.sessionRecord("execute", "M01-T01").Quality.ConfirmedTurns)
	}
}

func TestSessionRecoveryWithoutTerminalCandidateOnlyAttributesUsage(t *testing.T) {
	for _, stage := range []string{"thread-ready", "dispatch-intent", "dispatch-possible", "turn-accepted", "terminal"} {
		t.Run(stage, func(t *testing.T) {
			fx, c, a, completed := p3Invocation(t, "review", "M01-T01", stage, false)
			queries := injectP3Recovery(t, completed)
			c = restartSessionGuardContext(t, fx, c)
			_, err := c.invokeScoped("review", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
				t.Fatal("unknown candidate must not replay")
				return drivers.Result{}
			})
			require.Error(t, err)
			assert.Equal(t, 1, *queries)
			assert.Equal(t, int64(12), c.State.Budget.Totals.Tokens)
			r := c.sessionRecord("review", "M01-T01")
			assert.Equal(t, "blocked", r.Last.Status)
			assert.True(t, r.Last.Accounted)
			assert.Empty(t, r.Last.GuardAfter)
			assert.Empty(t, r.Last.CandidateAfter)
			assert.False(t, r.ResumeCandidate)
			c = restartSessionGuardContext(t, fx, c)
			require.Error(t, c.reconcileSessionReceipts())
			assert.Equal(t, int64(12), c.State.Budget.Totals.Tokens)
		})
	}
}

func TestSessionRecoveryUnknownOutcomeDoesNotDiscardAttributableUsage(t *testing.T) {
	fx, c, a, completed := p3Invocation(t, "execute", "M01-T01", "cleanup-confirmed", false)
	facts := *completed.Session
	facts.Dispatch, facts.Outcome, facts.ResumeCandidate = "unknown", "unknown", false
	completed.Session, completed.Settled = &facts, false
	injectP3Recovery(t, completed)
	c = restartSessionGuardContext(t, fx, c)
	_, err := c.invokeScoped("execute", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
		t.Fatal("partial outcome cannot authorize another model turn")
		return drivers.Result{}
	})
	require.Error(t, err)
	assert.Equal(t, int64(12), c.State.Budget.Totals.Tokens)
	assert.Zero(t, c.State.Budget.UnknownUsageCalls)
	r := c.sessionRecord("execute", "M01-T01")
	assert.Equal(t, "blocked", r.Last.Status)
	assert.Equal(t, "unknown", r.Last.Outcome)
	assert.False(t, r.ResumeCandidate)
}

func TestSessionRecoveryUnopenedClaudeProposalDoesNotBecomeHistory(t *testing.T) {
	fx, c := sessionFixture(t)
	a := sessionArgs(c)
	a.Binding.Harness, a.Model, a.PermissionMode = "claude-code-pro", "sonnet", "danger-full-access"
	previous := persistSessionReceipt
	t.Cleanup(func() { persistSessionReceipt = previous })
	persistSessionReceipt = func(task, key string, receipt state.SessionReceipt) (string, error) {
		if !strings.HasSuffix(key, "/lifecycle") {
			return "", errors.New("Go interrupted before terminal receipt")
		}
		return previous(task, key, receipt)
	}
	var completed drivers.Result
	_, err := c.invokeScoped("execute", "M01-T01", a, func(call drivers.CallArgs) drivers.Result {
		require.True(t, call.PersistentSession)
		completed = drivers.Result{Harness: "claude-code-pro", Model: call.Model, Usage: &drivers.Usage{}, CostUSD: new(0.0),
			Session: &drivers.SessionOutcome{Protocol: "claude-code-pro/" + drivers.ClaudeCodeProPackageVersion + "/worker-v3",
				RequestID: call.RequestID, RunID: "claude-never-started", Dispatch: "not-dispatched", Outcome: "failed", CleanupConfirmed: true, UsageStatus: "complete"}}
		event := drivers.MachineLifecycle{LifecycleVersion: 1, Type: "lifecycle", RequestID: call.RequestID, RunID: completed.Session.RunID,
			ThreadID: new("11111111-1111-4111-8111-111111111111"), Persistent: true, Configuration: json.RawMessage("null"),
			Dispatch: "not-dispatched", Outcome: "unknown"}
		for i, stage := range []string{"dispatch-intent", "dispatch-possible", "terminal", "cleanup-confirmed"} {
			event.Sequence, event.Event = i+1, stage
			if stage == "dispatch-possible" {
				event.Dispatch = "possible"
			}
			if stage == "terminal" {
				event.Dispatch, event.Outcome, event.CleanupConfirmed = "not-dispatched", "failed", true
			}
			require.NoError(t, call.OnLifecycle(event))
		}
		return completed
	})
	require.Error(t, err)
	persistSessionReceipt = previous
	injectP3Recovery(t, completed)
	c = restartSessionGuardContext(t, fx, c)
	result, err := c.invokeScoped("execute", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
		t.Fatal("receipt recovery must not create another native session")
		return drivers.Result{}
	})
	require.NoError(t, err)
	assert.Equal(t, "not-dispatched", result.Session.Dispatch)
	assert.Empty(t, result.Session.SessionID)
	r := c.sessionRecord("execute", "M01-T01")
	assert.Equal(t, "received", r.Last.Status)
	assert.Empty(t, r.ThreadID)
	assert.False(t, r.ResumeCandidate)
	assert.Zero(t, c.State.Budget.UnknownUsageCalls)
	assert.Zero(t, c.State.Budget.UnknownCostCalls)
}

func TestSessionRecoveryKeepsOriginalRoleGuardAndStopsChangedCandidate(t *testing.T) {
	fx, c, a, completed := p3Invocation(t, "review", "M01-T01", "cleanup-confirmed", true)
	injectP3Recovery(t, completed)
	writeFile(t, filepath.Join(fx.root, "implementation-after-crash.txt"), "external change\n")
	c = restartSessionGuardContext(t, fx, c)
	_, err := c.invokeScoped("review", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
		t.Fatal("changed candidate cannot reuse old pass")
		return drivers.Result{}
	})
	require.ErrorContains(t, err, "candidate changed")
	assert.Equal(t, int64(12), c.State.Budget.Totals.Tokens)
}

func TestSessionRecoveryRetainsUnknownQueryAndRejectsReceiptIdentity(t *testing.T) {
	for _, scenario := range []string{"pending", "wrong-result", "wrong-thread", "wrong-turn", "contradictory-cleanup", "contradictory-outcome", "missing-checkpoint", "bad-directory", "bad-digest", "bad-event", "write-failure", "serialization"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, _, completed := p3Invocation(t, "execute", "M01-T01", "cleanup-confirmed", true)
			injectP3Recovery(t, completed)
			c = restartSessionGuardContext(t, fx, c)
			r := c.sessionRecord("execute", "M01-T01")
			switch scenario {
			case "pending":
				recoverSessionMachine = func(drivers.MachineRecoveryArgs) (drivers.Result, error) {
					return drivers.Result{}, errors.New("pending")
				}
			case "wrong-result":
				recoverSessionMachine = func(drivers.MachineRecoveryArgs) (drivers.Result, error) { return drivers.Result{}, nil }
			case "wrong-thread", "wrong-turn", "contradictory-cleanup", "contradictory-outcome":
				recoverSessionMachine = func(drivers.MachineRecoveryArgs) (drivers.Result, error) {
					result, facts := completed, *completed.Session
					result.Session = &facts
					switch scenario {
					case "wrong-thread":
						facts.SessionID = "unrelated-thread"
					case "wrong-turn":
						facts.TurnID = "unrelated-turn"
					case "contradictory-cleanup":
						facts.CleanupConfirmed = false
					case "contradictory-outcome":
						facts.Outcome = "failed"
					}
					return result, nil
				}
			case "missing-checkpoint":
				files, _ := filepath.Glob(filepath.Join(c.TaskDir, ".sessions", "*.json"))
				require.Len(t, files, 1)
				require.NoError(t, os.Remove(files[0]))
			case "bad-directory":
				r.Last.Recovery.Directory = t.TempDir()
			case "bad-digest":
				r.Last.Recovery.CheckpointDigest = strings.Repeat("0", 64)
			case "bad-event":
				checkpoint, _, err := state.ReadSessionReceipt(c.TaskDir, "execute/M01-T01/lifecycle", r.Last.ID, "")
				require.NoError(t, err)
				checkpoint.Result = json.RawMessage(`{"bad":true}`)
				_, err = state.WriteSessionReceipt(c.TaskDir, "execute/M01-T01/lifecycle", *checkpoint)
				require.NoError(t, err)
			case "write-failure":
				old := persistSessionReceipt
				t.Cleanup(func() { persistSessionReceipt = old })
				persistSessionReceipt = func(string, string, state.SessionReceipt) (string, error) { return "", os.ErrPermission }
			case "serialization":
				recoverSessionMachine = func(drivers.MachineRecoveryArgs) (drivers.Result, error) {
					result := completed
					result.CostUSD = new(math.NaN())
					return result, nil
				}
			}
			require.Error(t, c.recoverSessionReceipt("execute/M01-T01", r))
			if scenario == "pending" {
				require.ErrorContains(t, c.reconcileSessionReceipts(), "pending")
			}
		})
	}
}

func TestSessionLifecycleAdmissionCheckpointAndIOFailures(t *testing.T) {
	for _, scenario := range []string{"prepare", "cancel", "identity", "foreign-history", "snapshot", "guard", "serialization", "write", "cas"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c := sessionFixture(t)
			a := sessionArgs(c)
			a.Binding.Harness = "codex-pro"
			a.Prompt = "task"
			oldPrepare, oldPersist, oldSnapshot := prepareSessionRecovery, persistSessionReceipt, snapshotOpen
			t.Cleanup(func() {
				prepareSessionRecovery, persistSessionReceipt, snapshotOpen = oldPrepare, oldPersist, oldSnapshot
			})
			if scenario == "prepare" {
				prepareSessionRecovery = func(string, string) (string, error) { return "", os.ErrPermission }
			}
			called := false
			_, err := c.invokeScoped("review", "M01-T01", a, func(call drivers.CallArgs) drivers.Result {
				called = true
				r := c.sessionRecord("review", "M01-T01")
				thread := "owned-thread"
				e := drivers.MachineLifecycle{LifecycleVersion: 1, Type: "lifecycle", RequestID: call.RequestID, RunID: "run-one", Sequence: 1, Event: "cleanup-confirmed", ThreadID: &thread, Persistent: true, Configuration: json.RawMessage(`{}`), CleanupConfirmed: true}
				switch scenario {
				case "cancel":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					c.RunContext = ctx
				case "identity":
					e.RequestID = "foreign"
				case "foreign-history":
					other := *r
					other.ThreadID, other.Protocol = "owned-thread", "codex-pro/0.10.0/worker-v3"
					c.State.Sessions.Records["execute/M01-T01"] = &other
				case "snapshot":
					snapshotOpen = func(string) (io.ReadCloser, error) { return nil, os.ErrPermission }
				case "guard":
					delete(c.Packets, "M01-T01")
				case "serialization":
					e.Event = "thread-ready"
					e.Configuration = json.RawMessage(`{`)
				case "write":
					persistSessionReceipt = func(string, string, state.SessionReceipt) (string, error) { return "", os.ErrPermission }
				case "cas":
					bumpFaultRevision(t, fx)
				}
				require.Error(t, call.OnLifecycle(e))
				return drivers.Result{}
			})
			require.Error(t, err)
			assert.Equal(t, scenario != "prepare", called)
		})
	}
}

func TestSessionRecoveryCanConfirmUnsentAttemptWithoutSamplingANewGuard(t *testing.T) {
	for _, changed := range []bool{false, true} {
		fx, c, _, completed := p3Invocation(t, "review", "M01-T01", "thread-ready", true)
		completed.Session.Dispatch, completed.Session.Outcome, completed.Session.CleanupConfirmed = "not-dispatched", "failed", true
		completed.Session.ResumeAccepted, completed.Session.ResumeCandidate, completed.Settled = false, false, false
		completed.Usage, completed.CostUSD = &drivers.Usage{}, new(float64)
		injectP3Recovery(t, completed)
		if changed {
			writeFile(t, filepath.Join(fx.root, "after.txt"), "changed\n")
		}
		c = restartSessionGuardContext(t, fx, c)
		err := c.reconcileSessionReceipts()
		if changed {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			assert.Equal(t, "received", c.sessionRecord("review", "M01-T01").Last.Status)
		}
	}
}

func TestSessionRecoveryPreparationPreservesLocalAdmissionAndDefaults(t *testing.T) {
	_, c := sessionFixture(t)
	a := sessionArgs(c)
	r := &state.SessionRecord{Last: &state.SessionInvocation{ID: "call-config"}}
	a.Binding.Harness = "codex-pro"
	a.Model = ""
	a.Binding.Model = ""
	require.NoError(t, c.prepareSessionLifecycle(r, &a))
	assert.Nil(t, r.Last.Recovery)
	a.Binding.Model = "test"
	a.PermissionMode = ""
	require.NoError(t, c.prepareSessionLifecycle(r, &a))
	assert.Equal(t, "read-only", r.Last.Recovery.PermissionMode)
	a.Binding.Thinking = "max"
	require.NoError(t, c.prepareSessionLifecycle(r, &a))
	assert.Equal(t, "max", r.Last.Recovery.Effort)
	a.Binding.Harness, a.PermissionMode, a.Model = "claude-code-pro", "danger-full-access", "sonnet"
	require.NoError(t, c.prepareSessionLifecycle(r, &a))
	assert.Equal(t, drivers.ClaudeCodeProPackageVersion, r.Last.Recovery.AdapterVersion)
	r.Task, r.Cwd, r.Binding, r.Role, r.Scope = c.TaskDir, c.Cwd, strings.Repeat("b", 64), "execute", "M01-T01"
	event := drivers.MachineLifecycle{LifecycleVersion: 1, Type: "lifecycle", RequestID: r.Last.ID, RunID: "claude-run", Sequence: 1,
		Event: "dispatch-intent", Configuration: json.RawMessage("null"), Dispatch: "not-dispatched", Outcome: "unknown"}
	raw, err := json.Marshal(event)
	require.NoError(t, err)
	_, err = state.WriteSessionReceipt(c.TaskDir, "execute/M01-T01/lifecycle", state.SessionReceipt{Version: 1, InvocationID: r.Last.ID,
		Key: "execute/M01-T01/lifecycle", Task: c.TaskDir, Cwd: c.Cwd, Binding: r.Binding, Result: raw})
	require.NoError(t, err)
	_, got, err := c.loadSessionLifecycle(r)
	require.NoError(t, err)
	assert.Equal(t, "dispatch-intent", got.Event, "Claude cannot invent a pre-dispatch thread-ready")
}
