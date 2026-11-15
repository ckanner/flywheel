package config

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func TestNativeProfilesLoadAndReferences(t *testing.T) {
	for _, harness := range []string{"dsh", "codex-pro", "claude-code-pro", "kimi"} {
		opts := `{"version":1,"agents":{"enabled":true}}`
		if harness == "dsh" {
			opts = `{"version":1,"preset":"heavy","agents":{"enabled":true},"team":{"enabled":true},"workflows":{"enabled":true}}`
		}
		if harness == "kimi" {
			opts = `{"version":1,"mode":"tower","agentProfile":"default"}`
		}
		c := Default()
		c.NativeProfiles = map[string]NativeProfile{"native": {Harness: harness, Options: json.RawMessage(opts)}}
		c.Loops = map[string]LoopBinding{"plan": {Harness: harness, NativeProfile: "native"}}
		c.Routes = map[string]RouteBinding{"default": {Harness: harness, NativeProfile: "native"}}
		c.Profiles = map[string]ModelProfile{"model": {Harness: harness, NativeProfile: "native"}}
		c.JudgePanel = &LoopBinding{Harness: harness, NativeProfile: "native"}
		c.ExecutionPolicy = &ExecutionPolicy{Bindings: map[string]ExecutionBinding{"native": {Harness: harness, NativeProfile: "native"}}}
		require.NoError(t, c.validateNativeProfiles(), harness)
		raw, err := json.Marshal(c.NativeProfiles)
		require.NoError(t, err)
		require.NoError(t, prepareNativeProfiles(map[string]json.RawMessage{"native-profiles": raw}))
	}
	_, err := parseConfig([]byte(`{"native-profiles":{"heavy":{"harness":"dsh","options":{"version":1,"preset":"heavy"}}},"profiles":{"flash":{"harness":"dsh","provider":"deepseek-account","model":"flash","channel":"account","native-profile":"heavy"}},"loops":{"plan":{"profile":"flash"}}}`), "native.json")
	require.NoError(t, err)
}

func TestClaudeAgentDefinitionEffort(t *testing.T) {
	for _, effort := range []string{"", "low", "medium", "high", "xhigh", "max"} {
		t.Run("accepted/"+effort, func(t *testing.T) {
			definition := map[string]any{"description": "Inspect the candidate", "prompt": "Report concrete defects", "model": "sonnet"}
			if effort != "" {
				definition["effort"] = effort
			}
			options, err := json.Marshal(map[string]any{"version": 1, "agents": map[string]any{"enabled": true, "definitions": map[string]any{"reviewer": definition}}})
			require.NoError(t, err)
			input := `{"native-profiles":{"claude":{"harness":"claude-code-pro","options":` + string(options) + `}}}`
			c, err := parseConfig([]byte(input), "native.json")
			require.NoError(t, err)
			require.NoError(t, c.validateNativeProfiles())
			require.JSONEq(t, string(options), string(c.NativeProfiles["claude"].Options), "loading must preserve the explicit child effort and leave an omitted effort absent")
		})
	}
	for _, effort := range []string{`""`, `"ultra"`, `"auto"`, `"HIGH"`, `null`, `true`, `1`, `{}`, `[]`} {
		t.Run("rejected/"+effort, func(t *testing.T) {
			input := `{"native-profiles":{"claude":{"harness":"claude-code-pro","options":{"version":1,"agents":{"enabled":true,"definitions":{"reviewer":{"description":"Inspect","prompt":"Report","effort":` + effort + `}}}}}}}`
			_, err := parseConfig([]byte(input), "native.json")
			require.ErrorContains(t, err, "native-profiles")
		})
	}
}

func TestNativeProfilesRejectUnsupportedContractsBeforeDispatch(t *testing.T) {
	for _, options := range []string{
		`{"version":2,"preset":"heavy"}`, `{"version":1,"preset":"unknown"}`,
		`{"version":1,"preset":"heavy","team":{"enabled":true}}`,
		`{"version":1,"preset":"heavy","agents":{"enabled":false},"workflows":{"enabled":true}}`,
		`{"version":1,"preset":"heavy","agents":{"enabled":true,"maxConcurrent":0}}`,
		`{"version":1,"preset":"heavy","lcu":{"enabled":true,"config":{"approval":"always"}}}`,
	} {
		c := Default()
		c.NativeProfiles = map[string]NativeProfile{"bad": {Harness: "dsh", Options: json.RawMessage(options)}}
		require.ErrorContains(t, c.Validate(), "native-profiles")
	}
	for _, input := range []string{`null`, `[]`, `{"bad key":{}}`, `{"n":{"harness":"codex-pro","options":{"version":1,"review":{"target":{"type":"uncommittedChanges"}}}}}`, `{"n":{"harness":"kimi","options":{"version":1,"mode":"tower","modelSlots":4}}}`} {
		_, err := parseConfig([]byte(`{"native-profiles":`+input+`}`), "bad.json")
		require.ErrorContains(t, err, "native-profiles")
	}
	mutations := []func(*Config){
		func(c *Config) { c.Loops = map[string]LoopBinding{"plan": {Harness: "dsh", NativeProfile: "missing"}} },
		func(c *Config) {
			c.Routes = map[string]RouteBinding{"default": {Harness: "kimi", NativeProfile: "known"}}
		},
		func(c *Config) {
			c.Profiles = map[string]ModelProfile{"model": {Harness: "kimi", NativeProfile: "known"}}
		},
		func(c *Config) { c.JudgePanel = &LoopBinding{Harness: "kimi", NativeProfile: "known"} },
		func(c *Config) {
			c.ExecutionPolicy = &ExecutionPolicy{Bindings: map[string]ExecutionBinding{"x": {Harness: "kimi", NativeProfile: "known"}}}
		},
	}
	for _, mutate := range mutations {
		c := Default()
		c.NativeProfiles = map[string]NativeProfile{"known": {Harness: "dsh", Options: json.RawMessage(`{"version":1,"preset":"daily"}`)}}
		mutate(&c)
		require.ErrorContains(t, c.validateNativeProfiles(), "native-profile")
	}
	c := Default()
	c.NativeProfiles = map[string]NativeProfile{"broken": {Options: json.RawMessage(`{`)}}
	require.ErrorContains(t, c.validateNativeProfiles(), "invalid JSON")
	old := nativeProfilesSchema
	t.Cleanup(func() { nativeProfilesSchema = old })
	nativeProfilesSchema = func() (*jsonschema.Schema, error) { return nil, errors.New("embedded schema unavailable") }
	require.ErrorContains(t, validateNativeProfiles(map[string]any{}), "schema unavailable")
}
