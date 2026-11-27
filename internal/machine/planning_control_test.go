package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func controlPlanningFixture(t *testing.T) (RunOpts, *state.State, planning.Brief) {
	t.Helper()
	fx := newGitFixture(t)
	opts := RunOpts{TaskDir: filepath.Join(fx.root, "docs", "control-planning"), Cwd: fx.root, Config: mustConfig(t, fx),
		Now: func() time.Time { return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC) }}
	brief := planning.Brief{ID: "control-planning", Goal: "Design before implementing", OriginalRequest: "Discuss and independently review the exact design first"}
	st, receipt, err := ControlPlanning(opts, PlanningCommand{Action: "prepare", Brief: &brief, Start: new(false)}, "")
	require.NoError(t, err)
	require.Equal(t, "applied", receipt.Status)
	return opts, st, brief
}

func reviewedControlDesign(t *testing.T, opts RunOpts, st *state.State) *state.State {
	t.Helper()
	output := engineDesignOutput(t)
	design, err := planning.SealDesign(planning.Design{Ref: planning.Ref{Revision: len(st.Planning.Designs) + 1}, InputRevision: st.Planning.InputRevision,
		Document: output.Document, Requirements: output.Requirements, Decisions: output.Decisions, Milestones: output.Milestones,
		ImplementationScope: output.ImplementationScope, Baseline: "baseline", PlannerInvocation: "planner-" + string(rune('a'+len(st.Planning.Designs)))})
	require.NoError(t, err)
	require.NoError(t, planning.RecordDesign(st.Planning, design))
	require.NoError(t, planning.RecordReview(st.Planning, planning.Review{ID: "review-" + string(rune('a'+len(st.Planning.Reviews))), Kind: "design", Target: design.Ref,
		InputRevision: st.Planning.InputRevision, Invocation: "independent-review-" + string(rune('a'+len(st.Planning.Reviews))),
		Role: "design-review", Binding: "independent", Verdict: "pass", Summary: "The exact design has executable acceptance.", Findings: []planning.Finding{}}))
	st, err = state.Save(opts.TaskDir, st, nil)
	require.NoError(t, err)
	return st
}

func TestPlanningControlValidatesTypedScopesAndDoesNotAcceptAuthorityInData(t *testing.T) {
	base := PlanningCommand{Action: "plan", Revision: 1, RequestID: "user-request", Scope: "design"}
	require.NoError(t, ValidatePlanningCommand(base))
	require.True(t, base.StartRequested(true))
	base.Start = new(false)
	require.False(t, base.StartRequested(true))
	for _, change := range []func(*PlanningCommand){
		func(r *PlanningCommand) { r.RequestID = "../escape" },
		func(r *PlanningCommand) { r.RequestID = "" },
		func(r *PlanningCommand) { r.Revision = 0 },
		func(r *PlanningCommand) { r.Action = "unknown" },
		func(r *PlanningCommand) { r.Scope = "unknown" },
		func(r *PlanningCommand) { r.Scope = "packet"; r.ScopeID = "" },
		func(r *PlanningCommand) { r.ScopeID = "../escape" },
		func(r *PlanningCommand) { r.ScopeID = "foreign-root" },
		func(r *PlanningCommand) { r.Scope = "replan"; r.ScopeID = "foreign" },
		func(r *PlanningCommand) { r.Policy = "delegated" },
		func(r *PlanningCommand) { r.Action = "prepare"; r.Scope = ""; r.Brief = nil },
		func(r *PlanningCommand) { r.Action = "status"; r.Scope = "" },
		func(r *PlanningCommand) { r.Action = "feedback"; r.Scope = ""; r.Text = "" },
		func(r *PlanningCommand) { r.Action = "feedback"; r.Scope = ""; r.Text = strings.Repeat("x", 8193) },
		func(r *PlanningCommand) {
			r.Action = "feedback"
			r.Scope = ""
			r.Text = "Discuss"
			r.Policy = "auto-approve"
		},
		func(r *PlanningCommand) { r.Action = "approve"; r.Scope = "" },
		func(r *PlanningCommand) {
			r.Action = "approve"
			r.Scope = ""
			r.Design = &planning.Ref{Revision: 1, Digest: "fake"}
		},
	} {
		r := base
		change(&r)
		require.Error(t, ValidatePlanningCommand(r), "%+v", r)
	}
	require.NoError(t, ValidatePlanningCommand(PlanningCommand{Action: "status"}))
	require.NoError(t, ValidatePlanningCommand(PlanningCommand{Action: "prepare", Brief: &planning.Brief{}}))
	require.Error(t, ValidatePlanningCommand(PlanningCommand{Action: "prepare"}))
	require.NoError(t, ValidatePlanningCommand(PlanningCommand{Action: "migrate", Revision: 1, RequestID: "migrate"}))
	require.Error(t, ValidatePlanningCommand(PlanningCommand{Action: "unknown", Revision: 1, RequestID: "unknown"}))
	for _, tc := range []struct{ id, scope, normalized string }{{"", "design", ""}, {"root", "design", ""}, {"M-01", "milestone", "M-01"}, {"M01-T01", "packet", "M01-T01"}} {
		r := base
		r.Scope, r.ScopeID = "replan", tc.id
		got, err := normalizePlanningScope(r)
		require.NoError(t, err)
		require.Equal(t, tc.scope, got.Scope)
		require.Equal(t, tc.normalized, got.ScopeID)
	}
	_, _, err := ControlPlanning(RunOpts{}, PlanningCommand{Action: "approve", Revision: 1, RequestID: "forged", Design: &planning.Ref{Revision: 1, Digest: strings.Repeat("a", 64)}}, "actor:user")
	require.ErrorContains(t, err, "verified human")
	_, _, err = ControlPlanning(RunOpts{}, PlanningCommand{Action: "status"}, "tty:forged")
	require.ErrorContains(t, err, "only for design approval")
	_, _, err = ControlPlanning(RunOpts{}, PlanningCommand{Action: "unknown"}, "")
	require.Error(t, err)
}

func TestPlanningControlThinPreparationAndIdempotentVersionedMutations(t *testing.T) {
	opts, st, brief := controlPlanningFixture(t)
	require.Equal(t, 4, st.Sessions.Version)
	require.Equal(t, opts.Cwd, st.Project.Cwd)
	require.Equal(t, "review-first", st.Planning.Policy)
	require.Empty(t, st.Planning.Designs)
	require.Empty(t, st.Planning.Reviews)
	require.Empty(t, st.Planning.Approvals)
	require.NoFileExists(t, filepath.Join(opts.TaskDir, "spec.md"))
	replayed, receipt, err := ControlPlanning(opts, PlanningCommand{Action: "prepare", Brief: &brief}, "")
	require.NoError(t, err)
	require.Equal(t, st.Revision, replayed.Revision)
	require.Equal(t, "unchanged", receipt.Status)
	observed, receipt, err := ControlPlanning(opts, PlanningCommand{Action: "status"}, "")
	require.NoError(t, err)
	require.Nil(t, receipt)
	require.Equal(t, st.Revision, observed.Revision)
	brief.Goal = "A different goal"
	_, _, err = ControlPlanning(opts, PlanningCommand{Action: "prepare", Brief: &brief}, "")
	require.ErrorContains(t, err, "different brief")

	request := PlanningCommand{Action: "plan", Revision: st.Revision, RequestID: "design-once", Scope: "replan", ScopeID: "root", Start: new(false)}
	st, receipt, err = ControlPlanning(opts, request, "")
	require.NoError(t, err)
	require.Equal(t, "applied", receipt.Status)
	require.Len(t, st.Planning.Requests, 1)
	require.Equal(t, "design", st.Planning.Requests[0].Scope)
	require.Empty(t, st.Planning.Requests[0].ScopeID)
	require.True(t, st.Planning.Requests[0].PlanningOnly)
	require.False(t, st.Planning.StartAuthorized)
	budget := jsonClone(st.Budget)
	again, receipt, err := ControlPlanning(opts, request, "")
	require.NoError(t, err)
	require.Equal(t, "unchanged", receipt.Status)
	require.Equal(t, st.Revision, again.Revision)
	request.Scope, request.ScopeID = "packet", "M01-T01"
	_, _, err = ControlPlanning(opts, request, "")
	require.ErrorContains(t, err, "different content")
	request.RequestID = "stale-new-request"
	_, _, err = ControlPlanning(opts, request, "")
	require.ErrorContains(t, err, "revision changed")
	feedback := PlanningCommand{Action: "feedback", Revision: st.Revision, RequestID: "feedback-once", Text: "Include an offline recovery scenario."}
	st, _, err = ControlPlanning(opts, feedback, "")
	require.NoError(t, err)
	require.Equal(t, 2, st.Planning.InputRevision)
	require.Equal(t, budget, st.Budget)
	require.False(t, st.Planning.StartAuthorized)
	again, receipt, err = ControlPlanning(opts, feedback, "")
	require.NoError(t, err)
	require.Equal(t, "unchanged", receipt.Status)
	require.Equal(t, st.Revision, again.Revision)

	st = reviewedControlDesign(t, opts, st)
	approval := PlanningCommand{Action: "approve", Revision: st.Revision, RequestID: "approve-only", Design: st.Planning.CurrentDesign, Start: new(false)}
	st, _, err = ControlPlanning(opts, approval, "tty:exact-approval")
	require.NoError(t, err)
	require.False(t, st.Planning.StartAuthorized)
	again, receipt, err = ControlPlanning(opts, approval, "tty:exact-approval")
	require.NoError(t, err)
	require.Equal(t, "unchanged", receipt.Status)
	require.Equal(t, st.Revision, again.Revision)
	approval.RequestID, approval.Revision, approval.Start = "approve-and-start", st.Revision, new(true)
	st, _, err = ControlPlanning(opts, approval, "dsh:session:command")
	require.NoError(t, err)
	require.True(t, st.Planning.StartAuthorized)
	require.Equal(t, budget, st.Budget)
	require.Empty(t, st.Planning.Plans, "control never invents an execution plan or model result")
}

func TestPlanningControlRejectsStaleReviewsUnknownCallsAndFilesystemChanges(t *testing.T) {
	opts, st, _ := controlPlanningFixture(t)
	approval := PlanningCommand{Action: "approve", Revision: st.Revision, RequestID: "unreviewed", Design: &planning.Ref{Revision: 1, Digest: strings.Repeat("a", 64)}}
	_, _, err := ControlPlanning(opts, approval, "tty:reviewer")
	require.ErrorContains(t, err, "independent review")
	st = reviewedControlDesign(t, opts, st)
	approval.Revision, approval.Design = st.Revision, st.Planning.CurrentDesign
	st.PlanSession = map[string]any{"outcome-unknown": true}
	st, err = state.Save(opts.TaskDir, st, nil)
	require.NoError(t, err)
	approval.Revision = st.Revision
	_, _, err = ControlPlanning(opts, approval, "tty:reviewer")
	require.ErrorContains(t, err, "original invocation")
	st.PlanSession = nil
	st.Handoff = &state.Handoff{Key: "human", Thread: "thread", Cwd: opts.Cwd, Packet: "M01-T01", Candidate: strings.Repeat("a", 64), StartedAt: state.NowISO(), Reason: "Human has control"}
	st, err = state.Save(opts.TaskDir, st, nil)
	require.NoError(t, err)
	approval.Revision = st.Revision
	_, _, err = ControlPlanning(opts, approval, "tty:reviewer")
	require.ErrorContains(t, err, "human session")
	st.Handoff = nil
	st, err = state.Save(opts.TaskDir, st, nil)
	require.NoError(t, err)
	foreign := opts
	foreign.Cwd = t.TempDir()
	_, _, err = ControlPlanning(foreign, PlanningCommand{Action: "plan", Revision: st.Revision, RequestID: "foreign", Scope: "design"}, "")
	require.ErrorContains(t, err, "workspace differs")
	foreign.Cwd = filepath.Join(opts.Cwd, "missing")
	require.Error(t, planningControlWorkspace(foreign, st))
	approval.Revision = st.Revision
	approval.Design = &planning.Ref{Revision: 2, Digest: strings.Repeat("b", 64)}
	_, _, err = ControlPlanning(opts, approval, "tty:reviewer")
	require.Error(t, err)
	release, err := AcquireRunLease(opts.TaskDir)
	require.NoError(t, err)
	_, _, err = ControlPlanning(opts, approval, "tty:reviewer")
	require.ErrorContains(t, err, "task lease")
	release()
	writeFile(t, state.StateFile(opts.TaskDir), "{invalid")
	_, _, err = ControlPlanning(opts, PlanningCommand{Action: "plan", Revision: 1, RequestID: "invalid-state", Scope: "design"}, "")
	require.Error(t, err)
}

func TestPlanningControlHelpersKeepErrorsAndAuthoritySeparate(t *testing.T) {
	opts, st, _ := controlPlanningFixture(t)
	require.Nil(t, planningControlResult(nil, PlanningCommand{}, false, ""))
	require.NotEmpty(t, planningControlTime(RunOpts{}))
	require.False(t, planningHasPendingReceipt(st))
	st.WorkerPrewarm = &state.WorkerPrewarm{}
	require.True(t, planningHasPendingReceipt(st), "an unsettled process pool blocks authority changes")
	st.WorkerPrewarm = nil
	st.Sessions = nil
	require.False(t, planningHasPendingReceipt(st))
	st.Sessions = &state.SessionLedger{Records: map[string]*state.SessionRecord{"no-last": {}, "received": {Last: &state.SessionInvocation{Status: "received"}}}}
	require.True(t, planningHasPendingReceipt(st))
	delete(st.Sessions.Records, "received")
	require.False(t, planningHasPendingReceipt(st))
	st.Sessions.Records["consumed"] = &state.SessionRecord{Last: &state.SessionInvocation{Status: "consumed"}}
	require.False(t, planningHasPendingReceipt(st))
	r := PlanningCommand{Action: "plan", RequestID: "bad", Scope: "replan", ScopeID: "foreign"}
	_, err := planningCommandApplied(st, r, "")
	require.Error(t, err)
	r.Scope, r.ScopeID = "design", ""
	st.Planning.MaxRounds = 0
	require.Error(t, applyPlanningCommand(opts, st, r, ""))
	st.Planning.MaxRounds = 3
	require.Error(t, applyPlanningCommand(opts, st, PlanningCommand{Action: "feedback", RequestID: "bad-feedback", Policy: "delegated", Text: "change"}, ""))
	require.NoError(t, applyPlanningCommand(opts, st, PlanningCommand{Action: "status"}, ""))
}

func TestPlanningControlRequiresExplicitMigrationAndSettledCurrentDesign(t *testing.T) {
	fx := newGitFixture(t)
	legacy := RunOpts{TaskDir: fx.taskDir, Cwd: fx.root, Config: mustConfig(t, fx)}
	brief := planning.Brief{Version: 1, ID: fx.slug, Goal: "Review the legacy implementation", OriginalRequest: "Migrate explicitly before revising legacy contracts", Policy: "review-first", Constraints: []string{}, Evidence: []string{}}
	_, _, err := ControlPlanning(legacy, PlanningCommand{Action: "prepare", Brief: &brief}, "")
	require.ErrorContains(t, err, "explicit planning migrate")
	_, _, err = ControlPlanning(legacy, PlanningCommand{Action: "plan", Revision: 1, RequestID: "legacy-plan", Scope: "design"}, "")
	require.ErrorContains(t, err, "explicit planning migrate")
	opts, st, _ := controlPlanningFixture(t)
	st = reviewedControlDesign(t, opts, st)
	old := *st.Planning.CurrentDesign
	st = reviewedControlDesign(t, opts, st)
	_, _, err = ControlPlanning(opts, PlanningCommand{Action: "approve", Revision: st.Revision, RequestID: "stale-reviewed-design", Design: &old}, "tty:stale-design")
	require.Error(t, err, "a real earlier passing review cannot approve a newer current design")
	st.PlanSession = map[string]any{"outcome-unknown": true}
	st, err = state.Save(opts.TaskDir, st, nil)
	require.NoError(t, err)
	feedback := PlanningCommand{Action: "feedback", Revision: st.Revision, RequestID: "wait-for-receipt", Text: "Apply this only after the original call is reconciled."}
	_, receipt, err := ControlPlanning(opts, feedback, "")
	require.NoError(t, err)
	require.Equal(t, "queued", receipt.Status)
	saved, err := state.Load(opts.TaskDir)
	require.NoError(t, err)
	require.Equal(t, st.Planning.InputRevision, saved.Planning.InputRevision)
	require.Empty(t, saved.Planning.Feedback)
}

func TestPlanningPrepareFailureNeverCreatesImplementation(t *testing.T) {
	opts, _, brief := controlPlanningFixture(t)
	for _, scenario := range []string{"non-git", "missing-workspace", "invalid-brief", "existing-contracts", "invalid-state", "invalid-preparation", "blocked-task-path", "default-config"} {
		t.Run(scenario, func(t *testing.T) {
			input := opts
			input.TaskDir = filepath.Join(opts.Cwd, "docs", scenario)
			b := brief
			switch scenario {
			case "non-git":
				input.Cwd = t.TempDir()
			case "missing-workspace":
				input.Cwd = filepath.Join(opts.Cwd, "missing")
			case "invalid-brief":
				b.OriginalRequest = ""
			case "existing-contracts":
				writeFile(t, filepath.Join(input.TaskDir, "spec.md"), fixtureSpec)
			case "invalid-state":
				writeFile(t, state.StateFile(input.TaskDir), "{broken")
			case "invalid-preparation":
				writeFile(t, state.PreparationFile(input.TaskDir), "{broken")
			case "blocked-task-path":
				writeFile(t, input.TaskDir, "file")
			case "default-config":
				input.Config = nil
			}
			st, _, err := ControlPlanning(input, PlanningCommand{Action: "prepare", Brief: &b}, "")
			if scenario == "default-config" {
				require.NoError(t, err)
				require.Equal(t, "off", st.Budget.Enforce)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestPlanningQueuedFeedbackWaitsForOwnerAndConsumesOnceAcrossRestart(t *testing.T) {
	opts, st, _ := controlPlanningFixture(t)
	release, err := AcquireRunLease(opts.TaskDir)
	require.NoError(t, err)
	defer release()
	request := PlanningCommand{Action: "feedback", Revision: st.Revision, RequestID: "a-original-feedback", Text: "Preserve the existing API and add recovery tests."}
	before := mustReadFile(t, state.StateFile(opts.TaskDir))
	_, receipt, err := ControlPlanning(opts, request, "")
	require.NoError(t, err)
	require.Equal(t, "queued", receipt.Status)
	require.Equal(t, before, mustReadFile(t, state.StateFile(opts.TaskDir)))
	_, receipt, err = ControlPlanning(opts, request, "")
	require.NoError(t, err)
	require.Equal(t, "queued", receipt.Status)
	changed := request
	changed.Text = "Different input cannot reuse the id"
	_, _, err = ControlPlanning(opts, changed, "")
	require.ErrorContains(t, err, "different feedback")
	second := request
	second.RequestID, second.Text = "b-concurrent-feedback", "This observation belongs to the same original input."
	_, _, err = ControlPlanning(opts, second, "")
	require.NoError(t, err)
	c, err := CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Now: opts.Now})
	require.NoError(t, err)
	pending, err := c.planningQueuePending()
	require.NoError(t, err)
	require.True(t, pending)
	c.parallel = &parallelLane{}
	require.ErrorContains(t, c.applyPlanningQueue(), "parallel workers")
	c.parallel = nil
	c.State.PlanSession = map[string]any{"outcome-unknown": true}
	require.NoError(t, c.applyPlanningQueue())
	require.Equal(t, 1, c.State.Planning.InputRevision)
	c.State.PlanSession = nil
	oldWrite := writePlanningBytes
	writePlanningBytes = func(root *os.Root, name string, raw []byte) error {
		if strings.HasPrefix(name, "receipt-") {
			return os.ErrPermission
		}
		return oldWrite(root, name, raw)
	}
	t.Cleanup(func() { writePlanningBytes = oldWrite })
	require.ErrorIs(t, c.applyPlanningQueue(), os.ErrPermission)
	require.Len(t, c.State.Planning.Feedback, 1)
	require.True(t, c.planningPause)
	accounted := jsonClone(c.State.Budget)
	writePlanningBytes = oldWrite
	c, err = CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Now: opts.Now})
	require.NoError(t, err)
	require.NoError(t, c.applyPlanningQueue())
	require.True(t, c.planningPause, "recovered adoption retains start:false")
	require.Len(t, c.State.Planning.Feedback, 1)
	require.Equal(t, 2, c.State.Planning.InputRevision)
	require.Equal(t, accounted, c.State.Budget)
	require.Len(t, c.State.Inbox, 1, "the second input has a visible rejection instead of poisoning the queue")
	pending, err = c.planningQueuePending()
	require.NoError(t, err)
	require.False(t, pending)
	var terminal struct {
		Request planningFeedbackRequest `json:"request"`
		Receipt PlanningControlReceipt  `json:"receipt"`
	}
	require.NoError(t, json.Unmarshal(mustReadFile(t, filepath.Join(opts.TaskDir, ".control", "planning", "receipt-b-concurrent-feedback.json")), &terminal))
	require.Equal(t, "rejected", terminal.Receipt.Status)
	require.Equal(t, second.Text, terminal.Request.Command.Text)
	require.NoError(t, c.applyPlanningQueue())
	_, receipt, err = ControlPlanning(opts, request, "")
	require.NoError(t, err)
	require.Equal(t, "unchanged", receipt.Status)
}
