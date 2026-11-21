package investigation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func settledRoundFixture(t *testing.T) (string, *state.PreparationState) {
	t.Helper()
	dir, cwd, p := preparationFixture(t)
	p.Limits.MaxActions = 12
	startFixture(t, dir, cwd, p)
	st, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	st, err = Finish(dir, reportFixture(st), Options{})
	require.NoError(t, err)
	return dir, st
}

func newRoundContract(st *state.PreparationState) contract.Preparation {
	p := st.Contract
	p.KnownEvidence = []string{"Additional log supplied by the user; recheck the approved source against it."}
	p.Limits = state.PreparationRemaining(st)
	return p
}

func roundJSON(t *testing.T, p contract.Preparation) []byte {
	t.Helper()
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	return raw
}

func TestContinuePreservesExactReportsAndReadsNewEvidenceWithinRemainingBudget(t *testing.T) {
	dir, first := settledRoundFixture(t)
	firstHash := state.PreparationHash(first)
	later := time.Now().Add(24 * time.Hour)
	opts := Options{ExpectedRevision: first.Revision, Now: func() time.Time { return later }}
	second, err := Continue(dir, roundJSON(t, newRoundContract(first)), opts)
	require.NoError(t, err)
	require.Equal(t, first.RunID, second.RunID)
	require.Equal(t, first.Revision+1, second.Revision)
	require.Equal(t, firstHash, second.History[0].Digest)
	require.Equal(t, first.Report, second.History[0].Snapshot.Report)
	require.Nil(t, second.Report)
	require.Equal(t, state.PreparationRemaining(first), second.Contract.Limits)
	opts.ExpectedRevision = second.Revision
	second, err = Run(context.Background(), dir, opts)
	require.NoError(t, err)
	require.Equal(t, "awaiting-report", second.Status)
	require.NotEqual(t, first.Observations[0].Invocation.ID, second.Observations[0].Invocation.ID)
	// An explicit new round can repeat an approved read; it cannot reuse a prior
	// dispatch receipt or replace its original observed bytes.
	report := reportFixture(second)
	report.Findings[0].Evidence = append(report.Findings[0].Evidence, first.Observations[0].Invocation.ID)
	opts.ExpectedRevision = second.Revision
	second, err = Finish(dir, report, opts)
	require.NoError(t, err)
	actions, output, elapsed := state.PreparationTotals(second)
	require.Equal(t, first.ActionsUsed+second.ActionsUsed, actions)
	require.Equal(t, first.OutputBytes+second.OutputBytes, output)
	require.Equal(t, first.ElapsedMS+second.ElapsedMS, elapsed)
	third, err := Continue(dir, roundJSON(t, newRoundContract(second)), Options{ExpectedRevision: second.Revision, Now: func() time.Time { return later.Add(time.Hour) }})
	require.NoError(t, err, "a report citing first-round evidence remains valid when archived again")
	require.Len(t, third.History, 2)
	require.Equal(t, firstHash, third.History[0].Digest)
	require.NoError(t, validateReport(third, report))
}

func TestContinueRequiresExplicitCurrentRevisionSettledReportAndNewBasis(t *testing.T) {
	dir, st := settledRoundFixture(t)
	raw := roundJSON(t, newRoundContract(st))
	_, err := Continue(dir, raw, Options{})
	require.ErrorContains(t, err, "exact current revision")
	_, err = Continue(dir, []byte("{"), Options{ExpectedRevision: st.Revision})
	require.Error(t, err)
	_, err = Continue(dir, raw, Options{ExpectedRevision: st.Revision + 1})
	require.ErrorContains(t, err, "stale")
	_, err = Continue(t.TempDir(), raw, Options{ExpectedRevision: st.Revision})
	require.Error(t, err)
	for name, mutate := range map[string]func(*contract.Preparation){
		"new identity": func(p *contract.Preparation) { p.ID = "different-task" },
		"new goal":     func(p *contract.Preparation) { p.Goal = "Change a different subsystem" },
		"new repair authority": func(p *contract.Preparation) {
			p.Intent, p.ImplementationScope = "investigate-and-fix", []string{"src"}
		},
		"missing basis":       func(p *contract.Preparation) { p.KnownEvidence = []string{} },
		"reset actions":       func(p *contract.Preparation) { p.Limits.MaxActions++ },
		"reset time":          func(p *contract.Preparation) { p.Limits.MaxSeconds++ },
		"reset output":        func(p *contract.Preparation) { p.Limits.MaxOutputBytes++ },
		"increased file read": func(p *contract.Preparation) { p.Limits.MaxFileBytes++ },
	} {
		t.Run(name, func(t *testing.T) {
			p := newRoundContract(st)
			mutate(&p)
			_, err := Continue(dir, roundJSON(t, p), Options{ExpectedRevision: st.Revision})
			require.Error(t, err)
		})
	}
	// Preserve a pre-existing repair grant exactly as well; Continue is not a
	// way to broaden or retarget it by keeping only the task ID unchanged.
	dirWithRepair, cwd, p := preparationFixture(t)
	p.Intent, p.ImplementationScope, p.Limits.MaxActions = "investigate-and-fix", []string{"src"}, 12
	startFixture(t, dirWithRepair, cwd, p)
	repair, err := Run(context.Background(), dirWithRepair, Options{})
	require.NoError(t, err)
	repair, err = Finish(dirWithRepair, reportFixture(repair), Options{})
	require.NoError(t, err)
	for _, scope := range [][]string{{"src", "other"}, {"other"}} {
		p := newRoundContract(repair)
		p.ImplementationScope = scope
		_, err := Continue(dirWithRepair, roundJSON(t, p), Options{ExpectedRevision: repair.Revision})
		require.ErrorContains(t, err, "original goal, intent and implementation scope")
	}
	unchanged, err := state.LoadPreparation(dirWithRepair)
	require.NoError(t, err)
	require.Equal(t, repair, unchanged)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "flywheel.config.json"), []byte("{"), 0o600))
	_, err = Continue(dir, raw, Options{ExpectedRevision: st.Revision})
	require.Error(t, err)
	loaded, err := state.LoadPreparation(dir)
	require.NoError(t, err)
	require.Equal(t, st, loaded)
}

func TestContinueDoesNotDiscardUnknownWorkOrAdmittedImplementationIdentity(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	p.Limits.MaxActions = 12
	st := startFixture(t, dir, cwd, p)
	raw := roundJSON(t, newRoundContract(st))
	_, err := Continue(dir, raw, Options{ExpectedRevision: st.Revision})
	require.ErrorContains(t, err, "settled report")
	a := st.Contract.Actions[0]
	st.Pending = &state.PreparationInvocation{ID: state.PreparationHash("unknown"), ActionID: a.ID, BindingHash: state.PreparationHash(a), CandidateHash: state.PreparationHash("candidate"), StartedAt: st.StartedAt, Reason: "durable unknown dispatch"}
	st.ActionsUsed, st.Status = 1, "running"
	st, err = state.SavePreparation(dir, st)
	require.NoError(t, err)
	_, err = Continue(dir, raw, Options{ExpectedRevision: st.Revision})
	require.ErrorContains(t, err, "settled report")
	dir, st = settledRoundFixture(t)
	thinInvestigationBrief(t, dir, st.Cwd)
	_, err = Continue(dir, roundJSON(t, newRoundContract(st)), Options{ExpectedRevision: st.Revision})
	require.NoError(t, err, "a thin brief retains the same task while new evidence is collected")
	dir, st = settledRoundFixture(t)
	// A corrupt or already admitted implementation cannot become a blank task.
	require.NoError(t, os.WriteFile(state.StateFile(dir), []byte("{}"), 0o600))
	_, err = Continue(dir, roundJSON(t, newRoundContract(st)), Options{ExpectedRevision: st.Revision})
	require.ErrorContains(t, err, "precede implementation planning")
}

func TestContinueRetainsDecisionAllowanceAcrossDisabledRound(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	p.Limits.MaxActions = 12
	withDecision(&p, "http://127.0.0.1:1/v1/systemone")
	startFixture(t, dir, cwd, p)
	st, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { return "test-key", true }})
	require.NoError(t, err)
	st, err = Finish(dir, reportFixture(st), Options{})
	require.NoError(t, err)
	require.Zero(t, state.PreparationDecisionRemaining(st, 10))
	next, err := Continue(dir, roundJSON(t, newRoundContract(st)), Options{ExpectedRevision: st.Revision})
	require.NoError(t, err)
	require.Nil(t, next.Contract.Decision, "exhausted optional advice stays disabled")
	next, err = Finish(dir, *st.Report, Options{ExpectedRevision: next.Revision})
	require.NoError(t, err)
	next, err = Continue(dir, roundJSON(t, newRoundContract(next)), Options{ExpectedRevision: next.Revision})
	require.NoError(t, err)
	require.Nil(t, next.Contract.Decision)
	// With no dispatch, a configured allowance remains available in the new round.
	dir, cwd, p = preparationFixture(t)
	p.Limits.MaxActions = 12
	withDecision(&p, "http://127.0.0.1:1/v1/systemone")
	startFixture(t, dir, cwd, p)
	st, err = Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { return "", false }})
	require.NoError(t, err)
	st, err = Finish(dir, reportFixture(st), Options{})
	require.NoError(t, err)
	next, err = Continue(dir, roundJSON(t, newRoundContract(st)), Options{ExpectedRevision: st.Revision})
	require.NoError(t, err)
	require.NotNil(t, next.Contract.Decision)
	require.Equal(t, 1, next.Contract.Decision.Policy.MaxCalls)
}
