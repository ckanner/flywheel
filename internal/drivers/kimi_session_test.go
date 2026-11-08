package drivers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKimiScopedCheckpointAndExplicitResume(t *testing.T) {
	command := kimiScript(t, `printf '%s\n' "$@" > arguments
printf '%s\n' '{"role":"assistant","content":"fixed current candidate","usage":{"input":9999,"output":500,"totalTokens":10499}}'
printf '%s\n' '`+kimiComplete+`'
`)
	cwd := t.TempDir()
	for _, id := range []string{"", "session-one"} {
		r := RunKimi(KimiArgs{Command: command, Prompt: "current constraints and failure", Cwd: cwd,
			PersistentSession: true, RequestID: "invocation-" + id, SessionID: id})
		require.True(t, r.Settled, "%+v", r)
		require.NotNil(t, r.Session)
		assert.Equal(t, "kimi-print/v1", r.Session.Protocol)
		assert.Equal(t, "session-one", r.Session.SessionID)
		assert.Equal(t, "invocation-"+id, r.Session.RequestID)
		assert.Equal(t, "completed", r.Session.Outcome)
		assert.Equal(t, "dispatched", r.Session.Dispatch)
		assert.True(t, r.Session.CleanupConfirmed)
		assert.True(t, r.Session.ResumeCandidate)
		assert.Equal(t, id != "", r.Session.ResumeAccepted)
		assert.Equal(t, "unknown", r.Session.UsageStatus)
		assert.Nil(t, r.Usage, "an undocumented usage-shaped snapshot cannot be charged as a run delta")
		raw, err := os.ReadFile(filepath.Join(cwd, "arguments"))
		require.NoError(t, err)
		assert.Equal(t, id != "", strings.HasPrefix(string(raw), "--session\nsession-one\n"))
		assert.Contains(t, string(raw), "current constraints and failure")
	}
}

func TestKimiScopedUnknownOutcomesNeverBecomeResumeCheckpoints(t *testing.T) {
	for _, tc := range []struct{ name, body, contains string }{
		{"wrong identity", "printf '%s\\n' '" + kimiComplete + "'", "another session"},
		{"broken stream", "printf '%s\\n' 'broken-json' '" + kimiComplete + "'", "malformed stream-json"},
		{"late output", "printf '%s\\n' '" + kimiComplete + "' '{\"role\":\"assistant\",\"content\":\"late\"}'", "after session.resume_hint"},
		{"failed call", "printf '%s\\n' 'rate limit' >&2; exit 1", "rate limit"},
		{"no completion", "printf '%s\\n' '{\"role\":\"assistant\",\"content\":\"partial\"}'", "without session.resume_hint"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := "session-one"
			if tc.name == "wrong identity" {
				id = "different-session"
			}
			r := RunKimi(KimiArgs{Command: kimiScript(t, tc.body), Cwd: t.TempDir(), Prompt: "p",
				PersistentSession: true, RequestID: "r", SessionID: id})
			assert.False(t, r.Settled)
			assert.Empty(t, r.SessionID)
			require.NotNil(t, r.ErrorMessage)
			assert.Contains(t, *r.ErrorMessage, tc.contains)
			require.NotNil(t, r.Session)
			assert.Equal(t, "unknown", r.Session.Outcome)
			assert.False(t, r.Session.ResumeCandidate)
			assert.True(t, r.Session.CleanupConfirmed, "a drained CLI is independent of the unknown remote turn outcome")
		})
	}
}

func TestKimiScopedKnownAdmissionAndUncertainCancellation(t *testing.T) {
	for _, requestID := range []string{"", "bad\nidentity"} {
		r := RunKimi(KimiArgs{Command: "/must-not-spawn", PersistentSession: true, RequestID: requestID})
		require.NotNil(t, r.Session)
		assert.Equal(t, "not-dispatched", r.Session.Dispatch)
		assert.Equal(t, "failed", r.Session.Outcome)
		assert.True(t, r.Session.CleanupConfirmed)
	}
	missing := RunKimi(KimiArgs{Command: filepath.Join(t.TempDir(), "missing"), PersistentSession: true, RequestID: "r"})
	assert.NotEmpty(t, missing.SpawnError)
	assert.Equal(t, "not-dispatched", missing.Session.Dispatch)
	assert.True(t, missing.Session.CleanupConfirmed)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := RunKimi(KimiArgs{Command: "/must-not-spawn", PersistentSession: true, RequestID: "r",
		Watchdog: watchdog.Opts{Context: ctx}})
	assert.True(t, r.Cancelled)
	assert.Equal(t, "not-dispatched", r.Session.Dispatch)
	assert.Equal(t, "failed", r.Session.Outcome)
	assert.True(t, r.Session.CleanupConfirmed)
	require.NotNil(t, r.Usage)
	assert.Zero(t, r.Usage.TotalTokens)
	require.NotNil(t, r.CostUSD)
	assert.Zero(t, *r.CostUSD)
	assert.False(t, r.Session.ResumeCandidate)

	// Once the CLI has started, cancellation without a terminal hint is an
	// uncertain invocation even if the local process group is fully drained.
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	r = RunKimi(KimiArgs{Command: kimiScript(t, `printf '%s\n' '{"role":"assistant","content":"started"}'
sleep 10
`), Cwd: t.TempDir(), Prompt: "p", PersistentSession: true, RequestID: "started",
		Watchdog: watchdog.Opts{Context: ctx, KillGraceMs: 20}, OnEvent: func(string) { cancel() }})
	assert.True(t, r.Cancelled)
	assert.Equal(t, "unknown", r.Session.Outcome)
	assert.True(t, r.Session.CleanupConfirmed)
	assert.False(t, r.Session.ResumeCandidate)
	assert.Nil(t, r.Usage)
	assert.Nil(t, r.CostUSD)
}

func TestKimiTrackedFreshCallsUseStrictStreamAndUnknownUsage(t *testing.T) {
	command := kimiScript(t, `printf '%s\n' '{"role":"assistant","content":"answer","usage":{"input":9,"output":1,"totalTokens":10}}'
printf '%s\n' '`+kimiComplete+`'
`)
	legacy := RunKimi(KimiArgs{Command: command, Cwd: t.TempDir(), Prompt: "p"})
	require.True(t, legacy.Settled)
	require.NotNil(t, legacy.Usage, "untracked external API keeps its previous usage parser")
	r := RunKimi(KimiArgs{Command: command, Cwd: t.TempDir(), Prompt: "p", RequestID: "tracked-fresh"})
	require.True(t, r.Settled)
	require.NotNil(t, r.Session)
	assert.Equal(t, "tracked-fresh", r.Session.RequestID)
	assert.Equal(t, "completed", r.Session.Outcome)
	assert.Equal(t, "dispatched", r.Session.Dispatch)
	assert.True(t, r.Session.CleanupConfirmed)
	assert.False(t, r.Session.ResumeAccepted)
	assert.Equal(t, "unknown", r.Session.UsageStatus)
	assert.Nil(t, r.Usage)

	broken := kimiScript(t, `printf '%s\n' 'broken-json' '`+kimiComplete+`'`)
	r = RunKimi(KimiArgs{Command: broken, Cwd: t.TempDir(), Prompt: "p", RequestID: "tracked-broken"})
	assert.False(t, r.Settled)
	assert.Equal(t, "unknown", r.Session.Outcome)
	assert.True(t, r.Session.CleanupConfirmed)
	assert.False(t, r.Session.ResumeCandidate)
	legacy = RunKimi(KimiArgs{Command: broken, Cwd: t.TempDir(), Prompt: "p"})
	assert.True(t, legacy.Settled, "untracked external calls retain their legacy parser behavior")
	assert.Equal(t, "unknown", legacy.Session.Outcome, "legacy parsing cannot invent a safe replay fact")
	r = RunKimi(KimiArgs{Command: "/must-not-start", RequestID: "bad\nrequest"})
	assert.Equal(t, "not-dispatched", r.Session.Dispatch)
	assert.Equal(t, "failed", r.Session.Outcome)
	assert.True(t, r.Session.CleanupConfirmed)
}
