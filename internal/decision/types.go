// Package decision evaluates bounded, preauthorized actions. It never executes
// an action or changes task state: the orchestrator owns adoption and dispatch.
package decision

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"unicode/utf8"
)

type Mode string

const (
	Off      Mode = "off"
	Offline  Mode = "offline"
	Shadow   Mode = "shadow"
	Advisory Mode = "advisory"
	Adopt    Mode = "adopt"
)

type Category string

const (
	TaskSummary     Category = "task-summary"
	DiagnosticFacts Category = "diagnostic-facts"
	Logs            Category = "logs"
	Source          Category = "source"
	Diff            Category = "diff"
)

type ActionKind string

const (
	Diagnostic ActionKind = "diagnostic"
	Repair     ActionKind = "repair"
	Replan     ActionKind = "replan"
	Model      ActionKind = "model"
)

// Identity refers to the state AFTER the owner has reserved the decision. The
// separate decision journal does not advance this revision. Any later change to
// relevant facts must create a new identity and makes the answer stale.
type Identity struct {
	TaskID          string `json:"task-id"`
	RunID           string `json:"run-id"`
	Revision        int64  `json:"state-revision"`
	ContractHash    string `json:"contract-hash"`
	CandidateHash   string `json:"candidate-hash"`
	ObservationHash string `json:"observation-hash"`
}

type Fact struct {
	Category Category `json:"category"`
	Text     string   `json:"text"`
}

// BindingHash identifies a concrete action whose arguments, environment,
// preconditions, timeout, and authority the Go owner has already checked. The
// API never receives a command and cannot create or edit an action binding.
type Action struct {
	ID          string     `json:"id"`
	Kind        ActionKind `json:"kind"`
	Description string     `json:"description"`
	BindingHash string     `json:"binding-hash"`
}

type Snapshot struct {
	Version             int      `json:"version"`
	ID                  string   `json:"decision-id"`
	Identity            Identity `json:"identity"`
	Facts               []Fact   `json:"facts"`
	MissingFacts        []string `json:"missing-facts"`
	Actions             []Action `json:"actions"`
	BaselineID          string   `json:"baseline-id"`
	SemanticUncertainty bool     `json:"semantic-uncertainty"`
	ExecutorUnknown     bool     `json:"executor-unknown"`
}

// Runtime policies are frozen by the owner from the unified JEV configuration.
// Thresholds are explicit decision parameters, not calibrated accuracy.
type Policy struct {
	Version          string     `json:"version"`
	Mode             Mode       `json:"mode"`
	AllowedData      []Category `json:"allowed-data"`
	MinApplicability float64    `json:"min-applicability"`
	MinChoice        float64    `json:"min-choice"`
	MaxCalls         int        `json:"max-calls"`
}

type Usage struct {
	Input  *int64 `json:"input_tokens"`
	Output *int64 `json:"output_tokens"`
}

type Receipt struct {
	Backend          string             `json:"backend,omitempty"`
	Native           *NativeExecution   `json:"native,omitempty"`
	Version          int                `json:"version"`
	DecisionID       string             `json:"decision-id"`
	SnapshotHash     string             `json:"snapshot-hash"`
	PolicyHash       string             `json:"policy-hash"`
	TargetHash       string             `json:"target-hash"`
	QuestionHash     string             `json:"question-hash"`
	Status           string             `json:"status"`
	Reason           string             `json:"reason"`
	ActionID         string             `json:"action-id"`
	Model            string             `json:"model"`
	Applicability    *float64           `json:"applicability"`
	ChoiceConfidence *float64           `json:"choice-confidence"`
	Probabilities    map[string]float64 `json:"probabilities"`
	Usage            Usage              `json:"usage"`
	HTTPStatus       int                `json:"http-status"`
	DurationMS       int64              `json:"duration-ms"`
}

type Selection struct {
	ActionID     string `json:"action-id"`
	InvocationID string `json:"action-invocation-id"`
	Adopted      bool   `json:"adopted"`
	Reason       string `json:"reason"`
}

var token = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,119}$`)

func raw(value any) []byte { b, _ := json.Marshal(value); return b }
func hash(value any) string {
	sum := sha256.Sum256(raw(value))
	return hex.EncodeToString(sum[:])
}

func probability(value float64) bool { return !math.IsNaN(value) && value >= 0 && value <= 1 }

func validText(value string, limit int) bool {
	return len(value) > 0 && len(value) <= limit && utf8.ValidString(value)
}

func validCategory(c Category) bool {
	return c == TaskSummary || c == DiagnosticFacts || c == Logs || c == Source || c == Diff
}

func Validate(s Snapshot, p Policy) error {
	if (s.Version != 1 && s.Version != 2) || !token.MatchString(s.ID) || s.Identity.Revision < 0 || !validText(s.Identity.TaskID, 512) || !validText(s.Identity.RunID, 256) || !validText(s.Identity.ContractHash, 128) || !validText(s.Identity.CandidateHash, 128) || !validText(s.Identity.ObservationHash, 128) {
		return fmt.Errorf("invalid decision identity")
	}
	if !token.MatchString(p.Version) || (p.Mode != Off && p.Mode != Offline && p.Mode != Shadow && p.Mode != Advisory && p.Mode != Adopt) || p.MaxCalls < 1 || p.MaxCalls > 100 || !probability(p.MinApplicability) || p.MinApplicability <= 0 || !probability(p.MinChoice) || p.MinChoice <= 0 {
		return fmt.Errorf("invalid decision policy")
	}
	for _, category := range p.AllowedData {
		if !validCategory(category) {
			return fmt.Errorf("invalid decision data category")
		}
	}
	if len(s.Facts) > 32 || len(s.MissingFacts) > 16 || len(s.Actions) < 1 || len(s.Actions) > 16 {
		return fmt.Errorf("decision snapshot exceeds bounds")
	}
	for _, fact := range s.Facts {
		if !validCategory(fact.Category) || !validText(fact.Text, 2048) {
			return fmt.Errorf("invalid decision fact")
		}
	}
	for _, missing := range s.MissingFacts {
		if !validText(missing, 512) {
			return fmt.Errorf("invalid missing fact")
		}
	}
	seen := map[string]bool{}
	for _, action := range s.Actions {
		if !token.MatchString(action.ID) || seen[action.ID] || !validText(action.Description, 1024) || !validText(action.BindingHash, 128) || (action.Kind != Diagnostic && action.Kind != Repair && action.Kind != Replan && action.Kind != Model) {
			return fmt.Errorf("invalid or duplicate decision action")
		}
		seen[action.ID] = true
	}
	if !seen[s.BaselineID] {
		return fmt.Errorf("decision baseline is not an allowed action")
	}
	return nil
}

func dataAllowed(s Snapshot, p Policy) bool {
	allowed := map[Category]bool{}
	for _, category := range p.AllowedData {
		allowed[category] = true
	}
	// Action descriptions and missing-fact descriptions are diagnostic data too.
	if !allowed[DiagnosticFacts] {
		return false
	}
	for _, fact := range s.Facts {
		if !allowed[fact.Category] {
			return false
		}
	}
	return true
}

// Select only returns a proposal. The owner must recheck the selected concrete
// binding and budget and CAS-save this invocation ID before dispatch. Unknown
// executor outcomes prohibit even the deterministic baseline.
func Select(current Snapshot, receipt Receipt, p Policy, options Options) Selection {
	selection := Selection{ActionID: current.BaselineID, Reason: "deterministic-baseline"}
	if Validate(current, p) != nil || current.ExecutorUnknown {
		return Selection{Reason: "invalid-or-unknown-execution"}
	}
	if p.Mode != Adopt {
		return selection
	}
	if !validTarget(options) || !dataAllowed(current, p) || options.HardBudget || !current.SemanticUncertainty || len(current.Actions) < 2 {
		selection.Reason = "adoption-not-eligible"
		return selection
	}
	if receipt.Version != 1 || receipt.DecisionID != current.ID || receipt.SnapshotHash != hash(current) || receipt.PolicyHash != hash(p) || receipt.QuestionHash != hash(questions(current)) || receipt.TargetHash != targetHash(options) || receipt.Model != options.ExpectedModel {
		selection.Reason = "stale-decision"
		return selection
	}
	if receipt.Status != "ok" || !successfulTransport(receipt, options) || receipt.ActionID == "" {
		selection.Reason = "decision-abstained"
		return selection
	}
	if !validAnswer(current, receipt) || receipt.Applicability == nil || *receipt.Applicability < p.MinApplicability || receipt.Probabilities[receipt.ActionID] < p.MinChoice {
		selection.Reason = "decision-not-applicable"
		return selection
	}
	kinds := options.AllowedKinds
	if kinds == nil {
		kinds = []ActionKind{Diagnostic}
	}
	for _, action := range current.Actions {
		if action.ID == receipt.ActionID && slices.Contains(kinds, action.Kind) {
			selection.ActionID = action.ID
			selection.Adopted = true
			selection.Reason = "approved-" + string(action.Kind) + "-choice"
			selection.InvocationID = hash(struct{ Decision, Snapshot, Action string }{current.ID, receipt.SnapshotHash, action.BindingHash})
			return selection
		}
	}
	selection.Reason = "action-family-not-enabled"
	return selection
}
