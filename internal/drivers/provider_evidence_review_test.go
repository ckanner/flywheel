package drivers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDSHCleanupAndProviderCauseRequireIntactTransportInEverySessionMode(t *testing.T) {
	for _, persistent := range []bool{false, true} {
		for _, scenario := range []string{"intact", "write-failure", "truncated", "signalled", "no-exit", "outer-timeout-with-truncation"} {
			t.Run(fmt.Sprintf("persistent=%t/%s", persistent, scenario), func(t *testing.T) {
				request := scopedDSHRequest()
				if !persistent {
					request.Protocol, request.Persistent = 1, false
				}
				ready, terminal := dshFixtureFrames(request)
				terminal["status"], terminal["text"] = "error", ""
				terminal["error"] = map[string]any{"code": "TIMEOUT", "message": "provider deadline elapsed"}
				if persistent {
					lifecycle := terminal["lifecycle"].(map[string]any)
					lifecycle["outcome"], lifecycle["resumeCandidate"] = "failed", false
				}
				exit := 1
				physical := watchdog.Result{Stdout: dshJSONL(ready, terminal), ExitCode: &exit}
				var writeErr error
				switch scenario {
				case "write-failure":
					writeErr = errors.New("start request was not written completely")
				case "truncated":
					physical.Truncated = true
				case "signalled":
					physical.Signal, physical.ExitCode = "killed", nil
				case "no-exit":
					physical.ExitCode = nil
				case "outer-timeout-with-truncation":
					physical.Truncated, physical.TimedOut, physical.IdleTimedOut = true, true, true
				}
				result := finishDSH(DSHArgs{PersistentSession: persistent, RequestID: request.RequestID}, request, physical, writeErr)
				require.NotNil(t, result.Session)
				assert.False(t, result.Settled)
				assert.False(t, result.Session.ResumeCandidate)
				assert.Equal(t, scenario == "intact", result.Session.CleanupConfirmed,
					"fresh and persistent calls require the same complete cleanup evidence")
				if scenario == "truncated" || scenario == "write-failure" {
					assert.False(t, result.TimedOut, "untrusted provider bytes cannot establish the initiating cause")
					assert.Equal(t, errs.TRANSPORT, result.ErrorCode)
				}
				if scenario == "outer-timeout-with-truncation" {
					assert.True(t, result.TimedOut, "locally observed watchdog timeout remains independent of receipt trust")
					assert.True(t, result.IdleTimedOut)
					assert.Equal(t, errs.TIMEOUT, result.ErrorCode)
				}
			})
		}
	}
}

func TestProEvidenceReviewProcessFixture(t *testing.T) {
	harness := os.Getenv("FLYWHEEL_TEST_PRO_EVIDENCE_REVIEW")
	if harness == "" {
		return
	}
	var request codexProStart
	if json.NewDecoder(os.Stdin).Decode(&request) != nil {
		os.Exit(91)
	}
	receipt := cancelledProFixture(harness, request)
	var execution map[string]any
	if harness == "codex-pro" {
		execution = proObject(receipt, "outcome", "execution")
	} else {
		execution = proObject(receipt, "execution")
	}
	execution["timedOut"] = true
	if request.Prompt == "unowned-execution" {
		execution["ownership"] = "unowned"
	}
	frame := codexProFrame(request, receipt)
	if harness == "claude-code-pro" {
		frame = claudeCodeProFrame(claudeCodeProStart(request), receipt)
	}
	if strings.HasPrefix(request.Prompt, "worker-error") {
		code := "INVALID_ACTIVITY"
		if request.Prompt == "worker-error-native" {
			code = "INVALID_NATIVE_EVENT"
		} else if request.Prompt == "worker-error-limit" {
			code = "OUTPUT_LIMIT"
		} else if request.Prompt == "worker-error-unknown" {
			code = "TRANSPORT"
		}
		frame["error"] = map[string]any{"code": code, "message": "worker observation failed"}
	}
	if request.Prompt == "foreign-adapter" || request.Prompt == "worker-error-foreign-adapter" {
		proObject(frame, "adapter")["version"] = "0.0.0"
	}
	_ = json.NewEncoder(os.Stdout).Encode(frame)
	if request.Prompt == "damaged-tail" || request.Prompt == "worker-error-damaged-tail" {
		fmt.Println("{damaged-after-timeout-receipt}")
	}
	if request.Prompt == "truncated-stderr" {
		fmt.Fprintln(os.Stderr, strings.Repeat("diagnostic", 8192))
	}
	os.Exit(1)
}

func TestProProviderTimeoutCannotReplaceKnownTransportFailure(t *testing.T) {
	bufferLimit := watchdog.MaxBuffer
	watchdog.MaxBuffer = 32768
	t.Cleanup(func() { watchdog.MaxBuffer = bufferLimit })
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		for _, scenario := range []string{"intact", "damaged-tail", "foreign-adapter", "worker-error", "worker-error-native", "worker-error-limit", "worker-error-unknown", "worker-error-foreign-adapter", "worker-error-damaged-tail", "truncated-stderr", "unowned-execution"} {
			t.Run(harness+"/"+scenario, func(t *testing.T) {
				command := kimiScript(t, "FLYWHEEL_TEST_PRO_EVIDENCE_REVIEW="+harness+" exec "+shQuote(os.Args[0])+" -test.run '^TestProEvidenceReviewProcessFixture$'\n")
				limits := watchdog.Opts{IdleTimeoutMs: 5000, WallTimeoutMs: 10000}
				var result Result
				if harness == "codex-pro" {
					result = RunCodexPro(CodexProArgs{Command: command, Cwd: t.TempDir(), Model: "gpt-test", Prompt: scenario,
						PermissionMode: "read-only", RequestID: "codex-evidence-review", Watchdog: limits})
				} else {
					result = RunClaudeCodePro(ClaudeCodeProArgs{Command: command, Cwd: t.TempDir(), Model: "sonnet", Prompt: scenario,
						PermissionMode: "danger-full-access", RequestID: "claude-evidence-review", Watchdog: limits})
				}
				assert.Equal(t, scenario == "intact", result.TimedOut)
				assert.False(t, result.Cancelled, "cleanup cancellation is not a caller stop")
				assert.False(t, result.Settled)
				require.NotNil(t, result.Session)
				observationFailure := scenario == "worker-error" || scenario == "worker-error-native" || scenario == "worker-error-limit"
				assert.Equal(t, scenario == "intact" || observationFailure, result.Session.CleanupConfirmed,
					"a valid worker error report does not corrupt its independently validated cleanup receipt")
				assert.False(t, result.Session.ResumeCandidate)
				if observationFailure {
					assert.NotEmpty(t, result.Session.RunID)
					assert.NotEmpty(t, result.Session.SessionID)
					assert.Equal(t, "unknown", result.Session.Outcome)
					assert.Empty(t, result.CleanupError)
				}
				require.NotNil(t, result.ErrorMessage)
				if scenario == "intact" {
					assert.Equal(t, errs.TIMEOUT, result.ErrorCode)
				} else {
					assert.NotEqual(t, errs.TIMEOUT, result.ErrorCode)
					assert.NotContains(t, *result.ErrorMessage, "timed out in the provider runtime")
				}
			})
		}
	}
}

func TestProObservationFailureNeverGrantsACompletedCheckpoint(t *testing.T) {
	request := codexProRequest(t)
	request.ThreadMode = "persistent"
	frame := codexProFrame(request, codexProFixture(request))
	frame["error"] = map[string]any{"code": "INVALID_ACTIVITY", "message": "invalid provider activity metadata"}
	codex := parseCodexProStream(dshJSONL(frame), request)
	assert.False(t, codex.StreamFailed)
	assert.True(t, codex.CleanupConfirmed)
	assert.False(t, codex.Settled)
	assert.False(t, codex.Session.ResumeCandidate)
	assert.Equal(t, "unknown", codex.Session.Outcome)
	assert.NotNil(t, codex.Usage)

	claudeRequest := claudeCodeProRequest(t)
	claudeRequest.ThreadMode = "persistent"
	claudeFrame := claudeCodeProFrame(claudeRequest, claudeCodeProFixture(claudeRequest))
	claudeFrame["error"] = frame["error"]
	claude := parseClaudeCodeProStream(dshJSONL(claudeFrame), claudeRequest)
	assert.False(t, claude.StreamFailed)
	assert.True(t, claude.CleanupConfirmed)
	assert.False(t, claude.Settled)
	assert.False(t, claude.Session.ResumeCandidate)
	assert.Equal(t, "unknown", claude.Session.Outcome)
	assert.NotNil(t, claude.Usage)
}

func TestProMissingDurableLifecycleCannotAttestCleanup(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		for _, observationError := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/observation-error=%t", harness, observationError), func(t *testing.T) {
				request := codexProRequest(t)
				request.Protocol = 3
				frame := codexProFrame(request, codexProFixture(request))
				if harness == "claude-code-pro" {
					request = codexProStart(claudeCodeProRequest(t))
					request.Protocol = 3
					frame = claudeCodeProFrame(claudeCodeProStart(request), claudeCodeProFixture(claudeCodeProStart(request)))
				}
				if observationError {
					frame["error"] = map[string]any{"code": "INVALID_ACTIVITY", "message": "worker observation failed"}
				}
				if harness == "codex-pro" {
					parsed := parseCodexProStream(dshJSONL(frame), request)
					assert.True(t, parsed.StreamFailed)
					assert.False(t, parsed.CleanupConfirmed)
					assert.False(t, parsed.Session.CleanupConfirmed)
					assert.False(t, parsed.Session.ResumeCandidate)
				} else {
					parsed := parseClaudeCodeProStream(dshJSONL(frame), claudeCodeProStart(request))
					assert.True(t, parsed.StreamFailed)
					assert.False(t, parsed.CleanupConfirmed)
					assert.False(t, parsed.Session.CleanupConfirmed)
					assert.False(t, parsed.Session.ResumeCandidate)
				}
			})
		}
	}
}
