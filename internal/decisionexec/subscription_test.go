package decisionexec

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func binding() *decision.SubscriptionBinding {
	return &decision.SubscriptionBinding{Profile: "hydrogen-local", Harness: "antigravity", Channel: "agy-native", Model: "gemini-4-hydrogen-low", Effort: "low"}
}

func nativeResult(id string) drivers.Result {
	return drivers.Result{Text: `{"answer":"bounded"}`, Model: "gemini-4-hydrogen-low", Settled: true,
		Session:            &drivers.SessionOutcome{RequestID: id, RunID: "original-run", Dispatch: "dispatched", Outcome: "completed", CleanupConfirmed: true},
		AntigravityReceipt: json.RawMessage(`{"fixture":"verified by the native driver"}`)}
}

func event(id string, sequence int) drivers.MachineLifecycle {
	return drivers.MachineLifecycle{LifecycleVersion: 1, Type: "lifecycle", RequestID: id, RunID: "original-run", Sequence: sequence,
		Event: "dispatch-intent", Persistent: true, Configuration: json.RawMessage("null"), Dispatch: "not-dispatched", Outcome: "unknown"}
}

func testNative(t *testing.T, call func(drivers.CallArgs, string) drivers.Result) {
	t.Helper()
	saved := invoke
	invoke = call
	t.Cleanup(func() { invoke = saved })
}

func TestOptionsUseOnlyFrozenToolFreeSubscriptionAndDurableACK(t *testing.T) {
	// Durable task identities use physical paths, including macOS /var aliases.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	b := binding()
	seen := ""
	testNative(t, func(args drivers.CallArgs, model string) drivers.Result {
		require.Equal(t, "prompt", args.Prompt)
		require.Equal(t, dir, args.Cwd)
		require.Equal(t, "antigravity", args.Binding.Harness)
		require.Equal(t, "gemini-4-hydrogen-low", model)
		require.Equal(t, "low", args.Binding.Thinking)
		require.Empty(t, args.Binding.Provider)
		require.Empty(t, args.APIKeys)
		require.Empty(t, args.Native)
		require.True(t, args.EvidenceOnly)
		require.Equal(t, "read-only", args.PermissionMode)
		require.Equal(t, &config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"}, args.Subscription)
		require.False(t, args.PersistentSession)
		require.Empty(t, args.SessionID)
		require.Equal(t, int64(1000), args.Watchdog.WallTimeoutMs)
		require.Equal(t, state.SessionRecoveryDirectory(dir, args.RequestID), args.RecoveryDirectory)
		_, err := os.Stat(args.RecoveryDirectory)
		require.True(t, os.IsNotExist(err), "only the adapter creates its exclusive leaf")
		require.NoError(t, args.OnLifecycle(event(args.RequestID, 1)))
		checkpoint, _, err := state.ReadSessionReceipt(dir, args.RequestID, args.RequestID, "")
		require.NoError(t, err)
		require.Equal(t, state.PreparationHash(binding()), checkpoint.Binding)
		require.Contains(t, string(checkpoint.Result), "original-run")
		require.NoError(t, args.OnLifecycle(event(args.RequestID, 2)))
		seen = args.RequestID
		return nativeResult(args.RequestID)
	})
	o := Options(context.Background(), b, dir, dir, time.Second)
	b.Model = "gemini-4-neon-high" // caller mutations cannot change the frozen process binding
	r := o.SubscriptionCall(context.Background(), "prompt", "decision-A")
	require.Equal(t, seen, r.Native.RequestID)
	require.Equal(t, "accepted", r.Native.Dispatch)
	require.True(t, r.Native.CleanupConfirmed)
	require.Len(t, r.Native.ReceiptHash, 64)
	require.Nil(t, r.Usage.Input)
	require.NotEqual(t, requestIdentity("decision-B"), seen)
	stats, err := os.Stat(filepath.Join(dir, ".sessions"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), stats.Mode().Perm())
}

func TestOptionsRejectStorageAndCheckpointFailuresBeforeAcknowledging(t *testing.T) {
	require.Nil(t, Options(context.Background(), nil, "", "", time.Second).SubscriptionCall)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	testNative(t, func(args drivers.CallArgs, _ string) drivers.Result {
		require.Error(t, args.OnLifecycle(event("wrong", 1)))
		require.Error(t, args.OnLifecycle(event(args.RequestID, 2)))
		require.NoError(t, args.OnLifecycle(event(args.RequestID, 1)))
		changed := event(args.RequestID, 2)
		changed.RunID = "new-run"
		require.Error(t, args.OnLifecycle(changed))
		saved := writeCheckpoint
		writeCheckpoint = func(string, string, state.SessionReceipt) (string, error) { return "", errors.New("disk full") }
		err := args.OnLifecycle(event(args.RequestID, 2))
		writeCheckpoint = saved
		require.ErrorContains(t, err, "disk full")
		require.NoError(t, args.OnLifecycle(event(args.RequestID, 2)), "failed persistence did not acknowledge or advance sequence")
		return drivers.Result{}
	})
	o := Options(context.Background(), binding(), dir, dir, time.Second)
	r := o.SubscriptionCall(context.Background(), "prompt", "checkpoint")
	require.False(t, r.Native.CleanupConfirmed)
	require.Equal(t, "possible", r.Native.Dispatch)
	for _, task := range []string{filepath.Join(dir, "missing"), filepath.Join(dir, "unsafe")} {
		if filepath.Base(task) == "unsafe" {
			require.NoError(t, os.Mkdir(task, 0700))
			require.NoError(t, os.Symlink(dir, filepath.Join(task, ".sessions")))
		}
		o := Options(context.Background(), binding(), dir, task, time.Second)
		r := o.SubscriptionCall(context.Background(), "prompt", "not-started")
		require.Equal(t, "not-dispatched", r.Native.Dispatch)
		require.True(t, r.Native.CleanupConfirmed)
	}
}

func TestCanonicalPathFailurePreventsDispatchAndRecovery(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	id := requestIdentity("path-failure")
	raw, err := json.Marshal(event(id, 1))
	require.NoError(t, err)
	_, err = state.WriteSessionReceipt(dir, id, state.SessionReceipt{Version: 1, InvocationID: id, Key: id,
		Task: dir, Cwd: dir, Binding: state.PreparationHash(binding()), Result: raw})
	require.NoError(t, err)

	calls, queries := 0, 0
	testNative(t, func(drivers.CallArgs, string) drivers.Result {
		calls++
		return nativeResult(id)
	})
	savedRecover, savedAbsolute := recoverMachine, absolutePath
	t.Cleanup(func() { recoverMachine, absolutePath = savedRecover, savedAbsolute })
	recoverMachine = func(drivers.MachineRecoveryArgs) (drivers.Result, error) {
		queries++
		return nativeResult(id), nil
	}
	wantErr := errors.New("absolute task path unavailable")
	absolutePath = func(path string) (string, error) {
		require.Equal(t, dir, path)
		// A usable path must never override the failed resolution.
		return dir, wantErr
	}
	root, err := canonical(dir)
	require.Empty(t, root)
	require.Same(t, wantErr, err)

	o := Options(context.Background(), binding(), dir, dir, time.Second)
	r := o.SubscriptionCall(context.Background(), "prompt", "path-failure")
	require.Equal(t, decision.NativeExecution{RequestID: id, Dispatch: "not-dispatched", Outcome: "failed", CleanupConfirmed: true}, r.Native)
	r = o.SubscriptionRecover("path-failure")
	require.Equal(t, decision.NativeExecution{RequestID: id, Dispatch: "unknown", Outcome: "unknown"}, r.Native)
	require.Zero(t, calls, "a failed task path must not dispatch a native invocation")
	require.Zero(t, queries, "even a valid checkpoint cannot authorize recovery without a physical task path")
}

func TestReadOnlyRecoveryUsesOriginalRunAndNeverDispatches(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	calls, queries := 0, 0
	testNative(t, func(args drivers.CallArgs, _ string) drivers.Result {
		calls++
		require.NoError(t, args.OnLifecycle(event(args.RequestID, 1)))
		return drivers.Result{Session: &drivers.SessionOutcome{RequestID: args.RequestID, Dispatch: "unknown", Outcome: "unknown"}}
	})
	saved := recoverMachine
	t.Cleanup(func() { recoverMachine = saved })
	recoverMachine = func(args drivers.MachineRecoveryArgs) (drivers.Result, error) {
		queries++
		require.Equal(t, "original-run", args.RunID)
		require.Equal(t, dir, args.Cwd)
		require.Equal(t, "antigravity", args.Harness)
		require.Equal(t, "low", args.ReasoningEffort)
		require.Equal(t, "read-only", args.PermissionMode)
		require.True(t, args.EvidenceOnly)
		require.Equal(t, &config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"}, args.Subscription)
		require.Equal(t, state.SessionRecoveryDirectory(dir, args.RequestID), args.RecoveryDirectory)
		return nativeResult(args.RequestID), nil
	}
	o := Options(context.Background(), binding(), dir, dir, time.Second)
	unknown := o.SubscriptionCall(context.Background(), "prompt", "recover-me")
	require.False(t, unknown.Native.CleanupConfirmed)
	r := o.SubscriptionRecover("recover-me")
	require.True(t, r.Native.CleanupConfirmed)
	require.Equal(t, "accepted", r.Native.Dispatch)
	require.Equal(t, 1, calls)
	require.Equal(t, 1, queries)
	recoverMachine = func(drivers.MachineRecoveryArgs) (drivers.Result, error) {
		return drivers.Result{}, errors.New("not yet terminal")
	}
	require.False(t, o.SubscriptionRecover("recover-me").Native.CleanupConfirmed)
}

func TestRecoveryRejectsMissingOrChangedCheckpoint(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	id := requestIdentity("one")
	o := Options(context.Background(), binding(), dir, dir, time.Second)
	saved := recoverMachine
	t.Cleanup(func() { recoverMachine = saved })
	recoverMachine = func(drivers.MachineRecoveryArgs) (drivers.Result, error) {
		t.Fatal("unproven invocation was queried")
		return drivers.Result{}, nil
	}
	require.False(t, o.SubscriptionRecover("one").Native.CleanupConfirmed)
	require.False(t, Options(context.Background(), binding(), dir, filepath.Join(dir, "missing"), time.Second).SubscriptionRecover("one").Native.CleanupConfirmed)
	for _, change := range []func(*state.SessionReceipt){
		func(c *state.SessionReceipt) { c.Cwd = "different" },
		func(c *state.SessionReceipt) { c.Binding = "different" },
		func(c *state.SessionReceipt) {
			c.Result = json.RawMessage(`{"requestId":"other","sequence":1,"runId":"x"}`)
		},
		func(c *state.SessionReceipt) { c.Result = json.RawMessage(`{"requestId":2}`) },
	} {
		raw, _ := json.Marshal(event(id, 1))
		checkpoint := state.SessionReceipt{Version: 1, InvocationID: id, Key: id, Task: dir, Cwd: dir, Binding: state.PreparationHash(binding()), Result: raw}
		change(&checkpoint)
		_, err := state.WriteSessionReceipt(dir, id, checkpoint)
		require.NoError(t, err)
		require.False(t, o.SubscriptionRecover("one").Native.CleanupConfirmed)
	}
}

func TestNativeResultDoesNotInferCompletionOrUsage(t *testing.T) {
	r := nativeResult("one")
	r.Settled = false
	r.Session.Outcome = "failed"
	r.Usage = &drivers.Usage{Input: 17, Output: 3}
	out := result(r, "one")
	require.Equal(t, "dispatched", out.Native.Dispatch)
	require.EqualValues(t, 17, *out.Usage.Input)
	require.EqualValues(t, 3, *out.Usage.Output)
}
