package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/diagnostic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func diagnosticCLIFixture(t *testing.T) (string, string) {
	t.Helper()
	t.Setenv("FLYWHEEL_CONFIG", filepath.Join(t.TempDir(), "not-created", "flywheel.config.json"))
	taskDir := newCLISandbox(t)
	root := filepath.Dir(filepath.Dir(taskDir))
	raw, err := os.ReadFile(filepath.Join(taskDir, "flywheel.config.json"))
	require.NoError(t, err)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(raw, &cfg))
	cfg["profiles"] = map[string]any{"flash": map[string]any{
		"harness": "dsh", "provider": "deepseek", "channel": "paid-account", "model": "deepseek-v4-flash", "thinking": "high",
	}}
	cfg["loops"].(map[string]any)["plan"] = map[string]any{"profile": "flash", "resident-session": false}
	writeFileCli(t, filepath.Join(taskDir, "flywheel.config.json"), mustJSONT(t, cfg))
	return root, taskDir
}

func TestConfigInspectCommandResolvesProfilesAndDoesNotWrite(t *testing.T) {
	root, taskDir := diagnosticCLIFixture(t)
	before, err := os.ReadDir(taskDir)
	require.NoError(t, err)
	var stdout, stderr bytes.Buffer
	code := runRoot([]string{"config", "inspect", "--cwd", root, "--task-dir", "docs/clitask", "--json"}, &stdout, &stderr)
	require.Zero(t, code, stderr.String())
	assert.Empty(t, stderr.String())
	var view diagnostic.Configuration
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &view), stdout.String())
	assert.Equal(t, "task", view.Scope)
	assert.Equal(t, taskDir, view.TaskDir)
	var plan diagnostic.Binding
	for _, b := range view.Bindings {
		if b.ID == "loops.plan" {
			plan = b
		}
	}
	assert.Equal(t, "flash", plan.Profile)
	assert.Equal(t, "paid-account", plan.Channel)
	assert.Equal(t, "deepseek", plan.Provider)
	assert.Equal(t, "deepseek-v4-flash", plan.Model)
	assert.False(t, plan.ResidentSession)
	code, human := runCLI(t, "config", "inspect", "--cwd", root, "--task-dir", taskDir)
	require.Zero(t, code, human)
	assert.Contains(t, human, "配置来源:")
	assert.Contains(t, human, "loops.plan")
	assert.Contains(t, human, "paid-account")
	assert.Contains(t, human, "账号授权、额度与模型服务能力未验证")
	after, err := os.ReadDir(taskDir)
	require.NoError(t, err)
	assert.Equal(t, before, after, "inspect may not create state, logs, locks, or worktrees")
	assert.NoFileExists(t, os.Getenv("FLYWHEEL_CONFIG"))
	code, failed := runCLI(t, "config", "inspect", "--cwd", filepath.Join(root, "missing"))
	assert.Equal(t, 1, code)
	assert.Contains(t, failed, "resolve working directory")
}

func TestDoctorCLICheckedFailureStillHasOneJSONDocument(t *testing.T) {
	root, taskDir := diagnosticCLIFixture(t)
	for _, asJSON := range []bool{true, false} {
		t.Run(map[bool]string{true: "json", false: "human"}[asJSON], func(t *testing.T) {
			args := []string{"doctor", "--cwd", root, "--task-dir", taskDir, "--probe"}
			if asJSON {
				args = append(args, "--json")
			}
			var stdout, stderr bytes.Buffer
			code := runRoot(args, &stdout, &stderr)
			require.Zero(t, code, stdout.String()+stderr.String())
			assert.Empty(t, stderr.String())
			if asJSON {
				var report diagnostic.Report
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &report))
				assert.True(t, report.OK)
				assert.True(t, report.ProbeRequested)
				assert.False(t, report.InferenceTested)
				assert.Equal(t, "doctor", report.Kind)
				assert.Equal(t, taskDir, report.TaskDir)
			} else {
				assert.Contains(t, stdout.String(), "未发现阻断项")
				assert.Contains(t, stdout.String(), "未发送推理请求")
			}
		})
	}
	writeFileCli(t, filepath.Join(taskDir, "spec.md"), "not a spec")
	for _, asJSON := range []bool{true, false} {
		args := []string{"doctor", "--cwd", root, "--task-dir", taskDir}
		if asJSON {
			args = append(args, "--json")
		}
		var stdout, stderr bytes.Buffer
		code := runRoot(args, &stdout, &stderr)
		assert.Equal(t, 1, code)
		assert.Empty(t, stderr.String(), "diagnostic failure is already explained in the report")
		if asJSON {
			var report diagnostic.Report
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &report))
			assert.False(t, report.OK)
			assert.False(t, report.InferenceTested)
		} else {
			assert.Contains(t, stdout.String(), "存在阻断项")
			assert.Contains(t, stdout.String(), "建议:")
		}
	}
	assert.NoFileExists(t, filepath.Join(taskDir, "state.json"))
	assert.NoFileExists(t, filepath.Join(taskDir, "orchestrator.log"))
}

func TestControlConfigurationAndDoctorShareCLIContracts(t *testing.T) {
	root, taskDir := diagnosticCLIFixture(t)
	// The task-less control actions inspect the project; task controls still need
	// an absolute task path. Neither action starts an engine or creates a lease.
	raw, err := os.ReadFile(filepath.Join(taskDir, "flywheel.config.json"))
	require.NoError(t, err)
	writeFileCli(t, filepath.Join(root, "flywheel.config.json"), string(raw))
	for _, task := range []string{"", taskDir} {
		code, reply := runControlTest(t, "config", task, root)
		require.Zero(t, code, "%+v", reply)
		require.NotNil(t, reply.Configuration)
		assert.Equal(t, task, reply.TaskDir)
		assert.Equal(t, task, reply.Configuration.TaskDir)
		assert.False(t, reply.Running)
		assert.Nil(t, reply.State)
		code, reply = runControlTest(t, "doctor", task, root, "--probe")
		require.Zero(t, code, "%+v", reply)
		require.NotNil(t, reply.Diagnostic)
		assert.True(t, reply.Diagnostic.OK)
		assert.True(t, reply.Diagnostic.ProbeRequested)
		assert.False(t, reply.Diagnostic.InferenceTested)
		assert.Equal(t, task, reply.Diagnostic.TaskDir)
	}
	code, reply := runControlTest(t, "status", "", root)
	assert.Equal(t, 1, code)
	assert.Contains(t, reply.Error, "absolute")
	for _, action := range []string{"config", "status"} {
		code, reply = runControlTest(t, action, taskDir, root, "--probe=false")
		assert.Equal(t, 1, code)
		assert.Contains(t, reply.Error, "only valid")
	}
	writeFileCli(t, filepath.Join(taskDir, "flywheel.config.json"), `{ "secret-mistake": "do-not-echo-value" }`)
	code, reply = runControlTest(t, "doctor", taskDir, root)
	assert.Equal(t, 1, code)
	assert.False(t, reply.OK)
	require.NotNil(t, reply.Diagnostic, "checked failures must remain reviewable in the panel")
	assert.False(t, reply.Diagnostic.OK)
	assert.Nil(t, reply.Diagnostic.Configuration)
	assert.NotContains(t, mustJSONT(t, reply), "do-not-echo-value")
	code, reply = runControlTest(t, "config", taskDir, root)
	assert.Equal(t, 1, code)
	assert.False(t, reply.OK)
	assert.Contains(t, reply.Error, "effective configuration")
	assert.NotContains(t, reply.Error, "do-not-echo-value")
	assert.NoFileExists(t, filepath.Join(taskDir, ".run.lock"))
}

func TestDiagnosticCLIHandlesSignalsBeforeReturning(t *testing.T) {
	for _, command := range [][]string{{"doctor", "--json"}, {"control", "doctor"}, {"config", "inspect", "--json"}, {"control", "config"}} {
		t.Run(strings.Join(command, "_"), func(t *testing.T) {
			root, taskDir := diagnosticCLIFixture(t)
			bin, markers := t.TempDir(), t.TempDir()
			pidPath, cleaned := filepath.Join(markers, "pid"), filepath.Join(markers, "cleaned")
			quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
			// A local Git-discovery subprocess takes longer than the old 500ms
			// host kill grace to clean up. No model or SDK process is involved.
			script := "#!/bin/sh\ntrap \"sleep 0.7; printf done > " + quote(cleaned) + "; exit 0\" TERM INT\nprintf '%s' $$ > " + quote(pidPath) + "\nwhile :; do sleep 0.01; done\n"
			require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755))
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			args := append(append([]string{}, command...), "--cwd", root, "--task-dir", taskDir)
			child := exec.CommandContext(ctx, os.Args[0], args...)
			child.Env = append(os.Environ(), "FLYWHEEL_CLI_EXEC_HELPER=diagnostic", "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			var out, stderr bytes.Buffer
			child.Stdout, child.Stderr = &out, &stderr
			require.NoError(t, child.Start())
			defer child.Process.Kill()
			var ownedPID int
			t.Cleanup(func() {
				if ownedPID > 0 {
					_ = syscall.Kill(-ownedPID, syscall.SIGKILL)
				}
			})
			require.Eventually(t, func() bool {
				raw, err := os.ReadFile(pidPath)
				if err != nil {
					return false
				}
				ownedPID, err = strconv.Atoi(string(raw))
				return err == nil && ownedPID > 0
			}, 3*time.Second, 10*time.Millisecond)
			require.NoError(t, child.Process.Signal(syscall.SIGTERM))
			err := child.Wait()
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit, out.String()+stderr.String())
			assert.Equal(t, 1, exit.ExitCode(), "handled termination produces a failed diagnostic, not abrupt signal death")
			assert.FileExists(t, cleaned, "CLI return must follow managed process cleanup")
			assert.Equal(t, syscall.ESRCH, syscall.Kill(ownedPID, 0), "the owned command must be gone")
			ownedPID = 0 // do not retain a reaped process identity for test cleanup
			if command[0] == "control" {
				var reply controlResponse
				require.NoError(t, json.Unmarshal(out.Bytes(), &reply), out.String())
				assert.False(t, reply.OK)
				assert.Empty(t, stderr.String())
			} else if command[0] == "doctor" {
				var report diagnostic.Report
				require.NoError(t, json.Unmarshal(out.Bytes(), &report), out.String())
				assert.False(t, report.OK)
				assert.False(t, report.InferenceTested)
			}
			assert.NoFileExists(t, filepath.Join(taskDir, "state.json"))
		})
	}
}

func TestStatusDoesNotMaterializeGlobalConfiguration(t *testing.T) {
	taskDir := t.TempDir()
	chdirT(t, taskDir)
	global := filepath.Join(t.TempDir(), "not-created", "flywheel.config.json")
	t.Setenv("FLYWHEEL_CONFIG", global)
	writeFileCli(t, filepath.Join(taskDir, "state.json"), cliStateFixture)
	before := mustReadFile(filepath.Join(taskDir, "state.json"))
	code, output := runCLI(t, "status", taskDir)
	require.Zero(t, code, output)
	assert.NoFileExists(t, global, "reading progress must not materialize global configuration")
	assert.NoDirExists(t, filepath.Dir(global))
	assert.Equal(t, before, mustReadFile(filepath.Join(taskDir, "state.json")))
}
