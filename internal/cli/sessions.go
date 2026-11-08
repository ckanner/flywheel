package cli

import (
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/ckanner/flywheel/internal/state"
)

// Compact progress never opens backend history or receipt files. The full
// typed state remains in the existing control response for compatibility.
type sessionSummary struct {
	Role            string                   `json:"role"`
	Scope           string                   `json:"scope"`
	Generation      int                      `json:"generation"`
	Action          string                   `json:"action"`
	Reason          string                   `json:"reason"`
	ResumeCandidate bool                     `json:"resumeCandidate"`
	Last            sessionInvocationSummary `json:"last"`
}

type sessionInvocationSummary struct {
	Status           string `json:"status"`
	Persistent       bool   `json:"persistent"`
	Dispatch         string `json:"dispatch"`
	Outcome          string `json:"outcome"`
	CleanupConfirmed bool   `json:"cleanupConfirmed"`
	UsageStatus      string `json:"usageStatus"`
	Accounted        bool   `json:"accounted"`
}

func sessionSummaries(st *state.State) []sessionSummary {
	if st == nil || st.Sessions == nil {
		return nil
	}
	summaries := make([]sessionSummary, 0, len(st.Sessions.Records))
	for _, key := range slices.Sorted(maps.Keys(st.Sessions.Records)) {
		record := st.Sessions.Records[key]
		// state.Load validates non-null records and invocations before projection.
		last := record.Last
		summaries = append(summaries, sessionSummary{
			Role: record.Role, Scope: record.Scope, Generation: record.Generation,
			Action: record.Action, Reason: record.Reason, ResumeCandidate: record.ResumeCandidate,
			Last: sessionInvocationSummary{
				Status: last.Status, Persistent: last.Persistent, Dispatch: last.Dispatch, Outcome: last.Outcome,
				CleanupConfirmed: last.CleanupConfirmed, UsageStatus: last.UsageStatus, Accounted: last.Accounted,
			},
		})
	}
	return summaries
}

func printSessionSummaries(out io.Writer, st *state.State) {
	summaries := sessionSummaries(st)
	if len(summaries) == 0 {
		return
	}
	fmt.Fprintf(out, "会话记录: v%d（恢复候选不代表历史恢复已验证）\n", st.Sessions.Version)
	for _, session := range summaries {
		fmt.Fprintf(out, "  %s/%s: generation=%d action=%s reason=%s resume-candidate=%t\n", session.Role, session.Scope, session.Generation, session.Action, session.Reason, session.ResumeCandidate)
		last := session.Last
		fmt.Fprintf(out, "    latest=%s persistent=%t dispatch=%s outcome=%s cleanup=%t usage=%s accounted=%t\n", last.Status, last.Persistent, last.Dispatch, last.Outcome, last.CleanupConfirmed, last.UsageStatus, last.Accounted)
	}
}
