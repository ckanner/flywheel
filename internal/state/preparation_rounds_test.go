package state

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/decision"
	"github.com/stretchr/testify/require"
)

func copyPreparation(t *testing.T, st *PreparationState) *PreparationState {
	t.Helper()
	var result PreparationState
	require.NoError(t, json.Unmarshal(prepJSON(t, st), &result))
	return &result
}

func nextPreparationRound(t *testing.T, st *PreparationState) *PreparationState {
	t.Helper()
	snapshot := copyPreparation(t, st)
	snapshot.History = nil
	next := copyPreparation(t, st)
	next.History = append(next.History, PreparationRound{Version: 1, Number: len(next.History) + 1, Digest: PreparationHash(snapshot), Snapshot: snapshot})
	next.Revision++
	next.Contract.Limits = PreparationRemaining(st)
	next.Contract.Decision = nil
	next.ContractHash = PreparationHash(next.Contract)
	start, err := time.Parse(time.RFC3339Nano, st.StartedAt)
	require.NoError(t, err)
	start = start.Add(24 * time.Hour)
	next.StartedAt, next.Deadline = start.Format(time.RFC3339Nano), start.Add(time.Duration(next.Contract.Limits.MaxSeconds)*time.Second).Format(time.RFC3339Nano)
	next.ActionsUsed, next.OutputBytes, next.ElapsedMS = 0, 0, 0
	next.Observations, next.Decisions, next.DecisionEffects, next.DecisionStartedAt = []InvestigationObservation{}, []decision.Receipt{}, nil, nil
	next.Pending, next.DecisionPending = nil, nil
	next.AnalysisPending, next.Analysis = nil, nil
	return next
}

func TestInvestigationRoundsKeepReportsEvidenceAndCumulativeBudgets(t *testing.T) {
	first := supportedPreparation(t)
	second := nextPreparationRound(t, first)
	// A new conclusion may cite original immutable evidence. Archiving that
	// conclusion later must retain the same reference across three rounds.
	require.NoError(t, validatePreparationState(second))
	third := nextPreparationRound(t, second)
	require.NoError(t, validatePreparationState(third))
	require.Len(t, PreparationRounds(third), 3)
	actions, output, elapsed := PreparationTotals(third)
	require.Equal(t, first.ActionsUsed, actions)
	require.Equal(t, first.OutputBytes, output)
	require.Equal(t, first.ElapsedMS, elapsed)
	require.Equal(t, first.Contract.Limits.MaxActions-first.ActionsUsed, third.Contract.Limits.MaxActions)
	spent := *first
	spent.ElapsedMS = int64(first.Contract.Limits.MaxSeconds)*1000 + 1
	require.Zero(t, PreparationRemaining(&spent).MaxSeconds)
	third.History[0].Snapshot.Report.Summary = "rewritten"
	require.ErrorContains(t, validatePreparationState(third), "immutable snapshot")
}

func TestInvestigationRoundSemanticHistoryRejectsAlteredIdentityAndBudget(t *testing.T) {
	for name, mutate := range map[string]func(*PreparationState){
		"nil snapshot":   func(st *PreparationState) { st.History[0].Snapshot = nil },
		"unfinished":     func(st *PreparationState) { st.History[0].Snapshot.Report = nil },
		"different task": func(st *PreparationState) { st.History[0].Snapshot.Contract.ID = "another" },
		"different goal": func(st *PreparationState) { st.Contract.Goal = "Another objective" },
		"new repair authority": func(st *PreparationState) {
			st.Contract.Intent, st.Contract.ImplementationScope = "investigate-and-fix", []string{"src"}
		},
		"revision rollback":   func(st *PreparationState) { st.Revision = st.History[0].Snapshot.Revision },
		"invalid old receipt": func(st *PreparationState) { st.History[0].Snapshot.Observations[0].ContentHash = "" },
		"action budget reset": func(st *PreparationState) { st.Contract.Limits.MaxActions++ },
		"output budget reset": func(st *PreparationState) { st.Contract.Limits.MaxOutputBytes++ },
		"time budget reset":   func(st *PreparationState) { st.Contract.Limits.MaxSeconds++ },
	} {
		t.Run(name, func(t *testing.T) {
			st := nextPreparationRound(t, supportedPreparation(t))
			mutate(st)
			st.History[0].Digest = PreparationHash(st.History[0].Snapshot)
			require.Error(t, validatePreparationRounds(st))
		})
	}
	st := nextPreparationRound(t, supportedPreparation(t))
	st.Observations = append(st.Observations, st.History[0].Snapshot.Observations[0])
	require.ErrorContains(t, validatePreparationRounds(st), "identities cannot repeat")
	st.Pending = &st.Observations[0].Invocation
	st.Observations = nil
	require.ErrorContains(t, validatePreparationRounds(st), "pending invocation repeats")
}

func TestPreparationWriterRejectsChangingAuthorityAcrossNewRounds(t *testing.T) {
	first := supportedPreparation(t)
	first.Contract.Intent, first.Contract.ImplementationScope = "investigate-and-fix", []string{"src"}
	first.ContractHash, first.Revision = PreparationHash(first.Contract), 0
	dir := t.TempDir()
	first, err := SavePreparation(dir, first)
	require.NoError(t, err)
	for name, mutate := range map[string]func(*PreparationState){
		"broaden":  func(st *PreparationState) { st.Contract.ImplementationScope = []string{"src", "other"} },
		"retarget": func(st *PreparationState) { st.Contract.ImplementationScope = []string{"other"} },
		"intent": func(st *PreparationState) {
			st.Contract.Intent, st.Contract.ImplementationScope = "investigate", []string{}
		},
		"goal": func(st *PreparationState) { st.Contract.Goal = "Another objective" },
	} {
		t.Run(name, func(t *testing.T) {
			next := nextPreparationRound(t, first)
			next.Revision = first.Revision
			mutate(next)
			next.ContractHash = PreparationHash(next.Contract)
			_, err := SavePreparation(dir, next)
			require.ErrorContains(t, err, "original goal, intent or implementation scope")
			saved, err := LoadPreparation(dir)
			require.NoError(t, err)
			require.Equal(t, first, saved)
		})
	}
}

func TestDecisionBudgetCannotResetThroughDisabledOrUnknownRounds(t *testing.T) {
	first := supportedPreparation(t)
	addPreparationDecision(first)
	first.Decisions[0].Status = "unknown"
	require.Equal(t, 2, PreparationDecisionRemaining(first, 20))
	first.DecisionPending = &decision.Snapshot{}
	require.Equal(t, 1, PreparationDecisionRemaining(first, 20))
	first.DecisionPending = nil
	second := nextPreparationRound(t, first)
	third := nextPreparationRound(t, second)
	require.Equal(t, 2, PreparationDecisionRemaining(third, 20))
	addPreparationDecision(third)
	third.Decisions = nil
	third.DecisionStartedAt = nil
	require.ErrorContains(t, validatePreparationRounds(third), "reset decision calls")
	third.Contract.Decision.Policy.MaxCalls = 2
	require.NoError(t, validatePreparationRounds(third))
	first.Decisions[0].Status = "skipped"
	require.Equal(t, 3, PreparationDecisionRemaining(first, 20))
	first.Contract.Decision.Policy.MaxCalls = 0
	first.Decisions[0].Status = "unknown"
	require.Zero(t, PreparationDecisionRemaining(first, 20))
}

func TestRoundReportsCannotEraseUnknownNativeDecisionCleanup(t *testing.T) {
	st := supportedPreparation(t)
	require.False(t, PreparationCleanupUnknown(st))
	st.Decisions = []decision.Receipt{{Backend: "subscription", Status: "unknown"}}
	require.True(t, PreparationCleanupUnknown(st))
	require.ErrorContains(t, validatePreparationReport(st), "unknown native decision cleanup")
	next := nextPreparationRound(t, st)
	next.Decisions = nil
	require.True(t, PreparationCleanupUnknown(next), "archiving a receipt does not resolve native cleanup")
}

func TestPreparationCASOnlyAppendsRoundsAndNeverRewritesReceipts(t *testing.T) {
	old := supportedPreparation(t)
	require.NoError(t, validatePreparationTransition(nil, old))
	require.NoError(t, validatePreparationTransition(old, copyPreparation(t, old)))
	appended := nextPreparationRound(t, old)
	require.NoError(t, validatePreparationTransition(old, appended))
	for name, mutate := range map[string]func(*PreparationState){
		"run identity":  func(st *PreparationState) { st.RunID = "different" },
		"contract":      func(st *PreparationState) { st.ContractHash = PreparationHash("changed") },
		"spent actions": func(st *PreparationState) { st.ActionsUsed-- },
		"observation":   func(st *PreparationState) { st.Observations[0].Output = "rewritten" },
		"report":        func(st *PreparationState) { st.Report.Summary = "rewritten" },
	} {
		t.Run(name, func(t *testing.T) {
			next := copyPreparation(t, old)
			mutate(next)
			require.Error(t, validatePreparationTransition(old, next))
		})
	}
	appended.History[0].Snapshot.Reason = "not the previous revision"
	require.ErrorContains(t, validatePreparationTransition(old, appended), "exact previous revision")
	appended = nextPreparationRound(t, old)
	rewrite := copyPreparation(t, appended)
	rewrite.History[0].Snapshot.Report.Summary = "rewritten archive"
	require.ErrorContains(t, validatePreparationTransition(appended, rewrite), "cannot be rewritten")
	// The CAS writer, not just the loading validator, enforces immutable reports.
	dir := t.TempDir()
	old.Revision = 0
	saved, err := SavePreparation(dir, old)
	require.NoError(t, err)
	saved.Report.Summary = "different but structurally valid"
	_, err = SavePreparation(dir, saved)
	require.ErrorContains(t, err, "report is immutable")
}
