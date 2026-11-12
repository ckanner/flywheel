package state

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fmtPID() string { return strconv.Itoa(os.Getpid()) }

func TestPreparationReadsAreBoundedAndNeverFollowRecordSymlinks(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), filepath.Join(dir, "record")))
	_, err := readPreparationData(dir, "record", 100)
	require.Error(t, err)
	require.NoError(t, os.Remove(filepath.Join(dir, "record")))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, ".investigation")))
	_, err = readPreparationData(dir, ".investigation/secret", 100)
	require.Error(t, err)
	_, err = readPreparationData(dir, "../secret", 100)
	require.Error(t, err)
	_, err = readPreparationData(dir, "missing/record", 100)
	require.Error(t, err)
	_, err = readPreparationData(filepath.Join(dir, "absent"), "record", 100)
	require.Error(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "record"), 0o700))
	_, err = readPreparationData(dir, "record", 100)
	require.ErrorContains(t, err, "regular file")
	require.NoError(t, os.Remove(filepath.Join(dir, "record")))
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "record"), 0o600))
	_, err = readPreparationData(dir, "record", 100)
	require.ErrorContains(t, err, "regular file")
	require.NoError(t, os.Remove(filepath.Join(dir, "record")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "record"), []byte("123456789"), 0o600))
	_, err = readPreparationData(dir, "record", 8)
	require.ErrorContains(t, err, "exceeds 8 bytes")
	raw, err := readPreparationData(dir, "record", 9)
	require.NoError(t, err)
	assert.Equal(t, "123456789", string(raw))
	require.NoError(t, os.WriteFile(PreparationFile(dir), bytes.Repeat([]byte(" "), (8<<20)+1), 0o600))
	_, err = LoadPreparation(dir)
	require.ErrorContains(t, err, "exceeds")
}

func TestPreparationDescriptorFailuresDoNotReturnPartialEvidence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record")
	require.NoError(t, os.WriteFile(path, []byte("evidence"), 0o600))
	original := openPreparationEntry
	t.Cleanup(func() { openPreparationEntry = original })
	openPreparationEntry = func(*os.Root, string, int, os.FileMode) (*os.File, error) {
		f, err := os.Open(path)
		require.NoError(t, err)
		require.NoError(t, f.Close())
		return f, nil
	}
	_, err := readPreparationData(dir, "record", 100)
	require.Error(t, err, "a descriptor lost before stat is not valid evidence")
	openPreparationEntry = func(*os.Root, string, int, os.FileMode) (*os.File, error) { return os.OpenFile(path, os.O_WRONLY, 0) }
	_, err = readPreparationData(dir, "record", 100)
	require.Error(t, err, "read failures must propagate rather than publish an empty observation")
}

func TestPreparationWritesAreAtomicExclusiveAndDurable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writePreparationData(dir, "record", []byte("first"), false))
	require.NoError(t, writePreparationData(dir, "record", []byte("second"), false))
	raw, err := os.ReadFile(filepath.Join(dir, "record"))
	require.NoError(t, err)
	assert.Equal(t, "second", string(raw))
	info, err := os.Stat(filepath.Join(dir, "record"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	require.ErrorIs(t, writePreparationData(dir, "record", []byte("must not replace"), true), os.ErrExist)
	raw, err = os.ReadFile(filepath.Join(dir, "record"))
	require.NoError(t, err)
	assert.Equal(t, "second", string(raw))
	require.NoError(t, writePreparationData(dir, "immutable", []byte("receipt"), true))
	require.NoFileExists(t, filepath.Join(dir, "immutable.tmp-"+fmtPID()))
	require.Error(t, writePreparationData(filepath.Join(dir, "missing"), "record", nil, false))
	require.Error(t, writePreparationData(dir, "../outside", nil, false))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "reserved.tmp-"+fmtPID()), []byte("unknown writer"), 0o600))
	require.ErrorIs(t, writePreparationData(dir, "reserved", []byte("new"), false), os.ErrExist)
}

func TestPreparationWriteFailuresPreserveUnknownOrPublishedData(t *testing.T) {
	dir := t.TempDir()
	sentinel := errors.New("storage unavailable")
	oldOpen, oldSync, oldRename, oldLink := openPreparationEntry, syncStateFile, renamePreparationEntry, linkPreparationEntry
	t.Cleanup(func() {
		openPreparationEntry, syncStateFile, renamePreparationEntry, linkPreparationEntry = oldOpen, oldSync, oldRename, oldLink
	})
	readonly := filepath.Join(dir, "readonly")
	require.NoError(t, os.WriteFile(readonly, []byte("prior"), 0o600))
	openPreparationEntry = func(*os.Root, string, int, os.FileMode) (*os.File, error) { return os.Open(readonly) }
	require.Error(t, writePreparationData(dir, "record", []byte("new"), false))
	openPreparationEntry = oldOpen
	syncStateFile = func(*os.File) error { return sentinel }
	require.ErrorIs(t, writePreparationData(dir, "record", []byte("new"), false), sentinel)
	syncStateFile = oldSync
	renamePreparationEntry = func(*os.Root, string, string) error { return sentinel }
	require.ErrorIs(t, writePreparationData(dir, "record", []byte("new"), false), sentinel)
	renamePreparationEntry = oldRename
	linkPreparationEntry = func(*os.Root, string, string) error { return sentinel }
	require.ErrorIs(t, writePreparationData(dir, "record", []byte("new"), true), sentinel)
	linkPreparationEntry = oldLink
	syncStateFile = func(f *os.File) error {
		info, err := f.Stat()
		require.NoError(t, err)
		if info.IsDir() {
			return sentinel
		}
		return oldSync(f)
	}
	require.ErrorIs(t, writePreparationData(dir, "record", []byte("published but sync unconfirmed"), false), sentinel)
	raw, err := os.ReadFile(filepath.Join(dir, "record"))
	require.NoError(t, err)
	assert.Equal(t, "published but sync unconfirmed", string(raw), "durability uncertainty does not delete the published record")
	syncStateFile = oldSync
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o700))
	renamePreparationEntry = func(root *os.Root, from, to string) error {
		if err := oldRename(root, from, to); err != nil {
			return err
		}
		return root.Rename("sub", "moved")
	}
	require.Error(t, writePreparationData(dir, "sub/record", []byte("preserved"), false))
	raw, err = os.ReadFile(filepath.Join(dir, "moved", "record"))
	require.NoError(t, err)
	assert.Equal(t, "preserved", string(raw))
}

func TestPreparationLeaseRejectsOtherOwnersAndSpecialEntries(t *testing.T) {
	dir := t.TempDir()
	release, err := AcquirePreparationLease(dir)
	require.NoError(t, err)
	_, err = AcquirePreparationLease(dir)
	require.ErrorContains(t, err, "already has an owner")
	release()
	release, err = AcquirePreparationLease(dir)
	require.NoError(t, err)
	release()
	_, err = AcquirePreparationLease(filepath.Join(dir, "missing"))
	require.Error(t, err)
	require.NoError(t, os.Remove(filepath.Join(dir, ".preparation.lock")))
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, ".preparation.lock"), 0o600))
	_, err = AcquirePreparationLease(dir)
	require.ErrorContains(t, err, "regular file")
	require.NoError(t, os.Remove(filepath.Join(dir, ".preparation.lock")))
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "outside"), filepath.Join(dir, ".preparation.lock")))
	_, err = AcquirePreparationLease(dir)
	require.Error(t, err)
}

func TestPreparationReceiptIsAnImmutableExactInvocationResult(t *testing.T) {
	dir := t.TempDir()
	st := preparationFixture(t)
	observation := preparationObservation(st, 0)
	require.NoError(t, SavePreparationReceipt(dir, observation))
	got, err := ReadPreparationReceipt(dir, observation.Invocation)
	require.NoError(t, err)
	assert.Equal(t, observation, *got)
	require.Error(t, SavePreparationReceipt(dir, observation), "an existing terminal result cannot be overwritten")
	wrong := observation.Invocation
	wrong.BindingHash = PreparationHash("different")
	_, err = ReadPreparationReceipt(dir, wrong)
	require.ErrorContains(t, err, "identity or result mismatch")
	_, err = ReadPreparationReceipt(t.TempDir(), observation.Invocation)
	require.ErrorIs(t, err, os.ErrNotExist)
	observation.Status = "unknown"
	require.Error(t, SavePreparationReceipt(t.TempDir(), observation))
	observation = preparationObservation(st, 0)
	observation.ContentHash = ""
	require.Error(t, SavePreparationReceipt(t.TempDir(), observation))
	blockedDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(blockedDir, ".investigation"), []byte("not a directory"), 0o600))
	require.Error(t, SavePreparationReceipt(blockedDir, preparationObservation(st, 0)))
}

func TestPreparationReceiptLoadingRejectsCorruptionInsteadOfReplaying(t *testing.T) {
	dir := t.TempDir()
	st := preparationFixture(t)
	observation := preparationObservation(st, 0)
	require.NoError(t, SavePreparationReceipt(dir, observation))
	file := preparationReceiptFile(dir, observation.Invocation.ID)
	valid := prepJSON(t, observation)
	for _, raw := range [][]byte{[]byte("{"), []byte(`{}`), []byte(strings.Replace(string(valid), `"duration-ms":1000`, `"duration-ms":1000.0`, 1)), bytes.Repeat([]byte(" "), (1<<20)+1)} {
		require.NoError(t, os.WriteFile(file, raw, 0o600))
		_, err := ReadPreparationReceipt(dir, observation.Invocation)
		require.Error(t, err)
	}
	observation.FinishedAt = "2026-11-11T00:00:00Z"
	require.NoError(t, os.WriteFile(file, prepJSON(t, observation), 0o600))
	_, err := ReadPreparationReceipt(dir, observation.Invocation)
	require.ErrorContains(t, err, "observation time")
	require.NoError(t, os.Remove(file))
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "external"), file))
	_, err = ReadPreparationReceipt(dir, observation.Invocation)
	require.Error(t, err)
}
