package decision

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func successfulReceipt(t *testing.T, s Snapshot, p Policy, o Options) Receipt {
	t.Helper()
	r := receiptFor(s, p, o)
	require.NoError(t, parseResponse(native(), s, o.ExpectedModel, &r))
	r.Status, r.Reason, r.HTTPStatus = "ok", "fixture-response", 200
	return r
}

func TestVersionTwoRequiresOwnersExplicitActionFamilyAuthority(t *testing.T) {
	for _, kind := range []ActionKind{Diagnostic, Repair, Replan, Model} {
		t.Run(string(kind), func(t *testing.T) {
			s, p, o := fixture()
			s.Version, s.Actions[0].Kind = 2, kind
			r := successfulReceipt(t, s, p, o)
			require.NoError(t, Validate(s, p))
			require.Contains(t, questions(s)["action"].Instructions, "failed coding task")
			selection := Select(s, r, p, o)
			require.Equal(t, kind == Diagnostic, selection.Adopted, "nil allowlist retains diagnostic-only authority")
			o.AllowedKinds = []ActionKind{}
			require.False(t, Select(s, r, p, o).Adopted, "explicit empty authority must never expand to the default")
			o.AllowedKinds = []ActionKind{kind}
			selection = Select(s, r, p, o)
			require.True(t, selection.Adopted)
			require.Equal(t, "approved-"+string(kind)+"-choice", selection.Reason)
			require.NotEmpty(t, selection.InvocationID)
			s.Actions[0].BindingHash = "different-authority"
			require.False(t, Select(s, r, p, o).Adopted, "an explicit family is not authority for changed command/model arguments")
		})
	}
}

func TestRuntimeQuestionsDoNotReinterpretVersionOneReceipts(t *testing.T) {
	s, p, o := fixture()
	legacy := successfulReceipt(t, s, p, o)
	require.Equal(t, "66dcdd570a755daea11b9506f5298db1aaaac62b0b7646d4570cc1749c06936f", legacy.QuestionHash)
	before := string(request(s, o.Model))
	o.AllowedKinds = []ActionKind{Diagnostic, Repair, Replan, Model}
	require.Equal(t, before, string(request(s, o.Model)), "owner capabilities do not alter the wire request")
	require.NoError(t, ValidateReceiptIdentity(s, p, o, legacy))
	require.True(t, Select(s, legacy, p, o).Adopted)
	s.Version = 2
	require.NotEqual(t, legacy.QuestionHash, receiptFor(s, p, o).QuestionHash)
	require.Error(t, ValidateReceiptIdentity(s, p, o, legacy))
	require.False(t, Select(s, legacy, p, o).Adopted, "a known v1 terminal must not authorize a different v2 question")
	changed := successfulReceipt(t, s, p, o)
	require.NoError(t, ValidateReceiptIdentity(s, p, o, changed))
}

func TestStoredReceiptIdentityRejectsCorruptionBeforeOuterStateLoadsIt(t *testing.T) {
	for name, mutate := range map[string]func(*Snapshot, *Policy, *Options, *Receipt){
		"snapshot": func(s *Snapshot, _ *Policy, _ *Options, _ *Receipt) { s.Version = 99 },
		"target": func(_ *Snapshot, _ *Policy, o *Options, _ *Receipt) {
			o.Endpoint = "http://remote.example/v1/systemone"
		},
		"version":                   func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.Version++ },
		"decision":                  func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.DecisionID = "different" },
		"snapshot hash":             func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.SnapshotHash = "different" },
		"policy hash":               func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.PolicyHash = "different" },
		"target hash":               func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.TargetHash = "different" },
		"question hash":             func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.QuestionHash = "different" },
		"duration":                  func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.DurationMS = -1 },
		"input usage":               func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.Usage.Input = new(int64(-1)) },
		"output usage":              func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.Usage.Output = new(int64(-1)) },
		"terminal":                  func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.Status = "complete" },
		"reason":                    func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.Reason = "" },
		"negative HTTP":             func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.HTTPStatus = -1 },
		"HTTP outside protocol":     func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.HTTPStatus = 600 },
		"successful HTTP":           func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.HTTPStatus = 503 },
		"successful model":          func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.Model = "jev-1.15.0" },
		"successful unknown input":  func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.Usage.Input = nil },
		"successful unknown output": func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.Usage.Output = nil },
		"successful answer":         func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.Probabilities = nil },
		"skipped claims response":   func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.Status = "skipped" },
		"skipped unknown usage": func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) {
			*r = skipped(*r, "not-sent")
			r.HTTPStatus, r.Model, r.ActionID, r.Usage.Input = 0, "", "", nil
		},
		"skipped nonzero usage": func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) {
			*r = skipped(*r, "not-sent")
			r.HTTPStatus, r.Model, r.ActionID = 0, "", ""
			r.Usage.Output = new(int64(1))
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, p, o := fixture()
			r := successfulReceipt(t, s, p, o)
			mutate(&s, &p, &o, &r)
			require.Error(t, ValidateReceiptIdentity(s, p, o, r))
		})
	}
	s, p, o := fixture()
	for _, status := range []string{"error", "unknown", "skipped"} {
		r := receiptFor(s, p, o)
		r.Status = status
		if status == "skipped" {
			r = skipped(r, "not-sent")
		}
		require.NoError(t, ValidateReceiptIdentity(s, p, o, r), status)
	}
}

func TestCorruptDecisionJournalPreservesReservedReceiptIdentity(t *testing.T) {
	s, p, o := fixture()
	calls := 0
	o.Transport = transport(&calls)
	directory := filepath.Join(t.TempDir(), "journal")
	_, err := Evaluate(context.Background(), directory, s, p, o)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, journalKey(s)+".json"), []byte("corrupt terminal"), 0600))
	r, err := Evaluate(context.Background(), directory, s, p, o)
	require.Error(t, err)
	require.Equal(t, "unknown", r.Status)
	require.Equal(t, s.ID, r.DecisionID)
	require.NoError(t, ValidateReceiptIdentity(s, p, o, r))
	require.False(t, Select(s, r, p, o).Adopted)
	require.Equal(t, 1, calls, "journal corruption must never resend")
}
