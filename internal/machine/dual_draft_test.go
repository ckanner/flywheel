package machine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func dualDraftFixture(t *testing.T) (fixture, RunTaskOpts) {
	t.Helper()
	fx, opts, _ := briefEngineFixture(t, false)
	cfg := mustConfig(t, fx)
	cfg.Profiles = map[string]config.ModelProfile{
		"carbon":    {Harness: "antigravity", Model: "gemini-4-carbon-high", Channel: "kimi-subscription", Thinking: "high"},
		"opus-plan": {Harness: "claude-code-pro", Model: "claude-opus-5-5", Channel: "kimi-subscription", Thinking: "high"},
	}
	cfg.ProfileRefs = map[string]string{"loops.design-plan": "carbon"}
	cfg.Loops["design-plan"] = config.LoopBinding{Harness: "antigravity", Model: "gemini-4-carbon-high", Channel: "kimi-subscription", Thinking: "high"}
	cfg.Planning.DualDraft = &config.DualDraftConfig{
		Enabled:            true,
		SecondaryProfile:   "opus-plan",
		SynthesizerProfile: "opus-plan",
	}
	opts.Config = cfg
	opts.Drivers.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(cfg).DesignPlan, fn: func(drivers.CallArgs) drivers.Result {
		return planningJSONResult(t, engineDesignOutput(t))
	}}
	return fx, opts
}

func TestDualDraftDesignRunsBlindParallelDraftsPersistsCandidatesAndSynthesizes(t *testing.T) {
	fx, opts := dualDraftFixture(t)
	designOut := engineDesignOutput(t)
	var draftCalls atomic.Int32
	var synthCalls atomic.Int32
	opts.Drivers.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(opts.Config).DesignPlan, fn: func(a drivers.CallArgs) drivers.Result {
		if a.Model == "claude-opus-5-5" && len(a.Prompt) > 0 && containsAll(a.Prompt, "draft-a", "draft-b", "来源对照表") {
			synthCalls.Add(1)
			synth := designOut
			synth.Document += "\n\n## 来源对照表\n| 决策 | 来源 | 说明 |\n|---|---|---|\n| D-01 | draft-a | Adopted |"
			return planningJSONResult(t, synth)
		}
		draftCalls.Add(1)
		out := designOut
		out.Document = "# Blind draft by " + a.Model
		return planningJSONResult(t, out)
	}}

	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "awaiting-design-approval", summary.PlanningPhase)
	require.Equal(t, int32(2), draftCalls.Load())
	require.Equal(t, int32(1), synthCalls.Load())

	primaryMD, err := os.ReadFile(filepath.Join(fx.taskDir, "design-candidates", "rev-000001-primary.md"))
	require.NoError(t, err)
	require.Contains(t, string(primaryMD), "gemini-4-carbon-high")

	secondaryMD, err := os.ReadFile(filepath.Join(fx.taskDir, "design-candidates", "rev-000001-secondary.md"))
	require.NoError(t, err)
	require.Contains(t, string(secondaryMD), "claude-opus-5-5")

	metaRaw, err := os.ReadFile(filepath.Join(fx.taskDir, "design-candidates", "rev-000001-meta.json"))
	require.NoError(t, err)
	var meta dualDraftMeta
	require.NoError(t, json.Unmarshal(metaRaw, &meta))
	require.Equal(t, 1, meta.Revision)
	require.Equal(t, "carbon", meta.Primary.Profile)
	require.Equal(t, "opus-plan", meta.Secondary.Profile)
	require.Equal(t, "opus-plan", meta.Synthesizer.Profile)

	st := loadFixtureState(t, fx)
	require.GreaterOrEqual(t, st.Budget.Totals.Calls, int64(4))
}

func TestDualDraftDesignErrorAndGuardPaths(t *testing.T) {
	require.Equal(t, "plain text", extractDesignDraftMarkdown("plain text"))

	t.Run("missing profiles and missing real env", func(t *testing.T) {
		_, opts := dualDraftFixture(t)
		opts.Config.Planning.DualDraft.SecondaryProfile = "missing"
		summary, err := RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "blocked", summary.PlanningPhase)

		_, opts = dualDraftFixture(t)
		opts.Config.Planning.DualDraft.SynthesizerProfile = "missing"
		summary, err = RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "blocked", summary.PlanningPhase)

		_, opts = dualDraftFixture(t)
		opts.Config.Drivers["design-plan"] = "real"
		opts.Config.Channels["need-key"] = config.Channel{EnvKey: new("DUAL_DRAFT_MISSING_KEY")}
		opts.Config.Profiles["opus-plan"] = config.ModelProfile{Harness: "kimi", Model: "k", Channel: "need-key"}
		summary, err = RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "blocked", summary.PlanningPhase)
	})

	t.Run("primary and secondary failure paths", func(t *testing.T) {
		for _, failModel := range []string{"gemini-4-carbon-high", "claude-opus-5-5"} {
			t.Run("cancelled_"+failModel, func(t *testing.T) {
				_, opts := dualDraftFixture(t)
				opts.Drivers.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(opts.Config).DesignPlan, fn: func(a drivers.CallArgs) drivers.Result {
					if a.Model == failModel {
						return drivers.MockResult(func(r *drivers.Result) { r.Cancelled = true; r.Settled = false })
					}
					return planningJSONResult(t, engineDesignOutput(t))
				}}
				summary, err := RunTask(opts)
				require.NoError(t, err)
				require.Equal(t, "cancelled", summary.PlanningPhase)
			})
			t.Run("cleanup_error_"+failModel, func(t *testing.T) {
				_, opts := dualDraftFixture(t)
				opts.Drivers.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(opts.Config).DesignPlan, fn: func(a drivers.CallArgs) drivers.Result {
					if a.Model == failModel {
						return drivers.MockResult(func(r *drivers.Result) { r.CleanupError = "orphan process" })
					}
					return planningJSONResult(t, engineDesignOutput(t))
				}}
				_, err := RunTask(opts)
				require.ErrorContains(t, err, "orphan process")
			})
		}
	})

	t.Run("guard mutation during drafts or synthesis and persist errors", func(t *testing.T) {
		fx, opts := dualDraftFixture(t)
		opts.Drivers.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(opts.Config).DesignPlan, fn: func(a drivers.CallArgs) drivers.Result {
			if a.Model == "gemini-4-carbon-high" {
				writeFile(t, filepath.Join(fx.root, "dirty.txt"), "mutated")
			}
			return planningJSONResult(t, engineDesignOutput(t))
		}}
		summary, err := RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "blocked", summary.PlanningPhase)

		fx, opts = dualDraftFixture(t)
		opts.Drivers.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(opts.Config).DesignPlan, fn: func(a drivers.CallArgs) drivers.Result {
			if containsAll(a.Prompt, "draft-a") {
				writeFile(t, filepath.Join(fx.root, "dirty-synth.txt"), "mutated")
			}
			return planningJSONResult(t, engineDesignOutput(t))
		}}
		summary, err = RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "blocked", summary.PlanningPhase)

		fx, opts = dualDraftFixture(t)
		origMkdir := mkdirDualDraftDir
		mkdirDualDraftDir = func(string, os.FileMode) error { return errors.New("mkdir boom") }
		t.Cleanup(func() { mkdirDualDraftDir = origMkdir })
		summary, err = RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "blocked", summary.PlanningPhase)
		mkdirDualDraftDir = origMkdir

		fx, opts = dualDraftFixture(t)
		origWrite := writeDualDraftFile
		writeDualDraftFile = func(string, []byte, os.FileMode) error { return errors.New("write boom") }
		t.Cleanup(func() { writeDualDraftFile = origWrite })
		summary, err = RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "blocked", summary.PlanningPhase)
		writeDualDraftFile = origWrite
	})

	t.Run("synthesis cleanup, fatal, session, and invocation id", func(t *testing.T) {
		fx, opts := dualDraftFixture(t)
		opts.Drivers.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(opts.Config).DesignPlan, fn: func(a drivers.CallArgs) drivers.Result {
			if containsAll(a.Prompt, "draft-a") {
				return drivers.MockResult(func(r *drivers.Result) { r.CleanupError = "synth orphan" })
			}
			return planningJSONResult(t, engineDesignOutput(t))
		}}
		_, err := RunTask(opts)
		require.ErrorContains(t, err, "synth orphan")

		fx, opts = dualDraftFixture(t)
		opts.Drivers.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(opts.Config).DesignPlan, fn: func(a drivers.CallArgs) drivers.Result {
			res := planningJSONResult(t, engineDesignOutput(t))
			if containsAll(a.Prompt, "draft-a") {
				res.Session = &drivers.SessionOutcome{RequestID: "synth-req-1", Dispatch: "dispatched", Outcome: "completed", CleanupConfirmed: true}
			}
			return res
		}}
		summary, err := RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "awaiting-design-approval", summary.PlanningPhase)
		st := loadFixtureState(t, fx)
		require.Equal(t, "synth-req-1", st.Planning.Designs[0].PlannerInvocation)

		// Fatal metrics error after drafts and after synthesis.
		fx, opts = dualDraftFixture(t)
		c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: opts.Drivers, Log: func(string) {}})
		require.NoError(t, err)
		c.metricsFile = fx.taskDir // directory path causes AppendOnce error -> c.fatal != nil
		req := &planning.Request{ID: "r1", Scope: "design", InputRevision: 1, Rounds: 1}
		_, _, _, err = c.invokeDualDraftDesign(req, "design-plan", "planning:r1", "", mustDualBinding(t, c, "carbon"), c.Drivers.DesignPlan, "p", "guard")
		require.Error(t, err)

		// invokeScoped error when synthesis fails receipt persistence.
		fx, opts = dualDraftFixture(t)
		opts.Config.Sessions.Mode = "scoped"
		c, err = CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: opts.Drivers, Log: func(string) {}})
		require.NoError(t, err)
		require.NoError(t, c.checkPlanningExports(true))
		c.Drivers.DesignPlan = &fnDriver{role: "design-plan", base: c.Drivers.DesignPlan, fn: func(a drivers.CallArgs) drivers.Result {
			if containsAll(a.Prompt, "draft-a") {
				c.fatal = errors.New("synth fatal")
			}
			return planningJSONResult(t, engineDesignOutput(t))
		}}
		before, err := c.planningGuard()
		require.NoError(t, err)
		_, _, _, err = c.invokeDualDraftDesign(req, "design-plan", "planning:r1", "", mustDualBinding(t, c, "carbon"), c.Drivers.DesignPlan, "p", before)
		require.Error(t, err)

		// Synthesis invokeScoped direct error path (blocked session record).
		fx, opts = dualDraftFixture(t)
		opts.Config.Sessions.Mode = "scoped"
		c, err = CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: opts.Drivers, Log: func(string) {}})
		require.NoError(t, err)
		require.NoError(t, c.checkPlanningExports(true))
		c.State.Sessions.Records[sessionKey("design-plan", "planning:r1")] = &state.SessionRecord{
			Reason: "unconfirmed previous invocation",
			Last:   &state.SessionInvocation{ID: "call-blocked", Status: "blocked"},
		}
		before, err = c.planningGuard()
		require.NoError(t, err)
		_, _, _, err = c.invokeDualDraftDesign(req, "design-plan", "planning:r1", "", mustDualBinding(t, c, "carbon"), c.Drivers.DesignPlan, "p", before)
		require.Error(t, err)
	})
}

func containsAll(text string, subs ...string) bool {
	for _, s := range subs {
		if !strings.Contains(text, s) {
			return false
		}
	}
	return true
}

func mustDualBinding(t *testing.T, c *Context, profile string) fallback.Binding {
	t.Helper()
	b, err := c.dualDraftProfileBinding(profile)
	require.NoError(t, err)
	return b
}
