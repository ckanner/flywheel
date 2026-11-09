package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sessionTestRecord(taskDir, role, scope string) *SessionRecord {
	return &SessionRecord{
		Task: taskDir, Role: role, Scope: scope, Cwd: taskDir,
		Binding: strings.Repeat("a", 64), Goal: strings.Repeat("b", 64), Contract: strings.Repeat("c", 64),
		Generation: 1, ThreadID: "thread-one", Protocol: "test/v1", ResumeCandidate: true,
		Action: "create", Reason: "first invocation",
		Last: &SessionInvocation{
			ID: "call-one", Status: "received", StartedAt: "2026-11-07T12:00:00Z", Attempt: 1, Persistent: true,
			CandidateBefore: strings.Repeat("d", 64), CandidateAfter: strings.Repeat("e", 64),
			GuardBefore: strings.Repeat("a", 64), GuardAfter: strings.Repeat("b", 64),
			ReceiptDigest: strings.Repeat("f", 64), Dispatch: "dispatched", Outcome: "completed",
			CleanupConfirmed: true, UsageStatus: "complete",
		},
	}
}

func sessionTestState(t *testing.T, dir string) *State {
	t.Helper()
	st, err := Init("mytask", &contract.Spec{Milestones: []contract.Milestone{{ID: "M-01"}}},
		[]*contract.Packet{packet("M01-T01", "default")}, nil)
	require.NoError(t, err)
	st.Sessions = &SessionLedger{Version: 1, Records: map[string]*SessionRecord{
		"execute/M01-T01": sessionTestRecord(dir, "execute", "M01-T01"),
	}}
	return st
}

func TestSessionStateRoundTripAndLegacyLoad(t *testing.T) {
	dir := t.TempDir()
	st := sessionTestState(t, dir)
	saved, err := Save(dir, st, nil)
	require.NoError(t, err)
	loaded, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, saved.Sessions, loaded.Sessions)
	loaded.Sessions.Records["execute/M01-T01"].Last.Status = "consumed"
	loaded.Sessions.Records["execute/M01-T01"].Last.Accounted = true
	_, err = Save(dir, loaded, nil)
	require.NoError(t, err)
	loaded, err = Load(dir)
	require.NoError(t, err)
	assert.True(t, loaded.Sessions.Records["execute/M01-T01"].Last.Accounted)
	loaded.Sessions = nil
	_, err = Save(dir, loaded, nil)
	require.NoError(t, err)
	loaded, err = Load(dir)
	require.NoError(t, err)
	assert.Nil(t, loaded.Sessions, "legacy state does not opt into history implicitly")
}

func TestSessionStateRejectsMalformedOrContradictoryCheckpoints(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*SessionLedger)
	}{
		{"unknown version", func(s *SessionLedger) { s.Version = 3 }},
		{"nil records", func(s *SessionLedger) { s.Records = nil }},
		{"nil record", func(s *SessionLedger) { s.Records["execute/M01-T01"] = nil }},
		{"nil invocation", func(s *SessionLedger) { s.Records["execute/M01-T01"].Last = nil }},
		{"incorrect key", func(s *SessionLedger) {
			s.Records["review/M01-T01"] = s.Records["execute/M01-T01"]
			delete(s.Records, "execute/M01-T01")
		}},
		{"unaccounted consumed", func(s *SessionLedger) { s.Records["execute/M01-T01"].Last.Status = "consumed" }},
		{"accounted intent", func(s *SessionLedger) {
			s.Records["execute/M01-T01"].Last.Status = "intent"
			s.Records["execute/M01-T01"].Last.Accounted = true
		}},
		{"received unknown outcome", func(s *SessionLedger) { s.Records["execute/M01-T01"].Last.Outcome = "unknown" }},
		{"received no cleanup", func(s *SessionLedger) { s.Records["execute/M01-T01"].Last.CleanupConfirmed = false }},
		{"received unknown dispatch", func(s *SessionLedger) { s.Records["execute/M01-T01"].Last.Dispatch = "unknown" }},
		{"shared thread", func(s *SessionLedger) { s.Records["review/M01-T01"] = sessionTestRecord("task", "review", "M01-T01") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			st := sessionTestState(t, dir)
			tc.change(st.Sessions)
			raw, err := json.Marshal(st)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(StateFile(dir), raw, 0o600))
			_, err = Load(dir)
			require.Error(t, err, "invalid session state must fail before dispatch")
		})
	}
	for _, raw := range []string{
		`{"revision":0,"sessions":null}`,
		`{"revision":0,"sessions":{"version":1,"records":{},"unknown":true}}`,
	} {
		dir := t.TempDir()
		writeRaw(t, StateFile(dir), raw)
		_, err := Load(dir)
		require.Error(t, err)
	}
	st := sessionTestState(t, t.TempDir())
	raw, err := json.Marshal(st.Sessions)
	require.NoError(t, err)
	raw = bytes.Replace(raw, []byte(`"generation":1`), []byte(`"generation":1e30`), 1)
	require.Error(t, validateSessions(raw), "schema-valid numbers outside Go's integer range must not panic")
}

func TestSessionStateKeepsDistinctBackendNamespacesAndNoThread(t *testing.T) {
	st := sessionTestState(t, t.TempDir())
	st.Sessions.Records["review/M01-T01"] = sessionTestRecord("task", "review", "M01-T01")
	st.Sessions.Records["review/M01-T01"].Protocol = "another-backend/v1"
	st.Sessions.Records["judge/M-01"] = sessionTestRecord("task", "judge", "M-01")
	st.Sessions.Records["judge/M-01"].ThreadID = ""
	st.Sessions.Records["judge/M-01"].ResumeCandidate = false
	raw, err := json.Marshal(st.Sessions)
	require.NoError(t, err)
	require.NoError(t, validateSessions(raw))
}

func sessionTestReceipt(dir, key string) SessionReceipt {
	return SessionReceipt{Version: 1, InvocationID: "call-one", Key: key, Task: dir, Cwd: dir,
		Binding: strings.Repeat("a", 64), CandidateAfter: strings.Repeat("b", 64),
		GuardBefore: strings.Repeat("c", 64), GuardAfter: strings.Repeat("d", 64),
		Report: []byte(`{"done":true}`), Result: json.RawMessage(`{"output":"normal result","session":{"requestId":"call-one"}}`)}
}

func TestSessionReceiptAtomicRoundTripAndBoundedSlot(t *testing.T) {
	dir, key := t.TempDir(), "execute/M01-T01"
	want := sessionTestReceipt(dir, key)
	digest, err := WriteSessionReceipt(dir, key, want)
	require.NoError(t, err)
	path := sessionReceiptPath(dir, key)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%x", sha256.Sum256(raw)), digest)
	actual, gotDigest, err := ReadSessionReceipt(dir, key, want.InvocationID, digest)
	require.NoError(t, err)
	assert.Equal(t, want, *actual)
	assert.Equal(t, digest, gotDigest)
	_, recoveredDigest, err := ReadSessionReceipt(dir, key, want.InvocationID, "")
	require.NoError(t, err)
	assert.Equal(t, digest, recoveredDigest, "intent recovery discovers its digest only from an identity-checked receipt")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	info, err = os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	want.InvocationID, want.Report = "call-two", []byte(`{"done":"next"}`)
	nextDigest, err := WriteSessionReceipt(dir, key, want)
	require.NoError(t, err)
	assert.NotEqual(t, digest, nextDigest)
	actual, _, err = ReadSessionReceipt(dir, key, want.InvocationID, nextDigest)
	require.NoError(t, err)
	assert.Equal(t, want, *actual)
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1, "replacement retains one scope slot and no abandoned temporary file")
	_, _, err = ReadSessionReceipt(dir, key, "call-one", digest)
	require.Error(t, err, "a stale invocation cannot consume the replacement")
}

func restoreSessionIO(t *testing.T) {
	t.Helper()
	openDir, mkdir, openFile := openSessionReceiptDirectory, mkdirSessionReceiptDirectory, openSessionReceiptFile
	read, write, sync, close, rename := readSessionReceipt, writeSessionReceipt, syncSessionReceipt, closeSessionReceipt, renameSessionReceipt
	t.Cleanup(func() {
		openSessionReceiptDirectory, mkdirSessionReceiptDirectory, openSessionReceiptFile = openDir, mkdir, openFile
		readSessionReceipt, writeSessionReceipt, syncSessionReceipt, closeSessionReceipt, renameSessionReceipt = read, write, sync, close, rename
	})
}

func TestSessionReceiptFailuresBeforeRenameKeepPreviousBytes(t *testing.T) {
	for _, failure := range []string{"mkdir", "parent-sync", "marshal", "too-large", "open-temp", "partial-write", "file-sync", "close", "rename"} {
		t.Run(failure, func(t *testing.T) {
			dir, key := t.TempDir(), "execute/M01-T01"
			receipt := sessionTestReceipt(dir, key)
			digest, err := WriteSessionReceipt(dir, key, receipt)
			require.NoError(t, err)
			path := sessionReceiptPath(dir, key)
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			restoreSessionIO(t)
			fault := errors.New("injected storage failure")
			receipt.InvocationID = "call-two"
			switch failure {
			case "mkdir":
				mkdirSessionReceiptDirectory = func(*os.Root, string, os.FileMode) error { return fault }
			case "parent-sync":
				syncSessionReceipt = func(*os.File) error { return fault }
			case "marshal":
				receipt.Result = json.RawMessage(`{broken`)
			case "too-large":
				receipt.Report = make([]byte, maxSessionReceiptSize/4*3)
			case "open-temp":
				openSessionReceiptFile = func(*os.Root, string, int, os.FileMode) (*os.File, error) { return nil, fault }
			case "partial-write":
				writeSessionReceipt = func(f *os.File, p []byte) (int, error) { n, _ := f.Write(p[:len(p)/2]); return n, fault }
			case "file-sync":
				original := syncSessionReceipt
				syncSessionReceipt = func(f *os.File) error {
					info, err := f.Stat()
					require.NoError(t, err)
					if info.IsDir() {
						return original(f)
					}
					return fault
				}
			case "close":
				closeSessionReceipt = func(*os.File) error { return fault }
			case "rename":
				renameSessionReceipt = func(*os.Root, string, string) error { return fault }
			}
			gotDigest, err := WriteSessionReceipt(dir, key, receipt)
			require.Error(t, err)
			assert.Empty(t, gotDigest)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, before, after, "a failed replacement must preserve the only durable prior receipt")
			assert.Equal(t, digest, fmt.Sprintf("%x", sha256.Sum256(after)))
			entries, err := os.ReadDir(filepath.Dir(path))
			require.NoError(t, err)
			assert.Len(t, entries, 1, "temporary partial writes are removed")
		})
	}
}

func TestSessionReceiptPostRenameSyncFailureDoesNotClaimDurability(t *testing.T) {
	dir, key := t.TempDir(), "execute/M01-T01"
	restoreSessionIO(t)
	original, count := syncSessionReceipt, 0
	syncSessionReceipt = func(f *os.File) error {
		count++
		if count == 3 {
			return errors.New("directory sync failed after rename")
		}
		return original(f)
	}
	receipt := sessionTestReceipt(dir, key)
	digest, err := WriteSessionReceipt(dir, key, receipt)
	require.ErrorContains(t, err, "directory sync failed")
	assert.Empty(t, digest)
	actual, _, err := ReadSessionReceipt(dir, key, receipt.InvocationID, "")
	require.NoError(t, err, "recovery must check the surviving slot instead of replaying the model call")
	assert.Equal(t, receipt, *actual)
}

func TestSessionReceiptDirectoryErrorsAndReplacement(t *testing.T) {
	dir := t.TempDir()
	_, err := sessionReceiptDirectory(filepath.Join(dir, "missing"), false)
	require.Error(t, err)
	_, _, err = ReadSessionReceipt(dir, "scope", "call-one", "")
	require.Error(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".sessions"), []byte("not a directory"), 0o600))
	_, err = WriteSessionReceipt(dir, "scope", sessionTestReceipt(dir, "scope"))
	require.ErrorContains(t, err, "real directory")
	for _, failure := range []string{"open", "closed-root", "changed-directory"} {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(dir, ".sessions"), 0o700))
			require.NoError(t, os.Mkdir(filepath.Join(dir, "another"), 0o700))
			restoreSessionIO(t)
			original := openSessionReceiptDirectory
			openSessionReceiptDirectory = func(root *os.Root, name string) (*os.Root, error) {
				if failure == "open" {
					return nil, errors.New("directory open failed")
				}
				if failure == "changed-directory" {
					return original(root, "another")
				}
				closed, err := original(root, name)
				require.NoError(t, err)
				require.NoError(t, closed.Close())
				return closed, nil
			}
			_, err := sessionReceiptDirectory(dir, false)
			require.Error(t, err)
		})
	}
	root, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, root.Close())
	require.Error(t, syncSessionDirectory(root))
}

func TestSessionReceiptPathsDoNotFollowSymlinksOrBlockOnSpecialFiles(t *testing.T) {
	for _, kind := range []string{"outside-directory-link", "inside-directory-link", "outside-file-link", "inside-file-link", "directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			dir, outside, key := t.TempDir(), t.TempDir(), "execute/M01-T01"
			path := sessionReceiptPath(dir, key)
			require.NoError(t, os.WriteFile(filepath.Join(outside, "untouched"), []byte("sentinel"), 0o600))
			if kind == "outside-directory-link" || kind == "inside-directory-link" {
				target := outside
				if kind == "inside-directory-link" {
					target = "inside"
					require.NoError(t, os.Mkdir(filepath.Join(dir, target), 0o700))
				}
				require.NoError(t, os.Symlink(target, filepath.Dir(path)))
				_, err := WriteSessionReceipt(dir, key, sessionTestReceipt(dir, key))
				require.Error(t, err)
			} else {
				require.NoError(t, os.Mkdir(filepath.Dir(path), 0o700))
				switch kind {
				case "outside-file-link":
					require.NoError(t, os.Symlink(filepath.Join(outside, "untouched"), path))
				case "inside-file-link":
					require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(path), "inside"), []byte("sentinel"), 0o600))
					require.NoError(t, os.Symlink("inside", path))
				case "directory":
					require.NoError(t, os.Mkdir(path, 0o700))
				case "fifo":
					require.NoError(t, syscall.Mkfifo(path, 0o600))
				}
			}
			_, _, err := ReadSessionReceipt(dir, key, "call-one", "")
			require.Error(t, err)
			raw, err := os.ReadFile(filepath.Join(outside, "untouched"))
			require.NoError(t, err)
			assert.Equal(t, "sentinel", string(raw))
		})
	}
}

func TestSessionReceiptWriteDoesNotFollowPredictableTempOrFinalLink(t *testing.T) {
	dir, key := t.TempDir(), "../../outside/attempt"
	path := sessionReceiptPath(dir, key)
	require.Equal(t, filepath.Join(dir, ".sessions"), filepath.Dir(path), "opaque keys are hashed, never interpreted as paths")
	require.NoError(t, os.Mkdir(filepath.Dir(path), 0o700))
	outside := filepath.Join(t.TempDir(), "untouched")
	require.NoError(t, os.WriteFile(outside, []byte("sentinel"), 0o600))
	require.NoError(t, os.Symlink(outside, path+".tmp"))
	require.NoError(t, os.Symlink(outside, path))
	receipt := sessionTestReceipt(dir, key)
	digest, err := WriteSessionReceipt(dir, key, receipt)
	require.NoError(t, err)
	_, _, err = ReadSessionReceipt(dir, key, receipt.InvocationID, digest)
	require.NoError(t, err)
	raw, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "sentinel", string(raw), "neither the predictable temp nor the old final link may be followed for writing")
}

func TestSessionReceiptRejectsWrongIdentityDigestAndMalformedEnvelope(t *testing.T) {
	for _, change := range []struct {
		name string
		edit func(*SessionReceipt)
	}{
		{"version", func(r *SessionReceipt) { r.Version = 2 }},
		{"key", func(r *SessionReceipt) { r.Key = "review/M01-T01" }},
		{"invocation", func(r *SessionReceipt) { r.InvocationID = "call-other" }},
		{"empty invocation", func(r *SessionReceipt) { r.InvocationID = "" }},
		{"task", func(r *SessionReceipt) { r.Task = "another-task" }},
		{"cwd", func(r *SessionReceipt) { r.Cwd = "" }},
		{"binding", func(r *SessionReceipt) { r.Binding = "" }},
		{"null result", func(r *SessionReceipt) { r.Result = nil }},
		{"nonobject result", func(r *SessionReceipt) { r.Result = json.RawMessage(`[]`) }},
	} {
		t.Run(change.name, func(t *testing.T) {
			dir, key := t.TempDir(), "execute/M01-T01"
			receipt := sessionTestReceipt(dir, key)
			change.edit(&receipt)
			_, err := WriteSessionReceipt(dir, key, receipt)
			require.NoError(t, err)
			_, _, err = ReadSessionReceipt(dir, key, "call-one", "")
			require.ErrorContains(t, err, "identity mismatch")
		})
	}
	dir, key := t.TempDir(), "execute/M01-T01"
	receipt := sessionTestReceipt(dir, key)
	_, err := WriteSessionReceipt(dir, key, receipt)
	require.NoError(t, err)
	_, _, err = ReadSessionReceipt(dir, key, receipt.InvocationID, strings.Repeat("0", 64))
	require.ErrorContains(t, err, "digest mismatch")
	valid, err := json.Marshal(receipt)
	require.NoError(t, err)
	for _, raw := range [][]byte{[]byte(`{broken`), append([]byte(`{"unrecognized":true,`), valid[1:]...), append(append([]byte{}, valid...), []byte(` {}`)...)} {
		require.NoError(t, os.WriteFile(sessionReceiptPath(dir, key), raw, 0o600))
		_, _, err = ReadSessionReceipt(dir, key, receipt.InvocationID, "")
		require.Error(t, err)
	}
}

func TestSessionReceiptCheckpointFieldsRequireCanonicalHashesOrAbsence(t *testing.T) {
	for _, field := range []string{"candidate", "guard-before", "guard-after"} {
		for _, proof := range []string{"", "short", strings.Repeat("g", 64), strings.Repeat("A", 64), strings.Repeat("e", 64)} {
			t.Run(field+"/"+proof, func(t *testing.T) {
				dir, key := t.TempDir(), "execute/M01-T01"
				receipt := sessionTestReceipt(dir, key)
				switch field {
				case "candidate":
					receipt.CandidateAfter = proof
				case "guard-before":
					receipt.GuardBefore = proof
				case "guard-after":
					receipt.GuardAfter = proof
				}
				_, err := WriteSessionReceipt(dir, key, receipt)
				require.NoError(t, err)
				_, _, err = ReadSessionReceipt(dir, key, receipt.InvocationID, "")
				if proof == "" || proof == strings.Repeat("e", 64) {
					require.NoError(t, err, "absent post-call proof must remain distinguishable from malformed evidence")
				} else {
					require.Error(t, err, "invalid checkpoints must not be copied into schema-validated state during recovery")
				}
			})
		}
	}
}

func TestSessionReceiptReadFailuresAndGrowthAreBounded(t *testing.T) {
	for _, failure := range []string{"missing", "stat", "oversized", "read", "growth"} {
		t.Run(failure, func(t *testing.T) {
			dir, key := t.TempDir(), "execute/M01-T01"
			receipt := sessionTestReceipt(dir, key)
			_, err := WriteSessionReceipt(dir, key, receipt)
			require.NoError(t, err)
			path := sessionReceiptPath(dir, key)
			restoreSessionIO(t)
			switch failure {
			case "missing":
				require.NoError(t, os.Remove(path))
			case "stat":
				original := openSessionReceiptFile
				openSessionReceiptFile = func(root *os.Root, name string, flags int, mode os.FileMode) (*os.File, error) {
					f, err := original(root, name, flags, mode)
					require.NoError(t, err)
					require.NoError(t, f.Close())
					return f, nil
				}
			case "oversized":
				require.NoError(t, os.Truncate(path, maxSessionReceiptSize+1))
			case "read":
				readSessionReceipt = func(io.Reader) ([]byte, error) { return nil, errors.New("read failed") }
			case "growth":
				original := readSessionReceipt
				readSessionReceipt = func(r io.Reader) ([]byte, error) {
					limit, ok := r.(*io.LimitedReader)
					require.True(t, ok)
					assert.Equal(t, int64(maxSessionReceiptSize+1), limit.N)
					require.NoError(t, os.Truncate(path, 2*maxSessionReceiptSize))
					raw, err := original(r)
					assert.Len(t, raw, maxSessionReceiptSize+1, "growth after fstat cannot become an unbounded allocation")
					return raw, err
				}
			}
			got, digest, err := ReadSessionReceipt(dir, key, receipt.InvocationID, "")
			require.Error(t, err)
			assert.Nil(t, got)
			assert.Empty(t, digest)
		})
	}
}

func TestLoadRejectsInvalidConsoleExtensionsBeforeAnyControlMutation(t *testing.T) {
	for _, name := range []string{"handoff", "delivery", "scheduler", "run", "control", "project"} {
		dir := t.TempDir()
		writeRaw(t, StateFile(dir), `{"revision":1,"`+name+`":{}}`)
		_, err := Load(dir)
		require.Error(t, err, name)
	}
	dir := t.TempDir()
	writeRaw(t, StateFile(dir), `{"revision":1,"project":{"cwd":"/repo"},"scheduler":{"mode":"serial","max-parallel":2}}`)
	_, err := Load(dir)
	require.NoError(t, err)
}
