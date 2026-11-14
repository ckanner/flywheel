package drivers

import (
	"encoding/json"
	"testing"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/require"
)

func nativeReceiptFixture(request json.RawMessage, matched, cleanup bool) map[string]any {
	return map[string]any{"version": 1, "requested": request, "effective": map[string]any{"matched": matched}, "cleanup": map[string]any{"confirmed": cleanup}, "completion": map[string]any{"confirmed": matched, "pendingIds": []string{}}}
}

func TestNativeReceiptRequiresExactRequestAndCleanup(t *testing.T) {
	requested := json.RawMessage(`{"version":1,"preset":"heavy"}`)
	raw, err := json.Marshal(nativeReceiptFixture(requested, true, true))
	require.NoError(t, err)
	require.NoError(t, validateNativeReceipt(requested, raw, true, true))
	require.NoError(t, validateNativeReceipt(nil, nil, true, true))
	require.NoError(t, validateNativeReceipt(nil, json.RawMessage(`null`), false, true))
	require.NoError(t, validateNativeReceipt(requested, nil, false, true))
	require.Error(t, validateNativeReceipt(nil, raw, false, true))
	require.Error(t, validateNativeReceipt(requested, nil, true, true))
	require.Error(t, validateNativeReceipt(requested, raw, true, false))
	require.Error(t, validateNativeReceipt(json.RawMessage(`{"version":1}`), raw, true, true))
	require.Error(t, validateNativeReceipt(json.RawMessage(`{`), raw, true, true))
	for _, bad := range []string{`{`, `null`, `{}`, `{"version":1,"requested":{"version":1,"preset":"heavy"},"cleanup":{"confirmed":true}}`} {
		require.Error(t, validateNativeReceipt(requested, json.RawMessage(bad), true, true))
	}
	raw, err = json.Marshal(nativeReceiptFixture(requested, false, false))
	require.NoError(t, err)
	require.NoError(t, validateNativeReceipt(requested, raw, false, false))
	require.Equal(t, int64(145000), dshCleanupGrace(DSHArgs{Native: json.RawMessage(`{"lcu":{"enabled":true}}`)}))
	require.Equal(t, int64(200000), dshCleanupGrace(DSHArgs{Native: json.RawMessage(`{"lcu":{"enabled":true}}`), Watchdog: watchdog.Opts{KillGraceMs: 200000}}))
	require.Equal(t, int64(42), dshCleanupGrace(DSHArgs{Watchdog: watchdog.Opts{KillGraceMs: 42}}))
	require.Equal(t, int64(20000), dshCleanupGrace(DSHArgs{Native: requested}))
}

func TestDSHNativeEventsAndAdmissionCannotMasqueradeAsRootCompletion(t *testing.T) {
	r := dshStart{Protocol: 1, RequestID: "native", Cwd: "/repo", Provider: "account", Model: "flash", PermissionMode: "danger-full-access", ReasoningEffort: "high", Native: json.RawMessage(`{"version":1,"preset":"heavy"}`)}
	ready, end := dshFixtureFrames(r)
	end["native"] = nativeReceiptFixture(r.Native, true, true)
	event := map[string]any{"type": "native-event", "protocol": 1, "requestId": r.RequestID, "sessionId": "sdk-session-one", "event": map[string]any{"type": "subagent.started", "childId": "child"}}
	p := parseDSHStream(dshJSONL(ready, event, end), r)
	require.True(t, p.Settled)
	require.NotEmpty(t, p.NativeReceipt)
	end["native"] = nativeReceiptFixture(r.Native, false, true)
	require.NotEmpty(t, parseDSHStream(dshJSONL(ready, end), r).Error)
	r.Native = nil
	require.NotEmpty(t, parseDSHStream(dshJSONL(ready, event, end), r).Error)
}

func TestNativeObservationFramesNeverAcknowledgeALifecycleCheckpoint(t *testing.T) {
	r := codexProStart{Protocol: 3, RequestID: "native", Native: json.RawMessage(`{"version":1}`)}
	w := &lifecycleWriter{}
	m := newMachineInteraction("codex-pro", r, func(MachineLifecycle) error {
		t.Fatal("native observation must not be a lifecycle checkpoint")
		return nil
	}, nil)
	m.start(w, func() {}, []byte("request\n"))
	frame := map[string]any{"type": "native-event", "protocol": 3, "requestId": "native", "adapter": machineAdapter{Package: "dsh-subagent-codex-pro", Version: CodexProPackageVersion, Protocol: 1}, "native": map[string]any{"version": 1, "type": "child.started"}}
	m.line(dshJSONL(frame))
	require.NoError(t, m.failure())
	require.Equal(t, "request\n", w.String())
	delete(frame, "native")
	m.line(dshJSONL(frame))
	require.ErrorContains(t, m.failure(), "native event")
}
