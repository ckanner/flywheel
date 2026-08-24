// Package donereport — done-report 解析与格式校验（DESIGN.md §5.2、§6）。
// agent 只报告事实（"做了什么、哪些命令过/没过"），永不宣布完成。
// 格式坏的报告由调用方按可重试错误（EMPTY_RESPONSE）处理，绝不静默接受。
package donereport

import (
	"encoding/json"
	"sort"
)

// DefaultFormatKeys packet 未提供 done-report-format 时的默认形状
// （schema 现已强制恰好 4 键，此处为 Node 同款兜底）。
var DefaultFormatKeys = []string{"requirements-covered", "changed-files", "commands-run", "failures"}

// Result 解析结果：ok=false 时 errors 给出全部原因（供 retry 记录）。
type Result struct {
	OK     bool
	Report map[string]any
	Errors []string
}

// Parse 校验 raw 报告内容。format 为 packet 的 done-report-format；
// nil → 用默认键集（非 nil 空表忠实 Node 语义：零键零检查）。
//
// 与 Node 的差异说明：JS Object.keys 保插入序，Go map 无序——format 非 nil 时
// 按排序后键名遍历保证确定性；format 为 nil 时用 DefaultFormatKeys 常量序
// （= Node 的 DEFAULT_FORMAT_KEYS 插入序）。errors 内容与 ok 判定不受顺序影响。
func Parse(raw []byte, format map[string][]string) Result {
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Result{OK: false, Errors: []string{"invalid-json"}}
	}
	report, ok := parsed.(map[string]any)
	if !ok {
		// JSON null / 数组 / 标量：合法 JSON 但不是对象。
		return Result{OK: false, Errors: []string{"not-an-object"}}
	}

	keys := DefaultFormatKeys
	if format != nil {
		keys = make([]string, 0, len(format))
		for k := range format {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}

	var errs []string
	for _, key := range keys {
		v, present := report[key]
		if !present {
			errs = append(errs, "missing-key: "+key)
			continue
		}
		arr, ok := v.([]any)
		if !ok {
			errs = append(errs, "not-an-array: "+key)
			continue
		}
		bad := false
		for _, el := range arr {
			if _, isStr := el.(string); !isStr {
				bad = true
				break
			}
		}
		if bad {
			// 非字符串元素（嵌套数组/对象）会静默破坏下游空转检测的集合比较
			// （P0-8，用户实证）。
			errs = append(errs, "non-string-element: "+key)
		}
	}
	return Result{OK: len(errs) == 0, Report: report, Errors: errs}
}
