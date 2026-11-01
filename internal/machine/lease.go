package machine

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// AcquireRunLease 持有整个运行期的排他锁，封闭“读 pid → 写 pid”的并发
// 启动窗口。锁文件不删除，否则另一进程可能锁住不同 inode。
// 控制端只在确认运行进程退出后短暂持锁写 stopped 状态。
func AcquireRunLease(taskDir string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(taskDir, ".run.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("another orchestrator holds the task lease: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
