package shadow

import (
	"context"
	"errors"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRejectsUnsafeRootsAndIdentityChanges(t *testing.T) {
	dir := privateDir(t)
	file := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(file, nil, 0600))
	for _, entry := range []struct {
		path   string
		create bool
	}{{file + "/child", true}, {file, false}, {filepath.Join(dir, "missing"), false}, {t.TempDir(), false}} {
		_, err := openStore(entry.path, entry.create)
		require.Error(t, err)
	}
	previous := storeRootOpen
	t.Cleanup(func() { storeRootOpen = previous })
	storeRootOpen = func(string) (*os.Root, error) { return nil, os.ErrPermission }
	_, err := openStore(dir, false)
	require.Error(t, err)
	storeRootOpen = previous
	original := storeRootStat
	t.Cleanup(func() { storeRootStat = original })
	storeRootStat = func(*os.Root, string) (os.FileInfo, error) { return nil, os.ErrPermission }
	_, err = openStore(dir, false)
	require.Error(t, err)
	storeRootStat = func(*os.Root, string) (os.FileInfo, error) { return os.Stat(file) }
	_, err = openStore(dir, false)
	require.Error(t, err)
}

func TestStoreDurabilityFailuresNeverReplayOrOverwrite(t *testing.T) {
	for _, failure := range []string{"open", "write", "sync", "close", "link", "directory-open", "directory-sync"} {
		t.Run(failure, func(t *testing.T) {
			root, err := openStore(privateDir(t), false)
			require.NoError(t, err)
			defer root.Close()
			open, write, sync, close, link := storeOpen, storeWrite, storeSync, storeClose, storeLink
			t.Cleanup(func() { storeOpen, storeWrite, storeSync, storeClose, storeLink = open, write, sync, close, link })
			storeOpen = func(r *os.Root, n string, f int, p os.FileMode) (*os.File, error) {
				if failure == "open" || failure == "directory-open" && n == "." {
					return nil, os.ErrPermission
				}
				return open(r, n, f, p)
			}
			storeWrite = func(f *os.File, p []byte) (int, error) {
				if failure == "write" {
					return 0, io.ErrShortWrite
				}
				return write(f, p)
			}
			storeSync = func(f *os.File) error {
				if failure == "sync" || failure == "directory-sync" && filepath.Base(f.Name()) == "." {
					return os.ErrPermission
				}
				return sync(f)
			}
			storeClose = func(f *os.File) error {
				if failure == "close" {
					return os.ErrPermission
				}
				return close(f)
			}
			storeLink = func(r *os.Root, a, b string) error {
				if failure == "link" {
					return os.ErrPermission
				}
				return link(r, a, b)
			}
			require.Error(t, publish(root, "result.json", []byte("result")))
			if failure != "link" {
				_, err := reserve(root, recordFor(sample().Cases[0], targetFor(opts("https://example.org/v1/systemone"))))
				require.Error(t, err)
			}
		})
	}
	root, err := openStore(privateDir(t), false)
	require.NoError(t, err)
	defer root.Close()
	require.NoError(t, publish(root, "result.json", []byte("first")))
	require.Error(t, publish(root, "result.json", []byte("second")))
	raw, _, err := readFile(root, "result.json")
	require.NoError(t, err)
	require.Equal(t, "first", string(raw))
	previous := storeOpen
	t.Cleanup(func() { storeOpen = previous })
	storeOpen = func(*os.Root, string, int, os.FileMode) (*os.File, error) { return nil, os.ErrExist }
	require.Error(t, publish(root, "never.json", nil))
}

func TestCorruptRecordsAndManifestsCannotProduceAdvice(t *testing.T) {
	o := opts("https://example.org/v1/systemone")
	target := targetFor(o)
	data := sample()
	for _, scenario := range []string{"missing-target", "bad-target", "digest-target", "unreadable-target", "wrong-target", "bad-result", "wrong-result", "pending-result", "unsafe-result", "unsafe-pending", "wrong-semantics"} {
		t.Run(scenario, func(t *testing.T) {
			dir := privateDir(t)
			root, err := openStore(dir, false)
			require.NoError(t, err)
			defer root.Close()
			manifest := jsonBytes(target)
			switch scenario {
			case "missing-target":
			case "bad-target":
				manifest = []byte("{")
			case "digest-target":
				bad := target
				bad.QuestionDigest = strings.Repeat("0", 64)
				manifest = jsonBytes(bad)
			}
			if scenario != "missing-target" {
				require.NoError(t, writeFile(root, "target.json", manifest))
			}
			if scenario == "unreadable-target" {
				require.NoError(t, root.Chmod("target.json", 0644))
			}
			if scenario == "wrong-target" {
				different := target
				different.Model = "other"
				require.Error(t, installTarget(root, different))
				return
			}
			rec := recordFor(data.Cases[0], target)
			rec.Status = "ok"
			rec.Model = o.ExpectedModel
			rec.HTTPStatus = 200
			rec.Probabilities = probabilities()
			rec.Advice, rec.Reason = advice(rec.Probabilities, policy)
			raw := jsonBytes(rec)
			switch scenario {
			case "bad-result":
				raw = []byte("{")
			case "wrong-result":
				rec.ActualMode = "full"
				raw = jsonBytes(rec)
			case "pending-result":
				rec.Status = "pending"
				raw = jsonBytes(rec)
			case "wrong-semantics":
				rec.Reason = "wrong"
				raw = jsonBytes(rec)
			case "unsafe-pending":
				require.NoError(t, root.Symlink("target.json", rec.Key+".pending"))
			}
			if strings.Contains(scenario, "result") || scenario == "wrong-semantics" {
				require.NoError(t, writeFile(root, rec.Key+".json", raw))
			}
			if scenario == "unsafe-result" {
				require.NoError(t, root.Chmod(rec.Key+".json", 0644))
			}
			_, err = ReadReport(data, dir)
			require.Error(t, err)
			if scenario != "missing-target" && scenario != "wrong-semantics" {
				_, err = Run(context.Background(), data, dir, o)
				require.Error(t, err)
			}
		})
	}
}

func TestReadFailuresAndCaptureIdentity(t *testing.T) {
	dir := privateDir(t)
	root, err := openStore(dir, false)
	require.NoError(t, err)
	defer root.Close()
	old := storeLstat
	t.Cleanup(func() { storeLstat = old })
	storeLstat = func(*os.Root, string) (os.FileInfo, error) { return nil, os.ErrPermission }
	_, _, err = readFile(root, "target.json")
	require.Error(t, err)
	require.Error(t, installTarget(root, targetFor(opts("https://example.org/v1/systemone"))))
	require.Error(t, installDataset(root, sample()))
	storeLstat = old
	require.NoError(t, writeFile(root, "target.json", []byte("{}")))
	open := storeOpen
	t.Cleanup(func() { storeOpen = open })
	storeOpen = func(*os.Root, string, int, os.FileMode) (*os.File, error) { return nil, os.ErrPermission }
	_, _, err = readFile(root, "target.json")
	require.Error(t, err)
	storeOpen = open
	read := storeRead
	t.Cleanup(func() { storeRead = read })
	storeRead = func(io.Reader) ([]byte, error) { return nil, io.ErrUnexpectedEOF }
	_, err = readTarget(root)
	require.Error(t, err)
	storeRead = read
	data := sample()
	require.NoError(t, installDataset(root, data))
	require.NoError(t, installDataset(root, data))
	data.Cases[0].Task = "Different"
	require.Error(t, installDataset(root, data))
	_, err = ReadReport(Dataset{}, dir)
	require.Error(t, err)
	_, err = ReadReport(sample(), filepath.Join(dir, "missing"))
	require.Error(t, err)
	assert.EqualValues(t, 2, *summarize([]Row{{Record: &Record{Status: "ok", DurationMS: 1}}, {Record: &Record{Status: "ok", DurationMS: 3}}}).P50MS)
	assert.Equal(t, 1, summarize([]Row{{Source: Source{Kind: "historical"}, Baseline: Baseline{Mode: "unknown"}}}).NotRun)
}

func TestRunJournalFaultsBeforeAndAfterDispatch(t *testing.T) {
	for _, failAt := range []string{"root", "manifest", "preflight", "second-read", "reserve", "publish", "report", "capture"} {
		t.Run(failAt, func(t *testing.T) {
			dir := privateDir(t)
			o := opts("https://example.org/v1/systemone")
			o.SaveInput = failAt == "capture"
			dispatched := 0
			o.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { dispatched++; return nil, errors.New("offline") })
			root, err := openStore(dir, false)
			require.NoError(t, err)
			if failAt != "manifest" {
				require.NoError(t, installTarget(root, targetFor(o)))
			}
			root.Close()
			if failAt == "root" {
				dir = filepath.Join(dir, "missing-parent-file")
				require.NoError(t, os.WriteFile(dir, nil, 0600))
			}
			lstat, open, link := storeLstat, storeOpen, storeLink
			t.Cleanup(func() { storeLstat, storeOpen, storeLink = lstat, open, link })
			checks := 0
			storeLstat = func(r *os.Root, n string) (os.FileInfo, error) {
				if strings.HasSuffix(n, ".json") && n != "target.json" && n != "cases.json" {
					checks++
					if failAt == "preflight" && checks == 1 || failAt == "second-read" && checks == 2 || failAt == "report" && checks == 3 {
						return nil, os.ErrPermission
					}
				}
				return lstat(r, n)
			}
			storeOpen = func(r *os.Root, n string, f int, p os.FileMode) (*os.File, error) {
				if failAt == "reserve" && strings.HasSuffix(n, ".pending") {
					return nil, os.ErrPermission
				}
				return open(r, n, f, p)
			}
			storeLink = func(r *os.Root, a, b string) error {
				if failAt == "manifest" && b == "target.json" || failAt == "capture" && b == "cases.json" || failAt == "publish" && strings.HasSuffix(b, ".json") {
					return os.ErrPermission
				}
				return link(r, a, b)
			}
			_, err = Run(context.Background(), sample(), dir, o)
			require.Error(t, err)
			if failAt == "publish" || failAt == "report" {
				require.Equal(t, 1, dispatched)
			} else {
				require.Zero(t, dispatched)
			}
		})
	}
}

func TestCaptureAndLostReservationRace(t *testing.T) {
	dir := contractFixture(t)
	s, err := readSnapshot(dir)
	require.NoError(t, err)
	selection, err := state.ResolveWorkflow(nil, s.Spec, s.Packets, "")
	require.NoError(t, err)
	data, err := Capture(dir, s.Spec, s.Packets, selection)
	require.NoError(t, err)
	require.Equal(t, "decision-time", data.Cases[0].Source.Kind)
	require.Equal(t, "quick", data.Cases[0].Baseline.Mode)
	o := opts("https://example.org/v1/systemone")
	base := privateDir(t)
	dest := ObservationDirectory(base, data, o)
	require.NotEqual(t, base, dest)
	_, err = Run(context.Background(), Dataset{}, base, o)
	require.Error(t, err)
	open := storeOpen
	t.Cleanup(func() { storeOpen = open })
	storeOpen = func(root *os.Root, n string, f int, p os.FileMode) (*os.File, error) {
		if strings.HasSuffix(n, ".pending") {
			file, err := open(root, n, f, p)
			require.NoError(t, err)
			file.Close()
			return nil, os.ErrExist
		}
		return open(root, n, f, p)
	}
	report, err := Run(context.Background(), data, dest, o)
	require.NoError(t, err)
	require.Equal(t, 1, report.Summary.Unknown)
	require.Zero(t, report.NewCalls)
}

func TestEncodedCredentialsAndFalseSkipAreRejected(t *testing.T) {
	raw := jsonBytes(sample())
	encoded := strings.Replace(string(raw), "Fix the empty list display", `\u0073k-abcdefghijklmnopqrstuvwxyz0123456789`, 1)
	_, err := ParseDataset([]byte(encoded))
	require.ErrorContains(t, err, "encoded credential")
	require.Error(t, validate([]byte{'"', 255, '"'}, "dataset"))
	dir := privateDir(t)
	root, err := openStore(dir, false)
	require.NoError(t, err)
	defer root.Close()
	target := targetFor(opts("https://example.org/v1/systemone"))
	require.NoError(t, installTarget(root, target))
	rec := recordFor(sample().Cases[0], target)
	rec.Status = "skipped"
	rec.Advice = "full"
	rec.Reason = "fabricated blocker"
	require.NoError(t, publish(root, rec.Key+".json", jsonBytes(rec)))
	_, err = ReadReport(sample(), dir)
	require.ErrorContains(t, err, "blockers")
	rec.Status = "error"
	rec.Advice = "quick"
	require.Error(t, validate(jsonBytes(rec), "record"))
}
