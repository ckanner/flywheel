package machine

import (
	"encoding/json"
	"fmt"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/watchdog"
)

var recoverWebSession = drivers.RecoverDSHWeb

func (c *Context) prepareWebSessionRecovery(r *state.SessionRecord, args *drivers.CallArgs) error {
	directory, err := prepareSessionRecovery(c.TaskDir, r.Last.ID)
	if err != nil {
		return fmt.Errorf("cannot prepare durable Web invocation evidence: %w", err)
	}
	permission := args.PermissionMode
	if permission == "" {
		permission = "read-only"
	}
	r.Last.Recovery = &state.SessionRecovery{Version: 1, Harness: "dsh", AdapterVersion: drivers.DSHWebRecoveryVersion, Directory: directory,
		Model: drivers.DSHWebModel, Effort: "pro", PermissionMode: permission}
	args.Web.RecoveryDirectory = directory
	return nil
}

func (c *Context) recoverWebSessionReceipt(key string, r *state.SessionRecord) error {
	recovery := r.Last.Recovery
	configuration := r.Configuration
	if recovery.AdapterVersion != drivers.DSHWebRecoveryVersion || recovery.Directory != state.SessionRecoveryDirectory(c.TaskDir, r.Last.ID) || r.Task != c.TaskDir ||
		configuration == nil || configuration.Harness != "dsh" || configuration.Model != drivers.DSHWebModel || configuration.Thinking != "pro" {
		return fmt.Errorf("Web recovery does not match the original task and runtime contract")
	}
	var expected *drivers.DSHWebBinding
	if len(r.WebBinding) != 0 {
		if err := json.Unmarshal(r.WebBinding, &expected); err != nil {
			return fmt.Errorf("original Web binding is invalid: %w", err)
		}
	}
	result, err := recoverWebSession(drivers.DSHWebRecoveryArgs{RequestID: r.Last.ID, RecoveryDirectory: recovery.Directory, Cwd: r.Cwd, WorkspaceCwd: r.WorkspaceCwd,
		Provider: configuration.Provider, PermissionMode: recovery.PermissionMode, RequestedSessionID: r.Last.RequestedThread, Native: configuration.NativeOptions,
		ExecutionDomainID: "flywheel-" + sessionHash([]any{"web-domain-v1", c.TaskDir}), ExpectedBinding: expected, Watchdog: watchdog.Opts{Context: c.RunContext}})
	if err != nil {
		return err
	}
	if !safeSessionOutcome(result, r.Last.ID) || result.Session.Protocol != drivers.DSHSessionProtocol || len(result.WebReceipt) == 0 ||
		(r.ThreadID != "" && result.Session.SessionID != r.ThreadID) || (r.Last.RequestedThread != "" && result.Session.SessionID != r.Last.RequestedThread) {
		return fmt.Errorf("Web recovery did not prove the original complete invocation")
	}
	// A model response, tool/summary adoption, or process exit never invents the
	// candidate/role checkpoint the original Go invocation failed to capture.
	originalKey := key + "/web-original"
	receiptKey, receiptDigest := key, r.Last.ReceiptDigest
	if recovery.CheckpointDigest != "" {
		receiptKey, receiptDigest = originalKey, recovery.CheckpointDigest
	}
	receipt, _, readErr := loadSessionReceipt(c.TaskDir, receiptKey, r.Last.ID, receiptDigest)
	if readErr != nil && recovery.CheckpointDigest != "" {
		return fmt.Errorf("Web recovery cannot verify its preserved original checkpoint: %w", readErr)
	}
	if readErr != nil && result.Session.Dispatch != "not-dispatched" {
		return fmt.Errorf("Web recovery has no original terminal candidate checkpoint: %w", readErr)
	}
	candidate, candidateErr := c.sessionCandidateFingerprint(r.Cwd, r.CandidateScope)
	guard, guardErr := c.sessionGuard(r.Role, r.Scope)
	if candidateErr != nil || guardErr != nil {
		return fmt.Errorf("Web recovery cannot verify the current candidate and role guard")
	}
	if readErr != nil {
		if candidate != r.Last.CandidateBefore || guard != r.Last.GuardBefore {
			return fmt.Errorf("unsent Web invocation no longer has its original candidate and guard")
		}
		receipt = &state.SessionReceipt{Version: 1, InvocationID: r.Last.ID, Key: key, Task: c.TaskDir, Cwd: r.Cwd, Binding: r.Binding,
			CandidateAfter: candidate, GuardBefore: r.Last.GuardBefore, GuardAfter: guard}
	}
	if receipt.Cwd != r.Cwd || receipt.Binding != r.Binding || receipt.GuardBefore != r.Last.GuardBefore || receipt.CandidateAfter == "" ||
		receipt.CandidateAfter != candidate || receipt.GuardAfter != guard {
		return fmt.Errorf("Web recovery cannot replace the original candidate or role guard")
	}
	result.Recovered = true
	rejectChangedSessionGuard(r.Role, receipt.GuardBefore, receipt.GuardAfter, &result)
	raw, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("recovered Web result cannot be serialized: %w", err)
	}
	// Preserve the original candidate proof before replacing the result slot.
	// CAS must acknowledge its digest first, so a crash or failed final CAS can
	// repeat recovery without relaxing the stale result digest.
	if recovery.CheckpointDigest == "" {
		original := *receipt
		original.Key = originalKey
		if len(original.Result) == 0 {
			original.Result = raw // Only a positively unsent, missing receipt.
		}
		checkpoint, err := persistSessionReceipt(c.TaskDir, originalKey, original)
		if err != nil {
			return fmt.Errorf("original Web candidate checkpoint could not be preserved: %w", err)
		}
		recovery.CheckpointDigest = checkpoint
		c.save()
		if err := c.err(); err != nil {
			return fmt.Errorf("original Web checkpoint acknowledgement failed: %w", err)
		}
	}
	receipt.Key = key
	receipt.Result = raw
	digest, err := persistSessionReceipt(c.TaskDir, key, *receipt)
	if err != nil {
		return fmt.Errorf("recovered Web terminal receipt could not be saved: %w", err)
	}
	r.Last.ReceiptDigest = digest
	return nil
}
