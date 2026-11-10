// Package shadow evaluates workflow advice independently of the task engine.
// It never starts a task, changes a workflow, or writes task state/configuration.
package shadow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type Source struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}
type Baseline struct {
	Mode   string `json:"mode"`
	Reason string `json:"reason"`
}
type Label struct {
	Mode   string `json:"mode"`
	Status string `json:"status"`
	Author string `json:"author"`
	Reason string `json:"reason"`
}
type Case struct {
	ID           string   `json:"id"`
	Source       Source   `json:"source"`
	Task         string   `json:"task"`
	Evidence     []string `json:"evidence"`
	Baseline     Baseline `json:"baseline"`
	HardBlockers []string `json:"hard-blockers"`
	Label        *Label   `json:"label,omitempty"`
}
type Dataset struct {
	Version int    `json:"version"`
	Mode    string `json:"mode"`
	Cases   []Case `json:"cases"`
}
type Usage struct {
	Input  *int64 `json:"input_tokens"`
	Output *int64 `json:"output_tokens"`
}
type Record struct {
	Version        int                `json:"version"`
	Mode           string             `json:"mode"`
	Key            string             `json:"key"`
	CaseID         string             `json:"case-id"`
	InputDigest    string             `json:"input-digest"`
	QuestionDigest string             `json:"question-digest"`
	PolicyDigest   string             `json:"policy-digest"`
	TargetDigest   string             `json:"target-digest"`
	Status         string             `json:"status"`
	Advice         string             `json:"advice"`
	Reason         string             `json:"reason"`
	ActualMode     string             `json:"actual-mode"`
	Model          string             `json:"model"`
	At             string             `json:"at"`
	DurationMS     int64              `json:"duration-ms"`
	HTTPStatus     int                `json:"http-status"`
	Probabilities  map[string]float64 `json:"probabilities"`
	Usage          Usage              `json:"usage"`
}

// jsonBytes only receives schema-validated, acyclic data with finite counters.
func jsonBytes(value any) []byte {
	raw, _ := json.Marshal(value)
	return raw
}

func digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Labels are attached after assessment; changing a label never calls a model.
func caseDigest(c Case) string {
	c.Label = nil
	return digest(jsonBytes(c))
}
