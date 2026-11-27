package drivers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexProScopedStartAndResumeFacts(t *testing.T) {
	for _, threadID := range []string{"", "opaque thread/with\"punctuation"} {
		request := codexProRequest(t)
		request.Protocol, request.ThreadMode, request.ThreadID = 2, "persistent", threadID
		receipt := codexProFixture(request)
		proObject(receipt, "outcome", "usage")["threadTotal"] = map[string]any{"totalTokens": 1000000}
		parsed := parseCodexProStream(dshJSONL(codexProFrame(request, receipt)), request)
		require.True(t, parsed.Settled, "%+v", parsed)
		require.NotNil(t, parsed.Session)
		assert.Equal(t, "codex-pro/"+CodexProPackageVersion+"/worker-v2", parsed.Session.Protocol)
		assert.Equal(t, request.RequestID, parsed.Session.RequestID)
		assert.Equal(t, "run-one", parsed.Session.RunID)
		assert.Equal(t, "turn-one", parsed.Session.TurnID)
		assert.Equal(t, parsed.SessionID, parsed.Session.SessionID)
		assert.Equal(t, "completed", parsed.Session.Outcome)
		assert.Equal(t, "dispatched", parsed.Session.Dispatch)
		assert.True(t, parsed.Session.CleanupConfirmed)
		assert.True(t, parsed.Session.ResumeCandidate)
		assert.Equal(t, threadID != "", parsed.Session.ResumeAccepted)
		assert.Equal(t, "complete", parsed.Session.UsageStatus)
		assert.Equal(t, int64(18), parsed.Usage.TotalTokens, "only the attributable run delta is billable")
	}
	request := codexProRequest(t)
	request.Protocol, request.ThreadMode = 2, "persistent"
	for _, id := range []string{" ", "bad\x00id"} {
		receipt := codexProFixture(request)
		proObject(receipt, "outcome")["threadId"] = id
		proObject(receipt, "outcome", "execution")["threadId"] = id
		parsed := parseCodexProStream(dshJSONL(codexProFrame(request, receipt)), request)
		assert.False(t, parsed.Settled)
		assert.False(t, parsed.Session.ResumeCandidate)
	}
}

func TestCodexProResumeCannotAdmitAnotherThreadOrStartEvidence(t *testing.T) {
	request := codexProRequest(t)
	request.Protocol, request.ThreadMode, request.ThreadID = 2, "persistent", "owned-thread"
	for name, change := range map[string]func(map[string]any){
		"requested another thread":    func(r map[string]any) { proObject(r, "requested")["threadId"] = "other" },
		"requested thread missing":    func(r map[string]any) { delete(proObject(r, "requested"), "threadId") },
		"requested thread null":       func(r map[string]any) { proObject(r, "requested")["threadId"] = nil },
		"requested thread non-string": func(r map[string]any) { proObject(r, "requested")["threadId"] = 42 },
		"silent cold start":           func(r map[string]any) { proObject(r, "outcome", "configuration")["source"] = "thread/start" },
		"foreign returned thread": func(r map[string]any) {
			proObject(r, "outcome")["threadId"] = "foreign"
			proObject(r, "outcome", "execution")["threadId"] = "foreign"
		},
		"foreign turn execution":  func(r map[string]any) { proObject(r, "outcome", "execution")["threadId"] = "foreign" },
		"missing turn acceptance": func(r map[string]any) { proObject(r, "outcome", "turnConfiguration")["accepted"] = false },
		"bad usage":               func(r map[string]any) { proObject(r, "outcome", "usage")["status"] = "guessed" },
		"timed out":               func(r map[string]any) { proObject(r, "outcome", "execution")["timedOut"] = true },
		"cancelled":               func(r map[string]any) { proObject(r, "outcome", "execution")["cancelled"] = true },
		"cleanup unknown":         func(r map[string]any) { proObject(r, "outcome", "cleanup")["confirmed"] = false },
	} {
		t.Run(name, func(t *testing.T) {
			receipt := codexProFixture(request)
			change(receipt)
			parsed := parseCodexProStream(dshJSONL(codexProFrame(request, receipt)), request)
			assert.False(t, parsed.Settled)
			assert.False(t, parsed.Session.ResumeCandidate)
			assert.Equal(t, "unknown", parsed.Session.Outcome)
		})
	}
	frame := codexProFrame(request, codexProFixture(request))
	for name, stream := range map[string]string{
		"trailing corruption": dshJSONL(frame) + "bad\n", "duplicate receipt": dshJSONL(frame, frame), "missing receipt": "",
	} {
		t.Run(name, func(t *testing.T) {
			parsed := parseCodexProStream(stream, request)
			assert.False(t, parsed.Session.ResumeCandidate)
			assert.Equal(t, "unknown", parsed.Session.Outcome)
			assert.False(t, parsed.Session.CleanupConfirmed)
			assert.Nil(t, parsed.Usage, "a damaged/duplicated stream cannot establish accountable usage")
		})
	}
	frame["error"] = map[string]any{"code": "INVALID_REQUEST", "message": "second request"}
	assert.Equal(t, "unknown", parseCodexProStream(dshJSONL(frame), request).Session.Outcome)
}

func TestCodexProTerminalFactsSurviveModelOutputAndUsageGaps(t *testing.T) {
	request := codexProRequest(t)
	request.Protocol, request.ThreadMode = 2, "persistent"
	for _, status := range []string{"complete", "partial", "unknown"} {
		receipt := codexProFixture(request)
		proObject(receipt, "outcome", "usage")["status"] = status
		proObject(receipt, "outcome")["output"] = []any{map[string]any{"type": "text", "text": " "}}
		parsed := parseCodexProStream(dshJSONL(codexProFrame(request, receipt)), request)
		assert.False(t, parsed.Settled)
		assert.Contains(t, parsed.Error, "empty response")
		assert.Equal(t, "completed", parsed.Session.Outcome)
		assert.True(t, parsed.Session.ResumeCandidate, "a confirmed completed attempt may repair its output")
		assert.True(t, parsed.Session.CleanupConfirmed)
		assert.Equal(t, status, parsed.Session.UsageStatus)
		if status != "complete" {
			assert.Nil(t, parsed.Usage)
		} else {
			require.NotNil(t, parsed.Usage, "invalid output does not erase attributable consumption")
			assert.Equal(t, int64(18), parsed.Usage.TotalTokens)
		}
	}
}

func TestCodexProUsageRequiresMatchingInvocationAndExecution(t *testing.T) {
	request := codexProRequest(t)
	request.Protocol, request.ThreadMode, request.ThreadID = 2, "persistent", "owned-thread"
	for name, change := range map[string]func(map[string]any){
		"request ID":       func(r map[string]any) { r["requestId"] = "foreign" },
		"run ID":           func(r map[string]any) { proObject(r, "outcome", "execution")["runId"] = "foreign" },
		"execution thread": func(r map[string]any) { proObject(r, "outcome", "execution")["threadId"] = "foreign" },
		"returned thread": func(r map[string]any) {
			proObject(r, "outcome")["threadId"] = "foreign"
			proObject(r, "outcome", "execution")["threadId"] = "foreign"
		},
		"missing execution":     func(r map[string]any) { proObject(r, "outcome")["execution"] = nil },
		"unowned execution":     func(r map[string]any) { proObject(r, "outcome", "execution")["ownership"] = "foreign-process" },
		"missing turn identity": func(r map[string]any) { proObject(r, "outcome", "execution")["turnId"] = nil },
		"invalid turn identity": func(r map[string]any) { proObject(r, "outcome", "execution")["turnId"] = "bad\x00turn" },
	} {
		t.Run(name, func(t *testing.T) {
			receipt := codexProFixture(request)
			change(receipt)
			parsed := parseCodexProStream(dshJSONL(codexProFrame(request, receipt)), request)
			assert.Nil(t, parsed.Usage)
			assert.Equal(t, "unknown", parsed.Session.UsageStatus)
			assert.NotEmpty(t, parsed.Receipt)
		})
	}
	for _, corruption := range []string{"adapter", "worker-error", "missing-terminal-field"} {
		t.Run(corruption, func(t *testing.T) {
			frame := codexProFrame(request, codexProFixture(request))
			switch corruption {
			case "adapter":
				proObject(frame, "adapter")["version"] = "unverified"
			case "worker-error":
				frame["error"] = map[string]any{"code": "TRANSPORT", "message": "outer protocol failure"}
			case "missing-terminal-field":
				delete(frame, "error")
			}
			parsed := parseCodexProStream(dshJSONL(frame), request)
			assert.Nil(t, parsed.Usage)
			assert.Equal(t, "unknown", parsed.Session.UsageStatus)
			assert.NotEmpty(t, parsed.Receipt)
		})
	}
	// A failure of this identified run does not erase its known consumption.
	receipt := codexProFixture(request)
	outcome := proObject(receipt, "outcome")
	execution := proObject(outcome, "execution")
	execution["failure"] = map[string]any{"stage": "turn-stream", "category": "transport"}
	receipt["ok"], receipt["outcome"] = false, nil
	receipt["failure"] = map[string]any{"stage": "turn-stream", "category": "transport", "diagnostic": "lost outcome",
		"execution": execution, "cleanup": outcome["cleanup"], "usage": outcome["usage"], "configuration": nil, "turnConfiguration": nil}
	parsed := parseCodexProStream(dshJSONL(codexProFrame(request, receipt)), request)
	assert.False(t, parsed.Settled)
	assert.Equal(t, "unknown", parsed.Session.Outcome)
	require.NotNil(t, parsed.Usage)
	assert.Equal(t, int64(18), parsed.Usage.TotalTokens)
	assert.Equal(t, "complete", parsed.Session.UsageStatus)
}

func TestCodexProKnownStartupFailureVersusAmbiguousResume(t *testing.T) {
	for _, stage := range []string{"request", "initialize", "thread-start", "thread-resume", "turn-start"} {
		request := codexProRequest(t)
		request.Protocol, request.ThreadMode = 2, "persistent"
		if stage == "thread-resume" {
			request.ThreadID = "owned-thread"
		}
		receipt := codexProFixture(request)
		execution := proObject(receipt, "outcome", "execution")
		execution["threadId"], execution["turnId"] = nil, nil
		execution["failure"] = map[string]any{"stage": stage, "category": "protocol"}
		receipt["ok"], receipt["outcome"] = false, nil
		receipt["failure"] = map[string]any{"stage": stage, "category": "protocol", "diagnostic": "no rollout or thread busy",
			"execution": execution, "cleanup": map[string]any{"confirmed": true}, "usage": nil, "configuration": nil, "turnConfiguration": nil}
		parsed := parseCodexProStream(dshJSONL(codexProFrame(request, receipt)), request)
		assert.False(t, parsed.Settled)
		assert.True(t, parsed.Session.CleanupConfirmed)
		assert.False(t, parsed.Session.ResumeCandidate)
		if stage == "thread-resume" || stage == "turn-start" {
			assert.Equal(t, "unknown", parsed.Session.Dispatch, "error prose is not proof of safe replay")
			assert.Equal(t, "unknown", parsed.Session.Outcome)
		} else {
			assert.Equal(t, "not-dispatched", parsed.Session.Dispatch)
			assert.Equal(t, "failed", parsed.Session.Outcome)
		}
	}
}

func TestRunCodexProUsesExplicitInvocationAndPersistentThread(t *testing.T) {
	a := codexProRunArgs(t, "record")
	a.RequestID, a.PersistentSession = "durable-invocation-1", true
	first := RunCodexPro(a)
	require.True(t, first.Settled, "%+v", first)
	assert.Equal(t, a.RequestID, first.Session.RequestID)
	assert.True(t, first.Session.ResumeCandidate)
	assert.False(t, first.Session.ResumeAccepted)
	a.RequestID, a.SessionID = "durable-invocation-2", first.SessionID
	second := RunCodexPro(a)
	require.True(t, second.Settled, "%+v", second)
	assert.Equal(t, a.SessionID, second.SessionID)
	assert.Equal(t, a.RequestID, second.Session.RequestID)
	assert.True(t, second.Session.ResumeAccepted)
	raw, err := os.ReadFile(filepath.Join(a.Cwd, "worker-input.json"))
	require.NoError(t, err)
	var input struct {
		Request codexProStart `json:"request"`
	}
	require.NoError(t, json.Unmarshal(raw, &input))
	assert.Equal(t, 2, input.Request.Protocol)
	assert.Equal(t, "persistent", input.Request.ThreadMode)
	assert.Equal(t, a.SessionID, input.Request.ThreadID)
	assert.Equal(t, a.RequestID, input.Request.RequestID)
	for _, change := range []func(*CodexProArgs){
		func(a *CodexProArgs) { a.PersistentSession = false },
		func(a *CodexProArgs) { a.SessionID = " \t" },
		func(a *CodexProArgs) { a.SessionID = "bad\x00id" },
		func(a *CodexProArgs) { a.RequestID = " " },
		func(a *CodexProArgs) { a.RequestID = "bad\x00id" },
		func(a *CodexProArgs) { a.RequestID = strings.Repeat("a", 257) },
	} {
		bad := a
		change(&bad)
		r := RunCodexPro(bad)
		assert.False(t, r.Settled)
		assert.Equal(t, "not-dispatched", r.Session.Dispatch)
		assert.Equal(t, "failed", r.Session.Outcome)
		assert.True(t, r.Session.CleanupConfirmed)
	}
}

func TestRunCodexProFreshUsesEngineInvocationWithoutPersistentHistory(t *testing.T) {
	a := codexProRunArgs(t, "record")
	a.RequestID = "call-freshEngine123"
	r := RunCodexPro(a)
	require.True(t, r.Settled, "%+v", r)
	require.NotNil(t, r.Session)
	assert.Equal(t, a.RequestID, r.Session.RequestID)
	assert.Equal(t, "codex-pro/"+CodexProPackageVersion+"/worker-v1", r.Session.Protocol)
	assert.Equal(t, "completed", r.Session.Outcome)
	assert.Equal(t, "dispatched", r.Session.Dispatch)
	assert.True(t, r.Session.CleanupConfirmed)
	assert.False(t, r.Session.ResumeCandidate)
	assert.False(t, r.Session.ResumeAccepted)
	raw, err := os.ReadFile(filepath.Join(a.Cwd, "worker-input.json"))
	require.NoError(t, err)
	var input struct {
		Request map[string]any `json:"request"`
	}
	require.NoError(t, json.Unmarshal(raw, &input))
	assert.Equal(t, a.RequestID, input.Request["requestId"])
	assert.Equal(t, float64(1), input.Request["protocol"])
	assert.NotContains(t, input.Request, "threadMode")
	assert.NotContains(t, input.Request, "threadId")
	var receipt codexProReceipt
	require.NoError(t, json.Unmarshal(r.CodexProReceipt, &receipt))
	assert.Equal(t, a.RequestID, receipt.RequestID)
	assert.Equal(t, "null", string(receipt.Requested.ThreadID))
}

func TestRunCodexProPhysicalFailureNeverGrantsContinuation(t *testing.T) {
	for _, scenario := range []string{"exit-error", "adapter-timeout", "adapter-truncated", "cleanup-error", "empty"} {
		a := codexProRunArgs(t, scenario)
		a.PersistentSession, a.RequestID = true, "owned-attempt"
		r := RunCodexPro(a)
		assert.False(t, r.Settled)
		assert.Equal(t, "unknown", r.Session.Outcome)
		assert.False(t, r.Session.ResumeCandidate)
	}
}

func TestRunCodexProTruncatedCaptureCannotBillCompleteLookingPrefix(t *testing.T) {
	previousLimit := watchdog.MaxBuffer
	t.Cleanup(func() { watchdog.MaxBuffer = previousLimit })
	watchdog.MaxBuffer = 8192
	r := RunCodexPro(codexProRunArgs(t, "outer-truncated"))
	require.True(t, r.Truncated)
	require.NotEmpty(t, r.CodexProReceipt)
	assert.False(t, r.Settled)
	assert.Nil(t, r.Usage)
	assert.Equal(t, "unknown", r.Session.UsageStatus)
	assert.Equal(t, "unknown", r.Session.Outcome)
}

func TestRunCodexProReadOnlyOutputLimitRetiresThreadForSafeRetry(t *testing.T) {
	a := codexProRunArgs(t, "read-only-output-limit")
	a.PermissionMode = "read-only"
	a.PersistentSession, a.RequestID = true, "ro-output-limit-1"
	r := RunCodexPro(a)
	require.False(t, r.Settled)
	require.True(t, r.Truncated)
	require.True(t, r.Session.CleanupConfirmed)
	assert.Equal(t, "dispatched", r.Session.Dispatch)
	assert.Equal(t, "failed", r.Session.Outcome)
	assert.False(t, r.Session.ResumeCandidate)
	assert.Equal(t, errs.TRANSPORT, r.ErrorCode)
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "16777216 bytes")

	// Workspace-write output-limit must still remain unknown (strictly blocked).
	w := codexProRunArgs(t, "read-only-output-limit")
	w.PermissionMode = "workspace-write"
	w.PersistentSession, w.RequestID = true, "rw-output-limit-1"
	wr := RunCodexPro(w)
	assert.False(t, wr.Settled)
	assert.True(t, wr.Truncated)
	assert.Equal(t, "unknown", wr.Session.Outcome)
	assert.False(t, wr.Session.ResumeCandidate)
}
