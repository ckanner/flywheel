package metrics

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Phase measures an observable engine boundary. It makes no claim about the
// provider's internal queue, reasoning, tool, or token-generation time.
type Phase struct {
	Version      int        `json:"phase-version"`
	Kind         string     `json:"record-kind"`
	ID           string     `json:"span-id"`
	Run          string     `json:"run-id"`
	Name         string     `json:"phase"`
	Scope        string     `json:"scope,omitempty"`
	ParentID     string     `json:"parent-span-id,omitempty"`
	InvocationID string     `json:"phase-invocation-id,omitempty"`
	StartedAt    time.Time  `json:"started-at"`
	EndedAt      *time.Time `json:"ended-at,omitempty"`
}

type Timeline struct {
	Coverage          string       `json:"coverage"`
	ProviderBreakdown string       `json:"providerBreakdown"`
	Spans             int          `json:"spans"`
	Complete          int          `json:"completeSpans"`
	Unknown           int          `json:"unknownSpans"`
	ClockRegressions  int          `json:"clockRegressions"`
	UnmatchedCalls    int          `json:"unmatchedCalls"`
	OmittedIntervals  int          `json:"omittedIntervals"`
	OmittedGroups     int          `json:"omittedGroups"`
	KnownSpanMS       int64        `json:"knownSpanMs"`
	ObservedWallMS    int64        `json:"observedWallMs"`
	Groups            []PhaseGroup `json:"groups"`
	Intervals         []Phase      `json:"intervals"`
}

type PhaseGroup struct {
	Run            string `json:"runId"`
	Phase          string `json:"phase"`
	Spans          int    `json:"spans"`
	Unknown        int    `json:"unknownSpans"`
	KnownSpanMS    int64  `json:"knownSpanMs"`
	ObservedWallMS int64  `json:"observedWallMs"`
}

func validPhase(p Phase) bool {
	return p.Version == 1 && p.ID != "" && len(p.ID) <= 120 && p.Run != "" && len(p.Run) <= 256 && p.Name != "" && len(p.Name) <= 80 && len(p.Scope) <= 256 && len(p.ParentID) <= 120 && len(p.InvocationID) <= 256 && !p.StartedAt.IsZero() && (p.Kind == "phase-start" && p.EndedAt == nil || p.Kind == "phase-end" && p.EndedAt != nil && !p.EndedAt.IsZero())
}

func appendPhase(file string, p Phase) error {
	if !validPhase(p) {
		return fmt.Errorf("invalid phase identity or timestamps")
	}
	entry := map[string]any{
		"phase-version": p.Version, "record-kind": p.Kind, "span-id": p.ID,
		"run-id": p.Run, "phase": p.Name, "scope": p.Scope,
		"parent-span-id": p.ParentID, "phase-invocation-id": p.InvocationID,
		"started-at": p.StartedAt.UTC(),
	}
	if p.EndedAt != nil {
		entry["ended-at"] = p.EndedAt.UTC()
	}
	return Append(file, entry)
}

// BeginPhase must succeed before entering the measured work. Its durable start
// makes a crash or missing terminal visible as an unknown interval.
func BeginPhase(file string, p Phase) error {
	p.Version, p.Kind, p.EndedAt = 1, "phase-start", nil
	return appendPhase(file, p)
}

func EndPhase(file string, p Phase) error {
	p.Version, p.Kind = 1, "phase-end"
	return appendPhase(file, p)
}

func phaseKey(p Phase) string { return p.Run + "\x00" + p.ID }

func collectPhase(phases map[string]Phase, data []byte) error {
	var event Phase
	if json.Unmarshal(data, &event) != nil || !validPhase(event) {
		return fmt.Errorf("invalid phase event")
	}
	key := phaseKey(event)
	prior, exists := phases[key]
	if event.Kind == "phase-start" {
		if exists {
			return fmt.Errorf("duplicate phase start")
		}
		phases[key] = event
		return nil
	}
	if !exists || prior.EndedAt != nil || prior.Name != event.Name || prior.Scope != event.Scope || prior.ParentID != event.ParentID || !prior.StartedAt.Equal(event.StartedAt) || prior.InvocationID != "" && prior.InvocationID != event.InvocationID {
		return fmt.Errorf("phase end has no matching open start")
	}
	phases[key] = event
	return nil
}

type interval struct{ start, end time.Time }

func unionMS(intervals []interval) int64 {
	if len(intervals) == 0 {
		return 0
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].start.Before(intervals[j].start) })
	start, end := intervals[0].start, intervals[0].end
	var total time.Duration
	for _, item := range intervals[1:] {
		if item.start.After(end) {
			total += end.Sub(start)
			start, end = item.start, item.end
		} else if item.end.After(end) {
			end = item.end
		}
	}
	return (total + end.Sub(start)).Milliseconds()
}

func timeline(phases map[string]Phase, calls []Call, run string) Timeline {
	t := Timeline{Coverage: "unobserved", ProviderBreakdown: "unknown", Groups: []PhaseGroup{}, Intervals: []Phase{}}
	groups := map[string]*PhaseGroup{}
	intervals := map[string][]interval{}
	matched := map[string]bool{}
	all := []interval{}
	for _, phase := range phases {
		if run != "" && phase.Run != run {
			continue
		}
		t.Spans++
		t.Intervals = append(t.Intervals, phase)
		key := phase.Run + "\x00" + phase.Name
		group := groups[key]
		if group == nil {
			group = &PhaseGroup{Run: phase.Run, Phase: phase.Name}
			groups[key] = group
		}
		group.Spans++
		if phase.EndedAt == nil || phase.EndedAt.Before(phase.StartedAt) {
			t.Unknown++
			group.Unknown++
			if phase.EndedAt != nil {
				t.ClockRegressions++
			}
			continue
		}
		t.Complete++
		if phase.InvocationID != "" {
			matched[phase.Run+"\x00"+phase.InvocationID] = true
		}
		elapsed := phase.EndedAt.Sub(phase.StartedAt).Milliseconds()
		t.KnownSpanMS += elapsed
		group.KnownSpanMS += elapsed
		span := interval{phase.StartedAt, *phase.EndedAt}
		intervals[key] = append(intervals[key], span)
		all = append(all, span)
	}
	for _, call := range calls {
		if call.Invocation == "" || !matched[call.Run+"\x00"+call.Invocation] {
			t.UnmatchedCalls++
		}
	}
	if t.Spans > 0 {
		t.Coverage = "partial-engine-timeline"
	}
	t.ObservedWallMS = unionMS(all)
	sort.Slice(t.Intervals, func(i, j int) bool {
		if t.Intervals[i].StartedAt.Equal(t.Intervals[j].StartedAt) {
			return phaseKey(t.Intervals[i]) < phaseKey(t.Intervals[j])
		}
		return t.Intervals[i].StartedAt.Before(t.Intervals[j].StartedAt)
	})
	t.OmittedIntervals = max(0, len(t.Intervals)-200)
	t.Intervals = t.Intervals[t.OmittedIntervals:]
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	t.OmittedGroups = max(0, len(keys)-500)
	for _, key := range keys[t.OmittedGroups:] {
		group := groups[key]
		group.ObservedWallMS = unionMS(intervals[key])
		t.Groups = append(t.Groups, *group)
	}
	return t
}
