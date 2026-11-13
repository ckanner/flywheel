package machine

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func trackedReadyQueueFixture(t *testing.T, mode string) (fixture, *config.Config) {
	t.Helper()
	fx, cfg := readyQueueFixture(t)
	cfg.Drivers["execute"] = "real"
	cfg.Sessions = config.SessionsConfig{Mode: mode, PolicyVersion: 1}
	cfg.Routes["default"] = config.RouteBinding{Harness: "dsh", Provider: "test", Channel: "test", Model: "test-model"}
	cfg.Channels["test"] = config.Channel{}
	return fx, cfg
}

func TestReadyQueueTrackedRefillLeavesLiveSiblingReceiptWithItsOwner(t *testing.T) {
	for _, scenario := range []string{"scoped", "fresh", "duplicate-history", "unknown-outcome"} {
		t.Run(scenario, func(t *testing.T) {
			mode := "scoped"
			if scenario == "fresh" {
				mode = "fresh"
			}
			fx, cfg := trackedReadyQueueFixture(t, mode)
			firstStarted, thirdStarted := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			base := drivers.NewMockDriver(nil)
			previous := parallelDrivers
			t.Cleanup(func() { parallelDrivers = previous })
			parallelDrivers = func(*config.Config) *drivers.Drivers {
				d := happyDrivers()
				d.execFn = func(a drivers.CallArgs) drivers.Result {
					calls.Add(1)
					duplicate := ""
					switch a.PacketID {
					case "M01-T01":
						close(firstStarted)
						select {
						case <-thirdStarted:
						case <-a.Watchdog.Context.Done():
							return drivers.Result{Cancelled: true}
						case <-time.After(10 * time.Second):
							t.Error("tracked replacement did not start while the first invocation remained live")
						}
					case "M01-T02":
						<-firstStarted
					case "M01-T03":
						st, err := state.Load(fx.taskDir)
						if assert.NoError(t, err) {
							assert.Equal(t, "intent", st.Sessions.Records["execute/M01-T01"].Last.Status)
							sibling := st.Sessions.Records["execute/M01-T02"]
							assert.Equal(t, "consumed", sibling.Last.Status)
							duplicate = sibling.ThreadID
						}
						close(thirdStarted)
					}
					result := sessionResult(a, base.Execute(a))
					if scenario == "unknown-outcome" && a.PacketID == "M01-T01" {
						result.Session.Outcome = "unknown"
					}
					if scenario == "duplicate-history" && a.PacketID == "M01-T03" {
						result.Session.SessionID = duplicate
					}
					return result
				}
				return d.set()
			}
			c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set(), Log: func(string) {}})
			require.NoError(t, err)
			started, err := c.runParallelExecute(c.PacketList)
			require.Equal(t, c.PacketList, started)
			require.Equal(t, int32(3), calls.Load(), "the third invocation must be admitted before the slow sibling finishes; error: %v", err)
			st := loadFixtureState(t, fx)
			if scenario == "duplicate-history" {
				require.ErrorContains(t, err, "parallel history belongs to another packet")
				require.NotNil(t, c.fatal)
				require.Empty(t, st.Packets["M01-T03"].Commit)
				return
			}
			if scenario == "unknown-outcome" {
				require.ErrorContains(t, err, "requires reconciliation")
				require.Equal(t, "blocked", st.Sessions.Records["execute/M01-T01"].Last.Status)
				opts := runOpts(fx, happyDrivers().set())
				opts.Config = cfg
				cfg.Sessions.Mode, cfg.Scheduler.Dispatch = "fresh", "batch"
				_, err = RunTask(opts)
				require.ErrorContains(t, err, "requires reconciliation", "changing dispatch or history mode cannot hide the coordinator's unresolved receipt")
				require.Equal(t, int32(3), calls.Load())
				return
			}
			require.NoError(t, err)
			require.Equal(t, int64(540), st.Budget.Totals.Tokens)
			require.Len(t, st.Sessions.Records, 3)
			for _, r := range st.Sessions.Records {
				require.Equal(t, "consumed", r.Last.Status)
				require.True(t, r.Last.Accounted)
				require.Equal(t, mode == "scoped", r.Last.Persistent)
			}
		})
	}
}

func TestReadyQueuePrivateLedgerRetainsUnresolvedEarlierInvocation(t *testing.T) {
	fx, cfg := trackedReadyQueueFixture(t, "scoped")
	opts := Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set(), Log: func(string) {}}
	c, err := CreateContext(opts)
	require.NoError(t, err)
	_, err = c.invokeScoped("execute", "M01-T01", sessionArgs(c), func(a drivers.CallArgs) drivers.Result {
		result := sessionResult(a, drivers.MockResult(nil))
		result.Session.Outcome = "unknown"
		return result
	})
	require.ErrorContains(t, err, "requires reconciliation")
	original := loadFixtureState(t, fx).Sessions.Records["execute/M01-T01"].Last.ID

	// Exercise lane snapshots directly, in addition to RunTask's restart check.
	// The old unresolved call remains visible even in a newly created cohort.
	restarted, err := CreateContext(opts)
	require.NoError(t, err)
	previous := parallelDrivers
	t.Cleanup(func() { parallelDrivers = previous })
	var calls atomic.Int32
	parallelDrivers = func(*config.Config) *drivers.Drivers {
		d := happyDrivers()
		d.execFn = func(a drivers.CallArgs) drivers.Result {
			calls.Add(1)
			return sessionResult(a, drivers.MockResult(nil))
		}
		return d.set()
	}
	_, err = restarted.runParallelExecute(restarted.PacketList)
	require.ErrorContains(t, err, "requires reconciliation")
	require.Zero(t, calls.Load(), "an unresolved pre-cohort receipt cannot be removed from a private lane snapshot")
	st := loadFixtureState(t, fx)
	require.Equal(t, original, st.Sessions.Records["execute/M01-T01"].Last.ID)
	require.Equal(t, "blocked", st.Sessions.Records["execute/M01-T01"].Last.Status)
	assertNotAdmitted(t, restarted, "M01-T03")
}

func TestParallelBlockedReceiptStopsSiblingAdmissionBeforeLaneCompletion(t *testing.T) {
	for _, scenario := range []string{"unknown-outcome", "receipt-save-failed"} {
		t.Run(scenario, func(t *testing.T) {
			owner, worker, lane := laneFixture(t)
			runContext, cancel := context.WithCancel(context.Background())
			defer cancel()
			lane.group.cancel = cancel
			worker.RunContext = runContext
			peer := *worker
			peer.State = jsonClone(owner.State)
			peer.parallel = &parallelLane{group: lane.group, id: "M01-T02", inbox: len(peer.State.Inbox)}
			peer.State.Packets["M01-T02"].Worktree = &state.WorktreeInfo{Mode: "worktree", Path: t.TempDir()}
			defer peer.parallel.finishTurn()
			previous := persistSessionReceipt
			t.Cleanup(func() { persistSessionReceipt = previous })
			if scenario == "receipt-save-failed" {
				persistSessionReceipt = func(string, string, state.SessionReceipt) (string, error) { return "", os.ErrPermission }
			}

			// invokeScoped publishes an unresolved outcome before its caller's
			// Execute loop returns. Peer admission must already be stopped at
			// this boundary, despite its legitimate pre-cohort private ledger.
			_, err := worker.invokeScoped("execute", "M01-T01", sessionArgs(worker), func(a drivers.CallArgs) drivers.Result {
				result := sessionResult(a, drivers.MockResult(nil))
				if scenario == "unknown-outcome" {
					result.Session.Outcome = "unknown"
				}
				return result
			})
			require.ErrorContains(t, err, "requires reconciliation")
			recorded := owner.State.Sessions.Records["execute/M01-T01"]
			if scenario == "unknown-outcome" {
				require.Equal(t, "blocked", recorded.Last.Status)
				require.True(t, recorded.Last.Accounted, "stopping admission must still account the terminal receipt")
			} else {
				require.Equal(t, "intent", recorded.Last.Status, "an unsaved receipt cannot fabricate a confirmed terminal")
				require.Equal(t, "wait", recorded.Action)
				require.False(t, recorded.Last.Accounted)
			}
			persistSessionReceipt = previous

			args := sessionArgs(&peer)
			args.PacketID = "M01-T02"
			calls := 0
			_, err = peer.invokeScoped("execute", args.PacketID, args, func(a drivers.CallArgs) drivers.Result {
				calls++
				return sessionResult(a, drivers.MockResult(nil))
			})
			require.Zero(t, calls, "a known blocked sibling must prevent a new backend dispatch before goroutine completion")
			require.Error(t, err)
		})
	}
}
