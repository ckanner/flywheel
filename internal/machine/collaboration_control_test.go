package machine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func collaborationEditFixture(t *testing.T) (fixture, *Context, RunOpts, PlanningCommand) {
	t.Helper()
	fx, c := collaborationProjectionFixture(t, true)
	c.save()
	require.NoError(t, c.err())
	node, err := ReadNode(c.State, "M01-T01", fx.taskDir, fx.root)
	require.NoError(t, err)
	v := node.CollaborationEditor
	require.True(t, v.Editable, v.Reason)
	edit := &CollaborationEdit{Packet: v.Packet, ContractFingerprint: v.ContractFingerprint, Plan: *v.Plan, Value: *jsonClone(v.Value)}
	edit.Value.Tasks[0].Goal = "Implement the separately reviewed primary output"
	return fx, c, RunOpts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config}, PlanningCommand{Action: "collaboration", Revision: c.State.Revision, RequestID: "edit-collaboration-1", Collaboration: edit, Start: new(true)}
}

func TestCollaborationEditPersistsExactUnreviewedPlanWithoutChangingActiveContract(t *testing.T) {
	fx, c, opts, request := collaborationEditFixture(t)
	oldState := jsonClone(c.State)
	packetPath := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
	oldPacket := mustReadFile(t, packetPath)
	st, receipt, err := ControlPlanning(opts, request, "")
	require.NoError(t, err)
	require.Equal(t, "applied", receipt.Status)
	require.Equal(t, "collaboration", receipt.Action)
	require.Equal(t, oldState.Revision+1, st.Revision)
	require.Equal(t, oldState.Planning.Approvals, st.Planning.Approvals)
	require.Equal(t, oldState.Planning.Reviews, st.Planning.Reviews)
	require.Equal(t, oldState.Planning.Active, st.Planning.Active)
	require.Equal(t, oldPacket, mustReadFile(t, packetPath))
	require.Equal(t, oldState.Planning.Plans, st.Planning.Plans[:len(oldState.Planning.Plans)])
	latest := st.Planning.Plans[len(st.Planning.Plans)-1]
	packet, err := contract.ParsePacket([]byte(latest.Files["task-packet-M01-T01.md"]), "new plan")
	require.NoError(t, err)
	require.Equal(t, &request.Collaboration.Value, packet.Collaboration)
	require.Equal(t, "control:collaboration:"+sessionHash(request.RequestID), latest.PlannerInvocation)
	require.Equal(t, "packet", latest.Scope)
	r := st.Planning.Requests[len(st.Planning.Requests)-1]
	require.True(t, r.PlanningOnly)
	require.Equal(t, "collaboration-edit", r.Origin)
	require.Equal(t, "reviewer", r.Stage)
	require.Equal(t, 1, r.Rounds)
	require.Nil(t, planning.LatestReview(st.Planning, "plan", latest.Ref))
	require.Error(t, planning.Admit(st.Planning, *c.Spec.PlanningRef), "unreviewed edited plan must stop implementation")
	stored, err := state.Load(opts.TaskDir)
	require.NoError(t, err)
	require.Equal(t, st, stored)
	read, err := ReadNode(st, request.Collaboration.Packet, fx.taskDir, fx.root)
	require.NoError(t, err)
	require.False(t, read.CollaborationEditor.Editable)
	require.Equal(t, &request.Collaboration.Value, read.CollaborationEditor.Value)
	require.Equal(t, "unknown", read.Eligibility.Status)
	replayed, receipt, err := ControlPlanning(opts, request, "")
	require.NoError(t, err)
	require.Equal(t, "unchanged", receipt.Status)
	require.Equal(t, st.Revision, replayed.Revision)
	changed := request
	changed.Collaboration = jsonClone(request.Collaboration)
	changed.Collaboration.Value.Tasks[0].Goal = "different input"
	_, _, err = ControlPlanning(opts, changed, "")
	require.ErrorContains(t, err, "already used for different")
	// The existing independent review pipeline must be able to activate exactly
	// the new revision without fabricating a second human design approval.
	c.State, c.revision = st, st.Revision
	require.NoError(t, c.checkPlanningExports(true))
	require.NoError(t, planning.RecordReview(st.Planning, planning.Review{ID: "manual-plan-review", Kind: "plan", Target: latest.Ref, Design: st.Planning.CurrentDesign,
		InputRevision: st.Planning.InputRevision, Invocation: "independent-manual-review", Role: "plan-review", Binding: "independent", Verdict: "pass", ScopeVerdict: "within-envelope", Summary: "The exact manual split is independent and in scope.", Findings: []planning.Finding{}}))
	st.Planning.Requests[len(st.Planning.Requests)-1].Status = "completed"
	st.Planning.Requests[len(st.Planning.Requests)-1].Stage = "finished"
	require.NoError(t, c.checkPlanningExports(true))
	require.NoError(t, c.activatePlanningPlans())
	require.NoError(t, c.reloadPackets())
	require.Equal(t, &request.Collaboration.Value, c.Packets["M01-T01"].Collaboration)
	require.Equal(t, oldState.Planning.Approvals, c.State.Planning.Approvals)
}

func TestCollaborationEditRejectsStaleUnsafeOrUnchangedInputWithoutChangingPlansOrContracts(t *testing.T) {
	for _, scenario := range []string{"revision", "fingerprint", "plan", "overlap", "outside", "unchanged", "completed", "pending-request", "handoff", "missing-scopes", "changed-packet", "changed-other-packet", "changed-export"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, opts, request := collaborationEditFixture(t)
			switch scenario {
			case "revision":
				request.Revision--
			case "fingerprint":
				request.Collaboration.ContractFingerprint = strings.Repeat("a", 64)
			case "plan":
				request.Collaboration.Plan.Revision++
			case "overlap":
				request.Collaboration.Value.Tasks[1].Writes = request.Collaboration.Value.Tasks[0].Writes
			case "outside":
				request.Collaboration.Value.Tasks[1].Writes = []string{"outside.txt"}
			case "unchanged":
				request.Collaboration.Value = *jsonClone(c.Packets["M01-T01"].Collaboration)
			case "completed":
				c.State.Packets["M01-T01"].Status = "completed"
			case "pending-request":
				require.NoError(t, planning.BeginRequest(c.State.Planning, planning.Request{ID: "pending", Scope: "packet", ScopeID: "M01-T01", InputRevision: 1, PlanningOnly: true}))
			case "handoff":
				c.State.Handoff = &state.Handoff{Key: "execute/M01-T01"}
			case "missing-scopes":
				c.State.Planning = nil
			case "changed-packet":
				writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), string(mustReadFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md")))+"\nChanged\n")
			case "changed-other-packet":
				writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T02.md"), string(mustReadFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T02.md")))+"\nChanged sibling\n")
			case "changed-export":
				writeFile(t, filepath.Join(fx.taskDir, "planning", "plan-000002.json"), "changed")
			}
			before := mustReadFile(t, state.StateFile(opts.TaskDir))
			original := jsonClone(c.State)
			contracts := map[string][]byte{}
			for _, name := range []string{"spec.md", "task-packet-M01-T01.md", "task-packet-M01-T02.md", "planning/plan-000002.json"} {
				contracts[name] = mustReadFile(t, filepath.Join(fx.taskDir, name))
			}
			if scenario == "completed" || scenario == "pending-request" || scenario == "handoff" || scenario == "missing-scopes" {
				require.Error(t, applyCollaborationEdit(opts, c.State, request))
				require.Equal(t, original, c.State, "private validation failure cannot mutate caller state")
				require.Equal(t, before, mustReadFile(t, state.StateFile(opts.TaskDir)))
			} else {
				saved, receipt, err := ControlPlanning(opts, request, "")
				require.NoError(t, err)
				require.Equal(t, "rejected", receipt.Status)
				require.Equal(t, "collaboration", receipt.Action)
				require.Equal(t, original.Revision+1, saved.Revision)
				require.Len(t, saved.Inbox, len(original.Inbox)+1)
				stored, err := state.Load(opts.TaskDir)
				require.NoError(t, err)
				require.Equal(t, saved, stored)
				// Only the confirmed rejection audit may be published. No request,
				// approval, active projection or implementation count has changed.
				saved = jsonClone(saved)
				saved.Revision, saved.Inbox = original.Revision, original.Inbox
				require.Equal(t, original, saved)
			}
			for name, raw := range contracts {
				require.Equal(t, raw, mustReadFile(t, filepath.Join(fx.taskDir, name)))
			}
		})
	}
}

func TestCollaborationEditStagesCompleteProjectionBeforeAdoptingAnyPlan(t *testing.T) {
	for _, scenario := range []string{"malformed-source-packet", "unapproved-goal", "unaffected-packet", "unreadable-repository-inventory", "reused-ordinary-plan-request"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, opts, request := collaborationEditFixture(t)
			previous := *jsonClone(planning.FindPlan(c.State.Planning, request.Collaboration.Plan))
			want := ""
			switch scenario {
			case "malformed-source-packet":
				previous.Files["task-packet-M01-T01.md"] = "---\ninvalid: [\n---\n"
				want = "yaml"
			case "unapproved-goal":
				require.Contains(t, previous.Files["spec.md"], c.State.Planning.Brief.Goal)
				previous.Files["spec.md"] = strings.Replace(previous.Files["spec.md"], c.State.Planning.Brief.Goal, "Unapproved replacement goal", 1)
				want = "approved goal"
			case "unaffected-packet":
				previous.Files["task-packet-M01-T02.md"] += "\nUnreviewed sibling instruction\n"
				want = "unaffected/completed packet M01-T02"
			case "unreadable-repository-inventory":
				writeFile(t, filepath.Join(fx.root, ".git", "index"), "corrupted index")
				want = "candidate file inventory"
			case "reused-ordinary-plan-request":
				require.NoError(t, planning.BeginRequest(c.State.Planning, planning.Request{ID: request.RequestID, Scope: "packet", ScopeID: request.Collaboration.Packet,
					InputRevision: c.State.Planning.InputRevision, PlanningOnly: true, Feedback: collaborationRequestIdentity(request.Collaboration)}))
				want = "identity was reused"
			}
			before := jsonClone(c.State)
			disk := mustReadFile(t, state.StateFile(opts.TaskDir))
			err := c.stageCollaborationEdit(c.State, previous, request.Collaboration, request.RequestID)
			require.ErrorContains(t, err, want)
			require.Equal(t, before, c.State, "no partial plan or request can escape failed staging")
			require.Equal(t, disk, mustReadFile(t, state.StateFile(opts.TaskDir)))
		})
	}
}

func TestCollaborationEditShapeRejectsCrossActionAndUnboundedPayloads(t *testing.T) {
	_, _, _, request := collaborationEditFixture(t)
	require.NoError(t, ValidatePlanningCommand(request))
	for _, change := range []func(*PlanningCommand){
		func(r *PlanningCommand) { r.Action = "plan"; r.Scope = "packet"; r.ScopeID = "M01-T01" },
		func(r *PlanningCommand) { r.Collaboration = nil },
		func(r *PlanningCommand) { r.Collaboration.Packet = "../outside" },
		func(r *PlanningCommand) { r.Collaboration.ContractFingerprint = "bad" },
		func(r *PlanningCommand) { r.Collaboration.Plan.Revision = 0 },
		func(r *PlanningCommand) { r.Collaboration.Value.Version = 2 },
		func(r *PlanningCommand) { r.Collaboration.Value.Tasks = nil },
		func(r *PlanningCommand) { r.Collaboration.Value.Tasks[0].Goal = strings.Repeat("a", 33<<10) },
	} {
		input := jsonClone(&request)
		change(input)
		require.Error(t, ValidatePlanningCommand(*input))
	}
}

func TestCollaborationEditRunsIndependentReviewerBeforeActivationWithoutImplementation(t *testing.T) {
	_, c, opts, request := collaborationEditFixture(t)
	request.RequestID = strings.Repeat("r", 120)
	st, _, err := ControlPlanning(opts, request, "")
	require.NoError(t, err, "every valid request ID must fit the bounded plan producer identity")
	c.State, c.revision = st, st.Revision
	require.NoError(t, c.checkPlanningExports(true))
	calls := 0
	c.Drivers.PlanReview = &fnDriver{role: "plan-review", fn: func(args drivers.CallArgs) drivers.Result {
		calls++
		require.Contains(t, args.Prompt, request.Collaboration.Value.Tasks[0].Goal)
		return planningJSONResult(t, prompts.DesignReviewOutput{Verdict: "pass", ScopeVerdict: "within-envelope", Summary: "The manual split has disjoint writes and retains the approved acceptance.", Findings: []planning.Finding{}})
	}}
	c.Drivers.Plan = &fnDriver{role: "plan", fn: func(args drivers.CallArgs) drivers.Result {
		t.Fatal("a manual exact split must enter the independent reviewer directly")
		return drivers.Result{}
	}}
	c.Drivers.Execute = &fnDriver{role: "execute", fn: func(args drivers.CallArgs) drivers.Result {
		t.Fatal("saving collaboration must not start implementation")
		return drivers.Result{}
	}}
	index := len(st.Planning.Requests) - 1
	ok, err := c.processPlanningRequest(index)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 1, calls)
	review := planning.LatestReview(c.State.Planning, "plan", *c.State.Planning.CurrentPlan)
	require.NotNil(t, review)
	require.Equal(t, "plan-review", review.Role)
	require.Equal(t, "pass", review.Verdict)
	require.True(t, c.State.Planning.Requests[index].PlanningOnly)
	require.Equal(t, 1, c.State.Planning.Requests[index].Rounds)
}
