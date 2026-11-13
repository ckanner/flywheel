package investigation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func selectResponse(t *testing.T, w http.ResponseWriter, r *http.Request, selectID func(map[string]string) string) {
	t.Helper()
	var request struct {
		Questions map[string]struct{ Criteria map[string]string }
	}
	require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
	criteria := request.Questions["action"].Criteria
	selected := selectID(criteria)
	require.Contains(t, criteria, selected)
	probabilities := map[string]float64{}
	for id := range criteria {
		probabilities[id] = 0
	}
	probabilities[selected] = 1
	require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"model": "jev-1.14.0", "answers": map[string]any{"applicable": map[string]any{"type": "noul", "noul": 0.99}, "action": map[string]any{"type": "choice", "choice": selected, "confidence": 1, "probabilities": probabilities}}, "usage": map[string]int{"input_tokens": 12, "output_tokens": 4}}))
}

func controlID(t *testing.T, criteria map[string]string, prefix string) string {
	t.Helper()
	for id := range criteria {
		if strings.HasPrefix(id, prefix) {
			return id
		}
	}
	t.Fatalf("no %s choice in %v", prefix, criteria)
	return ""
}

func changeRuntimeConfig(t *testing.T, dir string, change func(map[string]any)) {
	t.Helper()
	path := filepath.Join(dir, "flywheel.config.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(raw, &cfg))
	change(cfg)
	raw, err = json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
}

func TestMasterAutomaticallyEnablesEvidenceFinishAndStillAcceptsAnalystReport(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		selectResponse(t, w, r, func(criteria map[string]string) string {
			if calls == 1 {
				return "find-default"
			}
			return controlID(t, criteria, "jev-finish-")
		})
	}))
	defer server.Close()
	withDecision(&p, server.URL+"/v1/systemone")
	p.Decision.Policy.MaxCalls = 8
	configureDecision(t, dir, p.Decision, true)
	p.Decision = nil // ordinary Task does not handwrite a decision policy
	st := startFixture(t, dir, cwd, p)
	require.Equal(t, decision.Adopt, st.Contract.Decision.Policy.Mode)
	st, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { return "key", true }})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, "enabled", st.JEVStatus)
	require.Equal(t, "awaiting-report", st.Status)
	require.Equal(t, "jev-evidence-ready-for-report", st.Reason)
	require.Len(t, st.Observations, 1)
	require.Len(t, st.DecisionEffects, 1)
	require.Equal(t, "finish", st.DecisionEffects[0].Kind)
	require.Equal(t, []string{st.Observations[0].Invocation.ID}, st.DecisionEffects[0].Evidence)
	require.Equal(t, []string{"read-settings", "list-source"}, st.DecisionEffects[0].Actions)
	revision := st.Revision
	st, err = Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, revision, st.Revision)
	withoutLimits := reportFixture(st)
	withoutLimits.Limitations = nil
	_, err = Finish(dir, withoutLimits, Options{})
	require.ErrorContains(t, err, "limitations")
	st, err = Finish(dir, reportFixture(st), Options{})
	require.NoError(t, err)
	require.Equal(t, "supported", st.Status)
	require.False(t, state.Exists(dir), "collection stop and analyst report never create code-completion state")
}

func TestCollectionDecisionRecoversAfterFailedCASWithoutRepeatingTheRequest(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		selectResponse(t, w, r, func(criteria map[string]string) string {
			if calls == 1 {
				return "find-default"
			}
			blockPreparationWrites(t, dir)
			return controlID(t, criteria, "jev-finish-")
		})
	}))
	defer server.Close()
	withDecision(&p, server.URL+"/v1/systemone")
	p.Decision.Policy.MaxCalls = 8
	startFixture(t, dir, cwd, p)
	_, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { return "key", true }})
	require.Error(t, err)
	st, err := state.LoadPreparation(dir)
	require.NoError(t, err)
	require.NotNil(t, st.DecisionPending)
	require.Empty(t, st.DecisionEffects)
	require.Len(t, st.Observations, 1)
	allowPreparationWrites(t, dir)
	st, err = Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { t.Fatal("recovery must not read a key or repeat HTTP"); return "", false }})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Nil(t, st.DecisionPending)
	require.Len(t, st.DecisionEffects, 1)
	require.Equal(t, "jev-evidence-ready-for-report", st.Reason)
}

func TestInvalidEffectiveConfigCannotCreateAnUnconfiguredPreparation(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "flywheel.config.json"), []byte(`{`), 0o600))
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	_, err = Start(dir, cwd, raw, Options{})
	require.Error(t, err)
	_, err = os.Stat(state.PreparationFile(dir))
	require.True(t, os.IsNotExist(err))
}

func TestEvidenceBackedDeferPrunesOnlyBoundActions(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		selectResponse(t, w, r, func(criteria map[string]string) string {
			switch calls {
			case 1:
				return "read-settings"
			case 2:
				for id := range criteria {
					require.NotContains(t, id, "jev-", "unobserved default hypothesis cannot authorize pruning")
				}
				return "find-default"
			default:
				return controlID(t, criteria, "jev-defer-")
			}
		})
	}))
	defer server.Close()
	withDecision(&p, server.URL+"/v1/systemone")
	p.Decision.Policy.MaxCalls = 8
	startFixture(t, dir, cwd, p)
	st, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { return "key", true }})
	require.NoError(t, err)
	require.Equal(t, 3, calls)
	require.Len(t, st.Observations, 2)
	require.Len(t, st.DecisionEffects, 1)
	require.Equal(t, "defer", st.DecisionEffects[0].Kind)
	require.Equal(t, []string{"list-source"}, st.DecisionEffects[0].Actions)
	require.Equal(t, []string{st.Observations[1].Invocation.ID}, st.DecisionEffects[0].Evidence)
	require.Equal(t, "awaiting-report", st.Status)
	loaded, err := state.LoadPreparation(dir)
	require.NoError(t, err)
	require.Equal(t, st.DecisionEffects, loaded.DecisionEffects)
}

func TestMasterOffRejectsLegacyContractAuthorityAndFreezesBaseline(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	withDecision(&p, "http://127.0.0.1:1/v1/systemone")
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	st, err := Start(dir, cwd, raw, Options{}) // bypass helper's explicit global opt-in
	require.NoError(t, err)
	require.Nil(t, st.Contract.Decision)
	st, err = Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { t.Fatal("OFF must not read credentials"); return "", false }})
	require.NoError(t, err)
	require.Equal(t, "disabled", st.JEVStatus)
	require.Empty(t, st.Decisions)
	require.Empty(t, st.DecisionEffects)
	require.Len(t, st.Observations, 3)
	configureDecision(t, dir, p.Decision, true)
	active, _ := activePolicy(context.Background(), dir, st)
	require.False(t, active)
	require.Equal(t, "not-configured", st.JEVStatus, "enabling later cannot rewrite the frozen contract or reset a run")
}

func TestMasterDisableDuringHTTPOrFinalInspectionPreventsAdoption(t *testing.T) {
	for _, when := range []string{"http", "inspection", "hard-budget"} {
		t.Run(when, func(t *testing.T) {
			dir, cwd, p := preparationFixture(t)
			disable := func() {
				changeRuntimeConfig(t, dir, func(cfg map[string]any) {
					if when == "hard-budget" {
						cfg["budget"].(map[string]any)["enforce"] = "hard"
					} else {
						cfg["jev"].(map[string]any)["enabled"] = false
					}
				})
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if when == "http" {
					disable()
				}
				selectResponse(t, w, r, func(map[string]string) string { return "find-default" })
			}))
			defer server.Close()
			withDecision(&p, server.URL+"/v1/systemone")
			startFixture(t, dir, cwd, p)
			open, reads := openInspectionFile, 0
			t.Cleanup(func() { openInspectionFile = open })
			openInspectionFile = func(root *os.Root, path string, flags int, mode os.FileMode) (*os.File, error) {
				if path == "settings.json" {
					reads++
					if reads == 2 && when != "http" {
						disable()
					}
				}
				return open(root, path, flags, mode)
			}
			st, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { return "key", true }})
			require.NoError(t, err)
			require.Equal(t, 1, calls)
			require.Len(t, st.Decisions, 1)
			require.Equal(t, "ok", st.Decisions[0].Status, "already sent usage remains accounted even when adoption is revoked")
			require.Equal(t, "read-settings", st.Observations[0].Invocation.ActionID)
			require.False(t, st.Observations[0].Invocation.Adopted)
			require.Empty(t, st.DecisionEffects)
		})
	}
}

func TestCurrentAuthorizationCanNarrowButNeverRetargetFrozenRun(t *testing.T) {
	for _, field := range []string{"endpoint", "model", "key-env", "timeout-ms", "max-calls", "min-applicability", "min-choice", "allowed-data"} {
		t.Run(field, func(t *testing.T) {
			dir, cwd, p := preparationFixture(t)
			withDecision(&p, "http://127.0.0.1:1/v1/systemone")
			p.Decision.Policy.MaxCalls = 8
			st := startFixture(t, dir, cwd, p)
			changeRuntimeConfig(t, dir, func(cfg map[string]any) {
				j := cfg["jev"].(map[string]any)
				values := map[string]any{"endpoint": "http://127.0.0.1:2/v1/systemone", "model": "jev-2.0.0", "key-env": "OTHER_KEY", "timeout-ms": 500, "max-calls": 1, "min-applicability": .99, "min-choice": .99, "allowed-data": []string{"task-summary", "diagnostic-facts"}}
				j[field] = values[field]
			})
			frozen := state.PreparationHash(st.Contract)
			st, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { t.Fatal("restricted run must not look up credentials"); return "", false }})
			require.NoError(t, err)
			require.Equal(t, "policy-changed", st.JEVStatus)
			require.Equal(t, frozen, state.PreparationHash(st.Contract))
			require.Empty(t, st.Decisions)
			require.Len(t, st.Observations, 3)
		})
	}
}

func TestProjectMasterIsResolvedFromNestedWorkingDirectories(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	command := exec.Command("git", "init", "--quiet", cwd)
	require.NoError(t, command.Run())
	withDecision(&p, "http://127.0.0.1:1/v1/systemone")
	configureDecision(t, dir, p.Decision, true)
	require.NoError(t, os.Rename(filepath.Join(dir, "flywheel.config.json"), filepath.Join(cwd, "flywheel.config.json")))
	p.Decision = nil
	p.ReadPaths = []string{"loader.go"}
	p.Actions = p.Actions[:1]
	p.Actions[0].Path = "loader.go"
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	st, err := Start(dir, filepath.Join(cwd, "src"), raw, Options{})
	require.NoError(t, err)
	require.NotNil(t, st.Contract.Decision)
	require.Equal(t, "enabled", st.JEVStatus)
	changeRuntimeConfig(t, cwd, func(cfg map[string]any) { cfg["jev"].(map[string]any)["enabled"] = false })
	st, err = Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) {
		t.Fatal("project OFF must block credentials from nested cwd")
		return "", false
	}})
	require.NoError(t, err)
	require.Equal(t, "disabled", st.JEVStatus)
	require.Empty(t, st.Decisions)
	require.Len(t, st.Observations, 1)
	require.Equal(t, "observed", st.Observations[0].Status)
}

func TestChangedInputsCannotUseOldObservationsToStopOrPrune(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		selectResponse(t, w, r, func(criteria map[string]string) string {
			if calls == 1 {
				return "find-default"
			}
			for id := range criteria {
				require.NotContains(t, id, "jev-", "the earlier observation belongs to stale repository inputs")
			}
			return "read-settings"
		})
	}))
	defer server.Close()
	withDecision(&p, server.URL+"/v1/systemone")
	p.Decision.Policy.MaxCalls = 8
	startFixture(t, dir, cwd, p)
	open, reads := openInspectionFile, 0
	t.Cleanup(func() { openInspectionFile = open })
	openInspectionFile = func(root *os.Root, path string, flags int, mode os.FileMode) (*os.File, error) {
		if path == "src/loader.go" {
			reads++
			if reads == 3 {
				require.NoError(t, os.WriteFile(filepath.Join(cwd, "settings.json"), []byte(`{"timeout":99}`), 0o600))
			}
		}
		return open(root, path, flags, mode)
	}
	st, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { return "key", true }})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Len(t, st.Observations, 3)
	require.Empty(t, st.DecisionEffects)
	require.NotEqual(t, st.Observations[0].Invocation.CandidateHash, st.Observations[1].Invocation.CandidateHash)
}

func TestCollectionControlsFitTheBoundedChoiceProtocol(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	first := p.Actions[0]
	first.Hypotheses = []string{p.Hypotheses[0].ID, p.Hypotheses[1].ID}
	p.Actions = nil
	for i := range 16 {
		a := first
		a.ID = fmt.Sprintf("read-%02d", i)
		p.Actions = append(p.Actions, a)
	}
	p.Limits.MaxActions = 16
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		selectResponse(t, w, r, func(criteria map[string]string) string {
			require.Len(t, criteria, 16, "a finish choice cannot push the request over sixteen alternatives")
			if calls == 1 {
				return "read-00"
			}
			return controlID(t, criteria, "jev-finish-")
		})
	}))
	defer server.Close()
	withDecision(&p, server.URL+"/v1/systemone")
	p.Decision.Policy.MaxCalls = 8
	startFixture(t, dir, cwd, p)
	st, err := Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) { return "key", true }})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Len(t, st.Observations, 1)
	require.Len(t, st.DecisionEffects[0].Actions, 15)
}

func TestCancellationDuringFinalPolicyCheckCannotDispatchAnAction(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		selectResponse(t, w, r, func(map[string]string) string { return "find-default" })
	}))
	defer server.Close()
	withDecision(&p, server.URL+"/v1/systemone")
	startFixture(t, dir, cwd, p)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	load, checks := loadInvestigationConfiguration, 0
	t.Cleanup(func() { loadInvestigationConfiguration = load })
	loadInvestigationConfiguration = func(ctx context.Context, cwd, dir string) (*config.Config, error) {
		cfg, err := load(ctx, cwd, dir)
		checks++
		if checks == 4 { // run entry, before HTTP, after HTTP, immediately before adoption
			cancel()
		}
		return cfg, err
	}
	st, err := Run(ctx, dir, Options{LookupEnv: func(string) (string, bool) { return "key", true }})
	require.NoError(t, err)
	require.Equal(t, "cancelled", st.Status)
	require.Empty(t, st.Observations)
	require.Empty(t, st.DecisionEffects)
	require.Nil(t, st.Pending)
	require.Len(t, st.Decisions, 1)
	require.Equal(t, "ok", st.Decisions[0].Status)
}

func TestDisableAndReenableReconcilesUnknownRequestWithoutResettingItsBudget(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	withDecision(&p, "http://127.0.0.1:1/v1/systemone")
	st := reserveDecision(t, dir, startFixture(t, dir, cwd, p))
	identity := st.DecisionPending.Identity.CandidateHash
	decisionID := st.DecisionPending.ID
	changeRuntimeConfig(t, dir, func(cfg map[string]any) { cfg["jev"].(map[string]any)["enabled"] = false })
	_, selection, err := choose(context.Background(), dir, &st, remainingActions(st), identity, Options{LookupEnv: func(string) (string, bool) { t.Fatal("OFF recovery must not look up credentials"); return "", false }})
	require.NoError(t, err)
	require.False(t, selection.Adopted)
	require.Nil(t, st.DecisionPending)
	require.Len(t, st.Decisions, 1)
	require.Equal(t, decisionID, st.Decisions[0].DecisionID)
	require.Equal(t, "unknown", st.Decisions[0].Status)
	require.Nil(t, st.Decisions[0].Usage.Input)
	require.NoError(t, persist(dir, &st, Options{}))
	changeRuntimeConfig(t, dir, func(cfg map[string]any) { cfg["jev"].(map[string]any)["enabled"] = true })
	st, err = Run(context.Background(), dir, Options{LookupEnv: func(string) (string, bool) {
		t.Fatal("re-enabling must not replenish the exhausted one-call budget")
		return "", false
	}})
	require.NoError(t, err)
	require.Equal(t, "enabled", st.JEVStatus)
	require.Len(t, st.Decisions, 1)
	require.Len(t, st.Observations, 3)
	require.Equal(t, "decision-call-budget", st.Observations[0].Invocation.Reason)
}
