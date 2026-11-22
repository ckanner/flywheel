package state

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func runtimeLedgerFixture(t *testing.T) *State {
	t.Helper()
	j := config.Default().JEV
	j.Enabled, j.Endpoint = true, "https://jev.example.invalid/v1/systemone"
	snapshot := &decision.Snapshot{Version: 2, ID: "failure-M01-T01-0-1-1", Identity: decision.Identity{TaskID: "runtime-task", RunID: PreparationHash("task-dir"), Revision: 7, ContractHash: PreparationHash("contract"), CandidateHash: PreparationHash("candidate"), ObservationHash: PreparationHash("observation")},
		Facts: []decision.Fact{{Category: decision.DiagnosticFacts, Text: "The approved L1 check exited 1 with complete output."}}, MissingFacts: []string{"Root cause is not yet established."}, BaselineID: "repair", SemanticUncertainty: true}
	for _, action := range []struct {
		id   string
		kind decision.ActionKind
	}{{"repair", decision.Repair}, {"model-1", decision.Model}, {"replan", decision.Replan}, {"feedback-0", decision.Diagnostic}, {"recheck-0", decision.Diagnostic}} {
		snapshot.Actions = append(snapshot.Actions, decision.Action{ID: action.id, Kind: action.kind, Description: "Approved " + action.id, BindingHash: PreparationHash(action.id)})
	}
	r := &RuntimeDecisionRecord{ID: snapshot.ID, Packet: "M01-T01", Attempt: 1, StartedAt: "2026-11-13T00:00:00Z", Snapshot: snapshot, Action: snapshot.BaselineID, Status: "pending"}
	st := &State{SchemaVersion: 1, Slug: "runtime-task", Revision: 8, Status: "running", Packets: map[string]*PacketState{"M01-T01": {Attempts: 1, FailedAttempts: 1, PendingStrategy: r.ID}},
		RuntimeDecisions: &RuntimeDecisions{Version: 1, Config: &j, Used: 1, Status: "enabled", Records: []*RuntimeDecisionRecord{r}}}
	require.NoError(t, validateRuntimeDecisions(st))
	return st
}

func initialLedgerFixture(t *testing.T) *State {
	t.Helper()
	st := runtimeLedgerFixture(t)
	r := st.RuntimeDecisions.Records[0]
	r.ID, r.Attempt = "initial-"+r.Packet, 0
	r.Snapshot.ID, r.Snapshot.Version = r.ID, 3
	r.Snapshot.Actions = []decision.Action{
		{ID: "model-0", Kind: decision.Model, Description: "Configured baseline", BindingHash: PreparationHash("initial binding")},
		{ID: "model-1", Kind: decision.Model, Description: "Approved alternative", BindingHash: PreparationHash("alternate binding")},
	}
	r.Action, r.Snapshot.BaselineID = "model-0", "model-0"
	ps := st.Packets[r.Packet]
	ps.PendingStrategy, ps.Attempts, ps.FailedAttempts = r.ID, 0, 0
	require.NoError(t, validateRuntimeDecisions(st))
	return st
}

func TestInitialDecisionLedgerRequiresFirstExecutionAndRoundTripsEffects(t *testing.T) {
	for _, adopted := range []bool{false, true} {
		st := initialLedgerFixture(t)
		action, status := "model-0", "baseline"
		if adopted {
			action, status = "model-1", "applied"
		}
		settleRuntimeRecord(t, st, action, status, adopted)
		st.Packets["M01-T01"].Attempts = 1
		require.NoError(t, validateRuntimeDecisions(st), "a settled first choice remains valid after executing")
	}
	for name, mutate := range map[string]func(*State){
		"non-initial id": func(st *State) {
			r := st.RuntimeDecisions.Records[0]
			r.ID, r.Snapshot.ID, st.Packets[r.Packet].PendingStrategy = "another-id", "another-id", "another-id"
		},
		"after an attempt":            func(st *State) { st.RuntimeDecisions.Records[0].Attempt = 1 },
		"after a replan":              func(st *State) { st.RuntimeDecisions.Records[0].PlanRevisions = 1 },
		"executor already dispatched": func(st *State) { st.Packets["M01-T01"].Attempts = 1 },
		"plan already revised":        func(st *State) { st.Packets["M01-T01"].PlanRevisions = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			st := initialLedgerFixture(t)
			mutate(st)
			require.ErrorContains(t, validateRuntimeDecisions(st), "first execution")
		})
	}
}

func settleRuntimeRecord(t *testing.T, st *State, action, status string, adopted bool) {
	t.Helper()
	ledger := st.RuntimeDecisions
	r := ledger.Records[0]
	j := ledger.Config
	options := decision.Options{Endpoint: j.Endpoint, Model: j.Model, ExpectedModel: j.Model, Timeout: time.Duration(j.TimeoutMS) * time.Millisecond, AllowedKinds: []decision.ActionKind{decision.Diagnostic, decision.Repair, decision.Replan, decision.Model}}
	receipt, found, err := decision.Recover(filepath.Join(t.TempDir(), "absent-journal"), *r.Snapshot, j.DecisionPolicy(), options)
	require.NoError(t, err)
	require.False(t, found)
	selection := decision.Selection{ActionID: r.Snapshot.BaselineID, Reason: "recovered-without-adoption"}
	if adopted {
		receipt.Status, receipt.Reason, receipt.HTTPStatus, receipt.Model, receipt.ActionID = "ok", "native-response", 200, j.Model, action
		receipt.Applicability, receipt.ChoiceConfidence = new(float64(1)), new(float64(1))
		receipt.Usage = decision.Usage{Input: new(int64(17)), Output: new(int64(3))}
		receipt.Probabilities = map[string]float64{}
		for _, candidate := range r.Snapshot.Actions {
			receipt.Probabilities[candidate.ID] = 0
		}
		receipt.Probabilities[action] = 1
		selection = decision.Select(*r.Snapshot, receipt, j.DecisionPolicy(), options)
		require.True(t, selection.Adopted)
	}
	r.Receipt, r.Selection, r.Accounted, r.Status, r.Action = &receipt, &selection, true, status, action
	st.Packets[r.Packet].PendingStrategy = ""
	if status == "pending" || status == "feedback-running" || status == "feedback-unknown" || status == "feedback-stale" || status == "replan-running" || (status == "applied" && action == "replan") {
		st.Packets[r.Packet].PendingStrategy = r.ID
	}
	if status == "feedback-completed" || status == "feedback-unknown" || status == "feedback-stale" {
		r.Feedback = map[string]any{"pass": false, "results": []any{map[string]any{"cmd": "approved test", "exitCode": 1, "pass": false}}}
	}
}

func TestRuntimeLedgerRoundTripsEveryLegalRecoveryBoundary(t *testing.T) {
	for _, scenario := range []struct {
		name, action, status string
		adopted              bool
	}{
		{"pending reservation", "repair", "pending", false},
		{"accounted pending baseline", "repair", "pending", false},
		{"unknown keeps baseline", "repair", "baseline", false},
		{"cancelled proposal remains audit", "model-1", "baseline", true},
		{"adopted repair", "repair", "applied", true},
		{"adopted model", "model-1", "applied", true},
		{"adopted replan", "replan", "applied", true},
		{"deterministic replan", "replan", "applied", false},
		{"planner dispatch intent", "replan", "replan-running", true},
		{"planner completed", "replan", "completed", true},
		{"changed candidate retains proposal", "model-1", "stale", true},
		{"stale baseline", "repair", "stale", false},
		{"feedback dispatch intent", "feedback-0", "feedback-running", true},
		{"feedback completed", "feedback-0", "feedback-completed", true},
		{"feedback unknown", "recheck-0", "feedback-unknown", true},
		{"feedback changed candidate", "recheck-0", "feedback-stale", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			st := runtimeLedgerFixture(t)
			if scenario.name == "deterministic replan" {
				st.RuntimeDecisions.Records[0].Snapshot.BaselineID = "replan"
			}
			if scenario.name != "pending reservation" {
				settleRuntimeRecord(t, st, scenario.action, scenario.status, scenario.adopted)
			}
			if scenario.status == "replan-running" || scenario.status == "completed" {
				st.RuntimeDecisions.Records[0].PlanInvocationBefore = "call-before"
			}
			require.NoError(t, validateRuntimeDecisions(st))
			dir := t.TempDir()
			_, err := Save(dir, st, nil)
			require.NoError(t, err)
			loaded, err := Load(dir)
			require.NoError(t, err)
			require.Equal(t, PreparationHash(st.RuntimeDecisions), PreparationHash(loaded.RuntimeDecisions))
			require.Equal(t, st.Packets["M01-T01"].PendingStrategy, loaded.Packets["M01-T01"].PendingStrategy)
		})
	}
}

func TestRuntimeLedgerRejectsConflictingIdentityAuthorityAndAccounting(t *testing.T) {
	for name, mutate := range map[string]func(*State){
		"schema version":        func(st *State) { st.RuntimeDecisions.Version = 2 },
		"missing frozen config": func(st *State) { st.RuntimeDecisions.Config = nil },
		"nil record":            func(st *State) { st.RuntimeDecisions.Records[0] = nil },
		"missing snapshot":      func(st *State) { st.RuntimeDecisions.Records[0].Snapshot = nil },
		"duplicate record": func(st *State) {
			st.RuntimeDecisions.Records = append(st.RuntimeDecisions.Records, st.RuntimeDecisions.Records[0])
		},
		"other packet":          func(st *State) { st.RuntimeDecisions.Records[0].Packet = "M02-T01" },
		"snapshot decision":     func(st *State) { st.RuntimeDecisions.Records[0].Snapshot.ID = "other-decision" },
		"snapshot task":         func(st *State) { st.RuntimeDecisions.Records[0].Snapshot.Identity.TaskID = "other-task" },
		"legacy snapshot":       func(st *State) { st.RuntimeDecisions.Records[0].Snapshot.Version = 1 },
		"future snapshot":       func(st *State) { st.RuntimeDecisions.Records[0].Snapshot.Identity.Revision = st.Revision + 1 },
		"invalid time":          func(st *State) { st.RuntimeDecisions.Records[0].StartedAt = "yesterday" },
		"invalid fact":          func(st *State) { st.RuntimeDecisions.Records[0].Snapshot.Facts[0].Category = "secrets" },
		"deterministic request": func(st *State) { st.RuntimeDecisions.Records[0].Snapshot.SemanticUncertainty = false },
		"unknown executor":      func(st *State) { st.RuntimeDecisions.Records[0].Snapshot.ExecutorUnknown = true },
		"single legal action": func(st *State) {
			st.RuntimeDecisions.Records[0].Snapshot.Actions = st.RuntimeDecisions.Records[0].Snapshot.Actions[:1]
		},
		"unimplemented action":    func(st *State) { st.RuntimeDecisions.Records[0].Snapshot.Actions[1].ID = "delete-evidence" },
		"wrong family":            func(st *State) { st.RuntimeDecisions.Records[0].Snapshot.Actions[0].Kind = decision.Model },
		"absent selected action":  func(st *State) { st.RuntimeDecisions.Records[0].Action = "not-approved" },
		"settled still pending":   func(st *State) { st.Packets["M01-T01"].PendingStrategy = st.RuntimeDecisions.Records[0].ID },
		"missing receipt":         func(st *State) { st.RuntimeDecisions.Records[0].Receipt = nil },
		"missing selection":       func(st *State) { st.RuntimeDecisions.Records[0].Selection = nil },
		"unaccounted terminal":    func(st *State) { st.RuntimeDecisions.Records[0].Accounted = false },
		"different selection":     func(st *State) { st.RuntimeDecisions.Records[0].Selection.ActionID = "replan" },
		"empty selection reason":  func(st *State) { st.RuntimeDecisions.Records[0].Selection.Reason = " " },
		"receipt identity":        func(st *State) { st.RuntimeDecisions.Records[0].Receipt.DecisionID = "another" },
		"receipt target":          func(st *State) { st.RuntimeDecisions.Config.Endpoint = "http://example.invalid/v1/systemone" },
		"unknown receipt adopted": func(st *State) { st.RuntimeDecisions.Records[0].Selection.Adopted = true },
		"invented invocation":     func(st *State) { st.RuntimeDecisions.Records[0].Selection.InvocationID = "forged" },
		"baseline changes action": func(st *State) {
			r := st.RuntimeDecisions.Records[0]
			r.Action, r.Selection.ActionID = "model-1", "model-1"
		},
		"feedback on repair":        func(st *State) { st.RuntimeDecisions.Records[0].Feedback = map[string]any{} },
		"completed repair":          func(st *State) { st.RuntimeDecisions.Records[0].Status = "completed" },
		"unadopted applied repair":  func(st *State) { st.RuntimeDecisions.Records[0].Status = "applied" },
		"replan boundary on repair": func(st *State) { st.RuntimeDecisions.Records[0].PlanInvocationBefore = "call-before" },
		"replan status on repair": func(st *State) {
			r := st.RuntimeDecisions.Records[0]
			r.Status = "replan-running"
			st.Packets[r.Packet].PendingStrategy = r.ID
		},
		"call count reset": func(st *State) { st.RuntimeDecisions.Used = 0 },
		"orphan pending":   func(st *State) { st.Packets["M01-T01"].PendingStrategy = "absent" },
	} {
		t.Run(name, func(t *testing.T) {
			st := runtimeLedgerFixture(t)
			settleRuntimeRecord(t, st, "repair", "baseline", false)
			mutate(st)
			require.Error(t, validateRuntimeDecisions(st))
			dir := t.TempDir()
			_, err := Save(dir, st, nil)
			require.NoError(t, err)
			_, err = Load(dir)
			require.Error(t, err, "loading must reject before machine dispatch")
		})
	}
}

func TestRuntimeLedgerPendingAndEffectCannotBeReassigned(t *testing.T) {
	for _, scenario := range []string{"missing-reference", "wrong-packet", "no-ledger", "feedback-before-dispatch", "action-before-decision", "unaccounted-receipt", "accounted-no-receipt", "adopted-before-effect", "changed-adoption", "feedback-no-result", "feedback-unadopted", "feedback-wrong-kind", "applied-diagnostic"} {
		t.Run(scenario, func(t *testing.T) {
			st := runtimeLedgerFixture(t)
			r := st.RuntimeDecisions.Records[0]
			switch scenario {
			case "missing-reference":
				st.Packets[r.Packet].PendingStrategy = ""
			case "wrong-packet":
				st.Packets["M01-T02"] = &PacketState{PendingStrategy: r.ID}
			case "no-ledger":
				st.RuntimeDecisions = nil
			case "feedback-before-dispatch":
				r.Feedback = map[string]any{}
			case "action-before-decision":
				r.Action = "model-1"
			case "unaccounted-receipt":
				settleRuntimeRecord(t, st, "repair", "pending", false)
				r.Accounted = false
			case "accounted-no-receipt":
				r.Accounted = true
			case "adopted-before-effect":
				settleRuntimeRecord(t, st, "repair", "pending", true)
			case "changed-adoption":
				settleRuntimeRecord(t, st, "model-1", "applied", true)
				r.Selection.InvocationID = "other"
			case "feedback-no-result":
				settleRuntimeRecord(t, st, "feedback-0", "feedback-completed", true)
				r.Feedback = nil
			case "feedback-unadopted":
				r.Snapshot.BaselineID = "feedback-0"
				settleRuntimeRecord(t, st, "feedback-0", "feedback-completed", false)
			case "feedback-wrong-kind":
				settleRuntimeRecord(t, st, "model-1", "feedback-completed", true)
			case "applied-diagnostic":
				settleRuntimeRecord(t, st, "feedback-0", "applied", true)
			}
			require.Error(t, validateRuntimeDecisions(st))
		})
	}
}

func TestRuntimeLedgerCountsUnknownAndImportedButNotUndispatchedCalls(t *testing.T) {
	st := runtimeLedgerFixture(t)
	settleRuntimeRecord(t, st, "repair", "baseline", false)
	st.RuntimeDecisions.ImportedCalls, st.RuntimeDecisions.Used = 3, 4
	require.NoError(t, validateRuntimeDecisions(st))
	r := st.RuntimeDecisions.Records[0].Receipt
	r.Status, r.Reason, r.Usage = "skipped", "cancelled-before-dispatch", decision.Usage{Input: new(int64(0)), Output: new(int64(0))}
	st.RuntimeDecisions.Used = 3
	require.NoError(t, validateRuntimeDecisions(st))
	st.RuntimeDecisions.Records = []*RuntimeDecisionRecord{}
	require.NoError(t, validateRuntimeDecisions(st), "an initialized ledger may only contain imported usage")
	st.Packets["nil-entry"] = nil
	st.RuntimeDecisions = nil
	require.NoError(t, validateRuntimeDecisions(st), "legacy tasks remain loadable")
}

func TestRuntimeLedgerValidationReportsSchemaAndSerializationFailures(t *testing.T) {
	st := runtimeLedgerFixture(t)
	st.RuntimeDecisions.Records[0].Feedback = make(chan int)
	require.Error(t, validateRuntimeDecisions(st))
	saved := runtimeStateSchema
	t.Cleanup(func() { runtimeStateSchema = saved })
	runtimeStateSchema = func() (*jsonschema.Schema, error) { return nil, errors.New("schema unavailable") }
	require.ErrorContains(t, validateRuntimeDecisions(st), "schema unavailable")
}

func TestRuntimeActionIdentifiersBindToTheirImplementedFamily(t *testing.T) {
	for _, action := range []decision.Action{{ID: "repair", Kind: decision.Repair}, {ID: "replan", Kind: decision.Replan}, {ID: "model-0", Kind: decision.Model}, {ID: "feedback-12", Kind: decision.Diagnostic}, {ID: "recheck-1", Kind: decision.Diagnostic}} {
		require.True(t, validRuntimeAction(action))
	}
	for _, action := range []decision.Action{{ID: "arbitrary", Kind: decision.Diagnostic}, {ID: "repair", Kind: decision.Diagnostic}, {ID: "replan", Kind: decision.Model}, {ID: "model--1", Kind: decision.Model}, {ID: "model-01", Kind: decision.Model}, {ID: "model-x", Kind: decision.Model}, {ID: "model-1", Kind: decision.Repair}, {ID: "feedback-0", Kind: decision.Model}, {ID: "recheck-0", Kind: decision.Repair}} {
		require.False(t, validRuntimeAction(action))
	}
}
