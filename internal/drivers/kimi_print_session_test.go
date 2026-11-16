package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/require"
)

func TestKimiPrintPrepareProcessFixture(t *testing.T) {
	if os.Getenv("FLYWHEEL_PRINT_PREPARE_FIXTURE") == "" {
		return
	}
	var request map[string]any
	if json.NewDecoder(os.Stdin).Decode(&request) != nil {
		os.Exit(2)
	}
	raw, _ := json.Marshal(request)
	_ = os.WriteFile(filepath.Join(request["cwd"].(string), "prepare-request.json"), raw, 0600)
	frame := map[string]any{"protocol": 1, "type": "prepared-print", "requestId": request["requestId"], "status": "ok",
		"sessionId": "session-one", "sessionTitle": request["sessionTitle"], "cleanupConfirmed": true, "inferenceDispatched": false, "error": nil}
	exit := 0
	switch request["sessionTitle"] {
	case "invalid":
		fmt.Print("{invalid}")
		os.Exit(1)
	case "cleanup":
		frame["cleanupConfirmed"] = false
	case "cancel":
		frame["status"], exit = "cancelled", 130
	case "timeout":
		frame["status"], frame["error"], exit = "error", map[string]string{"code": "TIMEOUT", "message": "elapsed"}, 1
	case "wrong-title":
		frame["sessionTitle"] = "different"
	case "wrong-id":
		frame["requestId"] = "different"
	case "invalid-session-id":
		frame["sessionId"] = "bad\nidentity"
	}
	_ = json.NewEncoder(os.Stdout).Encode(frame)
	os.Exit(exit)
}

func prepareFixtureCommand(t *testing.T) string {
	t.Helper()
	return kimiScript(t, "FLYWHEEL_PRINT_PREPARE_FIXTURE=1 exec "+shQuote(os.Args[0])+" -test.run '^TestKimiPrintPrepareProcessFixture$'\n")
}

func TestKimiPrintUsesNamedPreparedHistoryAndPreservesResumeTitle(t *testing.T) {
	t.Setenv(KimiNativeWorkerEnv, prepareFixtureCommand(t))
	t.Setenv("KIMI_CODE_HOME", "/fixture/account")
	t.Setenv("FLYWHEEL_RESOURCE_DIR", "/fixture/resources")
	cwd := t.TempDir()
	command := kimiScript(t, `printf '%s\n' "$@" > print-arguments
printf '%s' "$KIMI_CODE_HOME" > print-home
printf '%s\n' '{"role":"assistant","content":"done"}' '`+kimiComplete+`'
`)
	t.Setenv(KimiCommandEnv, command)
	title := "Flywheel · design-plan · 排查告警失败"
	args := CallArgs{Cwd: cwd, SessionTitle: title, Prompt: "TASK_PROMPT_NOT_IN_PREPARATION", RequestID: "task-call",
		PermissionMode: "danger-full-access", Binding: config.LoopBinding{Harness: "kimi"}, Watchdog: watchdog.Opts{WallTimeoutMs: 10_000}}
	out := InvokeHarness(args, "")
	require.True(t, out.Settled, "%+v", out)
	require.False(t, out.Session.ResumeAccepted, "preparing a fresh title must not pretend this role resumed an earlier invocation")
	require.Equal(t, "session-one", out.Session.SessionID)
	raw, err := os.ReadFile(filepath.Join(cwd, "prepare-request.json"))
	require.NoError(t, err)
	require.NotContains(t, string(raw), args.Prompt)
	var prepared map[string]any
	require.NoError(t, json.Unmarshal(raw, &prepared))
	require.Equal(t, title, prepared["sessionTitle"])
	require.Equal(t, "task-call", prepared["requestId"])
	argv, err := os.ReadFile(filepath.Join(cwd, "print-arguments"))
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(argv), "--session\nsession-one\n"))
	require.Contains(t, string(argv), args.Prompt)
	home, err := os.ReadFile(filepath.Join(cwd, "print-home"))
	require.NoError(t, err)
	require.Equal(t, "/fixture/account", string(home))
	require.NoError(t, os.Remove(filepath.Join(cwd, "prepare-request.json")))
	args.SessionID, args.SessionTitle = "session-one", "do not rename the user's session"
	out = InvokeHarness(args, "")
	require.True(t, out.Settled)
	require.True(t, out.Session.ResumeAccepted)
	require.NoFileExists(t, filepath.Join(cwd, "prepare-request.json"))
}

func TestKimiPrintPreparationFailureNeverDispatchesTheTask(t *testing.T) {
	t.Setenv(KimiNativeWorkerEnv, prepareFixtureCommand(t))
	for _, title := range []string{"invalid", "cleanup", "cancel", "timeout", "wrong-title", "wrong-id", "invalid-session-id"} {
		t.Run(title, func(t *testing.T) {
			a := KimiArgs{Cwd: t.TempDir(), Command: "/must-not-launch-task", SessionTitle: title, Prompt: "task", RequestID: "invocation"}
			out := RunKimi(a)
			require.False(t, out.Settled)
			require.Equal(t, "not-dispatched", out.Session.Dispatch)
			require.NotNil(t, out.ErrorMessage)
			if title == "invalid-session-id" {
				require.Empty(t, out.Session.SessionID)
			}
			if title == "timeout" {
				require.True(t, out.TimedOut)
				require.False(t, out.Cancelled)
				require.Equal(t, errs.TIMEOUT, out.ErrorCode)
			}
		})
	}
	// Direct callers can request a readable title without a ledger request ID.
	defaults := prepareKimiPrintSession(KimiArgs{Cwd: t.TempDir(), SessionTitle: "readable"})
	require.True(t, defaults.Settled, "%+v", defaults)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := RunKimi(KimiArgs{Cwd: t.TempDir(), SessionTitle: "cancelled before startup", Watchdog: watchdog.Opts{Context: ctx}})
	require.True(t, out.Cancelled)
	require.True(t, out.Session.CleanupConfirmed)
	t.Setenv(KimiNativeWorkerEnv, "/does-not-exist")
	out = RunKimi(KimiArgs{Cwd: t.TempDir(), SessionTitle: "missing worker"})
	require.NotEmpty(t, out.SpawnError)
	require.True(t, out.Session.CleanupConfirmed)
}

func TestKimiPrintPreparationResolvesRelativeWorkspaceBeforeChangingDirectory(t *testing.T) {
	t.Setenv(KimiNativeWorkerEnv, prepareFixtureCommand(t))
	t.Chdir(t.TempDir())
	require.NoError(t, os.Mkdir("project", 0700))
	result := prepareKimiPrintSession(KimiArgs{Cwd: "project", SessionTitle: "Readable project"})
	require.True(t, result.Settled, "%+v", result)
	raw, err := os.ReadFile("project/prepare-request.json")
	require.NoError(t, err)
	var request map[string]any
	require.NoError(t, json.Unmarshal(raw, &request))
	require.True(t, filepath.IsAbs(request["cwd"].(string)))
	// An unresolvable caller directory is rejected before either owned process starts.
	require.NoError(t, os.Mkdir("removed-cwd", 0700))
	require.NoError(t, os.Chdir("removed-cwd"))
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Remove(cwd))
	result = prepareKimiPrintSession(KimiArgs{Cwd: ".", SessionTitle: "Unreadable workspace"})
	require.False(t, result.Settled)
	require.Equal(t, "not-dispatched", result.Session.Dispatch)
	require.True(t, result.Session.CleanupConfirmed)
}

func TestKimiPrintPreparationAndTaskShareOneWallBudget(t *testing.T) {
	original := kimiPrintSessionPreparer
	t.Cleanup(func() { kimiPrintSessionPreparer = original })
	for _, wall := range []int64{1, 0} {
		kimiPrintSessionPreparer = func(a KimiArgs) Result {
			return Result{Settled: true, SessionID: "prepared", DurationMS: watchdog.DefaultWallTimeoutMs + 1,
				Session: &SessionOutcome{Dispatch: "not-dispatched", Outcome: "failed", CleanupConfirmed: true}}
		}
		out := RunKimi(KimiArgs{SessionTitle: "bounded preparation", Command: "/must-not-spawn", Watchdog: watchdog.Opts{WallTimeoutMs: wall}})
		require.False(t, out.Settled)
		require.True(t, out.WallTimedOut)
		require.Equal(t, errs.TIMEOUT, out.ErrorCode)
		require.True(t, out.Session.CleanupConfirmed)
	}
}
