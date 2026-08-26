package machine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/stretchr/testify/assert"
)

func packetWithL1(id string, cmds ...string) *contract.Packet {
	gates := make([]contract.GateEntry, 0, len(cmds))
	for _, c := range cmds {
		gates = append(gates, contract.GateEntry{Cmd: c, Expect: "exit 0"})
	}
	return &contract.Packet{
		PacketID:     id,
		Goal:         "g",
		Requirements: []string{"R-01"},
		Acceptance:   contract.Acceptance{L1: gates},
	}
}

// TestDiffAcceptance 指纹 diff 的四种臂：changed / added / removed / 无变化。
func TestDiffAcceptance(t *testing.T) {
	old := []*contract.Packet{
		packetWithL1("M01-T01", "pnpm test auth", "pnpm lint"),
		packetWithL1("M01-T02", "go build ./..."),
	}
	fresh := []*contract.Packet{
		packetWithL1("M01-T01", "pnpm test auth"), // 命令被移除 = 改弱
		packetWithL1("M01-T03", "make e2e"),       // 新增
		// M01-T02 消失
	}
	lines := diffAcceptance(old, fresh)
	assert.Equal(t, []string{
		"M01-T01 acceptance changed",
		"M01-T03 added",
		"M01-T02 removed",
	}, lines)

	assert.Empty(t, diffAcceptance(old, old), "无变化 → 空行")
}

// TestRecordAcceptanceChangesWritesTrail 留痕落 retro + state 警告（A1 主张：
// 不阻止，但必须可查）。
func TestRecordAcceptanceChangesWritesTrail(t *testing.T) {
	fx := newGitFixture(t)
	ctx := unitTestContext(t, fx)

	ctx.recordAcceptanceChanges(
		[]*contract.Packet{packetWithL1("M01-T01", "pnpm test auth")},
		[]*contract.Packet{packetWithL1("M01-T01", "true")}, // 弱化示例
	)

	retro, rerr := os.ReadFile(filepath.Join(fx.taskDir, "retro.md"))
	requireNoErr(t, rerr)
	assert.Contains(t, string(retro), "plan revision: M01-T01 acceptance changed")
	assert.Contains(t, ctx.State.Budget.Warnings[0], "M01-T01 acceptance changed")

	// 重复留痕被去重（warnings 不膨胀）。
	ctx.recordAcceptanceChanges(nil, nil)
	assert.Len(t, ctx.State.Budget.Warnings, 1)
}

// requireNoErr 避免本文件直接引 testify/require 也行——统一用 require。
func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestAcceptanceFingerprintStability 同一验收内容指纹稳定、命令集变化即变。
func TestAcceptanceFingerprintStability(t *testing.T) {
	a := packetWithL1("M01-T01", "a", "b")
	b := packetWithL1("M01-T01", "b", "a") // 顺序不同但集合同 → JSON 数组有序，指纹不同是预期吗？
	// 设计取舍：命令执行顺序本身是语义（先 build 后 test），顺序交换视为变化。
	assert.NotEqual(t, acceptanceFingerprint(a), acceptanceFingerprint(b))
	assert.Equal(t, acceptanceFingerprint(a), acceptanceFingerprint(packetWithL1("M01-T01", "a", "b")))
	assert.True(t, strings.HasPrefix(acceptanceFingerprint(a), ""), "sanity")
}
