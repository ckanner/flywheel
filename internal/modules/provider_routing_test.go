package modules

import (
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type moduleProviderDriver struct {
	drivers.Driver
	calls []drivers.CallArgs
}

func (d *moduleProviderDriver) call(a drivers.CallArgs) drivers.Result {
	d.calls = append(d.calls, a)
	return drivers.MockResult(func(r *drivers.Result) {
		r.Harness, r.Provider = a.Binding.Harness, a.Binding.Provider
		if a.Binding.Harness == "codex" {
			r.Provider = "openai"
		}
		if a.Binding.Channel != "codex-account" {
			message := "429 rate limit"
			r.ErrorMessage, r.Settled = &message, false
		}
	})
}

func TestModuleFallbackInheritsMostRecentHarness(t *testing.T) {
	taskDir := t.TempDir()
	cfg := testConfigT(t, taskDir)
	cfg.Channels["primary-account"] = config.Channel{Harness: "dsh", Fallback: "backup-account"}
	cfg.Channels["backup-account"] = config.Channel{Harness: "codex", Fallback: "codex-account", Models: map[string]string{"intake": "second-model"}}
	cfg.Channels["codex-account"] = config.Channel{Models: map[string]string{"intake": "third-model"}}
	driver := &moduleProviderDriver{}
	m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: &drivers.Drivers{Intake: driver}, Sleep: func(int64) {}})
	r := CallAgent(m, CallOptions{Role: "intake", Prompt: "p", Binding: fallback.Binding{Harness: "dsh", Provider: "chatgpt", Channel: "primary-account", Model: "first-model"}})
	require.True(t, r.OK)
	require.Len(t, driver.calls, 3)
	for _, call := range driver.calls[1:] {
		assert.Equal(t, "codex", call.Binding.Harness, "an omitted later harness preserves the already selected backend")
		assert.Empty(t, call.Binding.Provider, "a DSH provider cannot leak across a harness change")
	}
}

func (d *moduleProviderDriver) Intake(a drivers.CallArgs) drivers.Result { return d.call(a) }
func (d *moduleProviderDriver) Garden(a drivers.CallArgs) drivers.Result { return d.call(a) }

func TestAdviceModulesRecomputeProvidersAcrossAccountFallback(t *testing.T) {
	for _, role := range []string{"intake", "garden"} {
		t.Run(role, func(t *testing.T) {
			taskDir := t.TempDir()
			cfg := testConfigT(t, taskDir)
			cfg.Channels["primary-account"] = config.Channel{Harness: "dsh", Fallback: "backup-account"}
			cfg.Channels["backup-account"] = config.Channel{Harness: "dsh", Provider: "backup-adapter", Fallback: "codex-account", Models: map[string]string{role: "backup-model"}}
			cfg.Channels["codex-account"] = config.Channel{Harness: "codex", Models: map[string]string{role: "codex-model"}}
			driver := &moduleProviderDriver{}
			m := NewContext(CtxOpts{TaskDir: taskDir, Config: cfg, Drivers: &drivers.Drivers{Intake: driver, Garden: driver}, Sleep: func(int64) {}})
			result := CallAgent(m, CallOptions{Role: role, Prompt: "advice only", Binding: fallback.Binding{Harness: "dsh", Provider: "explicit-adapter", Channel: "primary-account", Model: "primary-model", Thinking: "high"}})
			require.True(t, result.OK, "%+v", result)
			assert.Equal(t, "codex-account", result.Channel)
			require.Len(t, driver.calls, 3)
			entries := readMetricsLinesT(t, m.MetricsFile)
			require.Len(t, entries, 3)
			for i, expected := range []struct{ channel, provider, reported string }{
				{"primary-account", "explicit-adapter", "explicit-adapter"},
				{"backup-account", "backup-adapter", "backup-adapter"},
				{"codex-account", "", "openai"},
			} {
				assert.Equal(t, expected.channel, driver.calls[i].Binding.Channel)
				assert.Equal(t, expected.provider, driver.calls[i].Binding.Provider)
				assert.Equal(t, "read-only", driver.calls[i].PermissionMode)
				assert.Empty(t, driver.calls[i].SessionID)
				assert.Equal(t, expected.channel, entries[i]["channel"])
				assert.Equal(t, expected.reported, entries[i]["provider"])
			}
		})
	}
}
