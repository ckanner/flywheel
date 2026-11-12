package decision

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"syscall"
)

var journalRoot = os.OpenRoot
var journalDirStat = os.Lstat
var journalStat = (*os.Root).Stat
var journalLstat = (*os.Root).Lstat
var journalOpen = (*os.Root).OpenFile
var journalWrite = (*os.File).Write
var journalSync = (*os.File).Sync
var journalLink = (*os.Root).Link
var journalRead = io.ReadAll

func openJournal(directory string) (*os.Root, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	return existingJournal(directory)
}

func existingJournal(directory string) (*os.Root, error) {
	info, err := journalDirStat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("decision journal must be a private real directory")
	}
	root, err := journalRoot(directory)
	if err != nil {
		return nil, err
	}
	actual, err := journalStat(root, ".")
	if err != nil || !os.SameFile(info, actual) {
		root.Close()
		return nil, fmt.Errorf("decision journal identity changed")
	}
	return root, nil
}

func syncJournal(root *os.Root) error {
	f, err := journalOpen(root, ".", os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return journalSync(f)
}

// Even a partial exclusive intent permanently prevents automatic replay.
func exclusive(root *os.Root, name string, data []byte) error {
	f, err := journalOpen(root, name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if n, err := journalWrite(f, data); err != nil {
		return err
	} else if n != len(data) {
		return io.ErrShortWrite
	}
	return journalSync(f)
}

func publish(root *os.Root, name string, data []byte) error {
	temp := ".decision-" + rand.Text()
	defer root.Remove(temp)
	if err := exclusive(root, temp, data); err != nil {
		return err
	}
	if err := journalLink(root, temp, name); err != nil {
		return err
	}
	return syncJournal(root)
}

func read(root *os.Root, name string) ([]byte, bool, error) {
	info, err := journalLstat(root, name)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, false, fmt.Errorf("decision journal files must be private regular files")
	}
	f, err := journalOpen(root, name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	data, err := journalRead(io.LimitReader(f, 2*maxDocument+1))
	if len(data) > 2*maxDocument {
		return nil, false, fmt.Errorf("decision journal file exceeds bounds")
	}
	return data, true, err
}

func install(root *os.Root, name string, data []byte) error {
	if err := publish(root, name, data); err != nil && !os.IsExist(err) {
		return err
	}
	actual, _, err := read(root, name)
	if err != nil {
		return err
	}
	if hash(actual) != hash(data) {
		return fmt.Errorf("decision identity already belongs to a different snapshot or policy")
	}
	return nil
}

func load(root *os.Root, key string, expected Receipt) (Receipt, bool, error) {
	data, exists, err := read(root, key+".json")
	if err != nil {
		return Receipt{}, false, err
	}
	if exists {
		var actual Receipt
		if json.Unmarshal(data, &actual) != nil || actual.Version != expected.Version || actual.DecisionID != expected.DecisionID || actual.SnapshotHash != expected.SnapshotHash || actual.PolicyHash != expected.PolicyHash || actual.TargetHash != expected.TargetHash || actual.QuestionHash != expected.QuestionHash || actual.DurationMS < 0 || (actual.Usage.Input != nil && *actual.Usage.Input < 0) || (actual.Usage.Output != nil && *actual.Usage.Output < 0) || (actual.Status != "ok" && actual.Status != "error" && actual.Status != "unknown" && actual.Status != "skipped") {
			return Receipt{}, false, fmt.Errorf("invalid decision receipt identity or terminal")
		}
		return actual, true, nil
	}
	info, err := journalLstat(root, key+".pending")
	if os.IsNotExist(err) {
		return expected, false, nil
	}
	if err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0) {
		return Receipt{}, false, fmt.Errorf("decision intent must be a private regular file")
	}
	return expected, true, err
}

func reserveSlot(root *os.Root, key string, maxCalls int) (bool, error) {
	for slot := 0; slot < maxCalls; slot++ {
		err := exclusive(root, fmt.Sprintf("call-%03d", slot), []byte(key))
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		return true, syncJournal(root)
	}
	return false, nil
}
