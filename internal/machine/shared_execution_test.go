package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/stretchr/testify/require"
)

func TestSharedExecutionCompletesOnlyAfterCombinedCandidateReview(t *testing.T) {
	for _, mode := range []string{"shared", "mixed", "repair", "pause"} {
		t.Run(mode, func(t *testing.T) {
			fx, cfg := parallelFixture(t)
			off := false
			cfg.Worktree.Enabled = &off
			var started, active, peak atomic.Int32
			ready := make(chan struct{})
			var mu sync.Mutex
			calls, reviews := map[string]int{}, map[string]int{}
			base := drivers.NewMockDriver(nil)
			external := filepath.Join(t.TempDir(), "external")
			old := parallelDrivers
			t.Cleanup(func() { parallelDrivers = old })
			execute := func(a drivers.CallArgs) drivers.Result {
				mu.Lock()
				calls[a.PacketID]++
				attempt := calls[a.PacketID]
				mu.Unlock()
				if attempt == 1 {
					n := active.Add(1)
					defer active.Add(-1)
					for prev := peak.Load(); n > prev && !peak.CompareAndSwap(prev, n); prev = peak.Load() {
					}
					if started.Add(1) == 2 {
						close(ready)
					}
					select {
					case <-ready:
					case <-time.After(5 * time.Second):
						t.Error("shared executors did not overlap")
					}
					if mode == "mixed" && a.PacketID == "M01-T02" {
						gitCmd(t, fx.root, "worktree", "add", "-b", "external-shared-peer", external)
						raw, err := json.Marshal(map[string]any{"flywheelExecutionDirectory": map[string]string{"cwd": external, "branch": "external-shared-peer"}})
						require.NoError(t, err)
						return drivers.MockResult(func(r *drivers.Result) { r.Text = string(raw) })
					}
				}
				result := base.Execute(a)
				if mode == "repair" && a.PacketID == "M01-T02" && attempt > 1 {
					require.NoError(t, os.WriteFile(filepath.Join(a.Cwd, "build", a.PacketID+".txt"), []byte("repaired candidate\n"), 0644))
				}
				return result
			}
			parallelDrivers = func(*config.Config) *drivers.Drivers { d := happyDrivers(); d.execFn = execute; return d.set() }
			d := happyDrivers()
			d.execFn = execute
			d.reviewFn = func(a drivers.CallArgs) drivers.Result {
				require.Zero(t, active.Load(), "all parallel writes must end before review")
				id := strings.Split(strings.TrimPrefix(a.SessionTitle, "Flywheel · review/"), " · ")[0]
				require.Contains(t, []string{"M01-T01", "M01-T02"}, id)
				reviews[id]++
				if mode == "repair" && id == "M01-T02" && reviews[id] == 1 {
					return drivers.MockResult(func(r *drivers.Result) {
						r.Text = `{"verdict":"return","score":0.4,"reasons":["repair the second output"]}`
					})
				}
				return base.Review(a)
			}
			opts := runOpts(fx, d.set())
			opts.Config = cfg
			if mode == "pause" {
				opts.PauseBefore = "review"
			}
			summary, err := RunTask(opts)
			require.NoError(t, err)
			if mode == "pause" {
				st := loadFixtureState(t, fx)
				require.Equal(t, "paused", summary.Status)
				require.NotNil(t, st.SharedExecution)
				require.NotEqual(t, "completed", st.Packets["M01-T01"].Status)
				opts.PauseBefore = ""
				summary, err = RunTask(opts)
				require.NoError(t, err)
			}
			require.Equal(t, "completed", summary.Status)
			require.Equal(t, int32(2), peak.Load())
			st := loadFixtureState(t, fx)
			require.Nil(t, st.SharedExecution)
			for _, id := range []string{"M01-T01", "M01-T02"} {
				require.FileExists(t, filepath.Join(fx.root, "build", id+".txt"))
			}
			if mode != "mixed" {
				require.Nil(t, st.Packets["M01-T01"].Worktree)
				require.Nil(t, st.Packets["M01-T02"].Worktree)
				require.Equal(t, st.Packets["M01-T01"].Commit.(map[string]any)["sha"], st.Packets["M01-T02"].Commit.(map[string]any)["sha"])
			} else {
				require.DirExists(t, external)
			}
			if mode == "repair" {
				require.GreaterOrEqual(t, reviews["M01-T01"], 2, "peer repair invalidates the earlier whole-candidate review")
			}
			require.Equal(t, 1, calls["M01-T01"], "revalidation must not repeat a settled implementation")
		})
	}
}

func TestSharedExecutionTracksIndependentRoleReceiptsAndCombinedDelivery(t *testing.T) {
	fx, _ := parallelFixture(t)
	d := happyDrivers()
	opts := sessionRunOpts(fx, sessionDriverSet(d))
	off := false
	opts.Config.Worktree.Enabled = &off
	opts.Config.Scheduler.Mode = "parallel"
	var started atomic.Int32
	ready := make(chan struct{})
	old := parallelDrivers
	t.Cleanup(func() { parallelDrivers = old })
	parallelDrivers = func(*config.Config) *drivers.Drivers {
		worker := happyDrivers()
		worker.execFn = func(a drivers.CallArgs) drivers.Result {
			require.Equal(t, []string{"build/" + a.PacketID + ".txt"}, a.CandidateScope)
			if started.Add(1) == 2 {
				close(ready)
			}
			select {
			case <-ready:
			case <-time.After(5 * time.Second):
				t.Error("tracked executors did not overlap")
			}
			return worker.mock.Execute(a)
		}
		return sessionDriverSet(worker)
	}
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	st := loadFixtureState(t, fx)
	for _, id := range []string{"M01-T01", "M01-T02"} {
		require.Equal(t, []string{"build/" + id + ".txt"}, st.Sessions.Records["execute/"+id].CandidateScope)
		require.Empty(t, st.Sessions.Records["review/"+id].CandidateScope, "review always binds the complete combined candidate")
		require.Equal(t, "consumed", st.Sessions.Records["execute/"+id].Last.Status)
	}
	require.NotEqual(t, st.Sessions.Records["execute/M01-T01"].ThreadID, st.Sessions.Records["execute/M01-T02"].ThreadID)
	require.Equal(t, int64(1080), st.Budget.Totals.Tokens)
}
