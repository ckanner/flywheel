package investigation

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func thinInvestigationBrief(t *testing.T, dir, cwd string) *state.State {
	t.Helper()
	st, err := state.InitPlanning(planning.Brief{Version: 1, ID: "research-first", Goal: "Review a repair design", OriginalRequest: "Investigate the timeout before designing a repair", Constraints: []string{}, Evidence: []string{}, Policy: "review-first"}, nil)
	require.NoError(t, err)
	st.Project = &state.ProjectIdentity{Cwd: cwd, WorkspaceCwd: cwd}
	st, err = state.Save(dir, st, nil)
	require.NoError(t, err)
	return st
}

func TestInvestigationCanFollowThinBriefWithoutReplacingPlanningState(t *testing.T) {
	dir, cwd, contract := preparationFixture(t)
	thinInvestigationBrief(t, dir, cwd)
	before, err := os.ReadFile(state.StateFile(dir))
	require.NoError(t, err)
	require.True(t, allowsPlanningInvestigation(dir, cwd))
	raw, err := json.Marshal(contract)
	require.NoError(t, err)
	prepared, err := Start(dir, cwd, raw, Options{})
	require.NoError(t, err)
	require.Equal(t, "ready", prepared.Status)
	after, err := os.ReadFile(state.StateFile(dir))
	require.NoError(t, err)
	require.Equal(t, before, after, "starting investigation does not invent a design or change the brief revision")
	require.False(t, allowsPlanningInvestigation(dir, t.TempDir()))
	require.False(t, allowsPlanningInvestigation(t.TempDir(), cwd))
}

func TestInvestigationNeverReplacesPendingPlanningCall(t *testing.T) {
	for _, scenario := range []string{"legacy-unknown", "received", "consumed"} {
		t.Run(scenario, func(t *testing.T) {
			dir, cwd, contract := preparationFixture(t)
			st := thinInvestigationBrief(t, dir, cwd)
			if scenario == "legacy-unknown" {
				st.PlanSession = map[string]any{"outcome-unknown": true}
			} else {
				st.Sessions.Records["design-plan/planning:pending"] = &state.SessionRecord{Task: dir, Role: "design-plan", Scope: "planning:pending", Cwd: cwd,
					Binding: strings.Repeat("a", 64), Goal: strings.Repeat("b", 64), Contract: strings.Repeat("c", 64), Generation: 1,
					Action: "create", Reason: "unconsumed design", Last: &state.SessionInvocation{ID: "call-original", Status: scenario, StartedAt: "2026-11-16T12:00:00Z", Attempt: 1,
						CandidateBefore: strings.Repeat("d", 64), CandidateAfter: strings.Repeat("e", 64), GuardBefore: strings.Repeat("a", 64), GuardAfter: strings.Repeat("b", 64),
						ReceiptDigest: strings.Repeat("f", 64), Dispatch: "dispatched", Outcome: "completed", CleanupConfirmed: true, UsageStatus: "complete", Accounted: scenario == "consumed"}}
			}
			_, err := state.Save(dir, st, nil)
			require.NoError(t, err)
			_, err = state.Load(dir)
			require.NoError(t, err, "the pending record must be valid; rejection is the ownership check")
			raw, err := json.Marshal(contract)
			require.NoError(t, err)
			_, err = Start(dir, cwd, raw, Options{})
			if scenario == "consumed" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "implementation task already exists")
				require.NoFileExists(t, state.PreparationFile(dir))
			}
		})
	}
}
