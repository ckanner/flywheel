package drivers

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Called by both existing opt-in bridge gates. The actual installed public API,
// journal and supervisor are exercised; only the native model service is fake.
func testMachineLifecycleNodeBridge(t *testing.T, harness string) {
	for _, scenario := range []string{"complete", "reject-intent", "reject-possible", "missing-cli"} {
		t.Run("durable-lifecycle/"+scenario, func(t *testing.T) {
			worker, err := filepath.Abs("../../plugins/dsh/dist/" + harness + "-worker.js")
			require.NoError(t, err)
			fixture, err := filepath.Abs("../../plugins/dsh/test/fixtures/fake-" + harness + ".js")
			require.NoError(t, err)
			cwd, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			log := filepath.Join(cwd, "native.jsonl")
			a := CallArgs{Cwd: cwd, Prompt: "one durable invocation", Model: "gpt-test", Binding: config.LoopBinding{Harness: harness, Thinking: "high"},
				RequestID: "call-p3-" + scenario, PersistentSession: true, PermissionMode: "danger-full-access", RecoveryDirectory: filepath.Join(cwd, "recovery"),
				Watchdog: watchdog.Opts{IdleTimeoutMs: 5000, WallTimeoutMs: 15000}, APIKeys: map[string]string{"TEST_CODEX_PRO_LOG": log}}
			if harness == "claude-code-pro" {
				source, readErr := os.ReadFile(fixture)
				require.NoError(t, readErr)
				fixture = filepath.Join(t.TempDir(), "fake-claude.mjs")
				require.NoError(t, os.WriteFile(fixture, source, 0o755))
				a.Model, a.APIKeys = "sonnet", nil
				log = filepath.Join(cwd, "claude-fixture.jsonl")
				t.Setenv(ClaudeCodeProWorkerEnv, worker)
				t.Setenv(ClaudeCodeProBinaryEnv, fixture)
				if scenario == "missing-cli" {
					t.Setenv(ClaudeCodeProBinaryEnv, filepath.Join(cwd, "missing-cli"))
				}
			} else {
				t.Setenv(CodexProWorkerEnv, worker)
				t.Setenv(CodexProBinaryEnv, fixture)
				if scenario == "missing-cli" {
					t.Setenv(CodexProBinaryEnv, filepath.Join(cwd, "missing-cli"))
				}
			}
			var events []MachineLifecycle
			a.OnLifecycle = func(e MachineLifecycle) error {
				events = append(events, e)
				if scenario == "reject-intent" && e.Event == "dispatch-intent" || scenario == "reject-possible" && e.Event == "dispatch-possible" {
					return errors.New("durable checkpoint rejected")
				}
				return nil
			}
			result := InvokeHarness(a, "")
			require.NotEmpty(t, events, "%+v", result)
			assert.Equal(t, scenario == "complete", result.Settled, "%+v", result)
			q := MachineRecoveryArgs{Harness: harness, RequestID: a.RequestID, RunID: events[0].RunID, RecoveryDirectory: a.RecoveryDirectory, Cwd: cwd,
				Model: a.Model, ReasoningEffort: "high", PermissionMode: a.PermissionMode, PersistentSession: true}
			var recovered Result
			deadline := time.Now().Add(8 * time.Second)
			for {
				recovered, err = RecoverMachine(q)
				if err == nil || time.Now().After(deadline) {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			require.NoError(t, err)
			assert.True(t, recovered.Recovered)
			assert.True(t, recovered.Session.CleanupConfirmed, "%+v", recovered)
			if scenario == "complete" {
				assert.True(t, recovered.Settled, "%+v", recovered)
				assert.Equal(t, result.Text, recovered.Text)
				assert.Equal(t, result.Usage, recovered.Usage)
			}
			if scenario == "reject-possible" {
				assert.Equal(t, "unknown", recovered.Session.Outcome)
				assert.Nil(t, recovered.CostUSD)
			}
			if scenario == "reject-intent" || scenario == "missing-cli" {
				assert.Equal(t, "not-dispatched", recovered.Session.Dispatch)
				assert.NotNil(t, recovered.CostUSD)
			}
			before, _ := os.ReadFile(log)
			_, err = RecoverMachine(q)
			require.NoError(t, err)
			after, _ := os.ReadFile(log)
			assert.Equal(t, string(before), string(after), "read-only recovery may not create another native call")
			if scenario != "complete" && harness == "codex-pro" {
				assert.NotContains(t, string(before), `"method":"turn/start"`)
			}
			if scenario != "complete" && harness == "claude-code-pro" {
				assert.Empty(t, strings.TrimSpace(string(before)))
			}
		})
	}
}
