package machine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
)

// A proposal is a model result, retained in the invocation receipt. It never
// writes state itself and is not an implementation/acceptance success report.
type executionDirectoryProposal struct {
	Directory struct {
		Cwd    string `json:"cwd"`
		Branch string `json:"branch"`
	} `json:"flywheelExecutionDirectory"`
}

func executionDirectoryPrompt() string {
	return "\n## 自行管理执行目录\n引擎未启用自动 Worktree 管理，并行调度仍可开启。你可以在已指定目录执行，或先自行创建／选择同仓库的独立 Worktree。若选择另一个目录：先不要实现任务，不写 done-report；仅返回 JSON {\"flywheelExecutionDirectory\":{\"cwd\":\"真实绝对目录\",\"branch\":\"实际分支名\"}}。该目录须是同仓库已登记、干净且与本任务基线一致的独立 Worktree。引擎核对并登记后会再次调用；届时所有修改、测试、Review、交付跟随新目录。一次任务只登记一次，不自行删除该目录。不能只在文字中声称已经切换目录。\n"
}

func parseExecutionDirectory(text string) (*executionDirectoryProposal, error) {
	if !strings.HasPrefix(strings.TrimSpace(text), "{") || !strings.Contains(text, `"flywheelExecutionDirectory"`) {
		return nil, nil
	}
	if !json.Valid([]byte(text)) {
		return nil, fmt.Errorf("invalid or oversized execution directory proposal")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(text), &fields) // valid JSON beginning with '{' is an object
	if _, present := fields["flywheelExecutionDirectory"]; !present {
		return nil, nil // quoting the protocol in a normal result is not a control request
	}
	if len(text) > 16384 {
		return nil, fmt.Errorf("invalid or oversized execution directory proposal")
	}
	var proposal executionDirectoryProposal
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proposal); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(proposal.Directory.Cwd, "/") || proposal.Directory.Branch == "" {
		return nil, fmt.Errorf("execution directory proposal requires an absolute cwd and branch")
	}
	return &proposal, nil
}

func (c *Context) adoptExecutionDirectory(packet *contract.Packet, proposal *executionDirectoryProposal, result drivers.Result) error {
	if !c.sharedActive(packet.PacketID) && (c.Config.Worktree.Enabled == nil || *c.Config.Worktree.Enabled) {
		return fmt.Errorf("execution directory proposals require caller-managed worktrees")
	}
	if result.ExitCode == nil || *result.ExitCode != 0 || !result.Settled || result.Cancelled || result.TimedOut ||
		result.Signal != "" || result.Truncated || result.SpawnError != "" || result.CleanupError != "" || result.ErrorCode != "" || result.ErrorMessage != nil {
		return fmt.Errorf("execution directory proposal lacks a successful, quiescent invocation")
	}
	ps := c.State.Packets[packet.PacketID]
	if ps.Worktree != nil && ps.Worktree.Mode == "worktree" {
		return fmt.Errorf("task already has an execution worktree; retain and reconcile it instead of replacing it")
	}
	path, err := canonicalDirectory(proposal.Directory.Cwd)
	if err != nil {
		return err
	}
	if path == c.Cwd || path == c.WorkspaceCwd || ProjectRoot(path) != path || WorkspaceRoot(path) != c.WorkspaceCwd {
		return fmt.Errorf("proposed execution directory is not an independent checkout of this project")
	}
	base, ok := verify.GetBaseRef(path)
	if !ok || ps.BaseRef == nil || base != *ps.BaseRef {
		return fmt.Errorf("proposed execution checkout has a different task baseline")
	}
	branch, err := deliveryRun(c.RunContext, path, "git", "symbolic-ref", "--short", "HEAD")
	if err != nil || strings.TrimSpace(branch) != proposal.Directory.Branch {
		return fmt.Errorf("proposed execution checkout branch does not match")
	}
	dirty, err := deliveryRun(c.RunContext, path, "git", "status", "--porcelain", "--untracked-files=all")
	if err != nil || strings.TrimSpace(dirty) != "" {
		return fmt.Errorf("register the execution checkout before making changes; it must be clean")
	}
	worktrees, err := deliveryRun(c.RunContext, path, "git", "worktree", "list", "--porcelain", "-z")
	if err != nil || strings.Count(worktrees+"\x00", "\x00branch refs/heads/"+proposal.Directory.Branch+"\x00") != 1 {
		return fmt.Errorf("execution branch is shared by multiple worktrees or its ownership cannot be confirmed")
	}
	if c.sharedActive(packet.PacketID) {
		files, ok := verify.GetChangedFiles(c.Cwd, c.State.SharedExecution.BaseRef)
		if !ok {
			return fmt.Errorf("cannot inspect shared source before directory registration")
		}
		for _, file := range files {
			if !c.snapshotExcluded(file) && contract.InWriteScope(file, packet.Schedule.Writes) {
				return fmt.Errorf("register a new directory before modifying this task's shared files; preserve existing changes")
			}
		}
	}
	if err := c.reserveExecutionDirectory(packet.PacketID, path); err != nil {
		return err
	}
	ps.SharedPeers = nil
	ps.Worktree = &state.WorktreeInfo{Path: path, Branch: proposal.Directory.Branch, Mode: "worktree", External: true}
	ps.RetryHistory = append(ps.RetryHistory, map[string]any{"policy": "execution-directory", "cwd": path, "branch": proposal.Directory.Branch, "at": c.nowISO()})
	ps.Status = "pending"
	// Consume the original receipt and new binding in one state CAS. A crash
	// before this CAS replays the same proposal from its durable result.
	c.consumeSession("execute", packet.PacketID)
	c.save()
	return c.err()
}

func (c *Context) reserveExecutionDirectory(id, path string) error {
	owner := c
	if c.parallel != nil {
		group := c.parallel.group
		group.mu.Lock()
		defer group.mu.Unlock()
		if group.directories == nil {
			group.directories = map[string]string{}
		}
		if previous, exists := group.directories[path]; exists && previous != id {
			return fmt.Errorf("execution checkout is reserved by another parallel task")
		}
		group.directories[path] = id
		owner = group.owner
	}
	for key, packet := range owner.State.Packets {
		if key != id && packet.Worktree != nil && packet.Worktree.Path == path {
			return fmt.Errorf("execution checkout already belongs to task %s", key)
		}
	}
	return nil
}
