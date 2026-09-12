// zcode 桌面可见性（DESIGN v0.20 机制，与协议驱动正交）：--open-workspace
// 双触发注册 + 索引导入。仅 UX 增强——失败静默降级，绝不阻塞执行链路。
package drivers

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// zcodeWorkspaceOpener workspace 注册的系统出口（测试注入缝，AGENTS.md 覆盖
// 纪律：副作用经缝注入而非测试内 mock 进程）。
var zcodeWorkspaceOpener = defaultOpenZcodeWorkspace

// zcodeDesktopGOOS 平台闸注入缝：生产恒 runtime.GOOS，测试可强制 "darwin"
// 让 Linux CI 覆盖注册路径。
var zcodeDesktopGOOS = runtime.GOOS

// defaultZcodeDesktopBin 桌面二进制默认路径（const；env 覆盖见 zcodeDesktopBin）。
const defaultZcodeDesktopBin = "/Applications/ZCode.app/Contents/MacOS/ZCode"

// zcodeDesktopBin 解析桌面二进制路径：env 覆盖权威，缺省官方安装位置。
func zcodeDesktopBin() string {
	if bin := os.Getenv(ZcodeDesktopBinEnv); bin != "" {
		return bin
	}
	return defaultZcodeDesktopBin
}

func defaultOpenZcodeWorkspace(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, zcodeDesktopBin(), "--open-workspace", path)
	if err := cmd.Start(); err != nil {
		return err
	}
	// fire-and-forget，但只 Start 不 Wait 会留僵尸——Wait 仅负责回收。
	go func() { _ = cmd.Wait() }()
	return nil
}

// disableDesktopRegistration 测试闸：Run 级测试不得激活用户桌面（隔离纪律）。
var disableDesktopRegistration = false

// RegisterZcodeWorkspace 在 ZCode 桌面端注册 workspace 路径（macOS-only）并
// 触发该路径 CLI 存储会话的索引导入。实测语义（v0.20 九组探针）：GUI 按
// workspace_key 分组，未注册路径零显示；--open-workspace 每次处理都幂等注册
// 并导入。失败静默：可见性是 UX 增强，绝不阻塞执行链路。
func RegisterZcodeWorkspace(path string) {
	if disableDesktopRegistration || zcodeDesktopGOOS != "darwin" || path == "" {
		return
	}
	// 索引按归一化路径做 workspace_key（/tmp 符号链接注册成功但导入失配，
	// EvalSymlinks 后关闭该边缘——实测验证）。
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	_ = zcodeWorkspaceOpener(path)
}
