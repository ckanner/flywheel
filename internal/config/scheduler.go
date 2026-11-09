package config

import (
	"encoding/json"
	"fmt"
	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"sync"
)

var schedulerSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compilePolicySchema("scheduler-policy.schema.json", templates.FS.ReadFile)
})

func validateScheduler(value any) error {
	schema, err := schedulerSchema()
	if err != nil {
		return err
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("scheduler: %w", err)
	}
	return nil
}
func prepareScheduler(root map[string]json.RawMessage) error {
	raw, ok := root["scheduler"]
	if !ok {
		return nil
	}
	object, err := configObject(raw, "scheduler")
	if err != nil {
		return err
	}
	var value any
	_ = json.Unmarshal(configJSON(object), &value)
	if err := validateScheduler(value); err != nil {
		return err
	}
	root["scheduler"] = configJSON(object)
	return nil
}
