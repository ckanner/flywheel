package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"syscall"

	"github.com/ckanner/flywheel/internal/modules"
	"github.com/ckanner/flywheel/internal/skillsembed"
)

const snapshotMarker = ".flywheel-snapshot"
const legacySnapshotMarker = "flywheel embedded snapshot\n"

type snapshotEntry struct {
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

type snapshotManifest struct {
	Version int                      `json:"version"`
	Owner   string                   `json:"owner"`
	Skill   string                   `json:"skill"`
	Entries map[string]snapshotEntry `json:"entries"`
}

// The filesystem seams let tests inject failures during a real staged update.
// Production always uses the OS operations; content/ownership checks are never
// replaced with a trusted marker or an unchecked recursive deletion.
var (
	skillFSFor        = skillsembed.DirFS
	snapshotLstat     = os.Lstat
	snapshotReadFile  = os.ReadFile
	snapshotWriteFile = os.WriteFile
	snapshotMkdirTemp = os.MkdirTemp
	// POSIX rename can replace our empty reservation atomically. os.Rename
	// explicitly rejects an existing directory, including an empty reservation.
	snapshotRename    = syscall.Rename
	snapshotRemoveAll = os.RemoveAll
	snapshotOpenFile  = os.OpenFile
	snapshotFileLock  = syscall.Flock
)

func installEmbeddedSnapshot(projectAbs string) (modules.InstallSummary, error) {
	sum := modules.InstallSummary{Scope: "user (embedded snapshot)"}
	if projectAbs != "" {
		sum.TargetBase = filepath.Join(projectAbs, ".agents", "skills")
		sum.Scope = fmt.Sprintf("project (%s)", projectAbs)
	} else if home, err := os.UserHomeDir(); err == nil {
		sum.TargetBase = filepath.Join(home, ".agents", "skills")
	} else {
		return sum, fmt.Errorf("cannot resolve home directory: %w", err)
	}
	if err := os.MkdirAll(sum.TargetBase, 0o755); err != nil {
		return sum, err
	}
	lock, err := acquireSnapshotLock(sum.TargetBase)
	if err != nil {
		return sum, err
	}
	// Closing the descriptor releases the lock even after a process crash; the
	// lock inode stays in place so concurrent installers cannot lock replacements.
	defer lock.Close()
	for _, skill := range skillsembed.List() {
		dest := filepath.Join(sum.TargetBase, skill.Name)
		action, err := installSkillSnapshot(skill.Name, dest)
		result := modules.SkillInstallResult{Name: skill.Name, Dest: dest, Action: action}
		if err != nil {
			result.Action, result.Reason = "error", err.Error()
		}
		sum.Results = append(sum.Results, result)
	}
	return sum, nil
}

func acquireSnapshotLock(base string) (*os.File, error) {
	path := filepath.Join(base, ".flywheel-install.lock")
	f, err := snapshotOpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open skills installation lock: %w", err)
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("skills installation lock must be a readable regular file: %s", path)
	}
	if err := snapshotFileLock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("cannot lock skills installation directory; wait for the other installer and retry: %w", err)
	}
	return f, nil
}

// snapshotEntries inventories every entry, including empty directories and file
// modes. Links/special files are conflicts, never followed or silently omitted.
func snapshotEntries(root string, expected map[string]snapshotEntry) (map[string]snapshotEntry, error) {
	entries := make(map[string]snapshotEntry)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(root, path) // WalkDir only emits descendants of root.
		if rel == snapshotMarker {
			return nil
		}
		info, err := snapshotLstat(path)
		if err != nil {
			return err
		}
		entry := snapshotEntry{Mode: uint32(info.Mode())}
		if expected != nil {
			previous, registered := expected[filepath.ToSlash(rel)]
			if !registered || previous.Mode != entry.Mode || (!info.IsDir() && previous.Size != info.Size()) {
				return fmt.Errorf("snapshot was modified or contains additional entries; preserving %s — back it up and move it aside before reinstalling", root)
			}
		}
		if !info.IsDir() {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("unsupported entry %s; keep this directory and move it aside before reinstalling", rel)
			}
			data, err := snapshotReadFile(path)
			if err != nil {
				return err
			}
			hash := sha256.Sum256(data)
			entry.Size = int64(len(data))
			entry.SHA256 = hex.EncodeToString(hash[:])
		}
		entries[filepath.ToSlash(rel)] = entry
		return nil
	})
	return entries, err
}

// ownedSnapshot rejects any changed or additional content. The legacy marker
// proves no ownership by itself: only an exact current embedded tree may migrate.
func ownedSnapshot(name, dest string, desired map[string]snapshotEntry) (snapshotManifest, bool, error) {
	info, err := snapshotLstat(dest)
	if errors.Is(err, fs.ErrNotExist) {
		return snapshotManifest{}, false, nil
	}
	if err != nil {
		return snapshotManifest{}, false, err
	}
	if !info.IsDir() {
		return snapshotManifest{}, false, fmt.Errorf("a file or symlink already exists here — refusing to replace it; move %s aside before reinstalling", dest)
	}
	marker := filepath.Join(dest, snapshotMarker)
	markInfo, err := snapshotLstat(marker)
	if err != nil || !markInfo.Mode().IsRegular() {
		return snapshotManifest{}, false, fmt.Errorf("a real directory already exists here without a valid ownership manifest — refusing to remove it; move %s aside before reinstalling", dest)
	}
	raw, err := snapshotReadFile(marker)
	if err != nil {
		return snapshotManifest{}, false, err
	}
	legacy := string(raw) == legacySnapshotMarker
	var manifest snapshotManifest
	expected := desired
	if !legacy {
		if err := json.Unmarshal(raw, &manifest); err != nil || manifest.Version != 1 || manifest.Owner != "flywheel" || manifest.Skill != name || len(manifest.Entries) == 0 {
			return snapshotManifest{}, false, fmt.Errorf("invalid ownership manifest; preserving %s — back it up and move it aside before reinstalling", dest)
		}
		expected = manifest.Entries
	}
	actual, err := snapshotEntries(dest, expected)
	if err != nil {
		return snapshotManifest{}, false, err
	}
	if legacy {
		if !maps.Equal(actual, desired) {
			return snapshotManifest{}, false, fmt.Errorf("legacy snapshot differs from this embedded version; preserving all files at %s — back it up and move it aside before reinstalling", dest)
		}
		return snapshotManifest{Entries: actual}, true, nil
	}
	if !maps.Equal(actual, manifest.Entries) {
		return snapshotManifest{}, false, fmt.Errorf("snapshot was modified or contains additional entries; preserving %s — back it up and move it aside before reinstalling", dest)
	}
	return manifest, true, nil
}

// publishSnapshot reserves an absent destination before rename. If anything
// appears there concurrently, only an empty reservation can be removed; user
// files and directories are never recursively removed to force publication.
func publishSnapshot(source, dest string) error {
	if err := os.Mkdir(dest, 0o755); err != nil {
		return err
	}
	if err := snapshotRename(source, dest); err != nil {
		return errors.Join(err, os.Remove(dest))
	}
	return nil
}

func installSkillSnapshot(name, dest string) (action string, resultErr error) {
	sub, err := skillFSFor(name)
	if err != nil {
		return "error", err
	}
	stage, err := snapshotMkdirTemp(filepath.Dir(dest), "."+name+".install-")
	if err != nil {
		return "error", err
	}
	keepStage := false
	defer func() {
		if keepStage {
			return
		}
		if err := snapshotRemoveAll(stage); err != nil {
			action = "error"
			resultErr = errors.Join(resultErr, fmt.Errorf("cannot clean staging directory %s: %w", stage, err))
		}
	}()
	candidate := filepath.Join(stage, "new")
	if err := materializeFS(sub, ".", candidate); err != nil {
		return "error", err
	}
	entries, err := snapshotEntries(candidate, nil)
	if err != nil {
		return "error", err
	}
	manifest := snapshotManifest{Version: 1, Owner: "flywheel", Skill: name, Entries: entries}
	data, _ := json.MarshalIndent(manifest, "", "  ") // Only fixed JSON-safe scalar/map types.
	if err := snapshotWriteFile(filepath.Join(candidate, snapshotMarker), append(data, '\n'), 0o644); err != nil {
		return "error", err
	}
	old, exists, err := ownedSnapshot(name, dest, entries)
	if err != nil {
		return "error", err
	}
	if exists && old.Version == 1 && maps.Equal(old.Entries, entries) {
		return "skipped", nil
	}
	backup := filepath.Join(stage, "previous")
	if exists {
		if err := snapshotRename(dest, backup); err != nil {
			return "error", err
		}
		// Verify again after taking ownership of the old path. An editor may have
		// changed it between validation and rename; retain it on every failure.
		current, err := snapshotEntries(backup, old.Entries)
		if err != nil || !maps.Equal(current, old.Entries) {
			if restoreErr := publishSnapshot(backup, dest); restoreErr != nil {
				keepStage = true
				return "error", errors.Join(fmt.Errorf("snapshot changed during installation and rollback failed; previous files retained at %s", backup), err, restoreErr)
			}
			return "error", errors.Join(fmt.Errorf("snapshot changed during installation; previous files restored to %s", dest), err)
		}
	}
	if err := publishSnapshot(candidate, dest); err != nil {
		if exists {
			if restoreErr := publishSnapshot(backup, dest); restoreErr != nil {
				keepStage = true
				return "error", errors.Join(err, fmt.Errorf("rollback failed; previous files retained at %s: %w", backup, restoreErr))
			}
		}
		return "error", err
	}
	if exists {
		current, err := snapshotEntries(backup, old.Entries)
		if err != nil || !maps.Equal(current, old.Entries) {
			keepStage = true
			return "error", errors.Join(fmt.Errorf("previous snapshot changed during publication; preserving backup at %s", backup), err)
		}
	}
	return "copied", nil
}
