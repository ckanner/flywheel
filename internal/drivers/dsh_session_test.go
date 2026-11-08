package drivers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scopedDSHRequest() dshStart {
	return dshStart{Protocol: 2, RequestID: "request-one", Persistent: true, Cwd: "/repo", Provider: "deepseek-account",
		Model: "deepseek-v4-flash", ReasoningEffort: "high", PermissionMode: "workspace-write"}
}

func TestDSHScopedProtocolRejectsHistoryAndLifecycleContradictions(t *testing.T) {
	request := scopedDSHRequest()
	for _, change := range []struct {
		name string
		edit func(map[string]any, map[string]any, map[string]any)
	}{
		{"missing persistence echo", func(r, _, _ map[string]any) { delete(r, "persistent") }},
		{"ephemeral echo", func(r, _, _ map[string]any) { r["persistent"] = false }},
		{"missing resume echo", func(r, _, _ map[string]any) { delete(r, "resumed") }},
		{"unexpected resume", func(r, _, _ map[string]any) { r["resumed"] = true }},
		{"missing lifecycle", func(_, e, _ map[string]any) { delete(e, "lifecycle") }},
		{"wrong lifecycle protocol", func(_, _, s map[string]any) { s["protocol"] = "future-v99" }},
		{"wrong request identity", func(_, _, s map[string]any) { s["requestId"] = "foreign" }},
		{"wrong session identity", func(_, _, s map[string]any) { s["sessionId"] = "foreign" }},
		{"cleanup contradiction", func(_, _, s map[string]any) { s["cleanupConfirmed"] = false }},
		{"unexpected accepted resume", func(_, _, s map[string]any) { s["resumeAccepted"] = true }},
		{"unknown dispatch enum", func(_, _, s map[string]any) { s["dispatch"] = "possibly" }},
		{"unknown outcome enum", func(_, _, s map[string]any) { s["outcome"] = "possibly" }},
		{"unknown usage enum", func(_, _, s map[string]any) { s["usageStatus"] = "cumulative" }},
		{"outcome without dispatch", func(_, _, s map[string]any) { s["dispatch"] = "unknown" }},
		{"completed without dispatch", func(_, _, s map[string]any) { s["dispatch"] = "not-dispatched" }},
		{"known turn without identity", func(_, _, s map[string]any) { s["turnId"] = "" }},
		{"success with unknown outcome", func(_, _, s map[string]any) { s["outcome"] = "unknown" }},
		{"candidate with failed native turn", func(_, e, s map[string]any) { e["status"] = "error"; s["outcome"] = "failed" }},
		{"candidate despite unknown cleanup", func(_, e, s map[string]any) {
			e["status"] = "error"
			e["settled"] = false
			s["cleanupConfirmed"] = false
		}},
		{"candidate despite cancellation", func(_, e, _ map[string]any) { e["status"] = "cancelled" }},
		{"incomplete usage", func(_, e, _ map[string]any) { e["usage"] = map[string]any{"input_tokens": 1} }},
		{"claimed complete unknown usage", func(_, _, s map[string]any) { s["usageStatus"] = "complete" }},
		{"negative cost in v2", func(_, e, _ map[string]any) { e["costUsd"] = -0.1 }},
	} {
		t.Run(change.name, func(t *testing.T) {
			r, end := dshFixtureFrames(request)
			change.edit(r, end, end["lifecycle"].(map[string]any))
			parsed := parseDSHStream(dshJSONL(r, end), request)
			assert.False(t, parsed.Settled)
			assert.NotEmpty(t, parsed.Error)
			assert.Nil(t, parsed.Session, "untrusted lifecycle must never become a resumable handle")
		})
	}
	request.SessionID = "exact-owned-session"
	r, end := dshFixtureFrames(request)
	parsed := parseDSHStream(dshJSONL(r, end), request)
	require.True(t, parsed.Settled, "%+v", parsed)
	assert.Equal(t, request.SessionID, parsed.SessionID)
	assert.True(t, parsed.Session.ResumeAccepted)
	r["sessionId"] = "different-session"
	assert.Nil(t, parseDSHStream(dshJSONL(r, end), request).Session)

	// A restore rejection does not distinguish nonexistent history from an
	// occupied writer. Preserve that ambiguity even though no prompt was sent.
	r, end = dshFixtureFrames(request)
	end["status"] = "error"
	end["error"] = map[string]string{"code": "TRANSPORT", "message": "resume rejected"}
	s := end["lifecycle"].(map[string]any)
	s["dispatch"], s["outcome"], s["resumeCandidate"], s["resumeAccepted"], s["turnId"] = "not-dispatched", "unknown", false, false, ""
	parsed = parseDSHStream(dshJSONL(end), request)
	require.NotNil(t, parsed.Session)
	assert.Equal(t, "unknown", parsed.Session.Outcome)
	assert.True(t, parsed.Session.CleanupConfirmed)

	// Even on a failed call an accepted resume cannot name a different history.
	s["resumeAccepted"], s["sessionId"] = true, "different-session"
	end["sessionId"] = "different-session"
	assert.Nil(t, parseDSHStream(dshJSONL(end), request).Session)
	// A known model terminal always needs the pre-prompt effective settings receipt.
	request.SessionID = ""
	_, end = dshFixtureFrames(request)
	assert.Nil(t, parseDSHStream(dshJSONL(end), request).Session)
}

func TestRunDSHScopedCreateResumeAndOutputRepair(t *testing.T) {
	for _, scenario := range []string{"ok", "unknown-usage", "scoped-empty"} {
		t.Run(scenario, func(t *testing.T) {
			a := dshRunArgs(t, scenario)
			a.PersistentSession, a.RequestID, a.Prompt = true, "first-invocation", "record input"
			for _, id := range []string{"", "sdk-session-one"} {
				a.SessionID = id
				r := RunDSH(a)
				require.NotNil(t, r.Session, "%+v", r)
				assert.Equal(t, "dispatched", r.Session.Dispatch)
				assert.Equal(t, "completed", r.Session.Outcome)
				assert.True(t, r.Session.CleanupConfirmed)
				assert.True(t, r.Session.ResumeCandidate, "output format is independent of the native terminal")
				assert.Equal(t, id != "", r.Session.ResumeAccepted)
				assert.Equal(t, "sdk-session-one", r.Session.SessionID)
				assert.Equal(t, scenario != "scoped-empty", r.Settled)
				if scenario == "unknown-usage" {
					assert.Nil(t, r.Usage)
					assert.Equal(t, "unknown", r.Session.UsageStatus)
				} else {
					require.NotNil(t, r.Usage)
					assert.Equal(t, int64(15), r.Usage.TotalTokens)
					assert.Equal(t, "complete", r.Session.UsageStatus)
				}
				raw, err := os.ReadFile(filepath.Join(a.Cwd, "worker-input.json"))
				require.NoError(t, err)
				var captured struct{ Request dshStart }
				require.NoError(t, json.Unmarshal(raw, &captured))
				assert.Equal(t, 2, captured.Request.Protocol)
				assert.Equal(t, a.RequestID, captured.Request.RequestID)
				assert.True(t, captured.Request.Persistent)
				assert.Equal(t, id, captured.Request.SessionID)
				a.RequestID = "second-invocation"
			}
		})
	}
}

func TestRunDSHScopedAdmissionAndUncertainPhysicalOutcomes(t *testing.T) {
	for _, scenario := range []string{"wrong-settings", "missing-terminal", "exit-error", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			a := dshRunArgs(t, scenario)
			a.PersistentSession, a.RequestID = true, "r"
			r := RunDSH(a)
			require.NotNil(t, r.Session)
			assert.Equal(t, "unknown", r.Session.Outcome)
			assert.False(t, r.Session.ResumeCandidate)
		})
	}
	for _, edit := range []func(*DSHArgs){
		func(a *DSHArgs) { a.RequestID = "" },
		func(a *DSHArgs) { a.RequestID = "bad\nrequest" },
		func(a *DSHArgs) { a.Doctor = true },
		func(a *DSHArgs) { a.Cwd = filepath.Join(t.TempDir(), "missing") },
	} {
		a := dshRunArgs(t, "ok")
		a.PersistentSession, a.RequestID = true, "r"
		edit(&a)
		r := RunDSH(a)
		require.NotNil(t, r.Session)
		assert.Equal(t, "not-dispatched", r.Session.Dispatch)
		assert.Equal(t, "failed", r.Session.Outcome)
		assert.True(t, r.Session.CleanupConfirmed)
	}
	a := dshRunArgs(t, "ok")
	a.PersistentSession, a.RequestID, a.Command = true, "r", filepath.Join(t.TempDir(), "no-worker")
	r := RunDSH(a)
	assert.NotEmpty(t, r.SpawnError)
	assert.Equal(t, "not-dispatched", r.Session.Dispatch)
	assert.True(t, r.Session.CleanupConfirmed)
}

func TestRunDSHFreshCallsExposeCorrelatedFactsWithoutChangingV1History(t *testing.T) {
	for _, requestID := range []string{"", "tracked-fresh-invocation"} {
		a := dshRunArgs(t, "ok")
		a.RequestID, a.Prompt = requestID, "record input"
		r := RunDSH(a)
		require.True(t, r.Settled, "%+v", r)
		require.NotNil(t, r.Session)
		assert.Equal(t, DSHFreshSessionProtocol, r.Session.Protocol)
		assert.Equal(t, "dispatched", r.Session.Dispatch)
		assert.Equal(t, "completed", r.Session.Outcome)
		assert.True(t, r.Session.CleanupConfirmed)
		assert.False(t, r.Session.ResumeCandidate, "v1 must not opt into scoped history continuation")
		assert.False(t, r.Session.ResumeAccepted)
		assert.Equal(t, "complete", r.Session.UsageStatus)
		raw, err := os.ReadFile(filepath.Join(a.Cwd, "worker-input.json"))
		require.NoError(t, err)
		var input struct{ Request dshStart }
		require.NoError(t, json.Unmarshal(raw, &input))
		assert.Equal(t, 1, input.Request.Protocol)
		assert.False(t, input.Request.Persistent)
		assert.Empty(t, input.Request.SessionID)
		assert.NotEmpty(t, input.Request.RequestID)
		assert.Equal(t, input.Request.RequestID, r.Session.RequestID)
		if requestID != "" {
			assert.Equal(t, requestID, input.Request.RequestID)
		}
	}
	for _, scenario := range []string{"structured-quota", "exit-error", "missing-terminal"} {
		a := dshRunArgs(t, scenario)
		a.RequestID = "failed-fresh-invocation"
		r := RunDSH(a)
		require.NotNil(t, r.Session)
		assert.Equal(t, a.RequestID, r.Session.RequestID)
		assert.Equal(t, "unknown", r.Session.Dispatch, scenario)
		assert.Equal(t, "unknown", r.Session.Outcome, scenario)
		assert.False(t, r.Session.ResumeCandidate)
		assert.Equal(t, scenario != "missing-terminal", r.Session.CleanupConfirmed)
	}
	for _, requestID := range []string{"bad\nrequest", "   "} {
		a := dshRunArgs(t, "ok")
		a.RequestID, a.Command = requestID, "/must-not-start"
		r := RunDSH(a)
		assert.Equal(t, "not-dispatched", r.Session.Dispatch)
		assert.Equal(t, "failed", r.Session.Outcome)
		assert.True(t, r.Session.CleanupConfirmed)
	}
	a := dshRunArgs(t, "ok")
	a.RequestID, a.Command = "missing-worker", filepath.Join(t.TempDir(), "missing")
	r := RunDSH(a)
	assert.NotEmpty(t, r.SpawnError)
	assert.Equal(t, "not-dispatched", r.Session.Dispatch)
	assert.Equal(t, "failed", r.Session.Outcome)
	assert.True(t, r.Session.CleanupConfirmed)
}

func TestRunDSHRejectedTranscriptCannotChargeItsValidLookingPrefix(t *testing.T) {
	for _, persistent := range []bool{false, true} {
		for _, scenario := range []string{"success-malformed-tail", "success-extra-frame", "success-stderr-overflow"} {
			t.Run(scenario+map[bool]string{false: "/fresh", true: "/scoped"}[persistent], func(t *testing.T) {
				a := dshRunArgs(t, scenario)
				a.PersistentSession, a.RequestID = persistent, "current-charge"
				if scenario == "success-stderr-overflow" {
					previous := watchdog.MaxBuffer
					watchdog.MaxBuffer = 4096
					t.Cleanup(func() { watchdog.MaxBuffer = previous })
				}
				r := RunDSH(a)
				assert.False(t, r.Settled)
				require.NotNil(t, r.Session)
				assert.Equal(t, "unknown", r.Session.Outcome)
				assert.Equal(t, "unknown", r.Session.UsageStatus)
				assert.False(t, r.Session.ResumeCandidate)
				assert.Nil(t, r.Usage)
				assert.Nil(t, r.CostUSD)
				if scenario == "success-stderr-overflow" {
					assert.True(t, r.Truncated)
					assert.Contains(t, r.Stdout, `"status":"ok"`, "stdout contains an apparently complete success receipt")
				}
			})
		}
	}
}

func TestRunDSHUnexpectedPhysicalExitBlocksReplayButRetainsAttributableUsage(t *testing.T) {
	for _, persistent := range []bool{false, true} {
		a := dshRunArgs(t, "success-exit-one")
		a.PersistentSession, a.RequestID = persistent, "completed-receipt-failed-exit"
		r := RunDSH(a)
		assert.False(t, r.Settled)
		require.NotNil(t, r.Session)
		assert.Equal(t, "unknown", r.Session.Outcome)
		assert.False(t, r.Session.ResumeCandidate, "the wrong exit cannot retain a completed lifecycle")
		require.NotNil(t, r.Usage, "intact matching usage remains measured even when replay is unsafe")
		assert.EqualValues(t, 15, r.Usage.TotalTokens)
		assert.Equal(t, "complete", r.Session.UsageStatus)
		require.NotNil(t, r.CostUSD)
	}
}
