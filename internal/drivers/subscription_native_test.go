package drivers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionOnlyKimiPrintRefusesBeforeProcessOrSessionPreparation(t *testing.T) {
	policy := &config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"}
	result := RunKimi(KimiArgs{Subscription: policy, Command: "/must-not-run", Prompt: "task",
		SessionTitle: "must not prepare", RequestID: "subscription-print"})
	require.False(t, result.Settled)
	require.Equal(t, errs.INVALID_REQUEST, result.ErrorCode)
	require.Contains(t, *result.ErrorMessage, "native-profile")
	require.Equal(t, "not-dispatched", result.Session.Dispatch)
	require.True(t, result.Session.CleanupConfirmed)
}

func TestSubscriptionPolicyTravelsFromHarnessBindingToNativeWorkers(t *testing.T) {
	policy := &config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"}
	t.Run("codex-pro", func(t *testing.T) {
		args := codexProRunArgs(t, "record")
		t.Setenv(CodexProWorkerEnv, args.Command)
		result := InvokeHarness(CallArgs{Subscription: policy, Cwd: args.Cwd, Prompt: "record", Binding: config.LoopBinding{Harness: "codex-pro", Model: args.Model},
			PermissionMode: "read-only", APIKeys: args.APIKeys, Watchdog: args.Watchdog}, "")
		require.True(t, result.Settled, "%+v", result)
		raw, err := os.ReadFile(filepath.Join(args.Cwd, "worker-input.json"))
		require.NoError(t, err)
		var observed struct {
			Request codexProStart `json:"request"`
		}
		require.NoError(t, json.Unmarshal(raw, &observed))
		require.Equal(t, policy, observed.Request.Subscription)
	})
	t.Run("kimi", func(t *testing.T) {
		worker := kimiScript(t, "GORACE=atexit_sleep_ms=0 exec "+shQuote(os.Args[0])+" -test.run '^TestKimiNativeProcessFixture$'\n")
		t.Setenv(KimiNativeWorkerEnv, worker)
		cwd := t.TempDir()
		result := InvokeHarness(CallArgs{Subscription: policy, Cwd: cwd, Prompt: "record", Binding: config.LoopBinding{Harness: "kimi", Model: KimiDefaultModel},
			Native: json.RawMessage(`{"version":1,"mode":"agent","agents":{"enabled":false}}`), PermissionMode: "danger-full-access",
			APIKeys: map[string]string{"FLYWHEEL_TEST_KIMI_NATIVE": "1"}}, "")
		require.True(t, result.Settled, "%+v", result)
		raw, err := os.ReadFile(filepath.Join(cwd, "native-input.json"))
		require.NoError(t, err)
		var observed struct {
			Request kimiNativeStart `json:"request"`
		}
		require.NoError(t, json.Unmarshal(raw, &observed))
		require.Equal(t, policy, observed.Request.Subscription)
	})
}
