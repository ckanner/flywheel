package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
)

func packet(id, route string) *contract.Packet {
	return &contract.Packet{PacketID: id, Route: route}
}

func writeRaw(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestStateFileAndExists(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, filepath.Join(dir, "state.json"), StateFile(dir))
	assert.False(t, Exists(dir))
	writeRaw(t, StateFile(dir), "{}")
	assert.True(t, Exists(dir))
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	writeRaw(t, StateFile(dir), "{broken")
	_, err = Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")

	writeRaw(t, StateFile(dir), `{"slug":"x","status":"pending"}`)
	_, err = Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing integer revision")

	writeRaw(t, StateFile(dir), `{"revision":"x"}`)
	_, err = Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing integer revision")

	writeRaw(t, StateFile(dir), `{"revision":1.5}`)
	_, err = Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing integer revision")
}

func TestLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	raw := `{"schema-version":1,"slug":"t","revision":3,"status":"running","packets":{},"inbox":[]}`
	writeRaw(t, StateFile(dir), raw)
	st, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, int64(3), st.Revision)
	assert.Equal(t, "t", st.Slug)
}

func TestSaveFirstWrite(t *testing.T) {
	dir := t.TempDir()
	st := &State{Slug: "t", Revision: 0, Packets: map[string]*PacketState{}}
	next, err := Save(dir, st, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), next.Revision)

	// 落盘格式：缩进 2 + 尾换行；tmp 已清理。
	data, err := os.ReadFile(StateFile(dir))
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(string(data), "}\n"))
	assert.Contains(t, string(data), "\n  \"slug\"")
	entries, rerr := os.ReadDir(dir)
	require.NoError(t, rerr)
	assert.Len(t, entries, 1)

	reloaded, lerr := Load(dir)
	require.NoError(t, lerr)
	assert.Equal(t, int64(1), reloaded.Revision)
}

func TestSaveCASConflict(t *testing.T) {
	dir := t.TempDir()
	base := &State{Slug: "t", Revision: 5, Packets: map[string]*PacketState{}}
	_, err := Save(dir, base, nil)
	require.NoError(t, err)

	// 陈旧副本（仍以为 revision=5）再写 → 冲突（默认期望 = st.Revision）。
	stale := &State{Slug: "t", Revision: 5, Status: "stale", Packets: map[string]*PacketState{}}
	_, err = Save(dir, stale, nil)
	var cas *CasConflictError
	require.ErrorAs(t, err, &cas)
	assert.Equal(t, int64(5), cas.Expected)
	assert.Equal(t, int64(6), cas.Found)

	// 显式 expectedRevision。
	fresh, lerr := Load(dir)
	require.NoError(t, lerr)
	exp := fresh.Revision + 10
	_, err = Save(dir, fresh, &exp)
	require.ErrorAs(t, err, &cas)
	assert.Equal(t, exp, cas.Expected)
	assert.Contains(t, err.Error(), "CAS conflict")

	// 正确期望值 → 写入成功并推进。
	okRev := fresh.Revision
	next, serr := Save(dir, func() *State { s := *fresh; return &s }(), &okRev)
	require.NoError(t, serr)
	assert.Equal(t, okRev+1, next.Revision)
}

func TestSaveCASMissingRevisionKey(t *testing.T) {
	dir := t.TempDir()
	// 磁盘文件缺 revision 键 → 无法建立期望（Node: undefined !== n 恒冲突，
	// Go 以 found=0 表达缺失）。
	writeRaw(t, StateFile(dir), `{"slug":"t"}`)
	st := &State{Revision: 0, Packets: map[string]*PacketState{}}
	_, err := Save(dir, st, nil)
	var cas *CasConflictError
	require.ErrorAs(t, err, &cas)
	assert.Equal(t, int64(0), cas.Found)
}

func TestSaveUnreadableDuringCAS(t *testing.T) {
	dir := t.TempDir()
	writeRaw(t, StateFile(dir), "{nope")
	_, err := Save(dir, &State{Revision: 0, Packets: map[string]*PacketState{}}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unreadable during CAS check")
}

func TestSaveWriteFailureReadOnlyDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	ro := filepath.Join(dir, "ro")
	require.NoError(t, os.Mkdir(ro, 0o555))
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	_, err := Save(ro, &State{Revision: 0, Packets: map[string]*PacketState{}}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write failed")
}

func TestSaveRenameFailureInjected(t *testing.T) {
	orig := osRename
	t.Cleanup(func() { osRename = orig })
	osRename = func(string, string) error { return errors.New("boom") }
	_, err := Save(t.TempDir(), &State{Revision: 0, Packets: map[string]*PacketState{}}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rename failed")
}

func TestEmptyPacketState(t *testing.T) {
	ps, err := EmptyPacketState(packet("M02-T03", "default"))
	require.NoError(t, err)
	assert.Equal(t, "M-02", ps.Milestone)
	assert.Equal(t, "pending", ps.Status)
	assert.Equal(t, "default", ps.Route)
	assert.False(t, ps.Interrupted)
	assert.Empty(t, ps.Artifacts)
	assert.Nil(t, ps.Exit)
	assert.NotNil(t, ps.RetryHistory)

	_, err = EmptyPacketState(packet("bad-id", "default"))
	require.Error(t, err)
}

func TestEnsurePacketStates(t *testing.T) {
	st := &State{Packets: map[string]*PacketState{
		"M01-T01": {Status: "done"},
	}}
	require.NoError(t, EnsurePacketStates(st, []*contract.Packet{
		packet("M01-T01", "default"), // 已存在：保留原条目
		packet("M01-T02", "hard"),    // 新增
	}))
	assert.Equal(t, "done", st.Packets["M01-T01"].Status)
	assert.Equal(t, "pending", st.Packets["M01-T02"].Status)
	assert.Equal(t, "hard", st.Packets["M01-T02"].Route)

	err := EnsurePacketStates(st, []*contract.Packet{packet("nope", "default")})
	require.Error(t, err)
}

func TestInitState(t *testing.T) {
	spec := &contract.Spec{
		Milestones: []contract.Milestone{{ID: "M-01"}, {ID: "M-02"}},
	}
	packets := []*contract.Packet{packet("M01-T01", "default"), packet("M02-T01", "frontend")}
	st, err := Init("mytask", spec, packets, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, st.SchemaVersion)
	assert.Equal(t, "mytask", st.Slug)
	assert.Equal(t, int64(0), st.Revision)
	assert.Equal(t, "pending", st.Status)
	require.NotNil(t, st.CurrentMilestone)
	assert.Equal(t, "M-01", *st.CurrentMilestone)
	assert.Len(t, st.Milestones, 2)
	assert.Equal(t, "pending", st.Milestones["M-02"].Status)
	assert.Len(t, st.Packets, 2)
	assert.Equal(t, "soft", st.Budget.Enforce)
	assert.Nil(t, st.Budget.DailyTokenLimit)
	assert.Empty(t, st.Inbox)

	// config 提供 enforce 与限额。
	cfg := config.Default()
	cfg.Budget.Enforce = "hard"
	limit := int64(123456)
	cfg.Budget.DailyTokenLimit = &limit
	st2, err := Init("t2", spec, packets, &cfg)
	require.NoError(t, err)
	assert.Equal(t, "hard", st2.Budget.Enforce)
	require.NotNil(t, st2.Budget.DailyTokenLimit)
	assert.Equal(t, limit, *st2.Budget.DailyTokenLimit)

	// schema 校验（initState 形状与 state.schema.json lockstep）。
	val, merr := json.Marshal(st2)
	require.NoError(t, merr)
	var v any
	require.NoError(t, json.Unmarshal(val, &v))
	assert.NoError(t, contract.ValidateStateValue(v))
}

func TestMarkInterruptedUnits(t *testing.T) {
	now := "2026-08-22T00:00:00.000Z"
	st := &State{Packets: map[string]*PacketState{
		"M01-T01": {Status: "executing"},
		"M01-T02": {Status: "pending"},
		"M01-T03": {Status: "l1-failed"},
	}}
	affected := MarkInterruptedUnits(st, now)
	assert.ElementsMatch(t, []string{"M01-T01"}, affected)
	assert.Equal(t, "interrupted", st.Packets["M01-T01"].Status)
	assert.True(t, st.Packets["M01-T01"].Interrupted)
	require.NotNil(t, st.Packets["M01-T01"].UpdatedAt)
	assert.Equal(t, "pending", st.Packets["M01-T02"].Status)
}

func TestRecordUsageBasics(t *testing.T) {
	st := &State{Budget: Budget{Daily: map[string]DailyBucket{}, Cache: CacheStats{ByProvider: map[string]ProviderCache{}}}}
	total := int64(150)
	cost := 0.25
	RecordUsage(st, Usage{Input: 100, Output: 40, Reasoning: 10, CacheRead: 60, CacheWrite: 20, TotalTokens: &total},
		UsageOpts{Provider: "opencode-go", CostUSD: &cost, Day: "2026-08-22"})

	assert.Equal(t, int64(150), st.Budget.Totals.Tokens)
	assert.InDelta(t, 0.25, st.Budget.Totals.CostUSD, 1e-9)
	assert.Equal(t, int64(100), st.Budget.Totals.Input)
	assert.Equal(t, int64(40), st.Budget.Totals.Output)
	assert.Equal(t, int64(10), st.Budget.Totals.Reasoning)
	assert.Equal(t, int64(60), st.Budget.Totals.CacheRead)
	assert.Equal(t, int64(20), st.Budget.Totals.CacheWrite)
	assert.Equal(t, DailyBucket{Tokens: 150, CostUSD: 0.25}, st.Budget.Daily["2026-08-22"])

	bp := st.Budget.Cache.ByProvider["opencode-go"]
	assert.Equal(t, int64(100), bp.Input)
	assert.InDelta(t, 0.6, bp.HitRate, 1e-9) // 60/100 round 3 位
	assert.InDelta(t, 0.6, st.Budget.Cache.HitRate, 1e-9)
}

func TestRecordUsageDefaults(t *testing.T) {
	st := &State{Budget: Budget{Daily: map[string]DailyBucket{}, Cache: CacheStats{ByProvider: map[string]ProviderCache{}}}}
	// totalTokens 缺省 = input+output；cost 缺省 0；provider 缺省 unknown；
	// day 缺省走当前日期分支（断言 key 存在即可）。
	RecordUsage(st, Usage{Input: 30, Output: 12}, UsageOpts{})
	assert.Equal(t, int64(42), st.Budget.Totals.Tokens)
	assert.Equal(t, float64(0), st.Budget.Totals.CostUSD)
	bp := st.Budget.Cache.ByProvider["unknown"]
	assert.Equal(t, int64(30), bp.Input)
	assert.InDelta(t, 0, st.Budget.Cache.HitRate, 1e-9) // 无 cacheRead → 0
	found := false
	for range st.Budget.Daily {
		found = true
	}
	assert.True(t, found, "today bucket created by now() branch")
}

func TestRecordUsageZeroInputHitRate(t *testing.T) {
	st := &State{Budget: Budget{Daily: map[string]DailyBucket{}, Cache: CacheStats{ByProvider: map[string]ProviderCache{}}}}
	RecordUsage(st, Usage{Output: 5}, UsageOpts{Provider: "p", Day: "d"})
	assert.InDelta(t, 0, st.Budget.Cache.HitRate, 1e-9)
	assert.InDelta(t, 0, st.Budget.Cache.ByProvider["p"].HitRate, 1e-9)
}

func TestRecordUsageCostRounding(t *testing.T) {
	st := &State{Budget: Budget{Daily: map[string]DailyBucket{}, Cache: CacheStats{ByProvider: map[string]ProviderCache{}}}}
	c := 0.1
	for i := 0; i < 3; i++ {
		RecordUsage(st, Usage{}, UsageOpts{CostUSD: &c, Day: "d"})
	}
	// 0.1*3 浮点误差被 1e8 四舍五入吸收。
	assert.InDelta(t, 0.30000000, st.Budget.Totals.CostUSD, 1e-9)
}

func TestRecordUsagePrunesOldDays(t *testing.T) {
	st := &State{Budget: Budget{Daily: map[string]DailyBucket{}, Cache: CacheStats{ByProvider: map[string]ProviderCache{}}}}
	for i := range 35 {
		day := fmt.Sprintf("d%02d", i) // 字典序 = 插入序，最老的先被剪
		RecordUsage(st, Usage{}, UsageOpts{Day: day})
	}
	assert.LessOrEqual(t, len(st.Budget.Daily), 31)
	assert.NotContains(t, st.Budget.Daily, "d00")
	assert.NotContains(t, st.Budget.Daily, "d01")
	assert.NotContains(t, st.Budget.Daily, "d02")
	assert.NotContains(t, st.Budget.Daily, "d03")
	assert.Contains(t, st.Budget.Daily, "d34")
}

func TestNowISOFormat(t *testing.T) {
	// Node toISOString 等价：2026-08-22T12:34:56.789Z
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`, NowISO())
}

func TestLoadStructMismatchAfterProbe(t *testing.T) {
	// revision 合法但其余字段类型不符 → struct 解析失败分支。
	dir := t.TempDir()
	writeRaw(t, StateFile(dir), `{"revision":1,"packets":[]}`)
	_, err := Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestSaveCASReadFailureInjected(t *testing.T) {
	orig := osReadFile
	t.Cleanup(func() { osReadFile = orig })
	osReadFile = func(string) ([]byte, error) { return nil, errors.New("race lost") }
	dir := t.TempDir()
	writeRaw(t, StateFile(dir), "{}")
	_, err := Save(dir, &State{Revision: 0, Packets: map[string]*PacketState{}}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unreadable during CAS check")
}

func TestInitBadPacketIDPropagates(t *testing.T) {
	spec := &contract.Spec{Milestones: []contract.Milestone{{ID: "M-01"}}}
	_, err := Init("s", spec, []*contract.Packet{packet("bad", "default")}, nil)
	require.Error(t, err)
}
