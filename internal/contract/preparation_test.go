package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preparationContractFixture(t *testing.T) Preparation {
	t.Helper()
	raw, err := templates.FS.ReadFile("preparation.example.json")
	require.NoError(t, err)
	p, err := ValidatePreparation(raw)
	require.NoError(t, err)
	return *p
}

func preparationJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}

func preparationValue(t *testing.T, value any) any {
	t.Helper()
	var decoded any
	require.NoError(t, json.Unmarshal(preparationJSON(t, value), &decoded))
	return decoded
}

func TestPreparationContractRejectsUnboundActionsAndAmbiguousAuthority(t *testing.T) {
	for name, change := range map[string]func(*Preparation){
		"absolute read":      func(p *Preparation) { p.ReadPaths[0] = "/etc/hosts" },
		"traversal":          func(p *Preparation) { p.ReadPaths[0] = "../settings.json" },
		"dirty path":         func(p *Preparation) { p.ReadPaths[0] = "src/../settings.json" },
		"backslash":          func(p *Preparation) { p.ReadPaths[0] = `src\settings.json` },
		"git metadata":       func(p *Preparation) { p.ReadPaths[0] = ".git" },
		"git metadata child": func(p *Preparation) { p.ReadPaths[0] = ".git/config" },
		"duplicate hypothesis": func(p *Preparation) {
			p.Hypotheses[1].ID = p.Hypotheses[0].ID
		},
		"duplicate action":     func(p *Preparation) { p.Actions[1].ID = p.Actions[0].ID },
		"ungranted path":       func(p *Preparation) { p.Actions[0].Path = "secret.json" },
		"unbound query":        func(p *Preparation) { p.Actions[0].Query = "secret" },
		"missing query":        func(p *Preparation) { p.Actions[1].Query = "" },
		"unknown hypothesis":   func(p *Preparation) { p.Actions[0].Hypotheses = []string{"undeclared"} },
		"repair without scope": func(p *Preparation) { p.Intent = "investigate-and-fix" },
		"report grants writes": func(p *Preparation) { p.ImplementationScope = []string{"src"} },
		"unbounded actions":    func(p *Preparation) { p.Limits.MaxActions = 65 },
	} {
		t.Run(name, func(t *testing.T) {
			p := preparationContractFixture(t)
			change(&p)
			_, err := ValidatePreparation(preparationJSON(t, p))
			require.Error(t, err)
		})
	}
	p := preparationContractFixture(t)
	p.Intent, p.ImplementationScope = "investigate-and-fix", []string{"src"}
	accepted, err := ValidatePreparation(preparationJSON(t, p))
	require.NoError(t, err)
	assert.Equal(t, []string{"src"}, accepted.ImplementationScope)
}

func TestPreparationJSONMustDecodeWithoutDroppingIntegerFields(t *testing.T) {
	p := preparationContractFixture(t)
	raw := preparationJSON(t, p)
	for _, invalid := range [][]byte{[]byte("{"), bytes.Repeat([]byte(" "), (128<<10)+1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1.0`), 1)} {
		got, err := ValidatePreparation(invalid)
		require.Error(t, err)
		assert.Nil(t, got)
	}
	unknown := strings.TrimSuffix(string(raw), "}") + `,"extra":true}`
	_, err := ValidatePreparation([]byte(unknown))
	require.Error(t, err)
}

func TestPreparationSchemasPropagateLoadingAndCompilationFailures(t *testing.T) {
	sentinel := errors.New("embedded schema unavailable")
	for _, compile := range []func(func(string) ([]byte, error)) (*jsonschema.Schema, error){compilePreparationSchema, compilePreparationStateSchema} {
		_, err := compile(func(string) ([]byte, error) { return nil, sentinel })
		require.ErrorIs(t, err, sentinel)
		_, err = compile(func(string) ([]byte, error) { return []byte("{"), nil })
		require.Error(t, err)
		_, err = compile(func(string) ([]byte, error) { return []byte(`{"type":"unknown"}`), nil })
		require.Error(t, err)
		_, err = compile(templates.FS.ReadFile)
		require.NoError(t, err)
	}
	original := preparationSchema
	preparationSchema = func() (*jsonschema.Schema, error) { return nil, sentinel }
	t.Cleanup(func() { preparationSchema = original })
	_, err := ValidatePreparation([]byte(`{}`))
	require.ErrorIs(t, err, sentinel)
}

func TestPreparationRecordSchemasAreTheFieldTruth(t *testing.T) {
	p := preparationContractFixture(t)
	hash := strings.Repeat("a", 64)
	invocation := map[string]any{"id": hash, "action-id": p.Actions[0].ID, "binding-hash": hash, "candidate-hash": hash, "started-at": "2026-11-12T00:00:00Z", "adopted": false, "reason": "baseline"}
	observation := map[string]any{"invocation": invocation, "status": "observed", "output": "", "content-hash": hash, "truncated": false, "finished-at": "2026-11-12T00:00:00Z", "duration-ms": 0}
	require.NoError(t, ValidatePreparationObservationValue(observation))
	delete(observation, "truncated")
	require.Error(t, ValidatePreparationObservationValue(observation), "required false values cannot be silently fabricated")
	value := map[string]any{"version": 1, "revision": 1, "run-id": "run-a", "cwd": "/repo", "contract": p, "contract-hash": hash, "status": "ready", "reason": "approved", "started-at": "now", "deadline": "later", "actions-used": 0, "output-bytes": 0, "elapsed-ms": 0, "observations": []any{}, "decisions": []any{}}
	require.NoError(t, ValidatePreparationStateValue(preparationValue(t, value)))
	value["status"] = "ready|running"
	require.Error(t, ValidatePreparationStateValue(preparationValue(t, value)), "status is an exact enum, never a substring")
	value["status"] = "ready"
	delete(value, "revision")
	require.Error(t, ValidatePreparationStateValue(preparationValue(t, value)))

	sentinel := errors.New("record schema unavailable")
	oldState, oldObservation := preparationStateSchema, preparationObservationSchema
	t.Cleanup(func() { preparationStateSchema, preparationObservationSchema = oldState, oldObservation })
	preparationStateSchema = func() (*jsonschema.Schema, error) { return nil, sentinel }
	preparationObservationSchema = func() (*jsonschema.Schema, error) { return nil, sentinel }
	require.ErrorIs(t, ValidatePreparationStateValue(value), sentinel)
	require.ErrorIs(t, ValidatePreparationObservationValue(observation), sentinel)
}
