package machine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/state"
)

// WorkspaceRoot follows Git's common repository identity, not the task's
// checkout. Non-Git rehearsals retain their own directory.
func WorkspaceRoot(cwd string) string {
	// CreateContext already resolved the Git toplevel. A normal checkout owns
	// its .git directory; only linked/separate metadata needs Git's inventory.
	info, statErr := os.Lstat(filepath.Join(cwd, ".git"))
	if os.IsNotExist(statErr) || (statErr == nil && info.IsDir()) {
		return cwd
	}
	cmd := exec.Command("git", "-C", cwd, "worktree", "list", "--porcelain", "-z")
	cmd.Env = childenv.EnvSlice(childenv.GateEnv(childenv.OSLookupEnv, nil))
	out, err := cmd.Output()
	if err == nil {
		first, _, _ := strings.Cut(string(out), "\x00")
		if path, ok := strings.CutPrefix(first, "worktree "); ok {
			if root, err := canonicalDirectory(path); err == nil {
				return root
			}
		}
	}
	return cwd
}

func workspacePrompt(project, execution, role, scope string) string {
	if project == "" {
		return "" // direct legacy calls retain their original contract
	}
	return fmt.Sprintf("## 项目归属与任务执行位置（本次调用）\n项目工作区: %q\n实际执行目录: %q\n角色 / 任务: %s / %s\nSession 归属项目工作区；代码、文件、测试与 Git 操作必须使用实际执行目录。每次 shell 调用显式设置 workdir 或先 cd 到该目录；文件工具使用该目录内的绝对路径。先核对 pwd 与 git branch --show-current。不得修改、切换分支或提交实际执行目录以外的其他 checkout。只有项目目录与实际执行目录相同时，才在项目目录执行任务。恢复与返修仍使用这里声明的位置；目录不一致时停止报告。\n\n", project, execution, role, scope)
}

// The recovery adapter verifies native settings; task receipts and candidate
// guards continue to use r.Cwd, the separate execution checkout.
func sessionNativeCwd(r *state.SessionRecord) string {
	if r.WorkspaceCwd != "" && r.Configuration != nil && r.Configuration.Harness != "dsh" {
		return r.WorkspaceCwd
	}
	return r.Cwd
}
