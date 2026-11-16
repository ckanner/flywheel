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

func abandonProof(key string, revision int64, reason string) *SessionUserAction {
	return &SessionUserAction{Kind: "tty", Key: key, Revision: revision, Action: "abandon", Reason: reason}
}

func TestAbandonUnknownPlanningInvocationRetiresWithoutApplyingResult(t *testing.T) {
	fx := abandonTaskFixture(t)
	reason := "The design call was killed by the idle watchdog; retire it without replay."
	summary, err := fx.ctx.abandonSession(SessionCommand{
		Action: "abandon", Key: fx.key, Revision: fx.revision, Reason: reason,
		UserAction: abandonProof(fx.key, fx.revision, reason),
	})
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

func TestRunSingleStepAbandonsBeforeTheUnresolvedReceiptGate(t *testing.T) {
	fx := abandonTaskFixture(t)
	reason := "Human reviewed the failed attempt and retires it."
	request := SessionCommand{
		Action: "abandon", Key: fx.key, Revision: fx.revision, Reason: reason,
		UserAction: abandonProof(fx.key, fx.revision, reason),
	}
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
		{"proof mismatch", func(_ *abandonFixtureData, cmd *SessionCommand) {
			cmd.UserAction = abandonProof(cmd.Key, cmd.Revision, "different reason")
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := abandonTaskFixture(t)
			cmd := SessionCommand{Action: "abandon", Key: fx.key, Revision: fx.revision, Reason: reason,
				UserAction: abandonProof(fx.key, fx.revision, reason)}
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
	cmd := SessionCommand{Action: "abandon", Key: "plan/M-01", Revision: fx.revision, Reason: reason,
		UserAction: abandonProof("plan/M-01", fx.revision, reason)}
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
	cmd := SessionCommand{Action: "abandon", Key: fx.key, Revision: fx.revision, Reason: reason,
		UserAction: abandonProof(fx.key, fx.revision, reason)}
	_, err := fx.ctx.abandonSession(cmd)
	require.Error(t, err)
	assert.Empty(t, fx.ctx.summary().Step, "a failed persistence must not report an applied step")
}

func TestAbandonRejectsRepeatedTransition(t *testing.T) {
	fx := abandonTaskFixture(t)
	reason := "Retire this call."
	cmd := SessionCommand{Action: "abandon", Key: fx.key, Revision: fx.revision, Reason: reason,
		UserAction: abandonProof(fx.key, fx.revision, reason)}
	_, err := fx.ctx.abandonSession(cmd)
	require.NoError(t, err)
	_, err = fx.ctx.abandonSession(cmd)
	require.ErrorContains(t, err, "not blocked")
}

func TestSessionCommandValidateAbandon(t *testing.T) {
	base := SessionCommand{Action: "abandon", Key: "design-plan/planning:engine-1-1", Revision: 1}
	require.Error(t, base.Validate())
	base.Reason = "retire"
	require.NoError(t, base.Validate())
	base.Prompt = "do not send"
	require.Error(t, base.Validate())
}
