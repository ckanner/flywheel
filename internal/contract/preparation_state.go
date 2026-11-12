package contract

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

var preparationStateSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compilePreparationStateSchema(templates.FS.ReadFile)
})

var preparationObservationSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compilePreparationRecordSchema(templates.FS.ReadFile, "#/$defs/observation")
})

func compilePreparationStateSchema(load func(string) ([]byte, error)) (*jsonschema.Schema, error) {
	return compilePreparationRecordSchema(load, "")
}

func compilePreparationRecordSchema(load func(string) ([]byte, error), fragment string) (*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()
	const prefix = "https://flywheel.local/schemas/"
	for _, name := range []string{"preparation.schema.json", "preparation-state.schema.json"} {
		raw, err := load(name)
		if err != nil {
			return nil, err
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		// Each fixed URL is registered exactly once in this fresh compiler.
		_ = c.AddResource(prefix+name, value)
	}
	return c.Compile(prefix + "preparation-state.schema.json" + fragment)
}

// ValidatePreparationStateValue is the shared field/type truth for the durable
// record. The state owner additionally validates history references and budgets.
func ValidatePreparationStateValue(value any) error {
	schema, err := preparationStateSchema()
	if err != nil {
		return err
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("invalid preparation state: %w", err)
	}
	return nil
}

func ValidatePreparationObservationValue(value any) error {
	schema, err := preparationObservationSchema()
	if err != nil {
		return err
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("invalid preparation observation: %w", err)
	}
	return nil
}
