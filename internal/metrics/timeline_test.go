package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func phaseFixture(id, run, name string, start, end int64) Phase {
	s := time.Unix(100, start*int64(time.Millisecond)).UTC()
	p := Phase{Version: 1, Kind: "phase-start", ID: id, Run: run, Name: name, StartedAt: s}
	if end >= 0 {
		e := time.Unix(100, end*int64(time.Millisecond)).UTC()
		p.Kind, p.EndedAt = "phase-end", &e
	}
	return p
}

func addPhase(t *testing.T, file string, p Phase) {
	t.Helper()
	require.NoError(t, BeginPhase(file, p))
	if p.EndedAt != nil {
		require.NoError(t, EndPhase(file, p))
	}
}

func TestTimelineSeparatesOverlappingWallTimeFromSum(t *testing.T) {
	file := filepath.Join(t.TempDir(), "metrics.jsonl")
	p := phaseFixture("first", "run-1", "execute", 0, 10)
	p.InvocationID = "call-1"
	addPhase(t, file, p)
	p = phaseFixture("second", "run-1", "execute", 5, 15)
	p.InvocationID = "call-2"
	addPhase(t, file, p)
	p = phaseFixture("nested", "run-1", "persistence", 6, 7)
	p.ParentID = "second"
	addPhase(t, file, p)
	addPhase(t, file, phaseFixture("third", "run-1", "verify", 20, 25))
	addPhase(t, file, phaseFixture("open", "run-1", "judge", 25, -1))
	addPhase(t, file, phaseFixture("backwards", "run-1", "review", 30, 29))
	addPhase(t, file, phaseFixture("another", "run-2", "plan", 40, 50))
	for _, id := range []string{"call-1", "call-2", "unknown"} {
		require.NoError(t, AppendOnce(file, id, "receipt-"+id, map[string]any{"role": "execute", "run-id": "run-1", "duration-ms": 10}))
	}
	report, err := ReadReport(file, "run-1")
	require.NoError(t, err)
	require.Equal(t, 3, report.TotalCalls)
	require.EqualValues(t, 30, report.Groups[0].Duration)
	require.Equal(t, 6, report.Timeline.Spans)
	require.Equal(t, 4, report.Timeline.Complete)
	require.Equal(t, 2, report.Timeline.Unknown)
	require.Equal(t, 1, report.Timeline.ClockRegressions)
	require.Equal(t, 1, report.Timeline.UnmatchedCalls)
	require.EqualValues(t, 26, report.Timeline.KnownSpanMS)
	require.EqualValues(t, 20, report.Timeline.ObservedWallMS)
	require.Equal(t, "partial-engine-timeline", report.Timeline.Coverage)
	require.Equal(t, "unknown", report.Timeline.ProviderBreakdown)
	require.Equal(t, "execute", report.Timeline.Groups[0].Phase)
	require.EqualValues(t, 20, report.Timeline.Groups[0].KnownSpanMS)
	require.EqualValues(t, 15, report.Timeline.Groups[0].ObservedWallMS)
	require.Len(t, report.Timeline.Intervals, 6)
	all, err := ReadReport(file)
	require.NoError(t, err)
	require.Equal(t, 7, all.Timeline.Spans)
	require.EqualValues(t, 30, all.Timeline.ObservedWallMS)
}

func TestTimelineOldCallsRemainUnobservedAndOutputIsBounded(t *testing.T) {
	report, err := parseReport(strings.NewReader("{\"role\":\"execute\",\"duration-ms\":5}\n"))
	require.NoError(t, err)
	require.Equal(t, "unobserved", report.Timeline.Coverage)
	require.Equal(t, 1, report.Timeline.UnmatchedCalls)
	require.Zero(t, report.Timeline.ObservedWallMS)
	phases := map[string]Phase{}
	for i := range 503 {
		p := phaseFixture(fmt.Sprint(i), "run", fmt.Sprintf("phase-%03d", i), int64(i/2), int64(i/2)+1)
		phases[phaseKey(p)] = p
	}
	summary := timeline(phases, nil, "")
	require.Len(t, summary.Groups, 500)
	require.Equal(t, 3, summary.OmittedGroups)
	require.Len(t, summary.Intervals, 200)
	require.Equal(t, 303, summary.OmittedIntervals)
	missing, err := ReadReport(filepath.Join(t.TempDir(), "missing"))
	require.NoError(t, err)
	require.Equal(t, "unobserved", missing.Timeline.Coverage)
}

func TestTimelineRejectsBrokenPhaseIdentity(t *testing.T) {
	for _, scenario := range []string{"json", "version", "no-start", "double-start", "double-end", "scope", "invocation"} {
		t.Run(scenario, func(t *testing.T) {
			phases := map[string]Phase{}
			start := phaseFixture("span", "run", "execute", 0, -1)
			start.InvocationID = "call-1"
			data, _ := json.Marshal(start)
			if scenario != "no-start" {
				require.NoError(t, collectPhase(phases, data))
			}
			end := phaseFixture("span", "run", "execute", 0, 10)
			end.InvocationID = "call-1"
			switch scenario {
			case "json":
				data = []byte("{")
			case "version":
				end.Version = 10
				data, _ = json.Marshal(end)
			case "double-start":
			case "double-end":
				data, _ = json.Marshal(end)
				require.NoError(t, collectPhase(phases, data))
			case "scope":
				end.Scope = "different-packet"
				data, _ = json.Marshal(end)
			case "invocation":
				end.InvocationID = "other-call"
				data, _ = json.Marshal(end)
			default:
				data, _ = json.Marshal(end)
			}
			require.Error(t, collectPhase(phases, data))
		})
	}
	_, err := parseReport(strings.NewReader("{\"record-kind\":\"phase-start\"}\n"))
	require.Error(t, err)
	file := filepath.Join(t.TempDir(), "metrics")
	require.Error(t, BeginPhase(file, Phase{}))
	require.Error(t, EndPhase(file, phaseFixture("span", "run", "phase", 0, -1)))
}

func TestConcurrentPhasesCannotTearAccountingReadDeduplication(t *testing.T) {
	file := filepath.Join(t.TempDir(), "metrics")
	var workers sync.WaitGroup
	for i := range 12 {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			id := fmt.Sprint(i)
			p := phaseFixture(id, "run", "execute", int64(i), int64(i+10))
			require.NoError(t, BeginPhase(file, p))
			p.InvocationID = id // learned from the managed receipt after dispatch
			require.NoError(t, AppendOnce(file, id, "receipt", map[string]any{"role": "execute", "run-id": "run"}))
			require.NoError(t, AppendOnce(file, id, "receipt", map[string]any{"role": "execute", "run-id": "run"}))
			require.NoError(t, EndPhase(file, p))
		}(i)
	}
	workers.Wait()
	report, err := ReadReport(file)
	require.NoError(t, err)
	require.Equal(t, 12, report.TotalCalls)
	require.Equal(t, 12, report.Timeline.Complete)
	require.Zero(t, report.Timeline.UnmatchedCalls)
	require.EqualValues(t, 21, report.Timeline.ObservedWallMS)
	info, err := os.Stat(file)
	require.NoError(t, err)
	require.Positive(t, info.Size())
}
