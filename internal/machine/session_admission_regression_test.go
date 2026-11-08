package machine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/stretchr/testify/require"
)

// This fake CLI makes the dispatcher boundary observable without a live model:
// any actual launch creates a marker outside the guarded task workspace.
func admissionKimiCommand(t *testing.T) string {
	t.Helper()
	command := filepath.Join(t.TempDir(), "kimi")
	require.NoError(t, os.WriteFile(command, []byte(`#!/bin/sh
printf 'called\n' >> "$0.called"
printf '%s\n' '{"role":"assistant","content":"completed"}'
printf '%s\n' '{"role":"meta","type":"session.resume_hint","session_id":"admission-session"}'
`), 0o700))
	t.Setenv(drivers.KimiCommandEnv, command)
	return command
}

func TestSessionHarnessAdmissionCanBeRepairedAfterRestart(t *testing.T) {
	for _, mode := range []string{"fresh", "scoped"} {
		t.Run(mode, func(t *testing.T) {
			fx, c := sessionFixture(t)
			c.Config.Sessions.Mode = mode
			c.Config.Permissions.Mode = "role-based"
			a := sessionArgs(c)
			a.Binding = config.LoopBinding{Harness: "kimi", Model: "kimi-code/k3", Channel: "kimi-subscription"}
			a.Model = a.Binding.Model
			c.Config.Loops["review"] = a.Binding
			require.NoError(t, c.Config.Validate(), "the supported config admits this role/harness combination")
			a.PermissionMode = c.Config.PermissionMode("review", "kimi")
			require.Equal(t, "read-only", a.PermissionMode)
			command := admissionKimiCommand(t)
			call := func(a drivers.CallArgs) drivers.Result { return drivers.InvokeHarness(a, "") }

			result, err := c.invokeScoped("review", a.PacketID, a, call)
			require.NoError(t, err, "a proven local rejection must not become an unknown-effects reconciliation block")
			require.NotNil(t, result.ErrorMessage)
			require.Contains(t, *result.ErrorMessage, "cannot enforce read-only")
			require.NoFileExists(t, command+".called", "the rejected request must not launch the CLI")
			require.NotNil(t, result.Session)
			require.Equal(t, "not-dispatched", result.Session.Dispatch)
			require.Equal(t, "failed", result.Session.Outcome)
			require.True(t, result.Session.CleanupConfirmed)
			require.Equal(t, "complete", result.Session.UsageStatus)
			require.Equal(t, &drivers.Usage{}, result.Usage)
			require.NotNil(t, result.CostUSD)
			require.Zero(t, *result.CostUSD)
			require.Equal(t, "received", c.sessionRecord("review", a.PacketID).Last.Status)
			applySessionResult(t, c, "review", a.PacketID, result)
			require.Zero(t, c.State.Budget.Totals.Tokens)
			require.Zero(t, c.State.Budget.Totals.CostUSD)

			// A new process may fix the configuration and retry normally. The
			// failed admission is already consumed and carries no backend effects.
			restarted := unitTestContext(t, fx)
			enableSessionPolicy(restarted)
			restarted.Config.Sessions.Mode = mode
			restarted.Config.Permissions.Mode = "full-access"
			restarted.Config.Loops["review"] = a.Binding
			a.PermissionMode = restarted.Config.PermissionMode("review", "kimi")
			result, err = restarted.invokeScoped("review", a.PacketID, a, call)
			require.NoError(t, err)
			require.True(t, result.Settled)
			require.Nil(t, result.ErrorMessage)
			require.FileExists(t, command+".called")
			require.Equal(t, "dispatched", result.Session.Dispatch)
			require.Equal(t, "completed", result.Session.Outcome)
			applySessionResult(t, restarted, "review", a.PacketID, result)
			record := loadFixtureState(t, fx).Sessions.Records["review/"+a.PacketID]
			require.Equal(t, "consumed", record.Last.Status)
			require.True(t, record.Last.Accounted)
		})
	}
}

func TestSessionHarnessAdmissionKeepsConfirmedBackendCheckpoint(t *testing.T) {
	_, c := sessionFixture(t)
	a := sessionArgs(c)
	a.Binding = config.LoopBinding{Harness: "kimi", Model: "kimi-code/k3", Channel: "kimi-subscription"}
	a.Model = a.Binding.Model
	command := admissionKimiCommand(t)
	call := func(a drivers.CallArgs) drivers.Result { return drivers.InvokeHarness(a, "") }
	result, err := c.invokeScoped("execute", a.PacketID, a, call)
	require.NoError(t, err)
	require.True(t, result.Settled)
	applySessionResult(t, c, "execute", a.PacketID, result)
	before := *c.sessionRecord("execute", a.PacketID)
	require.Equal(t, "kimi-print/v1", before.Protocol)
	require.Equal(t, "admission-session", before.ThreadID)
	require.True(t, before.ResumeCandidate)

	// Exercise a dispatcher refusal after the engine selects a valid resume
	// checkpoint. Even a narrowed local permission policy must not destroy the
	// backend identity of history that was never dispatched to this time.
	result, err = c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
		require.Equal(t, before.ThreadID, a.SessionID)
		a.PermissionMode = "read-only"
		return call(a)
	})
	require.NoError(t, err)
	require.Equal(t, "not-dispatched", result.Session.Dispatch)
	applySessionResult(t, c, "execute", a.PacketID, result)
	after := c.sessionRecord("execute", a.PacketID)
	require.Equal(t, before.ThreadID, after.ThreadID)
	require.Equal(t, before.Protocol, after.Protocol)
	require.Equal(t, before.Generation, after.Generation)
	require.True(t, after.ResumeCandidate)

	result, err = c.invokeScoped("execute", a.PacketID, a, func(a drivers.CallArgs) drivers.Result {
		require.Equal(t, before.ThreadID, a.SessionID)
		return call(a)
	})
	require.NoError(t, err)
	require.True(t, result.Settled)
	require.True(t, result.Session.ResumeAccepted)
	applySessionResult(t, c, "execute", a.PacketID, result)
	launches, err := os.ReadFile(command + ".called")
	require.NoError(t, err)
	require.Equal(t, "called\ncalled\n", string(launches), "only the two accepted turns may launch")
}
