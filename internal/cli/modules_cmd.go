// intake / garden / install-skills 子命令（§4.5/§4.6 外挂模块 + skills 安装）。
package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/machine"
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
			cwd := machine.ProjectRoot(mustCwd())
			cfg, err := config.Load(taskDir, cwd)
			if err != nil {
				return err
			}
			sum, err := modules.RunIntake(modules.CtxOpts{
				TaskDir: taskDir, Cwd: cwd, Config: cfg,
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
			cwd := machine.ProjectRoot(mustCwd())
			cfg, err := config.Load(taskDir, cwd)
			if err != nil {
				return err
			}
			sum, err := modules.RunGarden(modules.CtxOpts{
				TaskDir: taskDir, Cwd: cwd, Config: cfg,
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
	var force, link, asJSON bool
	var project string
	cmd := &cobra.Command{
		Use:   "install-skills",
		Short: "安装 skills（内嵌快照复制到 ~/.agents/skills，安装用法人人一致；--link 开发机软链源码树）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectAbs := ""
			if project != "" {
				projectAbs, _ = filepath.Abs(project)
			}
			gp, created, configErr := config.MaterializeGlobal()
			configResult := installConfigResult{Path: gp, Action: "existing"}
			if created {
				configResult.Action = "created"
			}
			if configErr != nil {
				configResult.Action, configResult.Error = "error", configErr.Error()
			}
			var sum modules.InstallSummary
			var installErr error
			method := "embedded"
			if link {
				method = "link"
				root, haveSource := repoRoot()
				if !haveSource {
					installErr = fmt.Errorf("--link 需要在 flywheel 源码树内运行（未找到 go.mod）")
				} else {
					sum, installErr = modules.InstallSkills(modules.InstallSkillsOpts{
						Force: force, Project: projectAbs, Root: root,
					})
				}
			} else {
				sum, installErr = installEmbeddedSnapshot(projectAbs)
			}
			return renderSkillInstall(cmd, sum, method, configResult, errors.Join(installErr, configErr), asJSON)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "仅 --link：替换已有链接或文件，不删除真实目录")
	cmd.Flags().BoolVar(&link, "link", false, "软链到源码仓库（开发机专用，需在源码树内）")
	cmd.Flags().StringVar(&project, "project", "", "安装到 <path>/.agents/skills（项目级）")
	cmd.Flags().BoolVar(&asJSON, "json", false, "输出一份包含全部内嵌 skills 安装结果的 JSON")
	return cmd
}

type installConfigResult struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Error  string `json:"error,omitempty"`
}

type skillInstallResponse struct {
	Protocol int  `json:"protocol"`
	OK       bool `json:"ok"`
	modules.InstallSummary
	Method string              `json:"method"`
	Config installConfigResult `json:"config"`
	Error  string              `json:"error,omitempty"`
}

func installJSONRequested(args []string) bool {
	requested := false
	for _, arg := range args {
		switch arg {
		case "--":
			return requested
		case "--json", "--json=true":
			requested = true
		case "--json=false":
			requested = false
		}
	}
	return requested
}

// Every embedded skill gets a result, including before installation can start.
// This prevents an empty/partial list from being mistaken for a successful setup.
func skillInstallReport(sum modules.InstallSummary, method string, cfg installConfigResult, installErr error) skillInstallResponse {
	reply := skillInstallResponse{Protocol: 1, OK: installErr == nil, Method: method, Config: cfg, InstallSummary: sum}
	if installErr != nil {
		reply.Error = installErr.Error()
	}
	seen := make(map[string]bool)
	for _, result := range reply.Results {
		seen[result.Name] = true
		if result.Action == "error" {
			reply.OK = false
		}
	}
	for _, skill := range skillsembed.List() {
		if seen[skill.Name] {
			continue
		}
		reason := reply.Error
		if reason == "" {
			reason = "installer did not report this embedded skill; use the matching Flywheel source or snapshot installation"
		}
		dest := ""
		if reply.TargetBase != "" {
			dest = filepath.Join(reply.TargetBase, skill.Name)
		}
		reply.Results = append(reply.Results, modules.SkillInstallResult{Name: skill.Name, Dest: dest, Action: "error", Reason: reason})
		reply.OK = false
	}
	return reply
}

func renderSkillInstall(cmd *cobra.Command, sum modules.InstallSummary, method string, cfg installConfigResult, installErr error, asJSON bool) error {
	reply := skillInstallReport(sum, method, cfg, installErr)
	if asJSON {
		printJSON(cmd, reply)
	} else {
		state := "已存在"
		if cfg.Action == "created" {
			state = "本次新建（可直接编辑）"
		}
		if cfg.Action == "error" {
			state = "失败"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "全局配置: %s — %s\n", cfg.Path, state)
		if reply.Error != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "[flywheel] 错误: %s\n", reply.Error)
		}
		label := "复制（内嵌快照）"
		if method == "link" {
			label = "软链（源码树，开发机专用）"
		}
		_ = renderInstall(cmd, reply.TargetBase, reply.Scope, label, reply.Results)
	}
	if !reply.OK {
		return &exitError{code: 1}
	}
	return nil
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
	fmt.Fprintln(out, "验证方式: flywheel skills list / flywheel skills read <name>；在宿主中确认已加载 .agents/skills，并按宿主支持的语法调用。")
	if errCount > 0 {
		return &exitError{code: 1}
	}
	return nil
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
