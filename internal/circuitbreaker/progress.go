package circuitbreaker

import "encoding/json"

// CheckObservation is an orchestrator-run command result. ID identifies a fixed
// command and expectation; Failure identifies its complete diagnostic output.
// These are command IDs, never invented test-case IDs parsed from plain logs.
type CheckObservation struct {
	ID       string `json:"id"`
	Pass     bool   `json:"pass"`
	ExitCode int    `json:"exit-code"`
	Failure  string `json:"failure,omitempty"`
}

// ProgressEvidence records only facts whose scope is known. Environment covers
// filtered process variables, not arbitrary remote services or installed tools.
// Complete denotes command outcome collection, not a complete test inventory.
type ProgressEvidence struct {
	Version     int                `json:"version"`
	Candidate   string             `json:"candidate"`
	CheckSet    string             `json:"check-set"`
	Environment string             `json:"environment"`
	Complete    bool               `json:"complete"`
	Checks      []CheckObservation `json:"checks"`
}

type ProgressAssessment struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type failureSignature struct {
	Key      string            `json:"key"`
	Files    []string          `json:"files"`
	Progress *ProgressEvidence `json:"progress,omitempty"`
}

// AssessProgress compares recorded observations conservatively. Changed source
// plus fewer failed shell commands is not proof of progress: tests may have
// changed. Without a trusted fixed oracle, it remains unknown and the original
// attempt/budget limits still apply. Exact unchanged observations may stop early.
func AssessProgress(previousSignature, currentSignature string) ProgressAssessment {
	var previous, current failureSignature
	if json.Unmarshal([]byte(previousSignature), &previous) != nil || json.Unmarshal([]byte(currentSignature), &current) != nil {
		return ProgressAssessment{"unknown", "previous or current failure observation is unavailable"}
	}
	if previous.Key != current.Key {
		return ProgressAssessment{"unknown", "failure category or command set changed"}
	}
	return compareProgress(previous.Progress, current.Progress)
}

func completeProgress(observation *ProgressEvidence) bool {
	if observation == nil || observation.Version != 1 || !observation.Complete ||
		observation.Candidate == "" || observation.CheckSet == "" || observation.Environment == "" || len(observation.Checks) == 0 {
		return false
	}
	seen := map[string]bool{}
	failed := false
	for _, check := range observation.Checks {
		if check.ID == "" || seen[check.ID] || (!check.Pass && check.Failure == "") {
			return false
		}
		seen[check.ID] = true
		failed = failed || !check.Pass
	}
	return failed
}

func compareProgress(previous, current *ProgressEvidence) ProgressAssessment {
	if !completeProgress(previous) || !completeProgress(current) {
		return ProgressAssessment{"unknown", "complete authoritative command observations are required"}
	}
	if previous.CheckSet != current.CheckSet || previous.Environment != current.Environment || len(previous.Checks) != len(current.Checks) {
		return ProgressAssessment{"unknown", "check set or recorded environment changed"}
	}
	if previous.Candidate != current.Candidate {
		return ProgressAssessment{"unknown", "candidate changed; command logs do not prove a fixed test oracle"}
	}
	for i, check := range previous.Checks {
		if check != current.Checks[i] {
			return ProgressAssessment{"unknown", "unchanged candidate produced different command observations"}
		}
	}
	return ProgressAssessment{"stalled", "same candidate, check set, recorded environment and failures repeated"}
}
