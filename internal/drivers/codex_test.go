package drivers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/watchdog"
)

func TestCodexPermissionScopeAndInvalidScope(t *testing.T) {
	disableCodexDesktop(t)
	bin, cwd := fakeBin(t, "codex", codexNDJSON)
	setPath(t, bin)
	r := InvokeHarness(CallArgs{Prompt: "implement this", Cwd: cwd,
		Binding:        config.LoopBinding{Harness: "codex", Model: "gpt-6-astra", Thinking: "high"},
		PermissionMode: "workspace-write",
	}, "")
	require.True(t, r.Settled)
	args, err := os.ReadFile(filepath.Join(cwd, "args.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(args), "workspace-write")
	assert.NotContains(t, string(args), "danger-full-access")
	assert.Contains(t, string(args), "approval_policy=never")

	invalid := RunCodex(CodexArgs{Prompt: "p", PermissionMode: "danger-full-access"})
	assert.False(t, invalid.Settled)
	require.NotNil(t, invalid.ErrorMessage)
	assert.Contains(t, *invalid.ErrorMessage, "unsupported permission mode")
	assert.Empty(t, invalid.Stdout)
}

func TestEveryHarnessPropagatesCancelledContext(t *testing.T) {
	disableCodexDesktop(t)
	disableDesktop(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, harness := range []string{"kimi", "pi", "omp", "codex", "zcode", "dsh"} {
		t.Run(harness, func(t *testing.T) {
			r := InvokeHarness(CallArgs{Prompt: "must not start", Cwd: t.TempDir(),
				Binding:  config.LoopBinding{Harness: harness, Model: "test-model", Channel: "test-provider"},
				Watchdog: watchdog.Opts{Context: ctx},
			}, "")
			assert.True(t, r.Cancelled)
			assert.False(t, r.Settled)
			assert.Empty(t, r.SpawnError, "already cancelled calls must not start a process")
			assert.Empty(t, r.Stdout)
		})
	}
}

// codexNDJSON 一条完整 happy-path 事件流（结构取自 2026-09-10 gpt-6-astra 探针）。
const codexNDJSON = `{"type":"thread.started","thread_id":"01a08be8-c191"}
noise-not-json
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"review verdict ok"}}
{"type":"turn.completed","usage":{"input_tokens":20358,"cached_input_tokens":12160,"cache_write_input_tokens":3,"output_tokens":5,"reasoning_output_tokens":7}}
`

func TestParseCodexEventStream(t *testing.T) {
	p := ParseCodexEventStream(codexNDJSON)
	assert.Equal(t, "01a08be8-c191", p.SessionID)
	assert.Equal(t, "review verdict ok", p.Text)
	require.True(t, p.HasUsage)
	assert.Equal(t, int64(20358), p.Usage.Input)
	assert.Equal(t, int64(12160), p.Usage.CacheRead)
	assert.Equal(t, int64(3), p.Usage.CacheWrite)
	assert.Equal(t, int64(5), p.Usage.Output)
	assert.Equal(t, int64(7), p.Usage.Reasoning)
	assert.Equal(t, int64(20358+5), p.Usage.TotalTokens)
	assert.Empty(t, p.ErrorMessage)
	assert.True(t, p.Completed)

	errStream := `{"type":"error","message":"403 Forbidden via proxy"}
{"type":"turn.failed","error":{"message":"stream dead"}}`
	p2 := ParseCodexEventStream(errStream)
	assert.Contains(t, p2.ErrorMessage, "403 Forbidden")
	assert.Contains(t, p2.ErrorMessage, "stream dead", "多错误累积不覆盖")
}

func TestCodexRequiresCompletedTurnAndFlushesFinalEvent(t *testing.T) {
	disableCodexDesktop(t)
	for name, stream := range map[string]string{
		"text without completion":       `{"type":"item.completed","item":{"type":"agent_message","text":"partial"}}`,
		"next turn not completed":       codexNDJSON + "{\"type\":\"turn.started\"}\n",
		"next turn has no answer":       codexNDJSON + "{\"type\":\"turn.started\"}\n{\"type\":\"turn.completed\"}\n",
		"bare failure after completion": codexNDJSON + "{\"type\":\"turn.failed\"}\n",
		"structured error":              codexNDJSON + "{\"type\":\"error\",\"message\":{\"message\":\"rate limit\"}}\n",
		"malformed completion":          "{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"partial\"}}\n{\"type\":\"turn.completed\",\"usage\":\"broken\"}\n",
	} {
		t.Run(name, func(t *testing.T) {
			bin, cwd := fakeBin(t, "codex", stream)
			setPath(t, bin)
			r := RunCodex(CodexArgs{Prompt: "p", Cwd: cwd})
			assert.False(t, r.Settled)
			require.NotNil(t, r.ErrorMessage)
		})
	}
	bin, cwd := fakeBin(t, "codex", "")
	setPath(t, bin)
	terminal := `{"type":"turn.completed"}`
	script := "#!/bin/sh\nprintf '%s\\n' " + shQuote(`{"type":"item.completed","item":{"type":"agent_message","text":"done"}}`) + "\nprintf '%s' " + shQuote(terminal) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o755))
	var events []string
	r := RunCodex(CodexArgs{Prompt: "p", Cwd: cwd, OnEvent: func(line string) { events = append(events, line) }})
	assert.True(t, r.Settled)
	assert.Equal(t, terminal, events[len(events)-1])
	assert.Nil(t, r.Usage, "a completed turn without token fields has unknown usage")

	oldLimit := watchdog.MaxBuffer
	watchdog.MaxBuffer = int64(len(codexNDJSON) + 1)
	t.Cleanup(func() { watchdog.MaxBuffer = oldLimit })
	bin, cwd = fakeBin(t, "codex", codexNDJSON+strings.Repeat("x", 1024))
	setPath(t, bin)
	r = RunCodex(CodexArgs{Prompt: "p", Cwd: cwd})
	assert.True(t, r.Truncated)
	assert.False(t, r.Settled, "a retained turn.completed cannot hide a later lost error")
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "truncated")
}

func TestCodexPartialUsageStaysUnknown(t *testing.T) {
	for _, usage := range []string{`{}`, `null`, `{"input_tokens":1}`, `{"input_tokens":null,"output_tokens":2}`, `{"input_tokens":1,"output_tokens":-1}`, `{"input_tokens":9223372036854775807,"output_tokens":1}`, `{"input_tokens":1,"output_tokens":2,"cached_input_tokens":-1}`} {
		t.Run(usage, func(t *testing.T) {
			p := ParseCodexEventStream(codexNDJSON + "{\"type\":\"turn.started\"}\n{\"type\":\"turn.completed\",\"usage\":" + usage + "}\n")
			assert.True(t, p.Completed)
			assert.False(t, p.HasUsage, "incomplete usage cannot erase unknown-call accounting by becoming zero")
		})
	}
	p := ParseCodexEventStream(`{"type":"turn.completed","usage":{"input_tokens":0,"output_tokens":0}}`)
	assert.True(t, p.HasUsage)
	assert.Zero(t, p.Usage.TotalTokens)
}

// disableCodexDesktop / forceCodexDarwin / stubCodexOpener 测试缝三件套（同
// zcode 模式）。
func disableCodexDesktop(t *testing.T) {
	prev := disableCodexRegistration
	disableCodexRegistration = true
	t.Cleanup(func() { disableCodexRegistration = prev })
}

func forceCodexDarwin(t *testing.T) {
	prev := codexDesktopGOOS
	codexDesktopGOOS = "darwin"
	t.Cleanup(func() { codexDesktopGOOS = prev })
}

func stubCodexOpener(t *testing.T) *[]string {
	var got []string
	var mu sync.Mutex
	orig := codexProjectOpener
	codexProjectOpener = func(p string) error { mu.Lock(); got = append(got, p); mu.Unlock(); return nil }
	t.Cleanup(func() { codexProjectOpener = orig })
	return &got
}

func TestRunCodexAgainstFakeBinary(t *testing.T) {
	binDir, workDir := fakeBin(t, "codex", codexNDJSON)
	scriptPath := filepath.Join(binDir, "codex")
	raw, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(scriptPath, append(raw, []byte("echo stderr-noise >&2\n")...), 0o755))
	setPath(t, binDir)
	disableCodexDesktop(t)

	var events []string
	r := RunCodex(CodexArgs{
		Prompt:          "review this",
		Model:           "gpt-6-astra",
		ReasoningEffort: "max",
		Cwd:             workDir,
		APIKeys:         map[string]string{},
		Watchdog:        watchdog.Opts{IdleTimeoutMs: 30_000, WallTimeoutMs: 60_000},
		OnEvent: func(line string) {
			events = append(events, line)
		},
	})
	assert.Empty(t, r.SpawnError)
	assert.Equal(t, "codex", r.Harness)
	assert.Equal(t, "chatgpt-subscription", r.Provider)
	assert.Equal(t, "gpt-6-astra", r.Model)
	assert.Equal(t, "01a08be8-c191", r.SessionID)
	assert.True(t, r.Settled)
	assert.Equal(t, "review verdict ok", r.Text)
	require.NotNil(t, r.Usage)
	assert.Equal(t, int64(12160), r.Usage.CacheRead)
	assert.NotEmpty(t, events, "OnEvent 应收到 JSONL 行")

	raw2, err2 := os.ReadFile(filepath.Join(workDir, "args.txt"))
	require.NoError(t, err2)
	args := string(raw2)
	assert.Contains(t, args, "--json")
	assert.Contains(t, args, "-m")
	assert.Contains(t, args, "gpt-6-astra")
	assert.Contains(t, args, "model_reasoning_effort=max")
	assert.Contains(t, args, "--skip-git-repo-check")
	assert.Contains(t, args, "read-only", "评审默认只读，不继承用户 config")
	assert.NotContains(t, args, "danger-full-access")
	assert.Contains(t, args, "approval_policy=never", "审批策略显式固定（headless 不挂审批）")
}

// TestRunCodexDefaultsAndError 空 model/effort 兜底（gpt-6-astra + high）；流内
// error → Settled=false + ErrorMessage。
func TestRunCodexDefaultsAndError(t *testing.T) {
	stream := `{"type":"thread.started","thread_id":"t1"}
{"type":"error","message":"The 'gpt-6-astra' model requires a newer version of Codex"}
`
	binDir, workDir := fakeBin(t, "codex", stream)
	setPath(t, binDir)
	disableCodexDesktop(t)

	r := RunCodex(CodexArgs{Prompt: "x", Cwd: workDir, APIKeys: map[string]string{},
		Watchdog: watchdog.Opts{IdleTimeoutMs: 30_000, WallTimeoutMs: 60_000}})
	assert.False(t, r.Settled)
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "newer version")

	raw, err := os.ReadFile(filepath.Join(workDir, "args.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), CodexDefaultModel, "空 model → 法定默认")
	assert.Contains(t, string(raw), "model_reasoning_effort=high", "空 effort → high")
}

// TestRunCodexNoMessageArm exit 0 但流内无 agent_message → EMPTY 语义错误信息。
func TestRunCodexNoMessageArm(t *testing.T) {
	binDir, workDir := fakeBin(t, "codex", `{"type":"thread.started","thread_id":"t2"}
{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":1}}
`)
	setPath(t, binDir)
	disableCodexDesktop(t)

	r := RunCodex(CodexArgs{Prompt: "x", Cwd: workDir, APIKeys: map[string]string{},
		Watchdog: watchdog.Opts{IdleTimeoutMs: 30_000, WallTimeoutMs: 60_000}})
	assert.False(t, r.Settled)
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "no agent_message")
}

func TestRegisterCodexProjectVariants(t *testing.T) {
	forceCodexDarwin(t)
	called := stubCodexOpener(t)
	RegisterCodexProject("")
	assert.Empty(t, *called, "空路径不触发")
	RegisterCodexProject("/w")
	require.Len(t, *called, 1)
	assert.Equal(t, "/w", (*called)[0], "已解析路径直接透传（EvalSymlinks 失败回退原值）")

	disableCodexDesktop(t)
	RegisterCodexProject("/w2")
	assert.Len(t, *called, 1, "测试闸生效")
}

// TestRunCodexStderrErrorArm 非零退出 + stderr（流内无 error 事件）→ 错误取
// stderr（分类器定码输入）。
func TestRunCodexStderrErrorArm(t *testing.T) {
	binDir, workDir := fakeBin(t, "codex", "")
	scriptPath := filepath.Join(binDir, "codex")
	raw, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(scriptPath, append(raw, []byte("echo 'The 'gpt-6-astra' model requires a newer version' >&2\nexit 1\n")...), 0o755))
	setPath(t, binDir)
	disableCodexDesktop(t)

	r := RunCodex(CodexArgs{Prompt: "x", Cwd: workDir, APIKeys: map[string]string{},
		Watchdog: watchdog.Opts{IdleTimeoutMs: 30_000, WallTimeoutMs: 60_000}})
	assert.False(t, r.Settled)
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "newer version")
}

// TestRegisterCodexProjectEvalSymlinksFail 归一化失败回退原路径。
func TestRegisterCodexProjectEvalSymlinksFail(t *testing.T) {
	forceCodexDarwin(t)
	called := stubCodexOpener(t)
	RegisterCodexProject("/no/such/path")
	require.Len(t, *called, 1)
	assert.Equal(t, "/no/such/path", (*called)[0], "EvalSymlinks 失败回退原值")
}

// TestDefaultOpenCodexProjectExecutes 真出口冒烟：macOS 上 /usr/bin/open 存在
// 则 Start 成功；非 macOS Start 报错原样返回——两种平台都覆盖函数体。
func TestDefaultOpenCodexProjectExecutes(t *testing.T) {
	_ = defaultOpenCodexProject("/tmp")
}

// TestRunCodexSpawnError PATH 无 codex → SpawnError 直达 Result。
func TestRunCodexSpawnError(t *testing.T) {
	emptyDir := t.TempDir()
	t.Setenv("PATH", emptyDir)
	disableCodexDesktop(t)
	r := RunCodex(CodexArgs{Prompt: "x", Cwd: t.TempDir(), APIKeys: map[string]string{},
		Watchdog: watchdog.Opts{IdleTimeoutMs: 30_000, WallTimeoutMs: 60_000}})
	assert.NotEmpty(t, r.SpawnError)
	assert.False(t, r.Settled)
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "codex")
}

// TestRegisterCodexProjectRealPath 真实存在路径 → EvalSymlinks 成功归一化透传。
func TestRegisterCodexProjectRealPath(t *testing.T) {
	forceCodexDarwin(t)
	called := stubCodexOpener(t)
	dir := t.TempDir()
	RegisterCodexProject(dir)
	require.Len(t, *called, 1)
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, resolved, (*called)[0])
}

// TestDefaultOpenCodexProjectArms open 缝双臂：假脚本成功 + 不存在路径报错。
func TestDefaultOpenCodexProjectArms(t *testing.T) {
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "open")
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	orig := codexOpenBin
	codexOpenBin = fake
	t.Cleanup(func() { codexOpenBin = orig })
	require.NoError(t, defaultOpenCodexProject("/w"))
	codexOpenBin = filepath.Join(binDir, "missing")
	require.Error(t, defaultOpenCodexProject("/w"))
}
