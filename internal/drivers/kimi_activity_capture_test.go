package drivers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKimiNativeActivityCaptureKeepsFinalReceiptAndRejectsDamagedMetadata(t *testing.T) {
	request := kimiNativeStart{RequestID: "kimi-activity-call"}
	ready, terminal := kimiNativeFrames(request)
	progress := func() map[string]any {
		return map[string]any{"type": "event", "protocol": 1, "requestId": request.RequestID, "sessionId": "kimi-one",
			"event": map[string]any{"type": "item.delta", "agentId": "main", "coalescedTypes": []string{"item.delta"}}}
	}
	capture := newKimiActivityCapture(request)
	var output strings.Builder
	output.Write(capture.Filter([]byte(dshJSONL(ready))))
	line := []byte(dshJSONL(progress()))
	for range 100000 {
		output.Write(capture.Filter(line))
	}
	output.Write(capture.Filter([]byte(dshJSONL(terminal))))
	output.Write(capture.Flush())
	require.Less(t, output.Len(), 4096)
	require.Equal(t, 1, strings.Count(output.String(), `"type":"item.delta"`))
	require.Contains(t, output.String(), `"type":"result"`)
	require.NotContains(t, output.String(), advisoryCaptureRejected)
	for name, mutate := range map[string]func(map[string]any){
		"wrong request":    func(f map[string]any) { f["requestId"] = "foreign" },
		"foreign session":  func(f map[string]any) { f["sessionId"] = "foreign" },
		"missing event":    func(f map[string]any) { delete(f, "event") },
		"non-object event": func(f map[string]any) { f["event"] = "garbage" },
		"raw content":      func(f map[string]any) { f["event"].(map[string]any)["text"] = "private reasoning" },
		"missing agent":    func(f map[string]any) { delete(f["event"].(map[string]any), "agentId") },
		"too large":        func(f map[string]any) { f["event"].(map[string]any)["itemId"] = strings.Repeat("x", 5000) },
		"unknown type":     func(f map[string]any) { f["event"].(map[string]any)["coalescedTypes"] = []string{"heartbeat"} },
		"duplicate type": func(f map[string]any) {
			f["event"].(map[string]any)["coalescedTypes"] = []string{"item.delta", "item.delta"}
		},
		"wrong observed type": func(f map[string]any) { f["event"].(map[string]any)["coalescedTypes"] = []string{"turn.updated"} },
	} {
		t.Run(name, func(t *testing.T) {
			bad := progress()
			mutate(bad)
			c := newKimiActivityCapture(request)
			_ = c.Filter([]byte(dshJSONL(ready)))
			require.Contains(t, string(c.Filter([]byte(dshJSONL(bad)))), advisoryCaptureRejected)
		})
	}
	beforeReady := newKimiActivityCapture(request)
	require.Contains(t, string(beforeReady.Filter(line)), advisoryCaptureRejected)
	duplicate := newKimiActivityCapture(request)
	_ = duplicate.Filter([]byte(dshJSONL(ready)))
	require.Contains(t, string(duplicate.Filter([]byte(dshJSONL(ready)))), advisoryCaptureRejected)
	wrongResume := newKimiActivityCapture(kimiNativeStart{RequestID: request.RequestID, SessionID: "expected"})
	require.Contains(t, string(wrongResume.Filter([]byte(dshJSONL(ready)))), advisoryCaptureRejected)
	// Non-progress lifecycle observations stay verbatim for diagnosis.
	nativeEvent := progress()
	nativeEvent["event"] = map[string]any{"type": "turn.ended", "agentId": "main"}
	c := newKimiActivityCapture(request)
	_ = c.Filter([]byte(dshJSONL(ready)))
	require.Equal(t, dshJSONL(nativeEvent), string(c.Filter([]byte(dshJSONL(nativeEvent)))))
}
