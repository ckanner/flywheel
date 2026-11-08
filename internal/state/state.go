// Package state — state.json 管理（docs/design.md §5.2）：唯一信源，只由编排器写入
// ——被监督者永不写监督者的账本。每次写入对 revision 做 compare-and-set，
// 原子落盘（tmp+rename）。重启后进行中的单元显式标记 interrupted 且保留产物
// ——绝不静默重置为 pending。
package state

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
)

// CasConflictError CAS 冲突：磁盘 revision 与调用方决策依据不一致。
type CasConflictError struct {
	File     string
	Expected int64
	Found    int64
}

func (e *CasConflictError) Error() string {
	return fmt.Sprintf("CAS conflict on %s: expected revision %d, found %d", e.File, e.Expected, e.Found)
}

// StateError 状态层错误（缺失/损坏的 state.json）。
type StateError struct{ Msg string }

func (e *StateError) Error() string { return e.Msg }

func errf(format string, args ...any) error { return &StateError{Msg: fmt.Sprintf(format, args...)} }

// NowISO Node toISOString 等价（UTC 毫秒精度）。
func NowISO() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }

// ---- 类型化结构（与 Node 版 JSON 逐字兼容，kebab-case tags）----

// ExitInfo 出口契约正交量（§6）：exitCode / signal / timedOut，外加双计时
// 看门狗的细分事实（idleTimedOut / wallTimedOut——machine 逐字记录 Node 版
// ps.exit 的五个键，state.schema.json 同样声明了它们）。
// "超时被 trap 成 exit 0"不得读成绿灯。
type ExitInfo struct {
	Settled      bool    `json:"settled"`
	Cancelled    bool    `json:"cancelled"`
	Truncated    bool    `json:"truncated"`
	CleanupError string  `json:"cleanupError,omitempty"`
	ExitCode     *int    `json:"exitCode"`
	Signal       *string `json:"signal"`
	TimedOut     bool    `json:"timedOut"`
	IdleTimedOut bool    `json:"idleTimedOut"`
	WallTimedOut bool    `json:"wallTimedOut"`
}

// Evidence 完成状态证据来源记录（§5.2：L1 通过 / Review 通过 / executor 自报，
// 三者证据等级不同）。内容为 machine 层语义对象，此处仅保证 null 初始化与
// JSON 往返；credentials 由 machine 在凭证审批后动态出现。
type Evidence struct {
	Executor        any `json:"executor"`
	L1              any `json:"l1"`
	Review          any `json:"review"`
	DefectInjection any `json:"defect-injection,omitempty"`
	Credentials     any `json:"credentials,omitempty"`
	FinalGate       any `json:"final-gate,omitempty"`
}

// WorktreeInfo 单元 worktree 信息（verify.ensureUnitWorktree 写入）。
type WorktreeInfo struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Mode   string `json:"mode"` // worktree | in-place
	Reason string `json:"reason,omitempty"`
}

// osWriteFile 可替换注入点（写失败分支在真实环境不可稳定复现）。
var osWriteFile = os.WriteFile

// osRename 可替换注入点（POSIX 下同目录 rename 实际不失败，测试注入覆盖）。
var osRename = os.Rename

// osReadFile 可替换注入点（CAS 读盘竞态不可稳定复现，测试注入覆盖）。
var osReadFile = os.ReadFile

// PacketState 单个 packet 的执行状态。
type PacketState struct {
	Milestone            string    `json:"milestone"`
	Status               string    `json:"status"`
	Route                string    `json:"route"`
	Attempts             int       `json:"attempts"`
	FailedAttempts       int       `json:"failed-attempts"`
	PlanRevisions        int       `json:"plan-revisions"`
	Interrupted          bool      `json:"interrupted"`
	NeedsLiveCredentials bool      `json:"needs-live-credentials"`
	Artifacts            []string  `json:"artifacts"`
	Exit                 *ExitInfo `json:"exit"`
	BaseRef              *string   `json:"base-ref"`
	ContractSyncPending  bool      `json:"contract-sync-pending,omitempty"`
	// Commit 编排器单元提交记录：对象 {sha,message,at,ledger-sha?,merge-error?}
	// 或 {sha,message,at} / {skipped|error,...}（in-place 分支直接落 CommitOutcome
	// 形状）。state.schema.json 声明为对象——machine 层写入，此处仅保证 JSON 往返。
	Commit               any           `json:"commit"`
	Worktree             *WorktreeInfo `json:"worktree"`
	Evidence             Evidence      `json:"evidence"`
	LastFailureSignature *string       `json:"last-failure-signature"`
	LastFeedback         any           `json:"last-feedback,omitempty"`
	EvidenceHistory      []any         `json:"evidence-history,omitempty"`
	RetryHistory         []any         `json:"retry-history"`
	UpdatedAt            *string       `json:"updated-at"`
}

// MilestoneState 里程碑推进状态。
type MilestoneState struct {
	Status string `json:"status"`
	// Gate 门禁结论对象 {pass,levels,at,judge?}（§4.4/§7；schema 声明对象）。
	Gate     any     `json:"gate"`
	ClosedAt *string `json:"closed-at"`
}

// BudgetTotals 预算累计。
type BudgetTotals struct {
	Input      int64   `json:"input"`
	Output     int64   `json:"output"`
	Reasoning  int64   `json:"reasoning"`
	CacheRead  int64   `json:"cache-read"`
	CacheWrite int64   `json:"cache-write"`
	Tokens     int64   `json:"tokens"`
	CostUSD    float64 `json:"cost-usd"`
}

// DailyBucket 单日用量。
type DailyBucket struct {
	Tokens            int64   `json:"tokens"`
	CostUSD           float64 `json:"cost-usd"`
	UnknownUsageCalls int64   `json:"unknown-usage-calls,omitempty"`
	UnknownCostCalls  int64   `json:"unknown-cost-calls,omitempty"`
}

// ProviderCache 单 provider 缓存统计（铁律 10：逐 provider 命中率）。
type ProviderCache struct {
	Input      int64   `json:"input"`
	CacheRead  int64   `json:"cache-read"`
	CacheWrite int64   `json:"cache-write"`
	HitRate    float64 `json:"hit-rate"`
}

// CacheStats 全局缓存健康信号（突降 = 前缀被破坏）。
type CacheStats struct {
	HitRate    float64                  `json:"hit-rate"`
	ByProvider map[string]ProviderCache `json:"by-provider"`
}

// Budget 预算熔断 + 用量记账（铁律 9：记录永不关闭）。
type Budget struct {
	Enforce           string                 `json:"enforce"` // off | soft | hard
	DailyTokenLimit   *int64                 `json:"daily-token-limit"`
	Daily             map[string]DailyBucket `json:"daily"`
	Totals            BudgetTotals           `json:"totals"`
	Cache             CacheStats             `json:"cache"`
	Warnings          []string               `json:"warnings"`
	UnknownUsageCalls int64                  `json:"unknown-usage-calls,omitempty"`
	UnknownCostCalls  int64                  `json:"unknown-cost-calls,omitempty"`
}

// State state.json 根对象。
type State struct {
	SchemaVersion        int                       `json:"schema-version"`
	Slug                 string                    `json:"slug"`
	Revision             int64                     `json:"revision"`
	Status               string                    `json:"status"`
	CurrentMilestone     *string                   `json:"current-milestone"`
	PlanSession          any                       `json:"plan-session"`
	ContractFingerprints map[string]string         `json:"contract-fingerprints,omitempty"`
	Milestones           map[string]MilestoneState `json:"milestones"`
	Packets              map[string]*PacketState   `json:"packets"`
	Budget               Budget                    `json:"budget"`
	LastProgressAt       *string                   `json:"last-progress-at"`
	Inbox                []any                     `json:"inbox"`
	Workflow             *WorkflowState            `json:"workflow,omitempty"`
	Sessions             *SessionLedger            `json:"sessions,omitempty"`
}

// WorkflowState persists the selected policy before any model call. An upgrade
// is a recoverable checkpoint; budgets, packet workspaces and history stay owned
// by the same task.
type WorkflowState struct {
	contract.WorkflowSelection
	SelectedAt string            `json:"selected-at"`
	Upgrades   []WorkflowUpgrade `json:"upgrades"`
}

type WorkflowUpgrade struct {
	At                   string            `json:"at"`
	From                 string            `json:"from"`
	To                   string            `json:"to"`
	Reason               string            `json:"reason"`
	Evidence             []string          `json:"evidence"`
	ContractFingerprints map[string]string `json:"contract-fingerprints"`
}

// Usage 单次会话用量（driver 上报；totalTokens 缺省 = input+output）。
type Usage struct {
	Input       int64  `json:"input"`
	Output      int64  `json:"output"`
	Reasoning   int64  `json:"reasoning"`
	CacheRead   int64  `json:"cacheRead"`
	CacheWrite  int64  `json:"cacheWrite"`
	TotalTokens *int64 `json:"totalTokens,omitempty"`
}

// UsageOpts RecordUsage 可选项（provider 默认 unknown；day 注入便于测试）。
type UsageOpts struct {
	Provider string
	CostUSD  *float64
	Day      string
}

// ---- 文件操作 ----

// StateFile taskDir 下 state.json 路径。
func StateFile(taskDir string) string { return filepath.Join(taskDir, "state.json") }

// Exists state.json 是否存在。
func Exists(taskDir string) bool {
	_, err := os.Stat(StateFile(taskDir))
	return err == nil
}

// Load 读并校验 state.json（revision 必须是整数——JS Number.isInteger 语义）。
func Load(taskDir string) (*State, error) {
	file := StateFile(taskDir)
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, errf("state.json not found in %s", taskDir)
	}
	var probe struct {
		Revision any             `json:"revision"`
		Workflow json.RawMessage `json:"workflow"`
		Sessions json.RawMessage `json:"sessions"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, errf("%s: invalid JSON: %v", file, err)
	}
	if r, ok := probe.Revision.(float64); !ok || r != math.Trunc(r) {
		return nil, errf("%s: missing integer revision", file)
	}
	if len(probe.Workflow) > 0 {
		var workflow any
		_ = json.Unmarshal(probe.Workflow, &workflow) // the complete JSON parsed above
		if err := contract.ValidateWorkflowStateValue(workflow); err != nil {
			return nil, err
		}
	}
	if len(probe.Sessions) > 0 {
		if err := validateSessions(probe.Sessions); err != nil {
			return nil, err
		}
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, errf("%s: invalid JSON: %v", file, err)
	}
	return &st, nil
}

// sysFlock 可替换注入点（Flock 失败分支真实环境不可稳定复现）。
var sysFlock = func(fd uintptr) error { return syscall.Flock(int(fd), syscall.LOCK_EX) }

// lockStateFile CAS 写期间的进程间互斥（C9，v0.18：闭 CAS 读-写 TOCTOU
// 窗口——编排器与 intake/garden 模块的 pid 守卫之间存在毫秒级缝隙）。
// 阻塞式 LOCK_EX：持锁方都是短临界区。可注入供测试覆盖错误分支。
var lockStateFile = func(taskDir string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(taskDir, ".state.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := sysFlock(f.Fd()); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// Save 原子 CAS 写：锁内重读磁盘 revision，拒覆盖基于陈旧状态做出的决策。
// expectedRevision 为 nil 时以 st.Revision 为期望值。返回 revision+1 的新状态。
var syncStateFile = (*os.File).Sync

func syncStatePath(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return syncStateFile(f)
}

func Save(taskDir string, st *State, expectedRevision *int64) (*State, error) {
	release, err := lockStateFile(taskDir)
	if err != nil {
		return nil, errf("%s: lock failed: %v", taskDir, err)
	}
	defer release()

	file := StateFile(taskDir)
	if _, err := os.Stat(file); err == nil {
		raw, rerr := osReadFile(file)
		if rerr != nil {
			return nil, errf("%s: unreadable during CAS check: %v", file, rerr)
		}
		var onDisk struct {
			Revision *int64 `json:"revision"`
		}
		if jerr := json.Unmarshal(raw, &onDisk); jerr != nil {
			return nil, errf("%s: unreadable during CAS check: %v", file, jerr)
		}
		expected := st.Revision
		if expectedRevision != nil {
			expected = *expectedRevision
		}
		found := int64(0)
		if onDisk.Revision != nil {
			found = *onDisk.Revision
		}
		if onDisk.Revision == nil || found != expected {
			return nil, &CasConflictError{File: file, Expected: expected, Found: found}
		}
	}
	next := *st
	next.Revision = st.Revision + 1
	data, _ := json.MarshalIndent(next, "", "  ")
	tmp := fmt.Sprintf("%s.tmp-%d", file, os.Getpid())
	if err := osWriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return nil, errf("%s: write failed: %v", tmp, err)
	}
	if err := syncStatePath(tmp); err != nil {
		return nil, errf("%s: fsync failed: %v", tmp, err)
	}
	if err := osRename(tmp, file); err != nil {
		return nil, errf("%s: rename failed: %v", tmp, err)
	}
	if err := syncStatePath(taskDir); err != nil {
		return nil, errf("%s: directory fsync failed: %v", taskDir, err)
	}
	return &next, nil
}

// ---- packet 状态初始化 ----

// EmptyPacketState 新 packet 的初始状态条目。
func EmptyPacketState(packet *contract.Packet) (*PacketState, error) {
	ms, err := contract.MilestoneOfPacket(packet.PacketID)
	if err != nil {
		return nil, err
	}
	return &PacketState{
		Milestone:            ms,
		Status:               "pending",
		Route:                packet.Route,
		Interrupted:          false,
		NeedsLiveCredentials: packet.NeedsLiveCredentials,
		Artifacts:            []string{},
		Exit:                 nil,
		BaseRef:              nil,
		Commit:               nil,
		Worktree:             nil,
		Evidence:             Evidence{},
		RetryHistory:         []any{},
	}, nil
}

// EnsurePacketStates 保证每个已加载 packet 有状态条目。Plan Loop 运行中期可能
// 新增 packet（范围修订）；无状态条目会在 processPacket 崩溃。每次契约重载与
// 上下文创建后调用。
func EnsurePacketStates(st *State, packets []*contract.Packet) error {
	for _, p := range packets {
		if _, ok := st.Packets[p.PacketID]; !ok {
			ps, err := EmptyPacketState(p)
			if err != nil {
				return err
			}
			st.Packets[p.PacketID] = ps
		}
	}
	return nil
}

// Init 构建 state.json 初始内容（$flywheel-new 产出物）。
func Init(slug string, spec *contract.Spec, packets []*contract.Packet, cfg *config.Config) (*State, error) {
	milestones := map[string]MilestoneState{}
	for _, m := range spec.Milestones {
		milestones[m.ID] = MilestoneState{Status: "pending"}
	}
	packetStates := map[string]*PacketState{}
	for _, p := range packets {
		ps, err := EmptyPacketState(p)
		if err != nil {
			return nil, err
		}
		packetStates[p.PacketID] = ps
	}
	var current *string
	if len(spec.Milestones) > 0 {
		current = &spec.Milestones[0].ID
	}
	enforce := "soft"
	dailyLimit := (*int64)(nil)
	if cfg != nil {
		if cfg.Budget.Enforce != "" {
			enforce = cfg.Budget.Enforce
		}
		dailyLimit = cfg.Budget.DailyTokenLimit
	}
	return &State{
		SchemaVersion:    1,
		Slug:             slug,
		Revision:         0,
		Status:           "pending",
		CurrentMilestone: current,
		Milestones:       milestones,
		Packets:          packetStates,
		Budget: Budget{
			Enforce:         enforce,
			DailyTokenLimit: dailyLimit,
			Daily:           map[string]DailyBucket{},
			Cache:           CacheStats{ByProvider: map[string]ProviderCache{}},
			Warnings:        []string{},
		},
		Inbox: []any{},
	}, nil
}

// MarkInterruptedUnits 崩溃恢复（§5.2 v0.8）：重启后发现进行中的单元标记
// interrupted，产物保留。返回受影响的 id 列表。
func MarkInterruptedUnits(st *State, now string) []string {
	var affected []string
	for id, p := range st.Packets {
		if p.Status == "executing" {
			p.Status = "interrupted"
			p.Interrupted = true
			p.UpdatedAt = &now
			affected = append(affected, id)
		}
	}
	sort.Strings(affected)
	return affected
}

// RecordUsage 累计用量+成本到 totals、当日桶与 per-provider 缓存统计。
func RecordUsage(st *State, u Usage, opts UsageOpts) {
	b := &st.Budget
	day := opts.Day
	if day == "" {
		day = time.Now().UTC().Format("2006-01-02")
	}
	if _, ok := b.Daily[day]; !ok {
		b.Daily[day] = DailyBucket{}
	}
	// 日账本只保留最近 31 天（P2：长任务下 state.json 不得无限增长）。
	days := make([]string, 0, len(b.Daily))
	for k := range b.Daily {
		days = append(days, k)
	}
	sort.Strings(days)
	for i := 0; len(b.Daily) > 31; i++ {
		delete(b.Daily, days[i])
	}
	tokens := int64(0)
	if u.TotalTokens != nil {
		tokens = *u.TotalTokens
	} else {
		tokens = u.Input + u.Output
	}
	cost := 0.0
	if opts.CostUSD != nil {
		cost = *opts.CostUSD
	}

	b.Totals.Tokens += tokens
	b.Totals.CostUSD = math.Round((b.Totals.CostUSD+cost)*1e8) / 1e8
	d := b.Daily[day]
	d.Tokens += tokens
	d.CostUSD = math.Round((d.CostUSD+cost)*1e8) / 1e8
	b.Daily[day] = d
	b.Totals.Input += u.Input
	b.Totals.Output += u.Output
	b.Totals.Reasoning += u.Reasoning
	b.Totals.CacheRead += u.CacheRead
	b.Totals.CacheWrite += u.CacheWrite

	provider := opts.Provider
	if provider == "" {
		provider = "unknown"
	}
	bp, ok := b.Cache.ByProvider[provider]
	if !ok {
		bp = ProviderCache{}
	}
	bp.Input += u.Input
	bp.CacheRead += u.CacheRead
	bp.CacheWrite += u.CacheWrite
	if bp.Input > 0 {
		bp.HitRate = math.Round(float64(bp.CacheRead)/float64(bp.Input)*1000) / 1000
	} else {
		bp.HitRate = 0
	}
	b.Cache.ByProvider[provider] = bp
	if b.Totals.Input > 0 {
		b.Cache.HitRate = math.Round(float64(b.Totals.CacheRead)/float64(b.Totals.Input)*1000) / 1000
	} else {
		b.Cache.HitRate = 0
	}
}
