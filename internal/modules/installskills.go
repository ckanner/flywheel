// install-skills：把本仓库的 skills/ 暴露给 kimi code 与 pi——经共享的
// Agent Skills 安装点（~/.agents/skills/<name>，或项目的 .agents/skills/）。
// 默认 SYMLINK（单一信源，改动即时生效）；Copy=true 做分发快照。幂等：已存在
// 的条目如实上报并跳过，除非 Force 替换。
package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ckanner/flywheel/internal/contract"
)

// fileExists 任意类型条目存在性（含损坏符号链接）。
func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

const AnchorFile = "flywheel-home.txt"

// SkillInstallResult 单个 skill 的安装结果。
type SkillInstallResult struct {
	Name   string `json:"name"`
	Dest   string `json:"dest"`
	Action string `json:"action"` // linked | copied | skipped | stale | error
	Reason string `json:"reason,omitempty"`
}

// InstallSummary installSkills 结果。
type InstallSummary struct {
	TargetBase string               `json:"targetBase"`
	Scope      string               `json:"scope"`
	Results    []SkillInstallResult `json:"results"`
}

// ListRepoSkills 本仓库的 skill 目录（必须含带 name+description 的 SKILL.md）。
func ListRepoSkills(root string) []string {
	skillsDir := filepath.Join(root, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && fileExists(filepath.Join(skillsDir, e.Name(), "SKILL.md")) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// skillFrontmatterOk frontmatter 完整性检查：标准要求 name + description。
func skillFrontmatterOk(skillDir string) bool {
	raw, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return false
	}
	data, _, err := contract.ParseFrontmatter(raw, "SKILL.md")
	if err != nil {
		return false
	}
	name, _ := data["name"].(string)
	desc, _ := data["description"].(string)
	return name != "" && desc != ""
}

// InstallSkillsOpts InstallSkills 入参。
type InstallSkillsOpts struct {
	Force   bool   // 替换已有条目
	Project string // 非空 → 安装到 <project>/.agents/skills
	HomeDir string // 用户级基目录（空 = os.UserHomeDir）
	Root    string // 仓库根（必填）——本函数只做软链安装（--link 开发机路径）；
	// 分发安装（所有人一致的默认）走 CLI 侧内嵌快照物化。
}

// InstallSkills 幂等软链安装。真实目录绝不被自动删除，即使 Force（P1-11）。
func InstallSkills(o InstallSkillsOpts) (InstallSummary, error) {
	skillsDir := filepath.Join(o.Root, "skills")
	targetBase := filepath.Join(o.HomeDir, ".agents", "skills")
	scope := "user"
	if o.Project != "" {
		targetBase = filepath.Join(o.Project, ".agents", "skills")
		scope = fmt.Sprintf("project (%s)", o.Project)
	}
	if o.HomeDir == "" && o.Project == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return InstallSummary{}, err
		}
		targetBase = filepath.Join(home, ".agents", "skills")
	}
	if err := os.MkdirAll(targetBase, 0o755); err != nil {
		return InstallSummary{}, err
	}
	summary := InstallSummary{TargetBase: targetBase, Scope: scope}
	for _, name := range ListRepoSkills(o.Root) {
		src := filepath.Join(skillsDir, name)
		dest := filepath.Join(targetBase, name)
		if !skillFrontmatterOk(src) {
			summary.Results = append(summary.Results, SkillInstallResult{
				Name: name, Dest: dest, Action: "error",
				Reason: "SKILL.md frontmatter missing name/description"})
			continue
		}
		lst, lerr := os.Lstat(dest)
		if lerr == nil { // 不存在（含竞态中刚被删掉）都视为空位，落到下方安装
			if lst.Mode()&os.ModeSymlink != 0 {
				target, terr := os.Readlink(dest)
				if terr == nil && !o.Force {
					// 如实上报 stale 链接而不是"already installed"（P1-11）。
					if target != src || !fileExists(target) {
						summary.Results = append(summary.Results, SkillInstallResult{
							Name: name, Dest: dest, Action: "stale",
							Reason: fmt.Sprintf("symlink -> %s (expected %s; re-run with --force)", target, src)})
						continue
					}
					summary.Results = append(summary.Results, SkillInstallResult{
						Name: name, Dest: dest, Action: "skipped", Reason: "already installed"})
					continue
				}
				_ = os.Remove(dest) // 符号链接：只删链接本身，绝不碰目标
			} else if lst.Mode().IsRegular() {
				if !o.Force {
					summary.Results = append(summary.Results, SkillInstallResult{
						Name: name, Dest: dest, Action: "skipped", Reason: "already installed (file)"})
					continue
				}
				_ = os.Remove(dest) // 普通文件：可安全移除
			} else {
				// 真目录绝不自动删除，即使 --force（P1-11）。
				summary.Results = append(summary.Results, SkillInstallResult{
					Name: name, Dest: dest, Action: "error",
					Reason: "a real directory already exists here — refusing to remove it; handle it manually"})
				continue
			}
		}
		if err := os.Symlink(src, dest); err != nil {
			summary.Results = append(summary.Results, SkillInstallResult{Name: name, Dest: dest, Action: "error", Reason: err.Error()})
			continue
		}
		summary.Results = append(summary.Results, SkillInstallResult{Name: name, Dest: dest, Action: "linked"})
	}
	return summary, nil
}

// RenderActionText CLI 人话输出用（P3 修复：含 'stale' 动作）。
var actionOrder = map[string]int{"linked": 0, "copied": 1, "skipped": 2, "stale": 3, "error": 4}

func renderActionText(s InstallSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "installed skills -> %s (%s)\n", s.TargetBase, s.Scope)
	for _, r := range s.Results {
		line := fmt.Sprintf("  %-8s %s", r.Action, r.Name)
		if r.Reason != "" {
			line += " — " + r.Reason
		}
		b.WriteString(line + "\n")
	}
	_ = actionOrder // 排序由调用方按需处理；此处保序输出与 Node 一致
	return strings.TrimRight(b.String(), "\n")
}
