package machine

import (
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/require"
)

func TestParallelCohortHonorsPauseBeforeReviewAndResumesWithoutReexecution(t *testing.T) {
	for _, dispatch := range []string{"batch", "ready-queue"} {
		t.Run(dispatch, func(t *testing.T) {
			fx, cfg := readyQueueFixture(t)
			cfg.Scheduler.Dispatch = dispatch
			previous := parallelDrivers
			t.Cleanup(func() { parallelDrivers = previous })
			parallelDrivers = func(*config.Config) *drivers.Drivers { return happyDrivers().set() }
			d := happyDrivers()
			opts := runOpts(fx, d.set())
			opts.Config, opts.PauseBefore = cfg, "review"
			head, ok := verify.GetBaseRef(fx.root)
			require.True(t, ok)

			summary, err := RunTask(opts)
			require.NoError(t, err)
			require.Equal(t, "paused", summary.Status)
			require.Zero(t, len(d.log.review), "parallel L1 completion must not bypass the requested Review pause")
			after, ok := verify.GetBaseRef(fx.root)
			require.True(t, ok)
			require.Equal(t, head, after, "the paused cohort must not be integrated before Review")
			st := loadFixtureState(t, fx)
			for _, id := range []string{"M01-T01", "M01-T02", "M01-T03"} {
				ps := st.Packets[id]
				require.Nil(t, ps.Evidence.Review)
				require.NoFileExists(t, filepath.Join(fx.root, "build", id+".txt"))
				if dispatch == "batch" && id == "M01-T03" {
					require.Equal(t, "pending", ps.Status)
					require.Zero(t, ps.Attempts)
					require.Nil(t, ps.BaseRef)
					require.Nil(t, ps.Worktree)
					continue
				}
				require.Equal(t, "l1-passed", ps.Status)
				require.Equal(t, 1, ps.Attempts)
				require.True(t, l1PassTrue(ps))
				require.NotNil(t, ps.Worktree)
				require.FileExists(t, filepath.Join(ps.Worktree.Path, "build", id+".txt"))
			}

			opts.PauseBefore = ""
			summary, err = RunTask(opts)
			require.NoError(t, err)
			require.Equal(t, "completed", summary.Status)
			require.Len(t, d.log.review, 3)
			st = loadFixtureState(t, fx)
			for _, id := range []string{"M01-T01", "M01-T02", "M01-T03"} {
				ps := st.Packets[id]
				require.Equal(t, "completed", ps.Status)
				require.Equal(t, 1, ps.Attempts, "resuming a verified cohort must not repeat Execute")
				require.FileExists(t, filepath.Join(fx.root, "build", id+".txt"))
			}
		})
	}
}
