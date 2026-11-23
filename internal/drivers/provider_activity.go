package drivers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

var providerActivityTypes = map[string][]string{
	"antigravity": {"init", "step", "result", "agent_message_chunk", "agent_thought_chunk", "user_message_chunk",
		"tool_call", "tool_call_update", "subagent_update", "config_option_update", "current_mode_update",
		"available_commands_update", "plan", "usage_update", "session_info_update", "session_message",
		"session_message_chunk", "compaction_update", "compaction_summary_chunk", "unknown"},
	"codex-pro": {"turn/started", "turn/completed", "item/started", "item/completed", "item/agentMessage/delta",
		"item/plan/delta", "item/reasoning/summaryTextDelta", "item/reasoning/summaryPartAdded", "item/reasoning/textDelta",
		"item/commandExecution/outputDelta", "item/mcpToolCall/progress", "turn/plan/updated", "turn/diff/updated", "thread/tokenUsage/updated"},
	"claude-code-pro": {"session-init", "api-retry", "compaction", "task-started", "task-progress", "task-updated",
		"task-notification", "hook-started", "hook-progress", "hook-response", "assistant-text", "assistant-thinking",
		"tool-use", "tool-result", "tool-progress", "stream-start", "stream-delta", "stream-stop", "result"},
}

// Only bounded, correlated, monotonically newer observation is admitted. These
// frames never establish dispatch, result, usage, cleanup or durable ACK facts.
func validateProActivityEvent(raw json.RawMessage, harness string, request codexProStart, sequence *int64) error {
	var frame struct {
		Type      string `json:"type"`
		Protocol  int    `json:"protocol"`
		RequestID string `json:"requestId"`
		Adapter   struct {
			Package  string `json:"package"`
			Version  string `json:"version"`
			Protocol int    `json:"protocol"`
		} `json:"adapter"`
		Activity struct {
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
	version := machineAdapterVersion(harness)
	if len(raw) > 4096 || d.Decode(&frame) != nil || frame.Type != "activity" || frame.Protocol != request.Protocol || frame.RequestID != request.RequestID ||
		frame.Adapter.Package != adapterPackageFor(harness) || frame.Adapter.Version != version || frame.Adapter.Protocol != 1 {
		return fmt.Errorf("invalid request: uncorrelated or damaged %s activity frame", harness)
	}
	a := frame.Activity
	if a.Version != 1 || a.Kind != "provider-progress" || a.Source != harness ||
		!slices.Contains([]string{"root", "child", "mixed"}, a.Scope) || a.Sequence <= *sequence || a.Sequence > 9007199254740991 ||
		a.ObservedEvents <= 0 || a.ObservedEvents > 9007199254740991 || len(a.EventTypes) == 0 || len(a.EventTypes) > 16 {
		return fmt.Errorf("invalid request: invalid or stale %s activity metadata", harness)
	}
	for i, event := range a.EventTypes {
		if !slices.Contains(providerActivityTypes[harness], event) || slices.Contains(a.EventTypes[:i], event) {
			return fmt.Errorf("invalid request: unsupported %s activity event type", harness)
		}
	}
	*sequence = a.Sequence
	return nil
}
