// intake / garden / install-skills 子命令（§4.5/§4.6 外挂模块 + skills 安装）。
package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/modules"
	"github.com/ckanner/flywheel/internal/skillsembed"
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
	"stale":   "链接已失效（重跑加 --force 替换）",
	"error":   "错误",
}

func installSkillsCmd() *cobra.Command {
	var force, copy bool
	var project string
	cmd := &cobra.Command{
		Use:   "install-skills",
		Short: "把 skills/ 安装到 ~/.agents/skills（软链默认；--copy 快照；--project 项目级）",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectAbs := ""
			if project != "" {
				projectAbs, _ = filepath.Abs(project)
			}
			root, haveSource := repoRoot()
			if haveSource {
				sum, err := modules.InstallSkills(modules.InstallSkillsOpts{
					Force: force, Copy: copy, Project: projectAbs, Root: root,
				})
				if err != nil {
					return err
				}
				method := "软链（默认）"
				if copy {
					method = "复制（快照）"
				}
				return renderInstall(cmd, sum.TargetBase, sum.Scope, method, sum.Results)
			}
			sum, serr := installEmbeddedSnapshot(projectAbs)
			if serr != nil {
				return serr
			}
			fmt.Fprintln(cmd.OutOrStdout(),
				"未检测到 flywheel 源码树 —— 已从二进制内嵌快照复制安装（软链需要源码仓库）。")
			return renderInstall(cmd, sum.TargetBase, sum.Scope, "复制（内嵌快照）", sum.Results)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "替换已有条目")
	cmd.Flags().BoolVar(&copy, "copy", false, "复制而非软链（发布快照）")
	cmd.Flags().StringVar(&project, "project", "", "安装到 <path>/.agents/skills（项目级）")
	return cmd
}

func renderInstall(cmd *cobra.Command, targetBase, scope, method string, results []modules.SkillInstallResult) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "安装范围: %s  目标目录: %s  方式: %s\n", scope, targetBase, method)
	errCount := 0
	for _, r := range results {
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
}

// installEmbeddedSnapshot 无源码树时的回退：把 go:embed 的 skills 快照复制到
// 目标安装点（方案C）。锚文件注明来源为内嵌快照——skill 无法经它回到源码树，
// 但仍可定位同目录内的兄弟 skill。已存在的真目录绝不自动删除（P1-11）。

// skillFSFor 返回 skill 的内嵌子树；包级变量仅为测试注入错误 FS。
var skillFSFor = skillsembed.DirFS

func installEmbeddedSnapshot(projectAbs string) (modules.InstallSummary, error) {
	targetBase := ""
	scope := "user (embedded snapshot)"
	if projectAbs != "" {
		targetBase = filepath.Join(projectAbs, ".agents", "skills")
		scope = fmt.Sprintf("project (%s)", projectAbs)
	} else if home, herr := os.UserHomeDir(); herr == nil {
		targetBase = filepath.Join(home, ".agents", "skills")
	} else {
		return modules.InstallSummary{}, fmt.Errorf("cannot resolve home directory")
	}
	if err := os.MkdirAll(targetBase, 0o755); err != nil {
		return modules.InstallSummary{}, err
	}
	_ = os.WriteFile(filepath.Join(targetBase, modules.AnchorFile),
		[]byte("(embedded snapshot — no source tree on this machine)\n"), 0o644)

	summary := modules.InstallSummary{TargetBase: targetBase, Scope: scope}
	for _, s := range skillsembed.List() {
		dest := filepath.Join(targetBase, s.Name)
		if lst, lerr := os.Lstat(dest); lerr == nil && lst.IsDir() && lst.Mode()&os.ModeSymlink == 0 {
			summary.Results = append(summary.Results, modules.SkillInstallResult{
				Name: s.Name, Dest: dest, Action: "error",
				Reason: "a real directory already exists here — refusing to remove it; handle it manually"})
			continue
		} else if lerr == nil {
			_ = os.RemoveAll(dest) // 覆盖旧快照/失效链接（幂等物化）
		}
		// List 校验过 name 合法 → 不可能失败（测试注入除外）。
		sub, _ := skillFSFor(s.Name)
		if werr := materializeFS(sub, ".", dest); werr != nil {
			summary.Results = append(summary.Results, modules.SkillInstallResult{
				Name: s.Name, Dest: dest, Action: "error", Reason: werr.Error()})
			continue
		}
		summary.Results = append(summary.Results, modules.SkillInstallResult{
			Name: s.Name, Dest: dest, Action: "copied"})
	}
	return summary, nil
}

// materializeFS 把 fs.FS 内 srcDir 下的全部文件落到 dest（保留相对路径）。
func materializeFS(fsys fs.FS, srcDir, dest string) error {
	return fs.WalkDir(fsys, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, rerr := fs.ReadFile(fsys, path)
		if rerr != nil {
			return rerr
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// repoRoot 定位 flywheel 源码树（skills/ 的宿主）：从工作目录与可执行文件
// 目录分别向上找 go.mod；找不到返回 ok=false —— 调用方走内嵌快照物化
// （方案C），绝不允许静默装零个（review #6）。
func repoRoot() (string, bool) {
	candidates := []string{}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}
	for _, start := range candidates {
		for d := start; ; d = filepath.Dir(d) {
			if d == filepath.Dir(d) {
				break
			}
			if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
				return d, true
			}
		}
	}
	return "", false
}
