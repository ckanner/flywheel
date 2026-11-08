package machine

import (
	"fmt"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
)

// Role selection changes history reuse only. Tracking, budget and recovery
// remain mandatory for fresh calls too; the legacy Plan opt-out is checked by
// scopedSessions after this explicit override.
func sessionQualityMode(role string, configured config.SessionsConfig) string {
	return configured.ModeForRole(role)
}

// These are observed workload limits, not a guessed native context-window size
// or a semantic judgement by Go. Defaults are disabled until paired task
// evidence supports an explicit deployment policy. A rebuild never resolves an
// unknown invocation or consumes an unapplied result.
func sessionQualityReason(role string, previous *state.SessionRecord, policy config.SessionQualityPolicy) string {
	if previous == nil || previous.Quality == nil || !previous.ResumeCandidate || previous.Last == nil ||
		previous.Last.Status != "consumed" || !previous.Last.Accounted || !previous.Last.CleanupConfirmed ||
		(previous.Last.Dispatch != "dispatched" && previous.Last.Dispatch != "not-dispatched") ||
		(previous.Last.Outcome != "completed" && previous.Last.Outcome != "failed" && previous.Last.Outcome != "cancelled") {
		return ""
	}
	q := previous.Quality
	if policy.MaxConfirmedTurns > 0 && q.ConfirmedTurns >= policy.MaxConfirmedTurns {
		return fmt.Sprintf("context-quality: confirmed turn limit reached (%d >= %d)", q.ConfirmedTurns, policy.MaxConfirmedTurns)
	}
	if role == "execute" && policy.MaxUnproductiveTurns > 0 && q.UnproductiveTurns >= policy.MaxUnproductiveTurns {
		return fmt.Sprintf("context-quality: unchanged candidate limit reached (%d >= %d)", q.UnproductiveTurns, policy.MaxUnproductiveTurns)
	}
	if policy.MaxRunInputTokens > 0 && q.LastInputTokens != nil && *q.LastInputTokens >= policy.MaxRunInputTokens {
		return fmt.Sprintf("context-quality: measured run input limit reached (%d >= %d)", *q.LastInputTokens, policy.MaxRunInputTokens)
	}
	return ""
}

func nextSessionQuality(previous *state.SessionRecord, action, reason string) *state.SessionQuality {
	q := &state.SessionQuality{LastUsageStatus: "unknown"}
	if previous != nil && previous.Quality != nil {
		*q = *previous.Quality
	}
	if action == "rebuild" {
		q = &state.SessionQuality{LastUsageStatus: "unknown", Rebuilds: q.Rebuilds + 1, RebuildReason: reason}
	}
	return q
}

// Apply once per correlated, safely ended invocation. The same helper runs when
// a saved receipt is recovered after a failed state CAS, so recovery cannot
// inflate the quality counters. Unknown/partial usage is not filled with zero.
func observeSessionQuality(record *state.SessionRecord, result drivers.Result) {
	if record.Last == nil || !safeSessionOutcome(result, record.Last.ID) || result.Session.Dispatch != "dispatched" || record.Last.CandidateAfter == "" {
		return
	}
	if record.Quality == nil {
		record.Quality = nextSessionQuality(nil, "create", "")
	}
	q := record.Quality
	if q.LastInvocationID == record.Last.ID {
		return
	}
	q.LastInvocationID = record.Last.ID
	q.ConfirmedTurns++
	q.LastUsageStatus, q.LastInputTokens = result.Session.UsageStatus, nil
	if result.Session.UsageStatus == "complete" && result.Usage != nil {
		input := result.Usage.Input
		q.LastInputTokens = &input
	}
	if record.Role == "execute" && record.Last.CandidateBefore == record.Last.CandidateAfter {
		q.UnproductiveTurns++
	} else {
		q.UnproductiveTurns = 0
	}
}

// A rebuild hands over current authority and identifiers, never another role's
// transcript or private reasoning. It does not erase results, reset attempts or
// manufacture a model-generated compression summary.
func sessionRebuildHandoff(previous *state.SessionRecord, reason string) string {
	if previous == nil {
		return ""
	}
	return fmt.Sprintf("Session context rebuilt for %s/%s (previous generation %d, invocation %s).\nReason: %s.\nUse the current authoritative contracts, workspace, allowed reports and verification evidence below. Read the complete current scope; prior findings are navigation, not a substitute for independent checks. Preserve confirmed work and do not repeat confirmed external actions. No private transcript from another role is included.\n\n",
		previous.Role, previous.Scope, previous.Generation, previous.Last.ID, reason)
}
