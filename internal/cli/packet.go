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

func packetCmd() *cobra.Command {
	var packetID, reason, guidance string
	var retry, replan, accept bool
	cmd := &cobra.Command{
		Use:   "packet <docs/slug> --id <packet-id> (--retry|--replan|--accept) --reason <text> [--guidance <text>]",
		Short: "人工处置已升级到 inbox 的单元：带指导重试、发起单包契约修订、或 L1 全绿时人工签核放行",
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
			if replan {
				chosen++
				action = "replan"
			}
			if accept {
				chosen++
				action = "accept"
			}
			if chosen != 1 {
				return fmt.Errorf("specify exactly one of --retry, --replan, or --accept")
			}
			if strings.TrimSpace(packetID) == "" {
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
			request := &machine.PacketCommand{
				Action:   action,
				PacketID: packetID,
				Revision: st.Revision,
				Reason:   reason,
				Guidance: guidance,
			}
			if err := request.Validate(); err != nil {
				return err
			}
			if err := packetActionProof(request, taskDir, true); err != nil {
				return err
			}
			summary, err := machine.RunSingleStep(machine.RunSingleStepOpts{
				RunOpts: machine.RunOpts{
					TaskDir: taskDir, Cwd: cwd, Config: cfg, Drivers: drivers.CreateDrivers(cfg),
					Context: cmd.Context(), Log: logf(cmd), IndexDir: indexDir,
				},
				Only:             "packet",
				PacketID:         packetID,
				ExpectedRevision: &request.Revision,
				Packet:           request,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Packet %s triage (%s) completed at revision %d (task status: %s).\n", packetID, action, summary.Revision, summary.Status)
			return nil
		},
	}
	cmd.Flags().StringVar(&packetID, "id", "", "处于 inbox 状态的 packet id")
	cmd.Flags().BoolVar(&retry, "retry", false, "重新武装该单元重试（可附 --guidance 注入人工裁决指导）")
	cmd.Flags().BoolVar(&replan, "replan", false, "授权 Plan Loop 修订该单元契约并重新武装")
	cmd.Flags().BoolVar(&accept, "accept", false, "仅当 L1 确定性门禁在当前候选上全绿时，人工签核放行并提交合并")
	cmd.Flags().StringVar(&reason, "reason", "", "人工处置理由（最多 1000 字节）")
	cmd.Flags().StringVar(&guidance, "guidance", "", "可选：权威人工范围指导，将注入 Execute 与 Review 提示词（最多 2000 字节）")
	return cmd
}
