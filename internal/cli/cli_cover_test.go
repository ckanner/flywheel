// internal/cli 覆盖率收尾测试：intake/garden 命令细节、run JSON 契约分支、
// status 渲染分支、stop pid 分支、logf/printJSON/Execute 进程入口。
// 所有编排器路径都跑在 newCLISandbox / 独立 tempdir 沙箱里，绝不触碰真实仓库。
package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/state"
)

// ---- Execute 进程入口（子进程自调用模式）----

// TestMain 支持子进程自调用：FLYWHEEL_CLI_EXEC_HELPER=1 时以真实 os.Args 与
// 标准流执行 Execute()，退出码即返回值——父进程据此断言进程入口契约。
func TestMain(m *testing.M) {
	if mode := os.Getenv("FLYWHEEL_CLI_EXEC_HELPER"); mode != "" {
		os.Exit(executeHelperMain(mode))
	}
	os.Exit(m.Run())
}

func executeHelperMain(mode string) int {
	switch mode {
	case "version":
		os.Args = []string{"flywheel", "version"}
	case "error":
		os.Args = []string{"flywheel", "status", filepath.Join(os.TempDir(), "definitely-missing-task")}
	default:
		panic("unknown helper mode: " + mode)
	}
	return Execute()
}

func runExecuteHelper(t *testing.T, mode string) (int, string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run", "^TestMain$")
	cmd.Env = append(os.Environ(), "FLYWHEEL_CLI_EXEC_HELPER="+mode)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		require.True(t, ok, "helper crashed: %v — %s", err, out)
		code = exitErr.ExitCode()
	}
	return code, string(out)
}

func TestExecuteRealStreamsVersion(t *testing.T) {
	code, out := runExecuteHelper(t, "version")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "flywheel ")
}

func TestExecuteRealStreamsErrorToStderr(t *testing.T) {
	code, out := runExecuteHelper(t, "error")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "[flywheel] 错误")
}

// ---- exitError.Error / logf / printJSON 直接单测 ----

func TestExitErrorMessage(t *testing.T) {
	cause := &exitError{code: 3, cause: assert.AnError}
	assert.Equal(t, assert.AnError.Error(), cause.Error())
	bare := &exitError{code: 3}
	assert.Equal(t, "exit 3", bare.Error())
}

func TestLogfPrefixesFlywheelOnCmdStream(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	logf(cmd)("hello step")
	assert.Equal(t, "[flywheel] hello step\n", buf.String())
}

func TestPrintJSONTwoSpaceIndent(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	printJSON(cmd, map[string]any{"ok": true})
	assert.Equal(t, "{\n  \"ok\": true\n}\n", buf.String())
}

// ---- repoRoot 兜底爬升分支 ----
//
// tempdir 之上一无 go.mod 祖先（darwin /var/folders/...）：第一候选（cwd）
// 整段爬升触底 break，第二候选（exe 目录）同样触底，最终退回工作目录。
func TestRepoRootFallsBackToWdWhenNoGoModAncestor(t *testing.T) {
	chdirT(t, t.TempDir())
	wd := mustCwd()
	assert.Equal(t, wd, repoRoot())
}

func writePidFile(t *testing.T, taskDir string, payload string) {
	t.Helper()
	writeFileCli(t, filepath.Join(taskDir, "orchestrator.pid"), payload)
}

// deadReapedPid 返回一个确定已死且被收割的 pid（true 瞬退 + Wait 收割，
// kill(pid,0) 必然 ESRCH）。
func deadReapedPid(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	_ = cmd.Wait()
	return pid
}

func TestStatusAliveOrchestratorSuffix(t *testing.T) {
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
	writePidFile(t, taskDir, mustJSONT(t, map[string]int{"pid": os.Getpid()}))

	code, out := runCLI(t, "status", taskDir)
	require.Equal(t, 0, code)
	assert.Contains(t, out, "编排器运行中, pid "+strconv.Itoa(os.Getpid()))
}

func TestStatusPidFileVariantsRenderWithoutSuffix(t *testing.T) {
	cases := map[string]string{
		"garbage json": "not-json-at-all",
		"nonpositive":  `{"pid": 0}`,
		"dead pid":     mustJSONT(t, map[string]int{"pid": deadReapedPid(t)}),
		"no pid file":  "",
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			taskDir := t.TempDir()
			writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
			if payload != "" {
				writePidFile(t, taskDir, payload)
			}
			code, out := runCLI(t, "status", taskDir)
			require.Equal(t, 0, code)
			assert.NotContains(t, out, "编排器运行中")
			assert.Contains(t, out, "状态: running")
		})
	}
}

// statusBVariant 在基础 fixture 上翻转出第二组渲染分支：空里程碑指针、
// 已关闭里程碑、interrupted 包、无预算上限、无告警、无 inbox。
func statusBVariant(t *testing.T) string {
	t.Helper()
	raw := cliStateFixture
	repl := [][2]string{
		{`"current-milestone": "M-01"`, `"current-milestone": null`},
		{`"last-progress-at": "2026-08-22T00:00:00.000Z"`, `"last-progress-at": null`},
		{`"status": "in-progress", "gate": null, "closed-at": null`, `"status": "closed", "gate": null, "closed-at": "2026-08-20T00:00:00.000Z"`},
		{`"interrupted": false`, `"interrupted": true`},
		{`"daily-token-limit": 100000`, `"daily-token-limit": null`},
		{`"warnings": ["budget soft warning: daily token budget exceeded"]`, `"warnings": []`},
		{`"inbox": [{"at": "2026-08-22T00:00:00.000Z", "packet-id": "M01-T01", "reason": "needs human"}]`, `"inbox": []`},
		// 异里程碑包：status 渲染按里程碑分组时被 continue 跳过。
		{`"M01-T01": {`, `"ZZ-T01": { "milestone": "M-99", "status": "pending", "route": "default", "attempts": 0, "failed-attempts": 0, "plan-revisions": 0, "interrupted": false, "needs-live-credentials": false, "artifacts": [], "exit": null, "base-ref": null, "commit": null, "worktree": null, "evidence": {}, "last-failure-signature": null, "retry-history": [], "updated-at": null }, "M01-T01": {`},
	}
	for _, r := range repl {
		raw = strings.Replace(raw, r[0], r[1], 1)
	}
	return raw
}

func TestStatusRendersNullPointersClosedInterruptedNoBudget(t *testing.T) {
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), statusBVariant(t))

	code, out := runCLI(t, "status", taskDir)
	require.Equal(t, 0, code)
	for _, want := range []string{
		"任务: clitask  状态: running\n",
		"当前里程碑: —  上次推进: —",
		"里程碑 M-01: closed（关闭于 2026-08-20T00:00:00.000Z）",
		"M01-T01: l1-passed [interrupted]",
		"日上限 未设）",
	} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "预算告警")
	assert.NotContains(t, out, "inbox（需要人处理）")
}

func TestStatusInboxLastEntryNotAMap(t *testing.T) {
	taskDir := t.TempDir()
	raw := strings.Replace(cliStateFixture,
		`{"at": "2026-08-22T00:00:00.000Z", "packet-id": "M01-T01", "reason": "needs human"}`,
		`"just a plain string"`, 1)
	writeFileCli(t, filepath.Join(taskDir, "state.json"), raw)

	code, out := runCLI(t, "status", taskDir)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, "inbox（需要人处理）: 1 条，最近: \n")
}

func TestStatusBrokenStateReturnsError(t *testing.T) {
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), "{broken")

	code, out := runCLI(t, "status", taskDir)
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "[flywheel] 错误")
}

func TestEvidenceHelpersDirect(t *testing.T) {
	assert.Equal(t, "✓", evidencePass(map[string]any{"pass": true}, "pass"))
	assert.Equal(t, "✗", evidencePass(map[string]any{"pass": false}, "pass"))
	assert.Equal(t, "—", evidencePass("not a map", "pass"))
	assert.Equal(t, "pass", evidenceVerdict(map[string]any{"verdict": "pass"}))
	assert.Equal(t, "—", evidenceVerdict(nil))
}

// ---- stop pid 分支 ----

// stopPollFast 把确认轮询缩小到毫秒级（生产行为 50×100ms 由包变量注入）。
func stopPollFast(t *testing.T) {
	t.Helper()
	oldAttempts, oldInterval := stopPollAttempts, stopPollInterval
	stopPollAttempts, stopPollInterval = 50, 10*time.Millisecond
	t.Cleanup(func() { stopPollAttempts, stopPollInterval = oldAttempts, oldInterval })
}

// orphanedSleep 启动一个脱离测试进程的 sleep（sh 退出后被 launchd 收养，
// 绝不变成本测试的僵尸——僵尸会让 kill(pid,0) 恒成功）。返回其 pid。
func orphanedSleep(t *testing.T, seconds string) int {
	t.Helper()
	out, err := exec.Command("/bin/sh", "-c",
		"sleep "+seconds+" </dev/null >/dev/null 2>&1 & echo $!").Output()
	require.NoError(t, err, string(out))
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	require.NoError(t, err)
	return pid
}

func killForSure(t *testing.T, pid int) {
	t.Helper()
	_ = syscall.Kill(pid, syscall.SIGKILL)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if signalZero(pid) != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Logf("pid %d 未在 2s 内消失（收养方延迟），继续", pid)
}

func TestStopKillsLiveOrchestrator(t *testing.T) {
	stopPollFast(t)
	pid := orphanedSleep(t, "30")
	t.Cleanup(func() { killForSure(t, pid) })

	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
	writePidFile(t, taskDir, mustJSONT(t, map[string]int{"pid": pid}))

	code, out := runCLI(t, "stop", taskDir)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, "已停止编排器进程 (pid "+strconv.Itoa(pid)+")，状态定格。")
	assert.NoFileExists(t, filepath.Join(taskDir, "orchestrator.pid"))

	st, err := state.Load(taskDir)
	require.NoError(t, err)
	assert.Equal(t, "stopped", st.Status)
}

func TestStopReportsUnconfirmedKill(t *testing.T) {
	stopPollFast(t)
	// trap "" TERM：忽略 SIGTERM 的假编排器；它是本测试的直接子进程且始终
	// 存活（轮询期间不会变僵尸），结束后由 Cleanup 强杀并收割。
	cmd := exec.Command("/bin/sh", "-c", `trap "" TERM; sleep 30`)
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
	writePidFile(t, taskDir, mustJSONT(t, map[string]int{"pid": pid}))

	code, out := runCLI(t, "stop", taskDir)
	require.Equal(t, 1, code, out)
	assert.Contains(t, out, "SIGTERM 已发送给 pid "+strconv.Itoa(pid)+"，但 5s 内未确认退出")
	assert.Contains(t, out, "state.json 保持原样")
	assert.FileExists(t, filepath.Join(taskDir, "orchestrator.pid"), "未确认停止绝不删 pid 文件")

	st, err := state.Load(taskDir)
	require.NoError(t, err)
	assert.Equal(t, "running", st.Status, "未确认停止绝不改写 state.json")
}

func TestStopAlreadyDeadPidStillReportsStoppedWithPid(t *testing.T) {
	stopPollFast(t)
	pid := deadReapedPid(t)

	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
	writePidFile(t, taskDir, mustJSONT(t, map[string]int{"pid": pid}))

	code, out := runCLI(t, "stop", taskDir)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, "已停止编排器进程 (pid "+strconv.Itoa(pid)+")")
	assert.NoFileExists(t, filepath.Join(taskDir, "orchestrator.pid"))
}

func TestStopBrokenStateSurfacesError(t *testing.T) {
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), "{broken")

	code, out := runCLI(t, "stop", taskDir)
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "[flywheel] 错误")
}

// ---- intake / garden 命令（模块沙箱：只需 config，无需 git/spec）----

const missingKeyEnv = "FLYWHEEL_CLI_TEST_MISSING_KEY" // 测试专用、保证未设

// newModuleSandbox 最小任务沙箱：仅 flywheel.config.json（intake/garden 不读
// spec/packets）。mutate 在默认 mock 全绿配置上追加/覆盖键。
func newModuleSandbox(t *testing.T, mutate func(cfg map[string]any)) string {
	t.Helper()
	taskDir := t.TempDir()
	e2eConfigT(t, taskDir)
	path := filepath.Join(taskDir, "flywheel.config.json")
	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(path)), &cfg))
	mutate(cfg)
	writeFileCli(t, path, mustJSONT(t, cfg))
	return taskDir
}

func mustReadFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func intakeSourceCmd() string {
	// command 源：吐一条候选（mock intake 对它给 recommend）。
	return `printf '%s\n' '{"title":"Add retry budget","summary":"transport failures repeat","url":"https://issues/1"}'`
}

func TestIntakeHappyPathViaCLI(t *testing.T) {
	require.NoError(t, os.Unsetenv(missingKeyEnv))
	taskDir := newModuleSandbox(t, func(cfg map[string]any) {
		cfg["intake"] = map[string]any{
			"sources": []any{map[string]any{"name": "cmdsrc", "type": "command", "cmd": intakeSourceCmd()}},
		}
	})

	code, out := runCLI(t, "intake", taskDir)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"ok": true`)
	assert.Contains(t, out, `"scanned": 1`)
	assert.Contains(t, out, `"recommend": 1`)
	assert.Contains(t, out, `[flywheel] source cmdsrc: 1 candidates`) // logf 前缀走 cmd 流
	assert.FileExists(t, filepath.Join(taskDir, "triage-inbox.md"))
}

func TestIntakeFailedTriageExitsThree(t *testing.T) {
	require.NoError(t, os.Unsetenv(missingKeyEnv))
	taskDir := newModuleSandbox(t, func(cfg map[string]any) {
		cfg["intake"] = map[string]any{
			"sources": []any{map[string]any{"name": "cmdsrc", "type": "command", "cmd": intakeSourceCmd()}},
		}
		drv := cfg["drivers"].(map[string]any)
		drv["intake"] = "pi" // 真实驱动 + 渠道 env-key 未设 → fail-loud AUTH，绝不 spawn
		cfg["channels"] = map[string]any{
			"opencode-go": map[string]any{"env-key": missingKeyEnv}, // 无 fallback：链即刻终止
		}
	})

	code, out := runCLI(t, "intake", taskDir)
	require.Equal(t, 3, code, out)
	assert.Contains(t, out, "fail-loud — channel opencode-go requires env "+missingKeyEnv)
	assert.Contains(t, out, `"failed": 1`)
	assert.Contains(t, out, `"human": 1`)
}
func TestIntakeSourceFilterAndUnknownSource(t *testing.T) {
	taskDir := newModuleSandbox(t, func(cfg map[string]any) {
		cfg["intake"] = map[string]any{
			"sources": []any{
				map[string]any{"name": "cmdsrc", "type": "command", "cmd": intakeSourceCmd()},
				map[string]any{"name": "other", "type": "command", "cmd": "true"},
			},
		}
	})

	// --source 只扫指定源：other 无输出 → scanned 0，仍 ok。
	code, out := runCLI(t, "intake", taskDir, "--source", "other")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"scanned": 0`)
	assert.Contains(t, out, `"ok": true`)

	code, out = runCLI(t, "intake", taskDir, "--source", "bogus")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "unknown intake source: bogus")
}

func TestIntakeMissingTaskDirExitsOne(t *testing.T) {
	code, out := runCLI(t, "intake", filepath.Join(t.TempDir(), "nope"))
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "任务目录不存在")
}

func TestGardenHappyPathViaCLI(t *testing.T) {
	require.NoError(t, os.Unsetenv(missingKeyEnv))
	taskDir := newModuleSandbox(t, func(cfg map[string]any) {})
	_ = taskDir
	taskDir2 := newModuleSandbox(t, func(cfg map[string]any) {})

	code, out := runCLI(t, "garden", taskDir2)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"ok": true`)
	assert.Contains(t, out, `"proposals": 1`)
	assert.NotContains(t, out, `"file": null`)
	assert.FileExists(t, filepath.Join(taskDir2, "garden-proposals.md"))
}

func TestGardenAgentFailureExitsThree(t *testing.T) {
	require.NoError(t, os.Unsetenv(missingKeyEnv))
	taskDir := newModuleSandbox(t, func(cfg map[string]any) {
		drv := cfg["drivers"].(map[string]any)
		drv["garden"] = "pi" // fail-loud AUTH：渠道 env-key 未设，绝不 spawn
		loops := cfg["loops"].(map[string]any)
		loops["garden"] = map[string]string{
			"harness": "pi", "model": "opencode-go/deepseek-v4-flash",
			"channel": "gated-channel", "thinking": "max",
		}
		cfg["channels"] = map[string]any{
			"gated-channel": map[string]any{"env-key": missingKeyEnv},
		}
	})

	code, out := runCLI(t, "garden", taskDir)
	require.Equal(t, 3, code, out)
	assert.Contains(t, out, `"ok": false`)
	assert.Contains(t, out, `"file": null`)
	assert.Contains(t, out, "garden agent call failed: AUTH")
}

// ---- run JSON 契约分支：budget-stopped / escalated / 失败步骤 → exit 3 ----

func TestRunBudgetHardStopExitsThree(t *testing.T) {
	taskDir := newCLISandbox(t)
	chdirT(t, taskDir)
	// hard + 限额 1 token：plan 的 mock 用量(180)落账后，execute 前闸门熔断。
	path := filepath.Join(taskDir, "flywheel.config.json")
	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(path)), &cfg))
	cfg["budget"] = map[string]any{"enforce": "hard", "daily-token-limit": 1}
	writeFileCli(t, path, mustJSONT(t, cfg))

	code, out := runCLI(t, "run", taskDir)
	require.Equal(t, 3, code, out)
	assert.Contains(t, out, `"status": "budget-stopped"`)
}

func TestRunPlanFailLoudEscalatesExitsThree(t *testing.T) {
	require.NoError(t, os.Unsetenv(missingKeyEnv))
	taskDir := newCLISandbox(t)
	chdirT(t, taskDir)
	// plan 走真实驱动 + 渠道 env-key 未设 → fail-loud AUTH → 升级进 inbox。
	path := filepath.Join(taskDir, "flywheel.config.json")
	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(path)), &cfg))
	drv := cfg["drivers"].(map[string]any)
	drv["plan"] = "pi"
	cfg["channels"] = map[string]any{
		"kimi-subscription": map[string]any{"env-key": missingKeyEnv}, // 去掉 fallback
	}
	writeFileCli(t, path, mustJSONT(t, cfg))

	code, out := runCLI(t, "run", taskDir)
	require.Equal(t, 3, code, out)
	assert.Contains(t, out, `"status": "escalated"`)
	assert.Contains(t, out, "plan: fail-loud — channel kimi-subscription requires env "+missingKeyEnv)
	st, err := state.Load(taskDir)
	require.NoError(t, err)
	last := st.Inbox[len(st.Inbox)-1].(map[string]any)
	assert.Contains(t, last["reason"], "plan step failed on every channel")
}

func TestRunOnlyVerifyFailingGateExitsThree(t *testing.T) {
	taskDir := newCLISandbox(t)
	chdirT(t, taskDir)
	// L1 门禁命令恒败：--only verify 的步骤结果 ok=false → CLI 以 3 退出（#9）。
	failing := strings.Replace(cliE2EPacket(), `{cmd: "true", expect: "exit 0"}`, `{cmd: "false", expect: "exit 0"}`, 1)
	require.NotEqual(t, cliE2EPacket(), failing)
	writeFileCli(t, filepath.Join(taskDir, "task-packet-M01-T01.md"), failing)
	gitCliCommitAll(t, filepath.Dir(filepath.Dir(taskDir)), "chore: failing gate")

	code, out := runCLI(t, "run", taskDir, "--only", "verify", "--packet", "M01-T01")
	require.Equal(t, 3, code, out)
	assert.Contains(t, out, `"name": "verify"`)
	assert.Contains(t, out, `"ok": false`)
}

func TestRunPauseBeforeRejectsNonReview(t *testing.T) {
	code, out := runCLI(t, "run", t.TempDir(), "--pause-before", "bogus")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "--pause-before 只支持: review")
}

// ---- install-skills 分支：project/--copy/stale/skipped/error ----

func TestInstallSkillsProjectCopyScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := t.TempDir()

	code, out := runCLI(t, "install-skills", "--project", project, "--copy")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, "安装范围: project ("+project+")")
	assert.Contains(t, out, "方式: 复制（快照）")
	assert.FileExists(t, filepath.Join(project, ".agents", "skills", "flywheel-run", "SKILL.md"))
	assert.NoFileExists(t, filepath.Join(home, ".agents", "skills", "flywheel-run"), "项目级安装不动用户级")
}

func TestInstallSkillsSkippedSecondRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	code, _ := runCLI(t, "install-skills")
	require.Equal(t, 0, code)
	code, out := runCLI(t, "install-skills")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, "flywheel-run: 已安装，跳过（already installed） -> ")
}

func TestInstallSkillsStaleSymlinkReported(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillsDir := filepath.Join(home, ".agents", "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	require.NoError(t, os.Symlink(filepath.Join(home, "gone-target"),
		filepath.Join(skillsDir, "flywheel-run")))

	code, out := runCLI(t, "install-skills")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, "链接已失效（重跑加 --force 替换）")
	assert.Contains(t, out, "re-run with --force")
}

func TestInstallSkillsRefusesRealDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillsDir := filepath.Join(home, ".agents", "skills")
	require.NoError(t, os.MkdirAll(filepath.Join(skillsDir, "flywheel-run"), 0o755))

	code, out := runCLI(t, "install-skills")
	require.Equal(t, 1, code, out)
	assert.Contains(t, out, "错误（a real directory already exists here")
}

func TestGardenMissingTaskDirAndBrokenConfig(t *testing.T) {
	code, out := runCLI(t, "garden", filepath.Join(t.TempDir(), "nope"))
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "任务目录不存在")

	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "flywheel.config.json"), "{broken")
	code, out = runCLI(t, "garden", taskDir)
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "[flywheel] 错误")
}

func TestRunBrokenConfigSurfacesExitOne(t *testing.T) {
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "flywheel.config.json"), "{broken")
	code, out := runCLI(t, "run", taskDir)
	assert.Equal(t, 1, code, out)
	assert.Contains(t, out, "invalid JSON")
}

func TestStopMissingTaskDirExitsOne(t *testing.T) {
	code, out := runCLI(t, "stop", filepath.Join(t.TempDir(), "nope"))
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "任务目录不存在")
}

func TestStopStateSaveFailureSurfacesError(t *testing.T) {
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
	// 目录转只读：state.Save 落盘失败 → 错误原样上抛 exit 1。
	require.NoError(t, os.Chmod(taskDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(taskDir, 0o700) })

	code, out := runCLI(t, "stop", taskDir)
	assert.Equal(t, 1, code, out)
	assert.Contains(t, out, "[flywheel] 错误")
}

func TestInstallSkillsUnwritableTargetSurfacesError(t *testing.T) {
	// HOME 指向普通文件：MkdirAll(<file>/.agents/skills) 必败，错误上抛。
	home := filepath.Join(t.TempDir(), "home-is-a-file")
	writeFileCli(t, home, "not a directory")
	t.Setenv("HOME", home)

	code, out := runCLI(t, "install-skills")
	assert.Equal(t, 1, code, out)
	assert.Contains(t, out, "[flywheel] 错误")
}
