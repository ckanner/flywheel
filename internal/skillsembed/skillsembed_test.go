package skillsembed

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/contract"
)

// 本测试即 CI 的 skills 规范门禁（取代 Node skill-format-check.js 纯正则假绿）：
// 真 YAML 解析 + 官方 spec 必填字段 + kimi code 扩展字段（whenToUse/arguments，
// 自动调用依赖它们）+ metadata.version。

func TestListCoversAllRepoSkills(t *testing.T) {
	list := List()
	require.NotEmpty(t, list)
	names := map[string]bool{}
	for _, s := range list {
		names[s.Name] = true
	}
	for _, want := range []string{
		"flywheel-exec", "flywheel-garden", "flywheel-intake", "flywheel-new",
		"flywheel-plan", "flywheel-review", "flywheel-run", "flywheel-status",
		"flywheel-stop", "flywheel-verify",
	} {
		assert.True(t, names[want], "缺少 %s", want)
	}
	assert.Len(t, list, len(names), "不得有重名条目")
	// 排序稳定。
	for i := 1; i < len(list); i++ {
		assert.Less(t, list[i-1].Name, list[i].Name)
	}
}

func TestReadKnownAndUnknown(t *testing.T) {
	raw, err := Read("flywheel-run")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(raw, "---\n"))
	assert.Contains(t, raw, "name: flywheel-run")

	_, err = Read("no-such-skill")
	assert.ErrorContains(t, err, `unknown skill`)

	for _, bad := range []string{"../etc", "a/b", `x\y`, ".", ".."} {
		_, err = Read(bad)
		assert.ErrorContains(t, err, "invalid skill name", bad)
	}
}

// TestSkillSpecCompliance 每个内置 SKILL.md 必须满足：
//   - frontmatter 合法 YAML（P1-2 回归）
//   - name 与目录名一致、description 非空（官方 spec 必填）
//   - metadata.version 非空（Agent Skills spec 的 metadata map）
//   - whenToUse / arguments 非空（kimi code 扩展；自动调用的 load-bearing 字段）
func TestSkillSpecCompliance(t *testing.T) {
	for _, s := range List() {
		t.Run(s.Name, func(t *testing.T) {
			raw, err := Read(s.Name)
			require.NoError(t, err)
			fm, _, err := contract.ParseFrontmatter([]byte(raw), s.Path)
			require.NoError(t, err, "frontmatter 必须是合法 YAML")

			name, _ := fm["name"].(string)
			desc, _ := fm["description"].(string)
			wtu, _ := fm["whenToUse"].(string)
			meta, _ := fm["metadata"].(map[string]any)
			version, _ := meta["version"].(string)
			args := fm["arguments"]

			assert.Equal(t, s.Name, name, "name 必须与目录名一致")
			assert.NotEmpty(t, desc, "description 必填")
			assert.NotEmpty(t, wtu, "whenToUse 必填（kimi code 自动调用依赖）")
			assert.NotEmpty(t, version, "metadata.version 必填")
			assert.NotNil(t, args, "arguments 必填（kimi code 扩展）")

			// compatibility 若存在必须已是字符串（曾因裸标量含 ": " 使整个
			// frontmatter 解析失败——P1-2 根因）。
			if comp, ok := fm["compatibility"]; ok {
				compStr, isStr := comp.(string)
				assert.True(t, isStr, "compatibility 必须是引号字符串")
				if isStr {
					assert.NotEmpty(t, strings.TrimSpace(compStr))
				}
			}
		})
	}
}

func TestListFromInjectedFS(t *testing.T) {
	good := "---\nname: ok-skill\ndescription: fine\n---\nbody"
	fsys := fstest.MapFS{
		"good/SKILL.md":     &fstest.MapFile{Data: []byte(good)},
		"noskill/README.md": &fstest.MapFile{Data: []byte("not a skill")}, // 无 SKILL.md → 跳过
		"badfm/SKILL.md":    &fstest.MapFile{Data: []byte("no frontmatter here")},
		"embed.go":          &fstest.MapFile{Data: []byte("package skills")}, // 非目录 → 跳过
	}
	list := ListFrom(fsys)
	require.Len(t, list, 1)
	assert.Equal(t, "ok-skill", list[0].Name)
	assert.Equal(t, "fine", list[0].Description)
	assert.Equal(t, "good/SKILL.md", list[0].Path)
}

type errFS struct{}

func (errFS) Open(string) (fs.File, error) { return nil, fmt.Errorf("boom") }

func TestListFromErroringFS(t *testing.T) {
	assert.Nil(t, ListFrom(errFS{}))
}
