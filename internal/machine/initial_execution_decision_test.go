package machine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestInitialSelectionAndFailureSwitchShareAttemptsUsageAndSwitchCap(t *testing.T) {
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	p := c.Packets["M01-T01"]
	server, calls := runtimeServer(t, func(choices map[string]string) string {
		if _, failed := choices["repair"]; failed {
			return "model-2"
		}
		return "model-1"
	}, nil)
	enableRuntimeJEV(t, c, server.URL)
	sd := failingFirstDriver(t)
	c.Drivers = sessionDriverSet(sd)
	out, err := c.runExecuteLoop(p)
	require.NoError(t, err)
	require.True(t, out.Green)
	require.Equal(t, int32(2), calls.Load())
	require.Len(t, sd.log.exec, 2)
	require.Equal(t, "high", sd.log.exec[0].Binding.Thinking)
	require.Equal(t, "opencode", sd.log.exec[1].Binding.Channel)
	require.Empty(t, sd.log.exec[0].SessionID)
	require.Empty(t, sd.log.exec[1].SessionID)
	ps, ledger := c.State.Packets[p.PacketID], c.State.RuntimeDecisions
	require.Equal(t, 2, ps.Attempts)
	require.Equal(t, 1, ps.FailedAttempts)
	require.Equal(t, 2, ps.ExecutionPolicy.Switches)
	require.Equal(t, "jev-initial-selection", ps.ExecutionPolicy.History[0].Kind)
	require.Zero(t, ps.ExecutionPolicy.History[0].Attempt)
	require.Equal(t, "jev-model-selection", ps.ExecutionPolicy.History[1].Kind)
	require.Equal(t, 2, ledger.Used)
	require.Equal(t, 3, ledger.Records[0].Snapshot.Version)
	require.Equal(t, 2, ledger.Records[1].Snapshot.Version)
	for _, record := range ledger.Records {
		require.True(t, record.Accounted)
		require.True(t, record.Selection.Adopted)
		require.NotNil(t, record.Execution)
		require.NotNil(t, record.Execution.Invocation, "each selected model keeps its own exact subsequent call")
		require.Equal(t, record.Attempt+1, record.Execution.Invocation.Attempt)
		require.Equal(t, "dispatched", record.Execution.Invocation.Dispatch)
	}
	require.NotEqual(t, ledger.Records[0].Execution.Invocation.ID, ledger.Records[1].Execution.Invocation.ID)
	require.Equal(t, ledger.Used, loadFixtureState(t, fx).RuntimeDecisions.Used)
	before := c.State.Budget.Totals.Tokens
	require.NoError(t, c.runtimeInitialExecutionDecision(p))
	require.Equal(t, int32(2), calls.Load())
	require.Equal(t, before, c.State.Budget.Totals.Tokens)
}

func TestInitialChoiceExplainsReviewedScopeAcceptanceAndNativeLimits(t *testing.T) {
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	addSwarmChoice(c)
	p := c.Packets["M01-T01"]
	collaborationFixture(p)
	server, calls := runtimeServer(t, func(choices map[string]string) string {
		require.Contains(t, choices["model-3"], `"mode":"swarm"`)
		require.Contains(t, choices["model-3"], "not a hard concurrency cap")
		return "model-3"
	}, nil)
	enableRuntimeJEV(t, c, server.URL)
	require.NoError(t, c.runtimeInitialExecutionDecision(p))
	require.Equal(t, int32(1), calls.Load())
	ps := c.State.Packets[p.PacketID]
	require.Zero(t, ps.Attempts, "selecting an executor is not an execution attempt")
	require.Equal(t, "swarm", ps.ExecutionPolicy.Current)
	require.NotNil(t, ps.ExecutionPolicy.CurrentBinding)
	require.Equal(t, "swarm", ps.ExecutionPolicy.CurrentBinding.BindingID)
	require.JSONEq(t, string(c.Config.NativeProfiles["swarm"].Options), string(ps.ExecutionPolicy.CurrentBinding.Binding.NativeOptions), "the selected native mode is frozen before an execution session exists")
	r := c.State.RuntimeDecisions.Records[0]
	raw, err := json.Marshal(r.Snapshot.Facts)
	require.NoError(t, err)
	for _, fact := range []string{p.Goal, `route`, `workflow`, `full`, `schedule`, `acceptance`, p.Acceptance.L1[0].Cmd, `decision-calls-used`, `decision-call-limit`, `budget-policy`, p.Collaboration.Tasks[0].Goal} {
		require.Contains(t, string(raw), fact)
	}
	reloaded := loadFixtureState(t, fx).Packets[p.PacketID].ExecutionPolicy
	require.Equal(t, "swarm", reloaded.Current)
	require.Equal(t, ps.ExecutionPolicy.CurrentBinding, jsonClone(reloaded.CurrentBinding))
	require.NoError(t, c.runtimeInitialExecutionDecision(p))
	require.Equal(t, int32(1), calls.Load(), "a selected but undispatched executor must not be selected again")
	ps.ExecutionPolicy = nil
	c.State.Workflow = &state.WorkflowState{WorkflowSelection: contract.WorkflowSelection{Effective: "quick"}}
	snapshot, err := c.initialExecutionSnapshot(p, c.revision, "known remaining call budget")
	require.NoError(t, err)
	require.Contains(t, snapshot.Facts[1].Text, `"workflow":"quick"`)
	require.Len(t, snapshot.Actions, 3, "Quick cannot receive a Swarm action even if its packet was assembled programmatically")
}

func TestInitialAdmissionSkipsUnavailableChoicesAndAuthority(t *testing.T) {
	_, base := sessionFixture(t)
	enableExecutionPolicy(base)
	server, calls := runtimeServer(t, func(map[string]string) string { return "model-1" }, nil)
	enableRuntimeJEV(t, base, server.URL)
	for name, mutate := range map[string]func(*Context, *contract.Packet){
		"disabled":                 func(c *Context, _ *contract.Packet) { c.Config.JEV.Enabled = false },
		"without execution policy": func(c *Context, _ *contract.Packet) { c.Config.ExecutionPolicy = nil },
		"already executed":         func(c *Context, p *contract.Packet) { c.State.Packets[p.PacketID].Attempts = 1 },
		"already replanned":        func(c *Context, p *contract.Packet) { c.State.Packets[p.PacketID].PlanRevisions = 1 },
		"pending failure":          func(c *Context, p *contract.Packet) { c.State.Packets[p.PacketID].PendingStrategy = "failure-existing" },
		"no api key":               func(c *Context, _ *contract.Packet) { c.EnvLookup = func(string) (string, bool) { return "", false } },
		"calls exhausted": func(c *Context, _ *contract.Packet) {
			l := c.initializeRuntimeDecisions(&c.Config.JEV)
			l.Used = c.Config.JEV.MaxCalls
		},
		"source unavailable": func(c *Context, _ *contract.Packet) {
			c.State.PreparationSource = &state.PreparationSource{RunID: "not-present"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := *base
			c.Config, c.State = jsonClone(base.Config), jsonClone(base.State)
			p := jsonClone(base.Packets["M01-T01"])
			mutate(&c, p)
			require.NoError(t, c.initialExecutionDecision(p))
			require.Zero(t, calls.Load())
		})
	}
	base.fatal = errors.New("stopped")
	require.ErrorContains(t, base.initialExecutionDecision(base.Packets["M01-T01"]), "stopped")
	base.fatal = nil
	// One legal binding and a malformed public fact cannot reserve a paid call.
	for _, single := range []bool{true, false} {
		c := *base
		c.Config, c.State = jsonClone(base.Config), jsonClone(base.State)
		p := jsonClone(base.Packets["M01-T01"])
		if single {
			route := c.Config.ExecutionPolicy.Routes["default"]
			route.MaxSwitches = 0
			c.Config.ExecutionPolicy.Routes["default"] = route
		} else {
			p.Goal = ""
		}
		persistRuntimeConfig(t, &c)
		require.NoError(t, c.initialExecutionDecision(p))
		require.Empty(t, c.State.RuntimeDecisions.Records)
	}
	require.Zero(t, calls.Load())
}

func TestInitialFailureFallsBackOnceWithoutAFreeDecision(t *testing.T) {
	for _, journalError := range []bool{false, true} {
		t.Run(map[bool]string{true: "journal error", false: "unknown HTTP response"}[journalError], func(t *testing.T) {
			fx, c := sessionFixture(t)
			enableExecutionPolicy(c)
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(500) }))
			defer server.Close()
			enableRuntimeJEV(t, c, server.URL)
			if journalError {
				writeFile(t, filepath.Join(c.TaskDir, ".investigation"), "blocked journal")
			}
			p := c.Packets["M01-T01"]
			require.NoError(t, c.initialExecutionDecision(p))
			require.NoError(t, c.initialExecutionDecision(p))
			require.Equal(t, 1, c.State.RuntimeDecisions.Used)
			require.EqualValues(t, 1, c.State.Budget.UnknownUsageCalls)
			require.Equal(t, "low", c.State.Packets[p.PacketID].ExecutionPolicy.Current)
			r := c.State.RuntimeDecisions.Records[0]
			require.False(t, r.Selection.Adopted)
			require.Equal(t, "baseline", r.Status)
			require.Equal(t, map[bool]int{true: 0, false: 1}[journalError], calls)
			require.Equal(t, 1, loadFixtureState(t, fx).RuntimeDecisions.Used)
		})
	}
}

func pendingInitialFixture(t *testing.T, c *Context, p *contract.Packet) *state.RuntimeDecisionRecord {
	t.Helper()
	ledger := c.initializeRuntimeDecisions(&c.Config.JEV)
	snapshot, err := c.initialExecutionSnapshot(p, c.revision+1, "The frozen task call budget has one reservation available.")
	require.NoError(t, err)
	r := &state.RuntimeDecisionRecord{ID: snapshot.ID, Packet: p.PacketID, StartedAt: c.nowISO(), Snapshot: &snapshot, Action: snapshot.BaselineID, Status: "pending"}
	ledger.Used++
	ledger.Records = append(ledger.Records, r)
	c.State.Packets[p.PacketID].PendingStrategy = r.ID
	c.save()
	require.NoError(t, c.err())
	return r
}

func TestInitialCrashRecoveryNeverResendsOrAdoptsLateSelection(t *testing.T) {
	for _, sent := range []bool{false, true} {
		t.Run(map[bool]string{true: "received but not committed", false: "reservation only"}[sent], func(t *testing.T) {
			fx, c := sessionFixture(t)
			enableExecutionPolicy(c)
			server, calls := runtimeServer(t, func(map[string]string) string { return "model-1" }, func() {
				st, err := state.Load(c.TaskDir)
				require.NoError(t, err)
				_, err = state.Save(c.TaskDir, st, &st.Revision)
				require.NoError(t, err)
			})
			enableRuntimeJEV(t, c, server.URL)
			p := c.Packets["M01-T01"]
			if sent {
				err := c.initialExecutionDecision(p)
				var conflict *state.CasConflictError
				require.ErrorAs(t, err, &conflict)
			} else {
				pendingInitialFixture(t, c, p)
			}
			loaded := loadFixtureState(t, fx)
			require.Equal(t, "pending", loaded.RuntimeDecisions.Records[0].Status)
			c.State, c.revision, c.fatal = loaded, loaded.Revision, nil
			if sent {
				require.Empty(t, c.resumeRuntimeDecision(p))
			} else {
				require.NoError(t, c.initialExecutionDecision(p))
			}
			require.NoError(t, c.err())
			sd := happyDrivers()
			c.Drivers = sessionDriverSet(sd)
			out, err := c.executeStep(p)
			require.NoError(t, err)
			require.True(t, out.Success)
			require.Equal(t, "low", sd.log.exec[0].Binding.Thinking)
			require.Equal(t, map[bool]int32{true: 1, false: 0}[sent], calls.Load())
			require.Equal(t, 1, c.State.RuntimeDecisions.Used)
			r := c.State.RuntimeDecisions.Records[0]
			require.False(t, r.Selection.Adopted)
			require.Equal(t, "recovered-without-adoption", r.Selection.Reason)
			require.Empty(t, c.State.Packets[p.PacketID].ExecutionPolicy.History)
		})
	}
}

func TestInitialReservationAndSnapshotFailuresCannotDispatch(t *testing.T) {
	for _, problem := range []string{"CAS", "missing contract", "invalid baseline", "policy disappeared"} {
		t.Run(problem, func(t *testing.T) {
			_, c := sessionFixture(t)
			enableExecutionPolicy(c)
			server, calls := runtimeServer(t, func(map[string]string) string { return "model-1" }, nil)
			enableRuntimeJEV(t, c, server.URL)
			p := c.Packets["M01-T01"]
			switch problem {
			case "CAS":
				c.revision--
			case "missing contract":
				require.NoError(t, os.Remove(filepath.Join(c.TaskDir, "task-packet-"+p.PacketID+".md")))
			case "invalid baseline":
				addSwarmChoice(c)
				route := c.Config.ExecutionPolicy.Routes["default"]
				route.Initial = "swarm"
				c.Config.ExecutionPolicy.Routes["default"] = route
				persistRuntimeConfig(t, c)
			case "policy disappeared":
				_, _, err := c.executionPolicy(p)
				require.NoError(t, err)
				route := c.Config.ExecutionPolicy.Routes["default"]
				route.MaxSwitches++
				c.Config.ExecutionPolicy.Routes["default"] = route
				persistRuntimeConfig(t, c)
			}
			require.Error(t, c.runtimeInitialExecutionDecision(p))
			require.Zero(t, calls.Load())
			require.Zero(t, c.State.Packets[p.PacketID].Attempts)
		})
	}
}

func TestInitialRecoveryRejectsInconsistentStatusAndCorruptJournal(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(map[bool]string{true: "journal", false: "state"}[corrupt], func(t *testing.T) {
			_, c := sessionFixture(t)
			enableExecutionPolicy(c)
			server, calls := runtimeServer(t, func(map[string]string) string { return "model-1" }, nil)
			enableRuntimeJEV(t, c, server.URL)
			p := c.Packets["M01-T01"]
			r := pendingInitialFixture(t, c, p)
			if corrupt {
				writeFile(t, filepath.Join(c.TaskDir, ".investigation"), "not a directory")
			} else {
				r.Status = "applied"
			}
			require.Empty(t, c.resumeRuntimeDecision(p))
			require.Error(t, c.err())
			require.Zero(t, calls.Load())
			require.NotEmpty(t, c.State.Packets[p.PacketID].PendingStrategy)
		})
	}
}

func TestInitialParallelChoicesReserveOneSharedCallBudget(t *testing.T) {
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	server, calls := runtimeServer(t, func(map[string]string) string { return "model-1" }, nil)
	enableRuntimeJEV(t, c, server.URL)
	c.Config.JEV.MaxCalls = 1
	persistRuntimeConfig(t, c)
	g, workers := runtimeWorkers(c, "M01-T01", "M01-T02")
	defer g.cancel()
	var wg sync.WaitGroup
	for i, w := range workers {
		wg.Add(1)
		go func(i int, w *Context) {
			defer wg.Done()
			id := []string{"M01-T01", "M01-T02"}[i]
			require.NoError(t, w.runtimeInitialExecutionDecision(c.Packets[id]))
			w.save()
		}(i, w)
	}
	wg.Wait()
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, 1, c.State.RuntimeDecisions.Used)
	require.Len(t, c.State.RuntimeDecisions.Records, 1)
	require.Equal(t, 1, loadFixtureState(t, fx).RuntimeDecisions.Used)
	for _, w := range workers {
		require.NoError(t, w.err())
	}
}

func TestInitialParallelMergeAndCancellationStopBeforeExecution(t *testing.T) {
	for _, merge := range []bool{false, true} {
		t.Run(map[bool]string{true: "merge", false: "cancel"}[merge], func(t *testing.T) {
			_, c := sessionFixture(t)
			enableExecutionPolicy(c)
			var cancel context.CancelFunc
			server, calls := runtimeServer(t, func(map[string]string) string { return "model-1" }, func() { cancel() })
			enableRuntimeJEV(t, c, server.URL)
			g, workers := runtimeWorkers(c, "M01-T01")
			cancel = g.cancel
			defer cancel()
			w := workers[0]
			if merge {
				w.Config.Worktree.Enabled = new(true)
			}
			err := w.runtimeInitialExecutionDecision(c.Packets["M01-T01"])
			require.Error(t, err)
			if merge {
				require.Contains(t, err.Error(), "worktree")
				require.Zero(t, calls.Load())
			} else {
				require.ErrorIs(t, err, context.Canceled)
				require.Equal(t, int32(1), calls.Load())
			}
			require.Zero(t, c.State.Packets["M01-T01"].Attempts)
		})
	}
}

func TestInitialNativeDecisionCleanupBlocksUntilOriginalReceiptIsConfirmed(t *testing.T) {
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	enableRuntimeSubscription(t, c)
	p := c.Packets["M01-T01"]
	calls, queries := 0, 0
	confirmed := false
	fakeRuntimeSubscription(t, func(prompt string) decision.SubscriptionResult {
		calls++
		r := runtimeSubscriptionReply(t, prompt)
		r.Native.CleanupConfirmed = false
		return r
	}, func(string) decision.SubscriptionResult {
		queries++
		return decision.SubscriptionResult{Native: decision.NativeExecution{RequestID: "jev-runtime-one", Dispatch: "accepted", Outcome: "completed", CleanupConfirmed: confirmed, ReceiptHash: strings.Repeat("a", 64)}}
	})
	c.EnvLookup = func(string) (string, bool) { t.Fatal("subscription decision read an API key"); return "", false }
	require.ErrorContains(t, c.runtimeInitialExecutionDecision(p), "cleanup requires reconciliation")
	require.Zero(t, c.State.Packets[p.PacketID].Attempts)
	loaded := loadFixtureState(t, fx)
	c.State, c.revision, c.fatal = loaded, loaded.Revision, nil
	c.resumeRuntimeDecision(p)
	require.ErrorContains(t, c.err(), "cleanup requires reconciliation")
	confirmed, c.fatal = true, nil
	before := c.State.Budget.UnknownUsageCalls
	c.resumeRuntimeDecision(p)
	require.NoError(t, c.err())
	require.Equal(t, 1, calls)
	require.Equal(t, 2, queries)
	require.Equal(t, before, c.State.Budget.UnknownUsageCalls)
	require.Equal(t, "low", c.State.Packets[p.PacketID].ExecutionPolicy.Current)
	require.False(t, c.State.RuntimeDecisions.Records[0].Selection.Adopted)
	require.NoError(t, c.initialExecutionDecision(p))
	require.Equal(t, 1, calls)
}

func TestInitialSubscriptionChoiceUsesTheSameFrozenBindingAndImportedBudget(t *testing.T) {
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	enableRuntimeSubscription(t, c)
	p := c.Packets["M01-T01"]
	ledger := c.initializeRuntimeDecisions(&c.Config.JEV)
	ledger.ImportedCalls, ledger.Used = 2, 2
	calls := 0
	fakeRuntimeSubscription(t, func(prompt string) decision.SubscriptionResult {
		calls++
		require.Contains(t, prompt, `decision-calls-used`)
		return runtimeSubscriptionReply(t, prompt)
	}, nil)
	c.EnvLookup = func(string) (string, bool) { t.Fatal("subscription decision read an API key"); return "", false }
	require.NoError(t, c.runtimeInitialExecutionDecision(p))
	require.Equal(t, 1, calls)
	require.Equal(t, 3, ledger.Used)
	require.Equal(t, "hydrogen-local", ledger.Subscription.Profile)
	require.True(t, ledger.Records[0].Selection.Adopted)
	require.Equal(t, "high", c.State.Packets[p.PacketID].ExecutionPolicy.Current)
	require.Contains(t, ledger.Records[0].Snapshot.Facts[2].Text, `"decision-calls-used":2`)
	require.Contains(t, ledger.Records[0].Snapshot.Facts[2].Text, `"decision-calls-remaining":6`)
	require.Nil(t, ledger.Records[0].Receipt.Usage.Input, "native child usage must not be invented")
	require.EqualValues(t, 1, c.State.Budget.UnknownUsageCalls)
	require.Equal(t, 3, loadFixtureState(t, fx).RuntimeDecisions.Used)
}

func TestInitialDecisionCannotBypassAnotherPacketsPendingReservation(t *testing.T) {
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	server, calls := runtimeServer(t, func(map[string]string) string { return "model-1" }, nil)
	enableRuntimeJEV(t, c, server.URL)
	c.Config.JEV.MaxCalls = 1
	persistRuntimeConfig(t, c)
	first, next := c.Packets["M01-T01"], c.Packets["M01-T02"]
	pendingInitialFixture(t, c, first)
	loaded := loadFixtureState(t, fx)
	c.State, c.revision = loaded, loaded.Revision
	sd := happyDrivers()
	c.Drivers = sessionDriverSet(sd)
	_, err := c.executeStep(next)
	require.ErrorContains(t, err, "M01-T01 has an unresolved execution strategy")
	require.Empty(t, sd.log.exec)
	require.Zero(t, calls.Load())
	require.Zero(t, c.State.Packets[next.PacketID].Attempts)
	require.NoError(t, c.initialExecutionDecision(first), "recovery reads the original reservation without a second request")
	out, err := c.executeStep(next)
	require.NoError(t, err)
	require.True(t, out.Success)
	require.Len(t, sd.log.exec, 1)
	require.Zero(t, calls.Load(), "the recovered unknown reservation still consumes the only call")
	require.Equal(t, 1, c.State.RuntimeDecisions.Used)
	require.Equal(t, "baseline", c.State.RuntimeDecisions.Records[0].Status)
}

func TestTaskWideExecutionBarrierDistinguishesUnresolvedEffectsFromKnownFailures(t *testing.T) {
	_, c := sessionFixture(t)
	p := c.Packets["M01-T01"]
	r := &state.RuntimeDecisionRecord{Packet: "M01-T02", Snapshot: &decision.Snapshot{Version: 2}}
	c.State.RuntimeDecisions = &state.RuntimeDecisions{Records: []*state.RuntimeDecisionRecord{r}}
	for _, status := range []string{"pending", "feedback-running", "feedback-unknown", "feedback-stale", "replan-running"} {
		r.Status = status
		require.ErrorContains(t, c.runtimeExecutionBarrier(p), "unresolved execution strategy")
	}
	for _, status := range []string{"baseline", "stale", "applied", "feedback-completed", "completed"} {
		r.Status = status
		require.NoError(t, c.runtimeExecutionBarrier(p), "known terminal failures must not permanently block other packets")
	}
	r.Packet, r.Status, r.Action = p.PacketID, "applied", "replan"
	require.ErrorContains(t, c.runtimeExecutionBarrier(p), "pending replan")
	r.Packet = "M01-T02"
	require.NoError(t, c.runtimeExecutionBarrier(p), "an undispatched replan request is not an unknown side effect in another packet")
}

func TestInitialChoiceDoesNotRunWhileAnExecutorInvocationIsUnresolved(t *testing.T) {
	fx, c, _, _ := faultRecordedSession(t, "execute", "M01-T01")
	enableExecutionPolicy(c)
	server, calls := runtimeServer(t, func(map[string]string) string { return "model-1" }, nil)
	enableRuntimeJEV(t, c, server.URL)
	p := c.Packets["M01-T01"]
	require.NoError(t, c.initialExecutionDecision(p))
	r := c.sessionRecord("execute", p.PacketID)
	r.Last.Status = "intent"
	files, err := filepath.Glob(filepath.Join(fx.taskDir, ".sessions", "*.json"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.NoError(t, os.Remove(files[0]))
	_, err = c.executeStep(p)
	require.Error(t, err)
	require.Zero(t, calls.Load())
	require.Nil(t, c.State.RuntimeDecisions)
}

func TestInitialSelectionApplicationRejectsUnavailableBindings(t *testing.T) {
	for _, problem := range []string{"unknown id", "out of range", "negative", "missing policy", "changed policy", "switch cap", "ineligible swarm"} {
		t.Run(problem, func(t *testing.T) {
			_, c := sessionFixture(t)
			enableExecutionPolicy(c)
			p := c.Packets["M01-T01"]
			r := &state.RuntimeDecisionRecord{ID: "initial-" + p.PacketID, Action: "model-1", Selection: &decision.Selection{Adopted: true, Reason: "approved-model-choice"}}
			switch problem {
			case "unknown id":
				r.Action = "model-unknown"
			case "out of range":
				r.Action = "model-100"
			case "negative":
				r.Action = "model--1"
			case "missing policy":
				c.Config.ExecutionPolicy = nil
			case "changed policy":
				_, _, err := c.executionPolicy(p)
				require.NoError(t, err)
				binding := c.Config.ExecutionPolicy.Bindings["high"]
				binding.Thinking = "medium"
				c.Config.ExecutionPolicy.Bindings["high"] = binding
			case "switch cap":
				route := c.Config.ExecutionPolicy.Routes["default"]
				route.MaxSwitches = 0
				c.Config.ExecutionPolicy.Routes["default"] = route
			case "ineligible swarm":
				addSwarmChoice(c)
				r.Action = "model-3"
			}
			c.applyInitialExecutionSelection(p, r)
			require.Equal(t, "stale", r.Status)
			if policy := c.State.Packets[p.PacketID].ExecutionPolicy; policy != nil {
				require.Empty(t, policy.History)
				require.Equal(t, "low", policy.Current)
			}
			require.Zero(t, c.State.Packets[p.PacketID].Attempts)
		})
	}
}

func TestRuntimeCandidateAndNativeConfigurationChangesInvalidateSelection(t *testing.T) {
	for _, initial := range []bool{false, true} {
		t.Run(map[bool]string{true: "initial", false: "failure"}[initial], func(t *testing.T) {
			_, c := sessionFixture(t)
			enableExecutionPolicy(c)
			p := c.Packets["M01-T01"]
			server, calls := runtimeServer(t, func(map[string]string) string { return "model-1" }, func() {
				writeFile(t, filepath.Join(c.Cwd, "build", "other-change.txt"), "another process changed the reviewed candidate")
			})
			enableRuntimeJEV(t, c, server.URL)
			if initial {
				require.ErrorContains(t, c.initialExecutionDecision(p), "inputs changed")
			} else {
				facts := runtimeFailureFacts(t, c, p)
				require.Empty(t, c.failureDecision(p, errL1Failed, facts, false))
				require.NoError(t, c.err())
			}
			require.Equal(t, int32(1), calls.Load())
			require.Equal(t, "stale", c.State.RuntimeDecisions.Records[0].Status)
			require.False(t, c.State.RuntimeDecisions.Records[0].Selection.Adopted)
			require.Empty(t, c.State.Packets[p.PacketID].ExecutionPolicy.History)
		})
	}
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	addSwarmChoice(c)
	enableRuntimeJEV(t, c, "https://unused.invalid")
	copy := *c
	copy.Config = jsonClone(c.Config)
	copy.Config.NativeProfiles["unrelated"] = copy.Config.NativeProfiles["swarm"]
	persistRuntimeConfig(t, &copy)
	active, status := c.runtimeJEVConfig(nil)
	require.NotNil(t, active)
	require.Equal(t, "enabled", status)
	profile := copy.Config.NativeProfiles["swarm"]
	profile.Options = json.RawMessage(`{"version":1,"mode":"swarm","maxObservedAgents":8}`)
	copy.Config.NativeProfiles["swarm"] = profile
	persistRuntimeConfig(t, &copy)
	active, status = c.runtimeJEVConfig(nil)
	require.Nil(t, active)
	require.Equal(t, "execution-policy-changed", status)
}
