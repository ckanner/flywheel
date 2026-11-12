package circuitbreaker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func observedFailure() ProgressEvidence {
	return ProgressEvidence{
		Version: 1, Candidate: "candidate-a", CheckSet: "fixed-checks", Environment: "environment-a", Complete: true,
		Checks: []CheckObservation{
			{ID: "build", Pass: true},
			{ID: "test", ExitCode: 1, Failure: "failure-output-a"},
		},
	}
}

func observedSignature(p ProgressEvidence) string {
	return Signature([]string{"L1_FAILED"}, []string{"go test ./..."}, []string{"main.go"}, p)
}

func TestProgressStopsOnlyRepeatedAuthoritativeObservations(t *testing.T) {
	prior := observedSignature(observedFailure())
	assert.Equal(t, "stalled", AssessProgress(prior, prior).Status)
	assert.False(t, DetectSpin(prior, prior, 1))
	assert.True(t, DetectSpin(prior, prior, 2))

	for name, change := range map[string]func(*ProgressEvidence){
		"changed code is not no progress": func(p *ProgressEvidence) { p.Candidate = "candidate-b" },
		"changed environment":             func(p *ProgressEvidence) { p.Environment = "environment-b" },
		"narrowed checks":                 func(p *ProgressEvidence) { p.CheckSet = "narrower-checks" },
		"missing command":                 func(p *ProgressEvidence) { p.Checks = p.Checks[1:] },
		"alternating diagnostic":          func(p *ProgressEvidence) { p.Checks[1].Failure = "failure-output-b" },
		"different exit":                  func(p *ProgressEvidence) { p.Checks[1].ExitCode = 2 },
		"different command":               func(p *ProgressEvidence) { p.Checks[1].ID = "other-test" },
		"check transition on unchanged code": func(p *ProgressEvidence) {
			p.Checks[0] = CheckObservation{ID: "build", ExitCode: 1, Failure: "compile-error"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := observedFailure()
			change(&p)
			current := observedSignature(p)
			assert.Equal(t, "unknown", AssessProgress(prior, current).Status)
			assert.False(t, DetectSpin(prior, current, 9), "unknown progress must not invent an early spin proof")
		})
	}
	changedCode := Signature([]string{"SERVER"}, []string{"go test ./..."}, nil, observedFailure())
	assert.Equal(t, "unknown", AssessProgress(prior, changedCode).Status)
	assert.False(t, DetectSpin(prior, changedCode, 3))
}

func TestIncompleteProgressRetainsUnknown(t *testing.T) {
	prior := observedSignature(observedFailure())
	for name, change := range map[string]func(*ProgressEvidence){
		"legacy version":    func(p *ProgressEvidence) { p.Version = 0 },
		"interrupted":       func(p *ProgressEvidence) { p.Complete = false },
		"missing candidate": func(p *ProgressEvidence) { p.Candidate = "" },
		"missing checks":    func(p *ProgressEvidence) { p.CheckSet = "" },
		"unknown env":       func(p *ProgressEvidence) { p.Environment = "" },
		"no observations":   func(p *ProgressEvidence) { p.Checks = nil },
		"missing check ID":  func(p *ProgressEvidence) { p.Checks[0].ID = "" },
		"duplicate check":   func(p *ProgressEvidence) { p.Checks[1].ID = p.Checks[0].ID },
		"missing output":    func(p *ProgressEvidence) { p.Checks[1].Failure = "" },
		"no failed checks":  func(p *ProgressEvidence) { p.Checks[1] = CheckObservation{ID: "test", Pass: true} },
	} {
		t.Run(name, func(t *testing.T) {
			p := observedFailure()
			change(&p)
			current := observedSignature(p)
			assert.Equal(t, "unknown", AssessProgress(prior, current).Status)
			assert.False(t, DetectSpin(prior, current, 10))
		})
	}
	for _, previous := range []string{"", "{bad", Signature([]string{"L1_FAILED"}, []string{"go test ./..."}, nil)} {
		assert.Equal(t, "unknown", AssessProgress(previous, prior).Status)
		assert.False(t, DetectSpin(previous, prior, 2), "legacy signatures cannot fabricate a candidate observation")
	}
	assert.Equal(t, "unknown", AssessProgress(prior, "{bad").Status)
}
