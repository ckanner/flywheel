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
	var key string
	cmd := &cobra.Command{Use: "session <docs/slug> --key <role/scope>", Short: "在原 Provider Session 中交互；/return 验证交回，/release 交回并暂停，/quit 保留接管", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
	return cmd
}
