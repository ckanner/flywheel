package machine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func queuedPlanningFixture(t *testing.T) (RunOpts, *Context, planningFeedbackRequest) {
	t.Helper()
	opts, st, _ := controlPlanningFixture(t)
	request := PlanningCommand{Action: "feedback", Revision: st.Revision, RequestID: "pending-feedback", Text: "Keep the existing interface stable."}
	_, receipt, err := queuePlanningFeedback(opts, request)
	require.NoError(t, err)
	require.Equal(t, "queued", receipt.Status)
	c, err := CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Now: opts.Now})
	require.NoError(t, err)
	var entry planningFeedbackRequest
	require.NoError(t, json.Unmarshal(mustReadFile(t, filepath.Join(opts.TaskDir, ".control", "planning", "feedback-"+request.RequestID+".json")), &entry))
	return opts, c, entry
}

func TestPlanningControlReceiptQueriesQueuedAppliedAndRejectedAcrossRestart(t *testing.T) {
	opts, c, entry := queuedPlanningFixture(t)
	status := PlanningCommand{Action: "status", RequestID: entry.Command.RequestID}
	before := mustReadFile(t, state.StateFile(opts.TaskDir))
	_, receipt, err := ControlPlanning(opts, status, "")
	require.NoError(t, err)
	require.Equal(t, "queued", receipt.Status)
	require.Equal(t, before, mustReadFile(t, state.StateFile(opts.TaskDir)))
	require.NoError(t, c.applyPlanningQueue())
	_, receipt, err = ControlPlanning(opts, status, "")
	require.NoError(t, err)
	require.Equal(t, "applied", receipt.Status)
	require.NoError(t, os.Remove(filepath.Join(opts.TaskDir, ".control", "planning", "receipt-"+entry.Command.RequestID+".json")))
	_, receipt, err = ControlPlanning(opts, status, "")
	require.NoError(t, err)
	require.Equal(t, "applied", receipt.Status)
	plan := PlanningCommand{Action: "plan", Revision: c.State.Revision, RequestID: "specific-plan", Scope: "replan", ScopeID: "M-01", Text: "Compare the retry ordering with the original design.", Start: new(false)}
	st, _, err := ControlPlanning(opts, plan, "")
	require.NoError(t, err)
	require.Equal(t, plan.Text, st.Planning.Requests[len(st.Planning.Requests)-1].Feedback)
	_, receipt, err = ControlPlanning(opts, PlanningCommand{Action: "status", RequestID: plan.RequestID}, "")
	require.NoError(t, err)
	require.Equal(t, "plan", receipt.Action)
	plan.Text = "A different scoped discussion cannot reuse the same identity."
	_, _, err = ControlPlanning(opts, plan, "")
	require.ErrorContains(t, err, "different content")
	st = reviewedControlDesign(t, opts, st)
	approve := PlanningCommand{Action: "approve", Revision: st.Revision, RequestID: "human-review", Design: st.Planning.CurrentDesign}
	st, _, err = ControlPlanning(opts, approve, "tty:human-review")
	require.NoError(t, err)
	_, receipt, err = ControlPlanning(opts, PlanningCommand{Action: "status", RequestID: approve.RequestID}, "")
	require.NoError(t, err)
	require.Equal(t, "approve", receipt.Action)
	_, _, err = ControlPlanning(opts, PlanningCommand{Action: "status", RequestID: "missing"}, "")
	require.ErrorContains(t, err, "no saved request")
	_, err = readPlanningControlReceipt(opts.TaskDir, "legacy", &state.State{})
	require.ErrorContains(t, err, "reviewed planning")
	// A rejection is queryable even if the archive write failed after its state CAS.
	opts, c, entry = queuedPlanningFixture(t)
	root, err := planningControlRoot(opts.TaskDir, false)
	require.NoError(t, err)
	defer root.Close()
	write := writePlanningBytes
	writePlanningBytes = func(*os.Root, string, []byte) error { return os.ErrPermission }
	t.Cleanup(func() { writePlanningBytes = write })
	err = c.finishPlanningFeedback(root, "feedback-"+entry.Command.RequestID+".json", entry, false, fmt.Errorf("input became stale"))
	require.ErrorIs(t, err, os.ErrPermission)
	writePlanningBytes = write
	_, receipt, err = ControlPlanning(opts, PlanningCommand{Action: "status", RequestID: entry.Command.RequestID}, "")
	require.NoError(t, err)
	require.Equal(t, "rejected", receipt.Status)
	require.Equal(t, "input became stale", receipt.Message)
	_, receipt, err = ControlPlanning(opts, entry.Command, "")
	require.ErrorContains(t, err, "already rejected")
	require.Equal(t, "rejected", receipt.Status)
	_, receipt, err = queuePlanningFeedback(opts, entry.Command)
	require.ErrorContains(t, err, "already rejected")
	require.Equal(t, "rejected", receipt.Status)
	require.Len(t, c.State.Inbox, 1)
	require.NoError(t, c.finishPlanningFeedback(root, "feedback-"+entry.Command.RequestID+".json", entry, false, fmt.Errorf("input became stale")))
	require.Len(t, c.State.Inbox, 1, "recovery does not duplicate rejection audit")
	_, receipt, err = ControlPlanning(opts, PlanningCommand{Action: "status", RequestID: entry.Command.RequestID}, "")
	require.NoError(t, err)
	require.Equal(t, "rejected", receipt.Status)
}

func TestPlanningReceiptReaderRejectsForgedOrUnreadableArchives(t *testing.T) {
	for _, scenario := range []string{"malformed", "wrong-input", "without-evidence", "unproved-rejection", "future-revision", "archive-directory", "pending-directory", "pending-conflict", "root-file"} {
		t.Run(scenario, func(t *testing.T) {
			opts, c, entry := queuedPlanningFixture(t)
			path := filepath.Join(opts.TaskDir, ".control", "planning", "receipt-"+entry.Command.RequestID+".json")
			saved := savedPlanningControl{Request: entry, Receipt: &PlanningControlReceipt{RequestID: entry.Command.RequestID, Action: "feedback", Status: "applied", Revision: c.State.Revision}}
			switch scenario {
			case "malformed":
				writeFile(t, path, "{broken")
			case "wrong-input":
				saved.Request.Version = 2
			case "unproved-rejection":
				saved.Receipt.Status = "rejected"
			case "future-revision":
				saved.Receipt.Revision += 10
			case "archive-directory":
				require.NoError(t, os.Mkdir(path, 0700))
			case "pending-directory":
				pending := filepath.Join(opts.TaskDir, ".control", "planning", "feedback-"+entry.Command.RequestID+".json")
				require.NoError(t, os.Remove(pending))
				require.NoError(t, os.Mkdir(pending, 0700))
			case "pending-conflict":
				require.NoError(t, planning.ApplyFeedback(c.State.Planning, planning.Feedback{RequestID: entry.Command.RequestID, Text: "Different prior content", Source: "control:earlier", At: state.NowISO()}))
			case "root-file":
				require.NoError(t, os.RemoveAll(filepath.Join(opts.TaskDir, ".control")))
				writeFile(t, filepath.Join(opts.TaskDir, ".control"), "file")
			}
			if scenario == "wrong-input" || scenario == "without-evidence" || scenario == "unproved-rejection" || scenario == "future-revision" {
				raw, err := json.Marshal(saved)
				require.NoError(t, err)
				writeFile(t, path, string(raw))
			}
			_, err := readPlanningControlReceipt(opts.TaskDir, entry.Command.RequestID, c.State)
			require.Error(t, err)
		})
	}
}

func TestPlanningQueueAdmissionFailuresNeverChangeInputOrReplacePublishedRequest(t *testing.T) {
	for _, scenario := range []string{"missing-state", "foreign-workspace", "root-file", "bad-pending", "stale-revision", "policy-grant", "queue-full", "too-many", "directory-list", "write-failed", "publish-collision", "publish-directory", "publish-sync", "already-applied"} {
		t.Run(scenario, func(t *testing.T) {
			opts, st, _ := controlPlanningFixture(t)
			command := PlanningCommand{Action: "feedback", Revision: st.Revision, RequestID: "new-feedback", Text: "Add failure recovery to the design."}
			oldWrite, oldList, oldOpen, oldSync := writePlanningBytes, listPlanningDirectory, openPlanningDirectory, syncPlanningHandle
			t.Cleanup(func() {
				writePlanningBytes, listPlanningDirectory, openPlanningDirectory, syncPlanningHandle = oldWrite, oldList, oldOpen, oldSync
			})
			root, err := planningControlRoot(opts.TaskDir, true)
			require.NoError(t, err)
			root.Close()
			switch scenario {
			case "missing-state":
				require.NoError(t, os.Remove(state.StateFile(opts.TaskDir)))
			case "foreign-workspace":
				opts.Cwd = t.TempDir()
			case "root-file":
				require.NoError(t, os.RemoveAll(filepath.Join(opts.TaskDir, ".control")))
				writeFile(t, filepath.Join(opts.TaskDir, ".control"), "file")
			case "bad-pending":
				writeFile(t, filepath.Join(opts.TaskDir, ".control", "planning", "feedback-"+command.RequestID+".json"), "{bad")
			case "stale-revision":
				command.Revision++
			case "policy-grant":
				command.Policy = "delegated"
			case "queue-full", "too-many":
				n := 256
				if scenario == "too-many" {
					n++
				}
				for i := range n {
					writeFile(t, filepath.Join(opts.TaskDir, ".control", "planning", fmt.Sprintf("feedback-existing-%03d.json", i)), "{}")
				}
			case "directory-list":
				listPlanningDirectory = func(*os.Root) ([]os.DirEntry, error) { return nil, os.ErrPermission }
			case "write-failed":
				writePlanningBytes = func(*os.Root, string, []byte) error { return os.ErrPermission }
			case "publish-collision":
				writePlanningBytes = func(root *os.Root, name string, raw []byte) error {
					if err := oldWrite(root, name, raw); err != nil {
						return err
					}
					return root.WriteFile("feedback-"+command.RequestID+".json", []byte("concurrent publisher wins"), 0600)
				}
			case "publish-directory":
				opens := 0
				openPlanningDirectory = func(root *os.Root) (*os.File, error) {
					opens++
					if opens == 3 {
						return nil, os.ErrPermission
					}
					return oldOpen(root)
				}
			case "publish-sync":
				dirs := 0
				syncPlanningHandle = func(f *os.File) error {
					info, err := f.Stat()
					if err != nil {
						return err
					}
					if info.IsDir() {
						dirs++
						if dirs == 2 {
							return os.ErrPermission
						}
					}
					return oldSync(f)
				}
			case "already-applied":
				_, _, err := ControlPlanning(opts, command, "")
				require.NoError(t, err)
			}
			_, receipt, err := queuePlanningFeedback(opts, command)
			if scenario == "already-applied" {
				require.NoError(t, err)
				require.Equal(t, "unchanged", receipt.Status)
			} else {
				require.Error(t, err)
			}
			if scenario == "publish-collision" {
				require.Equal(t, "concurrent publisher wins", string(mustReadFile(t, filepath.Join(opts.TaskDir, ".control", "planning", "feedback-"+command.RequestID+".json"))))
			}
			if scenario != "missing-state" && scenario != "already-applied" {
				saved, err := state.Load(opts.TaskDir)
				require.NoError(t, err)
				require.Equal(t, st.Planning.InputRevision, saved.Planning.InputRevision)
				require.Empty(t, saved.Planning.Feedback)
			}
		})
	}
}

func TestPlanningQueueOwnerFailureBoundariesKeepPendingInputsRecoverable(t *testing.T) {
	for _, scenario := range []string{"root-file", "list-failed", "malformed", "collision", "collision-write-failed", "stale-write-failed", "rejected-policy", "rejected-policy-write-failed", "adoption-save-failed", "rejection-save-failed"} {
		t.Run(scenario, func(t *testing.T) {
			opts, c, entry := queuedPlanningFixture(t)
			pending := filepath.Join(opts.TaskDir, ".control", "planning", "feedback-"+entry.Command.RequestID+".json")
			oldWrite, oldList := writePlanningBytes, listPlanningDirectory
			t.Cleanup(func() { writePlanningBytes, listPlanningDirectory = oldWrite, oldList })
			switch scenario {
			case "root-file":
				require.NoError(t, os.RemoveAll(filepath.Join(opts.TaskDir, ".control")))
				writeFile(t, filepath.Join(opts.TaskDir, ".control"), "file")
				_, err := c.planningQueuePending()
				require.Error(t, err)
			case "list-failed":
				listPlanningDirectory = func(*os.Root) ([]os.DirEntry, error) { return nil, os.ErrPermission }
			case "malformed":
				writeFile(t, pending, "{broken")
			case "collision", "collision-write-failed":
				require.NoError(t, planning.ApplyFeedback(c.State.Planning, planning.Feedback{RequestID: entry.Command.RequestID, Text: "Different original observation", Source: "control:earlier", At: state.NowISO()}))
				c.save()
				require.NoError(t, c.err())
			case "stale-write-failed", "rejection-save-failed":
				entry.InputRevision++
				raw, err := json.Marshal(entry)
				require.NoError(t, err)
				writeFile(t, pending, string(raw))
			case "rejected-policy", "rejected-policy-write-failed":
				entry.Command.Policy = "delegated"
				raw, err := json.Marshal(entry)
				require.NoError(t, err)
				writeFile(t, pending, string(raw))
			}
			if strings.Contains(scenario, "write-failed") {
				writePlanningBytes = func(*os.Root, string, []byte) error { return os.ErrPermission }
			}
			if strings.Contains(scenario, "save-failed") {
				require.NoError(t, os.Remove(filepath.Join(opts.TaskDir, ".state.lock")))
				require.NoError(t, os.Mkdir(filepath.Join(opts.TaskDir, ".state.lock"), 0700))
			}
			err := c.applyPlanningQueue()
			if scenario == "collision" || scenario == "rejected-policy" {
				require.NoError(t, err)
				require.Len(t, c.State.Inbox, 1)
				require.NoFileExists(t, pending)
			} else {
				require.Error(t, err)
			}
			if strings.Contains(scenario, "write-failed") || strings.Contains(scenario, "save-failed") {
				require.FileExists(t, pending, "failed cleanup must retain the original request for owner recovery")
			}
		})
	}
}

func TestPlanningControlRootUsesConfinedDirectoriesAndFilesystemFailures(t *testing.T) {
	for _, scenario := range []string{"open-failed", "mkdir-failed", "stat-failed", "replaced-child"} {
		t.Run(scenario, func(t *testing.T) {
			opts, _, _ := controlPlanningFixture(t)
			oldOpen, oldMkdir, oldStat := openPlanningTask, mkdirPlanningDirectory, statPlanningPath
			t.Cleanup(func() { openPlanningTask, mkdirPlanningDirectory, statPlanningPath = oldOpen, oldMkdir, oldStat })
			switch scenario {
			case "open-failed":
				openPlanningTask = func(string) (*os.Root, error) { return nil, os.ErrPermission }
			case "mkdir-failed":
				mkdirPlanningDirectory = func(*os.Root, string, os.FileMode) error { return os.ErrPermission }
			case "stat-failed":
				statPlanningPath = func(*os.Root, string) (os.FileInfo, error) { return nil, os.ErrPermission }
			case "replaced-child":
				statPlanningPath = func(root *os.Root, name string) (os.FileInfo, error) {
					info, err := oldStat(root, name)
					if err == nil && name == ".control/planning" {
						require.NoError(t, root.Remove(name))
						require.NoError(t, root.WriteFile(name, []byte("replacement"), 0600))
					}
					return info, err
				}
			}
			_, err := planningControlRoot(opts.TaskDir, true)
			require.Error(t, err)
		})
	}
}

func TestReviewedParallelQueueRejectsDamagedControlDirectoriesBeforePreparingExecution(t *testing.T) {
	for _, path := range []string{".control", ".control/planning"} {
		t.Run(path, func(t *testing.T) {
			fx, c := newReviewedFixture(t)
			require.NoError(t, c.activatePlanningPlans())
			require.NoError(t, c.admitReviewedExecution(), "the undamaged task has valid execution authority")
			packet := c.PacketList[0]
			beforePacket := jsonClone(c.State.Packets[packet.PacketID])
			beforeState := mustReadFile(t, state.StateFile(fx.taskDir))
			beforeWorktrees := gitCmd(t, fx.root, "worktree", "list", "--porcelain")
			head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
			writeFile(t, filepath.Join(fx.taskDir, path), "retain this obstruction")
			err := c.prepareParallelPacket(packet, head)
			require.ErrorContains(t, err, "planning control requires real directories")
			require.Equal(t, beforePacket, c.State.Packets[packet.PacketID], "a failed queue check cannot prepare policy or worktree state")
			require.Equal(t, beforeState, mustReadFile(t, state.StateFile(fx.taskDir)))
			require.Equal(t, beforeWorktrees, gitCmd(t, fx.root, "worktree", "list", "--porcelain"))
			require.Equal(t, head, strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD")))
			require.NoDirExists(t, filepath.Join(fx.root, "build"))
			require.Equal(t, "retain this obstruction", string(mustReadFile(t, filepath.Join(fx.taskDir, path))))
		})
	}
}
