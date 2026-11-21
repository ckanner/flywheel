package machine

import (
	"encoding/json"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/stretchr/testify/require"
)

func TestAntigravityPlanningCollectsChildrenInForeground(t *testing.T) {
	for _, mode := range []string{"native", "subagents"} {
		t.Run(mode, func(t *testing.T) {
			_, c, _ := briefEngineContext(t)
			c.Config.Permissions.Mode = "full-access"
			c.Config.Loops["design-plan"] = config.LoopBinding{Harness: "antigravity", Model: "gemini-4-neon-high", Thinking: "high", NativeProfile: "research"}
			c.Config.NativeProfiles = map[string]config.NativeProfile{"research": {Harness: "antigravity", Options: json.RawMessage(`{"version":1,"mode":"` + mode + `"}`)}}
			called := false
			c.Drivers.DesignPlan.(*fnDriver).fn = func(a drivers.CallArgs) drivers.Result {
				called = true
				require.Equal(t, "research", a.Binding.NativeProfile)
				require.Contains(t, a.Prompt, "没有向 Flywheel 暴露后台子代理的生命周期")
				require.Contains(t, a.Prompt, "必须前台完成并立即消费其结论")
				require.Contains(t, a.Prompt, "所有调查都必须在当前调用结束前完成并汇总")
				return planningJSONResult(t, engineDesignOutput(t))
			}
			i := pendingDesignRequest(t, c)
			_, _, _, err := c.invokePlanning(&c.State.Planning.Requests[i], false)
			require.NoError(t, err)
			require.True(t, called)
		})
	}
}
