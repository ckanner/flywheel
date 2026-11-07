package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/skillsembed"
)

func snapshotHooksT(t *testing.T) {
	t.Helper()
	fsFor, lstat, readFile, writeFile := skillFSFor, snapshotLstat, snapshotReadFile, snapshotWriteFile
	mkdirTemp, rename, removeAll := snapshotMkdirTemp, snapshotRename, snapshotRemoveAll
	openFile, lock := snapshotOpenFile, snapshotFileLock
	t.Cleanup(func() {
		skillFSFor, snapshotLstat, snapshotReadFile, snapshotWriteFile = fsFor, lstat, readFile, writeFile
		snapshotMkdirTemp, snapshotRename, snapshotRemoveAll = mkdirTemp, rename, removeAll
		snapshotOpenFile, snapshotFileLock = openFile, lock
	})
}

func oldSnapshotT(t *testing.T) (string, []byte) {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "flywheel-run")
	old := []byte("previous shipped skill\n")
	original := skillFSFor
	skillFSFor = func(string) (fs.FS, error) {
		return fstest.MapFS{"SKILL.md": &fstest.MapFile{Data: old}}, nil
	}
	action, err := installSkillSnapshot("flywheel-run", dest)
	skillFSFor = original
	require.NoError(t, err)
	require.Equal(t, "copied", action)
	return dest, old
}

func snapshotBytesT(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return raw
}

func snapshotJSONT(t *testing.T, args ...string) (int, skillInstallResponse) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runRoot(append([]string{"install-skills", "--json"}, args...), &stdout, &stderr)
	require.Empty(t, stderr.String(), "JSON failures are already represented in the envelope")
	var reply skillInstallResponse
	decoder := json.NewDecoder(&stdout)
	require.NoError(t, decoder.Decode(&reply))
	assert.ErrorIs(t, decoder.Decode(&struct{}{}), io.EOF, "exactly one JSON document")
	assert.Equal(t, 1, reply.Protocol)
	require.Len(t, reply.Results, 11)
	return code, reply
}

func TestSnapshotVerifiedUpgradeAndIdempotency(t *testing.T) {
	dest, old := oldSnapshotT(t)
	action, err := installSkillSnapshot("flywheel-run", dest)
	require.NoError(t, err)
	assert.Equal(t, "copied", action)
	current, err := skillsembed.Read("flywheel-run")
	require.NoError(t, err)
	assert.NotEqual(t, string(old), current)
	assert.Equal(t, current, string(snapshotBytesT(t, filepath.Join(dest, "SKILL.md"))))
	var manifest snapshotManifest
	require.NoError(t, json.Unmarshal(snapshotBytesT(t, filepath.Join(dest, snapshotMarker)), &manifest))
	assert.Equal(t, 1, manifest.Version)
	assert.Equal(t, "flywheel", manifest.Owner)
	assert.Equal(t, "flywheel-run", manifest.Skill)
	assert.Len(t, manifest.Entries["SKILL.md"].SHA256, 64)
	before, err := os.Stat(dest)
	require.NoError(t, err)
	action, err = installSkillSnapshot("flywheel-run", dest)
	require.NoError(t, err)
	assert.Equal(t, "skipped", action)
	after, err := os.Stat(dest)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after), "an unchanged skill keeps its directory identity")
	staging, err := filepath.Glob(filepath.Join(filepath.Dir(dest), ".flywheel-run.install-*"))
	require.NoError(t, err)
	assert.Empty(t, staging)
}

func TestSnapshotPreservesUserChangesAndAdditionalEntries(t *testing.T) {
	for _, mutation := range []string{"content", "extra-file", "empty-directory", "permissions", "symlink"} {
		t.Run(mutation, func(t *testing.T) {
			dest, old := oldSnapshotT(t)
			switch mutation {
			case "content":
				writeFileCli(t, filepath.Join(dest, "SKILL.md"), strings.Repeat("+", len(old)))
			case "extra-file":
				writeFileCli(t, filepath.Join(dest, "notes.txt"), "user notes")
			case "empty-directory":
				require.NoError(t, os.Mkdir(filepath.Join(dest, "my-assets"), 0o755))
			case "permissions":
				require.NoError(t, os.Chmod(filepath.Join(dest, "SKILL.md"), 0o600))
			case "symlink":
				require.NoError(t, os.Symlink("unrelated-private-file", filepath.Join(dest, "user-link")))
			}
			before := snapshotBytesT(t, filepath.Join(dest, "SKILL.md"))
			marker := snapshotBytesT(t, filepath.Join(dest, snapshotMarker))
			action, err := installSkillSnapshot("flywheel-run", dest)
			require.Error(t, err)
			assert.Equal(t, "error", action)
			assert.Contains(t, err.Error(), "move it aside")
			assert.Equal(t, before, snapshotBytesT(t, filepath.Join(dest, "SKILL.md")))
			assert.Equal(t, marker, snapshotBytesT(t, filepath.Join(dest, snapshotMarker)))
			if mutation == "extra-file" {
				assert.Equal(t, "user notes", string(snapshotBytesT(t, filepath.Join(dest, "notes.txt"))))
			}
			if mutation == "empty-directory" {
				assert.DirExists(t, filepath.Join(dest, "my-assets"))
			}
			if mutation == "permissions" {
				info, err := os.Stat(filepath.Join(dest, "SKILL.md"))
				require.NoError(t, err)
				assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
			}
			if mutation == "symlink" {
				link, err := os.Readlink(filepath.Join(dest, "user-link"))
				require.NoError(t, err)
				assert.Equal(t, "unrelated-private-file", link)
			}
		})
	}
}

func TestSnapshotLegacyMigrationRequiresExactCurrentContent(t *testing.T) {
	for _, modified := range []bool{false, true} {
		t.Run(map[bool]string{false: "identical", true: "changed"}[modified], func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "flywheel-run")
			_, err := installSkillSnapshot("flywheel-run", dest)
			require.NoError(t, err)
			writeFileCli(t, filepath.Join(dest, snapshotMarker), legacySnapshotMarker)
			before := snapshotBytesT(t, filepath.Join(dest, "SKILL.md"))
			if modified {
				writeFileCli(t, filepath.Join(dest, "SKILL.md"), strings.Repeat("+", len(before)))
			}
			action, err := installSkillSnapshot("flywheel-run", dest)
			if modified {
				require.ErrorContains(t, err, "legacy snapshot differs")
				assert.Equal(t, "error", action)
				assert.Equal(t, strings.Repeat("+", len(before)), string(snapshotBytesT(t, filepath.Join(dest, "SKILL.md"))))
				assert.Equal(t, legacySnapshotMarker, string(snapshotBytesT(t, filepath.Join(dest, snapshotMarker))))
			} else {
				require.NoError(t, err)
				assert.Equal(t, "copied", action)
				var manifest snapshotManifest
				require.NoError(t, json.Unmarshal(snapshotBytesT(t, filepath.Join(dest, snapshotMarker)), &manifest))
				assert.Equal(t, 1, manifest.Version)
			}
		})
	}
}

func TestSnapshotRejectsInvalidOwnership(t *testing.T) {
	for _, marker := range []string{"{broken", `{"version":2,"owner":"flywheel","skill":"flywheel-run"}`, `{"version":1,"owner":"another-tool","skill":"flywheel-run"}`, "symlink", "directory"} {
		t.Run(marker, func(t *testing.T) {
			dest, old := oldSnapshotT(t)
			path := filepath.Join(dest, snapshotMarker)
			require.NoError(t, os.Remove(path))
			switch marker {
			case "symlink":
				require.NoError(t, os.Symlink("SKILL.md", path))
			case "directory":
				require.NoError(t, os.Mkdir(path, 0o755))
			default:
				writeFileCli(t, path, marker)
			}
			_, err := installSkillSnapshot("flywheel-run", dest)
			require.ErrorContains(t, err, "manifest")
			assert.Equal(t, old, snapshotBytesT(t, filepath.Join(dest, "SKILL.md")))
		})
	}
}

func TestSnapshotStagingAndReadFailuresPreservePreviousInstallation(t *testing.T) {
	for _, failure := range []string{"source", "stage", "manifest-write", "candidate-read", "candidate-stat", "destination-stat", "marker-read", "installed-read", "backup-rename", "publish-rename"} {
		t.Run(failure, func(t *testing.T) {
			snapshotHooksT(t)
			dest, old := oldSnapshotT(t)
			boom := errors.New("injected " + failure)
			switch failure {
			case "source":
				skillFSFor = func(string) (fs.FS, error) { return nil, boom }
			case "stage":
				snapshotMkdirTemp = func(string, string) (string, error) { return "", boom }
			case "manifest-write":
				snapshotWriteFile = func(string, []byte, fs.FileMode) error { return boom }
			case "candidate-read", "marker-read", "installed-read":
				snapshotReadFile = func(path string) ([]byte, error) {
					if (failure == "candidate-read" && strings.Contains(path, string(filepath.Separator)+"new"+string(filepath.Separator))) ||
						(failure == "marker-read" && path == filepath.Join(dest, snapshotMarker)) ||
						(failure == "installed-read" && path == filepath.Join(dest, "SKILL.md")) {
						return nil, boom
					}
					return os.ReadFile(path)
				}
			case "candidate-stat", "destination-stat":
				snapshotLstat = func(path string) (fs.FileInfo, error) {
					if (failure == "candidate-stat" && strings.Contains(path, string(filepath.Separator)+"new"+string(filepath.Separator))) ||
						(failure == "destination-stat" && path == dest) {
						return nil, boom
					}
					return os.Lstat(path)
				}
			case "backup-rename", "publish-rename":
				snapshotRename = func(source, target string) error {
					if (failure == "backup-rename" && source == dest) || (failure == "publish-rename" && filepath.Base(source) == "new") {
						return boom
					}
					return syscall.Rename(source, target)
				}
			}
			action, err := installSkillSnapshot("flywheel-run", dest)
			require.ErrorIs(t, err, boom)
			assert.Equal(t, "error", action)
			assert.Equal(t, old, snapshotBytesT(t, filepath.Join(dest, "SKILL.md")))
			staging, err := filepath.Glob(filepath.Join(filepath.Dir(dest), ".flywheel-run.install-*"))
			require.NoError(t, err)
			assert.Empty(t, staging, "failed publication must restore the old tree and remove only its own staging")
		})
	}
}

func TestSnapshotPublicationAndRollbackFailureKeepsRecoverableBackup(t *testing.T) {
	snapshotHooksT(t)
	dest, old := oldSnapshotT(t)
	snapshotRename = func(source, target string) error {
		if target == dest {
			return errors.New("publication or rollback unavailable")
		}
		return syscall.Rename(source, target)
	}
	_, err := installSkillSnapshot("flywheel-run", dest)
	require.ErrorContains(t, err, "rollback failed")
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(dest), ".flywheel-run.install-*", "previous"))
	require.NoError(t, err)
	require.Len(t, backups, 1)
	assert.Equal(t, old, snapshotBytesT(t, filepath.Join(backups[0], "SKILL.md")))
	assert.NoDirExists(t, dest)
	// A later install must not garbage-collect an earlier failed transaction.
	snapshotRename = syscall.Rename
	_, err = installSkillSnapshot("flywheel-run", dest)
	require.NoError(t, err)
	assert.Equal(t, old, snapshotBytesT(t, filepath.Join(backups[0], "SKILL.md")))
}

func TestSnapshotConcurrentChangesArePreserved(t *testing.T) {
	for _, when := range []string{"before-publication", "before-publication-rollback-fails", "after-publication"} {
		t.Run(when, func(t *testing.T) {
			snapshotHooksT(t)
			dest, old := oldSnapshotT(t)
			snapshotRename = func(source, target string) error {
				if when == "before-publication-rollback-fails" && filepath.Base(source) == "previous" {
					return errors.New("rollback unavailable")
				}
				if err := syscall.Rename(source, target); err != nil {
					return err
				}
				if strings.HasPrefix(when, "before-publication") && source == dest {
					writeFileCli(t, filepath.Join(target, "user-edit.txt"), "edit during move")
				}
				if when == "after-publication" && filepath.Base(source) == "new" {
					writeFileCli(t, filepath.Join(filepath.Dir(source), "previous", "user-edit.txt"), "edit during move")
				}
				return nil
			}
			_, err := installSkillSnapshot("flywheel-run", dest)
			require.ErrorContains(t, err, "changed during")
			if when == "before-publication" {
				assert.Equal(t, old, snapshotBytesT(t, filepath.Join(dest, "SKILL.md")))
				assert.Equal(t, "edit during move", string(snapshotBytesT(t, filepath.Join(dest, "user-edit.txt"))))
			} else {
				backups, err := filepath.Glob(filepath.Join(filepath.Dir(dest), ".flywheel-run.install-*", "previous"))
				require.NoError(t, err)
				require.Len(t, backups, 1)
				assert.Equal(t, old, snapshotBytesT(t, filepath.Join(backups[0], "SKILL.md")))
				assert.Equal(t, "edit during move", string(snapshotBytesT(t, filepath.Join(backups[0], "user-edit.txt"))))
			}
		})
	}
}

func TestSnapshotPublishNeverRemovesAConcurrentUserEntry(t *testing.T) {
	snapshotHooksT(t)
	base := t.TempDir()
	source, dest := filepath.Join(base, "candidate"), filepath.Join(base, "destination")
	writeFileCli(t, filepath.Join(source, "SKILL.md"), "new")
	writeFileCli(t, filepath.Join(dest, "user.txt"), "existing user file")
	require.Error(t, publishSnapshot(source, dest))
	assert.Equal(t, "existing user file", string(snapshotBytesT(t, filepath.Join(dest, "user.txt"))))
	require.NoError(t, os.Remove(filepath.Join(dest, "user.txt")))
	require.NoError(t, os.Remove(dest))
	snapshotRename = func(_, target string) error {
		writeFileCli(t, filepath.Join(target, "user.txt"), "concurrent user file")
		return errors.New("rename failed")
	}
	require.Error(t, publishSnapshot(source, dest))
	assert.Equal(t, "concurrent user file", string(snapshotBytesT(t, filepath.Join(dest, "user.txt"))))
	assert.Equal(t, "new", string(snapshotBytesT(t, filepath.Join(source, "SKILL.md"))))
}

func TestSnapshotFreshPublicationAndCleanupFailuresAreReported(t *testing.T) {
	for _, failure := range []string{"publish", "cleanup"} {
		t.Run(failure, func(t *testing.T) {
			snapshotHooksT(t)
			dest := filepath.Join(t.TempDir(), "flywheel-run")
			if failure == "publish" {
				snapshotRename = func(string, string) error { return errors.New("publish failed") }
			} else {
				snapshotRemoveAll = func(string) error { return errors.New("cleanup failed") }
			}
			action, err := installSkillSnapshot("flywheel-run", dest)
			require.ErrorContains(t, err, failure)
			assert.Equal(t, "error", action)
			if failure == "publish" {
				assert.NoDirExists(t, dest)
			} else {
				assert.FileExists(t, filepath.Join(dest, "SKILL.md"))
			}
		})
	}
}

func TestSnapshotInventoryErrors(t *testing.T) {
	_, err := snapshotEntries(filepath.Join(t.TempDir(), "missing"), nil)
	require.ErrorIs(t, err, fs.ErrNotExist)
	base := t.TempDir()
	require.NoError(t, os.Symlink("unreadable-target", filepath.Join(base, "link")))
	_, err = snapshotEntries(base, nil)
	require.ErrorContains(t, err, "unsupported entry")
}

func TestSnapshotInstallLockPreventsConcurrentUpdatesAndReleases(t *testing.T) {
	project := t.TempDir()
	base := filepath.Join(project, ".agents", "skills")
	require.NoError(t, os.MkdirAll(base, 0o755))
	lock, err := acquireSnapshotLock(base)
	require.NoError(t, err)
	_, err = installEmbeddedSnapshot(project)
	require.ErrorContains(t, err, "other installer")
	assert.NoDirExists(t, filepath.Join(base, "flywheel-run"))
	require.NoError(t, lock.Close())
	sum, err := installEmbeddedSnapshot(project)
	require.NoError(t, err)
	require.Len(t, sum.Results, 11)
	for _, result := range sum.Results {
		assert.Equal(t, "copied", result.Action, result.Reason)
	}
}

func TestSnapshotLockRejectsUnusableOrNonregularFiles(t *testing.T) {
	for _, failure := range []string{"directory", "symlink", "fifo", "closed"} {
		t.Run(failure, func(t *testing.T) {
			snapshotHooksT(t)
			base := t.TempDir()
			path := filepath.Join(base, ".flywheel-install.lock")
			switch failure {
			case "directory":
				require.NoError(t, os.Mkdir(path, 0o755))
			case "symlink":
				require.NoError(t, os.Symlink("unrelated", path))
			case "fifo":
				require.NoError(t, syscall.Mkfifo(path, 0o600))
			case "closed":
				snapshotOpenFile = func(path string, flag int, perm fs.FileMode) (*os.File, error) {
					f, err := os.OpenFile(path, flag, perm)
					require.NoError(t, err)
					require.NoError(t, f.Close())
					return f, nil
				}
			}
			_, err := acquireSnapshotLock(base)
			require.Error(t, err)
		})
	}
}

func TestInstallSkillsJSONSuccessAndModifiedConflict(t *testing.T) {
	t.Setenv("FLYWHEEL_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	project := t.TempDir()
	code, reply := snapshotJSONT(t, "--project", project)
	require.Zero(t, code)
	assert.True(t, reply.OK)
	assert.Equal(t, "created", reply.Config.Action)
	assert.Equal(t, "embedded", reply.Method)
	assert.Equal(t, filepath.Join(project, ".agents", "skills"), reply.TargetBase)
	for _, result := range reply.Results {
		assert.Equal(t, "copied", result.Action)
		assert.Equal(t, filepath.Join(reply.TargetBase, result.Name), result.Dest)
	}
	custom := filepath.Join(reply.TargetBase, "flywheel-run", "SKILL.md")
	writeFileCli(t, custom, "my custom workflow")
	code, reply = snapshotJSONT(t, "--project", project)
	assert.Equal(t, 1, code)
	assert.False(t, reply.OK)
	assert.Equal(t, "existing", reply.Config.Action)
	for _, result := range reply.Results {
		if result.Name == "flywheel-run" {
			assert.Equal(t, "error", result.Action)
			assert.Contains(t, result.Reason, "modified")
		} else {
			assert.Equal(t, "skipped", result.Action)
		}
	}
	assert.Equal(t, "my custom workflow", string(snapshotBytesT(t, custom)))
}

func TestInstallSkillsJSONTopLevelErrorsStillReportEverySkill(t *testing.T) {
	for _, failure := range []string{"target", "home", "source", "flag", "argument", "config"} {
		t.Run(failure, func(t *testing.T) {
			t.Setenv("FLYWHEEL_CONFIG", filepath.Join(t.TempDir(), "config.json"))
			args := []string{"--project", t.TempDir()}
			switch failure {
			case "target":
				blocker := filepath.Join(t.TempDir(), "file")
				writeFileCli(t, blocker, "preserve")
				args = []string{"--project", blocker}
			case "home":
				t.Setenv("HOME", "")
				args = nil
			case "source":
				chdirT(t, t.TempDir())
				args = append(args, "--link")
			case "flag":
				args = append(args, "--not-a-flag")
			case "argument":
				args = append(args, "unexpected")
			case "config":
				blocker := filepath.Join(t.TempDir(), "file")
				writeFileCli(t, blocker, "preserve")
				t.Setenv("FLYWHEEL_CONFIG", filepath.Join(blocker, "config.json"))
			}
			code, reply := snapshotJSONT(t, args...)
			assert.Equal(t, 1, code)
			assert.False(t, reply.OK)
			assert.NotEmpty(t, reply.Error)
			for _, result := range reply.Results {
				if failure == "config" {
					assert.Equal(t, "copied", result.Action)
					assert.Equal(t, "error", reply.Config.Action)
				} else {
					assert.Equal(t, "error", result.Action)
					assert.NotEmpty(t, result.Reason)
				}
			}
		})
	}
}

func TestInstallSkillsEmptySourceIsNotReportedAsSuccess(t *testing.T) {
	t.Setenv("FLYWHEEL_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	source, project := t.TempDir(), t.TempDir()
	writeFileCli(t, filepath.Join(source, "go.mod"), "module example.com/unrelated\n")
	chdirT(t, source)
	code, reply := snapshotJSONT(t, "--link", "--project", project)
	assert.Equal(t, 1, code)
	assert.False(t, reply.OK)
	require.Len(t, reply.Results, 11)
	for _, result := range reply.Results {
		assert.Equal(t, "error", result.Action)
		assert.Contains(t, result.Reason, "did not report")
		assert.Equal(t, filepath.Join(project, ".agents", "skills", result.Name), result.Dest)
	}
}

func TestInstallJSONFlagSemantics(t *testing.T) {
	for _, test := range []struct {
		args []string
		json bool
	}{
		{[]string{"--json=true", "--bad-flag"}, true},
		{[]string{"--json", "--json=false", "--bad-flag"}, false},
		{[]string{"--", "--json"}, false},
		{[]string{"--json", "--", "--json=false"}, true},
	} {
		var stdout, stderr bytes.Buffer
		assert.Equal(t, 1, runRoot(append([]string{"install-skills"}, test.args...), &stdout, &stderr))
		if test.json {
			var reply skillInstallResponse
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &reply))
			assert.False(t, reply.OK)
			assert.Len(t, reply.Results, 11)
			assert.Empty(t, stderr.String())
		} else {
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), "[flywheel] 错误")
		}
	}
}
