// units.go — 单元生命周期（DESIGN §4.2/§4.4/§6）：packet 处理状态机、
// worktree 提交/合并、完成判定、失败记账、升级路径、缺陷注入编排。入口
// RunTask/run（run.go）按里程碑驱动本层。
package machine

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/ckanner/flywheel/internal/circuitbreaker"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/defectinjection"
	"github.com/ckanner/flywheel/internal/donereport"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
)

func (c *Context) commitUnitChangesIn(cwd string, packet *contract.Packet, excludePaths []string, baseRef, restrictToPrefix string) verify.CommitOutcome {
	return verify.CommitUnitChanges(cwd, packet, excludePaths, baseRef, restrictToPrefix)
}

func (c *Context) mergeUnitWorktree(branch string) (string, error) {
	return verify.MergeUnitWorktree(c.Cwd, branch)
}

func (c *Context) removeUnitWorktree(branch, path string) error {
	return verify.RemoveUnitWorktree(c.Cwd, branch, path)
}

// commitTaskDirChanges 契约层提交（planner 修订之后）。
func (c *Context) commitTaskDirChanges(message string) verify.CommitOutcome {
	return verify.CommitTaskDirChanges(c.Cwd, c.taskDirPrefix(), message, c.commitExcludes(), contractFileRe)
}

func (c *Context) markInterruptedUnits() []string {
	return state.MarkInterruptedUnits(c.State, c.nowISO())
}

// failAttempt 一次尝试失败记账 + 空转检测。返回 "spin"（应立即升级）或
// "failed"。
//
// Changed files remain in the audit signature, preferring the authoritative Git
// diff. Early spin now requires unchanged candidate/check/failure observations;
// neither executor self-report nor file-name similarity is proof of no progress.
func (c *Context) failAttempt(packet *contract.Packet, errorCode errs.ErrorCode, failedCommands, authoritativeChangedFiles []string) string {
	id := packet.PacketID
	ps := c.State.Packets[id]
	ps.FailedAttempts++
	c.recordRetry(ps, "attempt", string(errorCode), "requeue")
	var details any
	if previous, ok := ps.LastFeedback.(map[string]any); ok {
		details = previous["details"]
	}
	// 下一次全新执行会话必须拿到真实失败原因；旧证据只作为反馈，不再算绿灯。
	ps.LastFeedback = map[string]any{
		"details":    details,
		"error-code": string(errorCode), "failed-commands": failedCommands,
		"l1": ps.Evidence.L1, "review": ps.Evidence.Review,
		"exit": ps.Exit, "at": c.nowISO(),
	}
	if ps.Evidence.FinalGate != nil {
		ps.LastFeedback.(map[string]any)["final-gate"] = ps.Evidence.FinalGate
	}

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
	progress := c.failureProgress(packet, errorCode)
	signature := circuitbreaker.Signature([]string{string(errorCode)}, failedCommands, changed, progress)
	assessment := circuitbreaker.AssessProgress(derefString(ps.LastFailureSignature), signature)
	ps.LastFeedback.(map[string]any)["progress"] = assessment
	ps.LastFeedback.(map[string]any)["progress-evidence"] = progress
	spin := circuitbreaker.DetectSpin(derefString(ps.LastFailureSignature), signature, ps.FailedAttempts)
	action := ""
	if c.Config.JEV.Enabled || c.State.RuntimeDecisions != nil {
		action = c.runtimeFailureDecision(packet, errorCode, progress, spin)
	}
	if action == "spin" {
		spin = true
	} else if action == "failed" || c.upgradeExecutionPolicy(packet, errorCode, assessment) {
		spin = false
	}
	sig := signature
	ps.LastFailureSignature = &sig
	at := c.nowISO()
	ps.UpdatedAt = &at
	c.save()
	if spin {
		c.Log(fmt.Sprintf("spin detected on %s: unchanged candidate and authoritative failure observations — escalating (§6)", id))
		return "spin"
	}
	return "failed"
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
	if c.quick() {
		c.finishRuntimeReplan(packet)
		feedback, _ := json.Marshal(ps.LastFeedback)
		err := c.upgradeWorkflow("Quick repair or verification requires Full planning", []string{string(feedback)})
		return "workflow-upgraded", err
	}
	onFail := packet.Escalation.OnFail

	if onFail == "return-to-planner" && ps.PlanRevisions < 1 {
		milestone, found := c.milestoneByID(ps.Milestone)
		if !found {
			return "", fmt.Errorf("packet %s references unknown milestone %q", id, ps.Milestone)
		}
		feedback, _ := json.Marshal(ps.LastFeedback)
		plan, err := c.runtimePlan(packet, milestone, "Returned packet "+id+": "+string(feedback))
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
			// 校验与持久化后，将合法修订快进到现存工作区。绝不删除失败产物；
			// executor 自提交导致无法快进时保留现场并返回可恢复错误。
			if err := c.persistPlanContracts(fmt.Sprintf("docs(%s): plan revision %d", c.State.Slug, ps.PlanRevisions+1)); err != nil {
				return "", err
			}
			invalidateEvidence(ps)
			c.finishRuntimeReplan(packet)
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

	c.finishRuntimeReplan(packet)
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

// P2 修复（审计）：先完成提交、再置 completed 并保存——Node 先写 completed
// 再提交，崩溃窗口内 state.json 谎报“已完成”而代码从未落库。
//
// 合并门控（review 修正）：以“分支是否领先 base”为准（verify.AheadCount），
// 而不是“本次 scoped 提交是否产生了新提交”——executor 自提交、或崩溃恢复
// 重跑 completeUnit 时 scoped 提交为空但分支领先，旧条件会静默跳过合并。
// ff-merge 失败时返回 ErrMergeConflict：worktree 原样保留、状态绝不标
// completed（§5.2 state.json 不说谎），由调用方升级到人。
func (c *Context) completeUnit(packet *contract.Packet) error {
	defer c.measurePhase("delivery", packet.PacketID)()
	if c.State.SharedExecution != nil {
		return fmt.Errorf("%w: shared candidate requires whole-batch verification and delivery", ErrCommitFailed)
	}
	if err := c.err(); err != nil {
		return err
	}
	id := packet.PacketID
	ps := c.State.Packets[id]
	review, _ := ps.Evidence.Review.(map[string]any)
	if c.quick() && (!c.currentFinalGate(packet) || !c.validFinalReview(packet)) {
		return fmt.Errorf("%w: Quick final verification or task review is stale", ErrCommitFailed)
	}
	if !c.currentL1Evidence(packet) || review["verdict"] != "pass" || evidenceIdentity(ps.Evidence.Review) != evidenceIdentity(ps.Evidence.L1) {
		return fmt.Errorf("%w: candidate changed or verification evidence is stale", ErrCommitFailed)
	}
	failPersistence := func(stage, message string) error {
		rec, ok := ps.Commit.(map[string]any)
		if !ok {
			rec = map[string]any{}
		}
		rec["stage"], rec["error"], rec["at"] = stage, message, c.nowISO()
		ps.Commit = rec
		c.save()
		return fmt.Errorf("%w: %s: %s", ErrCommitFailed, stage, message)
	}
	// A crash can follow successful cleanup but precede the final state CAS.
	// The checks above already proved the canonical checkout is that exact
	// accepted candidate; finish its ledger without recreating a deleted worker.
	if ps.Worktree != nil && ps.Worktree.Mode == "worktree" && !fileExists(ps.Worktree.Path) {
		ps.Worktree = nil
	}
	verifiedCandidate, err := c.candidateFingerprint(c.unitCwd(packet))
	if err != nil {
		return failPersistence("candidate", err.Error())
	}

	var commitNote string
	if ps.Worktree != nil && ps.Worktree.Mode == "worktree" {
		// 先验证分支持久 tree，再合并、提交主账本并验证主 tree，最后才清理。
		codeCommit := c.commitUnitChangesIn(ps.Worktree.Path, packet, c.ledgerExcludes(), "", "")
		ps.Commit = commitOutcomeMap(codeCommit)
		if codeCommit.Error != "" {
			return failPersistence("code-commit", codeCommit.Error)
		}
		codeHead, err := verify.PersistedHead(ps.Worktree.Path, c.ledgerExcludes())
		if err != nil {
			return failPersistence("code-tree", err.Error())
		}
		ps.Commit.(map[string]any)["sha"] = codeHead
		if !c.currentL1Evidence(packet) {
			return failPersistence("code-evidence", "commit hook changed the verified candidate")
		}
		// 合并门控（review 修正）：以“分支是否领先 base”为准（AheadCount），
		// 而不是“本次 scoped 提交是否产生了新提交”——executor 自提交、或崩溃
		// 恢复重跑 completeUnit 时 scoped 提交为空但分支领先，旧条件会静默跳过合并。
		ahead, aerr := verify.AheadCount(c.Cwd, c.baseRefOf(ps), ps.Worktree.Branch)
		if aerr != nil {
			ps.Commit.(map[string]any)["merge-error"] = aerr.Error()
			c.save()
			return fmt.Errorf("%w: ahead count failed: %v", ErrMergeConflict, aerr)
		}
		if ahead > 0 {
			if _, err := c.mergeUnitWorktree(ps.Worktree.Branch); err != nil {
				// §5.2 state.json 不说谎：合并失败绝不标 completed——worktree
				// 原样保留；先把失败事实落账（供人处理），再上抛升级。
				commitRec := map[string]any{
					"at":          c.nowISO(),
					"sha":         codeHead,
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
		} else {
			c.Log(fmt.Sprintf("%s: branch not ahead of base — nothing to merge", id))
		}
		ledgerCommit := c.commitUnitChangesIn(c.Cwd, packet, c.commitExcludes(), c.baseRefOf(ps), c.taskDirPrefix())
		ps.Commit = map[string]any{"sha": codeHead, "ledger-sha": jsonNilable(ledgerCommit.SHA), "at": c.nowISO()}
		if ledgerCommit.Error != "" {
			ps.Commit.(map[string]any)["ledger-error"] = ledgerCommit.Error
			return failPersistence("ledger-commit", ledgerCommit.Error)
		}
		finalSHA, err := verify.PersistedHead(c.Cwd, c.ledgerExcludes())
		if err != nil {
			return failPersistence("main-tree", err.Error())
		}
		mainCandidate, err := c.candidateFingerprint(c.Cwd)
		if err != nil {
			return failPersistence("main-candidate", err.Error())
		}
		if mainCandidate != verifiedCandidate {
			return failPersistence("main-candidate", "merged checkout differs from the verified candidate")
		}
		if !c.currentL1Evidence(packet) {
			return failPersistence("final-evidence", "candidate, contract, or report changed during persistence")
		}
		ps.Commit = map[string]any{
			"sha":        finalSHA,
			"message":    firstNonEmpty(codeCommit.Message, ledgerCommit.Message),
			"at":         c.nowISO(),
			"ledger-sha": jsonNilable(ledgerCommit.SHA),
		}
		if ps.Worktree.External {
			c.Log(fmt.Sprintf("%s: external worktree retained", id))
		} else if cleanupErr := c.removeUnitWorktree(ps.Worktree.Branch, ps.Worktree.Path); cleanupErr != nil {
			ps.Commit.(map[string]any)["cleanup-error"] = cleanupErr.Error()
			c.Log(fmt.Sprintf("%s: committed; worktree cleanup deferred: %v", id, cleanupErr))
		} else {
			ps.Worktree = nil
		}
		n := shortSHA(finalSHA)
		if ls := ledgerCommit.SHA; ls != "" {
			n += " + ledger " + shortSHA(ls)
		}
		commitNote = "commit " + n
	} else {
		// In-place 仍要求真实 Git commit；非 Git 演练不能声称交付持久化。
		outcome := c.commitUnitChangesIn(c.Cwd, packet, c.commitExcludes(), c.baseRefOf(ps), "")
		ps.Commit = commitOutcomeMap(outcome)
		if outcome.Error != "" {
			return failPersistence("code-commit", outcome.Error)
		}
		finalSHA, err := verify.PersistedHead(c.Cwd, c.ledgerExcludes())
		if err != nil {
			return failPersistence("code-tree", err.Error())
		}
		if !c.currentL1Evidence(packet) {
			return failPersistence("code-evidence", "commit hook changed the verified candidate")
		}
		ps.Commit.(map[string]any)["sha"] = finalSHA
		commitNote = fmt.Sprintf("commit %s", shortSHA(finalSHA))
	}
	ps.Status = "completed"
	ps.Parallel = false
	at := c.nowISO()
	ps.UpdatedAt = &at
	c.touchProgress()
	retroLine := fmt.Sprintf("- %s packet %s: completed (attempts=%d, failed=%d, %s)",
		c.nowISO(), id, ps.Attempts, ps.FailedAttempts, commitNote)
	c.appendRetro(retroLine)
	c.save()
	return c.err()
}

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
	if c.sharedActive(packet.PacketID) {
		// Batch completion is atomic, but sampling still uses each unit's
		// deterministic delivery position instead of sampling every peer as #1.
		for _, id := range c.State.SharedExecution.Packets {
			if id == packet.PacketID {
				break
			}
			if c.sharedActive(id) {
				completedCount++
			}
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
		Context:          c.RunContext,
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

	if outcome.Outcome == "pass" || outcome.Outcome == "fake-tests" || outcome.Outcome == "restore-failed" || outcome.Outcome == "inconclusive" {
		return &outcome, nil
	}
	// uninjectable / baseline-red：inbox 叫人，绝不指控假测试（P0-2）。
	return inboxContinue(
		fmt.Sprintf("defect injection for %s: %s — %s (human should refresh the patch / fix the suite)",
			packet.PacketID, outcome.Outcome, derefStringPtr(outcome.Detail)),
		ev)
}

// doneReportValid 仍有效的 done-report 是否在盘上（pause/resume 与中断恢复）。
func (c *Context) doneReportValid(packet *contract.Packet) bool {
	if evidence, ok := c.State.Packets[packet.PacketID].Evidence.Executor.(map[string]any); ok && evidence["source"] == "human-takeover" {
		identity, err := c.verificationIdentity(packet)
		return err == nil && evidence["identity"] == identity
	}

	reportAbs := filepath.Join(c.TaskDir, "reports", packet.PacketID+".done.json")
	raw, err := os.ReadFile(reportAbs)
	if err != nil {
		return false
	}
	if !donereport.Parse(raw, packet.DoneReportFormat).OK {
		return false
	}
	ex, ok := c.State.Packets[packet.PacketID].Evidence.Executor.(map[string]any)
	return ok && ex["report-sha256"] == fmt.Sprintf("%x", sha256.Sum256(raw))
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
	if oi.Outcome == "inconclusive" {
		reason = fmt.Sprintf("defect injection for %s was interrupted or incomplete; tests were not proven to detect the defect. Human needed.", id)
	}
	entry := map[string]any{"at": c.nowISO(), "packet-id": id, "reason": reason}
	c.State.Inbox = append(c.State.Inbox, entry)
	c.State.Status = "escalated"
	c.appendInbox(entry)
	c.appendRetro(fmt.Sprintf("- %v packet %s: INBOX — defect injection %s", entry["at"], id, oi.Outcome))
	c.save()
	return c.err()
}

// execLoopOutcome keeps execution bounded. Structured Quick risk discovery
// checkpoints a one-way upgrade; ordinary failures remain caller decisions.
type execLoopOutcome struct {
	Deferred bool // execution ended; shared-candidate acceptance waits for all workers
	Green    bool // L1 全绿：子环达标（done-condition 由冻结契约的验收命令构成）
	Stopped  bool // budget/凭证等待/fatal：跨环停止信号
	Bounded  bool // FailedAttempts 耗尽：有界停止（调用方裁决：升级或报告）
	Spun     bool // 空转检测命中：同签名连续失败（§6 DC4）
	Upgraded bool // structured executor risks switched the same task to Full
}

// runExecuteLoop execute 子环闭环（Osmani 对齐，v0.24）：转到单元 L1 验收
// 全绿，或有界停止。done-condition = 冻结契约的 L1 命令 + boundaries（铁律 4：
// 确定性 oracle，LLM 在其后）；每轮失败信号（错误码/失败命令/权威变更集）经
// failAttempt 落账并喂给下一轮执行调用（历史由会话策略管理）；界 = packet 的
// max-retries + 空转检测 + 预算闸门。凭证闸门在 executeStep 内（spawn 前）。
func (c *Context) runExecuteLoop(packet *contract.Packet, manualReason ...string) (execLoopOutcome, error) {
	reason := ""
	if len(manualReason) > 0 {
		reason = manualReason[0]
	}
	manualUsed := false
	if c.resumeRuntimeDecision(packet) == "spin" {
		return execLoopOutcome{Spun: true}, c.err()
	}
	for {
		if ferr := c.err(); ferr != nil {
			return execLoopOutcome{Stopped: true}, ferr
		}
		if !c.sessionReplay("execute", packet.PacketID) && c.budgetGate() == "stop" {
			return execLoopOutcome{Stopped: true}, nil
		}
		maxRetries := packet.Escalation.MaxRetries // 每轮重读：planner 可能修订它
		if manualUsed || (reason == "" && c.State.Packets[packet.PacketID].FailedAttempts >= maxRetries) {
			return execLoopOutcome{Bounded: true}, nil
		}
		if reason != "" {
			ps := c.State.Packets[packet.PacketID]
			ps.RetryHistory = append(ps.RetryHistory, map[string]any{"policy": "human-continue-once", "reason": reason, "at": c.nowISO(), "prior-failed-attempts": ps.FailedAttempts})
			c.save()
			if err := c.err(); err != nil {
				return execLoopOutcome{Stopped: true}, err
			}
			manualUsed = true
		}

		exec := execOutcome{Success: c.sharedActive(packet.PacketID) && c.State.Packets[packet.PacketID].Status == "executed" && c.doneReportValid(packet)}
		var err error
		if !exec.Success {
			exec, err = c.executeStep(packet)
		}
		if err != nil {
			return execLoopOutcome{Stopped: true}, err
		}
		if exec.Stopped {
			return execLoopOutcome{Stopped: true}, nil
		}
		if exec.Relocated {
			continue // no implementation or acceptance is claimed by a directory proposal
		}
		if !exec.Success {
			// A failed agent invocation has no completed acceptance observation.
			// Keep the bounded attempt exit; do not infer spin from file names.
			c.failAttempt(packet, orTransport(exec.ErrorCode), nil, c.authoritativeChangedFiles(packet))
			continue
		}
		if risks := c.executorWorkflowRisks(packet); c.quick() && len(risks) > 0 {
			err := c.upgradeWorkflow("Executor discovered risks requiring Full", risks)
			return execLoopOutcome{Upgraded: true}, err
		}

		if c.parallel != nil && c.sharedActive(packet.PacketID) {
			return execLoopOutcome{Deferred: true}, nil
		}
		if err := c.beginReviewPrewarm(packet); err != nil {
			return execLoopOutcome{Stopped: true}, err
		}
		l1 := c.l1Step(packet)
		if !l1.Pass {
			if err := c.closeReviewPrewarm("discarded: L1 did not pass"); err != nil {
				return execLoopOutcome{Stopped: true}, err
			}
			var failedCommands []string
			for _, r := range l1.Gate.Results {
				if !r.Pass {
					failedCommands = append(failedCommands, r.Cmd)
				}
			}
			for _, v := range l1.Boundary.Violations {
				if v.Path != "" {
					failedCommands = append(failedCommands, fmt.Sprintf("%s:%s", v.Type, v.Path))
				} else {
					failedCommands = append(failedCommands, fmt.Sprintf("%s:%d", v.Type, v.Count))
				}
			}
			authChanged := l1.Boundary.ChangedFiles
			if l1.Boundary.Skipped {
				authChanged = nil
			}
			if c.failAttempt(packet, errL1Failed, failedCommands, authChanged) == "spin" {
				return execLoopOutcome{Spun: true}, nil
			}
			continue
		}
		return execLoopOutcome{Green: true}, nil
	}
}

// Quick gathers injection with all other final checks in reviewStep, including
// manual review. Full keeps the original per-unit injection/pause semantics.
func (c *Context) unitReviewInjection(packet *contract.Packet) (*defectinjection.Outcome, error) {
	if c.quick() || c.sessionReplay("review", packet.PacketID) {
		return nil, nil
	}
	return c.maybeDefectInjection(packet)
}

// processPacket 单个 packet 的完整处理循环。
// P3 修复：resume 直达 review 时补跑缺陷注入采样（Node 直接跳过）。
func (c *Context) processPacket(packet *contract.Packet, opts processOpts) (outcome string, runErr error) {
	previousPrewarmPolicy := c.prewarmReview
	c.prewarmReview = opts.PauseBefore != "review" && c.parallel == nil && c.State.SharedExecution == nil
	defer func() {
		if err := c.closeReviewPrewarm("discarded: packet processing ended"); err != nil {
			runErr = errors.Join(runErr, err)
		}
		c.prewarmReview = previousPrewarmPolicy
	}()
	id := packet.PacketID
	ps := c.State.Packets[id]

	// P2：已升级到 inbox 的 packet 重跑时绝不重新 spawn 或重复追加 inbox 条目
	// ——修订任务契约后由引擎重新武装。凭证流程例外：人的修复就是
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
			c.Log(fmt.Sprintf("%s: already escalated to inbox — waiting for human (revise the task contract, then rerun; do not edit state.json)", id))
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
	if ps.Status == "l1-passed" && c.doneReportValid(packet) && c.currentL1Evidence(packet) {
		c.Log(fmt.Sprintf("%s: resuming at review (L1 green + done-report intact)", id))
		stage = "review"
		resumeAtReview = true
	}

	var pendingInjection *defectinjection.Outcome
	for {
		// escalate 裁决统一出口：review 退回累积 FailedAttempts 后回到循环顶，
		// 子环入口的界检查触发这里（与 Bounded/Spun 同一条升级路径）。
		escalate := func() (string, error) {
			esc, err := c.escalatePacket(packet)
			if err != nil {
				return "", err
			}
			if esc == "inbox" {
				return "inbox", nil
			}
			if p, ok := c.Packets[id]; ok { // P0-2：按修订后的契约跑
				packet = p
			}
			return esc, nil
		}
		if stage == "execute" {
			// v0.24：execute 子环闭环（runExecuteLoop）——大循环对出口裁决。
			r, err := c.runExecuteLoop(packet)
			if err != nil {
				return "", err
			}
			if r.Stopped {
				return "stop", nil
			}
			if r.Upgraded {
				return "upgrade", nil
			}
			if r.Bounded || r.Spun {
				esc, err := escalate()
				if err != nil {
					return "", err
				}
				if esc == "inbox" {
					return "stop", nil
				}
				if esc == "workflow-upgraded" {
					return "upgrade", nil
				}
				continue // 修订后契约 → 子环重入
			}
			// r.Green：L1 全绿，缺陷注入抽检在 L1 与 Review 之间（§4.3）。
			oi, err := c.unitReviewInjection(packet)
			if err != nil {
				return "", err
			}
			if oi != nil && (oi.Outcome == "fake-tests" || oi.Outcome == "restore-failed" || oi.Outcome == "inconclusive") {
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
			oi, err := c.unitReviewInjection(packet)
			if err != nil {
				return "", err
			}
			if oi != nil && (oi.Outcome == "fake-tests" || oi.Outcome == "restore-failed" || oi.Outcome == "inconclusive") {
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
			if opts.DeferCommit {
				return "verified", nil
			}
			if err := c.completeUnit(packet); err != nil {
				if errors.Is(err, ErrMergeConflict) || errors.Is(err, ErrCommitFailed) {
					// 合并冲突：worktree 已保留，升级到人（与其他升级同款
					// inbox 路由），任务状态不说谎。
					ps.Status = "inbox"
					at := c.nowISO()
					ps.UpdatedAt = &at
					entry := map[string]any{
						"at":        c.nowISO(),
						"packet-id": id,
						"reason":    fmt.Sprintf("packet %s: commit/merge failed — worktree preserved for human resolution (%v).", id, err),
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
		if c.quick() && verdict != nil && (verdict.Verdict == "upgrade" || !*verdict.QuickEligible) {
			return "upgrade", c.upgradeWorkflow("Final Review requires Full", verdict.Reasons)
		}
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
			if esc == "workflow-upgraded" {
				return "upgrade", nil
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
			if esc == "workflow-upgraded" {
				return "upgrade", nil
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
