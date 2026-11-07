package drivers

import (
	"testing"

	"github.com/ckanner/flywheel/internal/errs"
	"github.com/stretchr/testify/require"
)

func TestClaudeCodeProReviewRejectsContradictoryTerminalFacts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"success with failed process", func(r map[string]any) {
			proObject(r, "execution")["exitCode"] = 9
		}},
		{"success without cleanup", func(r map[string]any) {
			proObject(r, "cleanup")["confirmed"] = false
		}},
		{"success without terminal", func(r map[string]any) {
			proObject(r, "execution")["terminalReceived"] = false
		}},
		{"cancelled success", func(r map[string]any) {
			proObject(r, "execution")["cancelled"] = true
		}},
		{"timed out success", func(r map[string]any) {
			proObject(r, "execution")["timedOut"] = true
		}},
		{"failed result offers resumable history", func(r map[string]any) {
			r["ok"] = false
			r["output"] = []any{}
			r["failure"] = map[string]any{"stage": "query-run", "category": "product-error", "detail": "failed turn"}
			proObject(r, "execution")["outcome"] = "failed"
			proObject(r, "execution")["exitCode"] = 1
			proObject(r, "session")["resumable"] = true
		}},
		{"failed result contains successful output", func(r map[string]any) {
			r["ok"] = false
			r["failure"] = map[string]any{"stage": "query-run", "category": "product-error", "detail": "failed turn"}
			proObject(r, "execution")["outcome"] = "failed"
			proObject(r, "execution")["exitCode"] = 1
			proObject(r, "session")["resumable"] = false
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := claudeCodeProRequest(t)
			request.Protocol, request.ThreadMode = 2, "persistent"
			receipt := claudeCodeProFixture(request)
			tc.mutate(receipt)
			parsed := parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(request, receipt)), request)
			require.False(t, parsed.Settled)
			require.NotEmpty(t, parsed.Error)
			require.Equal(t, "unknown", parsed.Session.Outcome, "a contradictory terminal must not authorize another invocation")
			require.False(t, parsed.Session.ResumeCandidate)
			require.Nil(t, parsed.Usage, "contradictory success evidence must not produce trusted charges")
			require.Equal(t, "unknown", parsed.Session.UsageStatus)
		})
	}
}

func TestClaudeCodeProReviewAccountsTrustedTerminalWithoutGrantingReplay(t *testing.T) {
	for _, tc := range []struct {
		name     string
		category string
		flag     string
	}{
		{"cleanup unconfirmed", "cleanup-unconfirmed", ""},
		{"cancelled after terminal", "cancelled", "cancelled"},
		{"timed out after terminal", "timeout", "timedOut"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := claudeCodeProRequest(t)
			request.Protocol, request.ThreadMode = 2, "persistent"
			receipt := claudeCodeProFixture(request)
			receipt["ok"], receipt["output"] = false, []any{}
			receipt["failure"] = map[string]any{"stage": "run", "category": tc.category, "detail": tc.name}
			proObject(receipt, "session")["resumable"] = false
			if tc.flag == "" {
				proObject(receipt, "cleanup")["confirmed"] = false
			} else {
				proObject(receipt, "execution")[tc.flag] = true
			}
			parsed := parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(request, receipt)), request)
			require.False(t, parsed.Settled)
			require.False(t, parsed.Session.ResumeCandidate)
			require.False(t, parsed.Session.CleanupConfirmed && parsed.Session.Outcome != "unknown",
				"accounting certainty cannot authorize replay after cancellation, timeout or unknown cleanup")
			require.Equal(t, "complete", parsed.Session.UsageStatus)
			require.Equal(t, &Usage{Input: 17, Output: 5, CacheRead: 4, CacheWrite: 3, TotalTokens: 22}, parsed.Usage)
			require.NotEmpty(t, parsed.Receipt, "the original terminal remains available for audit")
		})
	}
}

func TestClaudeCodeProReviewSeparatesTerminalFailureFromDamagedProtocol(t *testing.T) {
	for _, category := range []string{"auth", "protocol"} {
		t.Run(category, func(t *testing.T) {
			request := claudeCodeProRequest(t)
			receipt := claudeCodeProFixture(request)
			receipt["ok"], receipt["output"] = false, []any{}
			receipt["failure"] = map[string]any{"stage": "query-run", "category": category, "detail": "failed"}
			proObject(receipt, "execution")["outcome"] = "failed"
			proObject(receipt, "execution")["exitCode"] = 1
			proObject(receipt, "session")["resumable"] = false
			parsed := parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(request, receipt)), request)
			require.False(t, parsed.Settled)
			require.True(t, parsed.CleanupConfirmed)
			if category == "auth" {
				require.Equal(t, "failed", parsed.Session.Outcome)
				require.Equal(t, errs.AUTH, parsed.ErrorCode)
				require.NotNil(t, parsed.Usage)
			} else {
				require.Equal(t, "unknown", parsed.Session.Outcome)
				require.Nil(t, parsed.Usage)
				require.Equal(t, "unknown", parsed.Session.UsageStatus)
			}
		})
	}
}

func TestClaudeCodeProReviewChecksObservedModelRatherThanMatchedFlag(t *testing.T) {
	for _, tc := range []struct {
		requested string
		observed  string
		accepted  bool
	}{
		{"sonnet", "claude-sonnet-4-6", true},
		{"sonnet", "claude-sonnet-4-6[1m]", true},
		{"opus", "claude-opus-4-6", true},
		{"haiku", "claude-haiku-4-5", true},
		{"claude-opus-custom", "claude-opus-custom", true},
		{"corporate-alias", "corporate-alias", true},
		{"sonnet", "claude-opus-4-6", false},
		{"haiku", "claude-sonnet-4-6", false},
		{"corporate-alias", "claude-opus-4-6", false},
		{"sonnet", "claude-sonnet-", false},
		{"sonnet", "claude-sonnet-4-6[other]", false},
		{"sonnet", "", false},
	} {
		t.Run(tc.requested+"/"+tc.observed, func(t *testing.T) {
			request := claudeCodeProRequest(t)
			request.Model = tc.requested
			receipt := claudeCodeProFixture(request)
			proObject(receipt, "configuration", "observed")["model"] = tc.observed
			parsed := parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(request, receipt)), request)
			require.Equal(t, tc.accepted, parsed.Settled)
			if !tc.accepted {
				require.Equal(t, "unknown", parsed.Session.Outcome)
				require.Nil(t, parsed.Usage)
			}
		})
	}
}
