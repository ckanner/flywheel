package state

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNativeSessionAndRunConfigurationSurviveStateReload(t *testing.T) {
	dir := t.TempDir()
	st := sessionTestState(t, dir)
	configuration := SessionConfiguration{Harness: "kimi", Model: "kimi-test", Channel: "subscription", Thinking: "high",
		Permission: "danger-full-access", SessionMode: "scoped", NativeProfile: "swarm",
		NativeOptions: json.RawMessage(`{"version":1,"mode":"swarm","agents":{"enabled":true}}`)}
	st.Sessions.Records["execute/M01-T01"].Configuration = &configuration
	st.Run = &RunIdentity{ID: "native-run", Kind: "run", Conditions: strings.Repeat("f", 64), StartedAt: "2026-11-14T00:00:00Z",
		Bindings: map[string]SessionConfiguration{"routes.default": configuration}}
	_, err := Save(dir, st, nil)
	require.NoError(t, err)
	loaded, err := Load(dir)
	require.NoError(t, err, "native metadata must pass the same strict schema used after process restart")
	actual := loaded.Sessions.Records["execute/M01-T01"].Configuration
	require.Equal(t, "swarm", actual.NativeProfile)
	require.JSONEq(t, string(configuration.NativeOptions), string(actual.NativeOptions))
	require.Equal(t, "swarm", loaded.Run.Bindings["routes.default"].NativeProfile)
	require.JSONEq(t, string(configuration.NativeOptions), string(loaded.Run.Bindings["routes.default"].NativeOptions))
	for _, invalid := range []json.RawMessage{json.RawMessage(`{"version":2}`), json.RawMessage(`{"mode":"swarm"}`), json.RawMessage(`[]`)} {
		st.Sessions.Records["execute/M01-T01"].Configuration.NativeOptions = invalid
		raw, err := json.Marshal(st)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(StateFile(dir), raw, 0600))
		_, err = Load(dir)
		require.Error(t, err, "unversioned or future native state cannot silently become current settings")
	}
}
