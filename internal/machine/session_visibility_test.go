package machine

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

// Exercise the actual Go lifecycle checkpoint and receipt path; only the
// provider invocation is replaced by deterministic, identity-bound evidence.
func visibleSessionResult(t *testing.T, call drivers.CallArgs) drivers.Result {
	t.Helper()
	result := sessionResult(call, drivers.MockResult(nil))
	result.Harness, result.Model = call.Binding.Harness, call.Model
	if call.OnLifecycle == nil {
		return result
	}
	version := drivers.CodexProPackageVersion
	if call.Binding.Harness == "claude-code-pro" {
		version = drivers.ClaudeCodeProPackageVersion
	}
	result.Session.Protocol = call.Binding.Harness + "/" + version + "/worker-v3"
	event := drivers.MachineLifecycle{LifecycleVersion: 1, Type: "lifecycle", RequestID: call.RequestID,
		RunID: result.Session.RunID, ThreadID: &result.Session.SessionID, Persistent: call.PersistentSession,
		Configuration: json.RawMessage(`{}`), Dispatch: "not-dispatched", Outcome: "unknown"}
	for index, stage := range []string{"thread-ready", "dispatch-intent", "dispatch-possible", "turn-accepted", "terminal", "cleanup-confirmed"} {
		event.Sequence, event.Event = index+1, stage
		if stage == "dispatch-possible" {
			event.Dispatch = "possible"
		}
		if stage == "turn-accepted" {
			event.Dispatch, event.TurnID = "accepted", &result.Session.TurnID
		}
		if stage == "terminal" {
			event.Outcome, event.CleanupConfirmed = "completed", true
		}
		require.NoError(t, call.OnLifecycle(event))
	}
	return result
}

func TestManagedHistoryVisibilityIsIndependentOfSessionReuse(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro", "dsh", "kimi"} {
		for _, mode := range []string{"fresh", "scoped"} {
			t.Run(harness+"/"+mode, func(t *testing.T) {
				_, c := sessionFixture(t)
				c.Config.Sessions.Mode = mode
				args := sessionArgs(c)
				args.Binding.Harness, args.Prompt = harness, "current approved task"
				var firstID string
				for index := 0; index < 2; index++ {
					result, err := c.invokeScoped("execute", args.PacketID, args, func(call drivers.CallArgs) drivers.Result {
						persist := mode == "scoped" || harness == "codex-pro" || harness == "claude-code-pro"
						require.Equal(t, persist, call.PersistentSession)
						require.Equal(t, persist, c.sessionRecord("execute", args.PacketID).Last.Persistent)
						require.Contains(t, call.SessionTitle, "Flywheel")
						if index == 0 || mode == "fresh" {
							require.Empty(t, call.SessionID, "fresh calls must not request the previous native history")
						} else {
							require.Equal(t, firstID, call.SessionID)
						}
						return visibleSessionResult(t, call)
					})
					require.NoError(t, err)
					applySessionResult(t, c, "execute", args.PacketID, result)
					record := c.sessionRecord("execute", args.PacketID)
					require.NotEmpty(t, record.ThreadID, "confirmed native history remains inspectable under both policies")
					require.Equal(t, result.Session.SessionID, record.ThreadID)
					require.Equal(t, mode, record.Configuration.SessionMode)
					require.Equal(t, mode == "scoped", record.ResumeCandidate)
					if index == 0 {
						firstID = record.ThreadID
					} else if mode == "fresh" {
						require.NotEqual(t, firstID, record.ThreadID)
						require.Equal(t, 2, record.Generation)
					} else {
						require.Equal(t, firstID, record.ThreadID)
						require.Equal(t, "resume", record.Action)
					}
					if mode == "fresh" {
						_, err := c.handoffArgs(record)
						require.Error(t, err, "native visibility does not grant managed manual continuation")
					}
				}
			})
		}
	}
}

func TestFreshNativeHistoryRecoveryUsesRecordedPersistenceWithoutReplay(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		for _, legacyEphemeral := range []bool{false, true} {
			name := "retained"
			if legacyEphemeral {
				name = "legacy-ephemeral"
			}
			t.Run(harness+"/"+name, func(t *testing.T) {
				fx, c := sessionFixture(t)
				c.Config.Sessions.Mode = "fresh"
				args := sessionArgs(c)
				args.Binding.Harness, args.Prompt = harness, "one native invocation"
				originalPersist := persistSessionReceipt
				t.Cleanup(func() { persistSessionReceipt = originalPersist })
				persistSessionReceipt = func(task, key string, receipt state.SessionReceipt) (string, error) {
					if !strings.HasSuffix(key, "/lifecycle") {
						return "", errors.New("interrupted before Go terminal receipt")
					}
					return originalPersist(task, key, receipt)
				}
				var completed drivers.Result
				calls := 0
				_, err := c.invokeScoped("execute", args.PacketID, args, func(call drivers.CallArgs) drivers.Result {
					calls++
					require.True(t, call.PersistentSession)
					if legacyEphemeral {
						// Reconstruct the exact original invocation contract written by
						// older releases; recovery must not reinterpret it as persistent.
						call.PersistentSession = false
						c.sessionRecord("execute", args.PacketID).Last.Persistent = false
						c.save()
						require.NoError(t, c.err())
					}
					completed = visibleSessionResult(t, call)
					return completed
				})
				require.Error(t, err)
				persistSessionReceipt = originalPersist
				originalRecover := recoverSessionMachine
				t.Cleanup(func() { recoverSessionMachine = originalRecover })
				queries := 0
				recoverSessionMachine = func(recovery drivers.MachineRecoveryArgs) (drivers.Result, error) {
					queries++
					require.Equal(t, !legacyEphemeral, recovery.PersistentSession)
					require.Empty(t, recovery.SessionID)
					require.Equal(t, completed.Session.RequestID, recovery.RequestID)
					require.Equal(t, completed.Session.RunID, recovery.RunID)
					return completed, nil
				}
				c = restartSessionGuardContext(t, fx, c)
				result, err := c.invokeScoped("execute", args.PacketID, args, func(drivers.CallArgs) drivers.Result {
					t.Fatal("read-only recovery cannot dispatch another model invocation")
					return drivers.Result{}
				})
				require.NoError(t, err)
				applySessionResult(t, c, "execute", args.PacketID, result)
				record := c.sessionRecord("execute", args.PacketID)
				require.Equal(t, completed.Session.SessionID, record.ThreadID)
				require.Equal(t, !legacyEphemeral, record.Last.Persistent)
				require.False(t, record.ResumeCandidate)
				require.Equal(t, "fresh", record.Configuration.SessionMode)
				require.Equal(t, 1, calls)
				require.Equal(t, 1, queries)
				require.Equal(t, int64(180), c.State.Budget.Totals.Tokens)
			})
		}
	}
}

func TestObservedSessionIdentityDoesNotAuthorizeUnsafeOrFreshResume(t *testing.T) {
	for _, scenario := range []string{"scoped", "fresh", "legacy-absent", "legacy-empty", "failed", "unknown", "cleanup-unconfirmed", "cleanup-error", "foreign-invocation", "backend-not-resumable"} {
		t.Run(scenario, func(t *testing.T) {
			record := &state.SessionRecord{Configuration: &state.SessionConfiguration{SessionMode: "scoped"}, Last: &state.SessionInvocation{ID: "owned-call", Persistent: true}}
			result := sessionResult(drivers.CallArgs{RequestID: "owned-call"}, drivers.MockResult(nil))
			wantResume := true
			switch scenario {
			case "fresh":
				record.Configuration.SessionMode, wantResume = "fresh", false
			case "legacy-absent":
				record.Configuration = nil
			case "legacy-empty":
				record.Configuration.SessionMode = ""
			case "failed":
				result.Session.Outcome = "failed"
			case "unknown":
				result.Session.Dispatch, result.Session.Outcome, wantResume = "unknown", "unknown", false
			case "cleanup-unconfirmed":
				result.Session.CleanupConfirmed, wantResume = false, false
			case "cleanup-error":
				result.CleanupError, wantResume = "owned process is not stopped", false
			case "foreign-invocation":
				result.Session.RequestID, wantResume = "foreign-call", false
			case "backend-not-resumable":
				result.Session.ResumeCandidate, wantResume = false, false
			}
			applySessionFacts(record, result, "receipt-hash", "candidate-hash")
			if scenario == "foreign-invocation" {
				require.Empty(t, record.ThreadID)
			} else {
				require.Equal(t, result.Session.SessionID, record.ThreadID)
			}
			require.Equal(t, wantResume, record.ResumeCandidate)
			if strings.HasPrefix(scenario, "cleanup-") || scenario == "unknown" || scenario == "foreign-invocation" {
				require.Equal(t, "blocked", record.Last.Status)
			}
		})
	}
}

func TestFreshVisibleHistoryStillBlocksUnknownNativeEffects(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		t.Run(harness, func(t *testing.T) {
			fx, c := sessionFixture(t)
			c.Config.Sessions.Mode = "fresh"
			args := sessionArgs(c)
			args.Binding.Harness = harness
			_, err := c.invokeScoped("execute", args.PacketID, args, func(call drivers.CallArgs) drivers.Result {
				require.True(t, call.PersistentSession)
				result := sessionResult(call, drivers.Result{})
				result.Session.Dispatch, result.Session.Outcome, result.Session.ResumeCandidate = "unknown", "unknown", false
				return result
			})
			require.Error(t, err)
			require.NotEmpty(t, c.sessionRecord("execute", args.PacketID).ThreadID)
			c = restartSessionGuardContext(t, fx, c)
			_, err = c.invokeScoped("execute", args.PacketID, args, func(drivers.CallArgs) drivers.Result {
				t.Fatal("fresh history visibility cannot authorize replaying unknown effects")
				return drivers.Result{}
			})
			require.ErrorContains(t, err, "resume and fresh replay are blocked")
			require.False(t, c.sessionRecord("execute", args.PacketID).ResumeCandidate)
		})
	}
}
