package diagnostic

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInspectAndDoctorUseTheSameFullAccessForEveryStage(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{
 "permissions":{"mode":"full-access"},
 "loops":{
   "plan":{"harness":"kimi","channel":"kimi-subscription"},
   "review":{"harness":"dsh","provider":"chatgpt","model":"gpt-test","channel":"subscription"},
   "judge":{"harness":"dsh","provider":"chatgpt","model":"gpt-test","channel":"subscription"},
   "intake":{"harness":"dsh","provider":"deepseek-official","model":"deepseek-flash","channel":"flash","thinking":"low"},
   "garden":{"harness":"kimi","channel":"kimi-subscription"}},
 "routes":{
   "default":{"harness":"dsh","provider":"chatgpt","model":"gpt-test","channel":"subscription"},
   "frontend":{"harness":"kimi","channel":"kimi-subscription"},
   "hard":{"harness":"codex-pro","model":"gpt-test","channel":"subscription"}},
 "judge-panel":{"harness":"dsh","provider":"chatgpt","model":"gpt-test","channel":"subscription"}
}`)
	runtime.command = func(call commandCall) watchdog.Result {
		if len(call.args) == 1 && call.args[0] == "--doctor" {
			return watchdog.Result{ExitCode: new(0), Stdout: codexProDoctorResponse}
		}
		return defaultCommand(t, root, call)
	}
	view, err := Inspect(root, "")
	require.NoError(t, err)
	require.Len(t, view.Bindings, 12)
	for _, b := range view.Bindings {
		assert.Equal(t, "danger-full-access", b.PermissionMode, b.ID)
		if b.Harness == "kimi" {
			assert.Equal(t, "kimi-code/k3.1-pro", b.Model)
			assert.Equal(t, "high", b.Thinking)
		}
	}
	report := Doctor(context.Background(), root, "", true)
	require.True(t, report.OK, "%+v", report.Checks)
	assert.Equal(t, view.Bindings, report.Configuration.Bindings)
	require.Len(t, runtime.probes, 2, "identical full-access DSH bindings share a no-inference probe")
	for _, probe := range runtime.probes {
		assert.Equal(t, "danger-full-access", probe.PermissionMode)
		assert.True(t, probe.Doctor)
		assert.Empty(t, probe.Prompt)
	}
	assert.Contains(t, checkByID(t, report, "routes.hard.sandbox").Summary, "完全权限")
	assert.Equal(t, "unknown", checkByID(t, report, "routes.hard.configuration").Status)
	for _, check := range report.Checks {
		if strings.HasSuffix(check.ID, ".sdk") && check.Status == "pass" {
			assert.Contains(t, check.Summary, "danger-full-access 权限策略")
			assert.NotContains(t, check.Summary, "完整沙箱")
		}
	}
}

func TestDoctorRejectsUnsupportedLegacyFullAccessBeforeStartingHarness(t *testing.T) {
	for _, harness := range []string{"pi", "omp", "zcode"} {
		t.Run(harness, func(t *testing.T) {
			root, runtime := diagnosticFixture(t, fmt.Sprintf(`{"permissions":{"mode":"full-access"},"loops":{"judge":{"harness":%q}}}`, harness))
			report := Doctor(context.Background(), root, "", true)
			assert.False(t, report.OK)
			assert.Equal(t, "fail", checkByID(t, report, "loops.judge.permission").Status)
			assert.Empty(t, runtime.probes)
			for _, call := range runtime.commands {
				assert.Equal(t, "git", call.binary)
			}
		})
	}
}
