package shadow

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"syscall"
)

type Target struct {
	Version        int                 `json:"version"`
	Mode           string              `json:"mode"`
	Endpoint       string              `json:"endpoint"`
	Model          string              `json:"model"`
	ExpectedModel  string              `json:"expected-model"`
	QuestionDigest string              `json:"question-digest"`
	PolicyDigest   string              `json:"policy-digest"`
	Questions      map[string]question `json:"questions"`
	Policy         Policy              `json:"policy"`
}

var storeLstat = (*os.Root).Lstat
var storeOpen = (*os.Root).OpenFile
var storeWrite = (*os.File).Write
var storeSync = (*os.File).Sync
var storeClose = (*os.File).Close
var storeLink = (*os.Root).Link
var storeRead = io.ReadAll
var storeRootOpen = os.OpenRoot
var storeRootStat = (*os.Root).Stat

func openStore(directory string, create bool) (*os.Root, error) {
	if create {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, err
		}
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("shadow result directory must be a private real directory (0700)")
	}
	root, err := storeRootOpen(directory)
	if err != nil {
		return nil, err
	}
	actual, err := storeRootStat(root, ".")
	if err != nil || !os.SameFile(info, actual) || actual.Mode().Perm()&0o077 != 0 {
		root.Close()
		return nil, fmt.Errorf("shadow result directory identity changed")
	}
	return root, nil
}

func syncDirectory(root *os.Root) error {
	f, err := storeOpen(root, ".", os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return storeSync(f)
}

func writeFile(root *os.Root, name string, raw []byte) error {
	f, err := storeOpen(root, name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := storeWrite(f, raw); err != nil {
		return err
	}
	if err := storeSync(f); err != nil {
		return err
	}
	return storeClose(f)
}

// A durable exclusive intent precedes dispatch. A partial/failed intent blocks
// later automatic replay too; it is never deleted to guess that no call ran.
func reserve(root *os.Root, record Record) (bool, error) {
	err := writeFile(root, record.Key+".pending", jsonBytes(record))
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, syncDirectory(root)
}

// Hard-link publication is atomic and cannot overwrite a completed record.
func publish(root *os.Root, name string, raw []byte) error {
	temp := ".shadow-" + rand.Text()
	if err := writeFile(root, temp, raw); err != nil {
		if !os.IsExist(err) {
			root.Remove(temp)
		}
		return err
	}
	defer root.Remove(temp)
	if err := storeLink(root, temp, name); err != nil {
		return err
	}
	return syncDirectory(root)
}

func readFile(root *os.Root, name string) ([]byte, bool, error) {
	info, err := storeLstat(root, name)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, false, fmt.Errorf("shadow records must be private regular files")
	}
	f, err := storeOpen(root, name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	raw, err := storeRead(io.LimitReader(f, maxDocument+1))
	return raw, true, err
}

func targetFor(o Options) Target {
	return Target{1, "shadow", o.Endpoint, o.Model, o.ExpectedModel, digest(jsonBytes(questions)), digest(jsonBytes(policy)), questions, policy}
}

func readTarget(root *os.Root) (Target, error) {
	raw, exists, err := readFile(root, "target.json")
	if err != nil {
		return Target{}, err
	}
	if !exists {
		return Target{}, fmt.Errorf("shadow target manifest is missing")
	}
	if err := validate(raw, "target"); err != nil {
		return Target{}, err
	}
	var target Target
	_ = json.Unmarshal(raw, &target) // validated types and finite policy values
	if digest(jsonBytes(target.Questions)) != target.QuestionDigest || digest(jsonBytes(target.Policy)) != target.PolicyDigest {
		return Target{}, fmt.Errorf("shadow target manifest digest mismatch")
	}
	return target, nil
}

func installTarget(root *os.Root, target Target) error {
	_, exists, err := readFile(root, "target.json")
	if err != nil {
		return err
	}
	if !exists {
		if err := publish(root, "target.json", jsonBytes(target)); err != nil && !os.IsExist(err) {
			return err
		}
	}
	actual, err := readTarget(root)
	if err != nil {
		return err
	}
	if digest(jsonBytes(actual)) != digest(jsonBytes(target)) {
		return fmt.Errorf("shadow directory belongs to a different target, question set or policy")
	}
	return nil
}

func recordFor(c Case, target Target) Record {
	input, binding := caseDigest(c), digest(jsonBytes(target))
	key := digest([]byte(input + binding))
	return Record{Version: 1, Mode: "shadow", Key: key, CaseID: c.ID, InputDigest: input,
		QuestionDigest: target.QuestionDigest, PolicyDigest: target.PolicyDigest, TargetDigest: binding,
		Status: "pending", Advice: "abstain", Reason: "reserved-without-terminal", ActualMode: c.Baseline.Mode, At: "not-dispatched"}
}

func loadRecord(root *os.Root, expected Record) (Record, bool, error) {
	raw, exists, err := readFile(root, expected.Key+".json")
	if err != nil {
		return Record{}, false, err
	}
	if exists {
		if err := validate(raw, "record"); err != nil {
			return Record{}, false, err
		}
		var record Record
		_ = json.Unmarshal(raw, &record)
		if record.Key != expected.Key || record.CaseID != expected.CaseID || record.InputDigest != expected.InputDigest || record.QuestionDigest != expected.QuestionDigest || record.PolicyDigest != expected.PolicyDigest || record.TargetDigest != expected.TargetDigest || record.ActualMode != expected.ActualMode || record.Status == "pending" {
			return Record{}, false, fmt.Errorf("shadow record identity or terminal mismatch")
		}
		return record, true, nil
	}
	// Do not read an in-progress intent: it may still be being written. Its
	// presence is sufficient to prohibit replay, even if its bytes are damaged.
	info, err := storeLstat(root, expected.Key+".pending")
	if os.IsNotExist(err) {
		return expected, false, nil
	}
	if err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0) {
		return Record{}, false, fmt.Errorf("shadow intent must be a private regular file")
	}
	return expected, true, err
}
