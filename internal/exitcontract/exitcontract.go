// Package exitcontract — 出口契约（docs/design.md §6）：单元成功由三项独立检查判定
// ——进程终态正常、stderr 无错误模式、交付物存在且有效。timedOut / signal /
// exitCode 正交记录；"超时被 trap 成 exit 0"不得读成绿灯。空产出（exit 0 但
// 无可用交付物）映射为可重试的 EMPTY_RESPONSE，不是静默成功。
package exitcontract

import (
	"fmt"

	"github.com/ckanner/flywheel/internal/errs"
)

// Deliverable 一项交付物检查结果（调用方负责 exists/valid 的实际判定）。
type Deliverable struct {
	Path   string
	Exists bool
	Valid  bool
	Detail string // valid=false 时的人类可读原因；空则记 "schema"
}

// Result 出口契约裁决。
type Result struct {
	OK             bool
	ProcessOK      bool
	StderrOK       bool
	DeliverablesOK bool
	ErrorCode      string   // errs.ErrorCode 空串 = 无错误
	Reasons        []string // 失败原因清单（按检查顺序）
}

// Evaluate 三项独立检查并合成错误码。优先级与 Node 一致：
// 进程失败 > 交付物缺失（EMPTY_RESPONSE）> stderr 污染。
func Evaluate(result errs.SpawnResult, deliverables []Deliverable) Result {
	var reasons []string

	// 1. 进程终态。
	processOK := !result.TimedOut && result.Signal == "" && result.SpawnError == nil && result.ExitCode == 0
	if !processOK {
		signal := result.Signal
		if signal == "" {
			signal = "null" // Node 模板对 null 的渲染，保持日志逐字兼容
		}
		reasons = append(reasons, fmt.Sprintf("process: exitCode=%d signal=%s timedOut=%t",
			result.ExitCode, signal, result.TimedOut))
	}

	// 2. stderr 卫生：即使退出码为 0 也不得出现可识别的错误模式。
	stderrCode := ""
	if result.Stderr != "" {
		stderrCode = string(errs.ClassifyError(errs.SpawnResult{ExitCode: 0, Stderr: result.Stderr}))
	}
	stderrOK := stderrCode == ""
	if !stderrOK {
		reasons = append(reasons, "stderr: looks like "+stderrCode)
	}

	// 3. 交付物存在且有效（空交付物列表同样是失败——没有可查项的契约什么都没查）。
	var missing, invalid []Deliverable
	for _, d := range deliverables {
		if !d.Exists {
			missing = append(missing, d)
		} else if !d.Valid {
			invalid = append(invalid, d)
		}
	}
	deliverablesOK := len(deliverables) > 0 && len(missing) == 0 && len(invalid) == 0
	for _, d := range missing {
		reasons = append(reasons, "deliverable missing: "+d.Path)
	}
	for _, d := range invalid {
		detail := d.Detail
		if detail == "" {
			detail = "schema"
		}
		reasons = append(reasons, fmt.Sprintf("deliverable invalid: %s (%s)", d.Path, detail))
	}

	errorCode := ""
	switch {
	case !processOK:
		errorCode = string(errs.ClassifyError(result))
	case !deliverablesOK:
		// exit 0 但无可用交付物 = EMPTY_RESPONSE（可重试），不是成功。
		errorCode = "EMPTY_RESPONSE"
	case !stderrOK:
		errorCode = stderrCode
	}

	return Result{
		OK:             processOK && stderrOK && deliverablesOK,
		ProcessOK:      processOK,
		StderrOK:       stderrOK,
		DeliverablesOK: deliverablesOK,
		ErrorCode:      errorCode,
		Reasons:        reasons,
	}
}
