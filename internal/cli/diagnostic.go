package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/diagnostic"
	"github.com/spf13/cobra"
)

// Diagnostic commands own short-lived process groups too. Convert termination
// into context cancellation so watchdog cleanup finishes before the CLI exits.
func diagnosticContext(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
}

func configCmd() *cobra.Command {
	var cwd, taskDir string
	var asJSON bool
	cmd := &cobra.Command{Use: "config", Short: "查看模型与渠道配置"}
	inspect := &cobra.Command{
		Use: "inspect", Short: "只读查看实际生效的配置和阶段绑定", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := diagnosticContext(cmd)
			defer cancel()
			view, err := diagnostic.InspectContext(ctx, cwd, taskDir)
			if err != nil {
				return err
			}
			if asJSON {
				printJSON(cmd, view)
			} else {
				printConfiguration(cmd, view)
			}
			return nil
		},
	}
	inspect.Flags().StringVar(&cwd, "cwd", ".", "工作目录；配置从其 Git 根目录发现")
	inspect.Flags().StringVar(&taskDir, "task-dir", "", "可选任务目录；相对路径按 --cwd 解析")
	inspect.Flags().BoolVar(&asJSON, "json", false, "输出有效配置摘要 JSON（不包含凭据值）")
	cmd.AddCommand(inspect, configMigrateCmd())
	return cmd
}

func configMigrateCmd() *cobra.Command {
	options := config.MigrationOptions{}
	var asJSON bool
	cmd := &cobra.Command{
		Use: "migrate", Short: "预览或显式应用订阅模型预设，备份并保留其他配置", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := diagnosticContext(cmd)
			defer cancel()
			report, _ := diagnostic.MigrateContext(ctx, options)
			if asJSON {
				printJSON(cmd, report)
			} else {
				printMigration(cmd, report)
			}
			if !report.OK {
				return &exitError{code: 1}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&options.Preset, "preset", "", "迁移预设：subscriptions（ChatGPT、Kimi Code、DeepSeek Flash）")
	cmd.Flags().StringVar(&options.Cwd, "cwd", ".", "工作目录；沿用 config inspect 的 Git 根目录发现规则")
	cmd.Flags().StringVar(&options.TaskDir, "task-dir", "", "可选任务目录；相对路径按 --cwd 解析")
	cmd.Flags().BoolVar(&options.Apply, "apply", false, "明确应用；默认仅预览，已有文件先备份")
	cmd.Flags().StringVar(&options.ExpectSHA256, "expect-sha256", "", "可选：要求源文件与预览时的 SHA-256 一致")
	cmd.Flags().BoolVar(&asJSON, "json", false, "输出来源、逐项变化、备份路径和实际模型绑定")
	return cmd
}

func printMigration(cmd *cobra.Command, report *diagnostic.MigrationReport) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "配置迁移: %s\n来源: %s (%s)\n目标: %s\n源 SHA-256: %s\n", report.Preset, report.Source, report.Scope, report.Target, report.SHA256)
	for _, change := range report.Changes {
		fmt.Fprintf(out, "  %s %s\n", change.Action, change.Path)
	}
	for _, blocker := range report.Blockers {
		fmt.Fprintf(out, "  阻断: %s\n", blocker)
	}
	if report.Error != "" {
		fmt.Fprintf(out, "迁移失败: %s\n", report.Error)
	}
	if report.Backup != "" {
		fmt.Fprintf(out, "原始文件备份: %s\n", report.Backup)
	}
	switch {
	case report.Applied:
		fmt.Fprintln(out, "配置已应用；下次引擎启动时生效。")
	case report.OK && !report.Changed:
		fmt.Fprintln(out, "配置已符合预设，无需修改或创建备份。")
	case report.OK:
		fmt.Fprintln(out, "仅预览，尚未写入；添加 --apply 明确应用，可用 --expect-sha256 绑定此预览。")
	}
	for _, note := range report.Notes {
		fmt.Fprintln(out, note)
	}
	if report.Configuration != nil {
		if report.ConfigurationStatus == "proposed" {
			fmt.Fprintln(out, "拟议配置（尚未应用）:")
		} else {
			fmt.Fprintln(out, "重新读取的实际配置（供下次引擎启动使用）:")
		}
		printConfiguration(cmd, report.Configuration)
	}
}

func printConfiguration(cmd *cobra.Command, view *diagnostic.Configuration) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "配置来源: %s (%s)\n工作目录: %s\n任务目录: %s\n", view.Source, view.Scope, view.Cwd, view.TaskDir)
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "阶段\tProfile\tHarness\tProvider\tChannel\tModel\tThinking\t权限")
	for _, b := range view.Bindings {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", b.ID, b.Profile, b.Harness, b.Provider, b.Channel, b.Model, b.Thinking, b.PermissionMode)
	}
	_ = table.Flush()
	fmt.Fprintln(out, "此命令只解析配置；账号授权、额度与模型服务能力未验证。")
}

func doctorCmd() *cobra.Command {
	var cwd, taskDir string
	var asJSON, probe bool
	cmd := &cobra.Command{
		Use: "doctor", Short: "只读安装与任务预检；可选 DSH SDK 无推理探测", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := diagnosticContext(cmd)
			defer cancel()
			report := diagnostic.Doctor(ctx, cwd, taskDir, probe)
			if asJSON {
				printJSON(cmd, report)
			} else {
				printDiagnostic(cmd, report)
			}
			if !report.OK {
				return &exitError{code: 1}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", ".", "工作目录；配置从其 Git 根目录发现")
	cmd.Flags().StringVar(&taskDir, "task-dir", "", "可选任务目录；相对路径按 --cwd 解析")
	cmd.Flags().BoolVar(&asJSON, "json", false, "输出诊断 JSON；阻断项退出码为 1")
	cmd.Flags().BoolVar(&probe, "probe", false, "探测 DSH SDK 配置、目录和沙箱；不发送推理请求")
	return cmd
}

func printDiagnostic(cmd *cobra.Command, report diagnostic.Report) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "工作目录: %s\n任务目录: %s\n", report.Cwd, report.TaskDir)
	for _, check := range report.Checks {
		fmt.Fprintf(out, "[%s] %s: %s\n", check.Status, check.ID, check.Summary)
		if check.Remedy != "" {
			fmt.Fprintf(out, "  建议: %s\n", check.Remedy)
		}
	}
	if report.OK {
		fmt.Fprintln(out, "检查结束：未发现阻断项。")
	} else {
		fmt.Fprintln(out, "检查结束：存在阻断项，请修复后重试。")
	}
	fmt.Fprintln(out, "未发送推理请求；账号授权、额度与实际模型服务能力未验证。")
}
