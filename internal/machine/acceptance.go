// A1（DESIGN §4.1 v0.18 已知风险缓解）：planner 在修订时有权修改 acceptance
// 命令本身——"减弱验收让单元通过"是真实的自利路径。铁律 1 要求监督可机械
// 检查：这里不阻止（范围裁决权归 Plan），但在每次 plan 落库后对全部 packet 的
// acceptance 指纹做 diff，任何变化写 retro 行 + state 预算警告，人经
// status/retro 可见。
package machine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/verify"
)

// acceptanceFingerprint 单个 packet 三层验收的稳定指纹（命令+expect 的规范
// JSON 哈希）。planner 改弱/改强/换命令都会改变指纹。
func acceptanceFingerprint(p *contract.Packet) string {
	type entry struct {
		Cmd    string `json:"cmd"`
		Script string `json:"script"`
		Expect string `json:"expect"`
	}
	snap := struct {
		L1 []entry `json:"L1"`
		L2 []entry `json:"L2"`
		L3 []entry `json:"L3"`
	}{}
	for _, level := range []string{"L1", "L2", "L3"} {
		var es []entry
		for _, g := range p.Acceptance.Level(level) {
			es = append(es, entry{Cmd: g.Cmd, Script: g.Script, Expect: g.Expect})
		}
		switch level {
		case "L1":
			snap.L1 = es
		case "L2":
			snap.L2 = es
		default:
			snap.L3 = es
		}
	}
	raw, _ := json.Marshal(snap)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// diffAcceptance 对比修订前后的 packet 集，返回人可读的变化行（空=无变化）。
func diffAcceptance(old, fresh []*contract.Packet) []string {
	oldFP := map[string]string{}
	for _, p := range old {
		oldFP[p.PacketID] = acceptanceFingerprint(p)
	}
	freshFP := map[string]string{}
	for _, p := range fresh {
		freshFP[p.PacketID] = acceptanceFingerprint(p)
	}
	var lines []string
	for _, p := range fresh { // 保持契约文件顺序
		fp := freshFP[p.PacketID]
		prev, existed := oldFP[p.PacketID]
		switch {
		case !existed:
			lines = append(lines, fmt.Sprintf("%s added", p.PacketID))
		case prev != fp:
			lines = append(lines, fmt.Sprintf("%s acceptance changed", p.PacketID))
		}
	}
	for _, p := range old {
		if _, ok := freshFP[p.PacketID]; !ok {
			lines = append(lines, fmt.Sprintf("%s removed", p.PacketID))
		}
	}
	return lines
}

// recordAcceptanceChanges 把 acceptance 变化落 retro 与 state 警告（不阻塞）。
func (c *Context) recordAcceptanceChanges(old, fresh []*contract.Packet) {
	for _, line := range diffAcceptance(old, fresh) {
		c.appendRetro(fmt.Sprintf("- %s plan revision: %s", c.nowISO(), line))
		c.Log("acceptance changed (§4.1 known-risk watch): " + line)
		msg := "plan revision touched acceptance: " + line
		if !containsString(c.State.Budget.Warnings, msg) {
			c.State.Budget.Warnings = append(c.State.Budget.Warnings, msg)
		}
	}
}

// authoritativeChangedFiles（C4）：execute 失败路径的空转签名证据。worktree
// 可用时编排器亲自 diff 单元 base ref；不可用（非 git / 无 base）返回 nil，
// 让 failAttempt 的兜底链接管。
func (c *Context) authoritativeChangedFiles(packet *contract.Packet) []string {
	ps := c.State.Packets[packet.PacketID]
	if ps == nil || ps.BaseRef == nil {
		return nil
	}
	files, ok := verify.GetChangedFiles(c.unitCwd(packet), *ps.BaseRef)
	if !ok {
		return nil
	}
	return files
}
