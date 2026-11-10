package config

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// JEVConfig controls an optional observer, never the workflow selector.
type JEVConfig struct {
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	Model         string `json:"model"`
	ExpectedModel string `json:"expected-model"`
	KeyEnv        string `json:"key-env"`
	TimeoutMS     int64  `json:"timeout-ms"`
}

var jevSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compilePolicySchema("jev-policy.schema.json", templates.FS.ReadFile)
})

func validateJEV(value any) error {
	schema, err := jevSchema()
	if err != nil {
		return err
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("jev-shadow: %w", err)
	}
	return nil
}

func prepareJEV(root map[string]json.RawMessage) error {
	raw, ok := root["jev-shadow"]
	if !ok {
		return nil
	}
	object, err := configObject(raw, "jev-shadow")
	if err != nil {
		return err
	}
	var value any
	_ = json.Unmarshal(configJSON(object), &value)
	if err := validateJEV(value); err != nil {
		return err
	}
	root["jev-shadow"] = configJSON(object)
	return nil
}

func (j *JEVConfig) normalize() {
	if j.Model == "" {
		j.Model = "jev"
	}
	if j.ExpectedModel == "" {
		j.ExpectedModel = "jev-1.14.0"
	}
	if j.KeyEnv == "" {
		j.KeyEnv = "MEROUTER_API_KEY"
	}
	if j.TimeoutMS == 0 {
		j.TimeoutMS = 5000
	}
}
