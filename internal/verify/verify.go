// Package verify — L1 确定性验收（DESIGN.md §4.4）：编排器亲自跑契约内的
// 验收命令——它们是 ground-truth oracle：便宜、瞬时、无法被执行者辩驳。
// coverage gate 只是 packet 里又一条 L1 命令，此处无任何特判。
//
// 测试/边界完整性由 git diff 事后判定（v0.10）：单元 diff 里出现测试文件
// 或 must-not-touch 路径即判失败。另含 per-unit worktree 隔离与编排器侧
// 单元提交（§4.2 v0.16）。
package verify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
)

// DefaultTestFilePatterns 测试文件识别模式（executor 永不写测试，DC1）。
var DefaultTestFilePatterns = []string{
	`(^|/)(tests?|__tests__|spec)/`,
	`\.(test|spec)\.[cm]?[jt]sx?$`,
}

// EmptyTree 零提交仓库的 diff 基准（git 众所周知空树 sha）。
const EmptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// DiffTextCap 评审用全文 diff 截断上限（~100KB）。
const DiffTextCap = 100_000

// ---- 环境传递 helpers ----

// envLookup 把环境来源归一为 childenv.LookupEnv；m 为 nil 时读进程环境。
func envLookup(m map[string]string) childenv.LookupEnv {
	if m == nil {
		m = map[string]string{}
		for _, kv := range os.Environ() {
			if i := strings.IndexByte(kv, '='); i >= 0 {
				m[kv[:i]] = kv[i+1:]
			}
		}
	}
	return func(key string) (string, bool) { v, ok := m[key]; return v, ok }
}

func envSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// ---- L1 gate 执行 ----

// GateResult 单条验收命令的结果。
type GateResult struct {
	Cmd        string `json:"cmd"`
	Expect     string `json:"expect"`
	ExitCode   int    `json:"exitCode"`
	Pass       bool   `json:"pass"`
	DurationMS int64  `json:"durationMs"`
	Output     string `json:"output,omitempty"`
}

// GateReport 一个 gate 层的完整结果（全部命令都跑，不早停——报告必须完整）。
type GateReport struct {
	Pass    bool         `json:"pass"`
	Results []GateResult `json:"results"`
}

// GateOpts RunGateCommands 选项。Env 为 nil 时用进程环境（经白名单过滤）。
type GateOpts struct {
	Cwd          string
	Env          map[string]string
	TimeoutMS    int64
	EnvAllowlist []string
}

// RunGateCommands 跑一个 gate 层的命令序列。每条命令拿白名单环境（P0-4）：
// 它们执行 executor 写的代码，不得看见编排器的密钥；config verify.env-allowlist
// 是唯一显式出口。
func RunGateCommands(commands []contract.GateEntry, opts GateOpts) GateReport {
	var report GateReport
	for _, entry := range commands {
		expected, perr := contract.ParseExpect(entry.Expect)
		if perr != nil {
			// 加载期已 fail-fast 校验过 expect；运行期再遇坏值按失败处理。
			report.Results = append(report.Results, GateResult{
				Cmd: entry.Command(), Expect: entry.Expect,
				ExitCode: 1, Pass: false, Output: perr.Error(),
			})
			report.Pass = false
			continue
		}
		command := entry.Command()
		started := time.Now()
		exitCode := 0
		output := ""

		timeout := time.Duration(opts.TimeoutMS) * time.Millisecond
		if timeout <= 0 {
			timeout = 300_000 * time.Millisecond // Node 默认 300s
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
		cmd.Dir = opts.Cwd
		cmd.Env = envSlice(childenv.GateEnv(envLookup(opts.Env), opts.EnvAllowlist))
		out, rerr := cmd.CombinedOutput()
		cancel()
		if rerr != nil {
			if ee, ok := rerr.(*exec.ExitError); ok && ee.ExitCode() >= 0 {
				exitCode = ee.ExitCode()
			} else {
				exitCode = 1 // Node: typeof err.code === 'number' ? code : 1
			}
			if len(out) > 2000 {
				out = out[:2000]
			}
			output = string(out)
		}
		report.Results = append(report.Results, GateResult{
			Cmd:        command,
			Expect:     entry.Expect,
			ExitCode:   exitCode,
			Pass:       exitCode == expected,
			DurationMS: time.Since(started).Milliseconds(),
			Output:     output,
		})
	}
	report.Pass = true
	for _, r := range report.Results {
		if !r.Pass {
			report.Pass = false
		}
	}
	return report
}

// ---- git 只读检查 ----

// gitOut / gitRun 可替换注入点（git 故障分支在真实环境不可稳定复现，
// 测试注入覆盖错误传播）。
var gitOut = func(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	cmd.Env = envSlice(childenv.GateEnv(childenv.OSLookupEnv, nil))
	out, err := cmd.Output()
	return string(out), err
}

var gitRun = func(cwd string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	cmd.Env = envSlice(childenv.GateEnv(childenv.OSLookupEnv, nil))
	return cmd.Run()
}

func identityArgs() []string {
	return []string{"-c", "user.name=flywheel-orchestrator", "-c", "user.email=flywheel-orchestrator@localhost"}
}

// ForeignDirtyFiles 任务目录之外的脏工作区路径（repo-relative）。任务目录是
// 编排器领地、预期会变（契约+账本）；运行起点上其他任何脏文件都是既有 WIP，
// 会被 scoped unit commit 卷入（P1-1 caveat——diff 是"自 base ref 以来的一切"，
// 不是"executor 摸过的一切"）。ok=false 表示不是 git checkout。
func ForeignDirtyFiles(cwd, taskDirPrefix string) (paths []string, ok bool) {
	stdout, err := gitOut(cwd, "status", "--porcelain")
	if err != nil {
		return nil, false
	}
	for _, line := range strings.Split(stdout, "\n") {
		if len(line) < 3 {
			continue
		}
		f := strings.TrimSpace(line[3:])
		if f != "" && !strings.HasPrefix(f, taskDirPrefix) {
			paths = append(paths, f)
		}
	}
	return paths, true
}

// ---- per-unit worktree 隔离（§4.2 v0.16）----

// WorktreeNames 单元 worktree 的确定性命名：分支 + checkout 路径。checkout
// 在仓库外的兄弟目录——放进 `<repo>/.git/` 会与 git 自己的 per-worktree 管理
// 目录冲突。
func WorktreeNames(cwd, slug, packetID string) (branch, path string) {
	repoName := filepath.Base(cwd)
	if repoName == "" || repoName == "." || repoName == "/" {
		repoName = "repo"
	}
	branch = fmt.Sprintf("flywheel/%s/%s", slug, packetID)
	path = filepath.Join(filepath.Dir(cwd), ".flywheel-worktrees",
		fmt.Sprintf("%s-%s-%s", repoName, slug, packetID))
	return branch, path
}

// EnsureUnitWorktree 保证单元 worktree 存在于确定性路径，从单元 base ref 创建。
// 已存在（崩溃恢复）则复用——内容保留。非 git checkout 或 worktree 不可用时
// 返回 mode=in-place 与原因。
func EnsureUnitWorktree(cwd, slug, packetID, baseRef string) state.WorktreeInfo {
	branch, path := WorktreeNames(cwd, slug, packetID)
	if _, err := gitOut(path, "rev-parse", "--git-dir"); err == nil {
		return state.WorktreeInfo{Path: path, Branch: branch, Mode: "worktree"} // 已存在：resume 复用
	}
	if err := gitRun(cwd, "worktree", "add", "-f", "-B", branch, path, baseRef); err != nil {
		return state.WorktreeInfo{Mode: "in-place", Path: path, Branch: branch,
			Reason: fmt.Sprintf("git worktree add failed: %v", err)}
	}
	return state.WorktreeInfo{Path: path, Branch: branch, Mode: "worktree"}
}

// MergeUnitWorktree 把单元分支 fast-forward 进主 checkout。绝不 force：
// 主 HEAD 被移动（单元期间用户提交）是上报错误，不是覆盖。
func MergeUnitWorktree(cwd, branch string) (string, error) {
	out, err := gitOut(cwd, "merge", "--ff-only", branch)
	if err != nil {
		return "", fmt.Errorf("%s%s", out, err)
	}
	return strings.TrimSpace(out), nil
}

// RemoveUnitWorktree 删除单元 worktree 与分支（best effort——清理失败绝不能
// 毁掉单元裁决）。
func RemoveUnitWorktree(cwd, branch, path string) {
	_, _ = gitOut(cwd, "worktree", "remove", "--force", path)
	_, _ = gitOut(cwd, "branch", "-D", branch)
}

// GetBaseRef 当前单元的 base ref：git checkout 用 HEAD（零提交仓库用空树）；
// 非 git repo 返回 ok=false。编排器在 executor 运行前记录它，边界判定一律对
// 它 diff——executor 中途自行提交也无法静默抽走 diff（§4.2/§4.4）。
func GetBaseRef(cwd string) (ref string, ok bool) {
	if _, err := gitOut(cwd, "rev-parse", "--git-dir"); err != nil {
		return "", false // 不是 git checkout
	}
	head, err := gitOut(cwd, "rev-parse", "HEAD")
	if err != nil {
		return EmptyTree, true
	}
	return strings.TrimSpace(head), true
}

// GetChangedFiles 当前单元的变更文件：对 baseRef（缺省 HEAD）做 git diff。
// 对单元 base ref 的 diff 同时覆盖 executor 自行提交与未提交改动；untracked
// 文件显式列出（git diff 从不显示它们）。ok=false = 不可用的 git checkout，
// 调用方把该检查记为 skipped。
func GetChangedFiles(cwd, baseRef string) (files []string, ok bool) {
	base := baseRef
	if base == "" {
		base = "HEAD"
	}
	diffOut, err := gitOut(cwd, "diff", "--name-only", base)
	if err != nil {
		return nil, false
	}
	untrackedOut, err := gitOut(cwd, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, false
	}
	set := map[string]bool{}
	for _, line := range strings.Split(diffOut+"\n"+untrackedOut, "\n") {
		f := strings.TrimSpace(line)
		if f != "" {
			set[f] = true
		}
	}
	for f := range set {
		files = append(files, f)
	}
	sort.Strings(files)
	return files, true
}

// GetDiffText 对 baseRef 的完整 unified diff 文本（§4.3 向评审者承诺的是
// git diff，不是文件清单）。~100KB 截断加标记。exclude pathspec（repo-relative）
// 把编排器账本文件从 diff 里剔除；契约文件（spec/packets）保留在内。
// ok=false = 不可用的 git checkout。
func GetDiffText(cwd, baseRef string, exclude []string) (text string, ok bool) {
	base := baseRef
	if base == "" {
		base = "HEAD"
	}
	args := []string{"diff", base, "--", "."}
	for _, p := range exclude {
		args = append(args, ":(exclude)"+p)
	}
	out, err := gitOut(cwd, args...)
	if err != nil {
		return "", false
	}
	if len(out) > DiffTextCap {
		return out[:DiffTextCap] + fmt.Sprintf("\n... [diff truncated at %d chars]", DiffTextCap), true
	}
	return out, true
}

// ---- 编排器侧提交（§4.2：提交权在编排器，永不在 executor）----

// CommitOutcome 提交结果（bookkeeping 失败上报而不抛出——不能毁掉已验证的工作）。
type CommitOutcome struct {
	SHA     string `json:"sha,omitempty"`
	Skipped string `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	At      string `json:"at"`
}

// CommitUnitChanges 单元粒度提交：范围 = 自记录 base ref 以来的单元变更，
// 减去 excludePaths（易变运行时文件）；restrictToPrefix 再收窄到单目录
// （worktree 模式下主 checkout 的账本提交：仅 docs/<slug>/）。永不 git add -A。
func CommitUnitChanges(cwd string, packet *contract.Packet, excludePaths []string, baseRef, restrictToPrefix string) CommitOutcome {
	id := packet.PacketID
	milestone, mErr := contract.MilestoneOfPacket(id)
	goal := strings.Join(strings.Fields(packet.Goal), " ")
	if len(goal) > 50 {
		goal = goal[:50]
	}
	message := strings.TrimSpace(fmt.Sprintf("feat(%s): %s %s", milestone, id, goal))
	out := CommitOutcome{Message: message, At: state.NowISO()}
	if mErr != nil {
		out.Error = mErr.Error()
		return out
	}
	base := baseRef
	if base == "" {
		var ok bool
		base, ok = GetBaseRef(cwd)
		if !ok {
			out.Skipped = "not a git checkout"
			return out
		}
	}
	changed, ok := GetChangedFiles(cwd, base)
	if !ok {
		out.Skipped = "changed-file scan failed"
		return out
	}
	exclude := map[string]bool{}
	for _, p := range excludePaths {
		exclude[p] = true
	}
	var addList []string
	for _, f := range changed {
		if exclude[f] {
			continue
		}
		if restrictToPrefix != "" && !strings.HasPrefix(f, restrictToPrefix) {
			continue
		}
		addList = append(addList, f)
	}
	if len(addList) == 0 {
		out.Skipped = "no changes"
		return out
	}
	addArgs := append(identityArgs(), "add", "--")
	if err := gitRun(cwd, append(addArgs, addList...)...); err != nil {
		out.Error = err.Error()
		return out
	}
	if err := gitRun(cwd, append(identityArgs(), "commit", "-m", message)...); err != nil {
		out.Error = err.Error()
		return out
	}
	sha, _ := gitOut(cwd, "rev-parse", "HEAD")
	out.SHA = strings.TrimSpace(sha)
	return out
}

// CommitTaskDirChanges 以调用方给定的 message 提交任务目录当前变更。用于
// planner 修订之后——契约层必须保持已提交状态（硬规则：worktree 只见已提交
// 契约；防篡改扫描 diff 的基准必须认识合法修订）。onlyFiles（正则，匹配
// repo-relative 路径）把提交收窄到如仅契约文件——planner 修订提交不得卷入
// 脏账本文件，否则移动 HEAD 会破坏 ff-merge。
func CommitTaskDirChanges(cwd, prefix, message string, excludePaths []string, onlyFiles *regexp.Regexp) CommitOutcome {
	out := CommitOutcome{At: state.NowISO()}
	base, ok := GetBaseRef(cwd)
	if !ok {
		out.Skipped = "not a git checkout"
		return out
	}
	changed, ok := GetChangedFiles(cwd, base)
	if !ok {
		out.Skipped = "changed-file scan failed"
		return out
	}
	exclude := map[string]bool{}
	for _, p := range excludePaths {
		exclude[p] = true
	}
	var addList []string
	for _, f := range changed {
		if !strings.HasPrefix(f, prefix) || exclude[f] {
			continue
		}
		if onlyFiles != nil {
			inner := strings.TrimPrefix(f, prefix)
			if !onlyFiles.MatchString(inner) && !onlyFiles.MatchString(f) {
				continue
			}
		}
		addList = append(addList, f)
	}
	if len(addList) == 0 {
		out.Skipped = "no changes"
		return out
	}
	addArgs := append(identityArgs(), "add", "--")
	if err := gitRun(cwd, append(addArgs, addList...)...); err != nil {
		out.Error = err.Error()
		return out
	}
	if err := gitRun(cwd, append(identityArgs(), "commit", "-m", message)...); err != nil {
		out.Error = err.Error()
		return out
	}
	sha, _ := gitOut(cwd, "rev-parse", "HEAD")
	out.SHA = strings.TrimSpace(sha)
	return out
}

// ---- 边界判定（纯函数）----

// BoundaryViolation 一条边界违规。
type BoundaryViolation struct {
	Type  string `json:"type"` // must-not-touch | test-file | max-files
	Path  string `json:"path,omitempty"`
	Count int    `json:"count,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// BoundaryReport 边界检查结论。
type BoundaryReport struct {
	Pass         bool                `json:"pass"`
	Skipped      bool                `json:"skipped"`
	Reason       string              `json:"reason,omitempty"`
	Violations   []BoundaryViolation `json:"violations"`
	ChangedFiles []string            `json:"changedFiles"`
}

// globToRegExp JS 版等价：转义 [.+^${}()|[\]\\]，* → .*（? 保持原义）。
func globToRegExp(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range glob {
		switch r {
		case '.', '+', '^', '$', '{', '}', '(', ')', '|', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '*':
			b.WriteString(".*")
		default:
			b.WriteRune(r)
		}
	}
	return regexp.MustCompile(b.String())
}

// CheckBoundaries 对变更文件列表做纯边界检查。changedFiles 为 nil（diff
// 不可用）时 skipped=true 且放行——skip 记录进证据，不冒充通过理由。
func CheckBoundaries(changedFiles []string, boundaries contract.Boundaries, testFilePatterns []string) BoundaryReport {
	if changedFiles == nil {
		return BoundaryReport{Pass: true, Skipped: true,
			Reason:       "git diff unavailable (not a git checkout)",
			Violations:   []BoundaryViolation{},
			ChangedFiles: []string{}}
	}
	type matcher struct {
		re     *regexp.Regexp
		exact  string
		prefix string
		isRe   bool
	}
	var matchers []matcher
	for _, entry := range boundaries.MustNotTouch {
		if strings.Contains(entry, "*") {
			matchers = append(matchers, matcher{re: globToRegExp(entry), isRe: true})
		} else {
			prefix := entry
			if !strings.HasSuffix(entry, "/") {
				prefix = entry + "/"
			}
			matchers = append(matchers, matcher{exact: entry, prefix: prefix})
		}
	}
	if testFilePatterns == nil {
		testFilePatterns = DefaultTestFilePatterns
	}
	testRes := make([]*regexp.Regexp, 0, len(testFilePatterns))
	for _, p := range testFilePatterns {
		testRes = append(testRes, regexp.MustCompile(p))
	}

	violations := []BoundaryViolation{}
	for _, file := range changedFiles {
		for _, m := range matchers {
			hit := m.isRe && m.re.MatchString(file) ||
				!m.isRe && (file == m.exact || strings.HasPrefix(file, m.prefix))
			if hit {
				violations = append(violations, BoundaryViolation{Type: "must-not-touch", Path: file})
			}
		}
		for _, re := range testRes {
			if re.MatchString(file) {
				violations = append(violations, BoundaryViolation{Type: "test-file", Path: file})
				break // Node some() 语义：每文件至多一条 test-file 违规
			}
		}
	}
	if boundaries.MaxFiles != nil && len(changedFiles) > *boundaries.MaxFiles {
		violations = append(violations, BoundaryViolation{
			Type: "max-files", Count: len(changedFiles), Limit: *boundaries.MaxFiles})
	}
	return BoundaryReport{Pass: len(violations) == 0, Violations: violations, ChangedFiles: changedFiles}
}
