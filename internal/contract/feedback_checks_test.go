package contract

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFeedbackChecksAreApprovedDiagnosticsSeparateFromAcceptance(t *testing.T) {
	base, err := ParsePacket([]byte(validPacket), "base.md")
	require.NoError(t, err)
	require.Empty(t, base.FeedbackChecks)
	input := strings.Replace(validPacket, "acceptance:", "feedback-checks:\n  - {cmd: 'go test ./config -run TestTimeout', expect: 'exit 0', purpose: '检查超时默认值是否导致失败'}\n  - {script: 'scripts/diagnose.sh', expect: 'exit 1'}\nacceptance:", 1)
	packet, err := ParsePacket([]byte(input), "diagnostic.md")
	require.NoError(t, err)
	require.Equal(t, base.Acceptance, packet.Acceptance, "extra diagnostics must not replace final gates")
	require.Len(t, packet.FeedbackChecks, 2)
	require.Equal(t, "go test ./config -run TestTimeout", packet.FeedbackChecks[0].Command())
	require.Equal(t, "检查超时默认值是否导致失败", packet.FeedbackChecks[0].Purpose)
	require.Equal(t, "scripts/diagnose.sh", packet.FeedbackChecks[1].Command())
	require.Equal(t, "exit 1", packet.FeedbackChecks[1].Expect)
}

func TestFeedbackChecksRejectInvalidOrUnboundedDeclarations(t *testing.T) {
	tooMany := "\n"
	for i := 0; i < 9; i++ {
		tooMany += fmt.Sprintf("  - {cmd: 'check-%d', expect: 'exit 0'}\n", i)
	}
	for name, checks := range map[string]string{
		"null":            " null\n",
		"not-array":       " {cmd: check, expect: 'exit 0'}\n",
		"missing-command": " [{expect: 'exit 0'}]\n",
		"bad-expect":      " [{cmd: check, expect: 'looks good'}]\n",
		"extra-field":     " [{cmd: check, expect: 'exit 0', skip-final-gate: true}]\n",
		"duplicate":       " [{cmd: check, expect: 'exit 0'}, {cmd: check, expect: 'exit 0'}]\n",
		"too-many":        tooMany,
		"empty-purpose":   " [{cmd: check, expect: 'exit 0', purpose: ''}]\n",
		"blank-purpose":   " [{cmd: check, expect: 'exit 0', purpose: '  '}]\n",
		"null-purpose":    " [{cmd: check, expect: 'exit 0', purpose: null}]\n",
		"long-purpose":    fmt.Sprintf(" [{cmd: check, expect: 'exit 0', purpose: '%s'}]\n", strings.Repeat("x", 161)),
	} {
		t.Run(name, func(t *testing.T) {
			input := strings.Replace(validPacket, "acceptance:", "feedback-checks:"+checks+"acceptance:", 1)
			_, err := ParsePacket([]byte(input), "invalid.md")
			require.Error(t, err)
		})
	}
	packet, err := ParsePacket([]byte(strings.Replace(validPacket, "acceptance:", "feedback-checks: []\nacceptance:", 1)), "empty.md")
	require.NoError(t, err)
	require.Empty(t, packet.FeedbackChecks)
}

func TestFeedbackPurposeDoesNotChangeLegacyGateIdentity(t *testing.T) {
	gate := GateEntry{Cmd: "go test ./...", Expect: "exit 0"}
	raw, err := json.Marshal(gate)
	require.NoError(t, err)
	require.JSONEq(t, `{"cmd":"go test ./...","script":"","expect":"exit 0"}`, string(raw))
	input := strings.Replace(validPacket, `cmd: "pnpm test auth", expect: "exit 0"`, `cmd: "pnpm test auth", expect: "exit 0", purpose: "not a gate field"`, 1)
	_, err = ParsePacket([]byte(input), "purpose-is-not-an-acceptance-field.md")
	require.Error(t, err)
}
