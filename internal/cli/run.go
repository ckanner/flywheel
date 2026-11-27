// run 子命令：以当前进程执行全自动循环；--only 只走一步状态转移（§12）。
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
)

var onlySteps = []string{"plan", "execute", "review", "verify", "integrate", "final"}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func runCmd() *cobra.Command {
	var only, packet, unit, pauseBefore, mode, retryReason, sessionRequest, sessionDigest, packetRequest, packetDigest, milestoneRequest, milestoneDigest string
	var revision int64
	var investigate, planningOnly bool
	cmd := &cobra.Command{
		Use:   "run <docs/slug>",
		Short: "以当前进程执行全自动循环（--only 只走一步状态转移）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if investigate {
				if revision < 1 || only != "" || packet != "" || unit != "" || mode != "" || pauseBefore != "" || retryReason != "" || sessionRequest != "" || sessionDigest != "" || packetRequest != "" || packetDigest != "" || milestoneRequest != "" || milestoneDigest != "" || planningOnly {
					return fmt.Errorf("--investigate requires only an observed positive --expect-revision")
				}
				dir, err := resolveTaskDir(args[0])
				if err != nil {
					return err
				}
				ctx, cancel := diagnosticContext(cmd)
				defer cancel()
				result, err := machine.RunInvestigation(ctx, dir, revision)
				printJSON(cmd, result)
				if ctx.Err() != nil || result != nil && result.Status == "cancelled" {
					return &exitError{code: 143}
				}
				if result != nil && result.Status == "blocked" {
					return &exitError{code: 3, cause: err}
				}
				return err
			}
			if planningOnly && (only != "" || packet != "" || unit != "" || pauseBefore != "" || retryReason != "" || sessionRequest != "" || sessionDigest != "" || packetRequest != "" || packetDigest != "" || milestoneRequest != "" || milestoneDigest != "") {
				return fmt.Errorf("--planning-only applies to a reviewed planning run, without implementation or manual-stage flags")
			}
			var expectedRevision *int64
			if retryReason != "" && (only != "execute" || !cmd.Flags().Changed("expect-revision") || len(retryReason) > 1000) {
				return fmt.Errorf("--retry-reason requires revision-bound Execute and at most 1000 bytes")
			}
			if cmd.Flags().Changed("expect-revision") {
				if only == "" || revision < 1 {
					return fmt.Errorf("--expect-revision requires --only and a positive revision")
				}
				expectedRevision = &revision
			}
			if cmd.Flags().Changed("mode") && !contract.ValidWorkflowMode(mode) {
				return fmt.Errorf("--mode only supports auto, quick, or full")
			}
			taskDir, err := resolveTaskDir(args[0])
			if err != nil {
				return err
			}
			if planningOnly {
				st, err := state.Load(taskDir)
				if err != nil {
					return err
				}
				if st.Planning == nil {
					return fmt.Errorf("--planning-only requires a reviewed planning task; migrate a legacy task explicitly")
				}
			}
			if only != "" && only != "session" && only != "packet" && only != "milestone" && !contains(onlySteps, only) {
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
			var session *machine.SessionCommand
			if only == "session" {
				session, err = loadSessionInput(taskDir, sessionRequest, sessionDigest)
				if err != nil {
					return err
				}
				if expectedRevision == nil || session.Revision != *expectedRevision {
					return fmt.Errorf("session payload revision does not match admission")
				}
				if err := sessionActionProof(session, nil, taskDir, false); err != nil {
					return err
				}
			} else if sessionRequest != "" || sessionDigest != "" {
				return fmt.Errorf("--session-request requires --only session")
			}
			var packetCmd *machine.PacketCommand
			if only == "packet" {
				packetCmd, err = loadPacketInput(taskDir, packetRequest, packetDigest)
				if err != nil {
					return err
				}
				if expectedRevision == nil || packetCmd.Revision != *expectedRevision {
					return fmt.Errorf("packet payload revision does not match admission")
				}
				if err := packetActionProof(packetCmd, taskDir, false); err != nil {
					return err
				}
			} else if packetRequest != "" || packetDigest != "" {
				return fmt.Errorf("--packet-request requires --only packet")
			}
			var milestoneCmd *machine.MilestoneCommand
			if only == "milestone" {
				milestoneCmd, err = loadMilestoneInput(taskDir, milestoneRequest, milestoneDigest)
				if err != nil {
					return err
				}
				if expectedRevision == nil || milestoneCmd.Revision != *expectedRevision {
					return fmt.Errorf("milestone payload revision does not match admission")
				}
				if err := milestoneActionProof(milestoneCmd, taskDir, false); err != nil {
					return err
				}
			} else if milestoneRequest != "" || milestoneDigest != "" {
				return fmt.Errorf("--milestone-request requires --only milestone")
			}
			cwd := machine.ProjectRoot(mustCwd())
			cfg, err := config.Load(taskDir, cwd)
			if err != nil {
				return err
			}
			indexDir, err := machine.RunIndexDirectory()
			if err != nil {
				return err
			}
			drv := drivers.CreateDrivers(cfg)
			packetID := orEmptyStr(packet, unit)
			var summary machine.TaskSummary
			if only != "" {
				summary, err = machine.RunSingleStep(machine.RunSingleStepOpts{
					RunOpts: machine.RunOpts{
						TaskDir: taskDir, Cwd: cwd, Config: cfg, Drivers: drv, Mode: mode, IndexDir: indexDir,
						Context: cmd.Context(),
						Log:     logf(cmd),
					},
					Only:             only,
					PacketID:         packetID,
					ExpectedRevision: expectedRevision,
					RetryReason:      retryReason,
					Session:          session,
					Packet:           packetCmd,
					Milestone:        milestoneCmd,
				})
			} else {
				summary, err = machine.RunTask(machine.RunTaskOpts{
					RunOpts: machine.RunOpts{
						TaskDir: taskDir, Cwd: cwd, Config: cfg, Drivers: drv, Mode: mode, IndexDir: indexDir,
						Context: cmd.Context(),
						Log:     logf(cmd),
					},
					PauseBefore:  pauseBefore,
					PlanningOnly: planningOnly,
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
	cmd.Flags().StringVar(&sessionDigest, "session-request-digest", "", "受管会话请求内容摘要")
	cmd.Flags().StringVar(&packetRequest, "packet-request", "", "人工处置单元请求文件（由 control packet 创建）")
	cmd.Flags().StringVar(&packetDigest, "packet-request-digest", "", "人工处置单元请求内容摘要")
	cmd.Flags().StringVar(&milestoneRequest, "milestone-request", "", "人工处置里程碑请求文件（由 control milestone 创建）")
	cmd.Flags().StringVar(&milestoneDigest, "milestone-request-digest", "", "人工处置里程碑请求内容摘要")
	cmd.Flags().BoolVar(&investigate, "investigate", false, "执行已保存的有界调查；不执行实现阶段")
	cmd.Flags().BoolVar(&planningOnly, "planning-only", false, "仅推进已保存的设计/计划及独立评审；不启动实施")
	cmd.Flags().StringVar(&sessionRequest, "session-request", "", "受管会话请求文件（由 control session 创建）")
	cmd.Flags().StringVar(&only, "only", "", "只走一步: plan|execute|review|verify|integrate|final")
	cmd.Flags().StringVar(&packet, "packet", "", "指定 packet id（与 --unit 同义）")
	cmd.Flags().StringVar(&unit, "unit", "", "指定单元 id（--packet 的别名）")
	cmd.Flags().StringVar(&pauseBefore, "pause-before", "", "在该步骤前暂停: review")
	cmd.Flags().StringVar(&mode, "mode", "", "请求流程: auto|quick|full；省略时沿用契约或已保存策略，旧任务保持 full")
	cmd.Flags().Int64Var(&revision, "expect-revision", 0, "受管单阶段操作：要求状态 revision 匹配")
	cmd.Flags().StringVar(&retryReason, "retry-reason", "", "受管 Execute：记录人工原因并仅追加一次执行/L1尝试")
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
