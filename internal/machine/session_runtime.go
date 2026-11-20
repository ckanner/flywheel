package machine

import (
	"os"
	"sort"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/drivers"
)

// Hash actual non-secret runtime/storage references, never credential values.
func sessionRuntimeIdentity(a drivers.CallArgs) string {
	keys := append([]string{}, childenv.ChildEnvKeys...)
	adapter := ""
	switch a.Binding.Harness {
	case "codex-pro":
		keys = append(keys, "CODEX_HOME", drivers.CodexProBinaryEnv, drivers.CodexProWorkerEnv)
		adapter = drivers.CodexProPackageVersion + "/lifecycle-v1"
	case "antigravity":
		keys = append(keys, drivers.AntigravityBinaryEnv, drivers.AntigravityWorkerEnv)
		adapter = drivers.AntigravityPackageVersion + "/lifecycle-v1"
	case "claude-code-pro":
		keys = append(keys, "CLAUDE_CONFIG_DIR", drivers.ClaudeCodeProBinaryEnv, drivers.ClaudeCodeProWorkerEnv)
		adapter = drivers.ClaudeCodeProPackageVersion + "/lifecycle-v1"
	case "dsh":
		keys = append(keys, "DSH_HOME", "FLYWHEEL_DSH_PROFILE", "FLYWHEEL_DSH_BIN", drivers.DSHWorkerEnv)
	case "kimi":
		keys = append(keys, drivers.KimiCommandEnv, "KIMI_CODE_HOME")
		if len(a.Native) != 0 {
			keys = append(keys, drivers.KimiNativeWorkerEnv)
			adapter = drivers.KimiNativeProtocol
		}
	}
	references := map[string]string{}
	for _, name := range keys {
		if value, ok := os.LookupEnv(name); ok {
			references[name] = value
		}
	}
	credentials := make([]string, 0, len(a.APIKeys))
	for name := range a.APIKeys {
		credentials = append(credentials, name)
	}
	sort.Strings(credentials)
	identity := []any{references, credentials}
	if a.Subscription != nil || a.EvidenceOnly {
		identity = append(identity, a.Subscription, a.EvidenceOnly)
	}
	if a.WorkspaceCwd != "" {
		identity = append(identity, "project-workspace-v1", a.WorkspaceCwd)
	}
	if len(a.CandidateScope) > 0 {
		identity = append(identity, "shared-declared-scope-v1", a.CandidateScope)
	}
	if adapter != "" {
		identity = append(identity, adapter)
	}
	return sessionHash(identity)
}
