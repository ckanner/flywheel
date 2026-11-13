package machine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readyQueueFixture(t *testing.T) (fixture, *config.Config) {
	fx, cfg := parallelFixture(t)
	id := "M01-T03"
	packet := strings.Replace(fixturePacket(id, "R-02"), "route: default", "route: default\nschedule:\n  after: []\n  writes: [build/"+id+".txt]", 1)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-"+id+".md"), packet)
	gitCmd(t, fx.root, "add", "-A")
	gitCmd(t, fx.root, "commit", "-q", "-m", "test: queue a third independent packet")
	cfg.Scheduler.Dispatch = "ready-queue"
	return fx, cfg
}

func assertNotAdmitted(t *testing.T, c *Context, id string) {
	t.Helper()
	ps := c.State.Packets[id]
	require.Zero(t, ps.Attempts)
	require.Nil(t, ps.BaseRef)
	require.Nil(t, ps.Worktree)
	require.False(t, ps.Parallel)
	require.Equal(t, "pending", ps.Status)
	require.Nil(t, ps.LastFeedback)
	require.Nil(t, ps.ExecutionPolicy)
	require.Nil(t, c.sessionRecord("execute", id))
}

func TestReadyQueueRefillsAfterL1WithoutWaitingForSlowSibling(t *testing.T) {
	fx, cfg := readyQueueFixture(t)
	thirdStarted := make(chan struct{})
	var active, peak atomic.Int32
	base := drivers.NewMockDriver(nil)
	previous := parallelDrivers
	t.Cleanup(func() { parallelDrivers = previous })
	parallelDrivers = func(*config.Config) *drivers.Drivers {
		d := happyDrivers()
		d.execFn = func(a drivers.CallArgs) drivers.Result {
			value := active.Add(1)
			defer active.Add(-1)
			for before := peak.Load(); value > before && !peak.CompareAndSwap(before, value); before = peak.Load() {
			}
			switch a.PacketID {
			case "M01-T01":
				select {
				case <-thirdStarted:
				case <-time.After(10 * time.Second):
					t.Error("a free execution lane waited for its slow sibling")
				}
			case "M01-T03":
				st, err := state.Load(fx.taskDir)
				if assert.NoError(t, err) {
					assert.Equal(t, "l1-passed", st.Packets["M01-T02"].Status, "a replacement waits for the previous lane's deterministic verification")
				}
				close(thirdStarted)
			}
			return base.Execute(a)
		}
		return d.set()
	}
	d := happyDrivers()
	d.reviewFn = func(a drivers.CallArgs) drivers.Result {
		assert.Zero(t, active.Load(), "Review still waits for the active cohort to quiesce")
		if a.PacketID == "M01-T03" {
			assert.FileExists(t, filepath.Join(a.Cwd, "build", "M01-T01.txt"))
			assert.FileExists(t, filepath.Join(a.Cwd, "build", "M01-T02.txt"))
		}
		return base.Review(a)
	}
	opts := runOpts(fx, d.set())
	opts.Config = cfg
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	require.Equal(t, int32(2), peak.Load())
	st := loadFixtureState(t, fx)
	for _, id := range []string{"M01-T01", "M01-T02", "M01-T03"} {
		require.Equal(t, 1, st.Packets[id].Attempts)
		require.Equal(t, "completed", st.Packets[id].Status)
	}
	require.NotEmpty(t, st.Packets["M01-T03"].EvidenceHistory, "delivery rechecks the integrated candidate")
	require.Equal(t, int64(1440), st.Budget.Totals.Tokens)
}

func TestReadyQueueEligibilityPreservesDependenciesAndModeBoundaries(t *testing.T) {
	fx, cfg := readyQueueFixture(t)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set()})
	require.NoError(t, err)
	require.Len(t, c.parallelBatch("M-01"), 3)
	cfg.Scheduler.Dispatch = "batch"
	require.Len(t, c.parallelBatch("M-01"), 2)
	cfg.Scheduler.Dispatch = "ready-queue"
	off := false
	cfg.Worktree.Enabled = &off
	require.False(t, c.readyQueueEnabled())
	require.Len(t, c.parallelBatch("M-01"), 2, "shared candidates retain the original bounded batch")
	cfg.Worktree.Enabled = nil
	cfg.Scheduler.MaxParallel = 1
	require.Empty(t, c.parallelBatch("M-01"))
	cfg.Scheduler.MaxParallel = 2
	cfg.Scheduler.Mode = "serial"
	require.Empty(t, c.parallelBatch("M-01"))
	cfg.Scheduler.Mode = "parallel"
	c.State.Workflow.Effective = "quick"
	require.Empty(t, c.parallelBatch("M-01"))
	c.State.Workflow.Effective = "full"
	third := c.PacketList[2]
	third.Schedule.After = []string{"M01-T01"}
	require.Len(t, c.parallelBatch("M-01"), 2)
	c.State.Packets["M01-T01"].Status = "l1-passed"
	require.Empty(t, c.parallelBatch("M-01"), "L1 does not deliver a dependency")
	c.State.Packets["M01-T01"].Status = "completed"
	require.Equal(t, []*contract.Packet{c.PacketList[1], third}, c.parallelBatch("M-01"))
	c.State.Packets["M01-T01"].Status = "pending"
	third.Schedule.After = nil
	third.Schedule.Writes = []string{"build/"}
	require.Len(t, c.parallelBatch("M-01"), 2)
	third.Schedule = nil
	require.Len(t, c.parallelBatch("M-01"), 2)
}

func TestReadyQueueBoundedLaneLeavesUnadmittedContractsUntouched(t *testing.T) {
	fx, cfg := readyQueueFixture(t)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set(), Log: func(string) {}})
	require.NoError(t, err)
	enableExecutionPolicy(c)
	c.Config.Worktree.Enabled = nil
	first := c.PacketList[0]
	c.State.Packets[first.PacketID].FailedAttempts = first.Escalation.MaxRetries
	started, err := c.runParallelExecute(c.parallelBatch("M-01"))
	require.NoError(t, err)
	require.Equal(t, c.PacketList[:2], started)
	assertNotAdmitted(t, c, "M01-T03")
	_, path := verify.WorktreeNames(c.Cwd, c.State.Slug, "M01-T03")
	require.NoDirExists(t, path)
	stopped, err := c.recoverParallelFailures(started, "")
	require.NoError(t, err)
	require.False(t, stopped)
	assertNotAdmitted(t, c, "M01-T03")
}

func TestReadyQueueReplanRebuildsTheUnadmittedSelection(t *testing.T) {
	fx, cfg := readyQueueFixture(t)
	base := drivers.NewMockDriver(nil)
	var revised atomic.Bool
	execute := func(a drivers.CallArgs) drivers.Result {
		if a.PacketID == "M01-T03" {
			assert.True(t, revised.Load(), "a bounded sibling stops admission under the old contract")
			assert.Contains(t, a.Prompt, "Use the revised queue contract")
			st, err := state.Load(fx.taskDir)
			if assert.NoError(t, err) {
				assert.Equal(t, "completed", st.Packets["M01-T01"].Status, "unadmitted work is reselected after bounded recovery, not executed as an old cohort member")
			}
		}
		return base.Execute(a)
	}
	d := happyDrivers()
	d.execFn = execute
	d.planFn = func(a drivers.CallArgs) drivers.Result {
		if strings.Contains(a.Prompt, "Returned packet M01-T01") {
			path := filepath.Join(fx.taskDir, "task-packet-M01-T03.md")
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			updated := strings.Replace(string(raw), "contract-version: 1", "contract-version: 2", 1)
			updated = strings.Replace(updated, "Make M01-T03 produce its artifact.", "Use the revised queue contract", 1)
			writeFile(t, path, updated)
			revised.Store(true)
		}
		return base.Plan(a)
	}
	previous := parallelDrivers
	t.Cleanup(func() { parallelDrivers = previous })
	parallelDrivers = func(*config.Config) *drivers.Drivers {
		worker := happyDrivers()
		worker.execFn = execute
		return worker.set()
	}
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: d.set(), Log: func(string) {}})
	require.NoError(t, err)
	c.State.Packets["M01-T01"].FailedAttempts = c.PacketList[0].Escalation.MaxRetries
	summary, err := c.run()
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	require.True(t, revised.Load())
	require.Equal(t, 1, c.State.Packets["M01-T03"].Attempts)
}

func TestReadyQueuePreparedWorkspaceResumesExecuteAfterEarlierDelivery(t *testing.T) {
	fx, cfg := readyQueueFixture(t)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set(), Log: func(string) {}})
	require.NoError(t, err)
	_, err = c.runParallelExecute(c.PacketList[:2])
	require.NoError(t, err)
	head, ok := verify.GetBaseRef(c.Cwd)
	require.True(t, ok)
	require.NoError(t, c.prepareParallelPacket(c.PacketList[2], head))
	for _, packet := range c.PacketList[:2] {
		require.NoError(t, c.prepareParallelIntegration(packet))
		outcome, err := c.processPacket(packet, processOpts{})
		require.NoError(t, err)
		require.Equal(t, "ok", outcome)
	}
	prepared := c.State.Packets["M01-T03"]
	require.Zero(t, prepared.Attempts)
	require.Nil(t, prepared.Evidence.Executor)
	require.Nil(t, c.sessionRecord("execute", "M01-T03"))

	// Reload the durable preparation, as after cancellation before dispatch.
	// Its old baseline must first absorb both deliveries, then Execute/L1 run.
	opts := runOpts(fx, happyDrivers().set())
	opts.Config = cfg
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	st := loadFixtureState(t, fx)
	for _, packet := range st.Packets {
		require.Equal(t, 1, packet.Attempts)
		require.Equal(t, "completed", packet.Status)
	}
	require.NotEmpty(t, st.Packets["M01-T03"].EvidenceHistory)
}

func TestReadyQueueCancellationWaitsForCleanupAndDoesNotRefill(t *testing.T) {
	fx, cfg := readyQueueFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls, ended atomic.Int32
	both := make(chan struct{})
	cleaning := make(chan struct{})
	cleanup := make(chan struct{})
	previous := parallelDrivers
	t.Cleanup(func() { parallelDrivers = previous })
	parallelDrivers = func(*config.Config) *drivers.Drivers {
		d := happyDrivers()
		d.execFn = func(a drivers.CallArgs) drivers.Result {
			defer ended.Add(1)
			if calls.Add(1) == 2 {
				close(both)
			}
			<-a.Watchdog.Context.Done()
			if a.PacketID == "M01-T02" {
				close(cleaning)
				<-cleanup
			}
			return drivers.Result{Cancelled: true}
		}
		return d.set()
	}
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set(), Log: func(string) {}, Context: ctx})
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		_, err := c.runParallelExecute(c.PacketList)
		done <- err
	}()
	select {
	case <-both:
	case <-time.After(10 * time.Second):
		cancel()
		close(cleanup)
		t.Fatal("initial queue lanes did not start")
	}
	cancel()
	<-cleaning
	select {
	case err := <-done:
		t.Errorf("queue returned before executor cleanup: %v", err)
	default:
	}
	close(cleanup)
	require.Error(t, <-done)
	require.Equal(t, int32(2), ended.Load())
	require.Equal(t, int32(2), calls.Load())
	assertNotAdmitted(t, c, "M01-T03")
}

func TestReadyQueueLateWorkspaceFailureCancelsAndPreservesStartedWork(t *testing.T) {
	fx, cfg := readyQueueFixture(t)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set(), Log: func(string) {}})
	require.NoError(t, err)
	_, path := verify.WorktreeNames(c.Cwd, c.State.Slug, "M01-T03")
	writeFile(t, path, "retain this obstruction")
	firstStarted := make(chan struct{})
	var stopped atomic.Bool
	base := drivers.NewMockDriver(nil)
	previous := parallelDrivers
	t.Cleanup(func() { parallelDrivers = previous })
	parallelDrivers = func(*config.Config) *drivers.Drivers {
		d := happyDrivers()
		d.execFn = func(a drivers.CallArgs) drivers.Result {
			if a.PacketID == "M01-T01" {
				close(firstStarted)
				<-a.Watchdog.Context.Done()
				stopped.Store(true)
				return drivers.Result{Cancelled: true}
			}
			<-firstStarted
			return base.Execute(a)
		}
		return d.set()
	}
	started, err := c.runParallelExecute(c.PacketList)
	require.ErrorContains(t, err, "parallel worktree unavailable")
	require.True(t, stopped.Load())
	require.Equal(t, c.PacketList[:2], started)
	assertNotAdmitted(t, c, "M01-T03")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "retain this obstruction", string(raw))
	for _, packet := range started {
		require.DirExists(t, c.State.Packets[packet.PacketID].Worktree.Path)
	}
}

func TestReadyQueueStillUsesSharedBudgetAndProviderPermits(t *testing.T) {
	for _, kind := range []string{"provider", "hard", "exhausted"} {
		t.Run(kind, func(t *testing.T) {
			fx, cfg := readyQueueFixture(t)
			if kind == "provider" {
				cfg.Scheduler.Providers = map[string]int{"opencode-go": 1}
			} else {
				cfg.Budget.Enforce = "hard"
				if kind == "exhausted" {
					limit := int64(1)
					cfg.Budget.DailyTokenLimit = &limit
				}
			}
			var active, peak, calls atomic.Int32
			base := drivers.NewMockDriver(nil)
			previous := parallelDrivers
			t.Cleanup(func() { parallelDrivers = previous })
			parallelDrivers = func(*config.Config) *drivers.Drivers {
				d := happyDrivers()
				d.execFn = func(a drivers.CallArgs) drivers.Result {
					calls.Add(1)
					value := active.Add(1)
					defer active.Add(-1)
					for before := peak.Load(); value > before && !peak.CompareAndSwap(before, value); before = peak.Load() {
					}
					return base.Execute(a)
				}
				return d.set()
			}
			c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set(), Log: func(string) {}})
			require.NoError(t, err)
			started, err := c.runParallelExecute(c.PacketList)
			require.Equal(t, int32(1), peak.Load())
			if kind == "exhausted" {
				require.Error(t, err)
				require.Equal(t, int32(1), calls.Load())
				require.Equal(t, int64(180), c.State.Budget.Totals.Tokens)
				require.Equal(t, "budget-stopped", c.State.Status)
				assertNotAdmitted(t, c, "M01-T03")
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.PacketList, started)
			require.Equal(t, int32(3), calls.Load())
			require.Equal(t, int64(540), c.State.Budget.Totals.Tokens)
		})
	}
}

func TestReadyQueueChecksDurableBudgetBeforePreparingAReplacement(t *testing.T) {
	fx, cfg := readyQueueFixture(t)
	cfg.Budget.Enforce = "hard"
	limit := int64(360)
	cfg.Budget.DailyTokenLimit = &limit
	gate := filepath.Join(t.TempDir(), "release-verification")
	t.Setenv("FLYWHEEL_TEST_QUEUE_GATE", gate)
	cfg.Verify.EnvAllowlist = append(cfg.Verify.EnvAllowlist, "FLYWHEEL_TEST_QUEUE_GATE")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set(), Log: func(string) {}, Context: ctx})
	require.NoError(t, err)
	for _, packet := range c.PacketList[:2] {
		packet.Acceptance.L1[0].Cmd += ` && while ! test -f "$FLYWHEEL_TEST_QUEUE_GATE"; do sleep 0.01; done`
	}
	done := make(chan error, 1)
	go func() {
		_, err := c.runParallelExecute(c.PacketList)
		done <- err
	}()
	require.Eventually(t, func() bool {
		st, err := state.Load(fx.taskDir)
		return err == nil && st.Budget.Totals.Tokens == limit
	}, 10*time.Second, 5*time.Millisecond, "both Execute charges must persist while their L1 gates are still running")
	writeFile(t, gate, "release")
	require.ErrorContains(t, <-done, "queue admission stopped by shared budget")
	require.Equal(t, "budget-stopped", c.State.Status)
	assertNotAdmitted(t, c, "M01-T03")
	_, path := verify.WorktreeNames(c.Cwd, c.State.Slug, "M01-T03")
	require.NoDirExists(t, path)
}
