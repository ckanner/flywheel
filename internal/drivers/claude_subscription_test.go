package drivers

import (
	"github.com/ckanner/flywheel/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func subscriptionClaudeReceipt(r codexProStart) map[string]any {
	m := claudeCodeProFixture(r)
	m["subscription"] = map[string]any{"version": 1, "mode": "subscription-only", "status": "verified", "source": "sdk-usage-0.3.326", "checkedAt": "2026-11-20T00:00:00Z", "apiProvider": "firstParty", "subscriptionType": "pro", "extraUsageEnabled": false, "resolvedModel": r.Model, "scope": "pre-prompt-snapshot"}
	proObject(m, "configuration")["modelEvidence"] = map[string]any{"version": 1, "rootMatched": true, "rootModels": []string{r.Model}, "source": "assistant.message.model"}
	return m
}
func TestClaudeSubscriptionRequiresNativeBillingAndRootModelEvidence(t *testing.T) {
	r := claudeCodeProRequest(t)
	r.Model = "claude-fable-5-2"
	r.Subscription = &config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"}
	p := parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(r, subscriptionClaudeReceipt(r))), r)
	require.True(t, p.Settled, "%+v", p)
	for _, change := range []func(map[string]any){
		func(m map[string]any) { delete(m, "subscription") },
		func(m map[string]any) { proObject(m, "subscription")["extraUsageEnabled"] = true },
		func(m map[string]any) { proObject(m, "subscription")["checkedAt"] = nil },
		func(m map[string]any) { proObject(m, "subscription")["apiProvider"] = "gateway" },
		func(m map[string]any) {
			proObject(m, "configuration", "modelEvidence")["rootModels"] = []string{"claude-opus-5-5"}
		},
	} {
		m := subscriptionClaudeReceipt(r)
		change(m)
		p = parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(r, m)), r)
		require.False(t, p.Settled)
		assert.Nil(t, p.Usage)
		assert.Equal(t, "unknown", p.Session.Outcome)
	}
	// Initialization and usage lookup can run a physical process while positively
	// proving that the user prompt never reached a model. Preserve this distinction.
	m := subscriptionClaudeReceipt(r)
	m["ok"] = false
	m["output"] = []any{}
	m["usage"] = claudeCodeProUnknownUsage()
	m["failure"] = map[string]any{"category": "subscription", "stage": "configuration", "detail": "extra usage is enabled"}
	e := proObject(m, "execution")
	e["turnDispatch"] = "not-dispatched"
	e["outcome"] = "failed"
	e["terminalReceived"] = false
	e["exitCode"] = 1
	p = parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(r, m)), r)
	assert.Equal(t, "not-dispatched", p.Session.Dispatch)
	assert.True(t, p.CleanupConfirmed)
	assert.Equal(t, &Usage{}, p.Usage)
}
