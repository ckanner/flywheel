package contract

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Preparation is an approved, bounded evidence-gathering contract. It is kept
// separate from Spec: investigation completion is never implementation completion.
type Preparation struct {
	Version             int                    `json:"version"`
	ID                  string                 `json:"id"`
	Goal                string                 `json:"goal"`
	Expected            string                 `json:"expected"`
	Intent              string                 `json:"intent"`
	Environment         string                 `json:"environment"`
	Window              string                 `json:"window"`
	KnownEvidence       []string               `json:"known-evidence"`
	ReadPaths           []string               `json:"read-paths"`
	ImplementationScope []string               `json:"implementation-scope"`
	Hypotheses          []Hypothesis           `json:"hypotheses"`
	Actions             []InvestigationAction  `json:"actions"`
	Limits              InvestigationLimits    `json:"limits"`
	Decision            *InvestigationDecision `json:"decision,omitempty"`
}

type Hypothesis struct {
	ID        string `json:"id"`
	Claim     string `json:"claim"`
	Falsifier string `json:"falsifier"`
}

// Actions have frozen arguments. File paths are exact grants; listing a
// directory never implicitly grants reads of its descendants. Command actions
// explicitly grant a host process and run against copied approved inputs.
type InvestigationAction struct {
	ID         string                `json:"id"`
	Kind       string                `json:"kind"`
	Path       string                `json:"path,omitempty"`
	Query      string                `json:"query,omitempty"`
	Purpose    string                `json:"purpose"`
	Hypotheses []string              `json:"hypotheses"`
	Command    *InvestigationCommand `json:"command,omitempty"`
	HTTP       *InvestigationHTTP    `json:"http,omitempty"`
}

type InvestigationCommand struct {
	Argv              []string `json:"argv"`
	Inputs            []string `json:"inputs"`
	Env               []string `json:"env"`
	TimeoutMS         int64    `json:"timeout-ms"`
	ExpectExit        int      `json:"expect-exit"`
	ExecutionBoundary string   `json:"execution-boundary"`
}

type InvestigationHTTP struct {
	URL          string            `json:"url"`
	Method       string            `json:"method"`
	HeadersEnv   map[string]string `json:"headers-env"`
	TimeoutMS    int64             `json:"timeout-ms"`
	MaxBytes     int               `json:"max-bytes"`
	ExpectStatus int               `json:"expect-status"`
}

type InvestigationLimits struct {
	MaxActions     int `json:"max-actions"`
	MaxSeconds     int `json:"max-seconds"`
	MaxFileBytes   int `json:"max-file-bytes"`
	MaxOutputBytes int `json:"max-output-bytes"`
}

// KeyEnv stores a credential NAME only. Start freezes this record from the
// unified JEV configuration; caller-provided historical modes cannot opt in.
type InvestigationDecision struct {
	Backend      string                        `json:"backend,omitempty"`
	Subscription *decision.SubscriptionBinding `json:"subscription-binding,omitempty"`
	Policy       decision.Policy               `json:"policy"`
	Endpoint     string                        `json:"endpoint,omitempty"`
	Model        string                        `json:"model"`
	KeyEnv       string                        `json:"key-env,omitempty"`
	TimeoutMS    int64                         `json:"timeout-ms"`
	HardBudget   bool                          `json:"hard-budget"`
}

var preparationSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compilePreparationSchema(templates.FS.ReadFile)
})

func compilePreparationSchema(load func(string) ([]byte, error)) (*jsonschema.Schema, error) {
	raw, err := load("preparation.schema.json")
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	const id = "https://flywheel.local/schemas/preparation.schema.json"
	// A fresh compiler has no resource with this identity.
	_ = c.AddResource(id, value)
	return c.Compile(id)
}

func ValidatePreparation(raw []byte) (*Preparation, error) {
	if len(raw) > 128<<10 {
		return nil, fmt.Errorf("preparation exceeds 128 KiB")
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	schema, err := preparationSchema()
	if err != nil {
		return nil, err
	}
	if err := validateAgainst(schema, value, "preparation contract"); err != nil {
		return nil, err
	}
	var p Preparation
	// JSON Schema accepts integral numbers such as 1.0; Go's typed integer
	// decoder deliberately rejects that syntax. Never return a partially decoded
	// contract when either representation is invalid.
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if d := p.Decision; d != nil && d.Backend == "subscription" {
		if err := decision.ValidateSubscriptionBinding(d.Subscription); err != nil {
			return nil, err
		}
		if d.Model != d.Subscription.Model {
			return nil, fmt.Errorf("preparation decision model contradicts its frozen subscription binding")
		}
	}
	paths := map[string]bool{}
	for _, path := range append(append([]string{}, p.ReadPaths...), p.ImplementationScope...) {
		if !filepath.IsLocal(path) || filepath.ToSlash(filepath.Clean(path)) != path || strings.Contains(path, "\\") || path == ".git" || strings.HasPrefix(path, ".git/") {
			return nil, fmt.Errorf("preparation path must be a clean repository-relative path: %s", path)
		}
	}
	for _, path := range p.ReadPaths {
		paths[path] = true
	}
	hypotheses := map[string]bool{}
	for _, h := range p.Hypotheses {
		if hypotheses[h.ID] {
			return nil, fmt.Errorf("duplicate hypothesis: %s", h.ID)
		}
		hypotheses[h.ID] = true
	}
	actions := map[string]bool{}
	for _, a := range p.Actions {
		if actions[a.ID] {
			return nil, fmt.Errorf("duplicate, ungranted, or inconsistent investigation action: %s", a.ID)
		}
		if err := validateInvestigationAction(a, paths, p.Limits); err != nil {
			return nil, err
		}
		actions[a.ID] = true
		for _, id := range a.Hypotheses {
			if !hypotheses[id] {
				return nil, fmt.Errorf("action %s refers to unknown hypothesis %s", a.ID, id)
			}
		}
	}
	if (p.Intent == "investigate-and-fix") != (len(p.ImplementationScope) > 0) {
		return nil, fmt.Errorf("repair intent requires an explicit implementation scope; report-only intent grants none")
	}
	return &p, nil
}
