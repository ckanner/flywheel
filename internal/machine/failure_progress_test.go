package machine

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/circuitbreaker"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func failingProgressFixture(t *testing.T) (fixture, *Context, *contract.Packet) {
	t.Helper()
	fx := newGitFixture(t, "source.go")
	packet := strings.Replace(fixturePacket("M01-T01", "R-01"), "test -f build/M01-T01.txt", "printf failure; exit 1", 1)
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), packet)
	gitCmd(t, fx.root, "add", "docs/minitask/task-packet-M01-T01.md")
	gitCmd(t, fx.root, "commit", "-qm", "test: fixed failing acceptance")
	c := unitTestContext(t, fx)
	return fx, c, c.Packets["M01-T01"]
}

func TestFailureProgressDoesNotSpinOnChangedCandidateInSameFile(t *testing.T) {
	fx, c, p := failingProgressFixture(t)
	for i, content := range []string{"first implementation\n", "revised implementation\n", "revised implementation\n"} {
		writeFile(t, filepath.Join(fx.root, "source.go"), content)
		gate := c.l1Step(p)
		require.False(t, gate.Pass)
		result := c.failAttempt(p, errL1Failed, []string{p.Acceptance.L1[0].Command()}, gate.Boundary.ChangedFiles)
		if i < 2 {
			assert.Equal(t, "failed", result)
		} else {
			assert.Equal(t, "spin", result, "unchanged code plus the same complete failure is early-stop evidence")
		}
		assessment := c.State.Packets[p.PacketID].LastFeedback.(map[string]any)["progress"].(circuitbreaker.ProgressAssessment)
		if i == 1 {
			assert.Equal(t, "unknown", assessment.Status)
			assert.Contains(t, assessment.Reason, "candidate changed")
		}
	}
	assert.Equal(t, 3, c.State.Packets[p.PacketID].FailedAttempts, "progress never resets total attempts")
	stored := loadFixtureState(t, fx).Packets[p.PacketID].LastFeedback.(map[string]any)
	assert.Equal(t, "stalled", stored["progress"].(map[string]any)["status"])
	assert.NotEmpty(t, stored["progress-evidence"].(map[string]any)["candidate"])
}

func TestFailureProgressEnvironmentAndDiagnosticChangesRemainUnknown(t *testing.T) {
	_, c, p := failingProgressFixture(t)
	c.Config.Verify.EnvAllowlist = []string{"FLYWHEEL_PROGRESS_ENV"}
	t.Setenv("FLYWHEEL_PROGRESS_ENV", "one")
	first := c.l1Step(p)
	require.False(t, first.Pass)
	assert.Equal(t, "failed", c.failAttempt(p, errL1Failed, []string{p.Acceptance.L1[0].Command()}, first.Boundary.ChangedFiles))
	t.Setenv("FLYWHEEL_PROGRESS_ENV", "two")
	second := c.l1Step(p)
	require.False(t, second.Pass)
	assert.Equal(t, "failed", c.failAttempt(p, errL1Failed, []string{p.Acceptance.L1[0].Command()}, second.Boundary.ChangedFiles))
	assessment := c.State.Packets[p.PacketID].LastFeedback.(map[string]any)["progress"].(circuitbreaker.ProgressAssessment)
	assert.Equal(t, "unknown", assessment.Status)
	assert.Contains(t, assessment.Reason, "environment changed")
}

func TestSingleExecuteStopsOnProvenUnchangedL1Failure(t *testing.T) {
	fx, _, _ := failingProgressFixture(t)
	opts := stepOpts(fx, happyDrivers().set())
	opts.Only, opts.PacketID = "execute", "M01-T01"
	summary, err := RunSingleStep(opts)
	require.NoError(t, err)
	require.NotNil(t, summary.Step.Detail)
	assert.False(t, summary.Step.OK)
	assert.Contains(t, *summary.Step.Detail, "spin detected")
	assert.Equal(t, 2, loadFixtureState(t, fx).Packets["M01-T01"].FailedAttempts)
}

func TestProgressCollectionRequiresValidAuthoritativeEvidence(t *testing.T) {
	_, c, p := failingProgressFixture(t)
	ps := c.State.Packets[p.PacketID]
	assert.False(t, c.failureProgress(p, errs.TRANSPORT).Complete)
	ps.Evidence.L1 = make(chan int)
	assert.False(t, c.failureProgress(p, errL1Failed).Complete)
	ps.Evidence.L1 = map[string]any{"identity-error": 3}
	assert.False(t, c.failureProgress(p, errL1Failed).Complete)
	ps.Evidence.L1 = nil
	assert.False(t, c.failureProgress(p, errL1Failed).Complete)

	require.False(t, c.l1Step(p).Pass)
	observation := c.failureProgress(p, errL1Failed)
	require.True(t, observation.Complete)
	require.Len(t, observation.Checks, 2)
	assert.Equal(t, "boundary/v1", observation.Checks[1].ID)
	assert.NotEmpty(t, observation.Checks[0].Failure)
	raw, err := json.Marshal(ps.Evidence.L1)
	require.NoError(t, err)
	var restored map[string]any
	require.NoError(t, json.Unmarshal(raw, &restored))
	ps.Evidence.L1 = restored
	assert.Equal(t, observation, c.failureProgress(p, errL1Failed), "state reload must preserve comparison facts")

	restored["boundary"] = verify.BoundaryReport{Pass: false, Violations: []verify.BoundaryViolation{{Type: "must-not-touch", Path: "frozen.test"}}}
	withBoundary := c.failureProgress(p, errL1Failed)
	assert.NotEmpty(t, withBoundary.Checks[1].Failure)
	restored["commands"] = []verify.GateResult{{CheckID: "test", Pass: false, ObservationComplete: false}}
	assert.False(t, c.failureProgress(p, errL1Failed).Complete)
	assert.False(t, c.failureProgress(p, errReviewReturned).Complete, "a failed L1 cannot produce independent review evidence")
}

func TestReviewProgressUsesIndependentCurrentVerdictWithoutVolatileTime(t *testing.T) {
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	p := c.Packets["M01-T01"]
	writeFile(t, filepath.Join(fx.root, "build", p.PacketID+".txt"), "ok\n")
	require.True(t, c.l1Step(p).Pass)
	ps := c.State.Packets[p.PacketID]
	ps.Evidence.Review = map[string]any{
		"identity": evidenceIdentity(ps.Evidence.L1), "verdict": "return", "reasons": []string{"missing assertion"}, "at": "first",
	}
	first := c.failureProgress(p, errReviewReturned)
	require.True(t, first.Complete)
	require.Len(t, first.Checks, 3)
	assert.Equal(t, "review/v1", first.Checks[2].ID)
	ps.Evidence.Review.(map[string]any)["at"] = "later"
	assert.Equal(t, first, c.failureProgress(p, errReviewReturned), "timestamps are not new failure facts")
	ps.Evidence.Review.(map[string]any)["reasons"] = []string{"new finding"}
	assert.NotEqual(t, first.Checks[2].Failure, c.failureProgress(p, errReviewReturned).Checks[2].Failure)
	ps.Evidence.Review.(map[string]any)["identity"] = "stale"
	assert.False(t, c.failureProgress(p, errReviewReturned).Complete)
	ps.Evidence.Review = nil
	assert.False(t, c.failureProgress(p, errReviewReturned).Complete)
}

func TestUnknownFailureStillRecordsAttemptsWithoutInventingChanges(t *testing.T) {
	c := unitTestContext(t)
	p := c.Packets["M01-T01"]
	assert.Equal(t, "failed", c.failAttempt(p, errs.TRANSPORT, nil, nil))
	ps := c.State.Packets[p.PacketID]
	assert.Equal(t, 1, ps.FailedAttempts)
	assert.Contains(t, *ps.LastFailureSignature, `"files":[]`)
	assessment := ps.LastFeedback.(map[string]any)["progress"].(circuitbreaker.ProgressAssessment)
	assert.Equal(t, "unknown", assessment.Status)
	assert.Nil(t, ps.Evidence.L1, "failure observations never fabricate a verification result")
}

func TestQuickProgressCannotBypassExistingPersistenceError(t *testing.T) {
	c := unitTestContext(t)
	c.fatal = errors.New("state persistence failed")
	upgraded, err := c.runQuick()
	assert.False(t, upgraded)
	require.ErrorIs(t, err, c.fatal)
}
