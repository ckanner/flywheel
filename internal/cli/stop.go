// stop 子命令：停编排器进程，状态定格。
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
)

func jsonUnmarshalStrict(raw []byte, v any) error { return json.Unmarshal(raw, v) }

// stop 进程确认轮询参数（生产行为：50 × 100ms = 5s；测试注入缩小等待）。
var (
	stopPollAttempts = 50
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
			stoppedPid := 0
			confirmedDead := false
			pidPath := machine.PidFile(taskDir)
			if raw, err := os.ReadFile(pidPath); err == nil {
				var pf struct {
					PID int `json:"pid"`
				}
				if jsonErr := jsonUnmarshalStrict(raw, &pf); jsonErr == nil && pf.PID > 0 {
					stoppedPid = pf.PID
					if sigErr := syscall.Kill(stoppedPid, syscall.SIGTERM); sigErr != nil {
						confirmedDead = true // 发信号前就已死
					} else {
						for range stopPollAttempts {
							if signalZero(stoppedPid) != nil {
								confirmedDead = true
								break
							}
							time.Sleep(stopPollInterval)
						}
					}
				} else {
					confirmedDead = true // pid 文件不可读 = 陈旧
				}
				if !confirmedDead {
					// 绝不声称一个无法确认的停止（#7）。
					fmt.Fprintf(out, "SIGTERM 已发送给 pid %d，但 5s 内未确认退出。state.json 保持原样；请检查进程后重试，或手动 kill。\n", stoppedPid)
					return &exitError{code: 1}
				}
				_ = os.Remove(pidPath)
			}
			if state.Exists(taskDir) {
				st, err := state.Load(taskDir)
				if err != nil {
					return err
				}
				if st.Status != "completed" {
					st.Status = "stopped"
					if _, err := state.Save(taskDir, st, nil); err != nil {
						return err
					}
				}
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
