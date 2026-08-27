package drivers

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// disableDesktop 测试闸：Run 级测试不得激活用户桌面（隔离纪律）。
func disableDesktop(t *testing.T) {
	prev := disableDesktopRegistration
	disableDesktopRegistration = true
	t.Cleanup(func() { disableDesktopRegistration = prev })
}

// stubOpener 换掉系统出口并记录调用路径；返回记录切片供断言。
func stubOpener(t *testing.T) *[]string {
	var got []string
	orig := zcodeWorkspaceOpener
	zcodeWorkspaceOpener = func(p string) error { got = append(got, p); return nil }
	t.Cleanup(func() { zcodeWorkspaceOpener = orig })
	return &got
}

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
	disableDesktop(t)

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
	disableDesktop(t)

	r := RunZcode(ZcodeArgs{
		Prompt:    "continue",
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
// 留给分类器定码——绝不把错误文本当任务产出。失败尝试同样双触发：后触发把
// 部分对话导入 GUI 供排障。
func TestRunZcodeNonJSONOutputMarkedUnsettled(t *testing.T) {
	binDir, workDir := fakeBin(t, "zcode", "Error: Turn execution failed")
	setPath(t, binDir)
	got := stubOpener(t)

	r := RunZcode(ZcodeArgs{Prompt: "x", Cwd: workDir, APIKeys: map[string]string{}})
	assert.False(t, r.Settled)
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "Turn execution failed")
	assert.Len(t, *got, 2, "失败尝试前后仍各触发一次")
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
	disableDesktop(t)

	r := RunZcode(ZcodeArgs{Prompt: "x", Cwd: workDir, APIKeys: map[string]string{}})
	assert.False(t, r.Settled)
	require.NotNil(t, r.ErrorMessage)

	raw2, rerr := os.ReadFile(filepath.Join(workDir, "args.txt"))
	require.NoError(t, rerr)
	assert.Contains(t, string(raw2), ZcodeDefaultModel) // Model 空 → 默认 slug
}

// TestRunZcodeRegistersWorkspace spawn 前后各触发一次 workspace 注册（AGENTS
// 注入缝记录调用，不真跑二进制）。前触发注册+导入历史（resume/重试）；后触发
// 把刚结束的会话立即导入桌面索引——首次执行的全新路径在前触发时存储为空，
// 没有后触发则首会话永远不可见。
func TestRunZcodeRegistersWorkspace(t *testing.T) {
	binDir, workDir := fakeBin(t, "zcode", fakeZcodeOK)
	setPath(t, binDir)
	got := stubOpener(t)

	r := RunZcode(ZcodeArgs{Prompt: "x", Model: "zai/glm-5.3-flash", Cwd: workDir, APIKeys: map[string]string{}})
	assert.True(t, r.Settled)
	// opener 收到的是 EvalSymlinks 后的真实路径（索引按真实路径做 key）。
	resolved, err := filepath.EvalSymlinks(workDir)
	require.NoError(t, err)
	assert.Equal(t, []string{resolved, resolved}, *got, "spawn 前后各一次")
}

// TestRegisterZcodeWorkspaceNoopOnEmpty 空路径绝不触发 open。
func TestRegisterZcodeWorkspaceNoopOnEmpty(t *testing.T) {
	called := stubOpener(t)
	RegisterZcodeWorkspace("")
	assert.Empty(t, *called)
}

// TestRegisterZcodeWorkspaceDisabledByTestGate 测试闸生效时不触碰系统出口。
func TestRegisterZcodeWorkspaceDisabledByTestGate(t *testing.T) {
	called := stubOpener(t)
	disableDesktop(t)
	RegisterZcodeWorkspace("/some/path")
	assert.Empty(t, *called)
}

// TestRegisterZcodeWorkspaceNormalizesSymlinks 索引按归一化路径做 key：
// /tmp→/private/tmp 类符号链接必须在触达 opener 前解析（实测未归一化注册
// 成功但会话导入失配）。非 macOS 跳过（GOOS 守卫差异由函数自身保证）。
func TestRegisterZcodeWorkspaceNormalizesSymlinks(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("EvalSymlinks 语义按 darwin 验证")
	}
	got := stubOpener(t)
	RegisterZcodeWorkspace("/tmp")
	require.NotEmpty(t, *got)
	assert.Equal(t, "/private/tmp", (*got)[0], "必须解析为真实路径再触发")
}
