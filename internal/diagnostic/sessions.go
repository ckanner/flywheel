package diagnostic

import "github.com/ckanner/flywheel/internal/config"

// SessionPolicy reports configured selection and adapter support, not proof
// that any particular saved backend history exists or has been restored.
type SessionPolicy struct {
	Mode                  string `json:"mode"`
	Supported             bool   `json:"supported"`
	Reason                string `json:"reason"`
	HistoryRecoveryTested bool   `json:"historyRecoveryTested"`
}

func sessionPolicy(cfg *config.Config, role, harness string, binding config.LoopBinding) SessionPolicy {
	policy := SessionPolicy{Mode: "fresh", Supported: config.SupportsScopedSessions(harness), Reason: "fresh-policy"}
	if role == "intake" || role == "garden" {
		policy.Reason = "advisory-role-outside-scoped-policy"
		return policy
	}
	if cfg.Sessions.Mode == "fresh" {
		if role == "plan" && harness == "kimi" && binding.ResidentSession {
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
