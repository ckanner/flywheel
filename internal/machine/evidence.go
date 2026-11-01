package machine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/ledger"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
)

// 验证证据绑定完整候选文件内容、契约及验证配置。不能用 diff 摘要、mtime、
// 自报 changed-files 或单独的 commit SHA：它们都漏掉未提交/未跟踪变更。
// Git 仓库仅纳入 tracked + non-ignored untracked；运行账本不影响候选身份。
var snapshotGitFiles = func(cwd string) ([]byte, error) {
	cmd := exec.Command("git", "-C", cwd, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Env = childenv.EnvSlice(childenv.GateEnv(childenv.OSLookupEnv, nil))
	return cmd.Output()
}

var snapshotOpen = func(path string) (io.ReadCloser, error) { return os.Open(path) }
var snapshotLstat = os.Lstat
var snapshotReadlink = os.Readlink

func (c *Context) snapshotExcluded(rel string) bool {
	prefix := filepath.ToSlash(c.taskDirPrefix())
	return strings.HasPrefix(rel, prefix) && ledger.IsLedgerTaskPath(strings.TrimPrefix(rel, prefix))
}

func (c *Context) candidateFingerprint(cwd string, extraExclude ...func(string) bool) (string, error) {
	var paths []string
	if _, isGit := verify.GetBaseRef(cwd); isGit {
		raw, err := snapshotGitFiles(cwd)
		if err != nil {
			return "", fmt.Errorf("candidate file inventory: %w", err)
		}
		paths = strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
	} else {
		if _, err := snapshotLstat(filepath.Join(cwd, ".git")); err == nil || !os.IsNotExist(err) {
			return "", fmt.Errorf("candidate Git metadata exists or is unreadable, but repository could not be inspected")
		}
		err := filepath.WalkDir(cwd, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(cwd, path)
			paths = append(paths, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	sort.Strings(paths)
	h := sha256.New()
	previous := ""
	for _, rel := range paths {
		if rel == "" || rel == previous || c.snapshotExcluded(rel) {
			continue
		}
		previous = rel
		if len(extraExclude) > 0 && extraExclude[0](rel) {
			continue
		}
		full := filepath.Join(cwd, filepath.FromSlash(rel))
		info, err := snapshotLstat(full)
		if os.IsNotExist(err) {
			// 缺失路径不属于候选tree；提交删除前后必须同一身份。删除仍会
			// 改变原先含此文件内容的指纹，不能把index墓碑当文件内容。
			continue
		}
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%q %o ", rel, info.Mode())
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := snapshotReadlink(full)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(h, "%q\n", target)
		case info.Mode().IsRegular():
			f, err := snapshotOpen(full)
			if err != nil {
				return "", err
			}
			fh := sha256.New()
			_, err = io.Copy(fh, f)
			_ = f.Close()
			if err != nil {
				return "", err
			}
			fmt.Fprintf(h, "%x\n", fh.Sum(nil))
		default:
			return "", fmt.Errorf("candidate path %q is not a regular file or symlink; submodules/special files require explicit verification", rel)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (c *Context) verificationIdentity(packet *contract.Packet) (string, error) {
	candidate, err := c.candidateFingerprint(c.unitCwd(packet))
	if err != nil {
		return "", err
	}
	// 同时绑定主任务目录的权威契约；worktree 内旧副本不是规划修订的信源。
	contractFiles := []string{"spec.md", "task-packet-" + packet.PacketID + ".md"}
	contractContent := make([]string, 0, len(contractFiles))
	for _, name := range contractFiles {
		raw, err := os.ReadFile(filepath.Join(c.TaskDir, name))
		if err != nil {
			return "", err
		}
		contractContent = append(contractContent, string(raw))
	}
	doneReport := ""
	report, err := os.ReadFile(filepath.Join(c.TaskDir, "reports", packet.PacketID+".done.json"))
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err == nil {
		h := sha256.Sum256(report)
		doneReport = hex.EncodeToString(h[:])
	}
	raw, _ := json.Marshal(struct {
		Version       int
		Candidate     string
		Packet        *contract.Packet
		Spec          *contract.Spec
		ContractFiles []string
		DoneReport    string
		Checks        any
	}{1, candidate, packet, c.Spec, contractContent, doneReport, c.Config.Verify})
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:]), nil
}

func evidenceIdentity(ev any) string {
	if m, ok := ev.(map[string]any); ok {
		id, _ := m["identity"].(string)
		return id
	}
	return ""
}

func (c *Context) currentL1Evidence(packet *contract.Packet) bool {
	ps := c.State.Packets[packet.PacketID]
	stored := evidenceIdentity(ps.Evidence.L1)
	if stored == "" || !l1PassTrue(ps) {
		return false // 旧版本证据无身份，保守地重验。
	}
	current, err := c.verificationIdentity(packet)
	return err == nil && stored == current
}

func invalidateEvidence(ps *state.PacketState) {
	if ps.Evidence.Executor != nil || ps.Evidence.L1 != nil || ps.Evidence.Review != nil || ps.Evidence.DefectInjection != nil {
		ps.EvidenceHistory = append(ps.EvidenceHistory, map[string]any{
			"at":       state.NowISO(),
			"evidence": ps.Evidence,
		})
	}
	ps.Evidence.Executor = nil
	ps.Evidence.L1 = nil
	ps.Evidence.Review = nil
	ps.Evidence.DefectInjection = nil
}
