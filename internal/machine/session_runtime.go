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
	switch a.Binding.Harness {
	case "codex-pro":
		keys = append(keys, "CODEX_HOME", drivers.CodexProBinaryEnv, drivers.CodexProWorkerEnv)
	case "dsh":
		keys = append(keys, "DSH_HOME", "FLYWHEEL_DSH_PROFILE", "FLYWHEEL_DSH_BIN", drivers.DSHWorkerEnv)
	case "kimi":
		keys = append(keys, drivers.KimiCommandEnv)
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
	return sessionHash([]any{references, credentials})
}
