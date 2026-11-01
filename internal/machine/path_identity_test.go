package machine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextCanonicalizesRepositoryAndTaskDirectoryAliases(t *testing.T) {
	for _, gitRepo := range []bool{false, true} {
		t.Run(fmt.Sprint(gitRepo), func(t *testing.T) {
			fx := newFixture(t)
			if gitRepo {
				gitCmd(t, fx.root, "init", "-q")
				gitCmd(t, fx.root, "add", "-A")
				gitCmd(t, fx.root, "commit", "-q", "-m", "initial contracts")
			}
			physicalRoot, err := filepath.EvalSymlinks(fx.root)
			require.NoError(t, err)
			physicalTask := filepath.Join(physicalRoot, "docs", fx.slug)
			alias := filepath.Join(t.TempDir(), "repository-alias")
			require.NoError(t, os.Symlink(physicalRoot, alias))
			aliasTask := filepath.Join(alias, "docs", fx.slug)
			for _, paths := range [][2]string{{alias, aliasTask}, {physicalRoot, aliasTask}, {alias, physicalTask}} {
				c, err := CreateContext(Opts{Cwd: paths[0], TaskDir: paths[1], Config: mustConfig(t, fx)})
				require.NoError(t, err)
				assert.Equal(t, physicalRoot, c.Cwd)
				assert.Equal(t, physicalTask, c.TaskDir)
				assert.Equal(t, "docs/minitask/", c.taskDirPrefix())
				assert.True(t, c.snapshotExcluded("docs/minitask/metrics.jsonl"))
				assert.True(t, c.isPlannerContract("docs/minitask/spec.md"))
				before, err := c.planScopeIdentity()
				require.NoError(t, err)
				writeFile(t, filepath.Join(fx.taskDir, "metrics.jsonl"), "runtime metrics changed\n")
				writeFile(t, filepath.Join(fx.taskDir, "retro.md"), "runtime notes changed\n")
				after, err := c.planScopeIdentity()
				require.NoError(t, err)
				assert.Equal(t, before, after, "directory aliases must not turn ledger writes into implementation changes")
				if gitRepo {
					_, err = verify.PersistedHead(c.Cwd, c.ledgerExcludes())
					require.NoError(t, err, "ledger exclusions must remain inside the repository")
				}
			}
		})
	}
}

func TestRunTaskThroughDirectoryAliasPausesAndResumesByPhysicalPath(t *testing.T) {
	for _, worktree := range []bool{false, true} {
		t.Run(fmt.Sprint(worktree), func(t *testing.T) {
			fx := newGitFixture(t)
			physicalRoot, err := filepath.EvalSymlinks(fx.root)
			require.NoError(t, err)
			alias := filepath.Join(t.TempDir(), "repository-alias")
			require.NoError(t, os.Symlink(physicalRoot, alias))
			sd := happyDrivers()
			opts := runOpts(fx, sd.set())
			opts.Config.Worktree.Enabled = &worktree
			opts.Cwd = alias
			opts.TaskDir = filepath.Join(alias, "docs", fx.slug)
			opts.PauseBefore = "review"
			summary, err := RunTask(opts)
			require.NoError(t, err)
			require.Equal(t, "paused", summary.Status)
			require.Len(t, sd.log.exec, 1)
			for _, call := range sd.log.exec {
				rel, err := filepath.Rel(call.Cwd, call.TaskDir)
				require.NoError(t, err)
				assert.Equal(t, "docs/minitask", filepath.ToSlash(rel))
				assert.False(t, strings.HasPrefix(rel, ".."), "reports stay inside the execution workspace")
			}
			opts.Cwd = physicalRoot
			opts.TaskDir = filepath.Join(physicalRoot, "docs", fx.slug)
			opts.PauseBefore = ""
			summary, err = RunTask(opts)
			require.NoError(t, err)
			assert.Equal(t, "completed", summary.Status)
			assert.Len(t, sd.log.exec, 2, "resuming through another alias must reuse the verified first candidate")
			assert.Equal(t, 1, loadFixtureState(t, fx).Packets["M01-T01"].Attempts)
			assert.Contains(t, gitCmd(t, physicalRoot, "show", "HEAD:build/M01-T01.txt"), "M01-T01")
		})
	}
}

func TestContextRejectsUnresolvableDirectories(t *testing.T) {
	fx := newFixture(t)
	_, err := CreateContext(Opts{Cwd: fx.root, TaskDir: filepath.Join(fx.root, "missing-task"), Config: mustConfig(t, fx)})
	require.ErrorContains(t, err, "resolve task directory")
	_, err = CreateContext(Opts{Cwd: filepath.Join(fx.root, "missing-cwd"), TaskDir: fx.taskDir, Config: mustConfig(t, fx)})
	require.ErrorContains(t, err, "resolve working directory")
}

func TestContextStopsWhenAbsoluteDirectoryResolutionFails(t *testing.T) {
	for _, directory := range []string{"task", "working"} {
		t.Run(directory, func(t *testing.T) {
			fx := newFixture(t)
			t.Chdir(fx.root)
			cfg := mustConfig(t, fx)
			statePath := filepath.Join(fx.taskDir, "state.json")
			before, err := os.ReadFile(statePath)
			require.NoError(t, err)
			taskDir := filepath.Join("docs", fx.slug)
			failedPath := taskDir
			if directory == "working" {
				failedPath = "."
			}
			cause := &os.PathError{Op: "getwd", Err: os.ErrNotExist}
			original := absoluteDirectoryPath
			t.Cleanup(func() { absoluteDirectoryPath = original })
			absoluteDirectoryPath = func(path string) (string, error) {
				if path == failedPath {
					return "", cause
				}
				return original(path)
			}
			ctx, err := CreateContext(Opts{TaskDir: taskDir, Config: cfg})
			require.ErrorIs(t, err, cause, "path resolution must preserve the original getwd failure")
			assert.ErrorContains(t, err, "resolve "+directory+" directory")
			assert.Nil(t, ctx, "an unresolved workspace must not produce an executable context")
			after, err := os.ReadFile(statePath)
			require.NoError(t, err)
			assert.Equal(t, before, after, "path resolution failure must not advance task state")
		})
	}
}

func TestCanonicalDirectoryRejectsRelativePathAfterWorkingDirectoryRemoval(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.Remove(dir))
	_, err := canonicalDirectory(".")
	require.Error(t, err, "relative paths cannot be canonicalized without an existing working directory")
}
