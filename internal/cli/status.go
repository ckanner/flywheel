// status 子命令：只读——读 state.json/指标，讲进度（人话输出）。
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
)

func signalZero(pid int) error { return syscall.Kill(pid, 0) }

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <docs/slug>",
		Short: "只读：讲任务进度",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskDir, err := resolveTaskDir(args[0])
			if err != nil {
				return err
			}
			if !state.Exists(taskDir) {
				return fmt.Errorf("state.json 不存在于 %s（任务尚未启动？）", taskDir)
			}
			st, err := state.Load(taskDir)
			if err != nil {
				return err
			}
			alive, pid := orchestratorAlive(taskDir)

			out := cmd.OutOrStdout()
			suffix := ""
			if alive {
				suffix = fmt.Sprintf("（编排器运行中, pid %d）", pid)
			}
			fmt.Fprintf(out, "任务: %s  状态: %s%s\n", st.Slug, st.Status, suffix)
			current := "—"
			if st.CurrentMilestone != nil {
				current = *st.CurrentMilestone
			}
			progress := "—"
			if st.LastProgressAt != nil {
				progress = *st.LastProgressAt
			}
			fmt.Fprintf(out, "当前里程碑: %s  上次推进: %s  revision: %d\n", current, progress, st.Revision)
			warnIfStalled(out, taskDir, st)
			fmt.Fprintln(out)
			for mid, ms := range st.Milestones {
				closed := ""
				if ms.ClosedAt != nil {
					closed = fmt.Sprintf("（关闭于 %s）", *ms.ClosedAt)
				}
				fmt.Fprintf(out, "里程碑 %s: %s%s\n", mid, ms.Status, closed)
				for pid2, p := range st.Packets {
					if p.Milestone != mid {
						continue
					}
					var flags []string
					if p.Interrupted {
						flags = append(flags, "interrupted")
					}
					flagText := strings.Join(flags, ",")
					if flagText != "" {
						flagText = " [" + flagText + "]"
					}
					executor := "—"
					if p.Evidence.Executor != nil {
						executor = "✓"
					}
					l1 := "—"
					if p.Evidence.L1 != nil {
						l1 = evidencePass(p.Evidence.L1, "pass")
					}
					review := evidenceVerdict(p.Evidence.Review)
					fmt.Fprintf(out,
						"  %s: %s%s  尝试 %d（失败 %d）  证据: executor=%s L1=%s review=%s\n",
						pid2, p.Status, flagText, p.Attempts, p.FailedAttempts, executor, l1, review)
				}
			}
			fmt.Fprintln(out)
			b := st.Budget
			fmt.Fprintf(out, "token 累计: %d（in %d / out %d / reasoning %d）  成本: $%v\n",
				b.Totals.Tokens, b.Totals.Input, b.Totals.Output, b.Totals.Reasoning, b.Totals.CostUSD)
			limit := "未设"
			if b.DailyTokenLimit != nil {
				limit = fmt.Sprint(*b.DailyTokenLimit)
			}
			fmt.Fprintf(out, "缓存命中率: %.1f%%  预算档: %s（日上限 %s）\n", b.Cache.HitRate*100, b.Enforce, limit)
			if len(b.Warnings) > 0 {
				fmt.Fprintf(out, "预算告警: %s\n", b.Warnings[len(b.Warnings)-1])
			}
			if len(st.Inbox) > 0 {
				fmt.Fprintln(out)
				lastReason := ""
				if last, ok := st.Inbox[len(st.Inbox)-1].(map[string]any); ok {
					lastReason, _ = last["reason"].(string)
				}
				fmt.Fprintf(out, "inbox（需要人处理）: %d 条，最近: %s\n", len(st.Inbox), lastReason)
			}
			return nil
		},
	}
}

// evidenceFlag 证据对象取标记：JSON 往返后是 map；进程内可能是结构体。
func evidencePass(ev any, key string) string {
	switch v := ev.(type) {
	case map[string]any:
		if b, ok := v[key].(bool); ok && b {
			return "✓"
		}
		return "✗"
	default:
		return "—"
	}
}

func evidenceVerdict(ev any) string {
	m, ok := ev.(map[string]any)
	if !ok {
		return "—"
	}
	verdict, _ := m["verdict"].(string)
	return verdict
}

// orchestratorAlive pid 文件存在且进程存活。
func orchestratorAlive(taskDir string) (bool, int) {
	raw, err := os.ReadFile(machine.PidFile(taskDir))
	if err != nil {
		return false, 0
	}
	var pf struct {
		PID int `json:"pid"`
	}
	if json.Unmarshal(raw, &pf) != nil || pf.PID <= 0 {
		return false, 0
	}
	if err := signalZero(pf.PID); err != nil {
		return false, 0
	}
	return true, pf.PID
}

// warnIfStalled A5①（DESIGN §6 v0.18）：上次推进距今超过看门狗 wall-clock
// 阈值 → 显式"疑似停滞/断链"提示。LastProgressAt 缺失/不可解析时静默
// （无信息不妄言）；阈值取任务配置的 watchdog.wall-timeout-ms（缺省 1h）。
func warnIfStalled(out io.Writer, taskDir string, st *state.State) {
	if st.LastProgressAt == nil {
		return
	}
	ts, err := time.Parse("2006-01-02T15:04:05.000Z", *st.LastProgressAt)
	if err != nil {
		return
	}
	wall := int64(3_600_000)
	if wd, werr := os.Getwd(); werr == nil {
		if cfg, cerr := config.Load(taskDir, wd); cerr == nil && cfg.Watchdog.WallTimeoutMs > 0 {
			wall = cfg.Watchdog.WallTimeoutMs
		}
	}
	if stalled := time.Since(ts); stalled > time.Duration(wall)*time.Millisecond {
		fmt.Fprintf(out, "⚠ 上次推进距今 %s，超过看门狗 wall-clock 阈值——疑似停滞/断链；检查编排器进程或 orchestrator.log\n",
			stalled.Round(time.Second))
	}
}
