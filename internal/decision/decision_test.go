package decision

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fixture() (Snapshot, Policy, Options) {
	s := Snapshot{Version: 1, ID: "decision-1", Identity: Identity{TaskID: "task", RunID: "run", Revision: 7, ContractHash: "contract", CandidateHash: "candidate", ObservationHash: "observations"}, Facts: []Fact{{DiagnosticFacts, "The isolated regression reproduces a missing config value."}}, MissingFacts: []string{"Which config source won?"}, Actions: []Action{{"inspect-config", Diagnostic, "Read the approved effective-config report.", "config-binding"}, {"inspect-log", Diagnostic, "Read the approved diagnostic log.", "log-binding"}}, BaselineID: "inspect-log", SemanticUncertainty: true}
	p := Policy{Version: "diagnostic-experiment-1", Mode: Adopt, AllowedData: []Category{DiagnosticFacts}, MinApplicability: .9, MinChoice: .9, MaxCalls: 3}
	o := Options{Endpoint: "https://example.test/v1/systemone", Model: "jev-1.14.0", ExpectedModel: "jev-1.14.0", APIKey: "test-secret-key", Timeout: time.Second}
	return s, p, o
}

func native() []byte {
	return []byte(`{"model":"jev-1.14.0","answers":{"action":{"type":"choice","choice":"inspect-config","confidence":0.83,"probabilities":{"inspect-config":0.96,"inspect-log":0.04}},"applicable":{"type":"noul","noul":0.98}},"usage":{"input_tokens":100,"output_tokens":20}}`)
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func reply(body []byte, code int) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}
}

func transport(counter *int) http.RoundTripper {
	return roundTrip(func(r *http.Request) (*http.Response, error) {
		*counter++
		return reply(native(), 200), nil
	})
}

func journalKey(s Snapshot) string {
	return hash(struct{ Task, Run, Decision string }{s.Identity.TaskID, s.Identity.RunID, s.ID})
}

func TestEvaluatePersistsAndDoesNotReplay(t *testing.T) {
	s, p, o := fixture()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+o.APIKey || r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing native authorization or content type")
		}
		var body struct {
			Model     string              `json:"model"`
			Questions map[string]question `json:"questions"`
			State     map[string]any      `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != o.ExpectedModel || body.Questions["action"].Type != "choice" || body.Questions["applicable"].Type != "noul" || len(body.Questions["action"].Criteria) != 2 || len(body.State) != 2 {
			t.Errorf("invalid bounded native request: %#v, %v", body, err)
		}
		_, _ = w.Write(native())
	}))
	defer server.Close()
	o.Endpoint = server.URL + "/v1/systemone"
	directory := filepath.Join(t.TempDir(), "journal")
	r, err := Evaluate(context.Background(), directory, s, p, o)
	if err != nil || r.Status != "ok" || r.Usage.Input == nil || *r.Usage.Input != 100 || r.Model != o.ExpectedModel {
		t.Fatalf("evaluate: %#v, %v", r, err)
	}
	selection := Select(s, r, p, o)
	if !selection.Adopted || selection.ActionID != "inspect-config" || selection.InvocationID == "" {
		t.Fatalf("expected concrete diagnostic proposal: %#v", selection)
	}
	second, err := Evaluate(context.Background(), directory, s, p, o)
	if err != nil || hash(second) != hash(r) || calls.Load() != 1 || Select(s, second, p, o) != selection {
		t.Fatalf("recovery reselected or replayed: %#v, calls=%d, %v", second, calls.Load(), err)
	}
	files, _ := os.ReadDir(directory)
	for _, file := range files {
		data, _ := os.ReadFile(filepath.Join(directory, file.Name()))
		if bytes.Contains(data, []byte(o.APIKey)) {
			t.Fatal("API key persisted")
		}
	}
}

func TestModesAndDeterministicSkips(t *testing.T) {
	tests := []struct {
		name, reason string
		change       func(*Snapshot, *Policy, *Options)
	}{
		{"off", "disabled", func(_ *Snapshot, p *Policy, _ *Options) { p.Mode = Off }},
		{"offline", "offline-no-network", func(_ *Snapshot, p *Policy, _ *Options) { p.Mode = Offline }},
		{"unknown", "executor-outcome-unknown", func(s *Snapshot, _ *Policy, _ *Options) { s.ExecutorUnknown = true }},
		{"single", "one-legal-action", func(s *Snapshot, _ *Policy, _ *Options) { s.Actions = s.Actions[1:] }},
		{"mechanical", "deterministic-choice", func(s *Snapshot, _ *Policy, _ *Options) { s.SemanticUncertainty = false }},
		{"no-data", "data-not-authorized", func(_ *Snapshot, p *Policy, _ *Options) { p.AllowedData = nil }},
		{"log-data", "data-not-authorized", func(s *Snapshot, _ *Policy, _ *Options) { s.Facts[0].Category = Logs }},
		{"hard-budget", "hard-budget-without-enforceable-bound", func(_ *Snapshot, _ *Policy, o *Options) { o.HardBudget = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, p, o := fixture()
			test.change(&s, &p, &o)
			o.Transport = roundTrip(func(*http.Request) (*http.Response, error) { t.Fatal("optional call was dispatched"); return nil, nil })
			directory := filepath.Join(t.TempDir(), "unused")
			r, err := Evaluate(context.Background(), directory, s, p, o)
			if err != nil || r.Reason != test.reason || r.Status != "skipped" || r.Usage.Input == nil || *r.Usage.Input != 0 {
				t.Fatalf("skip: %#v, %v", r, err)
			}
			if _, err := os.Stat(directory); !os.IsNotExist(err) {
				t.Fatal("skipped decision wrote a journal")
			}
		})
	}
}

func TestShadowOnlyCapturesAndOfflineReplayIsPure(t *testing.T) {
	s, p, o := fixture()
	p.Mode, o.APIKey = Shadow, ""
	directory := filepath.Join(t.TempDir(), "journal")
	r, err := Evaluate(context.Background(), directory, s, p, o)
	if err != nil || r.Reason != "snapshot-captured" {
		t.Fatalf("capture: %#v, %v", r, err)
	}
	if _, err := os.Stat(filepath.Join(directory, journalKey(s)+".pending")); !os.IsNotExist(err) {
		t.Fatal("capture reserved a live call")
	}
	if _, err := Evaluate(context.Background(), directory, s, p, o); err != nil {
		t.Fatal(err)
	}
	p.Mode = Offline
	r, err = Replay(s, p, o, native())
	if err != nil || r.Reason != "offline-recorded-response" || Select(s, r, p, o).Adopted {
		t.Fatalf("offline replay adopted or failed: %#v, %v", r, err)
	}
	p.Mode = Advisory
	if _, err := Replay(s, p, o, native()); err == nil {
		t.Fatal("live policy accepted as offline")
	}
	p.Mode = Offline
	if _, err := Replay(s, p, o, []byte("bad")); err == nil {
		t.Fatal("invalid recorded response accepted")
	}
	s.Version = 99
	if _, err := Replay(s, p, o, native()); err == nil {
		t.Fatal("invalid replay identity accepted")
	}
}

func TestUnknownAndBudgetNeverReplay(t *testing.T) {
	s, p, o := fixture()
	p.MaxCalls = 1
	calls := 0
	o.Transport = roundTrip(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("connection lost") })
	directory := filepath.Join(t.TempDir(), "journal")
	r, err := Evaluate(context.Background(), directory, s, p, o)
	if err != nil || r.Status != "unknown" || r.Usage.Input != nil {
		t.Fatalf("unknown: %#v, %v", r, err)
	}
	if selection := Select(s, r, p, o); selection.Adopted || selection.ActionID != s.BaselineID {
		t.Fatalf("optional unknown must retain safe baseline: %#v", selection)
	}
	if _, err := Evaluate(context.Background(), directory, s, p, o); err != nil || calls != 1 {
		t.Fatalf("unknown replay: %d, %v", calls, err)
	}
	s.ID = "decision-2"
	r, err = Evaluate(context.Background(), directory, s, p, o)
	if err != nil || r.Reason != "decision-call-budget" || calls != 1 {
		t.Fatalf("unknown consumed no call slot: %#v, calls=%d, %v", r, calls, err)
	}
}

func TestConcurrentReservationSendsOnce(t *testing.T) {
	s, p, o := fixture()
	var count atomic.Int32
	o.Transport = roundTrip(func(*http.Request) (*http.Response, error) { count.Add(1); return reply(native(), 200), nil })
	directory := filepath.Join(t.TempDir(), "journal")
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := Evaluate(context.Background(), directory, s, p, o); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if count.Load() != 1 {
		t.Fatalf("dispatched %d times", count.Load())
	}
}

func TestSelectionRechecksAllIdentityAndActionRules(t *testing.T) {
	s, p, o := fixture()
	r := receiptFor(s, p, o)
	r.Status = "ok"
	r.HTTPStatus = 200
	if err := parseResponse(native(), s, o.ExpectedModel, &r); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, reason string
		change       func(*Snapshot, *Policy, *Options, *Receipt)
	}{
		{"invalid", "invalid-or-unknown-execution", func(s *Snapshot, _ *Policy, _ *Options, _ *Receipt) { s.Version = 2 }},
		{"executor-unknown", "invalid-or-unknown-execution", func(s *Snapshot, _ *Policy, _ *Options, _ *Receipt) { s.ExecutorUnknown = true }},
		{"advisory", "deterministic-baseline", func(_ *Snapshot, p *Policy, _ *Options, _ *Receipt) { p.Mode = Advisory }},
		{"permission", "adoption-not-eligible", func(_ *Snapshot, p *Policy, _ *Options, _ *Receipt) { p.AllowedData = nil }},
		{"revision", "stale-decision", func(s *Snapshot, _ *Policy, _ *Options, _ *Receipt) { s.Identity.Revision++ }},
		{"target", "stale-decision", func(_ *Snapshot, _ *Policy, o *Options, _ *Receipt) { o.Endpoint = "https://changed.test/v1/systemone" }},
		{"status", "decision-abstained", func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.Status = "unknown" }},
		{"http", "decision-abstained", func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.HTTPStatus = 500 }},
		{"applicability", "decision-not-applicable", func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { v := .1; r.Applicability = &v }},
		{"distribution", "decision-not-applicable", func(_ *Snapshot, _ *Policy, _ *Options, r *Receipt) { r.Probabilities = nil }},
		{"non-diagnostic", "action-family-not-enabled", func(s *Snapshot, _ *Policy, _ *Options, r *Receipt) {
			s.Actions[0].Kind = Replan
			r.SnapshotHash = hash(*s)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ss, pp, oo, rr := s, p, o, r
			ss.Actions = append([]Action(nil), s.Actions...)
			test.change(&ss, &pp, &oo, &rr)
			selection := Select(ss, rr, pp, oo)
			if selection.Adopted || selection.Reason != test.reason || selection.InvocationID != "" {
				t.Fatalf("unsafe selection: %#v", selection)
			}
		})
	}
}

func TestValidationRejectsMalformedSnapshotsAndPolicies(t *testing.T) {
	tests := []func(*Snapshot, *Policy){
		func(s *Snapshot, _ *Policy) { s.Version = 2 },
		func(_ *Snapshot, p *Policy) { p.Mode = "unknown" },
		func(_ *Snapshot, p *Policy) { p.MinChoice = math.NaN() },
		func(_ *Snapshot, p *Policy) { p.AllowedData = []Category{"private-transcript"} },
		func(s *Snapshot, _ *Policy) { s.Actions = nil },
		func(s *Snapshot, _ *Policy) { s.Facts[0].Text = "" },
		func(s *Snapshot, _ *Policy) { s.MissingFacts = []string{""} },
		func(s *Snapshot, _ *Policy) { s.Actions[0].ID = s.Actions[1].ID },
		func(s *Snapshot, _ *Policy) { s.BaselineID = "unapproved" },
	}
	for i, change := range tests {
		s, p, o := fixture()
		change(&s, &p)
		if err := Validate(s, p); err == nil {
			t.Fatalf("case %d accepted", i)
		}
		if _, err := Evaluate(context.Background(), t.TempDir(), s, p, o); err == nil {
			t.Fatalf("case %d dispatched", i)
		}
	}
	if validCategory(TaskSummary) != true || validCategory(Logs) != true || validCategory(Source) != true || validCategory(Diff) != true {
		t.Fatal("known categories rejected")
	}
}

func TestEvaluateRejectsUnsafeInputsAndHandlesCancellation(t *testing.T) {
	s, p, o := fixture()
	if _, err := Evaluate(nil, t.TempDir(), s, p, o); err == nil {
		t.Fatal("nil context accepted")
	}
	secret := s
	secret.Facts = []Fact{{DiagnosticFacts, o.APIKey}}
	if _, err := Evaluate(context.Background(), t.TempDir(), secret, p, o); err == nil {
		t.Fatal("secret persisted")
	}
	large := s
	large.Facts = make([]Fact, 32)
	for i := range large.Facts {
		large.Facts[i] = Fact{DiagnosticFacts, strings.Repeat("x", 2048)}
	}
	if _, err := Evaluate(context.Background(), t.TempDir(), large, p, o); err == nil {
		t.Fatal("oversized request accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := Evaluate(ctx, t.TempDir(), s, p, o)
	if err != nil || r.Reason != "cancelled-before-dispatch" {
		t.Fatalf("cancel: %#v, %v", r, err)
	}
	o.APIKey = ""
	r, err = Evaluate(context.Background(), filepath.Join(t.TempDir(), "journal"), s, p, o)
	if err != nil || r.Reason != "missing-api-key" {
		t.Fatalf("key: %#v, %v", r, err)
	}
	if validTarget(Options{Endpoint: "://"}) || validTarget(Options{Endpoint: "https://name:secret@example.test/v1/systemone"}) {
		t.Fatal("invalid endpoint accepted")
	}
	o.Model = "jev-latest"
	if validTarget(o) {
		t.Fatal("mutable model alias accepted")
	}
}
