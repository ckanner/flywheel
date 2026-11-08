package config

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ckanner/flywheel/templates"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// SessionPolicyVersion is independent from both workflow and worker protocols.
const SessionPolicyVersion = 1

type SessionsConfig struct {
	Mode          string               `json:"mode"`
	PolicyVersion int                  `json:"policy-version"`
	Roles         map[string]string    `json:"roles,omitempty"`
	Quality       SessionQualityPolicy `json:"quality,omitzero"`
}

// Quality limits are disabled at zero. They bound verified workload observations,
// not native context capacity; Go never invokes an unmetered model compressor.
type SessionQualityPolicy struct {
	MaxConfirmedTurns    int   `json:"max-confirmed-turns,omitempty"`
	MaxUnproductiveTurns int   `json:"max-unproductive-turns,omitempty"`
	MaxRunInputTokens    int64 `json:"max-run-input-tokens,omitempty"`
}

func (s SessionsConfig) ModeForRole(role string) string {
	if mode, found := s.Roles[role]; found {
		return mode
	}
	return s.Mode
}

func (s SessionsConfig) validate() error {
	// Round-trip the same concrete policy that inspect, snapshots and config
	// files expose. Validating only mode/version would silently ignore invalid
	// role overrides or quality limits constructed by in-process callers.
	var value any
	_ = json.Unmarshal(configJSON(s), &value)
	return validateSessionPolicy(value)
}

// SupportsScopedSessions describes implemented adapter contracts, not evidence
// that a particular installed backend can restore a saved history.
func SupportsScopedSessions(harness string) bool {
	return harness == "codex-pro" || harness == "claude-code-pro" || harness == "dsh" || harness == "kimi"
}

// MarshalJSON preserves omission versus explicit false in effective config
// snapshots. A legacy programmatic true remains an explicit opt-in on disk.
func (b LoopBinding) MarshalJSON() ([]byte, error) {
	type binding LoopBinding
	var resident *bool
	if b.ResidentSessionSet || b.ResidentSession {
		resident = &b.ResidentSession
	}
	return json.Marshal(struct {
		binding
		ResidentSession *bool `json:"resident-session,omitempty"`
	}{binding(b), resident})
}

func markResidentSessionPresence(raw []byte, cfg *Config) {
	var presence struct {
		Loops      map[string]map[string]json.RawMessage `json:"loops"`
		JudgePanel map[string]json.RawMessage            `json:"judge-panel"`
	}
	// expandProfiles and typed decoding have already checked these shapes.
	_ = json.Unmarshal(raw, &presence)
	for role, fields := range presence.Loops {
		binding := cfg.Loops[role]
		_, binding.ResidentSessionSet = fields["resident-session"]
		cfg.Loops[role] = binding
	}
	if cfg.JudgePanel != nil {
		_, cfg.JudgePanel.ResidentSessionSet = presence.JudgePanel["resident-session"]
	}
}

const sessionPolicySchemaID = "https://flywheel.local/schemas/session-policy.schema.json"

func compileSessionPolicySchema(load func(string) ([]byte, error)) (*jsonschema.Schema, error) {
	raw, err := load("session-policy.schema.json")
	if err != nil {
		return nil, fmt.Errorf("embedded session policy schema: %w", err)
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("embedded session policy schema: invalid JSON: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	// Only one resource is registered, so AddResource cannot encounter a
	// duplicate URL (the same invariant as internal/contract.compileAll).
	_ = compiler.AddResource(sessionPolicySchemaID, document)
	return compiler.Compile(sessionPolicySchemaID)
}

var sessionPolicySchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileSessionPolicySchema(templates.FS.ReadFile)
})

func validateSessionPolicy(value any) error {
	schema, err := sessionPolicySchema()
	if err != nil {
		return fmt.Errorf("sessions schema: %w", err)
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("sessions: %w", err)
	}
	return nil
}

func prepareSessions(root map[string]json.RawMessage) error {
	raw, present := root["sessions"]
	if !present {
		return nil
	}
	object, err := configObject(raw, "sessions")
	if err != nil {
		return err
	}
	// Decode validated JSON into the value types consumed by jsonschema. Notes
	// follow the same supported annotation rule as other configuration sections.
	raw = configJSON(object)
	var value any
	_ = json.Unmarshal(raw, &value)
	if err := validateSessionPolicy(value); err != nil {
		return err
	}
	root["sessions"] = raw
	return nil
}
