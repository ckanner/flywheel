// Package drivers — zcode driver（DESIGN v0.22）：Z.ai 官方 GLM harness，经其
// app-server 协议（"ZCode Protocol"）驱动 Execute default/multimodal 与 Intake。
//
// 协议形态（2026-09-10 本机逆向实证，无公开文档——握手与方法名随桌面自动更新
// 可能漂移，driver 防御性处理：schema 校验失败即报错，绝不静默降级）：
//
//	信封：NDJSON 行分隔，请求 {"id":N,"method":...,"params":...}（无 jsonrpc 键）
//	① session/create {workspace{workspacePath,workspaceKey}, model{providerId,
//	   modelId}, thoughtLevel, mode:"yolo"} —— 模型与思考档按会话指定（一次性
//	   CLI 入口的全局单模型限制就此消解）
//	② 服务器反向请求 session/requestRuntimePreferences（携 sessionId）——应答
//	   {"nativeSearchEnhancementsEnabled":false}；官方 MCP 鉴权反向请求可忽略
//	③ session/send {sessionId, content} → {"accepted":true}
//	④ 事件流：state.updated / computer-use/operation-event(kind=turn-completed) /
//	   telemetry——流式活动天然满足看门狗 idle 语义（对比一次性模式全程静默）
//	⑤ turn-completed 后 session/read → messages[]，取末条含 text part 的
//	   assistant 消息为终态：text part 为最终文本，info.tokens 为 usage
//	   （total/input/output/reasoning/cache.read/cache.write），reasoning part
//	   文本量为推理深度
//	⑥ 会话结束调用 finish()——与超时同一条 kill 路径（kill 开关契约不变）
//
// 认证：Z.AI Coding Plan（provider zai 的 key 或 OAuth，~/.zcode/cli/config.json
// 用户侧配置），编排器零 key 注入。桌面可见性：双触发 --open-workspace 注册
// （v0.20 机制，与协议驱动正交）。
package drivers

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// ZcodeArgs RunZcode 入参。
type ZcodeArgs struct {
	Prompt       string
	Model        string // "zai/glm-5.3[-flash]"；空 → ZcodeDefaultModel
	ThoughtLevel string // 空 → "max"（GLM 档位 [low,high,max]，§7 统一档）
	Cwd          string
	APIKeys      map[string]string
	Watchdog     watchdog.Opts
}

// ZcodeDefaultModel Execute default 路由的法定模型（§7 矩阵：旗舰跑量）。
const ZcodeDefaultModel = "zai/glm-5.3"

// defaultZcodeRuntimePath app 内置官方 runtime（与桌面自动更新同步；第三方
// wrapper zcode-app-cli 的 vendor 快照已实证滞后于 app，弃用）。
const defaultZcodeRuntimePath = "/Applications/ZCode.app/Contents/Resources/glm/zcode.cjs"

// ZcodeServerBinEnv app-server launcher 覆盖环境变量：设为任意可执行 zcode
// 入口（如 wrapper 的 bin）时直接以 `<bin> app-server` 启动；缺省用 node 跑
// app 内置 runtime。与桌面注册二进制（ZcodeDesktopBinEnv）语义分离——同一
// env 双用途会在指向 launcher 时让注册拿错误二进制调 --open-workspace。
const ZcodeServerBinEnv = "FLY_ZCODE_SERVER_BIN"

// ZcodeDesktopBinEnv 桌面注册二进制覆盖（v0.20 语义：默认 macOS 安装路径）。
const ZcodeDesktopBinEnv = "FLY_ZCODE_BIN"

// zcodeRuntimeCommand app-server 子进程命令（测试注入缝：PATH 假二进制）。
var zcodeRuntimeCommand = defaultZcodeRuntimeCommand

func defaultZcodeRuntimeCommand() (string, []string) {
	if bin := os.Getenv(ZcodeServerBinEnv); bin != "" {
		return bin, []string{"app-server"}
	}
	return "node", []string{defaultZcodeRuntimePath, "app-server"}
}

// splitZcodeModel "zai/glm-5.3" → ("zai", "glm-5.3")；无斜杠时 provider 缺省
// zai（zcode 自身命名空间）。
func splitZcodeModel(model string) (providerID, modelID string) {
	providerID, modelID = "zai", model
	if i := strings.IndexByte(model, '/'); i > 0 {
		providerID, modelID = model[:i], model[i+1:]
	}
	return providerID, modelID
}

// ---- 桌面可见性（v0.20 机制，与协议驱动正交）----

// zcodeWorkspaceOpener workspace 注册的系统出口（测试注入缝，AGENTS.md 覆盖
// 纪律：副作用经缝注入而非测试内 mock 进程）。
var zcodeWorkspaceOpener = defaultOpenZcodeWorkspace

// zcodeDesktopGOOS 平台闸注入缝：生产恒 runtime.GOOS，测试可强制 "darwin"
// 让 Linux CI 覆盖注册路径。
var zcodeDesktopGOOS = runtime.GOOS

// defaultZcodeDesktopBin 桌面二进制默认路径（const；env 覆盖见 zcodeDesktopBin）。
const defaultZcodeDesktopBin = "/Applications/ZCode.app/Contents/MacOS/ZCode"

// zcodeDesktopBin 解析桌面二进制路径：env 覆盖权威，缺省官方安装位置。
func zcodeDesktopBin() string {
	if bin := os.Getenv(ZcodeDesktopBinEnv); bin != "" {
		return bin
	}
	return defaultZcodeDesktopBin
}

func defaultOpenZcodeWorkspace(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, zcodeDesktopBin(), "--open-workspace", path)
	if err := cmd.Start(); err != nil {
		return err
	}
	// fire-and-forget，但只 Start 不 Wait 会留僵尸——Wait 仅负责回收。
	go func() { _ = cmd.Wait() }()
	return nil
}

// disableDesktopRegistration 测试闸：Run 级测试不得激活用户桌面（隔离纪律）。
var disableDesktopRegistration = false

// RegisterZcodeWorkspace 在 ZCode 桌面端注册 workspace 路径（macOS-only）并
// 触发该路径 CLI 存储会话的索引导入。实测语义（v0.20 九组探针）：GUI 按
// workspace_key 分组，未注册路径零显示；--open-workspace 每次处理都幂等注册
// 并导入。失败静默：可见性是 UX 增强，绝不阻塞执行链路。
func RegisterZcodeWorkspace(path string) {
	if disableDesktopRegistration || zcodeDesktopGOOS != "darwin" || path == "" {
		return
	}
	// 索引按归一化路径做 workspace_key（/tmp 符号链接注册成功但导入失配，
	// EvalSymlinks 后关闭该边缘——实测验证）。
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	_ = zcodeWorkspaceOpener(path)
}

// ---- 协议状态机 ----

// zcodeProto app-server 会话状态机。事件从 watchdog 的 stdout 复制 goroutine
// 进入（feedLine），首个请求从 InteractStdin goroutine 发出（attach）——全部
// 状态经 mu 串行化；finish 幂等且绝不持 mu 调用（内部走 watchdog 清理路径）。
type zcodeProto struct {
	mu     sync.Mutex
	out    io.Writer
	finish func()

	prompt   string
	provider string
	modelID  string
	thought  string
	wsPath   string
	wsKey    string
	nextID   int
	kinds    map[int]string // 已发请求 id → create/send/read

	sessionID string
	created   bool
	sent      bool
	failed    string
	done      bool

	finalText   string
	usage       *Usage
	reasonChars int64
}

func newZcodeProto(prompt, model, thought, wsPath, wsKey string) *zcodeProto {
	provider, modelID := splitZcodeModel(model)
	if thought == "" {
		thought = "max"
	}
	return &zcodeProto{
		prompt: prompt, provider: provider, modelID: modelID, thought: thought,
		wsPath: wsPath, wsKey: wsKey, nextID: 1, kinds: map[int]string{},
	}
}

// attach InteractStdin 钩子：注入写端与 finish 后立即发出 session/create。
func (z *zcodeProto) attach(w io.WriteCloser, finish func()) {
	z.mu.Lock()
	z.out, z.finish = w, finish
	z.mu.Unlock()
	z.requestCreate()
}

// requestCreate 持锁发送 session/create；失败即终结。
// sendLocked 写一行 JSON 到子进程 stdin（需已持 mu）。载荷只含 JSON 安全的
// 标量/映射，Marshal 不可能失败（覆盖率纪律：不写不可达防御分支）。
func (z *zcodeProto) sendLocked(obj map[string]any) error {
	if z.out == nil {
		return nil // attach 前的事件回声（服务器主动通知先于钩子的理论竞态）
	}
	data, _ := json.Marshal(obj)
	_, err := z.out.Write(append(data, '\n'))
	return err
}

// requestCreate 持锁发送 session/create；失败即终结。
func (z *zcodeProto) requestCreate() {
	z.mu.Lock()
	defer z.mu.Unlock()
	id := z.nextID
	z.nextID++
	z.kinds[id] = "create"
	err := z.sendLocked(map[string]any{"id": id, "method": "session/create", "params": map[string]any{
		"workspace":    map[string]any{"workspacePath": z.wsPath, "workspaceKey": z.wsKey},
		"model":        map[string]any{"providerId": z.provider, "modelId": z.modelID},
		"thoughtLevel": z.thought,
		// 无人值守必须跳过审批（一次性 --mode yolo 的协议对应物；对执行期
		// 生效语义列入 §10-6 首跑验证）。
		"mode": "yolo",
	}})
	if err != nil {
		z.failLocked(err.Error())
	}
}

// maybeSendPrompt create 应答与 sessionId 双备即投递（两事件到达顺序均兼容）。
func (z *zcodeProto) maybeSendPrompt() {
	z.mu.Lock()
	defer z.mu.Unlock()
	if z.sent || !z.created || z.sessionID == "" || z.failed != "" {
		return
	}
	z.sent = true
	id := z.nextID
	z.nextID++
	z.kinds[id] = "send"
	err := z.sendLocked(map[string]any{"id": id, "method": "session/send", "params": map[string]any{
		"sessionId": z.sessionID, "content": z.prompt,
	}})
	if err != nil {
		z.failLocked(err.Error())
	}
}

// requestRead turn 完成后拉取终态（拉模式：文本/usage 不在推送事件里）。
func (z *zcodeProto) requestRead() {
	z.mu.Lock()
	defer z.mu.Unlock()
	if z.done || z.failed != "" {
		return
	}
	id := z.nextID
	z.nextID++
	z.kinds[id] = "read"
	err := z.sendLocked(map[string]any{"id": id, "method": "session/read", "params": map[string]any{
		"sessionId": z.sessionID,
	}})
	if err != nil {
		z.failLocked(err.Error())
	}
}

// failLocked 记录失败并终结会话（需已持 mu；finish 经 goroutine 调用防死锁）。
func (z *zcodeProto) failLocked(msg string) {
	if z.failed == "" {
		z.failed = msg
	}
	if z.finish != nil {
		go z.finish()
	}
}

// feedLine 消费一行 stdout JSON（NDJSON）。非 JSON 行忽略（容错纪律同 pi/omp）。
func (z *zcodeProto) feedLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}
	var ev struct {
		ID     any             `json:"id"` // 数字（我方请求）或字符串（服务器反向请求 "server-N"）
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"` // 服务器通知/反向请求的载荷
		Result json.RawMessage `json:"result"` // 我方请求应答的载荷
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(trimmed), &ev); err != nil {
		return
	}
	if ev.Error != nil && ev.Error.Message != "" {
		z.mu.Lock()
		z.failLocked("zcode protocol: " + ev.Error.Message)
		z.mu.Unlock()
		return
	}
	switch ev.Method {
	case "session/requestRuntimePreferences":
		var p struct {
			SessionID string `json:"sessionId"`
		}
		_ = json.Unmarshal(ev.Params, &p)
		z.mu.Lock()
		if p.SessionID != "" && z.sessionID == "" {
			z.sessionID = p.SessionID
		}
		err := z.sendLocked(map[string]any{"id": ev.ID, "result": map[string]any{
			"nativeSearchEnhancementsEnabled": false,
		}})
		if err != nil {
			z.failLocked(err.Error())
		}
		z.mu.Unlock()
		z.maybeSendPrompt()
	case "state.updated":
		var p struct {
			SessionID string `json:"sessionId"`
		}
		_ = json.Unmarshal(ev.Params, &p)
		z.mu.Lock()
		if p.SessionID != "" && z.sessionID == "" {
			z.sessionID = p.SessionID
		}
		z.mu.Unlock()
		z.maybeSendPrompt()
	case "computer-use/operation-event":
		var p struct {
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(ev.Params, &p)
		if p.Kind == "turn-completed" {
			z.requestRead()
		}
	case "": // 我方请求的应答（id 数字；载荷在 result 字段）
		z.handleResponse(ev.ID, ev.Result)
	}
}

// handleResponse 按发请求时的 kind 分发应答（id 只认数字——我方请求的编号）。
func (z *zcodeProto) handleResponse(idAny any, params json.RawMessage) {
	id, ok := idAny.(float64)
	if !ok {
		return
	}
	z.mu.Lock()
	kind := z.kinds[int(id)]
	if kind == "create" {
		z.created = true
	}
	z.mu.Unlock()
	switch kind {
	case "create":
		z.maybeSendPrompt()
	case "read":
		z.consumeRead(params)
	}
}

// consumeRead 解析 session/read 应答：末条含 text part 的 assistant 消息为
// 终态；提取文本、usage、reasoning 字符量后终结会话。
func (z *zcodeProto) consumeRead(raw json.RawMessage) {
	var res struct {
		Messages []struct {
			Info struct {
				Role   string `json:"role"`
				Tokens *struct {
					Total     int64 `json:"total"`
					Input     int64 `json:"input"`
					Output    int64 `json:"output"`
					Reasoning int64 `json:"reasoning"`
					Cache     struct {
						Read  int64 `json:"read"`
						Write int64 `json:"write"`
					} `json:"cache"`
				} `json:"tokens"`
			} `json:"info"`
			Parts []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		z.mu.Lock()
		z.failLocked("zcode protocol: session/read unparseable: " + err.Error())
		z.mu.Unlock()
		return
	}
	var text string
	var usage *Usage
	var reasonChars int64
	for i := len(res.Messages) - 1; i >= 0; i-- {
		m := res.Messages[i]
		if m.Info.Role != "assistant" {
			continue
		}
		gotText := false
		for _, part := range m.Parts {
			switch part.Type {
			case "text":
				if part.Text != "" {
					text, gotText = part.Text, true
				}
			case "reasoning":
				reasonChars += int64(len(part.Text))
			}
		}
		if gotText {
			if t := m.Info.Tokens; t != nil {
				usage = &Usage{
					Input: t.Input, Output: t.Output, Reasoning: t.Reasoning,
					CacheRead: t.Cache.Read, CacheWrite: t.Cache.Write,
					TotalTokens: t.Total,
				}
			}
			break
		}
	}
	z.mu.Lock()
	defer z.mu.Unlock()
	if text == "" && z.failed == "" {
		z.failed = "zcode protocol: no assistant text in session/read"
	}
	z.finalText, z.usage, z.reasonChars = text, usage, reasonChars
	z.done = true
	if z.finish != nil {
		go z.finish()
	}
}

// zcodeOutcome 协议终态快照（值拷贝出锁）。
type zcodeOutcome struct {
	sessionID   string
	text        string
	usage       *Usage
	reasonChars int64
	failed      string
	done        bool
}

func (z *zcodeProto) snapshot() zcodeOutcome {
	z.mu.Lock()
	defer z.mu.Unlock()
	return zcodeOutcome{
		sessionID: z.sessionID, text: z.finalText, usage: z.usage,
		reasonChars: z.reasonChars, failed: z.failed, done: z.done,
	}
}

// RunZcode 驱动一次 zcode app-server 会话（全新会话，干完即弃——§3 一次性
// 语义）。桌面可见性双触发：spawn 前注册导入历史，spawn 后导入本会话。
func RunZcode(a ZcodeArgs) Result {
	RegisterZcodeWorkspace(a.Cwd)
	wsKey := a.Cwd
	if resolved, err := filepath.EvalSymlinks(a.Cwd); err == nil && resolved != "" {
		wsKey = resolved // 协议 workspaceKey 与桌面索引同源：归一化真实路径
	}
	model := a.Model
	if model == "" {
		model = ZcodeDefaultModel
	}
	proto := newZcodeProto(a.Prompt, model, a.ThoughtLevel, a.Cwd, wsKey)

	var lineBuf string
	onActivity := func(chunk, stream string) {
		if stream != "stdout" {
			return
		}
		lineBuf += chunk
		for {
			i := strings.IndexByte(lineBuf, '\n')
			if i < 0 {
				break
			}
			proto.feedLine(lineBuf[:i])
			lineBuf = lineBuf[i+1:]
		}
	}

	name, cmdArgs := zcodeRuntimeCommand()
	result := watchdog.Run(name, cmdArgs, watchdog.Opts{
		Cwd:           a.Cwd,
		Env:           childenv.EnvSlice(childenv.BuildChildEnv(childenv.OSLookupEnv, a.APIKeys)),
		IdleTimeoutMs: a.Watchdog.IdleTimeoutMs,
		WallTimeoutMs: a.Watchdog.WallTimeoutMs,
		KillGraceMs:   a.Watchdog.KillGraceMs,
		OnActivity:    onActivity,
		InteractStdin: proto.attach,
	})
	RegisterZcodeWorkspace(a.Cwd)

	oc := proto.snapshot()
	out := Result{
		ExitCode:     result.ExitCode,
		Signal:       result.Signal,
		TimedOut:     result.TimedOut,
		IdleTimedOut: result.IdleTimedOut,
		WallTimedOut: result.WallTimedOut,
		Stdout:       result.Stdout,
		Stderr:       result.Stderr,
		Truncated:    result.Truncated,
		DurationMS:   result.DurationMS,
		SpawnError:   result.SpawnError,
		Harness:      "zcode",
		Provider:     "zai-coding-plan",
		Model:        model,
		SessionID:    oc.sessionID,
		Text:         oc.text,
	}
	if oc.usage != nil {
		u := *oc.usage
		out.Usage = &u
	}
	if oc.reasonChars > 0 {
		rc := oc.reasonChars
		out.ReasoningChars = &rc
	}
	// Settled 由协议完成度决定（进程终态被 finish 的 SIGTERM 收割是正常路径，
	// 不读作失败——与出口契约"正交记录"语义一致）。
	out.Settled = oc.done && oc.failed == "" && oc.text != "" &&
		!result.TimedOut && result.SpawnError == ""
	if !out.Settled {
		msg := oc.failed
		if msg == "" && result.TimedOut {
			msg = "zcode: session did not complete (idle/wall timeout)"
		}
		if msg == "" {
			msg = strings.TrimSpace(result.Stderr)
		}
		if msg == "" && result.SpawnError != "" {
			msg = result.SpawnError
		}
		if msg == "" {
			msg = "zcode: session ended without completion"
		}
		out.ErrorMessage = &msg
	}
	return out
}
