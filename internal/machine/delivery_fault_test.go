package machine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

type remoteFixture struct {
	head, repo, push, branch string
	pulls                    []PullRequest
	pushes, creates, merges  int
	queued                   bool
	hook                     func(string) error
}

func (f *remoteFixture) run(_ context.Context, _, name string, args ...string) (string, error) {
	command := name + " " + strings.Join(args, " ")
	if f.hook != nil {
		if err := f.hook(command); err != nil {
			return "", err
		}
	}
	switch {
	case command == "git remote get-url origin":
		return f.repo, nil
	case command == "git remote get-url --push --all origin":
		return f.push, nil
	case command == "git rev-parse HEAD":
		return f.head, nil
	case strings.HasPrefix(command, "git symbolic-ref"):
		return f.branch, nil
	case strings.HasPrefix(command, "git check-ref-format"):
		return args[len(args)-1], nil
	case strings.HasPrefix(command, "gh repo view"):
		return `{"defaultBranchRef":{"name":"main"}}`, nil
	case strings.HasPrefix(command, "gh pr list"):
		raw, _ := json.Marshal(f.pulls)
		return string(raw), nil
	case name == "git" && strings.Contains(command, " push "):
		f.pushes++
		for i := range f.pulls {
			if f.pulls[i].State == "OPEN" {
				f.pulls[i].Head = f.head
			}
		}
		return "pushed", nil
	case strings.HasPrefix(command, "gh pr create"):
		f.creates++
		f.pulls = append(f.pulls, PullRequest{Number: 9, URL: "https://github.com/example/repo/pull/9", State: "OPEN", Head: f.head, Base: "main", MergeState: "CLEAN"})
		return "created", nil
	case strings.HasPrefix(command, "gh pr merge"):
		f.merges++
		if !f.queued {
			f.pulls[0].State = "MERGED"
			f.pulls[0].MergeCommit = &struct {
				OID string `json:"oid"`
			}{OID: f.head}
		}
		return "accepted", nil
	}
	return "", fmt.Errorf("unexpected fixture command: %s", command)
}
func deliveryFixture(t *testing.T) (fixture, RunOpts, *remoteFixture) {
	fx := newGitFixture(t)
	opts := runOpts(fx, happyDrivers().set())
	_, err := RunTask(opts)
	require.NoError(t, err)
	f := &remoteFixture{head: strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD")), repo: "git@github.com:example/repo.git", push: "git@github.com:example/repo.git", branch: "feature"}
	old := deliveryRun
	t.Cleanup(func() { deliveryRun = old })
	deliveryRun = f.run
	return fx, opts.RunOpts, f
}
func deliveryRequestFor(t *testing.T, fx fixture, f *remoteFixture) DeliveryRequest {
	return DeliveryRequest{Action: "create", Confirmed: true, Revision: loadFixtureState(t, fx).Revision, ExpectedHead: f.head, ExpectedRepo: "github.com/example/repo", Base: "main", Title: "fixture", Body: "reviewable body"}
}

func TestDeliveryInspectionRejectsIncompleteIdentityAndRemoteDisagreement(t *testing.T) {
	f := &remoteFixture{head: "head", repo: "git@github.com:example/repo.git", push: "git@github.com:example/repo.git", branch: "feature"}
	old := deliveryRun
	t.Cleanup(func() { deliveryRun = old })
	for _, command := range []string{"remote get-url origin", "remote get-url --push --all origin", "rev-parse HEAD", "symbolic-ref", "repo view", "pr list", "check-ref-format"} {
		f.hook = func(value string) error {
			if strings.Contains(value, command) {
				return errors.New("unavailable")
			}
			return nil
		}
		deliveryRun = f.run
		_, err := InspectDelivery(context.Background(), t.TempDir(), "feature")
		require.Error(t, err, command)
	}
	f.hook = nil
	for _, value := range []string{"https://example.com/a/b\nhttps://example.com/c/d", "not-a-remote", "https://github.com/other/repo"} {
		f.push = value
		deliveryRun = f.run
		_, err := InspectDelivery(context.Background(), t.TempDir())
		require.Error(t, err)
	}
	f.push = f.repo
	f.repo = "not-a-remote"
	_, err := InspectDelivery(context.Background(), t.TempDir())
	require.Error(t, err)
	f.repo = f.push
	for _, branch := range []string{"--force", "bad\nbranch"} {
		_, err := InspectDelivery(context.Background(), t.TempDir(), branch)
		require.Error(t, err)
	}
	for _, command := range []string{"repo view", "pr list"} {
		deliveryRun = func(ctx context.Context, cwd, name string, args ...string) (string, error) {
			if strings.Contains(strings.Join(args, " "), command) {
				return "{", nil
			}
			return f.run(ctx, cwd, name, args...)
		}
		_, err := InspectDelivery(context.Background(), t.TempDir())
		require.Error(t, err)
	}
}

func TestDeliveryCommandOwnsItsPhysicalTerminalBoundary(t *testing.T) {
	output, err := deliveryRun(context.Background(), t.TempDir(), "gh", "--version")
	require.NoError(t, err)
	require.Contains(t, output, "gh version")
	_, err = deliveryRun(context.Background(), t.TempDir(), "does-not-exist-flywheel")
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = deliveryRun(ctx, t.TempDir(), "sh", "-c", "sleep 1")
	require.Error(t, err)
	_, err = deliveryRun(context.Background(), t.TempDir(), "head", "-c", "1048577", "/dev/zero")
	require.Error(t, err)
}

func TestDeliveryAdmissionsRefuseStaleOrUnverifiedTargets(t *testing.T) {
	fx, opts, f := deliveryFixture(t)
	for _, kind := range []string{"lease", "revision", "handoff", "head", "repo", "frozen-repo", "title", "base", "merge"} {
		t.Run(kind, func(t *testing.T) {
			request := deliveryRequestFor(t, fx, f)
			switch kind {
			case "lease":
				release, err := AcquireRunLease(fx.taskDir)
				require.NoError(t, err)
				defer release()
			case "revision":
				request.Revision++
			case "handoff":
				st := loadFixtureState(t, fx)
				st.Handoff = &state.Handoff{Key: "execute/M01-T01", Thread: "thread", Cwd: fx.root, Packet: "M01-T01", Candidate: strings.Repeat("a", 64), StartedAt: state.NowISO(), Reason: "test"}
				_, err := state.Save(fx.taskDir, st, nil)
				require.NoError(t, err)
				request.Revision = loadFixtureState(t, fx).Revision
				t.Cleanup(func() {
					st := loadFixtureState(t, fx)
					st.Handoff = nil
					_, err := state.Save(fx.taskDir, st, nil)
					require.NoError(t, err)
				})
			case "head":
				request.ExpectedHead = "different"
			case "repo":
				request.ExpectedRepo = "github.com/other/repo"
			case "frozen-repo":
				st := loadFixtureState(t, fx)
				st.Project = &state.ProjectIdentity{Cwd: fx.root, Repo: "github.com/original/repo"}
				_, err := state.Save(fx.taskDir, st, nil)
				require.NoError(t, err)
				request.Revision = loadFixtureState(t, fx).Revision
				t.Cleanup(func() {
					st := loadFixtureState(t, fx)
					st.Project = nil
					_, err := state.Save(fx.taskDir, st, nil)
					require.NoError(t, err)
				})
			case "title":
				request.Title = ""
			case "base":
				request.Base = "feature"
			case "merge":
				request.Action, request.Number = "merge", 9
			}
			_, err := Deliver(opts, request)
			require.Error(t, err)
			require.Zero(t, f.pushes)
		})
	}
	missing := RunOpts{TaskDir: t.TempDir()}
	request := DeliveryRequest{Action: "create", Confirmed: true, Revision: 1, ExpectedHead: "h", ExpectedRepo: "r"}
	_, err := Deliver(missing, request)
	require.Error(t, err)
	st := loadFixtureState(t, fx)
	st.Status = "paused"
	_, err = state.Save(fx.taskDir, st, nil)
	require.NoError(t, err)
	_, err = Deliver(opts, deliveryRequestFor(t, fx, f))
	require.ErrorContains(t, err, "complete the local task")
}

func TestDeliveryReconcilesDraftUpdatedByPushAndRecordsQueuedMerge(t *testing.T) {
	fx, opts, f := deliveryFixture(t)
	f.pulls = []PullRequest{{Number: 9, URL: "https://github.com/example/repo/pull/9", State: "OPEN", Head: "prior-contracts", Base: "main", Draft: true, MergeState: "DRAFT"}}
	_, err := Deliver(opts, deliveryRequestFor(t, fx, f))
	require.NoError(t, err)
	require.Equal(t, 1, f.pushes)
	require.Zero(t, f.creates, "pushing updates the existing Draft PR")
	f.pulls[0].Draft = false
	f.pulls[0].MergeState = "CLEAN"
	f.queued = true
	request := deliveryRequestFor(t, fx, f)
	request.Action, request.Number = "merge", 9
	_, err = Deliver(opts, request)
	require.NoError(t, err)
	require.Equal(t, "pending", loadFixtureState(t, fx).Delivery.Status)
	f.queued = false
	request.Revision = loadFixtureState(t, fx).Revision
	_, err = Deliver(opts, request)
	require.NoError(t, err)
	require.Equal(t, f.head, loadFixtureState(t, fx).Delivery.MergeSHA)
}

func TestDeliveryFailuresRetainUnknownReceiptsAndNeverRetarget(t *testing.T) {
	for _, kind := range []string{"inspect", "push", "after-push-read", "after-push-head", "after-push-repo", "create", "merge", "after-create-read", "after-create-repo", "missing-result", "body-directory", "intent-save", "receipt-save"} {
		t.Run(kind, func(t *testing.T) {
			fx, opts, f := deliveryFixture(t)
			request := deliveryRequestFor(t, fx, f)
			if kind == "merge" {
				request.Action, request.Number = "merge", 9
				f.pulls = []PullRequest{{Number: 9, State: "OPEN", Head: f.head, Base: "main", MergeState: "CLEAN"}}
			}
			blocker := state.StateFile(fx.taskDir) + fmt.Sprintf(".tmp-%d", os.Getpid())
			blocked := false
			f.hook = func(command string) error {
				switch kind {
				case "inspect":
					if strings.Contains(command, "remote get-url") {
						return errors.New("offline")
					}
				case "push":
					if strings.Contains(command, " push ") {
						return errors.New("push unconfirmed")
					}
				case "after-push-read":
					if f.pushes > 0 && strings.Contains(command, "remote get-url") {
						return errors.New("read unconfirmed")
					}
				case "after-push-head":
					if f.pushes > 0 {
						f.head = "changed"
					}
				case "after-push-repo":
					if f.pushes > 0 {
						f.repo = "https://github.com/another/repo"
						f.push = f.repo
					}
				case "create":
					if strings.HasPrefix(command, "gh pr create") {
						return errors.New("create unconfirmed")
					}
				case "merge":
					if strings.HasPrefix(command, "gh pr merge") {
						return errors.New("merge unconfirmed")
					}
				case "after-create-read":
					if f.creates > 0 {
						return errors.New("lost receipt")
					}
				case "after-create-repo":
					if f.creates > 0 {
						f.repo = "https://github.com/another/repo"
						f.push = f.repo
					}
				case "missing-result":
					if f.creates > 0 {
						f.pulls = nil
					}
				case "body-directory":
					if f.pushes > 0 && !blocked {
						blocked = true
						require.NoError(t, os.Rename(filepath.Join(fx.taskDir, ".control"), filepath.Join(fx.taskDir, ".control.saved")))
						require.NoError(t, os.WriteFile(filepath.Join(fx.taskDir, ".control"), nil, 0600))
					}
				case "intent-save":
					if strings.Contains(command, "gh pr list") && !blocked {
						blocked = true
						require.NoError(t, os.Mkdir(blocker, 0700))
					}
				case "receipt-save":
					if f.creates > 0 && !blocked {
						blocked = true
						require.NoError(t, os.Mkdir(blocker, 0700))
					}
				}
				return nil
			}
			_, err := Deliver(opts, request)
			require.Error(t, err, kind)
			st := loadFixtureState(t, fx)
			if kind == "inspect" || kind == "intent-save" {
				require.Zero(t, f.pushes)
			} else {
				require.NotNil(t, st.Delivery)
				require.Contains(t, []string{"pending", "unknown"}, st.Delivery.Status)
			}
		})
	}
}

func TestDeliveryRefusesStaleQuickEvidenceAndLocalFailures(t *testing.T) {
	fx := newQuickFixture(t, false)
	opts := runOpts(fx, quickDrivers().set())
	_, err := RunTask(opts)
	require.NoError(t, err)
	c := quickContext(t, fx, quickDrivers())
	require.NoError(t, c.deliveredCandidate())
	c.State.Packets["M01-T01"].Evidence.FinalGate = nil
	require.ErrorContains(t, c.deliveredCandidate(), "stale")
	fx2, opts2, f := deliveryFixture(t)
	writeFile(t, filepath.Join(fx2.root, "dirty.txt"), "uncommitted")
	_, err = Deliver(opts2, deliveryRequestFor(t, fx2, f))
	require.Error(t, err)
	require.NoError(t, os.Remove(filepath.Join(fx2.root, "dirty.txt")))
	original := snapshotGitFiles
	snapshotGitFiles = func(string) ([]byte, error) { return nil, errors.New("inventory failed") }
	_, err = Deliver(opts2, deliveryRequestFor(t, fx2, f))
	require.ErrorContains(t, err, "inventory")
	snapshotGitFiles = original
	spec, err := os.ReadFile(filepath.Join(fx2.taskDir, "spec.md"))
	require.NoError(t, err)
	writeFile(t, filepath.Join(fx2.taskDir, "spec.md"), "invalid")
	_, err = Deliver(opts2, deliveryRequestFor(t, fx2, f))
	require.Error(t, err)
	writeFile(t, filepath.Join(fx2.taskDir, "spec.md"), string(spec))
	require.NoError(t, os.RemoveAll(filepath.Join(fx2.taskDir, ".control")))
	writeFile(t, filepath.Join(fx2.taskDir, ".control"), "not a directory")
	_, err = Deliver(opts2, deliveryRequestFor(t, fx2, f))
	require.Error(t, err)
	require.NoError(t, os.Remove(filepath.Join(fx2.taskDir, ".control")))
	oldWrite := deliveryWriteBody
	t.Cleanup(func() { deliveryWriteBody = oldWrite; snapshotGitFiles = original })
	deliveryWriteBody = func(*os.File, string) (int, error) { return 0, errors.New("body write failed") }
	_, err = Deliver(opts2, deliveryRequestFor(t, fx2, f))
	require.ErrorContains(t, err, "body write failed")
	require.Equal(t, "unknown", loadFixtureState(t, fx2).Delivery.Status)
}

func TestDeliveryPIDPublicationFailurePreventsRemoteAdmission(t *testing.T) {
	fx, opts, f := deliveryFixture(t)
	original := writePIDData
	t.Cleanup(func() { writePIDData = original })
	writePIDData = func(string, []byte, os.FileMode) error { return errors.New("PID publication failed") }
	_, err := Deliver(opts, deliveryRequestFor(t, fx, f))
	require.ErrorContains(t, err, "PID publication failed")
	require.Zero(t, f.pushes)
}
