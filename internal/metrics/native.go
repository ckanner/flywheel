package metrics

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

// NativeObservation is a bounded, metadata-only view. CLI invocation completion
// is independent of child completion, which Antigravity has not established.
type NativeObservation struct {
	Version               int      `json:"version"`
	RequestedMode         string   `json:"requestedMode"`
	DispatchMethod        string   `json:"dispatchMethod"`
	ObservedAgents        int      `json:"observedAgents"`
	CollaborationObserved bool     `json:"collaborationObserved"`
	MetadataTruncated     bool     `json:"metadataTruncated"`
	MaxObservedAgents     *int     `json:"maxObservedAgents"`
	HardLimit             bool     `json:"hardLimit"`
	GuardExceeded         bool     `json:"guardExceeded"`
	CompletionScope       string   `json:"completionScope"`
	InvocationCompletion  string   `json:"invocationCompletion"`
	ChildCompletion       string   `json:"childCompletion"`
	AggregateUsageStatus  string   `json:"aggregateUsageStatus"`
	ChildIDs              []string `json:"childIds"`
	OmittedChildren       int      `json:"omittedChildren"`
}

var nativeChildID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func projectAntigravityNative(raw []byte, harness string) (*NativeObservation, error) {
	if harness != "antigravity" {
		return nil, nil
	}
	var envelope struct {
		Receipt *struct {
			OK      *bool `json:"ok"`
			Session struct {
				ID *string `json:"id"`
			} `json:"session"`
			Requested struct {
				Native *struct {
					Version           int    `json:"version"`
					Mode              string `json:"mode"`
					MaxObservedAgents *int   `json:"maxObservedAgents"`
				} `json:"native"`
			} `json:"requested"`
			Native json.RawMessage `json:"native"`
		} `json:"antigravity-receipt"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("invalid Antigravity metrics receipt: %w", err)
	}
	if envelope.Receipt == nil || len(envelope.Receipt.Native) == 0 || string(envelope.Receipt.Native) == "null" {
		return nil, nil
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
			Children string  `json:"children"`
			Source   *string `json:"source"`
		} `json:"completion"`
		Children *[]struct {
			ID    string  `json:"id"`
			State string  `json:"state"`
			Model *string `json:"model"`
		} `json:"children"`
		AggregateUsageStatus string `json:"aggregateUsageStatus"`
	}
	if err := json.Unmarshal(envelope.Receipt.Native, &n); err != nil {
		return nil, fmt.Errorf("invalid Antigravity native metrics: %w", err)
	}
	// Version 0.1 retained child metadata without a collaboration contract. It
	// cannot establish requested mode or dispatch, so leave this view absent.
	if n.Mode == "" && envelope.Receipt.Requested.Native == nil {
		return nil, nil
	}
	mode := "native"
	var limit *int
	if requested := envelope.Receipt.Requested.Native; requested != nil {
		if requested.Version != 1 || !slices.Contains([]string{"native", "subagents", "swarm"}, requested.Mode) || requested.MaxObservedAgents != nil && (*requested.MaxObservedAgents < 1 || *requested.MaxObservedAgents > 8) {
			return nil, fmt.Errorf("invalid Antigravity requested native metrics")
		}
		mode, limit = requested.Mode, requested.MaxObservedAgents
	}
	if envelope.Receipt.OK == nil || n.Mode != mode || n.Dispatch == nil || n.ObservedAgents == nil || n.CollaborationObserved == nil || n.MetadataTruncated == nil || n.Limit == nil || n.Limit.Hard == nil || n.Limit.Exceeded == nil ||
		n.Completion == nil || n.Completion.Scope != "cli-invocation" || n.Completion.Children != "unknown" || !slices.Contains([]string{"not-started", "completed", "failed", "unknown"}, n.Completion.State) ||
		n.Children == nil || len(*n.Children) > 1000 && !(len(*n.Children) == 1001 && *n.MetadataTruncated && !*envelope.Receipt.OK) || n.AggregateUsageStatus != "unknown" {
		return nil, fmt.Errorf("invalid Antigravity native observation shape")
	}
	method := map[string]string{"native": "prompt", "subagents": "subagent-instruction", "swarm": "slash-command"}[mode]
	var command *string
	if mode == "swarm" {
		command = new("/swarm")
	}
	count := len(*n.Children)
	if n.Dispatch.Method != method || !reflect.DeepEqual(n.Dispatch.Command, command) || *n.ObservedAgents != count || *n.CollaborationObserved != (count > 0) ||
		!reflect.DeepEqual(limit, n.Limit.MaxObservedAgents) || *n.Limit.Hard || *n.Limit.Exceeded != (limit != nil && count > *limit) ||
		*envelope.Receipt.OK && (*n.MetadataTruncated || *n.Limit.Exceeded || n.Completion.State != "completed" || mode != "native" && count == 0) ||
		(n.Completion.State == "completed") != (n.Completion.Source != nil && *n.Completion.Source == "normal-result-and-cli-exit") || n.Completion.Source != nil && *n.Completion.Source != "normal-result-and-cli-exit" {
		return nil, fmt.Errorf("contradictory Antigravity native observations")
	}
	out := &NativeObservation{Version: 1, RequestedMode: mode, DispatchMethod: method, ObservedAgents: count, CollaborationObserved: count > 0, MaxObservedAgents: limit,
		MetadataTruncated: *n.MetadataTruncated, GuardExceeded: *n.Limit.Exceeded, CompletionScope: "cli-invocation", InvocationCompletion: n.Completion.State, ChildCompletion: "unknown", AggregateUsageStatus: "unknown", ChildIDs: []string{}, OmittedChildren: max(0, count-20)}
	seen := map[string]bool{}
	for _, child := range *n.Children {
		if !nativeChildID.MatchString(child.ID) || seen[child.ID] || envelope.Receipt.Session.ID != nil && strings.EqualFold(child.ID, *envelope.Receipt.Session.ID) || child.State != "unknown" || child.Model != nil {
			return nil, fmt.Errorf("invalid Antigravity child observation identity or unverified state/model")
		}
		seen[child.ID] = true
		if len(out.ChildIDs) < 20 {
			out.ChildIDs = append(out.ChildIDs, child.ID)
		}
	}
	return out, nil
}
