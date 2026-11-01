// run 子命令：以当前进程执行全自动循环；--only 只走一步状态转移（§12）。
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/machine"
)

var onlySteps = []string{"plan", "execute", "review", "verify"}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func runCmd() *cobra.Command {
	var only, packet, unit, pauseBefore string
	cmd := &cobra.Command{
		Use:   "run <docs/slug>",
		Short: "以当前进程执行全自动循环（--only 只走一步状态转移）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskDir, err := resolveTaskDir(args[0])
			if err != nil {
				return err
			}
			if only != "" && !contains(onlySteps, only) {
				return fmt.Errorf("--only 只支持: %s", strings.Join(onlySteps, "|"))
			}
			if pauseBefore != "" && pauseBefore != "review" {
				return fmt.Errorf("--pause-before 只支持: review")
			}
			// v0.18：--only 单步本身就是一步一停，pause 语义不存在——
			// 静默忽略会让使用者误以为冻结点已生效，fail-loud。
			if only != "" && pauseBefore != "" {
				return fmt.Errorf("--pause-before 不适用于 --only 单步模式（单步天然逐步执行）；去掉 --only 即为冻结点全自动")
			}
			cwd := machine.ProjectRoot(mustCwd())
			cfg, err := config.Load(taskDir, cwd)
			if err != nil {
				return err
			}
			drv := drivers.CreateDrivers(cfg)
			packetID := orEmptyStr(packet, unit)
			var summary machine.TaskSummary
			if only != "" {
				summary, err = machine.RunSingleStep(machine.RunSingleStepOpts{
					RunOpts: machine.RunOpts{
						TaskDir: taskDir, Cwd: cwd, Config: cfg, Drivers: drv,
						Context: cmd.Context(),
						Log:     logf(cmd),
					},
					Only:     only,
					PacketID: packetID,
				})
			} else {
				summary, err = machine.RunTask(machine.RunTaskOpts{
					RunOpts: machine.RunOpts{
						TaskDir: taskDir, Cwd: cwd, Config: cfg, Drivers: drv,
						Context: cmd.Context(),
						Log:     logf(cmd),
					},
					PauseBefore: pauseBefore,
				})
			}
			printJSON(cmd, summary)
			if errors.Is(err, machine.ErrInterrupted) {
				return &exitError{code: 143}
			}
			// 升级 / 熔断 / 失败步骤 → 退出码 3（摘要已打印）。
			if summary.Status == "escalated" || summary.Status == "budget-stopped" {
				return &exitError{code: 3}
			}
			if summary.Step != nil && !summary.Step.OK {
				return &exitError{code: 3} // manual step failed (#9)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "只走一步: plan|execute|review|verify")
	cmd.Flags().StringVar(&packet, "packet", "", "指定 packet id（与 --unit 同义）")
	cmd.Flags().StringVar(&unit, "unit", "", "指定单元 id（--packet 的别名）")
	cmd.Flags().StringVar(&pauseBefore, "pause-before", "", "在该步骤前暂停: review")
	return cmd
}

func orEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// printJSON 摘要 JSON 到 stdout（两空格缩进）。入参恒为内部固定形状的摘要
// 结构体，json.MarshalIndent 不可能失败——Node 对照版
// console.log(JSON.stringify(...)) 同样无错误分支，故不设错误返回。
func printJSON(cmd *cobra.Command, v any) {
	raw, _ := json.MarshalIndent(v, "", "  ")
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", raw)
}
