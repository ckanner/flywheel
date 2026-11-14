package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func handoffFixture(t *testing.T) (fixture, *Context) {
	fx, c := sessionFixture(t)
	c.Config.Routes["default"] = config.RouteBinding{Harness: "dsh", Model: "test-model", Channel: "test"}
	c.Config.Channels["test"] = config.Channel{}
	c.Config.Drivers["review"] = "mock"
	a := sessionArgs(c)
	a.Binding.Provider = "test"
	result, err := c.invokeScoped("execute", "M01-T01", a, func(a drivers.CallArgs) drivers.Result {
		return sessionResult(a, drivers.Result{Harness: "dsh", Model: "test-model", Settled: true, Text: "original", Usage: &drivers.Usage{Input: 1}})
	})
	require.NoError(t, err)
	applySessionResult(t, c, "execute", "M01-T01", result)
	old := consoleInvoke
	consoleInvoke = func(a drivers.CallArgs, _ string) drivers.Result {
		writeFile(t, filepath.Join(a.Cwd, "build", "M01-T01.txt"), "manual repair")
		return sessionResult(a, drivers.Result{Harness: "dsh", Model: "test-model", Settled: true, ExitCode: new(int), Text: "human turn reply", Usage: &drivers.Usage{Input: 2}})
	}
	t.Cleanup(func() { consoleInvoke = old })
	return fx, c
}
func sessionRequest(action string) SessionCommand {
	r := SessionCommand{Action: action, Key: "execute/M01-T01", Revision: 1}
	if action == "claim" {
		r.Reason = "repair manually"
	}
	if action == "send" {
		r.Prompt = "check the implementation"
	}
	return r
}

func TestManagedSessionClaimPromptReturnAndExactHistory(t *testing.T) {
	fx, c := handoffFixture(t)
	history, err := ReadSessionHistory(fx.taskDir, "execute/M01-T01")
	require.NoError(t, err)
	require.Equal(t, "original", history.Text)
	original := c.State.Sessions.Records["execute/M01-T01"].ThreadID
	_, err = c.runSessionCommand(sessionRequest("claim"))
	require.NoError(t, err)
	require.NotNil(t, c.State.Handoff)
	_, err = c.run()
	require.ErrorContains(t, err, "human session control")
	_, err = c.runSingle("execute", "M01-T01")
	require.ErrorContains(t, err, "human session control")
	_, err = c.runSessionCommand(sessionRequest("send"))
	require.NoError(t, err)
	r := c.State.Sessions.Records["execute/M01-T01"]
	require.Equal(t, original, r.ThreadID)
	require.Equal(t, "consumed", r.Last.Status)
	require.Equal(t, int64(3), c.State.Budget.Totals.Tokens)
	history, err = ReadSessionHistory(fx.taskDir, "execute/M01-T01")
	require.NoError(t, err)
	require.Equal(t, "human turn reply", history.Text)
	// The handoff never writes a Review pass. The normal independent driver owns it.
	require.Nil(t, c.State.Packets["M01-T01"].Evidence.Review)
	c.Drivers = happyDrivers().set()
	_, err = c.runSessionCommand(sessionRequest("return"))
	require.NoError(t, err)
	require.Nil(t, c.State.Handoff)
	require.Equal(t, "paused", c.State.Status)
	require.NotNil(t, c.State.Packets["M01-T01"].Evidence.Review)
	require.True(t, c.doneReportValid(c.Packets["M01-T01"]))
	writeFile(t, filepath.Join(c.Cwd, "build", "M01-T01.txt"), "changed again")
	require.False(t, c.doneReportValid(c.Packets["M01-T01"]))
}

func TestManagedSessionRejectsChangedBindingsUnknownCallsAndQualityRebuild(t *testing.T) {
	for _, scenario := range []string{"missing", "unclaimed", "claimed", "pending", "binding", "quality", "budget", "unknown", "role", "candidate", "save", "return-failure", "review-failure", "review-error", "failed-call", "changed-after-claim", "missing-packet"} {
		t.Run(scenario, func(t *testing.T) {
			_, c := handoffFixture(t)
			request := sessionRequest("claim")
			switch scenario {
			case "missing":
				request.Key = "missing"
			case "unclaimed":
				request = sessionRequest("send")
			case "claimed":
				require.NoError(t, func() error { _, err := c.runSessionCommand(request); return err }())
			case "pending":
				c.State.Sessions.Records[request.Key].Last.Status = "blocked"
			case "binding":
				binding := c.Config.Routes["default"]
				binding.Model = "different"
				c.Config.Routes["default"] = binding
			case "role":
				r := c.State.Sessions.Records[request.Key]
				r.Scope = "M-01"
			case "candidate":
				r := c.State.Sessions.Records[request.Key]
				r.Cwd = filepath.Join(t.TempDir(), "absent")
			case "save":
				c.revision--
			default:
				_, err := c.runSessionCommand(request)
				require.NoError(t, err)
				request = sessionRequest("send")
				switch scenario {
				case "quality":
					c.Config.Sessions.Quality.MaxConfirmedTurns = 1
				case "budget":
					limit := int64(1)
					c.Config.Budget.Enforce = "hard"
					c.Config.Budget.DailyTokenLimit = &limit
					c.State.Budget.Enforce = "hard"
					c.State.Budget.DailyTokenLimit = &limit
				case "unknown":
					consoleInvoke = func(a drivers.CallArgs, _ string) drivers.Result {
						r := sessionResult(a, drivers.Result{})
						r.Session.CleanupConfirmed = false
						return r
					}
				case "return-failure":
					request = sessionRequest("return")
					c.Packets["M01-T01"].Acceptance.L1[0].Cmd = "exit 1"
				case "review-failure", "review-error":
					_, err := c.runSessionCommand(sessionRequest("send"))
					require.NoError(t, err)
					request = sessionRequest("return")
					d := happyDrivers()
					d.reviewFn = func(drivers.CallArgs) drivers.Result { return drivers.Result{ErrorCode: errs.INVALID_REQUEST} }
					c.Drivers = d.set()
					if scenario == "review-error" {
						delete(c.Config.Loops, "review")
					}
				case "changed-after-claim":
					c.Config.Sessions.Mode = "fresh"
				case "failed-call":
					consoleInvoke = func(a drivers.CallArgs, _ string) drivers.Result {
						return sessionResult(a, drivers.Result{ErrorCode: errs.SERVER, Settled: true, ExitCode: new(int)})
					}
				case "missing-packet":
					request = sessionRequest("return")
					delete(c.Packets, "M01-T01")
				}
			}
			_, err := c.runSessionCommand(request)
			require.Error(t, err)
		})
	}
	_, c := handoffFixture(t)
	delete(c.Config.Routes, "default")
	_, err := c.handoffArgs(c.State.Sessions.Records["execute/M01-T01"])
	require.Error(t, err)
	_, err = c.runSingle("session", "")
	require.Error(t, err)
}

func TestManagedSessionValidationAndHistoryFailures(t *testing.T) {
	for _, r := range []SessionCommand{{}, {Action: "bad", Key: "x", Revision: 1}, {Action: "claim", Key: "x", Revision: 1}, {Action: "send", Key: "x", Revision: 1}, {Action: "return", Key: "x", Revision: 1, Prompt: "ignored"}} {
		require.Error(t, r.Validate())
	}
	_, err := ReadSessionHistory(t.TempDir(), "x")
	require.Error(t, err)
	fx, c := handoffFixture(t)
	_, err = c.runSessionCommand(SessionCommand{})
	require.Error(t, err)
	_, err = ReadSessionHistory(fx.taskDir, "missing")
	require.Error(t, err)
	r := c.State.Sessions.Records["execute/M01-T01"]
	receipt, _, err := state.ReadSessionReceipt(fx.taskDir, "execute/M01-T01", r.Last.ID, r.Last.ReceiptDigest)
	require.NoError(t, err)
	receipt.Result = json.RawMessage(`{"text":` + strconvQuote(strings.Repeat("中", 30000)) + `}`)
	digest, err := state.WriteSessionReceipt(fx.taskDir, "execute/M01-T01", *receipt)
	require.NoError(t, err)
	r.Last.ReceiptDigest = digest
	c.save()
	history, err := ReadSessionHistory(fx.taskDir, "execute/M01-T01")
	require.NoError(t, err)
	require.True(t, history.Truncated)
	receipt.Result = json.RawMessage(`{"text":false}`)
	digest, err = state.WriteSessionReceipt(fx.taskDir, "execute/M01-T01", *receipt)
	require.NoError(t, err)
	c.State.Sessions.Records["execute/M01-T01"].Last.ReceiptDigest = digest
	c.save()
	_, err = ReadSessionHistory(fx.taskDir, "execute/M01-T01")
	require.Error(t, err)
	require.NoError(t, os.RemoveAll(filepath.Join(fx.taskDir, ".sessions")))
	_, err = ReadSessionHistory(fx.taskDir, "execute/M01-T01")
	require.Error(t, err)
	c.State.Sessions = nil
	c.save()
	_, err = ReadSessionHistory(fx.taskDir, "execute/M01-T01")
	require.Error(t, err)
}
func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestManualReceiptRecoveryConsumesOnceBeforeAnotherPrompt(t *testing.T) {
	fx, c := handoffFixture(t)
	_, err := c.runSessionCommand(sessionRequest("claim"))
	require.NoError(t, err)
	original := c.State.Sessions.Records["execute/M01-T01"]
	c.State.Handoff.Pending = true
	c.State.Handoff.PreviousInvocation = original.Last.ID
	c.save()
	_, err = c.runSessionCommand(sessionRequest("send"))
	require.ErrorContains(t, err, "preceding manual")
	a, err := c.handoffArgs(original)
	require.NoError(t, err)
	a.Prompt = "repair"
	result, err := c.invokeScoped(original.Role, original.Scope, a, func(a drivers.CallArgs) drivers.Result { return consoleInvoke(a, a.Model) })
	require.NoError(t, err)
	require.NotEmpty(t, result.Session.RequestID)
	// Crash before accounting/consumption: the new process recovers the exact
	// receipt. It must not send a second prompt or charge the first turn twice.
	oldInvoke := consoleInvoke
	consoleInvoke = func(drivers.CallArgs, string) drivers.Result {
		t.Fatal("reconcile dispatched a model")
		return drivers.Result{}
	}
	defer func() { consoleInvoke = oldInvoke }()
	st, err := state.Load(fx.taskDir)
	require.NoError(t, err)
	request := sessionRequest("reconcile")
	request.Revision = st.Revision
	opts := RunSingleStepOpts{RunOpts: RunOpts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config, Drivers: happyDrivers().set()}, Only: "session", Session: &request, ExpectedRevision: &request.Revision}
	summary, err := RunSingleStep(opts)
	require.NoError(t, err)
	require.Equal(t, "session-reconcile", summary.Step.Name)
	st, err = state.Load(fx.taskDir)
	require.NoError(t, err)
	require.False(t, st.Handoff.Pending)
	require.Equal(t, "consumed", st.Sessions.Records[request.Key].Last.Status)
	require.Equal(t, int64(3), st.Budget.Totals.Tokens)
	request.Revision = st.Revision
	_, err = RunSingleStep(opts)
	require.NoError(t, err)
	st, err = state.Load(fx.taskDir)
	require.NoError(t, err)
	require.Equal(t, int64(3), st.Budget.Totals.Tokens)
	c.State.Handoff = nil
	_, err = c.runSessionCommand(sessionRequest("reconcile"))
	require.Error(t, err)
	c.State.Handoff = &state.Handoff{Key: request.Key}
	c.State.Sessions.Records[request.Key].Last.Status = "blocked"
	_, err = c.runSessionCommand(sessionRequest("reconcile"))
	require.Error(t, err)
}

func TestMilestoneHistoryDoesNotOfferInteractivePacketTakeover(t *testing.T) {
	_, c := handoffFixture(t)
	c.Config.Loops["plan"] = config.LoopBinding{Harness: "dsh", Provider: "test", Model: "test-model", Channel: "test"}
	a := sessionArgs(c)
	a.Binding = c.Config.Loops["plan"]
	result, err := c.invokeScoped("plan", "M-01", a, func(a drivers.CallArgs) drivers.Result { return sessionResult(a, drivers.Result{Settled: true}) })
	require.NoError(t, err)
	applySessionResult(t, c, "plan", "M-01", result)
	request := sessionRequest("claim")
	request.Key = "plan/M-01"
	_, err = c.runSessionCommand(request)
	require.ErrorContains(t, err, "packet-scoped")
}

func TestSessionPublicEntryCannotBypassRevisionOrStage(t *testing.T) {
	revision := int64(1)
	for _, opts := range []RunSingleStepOpts{{Only: "session"}, {Only: "execute", Session: &SessionCommand{}}, {Only: "session", ExpectedRevision: &revision, Session: &SessionCommand{Revision: 1}}} {
		_, err := RunSingleStep(opts)
		require.Error(t, err)
	}
}

func TestHumanControlCanBeReleasedWithoutClaimingVerification(t *testing.T) {
	_, c := handoffFixture(t)
	c.Config.Sessions.Quality.MaxConfirmedTurns = 1
	_, err := c.runSessionCommand(sessionRequest("claim"))
	require.ErrorContains(t, err, "rebuild")
	c.Config.Sessions.Quality.MaxConfirmedTurns = 0
	_, err = c.runSessionCommand(sessionRequest("claim"))
	require.NoError(t, err)
	_, err = c.runSessionCommand(sessionRequest("release"))
	require.NoError(t, err)
	require.Nil(t, c.State.Handoff)
	require.Equal(t, "paused", c.State.Status)
	require.Nil(t, c.State.Packets["M01-T01"].Evidence.Review)
}

func nativeKimiHandoffFixture(t *testing.T, mode string) (fixture, *Context, drivers.CallArgs) {
	t.Helper()
	fx, c := sessionFixture(t)
	t.Setenv(drivers.KimiNativeWorkerEnv, "/owned/kimi-native-worker")
	c.Config.Permissions.Mode = "full-access"
	c.Config.Channels["native-account"] = config.Channel{}
	c.Config.NativeProfiles = map[string]config.NativeProfile{"native": {
		Harness: "kimi", Options: json.RawMessage(`{"version":1,"mode":"` + mode + `","agents":{"enabled":true}}`),
	}}
	c.Config.Routes["default"] = config.RouteBinding{Harness: "kimi", Model: "test-model", Channel: "native-account", NativeProfile: "native"}
	a := sessionArgs(c)
	a.PermissionMode = "danger-full-access"
	a.Binding = config.LoopBinding{Harness: "kimi", Model: "test-model", Channel: "native-account", NativeProfile: "native"}
	result, err := c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
		require.JSONEq(t, string(c.Config.NativeProfiles["native"].Options), string(a.Native))
		require.True(t, a.PersistentSession)
		require.Empty(t, a.SessionID)
		result := sessionResult(a, drivers.MockResult(nil))
		result.Harness, result.Session.Protocol = "kimi", drivers.KimiNativeProtocol
		return result
	})
	require.NoError(t, err)
	applySessionResult(t, c, "execute", a.PacketID, result)
	return fx, c, a
}

func TestNativeKimiScopedSessionCanBeClaimedContinuedAndReturnedToEngine(t *testing.T) {
	for _, mode := range []string{"agent", "swarm"} {
		t.Run(mode, func(t *testing.T) {
			fx, c, args := nativeKimiHandoffFixture(t, mode)
			original := c.sessionRecord("execute", args.PacketID)
			thread, binding, invocation := original.ThreadID, original.Binding, original.Last.ID
			// Reconstruct the record from the real schema-validated state. A
			// matching string in an in-memory fake is not enough for takeover.
			stored := loadFixtureState(t, fx)
			c.State.Sessions = stored.Sessions
			require.JSONEq(t, string(c.Config.NativeProfiles["native"].Options), string(stored.Sessions.Records["execute/M01-T01"].Configuration.NativeOptions))
			_, err := c.runSessionCommand(sessionRequest("claim"))
			require.NoError(t, err)
			oldInvoke := consoleInvoke
			t.Cleanup(func() { consoleInvoke = oldInvoke })
			manualCalls := 0
			consoleInvoke = func(a drivers.CallArgs, model string) drivers.Result {
				manualCalls++
				require.Equal(t, thread, a.SessionID)
				require.Equal(t, thread, a.ExpectedSessionID)
				require.True(t, a.PersistentSession)
				require.Equal(t, args.PermissionMode, a.PermissionMode)
				require.Equal(t, args.Binding, a.Binding)
				require.Equal(t, args.Model, model)
				require.JSONEq(t, string(c.Config.NativeProfiles["native"].Options), string(a.Native))
				require.Equal(t, a.RequestID, a.Managed.InvocationID)
				require.Equal(t, "execute", a.Managed.Role)
				result := sessionResult(a, drivers.MockResult(nil))
				result.Harness, result.Session.Protocol = "kimi", drivers.KimiNativeProtocol
				return result
			}
			_, err = c.runSessionCommand(sessionRequest("send"))
			require.NoError(t, err)
			require.Equal(t, 1, manualCalls)
			continued := c.sessionRecord("execute", args.PacketID)
			require.Equal(t, thread, continued.ThreadID)
			require.Equal(t, binding, continued.Binding)
			require.Equal(t, "resume", continued.Action)
			require.Equal(t, 1, continued.Generation)
			require.NotEqual(t, invocation, continued.Last.ID)
			require.Equal(t, "consumed", continued.Last.Status)
			require.Nil(t, c.State.Packets[args.PacketID].Evidence.Review)
			_, err = c.runSessionCommand(sessionRequest("release"))
			require.NoError(t, err)
			result, err := c.invokeScoped("execute", args.PacketID, args, func(a drivers.CallArgs) drivers.Result {
				require.Equal(t, thread, a.SessionID, "ordinary scoped execution resumes the same admitted native owner")
				require.JSONEq(t, string(c.Config.NativeProfiles["native"].Options), string(a.Native))
				return sessionResult(a, drivers.MockResult(nil))
			})
			require.NoError(t, err)
			applySessionResult(t, c, "execute", args.PacketID, result)
		})
	}
}

func TestNativeKimiHandoffRejectsMissingChangedOrForeignProfilesBeforeDispatch(t *testing.T) {
	for _, action := range []string{"claim", "send"} {
		for _, change := range []string{"missing profile", "foreign harness", "changed options", "changed worker"} {
			t.Run(action+"/"+change, func(t *testing.T) {
				_, c, args := nativeKimiHandoffFixture(t, "agent")
				if action == "send" {
					_, err := c.runSessionCommand(sessionRequest("claim"))
					require.NoError(t, err)
				}
				before := c.sessionRecord("execute", args.PacketID).Last.ID
				switch change {
				case "missing profile":
					delete(c.Config.NativeProfiles, "native")
				case "foreign harness":
					c.Config.NativeProfiles["native"] = config.NativeProfile{Harness: "codex-pro", Options: json.RawMessage(`{"version":1}`)}
				case "changed options":
					c.Config.NativeProfiles["native"] = config.NativeProfile{Harness: "kimi", Options: json.RawMessage(`{"version":1,"mode":"swarm"}`)}
				case "changed worker":
					t.Setenv(drivers.KimiNativeWorkerEnv, "/different/kimi-native-worker")
				}
				oldInvoke := consoleInvoke
				t.Cleanup(func() { consoleInvoke = oldInvoke })
				consoleInvoke = func(drivers.CallArgs, string) drivers.Result {
					t.Fatal("changed native ownership dispatched a manual model call")
					return drivers.Result{}
				}
				_, err := c.runSessionCommand(sessionRequest(action))
				if change == "missing profile" || change == "foreign harness" {
					require.ErrorContains(t, err, "native profile")
				} else {
					require.ErrorContains(t, err, "original session binding")
				}
				require.Equal(t, before, c.sessionRecord("execute", args.PacketID).Last.ID)
				if action == "claim" {
					require.Nil(t, c.State.Handoff)
				} else {
					require.False(t, c.State.Handoff.Pending)
				}
			})
		}
	}
}
