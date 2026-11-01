package machine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMilestoneGateRejectsCandidateMutation(t *testing.T) {
	for _, level := range []string{"L2", "L3"} {
		t.Run(level, func(t *testing.T) {
			fx := newGitFixture(t, "source.txt")
			c := unitTestContext(t, fx)
			p := c.Packets["M01-T01"]
			commands := []contract.GateEntry{{Cmd: "test -s source.txt && printf 'changed\\n' > source.txt", Expect: "exit 0"}}
			if level == "L2" {
				p.Acceptance.L2 = commands
			} else {
				p.Acceptance.L3 = commands
			}
			out := c.milestoneGateStep(milestoneOf(t, c))
			assert.True(t, out.Levels[level].(verify.GateReport).Pass, "真实命令 exit 0 不能为变化后的候选背书")
			require.False(t, out.Pass)
			gate := loadGateMap(t, c)
			assert.NotEqual(t, gate["identity-before"], gate["identity"])
			assert.Contains(t, gate["identity-error"], "changed during milestone acceptance")
			raw, err := os.ReadFile(filepath.Join(fx.root, "source.txt"))
			require.NoError(t, err)
			assert.Equal(t, "changed\n", string(raw), "保留失败现场，不悄悄回滚验收的副作用")
		})
	}
}

func TestMilestoneGateFailsClosedOnSnapshotErrors(t *testing.T) {
	for _, phase := range []string{"before", "after"} {
		t.Run(phase, func(t *testing.T) {
			fx := newGitFixture(t, "source.txt")
			c := unitTestContext(t, fx)
			p := c.Packets["M01-T01"]
			if phase == "before" {
				require.NoError(t, os.Remove(filepath.Join(fx.root, "source.txt")))
				require.NoError(t, os.Mkdir(filepath.Join(fx.root, "source.txt"), 0o755))
				p.Acceptance.L2 = []contract.GateEntry{{Cmd: "printf 'ran\\n' > must-not-run.txt", Expect: "exit 0"}}
			} else {
				p.Acceptance.L2 = []contract.GateEntry{{Cmd: "rm source.txt && mkdir source.txt", Expect: "exit 0"}}
			}
			out := c.milestoneGateStep(milestoneOf(t, c))
			require.False(t, out.Pass)
			assert.Contains(t, loadGateMap(t, c)["identity-error"], phase+" milestone acceptance")
			if phase == "before" {
				assert.NoFileExists(t, filepath.Join(fx.root, "must-not-run.txt"))
				assert.Equal(t, false, out.Levels["L2"].(map[string]any)["pass"])
			} else {
				assert.True(t, out.Levels["L2"].(verify.GateReport).Pass)
			}
		})
	}
}

func TestJudgeBindsPassingGateAndUsesMainCheckoutReadOnly(t *testing.T) {
	fx := newGitFixture(t, "source.txt")
	writeFile(t, filepath.Join(fx.root, ".gitignore"), "cache/\n")
	gitCmd(t, fx.root, "add", ".gitignore")
	gitCmd(t, fx.root, "commit", "-m", "ignore runtime cache")
	c := unitTestContext(t, fx)
	c.Packets["M01-T01"].Acceptance.L2 = []contract.GateEntry{{
		Cmd:    "mkdir -p cache && printf 'cache\\n' > cache/generated.txt && printf 'log\\n' > docs/minitask/orchestrator.log",
		Expect: "exit 0",
	}}
	milestone := milestoneOf(t, c)
	require.True(t, c.milestoneGateStep(milestone).Pass)
	identity := evidenceIdentity(loadGateMap(t, c))
	sd := happyDrivers()
	sd.judgeFn = func(a drivers.CallArgs) drivers.Result {
		assert.Equal(t, fx.root, a.Cwd, "Judge 读取已合并主 checkout，不能使用残留单元 worktree")
		assert.Equal(t, fx.taskDir, a.TaskDir)
		assert.Equal(t, "read-only", a.PermissionMode)
		assert.Contains(t, a.Prompt, "L2")
		writeFile(t, filepath.Join(fx.taskDir, "orchestrator.log"), "more runtime diagnostics\n")
		return passVerdict()
	}
	c.Drivers = sd.set()
	out, err := c.judgeStep(milestone)
	require.NoError(t, err)
	require.True(t, out.Pass)
	gate := loadGateMap(t, c)
	assert.Nil(t, gate["identity-error"])
	assert.Equal(t, identity, gate["judge"].(map[string]any)["identity"])
}

func TestJudgeRejectsStaleOrUnreadableGateBeforeCallingModel(t *testing.T) {
	for _, fault := range []string{"changed", "unreadable", "red-gate"} {
		t.Run(fault, func(t *testing.T) {
			fx := newGitFixture(t, "source.txt")
			c := unitTestContext(t, fx)
			milestone := milestoneOf(t, c)
			require.True(t, c.milestoneGateStep(milestone).Pass)
			switch fault {
			case "changed":
				writeFile(t, filepath.Join(fx.root, "new-source.txt"), "changed after L2/L3\n")
			case "unreadable":
				require.NoError(t, os.Remove(filepath.Join(fx.root, "source.txt")))
				require.NoError(t, os.Mkdir(filepath.Join(fx.root, "source.txt"), 0o755))
			case "red-gate":
				loadGateMap(t, c)["pass"] = false
			}
			sd := happyDrivers()
			c.Drivers = sd.set()
			out, err := c.judgeStep(milestone)
			require.NoError(t, err)
			assert.False(t, out.Pass)
			assert.Equal(t, errs.INVALID_REQUEST, out.ErrorCode)
			assert.Empty(t, sd.log.judge, "陈旧或红色门禁不产生模型调用")
			gate := loadGateMap(t, c)
			assert.Equal(t, false, gate["pass"])
			assert.Contains(t, gate["identity-error"], "before judge")
		})
	}
}

func TestJudgeCannotPassAfterMutatingCanonicalCandidate(t *testing.T) {
	for _, fault := range []string{"primary", "secondary", "unreadable", "provider-error"} {
		t.Run(fault, func(t *testing.T) {
			fx := newGitFixture(t, "source.txt")
			c := unitTestContext(t, fx)
			milestone := milestoneOf(t, c)
			require.True(t, c.milestoneGateStep(milestone).Pass)
			calls := 0
			sd := happyDrivers()
			sd.judgeFn = func(a drivers.CallArgs) drivers.Result {
				assert.Equal(t, fx.root, a.Cwd)
				assert.Equal(t, "read-only", a.PermissionMode)
				calls++
				if fault == "secondary" && calls == 1 {
					return passVerdict()
				}
				if fault == "unreadable" {
					require.NoError(t, os.Remove(filepath.Join(a.Cwd, "source.txt")))
					require.NoError(t, os.Mkdir(filepath.Join(a.Cwd, "source.txt"), 0o755))
				} else {
					writeFile(t, filepath.Join(a.Cwd, "source.txt"), "misbehaving harness changed source\n")
				}
				if fault == "provider-error" {
					return drivers.MockResult(func(r *drivers.Result) { r.ErrorMessage = strPtr("401 Invalid API key") })
				}
				return passVerdict()
			}
			c.Drivers = sd.set()
			if fault == "secondary" {
				c.Config.JudgePanel = panelBinding()
			}
			out, err := c.judgeStep(milestone)
			require.NoError(t, err)
			assert.False(t, out.Pass)
			assert.Nil(t, out.Verdict, "被拒绝的模型 pass 不得仍作为最终裁决展示")
			assert.Equal(t, errs.INVALID_REQUEST, out.ErrorCode)
			gate := loadGateMap(t, c)
			assert.Equal(t, false, gate["pass"])
			assert.Contains(t, gate["identity-error"], "after judge")
			judge := gate["judge"].(map[string]any)
			assert.Equal(t, "error", judge["verdict"])
			assert.NotNil(t, judge["rejected-judge"], "原始模型结论留作审计，不能抹成已通过")
			if fault == "secondary" {
				assert.Equal(t, 2, calls)
			}
		})
	}
}

func TestRunCannotCloseMilestoneAfterL2ChangesDeliveredSource(t *testing.T) {
	fx := newGitFixture(t, "source.txt")
	sd := happyDrivers()
	// run re-loads the contract after Plan; write the same L2 command into its
	// actual source of truth rather than depending on a mutated in-memory packet.
	path := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(raw)
	content = strings.Replace(content, "review-rubric: |", "  L2:\n    - {cmd: \"printf changed > source.txt\", expect: \"exit 0\"}\nreview-rubric: |", 1)
	writeFile(t, path, content)
	gitCmd(t, fx.root, "add", path)
	gitCmd(t, fx.root, "commit", "-m", "freeze milestone acceptance")
	summary, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "escalated", summary.Status)
	st := loadFixtureState(t, fx)
	assert.NotEqual(t, "closed", st.Milestones["M-01"].Status)
	assert.Empty(t, sd.log.judge, "正式 run 路径不能绕过变化后的 L2/L3 去问 Judge")
}
