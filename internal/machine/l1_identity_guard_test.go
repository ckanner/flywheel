package machine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestL1RejectsAcceptanceThatChangesCandidate(t *testing.T) {
	for _, tc := range []struct {
		name      string
		command   string
		worktree  bool
		untracked bool
	}{
		{name: "tracked-source", command: "test -s source.txt && printf 'changed\\n' > source.txt"},
		{name: "new-untracked-source", command: "test -s source.txt && printf 'new\\n' > generated.txt", worktree: true},
		{name: "existing-untracked-source", command: "test -s draft.txt && printf 'changed\\n' > draft.txt", untracked: true},
		{name: "removed-source", command: "test -s source.txt && rm source.txt", worktree: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newGitFixture(t, "source.txt")
			c := unitTestContext(t, fx)
			p := c.Packets["M01-T01"]
			ps := c.State.Packets[p.PacketID]
			base, ok := verify.GetBaseRef(fx.root)
			require.True(t, ok)
			ps.BaseRef = &base
			if tc.worktree {
				wt := verify.EnsureUnitWorktree(fx.root, fx.slug, p.PacketID, base)
				require.Equal(t, "worktree", wt.Mode)
				ps.Worktree = &wt
			}
			if tc.untracked {
				writeFile(t, filepath.Join(c.unitCwd(p), "draft.txt"), "candidate before acceptance\n")
			}
			p.Acceptance.L1 = []contract.GateEntry{{Cmd: tc.command, Expect: "exit 0"}}
			out := c.l1Step(p)
			assert.True(t, out.Gate.Pass, "验收命令确实正常退出为绿，不能靠 exit code 发现候选变更")
			assert.True(t, out.Boundary.Pass, "普通实现路径在任务范围内，失败应来自身份变化")
			require.False(t, out.Pass)
			assert.Equal(t, "l1-failed", ps.Status)
			assert.False(t, c.currentL1Evidence(p))
			ev := ps.Evidence.L1.(map[string]any)
			assert.NotEqual(t, ev["identity-before"], ev["identity"])
			assert.Contains(t, ev["identity-error"], "changed during acceptance")

			// 下一次必须实际重验；不能把上一轮绿灯套在修改后的候选上。
			p.Acceptance.L1 = []contract.GateEntry{{Cmd: "true", Expect: "exit 0"}}
			assert.True(t, c.l1Step(p).Pass)
			assert.True(t, c.currentL1Evidence(p))
		})
	}
}

func TestL1IdentityAllowsRuntimeLedgerAndIgnoredBuildOutputs(t *testing.T) {
	fx := newGitFixture(t, "source.txt")
	writeFile(t, filepath.Join(fx.root, ".gitignore"), "cache/\n")
	gitCmd(t, fx.root, "add", ".gitignore")
	gitCmd(t, fx.root, "commit", "-m", "ignore build cache")
	c := unitTestContext(t, fx)
	p := c.Packets["M01-T01"]
	p.Acceptance.L1 = []contract.GateEntry{{
		Cmd:    "mkdir -p docs/minitask/reports cache && printf 'log\\n' > docs/minitask/orchestrator.log && printf 'retro\\n' > docs/minitask/retro.md && printf '{}\\n' > docs/minitask/reports/other.done.json && printf 'cache\\n' > cache/generated.txt",
		Expect: "exit 0",
	}}
	out := c.l1Step(p)
	require.True(t, out.Pass)
	assert.True(t, c.currentL1Evidence(p))
	ev := c.State.Packets[p.PacketID].Evidence.L1.(map[string]any)
	assert.Equal(t, ev["identity-before"], ev["identity"])
	assert.Nil(t, ev["identity-error"])
	assert.FileExists(t, filepath.Join(fx.root, "cache", "generated.txt"))
	assert.FileExists(t, filepath.Join(fx.taskDir, "reports", "other.done.json"))
}

func TestL1FailsClosedWhenIdentityCannotBeRead(t *testing.T) {
	for _, phase := range []string{"before", "after"} {
		t.Run(phase, func(t *testing.T) {
			fx := newGitFixture(t)
			c := unitTestContext(t, fx)
			p := c.Packets["M01-T01"]
			if phase == "before" {
				require.NoError(t, os.Remove(filepath.Join(fx.taskDir, "spec.md")))
				p.Acceptance.L1 = []contract.GateEntry{{Cmd: "printf 'ran\\n' > must-not-run.txt", Expect: "exit 0"}}
			} else {
				p.Acceptance.L1 = []contract.GateEntry{{Cmd: "rm docs/minitask/spec.md", Expect: "exit 0"}}
			}
			out := c.l1Step(p)
			require.False(t, out.Pass)
			ev := c.State.Packets[p.PacketID].Evidence.L1.(map[string]any)
			assert.Contains(t, ev["identity-error"], phase+" acceptance")
			if phase == "before" {
				assert.Empty(t, out.Gate.Results, "没有执行前身份时不启动验收命令")
				assert.NoFileExists(t, filepath.Join(fx.root, "must-not-run.txt"))
			} else {
				assert.True(t, out.Gate.Pass)
			}
			assert.False(t, c.currentL1Evidence(p))
		})
	}
}

func TestExecutorReceivesActualProtectedPatternsInStablePrompt(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	c.Config.Verify.ProtectedTestPatterns = []string{`^acceptance/auth/`, `^e2e/auth-flow\.ts$`}
	sd := happyDrivers()
	c.Drivers = sd.set()
	out, err := c.executeStep(c.Packets["M01-T01"])
	require.NoError(t, err)
	require.True(t, out.Success)
	require.Len(t, sd.log.exec, 1)
	stable := strings.Split(sd.log.exec[0].Prompt, "## Runtime (volatile)")[0]
	for _, pattern := range c.Config.Verify.ProtectedTestPatterns {
		assert.Contains(t, stable, pattern)
	}
	assert.Contains(t, stable, "boundaries.must-not-touch: forbidden/")
}
