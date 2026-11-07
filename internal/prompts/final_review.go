package prompts

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ckanner/flywheel/internal/contract"
)

// ParseFinalReviewVerdict requires the last substantive line to be the complete
// final verdict. It never revives an earlier pass when the final line is invalid.
func ParseFinalReviewVerdict(text string, spec *contract.Spec) (*Verdict, error) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("missing final review verdict")
	}
	line := strings.TrimSpace(lines[len(lines)-1])
	line = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(line, "```json"), "```"), "```"))
	var v Verdict
	if err := json.Unmarshal([]byte(line), &v); err != nil {
		return nil, fmt.Errorf("invalid final review JSON: %w", err)
	}
	if err := ValidateFinalReviewVerdict(&v, spec); err != nil {
		return nil, err
	}
	return &v, nil
}

func ValidateFinalReviewVerdict(v *Verdict, spec *contract.Spec) error {
	if (!validVerdicts[v.Verdict] && v.Verdict != "upgrade") || v.Score == nil || *v.Score < 0 || *v.Score > 1 ||
		v.QuickEligible == nil || !nonblankStrings(v.Reasons) {
		return fmt.Errorf("final review requires a valid verdict, score, reasons and explicit Quick eligibility")
	}
	if v.Verdict == "pass" && !*v.QuickEligible || v.Verdict == "upgrade" && *v.QuickEligible {
		return fmt.Errorf("final verdict contradicts Quick eligibility")
	}
	expected := map[string]bool{}
	for _, r := range spec.Requirements {
		expected[r.ID] = true
	}
	for _, r := range v.Requirements {
		if !expected[r.ID] || r.Pass == nil || !nonblankStrings(r.Evidence) || (v.Verdict == "pass" && !*r.Pass) {
			return fmt.Errorf("final review requirement %q is missing, duplicated, unsupported or not proven", r.ID)
		}
		delete(expected, r.ID)
	}
	if len(expected) != 0 {
		return fmt.Errorf("final review omits %d requirements", len(expected))
	}
	return nil
}

func nonblankStrings(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true
}
