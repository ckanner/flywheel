package machine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/metrics"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

type preparedReviewStub struct {
	id       string
	matches  bool
	closes   int
	closeErr error
	closeFn  func()
}

func (s *preparedReviewStub) ID() string                   { return s.id }
func (s *preparedReviewStub) Matches(drivers.DSHArgs) bool { return s.matches }
func (s *preparedReviewStub) Run(drivers.DSHArgs) drivers.Result {
	panic("test driver owns formal results")
}
func (s *preparedReviewStub) Close() error {
	s.closes++
	if s.closeFn != nil {
		s.closeFn()
	}
	return s.closeErr
}

func prewarmFixture(t *testing.T) (fixture, *Context) {
	t.Helper()
	fx, c := sessionFixture(t)
	c.Config.Scheduler.Prewarm = true
	c.prewarmReview = true
	return fx, c
}

func stubReviewPrewarm(t *testing.T, create func(drivers.DSHArgs) drivers.PreparedDSH) {
	t.Helper()
	previous := prepareReviewDSH
	prepareReviewDSH = create
	t.Cleanup(func() { prepareReviewDSH = previous })
}

func startStubReview(t *testing.T, c *Context) *preparedReviewStub {
	t.Helper()
	s := &preparedReviewStub{matches: true}
	stubReviewPrewarm(t, func(a drivers.DSHArgs) drivers.PreparedDSH {
		s.id = a.PrewarmID
		disk, err := state.Load(c.TaskDir)
		require.NoError(t, err)
		require.Equal(t, s.id, disk.WorkerPrewarm.ID)
		require.Equal(t, 3, disk.Sessions.Version)
		require.Empty(t, a.Prompt)
		require.Empty(t, a.RequestID)
		require.Empty(t, a.SessionID)
		return s
	})
	require.NoError(t, c.beginReviewPrewarm(c.PacketList[0]))
	require.NotNil(t, c.prewarm)
	return s
}

func TestPrewarmFullAndQuickHandOffOnlyAfterVerificationAndDurableIntent(t *testing.T) {
	for _, quick := range []bool{false, true} {
		t.Run(fmt.Sprint(quick), func(t *testing.T) {
			fx := newGitFixture(t)
			if quick {
				fx = newQuickFixture(t, true)
			}
			sd := quickDrivers()
			prepared := map[string]*preparedReviewStub{}
			stubReviewPrewarm(t, func(a drivers.DSHArgs) drivers.PreparedDSH {
				disk, err := state.Load(fx.taskDir)
				require.NoError(t, err)
				record := disk.WorkerPrewarm
				require.Equal(t, a.PrewarmID, record.ID)
				require.Equal(t, 3, disk.Sessions.Version)
				packet := strings.TrimPrefix(record.Scope, "final:")
				require.Equal(t, "executed", disk.Packets[packet].Status, "startup overlaps L1, never Execute")
				require.Nil(t, disk.Packets[packet].Evidence.L1)
				require.Empty(t, a.Prompt)
				require.Empty(t, a.SessionID)
				value := &preparedReviewStub{id: a.PrewarmID, matches: true}
				prepared[packet] = value
				return value
			})
			sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
				require.NotNil(t, a.Prewarm)
				disk, err := state.Load(fx.taskDir)
				require.NoError(t, err)
				require.Nil(t, disk.WorkerPrewarm)
				for key, record := range disk.Sessions.Records {
					if record.Last.ID != a.RequestID {
						continue
					}
					require.Equal(t, "review", record.Role, key)
					require.Equal(t, "intent", record.Last.Status)
					packet := strings.TrimPrefix(record.Scope, "final:")
					require.Equal(t, "l1-passed", disk.Packets[packet].Status)
					require.Equal(t, prepared[packet].ID(), a.Prewarm.ID())
					if quick {
						require.NotNil(t, disk.Packets[packet].Evidence.FinalGate)
					}
				}
				if quick {
					return quickVerdict("pass", true)
				}
				return sd.mock.Review(a)
			}
			opts := sessionRunOpts(fx, sessionDriverSet(sd))
			opts.IndexDir = t.TempDir()
			opts.Config.Scheduler.Prewarm = true
			summary, err := RunTask(opts)
			require.NoError(t, err)
			require.Equal(t, "completed", summary.Status)
			st := loadFixtureState(t, fx)
			require.Nil(t, st.WorkerPrewarm)
			require.Equal(t, 3, st.Sessions.Version)
			require.Len(t, prepared, len(sd.log.review))
			for _, handle := range prepared {
				require.Equal(t, 1, handle.closes)
			}
			report, err := metrics.ReadReport(filepath.Join(fx.taskDir, "metrics.jsonl"))
			require.NoError(t, err)
			phases := 0
			for _, span := range report.Timeline.Intervals {
				if span.Name == "worker-prewarm" {
					phases++
					require.NotNil(t, span.EndedAt)
				}
			}
			require.Equal(t, len(prepared), phases)
			require.Equal(t, len(sd.log.plan)+len(sd.log.exec)+len(sd.log.review)+len(sd.log.judge), report.TotalCalls,
				"prewarm spans are not model calls or billable zero-usage entries")
		})
	}
}

func TestPrewarmEligibilityDoesNotStartUnsupportedOrUnusedWorkers(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Context)
	}{
		{"disabled", func(c *Context) { c.Config.Scheduler.Prewarm = false }},
		{"manual-execute-only", func(c *Context) { c.prewarmReview = false }},
		{"parallel-lane", func(c *Context) { c.parallel = &parallelLane{} }},
		{"shared-candidate", func(c *Context) { c.State.SharedExecution = &state.SharedExecution{} }},
		{"mock", func(c *Context) { c.Config.Drivers["review"] = "mock" }},
		{"unsupported", func(c *Context) { b := c.Config.Loops["review"]; b.Harness = "codex-pro"; c.Config.Loops["review"] = b }},
		{"missing-key", func(c *Context) {
			b := c.Config.Loops["review"]
			ch := c.Config.Channels[b.Channel]
			key := "MISSING_PREWARM_KEY"
			ch.EnvKey = &key
			c.Config.Channels[b.Channel] = ch
			c.EnvLookup = func(string) (string, bool) { return "", false }
		}},
		{"budget", func(c *Context) {
			limit := int64(1)
			c.Config.Budget.Enforce = "hard"
			c.Config.Budget.DailyTokenLimit = &limit
			c.State.Budget.Daily[c.Now().UTC().Format("2006-01-02")] = state.DailyBucket{Tokens: 1}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, c := prewarmFixture(t)
			stubReviewPrewarm(t, func(drivers.DSHArgs) drivers.PreparedDSH { t.Fatal("unexpected SDK preparation"); return nil })
			test.change(c)
			require.NoError(t, c.beginReviewPrewarm(c.PacketList[0]))
			require.Nil(t, c.prewarm)
			require.Nil(t, c.State.WorkerPrewarm)
		})
	}
	_, c := prewarmFixture(t)
	stub := startStubReview(t, c)
	require.NoError(t, c.beginReviewPrewarm(c.PacketList[0]))
	require.NoError(t, c.reconcileSessionReceipts(), "this live owner can verify without falsely recovering its own warm process")
	require.NoError(t, c.closeReviewPrewarm("unused"))
	require.Equal(t, 1, stub.closes)
	require.NoError(t, c.closeReviewPrewarm("already closed"))
}

func TestPrewarmPreparationErrorsNeverDispatchAndCASFailureHasRecoveryProof(t *testing.T) {
	for _, name := range []string{"cancelled", "pending-session", "binding", "directory", "runtime-binding", "metrics", "cas"} {
		t.Run(name, func(t *testing.T) {
			_, c := prewarmFixture(t)
			stubReviewPrewarm(t, func(drivers.DSHArgs) drivers.PreparedDSH {
				t.Fatal("no SDK may start before admission succeeds")
				return nil
			})
			oldDirectory, oldBinding := makePrewarmDirectory, reviewWarmBinding
			t.Cleanup(func() { makePrewarmDirectory, reviewWarmBinding = oldDirectory, oldBinding })
			switch name {
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.RunContext = ctx
			case "pending-session":
				c.State.PlanSession = map[string]any{"outcome-unknown": true}
			case "binding":
				delete(c.Config.Loops, "review")
			case "directory":
				makePrewarmDirectory = func(string, os.FileMode) error { return errors.New("mkdir failed") }
			case "runtime-binding":
				reviewWarmBinding = func(drivers.DSHArgs) (string, error) { return "", errors.New("runtime changed") }
			case "metrics":
				c.runID, c.metricsFile = "warm-test", c.TaskDir
			case "cas":
				c.revision--
			}
			err := c.beginReviewPrewarm(c.PacketList[0])
			require.Error(t, err)
			require.Nil(t, c.prewarm)
			if name == "cas" {
				r := c.State.WorkerPrewarm
				require.NotNil(t, r)
				require.NoError(t, drivers.RecoverDSHPrewarm(r.ID, c.prewarmDirectory(r.ID), r.Binding))
			}
		})
	}
}

func TestPrewarmCleanupObligationSurvivesDisableAndRecovery(t *testing.T) {
	_, c := prewarmFixture(t)
	handle := startStubReview(t, c)
	handle.closeErr = errors.New("SDK shutdown not acknowledged")
	require.ErrorContains(t, c.closeReviewPrewarm("unused"), "cleanup unconfirmed")
	require.NotNil(t, c.State.WorkerPrewarm)
	require.NotNil(t, loadFixtureState(t, fixture{taskDir: c.TaskDir}).WorkerPrewarm)
	c.Config.Scheduler.Prewarm = false
	c.fatal = nil
	previous := recoverReviewDSH
	t.Cleanup(func() { recoverReviewDSH = previous })
	recoverReviewDSH = func(string, string, string) error { return errors.New("still alive") }
	require.ErrorContains(t, c.reconcileSessionReceipts(), "still alive")
	require.NotNil(t, c.State.WorkerPrewarm)
	record := *c.State.WorkerPrewarm
	recoverReviewDSH = func(id, directory, binding string) error {
		require.Equal(t, record.ID, id)
		require.Equal(t, c.prewarmDirectory(id), directory)
		require.Equal(t, record.Binding, binding)
		return nil
	}
	require.NoError(t, c.reconcileSessionReceipts())
	require.Nil(t, c.State.WorkerPrewarm)
	require.Equal(t, 3, c.State.Sessions.Version)
	c.State.WorkerPrewarm = &record
	c.revision--
	require.ErrorContains(t, c.reconcileWorkerPrewarm(), "CAS conflict")
}

func TestPrewarmClaimsAreScopeAndChannelBoundAndAlwaysCleaned(t *testing.T) {
	for _, mode := range []string{"claimed", "role", "scope", "channel", "runtime", "cas", "metrics"} {
		t.Run(mode, func(t *testing.T) {
			_, c := prewarmFixture(t)
			if mode == "metrics" {
				c.runID = "warm-test"
				c.metricsFile = filepath.Join(c.TaskDir, "metrics.jsonl")
			}
			handle := startStubReview(t, c)
			a := drivers.CallArgs{Binding: c.prewarm.binding, RequestID: "test-real-invocation"}
			role, scope := "review", c.prewarm.record.Scope
			switch mode {
			case "role":
				role = "judge"
			case "scope":
				scope = "M01-T02"
			case "channel":
				a.Binding.Channel = "another-account-channel"
			case "runtime":
				handle.matches = false
			case "cas":
				c.revision--
			case "metrics":
				c.Log = func(line string) {
					if strings.Contains(line, "claimed by") {
						c.metricsFile = c.TaskDir
					}
				}
			}
			err := c.claimReviewPrewarm(role, scope, &a)
			if mode == "cas" || mode == "metrics" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if mode == "claimed" {
				require.Same(t, handle, a.Prewarm)
				require.Zero(t, handle.closes)
			} else {
				require.Nil(t, a.Prewarm)
				require.Equal(t, 1, handle.closes)
			}
			require.Nil(t, c.prewarm)
			require.NoError(t, c.claimReviewPrewarm(role, scope, &a))
		})
	}
}

func TestPrewarmCloseFailureBlocksL1RepairAndLeavesNoReviewCall(t *testing.T) {
	fx, c := prewarmFixture(t)
	sd := happyDrivers()
	c.Drivers = sessionDriverSet(sd)
	packet := c.PacketList[0]
	packet.Acceptance.L1[0].Cmd = "false"
	closed := false
	stubReviewPrewarm(t, func(a drivers.DSHArgs) drivers.PreparedDSH {
		return &preparedReviewStub{id: a.PrewarmID, matches: true, closeErr: errors.New("cleanup missing"), closeFn: func() { closed = true }}
	})
	_, err := c.processPacket(packet, processOpts{})
	require.ErrorContains(t, err, "cleanup unconfirmed")
	require.True(t, closed)
	require.Len(t, sd.log.exec, 1, "failed cleanup stops the next repair")
	require.Empty(t, sd.log.review)
	require.NotNil(t, loadFixtureState(t, fx).WorkerPrewarm)
}

func TestPrewarmFactoryAndCancelledAdmissionAreLocal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	handle := prepareReviewDSH(drivers.DSHArgs{})
	require.Error(t, handle.Close(), "invalid preparation cannot launch a runtime")
	_, c := prewarmFixture(t)
	c.RunContext = ctx
	require.ErrorIs(t, c.beginReviewPrewarm(c.PacketList[0]), context.Canceled)
}

func TestPrewarmFormalCleanupFailureCannotDeliverOrLaunchAnotherPacket(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	stubReviewPrewarm(t, func(a drivers.DSHArgs) drivers.PreparedDSH {
		return &preparedReviewStub{id: a.PrewarmID, matches: true, closeErr: errors.New("formal SDK cleanup missing")}
	})
	opts := sessionRunOpts(fx, sessionDriverSet(sd))
	opts.Config.Scheduler.Prewarm = true
	_, err := RunTask(opts)
	require.ErrorContains(t, err, "cleanup")
	require.Len(t, sd.log.exec, 1)
	require.Len(t, sd.log.review, 1)
	st := loadFixtureState(t, fx)
	require.Nil(t, st.WorkerPrewarm, "durable formal invocation owns cleanup after handoff")
	require.Equal(t, "blocked", st.Sessions.Records["review/M01-T01"].Last.Status)
	require.NotEqual(t, "completed", st.Packets["M01-T01"].Status)
	require.Zero(t, st.Packets["M01-T02"].Attempts)
}

func TestPrewarmInvocationHandoffFailureStopsBeforeAdapterDispatch(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	closed := false
	stubReviewPrewarm(t, func(a drivers.DSHArgs) drivers.PreparedDSH {
		return &preparedReviewStub{id: a.PrewarmID, matches: false,
			closeErr: errors.New("mismatched runtime cleanup missing"), closeFn: func() { closed = true }}
	})
	opts := sessionRunOpts(fx, sessionDriverSet(sd))
	opts.Config.Scheduler.Prewarm = true
	_, err := RunTask(opts)
	require.ErrorContains(t, err, "cleanup unconfirmed")
	require.True(t, closed, "a binding mismatch must await cleanup before another adapter can run")
	require.Len(t, sd.log.exec, 1)
	require.Empty(t, sd.log.review, "failed handoff must not dispatch either a warm or cold review")
	st := loadFixtureState(t, fx)
	require.NotNil(t, st.WorkerPrewarm, "unknown cleanup remains a durable obligation")
	require.Equal(t, "intent", st.Sessions.Records["review/M01-T01"].Last.Status)
	require.NotEqual(t, "completed", st.Packets["M01-T01"].Status)
	require.Zero(t, st.Packets["M01-T02"].Attempts)
}

func TestPrewarmPreparationFailurePropagatesThroughExecuteAndResumedReview(t *testing.T) {
	for _, review := range []bool{false, true} {
		t.Run(fmt.Sprint(review), func(t *testing.T) {
			_, c := prewarmFixture(t)
			sd := happyDrivers()
			c.Drivers = sessionDriverSet(sd)
			packet := c.PacketList[0]
			if review {
				out, err := c.executeStep(packet)
				require.NoError(t, err)
				require.True(t, out.Success)
				require.True(t, c.l1Step(packet).Pass)
			}
			previous := reviewWarmBinding
			reviewWarmBinding = func(drivers.DSHArgs) (string, error) { return "", errors.New("prewarm binding refused") }
			t.Cleanup(func() { reviewWarmBinding = previous })
			if review {
				_, err := c.reviewStep(packet, nil)
				require.ErrorContains(t, err, "prewarm binding refused")
			} else {
				_, err := c.processPacket(packet, processOpts{})
				require.ErrorContains(t, err, "prewarm binding refused")
			}
			require.Len(t, sd.log.exec, 1)
			require.Empty(t, sd.log.review)
			require.Nil(t, c.State.WorkerPrewarm)
		})
	}
}

func TestPrewarmQuickGateFailureClosesBeforeAnyRepairDecision(t *testing.T) {
	for _, cleanupFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(cleanupFailure), func(t *testing.T) {
			fx := newQuickFixture(t, true)
			sd := quickDrivers()
			c := quickContext(t, fx, sd)
			enableSessionPolicy(c)
			c.Drivers = sessionDriverSet(sd)
			c.Config.Scheduler.Prewarm, c.prewarmReview = true, true
			packet := c.PacketList[0]
			packet.Acceptance.L2[0].Cmd = "false"
			out, err := c.executeStep(packet)
			require.NoError(t, err)
			require.True(t, out.Success)
			require.True(t, c.l1Step(packet).Pass)
			handle := &preparedReviewStub{matches: true}
			if cleanupFailure {
				handle.closeErr = errors.New("quick SDK cleanup unknown")
			}
			stubReviewPrewarm(t, func(a drivers.DSHArgs) drivers.PreparedDSH { handle.id = a.PrewarmID; return handle })
			verdict, err := c.reviewStep(packet, nil)
			if cleanupFailure {
				require.ErrorContains(t, err, "cleanup unconfirmed")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, errL1Failed, verdict.ErrorCode)
			require.Equal(t, 1, handle.closes, "cleanup must precede caller's failAttempt/JEV/Plan")
			require.Nil(t, c.prewarm)
			if cleanupFailure {
				require.NotNil(t, c.State.WorkerPrewarm)
			} else {
				require.Nil(t, c.State.WorkerPrewarm)
			}
			require.Empty(t, sd.log.review)
			require.Equal(t, 3, c.State.Sessions.Version)
		})
	}
}

func TestPrewarmDefectInjectionStopStillWaitsForUnusedWorker(t *testing.T) {
	for _, cleanupFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(cleanupFailure), func(t *testing.T) {
			fx, c := prewarmFixture(t)
			sd := happyDrivers()
			c.Drivers = sessionDriverSet(sd)
			c.Config.DefectInjection.EveryNUnits = new(1)
			c.Config.DefectInjection.Patch = writeDefectPatch(t, &fx)
			c.Config.DefectInjection.TestCmd = "true"
			handle := &preparedReviewStub{matches: true}
			if cleanupFailure {
				handle.closeErr = errors.New("injection stop SDK cleanup unknown")
			}
			stubReviewPrewarm(t, func(a drivers.DSHArgs) drivers.PreparedDSH { handle.id = a.PrewarmID; return handle })
			outcome, err := c.processPacket(c.PacketList[0], processOpts{})
			if cleanupFailure {
				require.ErrorContains(t, err, "cleanup unconfirmed")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, "stop", outcome)
			require.Equal(t, 1, handle.closes)
			require.Nil(t, c.prewarm)
			require.Empty(t, sd.log.review)
			require.NoFileExists(t, filepath.Join(c.unitCwd(c.PacketList[0]), "defect-marker"))
			if cleanupFailure {
				require.NotNil(t, c.State.WorkerPrewarm)
			} else {
				require.Nil(t, c.State.WorkerPrewarm)
			}
		})
	}
}
