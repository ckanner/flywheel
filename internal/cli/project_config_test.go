package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCommandsDiscoverProjectConfigFromSubdirectory(t *testing.T) {
	for _, args := range [][]string{{"run"}, {"run", "--only", "verify"}, {"intake"}, {"garden"}} {
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			taskDir := newCLISandbox(t)
			root := filepath.Dir(filepath.Dir(taskDir))
			if args[0] == "intake" {
				raw, err := os.ReadFile(filepath.Join(taskDir, "flywheel.config.json"))
				require.NoError(t, err)
				var cfg map[string]any
				require.NoError(t, json.Unmarshal(raw, &cfg))
				cfg["intake"] = map[string]any{"sources": []any{map[string]any{
					"name": "project", "type": "command", "cmd": "test -f flywheel.config.json && printf '%s\\n' project-candidate",
				}}}
				writeFileCli(t, filepath.Join(taskDir, "flywheel.config.json"), mustJSONT(t, cfg))
			}
			require.NoError(t, os.Rename(filepath.Join(taskDir, "flywheel.config.json"), filepath.Join(root, "flywheel.config.json")))
			gitCliCommitAll(t, root, "test: use project-level configuration")
			global := filepath.Join(t.TempDir(), "global.json")
			writeFileCli(t, global, "{invalid-global")
			t.Setenv("FLYWHEEL_CONFIG", global)
			chdirT(t, taskDir)

			command := append([]string{args[0], "."}, args[1:]...)
			code, out := runCLI(t, command...)
			require.Equal(t, 0, code, out)
			require.NotContains(t, out, "invalid JSON")
		})
	}
}

func TestProjectConfigErrorsAndTaskPrecedenceFromSubdirectory(t *testing.T) {
	taskDir := newCLISandbox(t)
	root := filepath.Dir(filepath.Dir(taskDir))
	project := filepath.Join(root, "flywheel.config.json")
	writeFileCli(t, project, "{invalid-project")
	gitCliCommitAll(t, root, "test: keep task override above invalid project config")
	chdirT(t, taskDir)

	code, out := runCLI(t, "run", ".", "--only", "verify")
	require.Equal(t, 0, code, out)
	require.NoError(t, os.Remove(filepath.Join(taskDir, "flywheel.config.json")))
	for _, command := range []string{"run", "intake", "garden"} {
		code, out = runCLI(t, command, ".")
		require.Equal(t, 1, code, out)
		require.Contains(t, out, "invalid JSON")
		// Git normalizes macOS /var and /tmp aliases; match the canonical path.
		canonical, err := filepath.EvalSymlinks(project)
		require.NoError(t, err)
		require.Contains(t, out, canonical)
	}
}

func TestStatusUsesProjectWatchdogFromSubdirectory(t *testing.T) {
	taskDir := newCLISandbox(t)
	root := filepath.Dir(filepath.Dir(taskDir))
	require.NoError(t, os.Remove(filepath.Join(taskDir, "flywheel.config.json")))
	writeFileCli(t, filepath.Join(root, "flywheel.config.json"), `{"watchdog":{"wall-timeout-ms":1}}`)
	recent := time.Now().UTC().Add(-time.Second).Format("2006-01-02T15:04:05.000Z")
	writeFileCli(t, filepath.Join(taskDir, "state.json"), strings.Replace(cliStateFixture, "2026-08-22T00:00:00.000Z", recent, 1))
	chdirT(t, taskDir)

	code, out := runCLI(t, "status", ".")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "疑似停滞/断链")
}
