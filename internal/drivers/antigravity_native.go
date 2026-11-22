package drivers

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/ckanner/flywheel/internal/config"
)

// AGY exposes observed child metadata, not Pro's verified complete execution
// tree. Keep its receipt independent: invocation completion never settles a
// child's state or turns conversation counters into exclusive child usage.
func validateAntigravityNativeReceipt(raw json.RawMessage, request codexProStart, success bool, execution *antigravityExecution, rootID *string) error {
	wanted := config.AntigravityNative{Version: 1, Mode: "native"}
	if len(request.Native) > 0 {
		if err := config.ValidateAntigravityNative(request.Native); err != nil {
			return err
		}
		_ = json.Unmarshal(request.Native, &wanted)
	}
	var n struct {
		Mode     string `json:"mode"`
		Dispatch *struct {
			Method  string  `json:"method"`
			Command *string `json:"command"`
		} `json:"dispatch"`
		ObservedAgents        *int  `json:"observedAgents"`
		CollaborationObserved *bool `json:"collaborationObserved"`
		MetadataTruncated     *bool `json:"metadataTruncated"`
		Limit                 *struct {
			MaxObservedAgents *int  `json:"maxObservedAgents"`
			Hard              *bool `json:"hard"`
			Exceeded          *bool `json:"exceeded"`
		} `json:"limit"`
		Completion *struct {
			Scope    string  `json:"scope"`
			State    string  `json:"state"`
			Source   *string `json:"source"`
			Children string  `json:"children"`
		} `json:"completion"`
		Children *[]struct {
			ID    string  `json:"id"`
			State string  `json:"state"`
			Model *string `json:"model"`
		} `json:"children"`
		AggregateUsageStatus string             `json:"aggregateUsageStatus"`
		Artifacts            *[]json.RawMessage `json:"artifacts"`
	}
	if json.Unmarshal(raw, &n) != nil || n.Mode != wanted.Mode || n.Dispatch == nil || n.ObservedAgents == nil || n.CollaborationObserved == nil || n.MetadataTruncated == nil || n.Limit == nil || n.Limit.Hard == nil || n.Limit.Exceeded == nil ||
		n.Completion == nil || n.Completion.Scope != "cli-invocation" || n.Completion.Children != "unknown" || !slices.Contains([]string{"not-started", "completed", "failed", "unknown"}, n.Completion.State) ||
		n.Children == nil || len(*n.Children) > 1000 && !(len(*n.Children) == 1001 && *n.MetadataTruncated && !success) || n.AggregateUsageStatus != "unknown" || n.Artifacts == nil {
		return fmt.Errorf("invalid request: Antigravity native receipt omitted or contradicted observation facts")
	}
	method := map[string]string{"native": "prompt", "subagents": "subagent-instruction", "swarm": "slash-command"}[wanted.Mode]
	command := (*string)(nil)
	if wanted.Mode == "swarm" {
		command = new("/swarm")
	}
	count := len(*n.Children)
	exceeded := wanted.MaxObservedAgents != nil && count > *wanted.MaxObservedAgents
	if n.Dispatch.Method != method || !reflect.DeepEqual(n.Dispatch.Command, command) || *n.ObservedAgents != count || *n.CollaborationObserved != (count > 0) ||
		count > 0 && execution.TurnDispatch == "not-dispatched" ||
		!reflect.DeepEqual(n.Limit.MaxObservedAgents, wanted.MaxObservedAgents) || *n.Limit.Hard || *n.Limit.Exceeded != exceeded || request.EvidenceOnly && count > 0 ||
		(n.Completion.State == "completed") != (n.Completion.Source != nil && *n.Completion.Source == "normal-result-and-cli-exit") ||
		n.Completion.Source != nil && *n.Completion.Source != "normal-result-and-cli-exit" ||
		n.Completion.State == "completed" && (execution.Dispatch != "dispatched" || !*execution.TerminalReceived || execution.ExitCode == nil || *execution.ExitCode != 0 || execution.FinishedReason != nil) ||
		success && (n.Completion.State != "completed" || exceeded || *n.MetadataTruncated || wanted.Mode == "subagents" && count == 0) {
		return fmt.Errorf("invalid request: Antigravity native dispatch, observation guard or invocation completion mismatch")
	}
	seen := map[string]bool{}
	for _, child := range *n.Children {
		if !machineOpaqueID(child.ID) || seen[child.ID] || rootID != nil && strings.EqualFold(child.ID, *rootID) || child.State != "unknown" || child.Model != nil {
			return fmt.Errorf("invalid request: Antigravity child metadata duplicated identity or invented unobserved state/model")
		}
		seen[child.ID] = true
	}
	return nil
}
