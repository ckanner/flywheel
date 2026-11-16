package drivers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/ckanner/flywheel/internal/watchdog"
)

// Kimi uses event frames for observed activity. Compact only the three bounded
// progress metadata kinds; retain native lifecycle boundaries and final facts.
func newKimiActivityCapture(request kimiNativeStart) watchdog.OutputCapture {
	sessionID := ""
	return newAdvisoryActivityCapture(func(raw json.RawMessage) (bool, error) {
		var frame struct {
			Type      string          `json:"type"`
			Protocol  int             `json:"protocol"`
			RequestID string          `json:"requestId"`
			SessionID string          `json:"sessionId"`
			Event     json.RawMessage `json:"event"`
		}
		_ = json.Unmarshal(raw, &frame) // capture already checked the object/header
		if frame.Protocol != 1 || frame.RequestID != request.RequestID {
			return false, fmt.Errorf("Kimi activity capture identity mismatch")
		}
		if frame.Type == "ready" {
			if sessionID != "" || frame.SessionID == "" || (request.SessionID != "" && request.SessionID != frame.SessionID) {
				return false, fmt.Errorf("Kimi activity capture has no unique admitted session")
			}
			sessionID = frame.SessionID
		}
		if frame.Type != "event" {
			return false, nil
		}
		if sessionID == "" || frame.SessionID != sessionID || len(frame.Event) == 0 || string(frame.Event) == "null" {
			return false, fmt.Errorf("Kimi activity is outside its admitted session")
		}
		var kind struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(frame.Event, &kind) != nil {
			return false, fmt.Errorf("Kimi activity metadata is not an object")
		}
		progress := []string{"turn.updated", "item.updated", "item.delta"}
		if !slices.Contains(progress, kind.Type) {
			return false, nil
		}
		var event struct {
			Type           string   `json:"type"`
			AgentID        string   `json:"agentId"`
			TaskID         string   `json:"taskId"`
			TurnID         string   `json:"turnId"`
			ItemID         string   `json:"itemId"`
			Status         string   `json:"status"`
			Outcome        string   `json:"outcome"`
			CoalescedTypes []string `json:"coalescedTypes"`
		}
		decoder := json.NewDecoder(bytes.NewReader(frame.Event))
		decoder.DisallowUnknownFields()
		if len(raw) > 4096 || decoder.Decode(&event) != nil || strings.TrimSpace(event.AgentID) == "" ||
			len(event.AgentID) > 512 || strings.ContainsAny(event.AgentID, "\x00\r\n") ||
			len(event.CoalescedTypes) == 0 || len(event.CoalescedTypes) > len(progress) {
			return false, fmt.Errorf("Kimi progress is not bounded metadata")
		}
		seen := map[string]bool{}
		for _, name := range event.CoalescedTypes {
			if !slices.Contains(progress, name) || seen[name] {
				return false, fmt.Errorf("Kimi progress contains an invalid coalesced event type")
			}
			seen[name] = true
		}
		if !seen[kind.Type] {
			return false, fmt.Errorf("Kimi progress does not identify its observed event")
		}
		return true, nil
	})
}
