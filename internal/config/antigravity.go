package config

import (
	"fmt"
	"slices"
	"strings"
)

// The native CLI model slug pins its effort. A separate thinking value may not
// contradict it. Catalog/account availability is still checked by the CLI.
func ValidateAntigravityBinding(model, effort string) error {
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
