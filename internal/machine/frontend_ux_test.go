package machine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func frontendFixture(t *testing.T) (fixture, *scriptedDrivers, RunTaskOpts) {
	t.Helper()
	fx := newGitFixture(t)
	pktPath := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
	raw, err := os.ReadFile(pktPath)
	require.NoError(t, err)
	writeFile(t, pktPath, strings.Replace(string(raw), "route: default", "route: frontend", 1))
	gitCmd(t, fx.root, "add", "-A")
	gitCmd(t, fx.root, "commit", "-qm", "test: mark M01-T01 as frontend route")
	sd := happyDrivers()
	opts := runOpts(fx, sd.set())
	opts.Config.Profiles = map[string]config.ModelProfile{
		"neon": {Harness: "antigravity", Model: "gemini-4-neon-high", Channel: "kimi-subscription", Thinking: "high"},
	}
	opts.Config.FrontendUXReview = config.FrontendUXReviewConfig{Enabled: true, Profile: "neon"}
	return fx, sd, opts
}

func TestFrontendUXAuditRefinesBlockingOnceThenPassesToCodexReview(t *testing.T) {
	fx, sd, opts := frontendFixture(t)
	uxCalls := 0
	codeReviewCalls := 0
	execCalls := 0
	sd.execFn = func(a drivers.CallArgs) drivers.Result {
		execCalls++
		if execCalls == 2 {
			require.Contains(t, a.Prompt, "frontend-ux-review")
		}
		return sd.mock.Execute(a)
	}
	sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
		if strings.Contains(a.Prompt, "Frontend UX & Microcopy Audit") {
			uxCalls++
			require.Equal(t, "read-only", a.PermissionMode)
			return drivers.MockResult(func(r *drivers.Result) {
				r.Text = `{"verdict":"revise","summary":"Button label unclear","findings":[{"severity":"blocking","location":"ui.tsx:10","issue":"Ambiguous CTA","suggestion":"Use Approve & Start"},{"severity":"advisory","issue":"Spacing"}]}`
			})
		}
		codeReviewCalls++
		return passVerdict()
	}

	opts.Drivers = sd.set()
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	require.Equal(t, 1, uxCalls, "UX audit runs once and caps refinement at 1 pass")
	require.Equal(t, 2, codeReviewCalls, "both M01-T01 and M01-T02 receive Codex review")
	raw, err := os.ReadFile(filepath.Join(fx.taskDir, "reports", "M01-T01.ux-review.json"))
	require.NoError(t, err)
	var rec uxReviewRecord
	require.NoError(t, json.Unmarshal(raw, &rec))
	require.True(t, rec.Refined)
	require.Equal(t, "neon", rec.Profile)
}

func TestFrontendUXAuditAdvisoryOnlyAndEdgeBranches(t *testing.T) {
	t.Run("advisory only proceeds directly to code review", func(t *testing.T) {
		fx, sd, opts := frontendFixture(t)
		execCalls := 0
		sd.execFn = func(a drivers.CallArgs) drivers.Result {
			if a.PacketID == "M01-T01" {
				execCalls++
			}
			return sd.mock.Execute(a)
		}
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			if strings.Contains(a.Prompt, "Frontend UX & Microcopy Audit") {
				return drivers.MockResult(func(r *drivers.Result) {
					r.Text = `{"verdict":"pass","summary":"Clean UI","findings":[{"severity":"advisory","issue":"Minor spacing"}]}`
				})
			}
			return passVerdict()
		}
		opts.Drivers = sd.set()
		summary, err := RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "completed", summary.Status)
		require.Equal(t, 1, execCalls)
		require.FileExists(t, filepath.Join(fx.taskDir, "reports", "M01-T01.ux-review.json"))
	})

	t.Run("unparseable UX output logs and continues", func(t *testing.T) {
		_, sd, opts := frontendFixture(t)
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			if strings.Contains(a.Prompt, "Frontend UX & Microcopy Audit") {
				return drivers.MockResult(func(r *drivers.Result) { r.Text = "not json" })
			}
			return passVerdict()
		}
		opts.Drivers = sd.set()
		summary, err := RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "completed", summary.Status)
	})

	t.Run("cancelled, budget stop, missing profile, cleanup error, and write failures", func(t *testing.T) {
		fx, sd, opts := frontendFixture(t)
		writeFile(t, filepath.Join(fx.taskDir, "reports", "M01-T01.ux-review.json"), "{bad json")
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			if strings.Contains(a.Prompt, "Frontend UX & Microcopy Audit") {
				return drivers.MockResult(func(r *drivers.Result) { r.Cancelled = true })
			}
			return passVerdict()
		}
		opts.Drivers = sd.set()
		summary, err := RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "running", summary.Status)

		_, sd, opts = frontendFixture(t)
		opts.Config.Guardrails.MaxTaskCalls = new(1)
		cBudget, err := CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Drivers: sd.set(), Log: func(string) {}})
		require.NoError(t, err)
		cBudget.State.Budget.Totals.Calls = 1
		_, uxStopped, err := cBudget.frontendUXAuditStep(cBudget.Packets["M01-T01"])
		require.NoError(t, err)
		require.True(t, uxStopped)

		_, sd, opts = frontendFixture(t)
		opts.Config.Guardrails.MaxTaskCalls = new(1)
		opts.Drivers = sd.set()
		summary, err = RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "budget-stopped", summary.Status)

		_, sd, opts = frontendFixture(t)
		opts.Config.FrontendUXReview.Profile = "missing"
		c, err := CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Drivers: sd.set(), Log: func(string) {}})
		require.NoError(t, err)
		_, _, err = c.frontendUXAuditStep(c.Packets["M01-T01"])
		require.ErrorContains(t, err, "not defined")

		_, sd, opts = frontendFixture(t)
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			if strings.Contains(a.Prompt, "Frontend UX & Microcopy Audit") {
				return drivers.MockResult(func(r *drivers.Result) { r.CleanupError = "ux orphan" })
			}
			return passVerdict()
		}
		opts.Drivers = sd.set()
		_, err = RunTask(opts)
		require.ErrorContains(t, err, "ux orphan")

		_, sd, opts = frontendFixture(t)
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			if strings.Contains(a.Prompt, "Frontend UX & Microcopy Audit") {
				return drivers.MockResult(func(r *drivers.Result) {
					r.Text = `{"verdict":"pass","summary":"Clean","findings":[]}`
				})
			}
			return passVerdict()
		}
		origMkdir := mkdirUXReviewDir
		mkdirUXReviewDir = func(string, os.FileMode) error { return errors.New("ux mkdir boom") }
		t.Cleanup(func() { mkdirUXReviewDir = origMkdir })
		opts.Drivers = sd.set()
		_, err = RunTask(opts)
		require.ErrorContains(t, err, "ux mkdir boom")
		mkdirUXReviewDir = origMkdir

		_, sd, opts = frontendFixture(t)
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			if strings.Contains(a.Prompt, "Frontend UX & Microcopy Audit") {
				return drivers.MockResult(func(r *drivers.Result) {
					r.Text = `{"verdict":"pass","summary":"Clean","findings":[]}`
				})
			}
			return passVerdict()
		}
		origWrite := writeUXReviewFile
		writeUXReviewFile = func(string, []byte, os.FileMode) error { return errors.New("ux write boom") }
		t.Cleanup(func() { writeUXReviewFile = origWrite })
		opts.Drivers = sd.set()
		_, err = RunTask(opts)
		require.ErrorContains(t, err, "ux write boom")
		writeUXReviewFile = origWrite
	})
}

func TestRunGuardrailsEnforceMaxPacketAttemptsMaxReviewRoundsAndMaxTaskCalls(t *testing.T) {
	t.Run("max-packet-attempts caps higher packet max-retries", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		execCalls := 0
		sd.execFn = func(a drivers.CallArgs) drivers.Result {
			execCalls++
			return drivers.MockResult(func(r *drivers.Result) {
				code := 1
				r.ExitCode = &code
				msg := "error-" + string(rune('a'+execCalls))
				r.ErrorMessage = &msg
				r.Settled = false
			})
		}
		opts := runOpts(fx, sd.set())
		opts.Config.Guardrails.MaxPacketAttempts = new(1)
		opts.Drivers = sd.set()
		summary, err := RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "escalated", summary.Status)
		st := loadFixtureState(t, fx)
		require.Equal(t, 1, st.Packets["M01-T01"].FailedAttempts)
	})

	t.Run("max-review-rounds escalates after 2 review returns", func(t *testing.T) {
		fx := newGitFixture(t)
		sd := happyDrivers()
		reviewCalls := 0
		sd.reviewFn = func(a drivers.CallArgs) drivers.Result {
			reviewCalls++
			return drivers.MockResult(func(r *drivers.Result) {
				r.Text = `{"verdict":"return","score":0.4,"reasons":["defect round ` + string(rune('0'+reviewCalls)) + `"]}`
			})
		}
		opts := runOpts(fx, sd.set())
		opts.Config.Guardrails.MaxPacketAttempts = new(5)
		opts.Config.Guardrails.MaxReviewRounds = new(2)
		opts.Drivers = sd.set()
		summary, err := RunTask(opts)
		require.NoError(t, err)
		require.Equal(t, "escalated", summary.Status)
		require.Equal(t, 4, reviewCalls, "2 rounds before plan revision + 2 rounds after plan revision")
	})

	t.Run("reviewReturnCount skips non-map history entries", func(t *testing.T) {
		ps := &state.PacketState{RetryHistory: []any{
			map[string]any{"scope": "attempt", "error-code": "REVIEW_RETURNED"},
			map[string]any{"policy": "return-to-planner"},
			"not-a-map",
			map[string]any{"scope": "attempt", "error-code": "REVIEW_RETURNED"},
		}}
		require.Equal(t, 1, reviewReturnCount(ps))
	})
}
