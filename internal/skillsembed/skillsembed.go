// Package skillsembed — `flywheel skills list/read` 的数据层（docs/design.md §12）。
// 仓库 skills/ 目录经 go:embed 编进二进制（skills.FS），本包在其上提供
// List/Read/DirFS 三个纯数据函数；frontmatter 解析复用
// internal/contract.ParseFrontmatter，与契约层同一套 YAML 语义。
package skillsembed

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/skills"
)

// Skill 一个已安装（仓库内置）skill 的摘要。
type Skill struct {
	Name        string
	Description string
	WhenToUse   string
	Path        string // embed FS 内路径，如 "skills/flywheel-intake/SKILL.md"
}

// List 枚举全部内置 skill，按名称排序。判定标准与 install-skills 一致：
// 目录内含 SKILL.md 即为 skill。注意：skills.FS 的根即 skills/ 目录
// （embed 指令相对声明文件），路径无 "skills/" 前缀。
func List() []Skill { return ListFrom(skills.FS) }

// ListFrom 从任意 FS 枚举 skill（fsys 根即 skills 目录；注入点供测试覆盖
// 坏数据分支——embed FS 本身编译期保证好数据）。
func ListFrom(fsys fs.FS) []Skill {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil
	}
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue // embed.go 自身等非目录条目
		}
		path := e.Name() + "/SKILL.md"
		raw, err := fs.ReadFile(fsys, path)
		if err != nil {
			continue // 无 SKILL.md 的目录不是 skill（与 listRepoSkills 同判据）
		}
		fm, _, err := contract.ParseFrontmatter(raw, path)
		if err != nil {
			continue // frontmatter 缺失/非法：不列入（加载期坏数据绝不外泄）
		}
		out = append(out, Skill{
			Name:        str(fm["name"]),
			Description: str(fm["description"]),
			WhenToUse:   str(fm["whenToUse"]),
			Path:        path,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Read 返回指定 skill 的 SKILL.md 全文。name 为目录名（如 "flywheel-run"）。
func Read(name string) (string, error) {
	if strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		return "", fmt.Errorf("invalid skill name: %q", name)
	}
	raw, err := skills.FS.ReadFile(name + "/SKILL.md")
	if err != nil {
		return "", fmt.Errorf("unknown skill %q", name)
	}
	return string(raw), nil
}

// DirFS 返回指定 skill 目录的子文件系统（安装器物化内嵌快照用）。
func DirFS(name string) (fs.FS, error) {
	if _, err := Read(name); err != nil {
		return nil, err
	}
	return fs.Sub(skills.FS, name)
}

// str frontmatter 标量取值：缺省/非字符串一律空串。
func str(v any) string {
	s, _ := v.(string)
	return s
}
