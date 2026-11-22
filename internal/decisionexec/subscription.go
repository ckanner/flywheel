// Package decisionexec connects the bounded decision protocol to a tool-free
// subscribed native harness. It cannot execute the selected action.
package decisionexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/watchdog"
)

var invoke = drivers.InvokeHarness
var recoverMachine = drivers.RecoverMachine
var writeCheckpoint = state.WriteSessionReceipt
var absolutePath = filepath.Abs

func Options(ctx context.Context, binding *decision.SubscriptionBinding, cwd, taskDir string, timeout time.Duration) decision.Options {
	o := decision.Options{Backend: "subscription", Subscription: binding, Timeout: timeout}
	if binding == nil {
		return o
	}
	frozen := *binding
	o.Subscription = &frozen
	o.Model, o.ExpectedModel = frozen.Model, frozen.Model
	o.SubscriptionCall = func(ctx context.Context, prompt, id string) decision.SubscriptionResult {
		requestID := requestIdentity(id)
		root, err := canonical(taskDir)
		if err != nil {
			return notDispatched(requestID)
		}
		directory, err := state.PrepareSessionRecovery(root, requestID)
		if err != nil {
			return notDispatched(requestID)
		}
		var sequence int
		var runID string
		checkpoint := func(event drivers.MachineLifecycle) error {
			if event.RequestID != requestID || event.RunID == "" || event.Sequence != sequence+1 || runID != "" && event.RunID != runID {
				return fmt.Errorf("decision native lifecycle contradicts the frozen invocation")
			}
			raw, _ := json.Marshal(event) // The driver validated the bounded wire object.
			_, err := writeCheckpoint(root, requestID, state.SessionReceipt{Version: 1, InvocationID: requestID, Key: requestID,
				Task: root, Cwd: cwd, Binding: state.PreparationHash(frozen), Result: raw})
			if err == nil {
				sequence, runID = event.Sequence, event.RunID
			}
			return err
		}
		r := invoke(drivers.CallArgs{Prompt: prompt, Cwd: cwd, TaskDir: taskDir, RequestID: requestID,
			RecoveryDirectory: directory, OnLifecycle: checkpoint,
			Binding: config.LoopBinding{Harness: frozen.Harness, Channel: frozen.Channel, Model: frozen.Model, Thinking: frozen.Effort},
			Model:   frozen.Model, PermissionMode: "read-only", EvidenceOnly: true,
			Subscription: &config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"},
			Watchdog:     watchdog.Opts{Context: ctx, WallTimeoutMs: timeout.Milliseconds(), IdleTimeoutMs: timeout.Milliseconds()},
		}, frozen.Model)
		return result(r, requestID)
	}
	o.SubscriptionRecover = func(id string) decision.SubscriptionResult {
		requestID := requestIdentity(id)
		unknown := decision.SubscriptionResult{Native: decision.NativeExecution{RequestID: requestID, Dispatch: "unknown", Outcome: "unknown"}}
		root, err := canonical(taskDir)
		if err != nil {
			return unknown
		}
		checkpoint, _, err := state.ReadSessionReceipt(root, requestID, requestID, "")
		if err != nil || checkpoint.Cwd != cwd || checkpoint.Binding != state.PreparationHash(frozen) {
			return unknown
		}
		var event drivers.MachineLifecycle
		if json.Unmarshal(checkpoint.Result, &event) != nil || event.RequestID != requestID || event.RunID == "" || event.Sequence < 1 {
			return unknown
		}
		r, err := recoverMachine(drivers.MachineRecoveryArgs{Harness: frozen.Harness, RequestID: requestID, RunID: event.RunID,
			RecoveryDirectory: state.SessionRecoveryDirectory(root, requestID), Cwd: cwd, Model: frozen.Model, ReasoningEffort: frozen.Effort,
			PermissionMode: "read-only", EvidenceOnly: true, Subscription: &config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"},
			Watchdog: watchdog.Opts{Context: ctx}})
		if err != nil {
			return unknown
		}
		return result(r, requestID)
	}
	return o
}

func requestIdentity(id string) string {
	sum := sha256.Sum256([]byte(id))
	return "jev-" + hex.EncodeToString(sum[:])[:48]
}

func canonical(path string) (string, error) {
	abs, err := absolutePath(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func notDispatched(id string) decision.SubscriptionResult {
	return decision.SubscriptionResult{Native: decision.NativeExecution{RequestID: id, Dispatch: "not-dispatched", Outcome: "failed", CleanupConfirmed: true}}
}

func result(r drivers.Result, requestID string) decision.SubscriptionResult {
	out := decision.SubscriptionResult{Text: r.Text, Model: r.Model, Native: decision.NativeExecution{RequestID: requestID, Dispatch: "possible", Outcome: "unknown"}}
	if r.Session != nil {
		out.Native.Dispatch, out.Native.Outcome, out.Native.CleanupConfirmed = r.Session.Dispatch, r.Session.Outcome, r.Session.CleanupConfirmed
		if r.Settled && r.Session.Outcome == "completed" {
			out.Native.Dispatch = "accepted"
		}
	}
	if len(r.AntigravityReceipt) > 0 {
		hash := sha256.Sum256(r.AntigravityReceipt)
		out.Native.ReceiptHash = hex.EncodeToString(hash[:])
	}
	if r.Usage != nil {
		out.Usage = decision.Usage{Input: &r.Usage.Input, Output: &r.Usage.Output}
	}
	return out
}
