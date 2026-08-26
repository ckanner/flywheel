package ledger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLedgerClassification(t *testing.T) {
	ledger := []string{
		".state.lock", "state.json", "retro.md", "metrics.jsonl", "inbox.md",
		"orchestrator.pid", "orchestrator.log",
		"triage-inbox.md", "garden-proposals.md",
	}
	for _, p := range ledger {
		assert.True(t, IsLedgerTaskPath(p), "%s must be ledger", p)
	}
	notLedger := []string{
		"src/main.go", "build/x.txt", "spec.md", "task-packet-M01-T01.md",
		"flywheel.config.json", "state", "report.md", "reportsx/x",
	}
	for _, p := range notLedger {
		assert.False(t, IsLedgerTaskPath(p), "%s must NOT be ledger", p)
	}
}

func TestVolatileSet(t *testing.T) {
	assert.True(t, CommitVolatileFiles["metrics.jsonl"])
	assert.True(t, CommitVolatileFiles["orchestrator.log"])
	assert.True(t, CommitVolatileFiles["orchestrator.pid"])
	assert.False(t, CommitVolatileFiles["state.json"], "state 必须留在已提交契约层")
	assert.True(t, CommitVolatileFiles[".state.lock"], "flock 锁文件不入库（v0.18）")
	assert.False(t, CommitVolatileFiles["retro.md"])
	assert.False(t, CommitVolatileFiles["reports/"])
}

func TestLedgerDirsPrefixOnly(t *testing.T) {
	// 目录前缀匹配：reports/ 前缀命中，但 "report" 裸文件名不命中。
	assert.True(t, IsLedgerTaskPath("reports/a.json"))
	assert.False(t, IsLedgerTaskPath("report"))
	assert.False(t, IsLedgerTaskPath("reports"))
}
