package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
)

var maxLiveTraceBytes = 2 << 20 // 2 MiB bounded live trace stream per invocation

// liveTraceWriter appends real-time start/event/end JSONL frames under
// `<taskDir>/.traces/live-<invocationId>.jsonl` (commit-volatile zone) so UI
// consoles can render streaming model thinking, tool calls, and failure details
// while an invocation is still running, and retain failed attempt histories
// without growing the bounded per-scope `.sessions/` checkpoint slot.
type liveTraceWriter struct {
	mu        sync.Mutex
	file      *os.File
	written   int
	truncated bool
	now       func() string
}

func openLiveTrace(taskDir, key, role, scope string, r *state.SessionRecord, args drivers.CallArgs, now func() string) *liveTraceWriter {
	if taskDir == "" || r == nil || r.Last == nil || r.Last.ID == "" {
		return nil
	}
	dir := filepath.Join(taskDir, ".traces")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil
	}
	path := filepath.Join(dir, "live-"+r.Last.ID+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil
	}
	w := &liveTraceWriter{file: f, now: now}
	w.writeFrame(map[string]any{
		"kind":            "start",
		"ts":              w.now(),
		"invocationId":    r.Last.ID,
		"key":             key,
		"role":            role,
		"scope":           scope,
		"attempt":         r.Last.Attempt,
		"action":          r.Action,
		"harness":         args.Binding.Harness,
		"provider":        args.Binding.Provider,
		"channel":         args.Binding.Channel,
		"model":           args.Model,
		"thinking":        args.Binding.Thinking,
		"cwd":             args.Cwd,
		"workspaceCwd":    args.WorkspaceCwd,
		"sessionTitle":    args.SessionTitle,
		"requestedThread": r.Last.RequestedThread,
		"promptBytes":     len(args.Prompt),
	})
	return w
}

func (w *liveTraceWriter) writeFrame(frame map[string]any) {
	if w == nil {
		return
	}
	raw, _ := json.Marshal(frame)
	raw = append(raw, '\n')
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return
	}
	_, _ = w.file.Write(raw)
	w.written += len(raw)
}

func (w *liveTraceWriter) event(line string) {
	if w == nil || strings.TrimSpace(line) == "" {
		return
	}
	w.mu.Lock()
	if w.file == nil || w.truncated {
		w.mu.Unlock()
		return
	}
	if w.written+len(line)+96 > maxLiveTraceBytes {
		w.truncated = true
		raw, _ := json.Marshal(map[string]any{"kind": "truncated", "ts": w.now(), "maxBytes": maxLiveTraceBytes})
		_, _ = w.file.Write(append(raw, '\n'))
		w.mu.Unlock()
		return
	}
	raw, _ := json.Marshal(map[string]any{"kind": "event", "ts": w.now(), "line": line})
	raw = append(raw, '\n')
	_, _ = w.file.Write(raw)
	w.written += len(raw)
	w.mu.Unlock()
}

func (w *liveTraceWriter) finish(result drivers.Result) {
	if w == nil {
		return
	}
	sessionID := result.SessionID
	if sessionID == "" && result.Session != nil {
		sessionID = result.Session.SessionID
	}
	errMsg := ""
	if result.ErrorMessage != nil {
		errMsg = *result.ErrorMessage
	}
	frame := map[string]any{
		"kind":         "end",
		"ts":           w.now(),
		"exitCode":     result.ExitCode,
		"signal":       result.Signal,
		"timedOut":     result.TimedOut,
		"cancelled":    result.Cancelled,
		"durationMs":   result.DurationMS,
		"harness":      result.Harness,
		"model":        result.Model,
		"sessionId":    sessionID,
		"settled":      result.Settled,
		"errorCode":    result.ErrorCode,
		"errorMessage": errMsg,
		"text":         tailRunes(result.Text, 16000),
		"stderr":       tailRunes(result.Stderr, 16000),
		"usage":        result.Usage,
		"truncated":    w.truncated || result.Truncated,
	}
	w.writeFrame(frame)
	w.mu.Lock()
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	w.mu.Unlock()
}

func tailRunes(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return "… [truncated]\n" + string(r[len(r)-maxRunes:])
}
