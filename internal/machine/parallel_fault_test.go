package machine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/require"
)

func laneFixture(t *testing.T) (*Context, *Context, *parallelLane) {
	_, owner := handoffFixture(t)
	owner.Config.Scheduler = config.SchedulerConfig{Mode: "parallel", MaxParallel: 2}
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	group := &parallelGroup{owner: owner, cancel: cancel, slots: make(chan struct{}, 1), providers: map[string]chan struct{}{}, threads: map[string]string{}}
	worker := *owner
	worker.State = jsonClone(owner.State)
	worker.RunContext = context.Background()
	worker.State.Packets["M01-T01"].Worktree = &state.WorktreeInfo{Mode: "worktree", Path: t.TempDir()}
	lane := &parallelLane{group: group, id: "M01-T01", inbox: len(worker.State.Inbox)}
	worker.parallel = lane
	return owner, &worker, lane
}

func TestParallelEligibilityRequiresActualDeclaredIndependence(t *testing.T) {
	fx, cfg := parallelFixture(t)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set()})
	require.NoError(t, err)
	require.Len(t, c.parallelBatch("M-01"), 2)
	require.Empty(t, c.parallelBatch("M-02"))
	cfg.Scheduler.Mode = "serial"
	require.Empty(t, c.parallelBatch("M-01"))
	cfg.Scheduler.Mode = "parallel"
	c.State.Workflow.Effective = "quick"
	require.Empty(t, c.parallelBatch("M-01"))
	c.State.Workflow.Effective = "full"
	cfg.Scheduler.MaxParallel = 1
	require.Empty(t, c.parallelBatch("M-01"))
	cfg.Scheduler.MaxParallel = 2
	p1, p2 := c.PacketList[0], c.PacketList[1]
	p2.Schedule.After = []string{p1.PacketID}
	require.Empty(t, c.parallelBatch("M-01"))
	p2.Schedule.After = nil
	original := p2.Schedule.Writes
	for _, scope := range []string{"build/", "build/M01-T01.txt"} {
		p2.Schedule.Writes = []string{scope}
		require.Empty(t, c.parallelBatch("M-01"))
	}
	p2.Schedule.Writes = original
	c.State.Packets[p2.PacketID].Status = "l1-passed"
	require.Empty(t, c.parallelBatch("M-01"))
	c.State.Packets[p1.PacketID].Status = "l1-passed"
	require.Empty(t, c.parallelBatch("M-01"))
}

func TestParallelCoordinatorPreservesOwnershipAndDurableCharges(t *testing.T) {
	owner, worker, lane := laneFixture(t)
	r := worker.State.Sessions.Records["execute/M01-T01"]
	r.Last.Accounted = false
	worker.State.Inbox = append(worker.State.Inbox, map[string]any{"reason": "parallel observation"})
	at := owner.nowISO()
	worker.State.LastProgressAt = &at
	owner.State.LastProgressAt = nil
	lane.save(worker)
	require.NoError(t, worker.err())
	require.True(t, owner.State.Sessions.Records["execute/M01-T01"].Last.Accounted, "an old private view cannot undo an already durable charge")
	require.Len(t, owner.State.Inbox, 1)
	lane.save(worker)
	require.Len(t, owner.State.Inbox, 1)
	require.NoError(t, lane.reserveThread("codex-pro", "thread"))
	require.NoError(t, lane.reserveThread("codex-pro", "thread"))
	another := &parallelLane{group: lane.group, id: "M01-T02"}
	require.Error(t, another.reserveThread("codex-pro", "thread"))
	owner.State.Sessions.Records["review/M01-T02"] = &state.SessionRecord{Role: "review", Scope: "M01-T02", ThreadID: "review-thread", Last: &state.SessionInvocation{Recovery: &state.SessionRecovery{Harness: "claude-code-pro"}}}
	require.Error(t, lane.reserveThread("claude-code-pro", "review-thread"))
	duplicate := jsonClone(owner.State.Sessions.Records["execute/M01-T01"])
	duplicate.Scope = "M01-T02"
	owner.State.Sessions.Records["execute/M01-T02"] = duplicate
	require.ErrorContains(t, lane.merge(worker), "another packet")
}

func TestParallelFaultsStopAdmissionWithoutLosingAnotherPermit(t *testing.T) {
	t.Run("cancelled permit", func(t *testing.T) {
		_, worker, lane := laneFixture(t)
		release, err := lane.enter(worker, "channel")
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		other := *worker
		other.RunContext = ctx
		_, err = lane.enter(&other, "channel")
		require.ErrorIs(t, err, context.Canceled)
		require.Len(t, lane.group.slots, 1)
		release()
	})
	t.Run("budget", func(t *testing.T) {
		owner, worker, lane := laneFixture(t)
		limit := int64(1)
		owner.Config.Budget.Enforce = "hard"
		owner.Config.Budget.DailyTokenLimit = &limit
		owner.State.Budget.Enforce = "hard"
		owner.State.Budget.DailyTokenLimit = &limit
		owner.State.Budget.Daily[owner.Now().UTC().Format("2006-01-02")] = state.DailyBucket{Tokens: 2}
		_, err := lane.enter(worker, "channel")
		require.ErrorContains(t, err, "shared budget")
		require.Empty(t, lane.group.slots)
	})
	t.Run("lost workspace", func(t *testing.T) {
		_, worker, lane := laneFixture(t)
		worker.State.Packets[lane.id].Worktree = nil
		lane.save(worker)
		require.Error(t, worker.err())
		require.Equal(t, "stop", lane.budget(worker))
	})
	t.Run("unmergeable metrics", func(t *testing.T) {
		_, worker, lane := laneFixture(t)
		worker.State.Packets[lane.id].Worktree = nil
		lane.metrics(worker, spawnMetricsArgs{})
		require.Error(t, worker.err())
	})
	t.Run("metrics IO", func(t *testing.T) {
		owner, worker, lane := laneFixture(t)
		owner.metricsFile = t.TempDir()
		lane.metrics(worker, spawnMetricsArgs{role: "execute", result: &drivers.Result{Usage: &drivers.Usage{Input: 2}}})
		require.Error(t, worker.err())
		require.Equal(t, "stop", lane.budget(worker))
	})
	t.Run("CAS", func(t *testing.T) {
		owner, worker, lane := laneFixture(t)
		owner.revision--
		lane.save(worker)
		require.Error(t, worker.err())
		require.Equal(t, "stop", lane.budget(worker))
	})
}

func TestParallelPreflightNeverFallsBackToSharedExecution(t *testing.T) {
	noGit := unitTestContext(t)
	_, err := noGit.runParallelExecute(noGit.PacketList)
	require.ErrorContains(t, err, "Git checkout")
	for _, kind := range []string{"adapter", "binding", "worktree", "save"} {
		t.Run(kind, func(t *testing.T) {
			fx, cfg := parallelFixture(t)
			c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set()})
			require.NoError(t, err)
			switch kind {
			case "adapter":
				cfg.Drivers["execute"] = "real"
			case "binding":
				delete(cfg.Routes, "default")
			case "save":
				c.revision--
			case "worktree":
				_, path := verify.WorktreeNames(c.Cwd, c.State.Slug, c.PacketList[0].PacketID)
				writeFile(t, path, "preserve")
			}
			_, err = c.runParallelExecute(c.PacketList)
			require.Error(t, err)
			require.Zero(t, c.State.Packets[c.PacketList[0].PacketID].Attempts)
		})
	}
}

func integratedFirstFixture(t *testing.T) (fixture, *Context) {
	fx, cfg := parallelFixture(t)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set()})
	require.NoError(t, err)
	_, err = c.runParallelExecute(c.PacketList)
	require.NoError(t, err)
	result, err := c.reviewStep(c.PacketList[0], nil)
	require.NoError(t, err)
	require.True(t, result.Pass)
	require.NoError(t, c.completeUnit(c.PacketList[0]))
	return fx, c
}

func TestParallelIntegrationPreservesConflictsAndRechecksTheUnion(t *testing.T) {
	for _, kind := range []string{"missing", "commit", "conflict", "acceptance"} {
		t.Run(kind, func(t *testing.T) {
			fx, c := integratedFirstFixture(t)
			packet := c.PacketList[1]
			ps := c.State.Packets[packet.PacketID]
			path := ps.Worktree.Path
			switch kind {
			case "missing":
				ps.Worktree = nil
			case "commit":
				writeFile(t, filepath.Join(fx.root, ".git", "hooks", "pre-commit"), "#!/bin/sh\nexit 1\n")
				require.NoError(t, os.Chmod(filepath.Join(fx.root, ".git", "hooks", "pre-commit"), 0755))
			case "conflict":
				writeFile(t, filepath.Join(fx.root, "build", packet.PacketID+".txt"), "foreign hotfix")
				gitCmd(t, fx.root, "add", "build")
				gitCmd(t, fx.root, "commit", "-q", "-m", "fix: foreign change")
			case "acceptance":
				packet.Acceptance.L1[0].Cmd = "test ! -f build/M01-T01.txt"
			}
			require.Error(t, c.prepareParallelIntegration(packet))
			require.DirExists(t, path)
			require.NotEqual(t, "completed", ps.Status)
			if kind == "acceptance" {
				require.False(t, ps.Evidence.L1.(map[string]any)["pass"].(bool))
			}
		})
	}
	c := unitTestContext(t)
	packet := c.PacketList[0]
	require.NoError(t, c.prepareParallelIntegration(packet))
	c.State.Packets[packet.PacketID].Parallel = true
	require.ErrorContains(t, c.prepareParallelIntegration(packet), "HEAD")
}

func TestParallelFailureRecordsPauseAndPreservesWork(t *testing.T) {
	fx, cfg := parallelFixture(t)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set()})
	require.NoError(t, err)
	c.State.Status = "running"
	old := parallelDrivers
	t.Cleanup(func() { parallelDrivers = old })
	parallelDrivers = func(*config.Config) *drivers.Drivers {
		d := happyDrivers()
		d.execFn = func(drivers.CallArgs) drivers.Result { return drivers.Result{CleanupError: "unknown cleanup"} }
		return d.set()
	}
	_, err = c.runParallelExecute(c.PacketList)
	require.Error(t, err)
	require.Equal(t, "escalated", c.State.Status)
	require.NotNil(t, c.State.Packets[c.PacketList[0].PacketID].LastFeedback)
	require.DirExists(t, c.State.Packets[c.PacketList[0].PacketID].Worktree.Path)
	require.False(t, errors.Is(err, ErrMergeConflict))
}

func TestParallelBudgetAlreadySpentStopsBeforeAnyModelDispatch(t *testing.T) {
	fx, cfg := parallelFixture(t)
	limit := int64(1)
	cfg.Budget.Enforce, cfg.Budget.DailyTokenLimit = "hard", &limit
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set()})
	require.NoError(t, err)
	c.State.Budget.Daily[c.Now().UTC().Format("2006-01-02")] = state.DailyBucket{Tokens: 2}
	old := parallelDrivers
	t.Cleanup(func() { parallelDrivers = old })
	parallelDrivers = func(*config.Config) *drivers.Drivers {
		d := happyDrivers()
		d.execFn = func(drivers.CallArgs) drivers.Result {
			t.Error("exhausted shared budget must not dispatch")
			return drivers.MockResult(nil)
		}
		return d.set()
	}
	_, err = c.runParallelExecute(c.PacketList)
	require.ErrorContains(t, err, "stopped before L1")
	require.Equal(t, "budget-stopped", c.State.Status)
	for _, packet := range c.State.Packets {
		require.Zero(t, packet.Attempts)
	}
}

func TestParallelCollectionFailureDeterministicallyPausesWithoutCompletingPeers(t *testing.T) {
	fx, cfg := parallelFixture(t)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set()})
	require.NoError(t, err)
	c.State.Status = "running"
	old := writeCollectedReport
	t.Cleanup(func() { writeCollectedReport = old })
	writeCollectedReport = func(string, []byte, os.FileMode) error { return errors.New("collection unavailable") }
	_, err = c.runParallelExecute(c.PacketList)
	require.ErrorContains(t, err, "collection unavailable")
	require.Equal(t, "paused", c.State.Status)
	for _, packet := range c.State.Packets {
		require.NotEqual(t, "completed", packet.Status)
		require.DirExists(t, packet.Worktree.Path)
	}
}

func TestParallelRecoveryUsesTheExistingBoundedEscalationBeforeIntegration(t *testing.T) {
	for _, kind := range []string{"recover", "pause", "escalate", "error", "removed"} {
		t.Run(kind, func(t *testing.T) {
			fx, cfg := parallelFixture(t)
			d := happyDrivers()
			c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: d.set(), Log: func(string) {}})
			require.NoError(t, err)
			batch := []*contract.Packet{c.PacketList[0]}
			switch kind {
			case "removed":
				delete(c.Packets, batch[0].PacketID)
			case "error":
				c.fatal = errors.New("storage failed")
			case "escalate":
				c.State.Packets[batch[0].PacketID].Status = "inbox"
			default:
				c.State.Packets[batch[0].PacketID].FailedAttempts = batch[0].Escalation.MaxRetries
			}
			pause := ""
			if kind == "pause" {
				pause = "review"
			}
			stop, err := c.recoverParallelFailures(batch, pause)
			if kind == "error" {
				require.Error(t, err)
				require.True(t, stop)
				return
			}
			require.NoError(t, err)
			if kind == "recover" {
				require.False(t, stop)
				require.Equal(t, 1, c.State.Packets[batch[0].PacketID].PlanRevisions)
				require.Empty(t, d.log.review)
				require.Equal(t, "running", c.State.Status)
			}
			if kind == "pause" || kind == "escalate" {
				require.True(t, stop)
			}
		})
	}
}

func TestParallelInvocationCancellationAndLifecycleOwnershipAreEnforced(t *testing.T) {
	_, worker, lane := laneFixture(t)
	release, err := lane.enter(worker, "test")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	worker.RunContext = ctx
	_, err = worker.invokeScoped("execute", "M01-T01", sessionArgs(worker), func(drivers.CallArgs) drivers.Result { t.Fatal("cancelled permit dispatched"); return drivers.Result{} })
	require.Error(t, err)
	release()
	worker.RunContext = context.Background()
	worker.fatal = errors.New("local admission failed")
	_, err = worker.invokeScoped("execute", "M01-T01", sessionArgs(worker), func(drivers.CallArgs) drivers.Result { t.Fatal("failed admission dispatched"); return drivers.Result{} })
	require.Error(t, err)
	require.Empty(t, lane.group.slots)
	worker.fatal = nil
	record := worker.State.Sessions.Records["execute/M01-T01"]
	args := sessionArgs(worker)
	args.Binding.Harness = "codex-pro"
	require.NoError(t, worker.prepareSessionLifecycle(record, &args))
	thread := "new-native-thread"
	event := drivers.MachineLifecycle{LifecycleVersion: 1, Type: "lifecycle", RequestID: record.Last.ID, RunID: "native-run", Sequence: 1, Event: "thread-ready", ThreadID: &thread, Persistent: record.Last.Persistent, Configuration: json.RawMessage(`{}`)}
	lane.group.threads["codex-pro/"+thread] = "M01-T02"
	require.ErrorContains(t, args.OnLifecycle(event), "already owned")
	delete(lane.group.threads, "codex-pro/"+thread)
	require.NoError(t, args.OnLifecycle(event))
	require.Equal(t, 1, record.Last.Recovery.Sequence)
}

func TestDeclaredWriteScopeIsVerifiedAgainstTheActualDiff(t *testing.T) {
	fx, cfg := parallelFixture(t)
	file := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	writeFile(t, file, strings.Replace(string(raw), "writes: [build/M01-T01.txt]", "writes: [elsewhere/]", 1))
	gitCmd(t, fx.root, "add", "-A")
	gitCmd(t, fx.root, "commit", "-q", "-m", "test: mismatched write scope")
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set(), Log: func(string) {}})
	require.NoError(t, err)
	result, err := c.runExecuteLoop(c.PacketList[0])
	require.NoError(t, err)
	require.False(t, result.Green)
	st, err := state.Load(fx.taskDir)
	require.NoError(t, err)
	evidence := st.Packets["M01-T01"].Evidence.L1.(map[string]any)
	require.Equal(t, false, evidence["pass"])
	violations := evidence["boundary"].(map[string]any)["violations"].([]any)
	require.Contains(t, violations, map[string]any{"type": "schedule-write-scope", "path": "build/M01-T01.txt"})
}
