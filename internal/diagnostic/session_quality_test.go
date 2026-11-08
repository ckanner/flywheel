package diagnostic

import (
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestSessionInspectionUsesRoleOverridesAndRetainsPlanOptOut(t *testing.T) {
	cfg := config.Default()
	cfg.Sessions = config.SessionsConfig{Mode: "fresh", PolicyVersion: 1,
		Roles: map[string]string{"execute": "scoped", "plan": "fresh", "judge": "scoped"}}
	assert.Equal(t, "scoped", sessionPolicy(&cfg, "execute", "codex-pro", config.LoopBinding{}).Mode)
	assert.Equal(t, "fresh", sessionPolicy(&cfg, "review", "codex-pro", config.LoopBinding{}).Mode)
	assert.Equal(t, "fresh", sessionPolicy(&cfg, "plan", "kimi", config.LoopBinding{ResidentSession: true}).Mode)
	cfg.Sessions.Roles["plan"] = "scoped"
	p := sessionPolicy(&cfg, "plan", "kimi", config.LoopBinding{ResidentSessionSet: true, ResidentSession: false})
	assert.Equal(t, "fresh", p.Mode)
	assert.Equal(t, "plan-resident-session-disabled", p.Reason)
	assert.Equal(t, "fresh", sessionPolicy(&cfg, "intake", "dsh", config.LoopBinding{}).Mode)
	assert.False(t, sessionPolicy(&cfg, "judge", "codex-pro", config.LoopBinding{}).HistoryRecoveryTested)
	for _, harness := range []string{"codex-pro", "claude-code-pro", "dsh", "kimi"} {
		p := sessionPolicy(&cfg, "execute", harness, config.LoopBinding{})
		assert.Equal(t, harness == "codex-pro" || harness == "claude-code-pro", p.DurableJournal)
		assert.Equal(t, p.DurableJournal, p.ReadOnlyRecovery)
		assert.Equal(t, harness == "codex-pro", p.ThreadReadyBeforeDispatch)
		assert.False(t, p.HistoryRecoveryTested)
	}
}
