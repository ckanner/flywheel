package machine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func appendPacketRevision(t *testing.T, fx fixture, id string) {
	t.Helper()
	path := filepath.Join(fx.taskDir, "task-packet-"+id+".md")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	writeFile(t, path, string(raw)+"\nRevised requirement: preserve existing implementation and add regression evidence.\n")
}

func pausedContractFixture(t *testing.T, worktree bool) (fixture, *Context, string) {
	t.Helper()
	fx := newGitFixture(t, "wip.txt")
	c := unitTestContext(t, fx)
	id := "M01-T01"
	ps := c.State.Packets[id]
	head, ok := verify.GetBaseRef(fx.root)
	require.True(t, ok)
	ps.BaseRef = &head
	ps.Status = "l1-passed"
	ps.FailedAttempts = 2
	ps.LastFailureSignature = strPtrAlways("earlier failure")
	if worktree {
		wi := verify.EnsureUnitWorktree(fx.root, fx.slug, id, head)
		require.Equal(t, "worktree", wi.Mode)
		ps.Worktree = &wi
	}
	writeFile(t, filepath.Join(c.unitCwd(c.Packets[id]), "wip.txt"), "valuable uncommitted implementation\n")
	seedVerification(t, c, id)
	return fx, c, id
}

func TestPlanSelfCommittedContractStillSynchronizesPausedWorkWithoutNewCommit(t *testing.T) {
	for _, worktree := range []bool{true, false} {
		t.Run(map[bool]string{true: "worktree", false: "in-place"}[worktree], func(t *testing.T) {
			fx, c, id := pausedContractFixture(t, worktree)
			ps := c.State.Packets[id]
			appendPacketRevision(t, fx, id)
			writeFile(t, filepath.Join(fx.taskDir, "retro.md"), "prior audit ledger\n")
			gitCmd(t, fx.root, "add", "--", "docs/minitask/task-packet-"+id+".md", "docs/minitask/retro.md")
			gitCmd(t, fx.root, "commit", "-m", "planner already persisted contract and audit")
			head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
			require.NoError(t, c.reloadPackets())
			require.True(t, ps.ContractSyncPending)
			assert.Nil(t, ps.Evidence.L1)
			assert.NotEmpty(t, ps.EvidenceHistory)
			assert.Zero(t, ps.FailedAttempts)
			assert.Nil(t, ps.LastFailureSignature)
			require.NoError(t, c.persistPlanContracts("docs: no additional change"))
			assert.Equal(t, head, strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD")), "没有新SHA仍必须同步已有checkpoint")
			assert.Equal(t, head, *ps.BaseRef)
			assert.False(t, ps.ContractSyncPending)
			cwd := c.unitCwd(c.Packets[id])
			assert.Equal(t, head, strings.TrimSpace(gitCmd(t, cwd, "rev-parse", "HEAD")))
			wip, err := os.ReadFile(filepath.Join(cwd, "wip.txt"))
			require.NoError(t, err)
			assert.Equal(t, "valuable uncommitted implementation\n", string(wip))
			changed, ok := verify.GetChangedFiles(cwd, *ps.BaseRef)
			require.True(t, ok)
			assert.Contains(t, changed, "wip.txt", "重锚后WIP仍在执行器边界内")
			assert.NotContains(t, changed, "docs/minitask/task-packet-"+id+".md", "合法契约修订已并入新base")
		})
	}
}

func TestPlanRevisionOfAnotherPacketSynchronizesPausedCandidateContracts(t *testing.T) {
	fx, c, id := pausedContractFixture(t, true)
	ps := c.State.Packets[id]
	oldFingerprint := c.State.ContractFingerprints[id]
	appendPacketRevision(t, fx, "M01-T02")
	gitCmd(t, fx.root, "add", "--", "docs/minitask/task-packet-M01-T02.md")
	gitCmd(t, fx.root, "commit", "-m", "revise adjacent packet only")
	head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
	require.NoError(t, c.reloadPackets())
	assert.Equal(t, oldFingerprint, c.State.ContractFingerprints[id], "自身契约没改，但candidate中的其他契约已更新")
	require.NoError(t, c.persistPlanContracts("docs: adjacent packet revision"))
	assert.Equal(t, head, *ps.BaseRef)
	assert.Equal(t, head, strings.TrimSpace(gitCmd(t, ps.Worktree.Path, "rev-parse", "HEAD")))
	assert.Nil(t, ps.Evidence.L1, "候选目录契约集已变，旧L1不能继续复用")
	wip, err := os.ReadFile(filepath.Join(ps.Worktree.Path, "wip.txt"))
	require.NoError(t, err)
	assert.Equal(t, "valuable uncommitted implementation\n", string(wip))
	_, violations := c.partitionChangedFiles(c.authoritativeChangedFiles(c.Packets[id]))
	assert.Empty(t, violations, "合法邻近契约更新不能被误判executor越权")
}

func TestLoadedUserVerificationConfigCheckpointSynchronizesPausedWork(t *testing.T) {
	fx, original, id := pausedContractFixture(t, true)
	original.save()
	require.NoError(t, original.err())
	writeFile(t, filepath.Join(fx.taskDir, "flywheel.config.json"), strings.Replace(fixtureConfigJSON, `"command-timeout-ms": 10000`, `"command-timeout-ms": 10001`, 1))
	c := unitTestContext(t, fx)
	ps := c.State.Packets[id]
	require.True(t, ps.ContractSyncPending)
	require.NoError(t, c.persistPlanContracts("docs: user changed verification configuration"))
	head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
	assert.Equal(t, head, *ps.BaseRef)
	assert.Equal(t, head, strings.TrimSpace(gitCmd(t, ps.Worktree.Path, "rev-parse", "HEAD")))
	assert.Nil(t, ps.Evidence.L1)
	wip, err := os.ReadFile(filepath.Join(ps.Worktree.Path, "wip.txt"))
	require.NoError(t, err)
	assert.Equal(t, "valuable uncommitted implementation\n", string(wip))
}

func TestPlanReanchorRefusesMixedCheckpointAndDivergentWorktree(t *testing.T) {
	for _, scenario := range []string{"mixed-code", "divergent", "inventory-failure"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, id := pausedContractFixture(t, true)
			ps := c.State.Packets[id]
			originalBase := *ps.BaseRef
			if scenario == "divergent" {
				gitCmd(t, ps.Worktree.Path, "add", "--", "wip.txt")
				gitCmd(t, ps.Worktree.Path, "commit", "-m", "executor self-committed WIP")
			}
			appendPacketRevision(t, fx, id)
			if scenario == "mixed-code" {
				writeFile(t, filepath.Join(fx.root, "unexpected.go"), "package unexpected\n")
				gitCmd(t, fx.root, "add", "--", "unexpected.go")
			}
			gitCmd(t, fx.root, "add", "--", "docs/minitask/task-packet-"+id+".md")
			gitCmd(t, fx.root, "commit", "-m", "planner checkpoint")
			require.NoError(t, c.reloadPackets())
			if scenario == "inventory-failure" {
				original := planRevisionFiles
				t.Cleanup(func() { planRevisionFiles = original })
				planRevisionFiles = func(string, string, string) ([]byte, error) { return nil, os.ErrPermission }
			}
			err := c.persistPlanContracts("docs: preserve unsafe checkpoint")
			require.ErrorIs(t, err, ErrMergeConflict)
			assert.True(t, ps.ContractSyncPending)
			assert.Equal(t, originalBase, *ps.BaseRef, "不能把未验证实现变化抽走到新边界之外")
			assert.DirExists(t, ps.Worktree.Path)
			wip, err := os.ReadFile(filepath.Join(ps.Worktree.Path, "wip.txt"))
			require.NoError(t, err)
			assert.Equal(t, "valuable uncommitted implementation\n", string(wip))
			if scenario == "divergent" {
				assert.NotEmpty(t, ps.Commit.(map[string]any)["plan-sync-error"])
			}
		})
	}
}

func TestPlanContractCommitFailureAndNonGitPlanningPreserveDocuments(t *testing.T) {
	fx, c, id := pausedContractFixture(t, true)
	appendPacketRevision(t, fx, id)
	require.NoError(t, c.reloadPackets())
	installCommitHook(t, fx, "exit 1")
	err := c.persistPlanContracts("docs: rejected checkpoint")
	require.ErrorIs(t, err, ErrCommitFailed)
	assert.True(t, c.State.Packets[id].ContractSyncPending)
	assert.DirExists(t, c.State.Packets[id].Worktree.Path)
	raw, err := os.ReadFile(filepath.Join(fx.taskDir, "task-packet-"+id+".md"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "Revised requirement")

	nonGit := newFixture(t)
	nc := unitTestContext(t, nonGit)
	appendPacketRevision(t, nonGit, id)
	require.NoError(t, nc.reloadPackets())
	require.NoError(t, nc.persistPlanContracts("docs: planning rehearsal"), "非Git可保留规划草稿，最终completed另有持久化硬门禁")
}

func TestPlanScopeRejectsImplementationExternalOracleAndConfigurationEdits(t *testing.T) {
	for _, path := range []string{"source.go", "acceptance/external_oracle_test.go", "docs/minitask/flywheel.config.json"} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			fx := newGitFixture(t, "source.go", "acceptance/external_oracle_test.go")
			c := unitTestContext(t, fx)
			sd := happyDrivers()
			sd.planFn = func(a drivers.CallArgs) drivers.Result {
				writeFile(t, filepath.Join(a.Cwd, path), "unauthorized Plan mutation\n")
				return sd.mock.Plan(a)
			}
			c.Drivers = sd.set()
			head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
			_, err := c.planUntilValid(c.Spec.Milestones[0])
			require.ErrorContains(t, err, "Plan changed implementation, external acceptance, or configuration")
			assert.Equal(t, head, strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD")))
			raw, err := os.ReadFile(filepath.Join(fx.root, path))
			require.NoError(t, err)
			assert.Equal(t, "unauthorized Plan mutation\n", string(raw), "拒绝自动提交并保留修改供恢复")
		})
	}
}

func TestPlanScopeAllowsOnlyOwnContractsAndLedger(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	before, err := c.planScopeIdentity()
	require.NoError(t, err)
	appendPacketRevision(t, fx, "M01-T01")
	writeFile(t, filepath.Join(fx.taskDir, "spec.md"), fixtureSpec+"\nClarified scope.\n")
	writeFile(t, filepath.Join(fx.taskDir, "retro.md"), "Planner clarification was reviewed.\n")
	after, err := c.planScopeIdentity()
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.True(t, c.isPlannerContract("docs/minitask/spec.md"))
	assert.True(t, c.isPlannerContract("docs/minitask/task-packet-M01-T01.md"))
	assert.False(t, c.isPlannerContract("docs/other/spec.md"))
	assert.False(t, c.isPlannerContract("docs/minitask/flywheel.config.json"))
	assert.False(t, c.isPlannerContract("docs/minitask/unrelated.md"))
}

func TestContractRevisionAcrossRestartReopensChangedOrLegacyCompletion(t *testing.T) {
	for _, scenario := range []string{"unchanged", "packet", "spec", "verification-config", "legacy"} {
		t.Run(scenario, func(t *testing.T) {
			fx := newGitFixture(t)
			first, err := RunTask(runOpts(fx, happyDrivers().set()))
			require.NoError(t, err)
			require.Equal(t, "completed", first.Status)
			before := loadFixtureState(t, fx)
			fingerprint := before.ContractFingerprints["M01-T01"]
			require.NotEmpty(t, fingerprint)
			switch scenario {
			case "unchanged":
				writeFile(t, filepath.Join(fx.root, "later-unit.go"), "package later\n")
				gitCmd(t, fx.root, "add", "--", "later-unit.go")
				gitCmd(t, fx.root, "commit", "--only", "-m", "later independently delivered change", "--", "later-unit.go")
			case "packet":
				appendPacketRevision(t, fx, "M01-T01")
			case "spec":
				writeFile(t, filepath.Join(fx.taskDir, "spec.md"), fixtureSpec+"\nNew global requirement clarification.\n")
			case "legacy":
				before.ContractFingerprints = nil
				_, err := state.Save(fx.taskDir, before, nil)
				require.NoError(t, err)
			}
			cfg := configFor(fx)
			if scenario == "verification-config" {
				cfg.Verify.CommandTimeoutMs++
			}
			c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set()})
			require.NoError(t, err)
			if scenario == "unchanged" {
				assert.Equal(t, "completed", c.State.Status)
				assert.Equal(t, "completed", c.State.Packets["M01-T01"].Status)
				assert.Equal(t, fingerprint, c.State.ContractFingerprints["M01-T01"])
				assert.True(t, c.contractsComplete())
				return
			}
			ps := c.State.Packets["M01-T01"]
			assert.Equal(t, "pending", c.State.Status)
			assert.Equal(t, "pending", ps.Status)
			assert.Nil(t, ps.BaseRef, "已清理交付应从当前HEAD重开，不能重用旧执行base")
			assert.Nil(t, ps.Evidence.L1)
			assert.Nil(t, ps.Evidence.Review)
			assert.NotEmpty(t, ps.EvidenceHistory)
			assert.Equal(t, "pending", c.State.Milestones["M-01"].Status)
			assert.Nil(t, c.State.Milestones["M-01"].Gate)
			assert.Nil(t, c.State.Milestones["M-01"].ClosedAt)
			assert.False(t, c.contractsComplete())
			if scenario == "packet" {
				assert.Equal(t, "completed", c.State.Packets["M01-T02"].Status, "未改动packet的完成证据保留")
			}
		})
	}
}

func TestLegacyUnfinishedStateRetainsRetryAndCredentialHistory(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	ps := c.State.Packets["M01-T01"]
	ps.Status = "reviewing"
	ps.FailedAttempts = 2
	ps.Interrupted = true
	ps.LastFailureSignature = strPtrAlways("same-failure")
	ps.Evidence.Credentials = map[string]any{"approval": "approved names only"}
	c.State.ContractFingerprints = nil
	require.NoError(t, c.reconcileContractRevisions())
	assert.Equal(t, "reviewing", ps.Status)
	assert.Equal(t, 2, ps.FailedAttempts)
	assert.True(t, ps.Interrupted)
	assert.Equal(t, "same-failure", *ps.LastFailureSignature)
	assert.NotNil(t, ps.Evidence.Credentials)
	assert.NotEmpty(t, c.State.ContractFingerprints["M01-T01"])
}

func TestContractReloadFailsClosedWithoutPartialFingerprintUpdate(t *testing.T) {
	for _, failAt := range []string{"spec.md", "task-packet-M01-T02.md"} {
		t.Run(failAt, func(t *testing.T) {
			fx := newGitFixture(t)
			c := unitTestContext(t, fx)
			before := c.State.ContractFingerprints
			appendPacketRevision(t, fx, "M01-T01")
			original := readContractRevision
			t.Cleanup(func() { readContractRevision = original })
			readContractRevision = func(path string) ([]byte, error) {
				if filepath.Base(path) == failAt {
					return nil, os.ErrPermission
				}
				return original(path)
			}
			require.Error(t, c.reloadPackets())
			assert.Equal(t, before, c.State.ContractFingerprints, "读取整组契约失败不能更新一半身份")
		})
	}
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	writeFile(t, filepath.Join(fx.taskDir, "spec.md"), "invalid spec")
	require.Error(t, c.reloadPackets())
	writeFile(t, filepath.Join(fx.taskDir, "spec.md"), fixtureSpec)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), "invalid packet")
	require.Error(t, c.reloadPackets())
}

func TestClosedMilestoneReopensForNewPacketAfterRestart(t *testing.T) {
	fx := newGitFixture(t)
	first, err := RunTask(runOpts(fx, happyDrivers().set()))
	require.NoError(t, err)
	require.Equal(t, "completed", first.Status)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T03.md"), fixturePacket("M01-T03", "R-01"))
	sd := happyDrivers()
	second, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", second.Status)
	assert.Equal(t, "3/3", second.PacketsCompleted)
	require.Len(t, sd.log.exec, 1)
	assert.Equal(t, "M01-T03", sd.log.exec[0].PacketID)
	assert.Equal(t, "closed", loadFixtureState(t, fx).Milestones["M-01"].Status)
}

func TestDynamicMilestonesRevisitEarlierUnitsChangedByLaterPlan(t *testing.T) {
	fx := newGitFixture(t)
	spec := strings.Replace(fixtureSpec, "requirements:\n", "  - id: M-02\n    title: Second milestone\n    gate: Second artifact verified.\nrequirements:\n", 1)
	spec = strings.Replace(spec, "---\n\n# Mini spec", "  - id: R-03\n    title: Third thing works\n    milestone: M-02\n    verification: test -f build/M02-T01.txt\n---\n\n# Mini spec", 1)
	writeFile(t, filepath.Join(fx.taskDir, "spec.md"), spec)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M02-T01.md"), fixturePacket("M02-T01", "R-03"))
	gitCmd(t, fx.root, "add", "--", "docs/minitask/spec.md", "docs/minitask/task-packet-M02-T01.md")
	gitCmd(t, fx.root, "commit", "-m", "second milestone contracts")
	sd := happyDrivers()
	revised := false
	sd.planFn = func(a drivers.CallArgs) drivers.Result {
		if a.MilestoneID == "M-02" && !revised {
			revised = true
			appendPacketRevision(t, fx, "M01-T01")
			writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T03.md"), fixturePacket("M01-T03", "R-01"))
		}
		return sd.mock.Plan(a)
	}
	result, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)
	assert.Equal(t, "4/4", result.PacketsCompleted)
	var plans, executions []string
	for _, a := range sd.log.plan {
		plans = append(plans, a.MilestoneID)
	}
	for _, a := range sd.log.exec {
		executions = append(executions, a.PacketID)
	}
	assert.Equal(t, []string{"M-01", "M-02", "M-01"}, plans, "重新打开的较早里程碑必须再访")
	assert.Equal(t, []string{"M01-T01", "M01-T02", "M02-T01", "M01-T01", "M01-T03"}, executions)
	st := loadFixtureState(t, fx)
	assert.Equal(t, "closed", st.Milestones["M-01"].Status)
	assert.Equal(t, "closed", st.Milestones["M-02"].Status)
	assert.Equal(t, 2, st.Packets["M01-T01"].Attempts)
	assert.NotEmpty(t, st.Packets["M01-T01"].EvidenceHistory)
}

func TestContractsCompleteIgnoresRetiredStateButRequiresCurrentUnits(t *testing.T) {
	c := unitTestContext(t)
	c.State.Milestones["M-01"] = state.MilestoneState{Status: "closed"}
	assert.False(t, c.contractsComplete(), "已关闭里程碑不能掩盖未完成packet")
	for _, p := range c.PacketList {
		c.State.Packets[p.PacketID].Status = "completed"
	}
	c.State.Packets["M99-T99"] = &state.PacketState{Status: "pending"}
	c.State.Milestones["M-99"] = state.MilestoneState{Status: "pending"}
	assert.True(t, c.contractsComplete(), "已退役契约的历史记录不阻止当前任务完成")

	original := planRevisionFiles
	t.Cleanup(func() { planRevisionFiles = original })
	planRevisionFiles = func(string, string, string) ([]byte, error) { return nil, errors.New("git inventory failed") }
	require.ErrorContains(t, c.checkPlanReanchor("old", "new"), "cannot inspect plan checkpoint")
}

func TestContextRefusesUnreadableContractRevisionAfterParsing(t *testing.T) {
	fx := newGitFixture(t)
	original := readContractRevision
	t.Cleanup(func() { readContractRevision = original })
	readContractRevision = func(string) ([]byte, error) { return nil, os.ErrPermission }
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: configFor(fx), Drivers: happyDrivers().set()})
	require.ErrorIs(t, err, os.ErrPermission)
	assert.Nil(t, c, "schema可读但revision证据读取失败时不能进入运行")
}

func TestPlanScopeReadFailureBeforeOrAfterCallStopsProgress(t *testing.T) {
	for _, failureCall := range []int{1, 2} {
		t.Run(map[int]string{1: "before", 2: "after"}[failureCall], func(t *testing.T) {
			fx := newGitFixture(t)
			c := unitTestContext(t, fx)
			sd := happyDrivers()
			c.Drivers = sd.set()
			original := snapshotGitFiles
			t.Cleanup(func() { snapshotGitFiles = original })
			calls := 0
			snapshotGitFiles = func(cwd string) ([]byte, error) {
				calls++
				if calls == failureCall {
					return nil, os.ErrPermission
				}
				return original(cwd)
			}
			_, err := c.planUntilValid(c.Spec.Milestones[0])
			require.ErrorIs(t, err, os.ErrPermission)
			assert.Len(t, sd.log.plan, failureCall-1)
			assert.Empty(t, sd.log.exec)
		})
	}
}

func TestTaskAndSinglePlanStopWhenContractPersistenceFails(t *testing.T) {
	for _, single := range []bool{true, false} {
		t.Run(map[bool]string{true: "single", false: "task"}[single], func(t *testing.T) {
			fx := newGitFixture(t)
			installCommitHook(t, fx, "exit 1")
			sd := happyDrivers()
			sd.planFn = func(a drivers.CallArgs) drivers.Result {
				appendPacketRevision(t, fx, "M01-T01")
				return sd.mock.Plan(a)
			}
			var err error
			if single {
				opts := stepOpts(fx, sd.set())
				opts.Only = "plan"
				_, err = RunSingleStep(opts)
			} else {
				_, err = RunTask(runOpts(fx, sd.set()))
			}
			require.ErrorIs(t, err, ErrCommitFailed)
			assert.Empty(t, sd.log.exec, "契约未持久化之前不启动执行器")
			assert.NotEqual(t, "completed", loadFixtureState(t, fx).Status)
		})
	}
}

func TestRunRefusesRemovedActiveMilestoneAndInconsistentClosedState(t *testing.T) {
	t.Run("active milestone removed", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		sd.planFn = func(a drivers.CallArgs) drivers.Result {
			writeFile(t, filepath.Join(fx.taskDir, "spec.md"), strings.ReplaceAll(fixtureSpec, "M-01", "M-02"))
			return sd.mock.Plan(a)
		}
		_, err := RunTask(runOpts(fx, sd.set()))
		require.ErrorContains(t, err, "active milestone M-01 was removed by Plan")
		assert.Empty(t, sd.log.exec)
		assert.NotEqual(t, "completed", loadFixtureState(t, fx).Status)
	})
	t.Run("closed milestone with unfinished packet", func(t *testing.T) {
		fx := newGitFixture(t)
		c := unitTestContext(t, fx)
		c.State.Milestones["M-01"] = state.MilestoneState{Status: "closed"}
		c.State.Packets["M01-T01"].Status = "completed"
		c.save()
		require.NoError(t, c.err())
		sd := happyDrivers()
		_, err := RunTask(runOpts(fx, sd.set()))
		require.ErrorContains(t, err, "current contracts contain unfinished units")
		assert.Empty(t, sd.log.exec)
		assert.NotEqual(t, "completed", loadFixtureState(t, fx).Status)
	})
}

func TestFinalTaskCompletionRefusesCodeChangedAfterUnitPersistence(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	sd.judgeFn = func(a drivers.CallArgs) drivers.Result {
		writeFile(t, filepath.Join(fx.root, "after-unit-persistence.go"), "unverified late modification\n")
		return sd.mock.Judge(a)
	}
	result, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err, "Judge前后身份检查更早拒绝变更并升级")
	assert.Equal(t, "escalated", result.Status)
	st := loadFixtureState(t, fx)
	assert.Equal(t, "escalated", st.Status)
	gate := st.Milestones["M-01"].Gate.(map[string]any)
	assert.Equal(t, false, gate["pass"])
	assert.Contains(t, gate["identity-error"], "after judge")
	assert.FileExists(t, filepath.Join(fx.root, "after-unit-persistence.go"))
	for _, id := range []string{"M01-T01", "M01-T02"} {
		assert.Equal(t, "completed", st.Packets[id].Status, "先前已持久化的unit事实不因后来修改被伪造为丢失")
	}
}

func TestRepeatedContractReopeningStopsAtMilestoneVisitLimit(t *testing.T) {
	fx := newGitFixture(t)
	spec := strings.Replace(fixtureSpec, "requirements:\n", "  - id: M-02\n    title: Second milestone\n    gate: Second artifact verified.\nrequirements:\n", 1)
	spec = strings.Replace(spec, "---\n\n# Mini spec", "  - id: R-03\n    title: Third thing works\n    milestone: M-02\n    verification: test -f build/M02-T01.txt\n---\n\n# Mini spec", 1)
	writeFile(t, filepath.Join(fx.taskDir, "spec.md"), spec)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M02-T01.md"), fixturePacket("M02-T01", "R-03"))
	gitCmd(t, fx.root, "add", "--", "docs/minitask/spec.md", "docs/minitask/task-packet-M02-T01.md")
	gitCmd(t, fx.root, "commit", "-m", "second milestone contracts")
	sd := happyDrivers()
	visits := map[string]int{}
	sd.planFn = func(a drivers.CallArgs) drivers.Result {
		visits[a.MilestoneID]++
		if a.MilestoneID == "M-02" {
			appendPacketRevision(t, fx, "M01-T01")
		} else if visits[a.MilestoneID] > 1 {
			appendPacketRevision(t, fx, "M02-T01")
		}
		return sd.mock.Plan(a)
	}
	opts := runOpts(fx, sd.set())
	every := 100
	opts.Config.DefectInjection.EveryNUnits = &every
	opts.Log = func(line string) { t.Log(line) }
	result, err := RunTask(opts)
	require.NoError(t, err)
	assert.Equal(t, "escalated", result.Status)
	assert.Equal(t, map[string]int{"M-01": maxMilestoneVisits, "M-02": maxMilestoneVisits}, visits)
	st := loadFixtureState(t, fx)
	require.NotEmpty(t, st.Inbox)
	t.Logf("last feedback: %#v", st.Packets["M01-T01"].LastFeedback)
	last := st.Inbox[len(st.Inbox)-1].(map[string]any)
	assert.Contains(t, last["reason"], "repeatedly reopened by contract revisions")
	assert.Equal(t, "pending", st.Packets["M01-T01"].Status)
}
