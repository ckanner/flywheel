package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeliveryUsesExactHeadAndReconcilesAlreadyCreatedOrMergedPR(t *testing.T) {
	fx := newGitFixture(t)
	opts := runOpts(fx, happyDrivers().set())
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
	var pulls []PullRequest
	pushes, creates, merges := 0, 0, 0
	old := deliveryRun
	t.Cleanup(func() { deliveryRun = old })
	deliveryRun = func(_ context.Context, _, name string, args ...string) (string, error) {
		command := strings.Join(args, " ")
		switch {
		case name == "git" && (command == "remote get-url origin" || command == "remote get-url --push --all origin"):
			return "git@github.com:example/fixture.git", nil
		case name == "git" && strings.HasPrefix(command, "check-ref-format --branch"):
			return "feature", nil
		case name == "git" && command == "rev-parse HEAD":
			return head, nil
		case name == "git" && strings.HasPrefix(command, "symbolic-ref"):
			return "feature", nil
		case name == "gh" && strings.HasPrefix(command, "repo view"):
			return `{"defaultBranchRef":{"name":"main"}}`, nil
		case name == "gh" && strings.HasPrefix(command, "pr list"):
			raw, _ := json.Marshal(pulls)
			return string(raw), nil
		case name == "git" && strings.Contains(command, "push git@github.com:example/fixture.git"):
			pushes++
			require.Equal(t, []string{"-c", "push.followTags=false", "-c", "remote.origin.mirror=false", "-c", "push.recurseSubmodules=no", "push", "git@github.com:example/fixture.git", head + ":refs/heads/feature"}, args)
			return "ok", nil
		case name == "gh" && args[0] == "pr" && args[1] == "create":
			creates++
			body, err := os.ReadFile(args[len(args)-1])
			require.NoError(t, err)
			require.Equal(t, "line one\n`literal $(no execution)`", string(body))
			pulls = []PullRequest{{Number: 7, URL: "https://github.com/example/fixture/pull/7", State: "OPEN", Head: head, Base: "main", MergeState: "CLEAN"}}
			return pulls[0].URL, nil
		case name == "gh" && args[0] == "pr" && args[1] == "merge":
			merges++
			require.Equal(t, head, args[len(args)-1])
			require.Equal(t, "--match-head-commit", args[len(args)-2])
			pulls[0].State = "MERGED"
			return "merged", nil
		default:
			return "", fmt.Errorf("unexpected command %s %v", name, args)
		}
	}
	snapshot, err := InspectDelivery(context.Background(), fx.root)
	require.NoError(t, err)
	require.Equal(t, "github.com/example/fixture", snapshot.Repo)
	request := DeliveryRequest{Action: "create", Confirmed: true, Revision: loadFixtureState(t, fx).Revision, ExpectedHead: head, ExpectedRepo: snapshot.Repo, Base: "main", Title: "Implement fixture", Body: "line one\n`literal $(no execution)`"}
	snapshot, err = Deliver(opts.RunOpts, request)
	require.NoError(t, err)
	require.Len(t, snapshot.Pulls, 1)
	require.Equal(t, "confirmed", loadFixtureState(t, fx).Delivery.Status)
	request.Revision = loadFixtureState(t, fx).Revision
	_, err = Deliver(opts.RunOpts, request)
	require.NoError(t, err)
	require.Equal(t, 1, pushes)
	require.Equal(t, 1, creates)
	request.Action, request.Number = "merge", 7
	request.Revision = loadFixtureState(t, fx).Revision
	_, err = Deliver(opts.RunOpts, request)
	require.NoError(t, err)
	require.Equal(t, 1, merges)
	request.Revision = loadFixtureState(t, fx).Revision
	_, err = Deliver(opts.RunOpts, request)
	require.NoError(t, err)
	require.Equal(t, 1, merges)
	// A new local candidate invalidates delivery even if the prior run was green.
	writeFile(t, filepath.Join(fx.root, "new-code.go"), "package newcode\n")
	gitCmd(t, fx.root, "add", "new-code.go")
	gitCmd(t, fx.root, "commit", "-q", "-m", "feat: unrelated change")
	request.Revision = loadFixtureState(t, fx).Revision
	_, err = Deliver(opts.RunOpts, request)
	require.ErrorContains(t, err, "candidate changed")
}

func TestDeliveryRemoteParsingDoesNotLeakCredentialsOrGuessRepositories(t *testing.T) {
	for _, remote := range []string{"git@github.com:owner/repo.git", "https://github.com/owner/repo", "ssh://git@github.com/owner/repo.git"} {
		repo, url, err := githubRemote(remote)
		require.NoError(t, err)
		require.Equal(t, "github.com/owner/repo", repo)
		require.Equal(t, "https://github.com/owner/repo", url)
	}
	for _, remote := range []string{"/local/repo", "http://github.com/a/b", "https://token@github.com/a/b", "https://github.com/a/b?query=secret", "https://github.com/a", "https://github.com/a/b/c", "%bad"} {
		_, _, err := githubRemote(remote)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
	_, err := Deliver(RunOpts{}, DeliveryRequest{})
	require.Error(t, err)
}
