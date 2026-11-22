package machine

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/imageinput"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

// This starts the real Go Execute stage, the shipped worker, its pinned public
// companion and the real Agent SDK. Only the native CLI is an offline fixture.
// The fixture writes the artifact/report only after an actual SDK image block.
func TestPacketImagesClaudeSDKOfflineBridge(t *testing.T) {
	if os.Getenv("FLYWHEEL_TEST_NODE_BRIDGE") != "1" {
		t.Skip("set FLYWHEEL_TEST_NODE_BRIDGE=1 after building plugins/dsh with its pinned public Claude companion")
	}
	worker, err := filepath.Abs("../../plugins/dsh/dist/claude-code-pro-worker.js")
	require.NoError(t, err)
	source, err := os.ReadFile("../../plugins/dsh/test/fixtures/fake-claude-code-pro.js")
	require.NoError(t, err)
	native := filepath.Join(t.TempDir(), "fake-claude.mjs")
	require.NoError(t, os.WriteFile(native, source, 0o755))
	t.Setenv(drivers.ClaudeCodeProBinaryEnv, native)
	t.Setenv(drivers.ClaudeCodeProWorkerEnv, worker)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	for _, scenario := range []string{"execute", "recover-original", "recover-changed-source"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, original := imageExecutionFixture(t)
			c.Drivers = drivers.CreateDrivers(c.Config)
			require.Equal(t, "subscription-only", c.Config.Subscription.Mode)
			require.Equal(t, "claude-code-pro", c.Config.Routes["multimodal"].Harness)
			writeFile(t, filepath.Join(fx.root, ".git", "info", "exclude"), "claude-fixture*\nclaude-history-*\nclaude-preflight.jsonl\n")
			settings := map[string]any{
				"scenario": "image-execute", "executeArtifact": "build/M01-T01.txt", "executeArtifactText": "native CLI saw the real image\n",
				"executeReport": "docs/minitask/reports/M01-T01.done.json",
				"executeReportBody": map[string]any{
					"requirements-covered": []string{"R-01"}, "changed-files": []string{"build/M01-T01.txt"}, "commands-run": []string{}, "failures": []string{},
				},
			}
			writeFile(t, filepath.Join(fx.root, "claude-fixture.json"), string(mustJSON(settings)))
			previousPersist := persistSessionReceipt
			t.Cleanup(func() { persistSessionReceipt = previousPersist })
			if scenario != "execute" {
				persistSessionReceipt = func(task, key string, receipt state.SessionReceipt) (string, error) {
					if !strings.HasSuffix(key, "/lifecycle") {
						return "", errors.New("offline Go interruption after public cleanup ACK")
					}
					return previousPersist(task, key, receipt)
				}
			}
			p := c.Packets["M01-T01"]
			out, err := c.executeStep(p)
			if scenario == "execute" {
				require.NoError(t, err)
				require.True(t, out.Success, "%+v", out)
				require.Equal(t, "executed", c.State.Packets[p.PacketID].Status)
				require.NotNil(t, c.State.Packets[p.PacketID].Evidence.Executor)
				gate := c.l1Step(p)
				require.True(t, gate.Pass, "%+v", gate)
			} else {
				require.ErrorContains(t, err, "terminal receipt could not be saved")
				require.Equal(t, "cleanup-confirmed", c.sessionRecord("execute", p.PacketID).Last.Recovery.Event)
			}
			persistSessionReceipt = previousPersist
			artifact, err := os.ReadFile(filepath.Join(fx.root, "build", "M01-T01.txt"))
			require.NoError(t, err)
			require.Equal(t, "native CLI saw the real image\n", string(artifact))
			readLog := func(name string) []map[string]any {
				data, err := os.ReadFile(filepath.Join(fx.root, name))
				require.NoError(t, err)
				var frames []map[string]any
				for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
					var frame map[string]any
					require.NoError(t, json.Unmarshal([]byte(line), &frame))
					frames = append(frames, frame)
				}
				return frames
			}
			log := readLog("claude-fixture.jsonl")
			require.Len(t, log, 1)
			content := log[0]["userContent"].([]any)
			imageCount := 0
			for _, block := range content {
				block := block.(map[string]any)
				if block["type"] != "image" {
					continue
				}
				imageCount++
				source := block["source"].(map[string]any)
				require.Equal(t, "base64", source["type"])
				require.Equal(t, "image/png", source["media_type"])
				decoded, err := base64.StdEncoding.DecodeString(source["data"].(string))
				require.NoError(t, err)
				require.Equal(t, original, decoded)
			}
			require.Equal(t, 1, imageCount)
			preflight := readLog("claude-preflight.jsonl")
			require.Len(t, preflight, 1)
			require.Equal(t, true, preflight[0]["skipBehaviors"])
			require.Equal(t, log[0]["pid"], preflight[0]["pid"], "subscription admission and prompt share the exact initialized CLI")
			manifest := imageinput.Describe([]imageinput.Image{{MIMEType: "image/png", Data: original}})
			require.Equal(t, manifest, c.sessionRecord("execute", p.PacketID).Configuration.InputImages)
			if scenario != "execute" {
				if scenario == "recover-changed-source" {
					writeFile(t, filepath.Join(fx.root, "references", "layout.png"), "new bytes must not become a replacement image prompt")
				}
				c = restartSessionGuardContext(t, fx, c)
				out, err = c.executeStep(c.Packets[p.PacketID])
				if scenario == "recover-original" {
					require.NoError(t, err)
					require.True(t, out.Success, "%+v", out)
					require.Equal(t, "consumed", c.sessionRecord("execute", p.PacketID).Last.Status)
				} else {
					require.Error(t, err)
					require.False(t, out.Success)
				}
				require.Len(t, readLog("claude-fixture.jsonl"), 1, "public recovery must not send another native user message")
				require.Len(t, readLog("claude-preflight.jsonl"), 1, "read-only recovery must not launch another initialized CLI")
			}
			record := c.sessionRecord("execute", p.PacketID)
			require.True(t, record.Last.Accounted)
			require.Equal(t, int64(22), c.State.Budget.Totals.Tokens, "the original SDK result is accounted exactly once")
			receipt, _, err := state.ReadSessionReceipt(fx.taskDir, "execute/"+p.PacketID, record.Last.ID, record.Last.ReceiptDigest)
			require.NoError(t, err)
			var result drivers.Result
			require.NoError(t, json.Unmarshal(receipt.Result, &result))
			var nativeReceipt struct {
				Requested struct {
					InputImages *imageinput.Manifest `json:"inputImages"`
				} `json:"requested"`
			}
			require.NoError(t, json.Unmarshal(result.ClaudeCodeProReceipt, &nativeReceipt))
			require.Equal(t, manifest, nativeReceipt.Requested.InputImages)
			serialized, err := json.Marshal(c.State)
			require.NoError(t, err)
			require.NotContains(t, string(serialized), base64.StdEncoding.EncodeToString(original))
		})
	}
}
