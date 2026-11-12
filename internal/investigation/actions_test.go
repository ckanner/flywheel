package investigation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func inspectionFixture(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, "approved"), 0700))
	require.NoError(t, os.Mkdir(filepath.Join(directory, "other"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "approved", "file"), []byte("first\nneedle here\nlast\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "other", "file"), []byte("private and not granted"), 0600))
	return directory
}

func TestInspectionBindsOpenedInodeBeforeReadingAnExactGrant(t *testing.T) {
	directory := inspectionFixture(t)
	open := openInspectionFile
	t.Cleanup(func() { openInspectionFile = open })
	openInspectionFile = func(root *os.Root, name string, flag int, mode os.FileMode) (*os.File, error) {
		require.Equal(t, "approved/file", name)
		require.NoError(t, os.Rename(filepath.Join(directory, "approved"), filepath.Join(directory, "original")))
		require.NoError(t, os.Symlink("other", filepath.Join(directory, "approved")))
		return open(root, name, flag, mode)
	}
	reader := readInspectionBytes
	t.Cleanup(func() { readInspectionBytes = reader })
	readInspectionBytes = func(io.Reader) ([]byte, error) { t.Fatal("ungranted inode was read"); return nil, nil }
	raw, _, err := readPath(context.Background(), directory, "approved/file", 1024)
	require.ErrorContains(t, err, "path changed")
	require.Empty(t, raw)
}

func TestInspectionRejectsSymlinksAndPathEscape(t *testing.T) {
	directory := inspectionFixture(t)
	require.NoError(t, os.Symlink("other", filepath.Join(directory, "link")))
	require.NoError(t, os.Symlink("../other/file", filepath.Join(directory, "approved", "link")))
	for _, path := range []string{"link/file", "approved/link", "../outside", "missing"} {
		_, _, err := readPath(context.Background(), directory, path, 1024)
		require.Error(t, err, path)
	}
	_, _, err := readPath(context.Background(), filepath.Join(directory, "missing-root"), "file", 1024)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = readPath(ctx, directory, "approved/file", 1024)
	require.ErrorIs(t, err, context.Canceled)
}

func TestFileInspectionBoundsTypesAndUTF8(t *testing.T) {
	directory := inspectionFixture(t)
	_, _, err := readPath(context.Background(), directory, "approved/file", 2)
	require.ErrorContains(t, err, "approved inspection size")
	require.NoError(t, os.WriteFile(filepath.Join(directory, "binary"), []byte{255}, 0600))
	_, _, err = readPath(context.Background(), directory, "binary", 1024)
	require.ErrorContains(t, err, "UTF-8")
	require.NoError(t, syscall.Mkfifo(filepath.Join(directory, "fifo"), 0600))
	_, _, err = readPath(context.Background(), directory, "fifo", 1024)
	require.ErrorContains(t, err, "regular files")
	for _, scenario := range []string{"open", "stat", "read", "cancel-after-read"} {
		t.Run(scenario, func(t *testing.T) {
			open, reader := openInspectionFile, readInspectionBytes
			t.Cleanup(func() { openInspectionFile, readInspectionBytes = open, reader })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			openInspectionFile = func(root *os.Root, name string, flag int, mode os.FileMode) (*os.File, error) {
				if scenario == "open" {
					return nil, os.ErrPermission
				}
				file, err := open(root, name, flag, mode)
				if scenario == "stat" {
					require.NoError(t, err)
					require.NoError(t, file.Close())
				}
				return file, err
			}
			readInspectionBytes = func(input io.Reader) ([]byte, error) {
				if scenario == "read" {
					return nil, io.ErrUnexpectedEOF
				}
				data, err := reader(input)
				if scenario == "cancel-after-read" {
					cancel()
				}
				return data, err
			}
			_, _, err := readPath(ctx, directory, "approved/file", 1024)
			require.Error(t, err)
		})
	}
}

type inspectionNamedEntry struct {
	os.DirEntry
	name string
}

func (e inspectionNamedEntry) Name() string { return e.name }

func TestDirectoryInspectionIsBoundedAndNamesAreUnambiguous(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, "small"), 0700))
	for _, name := range []string{"z", "a\nb", "a"} {
		require.NoError(t, os.WriteFile(filepath.Join(directory, "small", name), nil, 0600))
	}
	raw, isDirectory, err := readPath(context.Background(), directory, "small", 1024)
	require.NoError(t, err)
	require.True(t, isDirectory)
	var names []string
	require.NoError(t, json.Unmarshal(raw, &names))
	require.Equal(t, []string{"a", "a\nb", "z"}, names)
	require.NoError(t, os.Mkdir(filepath.Join(directory, "large"), 0700))
	for i := range 4097 {
		require.NoError(t, os.WriteFile(filepath.Join(directory, "large", fmt.Sprintf("file-%04d", i)), nil, 0600))
	}
	_, _, err = readPath(context.Background(), directory, "large", 1024)
	require.ErrorContains(t, err, "4096")
	readDir := readInspectionDirectory
	t.Cleanup(func() { readInspectionDirectory = readDir })
	// APFS rejects this filename at creation; inject the directory-entry boundary
	// so every platform exercises the same invalid-input rejection.
	entries, err := os.ReadDir(filepath.Join(directory, "small"))
	require.NoError(t, err)
	readInspectionDirectory = func(*os.File, int) ([]os.DirEntry, error) {
		return []os.DirEntry{inspectionNamedEntry{DirEntry: entries[0], name: string([]byte{255})}}, nil
	}
	_, _, err = readPath(context.Background(), directory, "small", 1024)
	require.ErrorContains(t, err, "UTF-8 names")
	readInspectionDirectory = func(*os.File, int) ([]os.DirEntry, error) { return nil, os.ErrPermission }
	_, _, err = readPath(context.Background(), directory, "small", 1024)
	require.ErrorIs(t, err, os.ErrPermission)
}

func TestCandidateTracksExactReadableAndUnavailableInputs(t *testing.T) {
	directory := inspectionFixture(t)
	st := &state.PreparationState{Cwd: directory, Contract: contract.Preparation{ReadPaths: []string{"approved/file", "missing", "other"}, Limits: contract.InvestigationLimits{MaxFileBytes: 1024}}}
	first, err := candidate(context.Background(), st)
	require.NoError(t, err)
	require.NotEmpty(t, first)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "approved", "file"), []byte("changed"), 0600))
	second, err := candidate(context.Background(), st)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = candidate(ctx, st)
	require.ErrorIs(t, err, context.Canceled)
}

func TestConcreteActionsNeverExecuteCommandsAndPreserveContentIdentity(t *testing.T) {
	directory := inspectionFixture(t)
	limits := contract.InvestigationLimits{MaxFileBytes: 1024}
	action := contract.InvestigationAction{Kind: "search_text", Path: "approved/file", Query: "needle"}
	output, hash, truncated, err := executeAction(context.Background(), directory, action, limits, 1024)
	require.NoError(t, err)
	require.Equal(t, "2:needle here", output)
	require.NotEmpty(t, hash)
	require.False(t, truncated)
	action.Kind = "read_file"
	output, readHash, _, err := executeAction(context.Background(), directory, action, limits, 1024)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(output, "first"))
	require.Equal(t, hash, readHash)
	action.Kind = "list_files"
	_, _, _, err = executeAction(context.Background(), directory, action, limits, 1024)
	require.ErrorContains(t, err, "file type")
	action.Path = "approved"
	output, _, _, err = executeAction(context.Background(), directory, action, limits, 1024)
	require.NoError(t, err)
	require.Equal(t, `["file"]`, output)
	action.Path = "missing"
	_, _, _, err = executeAction(context.Background(), directory, action, limits, 1024)
	require.Error(t, err)
	action.Kind, action.Path = "read_file", "approved/file"
	require.NoError(t, os.WriteFile(filepath.Join(directory, action.Path), []byte("中文text"), 0600))
	output, _, truncated, err = executeAction(context.Background(), directory, action, limits, 4)
	require.NoError(t, err)
	require.Equal(t, "中", output)
	require.True(t, truncated)
}
