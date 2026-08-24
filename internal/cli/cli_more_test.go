package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/modules"
	"github.com/ckanner/flywheel/internal/skillsembed"
	"github.com/ckanner/flywheel/internal/state"
)

func writeFileCli(t *testing.T, path, content string) {
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

// runCLI 注入参数与缓冲流执行命令树，返回退出码与合并输出。
func runCLI(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runRoot(args, &stdout, &stderr)
	return code, stdout.String() + stderr.String()
}

// e2eConfigT mock 全绿 config：intake/garden 也走 mock，无真实凭证依赖。
// Default() 无 channels/loops（任务级配置）——必须自带。
func e2eConfigT(t *testing.T, taskDir string) {
	t.Helper()
	cfg := map[string]any{
		"version": 1,
		"drivers": map[string]string{
			"plan": "mock", "execute": "mock", "review": "mock",
			"judge": "mock", "intake": "mock", "garden": "mock",
		},
		"loops": map[string]any{
			"plan":   map[string]string{"harness": "kimi", "model": "kimi-code/k3", "channel": "kimi-subscription", "thinking": "max"},
			"review": map[string]string{"harness": "pi", "model": "opencode-go/deepseek-v4-pro", "channel": "opencode-go", "thinking": "max"},
			"judge":  map[string]string{"harness": "pi", "model": "kimi-coding/k3", "channel": "kimi-coding", "thinking": "max"},
			"intake": map[string]string{"harness": "pi", "model": "opencode-go/deepseek-v4-flash", "channel": "opencode-go", "thinking": "max"},
			"garden": map[string]string{"harness": "kimi", "model": "kimi-code/k3", "channel": "kimi-subscription", "thinking": "max"},
		},
		"routes": map[string]any{
			"default": map[string]string{"harness": "pi", "model": "opencode-go/deepseek-v4-flash", "channel": "opencode-go", "thinking": "max"},
		},
		"channels": map[string]any{
			"kimi-subscription": map[string]any{"env-key": nil, "fallback": "kimi-coding"},
		},
		"budget":   map[string]any{"enforce": "soft"},
		"watchdog": map[string]any{"idle-timeout-ms": 5000, "wall-timeout-ms": 30000, "kill-grace-ms": 500},
	}
	writeFileCli(t, filepath.Join(taskDir, "flywheel.config.json"), mustJSONT(t, cfg))
}

func gitCli(t *testing.T, root string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

// gitCliInitSandbox init + 身份配置；契约写盘后调 gitCliCommitAll 固定 base-ref。
func gitCliInitSandbox(t *testing.T, root string) {
	t.Helper()
	gitCli(t, root, "init", "-q")
	gitCli(t, root, "config", "user.email", "test@example.com")
	gitCli(t, root, "config", "user.name", "test")
}

func gitCliCommitAll(t *testing.T, root, msg string) {
	t.Helper()
	gitCli(t, root, "add", "-A")
	gitCli(t, root, "commit", "-q", "-m", msg)
}

func chdirT(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

const cliE2ESpec = `---
spec-version: 1
title: CLI e2e
goal: Prove the run command JSON contract.
milestones:
  - id: M-01
    title: Only milestone
requirements:
  - id: R-01
    title: Works
    milestone: M-01
---
`

func cliE2EPacket() string {
	return `---
packet-id: M01-T01
contract-version: 1
goal: Produce the artifact.
requirements: [R-01]
route: default
needs-live-credentials: false
context-files: []
cache-scope: []
boundaries:
  max-files: 5
acceptance:
  L1:
    - {cmd: "true", expect: "exit 0"}
review-rubric: pass line.
escalation: {max-retries: 2, on-fail: return-to-planner}
done-report-format:
  requirements-covered: []
  changed-files: []
  commands-run: []
  failures: []
---

Work.
`
}

// newCLISandbox 隔离沙箱：<root>/docs/clitask 内含完整契约，git 已提交
// （base-ref=HEAD）。返回 taskDir。调用方随后 chdirT(taskDir) 再跑命令——
// 编排器的 worktree/base-ref/提交全部落在沙箱仓库内，绝不触碰真实仓库。
func newCLISandbox(t *testing.T) string {
	root := t.TempDir()
	taskDir := filepath.Join(root, "docs", "clitask")
	e2eConfigT(t, taskDir)
	writeFileCli(t, filepath.Join(taskDir, "spec.md"), cliE2ESpec)
	writeFileCli(t, filepath.Join(taskDir, "task-packet-M01-T01.md"), cliE2EPacket())
	gitCliInitSandbox(t, root)
	gitCliCommitAll(t, root, "chore: initial contracts")
	return taskDir
}

func TestRunEndToEndMockJSONContract(t *testing.T) {
	taskDir := newCLISandbox(t)
	chdirT(t, taskDir)

	code, out := runCLI(t, "run", taskDir)
	if code != 0 {
		st, _ := os.ReadFile(filepath.Join(taskDir, "state.json"))
		t.Fatalf("run failed (code %d):\n%s\n--- state ---\n%s", code, out, st)
	}
	assert.Contains(t, out, `"slug": "clitask"`)
	assert.Contains(t, out, `"status": "completed"`)

	// --only execute 单步 + --packet。
	code, out = runCLI(t, "run", taskDir, "--only", "execute", "--packet", "M01-T01")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"step"`)

	// --unit 别名 + pause-before review。
	code, _ = runCLI(t, "run", taskDir, "--pause-before", "review")
	assert.Equal(t, 0, code)
}

func TestRunPacketAliasOrEmptyStr(t *testing.T) {
	taskDir := newCLISandbox(t)
	chdirT(t, taskDir)

	// --unit 别名路径覆盖 orEmptyStr 的 b 分支。
	code, out := runCLI(t, "run", taskDir, "--only", "verify", "--unit", "M01-T01")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"name": "verify"`)
}

const cliStateFixture = `{
  "schema-version": 1,
  "slug": "clitask",
  "revision": 3,
  "status": "running",
  "current-milestone": "M-01",
  "plan-session": null,
  "milestones": { "M-01": { "status": "in-progress", "gate": null, "closed-at": null } },
  "packets": {
    "M01-T01": {
      "milestone": "M-01", "status": "l1-passed", "route": "default",
      "attempts": 2, "failed-attempts": 1, "plan-revisions": 0,
      "interrupted": false, "needs-live-credentials": false,
      "artifacts": [], "exit": null, "base-ref": null, "commit": null, "worktree": null,
      "evidence": {"executor": {"done-report": "r"}, "l1": {"pass": true}, "review": {"verdict": "pass"}},
      "last-failure-signature": null, "retry-history": [], "updated-at": null
    }
  },
  "budget": {
    "enforce": "soft", "daily-token-limit": 100000,
    "daily": {},
    "totals": {"input": 10, "output": 20, "reasoning": 5, "cache-read": 8, "cache-write": 2, "tokens": 30, "cost-usd": 0.05},
    "cache": {"hit-rate": 0.8, "by-provider": {}},
    "warnings": ["budget soft warning: daily token budget exceeded"]
  },
  "last-progress-at": "2026-08-22T00:00:00.000Z",
  "inbox": [{"at": "2026-08-22T00:00:00.000Z", "packet-id": "M01-T01", "reason": "needs human"}]
}`

func TestVersionOutput(t *testing.T) {
	code, out := runCLI(t, "version")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "flywheel ")
}

func TestUnknownCommandExitsOne(t *testing.T) {
	code, out := runCLI(t, "definitely-not-a-command")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "unknown command") // cobra 原生错误文本
}

func TestRunMissingTaskDir(t *testing.T) {
	code, out := runCLI(t, "run", filepath.Join(t.TempDir(), "nope"))
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "任务目录不存在")
}

func TestRunInvalidOnlyStep(t *testing.T) {
	code, out := runCLI(t, "run", t.TempDir(), "--only", "bogus")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "--only 只支持")
}

func TestStatusRendersHumanSummary(t *testing.T) {
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)

	code, out := runCLI(t, "status", taskDir)
	require.Equal(t, 0, code)
	for _, want := range []string{
		"任务: clitask  状态: running",
		"当前里程碑: M-01",
		"revision: 3",
		"里程碑 M-01: in-progress",
		"M01-T01: l1-passed  尝试 2（失败 1）",
		"L1=✓ review=pass",
		"token 累计: 30（in 10 / out 20 / reasoning 5）  成本: $0.05",
		"缓存命中率: 80.0%  预算档: soft",
		"预算告警:",
		"inbox（需要人处理）: 1 条，最近: needs human",
	} {
		assert.Contains(t, out, want)
	}
}

func TestStatusWithoutStateFails(t *testing.T) {
	code, out := runCLI(t, "status", t.TempDir())
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "state.json 不存在")
}

func TestStatusEvidenceVariants(t *testing.T) {
	taskDir := t.TempDir()
	raw := strings.Replace(cliStateFixture,
		`"l1": {"pass": true}, "review": {"verdict": "pass"}`,
		`"l1": {"pass": false}, "review": null`, 1)
	raw = strings.Replace(raw, `"executor": {"done-report": "r"}, `, "", 1)
	writeFileCli(t, filepath.Join(taskDir, "state.json"), raw)
	_, out := runCLI(t, "status", taskDir)
	assert.Contains(t, out, "executor=— L1=✗ review=—")
}

func TestStopFreezesStateWithoutPidFile(t *testing.T) {
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)

	code, out := runCLI(t, "stop", taskDir)
	require.Equal(t, 0, code)
	assert.Contains(t, out, "编排器未在运行，状态已定格为 stopped")

	st, err := state.Load(taskDir)
	require.NoError(t, err)
	assert.Equal(t, "stopped", st.Status)
}

func TestStopKeepsCompletedStateUntouched(t *testing.T) {
	taskDir := t.TempDir()
	raw := strings.Replace(cliStateFixture, `"status": "running"`, `"status": "completed"`, 1)
	writeFileCli(t, filepath.Join(taskDir, "state.json"), raw)

	code, _ := runCLI(t, "stop", taskDir)
	require.Equal(t, 0, code)
	st, err := state.Load(taskDir)
	require.NoError(t, err)
	assert.Equal(t, "completed", st.Status, "已完成任务不被 stop 改写")
}

func TestStopToleratesGarbagePidFile(t *testing.T) {
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
	writeFileCli(t, filepath.Join(taskDir, "orchestrator.pid"), "garbage")

	code, out := runCLI(t, "stop", taskDir)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, "状态已定格")
	// 坏 pid 文件按 Node rmSync(force) 语义删除。
	assert.NoFileExists(t, filepath.Join(taskDir, "orchestrator.pid"))
	st, err := state.Load(taskDir)
	require.NoError(t, err)
	assert.Equal(t, "stopped", st.Status)
}

func TestSkillsListAndRead(t *testing.T) {
	code, out := runCLI(t, "skills", "list")
	require.Equal(t, 0, code)
	for _, name := range []string{"flywheel-run", "flywheel-intake", "flywheel-garden"} {
		assert.Contains(t, out, name)
	}

	code, out = runCLI(t, "skills", "read", "flywheel-run")
	require.Equal(t, 0, code)
	assert.True(t, strings.HasPrefix(out, "---\n"))

	code, out = runCLI(t, "skills", "read", "../evil")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "invalid skill name")

	code, _ = runCLI(t, "skills", "read", "missing-skill")
	assert.Equal(t, 1, code)
}

func TestInstallSkillsSmokeViaRepoRootDetection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	code, out := runCLI(t, "install-skills")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, "安装范围: user")
	assert.FileExists(t, filepath.Join(home, ".agents", "skills", modules.AnchorFile))
	linkPath := filepath.Join(home, ".agents", "skills", "flywheel-run")
	info, err := os.Lstat(linkPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "默认软链模式")
}

// TestInstallEmbeddedSnapshotWhenNoSourceTree review #6 回归：发布二进制
// （无源码树）运行 install-skills 时，从内嵌快照物化副本——绝不静默装零个。
func TestInstallEmbeddedSnapshotWhenNoSourceTree(t *testing.T) {
	// chdir 到无 go.mod 祖先的 tempdir → repoRoot ok=false → 内嵌快照臂。
	chdirT(t, t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	code, out := runCLI(t, "install-skills")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, "内嵌快照")
	// 全部 10 个内置 skill 均被物化为真实目录（含 SKILL.md）。
	for _, s := range skillsembed.List() {
		assert.FileExists(t, filepath.Join(home, ".agents", "skills", s.Name, "SKILL.md"), s.Name)
	}
	assert.FileExists(t, filepath.Join(home, ".agents", "skills", modules.AnchorFile))
}

// TestInstallEmbeddedSnapshotProjectScope 内嵌物化的 project 作用域分支。
func TestInstallEmbeddedSnapshotProjectScope(t *testing.T) {
	project := t.TempDir()
	chdirT(t, t.TempDir()) // 无源码树 → 走内嵌快照臂

	code, out := runCLI(t, "install-skills", "--project", project)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, "project (")
	assert.Contains(t, out, "内嵌快照")
	assert.FileExists(t, filepath.Join(project, ".agents", "skills", "flywheel-run", "SKILL.md"))
}

// TestInstallSkillsHomeErrorOutsideRepo：HOME="" 且在源码树外 →
// repoRoot ok=false + UserHomeDir 失败 → return serr（覆盖 CLI 层错误传播）。
func TestInstallSkillsHomeErrorOutsideRepo(t *testing.T) {
	chdirT(t, t.TempDir())
	t.Setenv("HOME", "")
	code, out := runCLI(t, "install-skills")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "home directory")
}

func TestMustCwdNonEmpty(t *testing.T) {
	assert.NotEmpty(t, mustCwd())
}

func TestConfigLoadErrorSurfacesExitOne(t *testing.T) {
	taskDir := t.TempDir()
	writeFileCli(t, filepath.Join(taskDir, "flywheel.config.json"), "{broken")
	code, out := runCLI(t, "intake", taskDir)
	assert.Equal(t, 1, code, out)
}

// TestStatusStallHint（A5①）：上次推进超过 wall-clock 阈值 → 停滞提示；
// 新鲜时间戳 → 不提示。
func TestStatusStallHint(t *testing.T) {
	staleKey := `"last-progress-at": "2026-08-22T00:00:00.000Z"`

	taskDir := t.TempDir()
	stale := strings.Replace(cliStateFixture, staleKey,
		`"last-progress-at": "2020-01-01T00:00:00.000Z"`, 1)
	require.NotEqual(t, cliStateFixture, stale, "fixture 必须含 last-progress-at 键")
	writeFileCli(t, filepath.Join(taskDir, "state.json"), stale)
	_, out := runCLI(t, "status", taskDir)
	assert.Contains(t, out, "疑似停滞/断链")

	taskDir2 := t.TempDir()
	fresh := strings.Replace(cliStateFixture, staleKey,
		`"`+state.NowISO()+`"`, 1)
	require.NotEqual(t, cliStateFixture, fresh)
	writeFileCli(t, filepath.Join(taskDir2, "state.json"), fresh)
	_, out = runCLI(t, "status", taskDir2)
	assert.NotContains(t, out, "疑似停滞/断链")
}

func TestStatusStallHintArms(t *testing.T) {
	staleKey := `"last-progress-at": "2026-08-22T00:00:00.000Z"`

	// 键缺失 → 静默（nil 臂）。
	taskDir := t.TempDir()
	noKey := strings.Replace(cliStateFixture,
		"\n  \"last-progress-at\": \"2026-08-22T00:00:00.000Z\",", "", 1)
	require.NotEqual(t, cliStateFixture, noKey)
	writeFileCli(t, filepath.Join(taskDir, "state.json"), noKey)
	_, out := runCLI(t, "status", taskDir)
	assert.NotContains(t, out, "疑似停滞")

	// 键存在但不可解析 → 静默（parse-error 臂）。
	taskDir2 := t.TempDir()
	bad := strings.Replace(cliStateFixture, staleKey, `"last-progress-at": "not-a-time"`, 1)
	writeFileCli(t, filepath.Join(taskDir2, "state.json"), bad)
	_, out = runCLI(t, "status", taskDir2)
	assert.NotContains(t, out, "疑似停滞")
}

func TestStatusStallHintUsesConfiguredWallTimeout(t *testing.T) {
	staleKey := `"last-progress-at": "2026-08-22T00:00:00.000Z"`
	taskDir := t.TempDir()
	stale := strings.Replace(cliStateFixture, staleKey,
		`"last-progress-at": "2020-01-01T00:00:00.000Z"`, 1)
	writeFileCli(t, filepath.Join(taskDir, "state.json"), stale)
	// 配置把 wall-clock 阈值压到极小 → 即便走配置命中臂也必然告警。
	writeFileCli(t, filepath.Join(taskDir, "flywheel.config.json"),
		`{"watchdog":{"wall-timeout-ms":1}}`)
	code, out := runCLI(t, "status", taskDir)
	require.Equal(t, 0, code)
	assert.Contains(t, out, "疑似停滞/断链")
}
