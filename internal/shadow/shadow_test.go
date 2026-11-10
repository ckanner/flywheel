package shadow

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sample() Dataset {
	return Dataset{1, "shadow", []Case{{ID: "local-fix", Source: Source{"synthetic", "fixture:v1"},
		Task: "Fix the empty list display", Evidence: []string{"One component, existing empty-state pattern and regression test"},
		Baseline: Baseline{"quick", "Qualified compact contract"}, HardBlockers: []string{},
		Label: &Label{"quick", "confirmed", "fixture", "Explicit bounded case"}}}}
}

func probabilities() map[string]float64 {
	p := map[string]float64{}
	for _, key := range positives {
		p[key] = 0.95
	}
	for _, key := range risks {
		p[key] = 0.05
	}
	return p
}

func responseJSON() []byte {
	answers := map[string]any{}
	for k, p := range probabilities() {
		answers[k] = map[string]any{"type": "noul", "noul": p}
	}
	return jsonBytes(map[string]any{"model": "jev-1.14.0", "answers": answers, "usage": map[string]any{"input_tokens": 100, "output_tokens": 40}})
}

func opts(endpoint string) Options {
	return Options{Endpoint: endpoint, Model: "jev", ExpectedModel: "jev-1.14.0", APIKey: "shadow-test-credential-0123456789", Timeout: time.Second, MaxCalls: 20}
}

func TestNativeTransportAndLabelIndependentReplay(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "/v1/systemone", r.URL.Path)
		assert.Equal(t, "Bearer shadow-test-credential-0123456789", r.Header.Get("Authorization"))
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, json.Unmarshal(raw, &body))
		assert.Equal(t, "jev", body["model"])
		assert.Len(t, body["questions"], 8)
		assert.Len(t, body["state"], 2)
		assert.NotContains(t, string(raw), "Qualified compact contract")
		assert.NotContains(t, string(raw), "fixture:v1")
		assert.NotContains(t, string(raw), "Explicit bounded case")
		w.Write(responseJSON())
	}))
	defer server.Close()
	dataset, directory := sample(), privateDir(t)
	report, err := Run(context.Background(), dataset, directory, opts(server.URL+"/v1/systemone"))
	require.NoError(t, err)
	assert.Equal(t, 1, report.NewCalls)
	assert.Equal(t, 1, report.Summary.QuickCandidates)
	assert.EqualValues(t, 100, report.Summary.KnownInputTokens)
	assert.False(t, report.Summary.ProductionCalibrated)
	assert.Equal(t, "quick", report.Rows[0].Record.ActualMode)
	assert.Equal(t, "quick", dataset.Cases[0].Baseline.Mode)
	assert.Equal(t, 1.0, *report.Summary.Labels["synthetic/confirmed"].QuickCoverage)
	assert.Nil(t, report.Summary.Labels["synthetic/confirmed"].FalseQuickRate)

	dataset.Cases[0].Label = &Label{"full", "provisional", "new-reviewer", "Later independent review"}
	o := opts(server.URL + "/v1/systemone")
	o.APIKey = ""
	again, err := Run(context.Background(), dataset, directory, o)
	require.NoError(t, err)
	assert.Zero(t, again.NewCalls)
	assert.EqualValues(t, 1, calls.Load())
	assert.Equal(t, 1, again.Summary.Labels["synthetic/provisional"].FalseQuick)
	assert.Nil(t, again.Summary.Labels["synthetic/provisional"].FalseQuickRate)
	offline, err := ReadReport(dataset, directory)
	require.NoError(t, err)
	assert.Equal(t, again.Summary, offline.Summary)

	files, err := os.ReadDir(directory)
	require.NoError(t, err)
	for _, file := range files {
		raw, err := os.ReadFile(filepath.Join(directory, file.Name()))
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "shadow-test-credential-0123456789")
		assert.NotContains(t, string(raw), dataset.Cases[0].Task)
		info, err := file.Info()
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestHardBlockDoesNotNeedKeyOrHTTP(t *testing.T) {
	d := sample()
	d.Cases[0].HardBlockers = []string{"Two packets require Full"}
	d.Cases[0].Baseline = Baseline{"full", "Two packets"}
	o := opts("https://invalid.example/v1/systemone")
	o.APIKey = ""
	r, err := Run(context.Background(), d, privateDir(t), o)
	require.NoError(t, err)
	assert.Zero(t, r.NewCalls)
	assert.Equal(t, 1, r.Summary.Skipped)
	assert.Equal(t, "full", r.Rows[0].Record.Advice)
	assert.Zero(t, r.Summary.UnknownInputCalls)
	assert.EqualValues(t, 0, *r.Rows[0].Record.Usage.Input)
}

func TestPendingAndConcurrentReservationsNeverReplay(t *testing.T) {
	d, directory := sample(), privateDir(t)
	o := opts("https://invalid.example/v1/systemone")
	root, err := openStore(directory, true)
	require.NoError(t, err)
	defer root.Close()
	target := targetFor(o)
	require.NoError(t, installTarget(root, target))
	r := recordFor(d.Cases[0], target)
	owned, err := reserve(root, r)
	require.NoError(t, err)
	assert.True(t, owned)
	owned, err = reserve(root, r)
	require.NoError(t, err)
	assert.False(t, owned)
	// Even a truncated intent must not become permission for a repeated call.
	require.NoError(t, os.WriteFile(filepath.Join(directory, r.Key+".pending"), []byte("{"), 0o600))
	o.APIKey = ""
	report, err := Run(context.Background(), d, directory, o)
	require.NoError(t, err)
	assert.Zero(t, report.NewCalls)
	assert.Equal(t, 1, report.Summary.Unknown)
	assert.Equal(t, 1, report.Summary.UnknownInputCalls)
	assert.Equal(t, "abstain", report.Rows[0].Record.Advice)
	assert.Nil(t, report.Summary.Labels["synthetic/confirmed"].QuickCoverage)

	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		close(entered)
		<-release
		w.Write(responseJSON())
	}))
	defer server.Close()
	dir2 := privateDir(t)
	done := make(chan error, 1)
	go func() { _, err := Run(context.Background(), d, dir2, opts(server.URL+"/v1/systemone")); done <- err }()
	<-entered
	second, err := Run(context.Background(), d, dir2, opts(server.URL+"/v1/systemone"))
	require.NoError(t, err)
	assert.Equal(t, 1, second.Summary.Unknown)
	close(release)
	require.NoError(t, <-done)
	assert.EqualValues(t, 1, calls.Load())
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("broken transport") }
func (brokenReader) Close() error             { return nil }

func TestFailuresAbstainAndKeepActualMode(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		handler      func(http.ResponseWriter, *http.Request)
	}{
		{"http", "http-error", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "Bearer shadow-test-credential-0123456789", 401)
		}},
		{"json", "invalid-response", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("{")) }},
		{"oversized", "invalid-response", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(strings.Repeat(" ", maxDocument+1))) }},
		{"model", "model-version-mismatch", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(strings.Replace(string(responseJSON()), "jev-1.14.0", "shadow-test-credential-0123456789", 1)))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(tc.handler))
			defer s.Close()
			dir := privateDir(t)
			r, err := Run(context.Background(), sample(), dir, opts(s.URL+"/v1/systemone"))
			require.NoError(t, err)
			assert.Equal(t, 1, r.Summary.Errors)
			assert.Equal(t, "quick", r.Rows[0].Record.ActualMode)
			assert.Equal(t, "abstain", r.Rows[0].Record.Advice)
			assert.Equal(t, tc.reason, r.Rows[0].Record.Reason)
			assert.NotContains(t, string(jsonBytes(r)), "shadow-test-credential-0123456789")
			assert.Equal(t, 1, r.Summary.UnknownInputCalls)
		})
	}
	for _, name := range []string{"transport", "read", "timeout", "cancel"} {
		t.Run(name, func(t *testing.T) {
			o := opts("https://invalid.example/v1/systemone")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			o.Timeout = 15 * time.Millisecond
			o.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch name {
				case "read":
					return &http.Response{StatusCode: 200, Header: http.Header{}, Body: brokenReader{}}, nil
				case "timeout", "cancel":
					if name == "cancel" {
						cancel()
					}
					<-r.Context().Done()
					return nil, r.Context().Err()
				default:
					return nil, errors.New("network unavailable")
				}
			})
			r, err := Run(ctx, sample(), privateDir(t), o)
			require.NoError(t, err)
			assert.Equal(t, 1, r.Summary.Errors)
			assert.Equal(t, "abstain", r.Rows[0].Record.Advice)
			assert.Equal(t, name == "cancel", r.Cancelled)
		})
	}
}

func TestRedirectCannotForwardAuthorization(t *testing.T) {
	var leaked atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked.Add(1) }))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, 302) }))
	defer origin.Close()
	r, err := Run(context.Background(), sample(), privateDir(t), opts(origin.URL+"/v1/systemone"))
	require.NoError(t, err)
	assert.Equal(t, 302, r.Rows[0].Record.HTTPStatus)
	assert.EqualValues(t, 0, leaked.Load())
}

func TestMissingUsageAndUncertaintyRemainVisible(t *testing.T) {
	var body map[string]any
	require.NoError(t, json.Unmarshal(responseJSON(), &body))
	delete(body, "usage")
	body["answers"].(map[string]any)["known_edit"] = map[string]any{"type": "noul", "noul": 0.5}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write(jsonBytes(body)) }))
	defer s.Close()
	r, err := Run(context.Background(), sample(), privateDir(t), opts(s.URL+"/v1/systemone"))
	require.NoError(t, err)
	assert.Equal(t, 1, r.Summary.Abstentions)
	assert.Equal(t, 1, r.Summary.UnknownInputCalls)
	assert.Equal(t, 1, r.Summary.UnknownOutputCalls)
	assert.Zero(t, r.Summary.Disagreements)
	assert.Equal(t, "quick", r.Rows[0].Record.ActualMode)
	assert.Equal(t, 1, r.Summary.Labels["synthetic/confirmed"].MissedQuick)
}

func TestInputsAndBudgetsFailBeforeDispatch(t *testing.T) {
	base := opts("https://example.org/v1/systemone")
	for _, endpoint := range []string{"%", "https:///v1/systemone", "https://key@example.org/v1/systemone", "https://example.org/v1/systemone?key=x", "https://example.org/v1/systemone#x", "https://example.org/chat", "http://example.org/v1/systemone"} {
		o := base
		o.Endpoint = endpoint
		_, err := Run(context.Background(), sample(), privateDir(t), o)
		require.Error(t, err)
	}
	for _, mutate := range []func(*Options){func(o *Options) { o.Model = "" }, func(o *Options) { o.ExpectedModel = "" }, func(o *Options) { o.Timeout = 0 }, func(o *Options) { o.MaxCalls = 0 }, func(o *Options) { o.APIKey = "" }, func(o *Options) { o.ExpectedModel = o.APIKey }} {
		o := base
		mutate(&o)
		_, err := Run(context.Background(), sample(), privateDir(t), o)
		require.Error(t, err)
	}
	_, err := Run(nil, sample(), privateDir(t), base)
	require.Error(t, err)
	d := sample()
	d.Cases = append(d.Cases, d.Cases[0])
	d.Cases[1].ID = "second"
	o := base
	o.MaxCalls = 1
	_, err = Run(context.Background(), d, privateDir(t), o)
	require.ErrorContains(t, err, "exceeding")
	d = sample()
	d.Cases[0].Task = base.APIKey
	_, err = Run(context.Background(), d, privateDir(t), base)
	require.ErrorContains(t, err, "credential")
	d = sample()
	d.Cases[0].Evidence = make([]string, 32)
	for i := range d.Cases[0].Evidence {
		d.Cases[0].Evidence[i] = strings.Repeat("字", 1500)
	}
	_, err = Run(context.Background(), d, privateDir(t), base)
	require.ErrorContains(t, err, "64 KiB")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := Run(ctx, sample(), privateDir(t), base)
	require.NoError(t, err)
	assert.True(t, r.Cancelled)
	assert.Equal(t, 1, r.Summary.NotRun)
}

func TestSchemasAndDuplicateKeys(t *testing.T) {
	d := sample()
	_, err := ParseDataset(jsonBytes(d))
	require.NoError(t, err)
	d.Cases = append(d.Cases, d.Cases[0])
	_, err = ParseDataset(jsonBytes(d))
	require.ErrorContains(t, err, "duplicate")
	for _, raw := range []string{"", "{", "[1,", "{1:2}", `{"a":}`, `{"a":1,"a":2}`, `{} {}`, `{}garbage`, `{"version":2}`, "Authorization: Bearer secret-value-with-32-characters"} {
		_, err := ParseDataset([]byte(raw))
		require.Error(t, err, raw)
	}
	_, err = ParseDataset([]byte(strings.Repeat(" ", maxDocument+1)))
	require.Error(t, err)
	_, err = LoadDataset("/does-not-exist-shadow")
	require.Error(t, err)
	_, err = LoadDataset(privateDir(t))
	require.Error(t, err)
	path := filepath.Join(privateDir(t), "cases.json")
	require.NoError(t, os.WriteFile(path, jsonBytes(sample()), 0o600))
	_, err = LoadDataset(path)
	require.NoError(t, err)
	for _, load := range []func(string) ([]byte, error){
		func(string) ([]byte, error) { return nil, errors.New("missing") },
		func(string) ([]byte, error) { return []byte("{"), nil },
		func(string) ([]byte, error) { return []byte(`{"$defs":{"dataset":{"type":"not-a-type"}}}`), nil },
	} {
		_, err := compileSchemas(load)
		require.Error(t, err)
	}
	_, err = compileSchemas(templates.FS.ReadFile)
	require.NoError(t, err)
	previous := schemas
	schemas = func() (map[string]*jsonschema.Schema, error) { return nil, errors.New("schema missing") }
	t.Cleanup(func() { schemas = previous })
	require.ErrorContains(t, validate([]byte(`{}`), "dataset"), "schema unavailable")
}

func TestPolicyAndLabelMetrics(t *testing.T) {
	p := probabilities()
	mode, _ := advice(p, policy)
	assert.Equal(t, "quick", mode)
	p["access_change"] = 0.95
	mode, reason := advice(p, policy)
	assert.Equal(t, "full", mode)
	assert.Equal(t, "access_change", reason)
	p["access_change"] = 0.4
	mode, _ = advice(p, policy)
	assert.Equal(t, "abstain", mode)
	rows := []Row{}
	for i, fixture := range []struct{ gold, advice, origin, label, baseline string }{
		{"full", "quick", "historical", "confirmed", "full"},
		{"quick", "full", "historical", "confirmed", "unknown"},
		{"full", "full", "historical", "provisional", "full"},
		{"quick", "abstain", "synthetic", "confirmed", "quick"},
	} {
		rows = append(rows, Row{Source: Source{Kind: fixture.origin}, Baseline: Baseline{Mode: fixture.baseline},
			Label: &Label{Mode: fixture.gold, Status: fixture.label}, Record: &Record{Status: "ok", Advice: fixture.advice, DurationMS: int64(i + 1)}})
	}
	s := summarize(rows)
	assert.Equal(t, 1, s.Disagreements)
	assert.Equal(t, 1.0, *s.Labels["historical/confirmed"].FalseQuickRate)
	assert.Equal(t, 0.0, *s.Labels["historical/confirmed"].QuickCoverage)
	assert.Nil(t, s.Labels["historical/provisional"].FalseQuickRate)
	assert.EqualValues(t, 2, *s.P50MS)
	assert.EqualValues(t, 4, *s.P95MS)
	assert.False(t, s.ProductionCalibrated)
	assert.Nil(t, summarize(nil).P50MS)
}

func privateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700))
	return dir
}
