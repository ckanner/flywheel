package investigation

import (
	"context"
	"fmt"
	"time"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/state"
)

// Continue starts an explicitly requested new evidence round, not a replay of
// Run. Exact CAS input, settled history and remaining limits are mandatory.
func Continue(dir string, raw []byte, o Options) (*state.PreparationState, error) {
	if o.ExpectedRevision <= 0 {
		return nil, fmt.Errorf("new investigation round requires the exact current revision")
	}
	next, err := contract.ValidatePreparation(raw)
	if err != nil {
		return nil, err
	}
	previous, release, err := loadOwned(dir, o)
	if err != nil {
		return nil, err
	}
	defer release()
	if previous.Report == nil || previous.Pending != nil || previous.DecisionPending != nil || previous.AnalysisPending != nil || state.PreparationCleanupUnknown(previous) {
		return previous, fmt.Errorf("new investigation round requires a settled report; reconcile the existing round first")
	}
	if next.ID != previous.Contract.ID || state.Exists(dir) && !allowsPlanningInvestigation(dir, previous.Cwd) {
		return previous, fmt.Errorf("new investigation round must retain task identity and precede implementation planning")
	}
	if !state.PreparationAuthorityMatches(previous.Contract, *next) {
		return previous, fmt.Errorf("new investigation round must retain the original goal, intent and implementation scope; repair authority uses the separate planning approval")
	}
	if len(next.KnownEvidence) == 0 {
		return previous, fmt.Errorf("new investigation round requires the newly supplied evidence or investigation basis")
	}
	remaining := state.PreparationRemaining(previous)
	if next.Limits.MaxActions > remaining.MaxActions || next.Limits.MaxSeconds > remaining.MaxSeconds || next.Limits.MaxOutputBytes > remaining.MaxOutputBytes || next.Limits.MaxFileBytes > remaining.MaxFileBytes {
		return previous, fmt.Errorf("new investigation limits exceed remaining cumulative budget: %+v", remaining)
	}
	global, err := loadInvestigationConfiguration(context.Background(), previous.Cwd, dir)
	if err != nil {
		return previous, err
	}
	next.Decision = nil
	status := "disabled"
	if j := global.JEV; j.Enabled {
		policy := j.DecisionPolicy()
		policy.MaxCalls = state.PreparationDecisionRemaining(previous, policy.MaxCalls)
		if policy.MaxCalls > 0 {
			next.Decision = freezeDecision(global, policy)
			status = "enabled"
		}
	}
	snapshot := *previous
	snapshot.History = nil
	history := append([]state.PreparationRound{}, previous.History...)
	history = append(history, state.PreparationRound{Version: 1, Number: len(history) + 1, Digest: state.PreparationHash(&snapshot), Snapshot: &snapshot})
	now := o.now()
	result := &state.PreparationState{Version: 1, Revision: previous.Revision, RunID: previous.RunID, Cwd: previous.Cwd, Contract: *next,
		ContractHash: state.PreparationHash(next), Status: "ready", Reason: "approved-additional-evidence-round", StartedAt: now.Format(time.RFC3339Nano),
		Deadline: now.Add(time.Duration(next.Limits.MaxSeconds) * time.Second).Format(time.RFC3339Nano), Observations: []state.InvestigationObservation{}, Decisions: []decision.Receipt{}, JEVStatus: status, History: history}
	return state.SavePreparation(dir, result)
}
