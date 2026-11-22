package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ckanner/flywheel/internal/config"
)

// Analysis has its own durable intent because a report-only task may have no
// implementation state or role session ledger. EvidenceOnly is a tool policy,
// not an assertion that the model process runs inside an OS sandbox.
type PreparationAnalysisRequest struct {
	Version        int                        `json:"version"`
	ID             string                     `json:"id"`
	RunID          string                     `json:"run-id"`
	Round          int                        `json:"round"`
	Revision       int64                      `json:"revision"`
	ContractHash   string                     `json:"contract-hash"`
	EvidenceHash   string                     `json:"evidence-hash"`
	Binding        config.LoopBinding         `json:"binding"`
	BindingSource  string                     `json:"binding-source"`
	Native         json.RawMessage            `json:"native,omitempty"`
	Subscription   *config.SubscriptionPolicy `json:"subscription,omitempty"`
	PermissionMode string                     `json:"permission-mode"`
	EvidenceOnly   bool                       `json:"evidence-only"`
	StartedAt      string                     `json:"started-at"`
}

type PreparationAnalysisPending struct {
	Request     PreparationAnalysisRequest `json:"request"`
	NativeRunID string                     `json:"native-run-id,omitempty"`
	Sequence    int                        `json:"sequence,omitempty"`
}

type PreparationAnalysisResult struct {
	Request          PreparationAnalysisRequest `json:"request"`
	ReceiptDigest    string                     `json:"receipt-digest"`
	Recovered        bool                       `json:"recovered"`
	FinishedAt       string                     `json:"finished-at"`
	DurationMS       int64                      `json:"duration-ms"`
	Dispatch         string                     `json:"dispatch"`
	Outcome          string                     `json:"outcome"`
	CleanupConfirmed bool                       `json:"cleanup-confirmed"`
	UsageStatus      string                     `json:"usage-status"`
	Usage            *Usage                     `json:"usage,omitempty"`
	CostUSD          *float64                   `json:"cost-usd,omitempty"`
	ReportHash       string                     `json:"report-hash,omitempty"`
	Error            string                     `json:"error,omitempty"`
}

type PreparationAnalysisReceipt struct {
	Version int                        `json:"version"`
	Request PreparationAnalysisRequest `json:"request"`
	Result  json.RawMessage            `json:"result"`
}

func PreparationEvidenceHash(st *PreparationState) string {
	history := make([]string, 0, len(st.History))
	for _, round := range st.History {
		history = append(history, round.Digest)
	}
	effects := append([]InvestigationDecisionEffect{}, st.DecisionEffects...)
	return PreparationHash([]any{st.ContractHash, history, st.Observations, effects})
}

// Check a prospective report without granting adoption or clearing an intent.
// The caller must still commit the matching receipt and report through CAS.
func ValidatePreparationReport(st *PreparationState, report InvestigationReport) error {
	candidate := *st
	candidate.Report, candidate.Status, candidate.AnalysisPending = &report, report.Status, nil
	return validatePreparationState(&candidate)
}

func validatePreparationAnalysis(st *PreparationState) error {
	var request *PreparationAnalysisRequest
	if p := st.AnalysisPending; p != nil {
		request = &p.Request
		if st.Analysis != nil || st.Report != nil || st.Pending != nil || st.DecisionPending != nil ||
			(st.Status != "awaiting-report" && st.Status != "blocked") || (p.NativeRunID == "") != (p.Sequence == 0) {
			return fmt.Errorf("analysis intent conflicts with the round or its lifecycle identity")
		}
	}
	if a := st.Analysis; a != nil {
		request = &a.Request
		started, firstErr := time.Parse(time.RFC3339Nano, a.Request.StartedAt)
		finished, lastErr := time.Parse(time.RFC3339Nano, a.FinishedAt)
		if firstErr != nil || lastErr != nil || finished.Before(started) || a.DurationMS > st.ElapsedMS || !a.CleanupConfirmed ||
			(a.ReportHash != "" && (st.Report == nil || a.ReportHash != PreparationHash(st.Report) || a.Outcome != "completed" || a.Dispatch != "dispatched")) {
			return fmt.Errorf("analysis result lacks settled time, cleanup, or exact report identity")
		}
	}
	if request == nil {
		return nil
	}
	started, err := time.Parse(time.RFC3339Nano, request.StartedAt)
	begin, _ := time.Parse(time.RFC3339Nano, st.StartedAt)
	if request.RunID != st.RunID || request.Round != len(st.History)+1 || request.Revision > st.Revision || request.ContractHash != st.ContractHash ||
		request.EvidenceHash != PreparationEvidenceHash(st) || err != nil || started.Before(begin) {
		return fmt.Errorf("analysis request does not match the frozen round and evidence")
	}
	return nil
}

func validatePreparationAnalysisTransition(old, next *PreparationState) error {
	if old.Analysis != nil && PreparationHash(old.Analysis) != PreparationHash(next.Analysis) {
		return fmt.Errorf("analysis result is immutable")
	}
	if pending := old.AnalysisPending; pending != nil {
		if current := next.AnalysisPending; current != nil {
			if PreparationHash(pending.Request) != PreparationHash(current.Request) || current.Sequence < pending.Sequence || (pending.NativeRunID != "" && current.NativeRunID != pending.NativeRunID) {
				return fmt.Errorf("analysis dispatch identity cannot change")
			}
		} else if next.Analysis == nil || PreparationHash(pending.Request) != PreparationHash(next.Analysis.Request) {
			return fmt.Errorf("unknown analysis must retain its pending intent")
		}
	}
	return nil
}

func preparationAnalysisFile(id string, recovered bool) string {
	kind := "original"
	if recovered {
		kind = "recovered"
	}
	return filepath.Join(".investigation", "analysis-"+PreparationHash(id)+"-"+kind+".json")
}

// Original and read-only recovered receipts have separate immutable slots.
// A failed cleanup observation never overwrites later authoritative recovery.
func SavePreparationAnalysisReceipt(dir string, receipt PreparationAnalysisReceipt, recovered bool) (string, error) {
	raw, err := json.Marshal(receipt)
	if err != nil {
		return "", err
	}
	if receipt.Version != 1 || receipt.Request.ID == "" || len(raw) > 32<<20 || len(receipt.Result) == 0 || bytes.TrimSpace(receipt.Result)[0] != '{' {
		return "", fmt.Errorf("invalid bounded analysis receipt")
	}
	if err := os.MkdirAll(filepath.Join(dir, ".investigation"), 0o700); err != nil {
		return "", err
	}
	if err := writePreparationData(dir, preparationAnalysisFile(receipt.Request.ID, recovered), raw, true); err != nil {
		return "", err
	}
	return PreparationHash(receipt), nil
}

func ReadPreparationAnalysisReceipt(dir string, request PreparationAnalysisRequest, recovered bool) (*PreparationAnalysisReceipt, string, error) {
	raw, err := readPreparationData(dir, preparationAnalysisFile(request.ID, recovered), 32<<20)
	if err != nil {
		return nil, "", err
	}
	var receipt PreparationAnalysisReceipt
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&receipt); err != nil || !json.Valid(raw) || receipt.Version != 1 || PreparationHash(receipt.Request) != PreparationHash(request) ||
		len(bytes.TrimSpace(receipt.Result)) == 0 || bytes.TrimSpace(receipt.Result)[0] != '{' {
		return nil, "", fmt.Errorf("analysis receipt does not match its frozen request")
	}
	return &receipt, PreparationHash(receipt), nil
}
