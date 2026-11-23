package machine

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/imageinput"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/templates"
	"github.com/stretchr/testify/require"
)

func imageExecutionFixture(t *testing.T) (fixture, *Context, []byte) {
	t.Helper()
	fx := newGitFixture(t)
	imageBytes, err := os.ReadFile("../../plugins/dsh/test/fixtures/multimodal-reference.png")
	require.NoError(t, err)
	writeFile(t, filepath.Join(fx.root, "references", "layout.png"), string(imageBytes))
	declaration := fmt.Sprintf("input-images: {version: 1, images: [{path: references/layout.png, sha256: %x, media-type: image/png}]}\n", sha256.Sum256(imageBytes))
	packetText := strings.Replace(fixturePacket("M01-T01", "R-01"), "packet-id:", declaration+"packet-id:", 1)
	packetText = strings.Replace(packetText, "route: default", "route: multimodal", 1)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), packetText)
	template, err := templates.FS.ReadFile("flywheel.config.example.json")
	require.NoError(t, err)
	writeFile(t, filepath.Join(fx.taskDir, "flywheel.config.json"), string(template))
	gitCmd(t, fx.root, "add", "-A")
	gitCmd(t, fx.root, "commit", "-q", "-m", "test: freeze the image packet and subscription configuration")
	cfg := configFor(fx)
	cfg.Worktree.Enabled = new(false)
	cfg.Sessions.Mode = "scoped"
	cfg.Retry.MaxTransportRetries = 0
	cfg.Watchdog.WallTimeoutMs, cfg.Watchdog.IdleTimeoutMs, cfg.Watchdog.KillGraceMs = 15_000, 5_000, 100
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: happyDrivers().set(), Log: func(string) {}})
	require.NoError(t, err)
	return fx, c, imageBytes
}

func imageExecutionArgs(c *Context) drivers.CallArgs {
	binding := c.Config.Routes["multimodal"]
	return drivers.CallArgs{Cwd: c.Cwd, TaskDir: c.TaskDir, PacketID: "M01-T01", Model: binding.Model,
		Binding:        config.LoopBinding{NativeProfile: binding.NativeProfile, Harness: binding.Harness, Model: binding.Model, Channel: binding.Channel, Thinking: binding.Thinking},
		PermissionMode: "danger-full-access", Prompt: "Implement the attached reference."}
}

func TestPacketImagesUseFrozenSourceCheckoutAndExplicitExecuteScope(t *testing.T) {
	fx, c, original := imageExecutionFixture(t)
	a := imageExecutionArgs(c)
	a.Cwd = t.TempDir() // worker checkout has no image at this relative path
	require.NoError(t, c.resolveInvocationImages("execute", a.PacketID, &a))
	require.Equal(t, []imageinput.Image{{MIMEType: "image/png", Data: original}}, a.Images)
	require.ErrorContains(t, c.resolveInvocationImages("execute", a.PacketID, &a), "caller-supplied")
	for _, scope := range [][2]string{{"review", "M01-T01"}, {"execute", "missing"}, {"execute", "M01-T02"}} {
		a = imageExecutionArgs(c)
		require.NoError(t, c.resolveInvocationImages(scope[0], scope[1], &a))
		require.Empty(t, a.Images)
	}
	a = imageExecutionArgs(c)
	a.Binding.Harness = "antigravity"
	a.Binding.Model = "gemini-4-neon-high"
	a.Binding.Channel = "google-subscription"
	a.Model = a.Binding.Model
	require.NoError(t, c.resolveInvocationImages("execute", a.PacketID, &a))
	require.Equal(t, []imageinput.Image{{MIMEType: "image/png", Data: original}}, a.Images)
	a = imageExecutionArgs(c)
	a.Binding.Harness = "codex-pro"
	require.ErrorContains(t, c.resolveInvocationImages("execute", a.PacketID, &a), "cannot accept")
	for _, p := range []*contract.Packet{nil, c.Packets["M01-T02"]} {
		guard, err := c.inputImageGuard(p)
		require.NoError(t, err)
		require.Empty(t, guard)
	}
	guard, err := c.sessionGuard("intake", "M01-T01")
	require.NoError(t, err)
	require.Empty(t, guard, "non-execution intake does not acquire packet image input authority")
	p := c.Packets["M01-T01"]
	guard, err = c.inputImageGuard(p)
	require.NoError(t, err)
	require.Len(t, guard, 64)
	identity, _, err := c.verificationSnapshot(p)
	require.NoError(t, err)
	c.State.Packets[p.PacketID].Evidence.L1 = map[string]any{"pass": true, "identity": identity}
	require.True(t, c.currentL1Evidence(p))
	writeFile(t, filepath.Join(fx.root, "references", "layout.png"), "not the frozen image")
	a = imageExecutionArgs(c)
	require.ErrorContains(t, c.resolveInvocationImages("execute", a.PacketID, &a), "changed")
	_, err = c.inputImageGuard(p)
	require.ErrorContains(t, err, "changed")
	_, _, err = c.verificationSnapshot(p)
	require.ErrorContains(t, err, "changed")
	require.False(t, c.currentL1Evidence(p), "old L1 evidence must not survive a changed external image")
}

func TestImageInputIdentityRetiresHistoryAndNeverPersistsOriginalBytes(t *testing.T) {
	fx, c, original := imageExecutionFixture(t)
	a := imageExecutionArgs(c)
	var calls []drivers.CallArgs
	call := func(args drivers.CallArgs) drivers.Result {
		calls = append(calls, args)
		require.NotEmpty(t, args.Images)
		return sessionResult(args, drivers.MockResult(nil))
	}
	invoke := func() *state.SessionRecord {
		out, err := c.invokeScoped("execute", a.PacketID, a, call)
		require.NoError(t, err)
		applySessionResult(t, c, "execute", a.PacketID, out)
		return c.sessionRecord("execute", a.PacketID)
	}
	first := invoke().ThreadID
	require.Equal(t, "resume", invoke().Action)
	require.Equal(t, first, calls[1].SessionID)
	_, previousMilestoneGoal, previousMilestoneContract := c.sessionIdentities("M-01", a)
	// A valid PNG may have trailing ancillary bytes; the exact frozen bytes,
	// including those bytes, still change identity and require new history.
	changed := append(append([]byte{}, original...), []byte("new frozen content")...)
	writeFile(t, filepath.Join(fx.root, "references", "layout.png"), string(changed))
	c.Packets[a.PacketID].InputImages.Images[0].SHA256 = fmt.Sprintf("%x", sha256.Sum256(changed))
	_, nextGoal, nextContract := c.sessionIdentities("M-01", a)
	require.NotEqual(t, previousMilestoneGoal, nextGoal)
	require.NotEqual(t, previousMilestoneContract, nextContract)
	rebuilt := invoke()
	require.Equal(t, "rebuild", rebuilt.Action)
	require.NotEqual(t, first, rebuilt.ThreadID)
	require.Empty(t, calls[2].SessionID)
	require.Equal(t, changed, calls[2].Images[0].Data)
	require.Equal(t, imageinput.Describe(calls[2].Images), rebuilt.Configuration.InputImages)
	raw, err := os.ReadFile(filepath.Join(fx.taskDir, "state.json"))
	require.NoError(t, err)
	require.NotContains(t, string(raw), base64.StdEncoding.EncodeToString(changed))
	require.NotContains(t, string(raw), string(changed))
	// Changing the model/effort independently cannot reuse an image-bearing
	// native history from the prior binding either.
	a.Binding.Thinking = "low"
	require.Equal(t, "rebuild", invoke().Action)
	require.Empty(t, calls[3].SessionID)
}

func TestTerminalImageResultCannotReplayWithChangedSourceOrContract(t *testing.T) {
	for _, change := range []string{"none", "source", "declaration"} {
		t.Run(change, func(t *testing.T) {
			fx, c, original := imageExecutionFixture(t)
			a := imageExecutionArgs(c)
			out, err := c.invokeScoped("execute", a.PacketID, a, func(args drivers.CallArgs) drivers.Result {
				return sessionResult(args, drivers.MockResult(nil))
			})
			require.NoError(t, err)
			require.Equal(t, "received", c.sessionRecord("execute", a.PacketID).Last.Status)
			if change == "source" {
				writeFile(t, filepath.Join(fx.root, "references", "layout.png"), "changed after receipt")
			} else if change == "declaration" {
				writeFile(t, filepath.Join(fx.root, "references", "other.png"), string(original))
				c.Packets[a.PacketID].InputImages.Images[0].Path = "references/other.png"
			}
			replayed, err := c.invokeScoped("execute", a.PacketID, a, func(drivers.CallArgs) drivers.Result {
				t.Fatal("a terminal receipt must never cause another model request")
				return drivers.Result{}
			})
			if change != "none" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, out.Session.RequestID, replayed.Session.RequestID)
			applySessionResult(t, c, "execute", a.PacketID, replayed)
			require.Equal(t, int64(180), c.State.Budget.Totals.Tokens)
		})
	}
}

func TestImageMutationDuringExecutionBlocksAcceptance(t *testing.T) {
	fx, c, _ := imageExecutionFixture(t)
	a := imageExecutionArgs(c)
	_, err := c.invokeScoped("execute", a.PacketID, a, func(args drivers.CallArgs) drivers.Result {
		writeFile(t, filepath.Join(fx.root, "references", "layout.png"), "mutated during native work")
		return sessionResult(args, drivers.MockResult(nil))
	})
	require.ErrorContains(t, err, "requires reconciliation")
	require.Equal(t, "escalated", c.State.Status)
	require.Empty(t, c.sessionRecord("execute", a.PacketID).Last.GuardAfter)
	require.Nil(t, c.State.Packets[a.PacketID].Evidence.Executor)
}

func TestImageUnknownInvocationRecoversOriginalManifestBeforeReadingNewSource(t *testing.T) {
	fx, c, original := imageExecutionFixture(t)
	a := imageExecutionArgs(c)
	previousPersist, previousRecover := persistSessionReceipt, recoverSessionMachine
	t.Cleanup(func() { persistSessionReceipt, recoverSessionMachine = previousPersist, previousRecover })
	persistSessionReceipt = func(task, key string, receipt state.SessionReceipt) (string, error) {
		if !strings.HasSuffix(key, "/lifecycle") {
			return "", errors.New("simulated Go terminal-receipt interruption")
		}
		return previousPersist(task, key, receipt)
	}
	var completed drivers.Result
	_, err := c.invokeScoped("execute", a.PacketID, a, func(args drivers.CallArgs) drivers.Result {
		completed = sessionResult(args, drivers.MockResult(nil))
		completed.Model = args.Model
		completed.Session.TurnID = ""
		event := drivers.MachineLifecycle{LifecycleVersion: 1, Type: "lifecycle", RequestID: args.RequestID, RunID: completed.Session.RunID,
			ThreadID: &completed.Session.SessionID, Persistent: true, Configuration: json.RawMessage(`{}`), Outcome: "completed", CleanupConfirmed: true, Sequence: 1, Event: "cleanup-confirmed"}
		require.NoError(t, args.OnLifecycle(event))
		return completed
	})
	require.ErrorContains(t, err, "terminal receipt could not be saved")
	persistSessionReceipt = previousPersist
	queries := 0
	recoverSessionMachine = func(query drivers.MachineRecoveryArgs) (drivers.Result, error) {
		queries++
		require.Equal(t, imageinput.Describe([]imageinput.Image{{MIMEType: "image/png", Data: original}}), query.InputImages)
		require.Equal(t, completed.Model, query.Model)
		require.Equal(t, completed.Session.RequestID, query.RequestID)
		return completed, nil
	}
	require.NoError(t, os.Remove(filepath.Join(fx.root, "references", "layout.png")))
	c = restartSessionGuardContext(t, fx, c)
	a.Model, a.Binding.Harness = "new-request-must-not-run", "codex-pro"
	_, err = c.invokeScoped("execute", a.PacketID, a, func(drivers.CallArgs) drivers.Result {
		t.Fatal("an unresolved image call cannot be replaced by new work")
		return drivers.Result{}
	})
	require.Error(t, err)
	require.Equal(t, 1, queries, "read-only reconciliation precedes the new backend and missing source checks")
	require.Equal(t, int64(180), c.State.Budget.Totals.Tokens)
	require.True(t, c.sessionRecord("execute", a.PacketID).Last.Accounted)
	_, err = c.invokeScoped("execute", a.PacketID, a, func(drivers.CallArgs) drivers.Result {
		t.Fatal("repeated reconciliation must not dispatch")
		return drivers.Result{}
	})
	require.Error(t, err)
	require.Equal(t, int64(180), c.State.Budget.Totals.Tokens)
	require.Equal(t, 1, queries)
}
