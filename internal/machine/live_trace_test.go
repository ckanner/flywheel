package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveTraceWriterStreamsBoundedFramesAndHandlesGuards(t *testing.T) {
	now := func() string { return "2026-11-23T12:00:00Z" }

	// Guard: missing taskDir / record / invocation id returns nil and nil receiver is safe.
	var nilWriter *liveTraceWriter
	nilWriter.writeFrame(map[string]any{"kind": "noop"})
	nilWriter.event("line")
	nilWriter.finish(drivers.Result{})
	assert.Nil(t, openLiveTrace("", "execute/M01-T01", "execute", "M01-T01", nil, drivers.CallArgs{}, now))

	// Guard: unwritable parent path (regular file where .sessions dir should be) fails safely.
	blocker := filepath.Join(t.TempDir(), "blocked-task")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	rec := &state.SessionRecord{Action: "create", Last: &state.SessionInvocation{ID: "call-err", Attempt: 1}}
	assert.Nil(t, openLiveTrace(blocker, "execute/M01-T01", "execute", "M01-T01", rec, drivers.CallArgs{}, now))

	// Guard: target live file path blocked by a directory fails safely.
	dirBlocked := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dirBlocked, ".traces", "live-call-dir.jsonl"), 0o700))
	assert.Nil(t, openLiveTrace(dirBlocked, "execute/M01-T01", "execute", "M01-T01", &state.SessionRecord{
		Action: "create",
		Last:   &state.SessionInvocation{ID: "call-dir"},
	}, drivers.CallArgs{}, now))

	// Happy path + truncation + error/session fallback + post-finish no-op.
	oldMax := maxLiveTraceBytes
	maxLiveTraceBytes = 600
	t.Cleanup(func() { maxLiveTraceBytes = oldMax })

	taskDir := t.TempDir()
	recOK := &state.SessionRecord{
		Action: "resume",
		Last:   &state.SessionInvocation{ID: "call-1", Attempt: 2, RequestedThread: "thread-prev"},
	}
	args := drivers.CallArgs{
		Cwd:          "/repo",
		WorkspaceCwd: "/repo",
		Model:        "gpt-6.2-astra",
		SessionTitle: "Flywheel · execute/M01-T01 · live trace test",
		Binding:      config.LoopBinding{Harness: "codex-pro", Channel: "chatgpt-subscription", Thinking: "high"},
	}
	w := openLiveTrace(taskDir, "execute/M01-T01", "execute", "M01-T01", recOK, args, now)
	require.NotNil(t, w)

	w.event("   ") // ignored empty line
	w.event(`{"type":"thinking","text":"analyzing bug"}`)
	w.event(strings.Repeat("x", 800)) // triggers truncation frame
	w.event(`{"type":"after-truncation"}`)

	code := 1
	errText := "process failed"
	w.finish(drivers.Result{
		ExitCode:     &code,
		Harness:      "codex-pro",
		Model:        "gpt-6.2-astra",
		ErrorCode:    errs.TIMEOUT,
		ErrorMessage: &errText,
		Text:         strings.Repeat("a", 20),
		Stderr:       strings.Repeat("e", 17000),
		Session:      &drivers.SessionOutcome{SessionID: "thread-live-99"},
	})
	// Subsequent calls after close are safe no-ops.
	w.writeFrame(map[string]any{"kind": "late"})
	w.event("late-event")
	w.finish(drivers.Result{})

	raw, err := os.ReadFile(filepath.Join(taskDir, ".traces", "live-call-1.jsonl"))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	require.Len(t, lines, 4) // start, 1 event, truncated, end

	var startFrame, eventFrame, truncFrame, endFrame map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &startFrame))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &eventFrame))
	require.NoError(t, json.Unmarshal([]byte(lines[2]), &truncFrame))
	require.NoError(t, json.Unmarshal([]byte(lines[3]), &endFrame))

	assert.Equal(t, "start", startFrame["kind"])
	assert.Equal(t, "call-1", startFrame["invocationId"])
	assert.Equal(t, "Flywheel · execute/M01-T01 · live trace test", startFrame["sessionTitle"])
	assert.Equal(t, "event", eventFrame["kind"])
	assert.Equal(t, "truncated", truncFrame["kind"])
	assert.Equal(t, "end", endFrame["kind"])
	assert.Equal(t, "thread-live-99", endFrame["sessionId"])
	assert.Equal(t, "process failed", endFrame["errorMessage"])
	assert.True(t, strings.HasPrefix(endFrame["stderr"].(string), "… [truncated]\n"))

	// Managed .ignore is created and excludes volatile runtime artifacts while
	// preserving user-authored .ignore files and refreshing outdated managed ones.
	ignorePath := filepath.Join(taskDir, ".ignore")
	ignoreRaw, err := os.ReadFile(ignorePath)
	require.NoError(t, err)
	assert.Equal(t, taskIgnoreBody, string(ignoreRaw))
	// Idempotent when already current.
	ensureTaskIgnore(taskDir)
	// Refresh when managed header is present with outdated content.
	require.NoError(t, os.WriteFile(ignorePath, []byte(taskIgnoreHeader+".traces/\n"), 0o644))
	ensureTaskIgnore(taskDir)
	ignoreRaw, err = os.ReadFile(ignorePath)
	require.NoError(t, err)
	assert.Equal(t, taskIgnoreBody, string(ignoreRaw))
	// Preserve custom user-authored .ignore untouched.
	customDir := t.TempDir()
	customBody := "custom-dir/\n"
	require.NoError(t, os.WriteFile(filepath.Join(customDir, ".ignore"), []byte(customBody), 0o644))
	ensureTaskIgnore(customDir)
	customRead, err := os.ReadFile(filepath.Join(customDir, ".ignore"))
	require.NoError(t, err)
	assert.Equal(t, customBody, string(customRead))
}

func TestSessionCallForwardsOnEventToLiveTraceAndCaller(t *testing.T) {
	fx, c := sessionFixture(t)
	a := sessionArgs(c)
	var received []string
	a.OnEvent = func(line string) { received = append(received, line) }
	r, err := c.invokeScoped("execute", a.PacketID, a, func(callArgs drivers.CallArgs) drivers.Result {
		require.NotNil(t, callArgs.OnEvent)
		callArgs.OnEvent(`{"type":"thinking","text":"tracing live"}`)
		return sessionResult(callArgs, drivers.MockResult(nil))
	})
	require.NoError(t, err)
	assert.Equal(t, []string{`{"type":"thinking","text":"tracing live"}`}, received)
	invID := c.sessionRecord("execute", a.PacketID).Last.ID
	raw, err := os.ReadFile(filepath.Join(fx.taskDir, ".traces", "live-"+invID+".jsonl"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "tracing live")
	assert.NotNil(t, r.Session)
}
