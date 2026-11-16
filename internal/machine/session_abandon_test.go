package machine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func abandonTaskFixture(t *testing.T) abandonFixtureData {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	cfg := config.Default()
	brief := planning.Brief{
		Version: 1, ID: "abandon-task", Goal: "Recover an interrupted design invocation",
		OriginalRequest: "Design the recovery path", Policy: "review-first",
		Constraints: []string{}, Evidence: []string{},
	}
	st, err := state.InitPlanning(brief, &cfg)
	require.NoError(t, err)
	st.Project = &state.ProjectIdentity{Cwd: root, WorkspaceCwd: root}
	st.Status = "escalated"
	st.Planning.Phase = "drafting"
	st.Planning.Rounds = 1
	st.Planning.Requests = []planning.Request{{
		ID: "engine-1-1", Scope: "design", ScopeID: "", InputRevision: 1, Rounds: 1,
		MaxRounds: 3, Status: "running", PlanningOnly: false, Stage: "planner",
		Role: "design-plan", Channel: "kimi-subscription",
	}}
	hash := strings.Repeat("a", 64)
	record := &state.SessionRecord{
		Task: root, Role: "design-plan", Scope: "planning:engine-1-1", Cwd: root,
		Binding: hash, Goal: hash, Contract: hash, Generation: 1,
		Action: "wait", Reason: "dispatch, outcome or cleanup remains unknown",
		Last: &state.SessionInvocation{
			ID: "call-abandon1", Status: "blocked", StartedAt: "2026-11-16T05:24:37Z",
			Attempt: 1, Persistent: true, CandidateBefore: hash, GuardBefore: hash,
			Accounted: true, Dispatch: "unknown", Outcome: "unknown",
			CleanupConfirmed: true, UsageStatus: "unknown",
		},
	}
	st.Sessions.Records = map[string]*state.SessionRecord{"design-plan/planning:engine-1-1": record}
	_, err = state.Save(root, st, nil)
	require.NoError(t, err)
	ctx, err := CreateContext(Opts{TaskDir: root, Cwd: root, Config: &cfg, Context: context.Background()})
	require.NoError(t, err)
	return abandonFixtureData{ctx: ctx, root: root, key: "design-plan/planning:engine-1-1", revision: ctx.State.Revision}
}

type abandonFixtureData struct {
	ctx      *Context
	root     string
	key      string
	revision int64
}

const abandonTestInvocation = "call-abandon1"

func abandonProof(key string, revision int64, reason, taskDir string) *SessionUserAction {
	return &SessionUserAction{Kind: "tty", TaskDir: taskDir, Key: key, Invocation: abandonTestInvocation,
		Revision: revision, Action: "abandon", Reason: reason}
}

func abandonCommand(fx *abandonFixtureData, key, reason string) SessionCommand {
	return SessionCommand{Action: "abandon", Key: key, Revision: fx.revision, Reason: reason,
		Invocation: abandonTestInvocation, UserAction: abandonProof(key, fx.revision, reason, fx.root)}
}

func TestAbandonUnknownPlanningInvocationRetiresWithoutApplyingResult(t *testing.T) {
	fx := abandonTaskFixture(t)
	reason := "The design call was killed by the idle watchdog; retire it without replay."
	summary, err := fx.ctx.abandonSession(abandonCommand(&fx, fx.key, reason))
	require.NoError(t, err)
	require.NotNil(t, summary.Step)
	assert.Equal(t, "session-abandon", summary.Step.Name)
	assert.True(t, summary.Step.OK)
	assert.Equal(t, "paused", summary.Status)

	loaded, err := state.Load(fx.root)
	require.NoError(t, err)
	record := loaded.Sessions.Records[fx.key]
	require.NotNil(t, record)
	assert.Equal(t, "abandoned", record.Last.Status)
	assert.True(t, record.Last.Accounted)
	assert.True(t, record.Last.CleanupConfirmed)
	assert.Equal(t, "unknown", record.Last.Dispatch)
	assert.Equal(t, "unknown", record.Last.Outcome)
	assert.False(t, record.ResumeCandidate)
	assert.Empty(t, record.ThreadID)
	require.NotNil(t, record.Last.Abandon)
	assert.Equal(t, "human", record.Last.Abandon.Authority)
	assert.Equal(t, "tty", record.Last.Abandon.Source)
	assert.Equal(t, "engine-1-1", record.Last.Abandon.RequestID)
	assert.Equal(t, "call-abandon1", record.Last.Abandon.Invocation)
	assert.Equal(t, "cancelled", loaded.Planning.Requests[0].Status)
	assert.False(t, planningHasPendingReceipt(loaded))
	require.Len(t, loaded.Inbox, 1)
	entry := loaded.Inbox[0].(map[string]any)
	assert.Equal(t, "session-abandon", entry["action"])
	assert.Equal(t, fx.key, entry["session"])
	assert.Equal(t, "call-abandon1", entry["invocation"])

	next, err := CreateContext(Opts{TaskDir: fx.root, Cwd: fx.root, Config: fx.ctx.Config, Context: context.Background()})
	require.NoError(t, err)
	require.NoError(t, next.reconcileSessionReceipts())
	assert.False(t, next.sessionReplay("design-plan", "planning:engine-1-1"))
}

func TestSessionUserActionDshProofBindsTaskAndInvocation(t *testing.T) {
	fx := abandonTaskFixture(t)
	cmd := abandonCommand(&fx, fx.key, "retire")
	proof := &SessionUserAction{Kind: "dsh", SessionID: "host", CommandID: "command-1", TaskDir: fx.root,
		Key: cmd.Key, Invocation: cmd.Invocation, Revision: cmd.Revision, Action: "abandon", Reason: cmd.Reason}
	source, err := proof.validate(cmd, fx.root, cmd.Invocation)
	require.NoError(t, err)
	assert.Equal(t, "dsh:host:command-1", source)
	for name, mutate := range map[string]func(*SessionUserAction){
		"missing host":       func(p *SessionUserAction) { p.SessionID = "" },
		"missing command":    func(p *SessionUserAction) { p.CommandID = "" },
		"other task":         func(p *SessionUserAction) { p.TaskDir = t.TempDir() },
		"other invocation":   func(p *SessionUserAction) { p.Invocation = "call-other" },
		"other proof kind":   func(p *SessionUserAction) { p.Kind = "model" },
		"other proof action": func(p *SessionUserAction) { p.Action = "reconcile" },
	} {
		t.Run(name, func(t *testing.T) {
			value := *proof
			mutate(&value)
			_, err := value.validate(cmd, fx.root, cmd.Invocation)
			require.Error(t, err)
		})
	}
}

func TestRunSingleStepAbandonsBeforeTheUnresolvedReceiptGate(t *testing.T) {
	fx := abandonTaskFixture(t)
	reason := "Human reviewed the failed attempt and retires it."
	request := abandonCommand(&fx, fx.key, reason)
	summary, err := RunSingleStep(RunSingleStepOpts{
		RunOpts:          RunOpts{TaskDir: fx.root, Cwd: fx.root, Config: fx.ctx.Config, Context: context.Background()},
		Only:             "session",
		ExpectedRevision: &fx.revision,
		Session:          &request,
	})
	require.NoError(t, err)
	assert.Equal(t, "session-abandon", summary.Step.Name)
	loaded, err := state.Load(fx.root)
	require.NoError(t, err)
	assert.Equal(t, "abandoned", loaded.Sessions.Records[fx.key].Last.Status)
}

func TestAbandonRejectsUnverifiedOrIneligibleCommands(t *testing.T) {
	reason := "Retire this unknown call."
	for _, tc := range []struct {
		name   string
		mutate func(*abandonFixtureData, *SessionCommand)
		want   string
	}{
		{"missing proof", func(_ *abandonFixtureData, cmd *SessionCommand) { cmd.UserAction = nil }, "verified human command"},
		{"proof mismatch", func(fx *abandonFixtureData, cmd *SessionCommand) {
			cmd.UserAction = abandonProof(cmd.Key, cmd.Revision, "different reason", fx.root)
		}, "does not match"},
		{"received record", func(fx *abandonFixtureData, _ *SessionCommand) {
			fx.ctx.State.Sessions.Records[fx.key].Last.Status = "received"
		}, "not blocked"},
		{"unaccounted", func(fx *abandonFixtureData, _ *SessionCommand) {
			fx.ctx.State.Sessions.Records[fx.key].Last.Accounted = false
		}, "not accounted"},
		{"cleanup unknown", func(fx *abandonFixtureData, _ *SessionCommand) {
			fx.ctx.State.Sessions.Records[fx.key].Last.CleanupConfirmed = false
		}, "cleanup is unconfirmed"},
		{"known outcome", func(fx *abandonFixtureData, _ *SessionCommand) {
			fx.ctx.State.Sessions.Records[fx.key].Last.Dispatch = "dispatched"
			fx.ctx.State.Sessions.Records[fx.key].Last.Outcome = "completed"
		}, "known dispatch and outcome"},
		{"non-planning role", func(fx *abandonFixtureData, cmd *SessionCommand) {
			record := fx.ctx.State.Sessions.Records[fx.key]
			delete(fx.ctx.State.Sessions.Records, fx.key)
			record.Role, record.Scope = "execute", "M01-T01"
			fx.ctx.State.Sessions.Records["execute/M01-T01"] = record
			cmd.Key = "execute/M01-T01"
		}, "limited to planning roles"},
		{"human control", func(fx *abandonFixtureData, _ *SessionCommand) {
			fx.ctx.State.Handoff = &state.Handoff{Key: "execute/M01-T01"}
		}, "human session control"},
		{"missing session", func(_ *abandonFixtureData, cmd *SessionCommand) { cmd.Key = "missing/scope" }, "session key not found"},
		{"missing ledger", func(fx *abandonFixtureData, _ *SessionCommand) { fx.ctx.State.Sessions = nil }, "session key not found"},
		{"missing invocation", func(fx *abandonFixtureData, _ *SessionCommand) {
			fx.ctx.State.Sessions.Records[fx.key].Last = nil
		}, "session key not found"},
		{"key mismatch", func(fx *abandonFixtureData, _ *SessionCommand) {
			fx.ctx.State.Sessions.Records[fx.key].Role = "execute"
		}, "does not match its recorded role"},
		{"invalid proof kind", func(_ *abandonFixtureData, cmd *SessionCommand) { cmd.UserAction.Kind = "model" }, "invalid kind"},
		{"wrong invocation", func(_ *abandonFixtureData, cmd *SessionCommand) { cmd.Invocation = "call-other" }, "does not match the recorded invocation"},
		{"terminal request", func(fx *abandonFixtureData, _ *SessionCommand) {
			fx.ctx.State.Planning.Requests[0].Status = "completed"
		}, "not pending or running"},
		{"pending activation", func(fx *abandonFixtureData, _ *SessionCommand) {
			fx.ctx.State.Planning.PendingActivation = &planning.Activation{}
		}, "activation transaction is pending"},
		{"request role mismatch", func(fx *abandonFixtureData, _ *SessionCommand) {
			fx.ctx.State.Planning.Requests[0].Role = "plan"
		}, "belongs to role plan"},
		{"missing planning request", func(fx *abandonFixtureData, cmd *SessionCommand) {
			record := fx.ctx.State.Sessions.Records[fx.key]
			delete(fx.ctx.State.Sessions.Records, fx.key)
			record.Scope = "planning:missing"
			fx.ctx.State.Sessions.Records["design-plan/planning:missing"] = record
			cmd.Key = "design-plan/planning:missing"
			cmd.UserAction.Key = cmd.Key
		}, "was not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := abandonTaskFixture(t)
			cmd := abandonCommand(&fx, fx.key, reason)
			tc.mutate(&fx, &cmd)
			_, err := fx.ctx.abandonSession(cmd)
			require.ErrorContains(t, err, tc.want)
			if fx.ctx.State.Sessions != nil {
				record := fx.ctx.State.Sessions.Records[cmd.Key]
				if record != nil && record.Last != nil {
					assert.NotEqual(t, "abandoned", record.Last.Status, "a rejected abandonment cannot retire the invocation")
				}
			}
		})
	}
}

func TestAbandonPlanRoleWithoutPlanningRequest(t *testing.T) {
	fx := abandonTaskFixture(t)
	record := fx.ctx.State.Sessions.Records[fx.key]
	delete(fx.ctx.State.Sessions.Records, fx.key)
	record.Role, record.Scope = "plan", "M-01"
	fx.ctx.State.Sessions.Records["plan/M-01"] = record
	reason := "Retire a legacy plan invocation."
	cmd := abandonCommand(&fx, "plan/M-01", reason)
	_, err := fx.ctx.abandonSession(cmd)
	require.NoError(t, err)
	assert.Equal(t, "abandoned", fx.ctx.State.Sessions.Records["plan/M-01"].Last.Status)
	assert.Empty(t, fx.ctx.State.Sessions.Records["plan/M-01"].Last.Abandon.RequestID)
}

func TestAbandonReportsSaveFailureWithoutClaimingSuccess(t *testing.T) {
	fx := abandonTaskFixture(t)
	require.NoError(t, os.Chmod(fx.root, 0o500))
	t.Cleanup(func() { _ = os.Chmod(fx.root, 0o700) })
	reason := "Retire this call."
	cmd := abandonCommand(&fx, fx.key, reason)
	_, err := fx.ctx.abandonSession(cmd)
	require.Error(t, err)
	assert.Empty(t, fx.ctx.summary().Step, "a failed persistence must not report an applied step")
}

func TestAbandonRejectsRepeatedTransition(t *testing.T) {
	fx := abandonTaskFixture(t)
	reason := "Retire this call."
	cmd := abandonCommand(&fx, fx.key, reason)
	_, err := fx.ctx.abandonSession(cmd)
	require.NoError(t, err)
	_, err = fx.ctx.abandonSession(cmd)
	require.ErrorContains(t, err, "not blocked")
}

func TestSessionCommandValidateAbandon(t *testing.T) {
	base := SessionCommand{Action: "abandon", Key: "design-plan/planning:engine-1-1", Revision: 1}
	require.Error(t, base.Validate())
	base.Reason, base.Invocation = "retire", "call-one"
	require.NoError(t, base.Validate())
	base.Invocation = ""
	require.Error(t, base.Validate())
	base.Invocation = "call-one"
	base.Prompt = "do not send"
	require.Error(t, base.Validate())
}
