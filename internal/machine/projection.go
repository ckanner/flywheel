package machine

import (
	"fmt"
	"sort"

	"github.com/ckanner/flywheel/internal/state"
)

// Display-only projection. Identity, revision, verdicts and counters are intact;
// bounded diagnostic excerpts keep large task graphs inside the control frame.
func ConsoleProjection(st *state.State) (*state.State, error) {
	if st == nil {
		return nil, nil
	}
	view := jsonClone(st)
	for id, packet := range view.Packets {
		if packet == nil {
			return nil, fmt.Errorf("state contains a null packet %s", id)
		}
		if len(view.ContractFingerprints) > 0 && view.ContractFingerprints[id] == "" {
			delete(view.Packets, id)
			continue
		}
		packet.Evidence.Executor = evidenceSummary(packet.Evidence.Executor, "source", "report-sha256", "identity", "at")
		packet.Evidence.L1 = evidenceSummary(packet.Evidence.L1, "pass", "identity", "identity-error", "at", "attribution")
		packet.Evidence.Review = evidenceSummary(packet.Evidence.Review, "verdict", "identity", "scope", "score", "quick-eligible", "at")
		packet.Evidence.FinalGate = evidenceSummary(packet.Evidence.FinalGate, "pass", "identity", "at")
		packet.Evidence.DefectInjection = evidenceSummary(packet.Evidence.DefectInjection, "outcome", "skipped", "detail", "at")
		packet.LastFeedback = evidenceSummary(packet.LastFeedback, "error-code", "reason", "parallel-error", "at")
		packet.Commit = compactDiagnostic(packet.Commit)
		packet.EvidenceHistory = historySummary(packet.EvidenceHistory)
		packet.RetryHistory = historySummary(packet.RetryHistory)
	}
	view.Inbox = compactDiagnostic(view.Inbox).([]any)
	return view, nil
}

func compactDiagnostic(value any) any {
	switch value := value.(type) {
	case string:
		text := []rune(value)
		if len(text) > 512 {
			return string(text[:512]) + "… [truncated]"
		}
		return value
	case []any:
		start := max(0, len(value)-10)
		result := make([]any, 0, len(value)-start+1)
		if start > 0 {
			result = append(result, map[string]any{"omitted-items": start})
		}
		for _, item := range value[start:] {
			result = append(result, compactDiagnostic(item))
		}
		return result
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		result := map[string]any{}
		for _, key := range keys[:min(len(keys), 32)] {
			result[key] = compactDiagnostic(value[key])
		}
		if len(keys) > 32 {
			result["omitted-fields"] = len(keys) - 32
		}
		return result
	default:
		return value
	}
}

type NodeDetail struct {
	ID       string             `json:"id"`
	Revision int64              `json:"revision"`
	Packet   *state.PacketState `json:"packet"`
}

func ReadNode(st *state.State, id string) (*NodeDetail, error) {
	if st == nil || st.Packets[id] == nil {
		return nil, fmt.Errorf("packet not found")
	}
	return &NodeDetail{ID: id, Revision: st.Revision, Packet: st.Packets[id]}, nil
}

func evidenceSummary(value any, keys ...string) any {
	object, ok := value.(map[string]any)
	if !ok {
		return compactDiagnostic(value)
	}
	summary := map[string]any{}
	for _, key := range keys {
		if field, ok := object[key]; ok {
			summary[key] = compactDiagnostic(field)
		}
	}
	if len(summary) < len(object) {
		summary["details-omitted"] = true
	}
	return summary
}
func historySummary(entries []any) []any {
	if len(entries) == 0 {
		return []any{}
	}
	return []any{map[string]any{"omitted-items": len(entries)}}
}
