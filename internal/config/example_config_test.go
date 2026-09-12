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

	// 双 judge 面板：顶层键，启用且绑定完整（A3 头号特性的开关）。v0.22 复核
	// 面板换防 GPT-6-Astra/Codex。
	require.NotNil(t, c.JudgePanel, "judge-panel 必须在顶层且存在")
	assert.Equal(t, "codex", c.JudgePanel.Harness)
	assert.Equal(t, "gpt-6-astra", c.JudgePanel.Model)
	assert.Equal(t, "chatgpt-subscription", c.JudgePanel.Channel)

	// v0.19 渠道收敛：无 fallback provider；v0.22 三渠道全部订阅委托（零 key）。
	for name, ch := range c.Channels {
		assert.Empty(t, ch.Fallback, "渠道 %s 不得声明 fallback（v0.19）", name)
	}
	require.Contains(t, c.Channels, "zai-coding-plan")
	require.Contains(t, c.Channels, "chatgpt-subscription")
	require.Contains(t, c.Channels, "kimi-subscription")
	assert.Nil(t, c.Channels["kimi-subscription"].EnvKey, "订阅 OAuth 无 key")
	assert.Nil(t, c.Channels["zai-coding-plan"].EnvKey, "Z.AI 凭证留 zcode 用户侧，编排器不注入 key")
	assert.Nil(t, c.Channels["chatgpt-subscription"].EnvKey, "ChatGPT 订阅 OAuth 无 key")
	assert.Equal(t, "omp", c.Channels["zhipu-coding-plan"].Harness, "B 通道经 omp（应急）")

	// Review → Codex/GPT-6-Astra（v0.22 换防）。
	review := c.Loops["review"]
	assert.Equal(t, "codex", review.Harness)
	assert.Equal(t, "gpt-6-astra", review.Model)
	assert.Equal(t, "chatgpt-subscription", review.Channel)

	// Judge 主判走 kimi code harness + 订阅。
	judge := c.Loops["judge"]
	assert.Equal(t, "kimi", judge.Harness)
	assert.Equal(t, "kimi-code/k3", judge.Model)
	assert.Equal(t, "kimi-subscription", judge.Channel)

	// default → zcode/GLM-5.3 旗舰；multimodal → zcode/flash（v0.22 路由表）。
	def := c.Routes["default"]
	assert.Equal(t, "zcode", def.Harness)
	assert.Equal(t, "zai/glm-5.3", def.Model)
	assert.Equal(t, "zai-coding-plan", def.Channel)
	mm := c.Routes["multimodal"]
	assert.Equal(t, "zcode", mm.Harness)
	assert.Equal(t, "zai/glm-5.3-flash", mm.Model, "multimodal 是 zcode 目录唯一多模态档")

	// hard 路由维持 K3（跨厂商制衡：GPT-6 已任 Review，§4.2）。
	hard := c.Routes["hard"]
	assert.Equal(t, "kimi-subscription", hard.Channel)
	assert.Equal(t, "kimi-code/k3", hard.Model)

	// 测试防篡改闸门的模式表不得为空（空切片=静默关闭 DC1=B 执行机制）。
	assert.NotEmpty(t, c.Verify.TestFilePatterns, "test-file-patterns 空数组会关掉 executor 改测试判死")
}
