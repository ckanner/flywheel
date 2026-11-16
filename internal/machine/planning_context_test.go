package machine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestReviewedContextLoadsAuthorityBeforeAnyProjectedContract(t *testing.T) {
	for _, raw := range []string{"", "{", "{}", `{"planning":null}`} {
		dir := t.TempDir()
		if raw != "" {
			writeFile(t, state.StateFile(dir), raw)
		}
		st, err := loadReviewedState(dir)
		require.NoError(t, err, "legacy diagnostics stay with the legacy loader")
		require.Nil(t, st)
	}
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(state.StateFile(dir), 0700))
	_, err := CreateContext(Opts{TaskDir: dir})
	require.Error(t, err, "unreadable planning authority cannot fall back to legacy contracts")
	require.NoError(t, os.Remove(state.StateFile(dir)))
	writeFile(t, state.StateFile(dir), `{"planning":{"unknown":"field"}}`)
	_, err = loadReviewedState(dir)
	require.Error(t, err)

	fx, c := newReviewedFixture(t)
	require.NoError(t, c.activatePlanningPlans())
	loaded := restartReviewedFixture(t, fx, c)
	require.NotNil(t, loaded.State.Planning)
	require.Nil(t, loaded.Spec)
	require.Empty(t, loaded.PacketList)
	require.NoError(t, os.Remove(state.StateFile(fx.taskDir)))
	_, err = CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config})
	require.ErrorContains(t, err, "planning-ref requires its original reviewed planning state")
	require.NoFileExists(t, state.StateFile(fx.taskDir), "downgrading the loader cannot create replacement legacy authority")
}

func TestReviewedContextDefaultsStayCancelableAndBoundToTheRecordedWorkspace(t *testing.T) {
	fx, opts, _ := briefEngineFixture(t, false)
	st := loadFixtureState(t, fx)
	input := Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config}
	c, err := newPlanningContext(input, st)
	require.NoError(t, err)
	require.NotNil(t, c.Drivers.DesignPlan)
	c.Log("thin brief has no implementation contracts")
	c.Sleep(0)
	require.False(t, c.Now().IsZero())
	t.Setenv("FLYWHEEL_PLANNING_CONTEXT_TEST", "visible")
	value, ok := c.EnvLookup("FLYWHEEL_PLANNING_CONTEXT_TEST")
	require.True(t, ok)
	require.Equal(t, "visible", value)
	require.Nil(t, c.Spec)
	require.Nil(t, c.State.Workflow)

	bad := input
	bad.TaskDir = filepath.Join(fx.root, "missing-task")
	_, err = newPlanningContext(bad, st)
	require.Error(t, err)
	bad = input
	bad.Cwd = filepath.Join(fx.root, "missing-workspace")
	_, err = newPlanningContext(bad, st)
	require.Error(t, err)
	bad.Cwd = t.TempDir()
	_, err = newPlanningContext(bad, st)
	require.ErrorContains(t, err, "recorded task workspace")
	st.Project = nil
	_, err = newPlanningContext(input, st)
	require.ErrorContains(t, err, "recorded task workspace")
	st = loadFixtureState(t, fx)
	input.Context = context.Background()
	c, err = newPlanningContext(input, st)
	require.NoError(t, err)
	c.Sleep(0)
	cancelled, cancel := context.WithCancel(context.Background())
	input.Context = cancelled
	c, err = newPlanningContext(input, st)
	require.NoError(t, err)
	cancel()
	done := make(chan struct{})
	go func() { c.Sleep(time.Hour); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("planning retry wait ignored cancellation")
	}
}

func TestReviewedContextLoadsOnlyCompleteValidatedExecutionContracts(t *testing.T) {
	fx, c := newReviewedFixture(t)
	require.ErrorContains(t, c.loadPlanningContracts(), "planning-ref downgrade fence")
	raw := mustReadFile(t, filepath.Join(fx.taskDir, "spec.md"))
	require.NoError(t, os.Remove(filepath.Join(fx.taskDir, "spec.md")))
	require.Error(t, c.loadPlanningContracts())
	writeFile(t, filepath.Join(fx.taskDir, "spec.md"), string(raw))
	require.NoError(t, c.activatePlanningPlans())
	name := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
	packet := mustReadFile(t, name)
	writeFile(t, name, "invalid packet")
	require.Error(t, c.loadPlanningContracts())
	writeFile(t, name, string(packet))
	writeFile(t, state.PreparationFile(fx.taskDir), "{")
	require.ErrorContains(t, c.loadPlanningContracts(), "investigation provenance")
	require.NoError(t, os.Remove(state.PreparationFile(fx.taskDir)))
	c.planningMode = "unrecognized-mode"
	require.Error(t, c.loadPlanningContracts())
	c.planningMode = ""
	read := readContractRevision
	t.Cleanup(func() { readContractRevision = read })
	readContractRevision = func(string) ([]byte, error) { return nil, os.ErrPermission }
	require.ErrorIs(t, c.loadPlanningContracts(), os.ErrPermission)
	readContractRevision = read
	require.NoError(t, c.loadPlanningContracts())
	require.Len(t, c.PacketList, 2)
	require.NotNil(t, c.Spec.PlanningRef)
}

func TestReviewedContextGuardsBindInputAndRejectUninspectableArtifacts(t *testing.T) {
	fx, c := newReviewedFixture(t)
	before, err := c.planningGuard()
	require.NoError(t, err)
	for _, role := range []string{"design-plan", "design-review", "plan", "plan-review"} {
		guard, err := c.sessionGuard(role, "planning:test")
		require.NoError(t, err)
		require.Equal(t, before, guard, "every planning role protects the same concrete repository and input")
	}
	inventory := snapshotGitFiles
	t.Cleanup(func() { snapshotGitFiles = inventory })
	snapshotGitFiles = func(string) ([]byte, error) { return nil, os.ErrPermission }
	_, err = c.planningGuard()
	require.ErrorIs(t, err, os.ErrPermission)
	snapshotGitFiles = inventory
	name := filepath.Join(fx.taskDir, "planning", "design-000001.md")
	raw := mustReadFile(t, name)
	writeFile(t, name, "changed outside Go")
	_, err = c.planningGuard()
	require.ErrorContains(t, err, "immutable planning export changed")
	writeFile(t, name, string(raw))
	require.NoError(t, planning.ApplyFeedback(c.State.Planning, planning.Feedback{RequestID: "input-change", Source: "user", At: c.nowISO(), Text: "Review the recovery sequence again."}))
	after, err := c.planningGuard()
	require.NoError(t, err)
	require.NotEqual(t, before, after, "unchanged repository bytes cannot make old planning input current again")
}

func TestReviewedSessionIdentitiesBindNativePolicyWithoutHashingCredentialValues(t *testing.T) {
	_, c := newReviewedFixture(t)
	args := sessionArgs(c)
	key := "PLANNING_PROVIDER_KEY"
	channel := c.Config.Channels[args.Binding.Channel]
	channel.EnvKey = &key
	c.Config.Channels[args.Binding.Channel] = channel
	args.APIKeys = map[string]string{key: "original-secret-value"}
	args.Binding.NativeProfile = "planner-team"
	c.Config.NativeProfiles = map[string]config.NativeProfile{}
	c.Config.NativeProfiles["planner-team"] = config.NativeProfile{Harness: "dsh", Options: json.RawMessage(`{"team":false}`)}
	args.SessionMode = "scoped"
	binding, goal, contractID := c.sessionIdentities("planning:root", args)
	args.APIKeys[key] = "rotated-secret-value"
	rotated, sameGoal, sameContract := c.sessionIdentities("planning:root", args)
	require.Equal(t, binding, rotated)
	require.Equal(t, goal, sameGoal)
	require.Equal(t, contractID, sameContract)
	args.APIKeys = nil
	missing, _, _ := c.sessionIdentities("planning:root", args)
	require.Equal(t, binding, missing, "the declared name survives credential removal during receipt adoption")
	args.APIKeys = map[string]string{"ANOTHER_KEY": "another-secret"}
	otherKey, _, _ := c.sessionIdentities("planning:root", args)
	require.NotEqual(t, binding, otherKey)
	args.APIKeys = nil
	c.Config.NativeProfiles["planner-team"] = config.NativeProfile{Harness: "dsh", Options: json.RawMessage(`{"team":true}`)}
	policyChanged, _, _ := c.sessionIdentities("planning:root", args)
	require.NotEqual(t, binding, policyChanged)
	_, otherGoal, _ := c.sessionIdentities("planning:another-root-request", args)
	require.NotEqual(t, goal, otherGoal)
	require.NoError(t, planning.ApplyFeedback(c.State.Planning, planning.Feedback{RequestID: "session-input", Source: "user", At: c.nowISO(), Text: "Include the restart acceptance criterion."}))
	_, feedbackGoal, feedbackContract := c.sessionIdentities("planning:root", args)
	require.Equal(t, goal, feedbackGoal, "history stays in the same declared goal")
	require.NotEqual(t, contractID, feedbackContract, "terminal result adoption binds the exact input revision")
	c.Spec = nil
	_, _, briefOnly := c.sessionIdentities("any-scope-before-projection", args)
	require.NotEmpty(t, briefOnly)
}

func TestReviewedContextAdmissionRejectsQueueErrorsMissingProjectionAndTamperedCopies(t *testing.T) {
	fx, c := newReviewedFixture(t)
	require.ErrorContains(t, c.admitReviewedExecution(), "activated first")
	require.NoError(t, c.activatePlanningPlans())
	name := filepath.Join(fx.taskDir, "planning", "design-000001.md")
	raw := mustReadFile(t, name)
	writeFile(t, name, "tampered immutable copy")
	require.ErrorContains(t, c.admitReviewedExecution(), "immutable planning export changed")
	writeFile(t, name, string(raw))
	control := filepath.Join(fx.taskDir, ".control")
	require.NoError(t, os.MkdirAll(control, 0700))
	writeFile(t, filepath.Join(control, "planning"), "not a real directory")
	require.ErrorContains(t, c.admitReviewedExecution(), "real directories")
	require.NoError(t, os.Remove(filepath.Join(control, "planning")))
	require.NoError(t, c.admitReviewedBoundary())

	// The queue is checked again after projection verification, so even a
	// concurrent invalid control-directory replacement cannot permit dispatch.
	list := listPlanningDirectory
	t.Cleanup(func() { listPlanningDirectory = list })
	calls := 0
	listPlanningDirectory = func(root *os.Root) ([]os.DirEntry, error) {
		entries, err := list(root)
		calls++
		if calls == 2 {
			writeFile(t, filepath.Join(control, "planning"), "concurrent replacement")
		}
		return entries, err
	}
	require.ErrorContains(t, c.admitReviewedBoundary(), "real directories")
}

func TestReviewedPlanningInvocationCannotBypassAdmissionOrQueuedInput(t *testing.T) {
	fx, c := newReviewedFixture(t)
	called := false
	call := func(drivers.CallArgs) drivers.Result { called = true; return drivers.MockResult(nil) }
	_, err := c.invokeScoped("execute", "M01-T01", sessionArgs(c), call)
	require.ErrorContains(t, err, "activated first")
	require.False(t, called)
	require.NoError(t, os.MkdirAll(filepath.Join(fx.taskDir, ".control"), 0700))
	writeFile(t, filepath.Join(fx.taskDir, ".control", "planning"), "invalid control directory")
	_, err = c.invokeScoped("design-plan", "planning:root", sessionArgs(c), call)
	require.ErrorContains(t, err, "real directories")
	require.False(t, called)
	require.NoError(t, os.Remove(filepath.Join(fx.taskDir, ".control", "planning")))
	require.NoError(t, os.Mkdir(filepath.Join(fx.taskDir, ".control", "planning"), 0700))
	writeFile(t, filepath.Join(fx.taskDir, ".control", "planning", "feedback-pending.json"), "unadopted request")
	_, err = c.invokeScoped("design-plan", "planning:root", sessionArgs(c), call)
	require.ErrorIs(t, err, errPlanningQueued)
	require.False(t, called, "fresh model calls cannot overtake queued user input")
}

func TestReviewedBriefRunCatalogDoesNotInventAnImplementationWorkflow(t *testing.T) {
	fx, opts, _ := briefEngineFixture(t, false)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: opts.Drivers})
	require.NoError(t, err)
	dir := t.TempDir()
	finish, err := c.beginRunRecord(dir, "plan")
	require.NoError(t, err)
	var outcome error
	c.State.Status = "paused"
	finish(&outcome)
	require.NoError(t, outcome)
	records, err := ListRuns(dir)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "planning", records[0].Workflow)
	require.Equal(t, "plan", records[0].Kind)
	require.NotEmpty(t, records[0].FinishedAt)
	require.Nil(t, c.State.Workflow)
	require.Equal(t, 4, loadFixtureState(t, fx).Sessions.Version)
}

func TestReviewedManualEntryRequiresReviewedScopeAndKeepsLegacyPlanExplicit(t *testing.T) {
	legacy := newGitFixture(t)
	require.NoError(t, unitTestContext(t, legacy).admitReviewedBoundary(), "legacy tasks keep their existing admission path")
	opts := runOpts(legacy, newScripted(nil).set())
	opts.PlanningOnly = true
	_, err := RunTask(opts)
	require.ErrorContains(t, err, "migrate legacy tasks explicitly")

	fx, fresh, calls := briefEngineFixture(t, false)
	revision := loadFixtureState(t, fx).Revision
	_, err = RunSingleStep(RunSingleStepOpts{RunOpts: fresh.RunOpts, Only: "plan", PacketID: "M01-T01", ExpectedRevision: &revision})
	require.ErrorContains(t, err, "scope packet")
	require.Empty(t, calls)
	revision = loadFixtureState(t, fx).Revision
	_, err = RunSingleStep(RunSingleStepOpts{RunOpts: fresh.RunOpts, Only: "plan", ExpectedRevision: &revision})
	require.ErrorContains(t, err, "control planning action plan for root design")
	require.NoFileExists(t, filepath.Join(fx.taskDir, "spec.md"))
	require.Empty(t, calls)
}

func TestReviewedManualEntryResumesAnOwnedPartialProjectionBeforeVerification(t *testing.T) {
	for _, public := range []bool{false, true} {
		t.Run(map[bool]string{false: "context", true: "public"}[public], func(t *testing.T) {
			fx, c := pendingReviewedFixture(t)
			var summary TaskSummary
			var err error
			if public {
				summary, err = RunSingleStep(RunSingleStepOpts{RunOpts: RunOpts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config, Drivers: c.Drivers}, Only: "verify", PacketID: "M01-T01"})
			} else {
				summary, err = c.runSingle("verify", "M01-T01")
			}
			require.NoError(t, err)
			require.NotNil(t, summary.Step)
			require.Equal(t, "verify", summary.Step.Name)
			require.False(t, summary.Step.OK, "the real gate still detects that execution has not produced the artifact")
			st := loadFixtureState(t, fx)
			require.Nil(t, st.Planning.PendingActivation)
			require.NotNil(t, st.Planning.Active)
			require.Equal(t, 4, st.Sessions.Version)
		})
	}
}

func TestReviewedManualEntryRechecksRecoveryContractsAndAuthority(t *testing.T) {
	for _, public := range []bool{false, true} {
		for _, failure := range []string{"interrupted activation", "invalid projection", "stale approval"} {
			t.Run(map[bool]string{false: "context", true: "public"}[public]+"/"+failure, func(t *testing.T) {
				fx, c := pendingReviewedFixture(t)
				if failure != "interrupted activation" {
					require.NoError(t, c.resumePlanningActivation())
				}
				switch failure {
				case "interrupted activation":
					writeFile(t, filepath.Join(fx.taskDir, "planning", "design-000001.md"), "changed while stopped")
				case "invalid projection":
					writeFile(t, filepath.Join(fx.taskDir, "spec.md"), "invalid projection")
				case "stale approval":
					require.NoError(t, planning.ApplyFeedback(c.State.Planning, planning.Feedback{RequestID: "manual-feedback", Source: "user", At: c.nowISO(), Text: "Reconsider the design before continuing."}))
					c.save()
					require.NoError(t, c.err())
				}
				var err error
				if public {
					_, err = RunSingleStep(RunSingleStepOpts{RunOpts: RunOpts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config, Drivers: c.Drivers}, Only: "verify", PacketID: "M01-T01"})
				} else {
					_, err = c.runSingle("verify", "M01-T01")
				}
				require.Error(t, err)
				require.NoDirExists(t, filepath.Join(fx.root, "build"))
			})
		}
	}
}

func TestReviewedContextPausePreservesBudgetStopAndPersistenceFailure(t *testing.T) {
	for _, phase := range []string{"awaiting-input", "budget-stopped"} {
		fx, opts, _ := briefEngineFixture(t, false)
		c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config})
		require.NoError(t, err)
		c.State.Planning.Phase = phase
		ready, err := c.pausePlanning("A human decision is needed.")
		require.NoError(t, err)
		require.False(t, ready)
		expected := "paused"
		if phase == "budget-stopped" {
			expected = "budget-stopped"
		}
		require.Equal(t, expected, loadFixtureState(t, fx).Status)
		c.fatal = errors.New("state persistence unavailable")
		_, err = c.pausePlanning("Still stopped.")
		require.ErrorIs(t, err, c.fatal)
	}
}

func reviewedHandoffFixture(t *testing.T) (fixture, *Context) {
	t.Helper()
	fx, c := newReviewedFixture(t)
	require.NoError(t, c.activatePlanningPlans())
	enableSessionPolicy(c)
	c.Config.Routes["default"] = config.RouteBinding{Harness: "dsh", Model: "test-model", Channel: "test"}
	c.Config.Channels["test"] = config.Channel{}
	a := sessionArgs(c)
	a.Binding.Provider = "test"
	result, err := c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
		return sessionResult(a, drivers.Result{Harness: "dsh", Model: "test-model", Settled: true, ExitCode: new(int), Text: "Known terminal result", Usage: &drivers.Usage{Input: 1}})
	})
	require.NoError(t, err)
	applySessionResult(t, c, "execute", a.PacketID, result)
	_, err = c.runSessionCommand(sessionRequest("claim"))
	require.NoError(t, err)
	require.NotNil(t, c.State.Handoff)
	return fx, c
}

func TestReviewedHandoffReleaseRecoversDamagedProjectionWithoutGrantingAcceptance(t *testing.T) {
	for _, public := range []bool{false, true} {
		t.Run(map[bool]string{false: "context", true: "public"}[public], func(t *testing.T) {
			fx, c := reviewedHandoffFixture(t)
			before := loadFixtureState(t, fx)
			writeFile(t, filepath.Join(fx.taskDir, "spec.md"), "Manual edit left the projection malformed.")
			request := sessionRequest("release")
			request.Revision = before.Revision
			var summary TaskSummary
			var err error
			if public {
				summary, err = RunSingleStep(RunSingleStepOpts{RunOpts: RunOpts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config, Drivers: c.Drivers}, Only: "session", Session: &request, ExpectedRevision: &request.Revision})
			} else {
				c.Spec, c.sessionCommand = nil, &request
				summary, err = c.runSingle("session", "")
			}
			require.NoError(t, err)
			require.Equal(t, "session-release", summary.Step.Name)
			require.True(t, summary.Step.OK)
			st := loadFixtureState(t, fx)
			require.Nil(t, st.Handoff)
			require.Equal(t, "paused", st.Status)
			require.Equal(t, before.Budget.Totals, st.Budget.Totals)
			require.Equal(t, before.Planning, st.Planning, "release cannot manufacture approval or active projection evidence")
			require.Nil(t, st.Packets["M01-T01"].Evidence.L1)
			require.Nil(t, st.Packets["M01-T01"].Evidence.Review)
			require.Equal(t, "Manual edit left the projection malformed.", string(mustReadFile(t, filepath.Join(fx.taskDir, "spec.md"))))
			stop := false
			_, receipt, err := ControlPlanning(RunOpts{TaskDir: fx.taskDir, Cwd: fx.root}, PlanningCommand{Action: "feedback", RequestID: "repair-after-release", Revision: st.Revision,
				Design: st.Planning.CurrentDesign, Text: "Reconcile the manual design edit before implementing anything else.", Start: &stop}, "")
			require.NoError(t, err, "releasing known human control makes the planning recovery path available again")
			require.Equal(t, "applied", receipt.Status)
		})
	}
}

func TestReviewedHandoffReleaseExceptionCannotDispatchOrDiscardUnknownEffects(t *testing.T) {
	for _, scenario := range []string{"send", "return", "claim", "reconcile", "wrong-key", "wrong-thread", "pending", "unconsumed", "unknown-cleanup", "other-unknown-invocation"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c := reviewedHandoffFixture(t)
			request := sessionRequest("release")
			r := c.State.Sessions.Records[request.Key]
			switch scenario {
			case "send", "return", "claim", "reconcile":
				request = sessionRequest(scenario)
			case "wrong-key":
				request.Key = "execute/M01-T02"
			case "wrong-thread":
				c.State.Handoff.Thread = "different-thread"
			case "pending":
				c.State.Handoff.Pending = true
			case "unconsumed":
				r.Last.Status = "received"
			case "unknown-cleanup":
				r.Last.Status = "blocked"
				r.Last.CleanupConfirmed = false
			case "other-unknown-invocation":
				a := sessionArgs(c)
				a.PacketID = "M01-T02"
				a.Binding.Provider = "test"
				_, err := c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
					result := sessionResult(a, drivers.MockResult(nil))
					result.Session.CleanupConfirmed = false
					result.CleanupError = "native child cleanup was not confirmed"
					return result
				})
				require.Error(t, err)
			}
			c.save()
			before := loadFixtureState(t, fx)
			writeFile(t, filepath.Join(fx.taskDir, "spec.md"), "broken manual contract")
			request.Revision = before.Revision
			_, err := RunSingleStep(RunSingleStepOpts{RunOpts: RunOpts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config, Drivers: c.Drivers}, Only: "session", Session: &request, ExpectedRevision: &request.Revision})
			require.Error(t, err)
			after := loadFixtureState(t, fx)
			require.Equal(t, before.Handoff, after.Handoff)
			require.Equal(t, before.Planning, after.Planning)
			require.Nil(t, after.Packets["M01-T01"].Evidence.Review)
			require.NoDirExists(t, filepath.Join(fx.root, "build"))
		})
	}
}
