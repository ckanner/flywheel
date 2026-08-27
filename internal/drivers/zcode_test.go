package drivers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 成功样本：取自 2026-08-27 本机真实 zcode --json 探针（字段结构不变、数值缩写）。
const fakeZcodeOK = `{
  "sessionId": "sess_test",
  "traceId": "trace_test",
  "turnId": "turn_test",
  "response": "task complete",
  "usage": {
    "source": "provider",
    "modelRequestCount": 2,
    "inputTokens": 500,
    "outputTokens": 80,
    "totalTokens": 600,
    "cacheReadTokens": 20,
    "cacheWriteTokens": 0,
    "reasoningTokens": 30
  },
  "eventCount": 12,
  "projection": {"status": "idle", "turnCount": 1, "contextWindow": 1000000}
}`

func TestParseZcodeJSONValid(t *testing.T) {
	z, ok := ParseZcodeJSON(fakeZcodeOK)
	require.True(t, ok)
	assert.Equal(t, "sess_test", z.SessionID)
	assert.Equal(t, "task complete", z.Response)
	assert.Equal(t, int64(500), z.Usage.InputTokens)
	assert.Equal(t, int64(30), z.Usage.ReasoningTokens)
}

func TestParseZcodeJSONInvalid(t *testing.T) {
	for _, bad := range []string{"", "not json", "Error: something broke"} {
		_, ok := ParseZcodeJSON(bad)
		assert.False(t, ok, "%q 不应解析成功", bad)
	}
}

func TestRunZcodeAgainstFakeBinary(t *testing.T) {
	binDir, workDir := fakeBin(t, "zcode", fakeZcodeOK)
	setPath(t, binDir)

	r := RunZcode(ZcodeArgs{
		Prompt:  "do the task",
		Model:   "zai/glm-5.3-flash",
		Cwd:     workDir,
		APIKeys: map[string]string{},
	})
	require.Empty(t, r.SpawnError)
	assert.True(t, r.Settled)
	assert.Equal(t, "zcode", r.Harness)
	assert.Equal(t, "zai-coding-plan", r.Provider)
	assert.Equal(t, "task complete", r.Text)
	assert.Equal(t, "sess_test", r.SessionID)

	raw, err := os.ReadFile(filepath.Join(workDir, "args.txt"))
	require.NoError(t, err)
	argsText := string(raw)
	assert.Contains(t, argsText, "--prompt")
	assert.Contains(t, argsText, "--json")
	assert.Contains(t, argsText, "--model")
	assert.Contains(t, argsText, "--mode")
}

func TestRunZcodeResumesSession(t *testing.T) {
	stream := `{"sessionId":"sess_resumed","response":"continued","usage":{"inputTokens":1,"outputTokens":1,"totalTokens":2},"projection":{"status":"idle"}}`
	binDir, workDir := fakeBin(t, "zcode", stream)
	setPath(t, binDir)

	r := RunZcode(ZcodeArgs{
		Prompt:    "continue",
		Model:     "zai/glm-5.3-flash",
		Cwd:       workDir,
		SessionID: "sess_prev",
		APIKeys:   map[string]string{},
	})
	require.Empty(t, r.SpawnError)
	assert.True(t, r.Settled)
	assert.Equal(t, "sess_resumed", r.SessionID)

	raw, err := os.ReadFile(filepath.Join(workDir, "args.txt"))
	require.NoError(t, err)
	argsText := string(raw)
	assert.Contains(t, argsText, "--resume")
	assert.Contains(t, argsText, "sess_prev")
}

// 非 JSON 输出（模型未回复/运行时错误文本）→ Settled=false + ErrorMessage
// 留给分类器定码——绝不把错误文本当任务产出。
func TestRunZcodeNonJSONOutputMarkedUnsettled(t *testing.T) {
	binDir, workDir := fakeBin(t, "zcode", "Error: Turn execution failed")
	setPath(t, binDir)

	r := RunZcode(ZcodeArgs{Prompt: "x", Model: "zai/glm-5.3-flash", Cwd: workDir, APIKeys: map[string]string{}})
	assert.False(t, r.Settled)
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "Turn execution failed")
}

// TestRunZcodeDefaultsAndStderrError 覆盖双臂：Model 空 → 默认 slug；
// 非零退出且 stderr 非空 → ErrorMessage 取 stderr（stdout 错误文本退居后位）。
func TestRunZcodeDefaultsAndStderrError(t *testing.T) {
	stream := `Error: Turn execution failed`
	binDir, workDir := fakeBin(t, "zcode", stream)
	// 追加非零退出 + stderr 输出
	scriptPath := filepath.Join(binDir, "zcode")
	raw, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(scriptPath, append(raw, []byte("echo boom >&2\nexit 1\n")...), 0o755))
	setPath(t, binDir)

	r := RunZcode(ZcodeArgs{Prompt: "x", Cwd: workDir, APIKeys: map[string]string{}})
	assert.False(t, r.Settled)
	require.NotNil(t, r.ErrorMessage)

	raw2, rerr := os.ReadFile(filepath.Join(workDir, "args.txt"))
	require.NoError(t, rerr)
	assert.Contains(t, string(raw2), ZcodeDefaultModel) // Model 空 → 默认 slug
}
