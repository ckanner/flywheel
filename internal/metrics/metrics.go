// Package metrics — 可观测（docs/design.md §6）：每次 spawn 追加一条 JSONL 记录
// ——model、harness、prompt 哈希、token、成本、缓存命中、耗时、退出码、错误码。
// 这张表是人不盯 loop 也能监督它的方式。
package metrics

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ckanner/flywheel/internal/config"
)

// Keep the I/O boundary injectable: an unflushed audit record must never let
// the caller commit its accounting CAS, even when Write already returned bytes.
var writeMetricsLine = (*os.File).Write
var syncMetricsHandle = (*os.File).Sync
var openMetricsSync = os.Open

// A phase append must not race an accounting read/deduplicate/append operation.
// Each record remains one durable JSONL write; the task lease still owns state.
var appendMetricsMu sync.Mutex

func flushMetrics(file *os.File) error {
	if err := syncMetricsHandle(file); err != nil {
		return err
	}
	directory, err := openMetricsSync(filepath.Dir(file.Name()))
	if err != nil {
		return err
	}
	defer directory.Close()
	return syncMetricsHandle(directory)
}

// PromptHash 提示词哈希（sha256 前 12 hex）——失败归因与缓存对账用。
func PromptHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])[:12]
}

// Append 追加一条指标记录。刻意同步：编排器随后立刻死掉也必须已落盘。
// entry 为任意键值集；ts 由本函数前置。Go map 序列化按键字典序（Node 版
// ts 在首为插入序）——JSONL 消费方（人/jq）不依赖键序。
func Append(file string, entry map[string]any) error {
	appendMetricsMu.Lock()
	defer appendMetricsMu.Unlock()
	return appendEntry(file, entry)
}

func appendEntry(file string, entry map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	rec := make(map[string]any, len(entry)+1)
	rec["ts"] = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	for k, v := range entry {
		rec[k] = v
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = writeMetricsLine(f, append(line, '\n')); err != nil {
		return err
	}
	return flushMetrics(f)
}

// AppendOnce deduplicates recovery telemetry by invocation and receipt digest.
// The task run lease serializes callers. A torn log is reported rather than
// silently appending a second ambiguous usage record; state remains authoritative.
func AppendOnce(file, invocationID, receiptDigest string, entry map[string]any) error {
	if invocationID == "" {
		return Append(file, entry)
	}
	appendMetricsMu.Lock()
	defer appendMetricsMu.Unlock()
	raw, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		return fmt.Errorf("cannot reconcile metrics: unterminated final record")
	}
	found := false
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var previous map[string]any
		if err := json.Unmarshal(line, &previous); err != nil {
			return fmt.Errorf("cannot reconcile metrics: %w", err)
		}
		if previous == nil {
			return fmt.Errorf("cannot reconcile metrics: record must be an object")
		}
		if previous["invocation-id"] == invocationID {
			if previous["receipt-sha256"] != receiptDigest {
				return fmt.Errorf("metrics receipt identity mismatch")
			}
			if found {
				return fmt.Errorf("cannot reconcile metrics: duplicate invocation %s", invocationID)
			}
			found = true
		}
	}
	if found {
		// A previous append may have failed during fsync after leaving readable
		// bytes. Reconfirm durability without appending another invocation.
		f, err := openMetricsSync(file)
		if err != nil {
			return err
		}
		defer f.Close()
		return flushMetrics(f)
	}
	record := make(map[string]any, len(entry)+2)
	for key, value := range entry {
		record[key] = value
	}
	record["invocation-id"], record["receipt-sha256"] = invocationID, receiptDigest
	return appendEntry(file, record)
}

// PriceForModel 模型定价查找（P1-13）：先全 id，再最后一段短 id，
// 再 kimi- 前缀短 id——"kimi-code/k3" 与 "kimi-coding/k3" 都必须落到
// "kimi-k3" 定价条目。未命中返回 nil。
func PriceForModel(pricing map[string]config.Price, model string) *config.Price {
	if len(pricing) == 0 || model == "" {
		return nil
	}
	short := model
	for i := len(model) - 1; i >= 0; i-- {
		if model[i] == '/' {
			short = model[i+1:]
			break
		}
	}
	if p, ok := pricing[model]; ok {
		return &p
	}
	if p, ok := pricing[short]; ok {
		return &p
	}
	if p, ok := pricing["kimi-"+short]; ok {
		return &p
	}
	return nil
}

// Usage 成本估算输入（driver 未报字段的缺省语义见 EstimateCostUSD）。
type Usage struct {
	Input              int64
	Output             int64
	CacheRead          int64
	CacheWrite         int64
	InputIncludesCache bool // false preserves the legacy pi disjoint input bucket
}

// InputIncludesCache identifies adapters whose normalized Input already counts
// cache reads and writes. Pricing must first recover the uncached input bucket;
// token totals and cache-hit metrics retain the original inclusive counters.
func InputIncludesCache(harness string) bool {
	switch harness {
	case "dsh", "codex-pro", "claude-code-pro":
		return true
	default:
		return false
	}
}

// EstimateCostUSD driver 不报成本时的兜底估算（docs/design.md 附录 D：usage 明细
// 可能缺字段，绝不假设存在）。price 为每百万 token 美元价。cache-write 按
// 未命中 input 价计费（DeepSeek 语义）；pi 的 usage 桶互不相交（已在 pi
// dist usage-totals.js 核实）。包含缓存的 Input 必须先拆成互不相交的计价桶，
// 缓存子集矛盾或无法估算时返回 nil，不能双计或得到负成本。
func EstimateCostUSD(usage *Usage, price *config.Price) *float64 {
	if usage == nil || price == nil {
		return nil
	}
	if usage.Input < 0 || usage.Output < 0 || usage.CacheRead < 0 || usage.CacheWrite < 0 {
		return nil
	}
	uncachedInput := usage.Input
	if usage.InputIncludesCache {
		// Subtract in stages so even invalid int64 counters cannot overflow
		// while checking whether the two disjoint cache buckets fit in Input.
		if usage.CacheRead > uncachedInput || usage.CacheWrite > uncachedInput-usage.CacheRead {
			return nil
		}
		uncachedInput -= usage.CacheRead + usage.CacheWrite
	}
	const perM = 1_000_000
	input := float64(uncachedInput) / perM * price.Input
	output := float64(usage.Output) / perM * price.Output
	cacheRead := float64(usage.CacheRead) / perM * price.CacheRead
	cacheWrite := float64(usage.CacheWrite) / perM * price.Input // 未命中 input 价
	total := (input + output + cacheRead + cacheWrite)
	cost := math.Round(total*1e8) / 1e8 // 与 state.RecordUsage 同一舍入纪律
	return &cost
}
