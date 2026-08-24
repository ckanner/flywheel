package modules

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/metrics"
	"github.com/ckanner/flywheel/internal/skillsembed"
	"github.com/ckanner/flywheel/internal/verify"
)

// ---- 纯函数直测 ----

func TestTruncateAndOrLimitAndFirstNonEmpty(t *testing.T) {
	assert.Equal(t, "abc", truncate("abcdef", 3))
	assert.Equal(t, "abc", truncate("abc", 3))
	assert.Equal(t, 50, orLimit(0))
	assert.Equal(t, 7, orLimit(7))
	assert.Equal(t, "a", firstNonEmpty("", "", "a", "b"))
	assert.Empty(t, firstNonEmpty())
}

func TestOrEmptyBothBranches(t *testing.T) {
	assert.Equal(t, "(empty)", orEmpty(""))
	assert.Equal(t, "content", orEmpty("content"))
}

func TestRenderActionText(t *testing.T) {
	s := InstallSummary{TargetBase: "/target", Scope: "user", Results: []SkillInstallResult{
		{Name: "alpha", Dest: "/target/alpha", Action: "linked"},
		{Name: "beta", Dest: "/target/beta", Action: "stale", Reason: "symlink -> gone"},
	}}
	out := renderActionText(s)
	assert.Contains(t, out, "installed skills -> /target (user)")
	assert.Contains(t, out, "linked   alpha")
	assert.Contains(t, out, "stale    beta — symlink -> gone")
}

// ---- dispatchRole 全角色 ----

func TestDispatchRoleAllRoles(t *testing.T) {
	stub := newStub(nil)
	drv := &drivers.Drivers{
		Plan: stub, Execute: stub, Review: stub, Judge: stub, Intake: stub, Garden: stub,
	}
	for _, role := range []string{"plan", "execute", "review", "judge", "intake", "garden"} {
		assert.NotPanics(t, func() { dispatchRole(drv, role, drivers.CallArgs{}) }, role)
	}
	assert.PanicsWithValue(t, "modules: unknown driver role wizard", func() {
		dispatchRole(drv, "wizard", drivers.CallArgs{})
	})
}

// ---- ListRepoSkills 边界 ----

func TestListRepoSkillsMissingDir(t *testing.T) {
	assert.Nil(t, ListRepoSkills(filepath.Join(t.TempDir(), "nope")))
}

// ---- skillFrontmatterOk 变体 ----

func TestSkillFrontmatterOkVariants(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, filepath.Join(dir, "SKILL.md"), "---\ndescription: no name\n---\n")
	assert.False(t, skillFrontmatterOk(dir))
	writeFileT(t, filepath.Join(dir, "SKILL.md"), "---\nname: x\ndescription: ok\n---\n")
	assert.True(t, skillFrontmatterOk(dir))
	writeFileT(t, filepath.Join(dir, "SKILL.md"), "broken\t yaml: [")
	assert.False(t, skillFrontmatterOk(dir))
}

func TestCopyDirMissingSource(t *testing.T) {
	err := copyDir(filepath.Join(t.TempDir(), "gone"), filepath.Join(t.TempDir(), "dst"))
	assert.Error(t, err)
}

func TestInstallSkillsDefaultHomeUsesUserHome(t *testing.T) {
	root := t.TempDir()
	writeFileT(t, filepath.Join(root, "skills", "solo", "SKILL.md"),
		"---\nname: solo\ndescription: d\n---\nbody")
	t.Setenv("HOME", t.TempDir()) // os.UserHomeDir 读 HOME（darwin/linux 一致）
	sum, err := InstallSkills(InstallSkillsOpts{Root: root})
	require.NoError(t, err)
	assert.Equal(t, "user", sum.Scope)
	require.NoError(t, err)
	_ = sum
}

// ---- CallAgent fallback 与重试耗尽 ----

func TestCallAgentFallbackChainThenSuccess(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	// 降级目标 opencode 必须声明该角色的模型映射（P1-12①：无映射则链终止）。
	ch := cfg.Channels["opencode"]
	ch.Models = map[string]string{"intake": "opencode/deepseek-v4-flash"}
	cfg.Channels["opencode"] = ch
	stub := newStub(nil)
	m := NewContext(CtxOpts{
		TaskDir: taskDir, Config: cfg,
		Drivers: &drivers.Drivers{Intake: &countingDriver{inner: stub, onCall: func(n int) drivers.Result {
			if n == 1 {
				return drivers.MockResult(func(x *drivers.Result) {
					code := 1
					x.ExitCode = &code
					msg := "403: quota exceeded"
					x.ErrorMessage = &msg // QUOTA/RATE_LIMIT：触发渠道降级链
					x.Settled = false
				})
			}
			return drivers.MockResult(nil)
		}}},
		Sleep: func(int64) {},
	})
	r := CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "p"})
	assert.True(t, r.OK, "降级到 opencode 后应成功")
	assert.Equal(t, "opencode", r.Channel, "SERVER 触发渠道降级链")
}

type countingDriver struct {
	inner  drivers.Driver
	onCall func(n int) drivers.Result
	n      int
}

func (d *countingDriver) Name() string { return "counting" }
func (d *countingDriver) next(a drivers.CallArgs) drivers.Result {
	d.n++
	return d.onCall(d.n)
}
func (d *countingDriver) Plan(a drivers.CallArgs) drivers.Result    { return d.next(a) }
func (d *countingDriver) Execute(a drivers.CallArgs) drivers.Result { return d.next(a) }
func (d *countingDriver) Review(a drivers.CallArgs) drivers.Result  { return d.next(a) }
func (d *countingDriver) Judge(a drivers.CallArgs) drivers.Result   { return d.next(a) }
func (d *countingDriver) Intake(a drivers.CallArgs) drivers.Result  { return d.next(a) }
func (d *countingDriver) Garden(a drivers.CallArgs) drivers.Result  { return d.next(a) }

func TestCallAgentRetriesExhaustedReturnsLastCode(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	cfg.Retry.MaxTransportRetries = 1
	m := NewContext(CtxOpts{
		TaskDir: taskDir, Config: cfg,
		Drivers: &drivers.Drivers{Intake: &countingDriver{onCall: func(int) drivers.Result {
			return drivers.MockResult(func(x *drivers.Result) {
				code := 1
				x.ExitCode = &code
				x.Settled = false
			})
		}}},
		Sleep: func(int64) {},
	})
	r := CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "p"})
	assert.False(t, r.OK)
	assert.NotEmpty(t, r.ErrorCode)
}

func TestCallAgentErrorMessageClassification(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	// errorMessage "401: ..." → AUTH（不可重试、不可降级时立即返回）。
	m := NewContext(CtxOpts{
		TaskDir: taskDir, Config: cfg,
		Drivers: &drivers.Drivers{Intake: &countingDriver{onCall: func(int) drivers.Result {
			return drivers.MockResult(func(x *drivers.Result) {
				code := 1
				x.ExitCode = &code
				msg := "401: invalid api key"
				x.ErrorMessage = &msg
			})
		}}},
		Sleep: func(int64) {},
	})
	r := CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "p"})
	assert.False(t, r.OK)
	assert.Equal(t, errs.AUTH, r.ErrorCode, "流内 401 必须分类为 AUTH")
}

// ---- RunIntake 补充分支 ----

func TestRunIntakeRealExecEchoSource(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	cfg.Intake.Sources = []config.Source{{Name: "echo-src", Type: "command", Cmd: `echo '{"title":"real exec candidate"}'`}}
	cfg.Loops["intake"] = config.LoopBinding{Harness: "pi", Model: "m/x", Channel: "ch", Thinking: "max"}

	sum, err := RunIntake(CtxOpts{
		TaskDir: taskDir, Config: cfg,
		Drivers: &drivers.Drivers{Intake: newStub(nil)},
	}, "echo-src", nil) // fn=nil → realExec 真执行
	require.NoError(t, err)
	assert.Equal(t, 1, sum.Scanned)

	// 指定源名过滤命中。
	sum2, err := RunIntake(CtxOpts{
		TaskDir: taskDir, Config: cfg,
		Drivers: &drivers.Drivers{Intake: newStub(nil)},
	}, "echo-src", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, sum2.Scanned)
}

func TestParseTriageVerdictReasonAbsentOrNonString(t *testing.T) {
	v, err := ParseTriageVerdict(`{"verdict":"human"}`)
	require.NoError(t, err)
	assert.Equal(t, "human", v.Verdict)
	assert.Empty(t, v.Reason, "缺 reason 键 → 归空（Node typeof 语义）")

	v, err = ParseTriageVerdict(`{"verdict":"skip","reason":["not","a","string"]}`)
	require.NoError(t, err)
	assert.Equal(t, "skip", v.Verdict)
	assert.Empty(t, v.Reason, "reason 为数组 → 归空而非解析失败")
}

// ---- recordModuleMetrics 缓存命中率阈值告警（§6-③ DC5）----

func TestRecordModuleMetricsCacheRateWarning(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	writeFileT(t, filepath.Join(taskDir, "state.json"), `{
		"schema-version": 1, "slug": "t", "revision": 1, "status": "running",
		"current-milestone": null, "plan-session": null,
		"milestones": {}, "packets": {},
		"budget": {"enforce": "soft", "daily-token-limit": null, "daily": {},
			"totals": {"input": 0, "output": 0, "reasoning": 0, "cache-read": 0, "cache-write": 0, "tokens": 0, "cost-usd": 0},
			"cache": {"hit-rate": 0, "by-provider": {}}, "warnings": []},
		"last-progress-at": null, "inbox": []
	}`)
	stub := newStub(nil)
	m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: &drivers.Drivers{Intake: stub}})

	// mock usage: input=120 cacheRead=40 → hit-rate 0.333 ≥ 0.3 默认阈值，
	// 但 provider 累计需 ≥1000 才评估——连打 20 次（input 累计 2400）。
	for range 20 {
		CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "p"})
	}
	st, err := os.ReadFile(filepath.Join(taskDir, "state.json"))
	require.NoError(t, err)
	var parsed struct {
		Budget struct {
			Cache struct {
				HitRate    float64 `json:"hit-rate"`
				ByProvider map[string]struct {
					Input   int64   `json:"input"`
					HitRate float64 `json:"hit-rate"`
				} `json:"by-provider"`
			} `json:"cache"`
			Warnings []string `json:"warnings"`
		} `json:"budget"`
	}
	require.NoError(t, json.Unmarshal(st, &parsed))
	bp := parsed.Budget.Cache.ByProvider["mock"]
	require.Greater(t, bp.Input, int64(1000), "provider 累计应达阈值评估线")
	assert.InDelta(t, 0.333, bp.HitRate, 0.001)
	assert.NotEqual(t, 0.333, 0)
	// hit-rate 0.333 高于默认阈值 0.3 → 无告警（正向场景锁定）。
	for _, w := range parsed.Budget.Warnings {
		assert.NotContains(t, w, "cache hit-rate", "高于阈值不得告警")
	}
}

var _ = strings.TrimSpace

// ---- 覆盖率收尾：intake github-issues / 失败 triage 标注 / 写入失败传播 ----

func readMetricsLinesT(t *testing.T, file string) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var e map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &e))
		out = append(out, e)
	}
	return out
}

func TestFetchSourceCandidatesGitHubIssues(t *testing.T) {
	var gotName string
	var gotArgs []string
	var gotCwd string
	src := config.Source{Name: "gh-src", Type: "github-issues", Repo: "org/repo",
		Labels: []string{"bug", "flywheel"}, Limit: 7}
	fn := ExecFunc(func(name string, args []string, cwd string) (string, error) {
		gotName, gotArgs, gotCwd = name, args, cwd
		return mustJSONT(t, []map[string]string{
			{"title": "Fix\t login  flow", "body": strings.Repeat("x", 350), "url": "https://git/1"},
			{"title": "Second issue", "body": "", "url": ""},
		}), nil
	})

	cs, err := FetchSourceCandidates(src, "/some/cwd", fn)
	require.NoError(t, err)
	require.Len(t, cs, 2)
	assert.Equal(t, "gh", gotName)
	assert.Equal(t, "/some/cwd", gotCwd)
	assert.Equal(t, []string{
		"issue", "list", "--repo", "org/repo", "--state", "open",
		"--json", "number,title,body,url,labels", "--limit", "7",
		"--label", "bug,flywheel",
	}, gotArgs, "gh 参数契约逐字锁定")
	assert.Equal(t, "Fix login flow", cs[0].Title, "空白折叠等价 Node .replace(/\\s+/g,' ').trim()")
	assert.Len(t, cs[0].Summary, 300, "body 截断到 300 字符")
	assert.Equal(t, "https://git/1", cs[0].URL)
	assert.Equal(t, "gh-src", cs[0].Source)
	assert.Equal(t, "github-issues", cs[0].SourceType)
	assert.Empty(t, cs[1].Summary)
	assert.Empty(t, cs[1].URL)

	// 无 labels → 无 --label 参数；limit 缺省走 orLimit → 50。
	_, err = FetchSourceCandidates(config.Source{Name: "b", Type: "github-issues", Repo: "o/r"}, "", fn)
	require.NoError(t, err)
	assert.NotContains(t, gotArgs, "--label")
	assert.Contains(t, gotArgs, "50")

	// gh 执行失败与坏 JSON 都原样上抛。
	_, err = FetchSourceCandidates(src, "", ExecFunc(func(string, []string, string) (string, error) {
		return "", fmt.Errorf("boom")
	}))
	assert.ErrorContains(t, err, "boom")
	_, err = FetchSourceCandidates(src, "", ExecFunc(func(string, []string, string) (string, error) {
		return "[not json", nil
	}))
	assert.Error(t, err)
}

func TestParseTriageVerdictSkipsBlankAndBrokenLines(t *testing.T) {
	// 尾部空行：自尾向前扫描必须 continue 跳过，不得误判 no verdict。
	v, err := ParseTriageVerdict("{\"verdict\":\"human\",\"reason\":\"r\"}\n\n")
	require.NoError(t, err)
	assert.Equal(t, "human", v.Verdict)

	// 以 { 开头但解析失败的行：跳过后继续向前找到合法行。
	v, err = ParseTriageVerdict("{\"verdict\":\"skip\",\"reason\":\"fine\"}\n{oops")
	require.NoError(t, err)
	assert.Equal(t, "skip", v.Verdict)
}

func TestListApprovedCandidatesMissingInbox(t *testing.T) {
	assert.Nil(t, ListApprovedCandidates(filepath.Join(t.TempDir(), "absent")))
}

func TestRunIntakeInboxAppendWriteFailure(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	cfg.Intake.Sources = []config.Source{{Name: "cmd", Type: "command", Cmd: "emit"}}
	// triage-inbox.md 是目录：追加写入必然失败并上抛。
	require.NoError(t, os.Mkdir(filepath.Join(taskDir, "triage-inbox.md"), 0o755))
	_, err := RunIntake(CtxOpts{TaskDir: taskDir, Cwd: ".", Config: cfg,
		Drivers: &drivers.Drivers{Intake: newStub(nil)}}, "",
		ExecFunc(func(string, []string, string) (string, error) { return "a candidate line", nil }))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "triage-inbox.md")
}

func TestRunIntakeVerdictWriteFailureMidLoop(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	cfg.Intake.Sources = []config.Source{{Name: "cmd", Type: "command", Cmd: "emit"}}
	inboxFile := filepath.Join(taskDir, "triage-inbox.md")
	drv := &drivers.Drivers{Intake: &countingDriver{onCall: func(int) drivers.Result {
		// 追加落盘后、裁决回写前把 inbox 变只读 → 循环内 WriteFile 失败。
		require.NoError(t, os.Chmod(inboxFile, 0o444))
		return drivers.Result{ExitCode: new(int), Settled: true,
			Text: `{"verdict":"recommend","reason":"r"}`}
	}}}
	t.Cleanup(func() { _ = os.Chmod(inboxFile, 0o644) })
	_, err := RunIntake(CtxOpts{TaskDir: taskDir, Cwd: ".", Config: cfg, Drivers: drv}, "",
		ExecFunc(func(string, []string, string) (string, error) { return "fresh candidate", nil }))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "triage-inbox.md")
}

func TestRunIntakeMissingBindingFailsAfterScan(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	cfg.Intake.Sources = []config.Source{{Name: "cmd", Type: "command", Cmd: "emit"}}
	delete(cfg.Loops, "intake")
	_, err := RunIntake(CtxOpts{TaskDir: taskDir, Cwd: ".", Config: cfg,
		Drivers: &drivers.Drivers{Intake: newStub(nil)}}, "",
		ExecFunc(func(string, []string, string) (string, error) { return "candidate one", nil }))
	require.ErrorContains(t, err, "no loop binding")
	assert.FileExists(t, filepath.Join(taskDir, "triage-inbox.md"),
		"候选先落盘，绑定解析在其后才失败")
}

func TestRunIntakeUnparseableAndFailedTriagesMarkHuman(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	cfg.Intake.Sources = []config.Source{{Name: "cmd", Type: "command", Cmd: "emit"}}
	one := 1
	unauthorized := "401: invalid api key"
	n := 0
	drv := &drivers.Drivers{Intake: &countingDriver{onCall: func(int) drivers.Result {
		n++
		if n == 1 {
			return drivers.Result{ExitCode: &one, ErrorMessage: &unauthorized, Settled: false} // 调用失败（AUTH 不可重试）
		}
		return drivers.Result{ExitCode: new(int), Settled: true, Text: "no verdict here"} // 成功但不可解析
	}}}
	sum, err := RunIntake(CtxOpts{TaskDir: taskDir, Cwd: ".", Config: cfg, Drivers: drv}, "",
		ExecFunc(func(string, []string, string) (string, error) { return "cand one\ncand two", nil }))
	require.NoError(t, err)
	assert.Equal(t, 2, sum.Triaged["failed"])
	assert.Equal(t, 2, sum.Triaged["human"], "失败候选一律归 human 待人处理")
	assert.Zero(t, sum.Triaged["recommend"])
	raw, rerr := os.ReadFile(sum.InboxFile)
	require.NoError(t, rerr)
	inbox := string(raw)
	assert.Contains(t, inbox, "triage failed (AUTH)", "调用失败 → 带错误码标注")
	assert.Contains(t, inbox, "unparseable output", "成功但无裁决 → unparseable 标注")
}

func TestRunGardenLogsInvalidProposals(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	var logs []string
	drv := &drivers.Drivers{Garden: &countingDriver{onCall: func(int) drivers.Result {
		return drivers.Result{ExitCode: new(int), Settled: true, Text: "no GP blocks in sight"}
	}}}
	sum, err := RunGarden(CtxOpts{TaskDir: taskDir, Cwd: taskDir, Config: cfg, Drivers: drv,
		Log: func(s string) { logs = append(logs, s) }})
	require.NoError(t, err)
	assert.False(t, sum.OK)
	assert.Contains(t, strings.Join(logs, "\n"),
		"garden proposals failed validation: no GP blocks found",
		"无效提案如实留痕，原始输出仍保留给人审")
	raw, rerr := os.ReadFile(*sum.File)
	require.NoError(t, rerr)
	assert.Contains(t, string(raw), "no GP blocks in sight")
}

// ---- 覆盖率收尾：recordModuleMetrics 估算回退 / CallAgent 分类与链终止 ----

func TestRecordModuleMetricsEstimatesCostAndSkipsNilUsage(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	binding := bindingFor(t, cfg, "intake")
	zero := 0
	n := 0
	drv := &drivers.Drivers{Intake: &countingDriver{onCall: func(int) drivers.Result {
		n++
		if n == 1 {
			return drivers.Result{ExitCode: &zero, Settled: true,
				Usage: &drivers.Usage{Input: 1000, Output: 100},
				Text:  `{"verdict":"recommend","reason":"r"}`}
		}
		return drivers.Result{ExitCode: &zero, Settled: true, Text: "plain ok"} // usage=nil → 提前返回
	}}}
	m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: drv})

	r := CallAgent(m, CallOptions{Role: "intake", Binding: binding, Prompt: "p1"})
	require.True(t, r.OK)
	lines := readMetricsLinesT(t, m.MetricsFile)
	require.Len(t, lines, 1)
	expCost := metrics.EstimateCostUSD(&metrics.Usage{Input: 1000, Output: 100},
		metrics.PriceForModel(cfg.Pricing, binding.Model))
	if expCost == nil {
		assert.Nil(t, lines[0]["cost-usd"])
	} else {
		assert.InDelta(t, *expCost, lines[0]["cost-usd"].(float64), 1e-9,
			"driver 未报成本 → 按价目表估算回退")
	}

	r = CallAgent(m, CallOptions{Role: "intake", Binding: binding, Prompt: "p2"})
	require.True(t, r.OK)
	lines = readMetricsLinesT(t, m.MetricsFile)
	require.Len(t, lines, 2)
	assert.Nil(t, lines[1]["usage"], "usage=nil 原样记录后提前返回")
	assert.Nil(t, lines[1]["cache-hit-rate"])
}

func TestCallAgentClassifiesBareAndSpawnErrorResults(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	cfg.Retry.MaxTransportRetries = 1 // 恰好两次尝试：一次裸 Result、一次 SpawnError
	spawnMsg := "spawn: fork failed"
	n := 0
	drv := &drivers.Drivers{Intake: &countingDriver{onCall: func(int) drivers.Result {
		n++
		if n%2 == 1 {
			// ExitCode 未报告：干净退出判定不成立，分类为空 → 兜底 TRANSPORT。
			return drivers.Result{}
		}
		return drivers.Result{SpawnError: spawnMsg} // spawnError → TRANSPORT
	}}}
	m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: drv, Sleep: func(int64) {}})
	r := CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "p"})
	assert.False(t, r.OK)
	assert.Equal(t, errs.TRANSPORT, r.ErrorCode)
	assert.Equal(t, 2, n)
}

func TestCallAgentDefaultsTransportRetriesWhenUnset(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	cfg.Retry.MaxTransportRetries = -1 // 直构 Config → 按 Node ?? 缺省 5
	one := 1
	n := 0
	drv := &drivers.Drivers{Intake: &countingDriver{onCall: func(int) drivers.Result {
		n++
		return drivers.Result{ExitCode: &one}
	}}}
	m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: drv, Sleep: func(int64) {}})
	r := CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "p"})
	assert.False(t, r.OK)
	assert.Equal(t, errs.TRANSPORT, r.ErrorCode)
	assert.Equal(t, 6, n, "maxTransport<=0 → 缺省 5（共 6 次尝试）")
}

func TestCallAgentFallbackChainEndsWithoutModelMapping(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	t.Setenv("OPENCODE_API_KEY", "sk-test")
	var logs []string
	quota := "403: quota exceeded"
	one := 1
	drv := &drivers.Drivers{Intake: &countingDriver{onCall: func(int) drivers.Result {
		return drivers.Result{ExitCode: &one, ErrorMessage: &quota, Settled: false}
	}}}
	m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: drv,
		Log: func(s string) { logs = append(logs, s) }})
	r := CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "p"})
	assert.False(t, r.OK)
	assert.Equal(t, errs.QUOTA, r.ErrorCode, "QUOTA 不可重试 → 立即返回")
	assert.Contains(t, strings.Join(logs, "\n"),
		"fallback chain ends: no model mapping for intake on opencode",
		"下一渠道未声明该角色映射 → 链终止（P1-12①）")
}

func TestRecordModuleMetricsLogsBudgetSkipOnCorruptState(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	writeFileT(t, filepath.Join(taskDir, "state.json"), "{corrupt")
	zero := 0
	drv := &drivers.Drivers{Intake: &countingDriver{onCall: func(int) drivers.Result {
		return drivers.Result{ExitCode: &zero, Settled: true,
			Usage: &drivers.Usage{Input: 10, Output: 5}, Text: "ok"}
	}}}
	var logs []string
	m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: drv,
		Log: func(s string) { logs = append(logs, s) }})
	CallAgent(m, CallOptions{Role: "intake", Binding: bindingFor(t, cfg, "intake"), Prompt: "p"})
	assert.Contains(t, strings.Join(logs, "\n"), "budget recording skipped:",
		"state.json 损坏 → 记账跳过并留痕，绝不 panic")
}

// ---- 覆盖率收尾：garden 缺省阈值 / prompt 边界 / RunGarden 失败传播 ----

func TestExtractFailurePatternsDefaultMinCountAndSort(t *testing.T) {
	retro := strings.Join([]string{
		"- packet M01-T01: INBOX — transport flake",
		"- packet M02-T02: INBOX — transport flake",
		"- packet M03-T03: INBOX — transport flake",
		"- packet M04-T04: escalated to human",
		"- packet M05-T05: escalated to human",
		"- packet M06-T06: INBOX — once only",
	}, "\n")
	pats := ExtractFailurePatterns(retro, -1) // minCount<=0 → 缺省 2
	require.Len(t, pats, 2, "缺省阈值下两组重复模式保留，单次出现被滤除")
	assert.Equal(t, 3, pats[0].Count, "按出现次数降序")
	assert.Equal(t, 2, pats[1].Count)
	assert.Contains(t, pats[0].Pattern, "transport flake")
	assert.NotContains(t, pats[0].Pattern, "M01", "packet id 归一化")
	assert.Empty(t, ExtractFailurePatterns("- INBOX once", 0))
}

func TestBuildGardenPromptCapsEvidenceAndEmptySections(t *testing.T) {
	patterns := []FailurePattern{{Pattern: "boom", Count: 9,
		Lines: []string{"l1", "l2", "l3", "l4", "l5"}}}
	prompt := buildGardenPrompt(patterns, "tail", "", nil)
	assert.Contains(t, prompt, `"l1"; "l2"; "l3"`)
	assert.NotContains(t, prompt, `"l4"`, "证据最多引用 3 行")
	assert.Contains(t, prompt, "(none in this repo)", "AGENTS.md 缺失占位")
	assert.Contains(t, prompt, "(none)", "skills 为空占位")

	empty := buildGardenPrompt(nil, "", "# AGENTS", []string{"s-a"})
	assert.Contains(t, empty, "(none repeated >= 2 times)")
	assert.Contains(t, empty, "- s-a")
}

func TestRunGardenRetroTailCappedAt40Lines(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	var b strings.Builder
	for i := range 45 {
		fmt.Fprintf(&b, "filler %d\n", i)
	}
	writeFileT(t, filepath.Join(taskDir, "retro.md"), b.String())
	sum, err := RunGarden(CtxOpts{TaskDir: taskDir, Cwd: taskDir, Config: cfg,
		Drivers: &drivers.Drivers{Garden: newStub(nil)}})
	require.NoError(t, err)
	assert.True(t, sum.OK, "errors: %v", sum.Errors)
}

func TestRunGardenBindingError(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	delete(cfg.Loops, "garden")
	_, err := RunGarden(CtxOpts{TaskDir: taskDir, Cwd: taskDir, Config: cfg})
	require.ErrorContains(t, err, "no loop binding")
}

func TestRunGardenWriteFileFailure(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	writeFileT(t, filepath.Join(taskDir, "retro.md"), "- nothing\n")
	require.NoError(t, os.Chmod(taskDir, 0o555)) // 任务目录只读 → 提案写入失败
	t.Cleanup(func() { _ = os.Chmod(taskDir, 0o755) })
	_, err := RunGarden(CtxOpts{TaskDir: taskDir, Cwd: taskDir, Config: cfg,
		Drivers: &drivers.Drivers{Garden: newStub(nil)}})
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(taskDir, "garden-proposals.md"))
}

// ---- 覆盖率收尾：installskills 分支矩阵 ----

func TestSkillFrontmatterOkRequiresSKILLFile(t *testing.T) {
	assert.False(t, skillFrontmatterOk(filepath.Join(t.TempDir(), "empty")))
}

func TestInstallSkillsHomeResolutionFailures(t *testing.T) {
	root := t.TempDir()
	writeFileT(t, filepath.Join(root, "skills", "alpha", "SKILL.md"),
		"---\nname: alpha\ndescription: d\n---\nbody")

	// HOME 空 → os.UserHomeDir 报错。
	t.Setenv("HOME", "")
	_, err := InstallSkills(InstallSkillsOpts{Root: root})
	require.ErrorContains(t, err, "$HOME")

	// HomeDir 指向普通文件 → MkdirAll 失败。
	homeFile := filepath.Join(t.TempDir(), "home-is-a-file")
	writeFileT(t, homeFile, "not a dir")
	_, err = InstallSkills(InstallSkillsOpts{Root: root, HomeDir: homeFile})
	require.Error(t, err)

	// 目标基目录只读且无锚文件 → 锚写入失败。
	home := t.TempDir()
	tb := filepath.Join(home, ".agents", "skills")
	require.NoError(t, os.MkdirAll(tb, 0o755))
	require.NoError(t, os.Chmod(tb, 0o555))
	t.Cleanup(func() { _ = os.Chmod(tb, 0o755) })
	_, err = InstallSkills(InstallSkillsOpts{Root: root, HomeDir: home})
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(tb, AnchorFile))
}

func TestInstallSkillsReportsBadFrontmatterAsError(t *testing.T) {
	root := t.TempDir()
	writeFileT(t, filepath.Join(root, "skills", "alpha", "SKILL.md"),
		"---\nname: alpha\ndescription: d\n---\nbody")
	writeFileT(t, filepath.Join(root, "skills", "badskill", "SKILL.md"),
		"---\ndescription: no name\n---\nbody")
	sum, err := InstallSkills(InstallSkillsOpts{Root: root, HomeDir: t.TempDir()})
	require.NoError(t, err)
	require.Len(t, sum.Results, 2)
	byName := map[string]SkillInstallResult{}
	for _, r := range sum.Results {
		byName[r.Name] = r
	}
	assert.Equal(t, "error", byName["badskill"].Action)
	assert.Contains(t, byName["badskill"].Reason, "frontmatter")
	assert.Equal(t, "linked", byName["alpha"].Action)
}

func TestInstallSkillsFileDestSkipAndForceReplace(t *testing.T) {
	root := t.TempDir()
	writeFileT(t, filepath.Join(root, "skills", "alpha", "SKILL.md"),
		"---\nname: alpha\ndescription: d\n---\nbody")
	home := t.TempDir()
	dest := filepath.Join(home, ".agents", "skills", "alpha")
	writeFileT(t, dest, "stale regular file")

	sum, err := InstallSkills(InstallSkillsOpts{Root: root, HomeDir: home})
	require.NoError(t, err)
	require.Len(t, sum.Results, 1)
	assert.Equal(t, "skipped", sum.Results[0].Action)
	assert.Contains(t, sum.Results[0].Reason, "already installed (file)")
	info, serr := os.Lstat(dest)
	require.NoError(t, serr)
	assert.True(t, info.Mode().IsRegular(), "无 force 不动普通文件")

	sum2, err := InstallSkills(InstallSkillsOpts{Root: root, HomeDir: home, Force: true})
	require.NoError(t, err)
	require.Len(t, sum2.Results, 1)
	assert.Equal(t, "linked", sum2.Results[0].Action, "force 把普通文件替换为符号链接")
	info, serr = os.Lstat(dest)
	require.NoError(t, serr)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
	target, rerr := os.Readlink(dest)
	require.NoError(t, rerr)
	assert.Equal(t, filepath.Join(root, "skills", "alpha"), target)
}

func TestInstallSkillsCopyErrorPropagates(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "beta")
	writeFileT(t, filepath.Join(skillDir, "SKILL.md"),
		"---\nname: beta\ndescription: d\n---\nbody")
	writeFileT(t, filepath.Join(skillDir, "locked.txt"), "secret")
	require.NoError(t, os.Chmod(filepath.Join(skillDir, "locked.txt"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(skillDir, "locked.txt"), 0o644) })
	sum, err := InstallSkills(InstallSkillsOpts{Root: root, HomeDir: t.TempDir(), Copy: true})
	require.NoError(t, err)
	require.Len(t, sum.Results, 1)
	assert.Equal(t, "error", sum.Results[0].Action)
	assert.Contains(t, sum.Results[0].Reason, "permission denied")
}

func TestInstallSkillsSymlinkFailureReported(t *testing.T) {
	root := t.TempDir()
	writeFileT(t, filepath.Join(root, "skills", "alpha", "SKILL.md"),
		"---\nname: alpha\ndescription: d\n---\nbody")
	home := t.TempDir()
	tb := filepath.Join(home, ".agents", "skills")
	require.NoError(t, os.MkdirAll(tb, 0o755))
	// 锚文件已存在可写（覆盖不需要目录写权），目录本身只读 → symlink 必败。
	writeFileT(t, filepath.Join(tb, AnchorFile), root+"\n")
	require.NoError(t, os.Chmod(tb, 0o555))
	t.Cleanup(func() { _ = os.Chmod(tb, 0o755) })
	sum, err := InstallSkills(InstallSkillsOpts{Root: root, HomeDir: home})
	require.NoError(t, err)
	require.Len(t, sum.Results, 1)
	assert.Equal(t, "error", sum.Results[0].Action)
	assert.Contains(t, sum.Results[0].Reason, "permission denied")
	assert.NoFileExists(t, filepath.Join(tb, "alpha"), "失败的条目不留半成品")
}

func TestCopyDirBranchMatrix(t *testing.T) {
	src := t.TempDir()
	writeFileT(t, filepath.Join(src, "top.txt"), "top")
	writeFileT(t, filepath.Join(src, "nested", "deep.txt"), "deep")

	// dest 父路径是普通文件 → MkdirAll 失败。
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeFileT(t, blocker, "file")
	err := copyDir(src, filepath.Join(blocker, "sub"))
	assert.ErrorContains(t, err, "not a directory")

	// 嵌套目录成功复制（递归 + continue）。
	dst := filepath.Join(t.TempDir(), "dst")
	require.NoError(t, copyDir(src, dst))
	data, rerr := os.ReadFile(filepath.Join(dst, "nested", "deep.txt"))
	require.NoError(t, rerr)
	assert.Equal(t, "deep", string(data))

	// 子目录不可读 → 递归错误上抛。
	src2 := t.TempDir()
	writeFileT(t, filepath.Join(src2, "nested", "inner.txt"), "x")
	require.NoError(t, os.Chmod(filepath.Join(src2, "nested"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(src2, "nested"), 0o755) })
	err = copyDir(src2, filepath.Join(t.TempDir(), "dst2"))
	assert.ErrorContains(t, err, "permission denied")

	// dest 已存在且只读 → 文件写入失败上抛（纯文件源，避免先撞递归 MkdirAll）。
	srcFlat := t.TempDir()
	writeFileT(t, filepath.Join(srcFlat, "only.txt"), "data")
	dstRO := filepath.Join(t.TempDir(), "dstro")
	require.NoError(t, os.Mkdir(dstRO, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dstRO, 0o755) })
	err = copyDir(srcFlat, dstRO)
	assert.ErrorContains(t, err, "permission denied")
}

// TestDirFS skillsembed 内嵌 FS 子目录访问（正向 + 错误）。
func TestDirFS(t *testing.T) {
	sub, err := skillsembed.DirFS("flywheel-run")
	require.NoError(t, err)
	data, rerr := fs.ReadFile(sub, "SKILL.md")
	require.NoError(t, rerr)
	assert.Contains(t, string(data), "flywheel-run")

	_, err = skillsembed.DirFS("no-such-skill")
	assert.Error(t, err)
}

func TestAheadCount(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
	} {
		out, oerr := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		require.NoError(t, oerr, "%s", out)
	}
	writeFileT(t, filepath.Join(repo, "init.txt"), "i")
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "init"}} {
		out, oerr := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		require.NoError(t, oerr, "%s", out)
	}
	baseOut := exec.Command("git", "-C", repo, "rev-parse", "HEAD")
	baseBytes, err := baseOut.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseBytes))

	writeFileT(t, filepath.Join(repo, "x.txt"), "x")
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "second"}} {
		out, oerr := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		require.NoError(t, oerr, "%s", out)
	}

	n, aerr := verify.AheadCount(repo, base, "HEAD")
	require.NoError(t, aerr)
	assert.Equal(t, 1, n)

	_, aerr = verify.AheadCount(filepath.Join(repo, "gone"), base, "HEAD")
	assert.Error(t, aerr)
}

// TestListApprovedCandidatesMultiBlock 回归：review #5 —— 旧正则用 lazy body
// 消费 "\n## " 分隔符，偶数序号的 approved 候选被静默丢块。header-index 切分
// 后所有块都必须被独立识别。
func TestListApprovedCandidatesMultiBlock(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("# Triage inbox (flywheel intake)\n\n")
	for i := 1; i <= 6; i++ {
		fp := fmt.Sprintf("%012x", i)
		fmt.Fprintf(&b, "## %s — candidate %d\n", fp, i)
		fmt.Fprintf(&b, "- source: s (command)\n- url: —\n- summary: —\n")
		if i%2 == 1 {
			fmt.Fprintf(&b, "- triage: recommend — ok\n- approval: approved\n")
		} else {
			fmt.Fprintf(&b, "- triage: skip\n- approval: pending\n")
		}
		b.WriteString("\n")
	}
	writeFileT(t, filepath.Join(dir, "triage-inbox.md"), b.String())

	got := ListApprovedCandidates(dir)
	require.Len(t, got, 3, "奇数候选（1/3/5）应为 approved，其余 pending")
	assert.Equal(t, "candidate 1", got[0].Title)
	assert.Equal(t, "candidate 3", got[1].Title)
	assert.Equal(t, "candidate 5", got[2].Title)
}
