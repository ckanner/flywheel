package diagnostic

import (
	"context"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

const agyDoctorResponse = `{"protocol":1,"type":"doctor","package":"@ckanner/dsh-subagent-antigravity","version":"0.3.1","machineProtocol":1,"available":true,"inferenceTested":false,"evidenceOnly":"verified-managed-agent","nativeVersion":1,"nativeOrchestrator":true,"nativeModes":["native","subagents","swarm"],"nativeAgentLimit":"observed-total-children-guard","hardNativeAgentLimit":false,"nativeCompletionScope":"cli-invocation"}`

func TestAntigravityDoctorReportsPublicCapabilitiesWithoutAccountClaims(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{"subscription":{"version":1,"mode":"subscription-only"},"permissions":{"mode":"full-access","bindings":{"loops.investigate":"read-only"}},"profiles":{"local":{"harness":"antigravity","channel":"g","model":"gemini-4-hydrogen-high","thinking":"high"}},"channels":{"g":{"harness":"antigravity","env-key":null}},"loops":{"investigate":{"profile":"local"}},"native-profiles":{"agy":{"harness":"antigravity","options":{"version":1,"mode":"native"}}},"routes":{"default":{"profile":"local","native-profile":"agy"}},"planning":{"compact":{"design-plan":"local","design-review":"local"}}}`)
	runtime.command = func(call commandCall) watchdog.Result {
		if call.args[len(call.args)-1] == "--doctor" {
			return watchdog.Result{ExitCode: new(0), Stdout: agyDoctorResponse}
		}
		return defaultCommand(t, root, call)
	}
	r := Doctor(context.Background(), root, "", true)
	require.True(t, r.OK, "%+v", r.Checks)
	assert.False(t, r.InferenceTested)
	assert.Equal(t, "pass", checkByID(t, r, "antigravity.api").Status)
	assert.Equal(t, "pass", checkByID(t, r, "routes.default.native.manifest").Status)
	assert.Equal(t, "unknown", checkByID(t, r, "routes.default.native.admission").Status)
	assert.Equal(t, "unknown", checkByID(t, r, "loops.investigate.subscription").Status)
	assert.Empty(t, runtime.probes)
	for _, body := range []string{"{", strings.Replace(agyDoctorResponse, "0.3.1", "0.0.1", 1), strings.Replace(agyDoctorResponse, `"inferenceTested":false`, `"inferenceTested":true`, 1)} {
		runtime.command = func(call commandCall) watchdog.Result {
			if call.args[len(call.args)-1] == "--doctor" {
				return watchdog.Result{ExitCode: new(0), Stdout: body}
			}
			return defaultCommand(t, root, call)
		}
		r = Report{OK: true, Cwd: root}
		cache := map[string]bool{}
		r.antigravity(context.Background(), Binding{ID: "one"}, cache)
		assert.False(t, r.OK)
		before := len(runtime.commands)
		r.antigravity(context.Background(), Binding{ID: "two"}, cache)
		assert.Len(t, runtime.commands, before)
	}
	t.Setenv(drivers.AntigravityWorkerEnv, "missing-worker")
	runtime.missing["missing-worker"] = true
	r = Report{OK: true, Cwd: root}
	r.antigravity(context.Background(), Binding{ID: "missing"}, map[string]bool{})
	assert.False(t, r.OK)
}
