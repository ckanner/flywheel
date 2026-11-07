package contract

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/ckanner/flywheel/templates"
)

const validPacket = `---
packet-id: M01-T01
contract-version: 1
goal: Implement the auth token parser.
requirements: [R-01, R-02]
route: default
needs-live-credentials: false
context-files: [src/auth/*.ts]
cache-scope: [docs/spec.md]
boundaries:
  must-not-touch: [src/payments/, tests/]
  max-files: 6
acceptance:
  L1:
    - {cmd: "pnpm test auth", expect: "exit 0"}
  L2:
    - {cmd: "pnpm test:integration auth", expect: "exit 0"}
review-rubric: "Pass line: score >= 0.8."
escalation: {max-retries: 3, on-fail: return-to-planner}
done-report-format:
  requirements-covered: []
  changed-files: []
  commands-run: []
  failures: []
---

# Work instructions

Implement only, never write tests.
`

const validSpec = `---
spec-version: 1
title: Auth token foundation
goal: Provide a verified JWT parsing path.
milestones:
  - id: M-01
    title: Token parser
requirements:
  - id: R-01
    title: parseToken returns claims
    milestone: M-01
    verification: pnpm test auth
  - id: R-02
    title: parseToken returns null
    milestone: M-01
    verification: pnpm test auth
---

# Spec body
`

func TestSplitFrontmatter(t *testing.T) {
	fm, body, ok := splitFrontmatter([]byte(validPacket))
	assert.True(t, ok)
	assert.Contains(t, fm, "packet-id: M01-T01")
	assert.Contains(t, body, "# Work instructions")

	// CRLF 首行。
	_, _, ok = splitFrontmatter([]byte("---\r\nx: 1\r\n---\r\nbody"))
	assert.True(t, ok)

	// 缺失 frontmatter。
	_, _, ok = splitFrontmatter([]byte("# no frontmatter"))
	assert.False(t, ok)

	// 首行后无收尾行。
	_, _, ok = splitFrontmatter([]byte("---\nx: 1\nno closing"))
	assert.False(t, ok)

	// 首个独立 --- 收尾（frontmatter 内含 "---" 前缀行不误收）。
	fm, body, ok = splitFrontmatter([]byte("---\na: ---b\n---\nbody"))
	assert.True(t, ok)
	assert.Contains(t, fm, "a: ---b")
	assert.Equal(t, "body", body)
}

func TestParseFrontmatterErrors(t *testing.T) {
	_, _, err := ParseFrontmatter([]byte("# no fm"), "p.md")
	assert.Error(t, err)

	// 真正的 YAML 语法错误（tab 开头）——注意输入须有 frontmatter 终结行，
	// 否则死在 splitFrontmatter 而非 Unmarshal。
	_, _, err = ParseFrontmatter([]byte("---\n\tfoo: bar\n---\n"), "p.md")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid YAML")

	// 解析成非 mapping（null）→ 专属错误。
	_, _, err = ParseFrontmatter([]byte("---\nnull\n---\n"), "p.md")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a mapping")
}

func TestParsePacketValid(t *testing.T) {
	p, err := ParsePacket([]byte(validPacket), "task-packet-M01-T01.md")
	require.NoError(t, err)
	assert.Equal(t, "M01-T01", p.PacketID)
	assert.Equal(t, 1, p.ContractVersion)
	assert.Equal(t, []string{"R-01", "R-02"}, p.Requirements)
	assert.Equal(t, "default", p.Route)
	assert.False(t, p.NeedsLiveCredentials)
	assert.Equal(t, []string{"src/payments/", "tests/"}, p.Boundaries.MustNotTouch)
	assert.Equal(t, 6, *p.Boundaries.MaxFiles)
	assert.Len(t, p.Acceptance.L1, 1)
	assert.Equal(t, "pnpm test auth", p.Acceptance.L1[0].Command())
	assert.Equal(t, "exit 0", p.Acceptance.L1[0].Expect)
	assert.Equal(t, 3, p.Escalation.MaxRetries)
	assert.Equal(t, "return-to-planner", p.Escalation.OnFail)
	assert.Equal(t, "task-packet-M01-T01.md", p.File)
	assert.Equal(t, "# Work instructions\n\nImplement only, never write tests.", p.Body)

	ms, err := MilestoneOfPacket(p.PacketID)
	require.NoError(t, err)
	assert.Equal(t, "M-01", ms)
}

func TestParsePacketSchemaViolations(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(m map[string]any)
		wantSub string
	}{
		{"unknown field", func(m map[string]any) { m["totally-extra"] = 1 }, "schema violation"},
		{"missing required", func(m map[string]any) { delete(m, "goal") }, "schema violation"},
		{"bad route", func(m map[string]any) { m["route"] = "sideways" }, "schema violation"},
		{"empty requirements", func(m map[string]any) { m["requirements"] = []any{} }, "schema violation"},
		{"bad requirement id", func(m map[string]any) { m["requirements"] = []any{"r-1"} }, "schema violation"},
		{"non-string must-not-touch", func(m map[string]any) {
			m["boundaries"] = map[string]any{"must-not-touch": []any{1, 2}}
		}, "schema violation"},
		{"empty boundaries", func(m map[string]any) { m["boundaries"] = map[string]any{} }, "schema violation"},
		{"done-report missing key", func(m map[string]any) {
			m["done-report-format"] = map[string]any{"requirements-covered": []any{}}
		}, "schema violation"},
		{"bad packet id", func(m map[string]any) { m["packet-id"] = "T01-M01" }, "schema violation"},
		{"bad expect", func(m map[string]any) {
			m["acceptance"] = map[string]any{"L1": []any{map[string]any{"cmd": "x", "expect": "success"}}}
		}, "schema violation"},
		{"gate entry without cmd or script", func(m map[string]any) {
			m["acceptance"] = map[string]any{"L1": []any{map[string]any{"expect": "exit 0"}}}
		}, "schema violation"},
		{"non-object acceptance", func(m map[string]any) { m["acceptance"] = []any{} }, "schema violation"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, _, err := ParseFrontmatter([]byte(validPacket), "p.md")
			require.NoError(t, err)
			c.mutate(m)
			_, err = ParsePacket(mustMarshalFrontmatter(t, m), "p.md")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), c.wantSub)
		})
	}
}

// mustMarshalFrontmatter 把 map 渲染成 frontmatter 文本（用 yaml.Marshal，
// 嵌套结构可靠）。
func mustMarshalFrontmatter(t *testing.T, m map[string]any) []byte {
	t.Helper()
	raw, err := yaml.Marshal(m)
	require.NoError(t, err)
	return append([]byte("---\n"), append(raw, []byte("---\n")...)...)
}

func TestParseSpec(t *testing.T) {
	s, err := ParseSpec([]byte(validSpec), "spec.md")
	require.NoError(t, err)
	assert.Equal(t, "Auth token foundation", s.Title)
	assert.Equal(t, 2, len(s.Requirements))
	assert.True(t, s.ReqIDs["R-01"])
	assert.True(t, s.MilestoneIDs["M-01"])

	// 重复需求 id。
	m, _, err := ParseFrontmatter([]byte(validSpec), "spec.md")
	require.NoError(t, err)
	m["requirements"] = []any{
		map[string]any{"id": "R-01", "title": "a", "milestone": "M-01"},
		map[string]any{"id": "R-01", "title": "b", "milestone": "M-01"},
	}
	_, err = ParseSpec(mustMarshalFrontmatter(t, m), "spec.md")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate requirement id")

	// 未知里程碑。
	m2, _, _ := ParseFrontmatter([]byte(validSpec), "spec.md")
	m2["requirements"] = []any{
		map[string]any{"id": "R-01", "title": "a", "milestone": "M-99"},
	}
	_, err = ParseSpec(mustMarshalFrontmatter(t, m2), "spec.md")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown milestone")

	// 缺 title → schema 拒绝（Node validateSpec 不查，审计 P2）。
	m3, _, _ := ParseFrontmatter([]byte(validSpec), "spec.md")
	delete(m3, "title")
	_, err = ParseSpec(mustMarshalFrontmatter(t, m3), "spec.md")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "schema violation")

	// 缺里程碑（schema minItems）。
	m4, _, _ := ParseFrontmatter([]byte(validSpec), "spec.md")
	m4["milestones"] = []any{}
	_, err = ParseSpec(mustMarshalFrontmatter(t, m4), "spec.md")
	assert.Error(t, err)
}

func TestMilestoneOfPacket(t *testing.T) {
	for _, c := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"M02-T03", "M-02", true},
		{"M123-T456", "M-123", true},
		{"M1-T1", "", false},
		{"M-02-T03", "", false},
		{"x", "", false},
	} {
		got, err := MilestoneOfPacket(c.in)
		if c.ok {
			assert.NoError(t, err)
			assert.Equal(t, c.want, got)
		} else {
			assert.Error(t, err)
		}
	}
}

func TestParseExpect(t *testing.T) {
	code, err := ParseExpect("exit 0")
	assert.NoError(t, err)
	assert.Equal(t, 0, code)
	code, err = ParseExpect(" exit 5 ")
	assert.NoError(t, err)
	assert.Equal(t, 5, code)
	_, err = ParseExpect("success")
	assert.Error(t, err)
	_, err = ParseExpect("0")
	assert.Error(t, err)
}

func TestLoadSpecAndPackets(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "spec.md"), []byte(validSpec), 0o644))

	// 缺 packet 文件。
	_, err := LoadPackets(dir, nil)
	assert.Error(t, err)

	// 两个 packet：文件按名排序 + 按 packet-id 排序（M01-T02 < M01-T10 词法序）。
	p10 := `---
packet-id: M01-T10
contract-version: 1
goal: g
requirements: [R-01]
route: default
context-files: []
boundaries: {max-files: 1}
acceptance: {L1: [{cmd: "true", expect: "exit 0"}]}
review-rubric: "r"
escalation: {max-retries: 1, on-fail: inbox}
done-report-format: {requirements-covered: [], changed-files: [], commands-run: [], failures: []}
---
`
	p2 := `---
packet-id: M01-T02
contract-version: 1
goal: g
requirements: [R-01]
route: default
context-files: []
boundaries: {max-files: 1}
acceptance: {L1: [{cmd: "true", expect: "exit 0"}]}
review-rubric: "r"
escalation: {max-retries: 1, on-fail: inbox}
done-report-format: {requirements-covered: [], changed-files: [], commands-run: [], failures: []}
---
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "task-packet-M01-T02.md"), []byte(p2), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "task-packet-M01-T10.md"), []byte(p10), 0o644))

	spec, err := LoadSpec(dir)
	require.NoError(t, err)
	packets, err := LoadPackets(dir, spec)
	require.NoError(t, err)
	assert.Equal(t, []string{"M01-T02", "M01-T10"}, []string{packets[0].PacketID, packets[1].PacketID})

	// requirement 未在 spec 声明 → 拒绝。
	badReq := `---
packet-id: M01-T99
contract-version: 1
goal: g
requirements: [R-99]
route: default
context-files: []
boundaries: {max-files: 1}
acceptance: {L1: [{cmd: "true", expect: "exit 0"}]}
review-rubric: "r"
escalation: {max-retries: 1, on-fail: inbox}
done-report-format: {requirements-covered: [], changed-files: [], commands-run: [], failures: []}
---
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "task-packet-M01-T99.md"), []byte(badReq), 0o644))
	_, err = LoadPackets(dir, spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "R-99")

	// 坏 packet 文件 → 错误（含文件名）。
	require.NoError(t, os.WriteFile(filepath.Join(dir, "task-packet-bad.md"), []byte("---\npacket-id: nope\n---\n"), 0o644))
	_, err = LoadPackets(dir, spec)
	assert.Error(t, err)
}

func TestLoadSpecMissing(t *testing.T) {
	_, err := LoadSpec(t.TempDir())
	assert.Error(t, err)
}

// example 文件必须与 schema 保持兼容（单一信源，AGENTS.md lockstep）。
func TestExampleContractsValidate(t *testing.T) {
	packetExample, err := templates.FS.ReadFile("task-packet.example.md")
	require.NoError(t, err)
	_, err = ParsePacket(packetExample, "task-packet.example.md")
	assert.NoError(t, err, "task-packet.example.md 必须通过 schema")

	specExample, err := templates.FS.ReadFile("spec.example.md")
	require.NoError(t, err)
	_, err = ParseSpec(specExample, "spec.example.md")
	assert.NoError(t, err, "spec.example.md 必须通过 schema")
}

func TestStateSchemaValidation(t *testing.T) {
	// 合法形状（与 state.schema.json 对齐；完整 initState 交叉测试在 state 包）。
	good := map[string]any{
		"schema-version":    1,
		"slug":              "minitask",
		"revision":          0,
		"status":            "pending",
		"current-milestone": "M-01",
		"plan-session":      nil,
		"milestones":        map[string]any{},
		"packets":           map[string]any{},
		"budget": map[string]any{
			"enforce":           "soft",
			"daily-token-limit": nil,
			"daily":             map[string]any{},
			"totals":            map[string]any{},
			"cache":             map[string]any{},
			"warnings":          []any{},
		},
		"last-progress-at": nil,
		"inbox":            []any{},
	}
	assert.NoError(t, ValidateStateValue(good))

	// 非法形状。
	bad := map[string]any{"revision": "x"}
	err := ValidateStateValue(bad)
	assert.Error(t, err)
}

func TestErrorString(t *testing.T) {
	assert.Equal(t, "boom", (&Error{Msg: "boom"}).Error())
	assert.Equal(t, "p.md: boom", (&Error{File: "p.md", Msg: "boom"}).Error())
}

func TestGateEntryCommand(t *testing.T) {
	assert.Equal(t, "pnpm test", (GateEntry{Cmd: "pnpm test", Script: "run.ts"}).Command())
	assert.Equal(t, "e2e/run.ts", (GateEntry{Script: "e2e/run.ts"}).Command())
	assert.Empty(t, (GateEntry{}).Command())
}

func TestAcceptanceLevel(t *testing.T) {
	a := Acceptance{
		L1: []GateEntry{{Cmd: "l1"}},
		L2: []GateEntry{{Cmd: "l2"}},
		L3: []GateEntry{{Script: "l3"}},
	}
	assert.Equal(t, a.L1, a.Level("L1"))
	assert.Equal(t, a.L2, a.Level("L2"))
	assert.Equal(t, a.L3, a.Level("L3"))
	assert.Nil(t, a.Level("L4"))
	assert.Nil(t, (Acceptance{}).Level("L1"))
}

func TestSpecSlug(t *testing.T) {
	// Node 语义 `spec.slug ?? basename(taskDir)`；Go 版 schema 禁 slug 字段，
	// 恒取目录尾段（contract.go Slug 行为差异注释）。
	s := &Spec{Title: "Some Task"}
	assert.Equal(t, "mytask", s.Slug("docs/mytask"))
	assert.Equal(t, "mytask", s.Slug("/abs/path/mytask/"))
	empty := &Spec{}
	assert.Equal(t, "mytask", empty.Slug("docs/mytask"))
}

func TestParsePacketTypedConversionError(t *testing.T) {
	// schema 放行（integer 无 maximum）但 2^63 溢出 int → typed conversion 失败。
	content := strings.Replace(validPacket, "contract-version: 1", "contract-version: 9223372036854775808", 1)
	_, err := ParsePacket([]byte(content), "big.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed typed conversion")
}

func TestParseSpecTypedConversionError(t *testing.T) {
	content := strings.Replace(validSpec, "spec-version: 1", "spec-version: 9223372036854775808", 1)
	if content == validSpec {
		t.Fatal("validSpec must contain spec-version: 1 for this test")
	}
	_, err := ParseSpec([]byte(content), "spec.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed typed conversion")
}

func TestParsePacketMilestoneGuard(t *testing.T) {
	// deriveMilestone 双保险分支：schema pattern 锚定后正常输入不可达，注入覆盖。
	orig := deriveMilestone
	defer func() { deriveMilestone = orig }()
	deriveMilestone = func(string) (string, error) { return "", errors.New("guard fired") }
	_, err := ParsePacket([]byte(validPacket), "p.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "guard fired")
}

func TestLoadPacketsReadDirError(t *testing.T) {
	_, err := LoadPackets(filepath.Join(t.TempDir(), "nope"), nil)
	require.Error(t, err)
}

func TestSchemaViolationsNonValidationError(t *testing.T) {
	assert.Equal(t, []string{"boom"}, schemaViolations(errors.New("boom")))
	assert.Nil(t, schemaViolations((*jsonschema.ValidationError)(nil)))
}

func TestFlattenErrorsEmptyTree(t *testing.T) {
	// BasicOutput 的 flatten 树必含 Error 单元，空树只能手造（Valid 单元无 Error）。
	root := &jsonschema.OutputUnit{
		Valid:  true,
		Errors: []jsonschema.OutputUnit{{Valid: true}},
	}
	assert.Nil(t, flattenErrors(root))
}

func TestCompileAllErrors(t *testing.T) {
	// happy path 与 embed 等价。
	okLoad := func(name string) ([]byte, error) { return templates.FS.ReadFile(name) }
	p, s, st, err := compileAll(okLoad)
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.NotNil(t, s)
	assert.NotNil(t, st)

	// load 失败。
	failLoad := func(string) ([]byte, error) { return nil, os.ErrNotExist }
	_, _, _, err = compileAll(failLoad)
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)

	// 非法 JSON。
	badJSON := func(string) ([]byte, error) { return []byte("{"), nil }
	_, _, _, err = compileAll(badJSON)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")

	// 合法 JSON 但编译失败（$ref 悬空 → packet Compile 失败）。
	badRef := func(string) ([]byte, error) {
		return []byte(`{"$ref": "https://flywheel.local/schemas/missing"}`), nil
	}
	_, _, _, err = compileAll(badRef)
	assert.Error(t, err)

	// 定向覆盖 spec / state 两个 Compile 失败分支（其余 schema 正常，
	// metaschema 违规在 Compile 期暴露）。
	breakOne := func(broken string) func(string) ([]byte, error) {
		return func(name string) ([]byte, error) {
			if name == broken {
				return []byte(`{"type": true}`), nil // boolean type 非法（want array）
			}
			return templates.FS.ReadFile(name)
		}
	}
	_, _, _, err = compileAll(breakOne("spec.schema.json"))
	assert.Error(t, err)
	_, _, _, err = compileAll(breakOne("state.schema.json"))
	assert.Error(t, err)
}

func TestCompileSchemasErrorPropagation(t *testing.T) {
	// 编译失败经 sync.Once 缓存后向 StateSchema / ValidateStateValue 传播；
	// 结束时恢复注入点并重置 Once，让后续真实编译重新发生。
	origFn := compileAllFn
	defer func() {
		compileAllFn = origFn
		schemaOnce = sync.Once{}
	}()
	compileAllFn = func(func(string) ([]byte, error)) (*jsonschema.Schema, *jsonschema.Schema, *jsonschema.Schema, error) {
		return nil, nil, nil, errors.New("boom")
	}
	schemaOnce = sync.Once{}

	_, err := StateSchema()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")

	err = ValidateStateValue(map[string]any{"revision": 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
	err = ValidateWorkflowStateValue(map[string]any{"version": 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")

	// 编译失败同样从两个解析入口传播。
	_, err = ParsePacket([]byte(validPacket), "p.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
	_, err = ParseSpec([]byte(validSpec), "spec.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestParseEntriesFrontmatterPassthrough(t *testing.T) {
	// 坏 frontmatter 从解析入口直通报错（file 上下文保留）。
	bad := []byte("# no frontmatter at all\n")
	_, err := ParsePacket(bad, "p.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "p.md")

	_, err = ParseSpec(bad, "s.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "s.md")
}

func TestLoadPacketsReadFileError(t *testing.T) {
	// 悬空 symlink：ReadDir 枚举到名字（非目录）但 ReadFile 失败。
	dir := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "task-packet-M01-T01.md")))
	_, err := LoadPackets(dir, nil)
	require.Error(t, err)
}
