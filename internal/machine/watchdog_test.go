package machine

import (
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestWdogForSelectsRoleOverrideAndFallsBackToGlobal(t *testing.T) {
	cfg := config.Default()
	c := &Context{Config: &cfg}
	assert.Equal(t, int64(900_000), c.wdogFor("design-plan").IdleTimeoutMs)
	assert.Equal(t, int64(600_000), c.wdogFor("execute").IdleTimeoutMs)
	assert.Equal(t, int64(300_000), c.wdogFor("review").IdleTimeoutMs)
	assert.Equal(t, int64(300_000), c.wdogFor("").IdleTimeoutMs)

	cfg.Watchdog.Roles["execute"] = config.WatchdogRoleConfig{IdleTimeoutMs: 120_000, WallTimeoutMs: 600_000}
	assert.Equal(t, int64(120_000), c.wdogFor("execute").IdleTimeoutMs)
	assert.Equal(t, int64(600_000), c.wdogFor("execute").WallTimeoutMs)
}
