package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestoreReservedReportRestoresOnlyOldTrackedFileAndMode(t *testing.T) {
	repo := initRepo(t)
	rel := "docs/task[1]/reports/M01-T01.done.json"
	path := filepath.Join(repo, filepath.FromSlash(rel))
	writeFile(t, path, "old tracked report\n")
	require.NoError(t, os.Chmod(path, 0o755))
	gitMust(t, repo, "add", "--", rel)
	gitMust(t, repo, "commit", "-m", "prior delivered report")
	baseline := strings.TrimSpace(gitMust(t, repo, "rev-parse", "HEAD"))
	writeFile(t, path, "new report retained in canonical task directory\n")
	require.NoError(t, os.Chmod(path, 0o644))
	writeFile(t, filepath.Join(repo, "wip.go"), "implementation WIP")
	restored, err := RestoreReservedReport(repo, baseline, rel)
	require.NoError(t, err)
	assert.True(t, restored)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "old tracked report\n", string(raw))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	assert.FileExists(t, filepath.Join(repo, "wip.go"), "不清理其它文件")
	assert.Empty(t, strings.TrimSpace(gitMust(t, repo, "diff", "HEAD", "--", rel)))

	newRel := "docs/task[1]/reports/M01-T02.done.json"
	writeFile(t, filepath.Join(repo, filepath.FromSlash(newRel)), "fresh untracked report")
	restored, err = RestoreReservedReport(repo, baseline, newRel)
	require.NoError(t, err)
	assert.False(t, restored)
	assert.FileExists(t, filepath.Join(repo, filepath.FromSlash(newRel)), "未跟踪报告的移除由收集器在确认保存后进行")
}

func TestRestoreReservedReportRejectsOutOfScopeAndInvalidHistoricalPaths(t *testing.T) {
	repo := initRepo(t)
	baseline, _ := GetBaseRef(repo)
	for _, path := range []string{"../reports/M01-T01.done.json", filepath.Join(t.TempDir(), "reports", "M01-T01.done.json"), "source.go", "docs/task/reports/nested/file.done.json"} {
		_, err := RestoreReservedReport(repo, baseline, path)
		require.ErrorContains(t, err, "non-reserved report path")
	}
	_, err := RestoreReservedReport(repo, "missing-reference", "reports/M01-T01.done.json")
	require.ErrorContains(t, err, "inspect prior report")
	writeFile(t, filepath.Join(repo, "reports", "M01-T01.done.json", "nested.txt"), "directory is not a report")
	gitMust(t, repo, "add", ".")
	gitMust(t, repo, "commit", "-m", "invalid historical report directory")
	baseline, _ = GetBaseRef(repo)
	_, err = RestoreReservedReport(repo, baseline, "reports/M01-T01.done.json")
	require.ErrorContains(t, err, "not a file")
}

func TestRestoreReservedReportFailurePreservesCollectedFile(t *testing.T) {
	repo := initRepo(t)
	rel := "reports/M01-T01.done.json"
	writeFile(t, filepath.Join(repo, rel), "old report")
	gitMust(t, repo, "add", ".")
	gitMust(t, repo, "commit", "-m", "old report")
	baseline, _ := GetBaseRef(repo)
	writeFile(t, filepath.Join(repo, rel), "new report")
	original := gitOut
	t.Cleanup(func() { gitOut = original })
	gitOut = func(cwd string, args ...string) (string, error) {
		if args[0] == "restore" {
			return "", fmt.Errorf("worktree write failed")
		}
		return original(cwd, args...)
	}
	restored, err := RestoreReservedReport(repo, baseline, rel)
	require.ErrorContains(t, err, "restore prior report")
	assert.False(t, restored)
	raw, err := os.ReadFile(filepath.Join(repo, rel))
	require.NoError(t, err)
	assert.Equal(t, "new report", string(raw))
}
