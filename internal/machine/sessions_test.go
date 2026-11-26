package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func enableSessionPolicy(c *Context) {
	c.Config.Sessions = config.SessionsConfig{Mode: "scoped", PolicyVersion: 1}
	for name, channel := range c.Config.Channels {
		channel.EnvKey = nil
		c.Config.Channels[name] = channel
	}
	for _, role := range []string{"plan", "execute", "review", "judge"} {
		c.Config.Drivers[role] = "real"
		if role != "execute" {
			binding := c.Config.Loops[role]
			binding.Harness = "dsh"
			c.Config.Loops[role] = binding
		}
	}
	for name, binding := range c.Config.Routes {
		binding.Harness = "dsh"
		c.Config.Routes[name] = binding
	}
}

func sessionFixture(t *testing.T) (fixture, *Context) {
	t.Helper()
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	enableSessionPolicy(c)
	return fx, c
}

func sessionArgs(c *Context) drivers.CallArgs {
	return drivers.CallArgs{Cwd: c.Cwd, TaskDir: c.TaskDir, PacketID: "M01-T01", Model: "test-model",
		PermissionMode: "workspace-write", Binding: config.LoopBinding{Harness: "dsh", Model: "test-model", Channel: "test"}}
}

func sessionResult(a drivers.CallArgs, r drivers.Result) drivers.Result {
	id := a.SessionID
	if id == "" {
		id = "history-" + a.RequestID
	}
	r.SessionID = id
	r.Session = &drivers.SessionOutcome{Protocol: "test-worker/v2", RequestID: a.RequestID, SessionID: id,
		RunID: "run-" + a.RequestID, TurnID: "turn-" + a.RequestID, Dispatch: "dispatched", Outcome: "completed",
		CleanupConfirmed: true, ResumeCandidate: true, ResumeAccepted: a.SessionID != "", UsageStatus: "complete"}
	return r
}

func applySessionResult(t *testing.T, c *Context, role, scope string, r drivers.Result) {
	t.Helper()
	c.recordSpawnMetrics(spawnMetricsArgs{role: role, packetID: scope, model: "test-model", result: &r})
	c.consumeSession(role, scope)
	c.save()
	require.NoError(t, c.err())
}

func sessionDriverSet(sd *scriptedDrivers) *drivers.Drivers {
	plan, execute, review, judge := sd.planFn, sd.execFn, sd.reviewFn, sd.judgeFn
	sd.planFn = func(a drivers.CallArgs) drivers.Result {
		if plan != nil {
			return sessionResult(a, plan(a))
		}
		return sessionResult(a, sd.mock.Plan(a))
	}
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		if execute != nil {
			return sessionResult(a, execute(a))
		}
		return sessionResult(a, sd.mock.Execute(a))
	}
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		if review != nil {
			return sessionResult(a, review(a))
		}
		return sessionResult(a, sd.mock.Review(a))
	}
	sd.judgeFn = func(a drivers.CallArgs) drivers.Result {
		if judge != nil {
			return sessionResult(a, judge(a))
		}
		return sessionResult(a, sd.mock.Judge(a))
	}
	return sd.set()
}

func sessionRunOpts(fx fixture, d *drivers.Drivers) RunTaskOpts {
	opts := runOpts(fx, d)
	enableSessionPolicy(&Context{Config: opts.Config})
	return opts
}

func TestScopedQuickRepairPreservesSeparateRoleHistories(t *testing.T) {
	for _, repair := range []bool{false, true} {
		t.Run(map[bool]string{false: "first-pass", true: "review-return"}[repair], func(t *testing.T) {
			fx := newQuickFixture(t, false)
			sd := quickDrivers()
			reviews := 0
			sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
				reviews++
				require.Contains(t, a.Prompt, "complete current candidate")
				if repair && reviews == 1 {
					return quickVerdict("return", true)
				}
				return quickVerdict("pass", true)
			}
			summary, err := RunTask(sessionRunOpts(fx, sessionDriverSet(sd)))
			require.NoError(t, err)
			require.Equal(t, "completed", summary.Status)
			calls := 1
			if repair {
				calls = 2
			}
			require.Len(t, sd.log.exec, calls)
			require.Len(t, sd.log.review, calls)
			require.Empty(t, sd.log.plan)
			require.Empty(t, sd.log.judge)
			st := loadFixtureState(t, fx)
			require.Equal(t, calls, st.Packets["M01-T01"].Attempts)
			require.Len(t, st.Sessions.Records, 2)
			executor := st.Sessions.Records["execute/M01-T01"]
			reviewer := st.Sessions.Records["review/final:M01-T01"]
			require.NotEqual(t, executor.ThreadID, reviewer.ThreadID)
			ids := map[string]bool{}
			for _, invocation := range append(sd.log.exec, sd.log.review...) {
				require.True(t, invocation.PersistentSession)
				require.NotEmpty(t, invocation.RequestID)
				require.False(t, ids[invocation.RequestID])
				ids[invocation.RequestID] = true
			}
			if repair {
				require.Equal(t, "history-"+sd.log.exec[0].RequestID, sd.log.exec[1].SessionID)
				require.Equal(t, "history-"+sd.log.review[0].RequestID, sd.log.review[1].SessionID)
			}
			for _, r := range st.Sessions.Records {
				require.Equal(t, "consumed", r.Last.Status)
				require.True(t, r.Last.Accounted)
			}
			require.Empty(t, strings.TrimSpace(gitCmd(t, fx.root, "ls-files", "docs/minitask/.sessions")))
			require.Equal(t, int64(calls*2*180), st.Budget.Totals.Tokens)
		})
	}
}

func TestScopedHistoryCompatibilityAndRuntimeBindings(t *testing.T) {
	fx, c := sessionFixture(t)
	a := sessionArgs(c)
	var calls []drivers.CallArgs
	call := func(a drivers.CallArgs) drivers.Result {
		calls = append(calls, a)
		return sessionResult(a, drivers.MockResult(nil))
	}
	run := func(role, scope string) *state.SessionRecord {
		r, err := c.invokeScoped(role, scope, a, call)
		require.NoError(t, err)
		applySessionResult(t, c, role, scope, r)
		return c.sessionRecord(role, scope)
	}
	first := run("execute", "M01-T01").ThreadID
	writeFile(t, filepath.Join(fx.root, "repair.go"), "package repair\n")
	c.Packets["M01-T01"].ContractVersion++
	require.Equal(t, first, run("execute", "M01-T01").ThreadID, "same-goal contract/candidate changes can continue")
	require.NotEqual(t, first, run("execute", "M01-T02").ThreadID)
	require.NotEqual(t, first, run("review", "M01-T01").ThreadID)
	require.NotEqual(t, c.sessionRecord("review", "M01-T01").ThreadID, run("review", "final:M01-T01").ThreadID)
	c.Spec.Goal = "An unrelated goal"
	rebuilt := run("execute", "M01-T01")
	require.Equal(t, "rebuild", rebuilt.Action)
	require.Equal(t, 2, rebuilt.Generation)
	require.NotEqual(t, first, rebuilt.ThreadID)
	a.Binding.Thinking = "high"
	require.Equal(t, 3, run("execute", "M01-T01").Generation)
	link := filepath.Join(t.TempDir(), "workspace")
	require.NoError(t, os.Symlink(fx.root, link))
	a.Cwd = link
	require.Equal(t, "resume", run("execute", "M01-T01").Action)
	require.Equal(t, fx.root, calls[len(calls)-1].Cwd)
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), "another-account"))
	require.Equal(t, "rebuild", run("execute", "M01-T01").Action)
	require.Equal(t, int64(len(calls)*180), c.State.Budget.Totals.Tokens)
}

func TestSessionRuntimeReferencesNeverHashCredentialValues(t *testing.T) {
	_, c := sessionFixture(t)
	for _, harness := range []string{"codex-pro", "dsh", "kimi", "pi"} {
		a := sessionArgs(c)
		a.Binding.Harness = harness
		a.APIKeys = map[string]string{"KEY_A": "secret-one"}
		before := sessionRuntimeIdentity(a)
		a.APIKeys["KEY_A"] = "secret-two"
		require.Equal(t, before, sessionRuntimeIdentity(a))
		a.APIKeys["KEY_B"] = "another-secret"
		require.NotEqual(t, before, sessionRuntimeIdentity(a))
	}
}

func TestSessionConfirmedUnsentResumePreservesPriorCheckpoint(t *testing.T) {
	_, c := sessionFixture(t)
	a := sessionArgs(c)
	result, err := c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result { return sessionResult(a, drivers.MockResult(nil)) })
	require.NoError(t, err)
	applySessionResult(t, c, "execute", a.PacketID, result)
	thread := c.sessionRecord("execute", a.PacketID).ThreadID
	for _, observedID := range []string{"", thread} {
		result, err = c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
			require.Equal(t, thread, a.SessionID)
			return drivers.MockResult(func(r *drivers.Result) {
				r.Session = &drivers.SessionOutcome{Protocol: "test-worker/v2", RequestID: a.RequestID, SessionID: observedID,
					Dispatch: "not-dispatched", Outcome: "failed", CleanupConfirmed: true, UsageStatus: "unknown"}
				r.Usage, r.CostUSD = nil, nil
			})
		})
		require.NoError(t, err)
		applySessionResult(t, c, "execute", a.PacketID, result)
		record := c.sessionRecord("execute", a.PacketID)
		require.True(t, record.ResumeCandidate)
		require.Equal(t, thread, record.ThreadID)
	}
	_, err = c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
		require.Equal(t, thread, a.SessionID)
		return sessionResult(a, drivers.MockResult(nil))
	})
	require.NoError(t, err)
}

func TestSessionConfirmedUnsentResumeRetiresOversizedThreadForRebuild(t *testing.T) {
	_, c := sessionFixture(t)
	a := sessionArgs(c)
	result, err := c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result { return sessionResult(a, drivers.MockResult(nil)) })
	require.NoError(t, err)
	applySessionResult(t, c, "execute", a.PacketID, result)
	thread := c.sessionRecord("execute", a.PacketID).ThreadID

	// Simulate codex-pro thread-resume failing with output-limit (>1 MiB protocol frame).
	result, err = c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
		require.Equal(t, thread, a.SessionID)
		return drivers.MockResult(func(r *drivers.Result) {
			r.Truncated = true
			r.Session = &drivers.SessionOutcome{Protocol: "test-worker/v2", RequestID: a.RequestID, SessionID: "",
				Dispatch: "not-dispatched", Outcome: "failed", CleanupConfirmed: true, UsageStatus: "complete"}
		})
	})
	require.NoError(t, err)
	applySessionResult(t, c, "execute", a.PacketID, result)
	record := c.sessionRecord("execute", a.PacketID)
	require.False(t, record.ResumeCandidate, "truncated thread-resume must retire the oversized thread instead of looping resume")

	// Next invocation automatically rebuilds a fresh session (empty SessionID).
	_, err = c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
		require.Empty(t, a.SessionID)
		return sessionResult(a, drivers.MockResult(nil))
	})
	require.NoError(t, err)
	require.Equal(t, "rebuild", c.sessionRecord("execute", a.PacketID).Action)
}

func TestSessionDurableReceiptReplaysCASWithoutDispatchOrDoubleAccounting(t *testing.T) {
	fx, c := sessionFixture(t)
	a := sessionArgs(c)
	calls := 0
	_, err := c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
		calls++
		onDisk := loadFixtureState(t, fx)
		_, err := state.Save(fx.taskDir, onDisk, nil)
		require.NoError(t, err)
		return sessionResult(a, drivers.MockResult(nil))
	})
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, "intent", loadFixtureState(t, fx).Sessions.Records["execute/M01-T01"].Last.Status)
	c = unitTestContext(t, fx)
	enableSessionPolicy(c)
	r, err := c.invokeScoped("execute", a.PacketID, sessionArgs(c), func(drivers.CallArgs) drivers.Result { t.Fatal("must replay receipt"); return drivers.Result{} })
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "reconcile", c.sessionRecord("execute", a.PacketID).Action)
	c.recordSpawnMetrics(spawnMetricsArgs{role: "execute", packetID: a.PacketID, result: &r})
	// Reload after recovery's accounting checkpoint; a stage caller cannot
	// account the same durable receipt again.
	c = unitTestContext(t, fx)
	enableSessionPolicy(c)
	r, err = c.invokeScoped("execute", a.PacketID, sessionArgs(c), func(drivers.CallArgs) drivers.Result { t.Fatal("must replay receipt"); return drivers.Result{} })
	require.NoError(t, err)
	applySessionResult(t, c, "execute", a.PacketID, r)
	c.recordSpawnMetrics(spawnMetricsArgs{role: "execute", packetID: a.PacketID, result: &r})
	require.Equal(t, int64(180), c.State.Budget.Totals.Tokens)
	raw, err := os.ReadFile(filepath.Join(fx.taskDir, "metrics.jsonl"))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	require.Len(t, lines, 1)
	var metric map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &metric))
	require.NotNil(t, metric["session"])
}

func TestFreshSupportedCallsRetainSafetyLedgerWithoutSharingHistory(t *testing.T) {
	_, c := sessionFixture(t)
	c.Config.Sessions.Mode = "fresh"
	a := sessionArgs(c)
	for i := 0; i < 2; i++ {
		res, err := c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
			require.False(t, a.PersistentSession)
			require.Empty(t, a.SessionID)
			require.NotEmpty(t, a.RequestID)
			return sessionResult(a, drivers.MockResult(nil))
		})
		require.NoError(t, err)
		applySessionResult(t, c, "execute", a.PacketID, res)
		r := c.sessionRecord("execute", a.PacketID)
		require.False(t, r.ResumeCandidate)
		require.Equal(t, res.Session.SessionID, r.ThreadID, "fresh policy retains the observed handle without permitting reuse")
		require.Equal(t, i+1, r.Generation)
	}
	require.Equal(t, int64(360), c.State.Budget.Totals.Tokens)
}
