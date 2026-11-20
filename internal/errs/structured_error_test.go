package errs

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStructuredCodesAreClosedAndIndependentOfRecoveryText(t *testing.T) {
	for _, code := range AllCodes {
		assert.True(t, IsKnownCode(code))
		assert.Equal(t, code, ClassifyError(SpawnResult{ErrorCode: code, ExitCode: 1, Stderr: "401 quota exceeded; malformed response"}))
	}
	for _, code := range []ErrorCode{"", "auth", "AUTH ", "AUTH: sign in", "ACCOUNT_TOKEN_INVALID", "FUTURE_CODE"} {
		assert.False(t, IsKnownCode(code))
		if code != "" {
			assert.Equal(t, TRANSPORT, ClassifyError(SpawnResult{ErrorCode: code, ExitCode: 1, Stderr: "401 unauthorized"}))
		}
	}
	assert.Equal(t, TRANSPORT, ClassifyError(SpawnResult{ExitCode: 1, Stderr: "AUTH: 请重新登录"}), "text prefixes do not become a new wire protocol")
}

func TestWebTransportAndDeliveryNeedReconciliation(t *testing.T) {
	for _, code := range []ErrorCode{WEB_UNCERTAIN, WEB_DELIVERY_CLAIMED, WEB_CONNECTION_LOST, WEB_PROTOCOL_ERROR} {
		assert.True(t, RequiresReconciliation(code))
		assert.False(t, IsRetryable(code))
		assert.False(t, ShouldFallback(code))
	}
	assert.False(t, RequiresReconciliation(SERVER))
	assert.False(t, RequiresReconciliation("WEB_SETUP_REQUIRED"), "unknown names do not expand the wire contract")
}

func TestPhysicalFailuresOverrideStructuredCodesWithoutChangingLegacyText(t *testing.T) {
	assert.Equal(t, TIMEOUT, ClassifyError(SpawnResult{ErrorCode: AUTH, TimedOut: true, SpawnError: errors.New("spawn"), Signal: "SIGKILL"}))
	assert.Equal(t, TRANSPORT, ClassifyError(SpawnResult{ErrorCode: QUOTA, SpawnError: errors.New("cleanup unconfirmed")}))
	assert.Equal(t, TRANSPORT, ClassifyError(SpawnResult{ErrorCode: AUTH, Signal: "SIGTERM", Stderr: "401 unauthorized"}))
	assert.Equal(t, AUTH, ClassifyError(SpawnResult{Signal: "SIGTERM", Stderr: "401 unauthorized"}), "legacy text drivers retain their previous classification order")
	for _, exit := range []int{0, 42} {
		assert.Equal(t, TRANSPORT, ClassifyError(SpawnResult{ErrorCode: QUOTA, ExitCode: exit, Stderr: "quota exceeded"}))
	}
}
