// steps_plan.go — Plan 步骤与里程碑闸门（DESIGN §4.1/§4.4）：kimi 长驻会话
// 驱动、live 凭证审批等待、L2/L3 gate。
package machine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/verify"
)

// awaitCredentialsApproval 单元执行前的人工检查点：暂停等待审批文件
// （approvals/<packet-id>.json，{"approved-env": ["KEY", ...]}）。只有被批准的
// 键从编排器环境注入单元子进程，注入事实记录为证据。超时升级到 inbox。
func (c *Context) awaitCredentialsApproval(packet *contract.Packet) credsApproval {
	id := packet.PacketID
	ps := c.State.Packets[id]
	approvalFile := filepath.Join(c.TaskDir, "approvals", id+".json")
	pollMs := c.Config.Credentials.PollIntervalMs
	timeoutMs := c.Config.Credentials.TimeoutMs
	deadline := c.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	announced := ps.Status == "awaiting-credentials"

	for {
		if c.err() != nil {
			return credsApproval{}
		}
		if fileExists(approvalFile) {
			raw, err := os.ReadFile(approvalFile)
			var parsed struct {
				ApprovedEnv []any `json:"approved-env"`
			}
			if err != nil || json.Unmarshal(raw, &parsed) != nil {
				return credsApproval{} // Node: catch 分支 → 非 JSON 一律拒绝
			}
			keys := []any{}
			allStrings := true
			for _, k := range parsed.ApprovedEnv {
				s, ok := k.(string)
				if !ok {
					allStrings = false
					break
				}
				keys = append(keys, s)
			}
			if !allStrings {
				return credsApproval{}
			}
			injected := map[string]string{}
			missing := []any{}
			for _, k := range keys {
				key := k.(string)
				if v, present := c.EnvLookup(key); present {
					injected[key] = v
				} else {
					missing = append(missing, key)
				}
			}
			injectedList := []any{}
			for _, k := range keys {
				found := false
				for _, m := range missing {
					if m.(string) == k.(string) {
						found = true
						break
					}
				}
				if !found {
					injectedList = append(injectedList, k)
				}
			}
			missSuffix := ""
			if len(missing) > 0 {
				names := make([]string, len(missing))
				for i, m := range missing {
					names[i] = m.(string)
				}
				missSuffix = ", missing in env: " + strings.Join(names, ",")
			}
			ps.Evidence.Credentials = map[string]any{
				"approved-env": keys,
				"injected-env": injectedList,
				"missing-env":  missing,
				"source":       "approvals/" + id + ".json",
				"at":           c.nowISO(),
			}
			ps.Status = "pending"
			at := c.nowISO()
			ps.UpdatedAt = &at
			c.Log(fmt.Sprintf("%s: credentials approved by human (%d keys%s)", id, len(keys), missSuffix))
			c.save()
			return credsApproval{OK: true, Injected: injected}
		}

		if !announced {
			ps.Status = "awaiting-credentials"
			at := c.nowISO()
			ps.UpdatedAt = &at
			entry := map[string]any{
				"at":        c.nowISO(),
				"packet-id": id,
				"reason": fmt.Sprintf("packet %s declares needs-live-credentials: approve by writing docs task approvals/%s.json "+
					`{"approved-env": ["KEY", ...]}; the listed keys will be injected for this unit only.`, id, id),
			}
			c.State.Inbox = append(c.State.Inbox, entry)
			c.appendInbox(entry)
			c.Log(fmt.Sprintf("%s: awaiting live-credentials approval (%s)", id, approvalFile))
			c.save()
			announced = true
		}

		if !c.Now().Before(deadline) {
			ps.Status = "inbox"
			at := c.nowISO()
			ps.UpdatedAt = &at
			entry := map[string]any{
				"at":        c.nowISO(),
				"packet-id": id,
				"reason":    fmt.Sprintf("credentials approval for %s timed out after %dms. Human needed.", id, timeoutMs),
			}
			c.State.Inbox = append(c.State.Inbox, entry)
			c.State.Status = "escalated"
			c.appendInbox(entry)
			c.save()
			return credsApproval{}
		}

		c.Sleep(time.Duration(pollMs) * time.Millisecond)
	}
}

// milestoneGateStep 里程碑全部 packet 的 L2/L3 命令并集。
// （Node 给每条命令附注 _packet 供展示，下游从不读取——Go 版省略。）
func (c *Context) milestoneGateStep(milestone contract.Milestone) gateOutcome {
	var packets []*contract.Packet
	for _, p := range c.PacketList {
		if m, err := contract.MilestoneOfPacket(p.PacketID); err == nil && m == milestone.ID {
			packets = append(packets, p)
		}
	}
	timeoutMs := c.Config.Verify.CommandTimeoutMs
	envAllowlist := c.Config.Verify.EnvAllowlist
	levels := map[string]any{}
	beforeIdentity, beforeErr := c.candidateFingerprint(c.Cwd)
	pass := beforeErr == nil
	for _, level := range []string{"L2", "L3"} {
		var commands []contract.GateEntry
		for _, p := range packets {
			commands = append(commands, p.Acceptance.Level(level)...)
		}
		if len(commands) == 0 {
			levels[level] = map[string]any{"pass": true, "results": []any{}, "skipped": true}
			continue
		}
		if beforeErr != nil {
			levels[level] = map[string]any{"pass": false, "results": []any{}, "skipped": true, "identity-error": beforeErr.Error()}
			continue
		}
		gate := verify.RunGateCommands(commands, verify.GateOpts{
			Context: c.RunContext,
			Cwd:     c.Cwd, TimeoutMS: timeoutMs, EnvAllowlist: envAllowlist,
		})
		levels[level] = gate
		if !gate.Pass {
			pass = false
		}
	}
	identity, identityErr := c.candidateFingerprint(c.Cwd)
	if beforeErr != nil {
		identityErr = fmt.Errorf("before milestone acceptance: %w", beforeErr)
	} else if identityErr != nil {
		identityErr = fmt.Errorf("after milestone acceptance: %w", identityErr)
	} else if identity != beforeIdentity {
		identityErr = fmt.Errorf("candidate changed during milestone acceptance; rerun verification on the resulting candidate")
	}
	if identityErr != nil {
		pass = false
	}
	ms := c.State.Milestones[milestone.ID]
	ms.Gate = map[string]any{
		"pass": pass, "levels": levels, "at": c.nowISO(),
		"identity": identity, "identity-before": beforeIdentity,
		"identity-error": jsonNilable(errorText(identityErr)),
	}
	c.State.Milestones[milestone.ID] = ms
	c.save()
	// 同 l1Step：无错误产生路径，删除防御性 error 返回（2026-08-23）。
	return gateOutcome{Pass: pass, Levels: levels}
}

// planStep 经长驻 kimi code 会话为里程碑签发/确认 packet 序列。会话纪律：
//   - 首调建会话，从流中捕获 id 存进 state.json（plan-session）；后续用
//     `kimi -r <id>` 续；
//   - 会话死亡（resume 失败/看门狗击杀挂起）触发冷启动协议：从契约层
//     （spec + state + retro 尾部）启动全新会话——契约层是唯一记忆；
//   - provider 失败走配置的渠道降级链；
//   - 所有渠道都失败则任务升级到 inbox（Plan 是定方向的，没了它继续跑没意义）。
func (c *Context) planStep(milestone contract.Milestone, feedback string) (planOutcome, error) {
	packets := []*contract.Packet{}
	for _, p := range c.PacketList {
		if m, err := contract.MilestoneOfPacket(p.PacketID); err == nil && m == milestone.ID {
			packets = append(packets, p)
		}
	}
	if len(packets) == 0 {
		return planOutcome{}, fmt.Errorf("milestone %s has no packets", milestone.ID)
	}
	binding, err := fallback.ResolveBinding(c.Config, "plan", "")
	if err != nil {
		return planOutcome{}, err
	}
	maxTransport := c.Config.Retry.MaxTransportRetries

	retroTail := func() string {
		raw, err := os.ReadFile(filepath.Join(c.TaskDir, "retro.md"))
		if err != nil {
			return ""
		}
		lines := strings.Split(string(raw), "\n")
		if len(lines) > 40 {
			lines = lines[len(lines)-40:]
		}
		return strings.Join(lines, "\n")
	}

	attempt := func(sessionID string, coldStart bool, channel, model, harness, provider string) (drivers.Result, errs.ErrorCode) {
		tail := ""
		if coldStart {
			tail = retroTail()
		}
		prompt := prompts.BuildPlanPrompt(prompts.PlanArgs{
			Spec:      c.Spec,
			State:     c.State,
			Milestone: milestone,
			Packets:   packets,
			ColdStart: coldStart,
			RetroTail: tail,
			Feedback:  feedback,
		})
		res := c.Drivers.Plan.Plan(drivers.CallArgs{
			Prompt:         prompt,
			MilestoneID:    milestone.ID,
			TaskDir:        c.TaskDir,
			Cwd:            c.Cwd,
			PermissionMode: c.Config.PermissionMode("plan", harness),
			SessionID:      sessionID,
			Model:          model,
			Binding: config.LoopBinding{
				Harness: harness, Provider: provider, Model: model, Channel: channel, Thinking: binding.Thinking,
			},
			APIKey:   c.channelApiKey(channel),
			APIKeys:  c.apiKeysFor(channel),
			Watchdog: c.wdog(),
		})
		c.recordSpawnMetrics(spawnMetricsArgs{
			role: "plan", model: model, channel: channel,
			prompt: prompt, result: &res,
		})
		msg := ""
		if res.ErrorMessage != nil {
			msg = *res.ErrorMessage
		}
		var code errs.ErrorCode
		if msg != "" && !res.TimedOut && !res.Cancelled && res.SpawnError == "" && res.CleanupError == "" {
			code = errs.ClassifyError(errs.SpawnResult{ExitCode: 1, Stderr: msg})
		} else if res.Settled && res.ExitCode != nil && *res.ExitCode == 0 && !res.TimedOut && !res.Cancelled && !res.Truncated && res.Signal == "" && res.CleanupError == "" && res.SpawnError == "" {
			code = ""
		} else {
			code = errs.ClassifyError(toSpawnResult(res))
		}
		return res, code
	}

	channel := binding.Channel
	model, harness := binding.Model, binding.Harness
	provider := fallback.ResolveProvider(c.Config, harness, binding.Provider, channel)
	resident := c.Config.Loops["plan"].ResidentSession && harness == "kimi"
	permission := c.Config.PermissionMode("plan", harness)
	// Resume only an explicitly matching model, channel, harness, directory,
	// effort and permission policy. Older incomplete identities cold-start once.
	var stored map[string]any
	if m, ok := c.State.PlanSession.(map[string]any); ok {
		stored = m
	}
	sessionID := ""
	if resident && stored != nil {
		sc, _ := stored["channel"].(string)
		sm, _ := stored["model"].(string)
		if sc == channel && sm == model && stored["harness"] == harness &&
			stored["cwd"] == c.Cwd && stored["thinking"] == binding.Thinking && stored["permission-mode"] == permission {
			sessionID, _ = stored["session-id"].(string)
		}
	}
	if stored != nil && sessionID == "" {
		sc, _ := stored["channel"].(string)
		sm, _ := stored["model"].(string)
		c.Log(fmt.Sprintf("stored plan session (%s/%s) mismatches current binding (%s/%s) — cold start", sc, sm, channel, model))
	}
	visited := map[string]bool{channel: true}

	var result drivers.Result
	var code errs.ErrorCode
	transportRetries := 0
	for {
		if ferr := c.err(); ferr != nil {
			return planOutcome{}, ferr
		}
		if c.budgetGate() == "stop" {
			return planOutcome{Stopped: true, Packets: packets}, nil
		}
		// 宁可 fail-loud 也不要无 key spawn（P1-12②）。
		if missingKey := c.missingKeyForRealDriver("plan", channel); missingKey != "" {
			c.Log(fmt.Sprintf("plan: fail-loud — channel %s requires env %s (not set)", channel, missingKey))
			return planOutcome{ErrorCode: errs.AUTH, Packets: packets}, nil
		}
		result, code = attempt(sessionID, sessionID == "", channel, model, harness, provider)
		if err := c.stopForUnconfirmedCleanup("plan", "", result); err != nil {
			return planOutcome{}, err
		}
		if code == "" {
			// C5（v0.18）：调用成功却没拿到 session id——上游 resume_hint 格式漂移
			// 或缺失，长驻会话将无声退化为每轮冷启动。必须留观测痕迹。
			if resident && result.SessionID == "" {
				c.Log("plan: successful call returned no session id (resume_hint missing?) — resident session degraded")
			}
			break
		}

		// 会话死亡 → 冷启动协议（§4.1）：全新会话，契约记忆。
		if sessionID != "" {
			c.Log(fmt.Sprintf("plan session %s is dead (%s) — cold restart with a fresh session", sessionID, code))
			sessionID = ""
			continue
		}
		// 渠道降级：配额/限流，外加有带 key 兜底在等的订阅渠道 AUTH（P0-7）。
		if errs.ShouldFallback(code) || fallback.AuthFallbackAllowed(c.Config, channel, code) {
			next := fallback.NextChannel(c.Config, channel)
			var mapped *fallback.ChannelModel
			if next != "" {
				mapped = fallback.ModelForChannel(c.Config, next, "plan", "")
			}
			if next != "" && !visited[next] && mapped != nil {
				visited[next] = true
				c.Log(fmt.Sprintf("plan fallback: %s -> %s (%s)", channel, next, code))
				channel = next
				model = mapped.Model
				if mapped.Harness != "" {
					harness = mapped.Harness
				}
				provider = fallback.ResolveProvider(c.Config, harness, "", channel)
				resident = c.Config.Loops["plan"].ResidentSession && harness == "kimi"
				sessionID = "" // 新渠道开新会话
				continue
			}
			if next != "" && mapped == nil {
				c.Log(fmt.Sprintf("plan fallback chain ends: no model mapping for plan on %s (P1-12①)", next))
			}
		}
		// 瞬态失败退避重试，而不是直奔 inbox。
		if errs.IsRetryable(code) && transportRetries < maxTransport {
			delay := errs.ComputeBackoff(transportRetries, errs.BackoffOptions{
				BaseDelayMs:  c.Config.Retry.BaseDelayMs,
				MaxDelayMs:   c.Config.Retry.MaxDelayMs,
				Jitter:       c.Config.Retry.Jitter,
				RetryAfterMs: float64(errs.ExtractRetryAfterMs(result.Stderr + "\n" + result.Stdout)),
			})
			transportRetries++
			c.Log(fmt.Sprintf("plan transport retry %d/%d after %s in %dms", transportRetries, maxTransport, code, delay))
			c.Sleep(time.Duration(delay) * time.Millisecond)
			continue
		}
		break
	}

	if code != "" {
		c.Log(fmt.Sprintf("plan step failed on every channel (last: %s)", code))
		return planOutcome{ErrorCode: code, Packets: packets}, nil
	}

	// 捕获/刷新长驻会话句柄（model+channel 钉死，铁律 7）。
	if resident && result.SessionID != "" {
		var prev map[string]any
		if m, ok := c.State.PlanSession.(map[string]any); ok {
			prev = m
		}
		prevSame := false
		if prev != nil {
			pid, _ := prev["session-id"].(string)
			prevSame = pid == result.SessionID
		}
		createdAt := c.nowISO()
		resumes := 0
		if prevSame {
			if ca, ok := prev["created-at"].(string); ok && ca != "" {
				createdAt = ca
			}
			if r, ok := prev["resumes"].(float64); ok {
				resumes = int(r)
			}
		}
		if sessionID != "" {
			resumes++
		}
		c.State.PlanSession = map[string]any{
			"session-id":      result.SessionID,
			"channel":         channel,
			"model":           model,
			"harness":         harness,
			"cwd":             c.Cwd,
			"thinking":        binding.Thinking,
			"permission-mode": c.Config.PermissionMode("plan", harness),
			"created-at":      createdAt,
			"updated-at":      c.nowISO(),
			"resumes":         resumes,
		}
		c.save()
	} else if !resident && c.State.PlanSession != nil {
		c.State.PlanSession = nil
		c.save()
	}
	return planOutcome{OK: true, Packets: packets, Result: &result}, nil
}

// planUntilValid plan 子环闭环（Osmani 对齐，v0.24）：done-condition =
// load-time 校验通过的合法契约集。planner 写出的文件不合法时，校验错误经
// Feedback 喂回长驻会话重试（planner 是修订器，修自己的产物是本职）；界 =
// planValidationRetries 次后 inbox 叫人（契约质量问题是结构性的，重试烧钱
// 无意义）。传输级失败仍由 planStep 内部闭环处理。
func (c *Context) planUntilValid(milestone contract.Milestone, initialFeedback ...string) (planOutcome, error) {
	var feedback string
	if len(initialFeedback) > 0 {
		feedback = initialFeedback[0]
	}
	for round := 0; round <= planValidationRetries; round++ {
		before, err := c.planScopeIdentity()
		if err != nil {
			return planOutcome{}, err
		}
		r, err := c.planStep(milestone, feedback)
		if err != nil {
			return r, err
		}
		after, err := c.planScopeIdentity()
		if err != nil {
			return r, err
		}
		if before != after {
			return r, fmt.Errorf("Plan changed implementation, external acceptance, or configuration outside spec/packet contracts; changes preserved, automatic commit refused")
		}
		if !r.OK || r.Stopped {
			return r, nil // 传输/预算在 planStep 内已有界，原样上抛
		}
		old := c.PacketList
		if err := c.reloadPackets(); err == nil {
			c.recordAcceptanceChanges(old, c.PacketList)
			if round > 0 {
				c.Log(fmt.Sprintf("plan loop: contract set valid after %d feedback round(s)", round))
			}
			return r, nil // 达标：合法契约集
		} else {
			feedback = err.Error()
			c.Log(fmt.Sprintf("plan loop round %d: contract validation failed — feeding errors back to planner", round+1))
		}
	}
	// 界尽：inbox 叫人（与 plan 全渠道失败同款出口，§6）。
	c.State.Status = "escalated"
	entry := map[string]any{
		"at":        c.nowISO(),
		"packet-id": nil,
		"milestone": milestone.ID,
		"reason": fmt.Sprintf("planner produced invalid contracts %d times (validation errors fed back each round). Human needed.",
			planValidationRetries+1),
	}
	c.State.Inbox = append(c.State.Inbox, entry)
	c.appendInbox(entry)
	c.save()
	return planOutcome{Packets: c.PacketList}, nil
}

// planValidationRetries plan 子环的校验反馈重试上限（v0.24 定界）。
const planValidationRetries = 2

var inspectPlanDirtyFiles = verify.GetChangedFiles

// persistPlanContracts 保留正在进行的实现，只把已校验的契约修订快进到其
// worktree。冲突/提交失败都保留文件与分支；不 reset，不 force remove。
func (c *Context) persistPlanContracts(message string) error {
	before, err := c.candidateFingerprint(c.Cwd)
	if err != nil {
		return fmt.Errorf("%w: cannot snapshot plan checkpoint: %v", ErrCommitFailed, err)
	}
	previousHead, _ := verify.GetBaseRef(c.Cwd)
	committed := c.commitTaskDirChanges(message)
	if committed.Error != "" {
		return fmt.Errorf("%w: plan contracts: %s", ErrCommitFailed, committed.Error)
	}
	after, err := c.candidateFingerprint(c.Cwd)
	if err != nil || before != after {
		return fmt.Errorf("%w: plan commit changed the loaded candidate or contracts; reload and revalidate before proceeding (%v)", ErrCommitFailed, err)
	}
	head, ok := verify.GetBaseRef(c.Cwd)
	if !ok {
		return nil
	}
	// Hooks run during commit. They may change only the index, leaving the
	// working-tree fingerprint unchanged; reject unexpected committed paths and
	// any contract content that differs from the exact HEAD sent to worktrees.
	if err := c.checkPlanReanchor(previousHead, head); err != nil {
		return fmt.Errorf("%w: plan checkpoint contains unexpected changes: %v", ErrMergeConflict, err)
	}
	changed, inspected := inspectPlanDirtyFiles(c.Cwd, head)
	if !inspected {
		return fmt.Errorf("%w: cannot verify persisted plan contracts", ErrCommitFailed)
	}
	for _, rel := range changed {
		if c.isPlannerContract(rel) || rel == c.taskDirPrefix()+"flywheel.config.json" {
			return fmt.Errorf("%w: plan contract %s differs from committed checkpoint; worktree preserved", ErrCommitFailed, rel)
		}
	}
	for id, ps := range c.State.Packets {
		if ps.Status == "completed" || ps.BaseRef == nil || !ps.ContractSyncPending {
			continue
		}
		if err := c.checkPlanReanchor(*ps.BaseRef, head); err != nil {
			return fmt.Errorf("%w: packet %s: %v", ErrMergeConflict, id, err)
		}
		if ps.Worktree != nil && ps.Worktree.Mode == "worktree" {
			if _, err := verify.MergeUnitWorktree(ps.Worktree.Path, head); err != nil {
				ps.Commit = map[string]any{"plan-sync-error": err.Error(), "at": c.nowISO()}
				c.save()
				return fmt.Errorf("%w: packet %s plan revision; worktree preserved: %v", ErrMergeConflict, id, err)
			}
		}
		sha := head
		ps.BaseRef = &sha
		ps.ContractSyncPending = false
		invalidateEvidence(ps)
	}
	return nil
}
