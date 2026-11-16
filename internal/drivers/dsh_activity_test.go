package drivers

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dshActivityFloodEvents = 70000

func emitDSHActivityFlood(request dshStart) {
	writer := bufio.NewWriterSize(os.Stdout, 65536)
	encoder := json.NewEncoder(writer)
	value := dshActivityFixture(request, "sdk-session-one", 1)
	for i := int64(1); i <= dshActivityFloodEvents; i++ {
		value["activity"].(map[string]any)["sequence"] = i
		if encoder.Encode(value) != nil {
			os.Exit(2)
		}
	}
	if writer.Flush() != nil {
		os.Exit(2)
	}
}

func dshActivityFixture(request dshStart, session string, sequence int64) map[string]any {
	return map[string]any{"type": "activity", "protocol": request.Protocol, "requestId": request.RequestID, "sessionId": session,
		"activity": map[string]any{"version": 1, "sequence": sequence, "kind": "provider-progress", "source": "dsh", "scope": "child",
			"eventTypes": []string{"assistant/reasoning-delta", "provider/codex-pro"}, "observedEvents": 50}}
}

func TestDSHActivityIsBoundedCorrelatedObservationOnly(t *testing.T) {
	request := scopedDSHRequest()
	ready, result := dshFixtureFrames(request)
	frame := dshActivityFixture(request, "sdk-session-one", 1)
	parsed := parseDSHStream(dshJSONL(ready, frame, result), request)
	require.True(t, parsed.Settled, "%+v", parsed)
	assert.Equal(t, "verified response", parsed.Text)
	assert.Nil(t, parsed.Usage, "activity counts are not token usage")
	for _, tc := range []struct {
		name string
		edit func(map[string]any, map[string]any)
	}{
		{"foreign session", func(f, _ map[string]any) { f["sessionId"] = "foreign" }},
		{"foreign request", func(f, _ map[string]any) { f["requestId"] = "foreign" }},
		{"unknown envelope field", func(f, _ map[string]any) { f["text"] = "private model body" }},
		{"unknown activity field", func(_, a map[string]any) { a["arguments"] = "private arguments" }},
		{"wrong version", func(_, a map[string]any) { a["version"] = 2 }},
		{"wrong source", func(_, a map[string]any) { a["source"] = "codex-pro" }},
		{"wrong kind", func(_, a map[string]any) { a["kind"] = "result" }},
		{"wrong scope", func(_, a map[string]any) { a["scope"] = "foreign" }},
		{"no progress", func(_, a map[string]any) { a["observedEvents"] = 0 }},
		{"unsafe count", func(_, a map[string]any) { a["observedEvents"] = int64(9007199254740992) }},
		{"invalid sequence", func(_, a map[string]any) { a["sequence"] = 0 }},
		{"unsafe sequence", func(_, a map[string]any) { a["sequence"] = int64(9007199254740992) }},
		{"empty events", func(_, a map[string]any) { a["eventTypes"] = []string{} }},
		{"unknown type", func(_, a map[string]any) { a["eventTypes"] = []string{"not-a-provider-event"} }},
		{"duplicate type", func(_, a map[string]any) { a["eventTypes"] = []string{"assistant/start", "assistant/start"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := dshActivityFixture(request, "sdk-session-one", 1)
			tc.edit(value, value["activity"].(map[string]any))
			parsed := parseDSHStream(dshJSONL(ready, value, result), request)
			assert.False(t, parsed.Settled)
			assert.NotEmpty(t, parsed.Error)
			assert.Nil(t, parsed.Session, "damaged activity cannot attest a safe terminal")
		})
	}
	assert.NotEmpty(t, parseDSHStream(dshJSONL(frame, ready, result), request).Error)
	assert.NotEmpty(t, parseDSHStream(dshJSONL(ready, frame, frame, result), request).Error)
	var sequence int64
	assert.Error(t, validateDshActivityEvent(json.RawMessage(strings.Repeat(" ", 4097)), request, "sdk-session-one", &sequence))
	assert.Error(t, validateDshActivityEvent(json.RawMessage("{broken}"), request, "sdk-session-one", &sequence))
	for _, event := range dshActivityTypes {
		assert.True(t, allowedDshActivityEvent(event))
	}
	assert.False(t, allowedDshActivityEvent("private text"))
}

func TestDSHInterruptedPersistentCallKeepsIndependentVerifiedFacts(t *testing.T) {
	request := scopedDSHRequest()
	ready, terminal := dshFixtureFrames(request)
	terminal["status"] = "cancelled"
	lifecycle := terminal["lifecycle"].(map[string]any)
	lifecycle["outcome"], lifecycle["resumeCandidate"], lifecycle["turnId"] = "unknown", false, ""
	stream := dshJSONL(ready, terminal)
	exit := 143
	a := DSHArgs{PersistentSession: true, RequestID: request.RequestID}
	for _, cause := range []string{"idle", "wall", "user"} {
		t.Run(cause, func(t *testing.T) {
			physical := watchdog.Result{Stdout: stream, ExitCode: &exit, TimedOut: cause != "user", IdleTimedOut: cause == "idle", WallTimedOut: cause == "wall", Cancelled: cause == "user"}
			result := finishDSH(a, request, physical, nil)
			require.NotNil(t, result.Session)
			assert.False(t, result.Settled)
			assert.Equal(t, "sdk-session-one", result.Session.SessionID)
			assert.Equal(t, "dispatched", result.Session.Dispatch)
			assert.Equal(t, "unknown", result.Session.Outcome)
			assert.True(t, result.Session.CleanupConfirmed)
			assert.False(t, result.Session.ResumeCandidate)
			assert.Equal(t, cause == "user", result.Cancelled)
			if cause != "user" {
				assert.Equal(t, errs.TIMEOUT, result.ErrorCode)
				require.NotNil(t, result.ErrorMessage)
				assert.Contains(t, *result.ErrorMessage, cause+" timeout")
			}
		})
	}
	for _, damage := range []string{"tail", "truncated", "signal", "unconfirmed cleanup", "no exit", "foreign lifecycle"} {
		t.Run(damage, func(t *testing.T) {
			physical := watchdog.Result{Stdout: stream, ExitCode: &exit, TimedOut: true}
			switch damage {
			case "tail":
				physical.Stdout += "{broken}\n"
			case "truncated":
				physical.Truncated = true
			case "signal":
				physical.Signal = "SIGKILL"
			case "unconfirmed cleanup":
				physical.CleanupError = "owned group still exists"
			case "no exit":
				physical.ExitCode = nil
			case "foreign lifecycle":
				physical.Stdout = strings.ReplaceAll(stream, "sdk-session-one", "different-session")
				physical.Stdout = strings.Replace(physical.Stdout, `"sessionId":"different-session"`, `"sessionId":"sdk-session-one"`, 1)
			}
			result := finishDSH(a, request, physical, nil)
			assert.False(t, result.Session.CleanupConfirmed)
			assert.False(t, result.Session.ResumeCandidate)
			assert.Equal(t, "unknown", result.Session.Outcome)
		})
	}
}

func TestDSHEffectiveTimeoutsAreExplicitForNode(t *testing.T) {
	a := dshRunArgs(t, "ok")
	a.Watchdog = watchdog.Opts{}
	request, _, _, _, err := dshCall(a)
	require.NoError(t, err)
	assert.Equal(t, int64(watchdog.DefaultWallTimeoutMs), request.TimeoutMS)
	assert.Equal(t, int64(watchdog.DefaultIdleTimeoutMs), request.IdleTimeoutMS)
	a.Watchdog.WallTimeoutMs, a.Watchdog.IdleTimeoutMs = 400000, 1200
	request, _, _, _, err = dshCall(a)
	require.NoError(t, err)
	assert.Equal(t, int64(400000), request.TimeoutMS)
	assert.Equal(t, int64(1200), request.IdleTimeoutMS)
}

func TestDSHActivityCaptureKeepsTerminalWithoutLosingInvalidStreamEvidence(t *testing.T) {
	request := scopedDSHRequest()
	ready, result := dshFixtureFrames(request)
	activity := dshActivityFixture(request, "sdk-session-one", 1)
	for _, tc := range []struct {
		name string
		data string
		ok   bool
	}{
		{"valid", dshJSONL(ready, activity, result), true},
		{"before ready", dshJSONL(activity, ready, result), false},
		{"invalid ready", dshJSONL(map[string]any{"type": "ready", "sessionId": "sdk-session-one"}, activity, result), false},
		{"duplicate ready", dshJSONL(ready, ready, activity, result), false},
		{"duplicate activity after filtering", dshJSONL(ready, activity, activity, result), false},
		{"foreign activity", strings.Replace(dshJSONL(ready, activity, result), `"source":"dsh"`, `"source":"foreign"`, 1), false},
		{"after terminal", dshJSONL(ready, activity, result, activity), false},
		{"unterminated terminal", strings.TrimSuffix(dshJSONL(ready, activity, result), "\n"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := newDSHActivityCapture(func() dshStart { return request })
			var out strings.Builder
			for start := 0; start < len(tc.data); start += 19 {
				out.Write(capture.Filter([]byte(tc.data[start:min(start+19, len(tc.data))])))
			}
			out.Write(capture.Flush())
			parsed := parseDSHStream(out.String(), request)
			assert.Equal(t, tc.ok, parsed.Settled, "%+v", parsed)
			if !tc.ok {
				assert.Contains(t, out.String(), advisoryCaptureRejected)
				assert.False(t, parsed.CleanupConfirmed)
				assert.Nil(t, parsed.Session)
			}
		})
	}
}

func TestDSHColdAndWarmActivityCannotDisplaceFinalReceipt(t *testing.T) {
	for _, warm := range []bool{false, true} {
		t.Run(map[bool]string{false: "cold", true: "prewarm"}[warm], func(t *testing.T) {
			a := dshRunArgs(t, "activity-flood")
			if warm {
				a = warmArgs(t, "activity-flood")
			}
			a.PersistentSession, a.RequestID = true, "captured-receipt"
			a.Watchdog.IdleTimeoutMs, a.Watchdog.WallTimeoutMs, a.Watchdog.KillGraceMs = 5000, 60000, 1000
			if warm {
				h := PrepareDSH(a)
				awaitWarm(t, h)
				t.Cleanup(func() { _ = h.Close() })
				a.Prewarm = h
			}
			var events, rawBytes int
			a.OnEvent = func(line string) {
				rawBytes += len(line) + 1
				if strings.Contains(line, `"type":"activity"`) {
					events++
				}
			}
			result := RunDSH(a)
			require.True(t, result.Settled, "%+v", result)
			assert.False(t, result.Truncated)
			assert.Equal(t, dshActivityFloodEvents, events, "raw observer callbacks are not filtered")
			assert.Greater(t, int64(rawBytes), watchdog.MaxBuffer)
			assert.Less(t, len(result.Stdout), 4096)
			assert.Equal(t, 1, strings.Count(result.Stdout, `"type":"activity"`), "only the latest advisory observation is retained")
			require.NotNil(t, result.Session)
			assert.True(t, result.Session.CleanupConfirmed)
			assert.Equal(t, "completed", result.Session.Outcome)
		})
	}
}

// This crosses the actual Go watchdog, one-use warm transport, built Node
// worker and published SDK. Only the remote model is deterministic/offline.
func TestDSHProviderActivityOfflineBridge(t *testing.T) {
	if os.Getenv("FLYWHEEL_TEST_NODE_BRIDGE") != "1" {
		t.Skip("set FLYWHEEL_TEST_NODE_BRIDGE=1 after building plugins/dsh")
	}
	worker, err := filepath.Abs("../../plugins/dsh/dist/worker.js")
	require.NoError(t, err)
	sdk, err := filepath.Abs("../../plugins/dsh/test/fixtures/persisted-sdk.js")
	require.NoError(t, err)
	t.Setenv("FLYWHEEL_DSH_BIN", sdk)
	a := warmArgs(t, "ok")
	a.Command, a.Provider, a.Model = worker, "offline-fixture", "test-model"
	a.RequestID, a.PersistentSession = "owned-streaming-call", true
	a.Watchdog.IdleTimeoutMs, a.Watchdog.WallTimeoutMs, a.Watchdog.KillGraceMs = 2000, 15000, 5000
	a.APIKeys = map[string]string{"TEST_DSH_HISTORY": filepath.Join(a.Cwd, "history"), "TEST_DSH_STREAM_CHUNKS": "50", "TEST_DSH_STREAM_DELAY_MS": "100"}
	h := PrepareDSH(a)
	t.Cleanup(func() { _ = h.Close() })
	select {
	case <-h.ready:
	case <-h.done:
		t.Fatalf("published SDK failed before readiness: %+v", h.result)
	case <-time.After(10 * time.Second):
		t.Fatal("published SDK did not initialize")
	}
	var observations []json.RawMessage
	a.OnEvent = func(line string) {
		var frame struct{ Type string }
		if json.Unmarshal([]byte(line), &frame) == nil && frame.Type == "activity" {
			observations = append(observations, json.RawMessage(line))
		}
	}
	a.Prewarm = h
	result := RunDSH(a)
	require.True(t, result.Settled, "%+v", result)
	assert.False(t, result.TimedOut)
	require.NotNil(t, result.Session)
	assert.True(t, result.Session.CleanupConfirmed)
	assert.Equal(t, "completed", result.Session.Outcome)
	assert.GreaterOrEqual(t, len(observations), 8, "one long attempt stays observable before its durable assistant/message")
	request, _, _, _, err := dshCall(a)
	require.NoError(t, err)
	var sequence int64
	for _, frame := range observations {
		require.NoError(t, validateDshActivityEvent(frame, request, result.SessionID, &sequence))
	}
}
