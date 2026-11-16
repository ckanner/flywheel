package diagnostic

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionInspectionSeparatesConfigurationFromRestoreEvidence(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{
 "sessions":{"mode":"scoped","policy-version":1},
 "permissions":{"mode":"full-access"},
 "loops":{
   "plan":{"harness":"kimi","resident-session":false},
   "review":{"harness":"dsh","provider":"chatgpt","model":"gpt-test","channel":"subscription"},
   "judge":{"harness":"dsh","provider":"chatgpt","model":"gpt-test","channel":"subscription"},
   "intake":{"harness":"dsh","provider":"adapter","model":"flash","channel":"account"},
   "garden":{"harness":"kimi"}},
 "routes":{
   "default":{"harness":"dsh","provider":"adapter","model":"flash","channel":"account"},
   "frontend":{"harness":"kimi"},
   "hard":{"harness":"codex-pro","model":"gpt-test","channel":"subscription"},
   "compatibility":{"harness":"codex"}},
 "judge-panel":{"harness":"dsh","provider":"chatgpt","model":"gpt-test","channel":"subscription"}
}`)
	runtime.command = func(call commandCall) watchdog.Result {
		if len(call.args) == 1 && call.args[0] == "--doctor" {
			return watchdog.Result{ExitCode: new(0), Stdout: codexProDoctorResponse}
		}
		return defaultCommand(t, root, call)
	}
	before, err := os.ReadDir(root)
	require.NoError(t, err)
	view, err := Inspect(root, "")
	require.NoError(t, err)
	assert.Equal(t, config.SessionsConfig{Mode: "scoped", PolicyVersion: 1}, view.Sessions)
	wantReasons := map[string]string{
		"loops.plan": "plan-resident-session-disabled", "loops.intake": "advisory-role-outside-scoped-policy",
		"loops.garden": "advisory-role-outside-scoped-policy", "routes.compatibility": "unsupported-adapter",
	}
	for _, binding := range view.Bindings {
		assert.False(t, binding.Session.HistoryRecoveryTested)
		assert.Equal(t, binding.Harness != "codex", binding.Session.Supported)
		if reason, ok := wantReasons[binding.ID]; ok {
			assert.Equal(t, "fresh", binding.Session.Mode)
			assert.Equal(t, reason, binding.Session.Reason)
		} else {
			assert.Equal(t, "scoped", binding.Session.Mode)
			assert.Equal(t, "normal-terminal-checkpoint-required", binding.Session.Reason)
		}
	}
	report := Doctor(context.Background(), root, "", true)
	require.True(t, report.OK, "%+v", report.Checks)
	assert.Equal(t, view.Bindings, report.Configuration.Bindings)
	assert.Equal(t, "pass", checkByID(t, report, "sessions.policy").Status)
	assert.Contains(t, checkByID(t, report, "sessions.policy").Summary, "scoped")
	for _, binding := range view.Bindings {
		check := checkByID(t, report, binding.ID+".session")
		assert.Equal(t, "unknown", check.Status)
		assert.Contains(t, check.Summary, "历史恢复未验证")
	}
	assert.False(t, report.InferenceTested)
	for _, probe := range runtime.probes {
		assert.True(t, probe.Doctor)
		assert.Empty(t, probe.Prompt)
	}
	after, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Equal(t, before, after, "config and doctor cannot create session state or receipts")
	assert.NoFileExists(t, filepath.Join(root, "state.json"))
}

func TestSessionInspectionPreservesLegacyPlanAndExplicitOptOut(t *testing.T) {
	for _, tc := range []struct {
		name, mode, resident, wantMode, wantReason string
	}{
		{"old resident", "", `,"resident-session":true`, "legacy-plan", "explicit-kimi-plan-history"},
		{"old fresh", "", "", "fresh", "fresh-policy"},
		{"explicit fresh", "fresh", `,"resident-session":false`, "fresh", "fresh-policy"},
		{"scoped omitted", "scoped", "", "scoped", "normal-terminal-checkpoint-required"},
		{"scoped true", "scoped", `,"resident-session":true`, "scoped", "normal-terminal-checkpoint-required"},
		{"scoped opt out", "scoped", `,"resident-session":false`, "fresh", "plan-resident-session-disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := ""
			if tc.mode != "" {
				policy = `"sessions":{"mode":"` + tc.mode + `","policy-version":1},`
			}
			root, _ := diagnosticFixture(t, `{`+policy+`"loops":{"plan":{"harness":"kimi"`+tc.resident+`}}}`)
			view, err := Inspect(root, "")
			require.NoError(t, err)
			require.Len(t, view.Bindings, 2)
			binding := bindingByID(t, view, "loops.plan")
			assert.Equal(t, tc.wantMode, binding.Session.Mode)
			assert.Equal(t, tc.wantReason, binding.Session.Reason)
			assert.False(t, binding.Session.HistoryRecoveryTested)
			raw, err := json.Marshal(view)
			require.NoError(t, err)
			assert.Contains(t, string(raw), `"historyRecoveryTested":false`)
		})
	}
}
