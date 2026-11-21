package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExampleConfigRoundTrip 解析仓库随附的 templates/flywheel.config.example.json
// （PR #3 review 教训：judge-panel 曾被嵌进 loops 层导致面板永不激活，而没有任何
// 测试解析这份文件——门禁因此抓不到）。本测试把 example 当真实用户输入对待：
// Unmarshal → Normalize → Validate，再断言关键语义字段。
func TestExampleConfigRoundTrip(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "templates", "flywheel.config.example.json"))
	require.NoError(t, err)

	c, err := parseConfig(raw, "embedded example")
	require.NoError(t, err, "example must use the same profile expansion and strict validation as a user file")

	assert.Nil(t, c.JudgePanel, "昂贵第二意见默认关闭")
	assert.Equal(t, "parallel", c.Scheduler.Mode)
	assert.Equal(t, 2, c.Scheduler.MaxParallel)
	assert.Equal(t, "batch", c.Scheduler.Dispatch, "P5 不自动改变现有批次调度")
	assert.False(t, c.Scheduler.Prewarm, "预热缺省关闭，实际收益另行验收")
	for name, ch := range c.Channels {
		assert.Empty(t, ch.Fallback, "渠道 %s 不自动转付费兜底", name)
	}
	assert.Equal(t, "antigravity", c.Routes["default"].Harness)
	assert.Equal(t, "gemini-4-hydrogen-high", c.Routes["default"].Model)
	assert.Equal(t, "google-subscription", c.Routes["default"].Channel)
	assert.Empty(t, c.Routes["default"].Provider)
	assert.Equal(t, "high", c.Routes["default"].Thinking)
	assert.Equal(t, "low", c.Loops["intake"].Thinking)
	assert.Equal(t, "gemini-4-neon-high", c.Loops["investigate"].Model)
	assert.Equal(t, "codex-pro", c.Loops["review"].Harness)
	assert.Equal(t, "gpt-6.2-sol", c.Loops["review"].Model)
	assert.Equal(t, "gpt-6.2-astra", c.Loops["judge"].Model)
	assert.Equal(t, "claude-code-pro", c.Loops["plan"].Harness)
	assert.Equal(t, "high", c.Loops["plan"].Thinking)
	assert.False(t, c.Loops["plan"].ResidentSession)
	assert.Equal(t, "codex-pro", c.Routes["hard"].Harness)
	assert.Equal(t, "claude-code-pro", c.Routes["multimodal"].Harness)
	assert.Equal(t, &SubscriptionPolicy{Version: 1, Mode: "subscription-only"}, c.Subscription)
	assert.Equal(t, "hydrogen-local", c.Planning.Compact["design-plan"])
	assert.NotEmpty(t, c.Verify.TestFilePatterns, "测试识别仍用于抽样")
	assert.Empty(t, c.Verify.ProtectedTestPatterns, "普通测试允许修改；oracle显式保护")
	assert.Empty(t, c.Pricing, "未知成本不能用零价格假装已测")
}
