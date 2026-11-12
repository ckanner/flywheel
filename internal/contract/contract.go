// Package contract — 契约文件解析与加载期校验（docs/design.md §5，铁律 6）。
// spec.md / task-packet-<id>.md 的 YAML frontmatter 即机器可读契约。
// 校验 = 前端解析 + JSON Schema（与 templates/*.schema.json 单一信源，
// additionalProperties:false） + 跨字段规则（需求映射、重复 id）。
// 坏契约在 LOAD 时 fail-fast，绝不让 spawn 后中途崩溃（P1）。
package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/ckanner/flywheel/templates"
)

// Error 契约错误（文件 + 原因）。
type Error struct {
	File string
	Msg  string
}

func (e *Error) Error() string {
	if e.File == "" {
		return e.Msg
	}
	return e.File + ": " + e.Msg
}

func errf(file, format string, args ...any) error {
	return &Error{File: file, Msg: fmt.Sprintf(format, args...)}
}

// ---- 类型化契约结构（§5.1） ----

// GateEntry 一条验收命令（cmd 或 script 二选一 + expect）。
type GateEntry struct {
	Cmd    string `json:"cmd"`
	Script string `json:"script"`
	Expect string `json:"expect"`
}

// Command 返回实际要执行的命令（cmd ?? script）。
func (g GateEntry) Command() string {
	if g.Cmd != "" {
		return g.Cmd
	}
	return g.Script
}

// Acceptance 三层确定性验收（§4.4）。
type Acceptance struct {
	L1 []GateEntry `json:"L1"`
	L2 []GateEntry `json:"L2"`
	L3 []GateEntry `json:"L3"`
}

// Level 取某层命令列表。
func (a Acceptance) Level(level string) []GateEntry {
	switch level {
	case "L1":
		return a.L1
	case "L2":
		return a.L2
	case "L3":
		return a.L3
	}
	return nil
}

// Boundaries 防改做边界（§5.1）。
type Boundaries struct {
	MustNotTouch []string `json:"must-not-touch"`
	MaxFiles     *int     `json:"max-files"`
}

// Escalation 升级路径（§5.1）：max-retries + on-fail。
type Escalation struct {
	MaxRetries int    `json:"max-retries"`
	OnFail     string `json:"on-fail"`
}

// Packet 一个 task packet 的完整契约。
type PacketSchedule struct {
	After  []string `json:"after"`
	Writes []string `json:"writes"`
}

type Packet struct {
	Schedule             *PacketSchedule     `json:"schedule,omitempty"`
	PacketID             string              `json:"packet-id"`
	ContractVersion      int                 `json:"contract-version"`
	Goal                 string              `json:"goal"`
	Requirements         []string            `json:"requirements"`
	Route                string              `json:"route"`
	NeedsLiveCredentials bool                `json:"needs-live-credentials"`
	ContextFiles         []string            `json:"context-files"`
	CacheScope           []string            `json:"cache-scope"`
	Boundaries           Boundaries          `json:"boundaries"`
	Acceptance           Acceptance          `json:"acceptance"`
	ReviewRubric         string              `json:"review-rubric"`
	Escalation           Escalation          `json:"escalation"`
	DoneReportFormat     map[string][]string `json:"done-report-format"`

	File string `json:"-"`
	Body string `json:"-"`
}

// Milestone 里程碑（spec.md）。
type Milestone struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Gate  string `json:"gate"`
}

// Requirement 编号需求（R-01…），映射到里程碑。
type Requirement struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Milestone    string `json:"milestone"`
	Verification string `json:"verification"`
}

// Spec 完整 spec.md 契约。
type Spec struct {
	SpecVersion  int           `json:"spec-version"`
	Title        string        `json:"title"`
	Goal         string        `json:"goal"`
	NonGoals     []string      `json:"non-goals"`
	Milestones   []Milestone   `json:"milestones"`
	Requirements []Requirement `json:"requirements"`
	Workflow     *Workflow     `json:"workflow,omitempty"`

	// 派生集合（交叉校验缓存）。
	ReqIDs       map[string]bool
	MilestoneIDs map[string]bool
}

// Slug 任务 slug。Node 语义是 `spec.slug ?? basename(taskDir)`；但 Go 版
// spec.schema.json 为 additionalProperties:false 且契约字段集（docs/design.md §5.3）
// 无 slug，Node 的自由 slug 字段在此被 schema 拒绝——故恒返回 taskDir 尾段。
// 行为差异注释：Node 允许 spec frontmatter 携带自由 slug 字段，Go 收紧。
func (s *Spec) Slug(taskDir string) string {
	return filepath.Base(taskDir)
}

// ---- frontmatter 解析 ----

// splitFrontmatter 切出 YAML frontmatter（首行 `---`，首个独立的 `---` 行收尾）。
// 等价 Node 惰性正则 `^---\r?\n([\s\S]*?)\r?\n---`（RE2 无惰性量词，手写扫描）。
func splitFrontmatter(content []byte) (fm, body string, ok bool) {
	var rest []byte
	switch {
	case bytes.HasPrefix(content, []byte("---\r\n")):
		rest = content[5:]
	case bytes.HasPrefix(content, []byte("---\n")):
		rest = content[4:]
	default:
		return "", "", false
	}
	// 逐行扫描到独立的 `---` 行。
	for {
		idx := bytes.IndexByte(rest, '\n')
		if idx < 0 {
			return "", "", false
		}
		line := strings.TrimSuffix(string(rest[:idx]), "\r")
		if line == "---" {
			return string(fm), string(rest[idx+1:]), true
		}
		fm += string(rest[:idx+1])
		rest = rest[idx+1:]
	}
}

// ParseFrontmatter 解析 YAML frontmatter 为 map。
func ParseFrontmatter(content []byte, file string) (map[string]any, string, error) {
	fm, body, ok := splitFrontmatter(content)
	if !ok {
		return nil, "", errf(file, "missing YAML frontmatter")
	}
	var data map[string]any
	if err := yaml.Unmarshal([]byte(fm), &data); err != nil {
		return nil, "", errf(file, "frontmatter is not valid YAML: %v", err)
	}
	if data == nil {
		return nil, "", errf(file, "frontmatter must be a mapping")
	}
	return data, body, nil
}

// ---- JSON Schema 校验 ----

var (
	schemaOnce   sync.Once
	packetSchema *jsonschema.Schema
	specSchema   *jsonschema.Schema
	stateSchema  *jsonschema.Schema
	schemaErr    error
)

const (
	packetSchemaID = "https://flywheel.local/schemas/task-packet.schema.json"
	specSchemaID   = "https://flywheel.local/schemas/spec.schema.json"
	stateSchemaID  = "https://flywheel.local/schemas/state.schema.json"
)

// compileAll 编译三个内嵌 schema。load 注入字节来源，测试可覆盖全部错误分支
// （embed FS 下读失败/坏 JSON/编译失败均不可达，可测性要求注入）。
// 返回 (packet, spec, state) 三个编译结果。
func compileAll(load func(string) ([]byte, error)) (*jsonschema.Schema, *jsonschema.Schema, *jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	for name, id := range map[string]string{
		"task-packet.schema.json":      packetSchemaID,
		"spec.schema.json":             specSchemaID,
		"state.schema.json":            stateSchemaID,
		"scheduler-policy.schema.json": "https://flywheel.local/schemas/scheduler-policy.schema.json",
	} {
		data, err := load(name)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("embedded schema %s: %w", name, err)
		}
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, nil, nil, fmt.Errorf("embedded schema %s: invalid JSON: %w", name, err)
		}
		// AddResource 失败仅发生在同 URL 重复注册（jsonschema/v6 实测）；
		// 这些 id 恒互异，此错误不可能发生。
		_ = compiler.AddResource(id, doc)
	}
	packetSchema, err := compiler.Compile(packetSchemaID)
	if err != nil {
		return nil, nil, nil, err
	}
	specSchema, err := compiler.Compile(specSchemaID)
	if err != nil {
		return nil, nil, nil, err
	}
	stateSchema, err := compiler.Compile(stateSchemaID)
	if err != nil {
		return nil, nil, nil, err
	}
	return packetSchema, specSchema, stateSchema, nil
}

// compileAllFn 可替换注入点（测试覆盖编译失败在 Once 缓存路径上的传播）。
var compileAllFn = compileAll

// compileSchemas 惰性编译内嵌 schema（编译一次复用）。
func compileSchemas() error {
	schemaOnce.Do(func() {
		packetSchema, specSchema, stateSchema, schemaErr = compileAllFn(templates.FS.ReadFile)
	})
	return schemaErr
}

// flattenErrors 展平 OutputUnit 树中所有 Error 单元（BasicOutput 语义：
// 每个失败单元都携带 Error；Valid 单元无 Error 不产出）。
func flattenErrors(root *jsonschema.OutputUnit) []string {
	var out []string
	var walk func(u *jsonschema.OutputUnit)
	walk = func(u *jsonschema.OutputUnit) {
		if u.Error != nil {
			out = append(out, fmt.Sprintf("%s: %s", u.InstanceLocation, u.Error.String()))
		}
		for i := range u.Errors {
			walk(&u.Errors[i])
		}
	}
	walk(root)
	return out
}

// schemaViolations 展平 BasicOutput 为可读行（instance path: message）。
func schemaViolations(err error) []string {
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return []string{err.Error()}
	}
	if ve == nil {
		return nil
	}
	root := ve.BasicOutput()
	// BasicOutput 的 flatten 树必含 Error 单元（jsonschema/v6 output() 不变式，
	// 已核实源码）：无 causes 时根必带 Error，有 causes 时每个 cause 必带。
	return flattenErrors(root)
}

// validateAgainst 用命名 schema 校验任意值。
func validateAgainst(sch *jsonschema.Schema, value any, file string) error {
	if err := sch.Validate(value); err != nil {
		lines := schemaViolations(err)
		return errf(file, "schema violation: %s", strings.Join(lines, "; "))
	}
	return nil
}

// ---- 跨字段规则 ----

var packetIDRe = regexp.MustCompile(`^M(\d{2,})-T(\d{2,})$`)

// deriveMilestone 可替换注入点。ParsePacket 的里程碑派生调用与 Node 同款双保险
// （contracts.js:75 milestoneOfPacket 独立于 schema 校验）；schema pattern 已锚定
// packet-id，正常输入下该错误分支不可达，仅防御未来 schema 放宽——测试注入覆盖。
var deriveMilestone = MilestoneOfPacket

// MilestoneOfPacket M02-T03 -> M-02（里程碑 id 派生约定，§5.1）。
func MilestoneOfPacket(packetID string) (string, error) {
	m := packetIDRe.FindStringSubmatch(packetID)
	if m == nil {
		return "", fmt.Errorf("invalid packet id %q (want M<NN>-T<NN>)", packetID)
	}
	return "M-" + m[1], nil
}

var expectRe = regexp.MustCompile(`^exit (\d+)$`)

// ParseExpect 解析 accept 的 expect 字段（加载期校验，坏 expect fail-fast）。
func ParseExpect(expect string) (int, error) {
	m := expectRe.FindStringSubmatch(strings.TrimSpace(expect))
	if m == nil {
		return 0, fmt.Errorf("unsupported expect: %q (want \"exit N\")", expect)
	}
	var code int
	fmt.Sscanf(m[1], "%d", &code)
	return code, nil
}

// ---- 解析入口 ----

// ParsePacket 解析并校验一个 packet 文件内容。
func ParsePacket(content []byte, file string) (*Packet, error) {
	data, body, err := ParseFrontmatter(content, file)
	if err != nil {
		return nil, err
	}
	if err := compileSchemas(); err != nil {
		return nil, errf(file, "%v", err)
	}
	if err := validateAgainst(packetSchema, data, file); err != nil {
		return nil, err
	}
	var p Packet
	raw, _ := json.Marshal(data)
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, errf(file, "schema-valid data failed typed conversion: %v", err)
	}
	p.File = file
	p.Body = strings.TrimSpace(body)
	if _, err := deriveMilestone(p.PacketID); err != nil {
		return nil, errf(file, "%v", err)
	}
	return &p, nil
}

// ParseSpec 解析并校验 spec.md 内容（含跨字段：重复 id、需求→里程碑映射）。
func ParseSpec(content []byte, file string) (*Spec, error) {
	data, _, err := ParseFrontmatter(content, file)
	if err != nil {
		return nil, err
	}
	if err := compileSchemas(); err != nil {
		return nil, errf(file, "%v", err)
	}
	if err := validateAgainst(specSchema, data, file); err != nil {
		return nil, err
	}
	var s Spec
	raw, _ := json.Marshal(data)
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, errf(file, "schema-valid data failed typed conversion: %v", err)
	}
	s.ReqIDs = map[string]bool{}
	s.MilestoneIDs = map[string]bool{}
	for _, m := range s.Milestones {
		s.MilestoneIDs[m.ID] = true
	}
	for _, r := range s.Requirements {
		if s.ReqIDs[r.ID] {
			return nil, errf(file, "duplicate requirement id %s", r.ID)
		}
		s.ReqIDs[r.ID] = true
		if !s.MilestoneIDs[r.Milestone] {
			return nil, errf(file, "requirement %s maps to unknown milestone %s", r.ID, r.Milestone)
		}
	}
	return &s, nil
}

// LoadSpec 从任务目录读 spec.md。
func LoadSpec(taskDir string) (*Spec, error) {
	file := filepath.Join(taskDir, "spec.md")
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, errf(file, "spec.md not found in %s", taskDir)
	}
	return ParseSpec(content, file)
}

// LoadPackets 读全部 task-packet-*.md，按 packet-id 排序；spec 非空时校验
// 每个 packet 的 requirements 都在 spec 中声明（防漏做的源头）。
func LoadPackets(taskDir string, spec *Spec) ([]*Packet, error) {
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return nil, err
	}
	packetRe := regexp.MustCompile(`^task-packet-.*\.md$`)
	var files []string
	for _, e := range entries {
		if !e.IsDir() && packetRe.MatchString(e.Name()) {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, errf(taskDir, "no task-packet-*.md found in %s", taskDir)
	}
	var packets []*Packet
	for _, f := range files {
		content, err := os.ReadFile(filepath.Join(taskDir, f))
		if err != nil {
			return nil, err
		}
		p, err := ParsePacket(content, f)
		if err != nil {
			return nil, err
		}
		packets = append(packets, p)
	}
	sort.Slice(packets, func(i, j int) bool { return packets[i].PacketID < packets[j].PacketID })
	if spec != nil {
		for _, p := range packets {
			for _, r := range p.Requirements {
				if !spec.ReqIDs[r] {
					return nil, errf(p.File, "requirement %s not declared in spec.md", r)
				}
			}
		}
	}
	if err := ValidateSchedules(packets); err != nil {
		return nil, err
	}
	return packets, nil
}

// StateSchema 暴露 state.schema.json（initState 形状自检/测试）。
func StateSchema() (*jsonschema.Schema, error) {
	if err := compileSchemas(); err != nil {
		return nil, err
	}
	return stateSchema, nil
}

// ValidateStateValue 用 state.schema.json 校验任意值（编排器自检/测试）。
func ValidateStateValue(value any) error {
	sch, err := StateSchema()
	if err != nil {
		return err
	}
	return validateAgainst(sch, value, "state.json")
}

// ValidateWorkflowStateValue validates the versioned extension on load without
// changing the historical loader's treatment of unrelated legacy state fields.
func ValidateWorkflowStateValue(value any) error {
	sch, err := StateSchema()
	if err != nil {
		return err
	}
	return validateAgainst(sch.Properties["workflow"], value, "state.json workflow")
}

// ValidateSessionStateValue rejects unknown policy versions and malformed
// lifecycle checkpoints before any backend invocation can be considered.
func ValidateSessionStateValue(value any) error {
	sch, err := StateSchema()
	if err != nil {
		return err
	}
	return validateAgainst(sch.Properties["sessions"], value, "state.json sessions")
}

// ValidateConsoleStateValue validates versioned control extensions without
// imposing newly required fields on unrelated legacy state sections.
func ValidateConsoleStateValue(name string, value any) error {
	if name != "handoff" && name != "delivery" && name != "scheduler" && name != "run" && name != "control" && name != "project" && name != "preparation-source" {
		return fmt.Errorf("unknown console state extension")
	}
	schema, err := StateSchema()
	if err != nil {
		return err
	}
	return validateAgainst(schema.Properties[name], value, "state.json "+name)
}
