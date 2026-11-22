package drivers

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type proNativeChild struct {
	ID       string          `json:"id"`
	ParentID *string         `json:"parentId"`
	OwnerID  *string         `json:"ownerId"`
	Kind     string          `json:"kind"`
	State    string          `json:"state"`
	TurnID   *string         `json:"turnId"`
	Usage    json.RawMessage `json:"usage"`
}

type proNativeReceipt struct {
	Version   int             `json:"version"`
	Requested json.RawMessage `json:"requested"`
	Effective *struct {
		Source       *string         `json:"source"`
		Matched      *bool           `json:"matched"`
		Verification string          `json:"verification"`
		Config       json.RawMessage `json:"config"`
	} `json:"effective"`
	Observed *struct {
		Events    *[]json.RawMessage `json:"events"`
		Truncated *bool              `json:"truncated"`
	} `json:"observed"`
	Children   *[]proNativeChild `json:"children"`
	Usage      json.RawMessage   `json:"usage"`
	Completion *struct {
		Confirmed  *bool     `json:"confirmed"`
		PendingIDs *[]string `json:"pendingIds"`
	} `json:"completion"`
	Cleanup *struct {
		Confirmed  *bool     `json:"confirmed"`
		Scope      string    `json:"scope"`
		PendingIDs *[]string `json:"pendingIds"`
	} `json:"cleanup"`
}

type proNativeOutcome struct {
	Usage      *Usage
	Status     string
	Cleanup    bool
	Completion bool
}

func proJSONEqual(a, b json.RawMessage) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}

func validateProNativeRequest(harness string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var options map[string]json.RawMessage
	if json.Unmarshal(raw, &options) != nil || options == nil || string(options["version"]) != "1" {
		return fmt.Errorf("native requires a version 1 options object")
	}
	if harness == "codex-pro" && len(options["review"]) != 0 {
		return fmt.Errorf("native review is an independent job; formal loops must retain their role prompt")
	}
	return nil
}

// Native metadata is neither a lifecycle checkpoint nor an alternate answer.
func validateProNativeEvent(raw json.RawMessage, harness string, request codexProStart, previous *int64) error {
	var frame struct {
		Native struct {
			Version  int     `json:"version"`
			Sequence int64   `json:"sequence"`
			ID       string  `json:"id"`
			ParentID *string `json:"parentId"`
			Kind     string  `json:"kind"`
			State    string  `json:"state"`
		} `json:"native"`
		Adapter struct {
			Package  string `json:"package"`
			Version  string `json:"version"`
			Protocol int    `json:"protocol"`
		} `json:"adapter"`
		Managed *NativeAdmission `json:"managed"`
	}
	version := CodexProPackageVersion
	if harness == "claude-code-pro" {
		version = ClaudeCodeProPackageVersion
	}
	if len(request.Native) == 0 || json.Unmarshal(raw, &frame) != nil || frame.Native.Version != 1 ||
		frame.Native.Sequence != *previous+1 || frame.Native.Sequence > 4096 || !machineOpaqueID(frame.Native.ID) ||
		!machineOpaqueID(frame.Native.Kind) || !machineOpaqueID(frame.Native.State) ||
		(frame.Native.ParentID != nil && !machineOpaqueID(*frame.Native.ParentID)) ||
		frame.Adapter.Package != adapterPackageFor(harness) || frame.Adapter.Version != version || frame.Adapter.Protocol != 1 ||
		!reflect.DeepEqual(frame.Managed, request.Managed) {
		return fmt.Errorf("invalid request: native child event identity or sequence mismatch")
	}
	*previous = frame.Native.Sequence
	return nil
}

// Root usage remains independently auditable in the complete provider receipt.
// Only exclusive, attributable child deltas can form a normalized family total.
func foldProNative(harness string, requested, raw json.RawMessage, success, localCleanup, notDispatched bool,
	rootID string, rootUsage *Usage, rootStatus string) (proNativeOutcome, error) {
	out := proNativeOutcome{Usage: rootUsage, Status: rootStatus, Cleanup: localCleanup, Completion: true}
	if len(requested) == 0 {
		if len(raw) != 0 && string(raw) != "null" {
			return out, fmt.Errorf("invalid request: unexpected native capability receipt")
		}
		return out, nil
	}
	if len(raw) == 0 || string(raw) == "null" {
		if success {
			return out, fmt.Errorf("invalid request: native success omitted its capability receipt")
		}
		if !notDispatched {
			out.Usage, out.Status, out.Cleanup, out.Completion = nil, "unknown", false, false
		}
		return out, nil
	}
	var receipt proNativeReceipt
	if json.Unmarshal(raw, &receipt) != nil || receipt.Version != 1 || !proJSONEqual(requested, receipt.Requested) ||
		receipt.Effective == nil || receipt.Effective.Matched == nil || receipt.Observed == nil || receipt.Observed.Events == nil ||
		receipt.Observed.Truncated == nil || receipt.Children == nil || len(*receipt.Children) > 4096 || len(*receipt.Observed.Events) > 4096 ||
		receipt.Completion == nil || receipt.Completion.Confirmed == nil || receipt.Completion.PendingIDs == nil ||
		receipt.Cleanup == nil || receipt.Cleanup.Confirmed == nil || receipt.Cleanup.PendingIDs == nil ||
		(receipt.Cleanup.Scope != "local-process-group" && (harness != "claude-code-pro" || receipt.Cleanup.Scope != "remote-job")) {
		return out, fmt.Errorf("invalid request: native receipt omitted or contradicted requested capability evidence")
	}
	out.Cleanup = localCleanup && *receipt.Cleanup.Confirmed
	out.Completion = *receipt.Completion.Confirmed
	if (*receipt.Cleanup.Confirmed && !localCleanup) ||
		(*receipt.Cleanup.Confirmed && len(*receipt.Cleanup.PendingIDs) != 0) ||
		(out.Completion && (len(*receipt.Completion.PendingIDs) != 0 || *receipt.Observed.Truncated)) ||
		(success && (!out.Cleanup || !out.Completion || !*receipt.Effective.Matched || receipt.Effective.Source == nil ||
			strings.TrimSpace(*receipt.Effective.Source) == "" || len(receipt.Effective.Config) == 0 || string(receipt.Effective.Config) == "null")) {
		return out, fmt.Errorf("invalid request: native success, configuration, completion or cleanup evidence is contradictory")
	}
	if success && ((harness == "codex-pro" && *receipt.Effective.Source != "config/read") ||
		(harness == "claude-code-pro" && (*receipt.Effective.Source != "cli-init+invocation" || receipt.Effective.Verification != "tools-only"))) {
		return out, fmt.Errorf("invalid request: native effective configuration has an unsupported evidence source")
	}
	children := *receipt.Children
	identities, ids := map[string]bool{}, map[string]bool{}
	for _, child := range children {
		key := child.ID
		if harness == "claude-code-pro" && child.TurnID != nil {
			key += "\x00" + *child.TurnID
		}
		if !machineOpaqueID(child.ID) || child.ID == rootID || identities[key] || !machineOpaqueID(child.Kind) || !machineOpaqueID(child.State) ||
			(child.ParentID != nil && !machineOpaqueID(*child.ParentID)) || (child.TurnID != nil && !machineOpaqueID(*child.TurnID)) {
			return out, fmt.Errorf("invalid request: native child identity is missing, duplicated or aliases the root")
		}
		identities[key], ids[child.ID] = true, true
		if out.Completion && !proNativeTerminal(harness, child.State) {
			return out, fmt.Errorf("invalid request: native completion includes a pending child")
		}
	}
	for _, pending := range append(append([]string{}, (*receipt.Completion.PendingIDs)...), (*receipt.Cleanup.PendingIDs)...) {
		if !ids[pending] {
			return out, fmt.Errorf("invalid request: native pending identity is absent from its owned ledger")
		}
	}
	if notDispatched {
		if len(children) != 0 || len(*receipt.Observed.Events) != 0 {
			return out, fmt.Errorf("invalid request: native children contradict non-dispatch proof")
		}
		return out, nil
	}
	if harness == "codex-pro" {
		return foldCodexNative(receipt, rootID, rootUsage, rootStatus, out)
	}
	return foldClaudeNative(receipt, requested, rootUsage, rootStatus, out)
}

func proNativeTerminal(harness, state string) bool {
	if harness == "codex-pro" {
		return state == "completed" || state == "failed" || state == "closed" || state == "shutdown" || state == "errored" || state == "interrupted" || state == "notFound"
	}
	return state == "completed" || state == "failed" || state == "stopped" || state == "killed" || state == "returned"
}

func foldCodexNative(receipt proNativeReceipt, rootID string, root *Usage, status string, out proNativeOutcome) (proNativeOutcome, error) {
	var usage struct {
		Strategy string                     `json:"strategy"`
		Scope    string                     `json:"scope"`
		Status   string                     `json:"status"`
		Children map[string]json.RawMessage `json:"children"`
		Total    *int64                     `json:"totalTokens"`
	}
	if json.Unmarshal(receipt.Usage, &usage) != nil || usage.Strategy != "per-thread" || usage.Scope != "observed-children" ||
		(usage.Status != "complete" && usage.Status != "partial" && usage.Status != "unknown") || usage.Children == nil ||
		!receiptHasFields(receipt.Usage, "totalTokens") || len(usage.Children) != len(*receipt.Children) {
		return out, fmt.Errorf("invalid request: Codex native usage is not an exclusive child report")
	}
	var controls struct {
		Agents struct {
			Enabled bool `json:"enabled"`
		} `json:"agents"`
	}
	_ = json.Unmarshal(receipt.Requested, &controls) // requested was already matched as valid JSON.
	if !controls.Agents.Enabled && len(*receipt.Children) != 0 {
		return out, fmt.Errorf("invalid request: Codex native children were observed with dispatch disabled")
	}
	if len(*receipt.Children) == 0 {
		if usage.Status != "unknown" || usage.Total != nil {
			return out, fmt.Errorf("invalid request: Codex no-child observation invented a usage total")
		}
		if controls.Agents.Enabled {
			out.Usage, out.Status = nil, "unknown"
		}
		return out, nil
	}
	byID := map[string]proNativeChild{}
	for _, child := range *receipt.Children {
		byID[child.ID] = child
	}
	owned := func(child proNativeChild) bool {
		seen := map[string]bool{}
		for child.ParentID != nil {
			if *child.ParentID == rootID && rootID != "" {
				return true
			}
			if seen[*child.ParentID] {
				return false
			}
			seen[*child.ParentID] = true
			var ok bool
			child, ok = byID[*child.ParentID]
			if !ok {
				return false
			}
		}
		return false
	}
	complete := !*receipt.Observed.Truncated
	var known Usage
	for _, child := range *receipt.Children {
		if !proJSONEqual(child.Usage, usage.Children[child.ID]) {
			return out, fmt.Errorf("invalid request: Codex native usage map differs from its child ledger")
		}
		value, err := parseCodexProUsage(child.Usage)
		if err != nil {
			return out, err
		}
		if value == nil || !owned(child) {
			complete = false
			continue
		}
		if !addProUsage(&known, value) {
			return out, fmt.Errorf("invalid request: Codex native usage exceeds safe counters")
		}
	}
	if usage.Status == "complete" && (!complete || usage.Total == nil || *usage.Total != known.TotalTokens) {
		return out, fmt.Errorf("invalid request: Codex native complete usage is not attributable or contradicts its total")
	}
	if usage.Status != "complete" && usage.Total != nil {
		return out, fmt.Errorf("invalid request: Codex partial native usage fabricated a total")
	}
	if !complete || !out.Completion || usage.Status != "complete" || root == nil || status != "complete" {
		out.Usage, out.Status = nil, "partial"
		return out, nil
	}
	total := *root
	if !addProUsage(&total, &known) {
		return out, fmt.Errorf("invalid request: Codex family usage exceeds safe counters")
	}
	out.Usage, out.Status = &total, "complete"
	return out, nil
}

func addProUsage(total, delta *Usage) bool {
	for _, pair := range [][2]*int64{{&total.Input, &delta.Input}, {&total.Output, &delta.Output}, {&total.Reasoning, &delta.Reasoning},
		{&total.CacheRead, &delta.CacheRead}, {&total.CacheWrite, &delta.CacheWrite}, {&total.TotalTokens, &delta.TotalTokens}} {
		if *pair[0] < 0 || *pair[1] < 0 || *pair[0] > (1<<53-1)-*pair[1] {
			return false
		}
		*pair[0] += *pair[1]
	}
	return true
}

func foldClaudeNative(receipt proNativeReceipt, requested json.RawMessage, root *Usage, status string, out proNativeOutcome) (proNativeOutcome, error) {
	var usage struct {
		Strategy  string          `json:"strategy"`
		Status    string          `json:"status"`
		Additive  *bool           `json:"childSnapshotsAreAdditive"`
		Aggregate string          `json:"aggregateFamilyStatus"`
		Root      json.RawMessage `json:"root"`
	}
	if json.Unmarshal(receipt.Usage, &usage) != nil || usage.Strategy != "root-result-only" || usage.Additive == nil || *usage.Additive ||
		(usage.Status != "complete" && usage.Status != "partial" && usage.Status != "unknown") {
		return out, fmt.Errorf("invalid request: Claude native task snapshots cannot be additive usage")
	}
	var controls struct {
		Advisor struct {
			Enabled bool `json:"enabled"`
		} `json:"advisor"`
	}
	_ = json.Unmarshal(requested, &controls)
	// Ordinary task snapshots, historical runs and advisor accounting have no
	// public proof of exclusivity relative to the root's result. Preserve them,
	// but never turn them into an invented full-family bill or a budget zero.
	if len(*receipt.Children) > 0 || *receipt.Observed.Truncated || controls.Advisor.Enabled || usage.Aggregate != "complete" || usage.Status != "complete" {
		out.Usage, out.Status = nil, "unknown"
		return out, nil
	}
	observed, _, err := parseClaudeCodeProUsage(usage.Root)
	if err != nil || !reflect.DeepEqual(observed, root) || status != "complete" {
		return out, fmt.Errorf("invalid request: Claude native root usage contradicts its independently attributed result")
	}
	return out, nil
}
