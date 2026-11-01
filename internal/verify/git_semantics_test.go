package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistedHeadRefusesIndexFlagsThatHideChangedContent(t *testing.T) {
	for _, flag := range []string{"--assume-unchanged", "--skip-worktree"} {
		t.Run(flag, func(t *testing.T) {
			repo := initRepo(t)
			gitMust(t, repo, "update-index", flag, "--", "README.md")
			writeFile(t, filepath.Join(repo, "README.md"), "different candidate hidden by index flags\n")
			require.Empty(t, gitMust(t, repo, "diff", "HEAD", "--", "README.md"), "真实Git的普通diff会漏掉此候选变化")
			before := gitMust(t, repo, "ls-files", "-v")
			_, err := PersistedHead(repo, nil)
			require.ErrorContains(t, err, "unsupported Git index flag")
			assert.Contains(t, err.Error(), "README.md")
			assert.Equal(t, before, gitMust(t, repo, "ls-files", "-v"), "不默默清除用户标记")
			raw, err := os.ReadFile(filepath.Join(repo, "README.md"))
			require.NoError(t, err)
			assert.Equal(t, "different candidate hidden by index flags\n", string(raw))
		})
	}
}

func TestPersistedHeadChecksExecutableModeEvenWhenGitIgnoresIt(t *testing.T) {
	repo := initRepo(t)
	gitMust(t, repo, "config", "core.filemode", "false")
	require.NoError(t, os.Chmod(filepath.Join(repo, "README.md"), 0o755))
	require.Empty(t, gitMust(t, repo, "diff", "HEAD", "--", "README.md"), "默认diff隐藏未保存的执行位")
	_, err := PersistedHead(repo, nil)
	require.ErrorContains(t, err, "differs from verified candidate")
	assert.Contains(t, err.Error(), "README.md")
	assert.Equal(t, "false", strings.TrimSpace(gitMust(t, repo, "config", "--get", "core.filemode")), "只读检查不修改用户配置")
	gitMust(t, repo, "-c", "core.filemode=true", "add", "--", "README.md")
	gitMust(t, repo, "commit", "-m", "persist executable mode")
	_, err = PersistedHead(repo, nil)
	require.NoError(t, err, "真实候选模式与提交一致时允许完成")
}

func TestPersistedHeadChecksSymlinkTypeEvenWhenGitIgnoresIt(t *testing.T) {
	repo := initRepo(t)
	link := filepath.Join(repo, "source-link")
	require.NoError(t, os.Symlink("README.md", link))
	gitMust(t, repo, "add", "--", "source-link")
	gitMust(t, repo, "commit", "-m", "persist symbolic link")
	gitMust(t, repo, "config", "core.symlinks", "false")
	require.NoError(t, os.Remove(link))
	writeFile(t, link, "README.md")
	require.Empty(t, gitMust(t, repo, "diff", "HEAD", "--", "source-link"), "默认diff将普通文件误当成链接的checkout表示")
	_, err := PersistedHead(repo, nil)
	require.ErrorContains(t, err, "differs from verified candidate")
	assert.Contains(t, err.Error(), "source-link")
	assert.Equal(t, "false", strings.TrimSpace(gitMust(t, repo, "config", "--get", "core.symlinks")), "只读检查不修改用户配置")
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink("README.md", link))
	_, err = PersistedHead(repo, nil)
	require.NoError(t, err, "真实候选类型与提交一致时允许完成")
}

func TestPersistedHeadRefusesEffectiveCleanAndProcessFiltersWithoutRunningThem(t *testing.T) {
	for _, setting := range []string{"clean", "process"} {
		t.Run(setting, func(t *testing.T) {
			repo := initRepo(t)
			writeFile(t, filepath.Join(repo, ".gitattributes"), "README.md filter=custom.driver\n")
			gitMust(t, repo, "add", ".gitattributes")
			gitMust(t, repo, "commit", "-m", "declare optional filter")
			_, err := PersistedHead(repo, nil)
			require.NoError(t, err, "只有属性、没有clean/process实现时仍是普通Git内容")
			marker := filepath.Join(repo, "filter-must-not-run")
			command := fmt.Sprintf("printf invoked > %q; cat", marker)
			gitMust(t, repo, "config", "filter.custom.driver."+setting, command)
			_, err = PersistedHead(repo, nil)
			require.ErrorContains(t, err, "unsupported Git clean/process filter")
			assert.Contains(t, err.Error(), "README.md")
			assert.NoFileExists(t, marker, "检查不能执行用户过滤命令")
			assert.Equal(t, command, strings.TrimSpace(gitMust(t, repo, "config", "--get", "filter.custom.driver."+setting)))
		})
	}
}

func TestPersistedHeadAllowsInactiveFiltersAndUnsetAttributes(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, filepath.Join(repo, ".gitattributes"), "README.md filter=optional\n*.missing working-tree-encoding=UTF-16\n")
	gitMust(t, repo, "add", ".gitattributes")
	gitMust(t, repo, "commit", "-m", "optional attributes")
	gitMust(t, repo, "config", "filter.unused.clean", "cat")
	gitMust(t, repo, "config", "filter.optional.smudge", "cat")
	gitMust(t, repo, "config", "filter.optional.clean", "")
	gitMust(t, repo, "config", "filter.optional.process", "")
	_, err := PersistedHead(repo, nil)
	require.NoError(t, err, "未命中路径的filter及无clean转换的可选配置不阻塞")
	writeFile(t, filepath.Join(repo, ".gitattributes"), "README.md -filter -working-tree-encoding\n")
	gitMust(t, repo, "add", ".gitattributes")
	gitMust(t, repo, "commit", "-m", "explicitly disable conversion attributes")
	_, err = PersistedHead(repo, nil)
	require.NoError(t, err)
}

func TestPersistedHeadRejectsWorkingTreeEncodingButExcludesRuntimeLedger(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, filepath.Join(repo, ".gitattributes"), "README.md working-tree-encoding=UTF-8\n")
	gitMust(t, repo, "add", ".gitattributes")
	gitMust(t, repo, "commit", "-m", "explicit file encoding")
	_, err := PersistedHead(repo, nil)
	require.ErrorContains(t, err, "unsupported Git working-tree-encoding")
	writeFile(t, filepath.Join(repo, ".gitattributes"), "ledger/state.json filter=runtime working-tree-encoding=UTF-8\n")
	writeFile(t, filepath.Join(repo, "ledger", "state.json"), "old state\n")
	gitMust(t, repo, "add", ".gitattributes", "ledger/state.json")
	gitMust(t, repo, "commit", "-m", "attributes limited to excluded runtime ledger")
	gitMust(t, repo, "config", "filter.runtime.clean", "cat")
	gitMust(t, repo, "update-index", "--assume-unchanged", "--", "ledger/state.json")
	gitMust(t, repo, "update-index", "--skip-worktree", "--", "ledger/state.json")
	writeFile(t, filepath.Join(repo, "ledger", "state.json"), "new runtime state\n")
	_, err = PersistedHead(repo, []string{"ledger/"})
	require.NoError(t, err, "非候选账本不触发源码Git语义限制")
	assert.Contains(t, gitMust(t, repo, "ls-files", "-v", "--", "ledger/state.json"), "s ledger/state.json")

	emptyCandidate := initRepo(t)
	_, err = PersistedHead(emptyCandidate, []string{"README.md"})
	require.NoError(t, err, "空候选无需启动属性检查")
}

func TestGitSemanticsInventoryAndConfigurationErrorsFailClosed(t *testing.T) {
	for _, scenario := range []string{"index-read", "index-malformed", "attribute-read", "attribute-incomplete", "attribute-unknown", "attribute-wrong-path", "attribute-duplicate", "filter-read", "filter-malformed"} {
		t.Run(scenario, func(t *testing.T) {
			repo := initRepo(t)
			oldOut, oldAttributes := gitOut, gitCheckAttributes
			t.Cleanup(func() { gitOut, gitCheckAttributes = oldOut, oldAttributes })
			gitOut = func(cwd string, args ...string) (string, error) {
				if args[0] == "ls-files" && len(args) > 1 && args[1] == "-v" {
					if scenario == "index-read" {
						return "", os.ErrPermission
					}
					if scenario == "index-malformed" {
						return "H\x00", nil
					}
				}
				if args[0] == "config" {
					if scenario == "filter-read" {
						return "", os.ErrPermission
					}
					if scenario == "filter-malformed" {
						return "not-a-config-record\x00", nil
					}
				}
				return oldOut(cwd, args...)
			}
			gitCheckAttributes = func(cwd, paths string) (string, error) {
				switch scenario {
				case "attribute-read":
					return "", os.ErrPermission
				case "attribute-incomplete":
					return "README.md\x00filter\x00unspecified\x00", nil
				case "attribute-unknown":
					return "README.md\x00unknown\x00unspecified\x00README.md\x00filter\x00unspecified\x00", nil
				case "attribute-wrong-path":
					return "not-in-candidate\x00filter\x00unspecified\x00README.md\x00working-tree-encoding\x00unspecified\x00", nil
				case "attribute-duplicate":
					return "README.md\x00filter\x00unspecified\x00README.md\x00filter\x00unspecified\x00", nil
				case "filter-read", "filter-malformed":
					return "README.md\x00filter\x00used\x00README.md\x00working-tree-encoding\x00unspecified\x00", nil
				default:
					return oldAttributes(cwd, paths)
				}
			}
			_, err := PersistedHead(repo, nil)
			require.Error(t, err, "不可读或不完整的Git语义证据不能当支持的普通Git")
		})
	}
}
