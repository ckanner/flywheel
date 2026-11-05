package drivers

import (
	"testing"

	"github.com/ckanner/flywheel/internal/errs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dshErrorFrame(request dshStart, code errs.ErrorCode) map[string]any {
	return map[string]any{"type": "result", "protocol": 1, "requestId": request.RequestID,
		"status": "error", "settled": true,
		"error": map[string]any{"code": code, "message": "请检查账号状态；quota 一词仅为恢复说明"}}
}

func TestDSHStructuredErrorsRequireACompleteMatchingProtocolReceipt(t *testing.T) {
	request := dshStart{Protocol: 1, RequestID: "structured-error", Cwd: "/repo", Provider: "deepseek-account",
		Model: "deepseek-flash", ReasoningEffort: "high", PermissionMode: "read-only"}
	ready, _ := dshFixtureFrames(request)
	for _, code := range errs.AllCodes {
		frame := dshErrorFrame(request, code)
		startup := parseDSHStream(dshJSONL(frame), request)
		assert.Equal(t, code, startup.ErrorCode)
		assert.False(t, startup.Settled)
		assert.Empty(t, startup.CleanupError)
		frame["sessionId"] = ready["sessionId"]
		afterReady := parseDSHStream(dshJSONL(ready, frame), request)
		assert.Equal(t, code, afterReady.ErrorCode)
		assert.Equal(t, string(code)+": 请检查账号状态；quota 一词仅为恢复说明", afterReady.Error)
		assert.False(t, afterReady.Settled)
	}
	for _, code := range []errs.ErrorCode{"", "auth", "ACCOUNT_SIGN_IN_REQUIRED", "FUTURE_CODE"} {
		parsed := parseDSHStream(dshJSONL(dshErrorFrame(request, code)), request)
		assert.Equal(t, errs.TRANSPORT, parsed.ErrorCode)
		assert.NotContains(t, parsed.Error, "quota", "invalid protocol codes must not re-enter message classification")
	}
	for _, scenario := range []string{"wrong session", "wrong request", "wrong protocol", "trailing frame", "malformed tail"} {
		t.Run(scenario, func(t *testing.T) {
			frame := dshErrorFrame(request, errs.AUTH)
			frame["sessionId"] = ready["sessionId"]
			suffix := ""
			switch scenario {
			case "wrong session":
				frame["sessionId"] = "another-session"
			case "wrong request":
				frame["requestId"] = "another-request"
			case "wrong protocol":
				frame["protocol"] = 2
			case "trailing frame":
				suffix = dshJSONL(ready)
			case "malformed tail":
				suffix = "{not-json}\n"
			}
			parsed := parseDSHStream(dshJSONL(ready, frame)+suffix, request)
			assert.Equal(t, errs.TRANSPORT, parsed.ErrorCode)
			assert.False(t, parsed.Settled)
			assert.NotContains(t, parsed.Error, "quota")
		})
	}
	frame := dshErrorFrame(request, errs.AUTH)
	frame["settled"] = false
	parsed := parseDSHStream(dshJSONL(frame), request)
	assert.Equal(t, errs.AUTH, parsed.ErrorCode, "retain the provider code separately from the cleanup fact")
	assert.Contains(t, parsed.CleanupError, "did not confirm")
	assert.False(t, parsed.Settled)
}

func TestRunDSHPreservesStructuredErrorsAndUnconfirmedCleanup(t *testing.T) {
	for _, scenario := range []string{"structured-auth", "structured-quota", "error-unconfirmed-cleanup", "error-before-timeout", "error-exit-zero", "error-exit-other"} {
		t.Run(scenario, func(t *testing.T) {
			args := dshRunArgs(t, scenario)
			if scenario == "error-before-timeout" {
				args.Watchdog.WallTimeoutMs = 100
			}
			result := RunDSH(args)
			assert.False(t, result.Settled)
			require.NotNil(t, result.ErrorMessage)
			if scenario == "structured-quota" {
				assert.Equal(t, errs.QUOTA, result.ErrorCode)
			} else if scenario == "error-exit-zero" || scenario == "error-exit-other" {
				assert.Equal(t, errs.TRANSPORT, result.ErrorCode)
			} else {
				assert.Equal(t, errs.AUTH, result.ErrorCode)
			}
			switch scenario {
			case "error-unconfirmed-cleanup":
				assert.Contains(t, result.CleanupError, "did not confirm SDK process cleanup")
				assert.Equal(t, result.CleanupError, *result.ErrorMessage)
			case "error-before-timeout":
				assert.True(t, result.TimedOut)
				assert.Contains(t, *result.ErrorMessage, "timed out")
			case "error-exit-zero", "error-exit-other":
				assert.Contains(t, *result.ErrorMessage, "did not match the expected failure exit 1")
			default:
				assert.Empty(t, result.CleanupError)
				require.NotNil(t, result.ExitCode)
				assert.Equal(t, 1, *result.ExitCode)
				assert.Contains(t, *result.ErrorMessage, "请重新检查")
			}
		})
	}
}

func TestDSHSuccessReceiptCannotOverrideUnexpectedPhysicalExit(t *testing.T) {
	result := RunDSH(dshRunArgs(t, "exit-error-auth-text"))
	assert.False(t, result.Settled)
	require.NotNil(t, result.ExitCode)
	assert.Equal(t, 42, *result.ExitCode)
	require.NotNil(t, result.ErrorMessage)
	assert.Contains(t, *result.ErrorMessage, "401 unauthorized")
	assert.Equal(t, errs.TRANSPORT, result.ErrorCode)
	assert.Equal(t, errs.TRANSPORT, errs.ClassifyError(errs.SpawnResult{ExitCode: *result.ExitCode,
		ErrorCode: result.ErrorCode, Stderr: *result.ErrorMessage}))
}
