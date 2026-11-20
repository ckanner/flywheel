package config

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// AntigravityOrchestrator identifies only the official fixed-effort CLI slugs.
// These models manage their own internal effort; an override is not supported.
func AntigravityOrchestrator(model string) bool {
	return slices.Contains([]string{"gemini-pro-orchestrator-neon", "gemini-pro-orchestrator", "gemini-pro-orchestrator-argon"}, model)
}

// AntigravityNative describes the public native collaboration request. The
// observation guard is not a hard concurrency or whole-tree completion limit.
type AntigravityNative struct {
	Version           int    `json:"version"`
	Mode              string `json:"mode"`
	MaxObservedAgents *int   `json:"maxObservedAgents,omitempty"`
}

// ValidateAntigravityNative shares the configuration schema with direct driver
// admission, so programmatic requests cannot bypass native profile validation.
func ValidateAntigravityNative(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var options any
	if err := json.Unmarshal(raw, &options); err != nil {
		return fmt.Errorf("antigravity native options: %w", err)
	}
	return validateNativeProfiles(map[string]any{"request": map[string]any{"harness": "antigravity", "options": options}})
}

// The native CLI model slug pins its effort. A separate thinking value may not
// contradict it. Catalog/account availability is still checked by the CLI.
func ValidateAntigravityBinding(model, effort string) error {
	if AntigravityOrchestrator(model) {
		if effort != "" {
			return fmt.Errorf("antigravity Orchestrator uses fixed native effort; thinking must be empty")
		}
		return nil
	}
	parts := strings.Split(model, "-")
	if len(parts) != 4 || parts[0] != "gemini" || parts[1] != "4" ||
		!slices.Contains([]string{"hydrogen", "neon", "carbon", "argon"}, parts[2]) {
		return fmt.Errorf("antigravity requires an explicit Gemini 4 model-and-effort slug")
	}
	levels := []string{"low", "medium", "high", "xhigh"}
	if parts[2] == "carbon" || parts[2] == "argon" {
		levels = levels[:3]
	}
	if !slices.Contains(levels, parts[3]) || effort != parts[3] {
		return fmt.Errorf("antigravity model slug and supported thinking level must agree")
	}
	return nil
}
