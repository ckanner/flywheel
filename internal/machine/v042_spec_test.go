package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpec01ReviewConvergenceGraceAndNoPlanLoopRewrite(t *testing.T) {
	// Helper that emits structured review returns with decreasing open findings
	// while changing a source file on each execute attempt so spin detector does not fire.
	t.Run("converging review gets bounded +1 grace and passes on attempt 4", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		execCall := 0
		sd.execFn = func(a drivers.CallArgs) drivers.Result {
			execCall++
			require.NoError(t, os.MkdirAll(filepath.Join(a.Cwd, "build"), 0o755))
			pid := "M01-T01"
			if strings.Contains(a.Prompt, "packet-id: M01-T02") {
				pid = "M01-T02"
			}
			require.NoError(t, os.WriteFile(filepath.Join(a.Cwd, "build", pid+".txt"), []byte(strings.Repeat("x", execCall)), 0o644))
			writeDoneReportFile(t, a.TaskDir, pid)
			return drivers.MockResult(nil)
		}
		reviewCall := 0
		sd.reviewFn = func(drivers.CallArgs) drivers.Result {
			reviewCall++
			switch reviewCall {
			case 1:
				return textResult(`{"verdict":"return","score":0.5,"reasons":["3 issues"],"findings":[` +
					`{"id":"F-1","severity":"major","evidence":"e1","status":"open"},` +
					`{"id":"F-2","severity":"major","evidence":"e2","status":"open"},` +
					`{"id":"F-3","severity":"major","evidence":"e3","status":"open"}]}`)
			case 2:
				return textResult(`{"verdict":"return","score":0.7,"reasons":["2 issues"],"findings":[` +
					`{"id":"F-1","severity":"major","evidence":"e1","status":"resolved","resolution":"ok"},` +
					`{"id":"F-2","severity":"major","evidence":"e2","status":"open"},` +
					`{"id":"F-3","severity":"major","evidence":"e3","status":"open"}]}`)
			case 3:
				// Reaches base maxRetries (3) while converging with <=2 open findings -> +1 grace!
				return textResult(`{"verdict":"return","score":0.85,"reasons":["1 issue"],"findings":[` +
					`{"id":"F-2","severity":"major","evidence":"e2","status":"resolved","resolution":"ok"},` +
					`{"id":"F-3","severity":"major","evidence":"e3","status":"open"}]}`)
			default:
				return passVerdict()
			}
		}

		opts := runOpts(fx, sd.set())
		summary, err := RunTask(opts)
		require.NoError(t, err)
		assert.Equal(t, "completed", summary.Status)
		st := loadFixtureState(t, fx)
		ps := st.Packets["M01-T01"]
		assert.Equal(t, "completed", ps.Status)
		assert.Equal(t, 0, ps.PlanRevisions)
		assert.Equal(t, 5, reviewCall) // 4 on M01-T01 (3 returns + grace pass) + 1 on M01-T02
		assert.True(t, convergenceGraceUsed(ps))
		// SPEC-07: normal milestone close writes Delivered Milestones memory.
		pcRaw, err := os.ReadFile(filepath.Join(fx.taskDir, "project-context.md"))
		require.NoError(t, err)
		assert.Contains(t, string(pcRaw), "## Delivered Milestones")
		assert.Contains(t, string(pcRaw), "- M-01 (")
	})

	t.Run("exhausted grace goes straight to inbox, never Plan Loop", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		execCall := 0
		sd.execFn = func(a drivers.CallArgs) drivers.Result {
			execCall++
			require.NoError(t, os.MkdirAll(filepath.Join(a.Cwd, "build"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(a.Cwd, "build", "M01-T01.txt"), []byte(strings.Repeat("y", execCall)), 0o644))
			writeDoneReportFile(t, a.TaskDir, "M01-T01")
			return drivers.MockResult(nil)
		}
		reviewCall := 0
		sd.reviewFn = func(drivers.CallArgs) drivers.Result {
			reviewCall++
			switch reviewCall {
			case 1:
				return textResult(`{"verdict":"return","score":0.5,"reasons":["3 issues"],"findings":[` +
					`{"id":"F-1","severity":"major","evidence":"e1","status":"open"},` +
					`{"id":"F-2","severity":"major","evidence":"e2","status":"open"},` +
					`{"id":"F-3","severity":"major","evidence":"e3","status":"open"}]}`)
			case 2:
				return textResult(`{"verdict":"return","score":0.7,"reasons":["2 issues"],"findings":[` +
					`{"id":"F-2","severity":"major","evidence":"e2","status":"open"},` +
					`{"id":"F-3","severity":"major","evidence":"e3","status":"open"}]}`)
			case 3:
				return textResult(`{"verdict":"return","score":0.85,"reasons":["1 issue"],"findings":[` +
					`{"id":"F-3","severity":"major","evidence":"e3","status":"open"}]}`)
			default:
				return textResult(`{"verdict":"return","score":0.85,"reasons":["still 1"],"findings":[` +
					`{"id":"F-3","severity":"major","evidence":"e3","status":"open"}]}`)
			}
		}

		summary, err := RunTask(runOpts(fx, sd.set()))
		require.NoError(t, err)
		assert.Equal(t, "escalated", summary.Status)
		st := loadFixtureState(t, fx)
		ps := st.Packets["M01-T01"]
		assert.Equal(t, "inbox", ps.Status)
		assert.Equal(t, 0, ps.PlanRevisions)
		assert.Equal(t, 4, reviewCall)
	})

	t.Run("resumed review-failed at maxRetries with REVIEW_RETURNED escalates to inbox", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		ctx := verdictCtx(t, fx, sd)
		ps := ctx.State.Packets["M01-T01"]
		ps.Status = "review-failed"
		ps.FailedAttempts = 3
		ps.LastFeedback = map[string]any{"error-code": string(errReviewReturned)}
		ps.RetryHistory = []any{
			"non-map-entry",
			map[string]any{"policy": "convergence-grace"},
			map[string]any{"policy": "human-triage"},
		}
		assert.False(t, convergenceGraceUsed(ps), "human-triage resets prior convergence-grace")
		assert.Equal(t, "", lastFeedbackErrorCode(&state.PacketState{}))

		outcome, err := ctx.processPacket(ctx.Packets["M01-T01"], processOpts{})
		require.NoError(t, err)
		assert.Equal(t, "stop", outcome)
		assert.Equal(t, "inbox", ctx.State.Packets["M01-T01"].Status)
		assert.Equal(t, 0, ctx.State.Packets["M01-T01"].PlanRevisions)
	})
}

func TestSpec03MilestoneTriageAndDeliveredMemory(t *testing.T) {
	// Validate command & proof error cases.
	var nilCmd *MilestoneCommand
	require.Error(t, nilCmd.Validate())
	for _, bad := range []MilestoneCommand{
		{Action: "replan", MilestoneID: "M-01", Revision: 1, Reason: "r"},
		{Action: "retry", MilestoneID: " ", Revision: 1, Reason: "r"},
		{Action: "retry", MilestoneID: "M-01", Revision: 0, Reason: "r"},
		{Action: "retry", MilestoneID: "M-01", Revision: 1, Reason: " "},
		{Action: "retry", MilestoneID: "M-01", Revision: 1, Reason: "r", Guidance: "bad\x00g"},
	} {
		require.Error(t, bad.Validate())
	}

	validReq := MilestoneCommand{Action: "retry", MilestoneID: "M-01", Revision: 1, Reason: "r"}
	var nilProof *MilestoneUserAction
	_, err := nilProof.validate(validReq, "/tmp")
	require.Error(t, err)
	_, err = (&MilestoneUserAction{Kind: "bad"}).validate(validReq, "/tmp")
	require.Error(t, err)
	_, err = (&MilestoneUserAction{Kind: "tty", TaskDir: "/no/such/path"}).validate(validReq, "/tmp")
	require.Error(t, err)
	tmpDir := t.TempDir()
	realTmp, _ := filepath.EvalSymlinks(tmpDir)
	_, err = (&MilestoneUserAction{Kind: "dsh", TaskDir: realTmp, MilestoneID: "M-01", Revision: 1, Action: "retry", Reason: "r"}).validate(validReq, realTmp)
	require.Error(t, err)
	src, err := (&MilestoneUserAction{Kind: "dsh", SessionID: "s1", CommandID: "c1", TaskDir: realTmp, MilestoneID: "M-01", Revision: 1, Action: "retry", Reason: "r"}).validate(validReq, realTmp)
	require.NoError(t, err)
	assert.Equal(t, "dsh:s1:c1", src)
	srcTTY, err := (&MilestoneUserAction{Kind: "tty", TaskDir: realTmp, MilestoneID: "M-01", Revision: 1, Action: "retry", Reason: "r"}).validate(validReq, realTmp)
	require.NoError(t, err)
	assert.Equal(t, "tty", srcTTY)

	// RunSingleStep validation arms for milestone stage.
	_, err = RunSingleStep(RunSingleStepOpts{Only: "milestone"})
	require.ErrorContains(t, err, "matching revision-bound input")
	rev1 := int64(1)
	_, err = RunSingleStep(RunSingleStepOpts{Only: "milestone", ExpectedRevision: &rev1, Milestone: &MilestoneCommand{Action: "bad", Revision: 1}})
	require.Error(t, err)
	_, err = RunSingleStep(RunSingleStepOpts{Only: "verify", Milestone: &validReq})
	require.ErrorContains(t, err, "milestone payload requires milestone stage")

	// Context triageMilestone error branches + retry + accept (single & multi-milestone).
	fx := newGitFixture(t)
	sd := happyDrivers()
	ctx := verdictCtx(t, fx, sd)
	_, err = ctx.runSingle("milestone", "")
	require.ErrorContains(t, err, "missing milestone command")

	_, err = ctx.triageMilestone(MilestoneCommand{Action: "bad"})
	require.Error(t, err)

	ctx.State.Handoff = &state.Handoff{Key: "execute/M01-T01"}
	_, err = ctx.triageMilestone(validReq)
	require.ErrorContains(t, err, "human session control")
	ctx.State.Handoff = nil

	_, err = ctx.triageMilestone(MilestoneCommand{Action: "retry", MilestoneID: "M-99", Revision: 1, Reason: "r"})
	require.ErrorContains(t, err, "not found")

	ms := ctx.State.Milestones["M-01"]
	ms.Status = "closed"
	ctx.State.Milestones["M-01"] = ms
	_, err = ctx.triageMilestone(validReq)
	require.ErrorContains(t, err, "already closed")
	ms.Status = "in-progress"
	ctx.State.Milestones["M-01"] = ms

	_, err = ctx.triageMilestone(validReq)
	require.ErrorContains(t, err, "verified human command")

	makeProof := func(req MilestoneCommand) *MilestoneUserAction {
		return &MilestoneUserAction{
			Kind: "dsh", SessionID: "s1", CommandID: "c1", TaskDir: ctx.TaskDir,
			MilestoneID: req.MilestoneID, Revision: req.Revision, Action: req.Action, Reason: req.Reason, Guidance: req.Guidance,
		}
	}

	// Retry happy path with guidance.
	ctx.State.Status = "escalated"
	ctx.save()
	retryReq := MilestoneCommand{Action: "retry", MilestoneID: "M-01", Revision: ctx.State.Revision, Reason: "recheck", Guidance: "honor delegated scope"}
	retryReq.UserAction = makeProof(retryReq)
	sum, err := ctx.triageMilestone(retryReq)
	require.NoError(t, err)
	assert.Equal(t, "paused", sum.Status)
	assert.Equal(t, "honor delegated scope", ctx.State.Milestones["M-01"].HumanGuidance)

	// Accept gates: incomplete packet -> no packets -> no passing gate -> mismatched candidate -> finishTask dirty tree -> pass.
	acceptReq := MilestoneCommand{Action: "accept", MilestoneID: "M-01", Revision: ctx.State.Revision, Reason: "looks good", Guidance: "note"}
	acceptReq.UserAction = makeProof(acceptReq)
	_, err = ctx.triageMilestone(acceptReq)
	require.ErrorContains(t, err, "is not completed")

	savedPackets := ctx.PacketList
	ctx.PacketList = nil
	_, err = ctx.triageMilestone(acceptReq)
	require.ErrorContains(t, err, "no completed packets")
	ctx.PacketList = savedPackets

	ctx.State.Packets["M01-T01"].Status = "completed"
	ctx.State.Packets["M01-T02"].Status = "completed"
	_, err = ctx.triageMilestone(acceptReq)
	require.ErrorContains(t, err, "does not have a passing deterministic L2/L3 gate")

	ms = ctx.State.Milestones["M-01"]
	ms.Gate = map[string]any{"pass": true, "identity": "stale-sha"}
	ctx.State.Milestones["M-01"] = ms
	_, err = ctx.triageMilestone(acceptReq)
	require.ErrorContains(t, err, "does not match the current repository candidate")

	// Dirty tree makes finishTask fail and roll back milestone close.
	dirtyFile := filepath.Join(ctx.Cwd, "uncommitted.txt")
	require.NoError(t, os.WriteFile(dirtyFile, []byte("dirty"), 0o644))
	candDirty, err := ctx.candidateFingerprint(ctx.Cwd)
	require.NoError(t, err)
	ms.Gate = map[string]any{"pass": true, "identity": candDirty}
	ctx.State.Milestones["M-01"] = ms
	_, err = ctx.triageMilestone(acceptReq)
	require.Error(t, err)
	assert.Equal(t, "in-progress", ctx.State.Milestones["M-01"].Status)
	require.NoError(t, os.Remove(dirtyFile))

	// Clean tree with matching candidate + second open milestone -> pauses for next milestone.
	candClean, err := ctx.candidateFingerprint(ctx.Cwd)
	require.NoError(t, err)
	ms.Gate = map[string]any{"pass": true, "identity": candClean}
	ctx.State.Milestones["M-01"] = ms
	ctx.Spec.Milestones = append(ctx.Spec.Milestones, contract.Milestone{ID: "M-02", Title: "Next"})
	ctx.State.Milestones["M-02"] = state.MilestoneState{Status: "pending"}
	sum, err = ctx.triageMilestone(acceptReq)
	require.NoError(t, err)
	assert.Equal(t, "paused", sum.Status)
	assert.Equal(t, "closed", ctx.State.Milestones["M-01"].Status)
	require.NotNil(t, ctx.State.Milestones["M-01"].HumanSignoff)

	// Re-accept with single milestone (all milestones closed -> completed) via RunSingleStep.
	ctx.Spec.Milestones = ctx.Spec.Milestones[:1]
	delete(ctx.State.Milestones, "M-02")
	ms = ctx.State.Milestones["M-01"]
	ms.Status = "in-progress"
	ms.ClosedAt = nil
	ms.HumanSignoff = nil
	ctx.State.Milestones["M-01"] = ms
	ctx.State.Status = "escalated"
	ctx.save()
	revNow := ctx.State.Revision
	acceptStep := &MilestoneCommand{Action: "accept", MilestoneID: "M-01", Revision: revNow, Reason: "final accept"}
	acceptStep.UserAction = &MilestoneUserAction{
		Kind: "tty", TaskDir: ctx.TaskDir, MilestoneID: "M-01", Revision: revNow, Action: "accept", Reason: "final accept",
	}
	stepSum, err := RunSingleStep(RunSingleStepOpts{
		RunOpts:          runOpts(fx, sd.set()).RunOpts,
		Only:             "milestone",
		ExpectedRevision: &revNow,
		Milestone:        acceptStep,
	})
	require.NoError(t, err)
	assert.Equal(t, "completed", stepSum.Status)

	// Delivered memory idempotency, existing section append, and non-git shortHeadSHA fallback.
	pcPath := filepath.Join(ctx.TaskDir, "project-context.md")
	beforePC, err := os.ReadFile(pcPath)
	require.NoError(t, err)
	ctx.appendDeliveredMilestoneMemory(ctx.Spec.Milestones[0], "duplicate should be ignored")
	afterPC, err := os.ReadFile(pcPath)
	require.NoError(t, err)
	assert.Equal(t, string(beforePC), string(afterPC))

	// Existing project-context.md without "## Delivered Milestones" header.
	require.NoError(t, os.WriteFile(pcPath, []byte("# Stack\nGo\n"), 0o644))
	ctx.appendDeliveredMilestoneMemory(contract.Milestone{ID: "M-02", Title: "Second"}, "ok")
	ctx.appendDeliveredMilestoneMemory(contract.Milestone{ID: "M-03", Title: "Third"}, "ok")
	rawPC, err := os.ReadFile(pcPath)
	require.NoError(t, err)
	assert.Contains(t, string(rawPC), "## Delivered Milestones\n- M-02 (Second)")
	assert.Contains(t, string(rawPC), "\n- M-03 (Third)")

	assert.Equal(t, "n/a", shortHeadSHA(t.TempDir()))
}

func TestSpec03CompactMilestoneGateProjection(t *testing.T) {
	longErr := strings.Repeat("e", 600)
	st := &state.State{
		Packets: map[string]*state.PacketState{},
		Milestones: map[string]state.MilestoneState{
			"M-01": {
				Status: "in-progress",
				Gate: map[string]any{
					"pass":     false,
					"at":       "2026-11-27T00:00:00Z",
					"identity": "sha1",
					"judge":    map[string]any{"verdict": "return", "score": 0.78, "reasons": []any{"r1"}, "extra": "dropped"},
					"levels": map[string]any{
						"prepare": "non-map-level",
						"L2": map[string]any{
							"pass": false,
							"results": []any{
								"non-map-result",
								map[string]any{"cmd": "npm test", "pass": false, "exitCode": 1, "stderr": longErr, "stdout": "ok"},
							},
						},
					},
				},
			},
			"M-02": {
				Status: "in-progress",
				Gate:   "raw-string-gate",
			},
		},
	}
	view, err := ConsoleProjection(st)
	require.NoError(t, err)
	g1 := view.Milestones["M-01"].Gate.(map[string]any)
	assert.Equal(t, false, g1["pass"])
	l2 := g1["levels"].(map[string]any)["L2"].(map[string]any)
	res1 := l2["results"].([]any)[1].(map[string]any)
	assert.True(t, strings.HasPrefix(res1["stderr"].(string), "[truncated]"))
	assert.Equal(t, "ok", res1["stdout"])
	assert.Equal(t, "raw-string-gate", view.Milestones["M-02"].Gate)
}

func TestSpec04MilestoneGatePrepareCmd(t *testing.T) {
	fx := newGitFixture(t)
	// Add an L2 command to M01-T01 packet so milestoneGateStep executes prepare-cmd.
	pktPath := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
	rawPkt, err := os.ReadFile(pktPath)
	require.NoError(t, err)
	updated := strings.Replace(string(rawPkt), "review-rubric:", "  L2:\n    - {cmd: \"test -f .prepared-marker\", expect: \"exit 0\"}\nreview-rubric:", 1)
	require.NotEqual(t, string(rawPkt), updated)
	require.NoError(t, os.WriteFile(pktPath, []byte(updated), 0o644))
	// Ignore the untracked marker in git so candidateFingerprint stays clean.
	require.NoError(t, os.WriteFile(filepath.Join(fx.root, ".gitignore"), []byte(".prepared-marker\n"), 0o644))
	gitCmd(t, fx.root, "add", ".gitignore", pktPath)
	gitCmd(t, fx.root, "commit", "-m", "add L2 and gitignore")

	cfg := config.Default()
	cfg.Verify.PrepareCmd = "touch .prepared-marker"
	ctx, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: &cfg, Drivers: happyDrivers().set(), Log: func(string) {}})
	require.NoError(t, err)
	res := ctx.milestoneGateStep(ctx.Spec.Milestones[0])
	assert.True(t, res.Pass)
	_, hasPrep := res.Levels["prepare"]
	assert.True(t, hasPrep)

	// Failing prepare-cmd fails gate cleanly and records levels["prepare"].
	ctx.Config.Verify.PrepareCmd = "exit 7"
	resFail := ctx.milestoneGateStep(ctx.Spec.Milestones[0])
	assert.False(t, resFail.Pass)
	_, hasPrepFail := resFail.Levels["prepare"]
	assert.True(t, hasPrepFail)
}

func TestSpec05SessionContextWorktreeLedgerFallback(t *testing.T) {
	fx := newGitFixture(t)
	ctx := verdictCtx(t, fx, happyDrivers())
	// Simulate a git worktree directory that does NOT contain untracked ledger files.
	wt := t.TempDir()
	ctx.WorkspaceCwd = fx.root
	// Put an extra ledger file in WorkspaceCwd only (missing from wt) and add to ContextFiles.
	designRel := filepath.Join(ctx.taskDirPrefix(), "planning", "design-000001.md")
	require.NoError(t, os.MkdirAll(filepath.Join(fx.root, filepath.Dir(designRel)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fx.root, designRel), []byte("# Design"), 0o644))
	ctx.Packets["M01-T01"].ContextFiles = []string{designRel}

	cp, _ := ctx.sessionContextPrompt("execute", "M01-T01", "fresh", "cand-1", nil, drivers.CallArgs{
		Cwd:                    wt,
		TaskDir:                ctx.TaskDir,
		Prompt:                 "full prompt",
		ContextTemplateVersion: executeContextVersion,
	})
	require.NotNil(t, cp)
	assert.NotEmpty(t, cp.Identity, "ledger refs must fall back to WorkspaceCwd and produce a valid identity")

	// Non-ledger source code file missing from wt MUST NOT fall back to WorkspaceCwd!
	require.NoError(t, os.WriteFile(filepath.Join(fx.root, "main-only.go"), []byte("package main"), 0o644))
	ctx.Packets["M01-T01"].ContextFiles = []string{"main-only.go"}
	cpMissing, _ := ctx.sessionContextPrompt("execute", "M01-T01", "fresh", "cand-1", nil, drivers.CallArgs{
		Cwd:                    wt,
		TaskDir:                ctx.TaskDir,
		Prompt:                 "full prompt",
		ContextTemplateVersion: executeContextVersion,
	})
	require.NotNil(t, cpMissing)
	assert.Empty(t, cpMissing.Identity)
	assert.Contains(t, cpMissing.Reason, "unavailable")

	// isLedgerRef unit checks.
	assert.False(t, isLedgerRef("/abs/path", "docs/slug"))
	assert.False(t, isLedgerRef("docs/slug/spec.md", ""))
	assert.True(t, isLedgerRef("docs/slug/spec.md", "docs/slug"))
	assert.False(t, isLedgerRef("src/app.ts", "docs/slug"))
}

func textResult(s string) drivers.Result {
	return drivers.MockResult(func(r *drivers.Result) {
		r.Text = s
	})
}

func writeDoneReportFile(t *testing.T, taskDir, packetID string) {
	t.Helper()
	dir := filepath.Join(taskDir, "reports")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	b, _ := json.Marshal(map[string]any{
		"requirements-covered": []string{"R-01"},
		"changed-files":        []string{"build/" + packetID + ".txt"},
		"commands-run":         []string{"test -f build/" + packetID + ".txt"},
		"failures":             []string{},
	})
	require.NoError(t, os.WriteFile(filepath.Join(dir, packetID+".done.json"), b, 0o644))
}
