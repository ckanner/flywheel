package modules

import (
	"encoding/json"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/stretchr/testify/require"
)

func TestModuleNativeProfilesUseSameAdmissionForIntakeAndGarden(t *testing.T) {
	for _, role := range []string{"intake", "garden"} {
		cfg := testConfigT(t, t.TempDir())
		cfg.NativeProfiles = map[string]config.NativeProfile{"native": {Harness: "codex", Options: json.RawMessage(`{"version":1}`)}}
		driver := &moduleProviderDriver{}
		m := NewContext(CtxOpts{TaskDir: t.TempDir(), Config: cfg, Drivers: &drivers.Drivers{Intake: driver, Garden: driver}})
		binding := fallback.Binding{Harness: "codex", Channel: "codex-account", Model: "chosen", NativeProfile: "native"}
		r := CallAgent(m, CallOptions{Role: role, Prompt: "p", Binding: binding})
		require.True(t, r.OK)
		require.Len(t, driver.calls, 1)
		a := driver.calls[0]
		require.JSONEq(t, `{"version":1}`, string(a.Native))
		require.NotEmpty(t, a.RequestID)
		require.Equal(t, a.RequestID, a.Managed.InvocationID)
		require.Equal(t, role, a.Managed.Role)
		require.Equal(t, m.TaskDir, a.Managed.TaskDir)
		binding.NativeProfile = "missing"
		r = CallAgent(m, CallOptions{Role: role, Prompt: "p", Binding: binding})
		require.False(t, r.OK)
		require.Len(t, driver.calls, 1)
		require.ErrorContains(t, m.fatalErr, "native profile")
	}
}
