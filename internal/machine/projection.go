package machine

import (
	"fmt"
	"sort"

	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/state"
)

// Display-only projection. Identity, revision, verdicts and counters are intact;
// bounded diagnostic excerpts keep large task graphs inside the control frame.
func ConsoleProjection(st *state.State, nodes ...*NodeDetail) (*state.State, error) {
	if st == nil {
		return nil, nil
	}
	// Planning histories retain complete immutable files for every version.
	// Select a bounded view before cloning instead of duplicating all histories.
	source := *st
	source.Planning = nil
	view := jsonClone(&source)
	bindingBudget := displayBindingBudget
	if view.Run != nil {
		view.Run.InitialBindings, view.Run.InitialBindingsOmitted = compactBindingSnapshots(view.Run.InitialBindings, &bindingBudget)
	}
	if st.Planning != nil {
		var err error
		view.Planning, err = consolePlanningProjection(st.Planning, 512<<10)
		if err != nil {
			return nil, err
		}
	}
	if ledger := view.RuntimeDecisions; ledger != nil {
		if ledger.Config != nil {
			ledger.MaxCalls = ledger.Config.MaxCalls
		}
		ledger.Config = nil
		ledger.Omitted = max(0, len(ledger.Records)-10)
		ledger.Records = ledger.Records[ledger.Omitted:]
		for _, record := range ledger.Records {
			if record == nil {
				return nil, fmt.Errorf("state contains a null runtime decision")
			}
			record.ExecutionView = runtimeDecisionExecutionView(st, record)
			record.Execution = nil
			record.ModelBindings, record.ModelBindingsOmitted = compactBindingSnapshots(record.ModelBindings, &bindingBudget)
			if record.Snapshot != nil {
				record.Phase = "after-failure"
				if record.Snapshot.Version == 3 {
					record.Phase = "initial-execution"
				}
				record.BaselineActionID = record.Snapshot.BaselineID
				record.Actions = record.Snapshot.Actions
			}
			record.Snapshot = nil
			if receipt := record.Receipt; receipt != nil {
				record.Receipt = &decision.Receipt{ActionID: receipt.ActionID, Status: receipt.Status, Reason: receipt.Reason, Model: receipt.Model, Usage: receipt.Usage, DurationMS: receipt.DurationMS}
			}
			record.Feedback = compactDiagnostic(record.Feedback)
		}
	}
	packetIDs := make([]string, 0, len(view.Packets))
	for id := range view.Packets {
		packetIDs = append(packetIDs, id)
	}
	sort.Strings(packetIDs)
	for _, id := range packetIDs {
		packet := view.Packets[id]
		if packet == nil {
			return nil, fmt.Errorf("state contains a null packet %s", id)
		}
		if len(view.ContractFingerprints) > 0 && view.ContractFingerprints[id] == "" {
			delete(view.Packets, id)
			continue
		}
		compactCurrentExecutionBinding(packet.ExecutionPolicy, &bindingBudget)
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
	// A node response also carries the selected packet's full evidence. Keep
	// those details, but bound its new display metadata within this same reply.
	for _, node := range nodes {
		if node != nil && node.Packet != nil && node.Packet.ExecutionPolicy != nil {
			packet := *node.Packet
			policy := *packet.ExecutionPolicy
			packet.ExecutionPolicy, node.Packet = &policy, &packet
			compactCurrentExecutionBinding(&policy, &bindingBudget)
		}
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
	Eligibility         *PacketEligibility       `json:"eligibility,omitempty"`
	ImageInputEditor    *ImageInputEditor        `json:"imageInputEditor,omitempty"`
	CollaborationEditor *CollaborationEditor     `json:"collaborationEditor,omitempty"`
	Collaboration       *CollaborationProjection `json:"collaboration,omitempty"`
	ID                  string                   `json:"id"`
	Revision            int64                    `json:"revision"`
	Packet              *state.PacketState       `json:"packet"`
}

func ReadNode(st *state.State, id string, location ...string) (*NodeDetail, error) {
	if st == nil || st.Packets[id] == nil {
		return nil, fmt.Errorf("packet not found")
	}
	eligibility, editor, images := packetControlViews(st, id, location)
	return &NodeDetail{ID: id, Revision: st.Revision, Packet: st.Packets[id], Collaboration: reviewedCollaboration(st, id, location), Eligibility: eligibility, CollaborationEditor: editor, ImageInputEditor: images}, nil
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
