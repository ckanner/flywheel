package diagnostic

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const codexProDoctorResponse = `{"protocol":1,"type":"doctor","package":"dsh-subagent-codex-pro","version":"0.8.0","machineProtocol":1,"available":true,"inferenceTested":false}`

func TestCodexProDoctorImportsThePinnedAPIWithoutInference(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{
 "loops":{"review":{"harness":"codex-pro","model":"gpt-6.1-sol","channel":"subscription"},"intake":{"harness":"codex-pro","model":"gpt-6.1-sol","channel":"subscription"}},
 "routes":{"hard":{"harness":"codex-pro","model":"gpt-6.1-sol","channel":"subscription"}}
}`)
	t.Setenv(drivers.CodexProWorkerEnv, "/custom/worker with spaces")
	t.Setenv(drivers.CodexProBinaryEnv, "/custom/codex with spaces")
	for _, probe := range []bool{false, true} {
		runtime.commands = nil
		runtime.command = func(call commandCall) watchdog.Result {
			if len(call.args) == 1 && call.args[0] == "--doctor" {
				assert.Equal(t, "/custom/worker with spaces", call.binary)
				assert.Equal(t, root, call.opts.Cwd)
				return watchdog.Result{ExitCode: new(0), Stdout: codexProDoctorResponse}
			}
			return defaultCommand(t, root, call)
		}
		report := Doctor(context.Background(), root, "", probe)
		require.True(t, report.OK, "%+v", report.Checks)
		assert.False(t, report.InferenceTested)
		assert.Equal(t, "pass", checkByID(t, report, "codex-pro.api").Status)
		assert.Contains(t, checkByID(t, report, "binary.codex").Summary, "/custom/codex with spaces")
		imports := 0
		for _, call := range runtime.commands {
			if call.args[len(call.args)-1] == "--doctor" {
				imports++
			}
		}
		assert.Equal(t, 1, imports, "all stage bindings share the same import-only check")
		assert.Empty(t, runtime.probes, "the Codex Pro backend is not a DSH native provider")
		for _, binding := range report.Configuration.Bindings {
			assert.Equal(t, "gpt-6.1-sol", binding.Model)
			assert.Equal(t, "high", binding.Thinking)
			if binding.Role != "execute" {
				assert.Equal(t, "read-only", binding.PermissionMode)
			}
			assert.Equal(t, "warn", checkByID(t, report, binding.ID+".sandbox").Status)
			assert.Equal(t, "unknown", checkByID(t, report, binding.ID+".configuration").Status)
			assert.Equal(t, "unknown", checkByID(t, report, binding.ID+".authentication").Status)
		}
	}
}

func TestCodexProDoctorRejectsUnprovenOrInconsistentImports(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{}`)
	for _, tc := range []struct {
		name   string
		result watchdog.Result
	}{
		{"invalid JSON", watchdog.Result{ExitCode: new(0), Stdout: "private-credential-value"}},
		{"missing inference assertion", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(codexProDoctorResponse, `,"inferenceTested":false`, "", 1)}},
		{"performed inference", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(codexProDoctorResponse, `"inferenceTested":false`, `"inferenceTested":true`, 1)}},
		{"wrong version", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(codexProDoctorResponse, "0.8.0", "0.7.0", 1)}},
		{"wrong package", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(codexProDoctorResponse, "dsh-subagent-codex-pro", "another-package", 1)}},
		{"wrong protocol", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(codexProDoctorResponse, `"protocol":1`, `"protocol":2`, 1)}},
		{"wrong machine protocol", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(codexProDoctorResponse, `"machineProtocol":1`, `"machineProtocol":2`, 1)}},
		{"wrong type", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(codexProDoctorResponse, `"doctor"`, `"result"`, 1)}},
		{"not available", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(codexProDoctorResponse, `"available":true`, `"available":false`, 1)}},
		{"exit failure", watchdog.Result{ExitCode: new(1), Stdout: codexProDoctorResponse}},
		{"truncation", watchdog.Result{ExitCode: new(0), Stdout: codexProDoctorResponse, Truncated: true}},
		{"cleanup", watchdog.Result{ExitCode: new(0), Stdout: codexProDoctorResponse, CleanupError: "private-credential-value"}},
		{"timeout", watchdog.Result{ExitCode: new(0), Stdout: codexProDoctorResponse, TimedOut: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime.command = func(call commandCall) watchdog.Result {
				if call.args[len(call.args)-1] == "--doctor" {
					return tc.result
				}
				return defaultCommand(t, root, call)
			}
			r := Report{OK: true, Cwd: root}
			cache := map[string]bool{}
			r.codexPro(context.Background(), Binding{ID: "routes.hard"}, cache)
			assert.False(t, r.OK)
			assert.Equal(t, "fail", checkByID(t, r, "codex-pro.api").Status)
			before := len(runtime.commands)
			r.codexPro(context.Background(), Binding{ID: "loops.review"}, cache)
			assert.Len(t, runtime.commands, before, "an unsuccessful check is also deduplicated")
			raw, err := json.Marshal(r)
			require.NoError(t, err)
			assert.NotContains(t, string(raw), "private-credential-value")
		})
	}
}

func TestCodexProDoctorPrerequisitesAndCancellationNeverStartMachine(t *testing.T) {
	for _, missing := range []string{"worker", "node", "cancelled"} {
		t.Run(missing, func(t *testing.T) {
			root, runtime := diagnosticFixture(t, `{}`)
			t.Setenv(drivers.CodexProWorkerEnv, "worker")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if missing == "cancelled" {
				cancel()
			} else {
				runtime.missing[missing] = true
			}
			r := Report{OK: true, Cwd: root}
			r.codexPro(ctx, Binding{ID: "routes.hard"}, map[string]bool{})
			for _, call := range runtime.commands {
				assert.Equal(t, []string{"--version"}, call.args)
			}
			assert.Empty(t, runtime.probes)
		})
	}
	root, runtime := diagnosticFixture(t, `{}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime.command = func(call commandCall) watchdog.Result {
		if call.binary == "codex" {
			cancel()
		}
		return defaultCommand(t, root, call)
	}
	r := Report{OK: true, Cwd: root}
	r.codexPro(ctx, Binding{ID: "routes.hard"}, map[string]bool{})
	assert.Equal(t, "codex", runtime.commands[len(runtime.commands)-1].binary, "no import command after a cancellation race")
}

func TestDoctorNodeVersionMatchesThePluginMinimum(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{}`)
	for _, tc := range []struct {
		version string
		ok      bool
	}{
		{"20.19.0", false}, {"22.0.0", false}, {"22.17.9", false}, {"22.18.0", true}, {"23.0.0", true}, {"24.22.1", true},
	} {
		runtime.command = func(commandCall) watchdog.Result {
			return watchdog.Result{ExitCode: new(0), Stdout: "v" + tc.version}
		}
		r := Report{OK: true, Cwd: root}
		assert.Equal(t, tc.ok, r.command(context.Background(), "node", "node", map[string]bool{}, true), tc.version)
	}
}
