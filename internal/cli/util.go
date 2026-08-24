// 共享 CLI 工具：任务目录解析、JSON 打印。
package cli

import (
	"os"
	"path/filepath"
)

// mustCwd 当前工作目录。os.Getwd 对运行中的进程实际不会失败（darwin
// __getwd 直接读 cwd vnode，目录被删除也返回最后路径；linux /proc 自身
// 挂载恒在；Node 对照 process.cwd() 同样恒存在），故不设错误分支。
func mustCwd() string {
	wd, _ := os.Getwd()
	return wd
}

// resolveTaskDir Node resolveTaskDir 等价：相对 cwd 解析并要求目录存在。
// filepath.Abs 仅在取不到工作目录时失败（同上不可能），相对解析结果交给
// 下面的 Stat 存在性检查把关。
func resolveTaskDir(slugArg string) (string, error) {
	if slugArg == "" {
		return "", filepath.ErrBadPattern
	}
	taskDir, _ := filepath.Abs(slugArg)
	if _, err := os.Stat(taskDir); err != nil {
		return "", dirMissingError{taskDir}
	}
	return taskDir, nil
}

type dirMissingError struct{ path string }

func (e dirMissingError) Error() string { return "任务目录不存在: " + e.path }
