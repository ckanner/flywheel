package state

import (
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/decision"
	"github.com/stretchr/testify/require"
)

func collectionFixture(t *testing.T, kind string) *PreparationState {
	t.Helper()
	st := preparationFixture(t)
	observation := preparationObservation(st, 1) // both declared hypotheses
	st.Observations, st.ActionsUsed, st.OutputBytes, st.ElapsedMS = []InvestigationObservation{observation}, 1, len(observation.Output), 3000
	addPreparationDecision(st)
	for _, control := range InvestigationControls(st, PreparationHash("candidate")) {
		if control.Kind == kind {
			st.Decisions[0].ActionID = control.ActionID
			st.DecisionEffects = []InvestigationDecisionEffect{{InvestigationControl: control, DecisionID: st.Decisions[0].DecisionID, Observations: 1, BindingHash: PreparationHash(control), CandidateHash: PreparationHash("candidate"), At: "2026-11-12T00:00:03Z"}}
			break
		}
	}
	if kind == "finish" {
		st.Status, st.Reason = "awaiting-report", "jev-evidence-ready-for-report"
	}
	require.NoError(t, validatePreparationState(st))
	return st
}

func TestCollectionChoicesRequireSuccessfulCompleteEvidenceForEachHypothesis(t *testing.T) {
	st := preparationFixture(t)
	require.Empty(t, InvestigationControls(st, PreparationHash("candidate")))
	o := preparationObservation(st, 1)
	for _, failure := range []string{"failed", "truncated", "empty"} {
		bad := o
		switch failure {
		case "failed":
			bad.Status, bad.Error = "failed", "denied"
		case "truncated":
			bad.Truncated = true
		case "empty":
			bad.Output = " \n"
		}
		st.Observations = []InvestigationObservation{bad}
		require.Empty(t, InvestigationControls(st, PreparationHash("candidate")), failure)
	}
	st.Observations = []InvestigationObservation{o}
	require.Empty(t, InvestigationControls(st, PreparationHash("changed-inputs")))
	controls := InvestigationControls(st, PreparationHash("candidate"))
	require.Len(t, controls, 3)
	require.Equal(t, "finish", controls[0].Kind)
	require.Equal(t, []string{o.Invocation.ID}, controls[0].Evidence, "one observed action bound to two hypotheses is cited only once")
	st.DecisionEffects = []InvestigationDecisionEffect{{InvestigationControl: controls[1]}}
	next := InvestigationControls(st, PreparationHash("candidate"))
	require.Len(t, next, 2)
	require.Equal(t, []string{"list-source"}, next[0].Actions)
	require.False(t, knownCollectionControl(st, "invented"))
	require.True(t, knownCollectionControl(st, controls[0].ActionID))
	require.True(t, knownCollectionControl(st, controls[1].ActionID))
}

func TestCollectionEffectsAreEvidenceBoundAndCannotResumeDeferredReads(t *testing.T) {
	for name, change := range map[string]func(*PreparationState){
		"invalid time":           func(st *PreparationState) { st.DecisionEffects[0].At = "not-a-time" },
		"before task":            func(st *PreparationState) { st.DecisionEffects[0].At = "2026-11-11T00:00:00Z" },
		"beyond elapsed":         func(st *PreparationState) { st.DecisionEffects[0].At = "2026-11-12T00:00:04Z" },
		"future observation":     func(st *PreparationState) { st.DecisionEffects[0].At = "2026-11-12T00:00:01Z" },
		"future evidence prefix": func(st *PreparationState) { st.DecisionEffects[0].Observations = 2 },
		"changed binding":        func(st *PreparationState) { st.DecisionEffects[0].BindingHash = PreparationHash("other") },
		"stale candidate":        func(st *PreparationState) { st.DecisionEffects[0].CandidateHash = PreparationHash("changed-inputs") },
		"changed evidence":       func(st *PreparationState) { st.DecisionEffects[0].Evidence = []string{PreparationHash("absent")} },
		"changed action":         func(st *PreparationState) { st.DecisionEffects[0].Actions = []string{"find-default"} },
		"missing receipt":        func(st *PreparationState) { st.DecisionEffects[0].DecisionID = "absent" },
		"failed receipt":         func(st *PreparationState) { st.Decisions[0].Status = "error" },
		"advisory cannot control": func(st *PreparationState) {
			st.Contract.Decision.Policy.Mode = decision.Advisory
			st.ContractHash = PreparationHash(st.Contract)
			st.Decisions[0].PolicyHash = PreparationHash(st.Contract.Decision.Policy)
		},
		"finish resumes":    func(st *PreparationState) { st.Status, st.Reason = "ready", "resumed" },
		"repeated decision": func(st *PreparationState) { st.DecisionEffects = append(st.DecisionEffects, st.DecisionEffects[0]) },
		"executed deferred action": func(st *PreparationState) {
			observation := preparationObservation(st, 0)
			st.Observations = append(st.Observations, observation)
			st.ActionsUsed++
			st.OutputBytes += len(observation.Output)
		},
		"pending deferred action": func(st *PreparationState) {
			invocation := preparationObservation(st, 0).Invocation
			st.Pending, st.Status = &invocation, "running"
			st.ActionsUsed++
		},
	} {
		t.Run(name, func(t *testing.T) {
			st := collectionFixture(t, "finish")
			change(st)
			require.Error(t, validatePreparationState(st))
		})
	}
	st := collectionFixture(t, "defer")
	control := InvestigationControls(st, PreparationHash("candidate"))[0]
	second := st.Decisions[0]
	second.DecisionID, second.ActionID = "decision-b", control.ActionID
	st.Decisions = append(st.Decisions, second)
	st.DecisionStartedAt[second.DecisionID] = "2026-11-12T00:00:03Z"
	st.DecisionEffects = append(st.DecisionEffects, InvestigationDecisionEffect{InvestigationControl: control, DecisionID: second.DecisionID, Observations: 1, BindingHash: PreparationHash(control), CandidateHash: PreparationHash("candidate"), At: "2026-11-12T00:00:03Z"})
	st.Status, st.Reason = "awaiting-report", "jev-evidence-ready-for-report"
	require.NoError(t, validatePreparationState(st))
	started, err := time.Parse(time.RFC3339Nano, st.StartedAt)
	require.NoError(t, err)
	st.DecisionEffects[1].Observations = 0
	require.ErrorContains(t, validatePreparationEffects(st, started), "history")
}

func TestPendingCollectionChoiceRequiresTheExactCurrentEvidenceBinding(t *testing.T) {
	st := collectionFixture(t, "finish")
	control := st.DecisionEffects[0].InvestigationControl
	st.DecisionEffects = nil
	st.Status, st.Reason = "ready", "ready"
	addPreparationDecisionPending(st)
	a := st.Contract.Actions[0]
	st.DecisionPending.Actions = []decision.Action{{ID: a.ID, Kind: decision.Diagnostic, Description: a.Purpose, BindingHash: PreparationHash(a)}, {ID: control.ActionID, Kind: decision.Diagnostic, Description: "Present evidence to the analyst", BindingHash: PreparationHash(control)}}
	st.DecisionPending.BaselineID = a.ID
	require.NoError(t, validatePreparationState(st))
	st.DecisionPending.Actions[1].BindingHash = PreparationHash("unbound control")
	require.ErrorContains(t, validatePreparationState(st), "unavailable action")
}
