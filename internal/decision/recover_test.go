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
	s.Version = 99
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

func TestVersionOneQuestionIdentityPreservesExistingKnownReceipts(t *testing.T) {
	s, p, o := fixture()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	recorded := receiptFor(s, p, o)
	// The diagnostic fixture's question identity in v0.31.1 (e035b5b). New
	// collection choices must not reinterpret an already dispatched v1 request.
	recorded.QuestionHash = "66dcdd570a755daea11b9506f5298db1aaaac62b0b7646d4570cc1749c06936f"
	if err := parseResponse(native(), s, o.ExpectedModel, &recorded); err != nil {
		t.Fatal(err)
	}
	recorded.Status, recorded.HTTPStatus = "ok", 200
	if err := os.WriteFile(filepath.Join(directory, journalKey(s)+".json"), raw(recorded), 0600); err != nil {
		t.Fatal(err)
	}
	recovered, found, err := Recover(directory, s, p, o)
	if err != nil || !found || recovered.Usage.Input == nil || *recovered.Usage.Input != 100 || !Select(s, recovered, p, o).Adopted {
		t.Fatalf("existing known receipt lost its identity or usage: %#v, %v", recovered, err)
	}
}
