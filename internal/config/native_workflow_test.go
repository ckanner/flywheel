package config

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNativeClaudeWorkflowRangeMatchesPublicMachineAPI(t *testing.T) {
	for _, maximum := range []int{1, 16, 17, 256, 0, 257} {
		c := Default()
		c.NativeProfiles = map[string]NativeProfile{"workflows": {
			Harness: "claude-code-pro",
			Options: json.RawMessage(fmt.Sprintf(`{"version":1,"workflows":{"enabled":true,"maxConcurrent":%d}}`, maximum)),
		}}
		err := c.validateNativeProfiles()
		if maximum >= 1 && maximum <= 256 {
			require.NoError(t, err, "the orchestrator must preserve the public machine API range")
		} else {
			require.Error(t, err)
		}
	}
}
