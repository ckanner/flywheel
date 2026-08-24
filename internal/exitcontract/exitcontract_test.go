package exitcontract

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ckanner/flywheel/internal/errs"
)

func okDeliverables() []Deliverable {
	return []Deliverable{{Path: "docs/t/done-report.md", Exists: true, Valid: true}}
}

func TestEvaluateAllGreen(t *testing.T) {
	r := Evaluate(errs.SpawnResult{ExitCode: 0}, okDeliverables())
	assert.True(t, r.OK)
	assert.True(t, r.ProcessOK)
	assert.True(t, r.StderrOK)
	assert.True(t, r.DeliverablesOK)
	assert.Empty(t, r.ErrorCode)
	assert.Empty(t, r.Reasons)
}

func TestEvaluateProcessFailures(t *testing.T) {
	cases := []struct {
		name     string
		result   errs.SpawnResult
		wantCode string
		wantMsg  string
	}{
		{"timeout", errs.SpawnResult{ExitCode: 0, TimedOut: true}, "TIMEOUT",
			"process: exitCode=0 signal=null timedOut=true"},
		{"signal", errs.SpawnResult{ExitCode: 0, Signal: "SIGTERM"}, "TRANSPORT",
			"process: exitCode=0 signal=SIGTERM timedOut=false"},
		{"non-zero", errs.SpawnResult{ExitCode: 3}, "TRANSPORT",
			"process: exitCode=3 signal=null timedOut=false"},
		{"spawn error", errs.SpawnResult{SpawnError: errors.New("ENOENT")}, "TRANSPORT",
			"process: exitCode=0 signal=null timedOut=false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Evaluate(c.result, okDeliverables())
			assert.False(t, r.OK)
			assert.False(t, r.ProcessOK)
			assert.Equal(t, c.wantCode, r.ErrorCode)
			assert.Contains(t, r.Reasons, c.wantMsg)
		})
	}
}

func TestEvaluateStderrPollutionWithCleanExit(t *testing.T) {
	// 退出码 0 但 stderr 出现错误模式：进程项绿、stderr 项红。
	r := Evaluate(errs.SpawnResult{ExitCode: 0, Stderr: "Error: quota exceeded"}, okDeliverables())
	assert.False(t, r.OK)
	assert.True(t, r.ProcessOK)
	assert.False(t, r.StderrOK)
	assert.Equal(t, "QUOTA", r.ErrorCode)
	assert.Equal(t, []string{"stderr: looks like QUOTA"}, r.Reasons)
}

func TestEvaluateEmptyDeliverablesIsEmptyResponse(t *testing.T) {
	r := Evaluate(errs.SpawnResult{ExitCode: 0}, nil)
	assert.False(t, r.OK)
	assert.True(t, r.ProcessOK)
	assert.True(t, r.StderrOK)
	assert.False(t, r.DeliverablesOK)
	assert.Equal(t, "EMPTY_RESPONSE", r.ErrorCode)
}

func TestEvaluateMissingAndInvalidDeliverables(t *testing.T) {
	ds := []Deliverable{
		{Path: "a.md", Exists: false},
		{Path: "b.md", Exists: true, Valid: false},
		{Path: "c.md", Exists: true, Valid: false, Detail: "missing key x"},
		{Path: "d.md", Exists: true, Valid: true},
	}
	r := Evaluate(errs.SpawnResult{ExitCode: 0}, ds)
	assert.False(t, r.DeliverablesOK)
	assert.Equal(t, "EMPTY_RESPONSE", r.ErrorCode)
	// exit 0：无进程原因，reasons 全部来自交付物。
	assert.ElementsMatch(t, []string{
		"deliverable missing: a.md",
		"deliverable invalid: b.md (schema)",
		"deliverable invalid: c.md (missing key x)",
	}, r.Reasons)
}

func TestProcessFailureWinsOverDeliverables(t *testing.T) {
	// 优先级：进程失败 → errorCode 来自 classify，而非 EMPTY_RESPONSE。
	r := Evaluate(errs.SpawnResult{ExitCode: 2}, []Deliverable{{Path: "x", Exists: false}})
	assert.False(t, r.OK)
	assert.Equal(t, "TRANSPORT", r.ErrorCode)
	assert.Len(t, r.Reasons, 2)
}
