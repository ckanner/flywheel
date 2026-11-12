package decision

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryReadsFrozenTerminalAndUnknownWithoutNewCalls(t *testing.T) {
	s, p, o := fixture()
	directory := filepath.Join(t.TempDir(), "missing")
	r, found, err := Recover(directory, s, p, o)
	if err != nil || found || r.Status != "unknown" {
		t.Fatalf("missing journal: %#v, %v, %v", r, found, err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("read-only recovery created a journal")
	}
	calls := 0
	o.Transport = transport(&calls)
	completed, err := Evaluate(context.Background(), directory, s, p, o)
	if err != nil {
		t.Fatal(err)
	}
	r, found, err = Recover(directory, s, p, o)
	if err != nil || !found || hash(r) != hash(completed) || calls != 1 {
		t.Fatalf("terminal recovery: %#v, %v, %v", r, found, err)
	}
	if err := os.Remove(filepath.Join(directory, journalKey(s)+".json")); err != nil {
		t.Fatal(err)
	}
	r, found, err = Recover(directory, s, p, o)
	if err != nil || !found || r.Status != "unknown" || r.Usage.Input != nil || calls != 1 {
		t.Fatalf("intent recovery: %#v, %v, %v", r, found, err)
	}
	s.Version = 2
	if _, _, err := Recover(directory, s, p, o); err == nil {
		t.Fatal("invalid frozen identity accepted")
	}
	s.Version = 1
	if err := os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Recover(directory, s, p, o); err == nil {
		t.Fatal("unsafe journal accepted")
	}
}
