package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/ckanner/flywheel/internal/watchdog"
)

type RemoteCheck struct {
	Name       string `json:"name"`
	Context    string `json:"context"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
	URL        string `json:"detailsUrl"`
	TargetURL  string `json:"targetUrl"`
}
type PullRequest struct {
	Checks      []RemoteCheck `json:"statusCheckRollup"`
	Number      int           `json:"number"`
	URL         string        `json:"url"`
	State       string        `json:"state"`
	Head        string        `json:"headRefOid"`
	Base        string        `json:"baseRefName"`
	Draft       bool          `json:"isDraft"`
	MergeState  string        `json:"mergeStateStatus"`
	Review      string        `json:"reviewDecision"`
	MergeCommit *struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
}
type DeliverySnapshot struct {
	pushURL     string
	LocalBranch string        `json:"localBranch"`
	Repo        string        `json:"repo"`
	URL         string        `json:"url"`
	Branch      string        `json:"branch"`
	Head        string        `json:"head"`
	DefaultBase string        `json:"defaultBase"`
	Endpoint    string        `json:"endpoint,omitempty"`
	Pulls       []PullRequest `json:"pulls"`
}
type DeliveryRequest struct {
	HeadBranch   string `json:"headBranch,omitempty"`
	Action       string `json:"action"`
	Revision     int64  `json:"revision,omitempty"`
	ExpectedHead string `json:"expectedHead,omitempty"`
	ExpectedRepo string `json:"expectedRepo,omitempty"`
	Confirmed    bool   `json:"confirmed,omitempty"`
	Base         string `json:"base,omitempty"`
	Title        string `json:"title,omitempty"`
	Body         string `json:"body,omitempty"`
	Number       int    `json:"number,omitempty"`
}

var deliveryWriteBody = (*os.File).WriteString

var deliveryRun = func(ctx context.Context, cwd, name string, args ...string) (string, error) {
	allow := []string{}
	if name == "gh" {
		allow = []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_HOST"}
	}
	result := watchdog.Run(name, args, watchdog.Opts{Context: ctx, Cwd: cwd, Env: childenv.EnvSlice(childenv.GateEnv(childenv.OSLookupEnv, allow)), WallTimeoutMs: 30000, IdleTimeoutMs: 30000})
	if result.ExitCode == nil || *result.ExitCode != 0 || result.Cancelled || result.TimedOut || result.Truncated || result.Signal != "" || result.CleanupError != "" || len(result.Stdout) > 1<<20 {
		return "", fmt.Errorf("%s did not confirm a clean terminal result; check its login, remote and repository rules", name)
	}
	return strings.TrimSpace(result.Stdout), nil
}

func githubRemote(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "git@") {
		value = "https://" + strings.Replace(strings.TrimPrefix(value, "git@"), ":", "/", 1)
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && parsed.Scheme != "ssh") {
		return "", "", fmt.Errorf("origin must be an explicit GitHub HTTPS or SSH repository")
	}
	if parsed.User != nil && (parsed.Scheme != "ssh" || parsed.User.String() != "git") {
		return "", "", fmt.Errorf("credential-bearing remote URLs are not supported")
	}
	path := strings.TrimSuffix(strings.Trim(parsed.Path, "/"), ".git")
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(path) {
		return "", "", fmt.Errorf("origin does not identify an owner/repository")
	}
	return parsed.Host + "/" + path, "https://" + parsed.Host + "/" + path, nil
}

func InspectDelivery(ctx context.Context, cwd string, headBranches ...string) (*DeliverySnapshot, error) {
	snapshot := &DeliverySnapshot{}
	origin, err := deliveryRun(ctx, cwd, "git", "remote", "get-url", "origin")
	if err != nil {
		return nil, err
	}
	snapshot.Repo, snapshot.URL, err = githubRemote(origin)
	if err != nil {
		return nil, err
	}
	pushURL, err := deliveryRun(ctx, cwd, "git", "remote", "get-url", "--push", "--all", "origin")
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(pushURL, "\r\n") {
		return nil, fmt.Errorf("delivery requires exactly one push URL")
	}
	pushRepo, _, err := githubRemote(pushURL)
	if err != nil {
		return nil, err
	}
	snapshot.pushURL = pushURL
	if pushRepo != snapshot.Repo {
		return nil, fmt.Errorf("origin fetch and push repositories differ")
	}
	snapshot.Head, err = deliveryRun(ctx, cwd, "git", "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	snapshot.Branch, err = deliveryRun(ctx, cwd, "git", "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return nil, err
	}
	snapshot.LocalBranch = snapshot.Branch
	if len(headBranches) > 0 && headBranches[0] != "" {
		branch := headBranches[0]
		if strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, "\x00\r\n") {
			return nil, fmt.Errorf("invalid remote task branch")
		}
		if _, err := deliveryRun(ctx, cwd, "git", "check-ref-format", "--branch", branch); err != nil {
			return nil, err
		}
		snapshot.Branch = branch
	}
	raw, err := deliveryRun(ctx, cwd, "gh", "repo", "view", snapshot.Repo, "--json", "defaultBranchRef")
	if err != nil {
		return nil, err
	}
	var repo struct {
		DefaultBranchRef struct {
			Name string `json:"name"`
		} `json:"defaultBranchRef"`
	}
	if err := json.Unmarshal([]byte(raw), &repo); err != nil {
		return nil, err
	}
	snapshot.DefaultBase = repo.DefaultBranchRef.Name
	raw, err = deliveryRun(ctx, cwd, "gh", "pr", "list", "--repo", snapshot.Repo, "--head", snapshot.Branch, "--state", "all", "--limit", "30", "--json", "number,url,state,headRefOid,baseRefName,isDraft,mergeStateStatus,reviewDecision,mergeCommit,statusCheckRollup")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), &snapshot.Pulls); err != nil {
		return nil, err
	}
	if snapshot.Pulls == nil {
		snapshot.Pulls = []PullRequest{}
	}
	return snapshot, nil
}

func (c *Context) deliveredCandidate() error {
	if c.State.Status != "completed" || !c.contractsComplete() {
		return fmt.Errorf("complete the local task and its acceptance before remote delivery")
	}
	if _, err := verify.PersistedHead(c.Cwd, c.ledgerExcludes()); err != nil {
		return err
	}
	if c.quick() {
		if !c.currentFinalGate(c.PacketList[0]) || !c.validFinalReview(c.PacketList[0]) {
			return fmt.Errorf("Quick delivery evidence is stale")
		}
		return nil
	}
	candidate, err := c.candidateFingerprint(c.Cwd)
	if err != nil {
		return err
	}
	last := c.Spec.Milestones[len(c.Spec.Milestones)-1]
	if evidenceIdentity(c.State.Milestones[last.ID].Gate) != candidate {
		return fmt.Errorf("final verified candidate changed; repeat acceptance before delivery")
	}
	return nil
}

func Deliver(opts RunOpts, request DeliveryRequest) (snapshot *DeliverySnapshot, err error) {
	if !request.Confirmed || request.Revision < 1 || request.ExpectedHead == "" || request.ExpectedRepo == "" || (request.Action != "create" && request.Action != "merge") {
		return nil, fmt.Errorf("remote writes require explicit confirmation, revision, repository and HEAD")
	}
	release, err := AcquireRunLease(opts.TaskDir)
	if err != nil {
		return nil, err
	}
	defer release()
	st, err := state.Load(opts.TaskDir)
	if err != nil {
		return nil, err
	}
	if st.Revision != request.Revision {
		return nil, fmt.Errorf("task revision changed; refresh before remote delivery")
	}
	parent := opts.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 40*time.Second)
	defer cancel()
	c, err := CreateContext(Opts{TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config, Drivers: opts.Drivers, Context: ctx, Log: opts.Log})
	if err != nil {
		return nil, err
	}
	if c.State.Planning != nil {
		if err := c.loadPlanningContracts(); err != nil {
			return nil, err
		}
		if err := c.admitReviewedExecution(); err != nil {
			return nil, err
		}
	}
	if c.State.Handoff != nil {
		return nil, fmt.Errorf("return the human-controlled session first")
	}
	if err := c.deliveredCandidate(); err != nil {
		return nil, err
	}
	if err := c.writePid(); err != nil {
		return nil, err
	}
	defer clearPid(c.TaskDir)
	signals := registerSigterm(c.Log, cancel)
	defer unregisterSigterm(signals)
	stop := watchStopRequests(ctx, c.TaskDir, c.controlToken, cancel)
	defer stop()
	endpoint := firstNonEmpty(c.Config.Delivery.Endpoint, "local")
	snapshot, err = InspectDelivery(ctx, c.Cwd, request.HeadBranch)
	if err != nil {
		return nil, err
	}
	snapshot.Endpoint = endpoint
	if c.State.Project != nil && c.State.Project.Repo != "" && c.State.Project.Repo != snapshot.Repo {
		return snapshot, fmt.Errorf("repository identity changed since this task was registered")
	}
	if snapshot.Head != request.ExpectedHead || snapshot.Repo != request.ExpectedRepo {
		return snapshot, fmt.Errorf("remote target or local HEAD changed")
	}
	var existing *PullRequest
	for i := range snapshot.Pulls {
		pr := &snapshot.Pulls[i]
		if pr.Head == snapshot.Head && (request.Action == "create" && pr.Base == request.Base && pr.State == "OPEN" || request.Action == "merge" && pr.Number == request.Number) {
			existing = pr
			break
		}
	}
	if request.Action == "create" && (request.Title == "" || len(request.Title) > 256 || len(request.Body) > 65536 || request.Base == "" || request.Base == snapshot.Branch) {
		return snapshot, fmt.Errorf("PR creation requires a bounded title/body and a different base branch")
	}
	if request.Action == "merge" && (request.Base == "" || existing == nil || existing.Base != request.Base || existing.State != "MERGED" && (existing.State != "OPEN" || existing.Draft || existing.MergeState != "CLEAN" || existing.Review == "CHANGES_REQUESTED" || existing.Review == "REVIEW_REQUIRED")) {
		return snapshot, fmt.Errorf("PR is not at the confirmed HEAD with passing merge requirements")
	}
	if c.State.Project == nil {
		c.State.Project = &state.ProjectIdentity{Cwd: c.Cwd}
	}
	c.State.Project.Repo, c.State.Project.URL = snapshot.Repo, snapshot.URL
	operation := &state.Delivery{Base: request.Base, Action: request.Action, Repo: snapshot.Repo, Head: snapshot.Head, Branch: snapshot.Branch, Status: "pending", At: c.nowISO()}
	if old := c.State.Delivery; old != nil {
		c.State.Inbox = append(c.State.Inbox, map[string]any{"at": c.nowISO(), "action": "delivery-history", "receipt": old})
	}
	c.State.Delivery = operation
	c.save()
	if err := c.err(); err != nil {
		return snapshot, err
	}
	defer func() {
		if err != nil {
			operation.Status = "unknown"
			operation.Error = err.Error()
		}
		c.State.Delivery = operation
		c.save()
		if err == nil {
			err = c.err()
		}
	}()
	if request.Action == "create" && existing == nil {
		if _, err = deliveryRun(ctx, c.Cwd, "git", "-c", "push.followTags=false", "-c", "remote.origin.mirror=false", "-c", "push.recurseSubmodules=no", "push", snapshot.pushURL, snapshot.Head+":refs/heads/"+snapshot.Branch); err != nil {
			return snapshot, err
		}
		snapshot, err = InspectDelivery(ctx, c.Cwd, operation.Branch)
		if err != nil {
			return nil, err
		}
		if snapshot.Repo != operation.Repo || snapshot.Head != operation.Head {
			return snapshot, fmt.Errorf("delivery target changed while pushing the approved commit")
		}
		snapshot.Endpoint = endpoint
		for i := range snapshot.Pulls {
			pr := &snapshot.Pulls[i]
			if pr.Head == operation.Head && pr.Base == request.Base && pr.State == "OPEN" {
				existing = pr
				break
			}
		}
		if existing == nil {
			var file *os.File
			file, err = os.CreateTemp(filepath.Join(c.TaskDir, ".control"), "pr-body-*")
			if err != nil {
				return snapshot, err
			}
			defer os.Remove(file.Name())
			defer file.Close()
			if _, err = deliveryWriteBody(file, request.Body); err != nil {
				return snapshot, err
			}
			prArgs := []string{"pr", "create", "--repo", snapshot.Repo, "--head", snapshot.Branch, "--base", request.Base, "--title", request.Title, "--body-file", file.Name()}
			if snapshot.Endpoint == "draft-pr" {
				prArgs = append(prArgs, "--draft")
			}
			if _, err = deliveryRun(ctx, c.Cwd, "gh", prArgs...); err != nil {
				return snapshot, err
			}
		}
	} else if request.Action == "merge" && existing.State != "MERGED" {
		if _, err = deliveryRun(ctx, c.Cwd, "gh", "pr", "merge", fmt.Sprint(existing.Number), "--repo", snapshot.Repo, "--merge", "--match-head-commit", snapshot.Head); err != nil {
			return snapshot, err
		}
	}
	snapshot, err = InspectDelivery(ctx, c.Cwd, operation.Branch)
	if err != nil {
		return nil, err
	}
	snapshot.Endpoint = endpoint
	if snapshot.Repo != operation.Repo {
		return snapshot, fmt.Errorf("delivery repository changed during the operation")
	}
	for _, pr := range snapshot.Pulls {
		if pr.Head == request.ExpectedHead && (request.Action == "create" && pr.Base == request.Base && pr.State == "OPEN" || request.Action == "merge" && pr.Number == request.Number && pr.Base == request.Base && pr.State == "MERGED") {
			operation.Status, operation.URL, operation.Number = "confirmed", pr.URL, pr.Number
			if pr.MergeCommit != nil {
				operation.MergeSHA = pr.MergeCommit.OID
			}
			return snapshot, nil
		}
	}
	// Merge queues can accept a matched-head request without completing it yet.
	// A clean command exit is a receipt of the request, never a MERGED verdict.
	if request.Action == "merge" {
		for _, pr := range snapshot.Pulls {
			if pr.Number == request.Number && pr.Base == request.Base && pr.Head == request.ExpectedHead && pr.State == "OPEN" {
				operation.Status, operation.URL, operation.Number = "pending", pr.URL, pr.Number
				return snapshot, nil
			}
		}
	}
	return snapshot, fmt.Errorf("remote result is not yet confirmed at the approved HEAD; refresh before another operation")
}
