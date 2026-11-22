package investigation

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestConfiguredJEVKeyNamesFreezeAndReloadUnchanged(t *testing.T) {
	for name, keyEnv := range map[string]string{
		"uppercase":      "MY_JEV_KEY",
		"lowercase":      "my_jev_key",
		"mixed-case":     "my_Jev_Key9",
		"underscore":     "_jev_key",
		"maximum-length": "a" + strings.Repeat("B", 119),
	} {
		t.Run(name, func(t *testing.T) {
			dir, cwd, p := preparationFixture(t)
			changeRuntimeConfig(t, dir, func(values map[string]any) {
				delete(values, "subscription")
				values["jev"] = map[string]any{"enabled": true, "endpoint": "https://example.org/v1/systemone", "key-env": keyEnv}
			})
			cfg, err := config.LoadReadOnly(dir, cwd)
			require.NoError(t, err)
			require.True(t, cfg.JEV.Enabled)
			require.Equal(t, keyEnv, cfg.JEV.KeyEnv)
			require.Nil(t, p.Decision, "Start must obtain the credential name from the effective configuration")
			raw, err := json.Marshal(p)
			require.NoError(t, err)
			started, err := Start(dir, cwd, raw, Options{})
			require.NoError(t, err)
			require.Equal(t, int64(1), started.Revision)
			require.Equal(t, "enabled", started.JEVStatus)
			require.Equal(t, keyEnv, started.Contract.Decision.KeyEnv)
			loaded, err := state.LoadPreparation(dir)
			require.NoError(t, err)
			require.Equal(t, started, loaded)
			saved, err := state.SavePreparation(dir, loaded)
			require.NoError(t, err)
			require.Equal(t, int64(2), saved.Revision)
			require.Equal(t, started.ContractHash, saved.ContractHash)
			loaded, err = state.LoadPreparation(dir)
			require.NoError(t, err)
			require.Equal(t, saved, loaded)
			require.Equal(t, keyEnv, loaded.Contract.Decision.KeyEnv, "freezing and recovery must preserve exact credential-name casing")
		})
	}
}

func TestConfiguredJEVKeyNamesKeepThe120CharacterLimit(t *testing.T) {
	for name, keyEnv := range map[string]string{
		"121-characters": strings.Repeat("K", 121),
		"128-characters": strings.Repeat("K", 128),
	} {
		t.Run(name, func(t *testing.T) {
			dir, cwd, p := preparationFixture(t)
			changeRuntimeConfig(t, dir, func(values map[string]any) {
				delete(values, "subscription")
				values["jev"] = map[string]any{"enabled": true, "endpoint": "https://example.org/v1/systemone", "key-env": keyEnv}
			})
			_, err := config.LoadReadOnly(dir, cwd)
			require.ErrorContains(t, err, "key-env")
			raw, err := json.Marshal(p)
			require.NoError(t, err)
			_, err = Start(dir, cwd, raw, Options{})
			require.ErrorContains(t, err, "effective configuration is invalid")
			_, err = state.LoadPreparation(dir)
			require.ErrorIs(t, err, os.ErrNotExist, "invalid configuration must not create a preparation")
		})
	}
}
