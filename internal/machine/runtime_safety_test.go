package machine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/circuitbreaker"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/investigation"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

// Use an actual consumed executor invocation and an authoritative failed gate.
// Delay the optional choice so each safety test controls the dispatch boundary.
func runtimeFailureFacts(t *testing.T, c *Context, packet *contract.Packet) circuitbreaker.ProgressEvidence {
	t.Helper()
	enabled := c.Config.JEV.Enabled
	c.Config.JEV.Enabled = false
	c.Drivers = sessionDriverSet(failingFirstDriver(t))
	out, err := c.executeStep(packet)
	require.NoError(t, err)
	require.True(t, out.Success)
	require.False(t, c.l1Step(packet).Pass)
	require.Equal(t, "failed", c.failAttempt(packet, errL1Failed, []string{packet.Acceptance.L1[0].Cmd}, nil))
	c.Config.JEV.Enabled = enabled
	require.NoError(t, c.err())
	facts := c.failureProgress(packet, errL1Failed)
	require.True(t, facts.Complete)
	return facts
}

func runtimeSafetyFixture(t *testing.T) (fixture, *Context, *contract.Packet, circuitbreaker.ProgressEvidence) {
	t.Helper()
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	server, _ := runtimeServer(t, func(map[string]string) string { return "repair" }, nil)
	enableRuntimeJEV(t, c, server.URL)
	packet := c.Packets["M01-T01"]
	return fx, c, packet, runtimeFailureFacts(t, c, packet)
}

func pendingRuntimeFixture(t *testing.T, c *Context, packet *contract.Packet, progress circuitbreaker.ProgressEvidence, spin bool) *state.RuntimeDecisionRecord {
	t.Helper()
	ledger := c.initializeRuntimeDecisions(&c.Config.JEV)
	snapshot, err := c.failureSnapshot(packet, errL1Failed, progress, spin, "pending-runtime-fixture", c.revision+1)
	require.NoError(t, err)
	record := &state.RuntimeDecisionRecord{ID: snapshot.ID, Packet: packet.PacketID, Attempt: c.State.Packets[packet.PacketID].Attempts, PlanRevisions: c.State.Packets[packet.PacketID].PlanRevisions, StartedAt: c.nowISO(), Snapshot: &snapshot, Action: snapshot.BaselineID, Status: "pending"}
	ledger.Records = append(ledger.Records, record)
	ledger.Used++
	c.State.Packets[packet.PacketID].PendingStrategy = record.ID
	c.save()
	require.NoError(t, c.err())
	return record
}

func TestRuntimeConfigurationRemainsFrozenAndLiveMasterWins(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	server, _ := runtimeServer(t, func(map[string]string) string { return "repair" }, nil)
	enableRuntimeJEV(t, c, server.URL)
	frozen := c.Config.JEV
	for _, test := range []struct {
		name, reason string
		modify       func(*config.Config)
	}{
		{"off", "disabled", func(cfg *config.Config) { cfg.JEV.Enabled = false }},
		{"hard", "hard-budget-without-enforceable-bound", func(cfg *config.Config) { cfg.Budget.Enforce = "hard" }},
		{"binding", "execution-policy-changed", func(cfg *config.Config) {
			b := cfg.ExecutionPolicy.Bindings["high"]
			b.Model = "another-approved-model"
			cfg.ExecutionPolicy.Bindings["high"] = b
		}},
		{"target", "policy-changed", func(cfg *config.Config) { cfg.JEV.Endpoint = "http://127.0.0.1:22/v1/systemone" }},
		{"threshold", "policy-changed", func(cfg *config.Config) { cfg.JEV.MinChoice = .9 }},
		{"calls", "policy-changed", func(cfg *config.Config) { cfg.JEV.MaxCalls = 1 }},
		{"data", "data-policy-changed", func(cfg *config.Config) { cfg.JEV.AllowedData = []decision.Category{decision.TaskSummary} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := *c
			copy.Config = jsonClone(c.Config)
			test.modify(copy.Config)
			persistRuntimeConfig(t, &copy)
			active, reason := c.runtimeJEVConfig(&frozen)
			require.Nil(t, active)
			require.Equal(t, test.reason, reason)
		})
	}
	persistRuntimeConfig(t, c)
	active, reason := c.runtimeJEVConfig(&frozen)
	require.Same(t, &frozen, active)
	require.Equal(t, "enabled", reason)
	c.Config.Budget.Enforce = "hard"
	active, reason = c.runtimeJEVConfig(nil)
	require.Nil(t, active)
	require.Equal(t, "hard-budget-without-enforceable-bound", reason)
	c.Config.Budget.Enforce = "soft"
	writeFile(t, filepath.Join(c.TaskDir, "flywheel.config.json"), "{")
	active, reason = c.runtimeJEVConfig(nil)
	require.Nil(t, active)
	require.Equal(t, "config-unavailable", reason)
	c.EnvLookup = nil
	t.Setenv(frozen.KeyEnv, "environment-fixture")
	require.Equal(t, "environment-fixture", c.decisionOptions(&frozen).APIKey)
	require.NotNil(t, c.decisionContext())
}

func TestRuntimeActionsAreUsefulBoundedAndPreserveFailureBaseline(t *testing.T) {
	_, c, p, _ := runtimeSafetyFixture(t)
	p.FeedbackChecks = []contract.GateEntry{{Cmd: "secret-command", Expect: "exit 0"}, {Purpose: "Distinguish stale cache from a missing generated artifact", Cmd: "secret-command", Expect: "exit 0"}}
	actions, baseline := c.failureActions(p, errL1Failed, false)
	require.Equal(t, "repair", baseline)
	byID := map[string]decision.Action{}
	for _, a := range actions {
		byID[a.ID] = a
		require.NotContains(t, a.Description, "secret-command")
	}
	require.NotContains(t, byID, "feedback-0")
	require.Contains(t, byID["feedback-1"].Description, "Distinguish stale cache")
	_, baseline = c.failureActions(p, errReviewReturned, true)
	require.Equal(t, "replan", baseline, "Review spin must not introduce a baseline quality upgrade")
	_, baseline = c.failureActions(p, errL1Failed, true)
	require.Equal(t, "model-1", baseline)
	route := c.Config.ExecutionPolicy.Routes[p.Route]
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("alternate-%d", i)
		route.Allowed = append(route.Allowed, id)
		c.Config.ExecutionPolicy.Bindings[id] = c.Config.ExecutionPolicy.Bindings["low"]
	}
	c.Config.ExecutionPolicy.Routes[p.Route] = route
	c.State.Packets[p.PacketID].ExecutionPolicy = nil
	actions, _ = c.failureActions(p, errL1Failed, false)
	require.Len(t, actions, 16)
	require.Equal(t, "你好", boundedDecisionText("你好世界", 7))
	c.State.Packets[p.PacketID].ExecutionPolicy.Identity = "invalid"
	actions, baseline = c.failureActions(p, errL1Failed, false)
	require.Empty(t, actions)
	require.Empty(t, baseline)
	require.ErrorContains(t, c.err(), "identity changed")
}

func TestRuntimeAdmissionNeverBypassesUncertaintyOrLimits(t *testing.T) {
	_, c, p, facts := runtimeSafetyFixture(t)
	for _, test := range []struct {
		name   string
		mutate func(*Context, *contract.Packet, *circuitbreaker.ProgressEvidence)
		code   errs.ErrorCode
	}{
		{"cancelled", func(c *Context, _ *contract.Packet, _ *circuitbreaker.ProgressEvidence) {
			c.fatal = errors.New("stopped")
		}, errL1Failed},
		{"incomplete", func(_ *Context, _ *contract.Packet, p *circuitbreaker.ProgressEvidence) { p.Complete = false }, errL1Failed},
		{"channel-auth", func(*Context, *contract.Packet, *circuitbreaker.ProgressEvidence) {}, errs.AUTH},
		{"attempt-cap", func(c *Context, p *contract.Packet, _ *circuitbreaker.ProgressEvidence) {
			c.State.Packets[p.PacketID].FailedAttempts = p.Escalation.MaxRetries
		}, errL1Failed},
		{"unknown-executor", func(c *Context, p *contract.Packet, _ *circuitbreaker.ProgressEvidence) {
			c.sessionRecord("execute", p.PacketID).Last.Status = "blocked"
		}, errL1Failed},
		{"no-key", func(c *Context, _ *contract.Packet, _ *circuitbreaker.ProgressEvidence) {
			c.EnvLookup = func(string) (string, bool) { return "", false }
		}, errL1Failed},
		{"call-cap", func(c *Context, _ *contract.Packet, _ *circuitbreaker.ProgressEvidence) {
			l := c.initializeRuntimeDecisions(&c.Config.JEV)
			l.Used = c.Config.JEV.MaxCalls
		}, errL1Failed},
		{"record-cap", func(c *Context, _ *contract.Packet, _ *circuitbreaker.ProgressEvidence) {
			l := c.initializeRuntimeDecisions(&c.Config.JEV)
			l.Records = make([]*state.RuntimeDecisionRecord, 100)
		}, errL1Failed},
		{"unknown-source", func(c *Context, _ *contract.Packet, _ *circuitbreaker.ProgressEvidence) {
			c.State.PreparationSource = &state.PreparationSource{RunID: "missing"}
		}, errL1Failed},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := *c
			copy.State = jsonClone(c.State)
			packet := *p
			f := facts
			test.mutate(&copy, &packet, &f)
			require.Empty(t, copy.runtimeFailureDecision(&packet, test.code, f, false))
			if copy.State.RuntimeDecisions != nil {
				require.LessOrEqual(t, len(copy.State.RuntimeDecisions.Records), 100)
			}
		})
	}
	// A changed or unreadable candidate cannot be used to construct a new request.
	require.NoError(t, os.Remove(filepath.Join(c.TaskDir, "task-packet-"+p.PacketID+".md")))
	require.Empty(t, c.runtimeFailureDecision(p, errL1Failed, facts, false))
	require.Empty(t, c.State.RuntimeDecisions.Records)
}

func TestRuntimeUsageIsCountedOnceAndPrunedDaysStayPruned(t *testing.T) {
	_, c := sessionFixture(t)
	ledger := c.initializeRuntimeDecisions(&config.JEVConfig{MaxCalls: 4})
	ledger.Used = 1
	skipped := &state.RuntimeDecisionRecord{Receipt: &decision.Receipt{Status: "skipped"}}
	c.accountRuntimeDecision(skipped)
	c.accountRuntimeDecision(skipped)
	require.Zero(t, ledger.Used)
	require.Zero(t, c.State.Budget.UnknownCostCalls)
	for i := 0; i < 31; i++ {
		c.State.Budget.Daily[time.Date(2026, 11, 1+i, 0, 0, 0, 0, time.UTC).Format("2006-01-02")] = state.DailyBucket{}
	}
	old := &state.RuntimeDecisionRecord{StartedAt: "2025-01-01T01:00:00Z", Receipt: &decision.Receipt{Status: "unknown"}}
	c.accountRuntimeDecision(old)
	c.accountRuntimeDecision(old)
	require.Len(t, c.State.Budget.Daily, 31)
	require.NotContains(t, c.State.Budget.Daily, "2025-01-01")
	require.EqualValues(t, 1, c.State.Budget.UnknownUsageCalls)
	require.EqualValues(t, 1, c.State.Budget.UnknownCostCalls)
}

func runtimeWorkers(c *Context, ids ...string) (*parallelGroup, []*Context) {
	ctx, cancel := context.WithCancel(context.Background())
	g := &parallelGroup{owner: c, cancel: cancel, slots: make(chan struct{}, 2), providers: map[string]chan struct{}{}, threads: map[string]string{}}
	workers := make([]*Context, len(ids))
	for i, id := range ids {
		w := *c
		w.State = jsonClone(c.State)
		w.RunContext = ctx
		w.parallel = &parallelLane{group: g, id: id, inbox: len(w.State.Inbox)}
		workers[i] = &w
	}
	return g, workers
}

func TestRuntimeParallelCapAndColdStartSurviveWorkerSaves(t *testing.T) {
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	server, count := runtimeServer(t, func(map[string]string) string { return "model-1" }, nil)
	enableRuntimeJEV(t, c, server.URL)
	c.Config.JEV.MaxCalls = 1
	persistRuntimeConfig(t, c)
	packets := []*contract.Packet{c.Packets["M01-T01"], c.Packets["M01-T02"]}
	facts := []circuitbreaker.ProgressEvidence{}
	for _, p := range packets {
		facts = append(facts, runtimeFailureFacts(t, c, p))
	}
	g, workers := runtimeWorkers(c, "M01-T01", "M01-T02")
	defer g.cancel()
	var wg sync.WaitGroup
	for i, w := range workers {
		wg.Add(1)
		go func(i int, w *Context) {
			defer wg.Done()
			w.runtimeFailureDecision(packets[i], errL1Failed, facts[i], false)
			w.save()
		}(i, w)
	}
	wg.Wait()
	require.Equal(t, int32(1), count.Load())
	require.Equal(t, 1, c.State.RuntimeDecisions.Used)
	for _, w := range workers {
		require.NoError(t, w.err())
	}
	chosen := c.State.RuntimeDecisions.Records[0].Packet
	require.Equal(t, "high", c.State.Packets[chosen].ExecutionPolicy.Current)
	require.False(t, c.sessionRecord("execute", chosen).ResumeCandidate)
	require.Contains(t, c.sessionRecord("execute", chosen).Reason, "cold start")
	loaded := loadFixtureState(t, fx)
	require.False(t, loaded.Sessions.Records[sessionKey("execute", chosen)].ResumeCandidate)
	require.Equal(t, "jev-model-selection", loaded.Packets[chosen].ExecutionPolicy.History[0].Kind)
}

func TestRuntimeParallelCancellationAndMergeFailuresDoNotDispatch(t *testing.T) {
	for _, scenario := range []string{"cancel", "merge", "resume-merge"} {
		t.Run(scenario, func(t *testing.T) {
			_, c := sessionFixture(t)
			enableExecutionPolicy(c)
			var cancel context.CancelFunc
			server, count := runtimeServer(t, func(map[string]string) string { return "model-1" }, func() { cancel() })
			enableRuntimeJEV(t, c, server.URL)
			p := c.Packets["M01-T01"]
			facts := runtimeFailureFacts(t, c, p)
			g, workers := runtimeWorkers(c, p.PacketID)
			cancel = g.cancel
			defer cancel()
			w := workers[0]
			if scenario != "cancel" {
				w.Config.Worktree.Enabled = new(true)
			}
			if scenario == "resume-merge" {
				w.State.Packets[p.PacketID].PendingStrategy = "missing"
				require.Empty(t, w.resumeRuntimeDecision(p))
			} else {
				require.Empty(t, w.runtimeFailureDecision(p, errL1Failed, facts, false))
			}
			require.Error(t, w.err())
			require.Empty(t, c.State.Packets[p.PacketID].ExecutionPolicy.History)
			if scenario == "cancel" {
				require.Equal(t, int32(1), count.Load())
				require.ErrorIs(t, w.err(), context.Canceled)
			} else {
				require.Zero(t, count.Load())
				require.Contains(t, w.err().Error(), "worktree")
			}
		})
	}
}

func TestRuntimeImportedInvestigationCallsShareTheTaskCap(t *testing.T) {
	for _, skipped := range []bool{false, true} {
		t.Run(fmt.Sprint(skipped), func(t *testing.T) {
			fx, p := managedPreparationFixture(t, true)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
			defer server.Close()
			j := config.Default().JEV
			j.Enabled = true
			j.Endpoint = server.URL + "/v1/systemone"
			j.MaxCalls = 2
			if skipped {
				j.AllowedData = []decision.Category{}
			}
			rewriteConfig(t, fx, func(v map[string]any) { v["jev"] = j })
			opts := investigation.Options{LookupEnv: func(string) (string, bool) { return "fixture", true }}
			startManagedPreparation(t, fx, p, opts)
			prepared := finishManagedPreparation(t, fx, opts)
			writeFile(t, state.StateFile(fx.taskDir), fixtureState(fx.slug))
			c := unitTestContext(t, fx)
			ledger := c.initializeRuntimeDecisions(&j)
			require.NotNil(t, ledger)
			want := 2
			if skipped {
				want = 0
			}
			require.Equal(t, want, ledger.ImportedCalls)
			require.Equal(t, want, ledger.Used)
			c.State.PreparationSource.RunID = "changed"
			require.Nil(t, c.initializeRuntimeDecisions(&j))
			require.NotEmpty(t, prepared.Decisions)
		})
	}
}

func TestRuntimeUnknownHTTPAndJournalErrorsFallBackWithoutFreeCalls(t *testing.T) {
	for _, journalError := range []bool{false, true} {
		t.Run(fmt.Sprint(journalError), func(t *testing.T) {
			_, c, p, facts := runtimeSafetyFixture(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
			defer server.Close()
			c.Config.JEV.Endpoint = server.URL + "/v1/systemone"
			persistRuntimeConfig(t, c)
			if journalError {
				writeFile(t, filepath.Join(c.TaskDir, ".investigation"), "not a directory")
			}
			require.Equal(t, "failed", c.runtimeFailureDecision(p, errL1Failed, facts, false))
			require.NoError(t, c.err())
			require.Equal(t, 1, c.State.RuntimeDecisions.Used)
			record := c.State.RuntimeDecisions.Records[0]
			require.False(t, record.Selection.Adopted)
			require.Equal(t, "repair", record.Action)
			require.EqualValues(t, 1, c.State.Budget.UnknownUsageCalls)
		})
	}
}

func TestRuntimePendingRecoveryRejectsStaleInputsAndNeverResends(t *testing.T) {
	for _, scenario := range []string{"valid", "changed-candidate", "changed-feedback", "changed-attempt", "unknown-executor", "disabled", "spin-baseline", "parallel", "corrupt-journal"} {
		t.Run(scenario, func(t *testing.T) {
			_, c, p, facts := runtimeSafetyFixture(t)
			spin := scenario == "spin-baseline"
			record := pendingRuntimeFixture(t, c, p, facts, spin)
			switch scenario {
			case "changed-candidate":
				writeFile(t, filepath.Join(c.Cwd, "build", "extra.txt"), "new candidate")
			case "changed-feedback":
				c.State.Packets[p.PacketID].LastFeedback = map[string]any{"error-code": string(errL1Failed), "progress": circuitbreaker.ProgressAssessment{Status: "stalled", Reason: "unchanged failure"}}
			case "changed-attempt":
				c.State.Packets[p.PacketID].Attempts++
			case "unknown-executor":
				c.sessionRecord("execute", p.PacketID).Last.Status = "blocked"
			case "disabled":
				copy := *c
				copy.Config = jsonClone(c.Config)
				copy.Config.JEV.Enabled = false
				persistRuntimeConfig(t, &copy)
			case "corrupt-journal":
				writeFile(t, filepath.Join(c.TaskDir, ".investigation"), "not a directory")
			}
			if scenario == "parallel" {
				g, workers := runtimeWorkers(c, p.PacketID)
				defer g.cancel()
				w := workers[0]
				require.Equal(t, "failed", w.resumeRuntimeDecision(p))
				require.NoError(t, w.err())
				w.save()
				require.NoError(t, w.err())
				return
			}
			result := c.resumeRuntimeDecision(p)
			if scenario == "corrupt-journal" {
				require.Empty(t, result)
				require.Error(t, c.err())
				return
			}
			require.NoError(t, c.err())
			require.Equal(t, "failed", result)
			require.True(t, record.Accounted)
			require.False(t, record.Selection.Adopted)
			require.Equal(t, 1, c.State.RuntimeDecisions.Used)
			require.EqualValues(t, 1, c.State.Budget.UnknownUsageCalls)
			if strings.HasPrefix(scenario, "changed-") || scenario == "disabled" || scenario == "unknown-executor" {
				require.Equal(t, "stale", record.Status)
			}
			if scenario == "spin-baseline" || scenario == "changed-feedback" {
				require.Equal(t, "high", c.State.Packets[p.PacketID].ExecutionPolicy.Current)
			}
		})
	}
}

func TestRuntimeLegacyV2ModelDescriptionsRecoverTheirOriginalJournal(t *testing.T) {
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	server, calls := runtimeServer(t, func(map[string]string) string { return "model-1" }, nil)
	enableRuntimeJEV(t, c, server.URL)
	p := c.Packets["M01-T01"]
	facts := runtimeFailureFacts(t, c, p)
	r := pendingRuntimeFixture(t, c, p, facts, false)
	for index, id := range c.Config.ExecutionPolicy.Routes[p.Route].Allowed {
		b := c.Config.ExecutionPolicy.Bindings[id]
		for i := range r.Snapshot.Actions {
			if r.Snapshot.Actions[i].ID == fmt.Sprintf("model-%d", index) {
				// Exact v2 model wording before initial-selection support.
				r.Snapshot.Actions[i].Description = fmt.Sprintf("Continue repair using approved binding %s: harness=%s provider=%s channel=%s model=%s effort=%s permission=%s session=%s. This requires a cold start and does not add attempts.", id, b.Harness, b.Provider, b.Channel, b.Model, b.Thinking, b.Permission, b.SessionMode)
			}
		}
	}
	c.save()
	require.NoError(t, c.err())
	oldSnapshot := *r.Snapshot
	receipt, err := decision.Evaluate(c.decisionContext(), filepath.Join(c.TaskDir, ".investigation", "runtime-decisions"), oldSnapshot, c.Config.JEV.DecisionPolicy(), c.decisionOptions(&c.Config.JEV))
	require.NoError(t, err)
	require.Equal(t, "ok", receipt.Status)
	loaded := loadFixtureState(t, fx)
	c.State, c.revision = loaded, loaded.Revision
	before := c.State.Budget.Totals.Tokens
	require.Equal(t, "failed", c.resumeRuntimeDecision(p))
	require.NoError(t, c.err())
	recovered := c.State.RuntimeDecisions.Records[0]
	require.Equal(t, "ok", recovered.Receipt.Status)
	require.Equal(t, receipt.QuestionHash, recovered.Receipt.QuestionHash)
	require.Equal(t, receipt.SnapshotHash, recovered.Receipt.SnapshotHash)
	require.Equal(t, "model-1", recovered.Receipt.ActionID)
	require.False(t, recovered.Selection.Adopted, "new candidate wording cannot authorize an old proposal")
	require.Equal(t, "repair", recovered.Action)
	require.Equal(t, "stale", recovered.Status)
	require.Equal(t, before+20, c.State.Budget.Totals.Tokens)
	require.Empty(t, c.resumeRuntimeDecision(p))
	require.Equal(t, before+20, c.State.Budget.Totals.Tokens)
	require.Equal(t, int32(1), calls.Load(), "recovery must read the old journal without resending")
	require.Empty(t, c.State.Packets[p.PacketID].ExecutionPolicy.History)
}
