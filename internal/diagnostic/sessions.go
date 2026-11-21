package diagnostic

import (
	"encoding/json"

	"github.com/ckanner/flywheel/internal/config"
)

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
		DurableJournal: harness == "codex-pro" || harness == "claude-code-pro" || harness == "antigravity", ReadOnlyRecovery: harness == "codex-pro" || harness == "claude-code-pro" || harness == "antigravity",
		ThreadReadyBeforeDispatch: harness == "codex-pro"}
	nativeKimi := harness == "kimi" && binding.NativeProfile != ""
	if nativeKimi {
		profile, found := cfg.NativeProfiles[binding.NativeProfile]
		var native struct {
			Version int    `json:"version"`
			Mode    string `json:"mode"`
		}
		if err := json.Unmarshal(profile.Options, &native); !found || profile.Harness != "kimi" || err != nil || native.Version != 1 || (native.Mode != "agent" && native.Mode != "swarm" && native.Mode != "tower") {
			policy.Supported, policy.Reason = false, "invalid-native-profile"
			return policy
		}
		if native.Mode == "tower" {
			policy.Supported, policy.Reason = false, "native-tower-fresh-only"
			return policy
		}
	}
	if role == "intake" || role == "garden" {
		policy.Reason = "advisory-role-outside-scoped-policy"
		return policy
	}
	if cfg.Sessions.ModeForRole(role) == "fresh" {
		if role == "plan" && harness == "kimi" && binding.ResidentSession && cfg.Sessions.Roles["plan"] != "fresh" {
			if nativeKimi {
				policy.Mode, policy.Reason = "scoped", "native-kimi-plan-resident"
			} else {
				policy.Mode, policy.Reason = "legacy-plan", "explicit-kimi-plan-history"
			}
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
