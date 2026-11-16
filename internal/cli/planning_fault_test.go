package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/diagnostic"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func planningCLIControl(t *testing.T, taskDir, cwd string, request machine.PlanningCommand, flags ...string) (controlResponse, error) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetIn(strings.NewReader(mustJSONT(t, request)))
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs(append([]string{"control", "planning", "--task-dir", taskDir, "--cwd", cwd}, flags...))
	err := cmd.Execute()
	require.Empty(t, stderr.String())
	var reply controlResponse
	require.NoError(t, json.Unmarshal(out.Bytes(), &reply), out.String())
	require.Equal(t, 1, strings.Count(out.String(), "\n"))
	require.Equal(t, err == nil, reply.OK, reply.Error)
	return reply, err
}

func planningCLIFixture(t *testing.T, reviewed bool) (string, string, *state.State) {
	t.Helper()
	cwd := t.TempDir()
	gitCliInitSandbox(t, cwd)
	writeFileCli(t, filepath.Join(cwd, "README.md"), "Planning control fixture\n")
	gitCliCommitAll(t, cwd, "test: planning baseline")
	taskDir := filepath.Join(cwd, "docs", "reviewed-cli")
	e2eConfigT(t, taskDir)
	brief := planning.Brief{Version: 1, ID: "reviewed-cli", Goal: "Recover acknowledged work", OriginalRequest: "Discuss recovery before implementation", Policy: "review-first", Constraints: []string{}, Evidence: []string{}}
	reply, err := planningCLIControl(t, taskDir, cwd, machine.PlanningCommand{Action: "prepare", Brief: &brief, Start: new(false)})
	require.NoError(t, err, reply.Error)
	st := reply.State
	design, err := planning.SealDesign(planning.Design{Ref: planning.Ref{Revision: 1}, InputRevision: 1,
		Document:     "# Authoritative recovery design\nPersist a checkpoint before acknowledging work. Preserve the public API and prove recovery with an interrupted worker test.",
		Requirements: []planning.Requirement{{ID: "R-01", Title: "Acknowledged work survives a restart", Milestone: "M-01", Verification: "restart regression passes"}},
		Milestones:   []planning.Milestone{{ID: "M-01", Title: "Recover work", Gate: "restart regression passes"}},
		Decisions:    []planning.Decision{}, ImplementationScope: []string{"src"}, Baseline: strings.Repeat("a", 40), PlannerInvocation: "fixture-planner"})
	require.NoError(t, err)
	require.NoError(t, planning.RecordDesign(st.Planning, design))
	if reviewed {
		require.NoError(t, planning.RecordReview(st.Planning, planning.Review{ID: "fixture-review", Kind: "design", Target: design.Ref, InputRevision: 1,
			Invocation: "independent-fixture-reviewer", Role: "design-review", Binding: "reviewer-binding", Verdict: "pass", Summary: "Checkpoint ordering and recovery verification are explicit.", Findings: []planning.Finding{}}))
	}
	st, err = state.Save(taskDir, st, nil)
	require.NoError(t, err)
	return taskDir, cwd, st
}

func planningCLIApproval(st *state.State, id string, start bool) machine.PlanningCommand {
	return machine.PlanningCommand{Action: "approve", Revision: st.Revision, RequestID: id, Design: st.Planning.CurrentDesign, Start: &start}
}

func planningCLIProof(t *testing.T, request machine.PlanningCommand) {
	t.Helper()
	t.Setenv("DSH_SESSION_ID", "test-desktop-session")
	t.Setenv("FLYWHEEL_PLANNING_USER_ACTION", mustJSONT(t, planningUserAction{Kind: "dsh", SessionID: "test-desktop-session", CommandID: "user-command-1",
		RequestID: request.RequestID, Revision: request.Revision, Design: *request.Design, Start: request.StartRequested(false)}))
}

type planningTTYFixture struct {
	input    io.Reader
	output   bytes.Buffer
	writeErr error
	closed   bool
}

func (f *planningTTYFixture) Read(p []byte) (int, error) { return f.input.Read(p) }
func (f *planningTTYFixture) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.output.Write(p)
}
func (f *planningTTYFixture) Close() error { f.closed = true; return nil }

func TestPlanningCLIRejectsMalformedAndActionSpecificInputBeforeMutation(t *testing.T) {
	for _, input := range []io.Reader{
		failingEditorReader{}, strings.NewReader(""), strings.NewReader("null"), strings.NewReader("[]"),
		strings.NewReader(strings.Repeat(" ", (128<<10)+1)), strings.NewReader(`{"action":"status"} {}`),
		strings.NewReader(`{"action":"status","actor":"user"}`), strings.NewReader(`{"action":"status","start":true}`),
		strings.NewReader(`{"action":"prepare","brief":{},"start":null}`), strings.NewReader(`{"action":"status","request-id":null}`),
		strings.NewReader(`{"action":"unsupported"}`), strings.NewReader(`{"action":"feedback","revision":1,"request-id":"f"}`),
		strings.NewReader(`{"action":"approve","revision":1,"request-id":"a"}`),
	} {
		_, err := readPlanningCommand(input)
		require.Error(t, err)
	}
	for _, text := range []string{`{"action":"status"}`, `{"action":"status","request-id":"f-1"}`, `{"action":"plan","revision":1,"request-id":"p1","scope":"design","text":"Explain recovery within the approved API","start":false}`} {
		_, err := readPlanningCommand(strings.NewReader(text))
		require.NoError(t, err)
	}
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(`{"action":"prepare","source":"human"}`))
	_, err := controlPlanning(cmd, t.TempDir(), t.TempDir())
	require.Error(t, err)
	directory := t.TempDir()
	resolved, err := planningControlDirectory(directory)
	require.NoError(t, err)
	require.Equal(t, directory, resolved)
	missing := filepath.Join(directory, "not-created")
	resolved, err = planningControlDirectory(missing)
	require.NoError(t, err)
	require.Equal(t, missing, resolved)
	require.NoDirExists(t, missing)
	_, err = planningControlDirectory("relative/task")
	require.Error(t, err)
	file := filepath.Join(directory, "ordinary-file")
	writeFileCli(t, file, "not a task directory")
	_, err = planningControlDirectory(file)
	require.Error(t, err)
}

func TestPlanningCLICreatesBriefWithoutModelsAndReadsLiveQueuedStatus(t *testing.T) {
	taskDir, cwd, st := planningCLIFixture(t, true)
	reply, err := planningCLIControl(t, taskDir, cwd, machine.PlanningCommand{Action: "status"}, "--compact")
	require.NoError(t, err)
	require.Equal(t, st.Planning.Brief.Goal, reply.TaskTitle)
	require.False(t, reply.Running)
	require.Equal(t, "console-excerpt", reply.Projection)
	require.NoFileExists(t, filepath.Join(taskDir, "spec.md"))
	release, err := machine.AcquireRunLease(taskDir)
	require.NoError(t, err)
	defer release()
	feedback := machine.PlanningCommand{Action: "feedback", Revision: st.Revision, RequestID: "queued-ui-feedback", Text: "Discuss an additional recovery constraint", Start: new(false)}
	reply, err = planningCLIControl(t, taskDir, cwd, feedback)
	require.NoError(t, err)
	require.True(t, reply.Running)
	require.Equal(t, "queued", reply.PlanningControl.Status)
	before, err := os.ReadFile(filepath.Join(taskDir, "state.json"))
	require.NoError(t, err)
	reply, err = planningCLIControl(t, taskDir, cwd, machine.PlanningCommand{Action: "status", RequestID: feedback.RequestID}, "--compact")
	require.NoError(t, err)
	require.True(t, reply.Running, "request receipt reads retain the actual lifetime lease instead of falsely reporting stopped")
	require.Equal(t, st.Planning.Brief.Goal, reply.TaskTitle)
	require.Equal(t, "queued", reply.PlanningControl.Status)
	require.Empty(t, reply.State.Planning.Feedback)
	after, err := os.ReadFile(filepath.Join(taskDir, "state.json"))
	require.NoError(t, err)
	require.Equal(t, before, after, "status never consumes the running owner's feedback or receipt")
}

func TestPlanningCLIRefinementInstructionsRemainScopedToTheirRequest(t *testing.T) {
	taskDir, cwd, st := planningCLIFixture(t, true)
	approval := planningCLIApproval(st, "approve-before-refinement", false)
	planningCLIProof(t, approval)
	reply, err := planningCLIControl(t, taskDir, cwd, approval)
	require.NoError(t, err)
	approved := reply.State.Planning
	text := "Expand restart recovery steps for this milestone while retaining the approved API."
	reply, err = planningCLIControl(t, taskDir, cwd, machine.PlanningCommand{Action: "plan", Revision: reply.State.Revision,
		RequestID: "local-refinement", Scope: "milestone", ScopeID: "M-01", Text: text, Start: new(false)})
	require.NoError(t, err)
	require.False(t, reply.Running)
	require.Equal(t, "applied", reply.PlanningControl.Status)
	p := reply.State.Planning
	require.Equal(t, approved.CurrentDesign, p.CurrentDesign)
	require.Equal(t, approved.InputRevision, p.InputRevision)
	require.Equal(t, approved.Approvals, p.Approvals, "local refinement does not revoke or replace root approval")
	require.Empty(t, p.Feedback)
	request := p.Requests[len(p.Requests)-1]
	require.Equal(t, "milestone", request.Scope)
	require.Equal(t, "M-01", request.ScopeID)
	require.Equal(t, text, request.Feedback)
	require.True(t, request.PlanningOnly)
}

func TestPlanningCLIStatusPreservesSettledInvestigationEvidence(t *testing.T) {
	_, cwd, contract := investigationCLIFixture(t)
	taskDir := filepath.Join(cwd, "docs", "report-to-plan")
	e2eConfigT(t, taskDir)
	started := startInvestigationCLI(t, taskDir, cwd, contract)
	finished, err := machine.RunInvestigation(nil, taskDir, started.Revision)
	require.NoError(t, err)
	report := investigationCLIReport(finished)
	reply, err := investigationControl(t, taskDir, cwd, investigationRequest{Action: "report", Revision: finished.Revision, Report: &report})
	require.NoError(t, err)
	require.Equal(t, "supported", reply.Preparation.Status)
	brief := planning.Brief{Version: 1, ID: "report-to-plan", Goal: "Design a repair based on the investigation", OriginalRequest: "Review a repair design before implementing it", Policy: "review-first"}
	reply, err = planningCLIControl(t, taskDir, cwd, machine.PlanningCommand{Action: "prepare", Brief: &brief, Start: new(false)})
	require.NoError(t, err)
	require.NotNil(t, reply.State.Planning)
	require.NotNil(t, reply.Preparation)
	require.Equal(t, report.Summary, reply.Preparation.Report.Summary)
	require.Equal(t, state.PreparationHash(&report), reply.State.PreparationSource.ReportHash)
	reply, err = planningCLIControl(t, taskDir, cwd, machine.PlanningCommand{Action: "status"}, "--compact")
	require.NoError(t, err)
	require.Equal(t, "supported", reply.Preparation.Status, "planning status must retain the settled investigation that authorized its handoff")
	require.Equal(t, brief.Goal, reply.TaskTitle)
	require.False(t, reply.State.Planning.StartAuthorized)
}

func TestPlanningCLIHumanStatusDistinguishesUnactivatedPlanningFromLegacyFull(t *testing.T) {
	taskDir, cwd, st := planningCLIFixture(t, true)
	before, err := os.ReadFile(filepath.Join(taskDir, "state.json"))
	require.NoError(t, err)
	code, human := runCLI(t, "status", taskDir)
	require.Zero(t, code, human)
	require.Contains(t, human, "规划: awaiting-design-approval")
	require.Contains(t, human, "实施启动已授权: false")
	require.Contains(t, human, st.Planning.CurrentDesign.Digest)
	require.Contains(t, human, "实现尚未激活")
	require.NotContains(t, human, "旧任务保持完整流程")
	after, err := os.ReadFile(filepath.Join(taskDir, "state.json"))
	require.NoError(t, err)
	require.Equal(t, before, after, "human status is read-only and cannot approve the displayed design")

	st, _, err = machine.ControlPlanning(machine.RunOpts{TaskDir: taskDir, Cwd: cwd}, planningCLIApproval(st, "old-design-start", true), "dsh:fixture:old-start")
	require.NoError(t, err)
	previousReview := *planning.LatestReview(st.Planning, "design", *st.Planning.CurrentDesign)
	design := *planning.FindDesign(st.Planning, *st.Planning.CurrentDesign)
	design.Ref = planning.Ref{Revision: design.Ref.Revision + 1}
	design.Document += "\nSpecify idempotent recovery before retry."
	design, err = planning.SealDesign(design)
	require.NoError(t, err)
	require.NoError(t, planning.RecordDesign(st.Planning, design))
	previousReview.ID, previousReview.Target, previousReview.Invocation = "new-root-review", design.Ref, "new-independent-review"
	require.NoError(t, planning.RecordReview(st.Planning, previousReview))
	st, err = state.Save(taskDir, st, nil)
	require.NoError(t, err)
	approveOnly := planningCLIApproval(st, "new-design-approval-only", false)
	planningCLIProof(t, approveOnly)
	reply, err := planningCLIControl(t, taskDir, cwd, approveOnly)
	require.NoError(t, err)
	require.True(t, reply.State.Planning.StartAuthorized, "historical start remains in the authoritative ledger")
	require.False(t, planning.StartAllowed(reply.State.Planning, design.Ref))
	code, human = runCLI(t, "status", taskDir)
	require.Zero(t, code, human)
	require.Contains(t, human, "实施启动已授权: false", "human status reports the current design grant rather than historical start")
	require.Contains(t, human, design.Ref.Digest)

	thinDir := filepath.Join(cwd, "docs", "unstarted-brief")
	e2eConfigT(t, thinDir)
	brief := planning.Brief{Version: 1, ID: "unstarted-brief", Goal: "Discuss a system design", OriginalRequest: "Only register the brief", Policy: "review-first"}
	_, err = planningCLIControl(t, thinDir, cwd, machine.PlanningCommand{Action: "prepare", Brief: &brief, Start: new(false)})
	require.NoError(t, err)
	code, human = runCLI(t, "status", thinDir)
	require.Zero(t, code, human)
	require.Contains(t, human, "规划: drafting")
	require.Contains(t, human, "实现尚未激活")
	require.NotContains(t, human, "当前方案:")
	require.NotContains(t, human, "旧任务保持完整流程")
}

func TestPlanningCLIApprovalProofBindsEveryExactHostField(t *testing.T) {
	_, _, st := planningCLIFixture(t, true)
	request := planningCLIApproval(st, "exact-approval", true)
	proof := planningUserAction{Kind: "dsh", SessionID: "desktop-session", CommandID: "command-1", RequestID: request.RequestID,
		Revision: request.Revision, Design: *request.Design, Start: true}
	t.Setenv("DSH_SESSION_ID", proof.SessionID)
	for name, modify := range map[string]func(*planningUserAction){
		"wrong kind": func(p *planningUserAction) { p.Kind = "model" }, "wrong session": func(p *planningUserAction) { p.SessionID = "other" },
		"empty command": func(p *planningUserAction) { p.CommandID = " " }, "multiline command": func(p *planningUserAction) { p.CommandID = "user\nmodel" },
		"long command":  func(p *planningUserAction) { p.CommandID = strings.Repeat("c", 201) },
		"wrong request": func(p *planningUserAction) { p.RequestID = "other" }, "wrong revision": func(p *planningUserAction) { p.Revision++ },
		"wrong design": func(p *planningUserAction) { p.Design.Digest = strings.Repeat("b", 64) },
		"wrong start":  func(p *planningUserAction) { p.Start = false },
	} {
		t.Run(name, func(t *testing.T) {
			changed := proof
			modify(&changed)
			t.Setenv("FLYWHEEL_PLANNING_USER_ACTION", mustJSONT(t, changed))
			_, err := planningSource("/task", request, nil, nil)
			require.Error(t, err)
		})
	}
	for _, raw := range []string{"{", strings.Repeat("x", 4097), strings.TrimSuffix(mustJSONT(t, proof), "}") + `,"actor":"user"}`} {
		t.Setenv("FLYWHEEL_PLANNING_USER_ACTION", raw)
		_, err := planningSource("/task", request, nil, nil)
		require.Error(t, err)
	}
	t.Setenv("FLYWHEEL_PLANNING_USER_ACTION", mustJSONT(t, proof))
	source, err := planningSource("/task", request, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "dsh:desktop-session:command-1", source)
}

func TestPlanningCLITerminalUsesAuthoritativeDesignAndIndependentInput(t *testing.T) {
	taskDir, cwd, st := planningCLIFixture(t, true)
	request := planningCLIApproval(st, "tty-approval", false)
	design := planning.FindDesign(st.Planning, *request.Design)
	review := planning.LatestReview(st.Planning, "design", *request.Design)
	original := openPlanningTTY
	t.Cleanup(func() { openPlanningTTY = original })
	// Exercise the production controlling-terminal opener without reading it.
	if terminal, err := original(); err == nil {
		require.NoError(t, terminal.Close())
	}
	oldProof, present := os.LookupEnv("FLYWHEEL_PLANNING_USER_ACTION")
	require.NoError(t, os.Unsetenv("FLYWHEEL_PLANNING_USER_ACTION"))
	t.Cleanup(func() {
		if present {
			_ = os.Setenv("FLYWHEEL_PLANNING_USER_ACTION", oldProof)
		} else {
			_ = os.Unsetenv("FLYWHEEL_PLANNING_USER_ACTION")
		}
	})
	openPlanningTTY = func() (io.ReadWriteCloser, error) { return nil, os.ErrPermission }
	_, err := planningSource(taskDir, request, design, review)
	require.ErrorContains(t, err, "interactive controlling terminal")
	for _, input := range []string{"approve wrong-digest\n", "approve " + request.Design.Digest, strings.Repeat("x", 300) + "\n"} {
		terminal := &planningTTYFixture{input: strings.NewReader(input)}
		openPlanningTTY = func() (io.ReadWriteCloser, error) { return terminal, nil }
		_, err := planningSource(taskDir, request, design, review)
		require.ErrorContains(t, err, "did not confirm")
		require.True(t, terminal.closed)
	}
	terminal := &planningTTYFixture{input: strings.NewReader("approve " + request.Design.Digest + "\n"), writeErr: os.ErrPermission}
	openPlanningTTY = func() (io.ReadWriteCloser, error) { return terminal, nil }
	_, err = planningSource(taskDir, request, design, review)
	require.ErrorIs(t, err, os.ErrPermission)
	require.True(t, terminal.closed)
	terminal = &planningTTYFixture{input: strings.NewReader("approve " + request.Design.Digest + "\n")}
	openPlanningTTY = func() (io.ReadWriteCloser, error) { return terminal, nil }
	writeFileCli(t, filepath.Join(taskDir, "planning", "design-000001.md"), "Untrusted externally edited export")
	reply, err := planningCLIControl(t, taskDir, cwd, request)
	require.NoError(t, err, reply.Error)
	require.Contains(t, terminal.output.String(), "Authoritative recovery design")
	require.Contains(t, terminal.output.String(), "Persist a checkpoint before acknowledging work.")
	require.Contains(t, terminal.output.String(), review.Summary)
	require.NotContains(t, terminal.output.String(), "Untrusted externally edited export")
	require.Contains(t, terminal.output.String(), "Start implementation: false")
	require.True(t, terminal.closed)
	require.Len(t, reply.State.Planning.Approvals, 1)
	require.Equal(t, "tty:"+request.RequestID, reply.State.Planning.Approvals[0].Source)
	require.False(t, reply.State.Planning.StartAuthorized)
}

func TestPlanningCLIApprovalAndConfigurationFailuresDoNotStart(t *testing.T) {
	for _, scenario := range []string{"missing state", "stale revision", "unreviewed", "invalid proof", "config", "lease owner"} {
		t.Run(scenario, func(t *testing.T) {
			taskDir, cwd, st := planningCLIFixture(t, scenario != "unreviewed")
			request := planningCLIApproval(st, "approval-failure", false)
			planningCLIProof(t, request)
			switch scenario {
			case "missing state":
				require.NoError(t, os.Remove(filepath.Join(taskDir, "state.json")))
			case "stale revision":
				request.Revision++
			case "invalid proof":
				t.Setenv("FLYWHEEL_PLANNING_USER_ACTION", "{}")
			case "config":
				writeFileCli(t, filepath.Join(taskDir, "flywheel.config.json"), "{invalid")
			case "lease owner":
				release, err := machine.AcquireRunLease(taskDir)
				require.NoError(t, err)
				defer release()
			}
			reply, err := planningCLIControl(t, taskDir, cwd, request)
			require.Error(t, err, scenario)
			require.False(t, reply.OK)
			require.NoFileExists(t, filepath.Join(taskDir, "orchestrator.log"))
		})
	}
	t.Run("prepare failure before state exists", func(t *testing.T) {
		directory := t.TempDir()
		reply, err := planningCLIControl(t, filepath.Join(directory, "new-task"), directory,
			machine.PlanningCommand{Action: "prepare", Brief: &planning.Brief{Version: 1, ID: "no-baseline", Goal: "Inspect", OriginalRequest: "Inspect", Policy: "review-first"}, Start: new(false)})
		require.Error(t, err)
		require.Nil(t, reply.State)
	})
}

func TestPlanningCLIStartsOnlyTheRequestedStageWithExactArgv(t *testing.T) {
	for _, action := range []string{"prepare", "plan", "feedback", "approve"} {
		t.Run(action, func(t *testing.T) {
			controlFixture(t, "never-ready") // records argv and exits, without a model or state mutation
			taskDir, cwd, st := planningCLIFixture(t, true)
			request := machine.PlanningCommand{Action: action, Start: new(true)}
			switch action {
			case "prepare":
				request.Brief, request.Start = &st.Planning.Brief, nil // default starts design, not implementation
			case "plan":
				request.Revision, request.RequestID, request.Scope = st.Revision, "design-next", "design"
			case "feedback":
				request.Revision, request.RequestID, request.Text = st.Revision, "feedback-and-plan", "Review the recovery constraint."
			case "approve":
				request = planningCLIApproval(st, "approve-and-start", true)
				planningCLIProof(t, request)
			}
			reply, err := planningCLIControl(t, taskDir, cwd, request)
			require.NoError(t, err, reply.Error)
			raw, err := os.ReadFile(filepath.Join(taskDir, "fixture-args.json"))
			require.NoError(t, err)
			var args []string
			require.NoError(t, json.Unmarshal(raw, &args))
			want := []string{"run", taskDir}
			if action == "plan" || action == "feedback" {
				want = append(want, "--planning-only")
			}
			require.Equal(t, want, args)
			require.NotNil(t, reply.PlanningControl)
		})
	}
	t.Run("spawn fails after durable preparation", func(t *testing.T) {
		taskDir, cwd, st := planningCLIFixture(t, true)
		original := controlExecutable
		t.Cleanup(func() { controlExecutable = original })
		controlExecutable = func() (string, error) { return "", errors.New("executable unavailable") }
		reply, err := planningCLIControl(t, taskDir, cwd, machine.PlanningCommand{Action: "plan", Revision: st.Revision, RequestID: "queued-plan", Scope: "design"})
		require.Error(t, err)
		require.Contains(t, reply.Error, "executable unavailable")
		require.Equal(t, "applied", reply.PlanningControl.Status, "failed launch does not erase the durable planning request")
	})
}

func TestPlanningOnlyCLIRejectsLegacyAndMixedImplementationFlags(t *testing.T) {
	legacy := newCLISandbox(t)
	for _, flags := range [][]string{{"--only", "execute"}, {"--packet", "M01-T01"}, {"--unit", "M01-T01"}, {"--pause-before", "review"}, {"--session-request", "request.json"}, {"--investigate", "--expect-revision", "1"}} {
		code, output := runCLI(t, append([]string{"run", legacy, "--planning-only"}, flags...)...)
		require.NotZero(t, code, output)
		if flags[0] == "--investigate" {
			require.Contains(t, output, "--investigate requires")
		} else {
			require.Contains(t, output, "planning-only")
		}
	}
	code, output := runCLI(t, "run", legacy, "--planning-only")
	require.NotZero(t, code, output)
	writeFileCli(t, filepath.Join(legacy, "state.json"), cliStateFixture)
	code, output = runCLI(t, "run", legacy, "--planning-only")
	require.NotZero(t, code, output)
	require.Contains(t, output, "migrate a legacy task explicitly")
	taskDir, cwd, st := planningCLIFixture(t, true)
	chdirT(t, cwd)
	code, output = runCLI(t, "run", taskDir, "--planning-only")
	require.Zero(t, code, output)
	require.Contains(t, output, "awaiting-design-approval")
	stored, err := state.Load(taskDir)
	require.NoError(t, err)
	require.Equal(t, st.Planning.CurrentDesign, stored.Planning.CurrentDesign)
	require.Equal(t, st.Budget.Totals, stored.Budget.Totals, "already reviewed design does not redispatch on a planning-only resume")
	require.NoFileExists(t, filepath.Join(taskDir, "spec.md"))
}

func TestPlanningCLIReportsInheritedBindingSourceWithoutChangingConfiguration(t *testing.T) {
	taskDir, cwd, _ := planningCLIFixture(t, true)
	before, err := os.ReadFile(filepath.Join(taskDir, "flywheel.config.json"))
	require.NoError(t, err)
	view, err := diagnostic.InspectContext(t.Context(), cwd, taskDir)
	require.NoError(t, err)
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	printConfiguration(cmd, view)
	require.Contains(t, out.String(), "绑定来源")
	require.Contains(t, out.String(), "loops.design-plan")
	for _, binding := range view.Bindings {
		if binding.ID == "loops.design-plan" {
			require.Equal(t, "loops.plan", binding.BindingSource)
		}
	}
	after, err := os.ReadFile(filepath.Join(taskDir, "flywheel.config.json"))
	require.NoError(t, err)
	require.Equal(t, before, after)
	_, err = config.LoadReadOnly(taskDir, cwd)
	require.NoError(t, err)
}
