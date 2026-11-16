package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionControlReceiptNeedsAnAttributableBeforeSnapshot(t *testing.T) {
	const key = "execute/M01-T01"
	for _, action := range []string{"claim", "reconcile", "abandon", "send", "return", "release"} {
		for _, missing := range []string{"state", "ledger", "record", "invocation", "identity"} {
			t.Run(action+"/"+missing, func(t *testing.T) {
				request := &machine.SessionCommand{Action: action, Key: key, Revision: 1}
				if action == "claim" || action == "abandon" {
					request.Reason = "manual repair"
				}
				if action == "send" {
					request.Prompt = "continue the repair"
				}
				if action == "abandon" {
					request.Invocation = "call-before"
				}
				require.NoError(t, request.Validate())
				after := receiptState(key, "call-after", "consumed", "")
				after.Revision = 2
				if action == "claim" || action == "reconcile" {
					after.Handoff = &state.Handoff{Key: key}
				}
				if action == "abandon" {
					after.Sessions.Records[key].Last.Status = "abandoned"
					after.Sessions.Records[key].Last.Abandon = &state.SessionAbandon{Reason: request.Reason}
				}
				before := receiptState(key, "call-before", "consumed", key)
				switch missing {
				case "state":
					before = nil
				case "ledger":
					before.Sessions = nil
				case "record":
					delete(before.Sessions.Records, key)
				case "invocation":
					before.Sessions.Records[key].Last = nil
				case "identity":
					before.Sessions.Records[key].Last.ID = ""
				}
				receipt := sessionControlReceipt(after, before, request)
				assert.False(t, receipt.Applied, "the current postcondition alone cannot prove this command caused a transition")
				assert.Contains(t, receipt.Message, "could not be verified")
				assert.Contains(t, receipt.Message, "may already have executed", "unverified does not mean no side effects occurred")
				assert.Equal(t, after.Revision, receipt.Revision)
				assert.Equal(t, after.Sessions.Records[key].Last.Status, receipt.Status)
				assert.Equal(t, "call-after", receipt.Invocation, "retain independently observed after-state facts")
			})
		}
	}
}

func TestSessionControlPreSnapshotFailureCannotTurnAnUnchangedReceiptIntoSuccess(t *testing.T) {
	taskDir := t.TempDir()
	const key = "execute/M01-T01"
	unchanged := receiptState(key, "call-already-consumed", "consumed", key)
	unchanged.Revision = 1
	originalExec, originalSnapshot := controlExecutable, controlSnapshotFn
	t.Cleanup(func() { controlExecutable, controlSnapshotFn = originalExec, originalSnapshot })
	controlExecutable = func() (string, error) { return "/usr/bin/true", nil }
	reads := 0
	controlSnapshotFn = func(string) (controlResponse, error) {
		reads++
		if reads == 1 {
			return controlResponse{}, errors.New("transient pre-command snapshot failure")
		}
		return controlResponse{State: unchanged}, nil
	}
	cmd := controlCmd()
	cmd.SetIn(strings.NewReader(`{"action":"reconcile","key":"execute/M01-T01","revision":1}`))
	reply, err := startSessionControl(cmd, taskDir, taskDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, reads, 3, "the subsequent admission and terminal snapshots recovered")
	require.NotNil(t, reply.SessionControl)
	assert.False(t, reply.SessionControl.Applied)
	assert.Contains(t, reply.SessionControl.Message, "could not be verified")
	assert.Contains(t, reply.SessionControl.Message, "may already have executed")
	assert.Equal(t, "call-already-consumed", reply.SessionControl.Invocation)
}

func TestSessionControlReleaseReceiptKeepsKnownNoOpDistinctFromUnknownEvidence(t *testing.T) {
	const key = "execute/M01-T01"
	for _, handoff := range []string{"", key} {
		before := receiptState(key, "call-one", "consumed", handoff)
		after := receiptState(key, "call-one", "consumed", handoff)
		receipt := sessionControlReceipt(after, before, &machine.SessionCommand{Action: "release", Key: key, Revision: 1})
		assert.False(t, receipt.Applied, "an absent or unchanged handoff is not a release transition")
		assert.Contains(t, receipt.Message, "the session was not under this handoff or remains controlled")
		assert.NotContains(t, receipt.Message, "could not be verified", "complete before-state distinguishes a known no-op from missing evidence")
	}
}
