package machine

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func blockedWebInvocation(t *testing.T) (fixture, *Context, drivers.CallArgs, drivers.Result) {
	t.Helper()
	fx, c := sessionFixture(t)
	a := sessionArgs(c)
	a.Model, a.Binding.Model, a.Binding.Provider, a.Binding.Thinking = drivers.DSHWebModel, drivers.DSHWebModel, "chatgpt", "pro"
	var completed drivers.Result
	_, err := c.invokeScoped("review", "M01-T01", a, func(call drivers.CallArgs) drivers.Result {
		require.NotEmpty(t, call.Web.RecoveryDirectory)
		completed = webSessionResult(call)
		completed.Session.Protocol = drivers.DSHSessionProtocol
		completed.Text = "original SDK completion"
		lost, lifecycle := completed, *completed.Session
		lost.Session = &lifecycle
		lost.Session.Outcome, lost.Session.Dispatch, lost.Session.CleanupConfirmed = "unknown", "unknown", false
		lost.Session.ResumeCandidate, lost.Settled = false, false
		return lost
	})
	require.Error(t, err)
	old := recoverWebSession
	t.Cleanup(func() { recoverWebSession = old })
	recoverWebSession = func(query drivers.DSHWebRecoveryArgs) (drivers.Result, error) {
		require.Equal(t, completed.Session.RequestID, query.RequestID)
		require.Equal(t, "chatgpt", query.Provider)
		require.Equal(t, "flywheel-"+sessionHash([]any{"web-domain-v1", c.TaskDir}), query.ExecutionDomainID)
		return completed, nil
	}
	c = restartSessionGuardContext(t, fx, c)
	return fx, c, a, completed
}

func TestWebSessionRecoveryKeepsOriginalCandidateAndUnknownUsageWithoutDispatch(t *testing.T) {
	fx, c, a, completed := blockedWebInvocation(t)
	c = restartSessionGuardContext(t, fx, c)
	got, err := c.invokeScoped("review", "M01-T01", a, func(drivers.CallArgs) drivers.Result {
		t.Fatal("read-only recovery cannot dispatch another model")
		return drivers.Result{}
	})
	require.NoError(t, err)
	assert.True(t, got.Recovered)
	assert.Equal(t, completed.Text, got.Text)
	assert.Nil(t, got.Usage)
	assert.Equal(t, "received", c.sessionRecord("review", "M01-T01").Last.Status)
	applySessionResult(t, c, "review", "M01-T01", got)
	c = restartSessionGuardContext(t, fx, c)
	require.NoError(t, c.reconcileSessionReceipts())
	assert.Equal(t, int64(1), c.State.Budget.UnknownUsageCalls)
	assert.Equal(t, int64(1), c.State.Budget.UnknownCostCalls)
}

func TestWebSessionRecoveryRejectsMissingOrChangedOriginalEvidence(t *testing.T) {
	for _, scenario := range []string{"contract", "binding", "pending", "unknown", "thread", "receipt-lost", "checkpoint-lost", "candidate", "guard", "snapshot", "serialize", "persist", "result-persist"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, _, completed := blockedWebInvocation(t)
			r := c.sessionRecord("review", "M01-T01")
			switch scenario {
			case "contract":
				r.Last.Recovery.AdapterVersion = "foreign"
			case "binding":
				r.WebBinding = json.RawMessage(`{`)
			case "pending":
				recoverWebSession = func(drivers.DSHWebRecoveryArgs) (drivers.Result, error) {
					return drivers.Result{}, errors.New("adopted tool is not an SDK terminal")
				}
			case "unknown":
				recoverWebSession = func(drivers.DSHWebRecoveryArgs) (drivers.Result, error) { return drivers.Result{}, nil }
			case "thread":
				r.Last.RequestedThread = "different-history"
			case "receipt-lost":
				old := loadSessionReceipt
				t.Cleanup(func() { loadSessionReceipt = old })
				loadSessionReceipt = func(string, string, string, string) (*state.SessionReceipt, string, error) {
					return nil, "", os.ErrNotExist
				}
			case "checkpoint-lost":
				r.Last.Recovery.CheckpointDigest = r.Last.ReceiptDigest
			case "candidate":
				writeFile(t, filepath.Join(fx.root, "changed-after-exit.txt"), "changed\n")
			case "guard":
				delete(c.Packets, "M01-T01")
			case "snapshot":
				old := snapshotOpen
				t.Cleanup(func() { snapshotOpen = old })
				snapshotOpen = func(string) (io.ReadCloser, error) { return nil, os.ErrPermission }
			case "serialize":
				completed.CostUSD = new(math.NaN())
				recoverWebSession = func(drivers.DSHWebRecoveryArgs) (drivers.Result, error) { return completed, nil }
			case "persist":
				old := persistSessionReceipt
				t.Cleanup(func() { persistSessionReceipt = old })
				persistSessionReceipt = func(string, string, state.SessionReceipt) (string, error) { return "", os.ErrPermission }
			case "result-persist":
				old := persistSessionReceipt
				t.Cleanup(func() { persistSessionReceipt = old })
				persistSessionReceipt = func(task, key string, receipt state.SessionReceipt) (string, error) {
					if key == "review/M01-T01" {
						return "", os.ErrPermission
					}
					return old(task, key, receipt)
				}
			}
			require.Error(t, c.recoverSessionReceipt("review/M01-T01", r))
			assert.Equal(t, "blocked", r.Last.Status)
			assert.False(t, r.ResumeCandidate)
		})
	}
}

func TestWebUnsentRecoveryCanCloseMissingReceiptOnlyWithOriginalCandidate(t *testing.T) {
	for _, changed := range []bool{false, true} {
		fx, c, _, failed := blockedWebInvocation(t)
		r := c.sessionRecord("review", "M01-T01")
		failed.Session.Dispatch, failed.Session.Outcome, failed.Session.ResumeCandidate = "not-dispatched", "failed", false
		recoverWebSession = func(drivers.DSHWebRecoveryArgs) (drivers.Result, error) { return failed, nil }
		old := loadSessionReceipt
		t.Cleanup(func() { loadSessionReceipt = old })
		loadSessionReceipt = func(string, string, string, string) (*state.SessionReceipt, string, error) {
			return nil, "", os.ErrNotExist
		}
		if changed {
			writeFile(t, filepath.Join(fx.root, "changed-unsent.txt"), "changed\n")
			require.ErrorContains(t, c.recoverSessionReceipt("review/M01-T01", r), "original candidate")
		} else {
			require.NoError(t, c.recoverSessionReceipt("review/M01-T01", r))
			receipt, _, err := old(c.TaskDir, "review/M01-T01", r.Last.ID, r.Last.ReceiptDigest)
			require.NoError(t, err)
			assert.Equal(t, r.Last.CandidateBefore, receipt.CandidateAfter)
			assert.Equal(t, r.Last.GuardBefore, receipt.GuardAfter)
		}
		loadSessionReceipt = old
	}
}

func TestWebRecoveryPreparationDefaultsAndStorageErrors(t *testing.T) {
	_, c := sessionFixture(t)
	a := sessionArgs(c)
	a.Web = &drivers.DSHWebRequest{Version: 1, ExecutionDomainID: "domain", RequireIsolation: true}
	a.PermissionMode = ""
	r := &state.SessionRecord{Last: &state.SessionInvocation{ID: "original"}}
	require.NoError(t, c.prepareSessionLifecycle(r, &a))
	assert.Equal(t, "read-only", r.Last.Recovery.PermissionMode)
	assert.Equal(t, drivers.DSHWebRecoveryVersion, r.Last.Recovery.AdapterVersion)
	assert.Equal(t, state.SessionRecoveryDirectory(c.TaskDir, "original"), a.Web.RecoveryDirectory)
	old := prepareSessionRecovery
	t.Cleanup(func() { prepareSessionRecovery = old })
	prepareSessionRecovery = func(string, string) (string, error) { return "", os.ErrPermission }
	require.ErrorContains(t, c.prepareSessionLifecycle(r, &a), "durable Web")
}

func TestWebRecoveryOriginalCheckpointSurvivesEveryReceiptCASWindow(t *testing.T) {
	for _, stage := range []string{"checkpoint-unacknowledged", "checkpoint-acknowledged", "replacement-unacknowledged"} {
		t.Run(stage, func(t *testing.T) {
			fx, c, _, completed := blockedWebInvocation(t)
			r := c.sessionRecord("review", "M01-T01")
			originalDigest := r.Last.ReceiptDigest
			original, _, err := state.ReadSessionReceipt(c.TaskDir, "review/M01-T01", r.Last.ID, originalDigest)
			require.NoError(t, err)
			old := persistSessionReceipt
			t.Cleanup(func() { persistSessionReceipt = old })
			if stage == "checkpoint-unacknowledged" {
				persistSessionReceipt = func(task, key string, receipt state.SessionReceipt) (string, error) {
					digest, err := old(task, key, receipt)
					if key == "review/M01-T01/web-original" && err == nil {
						bumpFaultRevision(t, fx)
					}
					return digest, err
				}
			} else if stage == "checkpoint-acknowledged" {
				persistSessionReceipt = func(task, key string, receipt state.SessionReceipt) (string, error) {
					if key == "review/M01-T01" {
						return "", errors.New("crashed before replacing result")
					}
					return old(task, key, receipt)
				}
			}
			err = c.recoverWebSessionReceipt("review/M01-T01", r)
			if stage == "replacement-unacknowledged" {
				require.NoError(t, err)
				// Replacement is durable, but state still has the old result
				// digest. The separately acknowledged original remains intact.
				bumpFaultRevision(t, fx)
				c.save()
				require.Error(t, c.err())
			} else {
				require.Error(t, err)
				unchanged, _, readErr := state.ReadSessionReceipt(c.TaskDir, "review/M01-T01", r.Last.ID, originalDigest)
				require.NoError(t, readErr)
				assert.Equal(t, original, unchanged)
			}
			persistSessionReceipt = old
			c = restartSessionGuardContext(t, fx, c)
			require.NoError(t, c.reconcileSessionReceipts(), "every durable transition can retry without another model call")
			r = c.sessionRecord("review", "M01-T01")
			assert.Equal(t, "received", r.Last.Status)
			assert.NotEmpty(t, r.Last.Recovery.CheckpointDigest)
			preserved, _, err := state.ReadSessionReceipt(c.TaskDir, "review/M01-T01/web-original", r.Last.ID, r.Last.Recovery.CheckpointDigest)
			require.NoError(t, err)
			assert.Equal(t, original.CandidateAfter, preserved.CandidateAfter)
			assert.Equal(t, original.GuardBefore, preserved.GuardBefore)
			assert.Equal(t, original.GuardAfter, preserved.GuardAfter)
			_, result, _, err := c.readSessionResult("review/M01-T01", r)
			require.NoError(t, err)
			assert.Equal(t, completed.Text, result.Text)
			assert.True(t, result.Recovered)
		})
	}
}
