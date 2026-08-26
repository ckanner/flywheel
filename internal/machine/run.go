package machine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/circuitbreaker"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/defectinjection"
	"github.com/ckanner/flywheel/internal/donereport"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// L1_FAILED / REVIEW_RETURNED 是 machine 层的复合错误码（不在 errs.AllCodes
// 的 spawn 分类里），Node 版即裸字符串。
var (
	errL1Failed       = errs.ErrorCode("L1_FAILED")
	errReviewReturned = errs.ErrorCode("REVIEW_RETURNED")
)

type childenvLookup = childenv.LookupEnv

// ---- verify 包装（保存失败的 fatal 检查点与 Node 异常传播等价）----

func (c *Context) commitUnitChangesIn(cwd string, packet *contract.Packet, excludePaths []string, baseRef, restrictToPrefix string) verify.CommitOutcome {
	return verify.CommitUnitChanges(cwd, packet, excludePaths, baseRef, restrictToPrefix)
}

func (c *Context) mergeUnitWorktree(branch string) (string, error) {
	return verify.MergeUnitWorktree(c.Cwd, branch)
}

func (c *Context) removeUnitWorktree(branch, path string) {
	verify.RemoveUnitWorktree(c.Cwd, branch, path)
}

// commitTaskDirChanges 契约层提交（planner 修订之后）。
func (c *Context) commitTaskDirChanges(message string) verify.CommitOutcome {
	return verify.CommitTaskDirChanges(c.Cwd, c.taskDirPrefix(), message, c.commitExcludes(), contractFileRe)
}

func (c *Context) markInterruptedUnits() []string {
	return state.MarkInterruptedUnits(c.State, c.nowISO())
}

// ---- 失败 / 升级 ----

// failAttempt 一次尝试失败记账 + 空转检测。返回 "spin"（应立即升级）或
// "failed"。
//
// P2 修复：空转检测的 changedFiles 优先取权威 git diff（本轮 L1 boundary，
// 编排器亲自 diff 的结果），executor 自报只作最后兜底——Node 直接信自报值，
// executor 谎报 changed-files 即可绕过 Jaccard 空转检测。
func (c *Context) failAttempt(packet *contract.Packet, errorCode errs.ErrorCode, failedCommands, authoritativeChangedFiles []string) string {
	id := packet.PacketID
	ps := c.State.Packets[id]
	ps.FailedAttempts++
	c.recordRetry(ps, "attempt", string(errorCode), "requeue")

	if failedCommands == nil {
		failedCommands = []string{}
	}
	changed := authoritativeChangedFiles
	if changed == nil {
		changed = l1BoundaryChangedFiles(ps)
	}
	if changed == nil {
		changed = executorSelfChangedFiles(ps)
	}
	if changed == nil {
		changed = []string{}
	}
	signature := circuitbreaker.Signature([]string{string(errorCode)}, failedCommands, changed)
	spin := circuitbreaker.DetectSpin(derefString(ps.LastFailureSignature), signature, ps.FailedAttempts)
	sig := signature
	ps.LastFailureSignature = &sig
	at := c.nowISO()
	ps.UpdatedAt = &at
	c.save()
	if spin {
		c.Log(fmt.Sprintf("spin detected on %s: consecutive identical failure signatures — escalating (§6)", id))
		return "spin"
	}
	return "failed"
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// l1BoundaryChangedFiles L1 边界证据里的权威 changedFiles（编排器亲自 git diff）。
// 进程内写入时 boundary 是结构体，JSON 往返后是 map——两种形状都认。
// skipped（diff 不可用，如非 git checkout）= 无权威可言 → 返回 nil，
// 让调用方落到 executor 自报兜底。
func l1BoundaryChangedFiles(ps *state.PacketState) []string {
	l1, ok := ps.Evidence.L1.(map[string]any)
	if !ok {
		return nil
	}
	switch b := l1["boundary"].(type) {
	case verify.BoundaryReport:
		if b.Skipped {
			return nil
		}
		return b.ChangedFiles
	case map[string]any:
		if skipped, _ := b["skipped"].(bool); skipped {
			return nil
		}
		list, _ := b["changedFiles"].([]any)
		out := make([]string, 0, len(list))
		for _, f := range list {
			if s, ok := f.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// executorSelfChangedFiles done-report 自报的变更文件（最弱证据等级，仅兜底）。
func executorSelfChangedFiles(ps *state.PacketState) []string {
	ex, ok := ps.Evidence.Executor.(map[string]any)
	if !ok {
		return nil
	}
	list, _ := ex["changed-files"].([]any)
	out := make([]string, 0, len(list))
	for _, f := range list {
		if s, ok := f.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// escalatePacket 重试耗尽后的升级（§4.2/§6）：return-to-planner 修订 packet
// （多给一轮完整循环）；仍在失败 → inbox 叫人并停。
func (c *Context) escalatePacket(packet *contract.Packet) (string, error) {
	id := packet.PacketID
	ps := c.State.Packets[id]
	onFail := packet.Escalation.OnFail

	if onFail == "return-to-planner" && ps.PlanRevisions < 1 {
		milestone, found := c.milestoneByID(ps.Milestone)
		if !found {
			return "", fmt.Errorf("packet %s references unknown milestone %q", id, ps.Milestone)
		}
		plan, err := c.planStep(milestone) // Plan Loop 修订 packet（§4.1）
		if err != nil {
			return "", err
		}
		if !plan.OK {
			c.Log(fmt.Sprintf("planner unavailable (%s) — skipping revision, straight to inbox", plan.ErrorCode))
		} else {
			// P0-2：planner 可能重写了契约文件——重载它们，否则下一轮跑在内存里的
			// 陈旧 packet 上。planner 还可能新增 packet；为它们同步状态条目。
			// A1（§4.1 已知风险）：重载前后 diff acceptance 指纹——planner 弱化
			// 验收必须留痕可查。
			old := c.PacketList
			fresh, err := contract.LoadPackets(c.TaskDir, c.Spec)
			if err != nil {
				return "", err
			}
			c.recordAcceptanceChanges(old, fresh)
			c.PacketList = fresh
			c.Packets = map[string]*contract.Packet{}
			for _, p := range fresh {
				c.Packets[p.PacketID] = p
			}
			// 同 CreateContext：EnsurePacketStates 的唯一错误路径是 id 派生，
			// schema 已锚定 packet-id——不可能失败（覆盖率审计 2026-08-23 删除）。
			_ = state.EnsurePacketStates(c.State, fresh)
			// v0.16：修订是合法契约变更——在主 checkout 提交它（契约层保持已提交），
			// 然后从修订提交重建单元 worktree：executor 面对修订后契约在干净 checkout
			// 里重做，防篡改扫描看到的是修订检查点而非 executor 违规。
			revisionCommit := c.commitTaskDirChanges(fmt.Sprintf("docs(%s): plan revision %d", c.State.Slug, ps.PlanRevisions+1))
			// P2 修复（审计）：base-ref 重锚对 in-place 模式同样生效——Node 只在
			// worktree 分支重锚；原地执行的单元会继续对着陈旧 base 判定边界，把
			// 合法修订读成 executor 违规。
			if revisionCommit.SHA != "" {
				if ps.Worktree != nil && ps.Worktree.Mode == "worktree" {
					c.removeUnitWorktree(ps.Worktree.Branch, ps.Worktree.Path) // 丢弃陈旧 checkout + 分支
					ps.Worktree = nil
				}
				sha := revisionCommit.SHA
				ps.BaseRef = &sha // 锚定到修订检查点
			}
			ps.PlanRevisions++
			ps.FailedAttempts = 0
			ps.LastFailureSignature = nil
			ps.Status = "pending"
			at := c.nowISO()
			ps.UpdatedAt = &at
			lastCode := "REVIEW_RETURNED"
			if n := len(ps.RetryHistory); n > 0 {
				if last, ok := ps.RetryHistory[n-1].(map[string]any); ok {
					if ec, ok := last["error-code"].(string); ok && ec != "" {
						lastCode = ec
					}
				}
			}
			c.recordRetry(ps, "attempt", lastCode, "return-to-planner")
			c.appendRetro(fmt.Sprintf("- %s packet %s: escalated to planner, revision %d", c.nowISO(), id, ps.PlanRevisions))
			c.save()
			return "revised", nil
		}
	}

	ps.Status = "inbox"
	at := c.nowISO()
	ps.UpdatedAt = &at
	entry := map[string]any{
		"at":        c.nowISO(),
		"packet-id": id,
		"reason": fmt.Sprintf("packet %s exhausted retries (%d per round, plan revisions: %d). Human needed.",
			id, packet.Escalation.MaxRetries, ps.PlanRevisions),
	}
	c.State.Inbox = append(c.State.Inbox, entry)
	c.State.Status = "escalated"
	c.appendInbox(entry)
	c.appendRetro(fmt.Sprintf("- %v packet %s: INBOX — %v", entry["at"], id, entry["reason"]))
	c.save()
	return "inbox", nil
}

func (c *Context) milestoneByID(id string) (contract.Milestone, bool) {
	for _, m := range c.Spec.Milestones {
		if m.ID == id {
			return m, true
		}
	}
	return contract.Milestone{}, false
}

func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// jsonNilable 空串映射为 JSON null（Node null 语义）。
func jsonNilable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func commitOutcomeMap(o verify.CommitOutcome) map[string]any {
	m := map[string]any{"at": o.At}
	if o.SHA != "" {
		m["sha"] = o.SHA
	}
	if o.Skipped != "" {
		m["skipped"] = o.Skipped
	}
	if o.Error != "" {
		m["error"] = o.Error
	}
	if o.Message != "" {
		m["message"] = o.Message
	}
	return m
}

// completeUnit 单元完成落盘。
//
// ErrMergeConflict 单元代码无法 ff-merge 进主 checkout——状态绝不说谎：
// 调用方必须把单元路由到 inbox/人工处理，而不是标 completed。
var ErrMergeConflict = errors.New("unit branch could not be merged into the main checkout")

// P2 修复（审计）：先完成提交、再置 completed 并保存——Node 先写 completed
// 再提交，崩溃窗口内 state.json 谎报“已完成”而代码从未落库。
//
// 合并门控（review 修正）：以“分支是否领先 base”为准（verify.AheadCount），
// 而不是“本次 scoped 提交是否产生了新提交”——executor 自提交、或崩溃恢复
// 重跑 completeUnit 时 scoped 提交为空但分支领先，旧条件会静默跳过合并。
// ff-merge 失败时返回 ErrMergeConflict：worktree 原样保留、状态绝不标
// completed（§5.2 state.json 不说谎），由调用方升级到人。
func (c *Context) completeUnit(packet *contract.Packet) error {
	id := packet.PacketID
	ps := c.State.Packets[id]

	var commitNote string
	if ps.Worktree != nil && ps.Worktree.Mode == "worktree" {
		// v0.16 流程：worktree 内提交代码 → ff-merge 进主 checkout →
		// 删 worktree+分支 → 主 checkout 的账本提交（限 docs/<slug>/）。
		codeCommit := c.commitUnitChangesIn(ps.Worktree.Path, packet, nil, "", "")
		// 合并门控（review 修正）：以“分支是否领先 base”为准（AheadCount），
		// 而不是“本次 scoped 提交是否产生了新提交”——executor 自提交、或崩溃
		// 恢复重跑 completeUnit 时 scoped 提交为空但分支领先，旧条件会静默跳过合并。
		ahead, aerr := verify.AheadCount(c.Cwd, c.baseRefOf(ps), ps.Worktree.Branch)
		if aerr != nil {
			return fmt.Errorf("%w: ahead count failed: %v", ErrMergeConflict, aerr)
		}
		if ahead > 0 {
			if _, err := c.mergeUnitWorktree(ps.Worktree.Branch); err != nil {
				// §5.2 state.json 不说谎：合并失败绝不标 completed——worktree
				// 原样保留；先把失败事实落账（供人处理），再上抛升级。
				commitRec := map[string]any{
					"at":          c.nowISO(),
					"merge-error": jsonNilable(err.Error()),
				}
				// 账本照常落库（审计链完整）：主 checkout 的契约/账本文件
				// 仍提交——只是单元不标 completed、不写完成 retro 行。
				if lc := c.commitUnitChangesIn(c.Cwd, packet, c.commitExcludes(), c.baseRefOf(ps), c.taskDirPrefix()); lc.SHA != "" {
					commitRec["ledger-sha"] = jsonNilable(lc.SHA)
					c.Log(fmt.Sprintf("%s: ledger committed despite merge failure (%s)", id, shortSHA(lc.SHA)))
				}
				ps.Commit = commitRec
				at := c.nowISO()
				ps.UpdatedAt = &at
				c.save()
				return fmt.Errorf("%w: %s", ErrMergeConflict, err.Error())
			}
			c.removeUnitWorktree(ps.Worktree.Branch, ps.Worktree.Path)
		} else {
			c.Log(fmt.Sprintf("%s: branch not ahead of base — nothing to merge", id))
			c.removeUnitWorktree(ps.Worktree.Branch, ps.Worktree.Path)
		}
		ledgerCommit := c.commitUnitChangesIn(c.Cwd, packet, c.commitExcludes(), c.baseRefOf(ps), c.taskDirPrefix())
		finalSHA := firstNonEmpty(codeCommit.SHA, ledgerCommit.SHA)
		ps.Commit = map[string]any{
			"sha":        finalSHA,
			"message":    firstNonEmpty(codeCommit.Message, ledgerCommit.Message),
			"at":         c.nowISO(),
			"ledger-sha": jsonNilable(ledgerCommit.SHA),
		}
		if finalSHA == "" {
			reason := firstNonEmpty(codeCommit.Error, codeCommit.Skipped, "unknown")
			commitNote = fmt.Sprintf("commit skipped (%s)", reason)
		} else {
			n := shortSHA(finalSHA)
			if ls := ledgerCommit.SHA; ls != "" {
				n += " + ledger " + shortSHA(ls)
			}
			commitNote = "commit " + n
		}
	} else {
		// In-place 回退（非 git 或 worktree 不可用）：一次 scoped 提交。
		outcome := c.commitUnitChangesIn(c.Cwd, packet, c.commitExcludes(), c.baseRefOf(ps), "")
		ps.Commit = commitOutcomeMap(outcome)
		if outcome.SHA != "" {
			commitNote = fmt.Sprintf("commit %s", shortSHA(outcome.SHA))
		} else {
			commitNote = fmt.Sprintf("commit skipped (%s)", firstNonEmpty(outcome.Skipped, outcome.Error, "unknown"))
		}
	}
	ps.Status = "completed"
	at := c.nowISO()
	ps.UpdatedAt = &at
	c.touchProgress()
	retroLine := fmt.Sprintf("- %s packet %s: completed (attempts=%d, failed=%d, %s)",
		c.nowISO(), id, ps.Attempts, ps.FailedAttempts, commitNote)
	c.appendRetro(retroLine)
	c.save()
	return c.err()
}

// ---- 缺陷注入抽检（§4.3）----

// maybeDefectInjection 确定性、常开（2026-08-20 用户裁决，无开关）、跑在 L1 与
// Review 之间；结论喂给评审者。命中抽样时返回结论对象，否则 nil。
// 分支处理在此（P0-2）：只有 fake-tests / restore-failed 停止单元；
// uninjectable / baseline-red / 缺配置进 inbox 让人介入、单元继续。
func (c *Context) maybeDefectInjection(packet *contract.Packet) (*defectinjection.Outcome, error) {
	di := c.Config.DefectInjection
	ps := c.State.Packets[packet.PacketID]
	completedCount := 0
	for _, p := range c.State.Packets {
		if p.Status == "completed" {
			completedCount++
		}
	}
	// 测试文件占比来自权威 diff（L1 boundary），与 verify 共用模式——
	// 不是 executor 自报（P2）。
	patterns := make([]*regexp.Regexp, 0, len(c.Config.Verify.TestFilePatterns))
	for _, p := range c.Config.Verify.TestFilePatterns {
		patterns = append(patterns, regexp.MustCompile(p))
	}
	changed := l1BoundaryChangedFiles(ps)
	if changed == nil {
		changed = executorSelfChangedFiles(ps)
	}
	testRatio := 0.0
	if len(changed) > 0 {
		hits := 0
		for _, f := range changed {
			for _, re := range patterns {
				if re.MatchString(f) {
					hits++
					break
				}
			}
		}
		testRatio = float64(hits) / float64(len(changed))
	}
	// EveryNUnits 为指针（nil = 未声明 → 默认 5）。
	everyN := 5
	if di.EveryNUnits != nil {
		everyN = *di.EveryNUnits
	}
	if !defectinjection.ShouldSample(completedCount, everyN, di.TestFileRatioBoost, testRatio) {
		return nil, nil
	}
	inboxContinue := func(reason string, evidence map[string]any) (*defectinjection.Outcome, error) {
		evidence["at"] = c.nowISO()
		ps.Evidence.DefectInjection = evidence
		entry := map[string]any{"at": c.nowISO(), "packet-id": packet.PacketID, "reason": reason}
		c.State.Inbox = append(c.State.Inbox, entry)
		c.appendInbox(entry)
		c.save()
		return nil, c.err()
	}

	if di.Patch == "" || di.TestCmd == "" {
		return inboxContinue(
			fmt.Sprintf("defect injection sampled for %s but defect-injection.patch/test-cmd are not configured "+
				"in flywheel.config.json — a human should add them for this task.", packet.PacketID),
			map[string]any{
				"skipped": "missing-config",
				"detail":  "sampled but defect-injection.patch/test-cmd not configured for this task",
			})
	}
	outcome := defectinjection.RunDefectInjection(defectinjection.Args{
		PatchFile:        di.Patch,
		TestCmd:          di.TestCmd,
		Cwd:              c.unitCwd(packet),
		TimeoutMs:        c.Config.Verify.CommandTimeoutMs,
		EnvAllowlist:     c.Config.Verify.EnvAllowlist,
		LedgerTaskPrefix: c.taskDirPrefix(), // v0.19：单一信源账本守卫
	})
	ev, _ := jsonToMap(outcome)
	ev["at"] = c.nowISO()
	ps.Evidence.DefectInjection = ev
	c.save()
	if ferr := c.err(); ferr != nil {
		return nil, ferr
	}

	if outcome.Outcome == "pass" || outcome.Outcome == "fake-tests" || outcome.Outcome == "restore-failed" {
		return &outcome, nil
	}
	// uninjectable / baseline-red：inbox 叫人，绝不指控假测试（P0-2）。
	return inboxContinue(
		fmt.Sprintf("defect injection for %s: %s — %s (human should refresh the patch / fix the suite)",
			packet.PacketID, outcome.Outcome, derefStringPtr(outcome.Detail)),
		ev)
}

func derefStringPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// jsonToMap 结构体 → 通用 map（键名经 json tag 与 Node 版逐字对齐）。
func jsonToMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// doneReportValid 仍有效的 done-report 是否在盘上（pause/resume 与中断恢复）。
func (c *Context) doneReportValid(packet *contract.Packet) bool {
	reportAbs := filepath.Join(c.TaskDir, "reports", packet.PacketID+".done.json")
	raw, err := os.ReadFile(reportAbs)
	if err != nil {
		return false
	}
	return donereport.Parse(raw, packet.DoneReportFormat).OK
}

// injectionStop fake-tests / restore-failed 的收尾：inbox + 停（v0.14 DC3）。
func (c *Context) injectionStop(packet *contract.Packet, oi *defectinjection.Outcome) error {
	id := packet.PacketID
	ps := c.State.Packets[id]
	ps.Status = "inbox"
	at := c.nowISO()
	ps.UpdatedAt = &at
	reason := fmt.Sprintf(
		"defect injection FAILED for %s: injected defect did not turn the tests red (fake/weak tests proven). Human needed.", id)
	if oi.Outcome != "fake-tests" {
		reason = fmt.Sprintf(
			"defect injection for %s: restore via git apply -R FAILED — the workspace may still hold the injected defect. Human needed.", id)
	}
	entry := map[string]any{"at": c.nowISO(), "packet-id": id, "reason": reason}
	c.State.Inbox = append(c.State.Inbox, entry)
	c.State.Status = "escalated"
	c.appendInbox(entry)
	c.appendRetro(fmt.Sprintf("- %v packet %s: INBOX — defect injection %s", entry["at"], id, oi.Outcome))
	c.save()
	return c.err()
}

// ---- packet 处理（attempts 循环）----

type processOpts struct {
	PauseBefore string // "review"：L1 绿后冻结，等下一次 run 从 review 继续
}

// processPacket 单个 packet 的完整处理循环。
// P3 修复：resume 直达 review 时补跑缺陷注入采样（Node 直接跳过）。
func (c *Context) processPacket(packet *contract.Packet, opts processOpts) (string, error) {
	id := packet.PacketID
	ps := c.State.Packets[id]

	// P2：已升级到 inbox 的 packet 重跑时绝不重新 spawn 或重复追加 inbox 条目
	// ——人通过编辑 state.json 重新武装（§12）。凭证流程例外：人的修复就是
	// 审批文件，它出现在超时升级之后即重新武装该单元。
	if ps.Status == "inbox" {
		approvalFile := filepath.Join(c.TaskDir, "approvals", id+".json")
		if packet.NeedsLiveCredentials && ps.Evidence.Credentials == nil && fileExists(approvalFile) {
			c.Log(fmt.Sprintf("%s: approval file appeared after inbox escalation — re-arming", id))
			ps.Status = "pending"
			at := c.nowISO()
			ps.UpdatedAt = &at
			c.save()
		} else {
			c.Log(fmt.Sprintf("%s: already escalated to inbox — waiting for human (edit state.json to re-arm)", id))
			// 任务仍是升级态；重跑绝不能把它翻回 running。
			c.State.Status = "escalated"
			c.save()
			if ferr := c.err(); ferr != nil {
				return "", ferr
			}
			return "stop", nil
		}
	}

	if ps.Status == "interrupted" {
		c.Log(fmt.Sprintf("%s: resuming interrupted unit (artifacts preserved, no penalty)", id))
		ps.Status = "pending"
		c.save()
	}

	// --pause-before review 之后 resume（或 stop 发生在 L1 与 review 之间）：
	// L1 绿且 done-report 完好——直接从 REVIEW 继续，不重执行、不抹报告（#6）。
	stage := "execute"
	resumeAtReview := false
	if ps.Status == "l1-passed" && l1PassTrue(ps) && c.doneReportValid(packet) {
		c.Log(fmt.Sprintf("%s: resuming at review (L1 green + done-report intact)", id))
		stage = "review"
		resumeAtReview = true
	}

	var pendingInjection *defectinjection.Outcome
	for {
		maxRetries := packet.Escalation.MaxRetries // 每轮重读：planner 可能修订它
		if ps.FailedAttempts >= maxRetries {
			escalation, err := c.escalatePacket(packet)
			if err != nil {
				return "", err
			}
			if escalation == "inbox" {
				return "stop", nil
			}
			if p, ok := c.Packets[id]; ok { // P0-2：按修订后的契约跑
				packet = p
			}
			stage = "execute"
		}

		if stage == "execute" {
			exec, err := c.executeStep(packet)
			if err != nil {
				return "", err
			}
			if exec.Stopped {
				return "stop", nil
			}
			if !exec.Success {
				// C4（v0.18）：execute 失败路径的空转签名用编排器亲自 diff 的
				// 权威变更集——此前退化为空集/executor 自报，谎报即可绕过 DC4。
				outcome := c.failAttempt(packet, orTransport(exec.ErrorCode), nil, c.authoritativeChangedFiles(packet))
				if outcome == "spin" {
					esc, err := c.escalatePacket(packet)
					if err != nil {
						return "", err
					}
					if esc == "inbox" {
						return "stop", nil
					}
					if p, ok := c.Packets[id]; ok {
						packet = p
					}
				}
				continue
			}

			l1 := c.l1Step(packet)
			if !l1.Pass {
				var failedCommands []string
				for _, r := range l1.Gate.Results {
					if !r.Pass {
						failedCommands = append(failedCommands, r.Cmd)
					}
				}
				violations := make([]string, 0, len(l1.Boundary.Violations))
				for _, v := range l1.Boundary.Violations {
					if v.Path != "" {
						violations = append(violations, fmt.Sprintf("%s:%s", v.Type, v.Path))
					} else {
						violations = append(violations, fmt.Sprintf("%s:%d", v.Type, v.Count))
					}
				}
				failedCommands = append(failedCommands, violations...)
				// 权威 changedFiles：本轮 L1 boundary 的 git diff
				// （diff 不可用时保持 nil → 兜底链接管）。
				authChanged := l1.Boundary.ChangedFiles
				if l1.Boundary.Skipped {
					authChanged = nil
				}
				outcome := c.failAttempt(packet, errL1Failed, failedCommands, authChanged)
				if outcome == "spin" {
					esc, err := c.escalatePacket(packet)
					if err != nil {
						return "", err
					}
					if esc == "inbox" {
						return "stop", nil
					}
					if p, ok := c.Packets[id]; ok {
						packet = p
					}
				}
				continue
			}

			// 缺陷注入抽检在 L1 与 Review 之间（§4.3）；结论喂给评审者。
			oi, err := c.maybeDefectInjection(packet)
			if err != nil {
				return "", err
			}
			if oi != nil && (oi.Outcome == "fake-tests" || oi.Outcome == "restore-failed") {
				// injectionStop 的保存失败记入 ctx.fatal；RunTask/runSingle 出口的
				// err() 检查必然上抛同一错误。两次相邻 save() 之间没有任何可注入
				// 故障的钩子，逐点传播不可测且行为等价（覆盖率审计 2026-08-23 删除
				// 冗余中间检查）。
				_ = c.injectionStop(packet, oi)
				return "stop", nil
			}
			pendingInjection = oi

			if opts.PauseBefore == "review" {
				c.State.Status = "paused"
				c.save()
				c.Log(fmt.Sprintf("%s: paused before review (--pause-before review)", id))
				if ferr := c.err(); ferr != nil {
					return "", ferr
				}
				return "pause", nil
			}
			stage = "review"
		}

		if resumeAtReview {
			// P3 修复：resume 到 review 的路径此前完全跳过缺陷注入采样；
			// 补一次（采样决策与正常路径同一函数）。
			oi, err := c.maybeDefectInjection(packet)
			if err != nil {
				return "", err
			}
			if oi != nil && (oi.Outcome == "fake-tests" || oi.Outcome == "restore-failed") {
				// 同上：出口 err() 检查必然上抛 injectionStop 的保存失败
				// （覆盖率审计 2026-08-23 删除冗余中间检查）。
				_ = c.injectionStop(packet, oi)
				return "stop", nil
			}
			pendingInjection = oi
			resumeAtReview = false
		}
		review, err := c.reviewStep(packet, pendingInjection)
		if err != nil {
			return "", err
		}
		pendingInjection = nil
		if review.Stopped {
			return "stop", nil
		}
		if review.Pass {
			if err := c.completeUnit(packet); err != nil {
				if errors.Is(err, ErrMergeConflict) {
					// 合并冲突：worktree 已保留，升级到人（与其他升级同款
					// inbox 路由），任务状态不说谎。
					ps.Status = "inbox"
					at := c.nowISO()
					ps.UpdatedAt = &at
					entry := map[string]any{
						"at":        c.nowISO(),
						"packet-id": id,
						"reason":    fmt.Sprintf("packet %s: merge conflict — worktree preserved for human resolution (%v).", id, err),
					}
					c.State.Inbox = append(c.State.Inbox, entry)
					c.State.Status = "escalated"
					c.appendInbox(entry)
					c.save()
					return "stop", nil
				}
				return "", err
			}
			return "ok", nil
		}

		verdict := review.Verdict
		if verdict != nil && verdict.Verdict == "verification-gap" {
			// v0.14 DC2：验证缺口退回 Plan 加强验收（消耗修订预算），然后复验
			// ——单元绝不在缺口上被判完成。
			c.Log(fmt.Sprintf("%s: verification-gap — returning to planner to strengthen acceptance", id))
			esc, err := c.escalatePacket(packet)
			if err != nil {
				return "", err
			}
			if esc == "inbox" {
				return "stop", nil
			}
			if p, ok := c.Packets[id]; ok { // 修订后的契约（P0-2）
				packet = p
			}
			recheck := c.l1Step(packet) // 用加强后的验收复验
			if !recheck.Pass {
				var failedCommands []string
				for _, r := range recheck.Gate.Results {
					if !r.Pass {
						failedCommands = append(failedCommands, r.Cmd)
					}
				}
				c.failAttempt(packet, errL1Failed, failedCommands, nil)
				stage = "execute"
				continue
			}
			stage = "review" // 对照加强后的验收重新评审
			continue
		}

		ps.Status = "review-failed"
		stage = "execute"
		code := errReviewReturned
		if review.ErrorCode != "" {
			code = review.ErrorCode
		}
		outcome := c.failAttempt(packet, code, nil, nil)
		if outcome == "spin" {
			esc, err := c.escalatePacket(packet)
			if err != nil {
				return "", err
			}
			if esc == "inbox" {
				return "stop", nil
			}
			if p, ok := c.Packets[id]; ok {
				packet = p
			}
		}
	}
}

func l1PassTrue(ps *state.PacketState) bool {
	l1, ok := ps.Evidence.L1.(map[string]any)
	if !ok {
		return false
	}
	pass, _ := l1["pass"].(bool)
	return pass
}

func orTransport(code errs.ErrorCode) errs.ErrorCode {
	if code == "" {
		return errs.TRANSPORT
	}
	return code
}

// ---- 入口 ----

func (c *Context) nextActionablePacket(milestoneID string) *contract.Packet {
	for _, p := range c.PacketList {
		if m, err := contract.MilestoneOfPacket(p.PacketID); err != nil || m != milestoneID {
			continue
		}
		if c.State.Packets[p.PacketID].Status != "completed" {
			return p
		}
	}
	return nil
}

func (c *Context) currentMilestone() *contract.Milestone {
	for i := range c.Spec.Milestones {
		ms, ok := c.State.Milestones[c.Spec.Milestones[i].ID]
		if !ok || ms.Status != "closed" {
			return &c.Spec.Milestones[i]
		}
	}
	return nil
}

func (c *Context) writePid() {
	_ = os.MkdirAll(c.TaskDir, 0o755)
	payload, _ := json.Marshal(map[string]any{"pid": os.Getpid(), "startedAt": c.nowISO()})
	_ = os.WriteFile(PidFile(c.TaskDir), payload, 0o644)
}

func clearPid(taskDir string) {
	_ = os.Remove(PidFile(taskDir)) // best effort
}

// registerSigterm P1 修复：注册 SIGTERM（Node 定义了 onSigterm 却从未
// process.once 注册，kill switch 失效）。收到信号 → 杀光在飞子进程组并等到
// quiescence → 以退出码 143 结束——绝不让孤儿进程继续写工作区。
func registerSigterm(log func(string)) chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	go func() {
		if _, ok := <-ch; ok {
			log("SIGTERM received — killing in-flight children, then exiting")
			watchdog.KillActiveChildren(watchdog.DefaultKillGraceMs)
			os.Exit(143)
		}
	}()
	return ch
}

func unregisterSigterm(ch chan os.Signal) {
	signal.Stop(ch)
	close(ch)
}

// TaskSummary runTask/runSingleStep 的返回摘要；state.json 才是事实信源。
// JSON 键与 Node 版逐字一致（CLI 直接输出）。
type TaskSummary struct {
	Slug             string   `json:"slug"`
	Status           string   `json:"status"`
	CurrentMilestone *string  `json:"current-milestone"`
	PacketsCompleted string   `json:"packets-completed"`
	Revision         int64    `json:"revision"`
	LastProgressAt   *string  `json:"last-progress-at"`
	Tokens           int64    `json:"tokens"`
	CostUSD          float64  `json:"cost-usd"`
	Step             *StepSum `json:"step,omitempty"`
}

// StepSum --only 单步模式的步骤结果（#9：失败步骤以非零码退出）。
type StepSum struct {
	Name    string           `json:"name"`
	OK      bool             `json:"ok"`
	Verdict *prompts.Verdict `json:"verdict,omitempty"`
	Detail  *string          `json:"detail,omitempty"`
}

func strPtrAlways(s string) *string { return &s }

func (c *Context) summary() TaskSummary {
	done := 0
	total := len(c.State.Packets)
	for _, p := range c.State.Packets {
		if p.Status == "completed" {
			done++
		}
	}
	return TaskSummary{
		Slug:             c.State.Slug,
		Status:           c.State.Status,
		CurrentMilestone: c.State.CurrentMilestone,
		PacketsCompleted: fmt.Sprintf("%d/%d", done, total),
		Revision:         c.State.Revision,
		LastProgressAt:   c.State.LastProgressAt,
		Tokens:           c.State.Budget.Totals.Tokens,
		CostUSD:          c.State.Budget.Totals.CostUSD,
	}
}

// RunOpts RunTask / RunSingleStep 共享的装配入参。
type RunOpts struct {
	TaskDir string
	Cwd     string
	Config  *config.Config
	Drivers *drivers.Drivers
	Log     func(string)
	Sleep   func(time.Duration)
	Now     func() time.Time
	Env     childenvLookup
}

// RunTaskOpts RunTask 入参。
type RunTaskOpts struct {
	RunOpts
	PauseBefore string
}

// RunTask 全自动运行：里程碑 → packet → attempts（execute → L1 → review）
// → 里程碑闸门。返回摘要；state.json 携带真相。
func RunTask(opts RunTaskOpts) (TaskSummary, error) {
	ctx, err := CreateContext(Opts{
		TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config,
		Drivers: opts.Drivers, Log: opts.Log, Sleep: opts.Sleep, Now: opts.Now, Env: opts.Env,
	})
	if err != nil {
		return TaskSummary{}, err
	}
	ctx.pauseBefore = opts.PauseBefore
	if err := AssertNoRunningOrchestrator(ctx.TaskDir); err != nil { // P1：每任务单实例
		return ctx.summary(), err
	}
	ctx.writePid()
	sigCh := registerSigterm(ctx.Log)
	defer unregisterSigterm(sigCh)
	defer clearPid(ctx.TaskDir)

	summary, err := ctx.run()
	if err != nil {
		return summary, err
	}
	return summary, ctx.err()
}

func (c *Context) reloadPackets() error {
	fresh, err := contract.LoadPackets(c.TaskDir, c.Spec)
	if err != nil {
		return err
	}
	c.PacketList = fresh
	c.Packets = map[string]*contract.Packet{}
	for _, p := range fresh {
		c.Packets[p.PacketID] = p
	}
	return state.EnsurePacketStates(c.State, fresh)
}

func (c *Context) run() (TaskSummary, error) {
	// 崩溃恢复：绝不静默重置在飞单元（§5.2）。
	interrupted := c.markInterruptedUnits()
	if len(interrupted) > 0 {
		c.Log(fmt.Sprintf("recovered interrupted units: %s", strings.Join(interrupted, ", ")))
	}
	c.State.Status = "running" // paused/stopped 任务 resume 时翻回 running（#6）
	c.save()

	for mi := range c.Spec.Milestones {
		milestone := c.Spec.Milestones[mi]
		ms := c.State.Milestones[milestone.ID]
		if ms.Status == "closed" {
			continue
		}

		c.State.CurrentMilestone = strPtrAlways(milestone.ID)
		ms.Status = "in-progress"
		c.State.Milestones[milestone.ID] = ms
		budgetStartTokens := c.State.Budget.Totals.Tokens
		budgetStartCost := c.State.Budget.Totals.CostUSD
		c.save()

		plan, err := c.planStep(milestone)
		if err != nil {
			return c.summary(), err
		}
		if plan.Stopped {
			return c.summary(), c.err()
		}
		if !plan.OK {
			c.State.Status = "escalated"
			entry := map[string]any{
				"at":        c.nowISO(),
				"packet-id": nil,
				"milestone": milestone.ID,
				"reason":    fmt.Sprintf("plan step failed on every channel (last: %s). Human needed.", plan.ErrorCode),
			}
			c.State.Inbox = append(c.State.Inbox, entry)
			c.appendInbox(entry)
			c.save()
			if ferr := c.err(); ferr != nil {
				return c.summary(), ferr
			}
			return c.summary(), nil
		}
		// P1：planner 可能在这步常规运行中修订过契约文件——像升级路径一样
		// 重载它们（P0-2 对称性）。A1：重载前后 diff acceptance 指纹留痕。
		old := c.PacketList
		if err := c.reloadPackets(); err != nil {
			return c.summary(), err
		}
		c.recordAcceptanceChanges(old, c.PacketList)
		// v0.16：仅契约提交（脏账本不卷入——这里做账本提交会移动 HEAD，
		// 破坏暂停单元 worktree 的 ff-merge）。无变更 = 无提交 = HEAD 不动。
		c.commitTaskDirChanges(fmt.Sprintf("docs(%s): plan update", c.State.Slug))
		c.save()

		for {
			if ferr := c.err(); ferr != nil {
				return c.summary(), ferr
			}
			packet := c.nextActionablePacket(milestone.ID)
			if packet == nil {
				break
			}
			outcome, err := c.processPacket(packet, processOpts{PauseBefore: c.pauseBefore})
			if err != nil {
				return c.summary(), err
			}
			if outcome == "stop" || outcome == "pause" {
				return c.summary(), nil
			}
		}

		gate := c.milestoneGateStep(milestone)
		if !gate.Pass {
			c.State.Status = "escalated"
			entry := map[string]any{
				"at":        c.nowISO(),
				"packet-id": nil,
				"milestone": milestone.ID,
				"reason":    fmt.Sprintf("milestone %s gate (L2/L3) failed", milestone.ID),
			}
			c.State.Inbox = append(c.State.Inbox, entry)
			c.appendInbox(entry)
			c.save()
			if ferr := c.err(); ferr != nil {
				return c.summary(), ferr
			}
			return c.summary(), nil
		}

		// Judge（§7 里程碑终验）：关闭前的全新上下文终验。
		judge, err := c.judgeStep(milestone)
		if err != nil {
			return c.summary(), err
		}
		if judge.Stopped {
			return c.summary(), c.err()
		}
		if !judge.Pass {
			c.State.Status = "escalated"
			verdictText := string(judge.ErrorCode)
			reasons := ""
			if judge.Verdict != nil {
				verdictText = judge.Verdict.Verdict
				reasons = strings.Join(judge.Verdict.Reasons, "; ")
			}
			if reasons == "" {
				reasons = "see metrics"
			}
			entry := map[string]any{
				"at":        c.nowISO(),
				"packet-id": nil,
				"milestone": milestone.ID,
				"reason":    fmt.Sprintf("milestone %s judge verdict: %s — %s", milestone.ID, verdictText, reasons),
			}
			c.State.Inbox = append(c.State.Inbox, entry)
			c.appendInbox(entry)
			c.save()
			if ferr := c.err(); ferr != nil {
				return c.summary(), ferr
			}
			return c.summary(), nil
		}

		ms = c.State.Milestones[milestone.ID]
		ms.Status = "closed"
		closedAt := c.nowISO()
		ms.ClosedAt = &closedAt
		c.State.Milestones[milestone.ID] = ms
		c.touchProgress()
		judgeScore := "n/a"
		if judge.Verdict != nil && judge.Verdict.Score != nil {
			judgeScore = strconv.FormatFloat(*judge.Verdict.Score, 'g', -1, 64)
		}
		c.appendRetro(fmt.Sprintf("- %s milestone %s: closed (gate pass, judge %s)", c.nowISO(), milestone.ID, judgeScore))
		c.appendRetro(c.milestoneSummaryLine(milestone, budgetStartTokens, budgetStartCost))
		c.save()
	}

	c.State.Status = "completed"
	c.State.CurrentMilestone = nil
	c.touchProgress()
	c.save()
	return c.summary(), c.err()
}

// milestoneSummaryLine retro.md 的里程碑摘要（§5.3 字段）：只记可核对事实。
func (c *Context) milestoneSummaryLine(milestone contract.Milestone, startTokens int64, startCost float64) string {
	var states []*state.PacketState
	for _, p := range c.PacketList {
		if m, err := contract.MilestoneOfPacket(p.PacketID); err == nil && m == milestone.ID {
			if ps, ok := c.State.Packets[p.PacketID]; ok {
				states = append(states, ps)
			}
		}
	}
	attempts, reviewFailed, reopens, scopeChanges := 0, 0, 0, 0
	for _, ps := range states {
		attempts += ps.Attempts
		for _, rh := range ps.RetryHistory {
			entry, ok := rh.(map[string]any)
			if !ok {
				continue
			}
			if ec, _ := entry["error-code"].(string); ec == "REVIEW_RETURNED" {
				reviewFailed++
			}
			if pol, _ := entry["policy"].(string); pol == "return-to-planner" {
				scopeChanges++
			}
		}
		reopens += ps.PlanRevisions
	}
	tokens := c.State.Budget.Totals.Tokens - startTokens
	cost := mathRound((c.State.Budget.Totals.CostUSD-startCost)*1e8) / 1e8
	// P2：人工介入只数本里程碑的条目（entry.milestone 或 entry.packet-id 解析
	// 到本里程碑）——绝不是全任务 inbox，那会随每次后续升级膨胀。
	interventions := 0
	for _, e := range c.State.Inbox {
		entry, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if m, _ := entry["milestone"].(string); m == milestone.ID {
			interventions++
			continue
		}
		if pid, _ := entry["packet-id"].(string); pid != "" {
			if m, err := contract.MilestoneOfPacket(pid); err == nil && m == milestone.ID {
				interventions++
			}
		}
	}
	return strings.Join([]string{
		fmt.Sprintf("  summary: rounds=%d, defects(review-failed)=%d, reopens=%d, "+
			"scope-changes=%d (return-to-planner revisions), budget=+%d tokens / $%s, "+
			"human-interventions=%d",
			attempts, reviewFailed, reopens, scopeChanges, tokens,
			strconv.FormatFloat(cost, 'f', -1, 64), interventions),
		"  improvements (<=3, placeholder — filled by Plan/human): -",
	}, "\n")
}

// RunSingleStepOpts 手工单步模式入参。
type RunSingleStepOpts struct {
	RunOpts
	Only     string // plan | execute | review | verify
	PacketID string
}

// RunSingleStep 手工单步模式（§12）：--only 恰好执行一次状态转移。
// 步骤结果落在摘要（step 字段），失败步骤由 CLI 以非零码退出（#9）。
func RunSingleStep(opts RunSingleStepOpts) (TaskSummary, error) {
	ctx, err := CreateContext(Opts{
		TaskDir: opts.TaskDir, Cwd: opts.Cwd, Config: opts.Config,
		Drivers: opts.Drivers, Log: opts.Log, Sleep: opts.Sleep, Now: opts.Now, Env: opts.Env,
	})
	if err != nil {
		return TaskSummary{}, err
	}
	if err := AssertNoRunningOrchestrator(ctx.TaskDir); err != nil {
		return ctx.summary(), err
	}
	ctx.writePid()
	sigCh := registerSigterm(ctx.Log)
	defer unregisterSigterm(sigCh)
	defer clearPid(ctx.TaskDir)

	summary, err := ctx.runSingle(opts.Only, opts.PacketID)
	if err != nil {
		return summary, err
	}
	return summary, ctx.err()
}

func (c *Context) runSingle(only, packetID string) (TaskSummary, error) {
	interrupted := c.markInterruptedUnits()
	if len(interrupted) > 0 {
		c.Log(fmt.Sprintf("recovered interrupted units: %s", strings.Join(interrupted, ", ")))
	}
	c.save()

	pickPacket := func() (*contract.Packet, error) {
		if packetID != "" {
			p, ok := c.Packets[packetID]
			if !ok {
				return nil, fmt.Errorf("unknown packet/unit: %s", packetID)
			}
			return p, nil
		}
		m := c.currentMilestone()
		if m == nil {
			return nil, fmt.Errorf("no open milestone")
		}
		p := c.nextActionablePacket(m.ID)
		if p == nil {
			return nil, fmt.Errorf("no actionable packet in milestone %s", m.ID)
		}
		return p, nil
	}

	var step *StepSum
	switch only {
	case "plan":
		m := c.currentMilestone()
		if m == nil {
			return c.summary(), fmt.Errorf("no open milestone")
		}
		old := c.PacketList // must-fix 5：单步 plan 与全自动路径同享指纹留痕
		r, err := c.planStep(*m)
		if err != nil {
			return c.summary(), err
		}
		if r.OK {
			if err := c.reloadPackets(); err != nil {
				return c.summary(), err
			}
			c.recordAcceptanceChanges(old, c.PacketList)
		}
		step = &StepSum{Name: "plan", OK: r.OK, Detail: optStr(string(r.ErrorCode))}
	case "execute":
		packet, perr := pickPacket()
		if perr != nil {
			return c.summary(), perr
		}
		r, err := c.executeStep(packet)
		if err != nil {
			return c.summary(), err
		}
		step = &StepSum{Name: "execute", OK: r.Success, Detail: optStr(string(r.ErrorCode))}
	case "review":
		packet, perr := pickPacket()
		if perr != nil {
			return c.summary(), perr
		}
		ps := c.State.Packets[packet.PacketID]
		// 铁律 4：确定性验收绿了评审才有发言权。
		if !l1PassTrue(ps) {
			return c.summary(), fmt.Errorf("unit %s has no PASSING L1 evidence — run execute/verify first", packet.PacketID)
		}
		r, err := c.reviewStep(packet, nil)
		if err != nil {
			return c.summary(), err
		}
		step = &StepSum{Name: "review", OK: r.Pass, Verdict: r.Verdict, Detail: optStr(string(r.ErrorCode))}
	case "verify":
		if packetID != "" {
			packet, perr := pickPacket()
			if perr != nil {
				return c.summary(), perr
			}
			r := c.l1Step(packet)
			step = &StepSum{Name: "verify", OK: r.Pass}
		} else {
			ok := true
			for _, p := range c.PacketList {
				if c.State.Packets[p.PacketID].Status != "completed" {
					r := c.l1Step(p)
					if !r.Pass {
						ok = false
					}
				}
			}
			if m := c.currentMilestone(); m != nil {
				g := c.milestoneGateStep(*m)
				if !g.Pass {
					ok = false
				}
			}
			step = &StepSum{Name: "verify", OK: ok}
		}
	default:
		return c.summary(), fmt.Errorf("unknown --only step: %s", only)
	}
	s := c.summary()
	s.Step = step
	return s, nil
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
