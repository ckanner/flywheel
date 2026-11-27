package drivers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAntigravityTurnUsageAndCrossDriverContract(t *testing.T) {
	// 1. ACP per-turn usage (scope == "turn", status == "complete") maps into canonical Usage
	// where Input = inputTokens + cacheReadTokens so CacheRead / Input matches Codex/Claude.
	r := agyRequest(t)
	m := agyReceipt(r)
	m["usage"] = map[string]any{
		"status":              "complete",
		"scope":               "turn",
		"inputTokens":         140305,
		"outputTokens":        51114,
		"thinkingTokens":      22532,
		"cacheReadTokens":     941195,
		"cacheCreationTokens": nil,
		"totalTokens":         1132614,
	}
	p := parseAgyFixture(r, m)
	require.True(t, p.Settled, "%+v", p)
	require.NotNil(t, p.Usage)
	assert.Equal(t, "complete", p.Session.UsageStatus)
	assert.Equal(t, int64(140305+941195), p.Usage.Input)
	assert.Equal(t, int64(941195), p.Usage.CacheRead)
	assert.Equal(t, int64(51114), p.Usage.Output)
	assert.Equal(t, int64(22532), p.Usage.Reasoning)
	assert.Equal(t, int64(1132614), p.Usage.TotalTokens)
	assert.LessOrEqual(t, p.Usage.CacheRead, p.Usage.Input, "cross-driver invariant: CacheRead <= Input")

	// 2. Partial turn usage or conversation-scoped usage stays unknown to avoid inaccurate accounting.
	partial := agyReceipt(r)
	partial["usage"] = map[string]any{
		"status":          "partial",
		"scope":           "turn",
		"inputTokens":     100,
		"outputTokens":    nil,
		"thinkingTokens":  nil,
		"cacheReadTokens": 0,
		"totalTokens":     nil,
	}
	pp := parseAgyFixture(r, partial)
	require.True(t, pp.Settled)
	assert.Nil(t, pp.Usage)
	assert.Equal(t, "unknown", pp.Session.UsageStatus)

	// 3. Invalid scope is rejected.
	bad := agyReceipt(r)
	bad["usage"] = map[string]any{
		"status":          "complete",
		"scope":           "invalid",
		"inputTokens":     10,
		"outputTokens":    5,
		"thinkingTokens":  0,
		"cacheReadTokens": 0,
		"totalTokens":     15,
	}
	pb := parseAgyFixture(r, bad)
	assert.False(t, pb.Settled)
	assert.Contains(t, pb.Error, "usage shape is invalid")
}
