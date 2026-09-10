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

// forceDarwin 平台闸注入缝：把注册路径的平台判定强制为 darwin——注册逻辑
// 在 Linux CI 上也能真实执行与断言（否则 darwin-only 分支既测不到也拉不满
// 覆盖率门禁）。真实 /tmp 符号链接语义仍按平台跳过（见 NormalizesSymlinks）。
func forceDarwin(t *testing.T) {
	prev := zcodeDesktopGOOS
	zcodeDesktopGOOS = "darwin"
	t.Cleanup(func() { zcodeDesktopGOOS = prev })
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
	// input 用上报值（500），不用 total−reasoning−cacheRead 派生（550）——
	// 两种算法在探针 fixture 上不一致，上报值权威（PR #4 review 修复）。
	require.NotNil(t, r.Usage)
	assert.Equal(t, int64(500), r.Usage.Input)
	assert.Equal(t, int64(80), r.Usage.Output)
	assert.Equal(t, int64(600), r.Usage.TotalTokens)

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
	forceDarwin(t)
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
	forceDarwin(t)
	got := stubOpener(t)

	r := RunZcode(ZcodeArgs{Prompt: "x", Model: "zai/glm-5.3-flash", Cwd: workDir, APIKeys: map[string]string{}})
	assert.True(t, r.Settled)
	// opener 收到的是 EvalSymlinks 后的真实路径（索引按真实路径做 key）。
	resolved, err := filepath.EvalSymlinks(workDir)
	require.NoError(t, err)
	assert.Equal(t, []string{resolved, resolved}, *got, "spawn 前后各一次")
}

// TestRunZcodeUsageDerivesWhenInputUnreported inputTokens 缺报（0）时从
// total 派生并 clamp ≥0——派生 fallback 与负值钳制两臂（PR #4 review 修复）。
func TestRunZcodeUsageDerivesWhenInputUnreported(t *testing.T) {
	disableDesktop(t) // 不触桌面：本测试只关心 usage 解析
	derive := `{"sessionId":"s1","response":"r","usage":{"outputTokens":3,"totalTokens":10,"reasoningTokens":2,"cacheReadTokens":1,"cacheWriteTokens":1}}`
	clamp := `{"sessionId":"s2","response":"r","usage":{"outputTokens":1,"totalTokens":2,"reasoningTokens":5}}`

	binDir1, workDir1 := fakeBin(t, "zcode", derive)
	setPath(t, binDir1)
	r1 := RunZcode(ZcodeArgs{Prompt: "x", Cwd: workDir1, APIKeys: map[string]string{}})
	require.NotNil(t, r1.Usage)
	assert.Equal(t, int64(6), r1.Usage.Input, "缺报时派生 total−reasoning−cacheRead−cacheWrite")
	assert.Equal(t, int64(10), r1.Usage.TotalTokens)

	binDir2, workDir2 := fakeBin(t, "zcode", clamp)
	setPath(t, binDir2)
	r2 := RunZcode(ZcodeArgs{Prompt: "x", Cwd: workDir2, APIKeys: map[string]string{}})
	require.NotNil(t, r2.Usage)
	assert.Equal(t, int64(0), r2.Usage.Input, "派生为负时 clamp 0，绝不倒挂")
}

// TestRegisterZcodeWorkspaceNoopOnEmpty 空路径绝不触发 open。
func TestRegisterZcodeWorkspaceNoopOnEmpty(t *testing.T) {
	forceDarwin(t) // 平台闸开到 darwin，断言的才是 path=="" 分支本身
	called := stubOpener(t)
	RegisterZcodeWorkspace("")
	assert.Empty(t, *called)
}

// TestRegisterZcodeWorkspaceDisabledByTestGate 测试闸生效时不触碰系统出口。
func TestRegisterZcodeWorkspaceDisabledByTestGate(t *testing.T) {
	forceDarwin(t) // 同上：断言 disable 闸而非平台闸
	called := stubOpener(t)
	disableDesktop(t)
	RegisterZcodeWorkspace("/some/path")
	assert.Empty(t, *called)
}

// TestRegisterZcodeWorkspaceNormalizesSymlinks 索引按归一化路径做 key：
// /tmp→/private/tmp 类符号链接必须在触达 opener 前解析（实测未归一化注册
// 成功但会话导入失配）。非 macOS 跳过（/tmp 符号链接是 darwin 文件系统事实，
// 与平台闸注入缝无关——注册逻辑本身已由 forceDarwin 测试覆盖）。
func TestRegisterZcodeWorkspaceNormalizesSymlinks(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("EvalSymlinks 语义按 darwin 验证")
	}
	got := stubOpener(t)
	RegisterZcodeWorkspace("/tmp")
	require.NotEmpty(t, *got)
	assert.Equal(t, "/private/tmp", (*got)[0], "必须解析为真实路径再触发")
}

// TestZcodeDesktopBinResolution env 覆盖权威、缺省走官方安装路径（双臂纯解析，
// 不执行二进制）。
func TestZcodeDesktopBinResolution(t *testing.T) {
	assert.Equal(t, defaultZcodeDesktopBin, zcodeDesktopBin())
	t.Setenv(ZcodeDesktopBinEnv, "/custom/zcode")
	assert.Equal(t, "/custom/zcode", zcodeDesktopBin())
}

// TestDefaultOpenZcodeWorkspaceStartsBin 真出口仅以假二进制冒烟：Start 成功
// 即返回 nil，回收由内部 goroutine Wait 承担（不 Wait 会留僵尸，PR #4 review）。
func TestDefaultOpenZcodeWorkspaceStartsBin(t *testing.T) {
	binDir := t.TempDir()
	script := "#!/bin/sh\nexit 0\n"
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "opener"), []byte(script), 0o755))
	t.Setenv(ZcodeDesktopBinEnv, filepath.Join(binDir, "opener"))
	require.NoError(t, defaultOpenZcodeWorkspace("/some/workspace"))

	// 二进制不存在 → Start 报错原样返回（生产路径静默降级由调用方保证）。
	t.Setenv(ZcodeDesktopBinEnv, filepath.Join(binDir, "missing"))
	require.Error(t, defaultOpenZcodeWorkspace("/some/workspace"))
}
