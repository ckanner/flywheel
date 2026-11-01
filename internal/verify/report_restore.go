package verify

import (
	"fmt"
	"path/filepath"
	"strings"
)

// RestoreReservedReport 只恢复本次调用之前已跟踪的 done-report 路径。
// 新报告已收集到权威任务目录；worktree 内不能留下旧 tracked 报告的删除，
// 也不能为了清理报告丢弃其它WIP。false表示基线中不存在，调用方可仅删除新报告。
func RestoreReservedReport(cwd, baseline, path string) (bool, error) {
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) ||
		filepath.Base(filepath.Dir(clean)) != "reports" || !strings.HasSuffix(filepath.Base(clean), ".done.json") {
		return false, fmt.Errorf("refusing to restore non-reserved report path %q", path)
	}
	pathspec := ":(literal)" + filepath.ToSlash(clean)
	entry, err := gitOut(cwd, "ls-tree", "-z", baseline, "--", pathspec)
	if err != nil {
		return false, fmt.Errorf("inspect prior report: %w", err)
	}
	if entry == "" {
		return false, nil
	}
	meta := strings.Fields(strings.SplitN(entry, "\t", 2)[0])
	if len(meta) != 3 || meta[1] != "blob" {
		return false, fmt.Errorf("prior report path %q is not a file", path)
	}
	if _, err := gitOut(cwd, "restore", "--source="+baseline, "--worktree", "--", pathspec); err != nil {
		return false, fmt.Errorf("restore prior report: %w", err)
	}
	return true, nil
}
