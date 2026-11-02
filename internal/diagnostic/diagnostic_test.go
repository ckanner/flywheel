package diagnostic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const specFixture = `---
spec-version: 1
title: Diagnostic fixture
goal: Inspect without executing anything.
milestones: [{id: M-01, title: First}]
requirements: [{id: R-01, title: Safe inspection, milestone: M-01}]
---
`

func writeFixture(t *testing.T, file, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
	require.NoError(t, os.WriteFile(file, []byte(body), 0o600))
}

func taskFixture(t *testing.T, root, route string) string {
	t.Helper()
	task := filepath.Join(root, "docs", "task")
	writeFixture(t, filepath.Join(task, "spec.md"), specFixture)
	if route != "" {
		writeFixture(t, filepath.Join(task, "task-packet-M01-T01.md"), fmt.Sprintf(`---
packet-id: M01-T01
contract-version: 1
goal: Inspect only.
requirements: [R-01]
route: %s
context-files: []
boundaries: {max-files: 1}
acceptance:
  L1: [{cmd: "touch acceptance-must-not-run", expect: "exit 0"}]
review-rubric: Check the result.
escalation: {max-retries: 1, on-fail: inbox}
done-report-format: {requirements-covered: [], changed-files: [], commands-run: [], failures: []}
---
`, route))
	}
	return task
}

type commandCall struct {
	binary string
	args   []string
	opts   watchdog.Opts
}

type fakeRuntime struct {
	commands []commandCall
	probes   []drivers.DSHArgs
	missing  map[string]bool
	command  func(commandCall) watchdog.Result
	probe    func(drivers.DSHArgs) drivers.Result
}

func useFakeRuntime(t *testing.T, root string) *fakeRuntime {
	t.Helper()
	f := &fakeRuntime{missing: map[string]bool{}}
	oldLook, oldRun, oldProbe := lookPath, runCommand, probeDSH
	t.Cleanup(func() { lookPath, runCommand, probeDSH = oldLook, oldRun, oldProbe })
	lookPath = func(binary string) (string, error) {
		if f.missing[binary] {
			return "", errors.New("not found")
		}
		return binary, nil
	}
	runCommand = func(binary string, args []string, opts watchdog.Opts) watchdog.Result {
		call := commandCall{binary: binary, args: append([]string{}, args...), opts: opts}
		f.commands = append(f.commands, call)
		if f.command != nil {
			return f.command(call)
		}
		return defaultCommand(t, root, call)
	}
	probeDSH = func(args drivers.DSHArgs) drivers.Result {
		f.probes = append(f.probes, args)
		if f.probe != nil {
			return f.probe(args)
		}
		return drivers.Result{ExitCode: new(0), Settled: true, Harness: "dsh", Provider: args.Provider, Model: args.Model}
	}
	return f
}

func defaultCommand(t *testing.T, root string, call commandCall) watchdog.Result {
	t.Helper()
	output := ""
	switch {
	case len(call.args) == 4 && call.args[3] == "--show-toplevel":
		output = root + "\n"
	case len(call.args) == 4 && call.args[3] == "--is-inside-work-tree":
		output = "true\n"
	case len(call.args) > 0 && call.args[len(call.args)-1] == "--version":
		output = "product 24.2.1\n"
	default:
		t.Fatalf("diagnostic attempted a non-inspection command: %s %q", call.binary, call.args)
	}
	return watchdog.Result{ExitCode: new(0), Stdout: output}
}

func diagnosticFixture(t *testing.T, raw string) (string, *fakeRuntime) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv(config.GlobalConfigEnv, filepath.Join(t.TempDir(), "absent", "global.json"))
	if raw != "" {
		writeFixture(t, filepath.Join(root, "flywheel.config.json"), raw)
	}
	return root, useFakeRuntime(t, root)
}

func checkByID(t *testing.T, report Report, id string) Check {
	t.Helper()
	for _, check := range report.Checks {
		if check.ID == id {
			return check
		}
	}
	t.Fatalf("missing check %q in %+v", id, report.Checks)
	return Check{}
}

const dshConfiguration = `{
  "loops": {
    "plan": {"harness":"dsh","provider":"adapter","model":"model","channel":"account"},
    "review": {"harness":"dsh","provider":"adapter","model":"model","channel":"account"},
    "judge": {"harness":"dsh","provider":"adapter","model":"model","channel":"account"},
    "intake": {"harness":"dsh","provider":"other","model":"other","channel":"advice"},
    "garden": {"harness":"kimi"}
  },
  "routes": {
    "default": {"harness":"codex"},
    "frontend": {"harness":"dsh","provider":"adapter","model":"model","channel":"account"}
  },
  "judge-panel": {"harness":"dsh","provider":"adapter","model":"model","channel":"account"}
}`

func TestInspectResolvesProfilesAndActualDriverDefaults(t *testing.T) {
	root, _ := diagnosticFixture(t, `{
 "profiles":{"reader":{"harness":"dsh","provider":"chatgpt","model":"gpt-test","channel":"personal-account","thinking":"low"}},
 "loops":{"plan":{"harness":"kimi","resident-session":true},"review":{"profile":"reader"},"judge":{"harness":"codex","resident-session":true},"intake":{},"garden":{"harness":"kimi"}},
 "routes":{"default":{},"frontend":{"harness":"omp"},"hard":{"harness":"zcode"}},
 "judge-panel":{"profile":"reader","thinking":"high"}
}`)
	view, err := Inspect(root, "")
	require.NoError(t, err)
	assert.Equal(t, "project", view.Scope)
	ids := []string{}
	bindings := map[string]Binding{}
	for _, b := range view.Bindings {
		ids = append(ids, b.ID)
		bindings[b.ID] = b
	}
	assert.IsIncreasing(t, ids)
	assert.Equal(t, "reader", bindings["loops.review"].Profile)
	assert.Equal(t, "chatgpt", bindings["loops.review"].Provider)
	assert.Equal(t, "personal-account", bindings["loops.review"].Channel)
	assert.Equal(t, "read-only", bindings["judge-panel"].PermissionMode)
	assert.Equal(t, "high", bindings["judge-panel"].Thinking)
	assert.Equal(t, drivers.KimiDefaultModel, bindings["loops.plan"].Model)
	assert.Equal(t, "high", bindings["loops.plan"].Thinking)
	assert.True(t, bindings["loops.plan"].ResidentSession)
	assert.False(t, bindings["loops.judge"].ResidentSession)
	assert.Equal(t, drivers.CodexDefaultModel, bindings["loops.judge"].Model)
	assert.Equal(t, drivers.ZcodeDefaultModel, bindings["routes.hard"].Model)
	assert.Equal(t, "max", bindings["routes.hard"].Thinking)
	assert.Equal(t, "pi", bindings["routes.default"].Harness)
	assert.Equal(t, "max", bindings["routes.default"].Thinking)
	assert.Empty(t, bindings["routes.default"].Model, "Flywheel has no known pi model default")
	assert.Empty(t, bindings["routes.frontend"].Model)
	assert.Empty(t, bindings["loops.intake"].PermissionMode)
	assert.Empty(t, bindings["loops.garden"].PermissionMode)
}

func TestInspectRealReadOnlyDiscoveryAndSymlinkPaths(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	git := exec.Command("git", "init", "-q", root)
	require.NoError(t, git.Run())
	task := taskFixture(t, root, "default")
	sub := filepath.Join(root, "src")
	require.NoError(t, os.Mkdir(sub, 0o755))
	alias := filepath.Join(t.TempDir(), "repo-alias")
	require.NoError(t, os.Symlink(root, alias))
	global := filepath.Join(t.TempDir(), "global-parent", "flywheel.config.json")
	t.Setenv(config.GlobalConfigEnv, global)
	projectFile, taskFile := filepath.Join(root, "flywheel.config.json"), filepath.Join(task, "flywheel.config.json")
	writeFixture(t, global, `{"loops":{"plan":{"harness":"kimi","model":"global-model"}}}`)
	writeFixture(t, projectFile, `{"loops":{"plan":{"harness":"kimi","model":"project-model"}}}`)
	writeFixture(t, taskFile, `{"loops":{"plan":{"harness":"kimi","model":"task-model"}}}`)
	for _, expected := range []struct{ scope, source string }{{"task", taskFile}, {"project", projectFile}, {"global", global}} {
		view, err := Inspect(filepath.Join(alias, "src"), "../docs/task")
		require.NoError(t, err)
		assert.Equal(t, root, view.Cwd)
		assert.Equal(t, task, view.TaskDir)
		assert.Equal(t, expected.scope, view.Scope)
		assert.Equal(t, expected.source, view.Source)
		require.NoError(t, os.Remove(expected.source))
	}
	require.NoError(t, os.Remove(filepath.Dir(global)))
	view, err := Inspect(sub, task)
	require.NoError(t, err)
	assert.Equal(t, "embedded", view.Scope)
	assert.NoDirExists(t, filepath.Dir(global), "read-only discovery never materializes global configuration")
	assert.NoFileExists(t, filepath.Join(task, "state.json"))
	assert.NoFileExists(t, filepath.Join(root, "acceptance-must-not-run"))
}

func TestInspectDirectoryAndConfigurationFailuresAreExplicit(t *testing.T) {
	root, _ := diagnosticFixture(t, `{"loops":{"plan":{"harness":"kimi"}}}`)
	_, err := Inspect(filepath.Join(root, "missing"), "")
	require.ErrorContains(t, err, "working directory")
	_, err = Inspect(root, "missing-task")
	require.ErrorContains(t, err, "task directory")
	file := filepath.Join(root, "regular-file")
	writeFixture(t, file, "x")
	_, err = resolveDirectory(file)
	require.ErrorContains(t, err, "not a directory")
	t.Run("getcwd failure", func(t *testing.T) {
		old := directoryAbs
		directoryAbs = func(string) (string, error) { return "", errors.New("getcwd failed") }
		t.Cleanup(func() { directoryAbs = old })
		_, err := resolveDirectory(".")
		require.ErrorContains(t, err, "getcwd failed")
	})
	t.Run("path disappears between canonicalization and stat", func(t *testing.T) {
		old := directoryStat
		directoryStat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		t.Cleanup(func() { directoryStat = old })
		_, err := resolveDirectory(root)
		require.ErrorIs(t, err, os.ErrNotExist)
	})
	task := taskFixture(t, root, "")
	secret := "sk-private-value-that-must-not-appear"
	writeFixture(t, filepath.Join(task, "flywheel.config.json"), `{"loops":{"plan":{"harness":"`+secret+`"}}}`)
	view, err := Inspect(root, task)
	require.Error(t, err)
	assert.Nil(t, view, "a broken task file must not select the valid project file")
	assert.NotContains(t, err.Error(), secret)
}

func TestProjectRootDistinguishesNonGitFromIncompleteDiscovery(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{}`)
	for _, tc := range []struct {
		name      string
		result    watchdog.Result
		wantError bool
	}{
		{"non-git", watchdog.Result{ExitCode: new(128)}, false},
		{"git missing", watchdog.Result{SpawnError: "not found"}, false},
		{"cancelled", watchdog.Result{Cancelled: true}, true},
		{"timeout", watchdog.Result{TimedOut: true}, true},
		{"truncated", watchdog.Result{ExitCode: new(0), Stdout: root, Truncated: true}, true},
		{"signal", watchdog.Result{Signal: "terminated"}, true},
		{"cleanup", watchdog.Result{CleanupError: "still running"}, true},
		{"invalid root", watchdog.Result{ExitCode: new(0), Stdout: "relative-path"}, true},
		{"missing root", watchdog.Result{ExitCode: new(0), Stdout: filepath.Join(root, "gone")}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime.command = func(commandCall) watchdog.Result { return tc.result }
			result, err := projectRoot(context.Background(), root)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, root, result)
			}
		})
	}
}

func TestDoctorTaskScopeAndNoInferenceDeduplication(t *testing.T) {
	root, runtime := diagnosticFixture(t, dshConfiguration)
	task := taskFixture(t, root, "frontend")
	runtime.missing["kimi"], runtime.missing["codex"] = true, true
	report := Doctor(context.Background(), root, task, true)
	require.True(t, report.OK, "%+v", report.Checks)
	assert.False(t, report.InferenceTested)
	assert.Len(t, runtime.probes, 2, "one write probe and one shared read-only probe")
	for _, call := range runtime.probes {
		assert.True(t, call.Doctor)
		assert.Empty(t, call.Prompt)
		assert.Equal(t, "adapter", call.Provider)
		assert.Equal(t, "model", call.Model)
		assert.Equal(t, "high", call.Thinking)
		assert.Equal(t, root, call.Cwd)
		assert.LessOrEqual(t, call.Watchdog.WallTimeoutMs, int64(10000))
		assert.Equal(t, int64(5000), call.Watchdog.KillGraceMs)
	}
	for _, id := range []string{"loops.intake", "loops.garden", "routes.default"} {
		assert.Equal(t, "skipped", checkByID(t, report, id).Status)
	}
	assert.Equal(t, "pass", checkByID(t, report, "contracts").Status)
	assert.Contains(t, checkByID(t, report, "loops.review.authentication").Summary, "不证明实际账号")
	assert.NoFileExists(t, filepath.Join(root, "acceptance-must-not-run"))
	assert.NoFileExists(t, filepath.Join(task, "state.json"))
	assert.NoFileExists(t, os.Getenv(config.GlobalConfigEnv))
}

func TestDoctorChecksAllBindingsWithoutTaskAndOnlyDefaultBeforePlanning(t *testing.T) {
	t.Run("no task checks all bindings", func(t *testing.T) {
		root, runtime := diagnosticFixture(t, dshConfiguration)
		report := Doctor(context.Background(), root, "", false)
		require.True(t, report.OK, "%+v", report.Checks)
		assert.Empty(t, runtime.probes)
		for _, binding := range report.Configuration.Bindings {
			assert.True(t, binding.InScope)
		}
		assert.Equal(t, "pass", checkByID(t, report, "binary.kimi").Status)
		assert.Equal(t, "pass", checkByID(t, report, "binary.codex").Status)
		assert.Equal(t, "unknown", checkByID(t, report, "loops.intake.authentication").Status)
	})
	t.Run("pre-planning checks only default execute route", func(t *testing.T) {
		root, runtime := diagnosticFixture(t, dshConfiguration)
		task := taskFixture(t, root, "")
		require.NoError(t, os.Mkdir(filepath.Join(task, "task-packet-ignored.md"), 0o755))
		writeFixture(t, filepath.Join(task, "notes.md"), "not a packet")
		report := Doctor(context.Background(), root, task, false)
		require.True(t, report.OK, "%+v", report.Checks)
		assert.Empty(t, runtime.probes)
		assert.Equal(t, "warn", checkByID(t, report, "contracts.packets").Status)
		assert.Equal(t, "pass", checkByID(t, report, "binary.codex").Status)
		assert.Equal(t, "skipped", checkByID(t, report, "routes.frontend").Status)
	})
}

func TestDoctorInvalidContractsAndMissingBindingsBlockProbes(t *testing.T) {
	for _, fault := range []string{"spec", "packet", "directory", "missing-route", "missing-loops"} {
		t.Run(fault, func(t *testing.T) {
			raw := dshConfiguration
			if fault == "missing-loops" {
				raw = `{}`
			}
			root, runtime := diagnosticFixture(t, raw)
			route := "frontend"
			if fault == "missing-route" {
				route = "hard"
			}
			if fault == "missing-loops" {
				route = ""
			}
			task := taskFixture(t, root, route)
			switch fault {
			case "spec":
				writeFixture(t, filepath.Join(task, "spec.md"), "bad spec includes secret-value")
			case "packet":
				writeFixture(t, filepath.Join(task, "task-packet-M01-T01.md"), "bad packet")
			case "directory":
				old := readDirectory
				readDirectory = func(string) ([]os.DirEntry, error) { return nil, os.ErrPermission }
				t.Cleanup(func() { readDirectory = old })
			}
			report := Doctor(context.Background(), root, task, true)
			assert.False(t, report.OK)
			assert.Empty(t, runtime.probes)
			if fault == "missing-loops" {
				var missing []string
				for _, check := range report.Checks {
					if check.Status == "fail" {
						missing = append(missing, check.ID)
					}
				}
				assert.Equal(t, []string{"loops.judge", "loops.plan", "loops.review", "routes.default"}, missing)
			} else if fault == "missing-route" {
				assert.Equal(t, "fail", checkByID(t, report, "routes.hard").Status)
			} else {
				assert.Equal(t, "fail", checkByID(t, report, "contracts").Status)
			}
		})
	}
}

func TestDoctorSkipsMockAndRejectsUnsupportedRealPermission(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{"drivers":{"review":"mock","execute":"mock"},"loops":{"review":{},"judge":{"harness":"kimi"}},"routes":{"default":{"harness":"dsh"}}}`)
	report := Doctor(context.Background(), root, "", true)
	assert.False(t, report.OK)
	assert.Equal(t, "skipped", checkByID(t, report, "loops.review").Status)
	assert.Equal(t, "skipped", checkByID(t, report, "routes.default").Status)
	assert.Equal(t, "fail", checkByID(t, report, "loops.judge.permission").Status)
	assert.Empty(t, runtime.probes)
	assert.Len(t, runtime.commands, 3, "only Git discovery/version/workspace checks, no model harness")
}

func TestDoctorCredentialChecksAndReportsNeverExposeValues(t *testing.T) {
	secret := "sk-doctor-private-credential"
	root, runtime := diagnosticFixture(t, `{"loops":{"review":{"harness":"dsh","provider":"adapter","model":"model","channel":"account"}},"channels":{"account":{"env-key":"FLYWHEEL_TEST_DIAGNOSTIC_KEY"}}}`)
	t.Setenv("FLYWHEEL_TEST_DIAGNOSTIC_KEY", "  ")
	missing := Doctor(context.Background(), root, "", true)
	assert.False(t, missing.OK)
	assert.Equal(t, "AUTH", checkByID(t, missing, "loops.review.credential").Code)
	assert.Empty(t, runtime.probes)
	t.Setenv("FLYWHEEL_TEST_DIAGNOSTIC_KEY", secret)
	runtime.command = func(call commandCall) watchdog.Result {
		result := defaultCommand(t, root, call)
		if call.args[len(call.args)-1] == "--version" {
			result.Stdout = "product 24.2.1-" + secret + "\n" + secret
		}
		return result
	}
	runtime.probe = func(a drivers.DSHArgs) drivers.Result {
		assert.Equal(t, map[string]string{"FLYWHEEL_TEST_DIAGNOSTIC_KEY": secret}, a.APIKeys)
		message := "401 unauthorized " + secret
		return drivers.Result{ExitCode: new(1), ErrorMessage: &message}
	}
	report := Doctor(context.Background(), root, "", true)
	assert.False(t, report.OK)
	assert.Equal(t, "AUTH", checkByID(t, report, "loops.review.sdk").Code)
	assert.Equal(t, "pass", checkByID(t, report, "loops.review.credential").Status)
	raw, err := json.Marshal(report)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), secret)
	for _, call := range runtime.commands {
		assert.NotContains(t, strings.Join(call.opts.Env, "\n"), "FLYWHEEL_TEST_DIAGNOSTIC_KEY=")
	}
}

func TestDoctorMissingRuntimeBindingAndWorkspaceDiagnostics(t *testing.T) {
	for _, fault := range []string{"worker", "node", "dsh", "git", "binding", "nongit", "nongit-task", "configuration"} {
		t.Run(fault, func(t *testing.T) {
			raw := dshConfiguration
			if fault == "binding" {
				raw = `{"loops":{"review":{"harness":"dsh"}}}`
			}
			if fault == "configuration" {
				raw = `{"unknown":"sk-secret"}`
			}
			root, runtime := diagnosticFixture(t, raw)
			t.Setenv(drivers.DSHWorkerEnv, "worker-binary")
			t.Setenv("FLYWHEEL_DSH_BIN", "dsh-custom")
			task := ""
			switch fault {
			case "worker":
				runtime.missing["worker-binary"] = true
			case "node":
				runtime.missing["node"] = true
			case "dsh":
				runtime.missing["dsh-custom"] = true
			case "git":
				runtime.missing["git"] = true
			case "nongit", "nongit-task":
				if fault == "nongit-task" {
					task = taskFixture(t, root, "frontend")
				}
				runtime.command = func(call commandCall) watchdog.Result {
					if call.args[len(call.args)-1] == "--is-inside-work-tree" {
						return watchdog.Result{ExitCode: new(128), Stderr: "not a repository"}
					}
					return defaultCommand(t, root, call)
				}
			}
			report := Doctor(context.Background(), root, task, true)
			if fault == "nongit" {
				assert.True(t, report.OK, "%+v", report.Checks)
				assert.Equal(t, "warn", checkByID(t, report, "workspace.git").Status)
			} else {
				assert.False(t, report.OK)
				assert.Empty(t, runtime.probes)
			}
			if fault == "configuration" {
				assert.Nil(t, report.Configuration)
				assert.NotContains(t, checkByID(t, report, "configuration").Summary, "sk-secret")
			}
		})
	}
}

func TestCommandVersionCapabilitiesAreNotSatisfiedByExistenceCache(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{}`)
	ctx := context.Background()
	r := Report{OK: true, Cwd: root}
	cache := map[string]bool{}
	runtime.command = func(commandCall) watchdog.Result { return watchdog.Result{ExitCode: new(0), Stdout: "v20.19.0"} }
	assert.True(t, r.command(ctx, "worker", "same-binary", cache, false))
	assert.True(t, r.command(ctx, "tool", "same-binary", cache, true))
	assert.Contains(t, checkByID(t, r, "binary.tool").Summary, "20.19.0")
	assert.False(t, r.command(ctx, "node", "same-binary", cache, true), "Node requires 22 even if the binary passed another capability")
	assert.False(t, r.command(ctx, "node", "same-binary", cache, true), "failed capability remains cached")
	assert.Len(t, runtime.commands, 2)
	assert.Equal(t, "INVALID_REQUEST", checkByID(t, r, "binary.node").Code)
	assert.True(t, r.command(ctx, "tool", "same-binary", cache, true, "runtime.cjs"), "different invocation prefixes are checked independently")
	assert.Len(t, runtime.commands, 3)
	for _, result := range []watchdog.Result{
		{ExitCode: new(0), Stdout: "no version"},
		{ExitCode: new(1), Stdout: "1.0.0"},
		{ExitCode: new(0), Stdout: "1.0.0", CleanupError: "child still running"},
		{ExitCode: new(0), Stdout: "1.0.0", Truncated: true},
	} {
		runtime.command = func(commandCall) watchdog.Result { return result }
		assert.False(t, r.command(ctx, "tool", "broken", map[string]bool{}, true))
	}
}

func TestHarnessVersionChecksUseTheRealDriverExecutable(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{}`)
	r := Report{OK: true, Cwd: root}
	ctx := context.Background()
	t.Setenv(drivers.KimiCommandEnv, "/custom/kimi with spaces")
	r.harnessCommand(ctx, Binding{Harness: "kimi"}, map[string]bool{})
	assert.Equal(t, "/custom/kimi with spaces", runtime.commands[len(runtime.commands)-1].binary)
	t.Setenv(drivers.ZcodeServerBinEnv, "/custom/zcode-server")
	r.harnessCommand(ctx, Binding{Harness: "zcode"}, map[string]bool{})
	assert.Equal(t, "/custom/zcode-server", runtime.commands[len(runtime.commands)-1].binary)
	assert.Equal(t, []string{"--version"}, runtime.commands[len(runtime.commands)-1].args)
	t.Setenv(drivers.ZcodeServerBinEnv, "")
	r.harnessCommand(ctx, Binding{Harness: "zcode"}, map[string]bool{})
	last := runtime.commands[len(runtime.commands)-1]
	assert.Equal(t, "node", last.binary)
	assert.Equal(t, []string{"/Applications/ZCode.app/Contents/Resources/glm/zcode.cjs", "--version"}, last.args)
	runtime.missing["node"] = true
	before := len(runtime.commands)
	r.harnessCommand(ctx, Binding{Harness: "zcode"}, map[string]bool{})
	assert.Len(t, runtime.commands, before, "no runtime command when Node is missing")
}

func TestProbeDeduplicationIncludesEveryExecutionIdentityDimension(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{}`)
	r := Report{OK: true, Cwd: root}
	cfg := config.Default()
	cache := map[string]bool{}
	b := Binding{ID: "one", Provider: "adapter", Channel: "account", Model: "model", PermissionMode: "read-only"}
	r.probe(context.Background(), &cfg, b, cache)
	b.Thinking = "high"
	r.probe(context.Background(), &cfg, b, cache)
	assert.Len(t, runtime.probes, 1, "omitted thinking and the driver's high default describe the same probe")
	for _, alter := range []func(*Binding){
		func(b *Binding) { b.Channel = "account-alias" },
		func(b *Binding) { b.Provider = "another-adapter" },
		func(b *Binding) { b.Model = "another-model" },
		func(b *Binding) { b.Thinking = "low" },
		func(b *Binding) { b.PermissionMode = "workspace-write" },
	} {
		variant := b
		alter(&variant)
		r.probe(context.Background(), &cfg, variant, cache)
	}
	r.Cwd = t.TempDir()
	r.probe(context.Background(), &cfg, b, cache)
	assert.Len(t, runtime.probes, 7)
}

func TestProbeRequiresQuiescenceAndNoInferenceUsage(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{}`)
	cfg := config.Default()
	for _, tc := range []struct {
		name   string
		result drivers.Result
		code   string
	}{
		{"unsettled", drivers.Result{ExitCode: new(0)}, "INVALID_REQUEST"},
		{"timeout", drivers.Result{TimedOut: true}, "TIMEOUT"},
		{"cancelled", drivers.Result{Cancelled: true}, "TRANSPORT"},
		{"cleanup", drivers.Result{CleanupError: "private detail"}, "TRANSPORT"},
		{"signal", drivers.Result{Signal: "terminated"}, "TRANSPORT"},
		{"truncated", drivers.Result{Truncated: true}, "TRANSPORT"},
		{"spawn", drivers.Result{SpawnError: "private path"}, "TRANSPORT"},
		{"unexpected inference", drivers.Result{ExitCode: new(0), Settled: true, Usage: &drivers.Usage{}}, "INVALID_REQUEST"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime.probe = func(drivers.DSHArgs) drivers.Result { return tc.result }
			r := Report{OK: true, Cwd: root}
			r.probe(context.Background(), &cfg, Binding{ID: "binding"}, map[string]bool{})
			assert.False(t, r.OK)
			assert.Equal(t, tc.code, checkByID(t, r, "binding.sdk").Code)
		})
	}
}

func TestWholeDoctorDeadlineIncludesRootDiscoveryAndAllCommands(t *testing.T) {
	for _, stalledStage := range []string{"root", "version"} {
		t.Run(stalledStage, func(t *testing.T) {
			root, runtime := diagnosticFixture(t, dshConfiguration)
			old := doctorBudget
			doctorBudget = 20 * time.Millisecond
			t.Cleanup(func() { doctorBudget = old })
			runtime.command = func(call commandCall) watchdog.Result {
				last := call.args[len(call.args)-1]
				if (stalledStage == "root" && last == "--show-toplevel") || (stalledStage == "version" && last == "--version") {
					<-call.opts.Context.Done()
					return watchdog.Result{Cancelled: true}
				}
				return defaultCommand(t, root, call)
			}
			start := time.Now()
			report := Doctor(context.Background(), root, "", true)
			assert.Less(t, time.Since(start), time.Second)
			assert.False(t, report.OK)
			assert.Equal(t, "TIMEOUT", checkByID(t, report, "deadline").Code)
			assert.Empty(t, runtime.probes)
			for _, call := range runtime.commands {
				_, hasDeadline := call.opts.Context.Deadline()
				assert.True(t, hasDeadline)
				assert.Equal(t, int64(5000), call.opts.KillGraceMs)
			}
		})
	}
}

func TestCancelledDiagnosticsDoNotStartDiscoveryOrProbe(t *testing.T) {
	root, runtime := diagnosticFixture(t, dshConfiguration)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	view, err := InspectContext(ctx, root, "")
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, view)
	report := Doctor(ctx, root, "", true)
	assert.False(t, report.OK)
	assert.Equal(t, "TIMEOUT", checkByID(t, report, "deadline").Code)
	assert.Empty(t, runtime.commands)
	assert.Empty(t, runtime.probes)
}

func TestCancelledLocalCheckCannotStartAnotherCommandOrSDK(t *testing.T) {
	for _, lastBinary := range []string{"node", "dsh"} {
		t.Run(lastBinary, func(t *testing.T) {
			root, runtime := diagnosticFixture(t, dshConfiguration)
			t.Setenv("FLYWHEEL_DSH_BIN", "dsh")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runtime.command = func(call commandCall) watchdog.Result {
				if call.binary == lastBinary {
					cancel()
				}
				// An otherwise successful version may race with SIGTERM. It
				// must not authorize the next subprocess or SDK initialization.
				return defaultCommand(t, root, call)
			}
			report := Doctor(ctx, root, "", true)
			assert.False(t, report.OK)
			assert.Equal(t, "TIMEOUT", checkByID(t, report, "deadline").Code)
			assert.Empty(t, runtime.probes)
			assert.Equal(t, lastBinary, runtime.commands[len(runtime.commands)-1].binary)
		})
	}
}

func TestInspectContextWaitsForCancelledDiscoveryCleanup(t *testing.T) {
	root, runtime := diagnosticFixture(t, dshConfiguration)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cleanup := filepath.Join(t.TempDir(), "term-cleanup-finished")
	var result watchdog.Result
	runtime.command = func(call commandCall) watchdog.Result {
		assert.Equal(t, []string{"-C", root, "rev-parse", "--show-toplevel"}, call.args)
		call.opts.OnActivity = func(chunk, stream string) {
			if stream == "stdout" && strings.Contains(chunk, "ready") {
				cancel()
			}
		}
		result = watchdog.Run("sh", []string{"-c", `trap 'touch "$1"; exit 0' TERM
printf 'ready\n'
while :; do sleep 1; done`, "diagnostic-cleanup-fixture", cleanup}, call.opts)
		return result
	}
	view, err := InspectContext(ctx, root, "")
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, view)
	assert.True(t, result.Cancelled, "a successful trap exit must retain cancellation")
	assert.Empty(t, result.CleanupError)
	assert.FileExists(t, cleanup, "inspection must await the owned child's cleanup")
	assert.Len(t, runtime.commands, 1)
	assert.Empty(t, runtime.probes)
}

func TestDoctorCancellationWaitsForProbeAndStopsLaterBindings(t *testing.T) {
	root, runtime := diagnosticFixture(t, dshConfiguration)
	task := taskFixture(t, root, "frontend")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cleaned := false
	runtime.probe = func(args drivers.DSHArgs) drivers.Result {
		cancel()
		<-args.Watchdog.Context.Done()
		cleaned = true
		return drivers.Result{Cancelled: true, Settled: true, ExitCode: new(0)}
	}
	report := Doctor(ctx, root, task, true)
	assert.True(t, cleaned, "doctor must return only after its synchronous driver cleanup")
	assert.False(t, report.OK)
	assert.Equal(t, "TIMEOUT", checkByID(t, report, "deadline").Code)
	assert.Equal(t, "TRANSPORT", checkByID(t, report, "judge-panel.sdk").Code)
	assert.Len(t, runtime.probes, 1, "cancellation must prevent all later bindings from initializing")
	for _, b := range report.Configuration.Bindings {
		assert.Equal(t, b.ID != "routes.default" && b.Role != "intake" && b.Role != "garden", b.InScope,
			"cancellation cannot leave an unused binding marked in scope: %s", b.ID)
	}
}
