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

const claudeCodeProDoctorResponse = `{"protocol":1,"type":"doctor","package":"dsh-subagent-claude-code-pro","version":"0.6.0","machineProtocol":1,"available":true,"inferenceTested":false}`

func TestClaudeCodeProDoctorShowsNativeHarnessAndImportsWithoutInference(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{
 "permissions":{"mode":"full-access"},
 "profiles":{"claude":{"harness":"claude-code-pro","model":"sonnet","channel":"claude-subscription"}},
 "channels":{"claude-subscription":{"env-key":null}},
 "loops":{"review":{"profile":"claude"},"intake":{"profile":"claude"},"garden":{"profile":"claude"}},
 "routes":{"hard":{"profile":"claude"}}
}`)
	t.Setenv(drivers.ClaudeCodeProWorkerEnv, "/custom/worker with spaces")
	t.Setenv(drivers.ClaudeCodeProBinaryEnv, "/custom/claude with spaces")
	for _, probe := range []bool{false, true} {
		runtime.commands = nil
		runtime.command = func(call commandCall) watchdog.Result {
			if len(call.args) == 1 && call.args[0] == "--doctor" {
				assert.Equal(t, "/custom/worker with spaces", call.binary)
				assert.Equal(t, root, call.opts.Cwd)
				return watchdog.Result{ExitCode: new(0), Stdout: claudeCodeProDoctorResponse}
			}
			return defaultCommand(t, root, call)
		}
		report := Doctor(context.Background(), root, "", probe)
		require.True(t, report.OK, "%+v", report.Checks)
		assert.False(t, report.InferenceTested)
		assert.Equal(t, "pass", checkByID(t, report, "claude-code-pro.api").Status)
		assert.Contains(t, checkByID(t, report, "binary.claude").Summary, "/custom/claude with spaces")
		imports := 0
		for _, call := range runtime.commands {
			if call.args[len(call.args)-1] == "--doctor" {
				imports++
			}
		}
		assert.Equal(t, 1, imports)
		assert.Empty(t, runtime.probes, "Claude is a native harness, not a DSH provider")
		for _, binding := range report.Configuration.Bindings {
			assert.Equal(t, "claude-code-pro", binding.Harness)
			assert.Equal(t, "claude-subscription", binding.Channel)
			assert.Empty(t, binding.Provider)
			assert.Equal(t, "sonnet", binding.Model)
			assert.Equal(t, "high", binding.Thinking)
			assert.Equal(t, "danger-full-access", binding.PermissionMode)
			assert.True(t, binding.Session.Supported)
			assert.Equal(t, "warn", checkByID(t, report, binding.ID+".sandbox").Status)
			assert.Equal(t, "unknown", checkByID(t, report, binding.ID+".configuration").Status)
		}
	}
}

func TestClaudeCodeProDoctorRejectsUnavailableOrUnpinnedPublicAPI(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{}`)
	for _, tc := range []struct {
		name   string
		result watchdog.Result
	}{
		{"missing peer", watchdog.Result{ExitCode: new(1), Stdout: `{"available":false}`}},
		{"malformed", watchdog.Result{ExitCode: new(0), Stdout: `{`}},
		{"older native contract", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(claudeCodeProDoctorResponse, "0.6.0", "0.5.0", 1)}},
		{"wrong package", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(claudeCodeProDoctorResponse, "dsh-subagent-claude-code-pro", "another-package", 1)}},
		{"wrong protocol", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(claudeCodeProDoctorResponse, `"protocol":1`, `"protocol":2`, 1)}},
		{"wrong machine protocol", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(claudeCodeProDoctorResponse, `"machineProtocol":1`, `"machineProtocol":2`, 1)}},
		{"wrong type", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(claudeCodeProDoctorResponse, `"doctor"`, `"result"`, 1)}},
		{"not available", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(claudeCodeProDoctorResponse, `"available":true`, `"available":false`, 1)}},
		{"unrequested inference", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(claudeCodeProDoctorResponse, `"inferenceTested":false`, `"inferenceTested":true`, 1)}},
		{"missing inference fact", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(claudeCodeProDoctorResponse, `,"inferenceTested":false`, ``, 1)}},
		{"exit failure", watchdog.Result{ExitCode: new(1), Stdout: claudeCodeProDoctorResponse}},
		{"truncation", watchdog.Result{ExitCode: new(0), Stdout: claudeCodeProDoctorResponse, Truncated: true}},
		{"cleanup", watchdog.Result{ExitCode: new(0), Stdout: claudeCodeProDoctorResponse, CleanupError: "private-credential-value"}},
		{"timeout", watchdog.Result{ExitCode: new(0), Stdout: claudeCodeProDoctorResponse, TimedOut: true}},
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
			r.claudeCodePro(context.Background(), Binding{ID: "routes.hard"}, cache)
			assert.False(t, r.OK)
			assert.Equal(t, "fail", checkByID(t, r, "claude-code-pro.api").Status)
			before := len(runtime.commands)
			r.claudeCodePro(context.Background(), Binding{ID: "loops.review"}, cache)
			assert.Len(t, runtime.commands, before)
			raw, err := json.Marshal(r)
			require.NoError(t, err)
			assert.NotContains(t, string(raw), "private-credential-value")
		})
	}
}

func TestClaudeCodeProDoctorRejectsRolePermissionsWithoutLoadingThePeer(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{"loops":{"plan":{"harness":"claude-code-pro","model":"sonnet","channel":"c"},"garden":{"harness":"claude-code-pro","model":"sonnet","channel":"c"}}}`)
	report := Doctor(context.Background(), root, "", true)
	assert.False(t, report.OK)
	for _, id := range []string{"loops.plan", "loops.garden"} {
		assert.Equal(t, "fail", checkByID(t, report, id+".permission").Status)
		assert.Contains(t, checkByID(t, report, id+".permission").Summary, "full-access")
	}
	for _, call := range runtime.commands {
		assert.Equal(t, "git", call.binary)
	}
	assert.Empty(t, runtime.probes)
}

func TestClaudeCodeProDoctorPrerequisitesAndCancellationNeverStartMachine(t *testing.T) {
	for _, missing := range []string{"worker", "node", "cancelled"} {
		t.Run(missing, func(t *testing.T) {
			root, runtime := diagnosticFixture(t, `{}`)
			t.Setenv(drivers.ClaudeCodeProWorkerEnv, "worker")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if missing == "cancelled" {
				cancel()
			} else {
				runtime.missing[missing] = true
			}
			r := Report{OK: true, Cwd: root}
			r.claudeCodePro(ctx, Binding{ID: "routes.hard"}, map[string]bool{})
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
		if call.binary == "claude" {
			cancel()
		}
		return defaultCommand(t, root, call)
	}
	r := Report{OK: true, Cwd: root}
	r.claudeCodePro(ctx, Binding{ID: "routes.hard"}, map[string]bool{})
	assert.Equal(t, "claude", runtime.commands[len(runtime.commands)-1].binary)
}
