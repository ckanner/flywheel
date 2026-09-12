package verify

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/contract"
)

func gateEntry(cmd, expect string) contract.GateEntry {
	return contract.GateEntry{Cmd: cmd, Expect: expect}
}

func scriptEntry(script, expect string) contract.GateEntry {
	return contract.GateEntry{Script: script, Expect: expect}
}

// initRepo 建一个带初始提交的真实 git 仓库。
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	writeFile(t, filepath.Join(dir, "README.md"), "# repo\n")
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func gitMust(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", cwd}, args...)...).CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return string(out)
}

func TestRunGateCommandsAllPassAndScriptFallback(t *testing.T) {
	r := RunGateCommands([]contract.GateEntry{
		// sleep 而非 echo：DurationMS 毫秒截断，快速 runner 上亚毫秒命令
		// 会取整为 0（Linux CI 实证）——计时断言需要确定性非零时长。
		gateEntry("sleep 0.05", "exit 0"),
		scriptEntry("e2e.sh", "exit 0"), // 缺失脚本 → sh 失败
	}, GateOpts{Cwd: t.TempDir(), TimeoutMS: 5000})
	assert.False(t, r.Pass)
	require.Len(t, r.Results, 2)
	assert.Equal(t, 0, r.Results[0].ExitCode)
	assert.True(t, r.Results[0].Pass)
	assert.Empty(t, r.Results[0].Output)
	assert.NotZero(t, r.Results[0].DurationMS)
	assert.Equal(t, "e2e.sh", r.Results[1].Cmd)
	assert.False(t, r.Results[1].Pass)
}

func TestRunGateCommandsExpectNonZero(t *testing.T) {
	r := RunGateCommands([]contract.GateEntry{gateEntry("sh -c 'exit 3'", "exit 3")}, GateOpts{Cwd: t.TempDir()})
	assert.True(t, r.Pass)
	assert.Equal(t, 3, r.Results[0].ExitCode)
}

func TestRunGateCommandsFailureCapturesOutput(t *testing.T) {
	r := RunGateCommands([]contract.GateEntry{gateEntry("echo out-line; echo err-line >&2; exit 2", "exit 0")}, GateOpts{Cwd: t.TempDir()})
	assert.False(t, r.Pass)
	assert.Equal(t, 2, r.Results[0].ExitCode)
	assert.Contains(t, r.Results[0].Output, "out-line")
	assert.Contains(t, r.Results[0].Output, "err-line")
}

func TestRunGateCommandsOutputTruncatedAt2000(t *testing.T) {
	r := RunGateCommands([]contract.GateEntry{gateEntry("yes x | head -c 5000; exit 1", "exit 0")}, GateOpts{Cwd: t.TempDir()})
	require.Len(t, r.Results, 1)
	assert.LessOrEqual(t, len(r.Results[0].Output), 2000)
}

func TestRunGateCommandsTimeout(t *testing.T) {
	r := RunGateCommands([]contract.GateEntry{gateEntry("sleep 5", "exit 0")},
		GateOpts{Cwd: t.TempDir(), TimeoutMS: 100})
	require.Len(t, r.Results, 1)
	assert.False(t, r.Results[0].Pass)
	assert.Equal(t, 1, r.Results[0].ExitCode, "Node 语义：timeout 的 err.code 非数字 → 1")
}

func TestRunGateCommandsEnvWhitelist(t *testing.T) {
	t.Setenv("FLYWHEEL_TEST_SECRET", "super-secret")
	t.Setenv("FLYWHEEL_TEST_ALLOWED", "visible")
	// 末尾强制非零退出，走失败路径让 output 被记录（Node 成功时 output=undefined 同款）。
	r := RunGateCommands(
		[]contract.GateEntry{gateEntry(`printf '[%s][%s]' "$FLYWHEEL_TEST_SECRET" "$FLYWHEEL_TEST_ALLOWED"; exit 9`, "exit 0")},
		GateOpts{Cwd: t.TempDir(), EnvAllowlist: []string{"FLYWHEEL_TEST_ALLOWED"}})
	out := r.Results[0].Output
	assert.Contains(t, out, "[]", "编排器密钥不得泄入闸门命令")
	assert.Contains(t, out, "[visible]")
}

func TestRunGateCommandsBadExpectFailsPerEntry(t *testing.T) {
	r := RunGateCommands([]contract.GateEntry{
		gateEntry("true", "exit zero"), // 非法 expect：运行期按失败处理
		gateEntry("true", "exit 0"),
	}, GateOpts{Cwd: t.TempDir()})
	assert.False(t, r.Pass)
	require.Len(t, r.Results, 2)
	assert.False(t, r.Results[0].Pass)
	assert.Contains(t, r.Results[0].Output, "unsupported expect")
	assert.True(t, r.Results[1].Pass)
}

func TestForeignDirtyFiles(t *testing.T) {
	repo := initRepo(t)
	taskPrefix := "docs/mytask/"
	// porcelain 对全新 untracked 目录按目录折叠（"?? src/"）。让任务目录与
	// src/ 先被跟踪，脏文件才以文件级粒度出现——与 Node 生产场景一致。
	writeFile(t, filepath.Join(repo, taskPrefix, "state.json"), "{}")
	writeFile(t, filepath.Join(repo, "src", "existing.go"), "package src")
	gitMust(t, repo, "add", ".")
	gitMust(t, repo, "commit", "-m", "seed task dir")

	writeFile(t, filepath.Join(repo, taskPrefix, "state.json"), `{"dirty":true}`) // 任务目录内修改
	writeFile(t, filepath.Join(repo, "src", "wip.go"), "package wip")             // 外来 WIP（已跟踪目录下新文件）
	writeFile(t, filepath.Join(repo, "untracked.txt"), "x")                       // 根下新文件

	files, ok := ForeignDirtyFiles(repo, taskPrefix)
	require.True(t, ok)
	assert.Contains(t, files, "src/wip.go")
	assert.Contains(t, files, "untracked.txt")
	for _, f := range files {
		assert.False(t, strings.HasPrefix(f, taskPrefix), "任务目录内路径必须被排除: %s", f)
	}

	_, ok = ForeignDirtyFiles(filepath.Join(repo, "no-such-subdir"), "")
	assert.False(t, ok, "非 git checkout → ok=false")
}

func TestWorktreeNames(t *testing.T) {
	branch, path := WorktreeNames("/repo", "mytask", "M01-T01")
	assert.Equal(t, "flywheel/mytask/M01-T01", branch)
	assert.Equal(t, "/.flywheel-worktrees/repo-mytask-M01-T01", path)
}

// TestForeignDirtyFilesRenameAndQuoted（C10 回归）：porcelain rename 拆新旧
// 两路径；引号路径剥引号；任务目录前缀过滤仍生效。
func TestForeignDirtyFilesRenameAndQuoted(t *testing.T) {
	orig := gitOut
	t.Cleanup(func() { gitOut = orig })
	gitOut = func(string, ...string) (string, error) {
		return strings.Join([]string{
			"R  src/old.go -> src/new.go", // rename（账本区外）→ 新旧都算既有 WIP
			` M "we ird\"name.go"`,
			"?? docs/t/reports/x.json", // 账本区 → 过滤
			"M  plain.go",
			"",   // 尾空行
			"??", // 短行忽略
		}, "\n"), nil
	}
	paths, ok := ForeignDirtyFiles("/repo", "docs/t/")
	assert.True(t, ok)
	assert.Equal(t, []string{
		"src/old.go",
		"src/new.go",
		`we ird"name.go`,
		"plain.go",
	}, paths)
}

func TestEnsureAndMergeAndRemoveWorktree(t *testing.T) {
	repo := initRepo(t)
	base, ok := GetBaseRef(repo)
	require.True(t, ok)

	wi := EnsureUnitWorktree(repo, "slug", "M01-T01", base)
	assert.Equal(t, "worktree", wi.Mode)
	assert.DirExists(t, wi.Path)

	wi2 := EnsureUnitWorktree(repo, "slug", "M01-T01", base)
	assert.Equal(t, wi.Path, wi2.Path)
	assert.Equal(t, "worktree", wi2.Mode)

	// worktree 内提交 → ff merge 回主 checkout。
	writeFile(t, filepath.Join(wi.Path, "feature.txt"), "code\n")
	gitMust(t, wi.Path, "add", ".")
	gitMust(t, wi.Path, "commit", "-m", "unit work")
	merged, merr := MergeUnitWorktree(repo, wi.Branch)
	require.NoError(t, merr)
	assert.NotEmpty(t, merged)

	// 主 HEAD 被移动后 ff-only 失败（绝不 force）。
	writeFile(t, filepath.Join(repo, "main-move.txt"), "user commit\n")
	gitMust(t, repo, "add", ".")
	gitMust(t, repo, "commit", "-m", "user moves HEAD")
	writeFile(t, filepath.Join(wi.Path, "more.txt"), "more\n")
	gitMust(t, wi.Path, "add", ".")
	gitMust(t, wi.Path, "commit", "-m", "second unit work")
	_, merr = MergeUnitWorktree(repo, wi.Branch)
	assert.Error(t, merr)

	RemoveUnitWorktree(repo, wi.Branch, wi.Path)
	_, err := gitOut(wi.Path, "rev-parse", "--git-dir")
	assert.Error(t, err, "worktree 应已删除")
}

func TestEnsureUnitWorktreeInPlaceFallback(t *testing.T) {
	notGit := t.TempDir()
	wi := EnsureUnitWorktree(notGit, "s", "M01-T01", EmptyTree)
	assert.Equal(t, "in-place", wi.Mode)
	assert.NotEmpty(t, wi.Reason)
	assert.Contains(t, wi.Reason, "git worktree add failed")
}

func TestGetBaseRef(t *testing.T) {
	repo := initRepo(t)
	ref, ok := GetBaseRef(repo)
	require.True(t, ok)
	assert.Len(t, ref, 40)

	empty := t.TempDir()
	require.NoError(t, exec.Command("git", "init", empty).Run())
	ref, ok = GetBaseRef(empty)
	require.True(t, ok)
	assert.Equal(t, EmptyTree, ref)

	_, ok = GetBaseRef(t.TempDir())
	assert.False(t, ok)
}

func TestGetChangedFiles(t *testing.T) {
	repo := initRepo(t)
	base, _ := GetBaseRef(repo)
	writeFile(t, filepath.Join(repo, "src/b.go"), "b")
	writeFile(t, filepath.Join(repo, "src/a.go"), "a")
	writeFile(t, filepath.Join(repo, "untracked.tmp"), "u")
	files, ok := GetChangedFiles(repo, base)
	require.True(t, ok)
	assert.Equal(t, []string{"src/a.go", "src/b.go", "untracked.tmp"}, files)

	_, ok = GetChangedFiles(filepath.Join(repo, "sub"), base)
	assert.False(t, ok)
}

func TestGetDiffTextExcludeAndTruncate(t *testing.T) {
	repo := initRepo(t)
	base, _ := GetBaseRef(repo)
	writeFile(t, filepath.Join(repo, "docs/t/ledger.jsonl"), `{"ts":1}`)
	writeFile(t, filepath.Join(repo, "docs/t/spec.md"), "# spec change")
	writeFile(t, filepath.Join(repo, "code.go"), "package main")
	// untracked 不进 git diff——先 stage（Node 生产路径：diff 对 base ref 含 staged）。
	gitMust(t, repo, "add", "-N", ".")

	text, ok := GetDiffText(repo, base, []string{"docs/t/ledger.jsonl"})
	require.True(t, ok)
	assert.Contains(t, text, "code.go")
	assert.Contains(t, text, "spec.md")
	assert.NotContains(t, text, "ledger.jsonl")

	huge := strings.Repeat("+x\n", DiffTextCap) // ~900KB diff 体
	writeFile(t, filepath.Join(repo, "huge.txt"), huge)
	gitMust(t, repo, "add", "huge.txt")
	text, ok = GetDiffText(repo, base, nil)
	require.True(t, ok)
	assert.Less(t, len(text), DiffTextCap+200)
	assert.Contains(t, text, fmt.Sprintf("\n... [diff truncated at %d chars]", DiffTextCap))
}

// TestGetDiffTextIncludesUntrackedContent（C2 回归）：git diff 不含 untracked，
// 评审者必须仍能看到 executor 新建文件的全文（greenfield 单元主内容）；
// exclude 同样作用于 untracked 段；二进制文件以占位标记出现。
func TestGetDiffTextIncludesUntrackedContent(t *testing.T) {
	repo := initRepo(t)
	base, _ := GetBaseRef(repo)
	writeFile(t, filepath.Join(repo, "new_src.go"), "package main\n\nfunc main() {}\n")
	writeFile(t, filepath.Join(repo, "docs/t/private.json"), `{"secret":1}`)
	writeFile(t, filepath.Join(repo, "blob.bin"), "ab\x00cd")

	text, ok := GetDiffText(repo, base, []string{"docs/t/private.json"})
	require.True(t, ok)
	assert.Contains(t, text, "diff --git a/new_src.go b/new_src.go")
	assert.Contains(t, text, "@@ -0,0 +1,3 @@")
	assert.Contains(t, text, "+func main() {}")
	assert.Contains(t, text, "[binary file]")
	assert.NotContains(t, text, "private.json")
}

func TestAppendUntrackedSectionsArms(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("x", 9000) // head 截断臂（>8000）
	noNL := "no trailing newline"    // 无尾换行臂
	writeFile(t, filepath.Join(dir, "big.txt"), big)
	writeFile(t, filepath.Join(dir, "empty.txt"), "")
	writeFile(t, filepath.Join(dir, "nonl.txt"), noNL)

	var b strings.Builder
	appendUntrackedSections(&b, dir, []string{"gone.txt", "big.txt", "empty.txt", "nonl.txt"})
	out := b.String()

	assert.Contains(t, out, "@@ -0,0 +0,0 @@")      // 空文件臂
	assert.Contains(t, out, "+no trailing newline") // 无尾换行正文
	assert.Contains(t, out, "\\ No newline at end of file")
	assert.Contains(t, out, "+"+big+"\n\\ No newline at end of file",
		">8000 字节文本全文输出；head 截断只作用于二进制探测窗口")

	// 读取失败臂：listing 含不存在文件 → 跳过不 panic。
	var b2 strings.Builder
	appendUntrackedSections(&b2, dir, []string{"missing.bin"})
	assert.Empty(t, b2.String())
}

// TestAppendUntrackedSectionsNonRegular（PR#3 复审）：非符号链接的非 regular
// 条目（目录被手动传入）→ [non-regular file] 占位。
func TestAppendUntrackedSectionsNonRegular(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))
	var b strings.Builder
	appendUntrackedSections(&b, dir, []string{"", "subdir"})
	assert.Contains(t, b.String(), "[non-regular file]")
}

// TestAppendUntrackedSectionsUnreadable（PR#3 复审）：Lstat 成功但 ReadFile
// 失败（chmod 000）→ 竞态臂 continue。
func TestAppendUntrackedSectionsUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "secret.txt")
	writeFile(t, p, "s")
	require.NoError(t, os.Chmod(p, 0o000))
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

	var b strings.Builder
	appendUntrackedSections(&b, dir, []string{"", "secret.txt"})
	out := b.String()
	assert.Contains(t, out, "diff --git a/secret.txt", "头部段照常渲染")
	assert.NotContains(t, out, "+s", "内容读取失败臂 → 无正文行泄漏")
}

func TestAppendUntrackedSectionsSymlinkAndLarge(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Symlink("/dev/zero", filepath.Join(dir, "danger.link")))
	hugePath := filepath.Join(dir, "huge.bin")
	f, err := os.Create(hugePath)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(int64(perFileContentCap)+1))
	require.NoError(t, f.Close())

	var b strings.Builder
	appendUntrackedSections(&b, dir, []string{"danger.link", "huge.bin"})
	out := b.String()
	assert.Contains(t, out, "[symlink -> /dev/zero]", "链接只渲染 target，绝不读内容")
	assert.Contains(t, out, "[file too large:")
	assert.NotContains(t, out, strings.Repeat("x", 10))
}

func TestGetDiffTextCJKFilenames(t *testing.T) {
	repo := initRepo(t)
	base, _ := GetBaseRef(repo)
	writeFile(t, filepath.Join(repo, "中文 说明.md"), "# 中文内容\n")

	text, ok := GetDiffText(repo, base, nil)
	require.True(t, ok)
	assert.Contains(t, text, "中文 说明.md", "-z 解析下非 ASCII 文件名不得丢失")
	assert.Contains(t, text, "# 中文内容")
}

func TestCommitUnitChanges(t *testing.T) {
	repo := initRepo(t)
	pkt := &contract.Packet{PacketID: "M02-T03", Goal: "Implement the   parser\nlibrary "}
	base, _ := GetBaseRef(repo)

	out := CommitUnitChanges(repo, pkt, nil, base, "")
	assert.Equal(t, "no changes", out.Skipped)
	assert.Equal(t, "feat(M-02): M02-T03 Implement the parser library", out.Message)

	writeFile(t, filepath.Join(repo, "feat.go"), "package feat")
	out = CommitUnitChanges(repo, pkt, nil, base, "")
	require.Empty(t, out.Error)
	require.NotEmpty(t, out.SHA)
	head, _ := gitOut(repo, "rev-parse", "HEAD")
	assert.Equal(t, strings.TrimSpace(head), out.SHA)

	// excludePaths 排除易变文件。
	writeFile(t, filepath.Join(repo, "volatile.log"), "noise")
	writeFile(t, filepath.Join(repo, "keep.txt"), "keep")
	out = CommitUnitChanges(repo, pkt, []string{"volatile.log"}, "", "")
	require.Empty(t, out.Error)
	logged, rerr := os.ReadFile(filepath.Join(repo, "volatile.log"))
	require.NoError(t, rerr)
	assert.Equal(t, "noise", string(logged))

	// restrictToPrefix 收窄到目录。
	writeFile(t, filepath.Join(repo, "outside.txt"), "o")
	writeFile(t, filepath.Join(repo, "docs/slug/in.txt"), "i")
	out = CommitUnitChanges(repo, pkt, nil, "", "docs/slug/")
	require.Empty(t, out.Error)
	stagedOut, serr := exec.Command("git", "-C", repo, "diff", "--name-only", "--cached").Output()
	require.NoError(t, serr)
	for _, line := range strings.Split(string(stagedOut), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		assert.True(t, strings.HasPrefix(line, "docs/slug/"), "收窄外的文件不得入暂存: %q", line)
	}
}

func TestCommitTaskDirChangesOnlyFiles(t *testing.T) {
	repo := initRepo(t)
	prefix := "docs/t/"
	writeFile(t, filepath.Join(repo, prefix, "spec.md"), "revised spec")
	writeFile(t, filepath.Join(repo, prefix, "metrics.jsonl"), "volatile ledger")
	writeFile(t, filepath.Join(repo, "other/root.txt"), "not task dir")

	onlyContract := regexp.MustCompile(`^spec\.md$`)
	out := CommitTaskDirChanges(repo, prefix, "docs(t): plan revision", nil, onlyContract)
	require.Empty(t, out.Error)
	status := gitMust(t, repo, "status", "--porcelain")
	assert.NotContains(t, status, "spec.md", "契约文件已被提交")
	// porcelain 对全新 untracked 目录按目录折叠。
	assert.Contains(t, status, "other/")
	assert.Contains(t, status, "metrics.jsonl")
}

func TestCommitTaskDirChangesSkips(t *testing.T) {
	notGit := t.TempDir()
	out := CommitTaskDirChanges(notGit, "docs/x/", "m", nil, nil)
	assert.Equal(t, "not a git checkout", out.Skipped)

	repo := initRepo(t)
	out = CommitTaskDirChanges(repo, "docs/y/", "m", nil, regexp.MustCompile(`^a$`))
	assert.Equal(t, "no changes", out.Skipped)
}

func TestCheckBoundaries(t *testing.T) {
	r := CheckBoundaries(nil, contract.Boundaries{}, nil)
	assert.True(t, r.Pass)
	assert.True(t, r.Skipped)
	assert.Equal(t, "git diff unavailable (not a git checkout)", r.Reason)

	mf := 2
	bounds := contract.Boundaries{MustNotTouch: []string{"src/payments/", "*.lock", "vendor"}, MaxFiles: &mf}

	r = CheckBoundaries([]string{"src/payments/core.go", "pkg/x.lock", "vendor/lib/h.h"}, bounds, nil)
	assert.False(t, r.Pass)
	// 3 个文件全中 must-not-touch，且 3 > max-files=2 追加一条。
	require.Len(t, r.Violations, 4)
	mnt := 0
	for _, v := range r.Violations {
		if v.Type == "must-not-touch" {
			mnt++
		}
	}
	assert.Equal(t, 3, mnt)

	r = CheckBoundaries([]string{"tests/auth_test.go", "a.go", "b.go", "c.ts.test.js"}, bounds, nil)
	assert.False(t, r.Pass)
	testHits := 0
	var maxV BoundaryViolation
	for _, v := range r.Violations {
		if v.Type == "test-file" {
			testHits++
		}
		if v.Type == "max-files" {
			maxV = v
		}
	}
	assert.Equal(t, 2, testHits, "auth_test.go 与 c.ts.test.js 各计一条")
	assert.Equal(t, 4, maxV.Count)
	assert.Equal(t, 2, maxV.Limit)

	r = CheckBoundaries([]string{"src/ok.go", "lib/main.py"}, bounds, nil)
	assert.True(t, r.Pass)
	assert.Empty(t, r.Violations)
	assert.Equal(t, []string{"src/ok.go", "lib/main.py"}, r.ChangedFiles)
}

func TestAheadCount(t *testing.T) {
	repo := initRepo(t)
	base, ok := GetBaseRef(repo)
	require.True(t, ok)
	writeFile(t, filepath.Join(repo, "x.txt"), "x")
	gitMust(t, repo, "add", "-A")
	gitMust(t, repo, "commit", "-q", "-m", "second")

	n, aerr := AheadCount(repo, base, "HEAD")
	require.NoError(t, aerr)
	assert.Equal(t, 1, n)

	_, aerr = AheadCount(filepath.Join(repo, "gone"), base, "HEAD")
	assert.Error(t, aerr)
}

func TestWorktreeNamesFallbackRepoName(t *testing.T) {
	for _, cwd := range []string{"", ".", "/"} {
		branch, path := WorktreeNames(cwd, "s", "M01-T01")
		assert.Equal(t, "flywheel/s/M01-T01", branch)
		assert.Contains(t, path, "repo-s-M01-T01")
	}
}

func TestGetChangedFilesAndDiffTextDefaultHEAD(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, filepath.Join(repo, "mod.txt"), "m")
	gitMust(t, repo, "add", "-N", ".")

	files, ok := GetChangedFiles(repo, "") // baseRef 空 → HEAD
	require.True(t, ok)
	assert.Equal(t, []string{"mod.txt"}, files)

	text, ok := GetDiffText(repo, "", nil) // 同款默认分支
	require.True(t, ok)
	assert.Contains(t, text, "mod.txt")
}

func TestCommitUnitChangesGoalTruncation(t *testing.T) {
	pkt := &contract.Packet{PacketID: "M01-T01", Goal: strings.Repeat("g", 80)}
	out := CommitUnitChanges(initRepo(t), pkt, nil, EmptyTree, "")
	assert.Len(t, out.Message, len("feat(M-01): M01-T01 ")+50)
	assert.True(t, strings.HasSuffix(out.Message, strings.Repeat("g", 50)))
}

func TestCommitUnitChangesBadPacketID(t *testing.T) {
	out := CommitUnitChanges(initRepo(t), &contract.Packet{PacketID: "bad"}, nil, "", "")
	assert.NotEmpty(t, out.Error)
	assert.Contains(t, out.Error, "invalid packet id")
}

func TestCommitUnitChangesNotGitCheckout(t *testing.T) {
	out := CommitUnitChanges(t.TempDir(), &contract.Packet{PacketID: "M01-T01", Goal: "g"}, nil, "", "")
	assert.Equal(t, "not a git checkout", out.Skipped)
}

func TestCommitUnitChangesGitFailuresInjected(t *testing.T) {
	origOut, origRun := gitOut, gitRun
	t.Cleanup(func() { gitOut, gitRun = origOut, origRun })

	repo := initRepo(t)
	pkt := &contract.Packet{PacketID: "M01-T01", Goal: "g"}
	base, _ := GetBaseRef(repo)
	writeFile(t, filepath.Join(repo, "x.go"), "x")

	// changed-file 扫描失败。
	gitOut = func(cwd string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "diff" {
			return "", fmt.Errorf("git exploded")
		}
		return origOut(cwd, args...)
	}
	out := CommitUnitChanges(repo, pkt, nil, base, "")
	assert.Equal(t, "changed-file scan failed", out.Skipped)
	gitOut = origOut

	gitRun = func(cwd string, args ...string) error {
		if slices.Contains(args, "add") {
			return fmt.Errorf("add boom")
		}
		return origRun(cwd, args...)
	}
	out = CommitUnitChanges(repo, pkt, nil, base, "")
	assert.Contains(t, out.Error, "add boom")

	// commit 失败。
	gitRun = func(cwd string, args ...string) error {
		if slices.Contains(args, "commit") {
			return fmt.Errorf("commit boom")
		}
		return origRun(cwd, args...)
	}
	out = CommitUnitChanges(repo, pkt, nil, base, "")
	assert.Contains(t, out.Error, "commit boom")
	gitRun = origRun
}

func TestCommitTaskDirChangesExcludesAndFailuresInjected(t *testing.T) {
	repo := initRepo(t)
	prefix := "docs/t/"
	writeFile(t, filepath.Join(repo, prefix, "a.md"), "a")
	writeFile(t, filepath.Join(repo, prefix, "skipme.md"), "s")

	origOut, origRun := gitOut, gitRun
	t.Cleanup(func() { gitOut, gitRun = origOut, origRun })

	// excludePaths 生效：skipme 被排除，onlyFiles 放行 a.md。
	onlyA := regexp.MustCompile(`^a\.md$|^skipme\.md$`)
	out := CommitTaskDirChanges(repo, prefix, "docs(t): x", []string{"docs/t/skipme.md"}, onlyA)
	require.Empty(t, out.Error)
	require.NotEmpty(t, out.SHA)
	status := gitMust(t, repo, "status", "--porcelain")
	assert.Contains(t, status, "skipme.md")

	// scan 失败注入。
	gitOut = func(cwd string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "ls-files" {
			return "", fmt.Errorf("ls-files boom")
		}
		return origOut(cwd, args...)
	}
	writeFile(t, filepath.Join(repo, prefix, "b.md"), "b")
	out = CommitTaskDirChanges(repo, prefix, "m", nil, nil)
	assert.Equal(t, "changed-file scan failed", out.Skipped)
	gitOut = origOut

	// add 失败（identity -c 前缀：子命令不在 args[0]，按子命令查找）。
	gitRun = func(cwd string, args ...string) error {
		if slices.Contains(args, "add") {
			return fmt.Errorf("taskdir add boom")
		}
		return origRun(cwd, args...)
	}
	out = CommitTaskDirChanges(repo, prefix, "m", nil, nil)
	assert.Contains(t, out.Error, "taskdir add boom")
	gitRun = func(cwd string, args ...string) error {
		if slices.Contains(args, "commit") {
			return fmt.Errorf("taskdir commit boom")
		}
		return origRun(cwd, args...)
	}
	out = CommitTaskDirChanges(repo, prefix, "m", nil, nil)
	assert.Contains(t, out.Error, "taskdir commit boom")
}

func TestGetChangedFilesScanFailureInjected(t *testing.T) {
	orig := gitOut
	t.Cleanup(func() { gitOut = orig })
	gitOut = func(cwd string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "ls-files" {
			return "", fmt.Errorf("boom")
		}
		return orig(cwd, args...)
	}
	repo := initRepo(t)
	_, ok := GetChangedFiles(repo, "")
	assert.False(t, ok)
}

func TestGetDiffTextFailureInjected(t *testing.T) {
	orig := gitOut
	t.Cleanup(func() { gitOut = orig })
	gitOut = func(cwd string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "diff" {
			return "", fmt.Errorf("diff boom")
		}
		return orig(cwd, args...)
	}
	_, ok := GetDiffText(initRepo(t), "", nil)
	assert.False(t, ok)
}
