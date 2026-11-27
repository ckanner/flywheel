package machine

import (
	"fmt"
	"sort"
	"strings"

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
		activeMilestone := ""
		if st.CurrentMilestone != nil {
			activeMilestone = *st.CurrentMilestone
		}
		view.Planning, err = consolePlanningProjection(st.Planning, 512<<10, activeMilestone)
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
		reviewKeys := []string{"verdict", "identity", "scope", "score", "quick-eligible", "at"}
		if packet.Status == "inbox" {
			reviewKeys = append(reviewKeys, "findings", "reasons", "advisory")
		}
		packet.Evidence.Review = evidenceSummary(packet.Evidence.Review, reviewKeys...)
		packet.Evidence.FinalGate = evidenceSummary(packet.Evidence.FinalGate, "pass", "identity", "at")
		packet.Evidence.DefectInjection = evidenceSummary(packet.Evidence.DefectInjection, "outcome", "skipped", "detail", "at")
		if packet.Status == "inbox" {
			packet.LastFeedback = compactInboxLastFeedback(packet.LastFeedback)
		} else {
			packet.LastFeedback = evidenceSummary(packet.LastFeedback, "error-code", "reason", "parallel-error", "at")
		}
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
	milestoneIDs := make([]string, 0, len(view.Milestones))
	for id := range view.Milestones {
		milestoneIDs = append(milestoneIDs, id)
	}
	sort.Strings(milestoneIDs)
	for _, id := range milestoneIDs {
		ms := view.Milestones[id]
		if ms.Gate != nil {
			ms.Gate = compactMilestoneGate(ms.Gate)
			view.Milestones[id] = ms
		}
	}
	return view, nil
}

func compactMilestoneGate(gate any) any {
	gmap, ok := gate.(map[string]any)
	if !ok {
		return compactDiagnostic(gate)
	}
	out := map[string]any{}
	for _, key := range []string{"pass", "at", "identity", "identity-error"} {
		if val, exists := gmap[key]; exists {
			out[key] = compactDiagnostic(val)
		}
	}
	if judge, exists := gmap["judge"]; exists {
		out["judge"] = evidenceSummary(judge, "verdict", "score", "reasons", "advisory", "error", "at")
	}
	if levels, ok := gmap["levels"].(map[string]any); ok {
		compactLevels := map[string]any{}
		for _, lvlName := range []string{"prepare", "L2", "L3"} {
			lvlVal, exists := levels[lvlName]
			if !exists {
				continue
			}
			lvlMap, ok := lvlVal.(map[string]any)
			if !ok {
				compactLevels[lvlName] = compactDiagnostic(lvlVal)
				continue
			}
			cl := map[string]any{}
			for _, k := range []string{"pass", "skipped", "identity-error"} {
				if v, ok := lvlMap[k]; ok {
					cl[k] = compactDiagnostic(v)
				}
			}
			if results, ok := lvlMap["results"].([]any); ok {
				compactResults := make([]any, 0, min(len(results), 10))
				for _, r := range results[:min(len(results), 10)] {
					rm, ok := r.(map[string]any)
					if !ok {
						compactResults = append(compactResults, compactDiagnostic(r))
						continue
					}
					cr := map[string]any{}
					for _, rk := range []string{"cmd", "pass", "exitCode", "exit-code", "timedOut", "durationMs"} {
						if rv, ok := rm[rk]; ok {
							cr[rk] = rv
						}
					}
					if stderr, ok := rm["stderr"].(string); ok && stderr != "" {
						cr["stderr"] = compactTail(stderr, 512)
					}
					if stdout, ok := rm["stdout"].(string); ok && stdout != "" {
						cr["stdout"] = compactTail(stdout, 512)
					}
					compactResults = append(compactResults, cr)
				}
				cl["results"] = compactResults
			}
			compactLevels[lvlName] = cl
		}
		out["levels"] = compactLevels
	}
	return out
}

func compactTail(value string, maxRunes int) string {
	text := []rune(value)
	if len(text) <= maxRunes {
		return value
	}
	return "[truncated] …" + string(text[len(text)-maxRunes:])
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

func compactInboxLastFeedback(value any) any {
	object, ok := value.(map[string]any)
	if !ok {
		return compactDiagnostic(value)
	}
	summary := evidenceSummary(object, "error-code", "reason", "parallel-error", "failed-commands", "at").(map[string]any)
	if l1, ok := object["l1"].(map[string]any); ok {
		if cmds, ok := l1["commands"].([]any); ok {
			var failed []any
			for _, item := range cmds {
				cm, ok := item.(map[string]any)
				if !ok || cm["pass"] == true {
					continue
				}
				entry := map[string]any{}
				if cmd, ok := cm["cmd"].(string); ok {
					entry["cmd"] = compactTail(cmd, 200)
				}
				if code, ok := cm["exitCode"]; ok {
					entry["exitCode"] = code
				}
				if out, ok := cm["output"].(string); ok && strings.TrimSpace(out) != "" {
					entry["output"] = compactTail(strings.TrimSpace(out), 420)
				}
				failed = append(failed, entry)
				if len(failed) >= 4 {
					break
				}
			}
			if len(failed) > 0 {
				summary["failed-l1-details"] = failed
			}
		}
	}
	return summary
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
