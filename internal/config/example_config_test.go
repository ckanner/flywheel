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

	// 双 judge 面板：顶层键，启用且绑定完整（A3 头号特性的开关）。
	require.NotNil(t, c.JudgePanel, "judge-panel 必须在顶层且存在")
	assert.Equal(t, "omp", c.JudgePanel.Harness)
	assert.NotEmpty(t, c.JudgePanel.Model)
	assert.Equal(t, "opencode-go", c.JudgePanel.Channel)

	// v0.19 渠道收敛：无 fallback provider；Kimi 系全订阅。
	for name, ch := range c.Channels {
		assert.Empty(t, ch.Fallback, "渠道 %s 不得声明 fallback（v0.19）", name)
	}
	require.Contains(t, c.Channels, "opencode-go")
	require.Contains(t, c.Channels, "kimi-subscription")
	assert.Nil(t, c.Channels["kimi-subscription"].EnvKey, "订阅 OAuth 无 key")

	// Judge 主判走 kimi code harness + 订阅。
	judge := c.Loops["judge"]
	assert.Equal(t, "kimi", judge.Harness)
	assert.Equal(t, "kimi-code/k3", judge.Model)
	assert.Equal(t, "kimi-subscription", judge.Channel)

	// hard 路由与 frontend 同渠道（v0.19）。
	hard := c.Routes["hard"]
	assert.Equal(t, "kimi-subscription", hard.Channel)

	// 测试防篡改闸门的模式表不得为空（空切片=静默关闭 DC1=B 执行机制）。
	assert.NotEmpty(t, c.Verify.TestFilePatterns, "test-file-patterns 空数组会关掉 executor 改测试判死")
}
