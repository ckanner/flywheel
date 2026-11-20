package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/stretchr/testify/require"
)

func pendingPreparationAnalysis(t *testing.T) *PreparationState {
	t.Helper()
	st := observedPreparation(t)
	st.Status = "awaiting-report"
	request := PreparationAnalysisRequest{Version: 1, RunID: st.RunID, Round: 1, Revision: st.Revision, ContractHash: st.ContractHash,
		EvidenceHash: PreparationEvidenceHash(st), Binding: config.LoopBinding{Harness: "antigravity", Model: "neon", Channel: "google", Thinking: "high"},
		BindingSource: "loops.investigate", Subscription: &config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"}, PermissionMode: "read-only", EvidenceOnly: true, StartedAt: st.StartedAt}
	request.ID = PreparationHash(request)
	st.AnalysisPending = &PreparationAnalysisPending{Request: request}
	return st
}

func settledPreparationAnalysis(t *testing.T) *PreparationState {
	t.Helper()
	st := pendingPreparationAnalysis(t)
	st.Analysis = &PreparationAnalysisResult{Request: st.AnalysisPending.Request, ReceiptDigest: PreparationHash("receipt"), FinishedAt: "2026-11-12T00:00:02Z",
		DurationMS: 1000, Dispatch: "dispatched", Outcome: "failed", CleanupConfirmed: true, UsageStatus: "unknown", Error: "No report returned"}
	st.AnalysisPending = nil
	return st
}

func TestPreparationAnalysisBindsCurrentEvidenceAndRequiresSettledReceipt(t *testing.T) {
	require.NoError(t, validatePreparationState(pendingPreparationAnalysis(t)))
	require.NoError(t, validatePreparationState(settledPreparationAnalysis(t)))
	for name, mutate := range map[string]func(*PreparationState){
		"wrong run":                func(st *PreparationState) { st.AnalysisPending.Request.RunID = "other" },
		"wrong round":              func(st *PreparationState) { st.AnalysisPending.Request.Round = 2 },
		"future revision":          func(st *PreparationState) { st.AnalysisPending.Request.Revision++ },
		"changed contract":         func(st *PreparationState) { st.AnalysisPending.Request.ContractHash = PreparationHash("different") },
		"changed observations":     func(st *PreparationState) { st.AnalysisPending.Request.EvidenceHash = PreparationHash("different") },
		"bad time":                 func(st *PreparationState) { st.AnalysisPending.Request.StartedAt = "not-time" },
		"before collection":        func(st *PreparationState) { st.AnalysisPending.Request.StartedAt = "2026-11-11T00:00:00Z" },
		"incomplete lifecycle":     func(st *PreparationState) { st.AnalysisPending.NativeRunID = "native" },
		"pending with report":      func(st *PreparationState) { st.Report = &InvestigationReport{} },
		"pending while collecting": func(st *PreparationState) { st.Status = "ready" },
	} {
		t.Run(name, func(t *testing.T) {
			st := pendingPreparationAnalysis(t)
			mutate(st)
			require.Error(t, validatePreparationAnalysis(st))
		})
	}
	for name, mutate := range map[string]func(*PreparationState){
		"bad finish time":         func(st *PreparationState) { st.Analysis.FinishedAt = "not-time" },
		"missing cleanup":         func(st *PreparationState) { st.Analysis.CleanupConfirmed = false },
		"unaccounted duration":    func(st *PreparationState) { st.Analysis.DurationMS = st.ElapsedMS + 1 },
		"invented adopted report": func(st *PreparationState) { st.Analysis.ReportHash = PreparationHash("report") },
	} {
		t.Run(name, func(t *testing.T) {
			st := settledPreparationAnalysis(t)
			mutate(st)
			require.Error(t, validatePreparationAnalysis(st))
		})
	}
	st := pendingPreparationAnalysis(t)
	st.AnalysisPending.Request.EvidenceHash = PreparationHash("changed observation set")
	require.ErrorContains(t, validatePreparationState(st), "frozen round and evidence")
	st = pendingPreparationAnalysis(t)
	report := *supportedPreparation(t).Report
	// The fixture's evidence ID is independent of its repository directory.
	require.NoError(t, ValidatePreparationReport(st, report))
	report.Findings[1].Evidence = []string{PreparationHash("not collected")}
	require.Error(t, ValidatePreparationReport(st, report))
}

func TestAnalysisIntentAndSettledResultCannotBeSilentlyReplaced(t *testing.T) {
	old := pendingPreparationAnalysis(t)
	next := copyPreparation(t, old)
	next.AnalysisPending.NativeRunID, next.AnalysisPending.Sequence = "native-run", 1
	require.NoError(t, validatePreparationAnalysisTransition(old, next))
	old = next
	next = copyPreparation(t, old)
	next.AnalysisPending.Sequence = 0
	require.ErrorContains(t, validatePreparationAnalysisTransition(old, next), "identity cannot change")
	next = copyPreparation(t, old)
	next.AnalysisPending.Request.Binding.Model = "retargeted"
	require.ErrorContains(t, validatePreparationAnalysisTransition(old, next), "identity cannot change")
	next = copyPreparation(t, old)
	next.AnalysisPending = nil
	require.ErrorContains(t, validatePreparationAnalysisTransition(old, next), "retain its pending intent")
	next.Analysis = &PreparationAnalysisResult{Request: old.AnalysisPending.Request}
	require.NoError(t, validatePreparationAnalysisTransition(old, next))
	old = settledPreparationAnalysis(t)
	next = copyPreparation(t, old)
	next.Analysis.Error = "rewritten"
	require.ErrorContains(t, validatePreparationAnalysisTransition(old, next), "immutable")
}

func TestAnalysisEvidenceIdentitySurvivesArchivingAndNativeJSONFormatting(t *testing.T) {
	first := settledPreparationAnalysis(t)
	first.Report = supportedPreparation(t).Report
	first.Status = first.Report.Status
	first.Analysis.Request.Native = json.RawMessage(`{"version":1}`)
	require.NoError(t, validatePreparationState(first))
	// The writer returns the typed state; JSON indentation changes when a native
	// policy is archived deeper in history. That cannot invalidate the next CAS.
	dir := t.TempDir()
	initial := copyPreparation(t, first)
	initial.Revision = 0
	saved, err := SavePreparation(dir, initial)
	require.NoError(t, err)
	archived := nextPreparationRound(t, saved)
	archived.Revision = saved.Revision
	saved, err = SavePreparation(dir, archived)
	require.NoError(t, err)
	saved.Reason = "same immutable history, another durable checkpoint"
	_, err = SavePreparation(dir, saved)
	require.NoError(t, err)
	next := nextPreparationRound(t, first)
	request := first.Analysis.Request
	request.Round, request.Revision, request.ContractHash = 2, next.Revision, next.ContractHash
	request.EvidenceHash, request.StartedAt = PreparationEvidenceHash(next), next.StartedAt
	request.ID = PreparationHash(request)
	next.Report, next.Status = nil, "awaiting-report"
	next.AnalysisPending = &PreparationAnalysisPending{Request: request}
	require.NoError(t, validatePreparationState(next))
	formatted := copyPreparation(t, next)
	formatted.AnalysisPending.Request.Native = json.RawMessage("{\n  \"version\": 1\n}")
	require.NoError(t, validatePreparationAnalysisTransition(next, formatted), "JSON indentation is not a new native policy")
	formatted.History[0].Digest = PreparationHash("changed-history")
	require.NotEqual(t, next.AnalysisPending.Request.EvidenceHash, PreparationEvidenceHash(formatted))
}

func TestAnalysisReceiptsAreBoundedImmutableAndCorrelated(t *testing.T) {
	dir := t.TempDir()
	request := pendingPreparationAnalysis(t).AnalysisPending.Request
	receipt := PreparationAnalysisReceipt{Version: 1, Request: request, Result: json.RawMessage(`{"settled":false}`)}
	digest, err := SavePreparationAnalysisReceipt(dir, receipt, false)
	require.NoError(t, err)
	loaded, actual, err := ReadPreparationAnalysisReceipt(dir, request, false)
	require.NoError(t, err)
	require.Equal(t, digest, actual)
	require.Equal(t, receipt, *loaded)
	_, err = SavePreparationAnalysisReceipt(dir, receipt, false)
	require.Error(t, err, "an original receipt can never be overwritten")
	_, err = SavePreparationAnalysisReceipt(dir, receipt, true)
	require.NoError(t, err)
	_, _, err = ReadPreparationAnalysisReceipt(dir, request, true)
	require.NoError(t, err)
	foreign := request
	foreign.EvidenceHash = PreparationHash("foreign")
	_, _, err = ReadPreparationAnalysisReceipt(dir, foreign, false)
	require.ErrorContains(t, err, "frozen request")
	_, _, err = ReadPreparationAnalysisReceipt(t.TempDir(), request, false)
	require.Error(t, err)
	receipt.Result = json.RawMessage(`{`)
	_, err = SavePreparationAnalysisReceipt(dir, receipt, false)
	require.Error(t, err)
	receipt.Result = json.RawMessage(`[]`)
	_, err = SavePreparationAnalysisReceipt(dir, receipt, false)
	require.ErrorContains(t, err, "bounded analysis receipt")
	receipt.Result = json.RawMessage(`{}`)
	badDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(badDir, ".investigation"), []byte("not a directory"), 0o600))
	_, err = SavePreparationAnalysisReceipt(badDir, receipt, false)
	require.Error(t, err)
	// External corruption is detected rather than interpreted as an empty reply.
	require.NoError(t, os.WriteFile(filepath.Join(dir, preparationAnalysisFile(request.ID, false)), []byte(`{}`), 0o600))
	_, _, err = ReadPreparationAnalysisReceipt(dir, request, false)
	require.Error(t, err)
}
