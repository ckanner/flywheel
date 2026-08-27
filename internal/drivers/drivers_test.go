package drivers

import (
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

// fakeBin 写一个可执行脚本到 t.TempDir() 并返回前置 PATH。脚本把 "$@" 逐行
// 记录到 args.txt，把环境写到 env.txt，再输出给定 stdout 内容。
func fakeBin(t *testing.T, name, stdout string) (binDir, workDir string) {
	t.Helper()
	binDir = t.TempDir()
	workDir = t.TempDir()
	argsPath := filepath.Join(workDir, "args.txt")
	envPath := filepath.Join(workDir, "env.txt")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do printf '%s\\n' \"$a\" >> " + shQuote(argsPath) + "; done\n" +
		"env > " + shQuote(envPath) + "\n" +
		"cat <<'NDJSON'\n" + stdout + "\nNDJSON\n"
	require.NoError(t, os.WriteFile(filepath.Join(binDir, name), []byte(script), 0o755))
	return binDir, workDir
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func setPath(t *testing.T, binDir string) {
	t.Helper()
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

const piNDJSON = `{"type":"system","x":1}
not-json-noise
{"type":"message_end","message":{"role":"user","content":[]}}
{"type":"agent_settled"}
{"type":"message_end","message":{"role":"assistant","provider":"opencode-go","model":"deepseek-v4-flash","stopReason":"stop","usage":{"input":100,"output":50,"cacheRead":20,"cacheWrite":10,"totalTokens":150,"cost":{"total":0.002}},"content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"final answer"}]}}
`

func TestParsePiEventStream(t *testing.T) {
	p := ParsePiEventStream(piNDJSON)
	assert.True(t, p.Settled)
	assert.Equal(t, "opencode-go", p.Provider)
	assert.Equal(t, "deepseek-v4-flash", p.Model)
	assert.Equal(t, int64(100), p.Usage.Input)
	assert.Equal(t, int64(50), p.Usage.Output)
	assert.Equal(t, int64(20), p.Usage.CacheRead)
	assert.Equal(t, int64(10), p.Usage.CacheWrite)
	assert.Equal(t, int64(150), p.Usage.TotalTokens)
	assert.Equal(t, int64(3), p.ReasoningChars) // len("hmm")
	assert.Equal(t, "final answer", p.Text)
	require.NotNil(t, p.CostUSD)
	assert.InDelta(t, 0.002, *p.CostUSD, 1e-12)
	assert.Empty(t, p.ErrorMessage)

	// 流内错误 + reasoning 缺省（附录 D）+ totalTokens 缺省 = input+output。
	errStream := `{"type":"message_end","message":{"role":"assistant","provider":"p","model":"m","stopReason":"error","errorMessage":"401: bad key","usage":{"input":7,"output":3},"content":[]}}`
	p2 := ParsePiEventStream(errStream)
	assert.Equal(t, "401: bad key", p2.ErrorMessage)
	assert.False(t, p2.Settled)
	assert.Equal(t, int64(10), p2.Usage.TotalTokens)
	assert.Nil(t, p2.CostUSD, "流未报成本 → nil")

	// 多事件累计。
	p3 := ParsePiEventStream(`{"type":"message_end","message":{"role":"assistant","usage":{"input":1},"content":[]}}
{"type":"message_end","message":{"role":"assistant","usage":{"input":2},"content":[]}}`)
	assert.Equal(t, int64(3), p3.Usage.Input)
}

func TestRunPiAgainstFakeBinary(t *testing.T) {
	binDir, workDir := fakeBin(t, "pi", piNDJSON)
	setPath(t, binDir)

	var mu sync.Mutex
	var events []string
	r := RunPi(PiArgs{
		Prompt:   "do the thing",
		Model:    "opencode-go/deepseek-v4-flash",
		Thinking: "max",
		Cwd:      workDir,
		APIKey:   "sk-test",
		APIKeys:  map[string]string{"OPENCODE_API_KEY": "sk-channel"},
		Watchdog: watchdog.Opts{IdleTimeoutMs: 120000, WallTimeoutMs: 300000}, // 全量跑时多包并行抢 CPU，超时必须宽裕
		OnEvent:  func(line string) { mu.Lock(); events = append(events, line); mu.Unlock() },
	})
	assert.Empty(t, r.SpawnError)
	require.NotNil(t, r.ExitCode)
	assert.Equal(t, 0, *r.ExitCode)
	assert.Equal(t, "pi", r.Harness)
	assert.True(t, r.Settled)
	assert.Equal(t, "final answer", r.Text)
	assert.NotNil(t, r.Usage)
	assert.Equal(t, int64(150), r.Usage.TotalTokens)

	// 参数逐行落盘：prompt / model / thinking / no-session / mode json / api-key。
	raw, err := os.ReadFile(filepath.Join(workDir, "args.txt"))
	require.NoError(t, err)
	argsText := string(raw)
	assert.Contains(t, argsText, "do the thing")
	assert.Contains(t, argsText, "--model")
	assert.Contains(t, argsText, "max")
	assert.Contains(t, argsText, "--no-session")
	assert.Contains(t, argsText, "json")
	assert.Contains(t, argsText, "--api-key")
	assert.Contains(t, argsText, "sk-test")

	// 凭证卫生：渠道 key 注入、编排器密钥不泄入。
	envRaw, err := os.ReadFile(filepath.Join(workDir, "env.txt"))
	require.NoError(t, err)
	envText := string(envRaw)
	assert.Contains(t, envText, "OPENCODE_API_KEY=sk-channel")
	assert.NotContains(t, envText, "FLYWHEEL_ORCH_SECRET")

	mu.Lock()
	defer mu.Unlock()
	assert.NotEmpty(t, events, "onEvent 应收到 NDJSON 行")
}

func TestParseKimiStreamJson(t *testing.T) {
	stream := `{"role":"meta","type":"system.version"}
{"role":"meta","type":"session.resume_hint","session_id":"session_abc"}
{"role":"assistant","content":"hello plan"}
{"role":"assistant","content":[{"text":"part1"},{"text":"part2"}]}
{"type":"error","message":"kimi blew up"}
`
	p := ParseKimiStreamJson(stream)
	assert.Equal(t, "session_abc", p.SessionID)
	assert.Equal(t, "part1part2", p.Text, "数组 content 覆盖字符串 content")
	assert.Equal(t, "kimi blew up", p.ErrorMessage)
	assert.Nil(t, p.Usage)

	p2 := ParseKimiStreamJson(`{"role":"assistant","content":"plain"}`)
	assert.Equal(t, "plain", p2.Text)

	p3 := ParseKimiStreamJson(`{"role":"error","error":"raw error"}`)
	assert.Equal(t, "raw error", p3.ErrorMessage)

	p4 := ParseKimiStreamJson(`{"usage":{"input":9,"output":1}}`)
	require.NotNil(t, p4.Usage)
	assert.Equal(t, int64(9), p4.Usage.Input)
}

func TestRunKimiAgainstFakeBinary(t *testing.T) {
	stream := `{"role":"meta","type":"session.resume_hint","session_id":"session_new"}
{"role":"assistant","content":"the plan text"}
`
	binDir, workDir := fakeBin(t, "kimi", stream)
	setPath(t, binDir)

	r := RunKimi(KimiArgs{
		Prompt:    "plan this milestone",
		SessionID: "session_old",
		Model:     "kimi-code/k3",
		Cwd:       workDir,
		APIKeys:   map[string]string{},
	})
	assert.Empty(t, r.SpawnError)
	assert.Equal(t, "kimi", r.Harness)
	assert.Equal(t, "kimi", r.Provider)
	assert.Equal(t, "session_new", r.SessionID, "resume hint 捕获新会话 id")
	assert.Equal(t, "the plan text", r.Text)
	assert.True(t, r.Settled)
	assert.Nil(t, r.Usage, "kimi 流不暴露 usage")

	raw, err := os.ReadFile(filepath.Join(workDir, "args.txt"))
	require.NoError(t, err)
	argsText := string(raw)
	assert.Contains(t, argsText, "-r")
	assert.Contains(t, argsText, "session_old")
	assert.Contains(t, argsText, "-m")
	assert.Contains(t, argsText, "kimi-code/k3")
	assert.Contains(t, argsText, "stream-json")

	envRaw, err := os.ReadFile(filepath.Join(workDir, "env.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(envRaw), "KIMI_MODEL_THINKING_EFFORT=max")

	// 非零退出且 stderr 有内容 → errorMessage 取 stderr。
	failBin, failDir := fakeBin(t, "kimi", "")
	setPath(t, failBin)
	failScript := filepath.Join(failBin, "kimi")
	require.NoError(t, os.WriteFile(failScript, []byte("#!/bin/sh\necho 'Session dead not found' >&2\nexit 1\n"), 0o755))
	r2 := RunKimi(KimiArgs{Prompt: "p", Cwd: failDir})
	require.NotNil(t, r2.ExitCode)
	assert.Equal(t, 1, *r2.ExitCode)
	require.NotNil(t, r2.ErrorMessage)
	assert.Contains(t, *r2.ErrorMessage, "Session dead not found")
	assert.False(t, r2.Settled)
}

// ---- omp driver（PR#3 review 后新增）----

func TestParseOmpEventStreamAgentEnd(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"session","version":3,"id":"01a","cwd":"/tmp"}`,
		`{"type":"message_end","message":{"role":"assistant","provider":"opencode-go","model":"glm-5.3-flash","usage":{"input":10,"output":2,"cacheRead":5,"reasoningTokens":7},"stopReason":"stop"}}`,
	}, "\n")
	p := ParseOmpEventStream(stream)
	assert.Equal(t, "opencode-go", p.Provider)
	assert.Equal(t, "glm-5.3-flash", p.Model)
	assert.Equal(t, int64(10), p.Usage.Input)
	assert.False(t, p.Settled, "omp 不产生 agent_settled——以 agent_end 为准")
}

func TestRunOmpAgainstFakeBinary(t *testing.T) {
	stream := `{"type":"session","version":3,"id":"01a"}
{"type":"message_end","message":{"role":"assistant","provider":"opencode-go","model":"glm-5.3-flash","content":[{"type":"text","text":"review done"},{"type":"thinking","thinking":"pondering"}],"stopReason":"stop"}}`
	binDir, workDir := fakeBin(t, "omp", stream)
	setPath(t, binDir)

	var events []string
	r := RunOmp(OmpArgs{
		Prompt:   "review this",
		Model:    "opencode-go/glm-5.3-flash",
		Thinking: "max",
		Cwd:      workDir,
		APIKeys:  map[string]string{},
		OnEvent: func(line string) {
			events = append(events, line)
		},
	})
	assert.Empty(t, r.SpawnError)
	assert.Equal(t, "omp", r.Harness)
	assert.Equal(t, "opencode-go", r.Provider)
	assert.Equal(t, "glm-5.3-flash", r.Model)
	assert.Equal(t, "review done", r.Text)
	require.NotNil(t, r.ReasoningChars)
	assert.Positive(t, *r.ReasoningChars)
	assert.NotEmpty(t, events, "OnEvent 回调必须收到完整 NDJSON 行")

	raw, err := os.ReadFile(filepath.Join(workDir, "args.txt"))
	require.NoError(t, err)
	argsText := string(raw)
	assert.Contains(t, argsText, "--no-session")
	assert.Contains(t, argsText, "--mode")
	assert.Contains(t, argsText, "json")
}

// TestRunOmpAgainstFakeBinaryWithErrorAndKey 覆盖错误捕获臂、--api-key 注入臂
// 与 stderr 活动臂（坏 key 在流内产生 stopReason:error + errorMessage，进程仍
// exit 0——与 pi 同款陷阱）。
func TestRunOmpAgainstFakeBinaryWithErrorAndKey(t *testing.T) {
	// stderr 活动 → onActivity 的非 stdout 分支；流内 error → ErrorMessage；
	// model 无 provider 前缀 → 回退拆分臂。
	stream := `noise-on-stderr-skipped
{"type":"message_end","message":{"role":"assistant","provider":"opencode-go","model":"glm-5.3-flash","stopReason":"error","errorMessage":"401: Invalid API key."}}`
	binDir, workDir := fakeBin(t, "omp", stream)
	scriptPath := filepath.Join(binDir, "omp")
	raw, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(scriptPath, append(raw, []byte("echo stderr-noise >&2\n")...), 0o755))
	setPath(t, binDir)

	r := RunOmp(OmpArgs{
		Prompt:  "x",
		Model:   "opencode-go/glm-5.3-flash",
		Cwd:     workDir,
		APIKey:  "sk-injected-test-key",
		APIKeys: map[string]string{},
		OnEvent: func(string) {},
	})
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "401")

	raw2, err := os.ReadFile(filepath.Join(workDir, "args.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(raw2), "--api-key")
}

// TestInvokeHarnessDispatchesOmp harness=omp 分发臂（PR#3 review 覆盖缺口）。
func TestInvokeHarnessDispatchesOmp(t *testing.T) {
	stream := `{"type":"session","version":3,"id":"01a"}
{"type":"agent_end","messages":[]}`
	binDir, workDir := fakeBin(t, "omp", stream)
	setPath(t, binDir)

	r := InvokeHarness(CallArgs{
		Prompt:  "dispatch via omp",
		Binding: config.LoopBinding{Harness: "omp", Model: "opencode-go/glm-5.3-flash"},
		Cwd:     workDir,
		APIKeys: map[string]string{},
	}, "")
	assert.Equal(t, "omp", r.Harness)
	assert.Equal(t, "opencode-go/glm-5.3-flash", r.Model)
}

// InvokeHarness 必须把 machine 层持有的长驻会话句柄传进 kimi CLI（-r <id>）。
func TestInvokeHarnessThreadsPlanSessionIDIntoKimi(t *testing.T) {
	binDir, workDir := fakeBin(t, "kimi", `{"role":"meta","type":"session.resume_hint","session_id":"session_new"}`)
	setPath(t, binDir)
	r := InvokeHarness(CallArgs{
		Prompt:    "continue planning",
		SessionID: "session_keep",
		Binding:   config.LoopBinding{Harness: "kimi", Model: "kimi-code/k3"},
		Cwd:       workDir,
		APIKeys:   map[string]string{},
	}, "")
	assert.Empty(t, r.SpawnError)
	raw, err := os.ReadFile(filepath.Join(workDir, "args.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "session_keep")
}

func TestMockDriverDeterministicDeliverables(t *testing.T) {
	d := NewMockDriver(nil)
	taskDir := t.TempDir()
	cwd := t.TempDir()

	r := d.Execute(CallArgs{PacketID: "M01-T01", TaskDir: taskDir, Cwd: cwd})
	assert.Equal(t, 0, *r.ExitCode)
	assert.Equal(t, "mock executed M01-T01", r.Text)
	assert.True(t, r.Settled)

	artifact, err := os.ReadFile(filepath.Join(cwd, "build", "M01-T01.txt"))
	require.NoError(t, err)
	assert.Equal(t, "ok M01-T01\n", string(artifact))

	report, err := os.ReadFile(filepath.Join(taskDir, "reports", "M01-T01.done.json"))
	require.NoError(t, err)
	assert.Contains(t, string(report), `"changed-files"`)
	assert.Contains(t, string(report), "build/M01-T01.txt")
}

func TestMockDriverBehaviorOverrides(t *testing.T) {
	behaviors := map[string]any{
		"execute": map[string]any{
			"M02-T01": BehaviorFn(func(a CallArgs) *Result {
				return &Result{ExitCode: intPtr(3), Text: "injected failure", Settled: false}
			}),
		},
	}
	d := NewMockDriver(behaviors)
	r := d.Execute(CallArgs{PacketID: "M02-T01", TaskDir: t.TempDir()})
	require.NotNil(t, r.ExitCode)
	assert.Equal(t, 3, *r.ExitCode)
	assert.False(t, r.Settled)

	// 无覆盖的 packet 走默认路径。
	r2 := d.Execute(CallArgs{PacketID: "M03-T01", TaskDir: t.TempDir()})
	assert.Equal(t, "mock executed M03-T01", r2.Text)
}

func TestMockDriverAllRoles(t *testing.T) {
	d := NewMockDriver(nil)
	a := CallArgs{MilestoneID: "M-01", PacketID: "M01-T01", Packets: []string{"M01-T01"}}

	pr := d.Plan(a)
	assert.Contains(t, pr.Text, `"milestone":"M-01"`)
	assert.Contains(t, pr.Text, `"action":"issue"`)

	rr := d.Review(a)
	assert.JSONEq(t, `{"verdict":"pass","score":1,"kind":"ok","reasons":["mock: L1 commands exit 0 (deterministic evidence)"]}`, rr.Text)

	jr := d.Judge(a)
	assert.Contains(t, jr.Text, `"verdict":"pass"`)

	ir := d.Intake(a)
	assert.Contains(t, ir.Text, "recommend")

	gr := d.Garden(a)
	assert.Contains(t, gr.Text, "## GP-01")

	for _, res := range []Result{pr, rr, jr, ir, gr} {
		assert.Equal(t, "mock", res.Harness)
		assert.Equal(t, "mock-model", res.Model)
	}
	assert.Equal(t, "mock", d.Name())
}

func TestCreateDriversSelectionAndRealDispatch(t *testing.T) {
	binDir, _ := fakeBin(t, "pi", piNDJSON)
	setPath(t, binDir)

	cfg := config.Default()
	cfg.Drivers = map[string]string{"execute": "mock", "review": "mock", "judge": "mock", "intake": "mock", "garden": "mock"}
	drivers := CreateDrivers(&cfg)

	assert.Equal(t, "real", drivers.Plan.Name())
	assert.Equal(t, "mock", drivers.Execute.Name())
	assert.Equal(t, "mock", drivers.Garden.Name())

	// Plan 是 real → 经 InvokeHarness 分发 pi（fake 二进制）。
	r := drivers.Plan.Plan(CallArgs{
		Prompt:  "plan prompt",
		Binding: config.LoopBinding{Harness: "pi", Model: "opencode-go/deepseek-v4-flash"},
		Cwd:     t.TempDir(),
	})
	assert.Equal(t, "pi", r.Harness)
	assert.True(t, r.Settled)

	// harness=kimi 分发 kimi 且默认模型兜底。
	binDir2, _ := fakeBin(t, "kimi", `{"role":"assistant","content":"k3 plan"}`)
	setPath(t, binDir2)
	r2 := drivers.Plan.Plan(CallArgs{
		Prompt:  "kimi prompt",
		Binding: config.LoopBinding{Harness: "kimi"},
	})
	assert.Equal(t, "kimi", r2.Harness)
	assert.Equal(t, KimiDefaultModel, r2.Model)
	assert.Equal(t, "k3 plan", r2.Text)
}

func TestRealDriverAllRolesDispatch(t *testing.T) {
	binDir, _ := fakeBin(t, "pi", piNDJSON)
	setPath(t, binDir)
	var real Driver = realDriver{}
	a := CallArgs{Prompt: "p", Binding: config.LoopBinding{Harness: "pi", Model: "m/x"}}
	for _, call := range []func(CallArgs) Result{
		real.Execute, real.Review, real.Judge, real.Intake, real.Garden,
	} {
		r := call(a)
		assert.Equal(t, "pi", r.Harness)
	}
}

func TestInvokeHarnessEmptyBindingDefaultsPi(t *testing.T) {
	binDir, _ := fakeBin(t, "pi", piNDJSON)
	setPath(t, binDir)
	r := InvokeHarness(CallArgs{Prompt: "x", Model: "prov/model"}, "")
	assert.Equal(t, "pi", r.Harness)
}

func TestParseKimiStreamNoiseAndBareError(t *testing.T) {
	p := ParseKimiStreamJson("garbage line\n{\"role\":\"error\"}")
	assert.Empty(t, p.SessionID)
	assert.NotEmpty(t, p.ErrorMessage)
}

func TestRunKimiInStreamErrorPropagates(t *testing.T) {
	binDir, workDir := fakeBin(t, "kimi", `{"type":"error","message":"stream exploded"}`)
	setPath(t, binDir)
	r := RunKimi(KimiArgs{Prompt: "p", Cwd: workDir})
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "stream exploded")
}

func TestMockDriverPlanReviewJudgeOverridesAndNilMilestone(t *testing.T) {
	behaviors := map[string]any{
		"plan": map[string]any{
			"M-09": BehaviorFn(func(a CallArgs) *Result { return &Result{Text: "plan override"} }),
		},
		"review": map[string]any{
			"M01-T01": BehaviorFn(func(a CallArgs) *Result { return &Result{Text: "review override"} }),
		},
		"judge": map[string]any{
			"M-01": BehaviorFn(func(a CallArgs) *Result { return &Result{Text: "judge override"} }),
		},
	}
	d := NewMockDriver(behaviors)
	assert.Contains(t, d.Plan(CallArgs{MilestoneID: "M-09"}).Text, "plan override")
	assert.Contains(t, d.Review(CallArgs{PacketID: "M01-T01"}).Text, "review override")
	assert.Contains(t, d.Judge(CallArgs{MilestoneID: "M-01"}).Text, "judge override")

	// 空 milestone → payload 里 milestone 为 null（nilIfEmpty 分支）。
	assert.Contains(t, d.Plan(CallArgs{}).Text, `"milestone":null`)

	// MockResult 帮手（Node mockResult 对应物）。
	base := MockResult(func(r *Result) { r.Text = "custom" })
	assert.Equal(t, "mock", base.Harness)
	assert.Equal(t, "custom", base.Text)
}

// TestParsePiEventStreamAccumulatesErrors（C6 回归）：多轮错误事件全部保留，
// 不被最后一条覆盖。
func TestParsePiEventStreamAccumulatesErrors(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"message_end","message":{"role":"assistant","stopReason":"error","errorMessage":"401: bad key"}}`,
		`{"type":"message_end","message":{"role":"assistant","stopReason":"error","errorMessage":"429: slow down"}}`,
	}, "\n")
	parsed := ParsePiEventStream(stream)
	assert.Contains(t, parsed.ErrorMessage, "401: bad key")
	assert.Contains(t, parsed.ErrorMessage, "429: slow down")
}

func TestRunPiFallbacksAndStderrSkipAndError(t *testing.T) {
	// 流内无 provider/model → 回退 model 拆分；assistant error → ErrorMessage；
	// stderr 活动 → onActivity 的非 stdout 分支。
	stream := `noise-on-stderr-should-be-skipped
{"type":"message_end","message":{"role":"assistant","stopReason":"error","errorMessage":"boom inside","content":[{"type":"text","text":"partial"}]}}
`
	binDir, workDir := fakeBin(t, "pi", stream)
	// 追加 stderr 输出，触发 onActivity 的 stderr 跳过分支。
	scriptPath := filepath.Join(binDir, "pi")
	raw, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(scriptPath, append(raw, []byte("echo stderr-noise >&2\n")...), 0o755))
	setPath(t, binDir)

	r := RunPi(PiArgs{
		Prompt:  "p",
		Model:   "prov/model-x",
		Cwd:     workDir,
		OnEvent: func(string) {},
	})
	assert.Equal(t, "prov", r.Provider, "流内无 provider → 从 model 前缀回退")
	assert.Equal(t, "prov/model-x", r.Model, "流内无 model → 回退入参完整 model id")
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "boom inside")
}
