package machine

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ckanner/flywheel/internal/circuitbreaker"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/require"
)

func runtimeFeedbackFixture(t *testing.T, command string) (fixture, *Context, *contract.Packet, circuitbreaker.ProgressEvidence, *atomic.Int32) {
	t.Helper()
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	p := c.Packets["M01-T01"]
	p.FeedbackChecks = []contract.GateEntry{{Purpose: "Check the approved reproducer before another repair", Cmd: command, Expect: "exit 0"}}
	server, requests := runtimeServer(t, func(map[string]string) string { return "feedback-0" }, nil)
	enableRuntimeJEV(t, c, server.URL)
	facts := runtimeFailureFacts(t, c, p)
	return fx, c, p, facts, requests
}

// Stop after a real optional decision has been received and accounted, before
// apply reserves its physical feedback action. The pending on-disk state still
// contains no model proposal until apply saves the dispatch boundary.
func selectedRuntimeFeedback(t *testing.T, c *Context, p *contract.Packet, facts circuitbreaker.ProgressEvidence) *state.RuntimeDecisionRecord {
	t.Helper()
	r := pendingRuntimeFixture(t, c, p, facts, false)
	j := c.State.RuntimeDecisions.Config
	options := c.decisionOptions(j)
	receipt, err := decision.Evaluate(context.Background(), filepath.Join(c.TaskDir, ".investigation", "runtime-decisions"), *r.Snapshot, j.DecisionPolicy(), options)
	require.NoError(t, err)
	selection := decision.Select(*r.Snapshot, receipt, j.DecisionPolicy(), options)
	require.True(t, selection.Adopted)
	require.Equal(t, "feedback-0", selection.ActionID)
	r.Receipt, r.Selection, r.Action = &receipt, &selection, selection.ActionID
	c.accountRuntimeDecision(r)
	return r
}

func advanceRuntimeStateRevision(t *testing.T, c *Context) {
	t.Helper()
	stored, err := state.Load(c.TaskDir)
	require.NoError(t, err)
	_, err = state.Save(c.TaskDir, stored, nil)
	require.NoError(t, err)
}

func TestRuntimeFeedbackCandidateMutationPersistsAndNeverReplays(t *testing.T) {
	fx, c, p, facts, requests := runtimeFeedbackFixture(t, "printf 'attempt\\n' >> feedback-proof.txt")
	require.Equal(t, "failed", c.runtimeFailureDecision(p, errL1Failed, facts, false))
	require.ErrorContains(t, c.err(), "changed candidate")
	before, err := os.ReadFile(filepath.Join(c.unitCwd(p), "feedback-proof.txt"))
	require.NoError(t, err)
	require.Equal(t, "attempt\n", string(before))
	stored := loadFixtureState(t, fx)
	r := stored.RuntimeDecisions.Records[0]
	require.Equal(t, "feedback-stale", r.Status)
	require.NotNil(t, r.Feedback)
	require.True(t, r.Accounted)
	require.Equal(t, r.ID, stored.Packets[p.PacketID].PendingStrategy)
	attempts := stored.Packets[p.PacketID].Attempts
	c.State, c.revision, c.fatal = stored, stored.Revision, nil
	outcome, err := c.runExecuteLoop(p)
	require.ErrorContains(t, err, "refusing replay")
	require.True(t, outcome.Stopped)
	require.Equal(t, attempts, c.State.Packets[p.PacketID].Attempts)
	after, err := os.ReadFile(filepath.Join(c.unitCwd(p), "feedback-proof.txt"))
	require.NoError(t, err)
	require.Equal(t, before, after, "a stale feedback result must not dispatch the approved check again")
	require.Equal(t, int32(1), requests.Load())
}

func TestRuntimeFeedbackUnknownCleanupPersistsAndBlocksResume(t *testing.T) {
	fx, c, p, facts, requests := runtimeFeedbackFixture(t, "printf evidence")
	previous := runRuntimeFeedback
	t.Cleanup(func() { runRuntimeFeedback = previous })
	physicalCalls := 0
	runRuntimeFeedback = func(checks []contract.GateEntry, opts verify.GateOpts) verify.GateReport {
		physicalCalls++
		require.Equal(t, p.FeedbackChecks, checks)
		require.Equal(t, c.unitCwd(p), opts.Cwd)
		stored := loadFixtureState(t, fx)
		require.Equal(t, "feedback-running", stored.RuntimeDecisions.Records[0].Status, "intent must be durable before process dispatch")
		return verify.GateReport{Results: []verify.GateResult{{Cmd: checks[0].Cmd, Expect: checks[0].Expect, ExitCode: -1, CleanupError: "fixture cannot establish process group quiescence"}}}
	}
	require.Equal(t, "failed", c.runtimeFailureDecision(p, errL1Failed, facts, false))
	require.ErrorContains(t, c.err(), "cleanup is unknown")
	stored := loadFixtureState(t, fx)
	require.Equal(t, "feedback-unknown", stored.RuntimeDecisions.Records[0].Status)
	require.Equal(t, stored.RuntimeDecisions.Records[0].ID, stored.Packets[p.PacketID].PendingStrategy)
	c.State, c.revision, c.fatal = stored, stored.Revision, nil
	require.Empty(t, c.resumeRuntimeDecision(p))
	require.ErrorContains(t, c.err(), "refusing replay")
	require.Equal(t, 1, physicalCalls)
	require.Equal(t, int32(1), requests.Load())
}

func TestRuntimeFeedbackSnapshotAndSaveBoundariesDoNotLoseDispatchState(t *testing.T) {
	for _, scenario := range []string{"snapshot-before", "snapshot-before-save-conflict", "save-before", "snapshot-after", "save-after", "save-after-changed-candidate"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, p, facts, requests := runtimeFeedbackFixture(t, "printf evidence")
			r := selectedRuntimeFeedback(t, c, p, facts)
			previous := runRuntimeFeedback
			t.Cleanup(func() { runRuntimeFeedback = previous })
			physicalCalls := 0
			runRuntimeFeedback = func(checks []contract.GateEntry, opts verify.GateOpts) verify.GateReport {
				physicalCalls++
				gate := previous(checks, opts)
				require.True(t, gate.Pass)
				switch scenario {
				case "snapshot-after":
					require.NoError(t, os.Remove(filepath.Join(c.TaskDir, "spec.md")))
				case "save-after-changed-candidate":
					writeFile(t, filepath.Join(c.unitCwd(p), "new-feedback-candidate.txt"), "changed by approved check")
					advanceRuntimeStateRevision(t, c)
				case "save-after":
					advanceRuntimeStateRevision(t, c)
				}
				return gate
			}
			if scenario == "snapshot-before" || scenario == "snapshot-before-save-conflict" {
				require.NoError(t, os.Remove(filepath.Join(c.TaskDir, "spec.md")))
			}
			if scenario == "save-before" || scenario == "snapshot-before-save-conflict" {
				advanceRuntimeStateRevision(t, c)
			}
			result := c.applyRuntimeDecision(p, r)
			require.Error(t, c.err())
			stored := loadFixtureState(t, fx)
			persisted := stored.RuntimeDecisions.Records[0]
			switch scenario {
			case "snapshot-before":
				require.Empty(t, result)
				require.Zero(t, physicalCalls)
				require.Equal(t, "stale", persisted.Status)
				require.Empty(t, stored.Packets[p.PacketID].PendingStrategy)
			case "save-before", "snapshot-before-save-conflict":
				require.Empty(t, result)
				require.Zero(t, physicalCalls)
				require.Equal(t, "pending", persisted.Status)
				require.Nil(t, persisted.Receipt, "failed CAS cannot partially publish its model proposal")
				var conflict *state.CasConflictError
				require.ErrorAs(t, c.err(), &conflict)
			case "snapshot-after":
				require.Equal(t, "failed", result)
				require.Equal(t, 1, physicalCalls)
				require.Equal(t, "feedback-stale", persisted.Status)
				require.Equal(t, persisted.ID, stored.Packets[p.PacketID].PendingStrategy)
			default:
				require.Equal(t, "failed", result)
				require.Equal(t, 1, physicalCalls)
				require.Equal(t, "feedback-running", persisted.Status, "a lost terminal preserves the non-replayable intent")
				var conflict *state.CasConflictError
				require.ErrorAs(t, c.err(), &conflict, "persistence error remains primary even if the check changed the candidate")
				c.State, c.revision, c.fatal = stored, stored.Revision, nil
				require.Empty(t, c.resumeRuntimeDecision(p))
				require.ErrorContains(t, c.err(), "refusing replay")
				require.Equal(t, 1, physicalCalls)
			}
			require.Equal(t, int32(1), requests.Load())
		})
	}
}

func TestRuntimeFeedbackAcceptsLegacyUnstructuredFailureContext(t *testing.T) {
	fx, c, p, facts, _ := runtimeFeedbackFixture(t, "printf evidence")
	c.State.Packets[p.PacketID].LastFeedback = "legacy failure description"
	r := selectedRuntimeFeedback(t, c, p, facts)
	require.Equal(t, "failed", c.applyRuntimeDecision(p, r))
	require.NoError(t, c.err())
	stored := loadFixtureState(t, fx)
	require.Equal(t, "feedback-completed", stored.RuntimeDecisions.Records[0].Status)
	feedback, ok := stored.Packets[p.PacketID].LastFeedback.(map[string]any)
	require.True(t, ok)
	require.Contains(t, feedback, "jev-feedback")
	require.Empty(t, stored.Packets[p.PacketID].PendingStrategy)
}

func TestRuntimeActionApplicationDefensivelyRejectsUnavailableBindings(t *testing.T) {
	for _, scenario := range []string{"unimplemented", "model-disabled", "model-invalid-index", "model-negative-index", "model-outside-route", "model-same-binding", "model-policy-changed", "feedback-invalid-index", "feedback-negative-index", "feedback-no-checks", "recheck-no-checks"} {
		t.Run(scenario, func(t *testing.T) {
			_, c := sessionFixture(t)
			enableExecutionPolicy(c)
			p := c.Packets["M01-T01"]
			r := &state.RuntimeDecisionRecord{ID: "defensive-application-fixture", Action: "model-1", Selection: &decision.Selection{Reason: "defensive fixture"}}
			// These malformed invocations are rejected by Load before normal
			// dispatch. Exercise apply's final defenses without installing a
			// fabricated record in the durable task ledger.
			switch scenario {
			case "unimplemented":
				r.Action = "unapproved-effect"
			case "model-disabled":
				c.Config.ExecutionPolicy = nil
			case "model-invalid-index":
				r.Action = "model-invalid"
			case "model-negative-index":
				r.Action = "model--1"
			case "model-outside-route":
				r.Action = "model-99"
			case "model-same-binding":
				r.Action = "model-0"
			case "model-policy-changed":
				policy, _, err := c.executionPolicy(p)
				require.NoError(t, err)
				policy.Identity = "changed"
			case "feedback-invalid-index":
				r.Action = "feedback-invalid"
			case "feedback-negative-index":
				r.Action = "feedback--1"
			case "feedback-no-checks":
				r.Action = "feedback-0"
			case "recheck-no-checks":
				r.Action = "recheck-0"
				p.Acceptance.L1 = nil
			}
			require.Empty(t, c.applyRuntimeDecision(p, r))
			require.Equal(t, "stale", r.Status)
			require.Empty(t, c.State.Packets[p.PacketID].PendingStrategy)
			if policy := c.State.Packets[p.PacketID].ExecutionPolicy; policy != nil {
				require.Zero(t, policy.Switches)
				require.Empty(t, policy.History)
			}
		})
	}
}
