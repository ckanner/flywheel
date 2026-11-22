package modelcatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/watchdog"
)

type inputCapture struct {
	bytes.Buffer
	closed bool
}

func (w *inputCapture) Close() error { w.closed = true; return nil }

func catalogFixture() Report {
	r := report(Request{Version: 1, Harness: "antigravity", Channel: "native"})
	r.Status = "available"
	effort := ""
	r.Models = []Model{{ID: "gemini-pro-orchestrator", Label: "Orchestrator", Efforts: []string{""}, DefaultEffort: &effort}}
	return r
}

func encoded(t *testing.T, r Report) string {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func replaceRunner(t *testing.T, run func(string, []string, watchdog.Opts) watchdog.Result) {
	t.Helper()
	previous := runCommand
	runCommand = run
	t.Cleanup(func() { runCommand = previous })
}

func TestCommandAndRequestValidation(t *testing.T) {
	t.Setenv(WorkerEnv, "")
	if Command() != "flywheel-model-catalog" {
		t.Fatal(Command())
	}
	t.Setenv(WorkerEnv, "/test/catalog")
	if Command() != "/test/catalog" {
		t.Fatal(Command())
	}
	for _, r := range []Request{{}, {Version: 2, Harness: "codex-pro"}, {Version: 1, Harness: "bad command"},
		{Version: 1, Harness: "codex-pro", Provider: "credential\nvalue"}, {Version: 1, Harness: "codex-pro", Channel: strings.Repeat("x", 257)}} {
		if r.Validate() == nil {
			t.Fatalf("accepted %#v", r)
		}
	}
	if (Request{Version: 1, Harness: "dsh", Provider: "chatgpt", Channel: "chatgpt-pro"}).Validate() != nil {
		t.Fatal("valid selectors rejected")
	}
}

func TestListValidatesDirectoryAndReportsUnsupportedWithoutAProcess(t *testing.T) {
	replaceRunner(t, func(string, []string, watchdog.Opts) watchdog.Result {
		t.Fatal("unsupported must not spawn")
		return watchdog.Result{}
	})
	if _, err := List(context.Background(), t.TempDir(), Request{}); err == nil {
		t.Fatal("invalid request accepted")
	}
	r := Request{Version: 1, Harness: "kimi"}
	for _, cwd := range []string{"relative", filepath.Join(t.TempDir(), "missing")} {
		if _, err := List(context.Background(), cwd, r); err == nil {
			t.Fatal("invalid directory accepted")
		}
	}
	file := filepath.Join(t.TempDir(), "regular-file")
	if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := List(context.Background(), file, r); err == nil {
		t.Fatal("file accepted as cwd")
	}
	for _, harness := range []string{"kimi", "kimi-native", "dsh", "pi", "unknown"} {
		r.Harness = harness
		result, err := List(context.Background(), t.TempDir(), r)
		if err != nil || result.Status != "unsupported" || result.Source != "unsupported" || len(result.Models) != 0 || result.AccountEntitlement != "unknown" || result.InferencePerformed {
			t.Fatalf("unsupported catalog %#v, %v", result, err)
		}
	}
}

func TestListBindsWorkerWithoutPromptOrCredentials(t *testing.T) {
	fixture := catalogFixture()
	t.Setenv("ANTHROPIC_API_KEY", "never-pass-this-secret")
	t.Setenv("OPENAI_API_KEY", "never-pass-this-secret")
	t.Setenv("NODE_OPTIONS", "--require=/private/mutation")
	t.Setenv("FLYWHEEL_AGY_BIN", "/owned/agy")
	t.Setenv(WorkerEnv, "/owned/catalog")
	zero := 0
	replaceRunner(t, func(command string, args []string, opts watchdog.Opts) watchdog.Result {
		if command != "/owned/catalog" || len(args) != 0 || opts.IdleTimeoutMs != 15000 || opts.WallTimeoutMs != 15000 || opts.KillGraceMs != 1000 {
			t.Fatalf("unbounded command %s %#v %#v", command, args, opts)
		}
		for _, item := range opts.Env {
			if strings.Contains(item, "never-pass") || strings.HasPrefix(item, "NODE_OPTIONS=") {
				t.Fatal("credential or executable injection escaped")
			}
		}
		if !strings.Contains(strings.Join(opts.Env, "\n"), "FLYWHEEL_AGY_BIN=/owned/agy") {
			t.Fatal("native executable not preserved")
		}
		var input inputCapture
		opts.InteractStdin(&input, func() { t.Fatal("metadata must finish by EOF") })
		var value map[string]any
		if err := json.Unmarshal(input.Bytes(), &value); err != nil || !input.closed || value["cwd"] != opts.Cwd || len(value) != 4 || value["harness"] != fixture.Harness {
			t.Fatalf("unexpected metadata request: %s", input.String())
		}
		opts.OnActivity("metadata", "stdout")
		return watchdog.Result{ExitCode: &zero, Stdout: encoded(t, fixture)}
	})
	actual, err := List(context.Background(), t.TempDir(), fixture.Request)
	if err != nil || actual.Status != "available" || actual.Models[0].DefaultEffort == nil || *actual.Models[0].DefaultEffort != "" {
		t.Fatalf("fixed effort lost: %#v, %v", actual, err)
	}
}

func TestListBoundsAndSanitizesProcessFailures(t *testing.T) {
	zero, one := 0, 1
	for _, result := range []watchdog.Result{
		{}, {ExitCode: &one}, {ExitCode: &zero, Signal: "SIGTERM"}, {ExitCode: &zero, Cancelled: true},
		{ExitCode: &zero, TimedOut: true}, {ExitCode: &zero, SpawnError: "secret"}, {ExitCode: &zero, CleanupError: "secret"},
		{ExitCode: &zero, Truncated: true}, {ExitCode: &zero, Stdout: strings.Repeat("x", maxBytes+1)},
	} {
		replaceRunner(t, func(string, []string, watchdog.Opts) watchdog.Result { return result })
		actual, err := List(context.Background(), t.TempDir(), catalogFixture().Request)
		if err != nil || actual.Status != "unavailable" || strings.Contains(encoded(t, actual), "secret") || len(actual.Models) != 0 {
			t.Fatalf("untrusted process escaped: %#v, %v", actual, err)
		}
	}
	replaceRunner(t, func(_ string, _ []string, opts watchdog.Opts) watchdog.Result {
		opts.OnActivity(strings.Repeat("x", maxBytes+1), "stderr")
		if opts.Context.Err() != context.Canceled {
			t.Fatal("overflow did not cancel worker group")
		}
		return watchdog.Result{ExitCode: &zero, Stdout: encoded(t, catalogFixture())}
	})
	actual, err := List(context.Background(), t.TempDir(), catalogFixture().Request)
	if err != nil || !actual.Truncated || actual.Error.Code != "OUTPUT_LIMIT" {
		t.Fatalf("overflow accepted: %#v, %v", actual, err)
	}
}

func TestDecodeCatalogValidatesCompleteMetadata(t *testing.T) {
	expected := report(catalogFixture().Request)
	var missingLimitations map[string]json.RawMessage
	if err := json.Unmarshal([]byte(encoded(t, catalogFixture())), &missingLimitations); err != nil {
		t.Fatal(err)
	}
	delete(missingLimitations, "limitations")
	missing, err := json.Marshal(missingLimitations)
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{"not-json", string(missing), encoded(t, catalogFixture()) + "{}", strings.Replace(encoded(t, catalogFixture()), `"status":`, `"unknown":true,"status":`, 1),
		strings.Replace(encoded(t, catalogFixture()), `"inferencePerformed":false,`, "", 1),
		strings.Replace(encoded(t, catalogFixture()), `"truncated":false`, `"truncated":null`, 1)} {
		if actual := decode(output, expected); actual.Error == nil || actual.Error.Code != "CATALOG_PROTOCOL" {
			t.Fatalf("invalid framing accepted: %#v", actual)
		}
	}
	mutations := []func(*Report){
		func(r *Report) { r.Harness = "codex-pro" }, func(r *Report) { r.Source = "made-up" }, func(r *Report) { r.Scope = "account" },
		func(r *Report) { r.InferencePerformed = true }, func(r *Report) { r.AccountEntitlement = "verified" },
		func(r *Report) { r.Status = "success" }, func(r *Report) { r.Models = nil }, func(r *Report) { r.Models = make([]Model, maxModels+1) },
		func(r *Report) { r.Limitations = nil }, func(r *Report) { r.Limitations = make([]string, 17) }, func(r *Report) { r.Status = "unavailable" },
		func(r *Report) { r.Models[0].ID = "bad id" }, func(r *Report) { r.Models = append(r.Models, r.Models[0]) },
		func(r *Report) { r.Models[0].Label = "" }, func(r *Report) { r.Models[0].Description = "secret\nline" },
		func(r *Report) { r.Models[0].Efforts = make([]string, 11) }, func(r *Report) { r.Models[0].Efforts = []string{"invented"} },
		func(r *Report) { r.Models[0].Efforts = []string{"high", "high"} }, func(r *Report) { r.Models[0].Efforts = []string{"high"} },
		func(r *Report) { r.Limitations = []string{""} }, func(r *Report) { r.Error = &Error{Code: "secret", Message: "secret"} },
	}
	for index, mutate := range mutations {
		fixture := catalogFixture()
		mutate(&fixture)
		if actual := decode(encoded(t, fixture), expected); actual.Error == nil || actual.Error.Code != "CATALOG_PROTOCOL" {
			t.Fatalf("invalid catalog %d accepted: %#v", index, actual)
		}
	}
	fixture := catalogFixture()
	fixture.Status, fixture.Models, fixture.Error = "unavailable", []Model{}, &Error{Code: "native-secret", Message: "credential-secret"}
	actual := decode(encoded(t, fixture), expected)
	if actual.Error.Code != "CATALOG_UNAVAILABLE" || strings.Contains(encoded(t, actual), "secret") {
		t.Fatal("raw native error escaped")
	}
	fixture = catalogFixture()
	fixture.Models[0].Description = "Native metadata"
	fixture.Truncated = true
	if actual = decode(encoded(t, fixture), expected); actual.Status != "available" || !actual.Truncated {
		t.Fatal("verified bounded catalog rejected")
	}
	if readable(" \n", 10) || readable(strings.Repeat("x", 11), 10) || readable("bad\x00", 10) || !readable("模型", 10) {
		t.Fatal("invalid metadata text rules")
	}
}

var _ io.WriteCloser = (*inputCapture)(nil)
