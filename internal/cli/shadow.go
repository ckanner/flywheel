package cli

import (
	"os"
	"time"

	"github.com/ckanner/flywheel/internal/shadow"
	"github.com/spf13/cobra"
)

func shadowCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "shadow", Short: "独立 JEV 影子评估；不启动或改变实际任务"}
	var taskDir, mode string
	prepare := &cobra.Command{Use: "prepare", Short: "只读准备任务摘要 JSON；发送前审阅内容", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dataset, err := shadow.Prepare(taskDir, mode)
			if err != nil {
				return err
			}
			printJSON(cmd, dataset)
			return nil
		}}
	prepare.Flags().StringVar(&taskDir, "task-dir", "", "实际任务资料目录")
	prepare.Flags().StringVar(&mode, "mode", "", "可选基线预览 auto|quick|full；不改任务模式")
	cmd.AddCommand(prepare, shadowRunCmd(), shadowReportCmd())
	return cmd
}

func shadowRunCmd() *cobra.Command {
	var input, directory, keyEnv string
	options := shadow.Options{}
	cmd := &cobra.Command{Use: "run", Short: "显式调用 System One 并保存影子记录（产生 API 用量）", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dataset, err := shadow.LoadDataset(input)
			if err != nil {
				return err
			}
			options.APIKey = os.Getenv(keyEnv)
			ctx, stop := diagnosticContext(cmd)
			defer stop()
			report, err := shadow.Run(ctx, dataset, directory, options)
			if err != nil {
				return err
			}
			printJSON(cmd, report)
			if report.Cancelled {
				return &exitError{code: 143}
			}
			if report.Summary.Errors+report.Summary.Unknown+report.Summary.NotRun > 0 {
				return &exitError{code: 3}
			}
			return nil
		}}
	cmd.Flags().StringVar(&input, "input", "", "已审阅的数据集 JSON")
	cmd.Flags().StringVar(&directory, "directory", "", "私有结果目录；同目录不会自动重发未知调用")
	cmd.Flags().StringVar(&options.Endpoint, "endpoint", "", "明确的 HTTPS /v1/systemone 地址")
	cmd.Flags().StringVar(&options.Model, "model", "jev", "MeRouter 模型 ID")
	cmd.Flags().StringVar(&options.ExpectedModel, "expected-model", "jev-1.14.0", "必须匹配的响应模型版本")
	cmd.Flags().StringVar(&keyEnv, "key-env", "MEROUTER_API_KEY", "密钥环境变量名；不要传密钥值")
	cmd.Flags().DurationVar(&options.Timeout, "timeout", 5*time.Second, "单次调用时限，不自动重试，最大 30s")
	cmd.Flags().IntVar(&options.MaxCalls, "max-calls", 20, "本次新调用上限，1..100；超出先拒绝整批")
	return cmd
}

func shadowReportCmd() *cobra.Command {
	var input, directory string
	cmd := &cobra.Command{Use: "report", Short: "离线汇总模式分歧、未知用量与独立标签；不调用模型", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dataset, err := shadow.LoadDataset(input)
			if err != nil {
				return err
			}
			report, err := shadow.ReadReport(dataset, directory)
			if err != nil {
				return err
			}
			printJSON(cmd, report)
			return nil
		}}
	cmd.Flags().StringVar(&input, "input", "", "同一数据集，可补充人工标签")
	cmd.Flags().StringVar(&directory, "directory", "", "影子结果目录")
	return cmd
}
