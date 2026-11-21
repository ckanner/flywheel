package machine

import (
	"errors"
	"testing"

	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestCollaborationReceiptRetainsItsActionAcrossLostResponseAndRejectsPlanImpersonation(t *testing.T) {
	_, _, opts, request := collaborationEditFixture(t)
	st, _, err := ControlPlanning(opts, request, "")
	require.NoError(t, err)
	read, receipt, err := ControlPlanning(opts, PlanningCommand{Action: "status", RequestID: request.RequestID}, "")
	require.NoError(t, err)
	require.Equal(t, "collaboration", receipt.Action)
	require.Equal(t, "applied", receipt.Status)
	require.Equal(t, request.RequestID, receipt.RequestID)
	require.Equal(t, st.Revision, read.Revision)
	_, _, err = ControlPlanning(opts, PlanningCommand{Action: "plan", Revision: st.Revision, RequestID: request.RequestID, Scope: "packet", ScopeID: request.Collaboration.Packet, Text: collaborationRequestIdentity(request.Collaboration)}, "")
	require.ErrorContains(t, err, "already used for different")
}

func TestCollaborationRejectionIsDurableTerminalAndNeverChangesPlanAuthority(t *testing.T) {
	_, c, opts, request := collaborationEditFixture(t)
	request.Collaboration.Value.Tasks[1].Writes = request.Collaboration.Value.Tasks[0].Writes
	before := jsonClone(c.State)
	st, receipt, err := ControlPlanning(opts, request, "")
	require.NoError(t, err)
	require.Equal(t, "rejected", receipt.Status)
	require.Equal(t, "collaboration", receipt.Action)
	require.Contains(t, receipt.Message, "overlaps")
	require.Equal(t, before.Planning, st.Planning, "refusal cannot append a plan, request, approval or review")
	require.Equal(t, before.Packets, st.Packets)
	require.Equal(t, before.Revision+1, st.Revision)
	require.Len(t, st.Inbox, len(before.Inbox)+1)
	require.Equal(t, sessionHash(request), st.Inbox[len(st.Inbox)-1].(map[string]any)["command-digest"])
	stored, err := state.Load(opts.TaskDir)
	require.NoError(t, err)
	require.Equal(t, st, stored)
	read, observed, err := ControlPlanning(opts, PlanningCommand{Action: "status", RequestID: request.RequestID}, "")
	require.NoError(t, err)
	require.Equal(t, receipt, observed)
	require.Equal(t, st.Revision, read.Revision)
	replayed, observed, err := ControlPlanning(opts, request, "")
	require.NoError(t, err)
	require.Equal(t, receipt, observed)
	require.Equal(t, st.Revision, replayed.Revision)
	reused := request
	reused.Collaboration = jsonClone(request.Collaboration)
	reused.Collaboration.Value.Tasks[0].Goal = "different rejected command"
	_, _, err = ControlPlanning(opts, reused, "")
	require.ErrorContains(t, err, "reused for different content")
	// A fresh, corrected request can proceed; the prior refusal must not lock a
	// valid task forever or silently reuse the rejected request identity.
	request.Revision, request.RequestID = st.Revision, "corrected-collaboration"
	request.Collaboration.Value.Tasks[1].Writes = []string{"build/M01-T01-extra.txt"}
	accepted, observed, err := ControlPlanning(opts, request, "")
	require.NoError(t, err)
	require.Equal(t, "applied", observed.Status)
	require.Len(t, accepted.Planning.Plans, len(before.Planning.Plans)+1)
}

func TestCollaborationRefusalCASFailureDoesNotClaimARejectedOutcome(t *testing.T) {
	_, c, opts, request := collaborationEditFixture(t)
	latest, err := state.Save(opts.TaskDir, c.State, nil)
	require.NoError(t, err)
	_, receipt, err := rejectPlanningCollaboration(opts, c.State, request, errors.New("invalid draft"))
	require.Error(t, err)
	require.Nil(t, receipt)
	stored, err := state.Load(opts.TaskDir)
	require.NoError(t, err)
	require.Equal(t, latest, stored)
	prior := errors.New("ordinary plan refusal")
	_, receipt, err = rejectPlanningCollaboration(opts, c.State, PlanningCommand{Action: "plan"}, prior)
	require.ErrorIs(t, err, prior)
	require.Nil(t, receipt)
	// A legacy planning request with identical free text retains plan identity.
	st := jsonClone(latest)
	require.NoError(t, planning.BeginRequest(st.Planning, planning.Request{ID: "ordinary-plan", Scope: "packet", ScopeID: request.Collaboration.Packet, InputRevision: st.Planning.InputRevision, PlanningOnly: true, Feedback: collaborationRequestIdentity(request.Collaboration)}))
	_, err = planningCommandApplied(st.Planning, PlanningCommand{Action: "collaboration", RequestID: "ordinary-plan", Collaboration: request.Collaboration}, "")
	require.ErrorContains(t, err, "already used for different")
}
