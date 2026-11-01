package machine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 生命周期函数的单测从一份已验证候选开始；真实 L1/Review 组合另由端到端
// fixture 覆盖。这里的身份不能硬编码，否则提交后篡改回归会被测试漏掉。
func seedVerification(t *testing.T, c *Context, id string) {
	t.Helper()
	identity, err := c.verificationIdentity(c.Packets[id])
	require.NoError(t, err)
	ps := c.State.Packets[id]
	ps.Evidence.L1 = map[string]any{"pass": true, "identity": identity}
	ps.Evidence.Review = map[string]any{"verdict": "pass", "identity": identity}
}

func TestCommitFailurePreservesVerifiedWork(t *testing.T) {
	for _, worktree := range []bool{true, false} {
		t.Run(fmt.Sprint(worktree), func(t *testing.T) {
			fx := newGitFixture(t)
			ctx := unitTestContext(t, fx)
			ctx.Config.Worktree.Enabled = &worktree
			packet := ctx.Packets["M01-T01"]
			result, err := ctx.executeStep(packet)
			require.NoError(t, err)
			require.True(t, result.Success)
			require.True(t, ctx.l1Step(packet).Pass)
			r, err := ctx.reviewStep(packet, nil)
			require.NoError(t, err)
			require.True(t, r.Pass)
			hook := filepath.Join(fx.root, ".git", "hooks", "pre-commit")
			writeFile(t, hook, "#!/bin/sh\nexit 1\n")
			require.NoError(t, os.Chmod(hook, 0o755))
			err = ctx.completeUnit(packet)
			require.ErrorIs(t, err, ErrCommitFailed)
			assert.NotEqual(t, "completed", ctx.State.Packets[packet.PacketID].Status)
			assert.FileExists(t, filepath.Join(ctx.unitCwd(packet), "build", packet.PacketID+".txt"))
			assert.NotNil(t, ctx.State.Packets[packet.PacketID].Commit.(map[string]any)["error"])
		})
	}
}

func TestResumeRejectsModifiedCandidateAndReport(t *testing.T) {
	for _, modified := range []string{"candidate", "report", "verification-config"} {
		t.Run(modified, func(t *testing.T) {
			fx := newGitFixture(t)
			sd := happyDrivers()
			opts := runOpts(fx, sd.set())
			opts.PauseBefore = "review"
			_, err := RunTask(opts)
			require.NoError(t, err)
			st := loadFixtureState(t, fx)
			switch modified {
			case "candidate":
				writeFile(t, filepath.Join(st.Packets["M01-T01"].Worktree.Path, "build", "M01-T01.txt"), "tampered")
			case "report":
				p := filepath.Join(fx.taskDir, "reports", "M01-T01.done.json")
				b, err := os.ReadFile(p)
				require.NoError(t, err)
				writeFile(t, p, string(b)+"\n")
			default:
				// 同一候选代码但验证配置不同，旧绿灯也无效。
				opts.Config.Verify.CommandTimeoutMs++
			}
			opts.PauseBefore = ""
			summary, err := RunTask(opts)
			require.NoError(t, err)
			assert.Equal(t, "completed", summary.Status)
			assert.Equal(t, 2, loadFixtureState(t, fx).Packets["M01-T01"].Attempts)
		})
	}
}

func TestRetryCarriesEvidenceAndReviewUsesCandidate(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	count := 0
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		assert.NotEqual(t, fx.root, a.Cwd)
		assert.Equal(t, "read-only", a.PermissionMode)
		assert.FileExists(t, filepath.Join(a.Cwd, "build", "M01-T01.txt"))
		count++
		if count == 1 {
			return drivers.MockResult(func(r *drivers.Result) {
				r.Text = `{"verdict":"return","score":0.4,"reasons":["build/M01-T01.txt:1 requires a useful regression assertion"]}`
			})
		}
		return sd.mock.Review(a)
	}
	_, err := RunTask(runOpts(fx, sd.set()))
	require.NoError(t, err)
	require.Len(t, sd.log.exec, 3)
	assert.Contains(t, sd.log.exec[1].Prompt, "requires a useful regression assertion")
	assert.Contains(t, sd.log.exec[1].Prompt, "Previous attempt feedback")
	assert.NotContains(t, sd.log.exec[0].Prompt, "Previous attempt feedback")
}

func TestRunLeaseAndCooperativeCancellation(t *testing.T) {
	fx := newFixture(t)
	release, err := AcquireRunLease(fx.taskDir)
	require.NoError(t, err)
	_, err = AcquireRunLease(fx.taskDir)
	require.Error(t, err)
	_, err = RunTask(runOpts(fx, happyDrivers().set()))
	require.Error(t, err)
	_, err = RunSingleStep(stepOpts(fx, happyDrivers().set()))
	require.Error(t, err)
	release()
	_, err = AcquireRunLease(filepath.Join(fx.root, "missing"))
	require.Error(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opts := runOpts(fx, happyDrivers().set())
	opts.Context = ctx
	_, err = RunTask(opts)
	require.ErrorIs(t, err, ErrInterrupted)
	_, err = RunSingleStep(RunSingleStepOpts{RunOpts: opts.RunOpts, Only: "execute", PacketID: "M01-T01"})
	require.ErrorIs(t, err, ErrInterrupted)

	c := unitTestContext(t, fx)
	c.RunContext = ctx
	p := c.Packets["M01-T01"]
	assert.False(t, c.awaitCredentialsApproval(p).OK)
}

func TestRunContextInterruptsSleepAndSignalHandler(t *testing.T) {
	fx := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: mustConfig(t, fx), Context: ctx})
	require.NoError(t, err)
	c.Sleep(time.Nanosecond)
	sig := registerSigterm(func(string) {}, cancel)
	sig <- syscall.SIGTERM // 不向测试进程发 OS 信号，走同一 handler 生命周期。
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("signal did not cancel run context")
	}
	c.Sleep(time.Hour)
	unregisterSigterm(sig)
}

func TestDriverCancellationCannotBecomeSuccess(t *testing.T) {
	r := drivers.MockResult(func(r *drivers.Result) { r.Cancelled = true; r.CleanupError = "group still alive" })
	sr := toSpawnResult(r)
	assert.Equal(t, "cancelled", sr.Signal)
	require.ErrorContains(t, sr.SpawnError, "group still alive")
}

func TestReportsStayWithinWorkspaceAndReadFailuresSurface(t *testing.T) {
	dir := t.TempDir()
	_, err := readUnitReport(filepath.Join(dir, "missing"), "report.json")
	require.Error(t, err)
	_, err = readUnitReport(dir, "missing.json")
	require.Error(t, err)
	outside := filepath.Join(t.TempDir(), "secret.json")
	writeFile(t, outside, `{"secret":"never collect"}`)
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "link.json")))
	_, err = readUnitReport(dir, "link.json")
	require.ErrorContains(t, err, "regular file")
	writeFile(t, filepath.Join(dir, "huge.json"), strings.Repeat("x", (1<<20)+1))
	_, err = readUnitReport(dir, "huge.json")
	require.ErrorContains(t, err, "1 MiB")
	writeFile(t, filepath.Join(dir, "ok.json"), "{}")
	orig := openReportFile
	t.Cleanup(func() { openReportFile = orig })
	openReportFile = func(*os.Root, string) (io.ReadCloser, error) { return nil, errors.New("open failed") }
	_, err = readUnitReport(dir, "ok.json")
	require.ErrorContains(t, err, "open failed")
	openReportFile = func(*os.Root, string) (io.ReadCloser, error) { return brokenSnapshotReader{}, nil }
	_, err = readUnitReport(dir, "ok.json")
	require.ErrorContains(t, err, "disk read failed")
}

func TestReportCollectionFailureDoesNotAdvanceExecutor(t *testing.T) {
	for _, failAt := range []string{"mkdir", "write", "remove"} {
		t.Run(failAt, func(t *testing.T) {
			fx := newGitFixture(t)
			ctx := unitTestContext(t, fx)
			sd := happyDrivers()
			sd.execFn = func(a drivers.CallArgs) drivers.Result {
				r := sd.mock.Execute(a)
				if failAt == "mkdir" {
					writeFile(t, filepath.Join(fx.taskDir, "reports"), "obstruction")
				}
				return r
			}
			ctx.Drivers = sd.set()
			ow, or := writeCollectedReport, removeCollectedReport
			t.Cleanup(func() { writeCollectedReport = ow; removeCollectedReport = or })
			if failAt == "write" {
				writeCollectedReport = func(string, []byte, os.FileMode) error { return errors.New("disk full") }
			}
			if failAt == "remove" {
				removeCollectedReport = func(string) error { return errors.New("remove failed") }
			}
			p := ctx.Packets["M01-T01"]
			_, err := ctx.executeStep(p)
			require.Error(t, err)
			assert.Nil(t, ctx.State.Packets[p.PacketID].Evidence.Executor)
			assert.FileExists(t, filepath.Join(ctx.unitCwd(p), "build", p.PacketID+".txt"))
		})
	}
}

func TestPIDWriteIsAtomicAndRunRejectsUnobservablePID(t *testing.T) {
	fx := newFixture(t)
	ctx := unitTestContext(t, fx)
	require.NoError(t, ctx.writePid())
	before, err := os.ReadFile(PidFile(fx.taskDir))
	require.NoError(t, err)
	orig := writePIDData
	t.Cleanup(func() { writePIDData = orig })
	writePIDData = func(string, []byte, os.FileMode) error { return errors.New("pid disk full") }
	require.ErrorContains(t, ctx.writePid(), "pid disk full")
	after, err := os.ReadFile(PidFile(fx.taskDir))
	require.NoError(t, err)
	assert.Equal(t, before, after)
	clearPid(fx.taskDir)
	_, err = RunTask(runOpts(fx, happyDrivers().set()))
	require.ErrorContains(t, err, "pid disk full")
	_, err = RunSingleStep(stepOpts(fx, happyDrivers().set()))
	require.ErrorContains(t, err, "pid disk full")
	writePIDData = orig
	ctx.TaskDir = filepath.Join(fx.root, "file")
	writeFile(t, ctx.TaskDir, "x")
	require.Error(t, ctx.writePid())
	ctx.TaskDir = fx.taskDir
	oldCreate := createPIDTemp
	t.Cleanup(func() { createPIDTemp = oldCreate })
	createPIDTemp = func(string, string) (*os.File, error) { return nil, errors.New("temp allocation failed") }
	require.ErrorContains(t, ctx.writePid(), "temp allocation failed")
}

func TestResidentDisabledClearsSessionAndPhysicalFailuresAreNotPlanSuccess(t *testing.T) {
	fx := newFixture(t)
	ctx := unitTestContext(t, fx)
	b := ctx.Config.Loops["plan"]
	b.ResidentSession = false
	ctx.Config.Loops["plan"] = b
	ctx.State.PlanSession = map[string]any{"session-id": "old"}
	r, err := ctx.planStep(ctx.Spec.Milestones[0], "")
	require.NoError(t, err)
	require.True(t, r.OK)
	assert.Nil(t, ctx.State.PlanSession)
	for _, fail := range []string{"cancel", "cleanup"} {
		sd := happyDrivers()
		sd.planFn = func(drivers.CallArgs) drivers.Result {
			return drivers.MockResult(func(r *drivers.Result) {
				if fail == "cancel" {
					r.Cancelled = true
				} else {
					r.CleanupError = "not quiescent"
				}
			})
		}
		ctx.Drivers = sd.set()
		r, err = ctx.planStep(ctx.Spec.Milestones[0], "")
		require.NoError(t, err)
		assert.False(t, r.OK)
	}
}

func TestMissingUsageIsExplicitAndRepeatedWarningsAreBounded(t *testing.T) {
	c := unitTestContext(t)
	for range 2 {
		c.recordSpawnMetrics(spawnMetricsArgs{role: "plan", result: &drivers.Result{Harness: "kimi"}})
	}
	assert.Equal(t, int64(2), c.State.Budget.UnknownUsageCalls)
	assert.Equal(t, int64(2), c.State.Budget.UnknownCostCalls)
	assert.Len(t, c.State.Budget.Warnings, 1)
	assert.Equal(t, int64(2), c.State.Budget.Daily[c.Now().UTC().Format("2006-01-02")].UnknownUsageCalls)
}

// 随下面的文件系统失败回归使用，模拟块设备在打开后返回 I/O 错误。
type brokenSnapshotReader struct{}

func (brokenSnapshotReader) Read([]byte) (int, error) { return 0, errors.New("disk read failed") }
func (brokenSnapshotReader) Close() error             { return nil }

var _ io.ReadCloser = brokenSnapshotReader{}
