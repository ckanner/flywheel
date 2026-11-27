package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchdogDefaultsKeepPlanningObservedLonger(t *testing.T) {
	d := Default()
	assert.Equal(t, int64(300_000), d.Watchdog.IdleTimeoutMs)
	assert.Equal(t, int64(900_000), d.Watchdog.Roles["design-plan"].IdleTimeoutMs)
	assert.Equal(t, int64(900_000), d.Watchdog.Roles["plan"].IdleTimeoutMs)
	assert.Equal(t, int64(600_000), d.Watchdog.Roles["execute"].IdleTimeoutMs)
	_, reviewOverride := d.Watchdog.Roles["review"]
	assert.False(t, reviewOverride)
}

func TestWatchdogRolesFillDefaultsPerRole(t *testing.T) {
	c := Default()
	c.Watchdog.Roles = nil
	c.Normalize()
	assert.Equal(t, int64(900_000), c.Watchdog.Roles["design-plan"].IdleTimeoutMs)
	assert.Equal(t, int64(900_000), c.Watchdog.Roles["plan-review"].IdleTimeoutMs)

	c.Watchdog.Roles = map[string]WatchdogRoleConfig{"design-plan": {WallTimeoutMs: 1_000}}
	c.Normalize()
	assert.Equal(t, int64(900_000), c.Watchdog.Roles["design-plan"].IdleTimeoutMs, "a partial planning override inherits the role default idle")
	assert.Equal(t, int64(1_000), c.Watchdog.Roles["design-plan"].WallTimeoutMs)

	c.Watchdog.Roles = map[string]WatchdogRoleConfig{"judge": {IdleTimeoutMs: 400_000}}
	c.Normalize()
	assert.Equal(t, int64(400_000), c.Watchdog.Roles["judge"].IdleTimeoutMs, "a role outside the planning defaults keeps its explicit override")
	assert.Equal(t, int64(900_000), c.Watchdog.Roles["design-plan"].IdleTimeoutMs)
	require.NoError(t, c.Validate())
}

func TestWatchdogRoleOverridesNormalizeAndValidate(t *testing.T) {
	c := Default()
	c.Watchdog.Roles = map[string]WatchdogRoleConfig{
		"design-plan": {IdleTimeoutMs: 1_200_000},
		"execute":     {WallTimeoutMs: 7_200_000},
	}
	c.Normalize()
	require.NoError(t, c.Validate())
	assert.Equal(t, int64(1_200_000), c.Watchdog.Roles["design-plan"].IdleTimeoutMs)
	assert.Equal(t, int64(7_200_000), c.Watchdog.Roles["execute"].WallTimeoutMs)
	assert.Equal(t, int64(900_000), c.Watchdog.Roles["plan"].IdleTimeoutMs, "missing default planning roles are filled")

	c.Watchdog.Roles["unknown"] = WatchdogRoleConfig{IdleTimeoutMs: 1_000}
	require.ErrorContains(t, c.Validate(), "unknown role")
	delete(c.Watchdog.Roles, "unknown")
	c.Watchdog.Roles["execute"] = WatchdogRoleConfig{}
	require.ErrorContains(t, c.Validate(), "requires an idle or wall")

	for name, mutate := range map[string]func(*Config){
		"global idle too small":      func(c *Config) { c.Watchdog.IdleTimeoutMs = 0 },
		"global wall too large":      func(c *Config) { c.Watchdog.WallTimeoutMs = 86_400_001 },
		"role idle too large":        func(c *Config) { c.Watchdog.Roles["execute"] = WatchdogRoleConfig{IdleTimeoutMs: 86_400_001} },
		"role wall too small":        func(c *Config) { c.Watchdog.Roles["execute"] = WatchdogRoleConfig{WallTimeoutMs: 0} },
		"role wall too large":        func(c *Config) { c.Watchdog.Roles["execute"] = WatchdogRoleConfig{WallTimeoutMs: 86_400_001} },
		"prepare-cmd blank":          func(c *Config) { c.Verify.PrepareCmd = "   " },
		"prepare-cmd contains NUL":   func(c *Config) { c.Verify.PrepareCmd = "npm\x00ci" },
		"prepare-cmd exceeds 1000 B": func(c *Config) { c.Verify.PrepareCmd = string(make([]byte, 1001)) },
	} {
		t.Run(name, func(t *testing.T) {
			c := Default()
			mutate(&c)
			require.Error(t, c.Validate())
		})
	}
	okCfg := Default()
	okCfg.Verify.PrepareCmd = "npm ci"
	require.NoError(t, okCfg.Validate())
}
