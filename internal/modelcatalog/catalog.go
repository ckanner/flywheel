// Package modelcatalog reads native-client model metadata without dispatching a
// task. Catalog membership is not evidence of account entitlement or quota.
package modelcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/watchdog"
)

const WorkerEnv = "FLYWHEEL_MODEL_CATALOG_WORKER"
const maxBytes = 1 << 20
const maxModels = 256

var runCommand = watchdog.Run
var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,255}$`)
var invisible = regexp.MustCompile(`[\p{Cc}\p{Cf}]`)

type Request struct {
	Version  int    `json:"version"`
	Harness  string `json:"harness"`
	Provider string `json:"provider,omitempty"`
	Channel  string `json:"channel,omitempty"`
}

type Model struct {
	ID            string   `json:"id"`
	Label         string   `json:"label"`
	Description   string   `json:"description,omitempty"`
	Efforts       []string `json:"efforts,omitempty"`
	DefaultEffort *string  `json:"defaultEffort,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Report struct {
	Request
	Status             string   `json:"status"`
	Source             string   `json:"source"`
	Scope              string   `json:"scope"`
	Models             []Model  `json:"models"`
	Truncated          bool     `json:"truncated"`
	InferencePerformed bool     `json:"inferencePerformed"`
	AccountEntitlement string   `json:"accountEntitlement"`
	Error              *Error   `json:"error,omitempty"`
	Limitations        []string `json:"limitations"`
}

// Command is selected by the host installation, never by request JSON.
func Command() string {
	if command := os.Getenv(WorkerEnv); command != "" {
		return command
	}
	return "flywheel-model-catalog"
}

// Validate accepts only non-secret binding selectors. A current model is not
// needed: an invalid old selection must not prevent discovering replacements.
func (r Request) Validate() error {
	if r.Version != 1 || !identifier.MatchString(r.Harness) ||
		r.Provider != "" && !identifier.MatchString(r.Provider) || r.Channel != "" && !identifier.MatchString(r.Channel) {
		return errors.New("model catalog requires version 1 and bounded harness/provider/channel identifiers")
	}
	return nil
}

func report(r Request) Report {
	source := map[string]string{"antigravity": "agy models", "codex-pro": "codex app-server model/list",
		"claude-code-pro": "Claude Agent SDK initialization.models"}[r.Harness]
	if source == "" {
		source = "unsupported"
	}
	return Report{Request: r, Status: "unsupported", Source: source, Scope: "native-client-catalog", Models: []Model{},
		AccountEntitlement: "unknown", Limitations: []string{"Catalog membership does not verify account access, remaining quota or execution eligibility."}}
}

// List starts only the bounded metadata worker. Neither configuration files nor
// task state are changed; API keys and custom credential variables are omitted.
func List(ctx context.Context, cwd string, request Request) (Report, error) {
	if err := request.Validate(); err != nil {
		return Report{}, err
	}
	r := report(request)
	if !filepath.IsAbs(cwd) {
		return r, errors.New("model catalog working directory must be absolute")
	}
	physical, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return r, errors.New("model catalog working directory is unavailable")
	}
	info, err := os.Stat(physical)
	if err != nil || !info.IsDir() {
		return r, errors.New("model catalog working directory must be an existing directory")
	}
	if r.Source == "unsupported" {
		r.Limitations = append(r.Limitations, "This binding has no verified read-only catalog transport; enter an exact model ID manually.")
		return r, nil
	}
	payload, _ := json.Marshal(struct {
		Request
		Cwd string `json:"cwd"`
	}{request, physical})
	childContext, cancel := context.WithCancel(ctx)
	defer cancel()
	var received atomic.Int64
	opts := watchdog.Opts{Context: childContext, Cwd: physical, IdleTimeoutMs: 15000, WallTimeoutMs: 15000, KillGraceMs: 1000,
		Env: childenv.EnvSlice(childenv.GateEnv(childenv.OSLookupEnv, []string{"FLYWHEEL_AGY_BIN", "FLYWHEEL_CODEX_BIN", "FLYWHEEL_CLAUDE_BIN", "CODEX_HOME", "CLAUDE_CONFIG_DIR"})),
		OnActivity: func(chunk, _ string) {
			if received.Add(int64(len(chunk))) > maxBytes {
				cancel()
			}
		},
		InteractStdin: func(stdin io.WriteCloser, _ func()) {
			_, _ = stdin.Write(append(payload, '\n'))
			_ = stdin.Close()
		}}
	result := runCommand(Command(), nil, opts)
	if result.Truncated || received.Load() > maxBytes || len(result.Stdout) > maxBytes {
		r.Truncated = true
		return unavailable(r, "OUTPUT_LIMIT"), nil
	}
	if result.ExitCode == nil || *result.ExitCode != 0 || result.Signal != "" || result.Cancelled || result.TimedOut || result.SpawnError != "" || result.CleanupError != "" {
		return unavailable(r, "CATALOG_UNAVAILABLE"), nil
	}
	return decode(result.Stdout, r), nil
}

func unavailable(r Report, code string) Report {
	r.Status = "unavailable"
	r.Models = []Model{}
	r.Error = &Error{Code: code, Message: "The native model catalog could not be read safely; exact model IDs can still be entered manually."}
	return r
}

func readable(value string, limit int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= limit && !invisible.MatchString(value)
}

// Validate the worker boundary independently. Never return raw process output
// or error text, even if a configured executable is missing or malfunctioning.
func decode(output string, expected Report) Report {
	var r Report
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(output), &fields)
	decoder := json.NewDecoder(strings.NewReader(output))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&r) != nil || decoder.Decode(new(any)) != io.EOF || r.Request != expected.Request ||
		string(fields["inferencePerformed"]) != "false" || !slices.Contains([]string{"false", "true"}, string(fields["truncated"])) ||
		r.Source != expected.Source || r.Scope != expected.Scope || r.InferencePerformed || r.AccountEntitlement != "unknown" ||
		!slices.Contains([]string{"available", "unavailable", "unsupported"}, r.Status) || r.Models == nil || len(r.Models) > maxModels ||
		r.Limitations == nil || len(r.Limitations) > 16 || r.Status != "available" && len(r.Models) != 0 {
		return unavailable(expected, "CATALOG_PROTOCOL")
	}
	seen := map[string]bool{}
	for _, model := range r.Models {
		if !identifier.MatchString(model.ID) || seen[model.ID] || !readable(model.Label, 256) ||
			model.Description != "" && !readable(model.Description, 1024) || len(model.Efforts) > 10 {
			return unavailable(expected, "CATALOG_PROTOCOL")
		}
		seen[model.ID] = true
		efforts := map[string]bool{}
		for _, effort := range model.Efforts {
			if !slices.Contains([]string{"", "none", "minimal", "low", "medium", "high", "xhigh", "max"}, effort) || efforts[effort] {
				return unavailable(expected, "CATALOG_PROTOCOL")
			}
			efforts[effort] = true
		}
		if model.DefaultEffort != nil && !efforts[*model.DefaultEffort] {
			return unavailable(expected, "CATALOG_PROTOCOL")
		}
	}
	for _, limitation := range r.Limitations {
		if !readable(limitation, 1024) {
			return unavailable(expected, "CATALOG_PROTOCOL")
		}
	}
	if r.Status != "available" {
		r = unavailable(r, "CATALOG_UNAVAILABLE")
	} else if r.Error != nil {
		return unavailable(expected, "CATALOG_PROTOCOL")
	}
	return r
}
