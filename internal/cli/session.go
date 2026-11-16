package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/spf13/cobra"
)

func sessionCmd() *cobra.Command {
	var key, reason string
	var abandon bool
	cmd := &cobra.Command{Use: "session <docs/slug> --key <role/scope> [--abandon --reason <text>]", Short: "在原 Provider Session 中交互；/return 验证交回，/release 交回并暂停，/quit 保留接管；--abandon 退役无法解开的规划调用", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		taskDir, err := resolveTaskDir(args[0])
		if err != nil {
			return err
		}
		if key == "" {
			return fmt.Errorf("--key is required")
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
		if !abandon && reason != "" {
			return fmt.Errorf("--reason requires --abandon")
		}
		if abandon {
			if strings.TrimSpace(reason) == "" {
				return fmt.Errorf("--abandon requires --reason")
			}
			if st.Sessions == nil {
				return fmt.Errorf("session key not found")
			}
			record := st.Sessions.Records[key]
			if record == nil || record.Last == nil {
				return fmt.Errorf("session key not found")
			}
			history := &machine.SessionHistory{Key: key, Invocation: record.Last.ID}
			request := &machine.SessionCommand{Action: "abandon", Key: key, Revision: st.Revision, Reason: reason, Invocation: record.Last.ID}
			if err := sessionActionProof(request, history, taskDir, true); err != nil {
				return err
			}
			summary, err := machine.RunSingleStep(machine.RunSingleStepOpts{RunOpts: machine.RunOpts{TaskDir: taskDir, Cwd: cwd, Config: cfg, Drivers: drivers.CreateDrivers(cfg), Context: cmd.Context(), Log: logf(cmd), IndexDir: indexDir}, Only: "session", ExpectedRevision: &request.Revision, Session: request})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Session %s retired as abandoned at revision %d; no result was applied and the old thread will not be resumed.\n", key, summary.Revision)
			return nil
		}
		run := func(action, prompt string) error {
			current, err := state.Load(taskDir)
			if err != nil {
				return err
			}
			request := &machine.SessionCommand{Action: action, Key: key, Revision: current.Revision, Prompt: prompt}
			if action == "claim" {
				request.Reason = "User opened the managed interactive terminal"
			}
			_, err = machine.RunSingleStep(machine.RunSingleStepOpts{RunOpts: machine.RunOpts{TaskDir: taskDir, Cwd: cwd, Config: cfg, Drivers: drivers.CreateDrivers(cfg), Context: cmd.Context(), Log: logf(cmd), IndexDir: indexDir}, Only: "session", ExpectedRevision: &request.Revision, Session: request})
			return err
		}
		if st.Handoff == nil {
			if err := run("claim", ""); err != nil {
				return err
			}
		} else if st.Handoff.Key != key {
			return fmt.Errorf("another session is under human control")
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Session %s\n输入 Prompt；/history 查看最近回复，/return 验证交回，/release 交回并暂停，/quit 保留接管退出。\n", key)
		scanner := bufio.NewScanner(cmd.InOrStdin())
		scanner.Buffer(make([]byte, 4096), 65536)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			switch line {
			case "":
				continue
			case "/quit":
				return nil
			case "/return":
				return run("return", "")
			case "/release":
				return run("release", "")
			case "/reconcile":
				if err := run("reconcile", ""); err != nil {
					return err
				}
				continue
			case "/history":
			default:
				if err := run("send", line); err != nil {
					return err
				}
			}
			history, err := machine.ReadSessionHistory(taskDir, key)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), history.Text)
		}
		return scanner.Err()
	}}
	cmd.Flags().StringVar(&key, "key", "", "准确的角色／作用域键，来自控制台")
	cmd.Flags().BoolVar(&abandon, "abandon", false, "退役一次无法对账的规划调用；必须提供原因并在控制终端确认精确调用 ID")
	cmd.Flags().StringVar(&reason, "reason", "", "退役未知调用的理由（最多 1000 字节）")
	return cmd
}
