package machine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- 纯函数 ----

func TestMathRound(t *testing.T) {
	assert.Equal(t, 2.0, mathRound(2.4))
	assert.Equal(t, 3.0, mathRound(2.5))
	assert.Equal(t, 1.0, mathRound(0.99))
	res := drivers.MockResult(nil) // ReasoningChars 已设
	assert.Equal(t, *res.ReasoningChars, reasoningCharsOf(&res))
	noRC := res
	noRC.ReasoningChars = nil
	assert.Nil(t, reasoningCharsOf(&noRC))
}

func TestContainsString(t *testing.T) {
	assert.True(t, containsString([]string{"a", "b"}, "b"))
	assert.False(t, containsString([]string{"a"}, "b"))
	assert.False(t, containsString(nil, ""))
}

func TestShortSHAOthers(t *testing.T) {
	assert.Equal(t, "12345678", shortSHA("123456789abcdef"))
	assert.Equal(t, "abc", shortSHA("abc"))
}

func TestFirstNonEmpty(t *testing.T) {
	assert.Equal(t, "", firstNonEmpty("", ""))
	assert.Equal(t, "x", firstNonEmpty("", "x", "y"))
}

func TestJSONNilable(t *testing.T) {
	assert.Nil(t, jsonNilable(""))
	assert.Equal(t, "v", jsonNilable("v"))
}

func TestCommitOutcomeMapArms(t *testing.T) {
	full := commitOutcomeMap(verify.CommitOutcome{
		SHA: "sha1", Skipped: "nothing", Error: "boom", Message: "msg", At: "at",
	})
	assert.Equal(t, "sha1", full["sha"])
	assert.Equal(t, "nothing", full["skipped"])
	assert.Equal(t, "boom", full["error"])
	assert.Equal(t, "msg", full["message"])
	minimal := commitOutcomeMap(verify.CommitOutcome{At: "at"})
	assert.Equal(t, map[string]any{"at": "at"}, minimal)
}

func TestDerefHelpers(t *testing.T) {
	assert.Equal(t, "", derefString(nil))
	v := "x"
	assert.Equal(t, "x", derefString(&v))
	assert.Equal(t, "", derefStringPtr(nil))
	assert.Equal(t, "x", derefStringPtr(&v))
}

func TestOrTransport(t *testing.T) {
	assert.Equal(t, errs.TRANSPORT, orTransport(""))
	assert.Equal(t, errs.AUTH, orTransport(errs.AUTH))
}

func TestL1PassTrueArms(t *testing.T) {
	ps := new(state.PacketState)
	assert.False(t, l1PassTrue(ps)) // L1 不是 map
	ps.Evidence.L1 = map[string]any{"pass": true}
	assert.True(t, l1PassTrue(ps))
	ps.Evidence.L1 = map[string]any{"pass": false}
	assert.False(t, l1PassTrue(ps))
}

func TestOptStrAndAlways(t *testing.T) {
	assert.Nil(t, optStr(""))
	require.NotNil(t, optStr("v"))
	assert.Equal(t, "v", *optStr("v"))
	p := strPtrAlways("")
	require.NotNil(t, p)
	assert.Empty(t, *p)
}

func TestStrPtrArms(t *testing.T) {
	assert.Nil(t, strPtr(""))
	require.NotNil(t, strPtr("v"))
	assert.Equal(t, "v", *strPtr("v"))
}

func TestToSpawnResultArms(t *testing.T) {
	r := drivers.MockResult(func(res *drivers.Result) {
		code := 7
		res.ExitCode = &code
		res.SpawnError = "spawn failed"
		res.Signal = "SIGKILL"
	})
	sr := toSpawnResult(r)
	assert.Equal(t, 7, sr.ExitCode)
	assert.Equal(t, "SIGKILL", sr.Signal)
	require.Error(t, sr.SpawnError)
	noExit := drivers.MockResult(func(res *drivers.Result) { res.ExitCode = nil })
	assert.Equal(t, 0, toSpawnResult(noExit).ExitCode)
}

func TestAppendFileArms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.md")
	appendFile(path, "one")
	appendFile(path, "two")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "onetwo", string(raw))
	appendFile(filepath.Join(dir, "missing-dir", "x.md"), "nope") // 打不开 → 静默
}

func TestBaseRefOfAndOrHEAD(t *testing.T) {
	ctx := unitTestContext(t)
	ps := new(state.PacketState)
	assert.Empty(t, ctx.baseRefOf(ps))
	ref := "deadbeef"
	ps.BaseRef = &ref
	assert.Equal(t, "deadbeef", ctx.baseRefOf(ps))
	assert.Equal(t, "HEAD", orHEAD(""))
	assert.Equal(t, "abc", orHEAD("abc"))
}

func TestJsonToMapArms(t *testing.T) {
	m, err := jsonToMap(map[string]any{"a": 1.0})
	require.NoError(t, err)
	assert.Equal(t, 1.0, m["a"])
	_, err = jsonToMap(make(chan int)) // marshal 失败
	require.Error(t, err)
	_, err = jsonToMap("scalar") // unmarshal 进 map 失败
	require.Error(t, err)
}

func TestMilestoneByIDArms(t *testing.T) {
	ctx := unitTestContext(t)
	m, ok := ctx.milestoneByID("M-01")
	require.True(t, ok)
	assert.Equal(t, "M-01", m.ID)
	_, ok = ctx.milestoneByID("M-99")
	assert.False(t, ok)
}

func TestNextActionablePacketSkipsUnparsable(t *testing.T) {
	ctx := unitTestContext(t)
	ctx.PacketList = append([]*contract.Packet{{PacketID: "WEIRD"}}, ctx.PacketList...) // MilestoneOfPacket 解析失败 → skip（置于队首才会被遍历到）
	p := ctx.nextActionablePacket("M-01")
	require.NotNil(t, p)
	assert.Equal(t, "M01-T01", p.PacketID)
}

func TestCurrentMilestoneArms(t *testing.T) {
	ctx := unitTestContext(t)
	m := ctx.currentMilestone()
	require.NotNil(t, m)
	assert.Equal(t, "M-01", m.ID)
	ms := ctx.State.Milestones["M-01"]
	ms.Status = "closed"
	ctx.State.Milestones["M-01"] = ms
	assert.Nil(t, ctx.currentMilestone())
}

// taskDirPrefix 的防御分支：Cwd 与 TaskDir 无法取相对路径时退化为绝对前缀。
func TestTaskDirPrefixDegenerate(t *testing.T) {
	c := &Context{Cwd: "relative/cwd", TaskDir: "/abs/task"}
	assert.Equal(t, "/abs/task/", c.taskDirPrefix())
}

// ---- partitionChangedFiles ----

func TestPartitionChangedFilesArms(t *testing.T) {
	ctx := unitTestContext(t) // Cwd=root, TaskDir=root/docs/minitask
	executor, violations := ctx.partitionChangedFiles(nil)
	assert.Nil(t, executor)
	assert.Nil(t, violations)

	executor, violations = ctx.partitionChangedFiles([]string{
		"src/outside.go",             // 任务目录外 → executor
		"docs/minitask/state.json",   // 账本文件 → 忽略
		"docs/minitask/reports/x.js", // 账本目录 → 忽略
		"docs/minitask/spec.md",      // 契约文件 → 违规
		"docs/minitask/notes/a.md",   // 任务目录内非契约 → executor
	})
	assert.ElementsMatch(t, []string{"src/outside.go", "docs/minitask/notes/a.md"}, executor)
	require.Len(t, violations, 1)
	assert.Equal(t, "contract-file", violations[0].Type)
	assert.Equal(t, "docs/minitask/spec.md", violations[0].Path)
}

// ---- l1BoundaryChangedFiles / executorSelfChangedFiles ----

func TestL1BoundaryChangedFilesArms(t *testing.T) {
	ps := new(state.PacketState)
	assert.Nil(t, l1BoundaryChangedFiles(ps)) // L1 缺失

	ps.Evidence.L1 = map[string]any{
		"boundary": verify.BoundaryReport{Skipped: true},
	}
	assert.Nil(t, l1BoundaryChangedFiles(ps)) // struct + skipped

	ps.Evidence.L1 = map[string]any{
		"boundary": verify.BoundaryReport{ChangedFiles: []string{"a.go"}},
	}
	assert.Equal(t, []string{"a.go"}, l1BoundaryChangedFiles(ps)) // struct 权威

	ps.Evidence.L1 = map[string]any{
		"boundary": map[string]any{"skipped": true},
	}
	assert.Nil(t, l1BoundaryChangedFiles(ps)) // JSON 往返形状 + skipped

	ps.Evidence.L1 = map[string]any{
		"boundary": map[string]any{"changedFiles": []any{"a.go", 3, "b.go"}},
	}
	assert.Equal(t, []string{"a.go", "b.go"}, l1BoundaryChangedFiles(ps)) // 非 string 元素剔除

	ps.Evidence.L1 = map[string]any{"boundary": 42}
	assert.Nil(t, l1BoundaryChangedFiles(ps)) // 未知形状
}

func TestExecutorSelfChangedFilesArms(t *testing.T) {
	ps := new(state.PacketState)
	assert.Nil(t, executorSelfChangedFiles(ps)) // 无自报证据
	ps.Evidence.Executor = map[string]any{"changed-files": []any{"x.go", 1}}
	assert.Equal(t, []string{"x.go"}, executorSelfChangedFiles(ps))
}

// ---- save / fatal ----

func TestSaveFailurePoisonsFatalThenShortCircuits(t *testing.T) {
	fx := newFixture(t)
	ctx := unitTestContext(t, fx)
	writeFile(t, filepath.Join(fx.taskDir, "state.json"), "garbage") // CAS 读盘解析失败

	ctx.save()
	require.Error(t, ctx.err())
	assert.Contains(t, ctx.err().Error(), "unreadable during CAS check")

	ctx.save() // fatal 已置位 → 直接短路
	assert.Contains(t, ctx.err().Error(), "unreadable during CAS check")
}

func TestRecordRetryVariadicArms(t *testing.T) {
	ctx := unitTestContext(t)
	ps := ctx.State.Packets["M01-T01"]
	ctx.recordRetry(ps, "attempt", "AUTH", "requeue")
	require.Len(t, ps.RetryHistory, 1)
	entry, _ := ps.RetryHistory[0].(map[string]any)
	assert.Equal(t, 0, entry["planned-delay-ms"])

	ctx.recordRetry(ps, "transport", "SERVER", "backoff", int64(250))
	entry, _ = ps.RetryHistory[1].(map[string]any)
	assert.Equal(t, int64(250), entry["planned-delay-ms"])
}

// ---- recordSpawnMetrics / warnIfCacheCold ----

func spawnMetricsCtx(t *testing.T) (*Context, string) {
	fx := newFixture(t)
	ctx := unitTestContext(t, fx)
	return ctx, fx.taskDir
}

func lastMetricsLine(t *testing.T, taskDir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(taskDir, "metrics.jsonl"))
	require.NoError(t, err, "metrics.jsonl 应存在")
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	m := map[string]any{}
	require.NoError(t, jsonUnmarshal([]byte(lines[len(lines)-1]), &m))
	return m
}

func TestRecordSpawnMetricsNoUsage(t *testing.T) {
	ctx, dir := spawnMetricsCtx(t)
	res := drivers.MockResult(func(r *drivers.Result) { r.Usage = nil })
	attempt := 2
	ctx.recordSpawnMetrics(spawnMetricsArgs{
		role: "execute", packetID: "M01-T01", prompt: "p",
		result: &res, errorCode: errs.EMPTY_RESPONSE, attempt: &attempt,
	})
	m := lastMetricsLine(t, dir)
	assert.Equal(t, "execute", m["role"])
	assert.Equal(t, "M01-T01", m["packet-id"])
	assert.Equal(t, "EMPTY_RESPONSE", m["error-code"])
	assert.Equal(t, 2.0, m["attempt"])
	assert.Nil(t, m["usage"]) // usage nil → 早退前的字段全是 nil
}

// TestRecordSpawnMetricsWiresTotalTokens driver 上报的 totalTokens 必须
// 记账（预算口径用 provider 总数，缺省 fallback i+o 会低估含 reasoning 的
// 上报）。
func TestRecordSpawnMetricsWiresTotalTokens(t *testing.T) {
	ctx, _ := spawnMetricsCtx(t)
	res := drivers.MockResult(func(r *drivers.Result) {
		r.Usage = &drivers.Usage{Input: 100, Output: 10, TotalTokens: 500}
	})
	ctx.recordSpawnMetrics(spawnMetricsArgs{role: "execute", result: &res})
	assert.Equal(t, int64(500), ctx.State.Budget.Totals.Tokens)
}

func TestRecordSpawnMetricsProviderFallbackArms(t *testing.T) {
	ctx, dir := spawnMetricsCtx(t)

	// Provider 空、channel 空 → unknown。
	res := drivers.MockResult(func(r *drivers.Result) { r.Provider = "" })
	ctx.recordSpawnMetrics(spawnMetricsArgs{role: "review", result: &res})
	m := lastMetricsLine(t, dir)
	assert.Equal(t, "review", m["role"])
	bp := ctx.State.Budget.Cache.ByProvider["unknown"]
	assert.Equal(t, int64(120), bp.Input)

	// Provider 空、channel 有值 → provider=channel。
	ctx2, _ := spawnMetricsCtx(t)
	ctx2.recordSpawnMetrics(spawnMetricsArgs{role: "judge", channel: "opencode-go", result: &res})
	bp = ctx2.State.Budget.Cache.ByProvider["opencode-go"]
	assert.Equal(t, int64(120), bp.Input)
}

func TestRecordSpawnMetricsCostEstimateArm(t *testing.T) {
	ctx, dir := spawnMetricsCtx(t)
	res := drivers.MockResult(func(r *drivers.Result) { r.CostUSD = nil }) // usage 在，CostUSD 缺 → 估价
	ctx.recordSpawnMetrics(spawnMetricsArgs{role: "plan", model: "no-such-model", result: &res})
	m := lastMetricsLine(t, dir)
	assert.Contains(t, m, "cost-usd")
	assert.NotNil(t, m["cache-hit-rate"]) // Input>0 → 命中率入账
}

func TestWarnIfCacheColdArms(t *testing.T) {
	fx := newFixture(t)
	var logs []string
	ctx := unitTestContext(t, fx)
	ctx.Log = func(msg string) { logs = append(logs, msg) }

	ctx.warnIfCacheCold("ghost", "") // ByProvider 无记录 → 静默
	assert.Empty(t, logs)

	// 未达阈值不告警。
	ctx.State.Budget.Cache.ByProvider["prov"] = state.ProviderCache{Input: 500, HitRate: 0}
	ctx.warnIfCacheCold("prov", "")
	assert.Empty(t, logs)

	// 达量且低于默认阈值 → 告警一次；重复告警被 containsString 抑制。
	ctx.State.Budget.Cache.ByProvider["prov"] = state.ProviderCache{Input: 2000, HitRate: 0.1}
	ctx.warnIfCacheCold("prov", "")
	require.Len(t, logs, 1)
	assert.Contains(t, logs[0], "below threshold 30%")
	ctx.warnIfCacheCold("prov", "")
	assert.Len(t, logs, 1)

	// 渠道级 cache-hit-rate-min 覆盖阈值。
	min := 0.9
	ch := ctx.Config.Channels["opencode-go"]
	ch.CacheHitRateMin = &min
	ctx.Config.Channels["opencode-go"] = ch
	ctx.State.Budget.Cache.ByProvider["opencode-go"] = state.ProviderCache{Input: 2000, HitRate: 0.5}
	ctx.warnIfCacheCold("opencode-go", "opencode-go")
	require.Len(t, logs, 2)
	assert.Contains(t, logs[1], "below threshold 90%")

	// 命中率达标 → 不告警。
	ctx.State.Budget.Cache.ByProvider["good"] = state.ProviderCache{Input: 2000, HitRate: 0.95}
	ctx.warnIfCacheCold("good", "")
	assert.Len(t, logs, 2)
}

// ---- apiKeysFor / channelApiKey / missingKeyForRealDriver ----

func TestAPIKeyLookupsWithInjectedEnv(t *testing.T) {
	fx := newFixture(t)
	ctx := unitTestContext(t, fx)
	ctx.EnvLookup = func(key string) (string, bool) {
		if key == "OPENCODE_API_KEY" {
			return "sk-test", true
		}
		return "", false
	}

	keys := ctx.apiKeysFor("opencode-go")
	assert.Equal(t, map[string]string{"OPENCODE_API_KEY": "sk-test"}, keys)
	assert.Empty(t, ctx.apiKeysFor("")) // 无 env-key 渠道

	assert.Equal(t, "sk-test", ctx.channelApiKey("opencode-go"))
	ctx.EnvLookup = func(string) (string, bool) { return "", false }
	assert.Empty(t, ctx.apiKeysFor("opencode-go"))
	assert.Empty(t, ctx.channelApiKey("opencode-go")) // 键未设 → ""
	assert.Empty(t, ctx.channelApiKey("kimi-subscription"))

	// mock 驱动不做 key 守卫；真实驱动缺 key 返回键名。
	assert.Empty(t, ctx.missingKeyForRealDriver("execute", "opencode-go"))
	assert.Equal(t, "OPENCODE_API_KEY", ctx.missingKeyForRealDriver("pi-role", "opencode-go"))
}

// ---- budgetGate ----

func TestBudgetGateStopWarnOk(t *testing.T) {
	fx := newFixture(t)
	var logs []string
	ctx := unitTestContext(t, fx)
	ctx.Log = func(msg string) { logs = append(logs, msg) }
	limit := int64(100)

	// ok：当日用量未达限。
	assert.Equal(t, "ok", ctx.budgetGate())

	// soft + 达限 → warn（去重后只追加一条 warning，但每次都 log）。
	ctx.Config.Budget.Enforce = "soft"
	ctx.Config.Budget.DailyTokenLimit = &limit
	day := time.Now().UTC().Format("2006-01-02")
	ctx.State.Budget.Daily[day] = state.DailyBucket{Tokens: 150}
	assert.Equal(t, "ok", ctx.budgetGate())
	require.Len(t, ctx.State.Budget.Warnings, 1)
	first := ctx.budgetGate()
	assert.Equal(t, "ok", first)
	assert.Len(t, ctx.State.Budget.Warnings, 1, "同日重复告警必须去重")
	assert.Len(t, logs, 2)

	// hard + 达限 → stop + budget-stopped + save。
	ctx.Config.Budget.Enforce = "hard"
	assert.Equal(t, "stop", ctx.budgetGate())
	assert.Equal(t, "budget-stopped", ctx.State.Status)
	st := loadFixtureState(t, fx)
	assert.Equal(t, "budget-stopped", st.Status)
}

// ---- 单实例守卫 / pid ----

func TestAssertNoRunningOrchestratorNonPositivePid(t *testing.T) {
	dir := t.TempDir()
	pidFile := PidFile(dir)
	require.NoError(t, os.WriteFile(pidFile, []byte(`{"pid":-5}`), 0o644))
	assert.NoError(t, AssertNoRunningOrchestrator(dir))
	require.NoError(t, os.WriteFile(pidFile, []byte(`{"pid":0}`), 0o644))
	assert.NoError(t, AssertNoRunningOrchestrator(dir))
}

// ---- WarnIfForeignDirty 截断采样 ----

func TestWarnIfForeignDirtyTruncatesSample(t *testing.T) {
	fx := newGitFixture(t)
	for i := range 5 {
		writeFile(t, filepath.Join(fx.root, fmt.Sprintf("stray%d.txt", i)), "wip")
	}
	var logs []string
	ctx := unitTestContext(t, fx)
	ctx.Log = func(msg string) { logs = append(logs, msg) }
	ctx.WarnIfForeignDirty()
	require.Len(t, logs, 1)
	assert.Contains(t, logs[0], "5 dirty file(s)")
	assert.Contains(t, logs[0], "stray2.txt")
	assert.NotContains(t, logs[0], "stray4.txt") // 只采前三
}

// ---- unitCwd ----

func TestUnitCwdArms(t *testing.T) {
	fx := newGitFixture(t)
	ctx := unitTestContext(t, fx)
	packet := ctx.Packets["M01-T01"]

	assert.Equal(t, ctx.Cwd, ctx.unitCwd(packet)) // 无 worktree 记录

	wtPath := filepath.Join(fx.root, ".flywheel-worktrees", "wt")
	require.NoError(t, os.MkdirAll(wtPath, 0o755))
	ps := ctx.State.Packets["M01-T01"]
	ps.Worktree = &state.WorktreeInfo{Mode: "worktree", Path: wtPath, Branch: "b"}
	assert.Equal(t, wtPath, ctx.unitCwd(packet))

	ps.Worktree.Path = filepath.Join(fx.root, "gone") // 陈旧引用 → 主 checkout
	assert.Equal(t, ctx.Cwd, ctx.unitCwd(packet))
	ps.Worktree.Mode = "in-place"
	assert.Equal(t, ctx.Cwd, ctx.unitCwd(packet))
}

// ---- CreateContext ----

func TestCreateContextDefaultsAndFreshInit(t *testing.T) {
	fx := newFixture(t)
	require.NoError(t, os.Remove(filepath.Join(fx.taskDir, "state.json")))

	ctx, err := CreateContext(Opts{TaskDir: fx.taskDir, Config: configFor(fx)})
	require.NoError(t, err)
	assert.True(t, ctx.Fresh, "无 state.json → Init 新建")
	require.NotNil(t, ctx.Drivers)
	require.NotNil(t, ctx.Sleep)
	require.NotNil(t, ctx.Now)
	require.NotNil(t, ctx.EnvLookup)
	require.NotNil(t, ctx.Log)
	assert.DirExists(t, fx.taskDir)
	assert.True(t, ctx.State.Packets["M01-T01"] != nil, "Init 已建 packet 状态")
	assert.Equal(t, "pending", ctx.State.Status)
	assert.NoFileExists(t, filepath.Join(fx.taskDir, "state.json"), "CreateContext 不落盘，首次 save 才写")
}

func TestCreateContextErrorPaths(t *testing.T) {
	fx := newFixture(t)

	// spec.md 缺失。
	broken := fixture{root: t.TempDir(), taskDir: filepath.Join(fx.root, "docs", "nospec"), slug: "nospec"}
	require.NoError(t, os.MkdirAll(broken.taskDir, 0o755))
	_, err := CreateContext(Opts{TaskDir: broken.taskDir, Config: configFor(fx)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.md not found")

	// 无 packet 文件。
	nopkt := fixture{root: t.TempDir(), taskDir: filepath.Join(fx.root, "docs", "nopkt"), slug: "nopkt"}
	writeFile(t, filepath.Join(nopkt.taskDir, "spec.md"), fixtureSpec)
	writeFile(t, filepath.Join(nopkt.taskDir, "flywheel.config.json"), fixtureConfigJSON)
	_, err = CreateContext(Opts{TaskDir: nopkt.taskDir, Config: configFor(fx)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no task-packet")

	// state.json 损坏。
	corrupt := newFixture(t)
	writeFile(t, filepath.Join(corrupt.taskDir, "state.json"), "{oops")
	_, err = CreateContext(Opts{TaskDir: corrupt.taskDir, Config: configFor(corrupt)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

// ---- 小工具收尾 ----

func TestNowISOUsesInjectedClock(t *testing.T) {
	ctx := unitTestContext(t)
	base := time.Date(2026, 8, 23, 10, 11, 12, 123_000_000, time.FixedZone("X", 8*3600))
	ctx.Now = func() time.Time { return base }
	assert.Equal(t, "2026-08-23T02:11:12.123Z", ctx.nowISO()) // UTC 化
}

func TestTouchProgress(t *testing.T) {
	ctx := unitTestContext(t)
	assert.Nil(t, ctx.State.LastProgressAt)
	ctx.touchProgress()
	require.NotNil(t, ctx.State.LastProgressAt)
	assert.NotEmpty(t, *ctx.State.LastProgressAt)
}

// unitTestContext 构造可直接操纵的 Context（mock 驱动 + noLog/noSleep）。
func unitTestContext(t *testing.T, fx ...fixture) *Context {
	t.Helper()
	var f fixture
	if len(fx) > 0 {
		f = fx[0]
	} else {
		f = newFixture(t)
	}
	cfg := configFor(f)
	drvs := happyDrivers().set()
	ctx, err := CreateContext(Opts{
		TaskDir: f.taskDir, Cwd: f.root, Config: cfg, Drivers: drvs,
		Log: func(string) {}, Sleep: func(time.Duration) {},
	})
	require.NoError(t, err)
	return ctx
}
