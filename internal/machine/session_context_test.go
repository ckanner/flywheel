package machine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestVerifiedContextIncrementRequiresReadableUnchangedAuthority(t *testing.T) {
	for _, change := range []string{"none", "contract", "file", "missing", "legacy", "version", "receipt", "effort"} {
		t.Run(change, func(t *testing.T) {
			fx, c := sessionFixture(t)
			c.Config.Sessions.Context = "verified-incremental"
			p := c.Packets["M01-T01"]
			p.ContextFiles, p.CacheScope = []string{"context.txt"}, []string{"context.txt"}
			writeFile(t, filepath.Join(fx.root, "context.txt"), "stable authoritative reference")
			a := sessionArgs(c)
			a.ContextTemplateVersion = executeContextVersion
			a.Prompt = "FULL CURRENT CONTRACT"
			var prompts []string
			call := func(a drivers.CallArgs) drivers.Result {
				prompts = append(prompts, a.Prompt)
				return sessionResult(a, drivers.MockResult(nil))
			}
			first, err := c.invokeScoped("execute", p.PacketID, a, call)
			require.NoError(t, err)
			applySessionResult(t, c, "execute", p.PacketID, first)
			record := c.sessionRecord("execute", p.PacketID)
			require.Equal(t, "full", record.Context.Mode)
			require.NotEmpty(t, record.Context.Identity)
			switch change {
			case "contract":
				p.ContractVersion++
			case "file":
				writeFile(t, filepath.Join(fx.root, "context.txt"), "revised authority")
			case "missing":
				require.NoError(t, os.Remove(filepath.Join(fx.root, "context.txt")))
			case "legacy":
				record.Context = nil
			case "version":
				record.Context.Version = 2
			case "receipt":
				record.Last.ReceiptDigest = strings.Repeat("0", 64)
			case "effort":
				a.Binding.Thinking = "high"
			}
			c.State.Packets[p.PacketID].LastFeedback = map[string]any{"failed-commands": []string{"test current candidate"}, "status": "unknown"}
			second, err := c.invokeScoped("execute", p.PacketID, a, call)
			require.NoError(t, err)
			applySessionResult(t, c, "execute", p.PacketID, second)
			require.Len(t, prompts, 2)
			current := c.sessionRecord("execute", p.PacketID)
			if change == "none" {
				require.Equal(t, "incremental", current.Context.Mode)
				require.Contains(t, prompts[1], "Verified context update")
				require.Contains(t, prompts[1], "test current candidate")
				require.Contains(t, prompts[1], "candidate-before")
				require.Contains(t, prompts[1], "candidate-current")
				require.NotContains(t, prompts[1], "FULL CURRENT CONTRACT")
			} else {
				require.Equal(t, "full", current.Context.Mode)
				require.Contains(t, prompts[1], "FULL CURRENT CONTRACT")
			}
			loaded := loadFixtureState(t, fx)
			require.Equal(t, current.Context, loaded.Sessions.Records["execute/"+p.PacketID].Context)
		})
	}
}

func TestVerifiedContextNeverTrimsIndependentReviewerOrMissingPacket(t *testing.T) {
	_, c := sessionFixture(t)
	c.Config.Sessions.Context = "verified-incremental"
	a := sessionArgs(c)
	a.ContextTemplateVersion = executeContextVersion
	a.Prompt = "COMPLETE CURRENT CANDIDATE AND REQUIREMENTS"
	for _, role := range []string{"review", "judge", "judge-panel", "plan"} {
		checkpoint, prompt := c.sessionContextPrompt(role, "M01-T01", "resume", "candidate", nil, a)
		require.Nil(t, checkpoint)
		require.Equal(t, a.Prompt, prompt)
	}
	checkpoint, prompt := c.sessionContextPrompt("execute", "missing-packet", "resume", "candidate", nil, a)
	require.Equal(t, "full", checkpoint.Mode)
	require.Contains(t, checkpoint.Reason, "unavailable")
	require.Equal(t, a.Prompt, prompt)
	checkpoint, _ = c.sessionContextPrompt("execute", "M01-T01", "create", "candidate", nil, a)
	previous := &state.SessionRecord{Context: checkpoint, Last: &state.SessionInvocation{Status: "received"}}
	current, prompt := c.sessionContextPrompt("execute", "M01-T01", "resume", "candidate", previous, a)
	require.Equal(t, "full", current.Mode, "an unconsumed invocation never establishes reusable input history")
	require.Equal(t, a.Prompt, prompt)

	a.ContextTemplateVersion = 0
	checkpoint, prompt = c.sessionContextPrompt("execute", "M01-T01", "resume", "candidate", previous, a)
	require.Nil(t, checkpoint, "a human takeover prompt must never be replaced with generated feedback")
	require.Equal(t, a.Prompt, prompt)
	a.ContextTemplateVersion = executeContextVersion
	c.Config.Sessions.Context = "full"
	checkpoint, prompt = c.sessionContextPrompt("execute", "M01-T01", "resume", "candidate", previous, a)
	require.Nil(t, checkpoint)
	require.Equal(t, a.Prompt, prompt)
}

func TestVerifiedContextResendsAuthorityAfterAnUncompletedDelivery(t *testing.T) {
	for _, dispatch := range []string{"not-dispatched", "dispatched"} {
		t.Run(dispatch, func(t *testing.T) {
			fx, c := sessionFixture(t)
			c.Config.Sessions.Context = "verified-incremental"
			packet := c.Packets["M01-T01"]
			packet.ContextFiles, packet.CacheScope = []string{"context.txt"}, []string{"context.txt"}
			writeFile(t, filepath.Join(fx.root, "context.txt"), "ORIGINAL AUTHORITY")
			args := sessionArgs(c)
			args.ContextTemplateVersion = executeContextVersion
			var seen []drivers.CallArgs
			call := func(a drivers.CallArgs) drivers.Result {
				seen = append(seen, a)
				result := sessionResult(a, drivers.MockResult(nil))
				if len(seen) == 2 {
					result.ExitCode, result.ErrorCode = new(1), errs.SERVER
					result.Session.Dispatch, result.Session.Outcome = dispatch, "failed"
					if dispatch == "not-dispatched" {
						// A proven admission rejection preserves the old history but
						// never delivers this attempt's updated full prompt to it.
						result.SessionID, result.Session.SessionID = "", ""
						result.Session.ResumeAccepted, result.Session.ResumeCandidate = false, false
						result.Usage, result.CostUSD = &drivers.Usage{}, new(float64)
					}
				}
				return result
			}
			args.Prompt = prompts.BuildExecutePrompt(packet, c.Spec, args.TaskDir, args.Cwd, 1)
			result, err := c.invokeScoped("execute", packet.PacketID, args, call)
			require.NoError(t, err)
			applySessionResult(t, c, "execute", packet.PacketID, result)
			thread := c.sessionRecord("execute", packet.PacketID).ThreadID
			require.Contains(t, seen[0].Prompt, "ORIGINAL AUTHORITY")
			writeFile(t, filepath.Join(fx.root, "context.txt"), "UPDATED AUTHORITY MUST BE DELIVERED")
			args.Prompt = prompts.BuildExecutePrompt(packet, c.Spec, args.TaskDir, args.Cwd, 2)
			result, err = c.invokeScoped("execute", packet.PacketID, args, call)
			require.NoError(t, err)
			applySessionResult(t, c, "execute", packet.PacketID, result)
			require.Contains(t, seen[1].Prompt, "UPDATED AUTHORITY MUST BE DELIVERED")
			require.NotContains(t, seen[1].Prompt, "Verified context update")
			prior := c.sessionRecord("execute", packet.PacketID)
			require.Equal(t, "consumed", prior.Last.Status)
			require.True(t, prior.ResumeCandidate, "the safe failure may preserve the old backend thread")
			require.Equal(t, "full", prior.Context.Mode)
			require.Equal(t, dispatch, prior.Last.Dispatch)
			priorBudget := c.State.Budget.Totals.Tokens

			// Reload the durable state so recovery cannot accidentally depend on
			// an in-memory distinction between planned and delivered context.
			c.State = loadFixtureState(t, fx)
			args.Prompt = prompts.BuildExecutePrompt(packet, c.Spec, args.TaskDir, args.Cwd, 3)
			result, err = c.invokeScoped("execute", packet.PacketID, args, call)
			require.NoError(t, err)
			require.Equal(t, priorBudget, c.State.Budget.Totals.Tokens)
			applySessionResult(t, c, "execute", packet.PacketID, result)
			require.Len(t, seen, 3)
			require.Equal(t, thread, seen[2].SessionID, "full authority does not require discarding the confirmed history")
			require.Contains(t, seen[2].Prompt, "UPDATED AUTHORITY MUST BE DELIVERED")
			require.Contains(t, seen[2].Prompt, "## Spec (stable contract content)")
			require.NotContains(t, seen[2].Prompt, "Verified context update")
			final := c.sessionRecord("execute", packet.PacketID)
			require.Equal(t, "resume", final.Action)
			require.Equal(t, "full", final.Context.Mode)
			require.Contains(t, final.Context.Reason, "did not confirm completed context delivery")
		})
	}
}

func TestExecutionSessionModeOverrideRemainsPartOfExactBinding(t *testing.T) {
	_, c := sessionFixture(t)
	a := sessionArgs(c)
	a.SessionMode = "fresh"
	first, err := c.invokeScoped("execute", "M01-T01", a, func(a drivers.CallArgs) drivers.Result {
		require.False(t, a.PersistentSession)
		return sessionResult(a, drivers.MockResult(nil))
	})
	require.NoError(t, err)
	applySessionResult(t, c, "execute", "M01-T01", first)
	freshBinding := c.sessionRecord("execute", "M01-T01").Binding
	a.SessionMode = "scoped"
	second, err := c.invokeScoped("execute", "M01-T01", a, func(a drivers.CallArgs) drivers.Result {
		require.True(t, a.PersistentSession)
		require.Empty(t, a.SessionID)
		return sessionResult(a, drivers.MockResult(nil))
	})
	require.NoError(t, err)
	applySessionResult(t, c, "execute", "M01-T01", second)
	require.NotEqual(t, freshBinding, c.sessionRecord("execute", "M01-T01").Binding)
	require.Equal(t, "rebuild", c.sessionRecord("execute", "M01-T01").Action)
}

func TestExecutionPolicyHumanTakeoverRetainsSelectedBindingAndPrompt(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	c.Config.Sessions.Mode = "fresh"
	c.Config.Sessions.Context = "verified-incremental"
	sd := happyDrivers()
	c.Drivers = sessionDriverSet(sd)
	p := c.Packets["M01-T01"]
	for range 2 {
		out, err := c.executeStep(p)
		require.NoError(t, err)
		require.True(t, out.Success)
	}
	before := c.sessionRecord("execute", p.PacketID)
	require.Equal(t, "incremental", before.Context.Mode)
	thread := before.ThreadID
	a, err := c.handoffArgs(before)
	require.NoError(t, err)
	require.Equal(t, "low", a.Binding.Thinking)
	require.Equal(t, "scoped", a.SessionMode, "the explicitly allowed history policy takes precedence over the role default")
	require.Zero(t, a.ContextTemplateVersion)
	old := consoleInvoke
	t.Cleanup(func() { consoleInvoke = old })
	consoleInvoke = func(a drivers.CallArgs, _ string) drivers.Result {
		require.Contains(t, a.Prompt, "Please inspect this distinct human request")
		require.NotContains(t, a.Prompt, "Verified context update")
		require.Equal(t, thread, a.SessionID)
		require.Equal(t, "low", a.Binding.Thinking)
		return sessionResult(a, drivers.MockResult(nil))
	}
	_, err = c.runSessionCommand(sessionRequest("claim"))
	require.NoError(t, err)
	send := sessionRequest("send")
	send.Prompt = "Please inspect this distinct human request"
	_, err = c.runSessionCommand(send)
	require.NoError(t, err)
	require.Nil(t, c.sessionRecord("execute", p.PacketID).Context, "the next automatic call must establish full authority after a human prompt")
	require.False(t, c.State.Handoff.Pending)
	c.Config.ExecutionPolicy = nil
	_, err = c.handoffArgs(c.sessionRecord("execute", p.PacketID))
	require.ErrorContains(t, err, "no longer configured")
}
