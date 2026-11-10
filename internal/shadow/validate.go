package shadow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"unicode/utf8"

	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const maxDocument = 1 << 20
const schemaID = "https://flywheel.local/schemas/workflow-shadow.schema.json"

var secretPattern = regexp.MustCompile(`(?i)(bearer[ \t]+[a-z0-9._~+/-]{16,}|(api[_-]?key|access[_-]?token|password|secret)[ \t]*[:=][ \t]*["']?[a-z0-9._~+/-]{16,}|sk-[a-z0-9_-]{16,}|-----BEGIN ([A-Z ]+)?PRIVATE KEY-----)`)

func compileSchemas(load func(string) ([]byte, error)) (map[string]*jsonschema.Schema, error) {
	raw, err := load("workflow-shadow.schema.json")
	if err != nil {
		return nil, err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	_ = c.AddResource(schemaID, doc) // one unique, constant absolute resource ID
	result := map[string]*jsonschema.Schema{}
	for _, name := range []string{"dataset", "response", "record", "target"} {
		s, err := c.Compile(schemaID + "#/$defs/" + name)
		if err != nil {
			return nil, err
		}
		result[name] = s
	}
	return result, nil
}

var schemas = sync.OnceValues(func() (map[string]*jsonschema.Schema, error) {
	return compileSchemas(templates.FS.ReadFile)
})

// readValue rejects duplicate keys before either schema validation or hashing.
func readValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return token, nil
	}
	if delimiter == '{' {
		object := map[string]any{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			name := key.(string) // encoding/json requires an object key here
			if _, duplicate := object[name]; duplicate {
				return nil, fmt.Errorf("duplicate JSON key")
			}
			value, err := readValue(decoder)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		_, err := decoder.Token()
		return object, err
	}
	// At a value boundary the JSON scanner permits only { or [ delimiters.
	array := []any{}
	for decoder.More() {
		value, err := readValue(decoder)
		if err != nil {
			return nil, err
		}
		array = append(array, value)
	}
	_, err = decoder.Token()
	return array, err
}

func validate(raw []byte, kind string) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("shadow JSON must be UTF-8")
	}
	if len(raw) > maxDocument {
		return fmt.Errorf("shadow document exceeds 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := readValue(decoder)
	if err != nil {
		return fmt.Errorf("invalid shadow JSON")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("shadow JSON must contain one document")
	}
	compiled, err := schemas()
	if err != nil {
		return fmt.Errorf("shadow schema unavailable: %w", err)
	}
	if err := compiled[kind].Validate(value); err != nil {
		// Do not echo untrusted field values, which can contain credentials.
		return fmt.Errorf("invalid shadow %s; check workflow-shadow.schema.json", kind)
	}
	return nil
}

func ParseDataset(raw []byte) (Dataset, error) {
	if secretPattern.Match(raw) {
		return Dataset{}, fmt.Errorf("possible credential in shadow data; prepare a reviewed summary")
	}
	if err := validate(raw, "dataset"); err != nil {
		return Dataset{}, err
	}
	var dataset Dataset
	_ = json.Unmarshal(raw, &dataset) // the schema fixes every field type and bound
	if secretPattern.Match(jsonBytes(dataset)) {
		return Dataset{}, fmt.Errorf("possible encoded credential in shadow data")
	}
	seen := map[string]bool{}
	for _, c := range dataset.Cases {
		if seen[c.ID] {
			return Dataset{}, fmt.Errorf("duplicate shadow case id")
		}
		seen[c.ID] = true
	}
	return dataset, nil
}

func LoadDataset(path string) (Dataset, error) {
	file, err := os.Open(path)
	if err != nil {
		return Dataset{}, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxDocument+1))
	if err != nil {
		return Dataset{}, err
	}
	return ParseDataset(raw)
}
