package drivers

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/require"
)

type activityWriteCloser struct{ bytes.Buffer }

func (w *activityWriteCloser) Close() error { return nil }

func TestProviderActivityDoesNotAcknowledgeOrAdvanceDurableDispatch(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		request := codexProRequest(t)
		request.Protocol = 3
		m := newMachineInteraction(harness, request, func(MachineLifecycle) error {
			t.Fatal("advisory activity cannot checkpoint a lifecycle transition")
			return nil
		}, nil)
		writer := &activityWriteCloser{}
		m.stdin = io.WriteCloser(writer)
		m.line(dshJSONL(activityFixture(harness, request, 1)))
		require.NoError(t, m.failure())
		require.Zero(t, writer.Len(), "only durable lifecycle events receive an ACK")
		require.False(t, m.terminal)
		m.line(dshJSONL(activityFixture(harness, request, 1)))
		require.Error(t, m.failure(), "a repeated activity sequence is not new progress")
	}
}

func activityFixture(harness string, request codexProStart, sequence int64) map[string]any {
	version := CodexProPackageVersion
	if harness == "claude-code-pro" {
		version = ClaudeCodeProPackageVersion
	}
	return map[string]any{"type": "activity", "protocol": request.Protocol, "requestId": request.RequestID,
		"adapter": map[string]any{"package": "dsh-subagent-" + harness, "version": version, "protocol": 1},
		"activity": map[string]any{"version": 1, "sequence": sequence, "kind": "provider-progress", "source": harness,
			"scope": "child", "eventTypes": []string{providerActivityTypes[harness][0]}, "observedEvents": 1}}
}

func TestProviderActivityCannotActAsAResultOrCrossAnInvocation(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		t.Run(harness, func(t *testing.T) {
			request := codexProRequest(t)
			if harness == "claude-code-pro" {
				request = codexProStart(claudeCodeProRequest(t))
			}
			frame := activityFixture(harness, request, 1)
			var sequence int64
			require.NoError(t, validateProActivityEvent(json.RawMessage(dshJSONL(frame)), harness, request, &sequence))
			require.Equal(t, int64(1), sequence)
			require.Error(t, validateProActivityEvent(json.RawMessage(dshJSONL(frame)), harness, request, &sequence), "a repeated observation is not fresh work")
			for name, change := range map[string]func(map[string]any){
				"wrong request":  func(f map[string]any) { f["requestId"] = "other" },
				"wrong protocol": func(f map[string]any) { f["protocol"] = 7 },
				"wrong adapter":  func(f map[string]any) { f["adapter"].(map[string]any)["version"] = "0.0.0" },
				"raw payload":    func(f map[string]any) { f["activity"].(map[string]any)["thinking"] = "private" },
				"unknown type":   func(f map[string]any) { f["activity"].(map[string]any)["eventTypes"] = []string{"heartbeat"} },
				"duplicate types": func(f map[string]any) {
					f["activity"].(map[string]any)["eventTypes"] = []string{providerActivityTypes[harness][0], providerActivityTypes[harness][0]}
				},
				"bad scope":    func(f map[string]any) { f["activity"].(map[string]any)["scope"] = "unowned" },
				"empty events": func(f map[string]any) { f["activity"].(map[string]any)["eventTypes"] = []string{} },
				"zero count":   func(f map[string]any) { f["activity"].(map[string]any)["observedEvents"] = 0 },
				"too big":      func(f map[string]any) { f["requestId"] = strings.Repeat("x", 5000) },
			} {
				t.Run(name, func(t *testing.T) {
					bad := activityFixture(harness, request, 2)
					change(bad)
					require.Error(t, validateProActivityEvent(json.RawMessage(dshJSONL(bad)), harness, request, &sequence))
					require.Equal(t, int64(1), sequence)
				})
			}
			if harness == "codex-pro" {
				require.False(t, parseCodexProStream(dshJSONL(frame), request).Settled, "activity alone is not a terminal result")
				valid := parseCodexProStream(dshJSONL(frame, codexProFrame(request, codexProFixture(request))), request)
				require.True(t, valid.Settled, valid.Error)
				frame["activity"].(map[string]any)["eventTypes"] = []string{"fake"}
				require.False(t, parseCodexProStream(dshJSONL(frame, codexProFrame(request, codexProFixture(request))), request).CleanupConfirmed)
			} else {
				claude := claudeCodeProStart(request)
				require.False(t, parseClaudeCodeProStream(dshJSONL(frame), claude).Settled)
				valid := parseClaudeCodeProStream(dshJSONL(frame, claudeCodeProFrame(claude, claudeCodeProFixture(claude))), claude)
				require.True(t, valid.Settled, valid.Error)
				frame["activity"].(map[string]any)["eventTypes"] = []string{"fake"}
				require.False(t, parseClaudeCodeProStream(dshJSONL(frame, claudeCodeProFrame(claude, claudeCodeProFixture(claude))), claude).CleanupConfirmed)
			}
		})
	}
}

func TestTerminationSeparatesTheCauseFromCleanupCancellation(t *testing.T) {
	for _, tc := range []struct {
		name                                      string
		outer                                     Result
		cancelled, timeout, truncated, wantCancel bool
	}{
		{"outer idle", Result{TimedOut: true, IdleTimedOut: true}, true, false, false, false},
		{"outer wall", Result{TimedOut: true, WallTimedOut: true}, true, false, false, false},
		{"provider deadline", Result{}, true, true, false, false},
		{"output cap", Result{}, true, false, true, false},
		{"user context", Result{Cancelled: true}, true, false, false, true},
		{"provider cancel", Result{}, true, false, false, true},
		{"success", Result{}, false, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := tc.outer
			mergeWorkerTermination(&out, tc.cancelled, tc.timeout, tc.truncated)
			require.Equal(t, tc.wantCancel, out.Cancelled)
			require.Equal(t, tc.outer.TimedOut || tc.timeout, out.TimedOut)
			require.Equal(t, tc.truncated, out.Truncated)
		})
	}
	for _, out := range []Result{{TimedOut: true, IdleTimedOut: true}, {TimedOut: true, WallTimedOut: true}, {TimedOut: true}} {
		require.Contains(t, timeoutMessage("Provider", out), "time")
	}
}

func TestKimiNativeTimeoutKeepsCauseAndTrustedCleanup(t *testing.T) {
	request := kimiNativeStart{RequestID: "r", Native: json.RawMessage(`{"version":1,"mode":"agent"}`)}
	ready, terminal := kimiNativeFrames(request)
	terminal["status"] = "cancelled"
	terminal["text"] = ""
	terminal["lifecycle"].(map[string]any)["outcome"] = "cancelled"
	terminal["native"] = nativeReceiptFixture(request.Native, false, true)
	exit := 130
	out := finishKimiNative(request, watchdog.Result{Stdout: dshJSONL(ready, terminal), ExitCode: &exit,
		TimedOut: true, IdleTimedOut: true}, nil)
	require.True(t, out.TimedOut)
	require.False(t, out.Cancelled)
	require.Equal(t, errs.TIMEOUT, out.ErrorCode)
	require.Contains(t, *out.ErrorMessage, "idle timeout")
	require.Equal(t, "unknown", out.Session.Outcome)
	require.True(t, out.Session.CleanupConfirmed)
}
