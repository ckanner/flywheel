package machine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func TestStage0ClarifyAndStage15UIPrototypeEndToEnd(t *testing.T) {
	fx := newGitFixture(t)
	taskDir := filepath.Join(fx.root, "docs", "pixel-ui")
	cfg := mustConfig(t, fx)
	cfg.Planning.Clarify = &config.ClarifyConfig{Mode: "always"}
	cfg.Planning.UIPrototype = &config.UIPrototypeConfig{Mode: "auto"}
	fx.taskDir = taskDir

	brief := planning.Brief{
		Version:         1,
		ID:              "pixel-ui",
		Goal:            "Build a pixel art Tetris web game",
		OriginalRequest: "Build a pixel art Tetris web game",
		Constraints:     []string{},
		Evidence:        []string{},
		Policy:          "review-first",
		Mode:            "full",
		ClarifyMode:     "always",
		UIPrototypeMode: "auto",
		QualityGates:    []string{"Engine coverage 100%"},
	}
	st, _, err := ControlPlanning(RunOpts{TaskDir: taskDir, Cwd: fx.root, Config: cfg}, PlanningCommand{
		Action: "prepare", RequestID: "prep-1", Brief: &brief,
	}, "")
	require.NoError(t, err)
	opts := runOpts(fx, drivers.CreateDrivers(cfg))
	opts.Config = cfg

	clarifyJSON := `{"needs-clarification":true,"is-frontend-ui":true,"summary":"Short brief needs style and gate confirmation","suggested-gates":["核心引擎单测覆盖率 100%","核心引擎单测覆盖率 100%","","引擎零 DOM 依赖"],"questions":[{"id":"Q-1","title":"选择视觉风格","kind":"choice","options":["复古像素风（推荐）","霓虹赛博风"],"recommended":"复古像素风（推荐）","rationale":"决定配色板","required":true},{"id":"bad id!","title":"其他要求","kind":"unknown","options":["默认"]},{"id":"Q-empty","title":"   ","kind":"choice"}]}`
	designOut := engineDesignOutput(t)
	designOut.Document = "# Pixel Tetris Design\nCanvas + Vite"
	protoJSON := `{"summary":"Pixel retro board mockup","design-tokens":{"bg":"#0b1020","accent":"#22c55e"},"states":["开始页","游戏中","暂停","结束"],"html":"<!DOCTYPE html><html><body><h1>Pixel Tetris</h1></body></html>"}`
	protoReviewJSON := `{"verdict":"pass","summary":"Clear visual hierarchy","findings":[]}`

	opts.Drivers.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(cfg).DesignPlan, fn: func(a drivers.CallArgs) drivers.Result {
		switch {
		case strings.Contains(a.Prompt, "Stage 0 需求澄清"):
			return drivers.Result{Text: clarifyJSON, Settled: true, ExitCode: new(0)}
		case strings.Contains(a.Prompt, "Stage 1.5 前端 UI/UX"):
			return drivers.Result{Text: protoJSON, Settled: true, ExitCode: new(0)}
		default:
			return planningJSONResult(t, designOut)
		}
	}}
	opts.Drivers.DesignReview = &fnDriver{role: "design-review", base: drivers.CreateDrivers(cfg).DesignReview, fn: func(a drivers.CallArgs) drivers.Result {
		if strings.Contains(a.Prompt, "独立 UI/UX 视觉与交互审查员") {
			return drivers.Result{Text: protoReviewJSON, Settled: true, ExitCode: new(0)}
		}
		return drivers.Result{Text: `{"verdict":"pass","summary":"Clean architecture","findings":[]}`, Settled: true, ExitCode: new(0)}
	}}

	// 1. Run stops at Stage 0 awaiting-clarification
	opts.PlanningOnly = true
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "paused", summary.Status)
	st = loadFixtureState(t, fx)
	require.Equal(t, "awaiting-clarification", st.Planning.Phase)
	require.NotNil(t, st.Planning.Clarification)
	require.Equal(t, "awaiting-answers", st.Planning.Clarification.Status)
	require.Len(t, st.Planning.Clarification.Questions, 2)

	// Re-running while awaiting-answers stays cleanly paused
	summary, err = RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "paused", summary.Status)
	st = loadFixtureState(t, fx)

	// 2. Submit clarification answers & locked measurable quality gates
	st, receipt, err := ControlPlanning(opts.RunOpts, PlanningCommand{
		Action:    "clarify",
		Revision:  st.Revision,
		RequestID: "clarify-ans-1",
		Clarify: &ClarifyAnswer{
			Answers:         map[string]string{"Q-1": "复古像素风（推荐）"},
			SelectedGates:   []string{"核心引擎单测覆盖率 100%"},
			FreeformNote:    "保持经典 7-bag",
			UIPrototypeMode: "auto",
		},
	}, "")
	require.NoError(t, err)
	require.Equal(t, "applied", receipt.Status)
	require.Equal(t, "confirmed", st.Planning.Clarification.Status)

	// Idempotent replay of exact same clarify request-id
	_, receipt2, err := ControlPlanning(opts.RunOpts, PlanningCommand{
		Action:    "clarify",
		Revision:  st.Revision,
		RequestID: "clarify-ans-1",
		Clarify:   &ClarifyAnswer{Answers: map[string]string{"Q-1": "复古像素风（推荐）"}},
	}, "")
	require.NoError(t, err)
	require.Equal(t, "unchanged", receipt2.Status)

	// 3. Run Stage 1 Design + Review -> awaiting-design-approval
	summary, err = RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "paused", summary.Status)
	st = loadFixtureState(t, fx)
	require.Equal(t, "awaiting-design-approval", st.Planning.Phase)
	require.NotNil(t, st.Planning.CurrentDesign)

	// 4. Human approves design with Start: true -> triggers Stage 1.5 UI/UX Prototype!
	startTrue := true
	st, _, err = ControlPlanning(opts.RunOpts, PlanningCommand{
		Action:    "approve",
		Revision:  st.Revision,
		RequestID: "approve-1",
		Design:    st.Planning.CurrentDesign,
		Start:     &startTrue,
	}, "dsh:sess:cmd1")
	require.NoError(t, err)

	opts.PlanningOnly = false
	summary, err = RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "paused", summary.Status)
	st = loadFixtureState(t, fx)
	require.Equal(t, "awaiting-prototype-approval", st.Planning.Phase)
	require.NotNil(t, st.Planning.UIPrototype)
	require.Equal(t, 1, st.Planning.UIPrototype.Revision)
	require.Equal(t, "ui-prototypes/ui-prototype-000001.html", st.Planning.UIPrototype.HTMLFile)
	require.NotNil(t, st.Planning.UIPrototype.Review)
	require.Equal(t, "pass", st.Planning.UIPrototype.Review.Verdict)
	rawHTML, err := os.ReadFile(filepath.Join(fx.taskDir, st.Planning.UIPrototype.HTMLFile))
	require.NoError(t, err)
	require.Contains(t, string(rawHTML), "Pixel Tetris")

	// Re-running while awaiting-prototype-approval stays paused
	summary, err = RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "paused", summary.Status)
	st = loadFixtureState(t, fx)

	// 5. Submit visual feedback -> generates Rev #2 prototype
	st, _, err = ControlPlanning(opts.RunOpts, PlanningCommand{
		Action:    "prototype-feedback",
		Revision:  st.Revision,
		RequestID: "proto-fb-1",
		Text:      "把配色调成经典 GameBoy 绿",
	}, "")
	require.NoError(t, err)
	require.Equal(t, "prototyping", st.Planning.Phase)

	summary, err = RunTask(opts)
	require.NoError(t, err)
	st = loadFixtureState(t, fx)
	require.Equal(t, 2, st.Planning.UIPrototype.Revision)
	require.Equal(t, "awaiting-approval", st.Planning.UIPrototype.Status)

	// 6. Approve prototype + idempotency check
	st, _, err = ControlPlanning(opts.RunOpts, PlanningCommand{
		Action:    "approve-prototype",
		Revision:  st.Revision,
		RequestID: "proto-approve-1",
	}, "")
	require.NoError(t, err)
	require.Equal(t, "approved", st.Planning.UIPrototype.Status)

	_, receipt3, err := ControlPlanning(opts.RunOpts, PlanningCommand{
		Action:    "approve-prototype",
		Revision:  st.Revision,
		RequestID: "proto-approve-1",
	}, "")
	require.NoError(t, err)
	require.Equal(t, "unchanged", receipt3.Status)
}

func TestStage0ClarifyJEVAutoSkip(t *testing.T) {
	server, _ := runtimeServer(t, func(choices map[string]string) string {
		return "skip-clarify"
	}, nil)

	fx, opts, _ := briefEngineFixture(t, false)
	opts.Config.Subscription = nil
	opts.Config.JEV = config.JEVConfig{
		Backend: "http", Enabled: true, Endpoint: server.URL + "/v1/systemone",
		Model: "jev-1.14.0", KeyEnv: "TEST_JEV_KEY", TimeoutMS: 2000, MaxCalls: 8,
		MinApplicability: 0.8, MinChoice: 0.6,
		AllowedData: []decision.Category{decision.TaskSummary, decision.DiagnosticFacts},
	}
	opts.Config.Planning.Clarify = &config.ClarifyConfig{Mode: "auto"}
	persistRuntimeConfig(t, &Context{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config})
	t.Setenv("TEST_JEV_KEY", "secret-test-key")
	opts.PlanningOnly = true

	_, err := RunTask(opts)
	require.NoError(t, err)
	st := loadFixtureState(t, fx)
	require.NotNil(t, st.Planning.Clarification)
	require.Equal(t, "skipped", st.Planning.Clarification.Status)
	require.Equal(t, "jev", st.Planning.Clarification.DecidedBy)
	require.Equal(t, "awaiting-design-approval", st.Planning.Phase)
}

func TestStage0AndPrototypeEdgeBranchesAndConvergenceDiagnostic(t *testing.T) {
	// Command validation errors
	require.Error(t, ValidatePlanningCommand(PlanningCommand{Action: "clarify", Revision: 1, RequestID: "r1"}))
	require.Error(t, ValidatePlanningCommand(PlanningCommand{Action: "clarify", Revision: 1, RequestID: "r1", Clarify: &ClarifyAnswer{UIPrototypeMode: "bad"}}))
	require.Error(t, ValidatePlanningCommand(PlanningCommand{Action: "prototype-feedback", Revision: 1, RequestID: "r1", Text: "   "}))

	// Apply errors on missing state
	fx, opts, _ := briefEngineFixture(t, false)
	st := loadFixtureState(t, fx)
	_, _, err := ControlPlanning(opts.RunOpts, PlanningCommand{Action: "approve-prototype", Revision: st.Revision, RequestID: "ap1"}, "")
	require.Error(t, err)
	_, _, err = ControlPlanning(opts.RunOpts, PlanningCommand{Action: "prototype-feedback", Revision: st.Revision, RequestID: "pf1", Text: "change color"}, "")
	require.Error(t, err)
	_, _, err = ControlPlanning(opts.RunOpts, PlanningCommand{Action: "skip-prototype", Revision: st.Revision, RequestID: "sp1"}, "")
	require.Error(t, err)

	// Clarify directly on nil Clarification creates confirmed clarification
	st, _, err = ControlPlanning(opts.RunOpts, PlanningCommand{
		Action: "clarify", Revision: st.Revision, RequestID: "cl-direct",
		Clarify: &ClarifyAnswer{Answers: map[string]string{"Q-1": "A"}, SelectedGates: []string{"100% cov"}},
	}, "")
	require.NoError(t, err)
	require.Equal(t, "confirmed", st.Planning.Clarification.Status)

	// Pure helpers
	require.Len(t, cleanStringSlice([]string{"a", "b", "c"}, 2), 2)
	manyQ := make([]planning.ClarificationQuestion, 10)
	for i := range manyQ {
		manyQ[i] = planning.ClarificationQuestion{Title: "Q", Kind: "choice"}
	}
	require.Len(t, sanitizeClarificationQuestions(manyQ), 8)
	require.False(t, inferFrontendUIBrief(planning.Brief{Goal: "SQL migration", OriginalRequest: "Add index"}))
	require.True(t, inferFrontendUIBrief(planning.Brief{Goal: "Canvas game", OriginalRequest: "Canvas"}))

	// Clarify failure fallthrough, invalid JSON fallthrough, and model needs-clarification=false
	for _, tc := range []struct {
		name string
		res  drivers.Result
	}{
		{"driver-error", drivers.Result{ExitCode: new(1), Settled: true}},
		{"bad-json", drivers.Result{Text: "not json", ExitCode: new(0), Settled: true}},
		{"model-skip", drivers.Result{Text: `{"needs-clarification":false,"is-frontend-ui":false,"summary":"clear","suggested-gates":[],"questions":[]}`, ExitCode: new(0), Settled: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subFx, subOpts, _ := briefEngineFixture(t, false)
			subOpts.Config.Profiles = map[string]config.ModelProfile{"cl-prof": {Harness: "antigravity", Channel: "kimi-subscription", Model: "gemini-4-carbon-high", Thinking: "high"}}
			subOpts.Config.Planning.Clarify = &config.ClarifyConfig{Mode: "auto", Profile: "cl-prof"}
			subOpts.Drivers.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(subOpts.Config).DesignPlan, fn: func(a drivers.CallArgs) drivers.Result {
				if strings.Contains(a.Prompt, "Stage 0 需求澄清") {
					return tc.res
				}
				return planningJSONResult(t, engineDesignOutput(t))
			}}
			subOpts.PlanningOnly = true
			_, err := RunTask(subOpts)
			require.NoError(t, err)
			subSt := loadFixtureState(t, subFx)
			require.Equal(t, "skipped", subSt.Planning.Clarification.Status)
		})
	}

	// Non-convergence diagnostic on MaxRounds reached with open blocker findings
	ncFx, ncOpts, _ := briefEngineFixture(t, false)
	ncOpts.Drivers.DesignReview = &fnDriver{role: "design-review", base: drivers.CreateDrivers(ncOpts.Config).DesignReview, fn: func(drivers.CallArgs) drivers.Result {
		return drivers.Result{
			Text:     `{"verdict":"revise","summary":"Still open","findings":[{"id":"D-1","severity":"blocker","target":"Auth","evidence":"Missing","impact":"High","status":"open"}]}`,
			ExitCode: new(0), Settled: true,
		}
	}}
	ncOpts.PlanningOnly = true
	_, err = RunTask(ncOpts)
	require.NoError(t, err)
	ncSt := loadFixtureState(t, ncFx)
	require.Equal(t, "budget-stopped", ncSt.Planning.Phase)
	require.Contains(t, ncSt.Planning.Reason, "收敛诊断")
	require.Contains(t, ncSt.Planning.Reason, "D-1(blocker)")

	// UIPrototype helpers & error branches
	ncCfg := ncOpts.Config
	ncCfg.Profiles = map[string]config.ModelProfile{"uxp": {Harness: "antigravity", Channel: "kimi-subscription", Model: "gemini-4-neon-high", Thinking: "high"}}
	ncCfg.Planning.UIPrototype = &config.UIPrototypeConfig{Mode: "auto", Profile: "uxp", ReviewProfile: "uxp"}
	cliPlan := &planning.State{Brief: planning.Brief{Goal: "SQL CLI", OriginalRequest: "Add flag"}}
	c := &Context{State: ncSt, Config: ncCfg, TaskDir: ncFx.taskDir, Cwd: ncFx.root, Drivers: &drivers.Drivers{}}
	require.Equal(t, "auto", c.effectiveUIPrototypeMode(cliPlan))
	require.True(t, c.shouldRunUIPrototype(cliPlan, &planning.Design{Document: "Uses react component"}))
	require.False(t, c.shouldRunUIPrototype(cliPlan, &planning.Design{Document: "Pure Go CLI"}))
	ncCfg.Planning.UIPrototype.Mode = "always"
	require.True(t, c.shouldRunUIPrototype(cliPlan, &planning.Design{Document: "Pure Go CLI"}))
	ncCfg.Planning.UIPrototype.Mode = "off"
	require.False(t, c.shouldRunUIPrototype(cliPlan, &planning.Design{Document: "Canvas"}))
	ncCfg.Planning.UIPrototype = nil
	require.Equal(t, "off", c.effectiveUIPrototypeMode(cliPlan))

	ncCfg.FrontendUXReview = config.FrontendUXReviewConfig{Enabled: true, Profile: "uxp"}
	b, err := c.uiPrototypeBinding(false)
	require.NoError(t, err)
	require.Equal(t, "gemini-4-neon-high", b.Model)
	ncCfg.FrontendUXReview.Enabled = false
	_, err = c.uiPrototypeBinding(true)
	require.NoError(t, err)
	delete(ncCfg.Loops, "design-plan")
	delete(ncCfg.Loops, "plan")
	_, err = c.clarifyBinding()
	require.Error(t, err)
	_, err = c.uiPrototypeBinding(false)
	require.Error(t, err)

	// skip-prototype on existing current design
	st2, _, err := ControlPlanning(ncOpts.RunOpts, PlanningCommand{
		Action: "skip-prototype", Revision: ncSt.Revision, RequestID: "skip-1",
	}, "")
	require.NoError(t, err)
	require.Equal(t, "skipped", st2.Planning.UIPrototype.Status)

	// Verify prototype failure & invalid HTML & review error paths
	pfFx, pfOpts, _ := briefEngineFixture(t, false)
	pfOpts.Config.Planning.UIPrototype = &config.UIPrototypeConfig{Mode: "always"}
	pfOpts.PlanningOnly = true
	_, err = RunTask(pfOpts)
	require.NoError(t, err)
	pfSt := loadFixtureState(t, pfFx)
	startTrue := true
	_, _, err = ControlPlanning(pfOpts.RunOpts, PlanningCommand{
		Action: "approve", Revision: pfSt.Revision, RequestID: "ap-pf", Design: pfSt.Planning.CurrentDesign, Start: &startTrue,
	}, "dsh:s:c")
	require.NoError(t, err)

	// Driver error in prototype generation pauses cleanly
	pfOpts.PlanningOnly = false
	pfOpts.Drivers.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(pfOpts.Config).DesignPlan, fn: func(drivers.CallArgs) drivers.Result {
		return drivers.Result{ExitCode: new(1), Settled: true}
	}}
	sum, err := RunTask(pfOpts)
	require.NoError(t, err)
	require.Equal(t, "paused", sum.Status)

	// Invalid HTML output pauses cleanly
	pfOpts.Drivers.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(pfOpts.Config).DesignPlan, fn: func(drivers.CallArgs) drivers.Result {
		return drivers.Result{Text: `{"summary":"bad","html":""}`, ExitCode: new(0), Settled: true}
	}}
	sum, err = RunTask(pfOpts)
	require.NoError(t, err)
	require.Equal(t, "paused", sum.Status)

	// Review error / bad json / unknown verdict still yields awaiting-approval
	pfOpts.Drivers.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(pfOpts.Config).DesignPlan, fn: func(drivers.CallArgs) drivers.Result {
		return drivers.Result{Text: `{"summary":"ok","html":"<html>ok</html>"}`, ExitCode: new(0), Settled: true}
	}}
	pfOpts.Drivers.DesignReview = &fnDriver{role: "design-review", base: drivers.CreateDrivers(pfOpts.Config).DesignReview, fn: func(drivers.CallArgs) drivers.Result {
		return drivers.Result{Text: `{"verdict":"weird","summary":"ok"}`, ExitCode: new(0), Settled: true}
	}}
	sum, err = RunTask(pfOpts)
	require.NoError(t, err)
	pfSt = loadFixtureState(t, pfFx)
	require.Equal(t, "awaiting-prototype-approval", pfSt.Planning.Phase)
	require.Equal(t, "pass", pfSt.Planning.UIPrototype.Review.Verdict)

	// Verify mkdirPrototypeDir & writePrototypeFile error branches
	origMkdir := mkdirPrototypeDir
	origWrite := writePrototypeFile
	t.Cleanup(func() { mkdirPrototypeDir = origMkdir; writePrototypeFile = origWrite })
	mkdirPrototypeDir = func(string, os.FileMode) error { return errors.New("mkdir fail") }
	require.Error(t, mkdirPrototypeDir("x", 0o700))
	writePrototypeFile = func(string, []byte, os.FileMode) error { return errors.New("write fail") }
	require.Error(t, writePrototypeFile("x", nil, 0o600))
}

func TestStage0ClarifyAndPrototypeFaultBranches(t *testing.T) {
	// 1. JEV Evaluate error branch (unreachable port / closed server)
	fx, opts, _ := briefEngineFixture(t, false)
	opts.Config.Subscription = nil
	opts.Config.JEV = config.JEVConfig{
		Backend: "http", Enabled: true, Endpoint: "http://127.0.0.1:1/v1/systemone",
		Model: "jev-1.14.0", KeyEnv: "TEST_JEV_KEY", TimeoutMS: 500, MaxCalls: 8,
		MinApplicability: 0.8, MinChoice: 0.6,
		AllowedData: []decision.Category{decision.TaskSummary, decision.DiagnosticFacts},
	}
	opts.Config.Planning.Clarify = &config.ClarifyConfig{Mode: "auto"}
	persistRuntimeConfig(t, &Context{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config})
	t.Setenv("TEST_JEV_KEY", "secret-test-key")
	st := loadFixtureState(t, fx)
	c, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: opts.Drivers})
	require.NoError(t, err)
	decided, _, _ := c.jevShouldClarify(st.Planning)
	require.False(t, decided)

	// 2. Clarify stage: c.err() after save, missing binding, nil driver, research err, guard err, unconfirmed cleanup, workspace mutation, JEV decidedBy
	badSaveCtx := &Context{State: st, Config: opts.Config, TaskDir: fx.taskDir, Cwd: fx.root, Drivers: opts.Drivers, fatal: errors.New("disk")}
	_, err = badSaveCtx.runClarificationStage()
	require.Error(t, err)

	noBindCfg := *opts.Config
	noBindCfg.Loops = map[string]config.LoopBinding{}
	noBindCtx, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: &noBindCfg, Drivers: opts.Drivers})
	require.NoError(t, err)
	_, err = noBindCtx.runClarificationStage()
	require.Error(t, err)

	nilDrvCtx, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: &drivers.Drivers{}})
	require.NoError(t, err)
	_, err = nilDrvCtx.runClarificationStage()
	require.ErrorContains(t, err, "no driver")

	badPrep := &state.PreparationSource{
		Version: 1, ID: "inv-1", RunID: "run-1",
		ContractHash: strings.Repeat("a", 64), ReportHash: strings.Repeat("b", 64),
		Accounted: true, ImplementationScope: []string{"src"},
	}
	badTaskCtx, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: opts.Drivers})
	require.NoError(t, err)
	badTaskCtx.State.PreparationSource = badPrep
	_, err = badTaskCtx.runClarificationStage()
	require.Error(t, err)
	badTaskCtx.State.PreparationSource = nil
	badTaskCtx.save()

	badCwdCtx, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: opts.Drivers})
	require.NoError(t, err)
	badCwdCtx.Cwd = filepath.Join(fx.root, "not-a-git-repo")
	_, err = badCwdCtx.runClarificationStage()
	require.Error(t, err)

	// Unconfirmed cleanup in clarify
	unconfDrv := *opts.Drivers
	unconfDrv.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(opts.Config).DesignPlan, fn: func(drivers.CallArgs) drivers.Result {
		return drivers.Result{CleanupError: "unconfirmed", ExitCode: new(1)}
	}}
	unconfCtx, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: &unconfDrv})
	require.NoError(t, err)
	require.NoError(t, unconfCtx.checkPlanningExports(true))
	_, err = unconfCtx.runClarificationStage()
	require.ErrorContains(t, err, "unconfirmed")

	// Workspace mutated during clarify
	mutDrv := *opts.Drivers
	mutDrv.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(opts.Config).DesignPlan, fn: func(drivers.CallArgs) drivers.Result {
		writeFile(t, filepath.Join(fx.root, "sneaky.txt"), "bad")
		return drivers.Result{Text: `{"needs-clarification":false,"is-frontend-ui":false,"summary":"ok"}`, ExitCode: new(0), Settled: true}
	}}
	mutCtx, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: &mutDrv})
	require.NoError(t, err)
	require.NoError(t, mutCtx.checkPlanningExports(true))
	_, err = mutCtx.runClarificationStage()
	require.ErrorContains(t, err, "changed workspace files")
	require.NoError(t, os.Remove(filepath.Join(fx.root, "sneaky.txt")))

	// Clarify auto with JEV enabled (when JEV doesn't skip) sets decidedBy="jev"
	okDrv := *opts.Drivers
	okDrv.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(opts.Config).DesignPlan, fn: func(drivers.CallArgs) drivers.Result {
		return drivers.Result{Text: `{"needs-clarification":false,"is-frontend-ui":false,"summary":"ok","suggested-gates":[],"questions":[]}`, ExitCode: new(0), Settled: true}
	}}
	jevDecidedCtx, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: opts.Config, Drivers: &okDrv})
	require.NoError(t, err)
	ok, err := jevDecidedCtx.runClarificationStage()
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "jev", jevDecidedCtx.State.Planning.Clarification.DecidedBy)

	// 3. Stage 1.5 UI Prototype fault & edge branches
	pfFx, pfOpts, _ := briefEngineFixture(t, false)
	pfOpts.Config.Profiles = map[string]config.ModelProfile{"uxp": {Harness: "antigravity", Channel: "kimi-subscription", Model: "gemini-4-neon-high", Thinking: "high"}}
	pfOpts.Config.Planning.UIPrototype = &config.UIPrototypeConfig{Mode: "auto", Profile: "uxp", ReviewProfile: "uxp"}
	pfOpts.PlanningOnly = true
	_, err = RunTask(pfOpts)
	require.NoError(t, err)
	pfSt := loadFixtureState(t, pfFx)
	pfSt.Planning.Brief.UIPrototypeMode = "always"
	pCtx, err := CreateContext(Opts{TaskDir: pfFx.taskDir, Cwd: pfFx.root, Config: pfOpts.Config, Drivers: pfOpts.Drivers})
	require.NoError(t, err)
	require.Equal(t, "always", pCtx.effectiveUIPrototypeMode(pfSt.Planning))
	require.True(t, pCtx.shouldRunUIPrototype(&planning.State{Brief: planning.Brief{Goal: "Canvas web", UIPrototypeMode: "auto"}}, &planning.Design{}))
	bRev, err := pCtx.uiPrototypeBinding(true)
	require.NoError(t, err)
	require.Equal(t, "gemini-4-neon-high", bRev.Model)

	// nil CurrentDesign returns true, nil
	nilDesignCtx := *pCtx
	nilDesignState := *pCtx.State
	nilPlan := *pCtx.State.Planning
	nilPlan.CurrentDesign = nil
	nilDesignState.Planning = &nilPlan
	nilDesignCtx.State = &nilDesignState
	ok, err = nilDesignCtx.runUIPrototypeStage()
	require.NoError(t, err)
	require.True(t, ok)

	// Not eligible (mode off) returns true, nil
	offCfg := *pfOpts.Config
	offCfg.Planning.UIPrototype = &config.UIPrototypeConfig{Mode: "off"}
	offCtx, err := CreateContext(Opts{TaskDir: pfFx.taskDir, Cwd: pfFx.root, Config: &offCfg, Drivers: pfOpts.Drivers})
	require.NoError(t, err)
	ok, err = offCtx.runUIPrototypeStage()
	require.NoError(t, err)
	require.True(t, ok)

	// Already approved or skipped returns true, nil
	pCtx.State.Planning.UIPrototype = &planning.UIPrototype{Revision: 1, Design: *pCtx.State.Planning.CurrentDesign, Status: "approved"}
	ok, err = pCtx.runUIPrototypeStage()
	require.NoError(t, err)
	require.True(t, ok)

	// awaiting-approval pauses cleanly
	pCtx.State.Planning.UIPrototype.Status = "awaiting-approval"
	ok, err = pCtx.runUIPrototypeStage()
	require.NoError(t, err)
	require.False(t, ok)

	// Fault paths during runUIPrototypeStage
	pCtx.State.Planning.UIPrototype = nil
	pCtx.save()
	pCtx.Config.Planning.UIPrototype = &config.UIPrototypeConfig{Mode: "always"}

	// fatal save error
	errCtx := *pCtx
	errCtx.fatal = errors.New("save err")
	_, err = errCtx.runUIPrototypeStage()
	require.Error(t, err)

	// binding error
	badBindCfg := *pCtx.Config
	badBindCfg.Planning.UIPrototype = &config.UIPrototypeConfig{Mode: "always", Profile: "missing"}
	badBindCtx, err := CreateContext(Opts{TaskDir: pfFx.taskDir, Cwd: pfFx.root, Config: &badBindCfg, Drivers: pfOpts.Drivers})
	require.NoError(t, err)
	_, err = badBindCtx.runUIPrototypeStage()
	require.Error(t, err)

	// nil driver
	nilProtoDrvCtx, err := CreateContext(Opts{TaskDir: pfFx.taskDir, Cwd: pfFx.root, Config: pCtx.Config, Drivers: &drivers.Drivers{}})
	require.NoError(t, err)
	_, err = nilProtoDrvCtx.runUIPrototypeStage()
	require.ErrorContains(t, err, "no driver")

	// research error
	badProtoTaskCtx, err := CreateContext(Opts{TaskDir: pfFx.taskDir, Cwd: pfFx.root, Config: pCtx.Config, Drivers: pfOpts.Drivers})
	require.NoError(t, err)
	badProtoTaskCtx.State.PreparationSource = badPrep
	_, err = badProtoTaskCtx.runUIPrototypeStage()
	require.Error(t, err)
	badProtoTaskCtx.State.PreparationSource = nil
	badProtoTaskCtx.save()

	// guard error
	badProtoCwdCtx, err := CreateContext(Opts{TaskDir: pfFx.taskDir, Cwd: pfFx.root, Config: pCtx.Config, Drivers: pfOpts.Drivers})
	require.NoError(t, err)
	badProtoCwdCtx.Cwd = filepath.Join(pfFx.root, "no-git")
	_, err = badProtoCwdCtx.runUIPrototypeStage()
	require.Error(t, err)

	// unconfirmed cleanup
	unconfProtoDrv := *pfOpts.Drivers
	unconfProtoDrv.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(pCtx.Config).DesignPlan, fn: func(drivers.CallArgs) drivers.Result {
		return drivers.Result{CleanupError: "unconfirmed", ExitCode: new(1)}
	}}
	unconfProtoCtx, err := CreateContext(Opts{TaskDir: pfFx.taskDir, Cwd: pfFx.root, Config: pCtx.Config, Drivers: &unconfProtoDrv})
	require.NoError(t, err)
	_, err = unconfProtoCtx.runUIPrototypeStage()
	require.Error(t, err)

	// workspace mutated during prototype
	mutProtoDrv := *pfOpts.Drivers
	mutProtoDrv.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(pCtx.Config).DesignPlan, fn: func(drivers.CallArgs) drivers.Result {
		writeFile(t, filepath.Join(pfFx.root, "sneaky2.txt"), "bad")
		return drivers.Result{Text: `{"summary":"ok","html":"<html>ok</html>"}`, ExitCode: new(0), Settled: true}
	}}
	mutProtoCtx, err := CreateContext(Opts{TaskDir: pfFx.taskDir, Cwd: pfFx.root, Config: pCtx.Config, Drivers: &mutProtoDrv})
	require.NoError(t, err)
	_, err = mutProtoCtx.runUIPrototypeStage()
	require.ErrorContains(t, err, "changed workspace files")
	require.NoError(t, os.Remove(filepath.Join(pfFx.root, "sneaky2.txt")))

	// mkdirPrototypeDir & writePrototypeFile error inside runUIPrototypeStage
	okProtoDrv := *pfOpts.Drivers
	okProtoDrv.DesignPlan = &fnDriver{role: "design-plan", base: drivers.CreateDrivers(pCtx.Config).DesignPlan, fn: func(drivers.CallArgs) drivers.Result {
		return drivers.Result{Text: `{"summary":"ok","html":"<html>ok</html>"}`, ExitCode: new(0), Settled: true}
	}}
	origMkdir, origWrite := mkdirPrototypeDir, writePrototypeFile
	t.Cleanup(func() { mkdirPrototypeDir = origMkdir; writePrototypeFile = origWrite })
	mkdirPrototypeDir = func(string, os.FileMode) error { return errors.New("mkdir err") }
	mkCtx, err := CreateContext(Opts{TaskDir: pfFx.taskDir, Cwd: pfFx.root, Config: pCtx.Config, Drivers: &okProtoDrv})
	require.NoError(t, err)
	_, err = mkCtx.runUIPrototypeStage()
	require.ErrorContains(t, err, "mkdir err")
	mkdirPrototypeDir = origMkdir

	writePrototypeFile = func(string, []byte, os.FileMode) error { return errors.New("write err") }
	wrCtx, err := CreateContext(Opts{TaskDir: pfFx.taskDir, Cwd: pfFx.root, Config: pCtx.Config, Drivers: &okProtoDrv})
	require.NoError(t, err)
	_, err = wrCtx.runUIPrototypeStage()
	require.ErrorContains(t, err, "write err")
	writePrototypeFile = origWrite

	// reviewUIPrototype nil returns: bad binding, nil driver, failed result, bad json
	design := planning.FindDesign(pCtx.State.Planning, *pCtx.State.Planning.CurrentDesign)
	proto := &planning.UIPrototype{Revision: 1, Design: design.Ref}
	require.Nil(t, badBindCtx.reviewUIPrototype(pCtx.State.Planning, design, proto, "<html></html>"))
	require.Nil(t, nilProtoDrvCtx.reviewUIPrototype(pCtx.State.Planning, design, proto, "<html></html>"))

	failRevDrv := *pfOpts.Drivers
	failRevDrv.DesignReview = &fnDriver{role: "design-review", base: drivers.CreateDrivers(pCtx.Config).DesignReview, fn: func(drivers.CallArgs) drivers.Result {
		return drivers.Result{ExitCode: new(1), Settled: true}
	}}
	failRevCtx, err := CreateContext(Opts{TaskDir: pfFx.taskDir, Cwd: pfFx.root, Config: pCtx.Config, Drivers: &failRevDrv})
	require.NoError(t, err)
	require.Nil(t, failRevCtx.reviewUIPrototype(pCtx.State.Planning, design, proto, "<html></html>"))

	badJSONRevDrv := *pfOpts.Drivers
	badJSONRevDrv.DesignReview = &fnDriver{role: "design-review", base: drivers.CreateDrivers(pCtx.Config).DesignReview, fn: func(drivers.CallArgs) drivers.Result {
		return drivers.Result{Text: "not json", ExitCode: new(0), Settled: true}
	}}
	badJSONRevCtx, err := CreateContext(Opts{TaskDir: pfFx.taskDir, Cwd: pfFx.root, Config: pCtx.Config, Drivers: &badJSONRevDrv})
	require.NoError(t, err)
	require.Nil(t, badJSONRevCtx.reviewUIPrototype(pCtx.State.Planning, design, proto, "<html></html>"))
}

func TestUnreviewedPlanResumeAfterCancel(t *testing.T) {
	// Direct helper coverage: passing plan, missing plan record fallback, and unreviewed plan.
	refPass := planning.Ref{Revision: 1, Digest: strings.Repeat("a", 64)}
	refRevise := planning.Ref{Revision: 2, Digest: strings.Repeat("b", 64)}
	ps := &planning.State{
		CurrentPlans: map[string]planning.Ref{"milestone:M-01": refPass},
		Reviews: []planning.Review{{
			ID: "r1", Kind: "plan", Target: refPass, Verdict: "pass", ScopeVerdict: "within-envelope",
		}},
	}
	_, _, needs := unreviewedPlanScope(ps)
	require.False(t, needs)

	// Unreviewed ref without matching Plan record falls back to cutting map key.
	ps.CurrentPlans["milestone:M-02"] = refRevise
	scope, scopeID, needs := unreviewedPlanScope(ps)
	require.True(t, needs)
	require.Equal(t, "milestone", scope)
	require.Equal(t, "M-02", scopeID)

	// Unreviewed ref with matching Plan record uses plan's scope & scopeID.
	ps.Plans = []planning.ExecutionPlan{{Ref: refRevise, Scope: "milestone", ScopeID: "M-02"}}
	scope, scopeID, needs = unreviewedPlanScope(ps)
	require.True(t, needs)
	require.Equal(t, "milestone", scope)
	require.Equal(t, "M-02", scopeID)

	// Integration coverage in prepareReviewedExecution when CurrentPlans holds a revise plan after cancelled request.
	fx := newGitFixture(t)
	fx.taskDir = filepath.Join(fx.root, "docs", "unrev-plan")
	cfg := mustConfig(t, fx)
	opts := runOpts(fx, drivers.CreateDrivers(cfg))
	opts.Config = cfg
	brief := planning.Brief{
		Version: 1, ID: "unrev-plan", Goal: "Resume cancelled milestone plan", OriginalRequest: "Resume cancelled milestone plan",
		Constraints: []string{}, Evidence: []string{}, Policy: "review-first", Mode: "full",
	}
	_, _, err := ControlPlanning(opts.RunOpts, PlanningCommand{Action: "prepare", RequestID: "prep", Brief: &brief}, "")
	require.NoError(t, err)

	ctx, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: cfg, Drivers: opts.Drivers})
	require.NoError(t, err)
	d, err := planning.SealDesign(planning.Design{
		Ref: planning.Ref{Revision: 1}, InputRevision: 1, Document: "# Design", Baseline: strings.Repeat("0", 40),
		PlannerInvocation:   "inv-d1",
		Requirements:        []planning.Requirement{{ID: "R-01", Title: "Core", Milestone: "M-01", Verification: "tests"}},
		Decisions:           []planning.Decision{},
		Milestones:          []planning.Milestone{{ID: "M-01", Title: "Core", Gate: "tests"}},
		ImplementationScope: []string{"."},
	})
	require.NoError(t, err)
	require.NoError(t, planning.RecordDesign(ctx.State.Planning, d))
	designRef := *ctx.State.Planning.CurrentDesign
	require.NoError(t, planning.RecordReview(ctx.State.Planning, planning.Review{
		ID: "drev-1", Kind: "design", Target: designRef, InputRevision: 1, Invocation: "inv-dr1", Role: "design-review", Binding: "b1", Verdict: "pass", ScopeVerdict: "within-envelope", Summary: "ok", Findings: []planning.Finding{},
	}))
	require.NoError(t, planning.Approve(ctx.State.Planning, planning.Approval{
		Design: designRef, ReviewID: "drev-1", RequestID: "app-1", Source: "human", Kind: "human", Start: false, At: "2026-11-25T00:00:00Z",
	}))
	pl, err := planning.SealPlan(planning.ExecutionPlan{
		Ref: planning.Ref{Revision: 1}, Design: designRef, InputRevision: 1, Scope: "milestone", ScopeID: "M-01",
		PlannerInvocation: "inv-p1", Files: map[string]string{"spec.md": "# Spec"}, Dependencies: strings.Repeat("0", 64), ScopeVerdict: "within-envelope",
	})
	require.NoError(t, err)
	require.NoError(t, planning.RecordPlan(ctx.State.Planning, pl))
	planRef := pl.Ref
	require.NoError(t, planning.RecordReview(ctx.State.Planning, planning.Review{
		ID: "rev-1", Kind: "plan", Target: planRef, Design: &designRef, InputRevision: 1, Invocation: "inv-pr1", Role: "plan-review", Binding: "b2", Verdict: "revise", ScopeVerdict: "within-envelope", Summary: "fix D-9", Findings: []planning.Finding{},
	}))
	ctx.State.Planning.Phase = "drafting"
	ctx.State.Planning.Requests = []planning.Request{{
		ID: "engine-1-1", Scope: "milestone", ScopeID: "M-01", InputRevision: 1, MaxRounds: 3, Status: "cancelled",
	}}
	ctx.save()
	require.NoError(t, ctx.err())

	// 1. planningOnly pauses cleanly instead of crashing activation.
	ctx.planningOnly = true
	ok, err := ctx.prepareReviewedExecution()
	require.NoError(t, err)
	require.False(t, ok)

	// 2. Without StartAllowed, pauses cleanly waiting for human start.
	ctx.planningOnly = false
	ctx.State.Planning.Phase = "drafting"
	ctx.save()
	ok, err = ctx.prepareReviewedExecution()
	require.NoError(t, err)
	require.False(t, ok)

	// 3. With StartAllowed, creates a fresh milestone planning request (and exercises newPlanningRequest error when MaxRounds=0).
	require.NoError(t, planning.Approve(ctx.State.Planning, planning.Approval{
		Design: designRef, ReviewID: "drev-1", RequestID: "app-2", Source: "human", Kind: "human", Start: true, At: "2026-11-25T00:00:01Z",
	}))
	ctx.State.Planning.Phase = "drafting"
	ctx.save()
	require.NoError(t, ctx.err())
	ctx.State.Planning.MaxRounds = 0
	_, err = ctx.prepareReviewedExecution()
	require.Error(t, err)

	ctx.State.Planning.MaxRounds = 3
	ctx.State.Planning.Phase = "drafting"
	ctx.Drivers.Plan = &fnDriver{role: "plan", log: &scriptLog{}, base: opts.Drivers.Plan, fn: func(a drivers.CallArgs) drivers.Result {
		return drivers.Result{
			Cancelled: true, Settled: true,
			Session: &drivers.SessionOutcome{RequestID: a.RequestID, Dispatch: "dispatched", Outcome: "cancelled", UsageStatus: "unknown", CleanupConfirmed: true},
		}
	}}
	ctx.save()
	ok, err = ctx.prepareReviewedExecution()
	require.NoError(t, err)
	require.False(t, ok)
	require.Len(t, ctx.State.Planning.Requests, 2)
	require.Equal(t, "milestone", ctx.State.Planning.Requests[1].Scope)
	require.Equal(t, "M-01", ctx.State.Planning.Requests[1].ScopeID)
}
