package config

import (
	"errors"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestJEVOptionalDefaultsAndStrictConfiguration(t *testing.T) {
	for _, raw := range []string{`{}`, `{"jev-shadow":{}}`, `{"jev-shadow":{"enabled":false,"endpoint":"unavailable","_note":"off"}}`} {
		cfg, err := parseConfig([]byte(raw), "test")
		require.NoError(t, err)
		require.False(t, cfg.JEVShadow.Enabled)
		require.Equal(t, int64(5000), cfg.JEVShadow.TimeoutMS)
		require.Equal(t, "MEROUTER_API_KEY", cfg.JEVShadow.KeyEnv)
	}
	cfg, err := parseConfig([]byte(`{"jev-shadow":{"enabled":true,"endpoint":"https://example.org/v1/systemone","model":"jev","expected-model":"jev-1.14.0","key-env":"TEST_KEY","timeout-ms":50}}`), "test")
	require.NoError(t, err)
	require.True(t, cfg.JEVShadow.Enabled)
	require.Equal(t, int64(50), cfg.JEVShadow.TimeoutMS)
	require.Contains(t, editorValues(cfg), "jev-shadow")
	for _, raw := range []string{`null`, `false`, `{"enabled":"false"}`, `{"api-key":"secret"}`, `{"timeout-ms":0}`, `{"timeout-ms":5001}`, `{"model":""}`, `{"key-env":"a=b"}`} {
		_, err := parseConfig([]byte(`{"jev-shadow":`+raw+`}`), "test")
		require.Error(t, err)
	}
	cfg.JEVShadow.TimeoutMS = 5001
	require.Error(t, cfg.Validate())
	old := jevSchema
	t.Cleanup(func() { jevSchema = old })
	jevSchema = func() (*jsonschema.Schema, error) { return nil, errors.New("schema missing") }
	require.Error(t, validateJEV(nil))
}
