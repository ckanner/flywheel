package machine

import (
	"encoding/json"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/stretchr/testify/require"
)

func TestNativeKimiPlanHistoryUsesOnlyScopedReceiptsAcrossTwoRealPlanSteps(t *testing.T) {
	for _, tc := range []struct {
		name, mode, configured          string
		resident, roleFresh, persistent bool
	}{
		{"agent scoped", "agent", "scoped", true, false, true},
		{"swarm scoped", "swarm", "scoped", true, false, true},
		{"agent resident intent", "agent", "fresh", true, false, true},
		{"swarm resident intent", "swarm", "fresh", true, false, true},
		{"global fresh without resident", "agent", "fresh", false, false, false},
		{"explicit resident disabled", "agent", "scoped", false, false, false},
		{"explicit role fresh", "agent", "scoped", true, true, false},
		{"explicit role fresh over resident", "swarm", "fresh", true, true, false},
		{"tower scoped selects fresh", "tower", "scoped", true, false, false},
		{"tower resident remains fresh", "tower", "fresh", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx, c := sessionFixture(t)
			c.Config.Permissions.Mode = "full-access"
			c.Config.Sessions.Mode = tc.configured
			if tc.roleFresh {
				c.Config.Sessions.Roles = map[string]string{"plan": "fresh"}
			}
			binding := c.Config.Loops["plan"]
			binding.Harness, binding.NativeProfile, binding.ResidentSession, binding.ResidentSessionSet = "kimi", "native", tc.resident, true
			c.Config.Loops["plan"] = binding
			native := json.RawMessage(`{"version":1,"mode":"` + tc.mode + `"}`)
			c.Config.NativeProfiles = map[string]config.NativeProfile{"native": {Harness: "kimi", Options: native}}
			c.State.PlanSession = map[string]any{"session-id": "old-legacy-cli-id", "harness": "kimi", "model": binding.Model,
				"channel": binding.Channel, "cwd": c.Cwd, "thinking": binding.Thinking, "permission-mode": "danger-full-access"}
			c.save()
			require.NoError(t, c.err())
			sd := happyDrivers()
			calls, firstThread := 0, ""
			sd.planFn = func(a drivers.CallArgs) drivers.Result {
				calls++
				require.JSONEq(t, string(native), string(a.Native))
				require.Equal(t, tc.persistent, a.PersistentSession)
				require.Equal(t, "danger-full-access", a.PermissionMode)
				require.NotEqual(t, "old-legacy-cli-id", a.SessionID)
				require.Equal(t, a.RequestID, a.Managed.InvocationID)
				require.Equal(t, "plan", a.Managed.Role)
				if calls == 2 && tc.persistent {
					require.Equal(t, firstThread, a.SessionID)
				} else {
					require.Empty(t, a.SessionID)
				}
				result := sessionResult(a, drivers.MockResult(nil))
				result.Harness, result.Session.Protocol = "kimi", drivers.KimiNativeProtocol
				if tc.mode == "tower" {
					result.Session.ResumeCandidate = false
				}
				if calls == 1 {
					firstThread = result.SessionID
				}
				return result
			}
			c.Drivers = sd.set()
			out, err := c.planStep(c.Spec.Milestones[0], "first native Plan")
			require.NoError(t, err)
			require.True(t, out.OK)
			require.Nil(t, c.State.PlanSession, "native IDs are never written to the legacy CLI cache")
			stored := loadFixtureState(t, fx)
			original := stored.Sessions.Records["plan/M-01"]
			require.JSONEq(t, string(native), string(original.Configuration.NativeOptions))
			require.Equal(t, tc.persistent, original.Last.Persistent)
			if tc.persistent {
				a, err := c.handoffArgs(original)
				require.NoError(t, err, "history inspection and takeover use the same effective native identity")
				require.Equal(t, firstThread, a.ExpectedSessionID)
			}
			restarted, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config, Drivers: sd.set()})
			require.NoError(t, err)
			out, err = restarted.planStep(restarted.Spec.Milestones[0], "second native Plan in the same milestone")
			require.NoError(t, err)
			require.True(t, out.OK)
			require.Equal(t, 2, calls, "both Plan steps dispatched once, including after schema-validated reload")
			record := loadFixtureState(t, fx).Sessions.Records["plan/M-01"]
			require.Equal(t, "consumed", record.Last.Status)
			if tc.persistent {
				require.Equal(t, firstThread, record.ThreadID)
				require.Equal(t, "resume", record.Action)
				require.Equal(t, "scoped", record.Configuration.SessionMode)
				require.Equal(t, 1, record.Generation)
			} else {
				require.Empty(t, record.ThreadID)
				require.False(t, record.ResumeCandidate)
				require.Equal(t, "fresh", record.Configuration.SessionMode)
				require.Equal(t, 2, record.Generation)
			}
			require.Nil(t, restarted.State.PlanSession)
		})
	}
}

func TestNativeKimiNeverUsesTheLegacyPlanResumeShortcut(t *testing.T) {
	for _, mode := range []string{"agent", "swarm", "tower"} {
		_, c := sessionFixture(t)
		c.Config.Sessions.Mode = "fresh"
		args := sessionArgs(c)
		args.Binding.Harness, args.SessionID = "kimi", "legacy-plan-thread"
		args.Native = json.RawMessage(`{"version":1,"mode":"` + mode + `"}`)
		if mode == "tower" {
			args.SessionMode = "scoped"
		}
		plan := c.Config.Loops["plan"]
		plan.ResidentSession = false
		c.Config.Loops["plan"] = plan
		_, err := c.invokeScoped("plan", "M-01", args, func(a drivers.CallArgs) drivers.Result {
			require.Empty(t, a.SessionID)
			require.False(t, a.PersistentSession)
			return sessionResult(a, drivers.MockResult(nil))
		})
		require.NoError(t, err)
	}
}

func TestNativeKimiMalformedHistoryPolicyFailsBeforeDispatch(t *testing.T) {
	_, c := sessionFixture(t)
	for _, raw := range []string{"", `{`, `{"version":2,"mode":"agent"}`, `{"version":1,"mode":"other"}`} {
		c.Config.NativeProfiles = map[string]config.NativeProfile{"native": {Harness: "kimi", Options: json.RawMessage(raw)}}
		args := sessionArgs(c)
		args.Binding.Harness, args.Binding.NativeProfile = "kimi", "native"
		_, err := c.invokeScoped("plan", "M-01", args, func(drivers.CallArgs) drivers.Result {
			t.Fatal("invalid native history policy dispatched")
			return drivers.Result{}
		})
		require.ErrorContains(t, err, "native Kimi history")
	}
}
