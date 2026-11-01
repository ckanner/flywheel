package circuitbreaker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/state"
)

var now = time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

func stWithDay(tokens int64) *state.State {
	st := &state.State{Budget: state.Budget{
		Daily: map[string]state.DailyBucket{"2026-08-22": {Tokens: tokens}},
	}}
	return st
}

func TestTodayKey(t *testing.T) {
	assert.Equal(t, "2026-08-22", TodayKey(now))
	// 非 UTC 输入按 UTC 归一。
	tz := time.Date(2026, 8, 23, 1, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	assert.Equal(t, "2026-08-22", TodayKey(tz))
}

func TestHardBudgetCannotTreatMissingUsageAsZero(t *testing.T) {
	st := stWithDay(1)
	st.Budget.Daily[TodayKey(now)] = state.DailyBucket{Tokens: 1, UnknownUsageCalls: 1}
	cfg := config.Default()
	limit := int64(10000)
	cfg.Budget.DailyTokenLimit = &limit
	cfg.Budget.Enforce = "hard"
	v := CheckBudget(st, &cfg, now)
	assert.Equal(t, ActionStop, v.Action)
	assert.Contains(t, v.Reason, "unknown")
	cfg.Budget.Enforce = "soft"
	assert.Equal(t, ActionOK, CheckBudget(st, &cfg, now).Action)
	assert.Equal(t, ActionOK, CheckBudget(st, &cfg, now.Add(24*time.Hour)).Action)
}

func TestCheckBudgetOffAndNoLimit(t *testing.T) {
	cfg := config.Default()
	cfg.Budget.Enforce = "off"
	limit := int64(100)
	cfg.Budget.DailyTokenLimit = &limit

	v := CheckBudget(stWithDay(9999), &cfg, now)
	assert.Equal(t, ActionOK, v.Action)
	assert.Empty(t, v.Reason)
	assert.Equal(t, "off", v.Enforce)

	cfg.Budget.Enforce = "hard"
	cfg.Budget.DailyTokenLimit = nil
	v = CheckBudget(stWithDay(9999), &cfg, now)
	assert.Equal(t, ActionOK, v.Action)
}

func TestCheckBudgetUnderLimit(t *testing.T) {
	cfg := config.Default()
	limit := int64(1000)
	cfg.Budget.Enforce = "soft"
	cfg.Budget.DailyTokenLimit = &limit
	v := CheckBudget(stWithDay(999), &cfg, now)
	assert.Equal(t, ActionOK, v.Action)
	assert.Empty(t, v.Reason)
}

func TestCheckBudgetSoftWarnHardStop(t *testing.T) {
	limit := int64(1000)
	cfgSoft := config.Default()
	cfgSoft.Budget.Enforce = "soft"
	cfgSoft.Budget.DailyTokenLimit = &limit

	v := CheckBudget(stWithDay(1000), &cfgSoft, now)
	assert.Equal(t, ActionWarn, v.Action)
	assert.Equal(t, "daily token budget exceeded: 1000 >= 1000 (2026-08-22)", v.Reason)
	assert.Equal(t, "soft", v.Enforce)

	cfgHard := cfgSoft
	cfgHard.Budget.Enforce = "hard"
	v = CheckBudget(stWithDay(5000), &cfgHard, now)
	assert.Equal(t, ActionStop, v.Action)
	assert.Contains(t, v.Reason, "5000 >= 1000")
}

func TestCheckBudgetNilConfigDefaultsSoftNoLimit(t *testing.T) {
	v := CheckBudget(stWithDay(9999), nil, now)
	assert.Equal(t, ActionOK, v.Action)
	assert.Equal(t, "soft", v.Enforce)
}

func TestSignatureDeterministicAndOrderIndependent(t *testing.T) {
	a := Signature([]string{"TIMEOUT", "SERVER"}, []string{"pnpm test", "pnpm lint"}, []string{"a.go", "b.go"})
	b := Signature([]string{"SERVER", "TIMEOUT", "SERVER"}, []string{"pnpm lint", "pnpm test"}, []string{"b.go", "a.go"})
	assert.Equal(t, a, b)
	require.NotEqual(t, "", a)
	assert.Contains(t, a, `"files":["a.go","b.go"]`)
}

func TestJaccard(t *testing.T) {
	assert.InDelta(t, 1, Jaccard(nil, nil), 1e-9)
	assert.InDelta(t, 1, Jaccard([]string{"x"}, []string{"x"}), 1e-9)
	assert.InDelta(t, 0, Jaccard([]string{"x"}, []string{"y"}), 1e-9)
	// {a,b,c} vs {b,c,d} → 2/4。
	assert.InDelta(t, 0.5, Jaccard([]string{"a", "b", "c"}, []string{"b", "c", "d"}), 1e-9)
	// 去重语义：重复元素不放大分母。
	assert.InDelta(t, 1, Jaccard([]string{"x", "x"}, []string{"x"}), 1e-9)
}

func TestDetectSpin(t *testing.T) {
	sigA := Signature([]string{"TIMEOUT"}, []string{"cmd-a"}, []string{"f1.go", "f2.go"})
	sigSameModeDiffFiles := Signature([]string{"TIMEOUT"}, []string{"cmd-a"}, []string{"z1.go", "z2.go"})

	// 次数不足 / 无前科 → 不算空转。
	assert.False(t, DetectSpin(sigA, sigA, 1))
	assert.False(t, DetectSpin("", sigA, 5))

	// 同模式 + 文件集完全不同（Jaccard=0）→ 不算空转。
	assert.False(t, DetectSpin(sigA, sigSameModeDiffFiles, 2))

	// 同模式同文件 → 空转。
	assert.True(t, DetectSpin(sigA, sigA, 2))

	// 高相似（3 个文件共享 2 个：2/4 = 0.5 < 0.7 → 否；4/5=0.8 ≥ 0.7 → 是）。
	sigNear := Signature([]string{"TIMEOUT"}, []string{"cmd-a"}, []string{"f1.go", "f2.go", "f3.go", "f4.go"})
	sigNearBase := Signature([]string{"TIMEOUT"}, []string{"cmd-a"}, []string{"f1.go", "f2.go", "f3.go", "f5.go"})
	assert.False(t, DetectSpin(sigNearBase, sigNear, 2)) // 3/5 = 0.6
	sigHot := Signature([]string{"TIMEOUT"}, []string{"cmd-a"}, []string{"f1.go", "f2.go", "f3.go", "f4.go"})
	sigHot2 := Signature([]string{"TIMEOUT"}, []string{"cmd-a"}, []string{"f1.go", "f2.go", "f3.go", "f4.go", "f9.go"})
	assert.True(t, DetectSpin(sigHot, sigHot2, 3)) // 4/5 = 0.8
}

func TestDetectSpinCorruptedPreviousNeverBlocks(t *testing.T) {
	sig := Signature([]string{"TIMEOUT"}, []string{"c"}, []string{"f"})
	assert.False(t, DetectSpin("{corrupt", sig, 3))
	assert.False(t, DetectSpin(sig, "{corrupt", 3))
}
