package machine

import (
	"strings"
	"unicode"
)

// Titles describe work; opaque request IDs stay in the ledger and receipts.
// Resolve the label before adding orchestration instructions to the prompt.
func (c *Context) sessionTitle(role, scope string) string {
	title := c.State.Slug
	if c.Spec != nil && strings.TrimSpace(c.Spec.Title) != "" {
		title = c.Spec.Title
	} else if c.State.Planning != nil && strings.TrimSpace(c.State.Planning.Brief.Goal) != "" {
		title = c.State.Planning.Brief.Goal
	}
	label := role
	if scope != "" && !strings.HasPrefix(scope, "planning:") {
		label += "/" + scope
	}
	value := "Flywheel · " + sessionTitleText(label, 64) + " · " + sessionTitleText(title, 100)
	// The strictest public companion name contract is 512 UTF-8 bytes.
	if len(value) > 512 {
		runes := []rune(value)
		for len(string(runes))+len("…") > 512 {
			runes = runes[:len(runes)-1]
		}
		value = string(runes) + "…"
	}
	return value
}

func sessionTitleText(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return ' '
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}
	return value
}
