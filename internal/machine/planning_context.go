package machine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
)

var errPlanningQueued = errors.New("planning input is queued; reconcile the previous invocation and review the adopted input before dispatch")

// Planning state is loaded before projected contracts: an interrupted activation
// must remain recoverable even when only half of its files reached disk.
func loadReviewedState(taskDir string) (*state.State, error) {
	raw, err := os.ReadFile(state.StateFile(taskDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var probe struct {
		Planning json.RawMessage `json:"planning"`
	}
	if json.Unmarshal(raw, &probe) != nil || len(probe.Planning) == 0 || string(probe.Planning) == "null" {
		return nil, nil // preserve the legacy loader and its diagnostics
	}
	return state.Load(taskDir)
}

func newPlanningContext(opts Opts, st *state.State) (*Context, error) {
	taskDir, err := canonicalDirectory(opts.TaskDir)
	if err != nil {
		return nil, err
	}
	cwd, err := canonicalDirectory(ProjectRoot(opts.Cwd))
	if err != nil {
		return nil, err
	}
	if st.Project == nil || st.Project.Cwd != cwd {
		return nil, fmt.Errorf("planning workspace differs from the recorded task workspace")
	}
	c := &Context{planningMode: opts.Mode, TaskDir: taskDir, Cwd: cwd, WorkspaceCwd: WorkspaceRoot(cwd), Config: opts.Config,
		State: st, Drivers: opts.Drivers, Log: opts.Log, Sleep: opts.Sleep, Now: opts.Now, EnvLookup: opts.Env,
		Packets: map[string]*contract.Packet{}, PacketList: []*contract.Packet{},
		RunContext: opts.Context, metricsFile: filepath.Join(taskDir, "metrics.jsonl"), revision: st.Revision}
	if c.Drivers == nil {
		c.Drivers = drivers.CreateDrivers(c.Config)
	}
	if c.Log == nil {
		c.Log = func(string) {}
	}
	if c.Sleep == nil {
		if opts.Context == nil {
			c.Sleep = time.Sleep
		} else {
			c.Sleep = func(d time.Duration) {
				timer := time.NewTimer(d)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-opts.Context.Done():
				}
			}
		}
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.EnvLookup == nil {
		c.EnvLookup = childenv.OSLookupEnv
	}
	return c, nil
}

func (c *Context) loadPlanningContracts() error {
	spec, err := contract.LoadSpec(c.TaskDir)
	if err != nil {
		return err
	}
	if spec.PlanningRef == nil {
		return fmt.Errorf("reviewed task is missing its planning-ref downgrade fence")
	}
	packets, err := contract.LoadPackets(c.TaskDir, spec)
	if err != nil {
		return err
	}
	c.Spec, c.PacketList, c.Packets = spec, packets, map[string]*contract.Packet{}
	for _, p := range packets {
		c.Packets[p.PacketID] = p
	}
	_ = state.EnsurePacketStates(c.State, packets) // IDs were checked by the contract loader.
	if err := c.attachPreparation(); err != nil {
		return err
	}
	if err := c.initializeWorkflow(firstNonEmpty(c.planningMode, c.State.Planning.Brief.Mode)); err != nil {
		return err
	}
	return c.reconcileContractRevisions()
}

func planningRole(role string) bool {
	return role == "design-plan" || role == "design-review" || role == "plan-review"
}

// Release only withdraws human control; it never verifies work or dispatches a
// model. Keep that recovery available if manual work damaged the projection,
// while every unknown call, mismatched handoff and productive action stays gated.
func (c *Context) canReleaseReviewedHandoff(only string, request *SessionCommand) bool {
	if c.State.Planning == nil || only != "session" || request == nil || request.Action != "release" || c.State.Handoff == nil || c.State.Sessions == nil {
		return false
	}
	h := c.State.Handoff
	r := c.State.Sessions.Records[request.Key]
	return r != nil && h.Key == request.Key && h.Thread == r.ThreadID && !h.Pending && r.Last.Status == "consumed" && r.Last.CleanupConfirmed
}

func (c *Context) planningGuard() (string, error) {
	candidate, err := c.candidateFingerprint(c.Cwd)
	if err != nil {
		return "", err
	}
	// Only Go writes immutable planning exports. They are not trusted merely
	// because ordinary candidate accounting classifies them as ledger files.
	if err := c.verifyPlanningArtifacts(); err != nil {
		return "", err
	}
	return sessionHash([]any{candidate, c.State.Planning.Brief, c.State.Planning.InputRevision,
		c.State.Planning.CurrentDesign, c.State.Planning.CurrentPlans}), nil
}

// The same check is used by normal, manual, parallel and recovery dispatch.
func (c *Context) admitReviewedExecution() error {
	if c.State.Planning == nil {
		return nil
	}
	if err := c.applyPlanningQueue(); err != nil {
		return err
	}
	if c.Spec == nil || c.Spec.PlanningRef == nil {
		return fmt.Errorf("design and execution plan must be reviewed, authorized and activated first")
	}
	if err := planning.Admit(c.State.Planning, *c.Spec.PlanningRef); err != nil {
		return err
	}
	if err := c.verifyPlanningArtifacts(); err != nil {
		return err
	}
	return c.verifyPlanningProjection(c.State.Planning.Active)
}

// Received invocations may be reconciled against their original input before
// queued feedback is adopted. Fresh verification, delivery and queue admission
// must wait for that boundary; they cannot extend the old authorization.
func (c *Context) admitReviewedBoundary() error {
	if err := c.admitReviewedExecution(); err != nil {
		return err
	}
	if c.State.Planning != nil {
		pending, err := c.planningQueuePending()
		if err != nil {
			return err
		}
		if pending {
			return fmt.Errorf("queued planning feedback must be resolved before starting another implementation stage")
		}
	}
	return nil
}

func (c *Context) pausePlanning(reason string) (bool, error) {
	c.State.Status = "paused"
	if c.State.Planning.Phase == "budget-stopped" {
		c.State.Status = "budget-stopped"
	}
	c.State.Planning.Reason = planningReason(reason)
	c.save()
	return false, c.err()
}
