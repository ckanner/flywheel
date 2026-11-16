// status 子命令：只读——读 state.json/指标，讲进度（人话输出）。
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
)

// Status only probes PID existence with signal 0. Stop never signals a PID read
// from disk; it uses the run lease and token-controlled file request instead.
var processSignal = syscall.Kill

func signalZero(pid int) error { return processSignal(pid, 0) }

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
				if preparation, preparationErr := state.LoadPreparation(taskDir); preparationErr == nil {
					fmt.Fprintf(cmd.OutOrStdout(), "调查: %s  状态: %s  revision: %d\n目标: %s\n动作: %d/%d  已耗时: %d ms\n原因: %s\n", preparation.Contract.ID, preparation.Status, preparation.Revision, preparation.Contract.Goal, preparation.ActionsUsed, preparation.Contract.Limits.MaxActions, preparation.ElapsedMS, preparation.Reason)
					if preparation.Report != nil {
						fmt.Fprintln(cmd.OutOrStdout(), preparation.Report.Summary)
					}
					return nil
				} else if !os.IsNotExist(preparationErr) {
					return preparationErr
				}
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
			if p := st.Planning; p != nil {
				startAllowed := p.CurrentDesign != nil && planning.StartAllowed(p, *p.CurrentDesign)
				fmt.Fprintf(out, "规划: %s  策略: %s  累计轮次: %d（每请求最多 %d）\n实施启动已授权: %t\n原因: %s\n", p.Phase, p.Policy, p.Rounds, p.MaxRounds, startAllowed, p.Reason)
				if p.CurrentDesign != nil {
					fmt.Fprintf(out, "当前方案: revision %d  digest %s\n", p.CurrentDesign.Revision, p.CurrentDesign.Digest)
				}
			}
			if st.Workflow != nil {
				fmt.Fprintf(out, "流程: %s → %s（策略 v%d）\n原因: %s\n", st.Workflow.Requested, st.Workflow.Effective, st.Workflow.Version, st.Workflow.Reason)
				for _, stage := range []string{"plan", "judge"} {
					applicability := st.Workflow.Stages[stage]
					if !applicability.Applicable {
						fmt.Fprintf(out, "%s 不适用: %s\n", stage, applicability.Reason)
					}
				}
				for _, upgrade := range st.Workflow.Upgrades {
					fmt.Fprintf(out, "流程升级 %s → %s: %s（%s）\n", upgrade.From, upgrade.To, upgrade.Reason, upgrade.At)
				}
			} else if st.Planning != nil {
				fmt.Fprintln(out, "流程: 实现尚未激活（先完成方案与执行计划审查）")
			} else {
				fmt.Fprintln(out, "流程: full（旧任务保持完整流程）")
			}
			printSessionSummaries(out, st)
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
			if b.UnknownUsageCalls > 0 || b.UnknownCostCalls > 0 {
				fmt.Fprintf(out, "已知量下限：%d 次调用未报告 usage，%d 次调用成本未知；订阅额度不能按 $0 解释。\n", b.UnknownUsageCalls, b.UnknownCostCalls)
			}
			limit := "未设"
			if b.DailyTokenLimit != nil {
				limit = fmt.Sprint(*b.DailyTokenLimit)
			}
			fmt.Fprintf(out, "缓存命中率: %.1f%%  预算档: %s（日上限 %s）\n", b.Cache.HitRate*100, b.Enforce, limit)
			if len(b.Warnings) > 0 {
				fmt.Fprintf(out, "预算告警: 共 %d 条，最近: %s\n", len(b.Warnings), b.Warnings[len(b.Warnings)-1])
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

// orchestratorAlive requires a held lifetime lease as well as a displayable PID.
func orchestratorAlive(taskDir string) (bool, int) {
	held, err := machine.RunLeaseHeld(taskDir)
	if err != nil || !held {
		return false, 0
	}
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
	if err := signalZero(pf.PID); err != nil && !errors.Is(err, syscall.EPERM) {
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
		if cfg, cerr := config.LoadReadOnly(taskDir, machine.ProjectRoot(wd)); cerr == nil && cfg.Watchdog.WallTimeoutMs > 0 {
			wall = cfg.Watchdog.WallTimeoutMs
		}
	}
	if stalled := time.Since(ts); stalled > time.Duration(wall)*time.Millisecond {
		fmt.Fprintf(out, "⚠ 上次推进距今 %s，超过看门狗 wall-clock 阈值——疑似停滞/断链；检查编排器进程或 orchestrator.log\n",
			stalled.Round(time.Second))
	}
}
