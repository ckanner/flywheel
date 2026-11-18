package drivers

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These cases feed actual Node recovery output to the production Go parser.
// The SDK and worker are real; the provider and final plugin inspection are
// offline fixtures. Recovery itself must not make a second model request.
func TestDSHWebRecoveryNodeBridge(t *testing.T) {
	if os.Getenv("FLYWHEEL_TEST_NODE_BRIDGE") != "1" {
		t.Skip("requires Node and the published SDK dependencies")
	}
	node, err := exec.LookPath("node")
	require.NoError(t, err)
	sdk, err := filepath.Abs("../../plugins/dsh/test/fixtures/persisted-sdk.js")
	require.NoError(t, err)
	owner, err := filepath.Abs("../../plugins/dsh/test/fixtures/web-recovery-owner.js")
	require.NoError(t, err)
	query, err := filepath.Abs("../../plugins/dsh/test/fixtures/web-recovery-query.js")
	require.NoError(t, err)
	t.Setenv("FLYWHEEL_DSH_BIN", sdk)
	t.Setenv("FLYWHEEL_DSH_PROFILE", "sdk")
	t.Setenv("DSH_HOME", t.TempDir())
	for _, scenario := range []string{"completed", "retired", "unsent"} {
		t.Run(scenario, func(t *testing.T) {
			cwd, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			id := "original-" + scenario
			directory := filepath.Join(cwd, ".sessions", fmt.Sprintf("recovery-%x", sha256.Sum256([]byte(id))))
			require.NoError(t, os.Mkdir(filepath.Dir(directory), 0o700))
			sdkScenario := "ok"
			if scenario == "retired" {
				sdkScenario = "rejected-response"
			}
			calls := filepath.Join(cwd, "calls.jsonl")
			env := map[string]string{"PATH": os.Getenv("PATH"), "HOME": os.Getenv("HOME"), "DSH_HOME": os.Getenv("DSH_HOME"),
				"FLYWHEEL_DSH_BIN": sdk, "FLYWHEEL_DSH_PROFILE": "sdk", "TEST_DSH_WEB_SCENARIO": sdkScenario,
				"TEST_DSH_HISTORY": filepath.Join(cwd, "history"), "TEST_DSH_LLM_CALLS": calls}
			request := dshStart{Type: "start", Protocol: 2, RequestID: id, Cwd: cwd, Provider: "offline-fixture", Model: DSHWebModel,
				ReasoningEffort: "pro", PermissionMode: "danger-full-access", Persistent: true, Prompt: "Original offline SDK operation", TimeoutMS: 15000,
				Web: &DSHWebRequest{Version: 1, ExecutionDomainID: "original-domain", RequireIsolation: true, RecoveryDirectory: directory}}
			input, err := json.Marshal(map[string]any{"request": request, "env": env, "unsent": scenario == "unsent"})
			require.NoError(t, err)
			command := exec.Command(node, owner)
			command.Stdin = bytes.NewReader(input)
			output, err := command.CombinedOutput()
			require.NoError(t, err, "%s", output)
			before, beforeErr := os.ReadFile(calls)
			if scenario == "unsent" {
				require.True(t, os.IsNotExist(beforeErr))
			} else {
				require.NoError(t, beforeErr)
				require.NotEmpty(t, before)
			}
			t.Setenv(DSHWorkerEnv, kimiScript(t, "exec "+shQuote(node)+" "+shQuote(query)+" "+shQuote(scenario)+"\n"))
			got, err := RecoverDSHWeb(DSHWebRecoveryArgs{RequestID: id, RecoveryDirectory: directory, Cwd: cwd, Provider: request.Provider,
				PermissionMode: request.PermissionMode, ExecutionDomainID: request.Web.ExecutionDomainID})
			require.NoError(t, err)
			assert.True(t, got.Recovered)
			assert.True(t, got.Session.CleanupConfirmed)
			assert.Equal(t, "unknown", got.Session.UsageStatus)
			assert.Nil(t, got.Usage)
			if scenario == "completed" {
				assert.Equal(t, "completed", got.Session.Outcome)
				assert.Equal(t, "published SDK fixture complete", got.Text)
			} else {
				assert.Equal(t, "failed", got.Session.Outcome)
				assert.False(t, got.Session.ResumeCandidate)
				assert.NotNil(t, got.ErrorMessage)
			}
			if scenario == "unsent" {
				assert.Empty(t, got.Session.TurnID)
				assert.Equal(t, "not-dispatched", got.Session.Dispatch)
				_, err = os.Stat(calls)
				assert.True(t, os.IsNotExist(err))
			} else {
				assert.NotEmpty(t, got.Session.TurnID)
				after, err := os.ReadFile(calls)
				require.NoError(t, err)
				assert.Equal(t, before, after, "recovery did not send another model request")
			}
		})
	}
}
