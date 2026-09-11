package drivers

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/watchdog"
)

// disableDesktop 测试闸：Run 级测试不得激活用户桌面（隔离纪律）。
func disableDesktop(t *testing.T) {
	prev := disableDesktopRegistration
	disableDesktopRegistration = true
	t.Cleanup(func() { disableDesktopRegistration = prev })
}

// forceDarwin 平台闸注入缝：把注册路径的平台判定强制为 darwin——注册逻辑
// 在 Linux CI 上也能真实执行与断言。真实 /tmp 符号链接语义仍按平台跳过。
func forceDarwin(t *testing.T) {
	prev := zcodeDesktopGOOS
	zcodeDesktopGOOS = "darwin"
	t.Cleanup(func() { zcodeDesktopGOOS = prev })
}

// stubOpener 换掉系统出口并记录调用路径；返回记录切片供断言。
func stubOpener(t *testing.T) *[]string {
	var got []string
	var mu sync.Mutex
	orig := zcodeWorkspaceOpener
	zcodeWorkspaceOpener = func(p string) error { mu.Lock(); got = append(got, p); mu.Unlock(); return nil }
	t.Cleanup(func() { zcodeWorkspaceOpener = orig })
	return &got
}

// zcodeOpenerCalls 线程安全读取 stubOpener 记录数。
func zcodeOpenerCalls(got *[]string) int {
	return len(*got)
}

// fakeZcodeServer 写一个 python3 交互假服务器并接管 zcodeRuntimeCommand 缝。
// variant 决定行为：ok = 完整协议闭环；error = create 后回协议错误；
// silent = 只排空 stdin（idle 看门狗臂）。
func fakeZcodeServer(t *testing.T, variant string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "zcode-fake")

	preamble := "import sys, json\n" +
		"sys.stderr.write('stderr-noise\\n')\n" +
		"def out(o):\n" +
		"    sys.stdout.write(json.dumps(o) + \"\\n\")\n" +
		"    sys.stdout.flush()\n"
	okBody := `
for line in sys.stdin:
    try:
        m = json.loads(line)
    except Exception:
        continue
    if m.get("method") == "session/create":
        out({"id": "server-1", "method": "session/requestRuntimePreferences",
             "params": {"sessionId": "sess_test", "scope": "runtime-materialization"}})
        out({"id": m["id"], "result": {"ok": True}})
    elif m.get("method") == "session/send":
        out({"id": m["id"], "result": {"accepted": True, "sessionId": "sess_test"}})
        out({"method": "state.updated",
             "params": {"patch": {"status": "running"}, "sessionId": "sess_test"}})
        out({"method": "computer-use/operation-event",
             "params": {"kind": "turn-completed", "sessionId": "sess_test", "turnId": "turn_test"}})
    elif m.get("method") == "session/read":
        out({"id": m["id"], "result": {"messages": [
            {"info": {"role": "user"}, "parts": [{"type": "text", "text": "do the task"}]},
            {"info": {"role": "assistant", "finish": "stop",
                      "tokens": {"total": 600, "input": 500, "output": 80, "reasoning": 20,
                                 "cache": {"read": 40, "write": 0}}},
             "parts": [{"type": "reasoning", "text": "ponder"},
                       {"type": "text", "text": "task complete"}]}]}})
        sys.exit(0)
`
	errorBody := `
for line in sys.stdin:
    try:
        m = json.loads(line)
    except Exception:
        continue
    if m.get("method") == "session/create":
        out({"id": m["id"], "error": {"code": -32602, "message": "boom schema drift"}})
        sys.exit(1)
`
	silentBody := `
import time
for _ in sys.stdin:
    pass
time.sleep(60)
`
	var body string
	switch variant {
	case "ok":
		body = okBody
	case "error":
		body = errorBody
	case "silent":
		body = silentBody
	}
	require.NoError(t, os.WriteFile(script, []byte("#!/usr/bin/env python3\n"+preamble+body), 0o755))

	orig := zcodeRuntimeCommand
	zcodeRuntimeCommand = func() (string, []string) { return script, []string{"app-server"} }
	t.Cleanup(func() { zcodeRuntimeCommand = orig })
	return script
}

func zcodeTestWatchdog() watchdog.Opts {
	return watchdog.Opts{IdleTimeoutMs: 30_000, WallTimeoutMs: 60_000, KillGraceMs: 1_000}
}

func TestSplitZcodeModel(t *testing.T) {
	p, m := splitZcodeModel("zai/glm-5.3")
	assert.Equal(t, "zai", p)
	assert.Equal(t, "glm-5.3", m)
	p2, m2 := splitZcodeModel("glm-5.3-flash")
	assert.Equal(t, "zai", p2, "无斜杠 → zcode 自身命名空间")
	assert.Equal(t, "glm-5.3-flash", m2)
}

// TestRunZcodeProtocolHappyPath 协议闭环：prefs 反向请求应答、create/send/
// turn-completed/read 四段、终态文本与 usage 提取、模型/思考档参数正确落到
// create 请求（由假服务器的应答链隐式验证——错参数即状态机走不通）。
func TestRunZcodeProtocolHappyPath(t *testing.T) {
	fakeZcodeServer(t, "ok")
	disableDesktop(t)
	workDir := t.TempDir()

	r := RunZcode(ZcodeArgs{
		Prompt:   "do the task",
		Model:    "zai/glm-5.3",
		Cwd:      workDir,
		APIKeys:  map[string]string{},
		Watchdog: zcodeTestWatchdog(),
	})
	assert.Empty(t, r.SpawnError)
	assert.True(t, r.Settled, "协议闭环完成即 settled（进程 exit 形态不参与判定）")
	assert.Equal(t, "zcode", r.Harness)
	assert.Equal(t, "zai-coding-plan", r.Provider)
	assert.Equal(t, "zai/glm-5.3", r.Model)
	assert.Equal(t, "task complete", r.Text)
	assert.Equal(t, "sess_test", r.SessionID)
	require.NotNil(t, r.Usage)
	assert.Equal(t, int64(500), r.Usage.Input)
	assert.Equal(t, int64(80), r.Usage.Output)
	assert.Equal(t, int64(20), r.Usage.Reasoning)
	assert.Equal(t, int64(40), r.Usage.CacheRead)
	assert.Equal(t, int64(600), r.Usage.TotalTokens)
	require.NotNil(t, r.ReasoningChars)
	assert.Equal(t, int64(len("ponder")), *r.ReasoningChars)
}

// TestRunZcodeProtocolErrorArm 服务器返回协议错误（schema 漂移形态）→ failed
// 记录 + finish 终结，绝不静默降级。
func TestRunZcodeProtocolErrorArm(t *testing.T) {
	fakeZcodeServer(t, "error")
	disableDesktop(t)
	r := RunZcode(ZcodeArgs{Prompt: "x", Cwd: t.TempDir(), APIKeys: map[string]string{},
		Watchdog: zcodeTestWatchdog()})
	assert.False(t, r.Settled)
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "boom schema drift")
}

// TestRunZcodeIdleTimeoutArm 静默服务器：无事件流活动 → idle 看门狗收割
// （流式 idle 语义对协议驱动生效的回归锁）。
func TestRunZcodeIdleTimeoutArm(t *testing.T) {
	fakeZcodeServer(t, "silent")
	disableDesktop(t)
	start := time.Now()
	r := RunZcode(ZcodeArgs{Prompt: "x", Cwd: t.TempDir(), APIKeys: map[string]string{},
		Watchdog: watchdog.Opts{IdleTimeoutMs: 600, WallTimeoutMs: 30_000, KillGraceMs: 500}})
	assert.False(t, r.Settled)
	assert.True(t, r.TimedOut)
	assert.Less(t, time.Since(start), 15*time.Second, "idle 600ms 必须近实时收割")
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "timeout")
}

// TestRunZcodeSpawnError 命令不存在 → SpawnError 直达 Result。
func TestRunZcodeSpawnError(t *testing.T) {
	disableDesktop(t)
	dir := t.TempDir()
	missing := filepath.Join(dir, "no-such-zcode")
	orig := zcodeRuntimeCommand
	zcodeRuntimeCommand = func() (string, []string) { return missing, []string{"app-server"} }
	t.Cleanup(func() { zcodeRuntimeCommand = orig })
	r := RunZcode(ZcodeArgs{Prompt: "x", Cwd: dir, APIKeys: map[string]string{},
		Watchdog: zcodeTestWatchdog()})
	assert.NotEmpty(t, r.SpawnError)
	assert.False(t, r.Settled)
}

// TestZcodeProtoOrderingVariants 状态机顺序鲁棒性：sessionId 先于 create 应答
// 到达（state.updated 携带）时，create 应答到达即投递 prompt。
func TestZcodeProtoOrderingVariants(t *testing.T) {
	var writes []string
	var mu sync.Mutex
	p := newZcodeProto("go", "zai/glm-5.3", "", "/w", "/w")
	p.mu.Lock()
	p.out = writerFunc(func(b []byte) (int, error) {
		mu.Lock()
		writes = append(writes, string(b))
		mu.Unlock()
		return len(b), nil
	})
	finished := false
	p.finish = func() { finished = true }
	p.mu.Unlock()

	// attach 的等价序列：先发 create（writes[0]），再消费乱序到达的事件。
	p.requestCreate()
	p.feedLine(`{"method":"state.updated","params":{"sessionId":"sess_a"}}`)
	p.feedLine(`{"id":"server-1","method":"session/requestRuntimePreferences","params":{"sessionId":"sess_a"}}`)
	mu.Lock()
	assert.Len(t, writes, 2, "create + prefs 应答；prompt 尚缺 create 应答不得投递")
	mu.Unlock()
	p.handleResponse(float64(1), []byte(`{}`)) // create 应答 → 立即投递 prompt
	mu.Lock()
	assert.Len(t, writes, 3, "create + prefs 应答 + send 投递")
	assert.Contains(t, writes[2], `"method":"session/send"`)
	assert.Contains(t, writes[2], `"content":"go"`)
	mu.Unlock()
	assert.False(t, finished)
}

// TestZcodeProtoReadWithoutText read 应答无 assistant 文本 → failed 记录但
// done 置位（会话终结，错误留给分类器）。
func TestZcodeProtoReadWithoutText(t *testing.T) {
	p := newZcodeProto("go", "zai/glm-5.3", "max", "/w", "/w")
	p.mu.Lock()
	p.out = writerFunc(func(b []byte) (int, error) { return len(b), nil })
	p.finish = func() {}
	p.mu.Unlock()
	p.consumeRead([]byte(`{"messages":[{"info":{"role":"assistant"},"parts":[{"type":"reasoning","text":"hmm"}]}]}`))
	oc := p.snapshot()
	assert.True(t, oc.done)
	assert.NotEmpty(t, oc.failed)
	assert.Empty(t, oc.text)
}

// TestZcodeProtoDefaultModelAndThought newZcodeProto 的空值兜底（max 档）。
func TestZcodeProtoDefaultModelAndThought(t *testing.T) {
	p := newZcodeProto("p", "zai/glm-5.3-flash", "", "/w", "/w")
	p.mu.Lock()
	thought, model := p.thought, p.modelID
	p.mu.Unlock()
	assert.Equal(t, "max", thought)
	assert.Equal(t, "glm-5.3-flash", model)
}

// writerFunc 测试用 io.Writer 适配。
type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(b []byte) (int, error) { return f(b) }

// ---- 桌面注册（v0.20 机制回归）----

func TestRegisterZcodeWorkspaceNoopOnEmpty(t *testing.T) {
	forceDarwin(t)
	called := stubOpener(t)
	RegisterZcodeWorkspace("")
	assert.Empty(t, *called)
}

func TestRegisterZcodeWorkspaceDisabledByTestGate(t *testing.T) {
	forceDarwin(t)
	called := stubOpener(t)
	disableDesktop(t)
	RegisterZcodeWorkspace("/some/path")
	assert.Empty(t, *called)
}

func TestRegisterZcodeWorkspaceNormalizesSymlinks(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("EvalSymlinks 语义按 darwin 验证")
	}
	got := stubOpener(t)
	RegisterZcodeWorkspace("/tmp")
	require.NotEmpty(t, *got)
	assert.Equal(t, "/private/tmp", (*got)[0], "必须解析为真实路径再触发")
}

func TestZcodeDesktopBinResolution(t *testing.T) {
	assert.Equal(t, defaultZcodeDesktopBin, zcodeDesktopBin())
	t.Setenv(ZcodeDesktopBinEnv, "/custom/zcode")
	assert.Equal(t, "/custom/zcode", zcodeDesktopBin())
}

func TestDefaultOpenZcodeWorkspaceStartsBin(t *testing.T) {
	binDir := t.TempDir()
	script := "#!/bin/sh\nexit 0\n"
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "opener"), []byte(script), 0o755))
	t.Setenv(ZcodeDesktopBinEnv, filepath.Join(binDir, "opener"))
	require.NoError(t, defaultOpenZcodeWorkspace("/some/workspace"))

	t.Setenv(ZcodeDesktopBinEnv, filepath.Join(binDir, "missing"))
	require.Error(t, defaultOpenZcodeWorkspace("/some/workspace"))
}

// TestRunZcodeRegistersWorkspace RPC 驱动保留双触发注册（spawn 前后各一次）。
func TestRunZcodeRegistersWorkspace(t *testing.T) {
	fakeZcodeServer(t, "ok")
	forceDarwin(t)
	got := stubOpener(t)
	workDir := t.TempDir()

	r := RunZcode(ZcodeArgs{Prompt: "x", Model: "zai/glm-5.3", Cwd: workDir,
		APIKeys: map[string]string{}, Watchdog: zcodeTestWatchdog()})
	assert.True(t, r.Settled)
	resolved, err := filepath.EvalSymlinks(workDir)
	require.NoError(t, err)
	assert.Equal(t, []string{resolved, resolved}, *got, "spawn 前后各一次")
	assert.True(t, strings.HasPrefix(resolved, "/"), "归一化路径")
	assert.Equal(t, 2, zcodeOpenerCalls(got))
}

// TestZcodeProtoSendErrorArms 写端失败路径：create/send/read 三处 sendLocked
// 错误都必须进 failLocked（防御性终结，绝不悬挂）。
func TestZcodeProtoSendErrorArms(t *testing.T) {
	mk := func() *zcodeProto {
		p := newZcodeProto("go", "zai/glm-5.3", "max", "/w", "/w")
		p.mu.Lock()
		p.out = writerFunc(func([]byte) (int, error) { return 0, os.ErrClosed })
		p.finish = func() {}
		p.mu.Unlock()
		return p
	}
	p := mk()
	p.requestCreate() // create 写失败 → failLocked
	oc := p.snapshot()
	assert.NotEmpty(t, oc.failed)

	p2 := mk()
	p2.mu.Lock()
	p2.created, p2.sessionID = true, "sess_x"
	p2.mu.Unlock()
	p2.maybeSendPrompt() // send 写失败 → failLocked
	assert.NotEmpty(t, p2.snapshot().failed)

	p3 := mk()
	p3.mu.Lock()
	p3.sessionID = "sess_x"
	p3.mu.Unlock()
	p3.requestRead() // read 写失败 → failLocked
	assert.NotEmpty(t, p3.snapshot().failed)
}

// TestZcodeProtoEdgeLines feedLine 边缘：空 error 消息跳过、乱序 prefs 在
// attach 前到达（out 为 nil 的零值安全臂）、非 JSON 行忽略。
func TestZcodeProtoEdgeLines(t *testing.T) {
	p := newZcodeProto("go", "zai/glm-5.3", "max", "/w", "/w")
	p.mu.Lock()
	p.finish = func() {}
	p.mu.Unlock()
	p.feedLine("")                         // 空行：忽略
	p.feedLine("   ")                      // 纯空白：忽略
	p.feedLine(`{"error":{"message":""}}`) // 空错误消息：不终结
	p.feedLine(`not-json`)                 // 非 JSON：忽略
	p.feedLine(`{"id":"server-9","method":"session/requestRuntimePreferences","params":{"sessionId":"sess_e"}}`)
	oc := p.snapshot()
	assert.Equal(t, "sess_e", oc.sessionID)
	assert.Empty(t, oc.failed, "out 为 nil 时 sendLocked 零值安全返回")

	p3 := newZcodeProto("go", "zai/glm-5.3", "max", "/w", "/w")
	p3.mu.Lock()
	p3.out = writerFunc(func([]byte) (int, error) { return 0, os.ErrClosed })
	p3.finish = func() {}
	p3.mu.Unlock()
	p3.feedLine(`{"id":"server-2","method":"session/requestRuntimePreferences","params":{"sessionId":"sess_f"}}`)
	assert.NotEmpty(t, p3.snapshot().failed, "prefs 应答写失败 → failLocked")

	p2 := newZcodeProto("go", "zai/glm-5.3", "max", "/w", "/w")
	p2.mu.Lock()
	p2.out = writerFunc(func([]byte) (int, error) { return len([]byte{}), nil })
	p2.finish = func() {}
	p2.mu.Unlock()
	p2.consumeRead([]byte(`not-json`)) // read 应答不可解析 → failLocked
	assert.NotEmpty(t, p2.snapshot().failed)
}

// TestZcodeConsumeReadSkipsNonAssistant 末条非 assistant（user 在后）→
// continue 回溯到 assistant 提取。
func TestZcodeConsumeReadSkipsNonAssistant(t *testing.T) {
	p := newZcodeProto("go", "zai/glm-5.3", "max", "/w", "/w")
	p.mu.Lock()
	p.out = writerFunc(func([]byte) (int, error) { return 0, nil })
	p.finish = func() {}
	p.mu.Unlock()
	p.consumeRead([]byte(`{"messages":[
		{"info":{"role":"assistant","tokens":{"total":9,"input":8,"output":1,"reasoning":0,"cache":{"read":0,"write":0}}},
		 "parts":[{"type":"text","text":"deep answer"}]},
		{"info":{"role":"user"},"parts":[{"type":"text","text":"late user"}]}]}`))
	oc := p.snapshot()
	assert.Equal(t, "deep answer", oc.text)
	require.NotNil(t, oc.usage)
	assert.Equal(t, int64(9), oc.usage.TotalTokens)
}

// TestZcodeProtoRequestReadGuards requestRead 的 done/failed 短路臂。
func TestZcodeProtoRequestReadGuards(t *testing.T) {
	p := newZcodeProto("go", "zai/glm-5.3", "max", "/w", "/w")
	var calls int
	p.mu.Lock()
	p.out = writerFunc(func([]byte) (int, error) { calls++; return 0, nil })
	p.finish = func() {}
	p.done = true
	p.mu.Unlock()
	p.requestRead() // done → 短路
	assert.Equal(t, 0, calls)
	p.mu.Lock()
	p.done = false
	p.failed = "x"
	p.mu.Unlock()
	p.requestRead() // failed → 短路
	assert.Equal(t, 0, calls)
}

// TestDefaultZcodeRuntimeCommand 命令缝缺省形态：node + app 内置 runtime；
// env 覆盖走自定义 launcher。
func TestDefaultZcodeRuntimeCommand(t *testing.T) {
	name, args := defaultZcodeRuntimeCommand()
	assert.Equal(t, "node", name)
	assert.Equal(t, []string{defaultZcodeRuntimePath, "app-server"}, args)
	t.Setenv(ZcodeDesktopBinEnv, "/custom/zcode-launcher")
	name2, args2 := defaultZcodeRuntimeCommand()
	assert.Equal(t, "/custom/zcode-launcher", name2)
	assert.Equal(t, []string{"app-server"}, args2)
}

// TestRunZcodeSilentQuickDeath 进程立即退出且无任何事件 → 通用兜底错误
// （"session ended without completion" 臂）。
func TestRunZcodeSilentQuickDeath(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\nexit 0\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "die"), []byte(script), 0o755))
	orig := zcodeRuntimeCommand
	zcodeRuntimeCommand = func() (string, []string) { return filepath.Join(dir, "die"), []string{"app-server"} }
	t.Cleanup(func() { zcodeRuntimeCommand = orig })
	disableDesktop(t)

	r := RunZcode(ZcodeArgs{Prompt: "x", Cwd: t.TempDir(), APIKeys: map[string]string{},
		Watchdog: zcodeTestWatchdog()})
	assert.False(t, r.Settled)
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "without completion")
}
