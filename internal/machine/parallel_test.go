package machine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func parallelFixture(t *testing.T) (fixture, *config.Config) {
	fx := newGitFixture(t)
	for _, id := range []string{"M01-T01", "M01-T02"} {
		file := filepath.Join(fx.taskDir, "task-packet-"+id+".md")
		raw, err := os.ReadFile(file)
		require.NoError(t, err)
		raw = []byte(strings.Replace(string(raw), "route: default", "route: default\nschedule:\n  after: []\n  writes: [build/"+id+".txt]", 1))
		writeFile(t, file, string(raw))
	}
	gitCmd(t, fx.root, "add", "-A")
	gitCmd(t, fx.root, "commit", "-q", "-m", "test: freeze independent schedules")
	cfg := configFor(fx)
	cfg.Scheduler = config.SchedulerConfig{Mode: "parallel", MaxParallel: 2}
	return fx, cfg
}

func TestParallelPacketsOverlapThenReviewAndIntegrateTheUnion(t *testing.T) {
	fx, cfg := parallelFixture(t)
	var started, active, peak atomic.Int32
	release := make(chan struct{})
	old := parallelDrivers
	t.Cleanup(func() { parallelDrivers = old })
	parallelDrivers = func(*config.Config) *drivers.Drivers {
		d := happyDrivers()
		base := drivers.NewMockDriver(nil)
		d.execFn = func(a drivers.CallArgs) drivers.Result {
			value := active.Add(1)
			defer active.Add(-1)
			for before := peak.Load(); value > before && !peak.CompareAndSwap(before, value); before = peak.Load() {
			}
			if started.Add(1) == 2 {
				close(release)
			}
			select {
			case <-release:
			case <-time.After(5 * time.Second):
				t.Error("independent executors did not overlap")
			}
			return base.Execute(a)
		}
		return d.set()
	}
	d := happyDrivers()
	base := drivers.NewMockDriver(nil)
	d.reviewFn = func(a drivers.CallArgs) drivers.Result {
		require.Zero(t, active.Load())
		if a.PacketID == "M01-T02" {
			require.FileExists(t, filepath.Join(a.Cwd, "build", "M01-T01.txt"))
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
	for _, id := range []string{"M01-T01", "M01-T02"} {
		require.Equal(t, 1, st.Packets[id].Attempts)
		require.Equal(t, "completed", st.Packets[id].Status)
		require.FileExists(t, filepath.Join(fx.root, "build", id+".txt"))
	}
	require.NotEmpty(t, st.Packets["M01-T02"].EvidenceHistory)
	require.Equal(t, "parallel", st.Scheduler.Mode)
	require.Equal(t, int64(1080), st.Budget.Totals.Tokens, "Plan + two Execute + two Review + Judge, without lost billing")
}

func TestParallelSharedHardBudgetAndProviderLimitsBoundDispatch(t *testing.T) {
	for _, hard := range []bool{false, true} {
		t.Run(map[bool]string{false: "provider", true: "hard-budget"}[hard], func(t *testing.T) {
			fx, cfg := parallelFixture(t)
			if hard {
				cfg.Budget.Enforce = "hard"
			} else {
				cfg.Scheduler.Providers = map[string]int{"opencode-go": 1}
			}
			var active, peak atomic.Int32
			old := parallelDrivers
			t.Cleanup(func() { parallelDrivers = old })
			parallelDrivers = func(*config.Config) *drivers.Drivers {
				d := happyDrivers()
				base := drivers.NewMockDriver(nil)
				d.execFn = func(a drivers.CallArgs) drivers.Result {
					value := active.Add(1)
					defer active.Add(-1)
					for before := peak.Load(); value > before && !peak.CompareAndSwap(before, value); before = peak.Load() {
					}
					time.Sleep(15 * time.Millisecond)
					return base.Execute(a)
				}
				return d.set()
			}
			c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set(), Log: func(string) {}})
			require.NoError(t, err)
			require.Len(t, c.parallelBatch("M-01"), 2)
			require.NoError(t, c.runParallelExecute(c.PacketList))
			require.Equal(t, int32(1), peak.Load())
			require.Equal(t, int64(360), c.State.Budget.Totals.Tokens)
		})
	}
}

func TestParallelScopedReceiptsRemainIndependentAndAccounted(t *testing.T) {
	fx, cfg := parallelFixture(t)
	cfg.Drivers["execute"] = "real"
	cfg.Sessions = config.SessionsConfig{Mode: "scoped", PolicyVersion: 1}
	cfg.Routes["default"] = config.RouteBinding{Harness: "dsh", Provider: "test", Channel: "test", Model: "test-model"}
	cfg.Channels["test"] = config.Channel{}
	old := parallelDrivers
	t.Cleanup(func() { parallelDrivers = old })
	parallelDrivers = func(*config.Config) *drivers.Drivers {
		d := happyDrivers()
		base := drivers.NewMockDriver(nil)
		d.execFn = func(a drivers.CallArgs) drivers.Result { return sessionResult(a, base.Execute(a)) }
		return d.set()
	}
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set(), Log: func(string) {}})
	require.NoError(t, err)
	require.NoError(t, c.runParallelExecute(c.PacketList))
	st, err := state.Load(fx.taskDir)
	require.NoError(t, err)
	require.Len(t, st.Sessions.Records, 2)
	require.NotEqual(t, st.Sessions.Records["execute/M01-T01"].ThreadID, st.Sessions.Records["execute/M01-T02"].ThreadID)
	for _, r := range st.Sessions.Records {
		require.True(t, r.Last.Accounted)
		require.Equal(t, "consumed", r.Last.Status)
	}
	require.Equal(t, int64(360), st.Budget.Totals.Tokens)
}

func TestParallelCancelWaitsForEveryStartedExecutor(t *testing.T) {
	fx, cfg := parallelFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var started, ended atomic.Int32
	both := make(chan struct{})
	var once sync.Once
	old := parallelDrivers
	t.Cleanup(func() { parallelDrivers = old })
	parallelDrivers = func(*config.Config) *drivers.Drivers {
		d := happyDrivers()
		d.execFn = func(a drivers.CallArgs) drivers.Result {
			defer ended.Add(1)
			if started.Add(1) == 2 {
				once.Do(func() { close(both) })
			}
			<-a.Watchdog.Context.Done()
			return drivers.Result{Cancelled: true}
		}
		return d.set()
	}
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set(), Log: func(string) {}, Context: ctx})
	require.NoError(t, err)
	c.State.Status = "running"
	done := make(chan error, 1)
	go func() { done <- c.runParallelExecute(c.PacketList) }()
	select {
	case <-both:
	case <-time.After(5 * time.Second):
		t.Fatal("workers did not start")
	}
	cancel()
	require.Error(t, <-done)
	require.Equal(t, int32(2), ended.Load())
}

func TestParallelHardBudgetAccountsBeforeAdmittingTheNextCall(t *testing.T) {
	fx, cfg := parallelFixture(t)
	cfg.Budget.Enforce = "hard"
	limit := int64(1)
	cfg.Budget.DailyTokenLimit = &limit
	var calls atomic.Int32
	old := parallelDrivers
	t.Cleanup(func() { parallelDrivers = old })
	parallelDrivers = func(*config.Config) *drivers.Drivers {
		d := happyDrivers()
		base := drivers.NewMockDriver(nil)
		d.execFn = func(a drivers.CallArgs) drivers.Result { calls.Add(1); return base.Execute(a) }
		return d.set()
	}
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set(), Log: func(string) {}})
	require.NoError(t, err)
	require.Error(t, c.runParallelExecute(c.PacketList))
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, int64(180), c.State.Budget.Totals.Tokens)
	require.Equal(t, "budget-stopped", c.State.Status)
}

func TestFullSchedulerRefusesUnsafeParallelismAndPreservesFailedBatch(t *testing.T) {
	for _, kind := range []string{"adapter", "bounded", "integration"} {
		t.Run(kind, func(t *testing.T) {
			fx, cfg := parallelFixture(t)
			d := happyDrivers()
			opts := runOpts(fx, d.set())
			opts.Config = cfg
			old := parallelDrivers
			t.Cleanup(func() { parallelDrivers = old })
			switch kind {
			case "adapter":
				cfg.Drivers["execute"] = "real"
			case "bounded":
				file := filepath.Join(fx.taskDir, "task-packet-M01-T02.md")
				raw, err := os.ReadFile(file)
				require.NoError(t, err)
				writeFile(t, file, strings.Replace(string(raw), "max-retries: 3, on-fail: return-to-planner", "max-retries: 1, on-fail: inbox", 1))
				gitCmd(t, fx.root, "add", "-A")
				gitCmd(t, fx.root, "commit", "-q", "-m", "test: bounded independent failure")
				parallelDrivers = func(*config.Config) *drivers.Drivers {
					unit := happyDrivers()
					mock := drivers.NewMockDriver(nil)
					unit.execFn = func(a drivers.CallArgs) drivers.Result {
						if a.PacketID == "M01-T02" {
							return drivers.MockResult(func(r *drivers.Result) { r.ExitCode = new(1) })
						}
						return mock.Execute(a)
					}
					return unit.set()
				}
			case "integration":
				file := filepath.Join(fx.taskDir, "task-packet-M01-T02.md")
				raw, err := os.ReadFile(file)
				require.NoError(t, err)
				writeFile(t, file, strings.Replace(string(raw), "test -f build/M01-T02.txt", "test -f build/M01-T02.txt && test ! -f build/M01-T01.txt", 1))
				gitCmd(t, fx.root, "add", "-A")
				gitCmd(t, fx.root, "commit", "-q", "-m", "test: combined candidate oracle")
			}
			summary, err := RunTask(opts)
			if kind == "bounded" {
				require.NoError(t, err)
				require.Equal(t, "escalated", summary.Status)
				require.NoFileExists(t, filepath.Join(fx.root, "build", "M01-T01.txt"))
				require.Empty(t, d.log.review)
			} else {
				require.Error(t, err)
			}
		})
	}
}
