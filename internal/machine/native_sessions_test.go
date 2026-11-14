package machine

import (
	"encoding/json"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/stretchr/testify/require"
)

func TestNativeBindingParticipatesInHistoryAndDurableInvocation(t *testing.T) {
	_, c := sessionFixture(t)
	c.Config.NativeProfiles = map[string]config.NativeProfile{"heavy": {Harness: "dsh", Options: json.RawMessage(`{"version":1,"preset":"heavy","agents":{"enabled":true}}`)}}
	args := sessionArgs(c)
	args.Binding.NativeProfile = "heavy"
	var previous string
	call := func(a drivers.CallArgs) drivers.Result {
		require.JSONEq(t, string(c.Config.NativeProfiles["heavy"].Options), string(a.Native))
		require.Equal(t, a.RequestID, a.Managed.InvocationID)
		require.Equal(t, "execute", a.Managed.Role)
		require.Equal(t, c.TaskDir, a.Managed.TaskDir)
		require.Empty(t, a.SessionID)
		previous = a.RequestID
		return sessionResult(a, drivers.MockResult(nil))
	}
	r, err := c.invokeScoped("execute", "M01-T01", args, call)
	require.NoError(t, err)
	require.NotEmpty(t, previous)
	applySessionResult(t, c, "execute", "M01-T01", r)
	before, _, _ := c.sessionIdentities("M01-T01", args)
	c.Config.NativeProfiles["heavy"] = config.NativeProfile{Harness: "dsh", Options: json.RawMessage(`{"version":1,"preset":"heavy","agents":{"enabled":false}}`)}
	after, _, _ := c.sessionIdentities("M01-T01", args)
	require.NotEqual(t, before, after)
	_, err = c.invokeScoped("execute", "M01-T01", args, call)
	require.NoError(t, err)
	args.Binding.NativeProfile = "missing"
	_, err = c.invokeScoped("execute", "M01-T01", args, call)
	require.ErrorContains(t, err, "native profile")
	args.Binding.Harness = "kimi"
	args.Native = json.RawMessage(`{"version":1,"mode":"agent"}`)
	t.Setenv(drivers.KimiNativeWorkerEnv, "/worker-one")
	identity := sessionRuntimeIdentity(args)
	t.Setenv(drivers.KimiNativeWorkerEnv, "/worker-two")
	require.NotEqual(t, identity, sessionRuntimeIdentity(args))
}

func TestExecutionPolicyFreezesNativeOptionsAsWellAsProfileName(t *testing.T) {
	_, c := sessionFixture(t)
	enableExecutionPolicy(c)
	c.Config.NativeProfiles = map[string]config.NativeProfile{"heavy": {Harness: "dsh", Options: json.RawMessage(`{"version":1,"preset":"heavy"}`)}}
	b := c.Config.ExecutionPolicy.Bindings["low"]
	b.NativeProfile = "heavy"
	c.Config.ExecutionPolicy.Bindings["low"] = b
	p := c.Packets["M01-T01"]
	_, _, err := c.executionPolicy(p)
	require.NoError(t, err)
	c.Config.NativeProfiles["unrelated"] = config.NativeProfile{Harness: "kimi", Options: json.RawMessage(`{"version":1,"mode":"agent"}`)}
	_, _, err = c.executionPolicy(p)
	require.NoError(t, err)
	c.Config.NativeProfiles["heavy"] = config.NativeProfile{Harness: "dsh", Options: json.RawMessage(`{"version":1,"preset":"daily"}`)}
	_, _, err = c.executionPolicy(p)
	require.ErrorContains(t, err, "identity changed")
}
