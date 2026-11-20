package drivers

import (
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/errs"
	"github.com/stretchr/testify/require"
)

func TestDSHWebDefaultAndExplicitEffort(t *testing.T) {
	a := DSHArgs{Cwd: t.TempDir(), Provider: "chatgpt", Model: "gpt-6-pro-web", PermissionMode: "read-only"}
	for _, thinking := range []string{"", "pro"} {
		a.Thinking = thinking
		request, _, _, _, err := dshCall(a)
		require.NoError(t, err)
		require.Equal(t, "pro", request.ReasoningEffort)
	}
	a.Thinking = "high"
	_, _, _, _, err := dshCall(a)
	require.ErrorContains(t, err, "requires reasoning effort pro")
	a.Model, a.Thinking = "gpt-6.2-sol", ""
	request, _, _, _, err := dshCall(a)
	require.NoError(t, err)
	require.Equal(t, "high", request.ReasoningEffort, "SIWC keeps its previous default")
}

func TestDSHWebErrorsCannotClaimKnownTaskOutcome(t *testing.T) {
	request := scopedDSHRequest()
	request.Provider, request.Model, request.ReasoningEffort = "chatgpt", "gpt-6-pro-web", "pro"
	for _, code := range []errs.ErrorCode{errs.WEB_UNCERTAIN, errs.WEB_DELIVERY_CLAIMED, errs.WEB_CONNECTION_LOST, errs.WEB_PROTOCOL_ERROR, errs.SERVER} {
		for _, outcome := range []string{"unknown", "failed", "completed"} {
			ready, end := dshFixtureFrames(request)
			requestID := "turn-" + strings.Repeat("a", 64)
			end["status"], end["error"] = "error", map[string]any{"code": code, "message": "Inspect the original browser request", "requestId": requestID}
			life := end["lifecycle"].(map[string]any)
			life["outcome"], life["resumeCandidate"] = outcome, false
			parsed := parseDSHStream(dshJSONL(ready, end), request)
			if outcome == "unknown" {
				require.Equal(t, code, parsed.ErrorCode)
				require.Equal(t, requestID, parsed.ErrorRequestID)
				require.NotNil(t, parsed.Session)
				require.Equal(t, "dispatched", parsed.Session.Dispatch, "SDK dispatch is distinct from website Send")
				require.True(t, parsed.Session.CleanupConfirmed)
				require.Nil(t, parsed.Usage)
			} else {
				require.Equal(t, errs.TRANSPORT, parsed.ErrorCode)
				require.Nil(t, parsed.Session, "a contradictory receipt cannot authorize a new session")
				require.False(t, parsed.CleanupConfirmed)
			}
		}
	}
	for _, outcome := range []string{"unknown", "cancelled"} {
		ready, end := dshFixtureFrames(request)
		end["status"] = "cancelled"
		life := end["lifecycle"].(map[string]any)
		life["outcome"], life["resumeCandidate"] = outcome, false
		parsed := parseDSHStream(dshJSONL(ready, end), request)
		if outcome == "unknown" {
			require.True(t, parsed.Cancelled)
			require.True(t, parsed.CleanupConfirmed)
			require.Equal(t, "unknown", parsed.Session.Outcome)
		} else {
			require.Equal(t, errs.TRANSPORT, parsed.ErrorCode)
			require.Nil(t, parsed.Session)
		}
	}
	for _, provider := range []string{"chatgpt", "renamed-provider"} {
		ready, end := dshFixtureFrames(request)
		ready["web"].(map[string]any)["summary"] = map[string]string{"provider": provider, "model": "gpt-6-pro-web"}
		parsed := parseDSHStream(dshJSONL(ready, end), request)
		require.Contains(t, parsed.Error, "did not attest verified Web readiness")
		require.False(t, parsed.Settled)
		require.Nil(t, parsed.Session, "another Web provider name cannot attest a capped auxiliary route")
	}
	ready, end := dshFixtureFrames(request)
	delete(ready, "web")
	parsed := parseDSHStream(dshJSONL(ready, end), request)
	require.Contains(t, parsed.Error, "did not attest verified Web readiness")
	require.False(t, parsed.Settled)
	require.Nil(t, parsed.Session)
}

func TestDSHWebColdAndWarmWaitUseOnlyWallDeadline(t *testing.T) {
	a := dshRunArgs(t, "ok")
	a.Provider, a.Model, a.Thinking = "chatgpt", "gpt-6-pro-web", "pro"
	a.Watchdog.IdleTimeoutMs = 1
	out := RunDSH(a)
	require.True(t, out.Settled, "%+v", out)
	require.False(t, out.IdleTimedOut)
	warm := warmArgs(t, "ok")
	warm.Provider, warm.Model, warm.Thinking = a.Provider, a.Model, a.Thinking
	warm.Watchdog.IdleTimeoutMs = 1
	h := PrepareDSH(warm)
	defer h.Close()
	out = h.Run(warm)
	require.True(t, out.Settled, "%+v", out)
	require.False(t, out.IdleTimedOut)
	expired := warmArgs(t, "expired")
	expired.Provider, expired.Model, expired.Thinking = a.Provider, a.Model, a.Thinking
	expired.Watchdog.IdleTimeoutMs = 1
	old := PrepareDSH(expired)
	defer old.Close()
	<-old.done
	out = old.Run(expired)
	require.True(t, out.Settled, "%+v", out)
	require.False(t, out.IdleTimedOut, "safe cold fallback preserves wall-only waiting")
	depleted := warmArgs(t, "expired")
	depleted.Provider, depleted.Model, depleted.Thinking = a.Provider, a.Model, a.Thinking
	ended := PrepareDSH(depleted)
	defer ended.Close()
	<-ended.done
	out = ended.coldAfterExpiry(depleted, time.Now().Add(-time.Hour), Result{})
	require.True(t, out.WallTimedOut)
	require.False(t, out.IdleTimedOut, "an exhausted wall budget does not introduce an idle timeout")
	require.NoFileExists(t, depleted.Cwd+"/cold-model-call")
}

func TestDSHWebWallDeadlineStillCancelsSilentCalls(t *testing.T) {
	a := dshRunArgs(t, "hang")
	a.Provider, a.Model, a.Thinking = "chatgpt", "gpt-6-pro-web", "pro"
	a.Watchdog.IdleTimeoutMs, a.Watchdog.WallTimeoutMs = 1, 100
	out := RunDSH(a)
	require.True(t, out.WallTimedOut, "%+v", out)
	require.False(t, out.IdleTimedOut)
	require.False(t, out.Settled)
	require.Equal(t, errs.TIMEOUT, out.ErrorCode)
}
