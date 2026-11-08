package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionStatusShowsBlockedFactsWithoutReadingBackendReceipts(t *testing.T) {
	taskDir := newCLISandbox(t)
	root := filepath.Dir(filepath.Dir(taskDir))
	writeFileCli(t, state.StateFile(taskDir), cliStateFixture)
	st, err := state.Load(taskDir)
	require.NoError(t, err)
	digest := strings.Repeat("a", 64)
	st.Sessions = &state.SessionLedger{Version: 1, Records: map[string]*state.SessionRecord{
		"review/M01-T01": {
			Task: taskDir, Role: "review", Scope: "M01-T01", Cwd: root,
			Binding: digest, Goal: digest, Contract: digest, Generation: 2,
			Action: "wait", Reason: "outcome-unknown", ResumeCandidate: false,
			Last: &state.SessionInvocation{
				ID: "call-review2", Status: "blocked", StartedAt: "2026-11-07T00:00:00Z", Attempt: 2,
				CandidateBefore: digest, Persistent: true, Dispatch: "dispatched", Outcome: "unknown",
				CleanupConfirmed: true, UsageStatus: "unknown", Accounted: true,
			},
		},
		"execute/M01-T01": {
			Task: taskDir, Role: "execute", Scope: "M01-T01", Cwd: root,
			Binding: digest, Goal: digest, Contract: digest, Generation: 1,
			Action: "resume", Reason: "same-role-scope", ResumeCandidate: true,
			ThreadID: "private-backend-history", Protocol: "test/session-v2",
			Last: &state.SessionInvocation{
				ID: "call-execute1", Status: "consumed", StartedAt: "2026-11-07T00:00:00Z", Attempt: 1,
				CandidateBefore: digest, CandidateAfter: digest, ReceiptDigest: digest,
				Persistent: true, Dispatch: "dispatched", Outcome: "completed",
				CleanupConfirmed: true, UsageStatus: "complete", Accounted: true,
			},
		},
	}}
	_, err = state.Save(taskDir, st, nil)
	require.NoError(t, err)
	// This cannot be opened as a receipt directory. Status still reads only state.
	writeFileCli(t, filepath.Join(taskDir, ".sessions"), "private raw receipt that must not be opened")
	before, err := os.ReadFile(state.StateFile(taskDir))
	require.NoError(t, err)
	code, human := runCLI(t, "status", taskDir)
	require.Zero(t, code, human)
	assert.Contains(t, human, "会话记录: v1")
	assert.Contains(t, human, "历史恢复已验证")
	assert.Contains(t, human, "review/M01-T01: generation=2 action=wait reason=outcome-unknown resume-candidate=false")
	assert.Contains(t, human, "latest=blocked persistent=true dispatch=dispatched outcome=unknown cleanup=true usage=unknown accounted=true")
	assert.NotContains(t, human, "private-backend-history")
	assert.NotContains(t, human, "private raw receipt")
	assert.Less(t, strings.Index(human, "execute/M01-T01"), strings.Index(human, "review/M01-T01"))
	code, reply := runControlTest(t, "status", taskDir, root)
	require.Zero(t, code, "%+v", reply)
	require.Len(t, reply.Sessions, 2)
	assert.Equal(t, "execute", reply.Sessions[0].Role)
	assert.Equal(t, "blocked", reply.Sessions[1].Last.Status)
	assert.True(t, reply.Sessions[1].Last.CleanupConfirmed, "cleanup alone does not imply a known outcome")
	assert.Equal(t, "unknown", reply.Sessions[1].Last.UsageStatus)
	assert.False(t, reply.Sessions[1].ResumeCandidate)
	require.NotNil(t, reply.State.Sessions, "the original typed state response remains available")
	raw, err := json.Marshal(reply.Sessions)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "private-backend-history")
	assert.NotContains(t, string(raw), "receipt")
	after, err := os.ReadFile(state.StateFile(taskDir))
	require.NoError(t, err)
	assert.Equal(t, before, after, "status must not reconcile or account an invocation")
}

func TestSessionStatusDoesNotInventHistoryForLegacyTasks(t *testing.T) {
	assert.Nil(t, sessionSummaries(nil))
	taskDir := newCLISandbox(t)
	writeFileCli(t, state.StateFile(taskDir), cliStateFixture)
	st, err := state.Load(taskDir)
	require.NoError(t, err)
	for _, ledger := range []*state.SessionLedger{nil, {Version: 1, Records: map[string]*state.SessionRecord{}}} {
		st.Sessions = ledger
		st, err = state.Save(taskDir, st, nil)
		require.NoError(t, err)
		var human bytes.Buffer
		printSessionSummaries(&human, st)
		assert.Empty(t, human.String())
		reply, err := controlSnapshot(taskDir)
		require.NoError(t, err)
		assert.Empty(t, reply.Sessions)
	}
}
