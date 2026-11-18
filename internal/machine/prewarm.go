package machine

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/metrics"
	"github.com/ckanner/flywheel/internal/state"
)

type reviewPrewarm struct {
	handle  drivers.PreparedDSH
	record  state.WorkerPrewarm
	binding config.LoopBinding
	phase   *metrics.Phase
}

var prepareReviewDSH = func(a drivers.DSHArgs) drivers.PreparedDSH { return drivers.PrepareDSH(a) }
var reviewWarmBinding = drivers.DSHPrewarmBinding
var recoverReviewDSH = drivers.RecoverDSHPrewarm
var recordUnstartedReviewDSH = drivers.RecordUnstartedDSHPrewarm
var makePrewarmDirectory = os.MkdirAll

func (c *Context) prewarmDirectory(id string) string {
	return filepath.Join(c.TaskDir, ".control", "prewarm", id)
}

func reviewDSHArgs(a drivers.CallArgs) drivers.DSHArgs {
	return drivers.DSHArgs{Web: a.Web, Native: a.Native, Managed: a.Managed, Prompt: a.Prompt, Provider: a.Binding.Provider, Model: a.Model, Thinking: a.Binding.Thinking,
		PermissionMode: a.PermissionMode, Cwd: a.Cwd, WorkspaceCwd: a.WorkspaceCwd, SessionTitle: a.SessionTitle,
		SessionID: a.SessionID, PersistentSession: a.PersistentSession, RequestID: a.RequestID,
		APIKeys: a.APIKeys, Watchdog: a.Watchdog, OnEvent: a.OnEvent}
}

// Prewarming owns no Session and sends no model input. The sole speculative
// resource belongs to this serial packet's Review, never to an execution lane.
func (c *Context) beginReviewPrewarm(packet *contract.Packet) error {
	if !c.prewarmReview || !c.Config.Scheduler.Prewarm || c.prewarm != nil || c.parallel != nil || c.State.SharedExecution != nil || c.Config.Drivers["review"] != "real" {
		return nil
	}
	if err := c.err(); err != nil {
		return err
	}
	if err := c.reconcileSessionReceipts(); err != nil {
		return err
	}
	scope := packet.PacketID
	if c.quick() {
		scope = "final:" + scope
	}
	if c.sessionReplay("review", scope) || c.budgetGate() == "stop" {
		return c.err()
	}
	binding, err := fallback.ResolveBinding(c.Config, "review", "")
	if err != nil {
		return err
	}
	if binding.Harness != "dsh" || c.missingKeyForRealDriver("review", binding.Channel) != "" {
		return nil
	}
	a := reviewDSHArgs(drivers.CallArgs{Native: c.Config.NativeProfiles[binding.NativeProfile].Options, Model: binding.Model, PermissionMode: c.Config.PermissionMode("review", binding.Harness),
		Cwd: c.unitCwd(packet), WorkspaceCwd: c.WorkspaceCwd, APIKeys: c.apiKeysFor(binding.Channel), Watchdog: c.wdogFor("review"),
		Binding: config.LoopBinding{NativeProfile: binding.NativeProfile, Harness: binding.Harness, Model: binding.Model, Channel: binding.Channel,
			Provider: fallback.ResolveProvider(c.Config, binding.Harness, binding.Provider, binding.Channel), Thinking: binding.Thinking}})
	if a.Model == drivers.DSHWebModel {
		a.Web = &drivers.DSHWebRequest{Version: 1, ExecutionDomainID: "flywheel-" + sessionHash([]any{"web-domain-v1", c.TaskDir}), RequireIsolation: true}
	}
	a.PrewarmID = "warm-" + rand.Text()
	a.PrewarmDirectory = c.prewarmDirectory(a.PrewarmID)
	if err := makePrewarmDirectory(a.PrewarmDirectory, 0o700); err != nil {
		return err
	}
	identity, err := reviewWarmBinding(a)
	if err != nil {
		return err
	}
	warm := &reviewPrewarm{binding: config.LoopBinding{NativeProfile: binding.NativeProfile, Harness: "dsh", Provider: a.Provider, Model: a.Model, Channel: binding.Channel, Thinking: a.Thinking}, record: state.WorkerPrewarm{Version: 1, ID: a.PrewarmID, Scope: scope,
		Binding: identity, StartedAt: c.nowISO(), EngineRunID: c.runID}}
	if c.runID != "" && c.metricsFile != "" {
		warm.phase = &metrics.Phase{ID: a.PrewarmID, Run: c.runID, Name: "worker-prewarm", Scope: scope, ParentID: c.phaseParent, StartedAt: c.Now()}
		if err := metrics.BeginPhase(c.metricsFile, *warm.phase); err != nil {
			return err
		}
	}
	c.State.WorkerPrewarm = &warm.record
	if c.State.Sessions == nil {
		c.State.Sessions = &state.SessionLedger{Records: map[string]*state.SessionRecord{}}
	}
	c.State.Sessions.Version = max(3, c.State.Sessions.Version) // preserve newer downgrade fences
	if a.Web != nil {
		c.State.Sessions.Version = max(5, c.State.Sessions.Version)
	}
	c.save()
	if c.fatal != nil {
		// Rename may have succeeded before a failed fsync. Record the positive
		// local no-process fact so restart can reconcile that durable intent.
		return errors.Join(c.fatal, recordUnstartedReviewDSH(a.PrewarmID, a.PrewarmDirectory, identity))
	}
	warm.handle = prepareReviewDSH(a)
	c.prewarm = warm
	c.Log(fmt.Sprintf("worker prewarm %s: preparing DSH Review %s without a model prompt", warm.record.ID, scope))
	return nil
}

func (c *Context) endPrewarmPhase(warm *reviewPrewarm, disposition string) error {
	c.Log(fmt.Sprintf("worker prewarm %s: %s (%s)", warm.record.ID, disposition, warm.record.Scope))
	if warm.phase == nil {
		return nil
	}
	ended := c.Now()
	warm.phase.EndedAt = &ended
	return metrics.EndPhase(c.metricsFile, *warm.phase)
}

// Always wait for the SDK and process group. A failed Close leaves the durable
// obligation intact; changing the optional setting cannot erase that fact.
func (c *Context) closeReviewPrewarm(reason string) error {
	warm := c.prewarm
	if warm == nil {
		return nil
	}
	c.prewarm = nil
	err := warm.handle.Close()
	if err != nil {
		c.fatal = errors.Join(c.fatal, fmt.Errorf("worker prewarm cleanup unconfirmed: %w", err), c.endPrewarmPhase(warm, "cleanup-unknown"))
		return c.fatal
	}
	if err := c.endPrewarmPhase(warm, reason); err != nil {
		c.fatal = errors.Join(c.fatal, err)
	}
	c.State.WorkerPrewarm = nil
	c.save()
	return c.err()
}

func (c *Context) reconcileWorkerPrewarm() error {
	record := c.State.WorkerPrewarm
	if record == nil {
		return nil
	}
	if c.prewarm != nil && c.prewarm.record == *record {
		return nil // owned by this process; no other caller may adopt the handle
	}
	if err := recoverReviewDSH(record.ID, c.prewarmDirectory(record.ID), record.Binding); err != nil {
		return fmt.Errorf("previous worker prewarm cleanup is unconfirmed; reconcile its original receipt before resuming: %w", err)
	}
	c.Log(fmt.Sprintf("worker prewarm %s: recovered confirmed cleanup without model dispatch", record.ID))
	c.State.WorkerPrewarm = nil
	c.save()
	return c.err()
}

// This handoff happens only after the real invocation intent is durable. The
// normal session protocol owns any subsequent unknown dispatch or cleanup.
func (c *Context) claimReviewPrewarm(role, scope string, a *drivers.CallArgs) error {
	warm := c.prewarm
	if warm == nil {
		return nil
	}
	if role != "review" || scope != warm.record.Scope || a.Binding != warm.binding || !warm.handle.Matches(reviewDSHArgs(*a)) {
		return c.closeReviewPrewarm("discarded: invocation binding changed")
	}
	c.State.WorkerPrewarm = nil
	c.save()
	if err := c.err(); err != nil {
		return errors.Join(err, c.closeReviewPrewarm("discarded: invocation handoff not durable"))
	}
	if err := c.endPrewarmPhase(warm, "claimed by durable Review invocation "+a.RequestID); err != nil {
		return errors.Join(err, c.closeReviewPrewarm("discarded: handoff metrics failed"))
	}
	a.Prewarm = warm.handle
	c.prewarm = nil
	return nil
}
