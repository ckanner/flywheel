package diagnostic

import "github.com/ckanner/flywheel/internal/config"

// SessionPolicy reports configured selection and adapter support, not proof
// that any particular saved backend history exists or has been restored.
type SessionPolicy struct {
	Mode                  string `json:"mode"`
	Supported             bool   `json:"supported"`
	Reason                string `json:"reason"`
	HistoryRecoveryTested bool   `json:"historyRecoveryTested"`
	// Static pinned public API capabilities, separate from account/history
	// evidence. Claude cannot attest initialized configuration before prompt
	// dispatch, so its durable admission is not called thread-ready.
	DurableJournal            bool `json:"durableJournal"`
	ReadOnlyRecovery          bool `json:"readOnlyRecovery"`
	ThreadReadyBeforeDispatch bool `json:"threadReadyBeforeDispatch"`
}

func sessionPolicy(cfg *config.Config, role, harness string, binding config.LoopBinding) SessionPolicy {
	policy := SessionPolicy{Mode: "fresh", Supported: config.SupportsScopedSessions(harness), Reason: "fresh-policy",
		DurableJournal: harness == "codex-pro" || harness == "claude-code-pro", ReadOnlyRecovery: harness == "codex-pro" || harness == "claude-code-pro",
		ThreadReadyBeforeDispatch: harness == "codex-pro"}
	if role == "intake" || role == "garden" {
		policy.Reason = "advisory-role-outside-scoped-policy"
		return policy
	}
	if cfg.Sessions.ModeForRole(role) == "fresh" {
		if role == "plan" && harness == "kimi" && binding.ResidentSession && cfg.Sessions.Roles["plan"] != "fresh" {
			policy.Mode, policy.Reason = "legacy-plan", "explicit-kimi-plan-history"
		}
		return policy
	}
	if !policy.Supported {
		policy.Reason = "unsupported-adapter"
		return policy
	}
	if role == "plan" && binding.ResidentSessionSet && !binding.ResidentSession {
		policy.Reason = "plan-resident-session-disabled"
		return policy
	}
	policy.Mode, policy.Reason = "scoped", "normal-terminal-checkpoint-required"
	return policy
}
