package config

import (
	"encoding/json"
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

	var c Config
	require.NoError(t, json.Unmarshal(raw, &c), "example config 必须是合法 JSON")
	c.Normalize()
	require.NoError(t, c.Validate(), "example config 必须通过法定校验")

	assert.Nil(t, c.JudgePanel, "昂贵第二意见默认关闭")
	for name, ch := range c.Channels {
		assert.Empty(t, ch.Fallback, "渠道 %s 不自动转付费兜底", name)
	}
	assert.Equal(t, "dsh", c.Routes["default"].Harness)
	assert.Equal(t, "deepseek-flash", c.Routes["default"].Model)
	assert.Equal(t, "deepseek-official", c.Routes["default"].Channel)
	assert.Equal(t, "high", c.Routes["default"].Thinking)
	assert.Equal(t, "dsh", c.Loops["review"].Harness)
	assert.Equal(t, "gpt-6.1-sol", c.Loops["review"].Model)
	assert.Equal(t, "chatgpt", c.Loops["review"].Channel)
	assert.Equal(t, "gpt-6-astra", c.Loops["judge"].Model)
	assert.Equal(t, "kimi", c.Loops["plan"].Harness)
	assert.Equal(t, "high", c.Loops["plan"].Thinking)
	assert.True(t, c.Loops["plan"].ResidentSession)
	assert.Equal(t, "codex", c.Routes["hard"].Harness)
	assert.NotContains(t, c.Routes, "multimodal", "附件能力未验证，不虚构多模态通路")
	assert.NotEmpty(t, c.Verify.TestFilePatterns, "测试识别仍用于抽样")
	assert.Empty(t, c.Verify.ProtectedTestPatterns, "普通测试允许修改；oracle显式保护")
	assert.Empty(t, c.Pricing, "未知成本不能用零价格假装已测")
}
