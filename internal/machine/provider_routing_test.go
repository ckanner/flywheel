package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every role crosses the same two account fallbacks: an explicitly selected DSH
// adapter, a different channel-owned adapter, then a harness with no DSH route.
// This catches both lost provider fields and a stale adapter after fallback.
func TestProviderRoutesAcrossPlanExecuteReviewAndJudge(t *testing.T) {
	for _, role := range []string{"plan", "execute", "review", "judge"} {
		t.Run(role, func(t *testing.T) {
			fx := newGitFixture(t)
			sd := happyDrivers()
			var calls []drivers.CallArgs
			call := func(a drivers.CallArgs) drivers.Result {
				calls = append(calls, a)
				var result drivers.Result
				if a.Binding.Channel != "codex-account" {
					result = drivers.MockResult(func(r *drivers.Result) {
						message := "429 rate limit"
						r.ErrorMessage = &message
						r.Settled = false
					})
				} else {
					switch role {
					case "plan":
						result = sd.mock.Plan(a)
					case "execute":
						result = sd.mock.Execute(a)
					case "review":
						result = sd.mock.Review(a)
					case "judge":
						result = sd.mock.Judge(a)
					}
				}
				result.Harness = a.Binding.Harness
				result.Provider = a.Binding.Provider
				if result.Harness == "codex" {
					result.Provider = "openai"
				}
				return result
			}
			sd.planFn, sd.execFn, sd.reviewFn, sd.judgeFn = call, call, call, call
			c := verdictCtx(t, fx, sd)
			c.Config.Channels["primary-account"] = config.Channel{Harness: "dsh", Fallback: "backup-account"}
			c.Config.Channels["backup-account"] = config.Channel{Harness: "dsh", Provider: "backup-adapter", Fallback: "codex-account",
				Models: map[string]string{"plan": "backup-model", "default": "backup-model", "review": "backup-model", "judge": "backup-model"}}
			c.Config.Channels["codex-account"] = config.Channel{Harness: "codex",
				Models: map[string]string{"plan": "codex-model", "default": "codex-model", "review": "codex-model", "judge": "codex-model"}}
			binding := config.LoopBinding{Harness: "dsh", Provider: "explicit-adapter", Channel: "primary-account", Model: "primary-model", Thinking: "high", ResidentSession: true}
			c.Config.Loops[role] = binding
			c.Config.Routes["default"] = config.RouteBinding{Harness: binding.Harness, Provider: binding.Provider, Channel: binding.Channel, Model: binding.Model, Thinking: binding.Thinking}
			// Even a matching-looking DSH session must not activate Kimi's resume path.
			c.State.PlanSession = map[string]any{"session-id": "old-dsh-session", "harness": "dsh", "model": binding.Model, "channel": binding.Channel, "cwd": c.Cwd, "thinking": binding.Thinking}
			switch role {
			case "plan":
				out, err := c.planStep(milestoneOf(t, c), "")
				require.NoError(t, err)
				assert.True(t, out.OK)
				assert.Nil(t, c.State.PlanSession)
			case "execute":
				out, err := c.executeStep(c.Packets["M01-T01"])
				require.NoError(t, err)
				assert.True(t, out.Success, "%+v", out)
			case "review":
				seedVerification(t, c, "M01-T01")
				out, err := c.reviewStep(c.Packets["M01-T01"], nil)
				require.NoError(t, err)
				assert.True(t, out.Pass, "%+v", out)
			case "judge":
				out, err := c.judgeStep(milestoneOf(t, c))
				require.NoError(t, err)
				assert.True(t, out.Pass, "%+v", out)
			}
			require.Len(t, calls, 3)
			for i, expected := range []struct{ channel, provider, harness, model string }{
				{"primary-account", "explicit-adapter", "dsh", "primary-model"},
				{"backup-account", "backup-adapter", "dsh", "backup-model"},
				{"codex-account", "", "codex", "codex-model"},
			} {
				assert.Equal(t, expected.channel, calls[i].Binding.Channel)
				assert.Equal(t, expected.provider, calls[i].Binding.Provider)
				assert.Equal(t, expected.harness, calls[i].Binding.Harness)
				assert.Equal(t, expected.model, calls[i].Model)
				assert.Equal(t, "high", calls[i].Binding.Thinking)
				assert.Empty(t, calls[i].SessionID, "DSH and Codex calls are fresh even for a resident Plan binding")
				permission := "read-only"
				if role == "plan" || role == "execute" {
					permission = "workspace-write"
				}
				assert.Equal(t, permission, calls[i].PermissionMode)
			}
			raw, err := os.ReadFile(filepath.Join(c.TaskDir, "metrics.jsonl"))
			require.NoError(t, err)
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			require.Len(t, lines, 3)
			for i, line := range lines {
				var entry map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &entry))
				assert.Equal(t, calls[i].Binding.Channel, entry["channel"])
				assert.Equal(t, []string{"explicit-adapter", "backup-adapter", "openai"}[i], entry["provider"])
			}
		})
	}
}

func TestJudgePanelPreservesExplicitAndChannelProviders(t *testing.T) {
	fx := newGitFixture(t)
	sd := happyDrivers()
	c := verdictCtx(t, fx, sd)
	c.Config.Channels["primary-subscription"] = config.Channel{Harness: "dsh", Provider: "chatgpt"}
	c.Config.Channels["second-subscription"] = config.Channel{Harness: "dsh"}
	c.Config.Loops["judge"] = config.LoopBinding{Harness: "dsh", Channel: "primary-subscription", Model: "primary-model", Thinking: "high"}
	c.Config.JudgePanel = &config.LoopBinding{Harness: "dsh", Provider: "second-adapter", Channel: "second-subscription", Model: "second-model", Thinking: "low"}
	out, err := c.judgeStep(milestoneOf(t, c))
	require.NoError(t, err)
	assert.True(t, out.Pass)
	require.Len(t, sd.log.judge, 2)
	assert.Equal(t, "chatgpt", sd.log.judge[0].Binding.Provider)
	assert.Equal(t, "second-adapter", sd.log.judge[1].Binding.Provider)
	assert.Equal(t, "second-subscription", sd.log.judge[1].Binding.Channel)
	assert.Equal(t, "low", sd.log.judge[1].Binding.Thinking)
	assert.Equal(t, "read-only", sd.log.judge[1].PermissionMode)
}

func TestMetricsDoesNotInventAnEffectiveProvider(t *testing.T) {
	c, dir := spawnMetricsCtx(t)
	r := drivers.MockResult(func(r *drivers.Result) { r.Provider = "" })
	c.recordSpawnMetrics(spawnMetricsArgs{role: "review", channel: "account-only", result: &r})
	entry := lastMetricsLine(t, dir)
	assert.Equal(t, "account-only", entry["channel"])
	assert.Contains(t, entry, "provider")
	assert.Nil(t, entry["provider"], "an account label does not attest which model adapter ran")
}
