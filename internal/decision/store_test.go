package decision

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func privateJournal(t *testing.T) (*os.Root, string) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "journal")
	root, err := openJournal(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return root, directory
}

func TestJournalRootIdentityAndPrivacy(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := openJournal(filepath.Join(file, "child")); err == nil {
		t.Fatal("non-directory root accepted")
	}
	if _, err := openJournal(directory); err == nil {
		t.Fatal("non-private directory accepted")
	}
	for _, scenario := range []string{"lstat", "open", "stat", "changed"} {
		t.Run(scenario, func(t *testing.T) {
			rootOpen, dirStat, stat := journalRoot, journalDirStat, journalStat
			t.Cleanup(func() { journalRoot, journalDirStat, journalStat = rootOpen, dirStat, stat })
			switch scenario {
			case "lstat":
				journalDirStat = func(string) (os.FileInfo, error) { return nil, os.ErrPermission }
			case "open":
				journalRoot = func(string) (*os.Root, error) { return nil, os.ErrPermission }
			case "stat":
				journalStat = func(*os.Root, string) (os.FileInfo, error) { return nil, os.ErrPermission }
			case "changed":
				journalStat = func(*os.Root, string) (os.FileInfo, error) { return os.Stat(file) }
			}
			if _, err := openJournal(filepath.Join(directory, scenario)); err == nil {
				t.Fatal("root fault accepted")
			}
		})
	}
}

func TestJournalWriteFailuresPreserveEvidence(t *testing.T) {
	for _, scenario := range []string{"open", "write", "short-write", "sync", "link", "directory-open", "directory-sync"} {
		t.Run(scenario, func(t *testing.T) {
			root, _ := privateJournal(t)
			open, write, sync, link := journalOpen, journalWrite, journalSync, journalLink
			t.Cleanup(func() { journalOpen, journalWrite, journalSync, journalLink = open, write, sync, link })
			journalOpen = func(root *os.Root, name string, flag int, mode os.FileMode) (*os.File, error) {
				if scenario == "open" || scenario == "directory-open" && name == "." {
					return nil, os.ErrPermission
				}
				return open(root, name, flag, mode)
			}
			journalWrite = func(file *os.File, data []byte) (int, error) {
				if scenario == "write" {
					return 0, io.ErrShortWrite
				}
				if scenario == "short-write" {
					return 0, nil
				}
				return write(file, data)
			}
			journalSync = func(file *os.File) error {
				if scenario == "sync" || scenario == "directory-sync" && filepath.Base(file.Name()) == "." {
					return os.ErrPermission
				}
				return sync(file)
			}
			journalLink = func(root *os.Root, from, to string) error {
				if scenario == "link" {
					return os.ErrPermission
				}
				return link(root, from, to)
			}
			if err := publish(root, "record.json", []byte("record")); err == nil {
				t.Fatal("durability fault ignored")
			}
			if err := install(root, "input.json", []byte("input")); err == nil {
				t.Fatal("input durability fault ignored")
			}
		})
	}
}

func TestJournalReadAndIdentityFailures(t *testing.T) {
	root, directory := privateJournal(t)
	if err := exclusive(root, "input", []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := install(root, "input", []byte("different")); err == nil {
		t.Fatal("existing input overwritten")
	}
	for _, scenario := range []string{"lstat", "open", "read", "oversize", "unsafe"} {
		t.Run(scenario, func(t *testing.T) {
			lstat, open, reader := journalLstat, journalOpen, journalRead
			t.Cleanup(func() { journalLstat, journalOpen, journalRead = lstat, open, reader })
			switch scenario {
			case "lstat":
				journalLstat = func(*os.Root, string) (os.FileInfo, error) { return nil, os.ErrPermission }
			case "open":
				journalOpen = func(*os.Root, string, int, os.FileMode) (*os.File, error) { return nil, os.ErrPermission }
			case "read":
				journalRead = func(io.Reader) ([]byte, error) { return nil, io.ErrUnexpectedEOF }
			case "oversize":
				journalRead = func(io.Reader) ([]byte, error) { return make([]byte, 2*maxDocument+1), nil }
			case "unsafe":
				if err := os.Chmod(filepath.Join(directory, "input"), 0644); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(filepath.Join(directory, "input"), 0600) })
			}
			if _, _, err := read(root, "input"); err == nil {
				t.Fatal("unsafe or unreadable input accepted")
			}
			if scenario == "lstat" || scenario == "read" {
				if err := install(root, "input", []byte("first")); err == nil {
					t.Fatal("failed input identity read accepted")
				}
			}
		})
	}
	s, p, o := fixture()
	expected := receiptFor(s, p, o)
	if err := exclusive(root, "bad.json", []byte("{")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := load(root, "bad", expected); err == nil {
		t.Fatal("corrupt terminal accepted")
	}
	if err := root.Symlink("input", "unsafe.pending"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := load(root, "unsafe", expected); err == nil {
		t.Fatal("symlink intent accepted")
	}
	lstat := journalLstat
	t.Cleanup(func() { journalLstat = lstat })
	journalLstat = func(root *os.Root, name string) (os.FileInfo, error) {
		if strings.HasSuffix(name, ".pending") {
			return nil, os.ErrPermission
		}
		return lstat(root, name)
	}
	if _, _, err := load(root, "unreadable", expected); err == nil {
		t.Fatal("unreadable intent accepted")
	}
}

func TestEvaluateJournalFaultsDoNotAuthorizeReplay(t *testing.T) {
	for _, failure := range []string{"root", "input", "read", "reserve", "lost-reservation", "intent-sync", "slot", "result"} {
		t.Run(failure, func(t *testing.T) {
			s, p, o := fixture()
			_, directory := privateJournal(t)
			calls := 0
			o.Transport = transport(&calls)
			open, link, sync, lstat := journalOpen, journalLink, journalSync, journalLstat
			t.Cleanup(func() { journalOpen, journalLink, journalSync, journalLstat = open, link, sync, lstat })
			reserved := false
			if failure == "root" {
				directory = filepath.Join(directory, "file")
				if err := os.WriteFile(directory, nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			journalOpen = func(root *os.Root, name string, flag int, mode os.FileMode) (*os.File, error) {
				if strings.HasSuffix(name, ".pending") {
					reserved = true
					if failure == "reserve" {
						return nil, os.ErrPermission
					}
					if failure == "lost-reservation" {
						return nil, os.ErrExist
					}
				}
				if failure == "slot" && strings.HasPrefix(name, "call-") {
					return nil, os.ErrPermission
				}
				return open(root, name, flag, mode)
			}
			journalLink = func(root *os.Root, from, to string) error {
				if failure == "input" && strings.HasSuffix(to, ".input.json") || failure == "result" && to == journalKey(s)+".json" {
					return os.ErrPermission
				}
				return link(root, from, to)
			}
			journalSync = func(file *os.File) error {
				if failure == "intent-sync" && reserved && filepath.Base(file.Name()) == "." {
					return os.ErrPermission
				}
				return sync(file)
			}
			journalLstat = func(root *os.Root, name string) (os.FileInfo, error) {
				if failure == "read" && name == journalKey(s)+".json" {
					return nil, os.ErrPermission
				}
				return lstat(root, name)
			}
			r, err := Evaluate(context.Background(), directory, s, p, o)
			if failure == "lost-reservation" {
				if err != nil || r.Status != "unknown" {
					t.Fatalf("lost reservation was replayed: %#v, %v", r, err)
				}
			} else if err == nil {
				t.Fatal("journal failure ignored")
			}
			if failure == "result" {
				journalOpen, journalLink, journalSync, journalLstat = open, link, sync, lstat
				again, err := Evaluate(context.Background(), directory, s, p, o)
				if err != nil || again.Status != "unknown" || again.Usage.Input != nil || calls != 1 {
					t.Fatalf("lost terminal replay: %#v, %v, calls=%d", again, err, calls)
				}
			} else if calls != 0 {
				t.Fatalf("dispatched through journal fault: %d", calls)
			}
		})
	}
}
