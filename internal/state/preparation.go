package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"syscall"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
)

type PreparationInvocation struct {
	ID            string `json:"id"`
	ActionID      string `json:"action-id"`
	BindingHash   string `json:"binding-hash"`
	CandidateHash string `json:"candidate-hash"`
	StartedAt     string `json:"started-at"`
	DecisionID    string `json:"decision-id,omitempty"`
	Adopted       bool   `json:"adopted"`
	Reason        string `json:"reason"`
}

type InvestigationObservation struct {
	Invocation  PreparationInvocation   `json:"invocation"`
	Status      string                  `json:"status"`
	Output      string                  `json:"output"`
	ContentHash string                  `json:"content-hash"`
	Truncated   bool                    `json:"truncated"`
	Error       string                  `json:"error,omitempty"`
	FinishedAt  string                  `json:"finished-at"`
	DurationMS  int64                   `json:"duration-ms"`
	Execution   *InvestigationExecution `json:"execution,omitempty"`
}

// Execution records physical outcomes; a copied working directory is not an
// OS sandbox. Retained workspaces with unknown process cleanup block replay.
type InvestigationExecution struct {
	Kind       string `json:"kind"`
	ExitCode   *int   `json:"exit-code,omitempty"`
	HTTPStatus int    `json:"http-status,omitempty"`
	Signal     string `json:"signal,omitempty"`
	TimedOut   bool   `json:"timed-out"`
	Cancelled  bool   `json:"cancelled"`
	Workspace  string `json:"workspace,omitempty"`
	Cleanup    string `json:"cleanup"`
	InputHash  string `json:"input-hash,omitempty"`
}

type InvestigationFinding struct {
	Hypothesis string   `json:"hypothesis"`
	Status     string   `json:"status"`
	Evidence   []string `json:"evidence"`
	Reason     string   `json:"reason"`
}

// A report records the analyst's conclusion and citations. It is neither a
// model pass nor a code-completion gate; the engine validates references only.
type InvestigationReport struct {
	Status          string                 `json:"status"`
	Summary         string                 `json:"summary"`
	Findings        []InvestigationFinding `json:"findings"`
	CounterEvidence string                 `json:"counter-evidence"`
	Limitations     []string               `json:"limitations"`
	Recommendation  string                 `json:"recommendation"`
	Regression      string                 `json:"regression"`
	Rollback        string                 `json:"rollback"`
}

type PreparationState struct {
	AnalysisPending   *PreparationAnalysisPending   `json:"analysis-pending,omitempty"`
	Analysis          *PreparationAnalysisResult    `json:"analysis,omitempty"`
	History           []PreparationRound            `json:"history,omitempty"`
	Version           int                           `json:"version"`
	Revision          int64                         `json:"revision"`
	RunID             string                        `json:"run-id"`
	Cwd               string                        `json:"cwd"`
	Contract          contract.Preparation          `json:"contract"`
	ContractHash      string                        `json:"contract-hash"`
	Status            string                        `json:"status"`
	Reason            string                        `json:"reason"`
	StartedAt         string                        `json:"started-at"`
	Deadline          string                        `json:"deadline"`
	ActionsUsed       int                           `json:"actions-used"`
	OutputBytes       int                           `json:"output-bytes"`
	ElapsedMS         int64                         `json:"elapsed-ms"`
	Pending           *PreparationInvocation        `json:"pending,omitempty"`
	DecisionPending   *decision.Snapshot            `json:"decision-pending,omitempty"`
	Observations      []InvestigationObservation    `json:"observations"`
	Decisions         []decision.Receipt            `json:"decisions"`
	DecisionStartedAt map[string]string             `json:"decision-started-at,omitempty"`
	JEVStatus         string                        `json:"jev-status,omitempty"`
	DecisionEffects   []InvestigationDecisionEffect `json:"decision-effects,omitempty"`
	Report            *InvestigationReport          `json:"report,omitempty"`
}

type PreparationSource struct {
	Round               int      `json:"round,omitempty"`
	Version             int      `json:"version"`
	ID                  string   `json:"id"`
	RunID               string   `json:"run-id"`
	ContractHash        string   `json:"contract-hash"`
	ReportHash          string   `json:"report-hash"`
	ImplementationScope []string `json:"implementation-scope"`
	ElapsedMS           int64    `json:"elapsed-ms"`
	Actions             int      `json:"actions"`
	Accounted           bool     `json:"accounted"`
}

func PreparationHash(value any) string {
	raw, _ := json.Marshal(value)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

func PreparationFile(dir string) string { return filepath.Join(dir, "preparation.json") }

func LoadPreparation(dir string) (*PreparationState, error) {
	raw, err := readPreparationData(dir, "preparation.json", 8<<20)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if err := contract.ValidatePreparationStateValue(value); err != nil {
		return nil, err
	}
	var st PreparationState
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&st); err != nil || !json.Valid(raw) {
		return nil, fmt.Errorf("invalid preparation state JSON")
	}
	if err := validatePreparationState(&st); err != nil {
		return nil, err
	}
	return &st, nil
}

// SavePreparation uses the same state-package CAS lock and durability contract
// as implementation state. Revision 0 can create only a previously absent file.
func SavePreparation(dir string, st *PreparationState) (*PreparationState, error) {
	if st.Revision < 0 || st.Revision == math.MaxInt64 {
		return nil, fmt.Errorf("invalid preparation revision")
	}
	next := *st
	next.Revision++
	if err := validatePreparationState(&next); err != nil {
		return nil, err
	}
	release, err := lockStateFile(dir)
	if err != nil {
		return nil, err
	}
	defer release()
	onDisk, err := LoadPreparation(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	found := int64(0)
	if onDisk != nil {
		found = onDisk.Revision
	}
	if found != st.Revision || (onDisk != nil && st.Revision == 0) {
		return nil, &CasConflictError{File: PreparationFile(dir), Expected: st.Revision, Found: found}
	}
	if err := validatePreparationTransition(onDisk, &next); err != nil {
		return nil, err
	}
	// validatePreparationState already marshalled and checked this exact value;
	// no fields are changed while holding the CAS lock.
	raw, _ := json.MarshalIndent(next, "", "  ")
	if err := writePreparationData(dir, "preparation.json", append(raw, '\n'), false); err != nil {
		return nil, err
	}
	return &next, nil
}

// An owned run holds this lock across selection, intent, dispatch and receipt.
// Status remains readable; concurrent run/report/cancel mutation is rejected.
func AcquirePreparationLease(dir string) (func(), error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	f, err := openPreparationEntry(root, ".preparation.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	root.Close()
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("preparation lease must be a regular file")
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("investigation already has an owner: %w", err)
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

func preparationReceiptFile(dir, id string) string {
	return filepath.Join(dir, ".investigation", "receipt-"+PreparationHash(id)+".json")
}

func SavePreparationReceipt(dir string, observation InvestigationObservation) error {
	raw, _ := json.Marshal(observation) // only concrete strings, integers and booleans
	var value any
	_ = json.Unmarshal(raw, &value)
	if err := contract.ValidatePreparationObservationValue(value); err != nil {
		return err
	}
	if err := validatePreparationObservation(observation); err != nil {
		return err
	}
	path := preparationReceiptFile(dir, observation.Invocation.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return fmt.Errorf("investigation receipt already exists or is inaccessible")
	}
	return writePreparationData(dir, filepath.Join(".investigation", filepath.Base(path)), raw, true)
}

func ReadPreparationReceipt(dir string, pending PreparationInvocation) (*InvestigationObservation, error) {
	path := preparationReceiptFile(dir, pending.ID)
	raw, err := readPreparationData(dir, filepath.Join(".investigation", filepath.Base(path)), 1<<20)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if err := contract.ValidatePreparationObservationValue(value); err != nil {
		return nil, err
	}
	var observation InvestigationObservation
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&observation) != nil || !json.Valid(raw) || PreparationHash(observation.Invocation) != PreparationHash(pending) || observation.DurationMS < 0 || (observation.Status != "observed" && observation.Status != "failed" && observation.Status != "cancelled" && observation.Status != "unknown") {
		return nil, fmt.Errorf("investigation receipt identity or result mismatch")
	}
	if err := validatePreparationObservation(observation); err != nil {
		return nil, err
	}
	return &observation, nil
}
