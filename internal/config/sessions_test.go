package config

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionPolicyDefaultsAndExplicitSelection(t *testing.T) {
	for _, tc := range []struct{ raw, mode string }{
		{`{}`, "fresh"},
		{`{"sessions":{"mode":"fresh","policy-version":1}}`, "fresh"},
		{`{"sessions":{"mode":"scoped","policy-version":1,"_note":"documented","_note_source":false}}`, "scoped"},
	} {
		cfg, err := parseConfig([]byte(tc.raw), "test.json")
		require.NoError(t, err)
		assert.Equal(t, SessionsConfig{Mode: tc.mode, PolicyVersion: 1}, cfg.Sessions)
	}
	for _, file := range []string{"flywheel.config.example.json", "flywheel.profiles.example.json"} {
		raw, err := templates.FS.ReadFile(file)
		require.NoError(t, err)
		cfg, err := parseConfig(raw, file)
		require.NoError(t, err)
		assert.Equal(t, SessionsConfig{Mode: "scoped", PolicyVersion: 1}, cfg.Sessions)
		assert.True(t, cfg.Loops["plan"].ResidentSession)
		assert.True(t, cfg.Loops["plan"].ResidentSessionSet)
		var object map[string]any
		require.NoError(t, json.Unmarshal(raw, &object))
		assert.NoError(t, validateSessionPolicy(object["sessions"]), "editor schema accepts the shipped annotations too")
	}
}

func TestSessionPolicyRejectsAmbiguousOrFutureValues(t *testing.T) {
	for _, raw := range []string{
		`null`, `[]`, `false`, `"scoped"`, `{}`,
		`{"mode":"scoped"}`, `{"policy-version":1}`,
		`{"mode":null,"policy-version":1}`, `{"mode":"","policy-version":1}`,
		`{"mode":"Scoped","policy-version":1}`, `{"mode":"automatic","policy-version":1}`,
		`{"mode":"fresh","policy-version":null}`, `{"mode":"fresh","policy-version":0}`,
		`{"mode":"fresh","policy-version":2}`, `{"mode":"fresh","policy-version":1.5}`,
		`{"mode":"fresh","policy-version":"1"}`, `{"mode":"fresh","policy-version":true}`,
		`{"mode":"fresh","policy-version":1,"reset":true}`,
		`{"mode":"fresh","policy-version":1,"_notes":"typo"}`,
		`{"Mode":"fresh","policy-version":1}`,
	} {
		t.Run(raw, func(t *testing.T) {
			cfg, err := parseConfig([]byte(`{"sessions":`+raw+`}`), "invalid.json")
			require.ErrorContains(t, err, "sessions")
			assert.Nil(t, cfg)
		})
	}
	// Programmatic partial policies are not erased by Normalize either.
	for _, policy := range []SessionsConfig{{Mode: "scoped"}, {PolicyVersion: 1}, {Mode: "fresh", PolicyVersion: 2}} {
		cfg := Default()
		cfg.Sessions = policy
		cfg.Normalize()
		assert.ErrorContains(t, cfg.Validate(), "sessions")
		assert.Equal(t, policy, cfg.Sessions)
	}
}

func TestPlanResidentSessionPresenceSurvivesProfilesAndRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name, member string
		set, value   bool
	}{
		{"omitted", "", false, false},
		{"explicit opt out", `,"resident-session":false`, true, false},
		{"explicit opt in", `,"resident-session":true`, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"sessions":{"mode":"scoped","policy-version":1},"profiles":{"kimi":{"harness":"kimi","model":"m","channel":"c"}},"loops":{"plan":{"profile":"kimi"` + tc.member + `}},"judge-panel":{"profile":"kimi"` + tc.member + `}}`
			cfg, err := parseConfig([]byte(raw), "profile.json")
			require.NoError(t, err)
			for _, binding := range []LoopBinding{cfg.Loops["plan"], *cfg.JudgePanel} {
				assert.Equal(t, tc.set, binding.ResidentSessionSet)
				assert.Equal(t, tc.value, binding.ResidentSession)
				encoded, err := json.Marshal(binding)
				require.NoError(t, err)
				var fields map[string]any
				require.NoError(t, json.Unmarshal(encoded, &fields))
				_, present := fields["resident-session"]
				assert.Equal(t, tc.set, present)
			}
			encoded, err := json.Marshal(cfg)
			require.NoError(t, err)
			reloaded, err := parseConfig(encoded, "effective.json")
			require.NoError(t, err)
			assert.Equal(t, cfg.Loops, reloaded.Loops)
			assert.Equal(t, cfg.JudgePanel, reloaded.JudgePanel)
			assert.Equal(t, cfg.Sessions, reloaded.Sessions)
		})
	}
	// Legacy callers constructing a true binding in Go retain that choice.
	raw, err := json.Marshal(LoopBinding{ResidentSession: true})
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"resident-session":true`)
}

func TestSessionPolicySchemaFailuresDoNotSelectFresh(t *testing.T) {
	for _, tc := range []struct {
		name string
		load func(string) ([]byte, error)
	}{
		{"unreadable", func(string) ([]byte, error) { return nil, errors.New("read failed") }},
		{"invalid JSON", func(string) ([]byte, error) { return []byte(`{`), nil }},
		{"invalid schema", func(string) ([]byte, error) { return []byte(`{"type":"invalid-type"}`), nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema, err := compileSessionPolicySchema(tc.load)
			require.Error(t, err)
			assert.Nil(t, schema)
		})
	}
	original := sessionPolicySchema
	t.Cleanup(func() { sessionPolicySchema = original })
	sessionPolicySchema = func() (*jsonschema.Schema, error) { return nil, errors.New("schema unavailable") }
	for _, raw := range []string{`{}`, `{"sessions":{"mode":"scoped","policy-version":1}}`} {
		cfg, err := parseConfig([]byte(raw), "test.json")
		require.ErrorContains(t, err, "schema unavailable")
		assert.Nil(t, cfg)
	}
}

func TestScopedSessionAdaptersAreExplicit(t *testing.T) {
	for _, harness := range []string{"codex-pro", "dsh", "kimi"} {
		assert.True(t, SupportsScopedSessions(harness))
	}
	for _, harness := range []string{"", "codex", "pi", "omp", "zcode", "mock", "future"} {
		assert.False(t, SupportsScopedSessions(harness))
	}
}

func TestSubscriptionsMigrationPreservesSessionPolicyAndPlanOptOut(t *testing.T) {
	for _, mode := range []string{"", "fresh", "scoped"} {
		t.Run(mode, func(t *testing.T) {
			var original map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(legacyMigrationConfig), &original))
			if mode != "" {
				original["sessions"] = configJSON(SessionsConfig{Mode: mode, PolicyVersion: 1})
			}
			options, file := migrationFixture(t, string(configJSON(original)))
			options.Apply = true
			migration, err := Migrate(options)
			require.NoError(t, err)
			require.True(t, migration.Applied)
			cfg, err := LoadReadOnly("", options.Cwd)
			require.NoError(t, err)
			want := mode
			if want == "" {
				want = "fresh"
			}
			assert.Equal(t, want, cfg.Sessions.Mode)
			assert.False(t, cfg.Loops["plan"].ResidentSession)
			assert.True(t, cfg.Loops["plan"].ResidentSessionSet)
			var migrated map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(configBytesT(t, file), &migrated))
			if mode == "" {
				assert.NotContains(t, migrated, "sessions", "model migration must not opt an existing file into history reuse")
			} else {
				assert.JSONEq(t, string(original["sessions"]), string(migrated["sessions"]))
			}
		})
	}
}
