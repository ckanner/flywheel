// Mock driver：确定性的假 Plan/Execute/Review，供测试与演练。
// Execute 把交付物（构建产物 + done-report）精确写到契约要求的位置——编排器
// 的出口契约、done-report 解析、L1 与状态转移全部对它真实运转。
package drivers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

var mockUsage = Usage{Input: 120, Output: 60, Reasoning: 15, CacheRead: 40, CacheWrite: 10, TotalTokens: 180}

func mockResult(apply func(*Result)) Result {
	cost := 0.001
	rc := int64(42)
	r := Result{
		ExitCode:       intPtr(0),
		DurationMS:     1,
		Harness:        "mock",
		Provider:       "mock",
		Model:          "mock-model",
		Usage:          usagePtr(mockUsage),
		ReasoningChars: &rc,
		CostUSD:        &cost,
		Settled:        true,
	}
	if apply != nil {
		apply(&r)
	}
	return r
}

func intPtr(v int) *int       { return &v }
func usagePtr(u Usage) *Usage { return &u }

// BehaviorFn mock 行为覆盖：返回非 nil 的 Result 完全替换默认结果
// （Node 版为 {...base, ...behavior(ctx)} 浅合并；Go 无零值可区分语义，
// 测试用 MockResult 构造基底后改字段——表达力等价）。
type BehaviorFn func(a CallArgs) *Result

// MockDriver 确定性假驱动。behaviors 按角色再按 id（milestone/packet）覆盖。
type MockDriver struct {
	name      string
	behaviors map[string]map[string]BehaviorFn
}

// NewMockDriver 构造。behaviors 形如 {"execute": {"M01-T01": fn}, "plan": {"M-01": fn}}。
func NewMockDriver(behaviors any) *MockDriver {
	d := &MockDriver{name: "mock", behaviors: map[string]map[string]BehaviorFn{}}
	if m, ok := behaviors.(map[string]any); ok {
		for role, byID := range m {
			if ids, ok := byID.(map[string]any); ok {
				d.behaviors[role] = map[string]BehaviorFn{}
				for id, fn := range ids {
					if f, ok := fn.(BehaviorFn); ok {
						d.behaviors[role][id] = f
					}
				}
			}
		}
	}
	return d
}

func (d *MockDriver) Name() string { return d.name }

func (d *MockDriver) behavior(role, id string) BehaviorFn {
	return d.behaviors[role][id]
}

// MockResult 测试构造 mock 基底的帮手（对应 Node mockResult）。
func MockResult(apply func(*Result)) Result { return mockResult(apply) }

func (d *MockDriver) Plan(a CallArgs) Result {
	base := mockResult(func(r *Result) {
		payload, _ := json.Marshal(map[string]any{
			"milestone": nilIfEmpty(a.MilestoneID),
			"packets":   a.Packets,
			"action":    "issue",
		})
		r.Text = string(payload)
	})
	if fn := d.behavior("plan", a.MilestoneID); fn != nil {
		if override := fn(a); override != nil {
			return *override
		}
	}
	return base
}

func (d *MockDriver) Execute(a CallArgs) Result {
	id := a.PacketID
	// 确定性交付物：构建产物在工作区根（L1 命令运行处），done-report 在任务目录。
	workspace := a.Cwd
	if workspace == "" {
		workspace = a.TaskDir
	}
	_ = os.MkdirAll(filepath.Join(workspace, "build"), 0o755)
	_ = os.MkdirAll(filepath.Join(a.TaskDir, "reports"), 0o755)
	artifact := "build/" + id + ".txt"
	_ = os.WriteFile(filepath.Join(workspace, artifact), []byte("ok "+id+"\n"), 0o644)

	doneReport := map[string]any{
		"requirements-covered": []string{},
		"changed-files":        []string{artifact},
		"commands-run":         []string{},
		"failures":             []string{},
	}
	reportPath := filepath.Join(a.TaskDir, "reports", id+".done.json")
	payload, _ := json.MarshalIndent(doneReport, "", "  ")
	_ = os.WriteFile(reportPath, append(payload, '\n'), 0o644)

	base := mockResult(func(r *Result) {
		r.Text = "mock executed " + id
	})
	if fn := d.behavior("execute", id); fn != nil {
		if override := fn(a); override != nil {
			return *override
		}
	}
	return base
}

func (d *MockDriver) Review(a CallArgs) Result {
	base := mockResult(func(r *Result) {
		r.Text = `{"verdict":"pass","score":1,"kind":"ok","reasons":[]}`
	})
	if fn := d.behavior("review", a.PacketID); fn != nil {
		if override := fn(a); override != nil {
			return *override
		}
	}
	return base
}

func (d *MockDriver) Judge(a CallArgs) Result {
	base := mockResult(func(r *Result) {
		r.Text = `{"verdict":"pass","score":1,"kind":"ok","reasons":[]}`
	})
	if fn := d.behavior("judge", a.MilestoneID); fn != nil {
		if override := fn(a); override != nil {
			return *override
		}
	}
	return base
}

func (d *MockDriver) Intake(CallArgs) Result {
	return mockResult(func(r *Result) {
		r.Text = `{"verdict":"recommend","reason":"mock: looks actionable and verifiable"}`
	})
}

func (d *MockDriver) Garden(CallArgs) Result {
	return mockResult(func(r *Result) {
		r.Text = strings.Join([]string{
			"## GP-01 — Recurring transport failures need a retry-hardening note",
			"- type: failure-pattern",
			`- evidence: retro.md:"packet <packet>: INBOX — transport" (x2)`,
			"- proposal: add a skill documenting the channel-fallback drill so future tasks reuse it",
		}, "\n")
	})
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
