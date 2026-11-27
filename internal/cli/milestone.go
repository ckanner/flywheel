package cli

import (
	"fmt"
	"strings"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/spf13/cobra"
)

func milestoneCmd() *cobra.Command {
	var milestoneID, reason, guidance string
	var retry, accept bool
	cmd := &cobra.Command{
		Use:   "milestone <docs/slug> --id <M-01> (--retry|--accept) --reason <text> [--guidance <text>]",
		Short: "人工处置里程碑终验升级：重跑 L2/L3 与 Judge（可附指导），或在门禁已通过当前 HEAD 时人工签核关闭里程碑",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskDir, err := resolveTaskDir(args[0])
			if err != nil {
				return err
			}
			chosen := 0
			action := ""
			if retry {
				chosen++
				action = "retry"
			}
			if accept {
				chosen++
				action = "accept"
			}
			if chosen != 1 {
				return fmt.Errorf("specify exactly one of --retry or --accept")
			}
			if strings.TrimSpace(milestoneID) == "" {
				return fmt.Errorf("--id is required")
			}
			if strings.TrimSpace(reason) == "" {
				return fmt.Errorf("--reason is required")
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
			st, err := state.Load(taskDir)
			if err != nil {
				return err
			}
			request := &machine.MilestoneCommand{
				Action:      action,
				MilestoneID: milestoneID,
				Revision:    st.Revision,
				Reason:      reason,
				Guidance:    guidance,
			}
			if err := request.Validate(); err != nil {
				return err
			}
			if err := milestoneActionProof(request, taskDir, true); err != nil {
				return err
			}
			summary, err := machine.RunSingleStep(machine.RunSingleStepOpts{
				RunOpts: machine.RunOpts{
					TaskDir: taskDir, Cwd: cwd, Config: cfg, Drivers: drivers.CreateDrivers(cfg),
					Context: cmd.Context(), Log: logf(cmd), IndexDir: indexDir,
				},
				Only:             "milestone",
				ExpectedRevision: &request.Revision,
				Milestone:        request,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Milestone %s triage (%s) completed at revision %d (task status: %s).\n", milestoneID, action, summary.Revision, summary.Status)
			return nil
		},
	}
	cmd.Flags().StringVar(&milestoneID, "id", "", "待处置的里程碑 id（例如 M-01）")
	cmd.Flags().BoolVar(&retry, "retry", false, "重新武装该里程碑以重跑 L2/L3 门禁与 Judge（可附 --guidance 注入人工指导）")
	cmd.Flags().BoolVar(&accept, "accept", false, "仅当全部单元已完成且确定性 L2/L3 门禁在当前 HEAD 通过时，人工签核关闭该里程碑")
	cmd.Flags().StringVar(&reason, "reason", "", "人工处置理由（最多 1000 字节）")
	cmd.Flags().StringVar(&guidance, "guidance", "", "可选：权威人工范围指导，将注入下次 Judge 提示词（最多 2000 字节）")
	return cmd
}
