package machine

import (
	"encoding/json"
	"os"
	"syscall"

	"github.com/ckanner/flywheel/internal/verify"
)

func jsonUnmarshal(raw []byte, v any) error { return json.Unmarshal(raw, v) }

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// processAlive 信号 0 存在性探测（Node process.kill(pid, 0) 等价）。
// nil = 存活；非 nil = 已死或探测失败（视为陈旧）。
func processAlive(pid int) error {
	return syscall.Kill(pid, 0)
}

func verifyForeignDirty(cwd, prefix string) ([]string, bool) {
	return verify.ForeignDirtyFiles(cwd, prefix)
}
