// intake / garden 外挂模块子命令（§4.5/§4.6）。
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/modules"
)

func intakeCmd() *cobra.Command {
	var source string
	cmd := &cobra.Command{
		Use:   "intake <docs/slug>",
		Short: "外挂：扫任务源 → triage-inbox 候选 + LLM 分诊建议（不冻结需求）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskDir, err := resolveTaskDir(args[0])
			if err != nil {
				return err
			}
			cfg, err := config.Load(taskDir, mustCwd())
			if err != nil {
				return err
			}
			sum, err := modules.RunIntake(modules.CtxOpts{
				TaskDir: taskDir, Cwd: mustCwd(), Config: cfg,
				Drivers: drivers.CreateDrivers(cfg),
				Log:     logf(cmd),
			}, source, nil)
			printJSON(cmd, sum)
			if sum.Triaged["failed"] > 0 {
				return &exitError{code: 3}
			}
			return err
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "只扫描指定的源名")
	return cmd
}

func gardenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "garden <docs/slug>",
		Short: "外挂：分析 retro/知识资产 → garden-proposals.md 提案（需人审）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskDir, err := resolveTaskDir(args[0])
			if err != nil {
				return err
			}
			cfg, err := config.Load(taskDir, mustCwd())
			if err != nil {
				return err
			}
			sum, err := modules.RunGarden(modules.CtxOpts{
				TaskDir: taskDir, Cwd: mustCwd(), Config: cfg,
				Drivers: drivers.CreateDrivers(cfg),
				Log:     logf(cmd),
			})
			printJSON(cmd, sum)
			if !sum.OK {
				return &exitError{code: 3}
			}
			return err
		},
	}
}

var installActionText = map[string]string{
	"linked":  "已软链",
	"copied":  "已复制",
	"skipped": "已安装，跳过",
	"stale":   "链接已失效（重跑加 --force 替换）", // P3：stale 必须如实出现
	"error":   "错误",
}

func installSkillsCmd() *cobra.Command {
	var force, copy bool
	var project string
	cmd := &cobra.Command{
		Use:   "install-skills",
		Short: "把 skills/ 安装到 ~/.agents/skills（软链默认；--copy 快照；--project 项目级）",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// --project 相对路径解析仅在取不到工作目录时失败（Getwd 恒可用，
			// 见 mustCwd 注释），故无错误分支。
			projectAbs := ""
			if project != "" {
				projectAbs, _ = filepath.Abs(project)
			}
			sum, err := modules.InstallSkills(modules.InstallSkillsOpts{
				Force: force, Copy: copy, Project: projectAbs, Root: repoRoot(),
			})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			method := "软链（默认）"
			if copy {
				method = "复制（快照）"
			}
			fmt.Fprintf(out, "安装范围: %s  目标目录: %s  方式: %s\n", sum.Scope, sum.TargetBase, method)
			errCount := 0
			for _, r := range sum.Results {
				if r.Action == "error" {
					errCount++
				}
				actionText := installActionText[r.Action]
				reason := ""
				if r.Reason != "" {
					reason = fmt.Sprintf("（%s）", r.Reason)
				}
				fmt.Fprintf(out, "  %s: %s%s -> %s\n", r.Name, actionText, reason, r.Dest)
			}
			fmt.Fprintln(out)
			fmt.Fprintln(out, "验证方式: 在 kimi code 或 pi 中调用 /skill:<name>，例如 /skill:flywheel-run。")
			if errCount > 0 {
				return &exitError{code: 1}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "替换已有条目")
	cmd.Flags().BoolVar(&copy, "copy", false, "复制而非软链（发布快照）")
	cmd.Flags().StringVar(&project, "project", "", "安装到 <path>/.agents/skills（项目级）")
	return cmd
}

// repoRoot 定位 flywheel 源码树（skills/ 的宿主）：先从工作目录向上找
// go.mod（开发场景），再从可执行文件目录向上（go run 场景 exe 在临时目录、
// cwd 已被切走时兜底），都找不到退回工作目录。安装后的发布二进制不在源码
// 树内，此时 skills 数据走 skillsembed 内嵌副本，install-skills 属开发命令。
// os.Getwd 对运行中的进程实际不会失败（见 mustCwd 注释），故无错误分支。
func repoRoot() string {
	wd, _ := os.Getwd()
	for _, start := range []string{wd, osExecutableDir()} {
		for d := start; ; d = filepath.Dir(d) {
			if d == filepath.Dir(d) {
				break
			}
			if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
				return d
			}
		}
	}
	return wd
}

// osExecutableDir 可执行文件目录。os.Executable 对运行中的进程实际不会失败
// （darwin _NSGetExecutablePath / linux /proc/self/exe 恒可用；Node 对照
// __dirname 同样恒存在），故不设错误分支——失败时退回 "."，仅少一个候选根。
func osExecutableDir() string {
	exe, _ := os.Executable()
	return filepath.Dir(exe)
}
