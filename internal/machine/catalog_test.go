package machine

import (
	"errors"
	"fmt"
	"github.com/ckanner/flywheel/internal/childenv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/metrics"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestRunCatalogKeepsConcurrentTasksAndFrozenConditions(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "runs")
	fx, c := sessionFixture(t)
	finish, err := c.beginRunRecord(directory)
	require.NoError(t, err)
	originalID := c.runID
	_, other := sessionFixture(t)
	second, err := other.beginRunRecord(directory)
	require.NoError(t, err)
	require.NotEqual(t, originalID, other.runID)
	records, err := ListRuns(directory)
	require.NoError(t, err)
	require.Len(t, records, 2)
	for _, record := range records {
		require.Empty(t, record.FinishedAt)
		require.NotEmpty(t, record.Conditions)
	}
	c.State.Status = "paused"
	var outcome error
	finish(&outcome)
	require.NoError(t, outcome)
	outcome = errors.New("explicit failure")
	second(&outcome)
	require.ErrorContains(t, outcome, "explicit failure")
	records, err = ListRuns(directory)
	require.NoError(t, err)
	for _, record := range records {
		require.NotEmpty(t, record.FinishedAt)
		if record.ID == originalID {
			require.Equal(t, fx.taskDir, record.TaskDir)
			require.Equal(t, "paused", record.Summary.Status)
		}
	}
	noop, err := c.beginRunRecord("")
	require.NoError(t, err)
	noop(&outcome)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0600))
	_, err = c.beginRunRecord(filepath.Join(file, "child"))
	require.ErrorContains(t, err, "register run")
	old := catalogRename
	t.Cleanup(func() { catalogRename = old })
	catalogRename = func(string, string) error { return errors.New("rename failure") }
	outcome = nil
	finish(&outcome)
	require.ErrorContains(t, outcome, "rename failure")
}

func TestRunIndexPathsAndMalformedRecords(t *testing.T) {
	t.Setenv("FLYWHEEL_RUNS_DIR", t.TempDir())
	dir, err := RunIndexDirectory()
	require.NoError(t, err)
	require.Equal(t, os.Getenv("FLYWHEEL_RUNS_DIR"), dir)
	t.Setenv("FLYWHEEL_RUNS_DIR", "relative")
	_, err = RunIndexDirectory()
	require.Error(t, err)
	t.Setenv("FLYWHEEL_RUNS_DIR", "")
	t.Setenv(config.GlobalConfigEnv, filepath.Join(t.TempDir(), "config.json"))
	dir, err = RunIndexDirectory()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(filepath.Dir(os.Getenv(config.GlobalConfigEnv)), "runs"), dir)
	t.Setenv(config.GlobalConfigEnv, "")
	t.Setenv("HOME", "")
	_, err = RunIndexDirectory()
	require.Error(t, err)
	list, err := ListRuns(filepath.Join(t.TempDir(), "missing"))
	require.NoError(t, err)
	require.Empty(t, list)
	for _, scenario := range []string{"invalid-json", "invalid-id", "symlink", "directory", "read-failure", "scan-failure", "too-many"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "one.json")
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".run-incomplete"), nil, 0600))
			oldRead, oldEntries := catalogRead, catalogEntries
			t.Cleanup(func() { catalogRead, catalogEntries = oldRead, oldEntries })
			switch scenario {
			case "invalid-json":
				require.NoError(t, os.WriteFile(file, []byte("{"), 0600))
			case "invalid-id":
				require.NoError(t, os.WriteFile(file, []byte("{}"), 0600))
			case "symlink":
				require.NoError(t, os.Symlink("missing", file))
			case "directory":
				require.NoError(t, os.Mkdir(file, 0700))
			case "read-failure":
				require.NoError(t, os.WriteFile(file, nil, 0600))
				catalogRead = func(string) ([]byte, error) { return nil, errors.New("read failure") }
			case "scan-failure":
				catalogEntries = func(string) ([]os.DirEntry, error) { return nil, errors.New("scan failure") }
			case "too-many":
				catalogEntries = func(string) ([]os.DirEntry, error) { return make([]os.DirEntry, 10001), nil }
			}
			_, err := ListRuns(dir)
			require.Error(t, err)
		})
	}
}

func TestRunCatalogIOFailuresNeverConfirmARegistration(t *testing.T) {
	for _, scenario := range []string{"lstat", "symlink", "temp", "write", "sync"} {
		t.Run(scenario, func(t *testing.T) {
			oldStat, oldTemp, oldWrite, oldSync := catalogLstat, catalogTemp, catalogWrite, catalogSync
			t.Cleanup(func() { catalogLstat, catalogTemp, catalogWrite, catalogSync = oldStat, oldTemp, oldWrite, oldSync })
			dir := filepath.Join(t.TempDir(), "runs")
			switch scenario {
			case "lstat":
				catalogLstat = func(string) (os.FileInfo, error) { return nil, errors.New("stat failure") }
			case "symlink":
				require.NoError(t, os.Symlink(t.TempDir(), dir))
			case "temp":
				catalogTemp = func(string, string) (*os.File, error) { return nil, errors.New("temp failure") }
			case "write":
				catalogWrite = func(*os.File, []byte) (int, error) { return 0, errors.New("write failure") }
			case "sync":
				catalogSync = func(*os.File) error { return errors.New("sync failure") }
			}
			require.Error(t, writeRunRecord(dir, &RunRecord{ID: "one"}))
		})
	}
}

func TestPublicRunRegistrationFailurePreventsWorkAndClearsPID(t *testing.T) {
	for _, single := range []bool{false, true} {
		fx := newGitFixture(t)
		opts := runOpts(fx, happyDrivers().set()).RunOpts
		blocker := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(blocker, nil, 0600))
		opts.IndexDir = filepath.Join(blocker, "runs")
		var err error
		if single {
			_, err = RunSingleStep(RunSingleStepOpts{RunOpts: opts, Only: "execute", PacketID: "M01-T01"})
		} else {
			_, err = RunTask(RunTaskOpts{RunOpts: opts})
		}
		require.ErrorContains(t, err, "register run")
		_, err = os.Stat(PidFile(fx.taskDir))
		require.True(t, os.IsNotExist(err))
		st, err := state.Load(fx.taskDir)
		require.NoError(t, err)
		require.Zero(t, st.Packets["M01-T01"].Attempts)
	}
}

func TestRunQualityKeepsFailuresWhenPlanResetsTheRetryThreshold(t *testing.T) {
	_, c := sessionFixture(t)
	before := map[string]state.PacketState{"M01-T01": {Attempts: 1, FailedAttempts: 3, RetryHistory: []any{map[string]any{"scope": "attempt", "policy": "requeue"}}}}
	packet := c.State.Packets["M01-T01"]
	packet.Attempts, packet.FailedAttempts, packet.Status = 2, 0, "completed"
	packet.RetryHistory = []any{map[string]any{"scope": "attempt", "policy": "requeue"}, map[string]any{"scope": "attempt", "policy": "requeue"}, map[string]any{"scope": "transport"}, map[string]any{"policy": "human-continue-once"}}
	second := c.State.Packets["M01-T02"]
	second.Attempts, second.Status = 1, "completed"
	quality := c.runQuality(before)
	require.Equal(t, 2, quality.NewCompletions)
	require.Equal(t, 2, quality.NewAttempts)
	require.Equal(t, 1, quality.NewFailedAttempts)
	require.Equal(t, 1, quality.FirstAttemptCompletions)
}

func TestRunRegistrationPinsRepositoryAndJudgePanelBinding(t *testing.T) {
	fx, c := sessionFixture(t)
	gitCmd(t, fx.root, "remote", "add", "origin", "git@github.com:example/repo.git")
	panel := c.Config.Loops["review"]
	c.Config.JudgePanel = &panel
	finish, err := c.beginRunRecord(t.TempDir(), "workflow")
	require.NoError(t, err)
	require.Equal(t, "github.com/example/repo", c.State.Project.Repo)
	require.Contains(t, c.State.Run.Bindings, "judge-panel")
	require.Equal(t, 2, c.State.Sessions.Version)
	var runErr error
	finish(&runErr)
	require.NoError(t, runErr)
	c.revision--
	_, err = c.beginRunRecord(t.TempDir())
	require.Error(t, err)
}

func TestRecoveredTelemetryKeepsItsOriginalEngineRun(t *testing.T) {
	fx, c := handoffFixture(t)
	c.runID = "original"
	r := c.State.Sessions.Records["execute/M01-T01"]
	args, err := c.handoffArgs(r)
	require.NoError(t, err)
	result, err := c.invokeScoped(r.Role, r.Scope, args, func(a drivers.CallArgs) drivers.Result { return consoleInvoke(a, a.Model) })
	require.NoError(t, err)
	require.Equal(t, "original", c.State.Sessions.Records["execute/M01-T01"].Last.EngineRunID)
	c.runID = "recovery"
	require.NoError(t, c.reconcileSessionReceipts())
	report, err := metrics.ReadReport(filepath.Join(fx.taskDir, "metrics.jsonl"), "original")
	require.NoError(t, err)
	require.Equal(t, 1, report.TotalCalls)
	require.Equal(t, result.Session.RequestID, report.Calls[0].Invocation)
	report, err = metrics.ReadReport(filepath.Join(fx.taskDir, "metrics.jsonl"), "recovery")
	require.NoError(t, err)
	require.Empty(t, report.Calls)
}

func TestRunCatalogBindsOnlyBoundedHostSessionDiscoveryIdentity(t *testing.T) {
	for _, id := range []string{"session-desktop-123", "fork-session-456", "", "../other", "bad\nvalue", strings.Repeat("a", 201)} {
		t.Run(fmt.Sprintf("length-%d-%x", len(id), []byte(id)[:min(len(id), 8)]), func(t *testing.T) {
			_, c := sessionFixture(t)
			previous := c.EnvLookup
			c.EnvLookup = func(key string) (string, bool) {
				if key == "DSH_SESSION_ID" {
					return id, true
				}
				return previous(key)
			}
			directory := t.TempDir()
			finish, err := c.beginRunRecord(directory, "review")
			require.NoError(t, err)
			var outcome error
			finish(&outcome)
			require.NoError(t, outcome)
			rows, err := ListRuns(directory)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			if strings.HasPrefix(id, "session-") || strings.HasPrefix(id, "fork-") {
				require.Equal(t, id, rows[0].HostSessionID)
			} else {
				require.Empty(t, rows[0].HostSessionID)
			}
			require.Equal(t, "review", rows[0].Kind)
			// Discovery identity is never forwarded to stage tools or acceptance code.
			require.NotContains(t, childenv.BuildChildEnv(c.EnvLookup, nil), "DSH_SESSION_ID")
			require.NotContains(t, childenv.GateEnv(c.EnvLookup, nil), "DSH_SESSION_ID")
		})
	}
}
