// Package circuitbreaker — 预算熔断（DESIGN.md §6，铁律 9）：enforce 是开关
// —— off | soft | hard（默认 soft：警告并记录，从不停机）。token/成本记录在
// 任何档位下都开启。空转检测与钱无关：同一 packet 以相同方式反复失败在任何
// 档位下都升级——那意味着 loop 不健康。
package circuitbreaker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/state"
)

// Action 预算检查结论。
type Action string

const (
	ActionOK   Action = "ok"
	ActionWarn Action = "warn"
	ActionStop Action = "stop"
)

// BudgetVerdict checkBudget 结果。
type BudgetVerdict struct {
	Action  Action
	Reason  string
	Enforce string
}

// TodayKey now 的 YYYY-MM-DD（UTC）。
func TodayKey(now time.Time) string {
	return now.UTC().Format("2006-01-02")
}

// CheckBudget 检查当日 token 用量对配置限额。off 或无限额 → ok；
// 用量 >= 限额时 hard 停、soft 警。记录永不关闭（那是 RecordUsage 的职责）。
func CheckBudget(st *state.State, cfg *config.Config, now time.Time) BudgetVerdict {
	enforce := "soft"
	var limit *int64
	if cfg != nil {
		if cfg.Budget.Enforce != "" {
			enforce = cfg.Budget.Enforce
		}
		limit = cfg.Budget.DailyTokenLimit
	}
	if enforce == "off" || limit == nil {
		return BudgetVerdict{Action: ActionOK, Enforce: enforce}
	}
	day := TodayKey(now)
	var used int64
	if bucket, ok := st.Budget.Daily[day]; ok {
		used = bucket.Tokens
	}
	if used < *limit {
		return BudgetVerdict{Action: ActionOK, Enforce: enforce}
	}
	reason := "daily token budget exceeded: " + strconv.FormatInt(used, 10) +
		" >= " + strconv.FormatInt(*limit, 10) + " (" + day + ")"
	if enforce == "hard" {
		return BudgetVerdict{Action: ActionStop, Reason: reason, Enforce: enforce}
	}
	return BudgetVerdict{Action: ActionWarn, Reason: reason, Enforce: enforce}
}

// Signature 失败签名（v0.14 DC4）：失败模式键（错误码+失败命令，去重排序后
// sha256 前 16 hex）+ 变更文件集（去重排序，供相似度比较）。序列化为 JSON
// 存入 state.json。
func Signature(errorCodes, failedCommands, changedFiles []string) string {
	mode := struct {
		ErrorCodes     []string `json:"errorCodes"`
		FailedCommands []string `json:"failedCommands"`
	}{
		ErrorCodes:     dedupSorted(errorCodes),
		FailedCommands: dedupSorted(failedCommands),
	}
	modeJSON, _ := json.Marshal(mode)
	sum := sha256.Sum256(modeJSON)
	sig := struct {
		Key   string   `json:"key"`
		Files []string `json:"files"`
	}{
		Key:   hex.EncodeToString(sum[:])[:16],
		Files: dedupSorted(changedFiles),
	}
	out, _ := json.Marshal(sig)
	return string(out)
}

func dedupSorted(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// SpinJaccardThreshold 空转变更文件集相似度阈值（DC4）。
const SpinJaccardThreshold = 0.7

// Jaccard 两个字符串集合的 Jaccard 相似度（|A∩B| / |A∪B|；双空 → 1）。
func Jaccard(a, b []string) float64 {
	setA := make(map[string]bool, len(a))
	for _, s := range a {
		setA[s] = true
	}
	setB := make(map[string]bool, len(b))
	for _, s := range b {
		setB[s] = true
	}
	if len(setA) == 0 && len(setB) == 0 {
		return 1
	}
	intersection := 0
	for x := range setA {
		if setB[x] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	return float64(intersection) / float64(union)
}

// DetectSpin 空转检测（v0.14 DC4）：同一 packet 连续 >=2 次失败且失败模式
// 一致（错误码+失败命令键相同）且变更文件集高度相似（Jaccard >= 0.7）。
// previousSignature 为 "" 表示无前科。损坏的存储签名永不阻断推进。
func DetectSpin(previousSignature, currentSignature string, consecutiveFailures int) bool {
	if consecutiveFailures < 2 || previousSignature == "" {
		return false
	}
	var prev, curr struct {
		Key   string   `json:"key"`
		Files []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(previousSignature), &prev); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(currentSignature), &curr); err != nil {
		return false
	}
	return prev.Key == curr.Key && Jaccard(prev.Files, curr.Files) >= SpinJaccardThreshold
}
