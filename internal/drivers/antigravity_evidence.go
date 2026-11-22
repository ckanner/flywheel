package drivers

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// init.tools is the CLI catalog, not the managed agent's allowlist. The pinned
// adapter enforces that allowlist and rejects actual tool/subagent steps. Bind
// its attestation to this invocation without reopening the already removed file;
// the same receipt must remain verifiable during read-only recovery.
func validAntigravityEvidenceAgent(raw json.RawMessage, cwd, runID, observedAgent string) bool {
	var native struct {
		ManagedAgent *struct {
			Name       string `json:"name"`
			Path       string `json:"path"`
			Digest     string `json:"digest"`
			OwnerRunID string `json:"ownerRunId"`
			Cleanup    string `json:"cleanup"`
		} `json:"managedAgent"`
	}
	if json.Unmarshal(raw, &native) != nil || native.ManagedAgent == nil {
		return false
	}
	agent := native.ManagedAgent
	name := "dsh-evidence-" + runID
	return agent.Name == name && observedAgent == name && filepath.Base(name) == name && !strings.ContainsAny(name, "\\\x00\r\n") &&
		agent.OwnerRunID == runID && agent.Path == filepath.Join(cwd, ".agents", "agents", name+".md") &&
		len(agent.Digest) == 64 && strings.Trim(agent.Digest, "0123456789abcdef") == "" && agent.Cleanup == "confirmed"
}
