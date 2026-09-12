// Package cli 组装 Flywheel 的命令树（DESIGN.md §12/§13）。所有子命令都是对
// 内部包的薄接线。输出契约（AGENTS.md 硬契约）：
//   - run / intake / garden：JSON 摘要到 stdout
//   - status / stop / install-skills / skills：人话文本到 stdout
//   - 错误一律走 stderr
//   - 退出码：0 ok / 1 用法·运行时 / 3 升级·熔断·失败步骤
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/ckanner/flywheel/internal/build"
)

// Execute 进程入口：真实参数与标准流。
func Execute() int {
	return runRoot(os.Args[1:], os.Stdout, os.Stderr)
}

// exitError 携带非零退出码的命令结果（3 = 升级/熔断/失败步骤）。
type exitError struct {
	code  int
	cause error
}

func (e *exitError) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return fmt.Sprintf("exit %d", e.code)
}

// runRoot 可注入参数与流的执行核心（测试直接调用）。
func runRoot(args []string, stdout, stderr io.Writer) int {
	root := newRootCmd()
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		var ee *exitError
		if errors.As(err, &ee) && ee.code == 3 {
			return 3 // 摘要已在 stdout 打印过；3 是语义信号不是文本错误
		}
		fmt.Fprintf(stderr, "[flywheel] 错误: %v\n", err)
		return 1
	}
	return 0
}

// logf Node 版 `[flywheel] ${msg}` 前缀日志。
func logf(cmd *cobra.Command) func(string) {
	return func(msg string) { fmt.Fprintf(cmd.ErrOrStderr(), "[flywheel] %s\n", msg) }
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "flywheel",
		Short: "Flywheel — Loop Engineering 编排器（docs/DESIGN.md）",
		Long: `Flywheel（飞轮）: 外部确定性编排器驱动 Plan / Execute / Review agent loops，
接收已冻结的契约（docs/<slug>/），自动完成规划、实现、评审与分层验证。

编排器不是 agent（设计铁律 1）: 所有机器决策都是可机械检查的状态转移。`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(versionCmd())
	root.AddCommand(runCmd())
	root.AddCommand(statusCmd())
	root.AddCommand(stopCmd())
	root.AddCommand(intakeCmd())
	root.AddCommand(gardenCmd())
	root.AddCommand(installSkillsCmd())
	root.AddCommand(skillsCmd())
	return root
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "打印版本与构建信息",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "flywheel %s (%s)\n", build.Version, build.Date)
		},
	}
}
