package machine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/ckanner/flywheel/internal/watchdog"
)

func jsonClone[T any](value T) T {
	raw, _ := json.Marshal(value)
	var copy T
	_ = json.Unmarshal(raw, &copy)
	return copy
}

type parallelGroup struct {
	owner       *Context
	mu          sync.Mutex
	logMu       sync.Mutex
	cancel      context.CancelFunc
	slots       chan struct{}
	providers   map[string]chan struct{}
	threads     map[string]string
	directories map[string]string
}
type parallelLane struct {
	group   *parallelGroup
	id      string
	inbox   int
	release func()
}

func (c *Context) parallelBatch(milestone string) []*contract.Packet {
	if c.Config.Scheduler.Mode != "parallel" || c.quick() {
		return nil
	}
	batch := []*contract.Packet{}
	scopes := []string{}
	for _, p := range c.PacketList {
		ps := c.State.Packets[p.PacketID]
		if ps.Milestone != milestone || ps.Status == "completed" {
			continue
		}
		if ps.Status != "pending" || ps.BaseRef != nil || p.Schedule == nil || p.NeedsLiveCredentials {
			if len(batch) == 0 {
				return nil
			}
			break
		}
		ready := true
		for _, id := range p.Schedule.After {
			if c.State.Packets[id].Status != "completed" {
				ready = false
			}
		}
		for _, scope := range p.Schedule.Writes {
			if contract.InWriteScope(scope, scopes) {
				ready = false
			}
			for _, earlier := range scopes {
				if contract.InWriteScope(earlier, []string{scope}) {
					ready = false
				}
			}
		}
		if !ready {
			continue
		}
		batch = append(batch, p)
		scopes = append(scopes, p.Schedule.Writes...)
		if len(batch) == c.Config.Scheduler.MaxParallel {
			break
		}
	}
	if len(batch) < 2 {
		return nil
	}
	return batch
}

func (lane *parallelLane) merge(worker *Context) error {
	owner := lane.group.owner
	ps := worker.State.Packets[lane.id]
	if (worker.Config.Worktree.Enabled == nil || *worker.Config.Worktree.Enabled) && (ps.Worktree == nil || ps.Worktree.Mode != "worktree") {
		return fmt.Errorf("parallel unit lost its isolated worktree")
	}
	owner.State.Packets[lane.id] = jsonClone(ps)
	if worker.State.Status == "escalated" && owner.State.Status != "budget-stopped" {
		owner.State.Status = "escalated"
	}
	if worker.State.Sessions != nil {
		if owner.State.Sessions == nil {
			owner.State.Sessions = &state.SessionLedger{Version: 2, Records: map[string]*state.SessionRecord{}}
		}
		owner.State.Sessions.Version = max(owner.State.Sessions.Version, worker.State.Sessions.Version)
		key := sessionKey("execute", lane.id)
		if incoming := worker.State.Sessions.Records[key]; incoming != nil {
			for otherKey, other := range owner.State.Sessions.Records {
				if otherKey != key && incoming.ThreadID != "" && incoming.ThreadID == other.ThreadID && incoming.Protocol == other.Protocol {
					return fmt.Errorf("parallel history belongs to another packet")
				}
			}
			copy := jsonClone(incoming)
			if previous := owner.State.Sessions.Records[key]; previous != nil && previous.Last.ID == copy.Last.ID && previous.Last.Accounted {
				copy.Last.Accounted, copy.Last.Accounting = true, jsonClone(previous.Last.Accounting)
			}
			owner.State.Sessions.Records[key] = copy
		}
	}
	owner.State.Inbox = append(owner.State.Inbox, jsonClone(worker.State.Inbox[lane.inbox:])...)
	lane.inbox = len(worker.State.Inbox)
	if worker.State.LastProgressAt != nil && (owner.State.LastProgressAt == nil || *worker.State.LastProgressAt > *owner.State.LastProgressAt) {
		owner.State.LastProgressAt = jsonClone(worker.State.LastProgressAt)
	}
	return nil
}

func (lane *parallelLane) save(worker *Context) {
	g := lane.group
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := lane.merge(worker); err != nil {
		g.owner.fatal = err
	}
	g.owner.save()
	if err := g.owner.err(); err != nil {
		worker.fatal = err
		g.cancel()
	}
}

func (lane *parallelLane) metrics(worker *Context, args spawnMetricsArgs) {
	defer lane.finishTurn()
	g := lane.group
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := lane.merge(worker); err != nil {
		worker.fatal = err
		g.cancel()
		return
	}
	g.logMu.Lock()
	g.owner.recordSpawnMetrics(args)
	g.owner.save() // charge is durable before another hard-budget slot is released
	g.logMu.Unlock()
	worker.State.Budget = jsonClone(g.owner.State.Budget)
	key := sessionKey("execute", lane.id)
	if g.owner.State.Sessions != nil && g.owner.State.Sessions.Records[key] != nil {
		worker.State.Sessions.Records[key] = jsonClone(g.owner.State.Sessions.Records[key])
	}
	if err := g.owner.err(); err != nil {
		worker.fatal = err
		g.cancel()
	}
}

func (lane *parallelLane) budget(worker *Context) string {
	g := lane.group
	g.mu.Lock()
	defer g.mu.Unlock()
	g.logMu.Lock()
	defer g.logMu.Unlock()
	if err := g.owner.err(); err != nil {
		worker.fatal = err
		g.cancel()
		return "stop"
	}
	result := g.owner.budgetGate()
	if result == "stop" {
		g.cancel()
	}
	return result
}

func (lane *parallelLane) enter(worker *Context, channel string) (func(), error) {
	g := lane.group
	g.mu.Lock()
	provider := g.providers[channel]
	if provider == nil {
		limit := g.owner.Config.Scheduler.MaxParallel
		if value := g.owner.Config.Scheduler.Providers[channel]; value > 0 {
			limit = value
		}
		provider = make(chan struct{}, limit)
		g.providers[channel] = provider
	}
	g.mu.Unlock()
	acquired := []chan struct{}{}
	release := func() {
		for _, slot := range acquired {
			<-slot
		}
	}
	for _, slot := range []chan struct{}{g.slots, provider} {
		select {
		case slot <- struct{}{}:
			acquired = append(acquired, slot)
		case <-worker.RunContext.Done():
			release()
			return nil, worker.RunContext.Err()
		}
	}
	if lane.budget(worker) == "stop" {
		release()
		return nil, fmt.Errorf("parallel model dispatch stopped by shared budget")
	}
	return release, nil
}

func (lane *parallelLane) reserveThread(harness, thread string) error {
	g := lane.group
	g.mu.Lock()
	defer g.mu.Unlock()
	key := harness + "/" + thread
	if owner, found := g.threads[key]; found && owner != lane.id {
		return fmt.Errorf("parallel lifecycle thread is already owned by %s", owner)
	}
	if g.owner.State.Sessions != nil {
		for _, record := range g.owner.State.Sessions.Records {
			if record.Scope != lane.id && record.ThreadID == thread && record.Last.Recovery != nil && record.Last.Recovery.Harness == harness {
				return fmt.Errorf("parallel lifecycle reused another role's history")
			}
		}
	}
	g.threads[key] = lane.id
	return nil
}

var parallelDrivers = drivers.CreateDrivers

func (c *Context) runParallelExecute(packets []*contract.Packet) error {
	head, ok := verify.GetBaseRef(c.Cwd)
	if !ok {
		return fmt.Errorf("parallel execution requires a Git checkout")
	}
	for _, packet := range packets {
		binding, err := fallback.ResolveBinding(c.Config, "execute", packet.Route)
		if err != nil {
			return err
		}
		if c.Config.Drivers["execute"] != "mock" && !config.SupportsScopedSessions(binding.Harness) {
			return fmt.Errorf("parallel execution requires a durable invocation adapter: %s", binding.Harness)
		}
	}
	if c.Config.Worktree.Enabled != nil && !*c.Config.Worktree.Enabled {
		ids := make([]string, len(packets))
		for i, packet := range packets {
			ids[i] = packet.PacketID
		}
		c.State.SharedExecution = &state.SharedExecution{Cwd: c.Cwd, BaseRef: head, Packets: ids}
	}
	for _, packet := range packets {
		ps := c.State.Packets[packet.PacketID]
		ps.BaseRef, ps.Parallel = &head, true
		if c.Config.Worktree.Enabled == nil || *c.Config.Worktree.Enabled {
			wt := verify.EnsureUnitWorktree(c.Cwd, c.State.Slug, packet.PacketID, head)
			if wt.Mode != "worktree" {
				return fmt.Errorf("parallel worktree unavailable: %s", wt.Reason)
			}
			ps.Worktree = &wt
		}
		c.save()
		if err := c.err(); err != nil {
			return err
		}
	}
	// Every workspace was published above before any worker starts.
	parent := c.RunContext
	if parent == nil {
		parent = context.Background()
	}
	runContext, cancel := context.WithCancel(parent)
	defer cancel()
	limit := c.Config.Scheduler.MaxParallel
	if c.Config.Budget.Enforce == "hard" {
		limit = 1
	}
	group := &parallelGroup{owner: c, cancel: cancel, slots: make(chan struct{}, limit), providers: map[string]chan struct{}{}, threads: map[string]string{}}
	outcomes := make([]error, len(packets))
	var wg sync.WaitGroup
	workers := make([]*Context, len(packets))
	for i, packet := range packets {
		worker := *c
		worker.State = jsonClone(c.State)
		worker.RunContext = runContext
		worker.Drivers = parallelDrivers(c.Config)
		worker.sessionReports = nil
		worker.parallel = &parallelLane{group: group, id: packet.PacketID, inbox: len(worker.State.Inbox)}
		worker.Log = func(line string) { group.logMu.Lock(); defer group.logMu.Unlock(); c.Log(line) }
		workers[i] = &worker
	}
	for i, packet := range packets {
		worker := workers[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := worker.runExecuteLoop(packet)
			if err == nil && !result.Green && !result.Bounded && !result.Spun && !result.Deferred {
				err = fmt.Errorf("parallel packet %s stopped before L1 passed", packet.PacketID)
			}
			outcomes[i] = err
			if err != nil {
				cancel()
			}
		}()
	}
	wg.Wait()
	err := errors.Join(outcomes...)
	if err != nil {
		if c.State.Status == "running" {
			c.State.Status = "paused"
		}
		for i, failure := range outcomes {
			if failure != nil {
				c.State.Packets[packets[i].PacketID].LastFeedback = map[string]any{"parallel-error": failure.Error(), "at": c.nowISO()}
			}
		}
		c.save()
	}
	return err
}

// Integration runs after all workers have quiesced. Incorporating earlier units
// invalidates this unit's test/review proof and requires acceptance on the union.
func (c *Context) prepareParallelIntegration(packet *contract.Packet) error {
	ps := c.State.Packets[packet.PacketID]
	if !ps.Parallel {
		return nil
	}
	head, ok := verify.GetBaseRef(c.Cwd)
	if !ok {
		return fmt.Errorf("cannot inspect integration HEAD")
	}
	if ps.Worktree == nil && c.Config.Worktree.Enabled != nil && !*c.Config.Worktree.Enabled {
		// Shared-checkout execution was allowed, but its concurrent test result
		// cannot certify the quiescent candidate. Recheck before review/delivery.
		if !c.l1Step(packet).Pass {
			return fmt.Errorf("shared execution candidate failed acceptance after parallel workers stopped")
		}
		ps.Parallel = false
		c.save()
		return c.err()
	}
	if ps.Worktree == nil || ps.Worktree.Mode != "worktree" {
		return fmt.Errorf("parallel integration workspace is missing")
	}
	if !fileExists(ps.Worktree.Path) && c.currentL1Evidence(packet) {
		ps.Worktree = nil
		ps.Parallel = false
		c.save()
		return c.err()
	}
	if head != c.baseRefOf(ps) {
		commit := c.commitUnitChangesIn(ps.Worktree.Path, packet, c.ledgerExcludes(), "", "")
		if commit.Error != "" {
			return fmt.Errorf("preserve parallel work: %s", commit.Error)
		}
		result := watchdog.Run("git", []string{"-c", "user.name=Flywheel", "-c", "user.email=flywheel@localhost", "merge", "--no-edit", head}, watchdog.Opts{Context: c.RunContext, Cwd: ps.Worktree.Path, Env: childenv.EnvSlice(childenv.GateEnv(c.EnvLookup, nil)), WallTimeoutMs: 30000, IdleTimeoutMs: 30000})
		if result.ExitCode == nil || *result.ExitCode != 0 || result.Cancelled || result.TimedOut || result.Truncated || result.Signal != "" || result.CleanupError != "" {
			return fmt.Errorf("parallel integration conflict or incomplete merge; worktree retained: %s", strings.TrimSpace(result.Stderr))
		}
		ps.EvidenceHistory = append(ps.EvidenceHistory, map[string]any{"at": c.nowISO(), "action": "parallel-integration", "evidence": ps.Evidence})
		ps.Evidence.L1, ps.Evidence.Review, ps.Evidence.FinalGate, ps.Evidence.DefectInjection = nil, nil, nil, nil
		ps.BaseRef = &head
		c.save()
		if !c.l1Step(packet).Pass {
			return fmt.Errorf("parallel integrated candidate failed acceptance")
		}
	}
	return c.err()
}

// Keep a hard-budget slot through accounting, not merely through process exit.
func (lane *parallelLane) finishTurn() {
	if lane.release != nil {
		lane.release()
		lane.release = nil
	}
}

// Recover bounded execution failures before integrating a green sibling. Plan
// revisions can then synchronize contract-only checkpoints to every worktree.
func (c *Context) recoverParallelFailures(batch []*contract.Packet, pauseBefore string) (bool, error) {
	for _, previous := range batch {
		packet := c.Packets[previous.PacketID]
		if packet == nil {
			continue
		}
		ps := c.State.Packets[packet.PacketID]
		if ps.Status == "l1-passed" && c.doneReportValid(packet) && c.currentL1Evidence(packet) {
			continue
		}
		outcome, err := c.processPacket(packet, processOpts{PauseBefore: "review"})
		if err != nil {
			return true, err
		}
		if outcome != "pause" || pauseBefore == "review" {
			return true, nil
		}
		c.State.Status = "running"
		c.save()
	}
	return false, c.err()
}
