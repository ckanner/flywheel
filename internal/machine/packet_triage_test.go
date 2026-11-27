package machine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func packetTriageFixture(t *testing.T, _ bool) (*Context, string, string) {
	t.Helper()
	fx := newGitFixture(t)
	ctx := unitTestContext(t, fx)
	disabled := false
	ctx.Config.Worktree.Enabled = &disabled
	ps := ctx.State.Packets["M01-T01"]
	ps.Status = "inbox"
	ps.FailedAttempts = 2
	sig := "sig-1"
	ps.LastFailureSignature = &sig
	ps.RetryHistory = []any{
		map[string]any{"scope": "attempt", "error-code": string(errReviewReturned), "policy": "requeue"},
		map[string]any{"scope": "attempt", "error-code": string(errReviewReturned), "policy": "requeue"},
	}
	ps.LastFeedback = map[string]any{"error-code": string(errReviewReturned)}
	ps.Evidence.Review = map[string]any{
		"verdict":  "return",
		"reasons":  []string{"defect A"},
		"advisory": []string{"polish B"},
		"findings": []any{map[string]any{"id": "F-1", "severity": "major", "status": "open", "evidence": "e"}},
	}
	ctx.State.Status = "escalated"
	ctx.save()
	require.NoError(t, ctx.err())
	return ctx, ctx.TaskDir, ctx.Cwd
}

func triageProof(taskDir string, cmd PacketCommand) *PacketUserAction {
	return &PacketUserAction{
		Kind:     "tty",
		TaskDir:  taskDir,
		PacketID: cmd.PacketID,
		Revision: cmd.Revision,
		Action:   cmd.Action,
		Reason:   cmd.Reason,
		Guidance: cmd.Guidance,
	}
}

func TestPacketCommandAndUserActionValidation(t *testing.T) {
	var nilCmd *PacketCommand
	require.ErrorContains(t, nilCmd.Validate(), "missing packet command")
	require.ErrorContains(t, (&PacketCommand{Action: "unknown", PacketID: "M01-T01", Revision: 1, Reason: "ok"}).Validate(), "retry, replan, or accept")
	require.ErrorContains(t, (&PacketCommand{Action: "retry", PacketID: "", Revision: 1, Reason: "ok"}).Validate(), "bounded packet-id")
	require.ErrorContains(t, (&PacketCommand{Action: "retry", PacketID: "M01-T01", Revision: 0, Reason: "ok"}).Validate(), "positive revision")
	require.ErrorContains(t, (&PacketCommand{Action: "retry", PacketID: "M01-T01", Revision: 1, Reason: "   "}).Validate(), "bounded reason")
	require.ErrorContains(t, (&PacketCommand{Action: "retry", PacketID: "M01-T01", Revision: 1, Reason: "ok", Guidance: strings.Repeat("x", 2001)}).Validate(), "at most 2000")

	valid := PacketCommand{Action: "retry", PacketID: "M01-T01", Revision: 2, Reason: "retry it", Guidance: "focus on R-01"}
	require.NoError(t, valid.Validate())

	taskDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	var nilAction *PacketUserAction
	_, err = nilAction.validate(valid, taskDir)
	require.ErrorContains(t, err, "verified human command")

	_, err = (&PacketUserAction{Kind: "bot"}).validate(valid, taskDir)
	require.ErrorContains(t, err, "invalid kind")

	_, err = (&PacketUserAction{Kind: "tty", TaskDir: t.TempDir(), PacketID: "M01-T01", Revision: 2, Action: "retry", Reason: "retry it", Guidance: "focus on R-01"}).validate(valid, taskDir)
	require.ErrorContains(t, err, "does not match")

	_, err = (&PacketUserAction{Kind: "dsh", SessionID: "", CommandID: "c1", TaskDir: taskDir, PacketID: "M01-T01", Revision: 2, Action: "retry", Reason: "retry it", Guidance: "focus on R-01"}).validate(valid, taskDir)
	require.ErrorContains(t, err, "host proof is invalid")

	src, err := (&PacketUserAction{Kind: "dsh", SessionID: "s1", CommandID: "c1", TaskDir: taskDir, PacketID: "M01-T01", Revision: 2, Action: "retry", Reason: "retry it", Guidance: "focus on R-01"}).validate(valid, taskDir)
	require.NoError(t, err)
	assert.Equal(t, "dsh:s1:c1", src)

	src, err = triageProof(taskDir, valid).validate(valid, taskDir)
	require.NoError(t, err)
	assert.Equal(t, "tty", src)
}

func TestTriagePacketRetryReArmsAndResetsReviewReturnCount(t *testing.T) {
	ctx, taskDir, root := packetTriageFixture(t, false)
	t.Setenv("FLYWHEEL_RUNS_DIR", t.TempDir())

	// Projection retains inbox review findings & reasons.
	proj, err := ConsoleProjection(ctx.State)
	require.NoError(t, err)
	revMap := proj.Packets["M01-T01"].Evidence.Review.(map[string]any)
	require.Contains(t, revMap, "findings")
	require.Contains(t, revMap, "reasons")

	// Reject malformed / wrong-state / handoff / proof errors first.
	_, err = ctx.triagePacket(PacketCommand{})
	require.Error(t, err)

	ctx.State.Handoff = &state.Handoff{Key: "execute/M01-T01"}
	_, err = ctx.triagePacket(PacketCommand{Action: "retry", PacketID: "M01-T01", Revision: ctx.State.Revision, Reason: "r"})
	require.ErrorContains(t, err, "human session control")
	ctx.State.Handoff = nil

	_, err = ctx.triagePacket(PacketCommand{Action: "retry", PacketID: "M99-T99", Revision: ctx.State.Revision, Reason: "r"})
	require.ErrorContains(t, err, "not found")

	ctx.State.Packets["M01-T01"].Status = "pending"
	_, err = ctx.triagePacket(PacketCommand{Action: "retry", PacketID: "M01-T01", Revision: ctx.State.Revision, Reason: "r"})
	require.ErrorContains(t, err, "not inbox")
	ctx.State.Packets["M01-T01"].Status = "inbox"

	_, err = ctx.triagePacket(PacketCommand{Action: "retry", PacketID: "M01-T01", Revision: ctx.State.Revision, Reason: "r"})
	require.ErrorContains(t, err, "verified human command")

	// Run via RunSingleStep with matching revision and proof.
	rev := ctx.State.Revision
	cmd := PacketCommand{
		Action:   "retry",
		PacketID: "M01-T01",
		Revision: rev,
		Reason:   "Fix F-1 and ignore non-contract meta-tests",
		Guidance: "Only fix core R-01 behavior; skip meta-tests.",
	}
	cmd.UserAction = triageProof(taskDir, cmd)
	summary, err := RunSingleStep(RunSingleStepOpts{
		RunOpts:          RunOpts{TaskDir: taskDir, Cwd: root, Config: ctx.Config, Context: context.Background()},
		Only:             "packet",
		PacketID:         "M01-T01",
		ExpectedRevision: &rev,
		Packet:           &cmd,
	})
	require.NoError(t, err)
	require.NotNil(t, summary.Step)
	assert.Equal(t, "packet-retry", summary.Step.Name)
	assert.True(t, summary.Step.OK)
	assert.Equal(t, "paused", summary.Status)

	loaded, err := state.Load(taskDir)
	require.NoError(t, err)
	ps := loaded.Packets["M01-T01"]
	assert.Equal(t, "pending", ps.Status)
	assert.Equal(t, 0, ps.FailedAttempts)
	assert.Nil(t, ps.LastFailureSignature)
	assert.Equal(t, "Only fix core R-01 behavior; skip meta-tests.", ps.HumanGuidance)
	assert.Equal(t, 0, reviewReturnCount(ps))

	// RunSingleStep admission validation errors for packet stage.
	_, err = RunSingleStep(RunSingleStepOpts{RunOpts: RunOpts{TaskDir: taskDir, Cwd: root}, Only: "packet"})
	require.ErrorContains(t, err, "matching revision-bound input")
	badCmd := &PacketCommand{Action: "bad", PacketID: "M01-T01", Revision: 1, Reason: "x"}
	one := int64(1)
	_, err = RunSingleStep(RunSingleStepOpts{RunOpts: RunOpts{TaskDir: taskDir, Cwd: root}, Only: "packet", ExpectedRevision: &one, Packet: badCmd})
	require.ErrorContains(t, err, "retry, replan, or accept")
	_, err = RunSingleStep(RunSingleStepOpts{RunOpts: RunOpts{TaskDir: taskDir, Cwd: root}, Only: "verify", Packet: &cmd})
	require.ErrorContains(t, err, "packet payload requires packet stage")

	// Missing packetCommand in direct runSingle.
	ctx.packetCommand = nil
	_, err = ctx.runSingle("packet", "M01-T01")
	require.ErrorContains(t, err, "missing packet command")
}

func TestTriagePacketReplanBranches(t *testing.T) {
	ctx, taskDir, _ := packetTriageFixture(t, true)
	rev := ctx.State.Revision
	cmd := PacketCommand{
		Action:   "replan",
		PacketID: "M01-T01",
		Revision: rev,
		Reason:   "Tighten scope",
		Guidance: "Keep R-01 focused",
	}
	cmd.UserAction = triageProof(taskDir, cmd)

	// Quick workflow rejects replan.
	ctx.State.Workflow = &state.WorkflowState{Effective: "quick", Requested: "quick"}
	_, err := ctx.triagePacket(cmd)
	require.ErrorContains(t, err, "Quick has no runtime Plan")
	ctx.State.Workflow = nil

	// Unknown milestone rejected.
	ctx.State.Packets["M01-T01"].Milestone = "M-99"
	_, err = ctx.triagePacket(cmd)
	require.ErrorContains(t, err, "unknown milestone")
	ctx.State.Packets["M01-T01"].Milestone = "M-01"

	// Planner returns error / not OK.
	ctx.Config.Sessions.Mode = "off"
	ctx.Drivers = &drivers.Drivers{
		Plan: fakePlanDriver{fn: func(drivers.CallArgs) drivers.Result {
			code := 1
			return drivers.Result{Settled: true, ExitCode: &code, ErrorCode: errs.INVALID_REQUEST}
		}},
	}
	_, err = ctx.triagePacket(cmd)
	require.ErrorContains(t, err, "planner could not produce a valid revision")

	// runtimePlan returns fatal error (missing PendingStrategy decision record).
	ctx.State.Packets["M01-T01"].PendingStrategy = "missing-decision"
	ctx.State.RuntimeDecisions = &state.RuntimeDecisions{}
	_, err = ctx.triagePacket(cmd)
	require.ErrorContains(t, err, "decision record is missing")
	ctx.State.Packets["M01-T01"].PendingStrategy = ""
	ctx.State.RuntimeDecisions = nil

	// Planner succeeds and persists contracts!
	ctx.Drivers = &drivers.Drivers{
		Plan: fakePlanDriver{fn: func(drivers.CallArgs) drivers.Result {
			code := 0
			return drivers.Result{Settled: true, ExitCode: &code, Text: "revised"}
		}},
	}
	sum, err := ctx.triagePacket(cmd)
	require.NoError(t, err)
	assert.Equal(t, "packet-replan", sum.Step.Name)
	assert.Equal(t, "pending", ctx.State.Packets["M01-T01"].Status)
	assert.Equal(t, 1, ctx.State.Packets["M01-T01"].PlanRevisions)
	assert.Equal(t, "Keep R-01 focused", ctx.State.Packets["M01-T01"].HumanGuidance)
	assert.Equal(t, "paused", ctx.State.Status)

	// Persist failure branch: place .git/index.lock after planner updates spec.md so commit fails inside persistPlanContracts.
	ctx3, taskDir3, root3 := packetTriageFixture(t, true)
	cmd3 := PacketCommand{Action: "replan", PacketID: "M01-T01", Revision: ctx3.State.Revision, Reason: "Revise"}
	cmd3.UserAction = triageProof(taskDir3, cmd3)
	ctx3.Config.Sessions.Mode = "off"
	ctx3.Drivers = &drivers.Drivers{
		Plan: fakePlanDriver{fn: func(drivers.CallArgs) drivers.Result {
			_ = os.WriteFile(filepath.Join(root3, ".git", "index.lock"), []byte("locked"), 0o644)
			p := filepath.Join(taskDir3, "spec.md")
			raw, _ := os.ReadFile(p)
			_ = os.WriteFile(p, append(raw, []byte("\n# revised\n")...), 0o644)
			code := 0
			return drivers.Result{Settled: true, ExitCode: &code, Text: "revised"}
		}},
	}
	_, err = ctx3.triagePacket(cmd3)
	require.Error(t, err)
}

func TestTriagePacketAcceptRequiresL1GreenAndCommits(t *testing.T) {
	ctx, taskDir, root := packetTriageFixture(t, true)
	cmd := PacketCommand{
		Action:   "accept",
		PacketID: "M01-T01",
		Revision: ctx.State.Revision,
		Reason:   "L1 is green and remaining review findings are non-blocking",
	}
	cmd.UserAction = triageProof(taskDir, cmd)

	// Without L1 green evidence, accept is rejected.
	_, err := ctx.triagePacket(cmd)
	require.ErrorContains(t, err, "does not have passing L1 evidence")

	// Produce real passing L1 evidence + valid done-report on current candidate.
	reportBytes, _ := json.Marshal(map[string]any{
		"requirements-covered": []string{"R-01"},
		"changed-files":        []string{"build/M01-T01.txt"},
		"commands-run":         []string{"test -f build/M01-T01.txt"},
		"failures":             []string{},
	})
	require.NoError(t, os.MkdirAll(filepath.Join(taskDir, "reports"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "reports", "M01-T01.done.json"), reportBytes, 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "build"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "build", "M01-T01.txt"), []byte("v2\n"), 0o644))
	ps := ctx.State.Packets["M01-T01"]
	ps.Evidence.Executor = map[string]any{
		"done-report":   "reports/M01-T01.done.json",
		"report-sha256": fmt.Sprintf("%x", sha256.Sum256(reportBytes)),
	}
	l1 := ctx.l1Step(ctx.Packets["M01-T01"])
	require.True(t, l1.Pass)
	ps.Status = "inbox"
	ctx.State.Status = "escalated"
	ctx.save()

	// Test completeUnit failure rollback (sharedExecution active forces ErrCommitFailed).
	ctx.State.SharedExecution = &state.SharedExecution{}
	cmd.Revision = ctx.State.Revision
	cmd.UserAction = triageProof(taskDir, cmd)
	_, err = ctx.triagePacket(cmd)
	require.Error(t, err)
	assert.Equal(t, "inbox", ctx.State.Packets["M01-T01"].Status)
	assert.Nil(t, ctx.State.Packets["M01-T01"].HumanSignoff)
	ctx.State.SharedExecution = nil

	// Now accept succeeds and commits!
	cmd.Revision = ctx.State.Revision
	cmd.UserAction = triageProof(taskDir, cmd)
	sum, err := ctx.triagePacket(cmd)
	require.NoError(t, err)
	assert.Equal(t, "packet-accept", sum.Step.Name)
	assert.True(t, sum.Step.OK)
	assert.Equal(t, "completed", ctx.State.Packets["M01-T01"].Status)
	assert.Equal(t, "paused", ctx.State.Status)
	require.NotNil(t, ctx.State.Packets["M01-T01"].HumanSignoff)
	assert.Equal(t, "human", ctx.State.Packets["M01-T01"].HumanSignoff.Authority)
	assert.Equal(t, "tty", ctx.State.Packets["M01-T01"].HumanSignoff.Source)
}

func TestFinishTriagePropagatesSaveErrorAndResolvedCount(t *testing.T) {
	ctx, _, _ := packetTriageFixture(t, false)
	ctx.fatal = errors.New("disk full")
	_, err := ctx.finishTriage("packet-retry")
	require.ErrorContains(t, err, "disk full")

	assert.Equal(t, 1, countResolvedReviewFindings([]prompts.ReviewFinding{
		{ID: "F-1", Status: "resolved"},
		{ID: "F-2", Status: "open"},
	}))
}

type fakePlanDriver struct {
	fn func(drivers.CallArgs) drivers.Result
}

func (f fakePlanDriver) Name() string                              { return "fake" }
func (f fakePlanDriver) Plan(a drivers.CallArgs) drivers.Result    { return f.fn(a) }
func (f fakePlanDriver) Execute(a drivers.CallArgs) drivers.Result { return f.fn(a) }
func (f fakePlanDriver) Review(a drivers.CallArgs) drivers.Result  { return f.fn(a) }
func (f fakePlanDriver) Judge(a drivers.CallArgs) drivers.Result   { return f.fn(a) }
func (f fakePlanDriver) Intake(a drivers.CallArgs) drivers.Result  { return f.fn(a) }
func (f fakePlanDriver) Garden(a drivers.CallArgs) drivers.Result  { return f.fn(a) }
