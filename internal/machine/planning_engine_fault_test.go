package machine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func briefEngineContext(t *testing.T) (fixture, *Context, map[string]int) {
	t.Helper()
	fx, opts, calls := briefEngineFixture(t, false)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: opts.Drivers})
	require.NoError(t, err)
	require.NoError(t, c.checkPlanningExports(true))
	return fx, c, calls
}

func pendingDesignRequest(t *testing.T, c *Context) int {
	t.Helper()
	i, err := c.newPlanningRequest("design", "", "test real dispatch boundary", false)
	require.NoError(t, err)
	return i
}

func TestPlanningRequestsReuseLimitsAndRejectInvalidScopes(t *testing.T) {
	_, c, _ := briefEngineContext(t)
	i := pendingDesignRequest(t, c)
	require.NoError(t, planning.BeginRound(c.State.Planning, c.State.Planning.Requests[i].ID))
	index, err := c.newPlanningRequest("design", "", "resume", false)
	require.NoError(t, err)
	require.Equal(t, i, index)
	require.Equal(t, 1, c.State.Planning.Requests[index].Rounds)
	require.Equal(t, "test real dispatch boundary", c.State.Planning.Requests[index].Feedback,
		"resuming a request retains the original scope-local planning instructions")
	_, err = c.newPlanningRequest("packet", "", "invalid missing packet identity", true)
	require.Error(t, err)
	require.Len(t, c.State.Planning.Requests, 1)
}

func TestPlanningResultNeverPassesProcessFailure(t *testing.T) {
	r := drivers.MockResult(func(r *drivers.Result) { r.Cancelled = true })
	require.NotEmpty(t, planningResultCode(r))
	r = drivers.MockResult(func(r *drivers.Result) { r.ErrorCode = errs.AUTH })
	require.Equal(t, errs.AUTH, planningResultCode(r))
}

func TestPlanningBindingRetainsExplicitFallbackAndLogicalRole(t *testing.T) {
	_, c, _ := briefEngineContext(t)
	r := planning.Request{Role: "design-plan"}
	b, err := c.planningBinding("design-plan", &r)
	require.NoError(t, err)
	require.Equal(t, b.Channel, r.Channel)
	r.Channel = "channel-without-an-explicit-role-model"
	_, err = c.planningBinding("design-plan", &r)
	require.ErrorContains(t, err, "no explicit")
	delete(c.Config.Loops, "plan")
	_, err = c.planningBinding("design-plan", &r)
	require.ErrorContains(t, err, "no loop binding")
}

func TestPlanningDispatchPrerequisiteFailuresDoNotCallTheProvider(t *testing.T) {
	for name, edit := range map[string]func(*testing.T, fixture, *Context){
		"missing binding": func(t *testing.T, fx fixture, c *Context) { delete(c.Config.Loops, "plan") },
		"missing real credentials": func(t *testing.T, fx fixture, c *Context) {
			c.Config.Drivers["plan"] = "real"
			b := c.Config.Loops["plan"]
			b.Channel = "guarded"
			c.Config.Loops["plan"] = b
			key := "FLYWHEEL_TEST_UNAVAILABLE_KEY"
			c.Config.Channels["guarded"] = config.Channel{EnvKey: &key}
			c.EnvLookup = func(string) (string, bool) { return "", false }
		},
		"hard budget": func(t *testing.T, fx fixture, c *Context) {
			limit := int64(1)
			c.Config.Budget.Enforce = "hard"
			c.Config.Budget.DailyTokenLimit = &limit
			state.RecordUsage(c.State, state.Usage{Input: 10}, state.UsageOpts{Day: c.Now().UTC().Format("2006-01-02"), Provider: "test"})
		},
		"missing driver": func(t *testing.T, fx fixture, c *Context) { c.Drivers.DesignPlan = nil; c.Drivers.Plan = nil },
		"stale CAS":      func(t *testing.T, fx fixture, c *Context) { c.revision-- },
		"changed export": func(t *testing.T, fx fixture, c *Context) {
			writeFile(t, filepath.Join(fx.taskDir, "planning", "brief.json"), "external change")
		},
		"unowned native outcome": func(t *testing.T, fx fixture, c *Context) {
			c.State.Sessions.Records["design-plan/planning:other"] = &state.SessionRecord{Role: "design-plan", Scope: "planning:other", Last: &state.SessionInvocation{ID: "unknown", Status: "blocked"}}
		},
		"changed input": func(t *testing.T, fx fixture, c *Context) {
			require.NoError(t, planning.ApplyFeedback(c.State.Planning, planning.Feedback{RequestID: "test-input", Source: "control:test-input", Text: "Revise the recovery design before continuing.", At: c.nowISO()}))
		},
	} {
		t.Run(name, func(t *testing.T) {
			fx, c, calls := briefEngineContext(t)
			i := pendingDesignRequest(t, c)
			require.NoError(t, planning.BeginRound(c.State.Planning, c.State.Planning.Requests[i].ID))
			edit(t, fx, c)
			_, _, _, err := c.invokePlanning(&c.State.Planning.Requests[i], false)
			require.Error(t, err)
			require.Empty(t, calls)
		})
	}
}

func TestPlanningAccountingFailureAndCancellationCannotAdoptAProposal(t *testing.T) {
	for _, cancelCall := range []bool{false, true} {
		t.Run(map[bool]string{false: "metrics disk failure", true: "context cancelled after response"}[cancelCall], func(t *testing.T) {
			fx, c, _ := briefEngineContext(t)
			i := pendingDesignRequest(t, c)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.RunContext = ctx
			old := c.Drivers.DesignPlan.(*fnDriver).fn
			c.Drivers.DesignPlan.(*fnDriver).fn = func(a drivers.CallArgs) drivers.Result {
				if cancelCall {
					cancel()
				} else {
					require.NoError(t, os.Mkdir(filepath.Join(fx.taskDir, "metrics.jsonl"), 0700))
				}
				return old(a)
			}
			_, err := c.processPlanningRequest(i)
			require.Error(t, err)
			require.Empty(t, c.State.Planning.Designs)
			require.Greater(t, c.State.Budget.Totals.Tokens, int64(0), "already returned usage remains recorded in memory for recovery")
		})
	}
}

func TestPlanningConfirmedCancellationStopsWithoutAutomaticRetry(t *testing.T) {
	_, c, _ := briefEngineContext(t)
	i := pendingDesignRequest(t, c)
	c.Drivers.DesignPlan.(*fnDriver).fn = func(a drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) { r.Cancelled = true })
	}
	ok, err := c.processPlanningRequest(i)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, "cancelled", c.State.Planning.Requests[i].Status)
	require.Zero(t, c.State.Planning.Requests[i].Retries)
	require.Empty(t, c.State.Planning.Designs)
}

func TestPlanningArtifactExportFailureRecoversWithoutRepeatingDesign(t *testing.T) {
	fx, c, calls := briefEngineContext(t)
	i := pendingDesignRequest(t, c)
	oldWrite := writePlanningBytes
	t.Cleanup(func() { writePlanningBytes = oldWrite })
	writePlanningBytes = func(root *os.Root, name string, raw []byte) error {
		if strings.HasPrefix(name, "design-") {
			return os.ErrPermission
		}
		return oldWrite(root, name, raw)
	}
	ok, err := c.processPlanningRequest(i)
	require.ErrorIs(t, err, os.ErrPermission)
	require.False(t, ok)
	require.Len(t, loadFixtureState(t, fx).Planning.Designs, 1)
	require.Zero(t, calls["design-review"])
	writePlanningBytes = oldWrite
	ready, err := c.prepareReviewedExecution()
	require.NoError(t, err)
	require.False(t, ready)
	require.Equal(t, 1, calls["design-plan"])
	require.Equal(t, 1, calls["design-review"])
	require.FileExists(t, filepath.Join(fx.taskDir, "planning", "design-000001.md"))
}

func TestPlanningQueueFailureAtDispatchAndConsumptionBoundaries(t *testing.T) {
	oldQueue := applyQueuedPlanning
	t.Cleanup(func() { applyQueuedPlanning = oldQueue })
	for _, at := range []string{"before dispatch", "after proposal consumed", "during retry"} {
		t.Run(at, func(t *testing.T) {
			_, c, calls := briefEngineContext(t)
			i := pendingDesignRequest(t, c)
			count := 0
			applyQueuedPlanning = func(c *Context) error {
				count++
				if at == "before dispatch" || count == 2 {
					return os.ErrPermission
				}
				return oldQueue(c)
			}
			if at == "during retry" {
				c.Drivers.DesignPlan.(*fnDriver).fn = func(drivers.CallArgs) drivers.Result {
					return drivers.MockResult(func(r *drivers.Result) { r.Text = "not JSON" })
				}
			}
			ok, err := c.processPlanningRequest(i)
			if at == "before dispatch" {
				require.NoError(t, err)
				require.Equal(t, "blocked", c.State.Planning.Phase)
				require.Empty(t, calls)
			} else {
				require.ErrorIs(t, err, os.ErrPermission)
			}
			require.False(t, ok)
			require.Zero(t, calls["design-review"])
			applyQueuedPlanning = oldQueue
		})
	}
}

func TestPlanningProcessPreservesExistingStopAndInputChanges(t *testing.T) {
	for name, edit := range map[string]func(*Context, int){
		"existing fatal":  func(c *Context, i int) { c.fatal = os.ErrPermission },
		"planning paused": func(c *Context, i int) { c.planningPause = true },
		"stale input": func(c *Context, i int) {
			require.NoError(t, planning.ApplyFeedback(c.State.Planning, planning.Feedback{RequestID: "test-input", Source: "control:test-input", Text: "Revise the recovery design before continuing.", At: c.nowISO()}))
		},
		"child without approval": func(c *Context, i int) {
			c.State.Planning.Requests[i].Scope = "milestone"
			c.State.Planning.Requests[i].ScopeID = "M-01"
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, c, calls := briefEngineContext(t)
			i := pendingDesignRequest(t, c)
			edit(c, i)
			_, err := c.processPlanningRequest(i)
			if name == "existing fatal" {
				require.ErrorIs(t, err, os.ErrPermission)
			} else if name == "child without approval" {
				require.Error(t, err, "rewriting a saved request identity must fail CAS")
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, calls)
		})
	}
}

func TestPlanningProviderFailureRetainsDiagnosticAndSpentRetries(t *testing.T) {
	_, c, _ := briefEngineContext(t)
	i := pendingDesignRequest(t, c)
	c.Drivers.DesignPlan.(*fnDriver).fn = func(a drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) {
			code := 1
			r.ExitCode = &code
			r.ErrorCode = errs.AUTH
			msg := "configured account is not authorized"
			r.ErrorMessage = &msg
		})
	}
	ok, err := c.processPlanningRequest(i)
	require.NoError(t, err)
	require.False(t, ok)
	require.Contains(t, c.State.Planning.Reason, "configured account")
	require.Equal(t, c.Config.Retry.MaxTransportRetries+planValidationRetries+1, c.State.Planning.Requests[i].Retries)
}

func TestPlanningPreparePropagatesStructuralFailureWithoutDispatch(t *testing.T) {
	for name, edit := range map[string]func(*testing.T, fixture, *Context){
		"human control": func(t *testing.T, fx fixture, c *Context) { c.State.Handoff = &state.Handoff{} },
		"damaged export": func(t *testing.T, fx fixture, c *Context) {
			writeFile(t, filepath.Join(fx.taskDir, "planning", "brief.json"), "untrusted")
		},
		"initial CAS failure": func(t *testing.T, fx fixture, c *Context) { c.revision-- },
		"queue filesystem failure": func(t *testing.T, fx fixture, c *Context) {
			writeFile(t, filepath.Join(fx.taskDir, ".control", "planning", "feedback-broken.json"), "not json")
		},
	} {
		t.Run(name, func(t *testing.T) {
			fx, c, calls := briefEngineContext(t)
			edit(t, fx, c)
			ready, err := c.prepareReviewedExecution()
			require.Error(t, err)
			require.False(t, ready)
			require.Empty(t, calls)
		})
	}
}

func TestPlanningHardBudgetStopsBeforeAnyCall(t *testing.T) {
	_, c, calls := briefEngineContext(t)
	limit := int64(1)
	c.Config.Budget.DailyTokenLimit = &limit
	c.Config.Budget.Enforce = "hard"
	state.RecordUsage(c.State, state.Usage{Input: 10}, state.UsageOpts{Day: c.Now().UTC().Format("2006-01-02"), Provider: "test"})
	_, err := c.prepareReviewedExecution()
	require.NoError(t, err)
	require.Equal(t, "budget-stopped", c.State.Planning.Phase)
	require.Empty(t, calls)
}

func TestPlanningRetryCannotIgnoreCASOrNewControlInput(t *testing.T) {
	oldQueue := applyQueuedPlanning
	t.Cleanup(func() { applyQueuedPlanning = oldQueue })
	for _, kind := range []string{"CAS", "pause", "changed input"} {
		t.Run(kind, func(t *testing.T) {
			_, c, _ := briefEngineContext(t)
			i := pendingDesignRequest(t, c)
			if kind == "CAS" {
				c.revision--
			}
			applyQueuedPlanning = func(c *Context) error {
				if kind == "pause" {
					c.planningPause = true
				}
				if kind == "changed input" {
					require.NoError(t, planning.ApplyFeedback(c.State.Planning, planning.Feedback{RequestID: "test-input", Source: "control:test-input", Text: "Revise the recovery design before continuing.", At: c.nowISO()}))
				}
				return nil
			}
			more, err := c.retryPlanning(&c.State.Planning.Requests[i], "design-plan", errs.SERVER, "confirmed server failure")
			require.False(t, more)
			if kind == "CAS" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 1, c.State.Planning.Requests[i].Retries)
			applyQueuedPlanning = oldQueue
		})
	}
}

func TestPlanningDiagnosticsRemainPersistableWhenProviderMessagesAreLarge(t *testing.T) {
	fx, c, _ := briefEngineContext(t)
	i := pendingDesignRequest(t, c)
	_, err := c.planningRequestStop(&c.State.Planning.Requests[i], "blocked", strings.Repeat("上游诊断", 5000))
	require.NoError(t, err)
	st := loadFixtureState(t, fx)
	require.LessOrEqual(t, len([]rune(st.Planning.Reason)), 8192)
	require.Contains(t, st.Planning.Reason, "[truncated]")
}

func TestPlanningReviewerStopsForUserDecisionOrBlockedEvidence(t *testing.T) {
	for _, verdict := range []string{"needs-user-decision", "blocked"} {
		t.Run(verdict, func(t *testing.T) {
			_, c, calls := briefEngineContext(t)
			i := pendingDesignRequest(t, c)
			c.Drivers.DesignReview.(*fnDriver).fn = func(drivers.CallArgs) drivers.Result {
				return planningJSONResult(t, map[string]any{"verdict": verdict, "summary": "The recovery policy requires an explicit product decision.", "findings": []any{}})
			}
			ok, err := c.processPlanningRequest(i)
			require.NoError(t, err)
			require.False(t, ok)
			require.Equal(t, 1, calls["design-plan"])
			require.Empty(t, c.State.Planning.Approvals)
			require.NotEqual(t, "running", c.State.Planning.Requests[i].Status)
		})
	}
}

func TestPlanningQueuedDispatchStopsAtTheBoundary(t *testing.T) {
	oldQueue := applyQueuedPlanning
	t.Cleanup(func() { applyQueuedPlanning = oldQueue })
	for _, failAdoption := range []bool{false, true} {
		t.Run(map[bool]string{false: "queued control pauses", true: "queue adoption filesystem failure"}[failAdoption], func(t *testing.T) {
			_, c, calls := briefEngineContext(t)
			i := pendingDesignRequest(t, c)
			count := 0
			applyQueuedPlanning = func(c *Context) error {
				count++
				if count == 1 {
					return errPlanningQueued
				}
				if failAdoption {
					return os.ErrPermission
				}
				return nil
			}
			ok, err := c.processPlanningRequest(i)
			require.False(t, ok)
			if failAdoption {
				require.ErrorIs(t, err, os.ErrPermission)
			} else {
				require.NoError(t, err)
				require.Equal(t, "paused", c.State.Status)
			}
			require.Empty(t, calls)
			applyQueuedPlanning = oldQueue
		})
	}
}

func TestPlanningPrepareConsumesQueuedFeedbackBeforeAnotherDesign(t *testing.T) {
	fx, c, calls := briefEngineContext(t)
	i := pendingDesignRequest(t, c)
	release, err := AcquireRunLease(fx.taskDir)
	require.NoError(t, err)
	st := loadFixtureState(t, fx)
	_, receipt, err := ControlPlanning(RunOpts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config}, PlanningCommand{Action: "feedback", Revision: st.Revision, RequestID: "prepare-boundary", Text: "Wait for discussion before making this decision."}, "")
	release()
	require.NoError(t, err)
	require.Equal(t, "queued", receipt.Status)
	ready, err := c.prepareReviewedExecution()
	require.NoError(t, err)
	require.False(t, ready)
	require.Empty(t, calls)
	require.Len(t, c.State.Planning.Feedback, 1)
	// A subsequent explicit continuation adopts a new input/request and
	// retains the old request's spent rounds and cancellation evidence.
	c.planningPause = false
	_, err = c.prepareReviewedExecution()
	require.NoError(t, err)
	require.Equal(t, "cancelled", c.State.Planning.Requests[i].Status)
	require.Equal(t, 1, calls["design-plan"])
}

func TestPlanningPrepareCannotRestartSpentInvalidOutputRequest(t *testing.T) {
	_, c, _ := briefEngineContext(t)
	c.Drivers.DesignPlan.(*fnDriver).fn = func(drivers.CallArgs) drivers.Result {
		return drivers.MockResult(func(r *drivers.Result) { r.Text = "not JSON" })
	}
	_, err := c.prepareReviewedExecution()
	require.NoError(t, err)
	before := c.State.Budget.Totals
	_, err = c.prepareReviewedExecution()
	require.NoError(t, err)
	require.Equal(t, before, c.State.Budget.Totals)
	require.Equal(t, "blocked", c.State.Planning.Phase)
}

func TestPlanningPrepareRejectsUnknownReceiptAndInterruptedProjection(t *testing.T) {
	t.Run("unknown original call", func(t *testing.T) {
		_, c, calls := briefEngineContext(t)
		c.State.Sessions.Records["design-plan/planning:original"] = &state.SessionRecord{Role: "design-plan", Scope: "planning:original", Last: &state.SessionInvocation{ID: "unknown", Status: "blocked"}}
		ready, err := c.prepareReviewedExecution()
		require.Error(t, err)
		require.False(t, ready)
		require.Empty(t, calls)
	})
	t.Run("interrupted projection", func(t *testing.T) {
		fx, c := newReviewedFixture(t)
		old := writePlanningBytes
		t.Cleanup(func() { writePlanningBytes = old })
		writePlanningBytes = func(root *os.Root, name string, raw []byte) error {
			if name == "spec.md" {
				return os.ErrPermission
			}
			return old(root, name, raw)
		}
		require.Error(t, c.activatePlanningPlans())
		writePlanningBytes = old
		writeFile(t, filepath.Join(fx.taskDir, "spec.md"), "human edit preserved during recovery")
		ready, err := c.prepareReviewedExecution()
		require.Error(t, err)
		require.False(t, ready)
		require.Equal(t, "human edit preserved during recovery", string(mustReadFile(t, filepath.Join(fx.taskDir, "spec.md"))))
	})
}

func TestPlanningPrepareRestoresAndChecksExistingChildActivation(t *testing.T) {
	for _, tamper := range []bool{false, true} {
		t.Run(map[bool]string{false: "reviewed child activates", true: "dependency change blocks activation"}[tamper], func(t *testing.T) {
			fx, c := newReviewedFixture(t)
			if tamper {
				writeFile(t, filepath.Join(fx.root, "external-change.txt"), "new dependency")
			}
			ready, err := c.prepareReviewedExecution()
			if tamper {
				require.ErrorContains(t, err, "dependencies changed")
				require.False(t, ready)
				require.Nil(t, c.State.Planning.Active)
			} else {
				require.NoError(t, err)
				require.True(t, ready)
				require.NotNil(t, c.State.Planning.Active)
			}
		})
	}
}

func approvedRootContext(t *testing.T) (fixture, *Context, map[string]int) {
	t.Helper()
	fx, opts, calls := briefEngineFixture(t, false)
	_, err := RunTask(opts)
	require.NoError(t, err)
	approveEngineDesign(t, fx, true, "start-current-root")
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: opts.Drivers})
	require.NoError(t, err)
	return fx, c, calls
}

func TestPlanningFirstChildStopsOnCASReviewAndActivationFailures(t *testing.T) {
	for _, failure := range []string{"CAS", "review", "projection"} {
		t.Run(failure, func(t *testing.T) {
			_, c, _ := approvedRootContext(t)
			old := writePlanningBytes
			t.Cleanup(func() { writePlanningBytes = old })
			if failure == "CAS" {
				c.revision--
			}
			if failure == "review" {
				c.Drivers.PlanReview.(*fnDriver).fn = func(drivers.CallArgs) drivers.Result {
					return planningJSONResult(t, map[string]any{"verdict": "blocked", "scope-verdict": "unclear", "summary": "Implementation scope requires further evidence.", "findings": []any{}})
				}
			}
			if failure == "projection" {
				writePlanningBytes = func(root *os.Root, name string, raw []byte) error {
					if name == "spec.md" {
						return os.ErrPermission
					}
					return old(root, name, raw)
				}
			}
			ready, err := c.prepareReviewedExecution()
			if failure == "review" {
				require.NoError(t, err)
				require.Equal(t, "blocked", c.State.Planning.Phase)
			} else {
				require.Error(t, err)
			}
			require.False(t, ready)
			require.Nil(t, c.State.Planning.Active)
		})
	}
}

func TestPlanningFinalLoadAndAdmissionCatchConcurrentFileDamage(t *testing.T) {
	for _, damage := range []string{"contract load", "immutable source"} {
		t.Run(damage, func(t *testing.T) {
			fx, c := newReviewedFixture(t)
			old := readContractRevision
			t.Cleanup(func() { readContractRevision = old })
			readContractRevision = func(path string) ([]byte, error) {
				if c.State.Planning.Active != nil {
					if damage == "contract load" {
						return nil, os.ErrPermission
					}
					writeFile(t, filepath.Join(fx.taskDir, "planning", "brief.json"), "changed after projection validation")
				}
				return old(path)
			}
			ready, err := c.prepareReviewedExecution()
			require.Error(t, err)
			require.False(t, ready)
			require.NoDirExists(t, filepath.Join(fx.root, "build"))
		})
	}
}

func TestPlanningPrepareRejectsQueueFailureAfterReviewAdoption(t *testing.T) {
	_, c, calls := briefEngineContext(t)
	old := applyQueuedPlanning
	t.Cleanup(func() { applyQueuedPlanning = old })
	afterReview := 0
	applyQueuedPlanning = func(c *Context) error {
		if len(c.State.Planning.Reviews) > 0 {
			afterReview++
			if afterReview == 2 {
				return os.ErrPermission
			}
		}
		return old(c)
	}
	ready, err := c.prepareReviewedExecution()
	require.ErrorIs(t, err, os.ErrPermission)
	require.False(t, ready)
	require.Equal(t, 1, calls["design-review"])
	require.Empty(t, c.State.Planning.Approvals)
}

func TestPlanningDispatchRequiresSettledInvestigationAndNativeAdmission(t *testing.T) {
	for _, failure := range []string{"investigation", "native profile"} {
		t.Run(failure, func(t *testing.T) {
			fx, c, calls := briefEngineContext(t)
			i := pendingDesignRequest(t, c)
			if failure == "investigation" {
				writeFile(t, state.PreparationFile(fx.taskDir), "not a supported report")
			} else {
				b := c.Config.Loops["plan"]
				b.NativeProfile = "missing-native-profile"
				c.Config.Loops["plan"] = b
			}
			_, _, _, err := c.invokePlanning(&c.State.Planning.Requests[i], false)
			require.Error(t, err)
			require.Empty(t, calls)
		})
	}
}
