package state

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preparationFixture(t *testing.T) *PreparationState {
	t.Helper()
	raw, err := templates.FS.ReadFile("preparation.example.json")
	require.NoError(t, err)
	p, err := contract.ValidatePreparation(raw)
	require.NoError(t, err)
	return &PreparationState{Version: 1, Revision: 1, RunID: "run-a", Cwd: t.TempDir(), Contract: *p,
		ContractHash: PreparationHash(p), Status: "ready", Reason: "approved", StartedAt: "2026-11-12T00:00:00Z", Deadline: "2026-11-12T00:01:00Z",
		Observations: []InvestigationObservation{}, Decisions: []decision.Receipt{}}
}

func preparationObservation(st *PreparationState, action int) InvestigationObservation {
	a := st.Contract.Actions[action]
	return InvestigationObservation{
		Invocation: PreparationInvocation{ID: PreparationHash(a.ID), ActionID: a.ID, BindingHash: PreparationHash(a), CandidateHash: PreparationHash("candidate"), StartedAt: "2026-11-12T00:00:01Z", Reason: "baseline"},
		Status:     "observed", Output: "evidence", ContentHash: PreparationHash("source contents"), FinishedAt: "2026-11-12T00:00:02Z", DurationMS: 1000,
	}
}

func observedPreparation(t *testing.T) *PreparationState {
	t.Helper()
	st := preparationFixture(t)
	observation := preparationObservation(st, 0)
	st.Observations, st.ActionsUsed, st.OutputBytes, st.ElapsedMS = []InvestigationObservation{observation}, 1, len(observation.Output), 2000
	return st
}

func prepJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}

func addPreparationDecision(st *PreparationState) {
	st.Contract.Decision = &contract.InvestigationDecision{
		Policy:   decision.Policy{Version: "v1", Mode: decision.Adopt, AllowedData: []decision.Category{decision.TaskSummary, decision.DiagnosticFacts}, MinApplicability: .8, MinChoice: .8, MaxCalls: 3},
		Endpoint: "https://api.jev.example/v1/decisions", Model: "jev-1.0.0", KeyEnv: "JEV_KEY", TimeoutMS: 100,
	}
	st.ContractHash = PreparationHash(st.Contract)
	st.Decisions = []decision.Receipt{{Version: 1, DecisionID: "decision-a", SnapshotHash: PreparationHash("snapshot"), PolicyHash: PreparationHash(st.Contract.Decision.Policy), TargetHash: PreparationHash("target"), QuestionHash: PreparationHash("question"), Status: "ok", Reason: "choice", ActionID: st.Contract.Actions[0].ID, Model: st.Contract.Decision.Model, HTTPStatus: 200}}
	st.DecisionStartedAt = map[string]string{"decision-a": st.StartedAt}
}

func addPreparationDecisionPending(st *PreparationState) {
	addPreparationDecision(st)
	st.Decisions = []decision.Receipt{}
	st.DecisionStartedAt = map[string]string{"decision-pending": st.StartedAt}
	st.DecisionPending = &decision.Snapshot{Version: 1, ID: "decision-pending", Identity: decision.Identity{
		TaskID: st.Contract.ID, RunID: st.RunID, Revision: st.Revision, ContractHash: st.ContractHash, CandidateHash: PreparationHash("candidate"), ObservationHash: PreparationHash(st.Observations)},
		Facts: []decision.Fact{}, Actions: []decision.Action{}, BaselineID: st.Contract.Actions[1].ID,
	}
	for _, action := range st.Contract.Actions[1:] {
		st.DecisionPending.Actions = append(st.DecisionPending.Actions, decision.Action{ID: action.ID, Kind: decision.Diagnostic, Description: action.Purpose, BindingHash: PreparationHash(action)})
	}
}

func TestPreparationHistoryBudgetsAndActionBindingsAreChecked(t *testing.T) {
	for name, change := range map[string]func(*PreparationState){
		"invalid enum substring": func(st *PreparationState) { st.Status = "ready|running" },
		"relative cwd":           func(st *PreparationState) { st.Cwd = "repo" },
		"action budget":          func(st *PreparationState) { st.ActionsUsed = 4 },
		"output budget":          func(st *PreparationState) { st.OutputBytes = st.Contract.Limits.MaxOutputBytes + 1 },
		"bad contract path":      func(st *PreparationState) { st.Contract.ReadPaths[0] = "../outside" },
		"invalid start":          func(st *PreparationState) { st.StartedAt = "invalid" },
		"invalid deadline":       func(st *PreparationState) { st.Deadline = "invalid" },
		"extended deadline":      func(st *PreparationState) { st.Deadline = "2026-11-12T00:02:00Z" },
		"changed contract":       func(st *PreparationState) { st.ContractHash = PreparationHash("different") },
		"duplicate receipt": func(st *PreparationState) {
			st.Observations = append(st.Observations, st.Observations[0])
			st.ActionsUsed = 2
		},
		"invalid invocation time":    func(st *PreparationState) { st.Observations[0].Invocation.StartedAt = "invalid" },
		"invocation before run":      func(st *PreparationState) { st.Observations[0].Invocation.StartedAt = "2026-11-11T00:00:00Z" },
		"action binding changed":     func(st *PreparationState) { st.Observations[0].Invocation.BindingHash = PreparationHash("changed") },
		"action not approved":        func(st *PreparationState) { st.Observations[0].Invocation.ActionID = "unapproved" },
		"missing adopted decision":   func(st *PreparationState) { st.Observations[0].Invocation.Adopted = true },
		"unknown decision reference": func(st *PreparationState) { st.Observations[0].Invocation.DecisionID = "absent" },
		"missing content identity":   func(st *PreparationState) { st.Observations[0].ContentHash = "" },
		"successful error":           func(st *PreparationState) { st.Observations[0].Error = "failed" },
		"failed without reason":      func(st *PreparationState) { st.Observations[0].Status = "failed" },
		"finished before invocation": func(st *PreparationState) { st.Observations[0].FinishedAt = st.StartedAt },
		"invalid finish":             func(st *PreparationState) { st.Observations[0].FinishedAt = "invalid" },
		"elapsed mismatch":           func(st *PreparationState) { st.ElapsedMS = 1000 },
		"duration mismatch":          func(st *PreparationState) { st.Observations[0].DurationMS = 3000 },
		"missing action accounting":  func(st *PreparationState) { st.ActionsUsed = 0 },
		"missing output accounting":  func(st *PreparationState) { st.OutputBytes = 0 },
		"pending repeats known action": func(st *PreparationState) {
			p := st.Observations[0].Invocation
			st.Pending = &p
			st.ActionsUsed = 2
			st.Status = "running"
		},
		"invalid pending binding": func(st *PreparationState) {
			p := preparationObservation(st, 1).Invocation
			p.BindingHash = PreparationHash("different")
			st.Pending = &p
			st.ActionsUsed = 2
			st.Status = "running"
		},
		"terminal without report": func(st *PreparationState) { st.Status = "supported" },
		"running without intent":  func(st *PreparationState) { st.Status = "running" },
	} {
		t.Run(name, func(t *testing.T) {
			st := observedPreparation(t)
			change(st)
			require.Error(t, validatePreparationState(st))
		})
	}
	st := observedPreparation(t)
	require.NoError(t, validatePreparationState(st))
	pending := preparationObservation(st, 1).Invocation
	st.Pending, st.ActionsUsed, st.Status = &pending, 2, "running"
	require.NoError(t, validatePreparationState(st), "an unresolved action stays counted and recoverable")
	st.Status = "cancelled"
	require.Error(t, validatePreparationState(st), "cancellation cannot discard an unknown action")
	st.Status = "ready"
	require.Error(t, validatePreparationState(st))
}

func TestPreparationDecisionHistoryCannotEraseUnknownUsageOrAlterBindings(t *testing.T) {
	for name, change := range map[string]func(*PreparationState){
		"missing dispatch time":      func(st *PreparationState) { st.DecisionStartedAt = nil },
		"invalid dispatch time":      func(st *PreparationState) { st.DecisionStartedAt["decision-a"] = "invalid" },
		"dispatch before run":        func(st *PreparationState) { st.DecisionStartedAt["decision-a"] = "2026-11-11T00:00:00Z" },
		"mismatched time key":        func(st *PreparationState) { st.DecisionStartedAt = map[string]string{"other": st.StartedAt} },
		"duplicate receipt":          func(st *PreparationState) { st.Decisions = append(st.Decisions, st.Decisions[0]) },
		"decision without policy":    func(st *PreparationState) { st.Contract.Decision = nil; st.ContractHash = PreparationHash(st.Contract) },
		"different model":            func(st *PreparationState) { st.Decisions[0].Model = "jev-2.0.0" },
		"different policy":           func(st *PreparationState) { st.Decisions[0].PolicyHash = PreparationHash("other") },
		"missing frozen request":     func(st *PreparationState) { st.Decisions[0].SnapshotHash = "" },
		"unapproved selected action": func(st *PreparationState) { st.Decisions[0].ActionID = "unapproved" },
		"duration exceeds history":   func(st *PreparationState) { st.Decisions[0].DurationMS = 3000 },
		"invalid JSON numeric value": func(st *PreparationState) { st.Decisions[0].Probabilities = map[string]float64{"x": math.NaN()} },
		"adopted wrong action": func(st *PreparationState) {
			st.Observations[0].Invocation.DecisionID = "decision-a"
			st.Observations[0].Invocation.Adopted = true
			st.Decisions[0].ActionID = st.Contract.Actions[1].ID
		},
		"adopted unknown result": func(st *PreparationState) {
			st.Observations[0].Invocation.DecisionID = "decision-a"
			st.Observations[0].Invocation.Adopted = true
			st.Decisions[0].Status = "unknown"
		},
		"unknown calls count toward budget": func(st *PreparationState) {
			st.Contract.Decision.Policy.MaxCalls = 1
			st.ContractHash = PreparationHash(st.Contract)
			st.Decisions[0].Status = "unknown"
			second := st.Decisions[0]
			second.DecisionID = "decision-b"
			st.Decisions = append(st.Decisions, second)
			st.DecisionStartedAt["decision-b"] = st.StartedAt
		},
	} {
		t.Run(name, func(t *testing.T) {
			st := observedPreparation(t)
			addPreparationDecision(st)
			change(st)
			require.Error(t, validatePreparationState(st))
		})
	}
	st := observedPreparation(t)
	addPreparationDecision(st)
	st.Observations[0].Invocation.DecisionID, st.Observations[0].Invocation.Adopted = "decision-a", true
	require.NoError(t, validatePreparationState(st))
	st.Observations[0].Invocation.Adopted = false
	st.Decisions[0].Status = "skipped"
	require.NoError(t, validatePreparationState(st), "a baseline action may reference declined optional advice")
	st.Contract.Decision = nil
	st.ContractHash = PreparationHash(st.Contract)
	require.Error(t, validatePreparationState(st))
}

func TestPreparationPendingDecisionIsARecoverableFrozenIntent(t *testing.T) {
	st := observedPreparation(t)
	addPreparationDecisionPending(st)
	require.NoError(t, validatePreparationState(st))
	for name, change := range map[string]func(*PreparationState){
		"action and decision unresolved": func(st *PreparationState) {
			inv := preparationObservation(st, 1).Invocation
			st.Pending = &inv
			st.ActionsUsed = 2
			st.Status = "running"
		},
		"different task":       func(st *PreparationState) { st.DecisionPending.Identity.TaskID = "other" },
		"future revision":      func(st *PreparationState) { st.DecisionPending.Identity.Revision++ },
		"changed observations": func(st *PreparationState) { st.DecisionPending.Identity.ObservationHash = PreparationHash("other") },
		"no policy":            func(st *PreparationState) { st.Contract.Decision = nil; st.ContractHash = PreparationHash(st.Contract) },
		"unknown baseline":     func(st *PreparationState) { st.DecisionPending.BaselineID = "other" },
		"changed binding":      func(st *PreparationState) { st.DecisionPending.Actions[0].BindingHash = PreparationHash("changed") },
		"completed action offered": func(st *PreparationState) {
			a := st.Contract.Actions[0]
			st.DecisionPending.Actions[0] = decision.Action{ID: a.ID, Kind: decision.Diagnostic, Description: a.Purpose, BindingHash: PreparationHash(a)}
			st.DecisionPending.BaselineID = a.ID
		},
		"already recorded decision": func(st *PreparationState) {
			st.Decisions = []decision.Receipt{{Version: 1, DecisionID: st.DecisionPending.ID, Status: "unknown", Reason: "reserved"}}
		},
		"pending exceeds remaining call budget": func(st *PreparationState) {
			st.Contract.Decision.Policy.MaxCalls = 1
			st.ContractHash = PreparationHash(st.Contract)
			st.DecisionPending.Identity.ContractHash = st.ContractHash
			st.Decisions = []decision.Receipt{{Version: 1, DecisionID: "prior", Status: "unknown", Reason: "reserved"}}
			st.DecisionStartedAt["prior"] = st.StartedAt
		},
	} {
		t.Run(name, func(t *testing.T) {
			st := observedPreparation(t)
			addPreparationDecisionPending(st)
			change(st)
			require.Error(t, validatePreparationState(st))
		})
	}
}

func supportedPreparation(t *testing.T) *PreparationState {
	t.Helper()
	st := observedPreparation(t)
	st.Status = "supported"
	st.Report = &InvestigationReport{Status: "supported", Summary: "Supported diagnosis", CounterEvidence: "Alternative unconfirmed", Recommendation: "Review repair", Regression: "Add reproducer", Rollback: "Restore setting", Limitations: []string{}, Findings: []InvestigationFinding{
		{Hypothesis: st.Contract.Hypotheses[0].ID, Status: "unknown", Reason: "Not yet falsified", Evidence: []string{}},
		{Hypothesis: st.Contract.Hypotheses[1].ID, Status: "supported", Reason: "Configuration observation", Evidence: []string{st.Observations[0].Invocation.ID}},
	}}
	return st
}

func TestPreparationReportKeepsDiagnosisSeparateFromImplementationProof(t *testing.T) {
	require.NoError(t, validatePreparationState(supportedPreparation(t)))
	for name, change := range map[string]func(*PreparationState){
		"mismatched terminal status": func(st *PreparationState) { st.Status = "inconclusive" },
		"blank summary":              func(st *PreparationState) { st.Report.Summary = "  " },
		"missing hypothesis":         func(st *PreparationState) { st.Report.Findings = st.Report.Findings[1:] },
		"duplicate hypothesis":       func(st *PreparationState) { st.Report.Findings[1].Hypothesis = st.Report.Findings[0].Hypothesis },
		"no supporting citation":     func(st *PreparationState) { st.Report.Findings[1].Evidence = []string{} },
		"absent observation":         func(st *PreparationState) { st.Report.Findings[1].Evidence = []string{PreparationHash("absent")} },
		"failed observation": func(st *PreparationState) {
			st.Observations[0].Status = "failed"
			st.Observations[0].Error = "access denied"
		},
		"no supported finding":           func(st *PreparationState) { st.Report.Findings[1].Status = "unknown" },
		"inconclusive lacks limitations": func(st *PreparationState) { st.Report.Status = "inconclusive"; st.Status = "inconclusive" },
	} {
		t.Run(name, func(t *testing.T) {
			st := supportedPreparation(t)
			change(st)
			require.Error(t, validatePreparationState(st))
		})
	}
	st := supportedPreparation(t)
	st.Report.Status, st.Status, st.Report.Limitations = "inconclusive", "inconclusive", []string{"Needs another observation"}
	require.NoError(t, validatePreparationState(st))
}

func TestPreparationStateCASCreatesOnlyAbsentRevisionZero(t *testing.T) {
	dir := t.TempDir()
	st := preparationFixture(t)
	st.Revision = 0
	next, err := SavePreparation(dir, st)
	require.NoError(t, err)
	assert.EqualValues(t, 1, next.Revision)
	assert.Zero(t, st.Revision, "caller snapshot is not mutated on save")
	loaded, err := LoadPreparation(dir)
	require.NoError(t, err)
	assert.Equal(t, next, loaded)
	before, err := os.ReadFile(PreparationFile(dir))
	require.NoError(t, err)
	_, err = SavePreparation(dir, st)
	var conflict *CasConflictError
	require.ErrorAs(t, err, &conflict)
	after, err := os.ReadFile(PreparationFile(dir))
	require.NoError(t, err)
	assert.Equal(t, before, after)
	_, err = SavePreparation(t.TempDir(), next)
	require.ErrorAs(t, err, &conflict, "a vanished record cannot be recreated from a stale snapshot")

	zeroDir := t.TempDir()
	require.NoError(t, os.WriteFile(PreparationFile(zeroDir), prepJSON(t, st), 0o600))
	_, err = SavePreparation(zeroDir, st)
	require.Error(t, err, "an existing record with revision zero is not absent")
	zeroAfter, err := os.ReadFile(PreparationFile(zeroDir))
	require.NoError(t, err)
	assert.Equal(t, prepJSON(t, st), zeroAfter)
	st.Revision = -1
	_, err = SavePreparation(dir, st)
	require.Error(t, err)
	st.Revision = math.MaxInt64
	_, err = SavePreparation(dir, st)
	require.Error(t, err)
}

func TestPreparationSavePropagatesLockValidationAndPersistenceFailures(t *testing.T) {
	dir := t.TempDir()
	st := preparationFixture(t)
	st.Revision = 0
	st.Status = "invalid"
	_, err := SavePreparation(dir, st)
	require.Error(t, err)
	st.Status = "ready"
	original := lockStateFile
	sentinel := errors.New("CAS lock unavailable")
	lockStateFile = func(string) (func(), error) { return nil, sentinel }
	_, err = SavePreparation(dir, st)
	require.ErrorIs(t, err, sentinel)
	lockStateFile = original
	t.Cleanup(func() { lockStateFile = original })
	require.NoError(t, os.WriteFile(PreparationFile(dir), []byte("{"), 0o600))
	_, err = SavePreparation(dir, st)
	require.Error(t, err)
	require.NoError(t, os.Remove(PreparationFile(dir)))
	tmp := PreparationFile(dir) + ".tmp-" + fmtPID()
	require.NoError(t, os.WriteFile(tmp, []byte("unknown partial write"), 0o600))
	_, err = SavePreparation(dir, st)
	require.Error(t, err)
	content, err := os.ReadFile(tmp)
	require.NoError(t, err)
	assert.Equal(t, "unknown partial write", string(content))
}

func TestLoadPreparationRejectsMalformedRequiredAndCrossRecordFacts(t *testing.T) {
	dir := t.TempDir()
	st := preparationFixture(t)
	valid := prepJSON(t, st)
	for _, raw := range [][]byte{[]byte("{"), []byte(`{}`), []byte(strings.Replace(string(valid), `"revision":1`, `"revision":1.0`, 1))} {
		require.NoError(t, os.WriteFile(PreparationFile(dir), raw, 0o600))
		_, err := LoadPreparation(dir)
		require.Error(t, err)
	}
	st.ActionsUsed = 1
	require.NoError(t, os.WriteFile(PreparationFile(dir), prepJSON(t, st), 0o600))
	_, err := LoadPreparation(dir)
	require.Error(t, err)
	_, err = LoadPreparation(t.TempDir())
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestPreparationObservationTimesCannotBeInvented(t *testing.T) {
	st := preparationFixture(t)
	o := preparationObservation(st, 0)
	o.Invocation.StartedAt = "invalid"
	require.Error(t, validatePreparationObservation(o))
	o = preparationObservation(st, 0)
	o.Status, o.Error, o.ContentHash = "cancelled", "context cancelled", ""
	require.NoError(t, validatePreparationObservation(o))
	o.Status = "arbitrary"
	require.Error(t, validatePreparationObservation(o))
	started, err := time.Parse(time.RFC3339Nano, st.StartedAt)
	require.NoError(t, err)
	inv := preparationObservation(st, 0).Invocation
	require.NoError(t, validatePreparationInvocation(st, inv, started))
}
