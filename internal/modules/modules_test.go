package modules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/fallback"
)

func writeFileT(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func mustJSONT(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}

// stubDriver 可脚本化驱动：按角色返回固定结果；未配置的角色走 mock 基底。
type stubDriver struct {
	mock    *drivers.MockDriver
	results map[string]drivers.Result
	calls   []string
}

func newStub(results map[string]drivers.Result) *stubDriver {
	return &stubDriver{mock: drivers.NewMockDriver(nil), results: results}
}

func (d *stubDriver) Name() string { return "stub" }

func (d *stubDriver) result(role string, a drivers.CallArgs) drivers.Result {
	d.calls = append(d.calls, role)
	if r, ok := d.results[role]; ok {
		return r
	}
	switch role {
	case "plan":
		return d.mock.Plan(a)
	case "execute":
		return d.mock.Execute(a)
	case "review":
		return d.mock.Review(a)
	case "judge":
		return d.mock.Judge(a)
	case "intake":
		return d.mock.Intake(a)
	default:
		return d.mock.Garden(a)
	}
}

func (d *stubDriver) Plan(a drivers.CallArgs) drivers.Result    { return d.result("plan", a) }
func (d *stubDriver) Execute(a drivers.CallArgs) drivers.Result { return d.result("execute", a) }
func (d *stubDriver) Review(a drivers.CallArgs) drivers.Result  { return d.result("review", a) }
func (d *stubDriver) Judge(a drivers.CallArgs) drivers.Result   { return d.result("judge", a) }
func (d *stubDriver) Intake(a drivers.CallArgs) drivers.Result  { return d.result("intake", a) }
func (d *stubDriver) Garden(a drivers.CallArgs) drivers.Result  { return d.result("garden", a) }

func testConfigT(t *testing.T, taskDir string) *config.Config {
	t.Helper()
	// Default() 无 channels/loops（任务级配置）——测试必须自带，否则
	// ResolveBinding/EnvKeyForChannel 找不到绑定。
	cfg := config.Default()
	cfg.Channels = map[string]config.Channel{
		"opencode-go":       {EnvKey: strPtrT("OPENCODE_API_KEY"), Fallback: "opencode"},
		"kimi-subscription": {Fallback: "kimi-coding"},
		"kimi-coding":       {EnvKey: strPtrT("KIMI_API_KEY"), Fallback: "opencode"},
	}
	cfg.Loops = map[string]config.LoopBinding{
		"plan":   {Harness: "kimi", Model: "kimi-code/k3", Channel: "kimi-subscription", Thinking: "max"},
		"review": {Harness: "pi", Model: "opencode-go/deepseek-v4-pro", Channel: "opencode-go", Thinking: "max"},
		"judge":  {Harness: "pi", Model: "kimi-coding/k3", Channel: "kimi-coding", Thinking: "max"},
		"intake": {Harness: "pi", Model: "opencode-go/deepseek-v4-flash", Channel: "opencode-go", Thinking: "max"},
		"garden": {Harness: "kimi", Model: "kimi-code/k3", Channel: "kimi-subscription", Thinking: "max"},
	}
	cfg.Routes = map[string]config.RouteBinding{
		"default": {Harness: "pi", Model: "opencode-go/deepseek-v4-flash", Channel: "opencode-go", Thinking: "max"},
	}
	cfg.Drivers = map[string]string{
		"plan": "mock", "execute": "mock", "review": "mock",
		"judge": "mock", "intake": "mock", "garden": "mock",
	}
	writeFileT(t, filepath.Join(taskDir, "flywheel.config.json"), mustJSONT(t, cfg))
	loaded, err := config.Load(taskDir, taskDir)
	require.NoError(t, err)
	return loaded
}

func strPtrT(s string) *string { return &s }

func bindingFor(t *testing.T, cfg *config.Config, role string) fallback.Binding {
	t.Helper()
	b, err := fallback.ResolveBinding(cfg, role, "")
	require.NoError(t, err)
	return b
}

func TestNewContextDefaults(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg})
	assert.Equal(t, taskDir, m.TaskDir)
	assert.NotEmpty(t, m.Cwd)
	assert.Equal(t, filepath.Join(taskDir, "metrics.jsonl"), m.MetricsFile)

	var logged []string
	m2 := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Log: func(s string) { logged = append(logged, s) }})
	m2.Log("hello")
	assert.Equal(t, []string{"hello"}, logged)
}

func TestModuleWatchdogOpts(t *testing.T) {
	cfg := config.Default()
	cfg.Watchdog.IdleTimeoutMs = 111
	cfg.Watchdog.WallTimeoutMs = 222
	cfg.Watchdog.KillGraceMs = 33
	o := ModuleWatchdogOpts(&cfg)
	assert.Equal(t, int64(111), o.IdleTimeoutMs)
	assert.Equal(t, int64(222), o.WallTimeoutMs)
	assert.Equal(t, int64(33), o.KillGraceMs)
}

func TestChannelAPIKeyAndAPIKeysFor(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg})
	t.Setenv("OPENCODE_API_KEY", "sk-live")

	assert.Equal(t, "sk-live", ChannelAPIKey(cfg, "opencode-go", m.EnvLookup))
	keys := APIKeysFor(cfg, "opencode-go", m.EnvLookup)
	assert.Equal(t, map[string]string{"OPENCODE_API_KEY": "sk-live"}, keys)

	// 订阅渠道 env-key=null → 恒空；kimi-coding 用注入值断言（本机可能
	// 真实持有 KIMI_API_KEY，不能假设"未设值"）。
	assert.Empty(t, ChannelAPIKey(cfg, "kimi-subscription", m.EnvLookup))
	t.Setenv("KIMI_API_KEY", "sk-kimi-test")
	assert.Equal(t, "sk-kimi-test", ChannelAPIKey(cfg, "kimi-coding", m.EnvLookup))
	assert.Equal(t, map[string]string{"KIMI_API_KEY": "sk-kimi-test"}, APIKeysFor(cfg, "kimi-coding", m.EnvLookup))
}

func TestCallAgentSuccessRetryAndAuthFailLoud(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	stub := newStub(nil)
	m := NewContext(CtxOpts{
		TaskDir: taskDir, Config: cfg,
		Drivers: &drivers.Drivers{Intake: stub, Garden: stub},
		Sleep:   func(int64) {},
	})

	// 成功：mock intake 文本即 recommend JSON。
	r := CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "triage this"})
	assert.True(t, r.OK)
	assert.Contains(t, r.Text, `"verdict":"recommend"`)
	assert.FileExists(t, m.MetricsFile, "每次 spawn 必须落指标")

	// AUTH fail-loud：真实驱动 + 渠道 env 缺失 → 不 spawn 直接 AUTH（P1-12②）。
	cfgReal := testConfigT(t, taskDir)
	cfgReal.Drivers["intake"] = "real"
	os.Unsetenv("OPENCODE_API_KEY")
	mReal := NewContext(CtxOpts{
		TaskDir: taskDir, Config: cfgReal,
		Drivers: &drivers.Drivers{Intake: stub, Garden: stub},
		Sleep:   func(int64) {},
	})
	r3 := CallAgent(mReal, CallOptions{Role: "intake", Binding: bindingFor(t, cfgReal, "intake"), Prompt: "p"})
	assert.False(t, r3.OK)
	assert.Equal(t, errs.AUTH, r3.ErrorCode)
}

func TestRecordModuleMetricsSkipsStateWhenPidFileExists(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	stub := newStub(nil)
	m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: &drivers.Drivers{Intake: stub}})

	// 无 state.json：仅 metrics 追加。
	CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "p"})
	assert.FileExists(t, m.MetricsFile)
	assert.NoFileExists(t, filepath.Join(taskDir, "state.json"))

	// state.json 存在且编排器 pid 文件在飞 → 绝不写 state（P0-10）。
	writeFileT(t, filepath.Join(taskDir, "state.json"),
		`{"schema-version":1,"slug":"t","revision":7,"status":"running","packets":{},"inbox":[]}`)
	writeFileT(t, filepath.Join(taskDir, "orchestrator.pid"), `{"pid":`+strconv.Itoa(os.Getpid())+`}`)
	before, rerr := os.ReadFile(filepath.Join(taskDir, "state.json"))
	require.NoError(t, rerr)
	CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "p"})
	after, rerr2 := os.ReadFile(filepath.Join(taskDir, "state.json"))
	require.NoError(t, rerr2)
	assert.Equal(t, string(before), string(after), "编排器运行中模块绝不能写 state.json")
}

func TestFingerprint(t *testing.T) {
	a := Fingerprint("gh", "Fix login")
	assert.Len(t, a, 12)
	assert.Equal(t, a, Fingerprint("gh", "Fix login"))
	assert.NotEqual(t, a, Fingerprint("rss", "Fix login"))
	assert.NotEqual(t, a, Fingerprint("gh", "Fix login v2"))
	// "::" 分隔防 (source,title) 拼接歧义；title 自带 "::" 时与 Node 同样
	// 可能碰撞（sha256 输入逐字一致），非 bug——不测该场景。
}

func TestParseTriageVerdict(t *testing.T) {
	v, err := ParseTriageVerdict("noise\n{\"verdict\":\"recommend\",\"reason\":\"clear scope\"}")
	require.NoError(t, err)
	assert.Equal(t, "recommend", v.Verdict)
	assert.Equal(t, "clear scope", v.Reason)

	v, err = ParseTriageVerdict("```json\n{\"verdict\":\"skip\",\"reason\":42}\n```")
	require.NoError(t, err)
	assert.Equal(t, "skip", v.Verdict)

	_, err = ParseTriageVerdict("")
	assert.ErrorContains(t, err, "empty triage output")
	_, err = ParseTriageVerdict("{\"verdict\":\"bogus\"}")
	assert.ErrorContains(t, err, "no verdict JSON found in triage output")
}

func TestFetchSourceCandidatesCommand(t *testing.T) {
	src := config.Source{Name: "ci", Type: "command", Cmd: "echo"}
	fn := ExecFunc(func(name string, args []string, cwd string) (string, error) {
		require.Equal(t, "/bin/sh", name)
		out := strings.Join([]string{
			`{"title":"JSON  candidate","summary":"with summary","url":"https://x"}`,
			"",
			"a plain failure line",
			"{broken json",
		}, "\n")
		return out, nil
	})
	cs, err := FetchSourceCandidates(src, "", fn)
	require.NoError(t, err)
	require.Len(t, cs, 3)
	assert.Equal(t, "JSON candidate", cs[0].Title, "空白折叠")
	assert.Equal(t, "with summary", cs[0].Summary)
	assert.Equal(t, "https://x", cs[0].URL)
	assert.Equal(t, "a plain failure line", cs[1].Title)
	assert.Empty(t, cs[1].Summary)
	assert.Equal(t, "{broken json", cs[2].Title, "坏 JSON 落纯行处理")
	for _, c := range cs {
		assert.Equal(t, "ci", c.Source)
		assert.Equal(t, "command", c.SourceType)
	}

	_, err = FetchSourceCandidates(config.Source{Name: "x", Type: "carrier-pigeon"}, "", fn)
	assert.ErrorContains(t, err, "unknown intake source type")
}

func TestRunIntakeEndToEndWithDedup(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	cfg.Intake.Sources = []config.Source{{Name: "cmdsrc", Type: "command", Cmd: "emit"}}
	cfg.Loops["intake"] = config.LoopBinding{
		Harness: "pi", Model: "opencode-go/deepseek-v4-flash", Channel: "opencode-go", Thinking: "max",
	}
	t.Setenv("OPENCODE_API_KEY", "sk-test")

	script := strings.Join([]string{
		`{"title":"Add retry budget","summary":"transport failures repeat","url":"https://issues/1"}`,
		`fix flaky e2e test`,
	}, "\n")
	fn := ExecFunc(func(name string, args []string, cwd string) (string, error) { return script, nil })
	drv := &drivers.Drivers{Intake: newStub(nil), Garden: newStub(nil)}

	sum, err := RunIntake(CtxOpts{TaskDir: taskDir, Cwd: ".", Config: cfg, Drivers: drv}, "", fn)
	require.NoError(t, err)
	assert.True(t, sum.OK)
	assert.Equal(t, 2, sum.Scanned)
	assert.Equal(t, 2, sum.New)
	assert.Equal(t, 0, sum.Duplicates)
	assert.Equal(t, 2, sum.Triaged["recommend"], "mock intake 文本对两条候选都是 recommend")
	assert.Equal(t, 0, sum.Triaged["failed"])

	inboxRaw, rerr := os.ReadFile(sum.InboxFile)
	require.NoError(t, rerr)
	inbox := string(inboxRaw)
	assert.Contains(t, inbox, "# Triage inbox (flywheel intake)")
	assert.Contains(t, inbox, "- approval: pending")
	fp := Fingerprint("cmdsrc", "Add retry budget")
	assert.Contains(t, inbox, "- triage: recommend — mock: looks actionable and verifiable")
	_ = fp

	// 第二轮：同源同输出 → 全部去重，无新增。
	sum2, err := RunIntake(CtxOpts{TaskDir: taskDir, Cwd: ".", Config: cfg, Drivers: drv}, "", fn)
	require.NoError(t, err)
	assert.Equal(t, 2, sum2.Scanned)
	assert.Equal(t, 0, sum2.New)
	assert.Equal(t, 2, sum2.Duplicates)

	approved := ListApprovedCandidates(taskDir)
	assert.Empty(t, approved, "无人批准前列表为空")

	// 人把第一条标记 approved → ListApprovedCandidates 命中。
	marked := strings.Replace(inbox, "- approval: pending", "- approval: approved", 1)
	writeFileT(t, sum.InboxFile, marked)
	approved = ListApprovedCandidates(taskDir)
	require.Len(t, approved, 1)
	assert.NotEmpty(t, approved[0].Fingerprint)

	// 未知源名报错；空源表报错。
	_, err = RunIntake(CtxOpts{TaskDir: taskDir, Cwd: ".", Config: cfg, Drivers: drv}, "nope", fn)
	assert.ErrorContains(t, err, "unknown intake source")
	cfgNoSrc := testConfigT(t, taskDir)
	cfgNoSrc.Intake.Sources = nil
	_, err = RunIntake(CtxOpts{TaskDir: taskDir, Cwd: ".", Config: cfgNoSrc}, "", fn)
	assert.ErrorContains(t, err, "intake.sources is empty")

	// 源执行失败只记日志不阻断。
	failFn := ExecFunc(func(string, []string, string) (string, error) { return "", fmt.Errorf("boom") })
	sum3, err := RunIntake(CtxOpts{TaskDir: taskDir, Cwd: ".", Config: cfg, Drivers: drv}, "cmdsrc", failFn)
	require.NoError(t, err)
	assert.Equal(t, 0, sum3.Scanned)
}

func TestExtractFailurePatterns(t *testing.T) {
	retro := strings.Join([]string{
		"- 2026-08-22T01:02:03.000Z packet M01-T01: INBOX — transport",
		"- 2026-08-22T02:00:00.000Z packet M01-T02: INBOX — transport",
		"- 2026-08-22T03:00:00.000Z packet M01-T03: failed=3 on pnpm lint",
		"- milestone M-01: closed (gate pass, judge 0.9)",
		"- rounds=4 defects(review-failed)=0 nothing bad",
	}, "\n")
	patterns := ExtractFailurePatterns(retro, 2)
	require.Len(t, patterns, 1)
	assert.Contains(t, patterns[0].Pattern, "packet <packet>: INBOX — transport")
	assert.NotRegexp(t, `2026|T01`, patterns[0].Pattern, "时间戳与 packet id 必须归一化")
	assert.Len(t, patterns[0].Lines, 2)

	assert.Empty(t, ExtractFailurePatterns("- failed=0 all good", 2))

	single := ExtractFailurePatterns("- packet M01-T09: INBOX — once only", 2)
	assert.Empty(t, single, "minCount=2 下单次出现不成模式")
}

func TestValidateGardenProposals(t *testing.T) {
	good := strings.Join([]string{
		"## GP-01 — Harden transport retries",
		"- type: failure-pattern",
		`- evidence: retro.md:"packet <packet>: INBOX — transport" (x2)`,
		"- proposal: document the channel-fallback drill as a skill.",
		"",
	}, "\n")
	v := ValidateGardenProposals(good)
	assert.True(t, v.OK, "errors: %v", v.Errors)
	require.Len(t, v.Proposals, 1)
	assert.Equal(t, "GP-01 — Harden transport retries", v.Proposals[0].Title)

	bad := strings.Join([]string{
		"## GP-02 — Unsupported idea",
		"- type: whim",
		"- proposal: do something",
		"## GP-03 — Wrong evidence",
		"- type: decay",
		"- evidence: vibes only",
	}, "\n")
	v = ValidateGardenProposals(bad)
	assert.False(t, v.OK)
	assert.Contains(t, strings.Join(v.Errors, "; "), "missing evidence line")
	assert.Contains(t, strings.Join(v.Errors, "; "), "missing/unknown type")
	assert.Contains(t, strings.Join(v.Errors, "; "), "evidence does not cite")
	assert.Len(t, v.Proposals, 2)

	v = ValidateGardenProposals("no proposals at all")
	assert.False(t, v.OK)
	assert.Equal(t, []string{"no GP blocks found"}, v.Errors)
}

func TestRunGardenEndToEnd(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	cfg.Loops["garden"] = config.LoopBinding{
		Harness: "kimi", Model: "kimi-code/k3", Channel: "kimi-subscription", Thinking: "max",
	}
	writeFileT(t, filepath.Join(taskDir, "retro.md"), strings.Repeat("- x packet M01-T01: INBOX — transport\n", 2))
	// 仓库根带 AGENTS.md 与 skills/（cwd 即 root）。
	root := t.TempDir()
	writeFileT(t, filepath.Join(root, "AGENTS.md"), "# Agents rules")
	writeFileT(t, filepath.Join(root, "skills", "demo-skill", "SKILL.md"), "---\nname: demo-skill\ndescription: demo\n---\nbody")

	sum, err := RunGarden(CtxOpts{TaskDir: taskDir, Cwd: root, Config: cfg,
		Drivers: &drivers.Drivers{Garden: newStub(nil)}})
	require.NoError(t, err)
	assert.True(t, sum.OK, "errors: %v", sum.Errors)
	assert.Equal(t, 1, sum.Proposals)
	raw, rerr := os.ReadFile(*sum.File)
	require.NoError(t, rerr)
	assert.Contains(t, string(raw), "# Garden proposals — ")
	assert.Contains(t, string(raw), "> Proposals only.")

	// agent 调用失败 → ok=false + errors。
	failCfg := testConfigT(t, taskDir)
	stubFail := newStub(map[string]drivers.Result{
		"garden": drivers.MockResult(func(x *drivers.Result) {
			code := 1
			x.ExitCode = &code
			msg := "hung"
			x.ErrorMessage = &msg
			x.Settled = false
		}),
	})
	failCfg.Drivers["garden"] = "real"
	sum2, err := RunGarden(CtxOpts{TaskDir: taskDir, Cwd: root, Config: failCfg,
		Drivers: &drivers.Drivers{Garden: stubFail}})
	require.NoError(t, err)
	assert.False(t, sum2.OK)
	assert.NotEmpty(t, sum2.Errors)
}

func TestListRepoSkillsAndInstallSkills(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		writeFileT(t, filepath.Join(root, "skills", name, "SKILL.md"),
			fmt.Sprintf("---\nname: %s\ndescription: d-%s\n---\nbody", name, name))
	}
	// 无 SKILL.md 的目录不算。
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills", "empty-dir"), 0o755))

	names := ListRepoSkills(root)
	assert.Equal(t, []string{"alpha", "beta"}, names)

	home := t.TempDir()
	sum, err := InstallSkills(InstallSkillsOpts{Root: root, HomeDir: home})
	require.NoError(t, err)
	assert.Equal(t, "user", sum.Scope)
	require.Len(t, sum.Results, 2)
	for _, r := range sum.Results {
		assert.Equal(t, "linked", r.Action)
	}
	anchorRaw, aerr := os.ReadFile(filepath.Join(sum.TargetBase, AnchorFile))
	require.NoError(t, aerr)
	assert.Equal(t, root+"\n", string(anchorRaw))

	// 二次安装：skipped。
	sum2, err := InstallSkills(InstallSkillsOpts{Root: root, HomeDir: home})
	require.NoError(t, err)
	for _, r := range sum2.Results {
		assert.Equal(t, "skipped", r.Action)
	}

	// 链接被改指向别处（模拟陈旧）→ stale 如实上报（P1-11）。skill 源仍在，
	// 才会进入遍历。
	staleLink := filepath.Join(home, ".agents", "skills", "alpha")
	require.NoError(t, os.Remove(staleLink))
	require.NoError(t, os.Symlink("/nonexistent/target", staleLink))
	sum3, err := InstallSkills(InstallSkillsOpts{Root: root, HomeDir: home})
	require.NoError(t, err)
	var staleFound bool
	for _, r := range sum3.Results {
		if r.Name == "alpha" {
			staleFound = true
		}
	}
	assert.True(t, staleFound, "断链必须报 stale 而非 already installed")

	// copy 模式 + force 替换文件条目。
	sum4, err := InstallSkills(InstallSkillsOpts{Root: root, HomeDir: home, Copy: true, Force: true})
	require.NoError(t, err)
	for _, r := range sum4.Results {
		assert.Equal(t, "copied", r.Action)
	}
	assert.FileExists(t, filepath.Join(sum4.TargetBase, "beta", "SKILL.md"))

	// 真目录占位 → error 绝不自动删除（P1-11）。
	realDir := filepath.Join(home, ".agents", "skills", "gamma")
	require.NoError(t, os.MkdirAll(realDir, 0o755))
	writeFileT(t, filepath.Join(root, "skills", "gamma", "SKILL.md"),
		"---\nname: gamma\ndescription: g\n---\nbody")
	sum5, err := InstallSkills(InstallSkillsOpts{Root: root, HomeDir: home})
	require.NoError(t, err)
	foundErr := false
	for _, r := range sum5.Results {
		if r.Name == "gamma" {
			assert.Equal(t, "error", r.Action)
			assert.Contains(t, r.Reason, "refusing to remove")
			foundErr = true
		}
	}
	assert.True(t, foundErr)
	assert.DirExists(t, realDir, "真目录绝不被自动删除")

	// project 作用域。
	project := t.TempDir()
	sum6, err := InstallSkills(InstallSkillsOpts{Root: root, Project: project})
	require.NoError(t, err)
	assert.Contains(t, sum6.TargetBase, filepath.Join(project, ".agents", "skills"))
	assert.Contains(t, sum6.Scope, "project")
}
