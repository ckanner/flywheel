// stop 子命令：停编排器进程，状态定格。
package cli

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ckanner/flywheel/internal/investigation"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
)

// 45s 包括在飞进程组退出及独立 DI restore；超时不强杀 engine，也不写假 stopped。
var (
	stopPollAttempts = 450
	stopPollInterval = 100 * time.Millisecond
)

func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <docs/slug>",
		Short: "停编排器进程，状态定格",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskDir, err := resolveTaskDir(args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			stoppedPid, err := stopTask(taskDir)
			if err != nil {
				return err
			}
			if stoppedPid > 0 {
				fmt.Fprintf(out, "已停止编排器进程 (pid %d)，状态定格。\n", stoppedPid)
			} else {
				fmt.Fprintln(out, "编排器未在运行，状态已定格为 stopped。")
			}
			return nil
		},
	}
}

// stopTask never signals a PID read from disk. A task-scoped nonce request asks
// the lease owner to cancel itself, then lease acquisition confirms cleanup.
func stopTask(taskDir string) (int, error) {
	pid := 0
	lastToken := ""
	for attempt := 0; ; attempt++ {
		release, err := machine.AcquireRunLease(taskDir)
		if err == nil {
			defer release()
			return pid, freezeStoppedTask(taskDir)
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return pid, fmt.Errorf("无法确认运行锁；state.json 保持原样: %w", err)
		}
		control, err := machine.ReadRunControl(taskDir)
		if err != nil && (!errors.Is(err, os.ErrNotExist) || lastToken == "") {
			return pid, fmt.Errorf("无法安全请求停止；state.json 保持原样: %w", err)
		}
		// The owner removes its PID before releasing the lease. Once a request
		// was sent, that expected cleanup window must still wait for the lease.
		if err == nil && control.ControlToken != lastToken {
			pid = control.PID // display only, never a signalling target
			if err := machine.WriteStopRequest(taskDir, control.ControlToken); err != nil {
				return pid, err
			}
			lastToken = control.ControlToken
		}
		if attempt >= stopPollAttempts {
			return pid, fmt.Errorf("停止请求已发送给当前任务运行，但 %s 内未确认清理完成。state.json 保持原样；请等待后重试", time.Duration(stopPollAttempts)*stopPollInterval)
		}
		time.Sleep(stopPollInterval)
	}
}

// freezeStoppedTask is called only while holding the lifetime lease.
func freezeStoppedTask(taskDir string) error {
	for _, path := range []string{machine.PidFile(taskDir), machine.StopRequestFile(taskDir)} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if state.Exists(taskDir) {
		st, err := state.Load(taskDir)
		if err != nil {
			return err
		}
		if st.Status != "completed" {
			expected := st.Revision
			st.Status = "stopped"
			if _, err := state.Save(taskDir, st, &expected); err != nil {
				return err
			}
		}
	} else if preparation, err := state.LoadPreparation(taskDir); err == nil {
		_, err = investigation.Cancel(taskDir, investigation.Options{ExpectedRevision: preparation.Revision})
		return err
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}
