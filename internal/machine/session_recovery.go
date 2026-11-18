package machine

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/watchdog"
)

var prepareSessionRecovery = state.PrepareSessionRecovery
var recoverSessionMachine = drivers.RecoverMachine

// Only locked public adapters advertising the typed durable lifecycle receive
// this path. Other tracked backends retain their terminal-receipt reconciliation
// without claiming an unimplemented live recovery contract.
func (c *Context) prepareSessionLifecycle(r *state.SessionRecord, args *drivers.CallArgs) error {
	harness := args.Binding.Harness
	if harness == "dsh" && args.Web != nil {
		return c.prepareWebSessionRecovery(r, args)
	}
	if harness != "codex-pro" && harness != "claude-code-pro" {
		return nil
	}
	model := args.Model
	if model == "" {
		model = args.Binding.Model
	}
	permission := args.PermissionMode
	if permission == "" && harness == "codex-pro" {
		permission = "read-only"
	}
	if model == "" || (permission != "read-only" && permission != "workspace-write" && permission != "danger-full-access") {
		return nil // normal local admission will reject before any backend dispatch
	}
	effort := args.Binding.Thinking
	if effort == "" {
		effort = "high"
	}
	directory, err := prepareSessionRecovery(c.TaskDir, r.Last.ID)
	if err != nil {
		return fmt.Errorf("cannot prepare durable invocation evidence: %w", err)
	}
	version := drivers.CodexProPackageVersion
	if harness == "claude-code-pro" {
		version = drivers.ClaudeCodeProPackageVersion
	}
	r.Last.Recovery = &state.SessionRecovery{Version: 1, Harness: harness, AdapterVersion: version, Directory: directory, Model: model, Effort: effort, PermissionMode: permission}
	args.RecoveryDirectory = directory
	args.OnLifecycle = func(event drivers.MachineLifecycle) error { return c.persistSessionLifecycle(r, event) }
	return nil
}

func (c *Context) persistSessionLifecycle(r *state.SessionRecord, event drivers.MachineLifecycle) error {
	if err := c.err(); err != nil {
		return err
	}
	recovery := r.Last.Recovery
	if recovery == nil || event.LifecycleVersion != 1 || event.Type != "lifecycle" || event.RequestID != r.Last.ID ||
		event.RunID == "" || (recovery.RunID != "" && recovery.RunID != event.RunID) || event.Sequence != recovery.Sequence+1 ||
		event.Persistent != r.Last.Persistent {
		return fmt.Errorf("lifecycle checkpoint does not belong to the current invocation")
	}
	if event.ThreadID != nil {
		if c.parallel != nil {
			if err := c.parallel.reserveThread(recovery.Harness, *event.ThreadID); err != nil {
				return err
			}
		}
		for _, other := range c.State.Sessions.Records {
			if other != r && other.ThreadID == *event.ThreadID && strings.HasPrefix(other.Protocol, recovery.Harness+"/") {
				return fmt.Errorf("lifecycle checkpoint belongs to another role's history")
			}
		}
	}
	key := sessionKey(r.Role, r.Scope) + "/lifecycle"
	receipt := state.SessionReceipt{Version: 1, InvocationID: r.Last.ID, Key: key, Task: c.TaskDir,
		Cwd: r.Cwd, Binding: r.Binding, GuardBefore: r.Last.GuardBefore}
	if event.Event == "cleanup-confirmed" && event.CleanupConfirmed {
		var err error
		receipt.CandidateAfter, err = c.sessionCandidateFingerprint(r.Cwd, r.CandidateScope)
		if err != nil {
			return fmt.Errorf("cannot checkpoint terminal candidate: %w", err)
		}
		receipt.GuardAfter, err = c.sessionGuard(r.Role, r.Scope)
		if err != nil {
			return fmt.Errorf("cannot checkpoint terminal role guard: %w", err)
		}
		if r.Role == "execute" {
			receipt.Report, _ = readUnitReport(r.Cwd, filepath.Join(c.taskDirPrefix(), "reports", r.Scope+".done.json"))
		}
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("cannot encode typed lifecycle checkpoint: %w", err)
	}
	receipt.Result = raw
	digest, err := persistSessionReceipt(c.TaskDir, key, receipt)
	if err != nil {
		return fmt.Errorf("cannot save durable lifecycle checkpoint: %w", err)
	}
	recovery.RunID, recovery.Sequence, recovery.Event, recovery.CheckpointDigest = event.RunID, event.Sequence, event.Event, digest
	c.save()
	return c.err()
}

// The latest evidence can be one fsynced event ahead of state when a CAS failed.
// Its exact invocation and binding are checked, and it only authorizes a
// read-only query. No lifecycle fact alone authorizes replaying model work.
func (c *Context) loadSessionLifecycle(r *state.SessionRecord) (*state.SessionReceipt, drivers.MachineLifecycle, error) {
	recovery := r.Last.Recovery
	version := drivers.CodexProPackageVersion
	if recovery.Harness == "claude-code-pro" {
		version = drivers.ClaudeCodeProPackageVersion
	}
	if recovery.AdapterVersion != version || recovery.Directory != state.SessionRecoveryDirectory(c.TaskDir, r.Last.ID) || r.Task != c.TaskDir {
		return nil, drivers.MachineLifecycle{}, fmt.Errorf("recovery directory does not belong to this task invocation")
	}
	key := sessionKey(r.Role, r.Scope) + "/lifecycle"
	receipt, digest, err := loadSessionReceipt(c.TaskDir, key, r.Last.ID, "")
	if err != nil {
		return nil, drivers.MachineLifecycle{}, err
	}
	var event drivers.MachineLifecycle
	if err := json.Unmarshal(receipt.Result, &event); err != nil || event.LifecycleVersion != 1 || event.Type != "lifecycle" ||
		event.RequestID != r.Last.ID || strings.TrimSpace(event.RunID) == "" || strings.ContainsRune(event.RunID, 0) ||
		receipt.Cwd != r.Cwd || receipt.Binding != r.Binding || receipt.GuardBefore != r.Last.GuardBefore || event.Persistent != r.Last.Persistent ||
		event.Sequence < max(1, recovery.Sequence) || event.Sequence > recovery.Sequence+1 ||
		(recovery.RunID != "" && recovery.RunID != event.RunID) ||
		(event.Sequence == recovery.Sequence && digest != recovery.CheckpointDigest) {
		return nil, event, fmt.Errorf("durable lifecycle checkpoint identity or digest is invalid")
	}
	return receipt, event, nil
}

func (c *Context) recoverSessionReceipt(key string, r *state.SessionRecord) error {
	if r.Last.Recovery.Harness == "dsh" {
		return c.recoverWebSessionReceipt(key, r)
	}
	checkpoint, event, err := c.loadSessionLifecycle(r)
	if err != nil {
		return err
	}
	recovery := r.Last.Recovery
	var native json.RawMessage
	if r.Configuration != nil {
		native = r.Configuration.NativeOptions
	}
	result, err := recoverSessionMachine(drivers.MachineRecoveryArgs{Native: native, Harness: recovery.Harness, RequestID: r.Last.ID, RunID: event.RunID,
		RecoveryDirectory: recovery.Directory, Cwd: sessionNativeCwd(r), Model: recovery.Model, ReasoningEffort: recovery.Effort, PermissionMode: recovery.PermissionMode,
		PersistentSession: r.Last.Persistent, SessionID: r.Last.RequestedThread, Watchdog: watchdog.Opts{Context: c.RunContext}})
	if err != nil {
		return err
	}
	result.Recovered = true
	// Claude can propose a new persistent session UUID before native startup.
	// A proven no-dispatch receipt has no opened session; the unobserved proposal
	// is not a history handle and must not be promoted to one during recovery.
	unopenedClaude := recovery.Harness == "claude-code-pro" && string(event.Configuration) == "null" && result.Session != nil &&
		result.Session.Dispatch == "not-dispatched" && result.Session.SessionID == ""
	if result.Session == nil || result.Session.RequestID != r.Last.ID || result.Session.RunID != event.RunID ||
		(event.ThreadID != nil && !unopenedClaude && result.Session.SessionID != *event.ThreadID) || (event.TurnID != nil && result.Session.TurnID != *event.TurnID) {
		return fmt.Errorf("recovered result does not match the durable lifecycle invocation")
	}
	if event.Event == "cleanup-confirmed" && (!event.CleanupConfirmed || !result.Session.CleanupConfirmed ||
		(event.Outcome != "unknown" && result.Session.Outcome != "unknown" && result.Session.Outcome != event.Outcome)) {
		return fmt.Errorf("recovered result contradicts the recorded terminal cleanup facts")
	}
	// A quiescent pre-dispatch rejection can be bound to the unchanged original
	// candidate without inventing a missing post-execution role guard.
	if safeSessionOutcome(result, r.Last.ID) && result.Session.Dispatch == "not-dispatched" && checkpoint.CandidateAfter == "" {
		candidate, candidateErr := c.sessionCandidateFingerprint(r.Cwd, r.CandidateScope)
		guard, guardErr := c.sessionGuard(r.Role, r.Scope)
		if candidateErr == nil && guardErr == nil && candidate == r.Last.CandidateBefore && guard == r.Last.GuardBefore {
			checkpoint.CandidateAfter, checkpoint.GuardAfter = candidate, guard
		}
	}
	// Without the original quiescent candidate checkpoint, attribution is still
	// useful but applying old outputs is not safe. Keep its absent proof absent.
	if event.Event != "cleanup-confirmed" && result.Session.Dispatch != "not-dispatched" {
		checkpoint.CandidateAfter, checkpoint.GuardAfter, checkpoint.Report = "", "", nil
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("recovered result cannot be serialized: %w", err)
	}
	digest, err := persistSessionReceipt(c.TaskDir, key, state.SessionReceipt{Version: 1, InvocationID: r.Last.ID, Key: key,
		Task: c.TaskDir, Cwd: r.Cwd, Binding: r.Binding, CandidateAfter: checkpoint.CandidateAfter,
		GuardBefore: r.Last.GuardBefore, GuardAfter: checkpoint.GuardAfter, Report: checkpoint.Report, Result: raw})
	if err != nil {
		return fmt.Errorf("recovered terminal receipt could not be saved: %w", err)
	}
	r.Last.ReceiptDigest = digest
	return nil
}
