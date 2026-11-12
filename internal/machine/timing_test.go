package machine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/metrics"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestPhaseTimingRecordsNestedActualIntervalsAndBackoff(t *testing.T) {
	clock := time.Unix(100, 0)
	c := &Context{runID: "run", metricsFile: filepath.Join(t.TempDir(), "metrics"), Now: func() time.Time { return clock }}
	finish := c.measurePhase("engine", "")
	clock = clock.Add(2 * time.Millisecond)
	child := c.measurePhase("verify", "M01-T01")
	clock = clock.Add(3 * time.Millisecond)
	child()
	child() // a second cleanup cannot produce a duplicate terminal
	c.Sleep = func(delay time.Duration) { clock = clock.Add(delay) }
	c.waitRetry(5*time.Millisecond, "M01-T01")
	finish()
	require.NoError(t, c.err())
	require.Empty(t, c.phaseParent)
	report, err := metrics.ReadReport(c.metricsFile)
	require.NoError(t, err)
	require.Equal(t, 3, report.Timeline.Complete)
	require.EqualValues(t, 18, report.Timeline.KnownSpanMS)
	require.EqualValues(t, 10, report.Timeline.ObservedWallMS)
	require.NotEmpty(t, report.Timeline.Intervals[1].ParentID)
	require.Equal(t, report.Timeline.Intervals[0].ID, report.Timeline.Intervals[1].ParentID)
}

func TestPhaseFailuresStayVisibleAndDoNotOverridePriorFailure(t *testing.T) {
	c := &Context{}
	c.measurePhase("unbound", "")()
	require.NoError(t, c.err())
	clock := func() time.Time { return time.Unix(100, 0) }
	c = &Context{runID: "run", metricsFile: t.TempDir(), Now: clock}
	c.measurePhase("start-failure", "")()
	require.ErrorContains(t, c.err(), "phase start")
	for _, prior := range []bool{false, true} {
		file := filepath.Join(t.TempDir(), "metrics")
		c = &Context{runID: "run", metricsFile: file, Now: clock}
		finish := c.measurePhase("interrupted", "")
		require.NoError(t, os.Rename(file, file+".preserved"))
		require.NoError(t, os.Mkdir(file, 0700))
		var expected error
		if prior {
			expected = errors.New("original failure")
			c.fatal = expected
		}
		finish()
		if prior {
			require.ErrorIs(t, c.err(), expected)
		} else {
			require.ErrorContains(t, c.err(), "phase end")
		}
		report, err := metrics.ReadReport(file + ".preserved")
		require.NoError(t, err)
		require.Equal(t, 1, report.Timeline.Unknown)
		require.Zero(t, report.Timeline.ObservedWallMS)
	}
}

func TestCancelledBackoffDoesNotSleepAndSaveCannotBypassTimingFault(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &Context{RunContext: ctx, Sleep: func(time.Duration) { t.Fatal("cancelled backoff slept") }}
	c.waitRetry(time.Second, "packet")
	fx := newQuickFixture(t, false)
	c = quickContext(t, fx, quickDrivers())
	c.runID = "run"
	c.metricsFile = t.TempDir()
	revision := c.revision
	c.save()
	require.ErrorContains(t, c.err(), "phase start")
	require.Equal(t, revision, c.revision)
}

func TestTimingFailureStopsExecuteAndAcceptanceBeforeWork(t *testing.T) {
	for _, stage := range []string{"execute", "verify", "review", "judge"} {
		t.Run(stage, func(t *testing.T) {
			fx := newQuickFixture(t, false)
			c := quickContext(t, fx, quickDrivers())
			c.runID, c.metricsFile = "run", t.TempDir()
			packet := c.PacketList[0]
			before := c.revision
			switch stage {
			case "execute":
				_, err := c.executeStep(packet)
				require.ErrorContains(t, err, "phase start")
			case "verify":
				require.False(t, c.l1Step(packet).Pass)
			case "review":
				_, err := c.reviewStep(packet, nil)
				require.ErrorContains(t, err, "phase start")
			case "judge":
				_, err := c.judgeStep(c.Spec.Milestones[0])
				require.ErrorContains(t, err, "phase start")
			}
			require.ErrorContains(t, c.err(), "phase start")
			require.Equal(t, before, c.revision)
			require.NoFileExists(t, filepath.Join(fx.root, "build", "M01-T01.txt"))
		})
	}
}

func TestManagedTimingBindsOnlyObservedInvocationIdentity(t *testing.T) {
	clock := time.Unix(100, 0)
	c := &Context{runID: "run", metricsFile: filepath.Join(t.TempDir(), "metrics"), Now: func() time.Time { return clock }, State: &state.State{}}
	c.measureManagedInvocation("execute", "packet")()
	c.State.Sessions = &state.SessionLedger{Records: map[string]*state.SessionRecord{}}
	end := c.measureManagedInvocation("execute", "packet")
	c.State.Sessions.Records[sessionKey("execute", "packet")] = &state.SessionRecord{Last: &state.SessionInvocation{ID: "real-call"}}
	end()
	report, err := metrics.ReadReport(c.metricsFile)
	require.NoError(t, err)
	require.Len(t, report.Timeline.Intervals, 2)
	ids := []string{report.Timeline.Intervals[0].InvocationID, report.Timeline.Intervals[1].InvocationID}
	require.ElementsMatch(t, []string{"", "real-call"}, ids)
}
