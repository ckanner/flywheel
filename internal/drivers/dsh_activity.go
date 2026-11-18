package drivers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/ckanner/flywheel/internal/watchdog"
)

var dshActivityTypes = []string{"assistant/start", "assistant/chunk", "assistant/end", "assistant/block-start", "assistant/block-end",
	"assistant/text-delta", "assistant/reasoning-delta", "assistant/tool-call-delta", "assistant/usage", "assistant/finish",
	"provider/codex-pro", "provider/claude-code-pro", "provider/chatgpt-web"}

func allowedDshActivityEvent(event string) bool {
	return slices.Contains(dshActivityTypes, event)
}

// Only the policy's metadata crosses this channel. It never attests outcome,
// billing or cleanup and cannot create a persistent Session event on replay.
func validateDshActivityEvent(raw json.RawMessage, request dshStart, sessionID string, sequence *int64) error {
	var frame struct {
		Type      string `json:"type"`
		Protocol  int    `json:"protocol"`
		RequestID string `json:"requestId"`
		SessionID string `json:"sessionId"`
		Activity  struct {
			Version        int      `json:"version"`
			Sequence       int64    `json:"sequence"`
			Kind           string   `json:"kind"`
			Source         string   `json:"source"`
			Scope          string   `json:"scope"`
			EventTypes     []string `json:"eventTypes"`
			ObservedEvents int64    `json:"observedEvents"`
		} `json:"activity"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 4096 || d.Decode(&frame) != nil || frame.Type != "activity" || frame.Protocol != request.Protocol ||
		frame.RequestID != request.RequestID || frame.SessionID != sessionID {
		return fmt.Errorf("DSH activity frame is damaged or belongs to a different invocation")
	}
	a := frame.Activity
	if a.Version != 1 || a.Kind != "provider-progress" || a.Source != "dsh" ||
		!slices.Contains([]string{"root", "child", "mixed"}, a.Scope) || a.Sequence <= *sequence || a.Sequence > 9007199254740991 ||
		a.ObservedEvents <= 0 || a.ObservedEvents > 9007199254740991 || len(a.EventTypes) == 0 || len(a.EventTypes) > 16 {
		return fmt.Errorf("DSH activity metadata is invalid or stale")
	}
	for i, event := range a.EventTypes {
		if !allowedDshActivityEvent(event) || slices.Contains(a.EventTypes[:i], event) {
			return fmt.Errorf("DSH activity event type is unsupported or duplicated")
		}
	}
	*sequence = a.Sequence
	return nil
}

// Keep the raw observation callback live while preventing advisory metadata
// from exhausting the final-receipt capture budget. Warm requests are resolved
// only after formal handoff; their warm-ready frame does not admit a Session.
func newDSHActivityCapture(request func() dshStart) watchdog.OutputCapture {
	var sessionID string
	var sequence int64
	return newAdvisoryActivityCapture(func(raw json.RawMessage) (bool, error) {
		var header struct {
			Type string `json:"type"`
		}
		// The shared capture already decoded this exact header successfully.
		_ = json.Unmarshal(raw, &header)
		switch header.Type {
		case "ready":
			// The normal parser verifies the complete requested route, history,
			// directory and permission echo before recording this Session ID.
			ready := parseDSHStream(string(raw), request())
			if sessionID != "" || ready.SessionID == "" {
				// Preserve this original non-advisory frame and its precise final
				// diagnostic. It cannot authorize filtering later activity.
				sessionID = ""
				return false, nil
			}
			sessionID = ready.SessionID
		case "activity":
			if sessionID == "" {
				return false, fmt.Errorf("DSH activity capture has no admitted session")
			}
			if err := validateDshActivityEvent(raw, request(), sessionID, &sequence); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, nil
	}, "result", "warm-result")
}
