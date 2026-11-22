package machine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/metrics"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

// Keep the public worker and pinned adapter real; only the native CLI is a
// fixture. Its nonempty init.tools catalog must not invalidate verified managed
// evidence-only execution. These cases do not establish live account behavior.
func antigravityEvidenceBridge(t *testing.T) {
	t.Helper()
	if os.Getenv("FLYWHEEL_TEST_NODE_BRIDGE") != "1" {
		t.Skip("requires built public adapter")
	}
	_, err := exec.LookPath("node")
	require.NoError(t, err)
	worker, err := filepath.Abs("../../plugins/dsh/dist/antigravity-worker.js")
	require.NoError(t, err)
	source, err := os.ReadFile("../drivers/testdata/fake-agy.js")
	require.NoError(t, err)
	source = []byte(strings.Replace(string(source), "const args=", `const fixtureConfig=JSON.parse(readFileSync('agy-fixture.json','utf8'));
process.env.FIXTURE_SCENARIO='tools-inherited';
process.env.FIXTURE_RESPONSE=fixtureConfig.response;
process.env.FIXTURE_CALLS='agy-calls.jsonl';
process.env.FIXTURE_INPUT='agy-input.jsonl';
const args=`, 1))
	fixture := filepath.Join(t.TempDir(), "fake-agy.mjs")
	require.NoError(t, os.WriteFile(fixture, source, 0o755))
	t.Setenv(drivers.AntigravityBinaryEnv, fixture)
	t.Setenv(drivers.AntigravityWorkerEnv, worker)
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("HOME", home)
}

func writeAntigravityEvidenceResponse(t *testing.T, cwd string, response any) {
	t.Helper()
	// Probe and dispatch logs are fixture instrumentation, not candidate edits.
	exclude := filepath.Join(cwd, ".git", "info", "exclude")
	existing, err := os.ReadFile(exclude)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(exclude, append(existing, []byte("\n/agy-fixture.json\n/agy-calls.jsonl\n/agy-input.jsonl\n")...), 0o600))
	raw, err := json.Marshal(response)
	require.NoError(t, err)
	writeFile(t, filepath.Join(cwd, "agy-fixture.json"), string(mustJSON(map[string]string{"response": string(raw)})))
}

func antigravityEvidenceCalls(t *testing.T, cwd string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(cwd, "agy-input.jsonl"))
	require.NoError(t, err)
	require.Len(t, strings.Split(strings.TrimSpace(string(raw)), "\n"), 1, "one prompt must reach the native CLI")
	var input struct {
		Event string `json:"event"`
	}
	require.NoError(t, json.Unmarshal(raw, &input))
	require.Equal(t, "user", input.Event)
	calls, err := os.ReadFile(filepath.Join(cwd, "agy-calls.jsonl"))
	require.NoError(t, err)
	return calls
}

func TestAntigravityEvidenceJEVOfflineBridge(t *testing.T) {
	antigravityEvidenceBridge(t)
	fx, c := sessionFixture(t)
	enableExecutionPolicy(c)
	enableRuntimeSubscription(t, c)
	c.Config.Profiles["hydrogen-local"] = config.ModelProfile{Harness: "antigravity", Channel: "jev-google", Model: "gemini-4-hydrogen-high", Thinking: "high"}
	c.Config.JEV.TimeoutMS = 10000
	persistRuntimeConfig(t, c)
	gitCmd(t, c.Cwd, "add", "--", filepath.Join(c.TaskDir, "flywheel.config.json"))
	gitCmd(t, c.Cwd, "commit", "-q", "-m", "test: configure native decision bridge")
	p := c.Packets["M01-T01"]
	actions, baseline, err := c.initialExecutionActions(p)
	require.NoError(t, err)
	require.Equal(t, "model-0", baseline)
	probabilities := map[string]float64{}
	for _, action := range actions {
		probabilities[action.ID] = 0
	}
	require.Contains(t, probabilities, "model-1")
	probabilities["model-1"] = 1
	writeAntigravityEvidenceResponse(t, c.Cwd, map[string]any{"action-id": "model-1", "applicability": 1, "choice-confidence": 1, "probabilities": probabilities})
	unknownBefore := c.State.Budget.UnknownUsageCalls
	require.NoError(t, c.runtimeInitialExecutionDecision(p))
	ledger := c.State.RuntimeDecisions
	require.NotNil(t, ledger)
	require.Equal(t, 1, ledger.Used)
	require.Len(t, ledger.Records, 1)
	r := ledger.Records[0]
	require.Equal(t, "applied", r.Status)
	require.True(t, r.Accounted)
	require.NotNil(t, r.Selection)
	require.True(t, r.Selection.Adopted, "%+v", r.Receipt)
	require.Equal(t, "model-1", r.Action)
	require.NotNil(t, r.Receipt.Native)
	require.Equal(t, "accepted", r.Receipt.Native.Dispatch)
	require.Equal(t, "completed", r.Receipt.Native.Outcome)
	require.True(t, r.Receipt.Native.CleanupConfirmed)
	require.NotEmpty(t, r.Receipt.Native.ReceiptHash)
	require.Nil(t, r.Receipt.Usage.Input, "conversation usage must remain unknown for task accounting")
	require.Equal(t, unknownBefore+1, c.State.Budget.UnknownUsageCalls)
	ps := c.State.Packets[p.PacketID]
	require.Zero(t, ps.Attempts, "selection does not execute the implementation")
	require.Equal(t, "high", ps.ExecutionPolicy.Current)
	require.NotNil(t, ps.ExecutionPolicy.CurrentBinding)
	require.Equal(t, "high", ps.ExecutionPolicy.CurrentBinding.Binding.Thinking)
	before := antigravityEvidenceCalls(t, c.Cwd)
	require.NoError(t, c.runtimeInitialExecutionDecision(p))
	c = restartSessionGuardContext(t, fx, c)
	require.NoError(t, c.runtimeInitialExecutionDecision(c.Packets[p.PacketID]))
	require.Equal(t, 1, c.State.RuntimeDecisions.Used)
	require.Equal(t, "high", c.State.Packets[p.PacketID].ExecutionPolicy.Current)
	require.Equal(t, unknownBefore+1, c.State.Budget.UnknownUsageCalls)
	require.Equal(t, before, antigravityEvidenceCalls(t, c.Cwd), "reread and restart must not redispatch JEV")
}

func TestAntigravityEvidenceInvestigationOfflineBridge(t *testing.T) {
	antigravityEvidenceBridge(t)
	fx, st, cfg := analysisFixture(t)
	cfg.Drivers["investigate"] = "real"
	cfg.Loops["investigate"] = config.LoopBinding{Harness: "antigravity", Model: "gemini-4-neon-high", Channel: "google-ultra", Thinking: "high"}
	wanted := supportedPreparationReport(st)
	writeAntigravityEvidenceResponse(t, st.Cwd, map[string]any{"version": 1, "report": wanted})
	ds := drivers.CreateDrivers(&cfg)

	// Interrupt only final report adoption, after the real worker and Go driver
	// have persisted their receipt. Recovery must consume that same invocation.
	oldSave := saveAnalysisState
	t.Cleanup(func() { saveAnalysisState = oldSave })
	interrupted := errors.New("evidence bridge report CAS interrupted")
	saveAnalysisState = func(dir string, next *state.PreparationState) (*state.PreparationState, error) {
		if next.Analysis != nil {
			return nil, interrupted
		}
		return oldSave(dir, next)
	}
	_, err := AnalyzeInvestigation(context.Background(), fx.taskDir, cfg, *ds)
	require.ErrorIs(t, err, interrupted)
	saveAnalysisState = oldSave
	pending, err := state.LoadPreparation(fx.taskDir)
	require.NoError(t, err)
	require.NotNil(t, pending.AnalysisPending)
	require.Nil(t, pending.Report)
	receipt, _, err := state.ReadPreparationAnalysisReceipt(fx.taskDir, pending.AnalysisPending.Request, false)
	require.NoError(t, err)
	var result drivers.Result
	require.NoError(t, json.Unmarshal(receipt.Result, &result))
	require.True(t, result.Settled, "%+v", result)
	require.Empty(t, errs.ClassifyError(toSpawnResult(result)), "successful structured receipts must not be reclassified by diagnostic words in protocol metadata")
	require.Equal(t, pending.AnalysisPending.Request.Binding.Model, result.Model)
	require.Equal(t, "antigravity", result.Harness)
	var native struct {
		Configuration struct {
			Observed struct {
				Agent string   `json:"agent"`
				Tools []string `json:"tools"`
			} `json:"observed"`
		} `json:"configuration"`
	}
	require.NoError(t, json.Unmarshal(result.AntigravityReceipt, &native))
	require.NotEmpty(t, native.Configuration.Observed.Agent)
	require.Contains(t, native.Configuration.Observed.Tools, "run_command")
	require.Contains(t, native.Configuration.Observed.Tools, "invoke_subagent")
	before := antigravityEvidenceCalls(t, st.Cwd)
	settled, err := AnalyzeInvestigation(context.Background(), fx.taskDir, cfg, *ds)
	require.NoError(t, err)
	require.Equal(t, "supported", settled.Status)
	require.Equal(t, &wanted, settled.Report)
	require.Nil(t, settled.AnalysisPending)
	require.Equal(t, state.PreparationHash(wanted), settled.Analysis.ReportHash)
	require.Equal(t, "completed", settled.Analysis.Outcome)
	require.True(t, settled.Analysis.CleanupConfirmed)
	require.Equal(t, "unknown", settled.Analysis.UsageStatus)
	require.Nil(t, settled.Analysis.Usage)
	_, err = AnalyzeInvestigation(context.Background(), fx.taskDir, cfg, *ds)
	require.NoError(t, err)
	require.Equal(t, before, antigravityEvidenceCalls(t, st.Cwd), "report recovery and reread must not redispatch analysis")
	require.False(t, state.Exists(fx.taskDir), "analysis does not create implementation authority")
	report, err := metrics.ReadReport(filepath.Join(fx.taskDir, "metrics.jsonl"))
	require.NoError(t, err)
	require.Equal(t, 1, report.TotalCalls)
}
