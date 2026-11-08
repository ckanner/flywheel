package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionRolesAndMeasuredQualityPolicyRoundTrip(t *testing.T) {
	raw := []byte(`{"sessions":{"mode":"fresh","policy-version":1,"roles":{"plan":"fresh","execute":"scoped","review":"scoped","judge":"fresh","judge-panel":"fresh"},"quality":{"max-confirmed-turns":8,"max-unproductive-turns":2,"max-run-input-tokens":120000}}}`)
	cfg, err := parseConfig(raw, "quality.json")
	require.NoError(t, err)
	assert.Equal(t, "scoped", cfg.Sessions.ModeForRole("execute"))
	assert.Equal(t, "fresh", cfg.Sessions.ModeForRole("judge"))
	assert.Equal(t, "fresh", cfg.Sessions.ModeForRole("unconfigured"))
	assert.Equal(t, SessionQualityPolicy{MaxConfirmedTurns: 8, MaxUnproductiveTurns: 2, MaxRunInputTokens: 120000}, cfg.Sessions.Quality)
	encoded, err := json.Marshal(cfg)
	require.NoError(t, err)
	reloaded, err := parseConfig(encoded, "effective.json")
	require.NoError(t, err)
	assert.Equal(t, cfg.Sessions, reloaded.Sessions)

	// Enabling only a role override does not enable a tuning threshold.
	baseline, err := parseConfig([]byte(`{"sessions":{"mode":"scoped","policy-version":1,"roles":{"review":"fresh"},"quality":{}}}`), "baseline.json")
	require.NoError(t, err)
	assert.Equal(t, SessionQualityPolicy{}, baseline.Sessions.Quality)
	assert.Equal(t, "scoped", baseline.Sessions.ModeForRole("execute"))
	assert.Equal(t, "fresh", baseline.Sessions.ModeForRole("review"))
}

func TestSessionQualityRejectsInvalidFilesAndProgrammaticPolicies(t *testing.T) {
	for _, member := range []string{
		`"roles":null`, `"roles":[]`, `"roles":{"execute":null}`, `"roles":{"execute":"automatic"}`,
		`"roles":{"intake":"scoped"}`, `"roles":{"executer":"scoped"}`,
		`"quality":null`, `"quality":true`, `"quality":{"max-context-tokens":100}`,
		`"quality":{"max-confirmed-turns":-1}`, `"quality":{"max-confirmed-turns":1000001}`,
		`"quality":{"max-unproductive-turns":1.5}`, `"quality":{"max-run-input-tokens":"100"}`,
		`"quality":{"max-run-input-tokens":9007199254740992}`,
	} {
		t.Run(member, func(t *testing.T) {
			_, err := parseConfig([]byte(`{"sessions":{"mode":"scoped","policy-version":1,`+member+`}}`), "invalid.json")
			require.ErrorContains(t, err, "sessions")
		})
	}
	for _, policy := range []SessionsConfig{
		{Mode: "scoped", PolicyVersion: 1, Roles: map[string]string{"execute": "invalid"}},
		{Mode: "scoped", PolicyVersion: 1, Quality: SessionQualityPolicy{MaxConfirmedTurns: -1}},
		{Roles: map[string]string{}},
		{Quality: SessionQualityPolicy{MaxUnproductiveTurns: 2}},
	} {
		cfg := Default()
		cfg.Sessions = policy
		cfg.Normalize()
		assert.Equal(t, policy, cfg.Sessions, "partial in-process policies must not silently normalize to fresh")
		require.ErrorContains(t, cfg.Validate(), "sessions")
	}
}
