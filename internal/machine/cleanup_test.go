package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A transport timeout or rate-limit label must never authorize an overlapping
// child when the adapter cannot prove that its preceding call has stopped.
func TestRunStopsEveryRoleWhenCleanupIsUnknown(t *testing.T) {
	for _, role := range []string{"plan", "plan-resume", "execute", "review", "judge"} {
		t.Run(role, func(t *testing.T) {
			fx := newGitFixture(t)
			initialHEAD := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
			sd := happyDrivers()
			const cleanup = "owned process group disappearance was not confirmed"
			receipt := json.RawMessage(`{"outcome":"error","cleanup":{"confirmed":false}}`)
			fail := func(base func(drivers.CallArgs) drivers.Result) func(drivers.CallArgs) drivers.Result {
				return func(a drivers.CallArgs) drivers.Result {
					r := base(a)
					r.Settled = false
					r.CleanupError = cleanup
					r.CodexProReceipt = receipt
					message := "429 rate limit"
					r.ErrorMessage = &message
					return r
				}
			}
			failureRole := strings.TrimSuffix(role, "-resume")
			switch failureRole {
			case "plan":
				sd.planFn = fail(sd.mock.Plan)
			case "execute":
				sd.execFn = fail(sd.mock.Execute)
			case "review":
				sd.reviewFn = fail(sd.mock.Review)
			case "judge":
				sd.judgeFn = fail(sd.mock.Judge)
			}
			opts := runOpts(fx, sd.set())
			opts.Sleep = func(time.Duration) { t.Error("unconfirmed cleanup must not enter retry backoff") }
			if role == "plan-resume" {
				b := opts.Config.Loops["plan"]
				st := loadFixtureState(t, fx)
				st.PlanSession = map[string]any{"session-id": "resident-plan", "harness": b.Harness,
					"model": b.Model, "channel": b.Channel, "cwd": fx.root, "thinking": b.Thinking}
				_, err := state.Save(fx.taskDir, st, nil)
				require.NoError(t, err)
			}

			summary, err := RunTask(opts)
			require.ErrorContains(t, err, "cleanup is unconfirmed")
			require.ErrorContains(t, err, cleanup)
			assert.Equal(t, "escalated", summary.Status)
			st := loadFixtureState(t, fx)
			assert.Equal(t, "escalated", st.Status)
			require.NotEmpty(t, st.Inbox)
			entry := st.Inbox[len(st.Inbox)-1].(map[string]any)
			assert.Equal(t, failureRole, entry["role"])
			assert.Equal(t, cleanup, entry["cleanup-error"])
			assert.Contains(t, entry["reason"], "confirm the owned processes have ended")
			inbox, err := os.ReadFile(filepath.Join(fx.taskDir, "inbox.md"))
			require.NoError(t, err)
			assert.Contains(t, string(inbox), cleanup)
			metric := lastMetricsLine(t, fx.taskDir)
			assert.Equal(t, failureRole, metric["role"])
			assert.Equal(t, cleanup, metric["result"].(map[string]any)["cleanup-error"])
			assert.Equal(t, false, metric["result"].(map[string]any)["settled"])
			assert.Equal(t, false, metric["codex-pro-receipt"].(map[string]any)["cleanup"].(map[string]any)["confirmed"])

			assert.Len(t, sd.log.plan, 1)
			switch failureRole {
			case "plan":
				assert.Empty(t, sd.log.exec)
				assert.Empty(t, sd.log.review)
				assert.Empty(t, sd.log.judge)
				if role == "plan-resume" {
					assert.Equal(t, "resident-plan", sd.log.plan[0].SessionID)
				}
			case "execute", "review":
				assert.Len(t, sd.log.exec, 1)
				if failureRole == "review" {
					assert.Len(t, sd.log.review, 1)
				} else {
					assert.Empty(t, sd.log.review)
				}
				assert.Empty(t, sd.log.judge)
				ps := st.Packets["M01-T01"]
				assert.Equal(t, "inbox", ps.Status)
				assert.Zero(t, ps.FailedAttempts)
				assert.Zero(t, ps.PlanRevisions)
				require.NotNil(t, ps.Worktree)
				require.DirExists(t, ps.Worktree.Path)
				assert.FileExists(t, filepath.Join(ps.Worktree.Path, "build", "M01-T01.txt"))
				assert.Equal(t, initialHEAD, strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD")), "unknown cleanup must not commit or merge the candidate")
				require.NotEmpty(t, ps.RetryHistory)
				assert.Equal(t, "cleanup-unconfirmed: no retry", ps.RetryHistory[len(ps.RetryHistory)-1].(map[string]any)["policy"])
			case "judge":
				assert.Len(t, sd.log.exec, 2)
				assert.Len(t, sd.log.review, 2)
				assert.Len(t, sd.log.judge, 1)
				assert.NotEqual(t, "closed", st.Milestones["M-01"].Status)
				assert.FileExists(t, filepath.Join(fx.root, "build", "M01-T01.txt"))
			}
		})
	}
}

func TestCleanupEscalationPreservesPersistenceFailures(t *testing.T) {
	for _, failedFile := range []string{"metrics.jsonl", ".state.lock"} {
		t.Run(failedFile, func(t *testing.T) {
			fx := newFixture(t)
			c := unitTestContext(t, fx)
			sd := happyDrivers()
			sd.planFn = func(a drivers.CallArgs) drivers.Result {
				path := filepath.Join(fx.taskDir, failedFile)
				_ = os.Remove(path)
				require.NoError(t, os.Mkdir(path, 0o755))
				r := sd.mock.Plan(a)
				r.CleanupError = "guardian ownership unconfirmed"
				return r
			}
			c.Drivers = sd.set()
			_, err := c.planStep(c.Spec.Milestones[0], "")
			require.ErrorContains(t, err, "cleanup is unconfirmed")
			require.ErrorContains(t, err, failedFile)
			assert.Equal(t, "escalated", c.State.Status)
			_, again := c.planStep(c.Spec.Milestones[0], "")
			require.Error(t, again)
			assert.Len(t, sd.log.plan, 1)
			if failedFile == "metrics.jsonl" {
				assert.Equal(t, "escalated", loadFixtureState(t, fx).Status, "a metrics error must not suppress the independent state escalation")
			}
			inbox, err := os.ReadFile(filepath.Join(fx.taskDir, "inbox.md"))
			require.NoError(t, err)
			assert.Contains(t, string(inbox), "guardian ownership unconfirmed")
		})
	}
}
