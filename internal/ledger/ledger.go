// Package ledger — 任务目录内（docs/<slug>）的账本区路径分类（单一信源，
// AGENTS.md 硬契约）。边界判定从 executor diff 排除这些路径；缺陷注入 patch
// 触碰它们被拒绝（P0-3）；单元提交排除易变运行时三件套。
package ledger

import "strings"

// LedgerFiles 账本文件（编排器专属/运行记录）。triage-inbox.md /
// garden-proposals.md 是 intake/garden 模块的产物（P3 修复：Node 版漏分类，
// 导致它们可能落进单元 diff 判定）。
var LedgerFiles = map[string]bool{
	".state.lock":         true, // C9 flock 锁文件（v0.18）——绝不进单元 diff/提交
	".run.lock":           true,
	".stop-request":       true,
	"state.json":          true,
	"retro.md":            true,
	"metrics.jsonl":       true,
	"inbox.md":            true,
	"orchestrator.pid":    true,
	"orchestrator.log":    true,
	"triage-inbox.md":     true,
	"garden-proposals.md": true,
}

// LedgerDirs 账本目录（带尾部斜杠，供前缀匹配）。
var LedgerDirs = []string{"reports/", "approvals/", ".control/"}

// CommitVolatileFiles 纯运行时产物：不进单元提交（活日志绝不能进 git 历史；
// metrics 是追加式遥测）。账本区其余（state/retro/inbox/reports/approvals）
// 留在已提交契约层。
var CommitVolatileFiles = map[string]bool{
	"metrics.jsonl":    true,
	"orchestrator.log": true,
	"orchestrator.pid": true,
	".state.lock":      true, // v0.18：flock 锁文件是运行时产物，不入库
	".run.lock":        true,
	".stop-request":    true,
	".control/":        true,
}

// IsLedgerTaskPath 判断任务目录相对路径（如 "reports/x.json"）是否为账本。
func IsLedgerTaskPath(inner string) bool {
	if LedgerFiles[inner] {
		return true
	}
	for _, d := range LedgerDirs {
		if strings.HasPrefix(inner, d) {
			return true
		}
	}
	return false
}
