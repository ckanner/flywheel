package machine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/donereport"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/exitcontract"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// ---- 小工具 ----

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// toSpawnResult driver 结果 → 错误分类输入（Node 直接传 result 对象等价）。
func toSpawnResult(r drivers.Result) errs.SpawnResult {
	sr := errs.SpawnResult{Signal: r.Signal, TimedOut: r.TimedOut, Stderr: r.Stderr, Stdout: r.Stdout}
	if r.ExitCode != nil {
		sr.ExitCode = *r.ExitCode
	}
	if r.SpawnError != "" {
		sr.SpawnError = errors.New(r.SpawnError)
	}
	return sr
}

func (c *Context) wdog() watchdog.Opts {
	return watchdog.Opts{
		IdleTimeoutMs: c.Config.Watchdog.IdleTimeoutMs,
		WallTimeoutMs: c.Config.Watchdog.WallTimeoutMs,
		KillGraceMs:   c.Config.Watchdog.KillGraceMs,
	}
}

// appendFile Node appendFileSync 等价（不存在则创建）。
func appendFile(path, content string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(content)
}

func (c *Context) appendInbox(entry map[string]any) {
	who := entry["packet-id"]
	if who == nil {
		who = "task"
	}
	appendFile(filepath.Join(c.TaskDir, "inbox.md"),
		fmt.Sprintf("\n## %s — %s\n\n%s\n", entry["at"], who, entry["reason"]))
}

func (c *Context) appendRetro(line string) {
	appendFile(filepath.Join(c.TaskDir, "retro.md"), line+"\n")
}

func (c *Context) baseRefOf(ps *state.PacketState) string {
	if ps.BaseRef == nil {
		return ""
	}
	return *ps.BaseRef
}

// orHEAD Node `ps['base-ref'] ?? 'HEAD'` 的展示回退。
func orHEAD(ref string) string {
	if ref == "" {
		return "HEAD"
	}
	return ref
}

// ---- 凭证人工确认（§5.1 needs-live-credentials / §9）----

type credsApproval struct {
	OK       bool
	Injected map[string]string
}

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

// ---- Execute 步骤（传输重试 + 渠道降级 §6）----

type execOutcome struct {
	Success   bool
	Stopped   bool
	Awaiting  string // "credentials"：等人批凭证，非失败
	ErrorCode errs.ErrorCode
}

// executeStep 成功时 packet 状态变为 executed 并记录 executor 证据。
func (c *Context) executeStep(packet *contract.Packet) (execOutcome, error) {
	id := packet.PacketID
	ps := c.State.Packets[id]

	// live 凭证闸门：任何 spawn 前先等人批准（§9）。
	credentialEnv := map[string]string{}
	if packet.NeedsLiveCredentials {
		if ps.Evidence.Credentials != nil {
			// 此前已批准（resume 场景）：仅重注入当时批准过的键。
			if cred, ok := ps.Evidence.Credentials.(map[string]any); ok {
				if list, ok := cred["injected-env"].([]any); ok {
					for _, k := range list {
						if key, ok := k.(string); ok {
							if v, present := c.EnvLookup(key); present {
								credentialEnv[key] = v
							}
						}
					}
				}
			}
		} else {
			approval := c.awaitCredentialsApproval(packet)
			if !approval.OK {
				return execOutcome{Stopped: true, Awaiting: "credentials"}, nil
			}
			credentialEnv = approval.Injected
		}
	}

	ps.Status = "executing"
	ps.Attempts++
	// 边界判定锚（§4.2/§4.4）：只在第一次尝试采样——重试保留原 base ref，
	// executor 无法靠提交自己的早期尝试逃过判定（审计修复 #5）。
	if ps.BaseRef == nil {
		if ref, ok := verify.GetBaseRef(c.Cwd); ok {
			ps.BaseRef = &ref
		}
	}
	if ps.Worktree != nil && ps.Worktree.Mode == "worktree" && !fileExists(ps.Worktree.Path) {
		ps.Worktree = nil // 完成时已删除——按需重建
	}
	if ps.BaseRef != nil && ps.Worktree == nil {
		if c.Config.Worktree.Enabled == nil || *c.Config.Worktree.Enabled {
			wt := verify.EnsureUnitWorktree(c.Cwd, c.State.Slug, id, *ps.BaseRef)
			ps.Worktree = &wt
			if wt.Mode == "in-place" {
				c.Log(fmt.Sprintf("%s: worktree unavailable (%s) — running in-place", id, wt.Reason))
			}
		}
		if c.unitCwd(packet) == c.Cwd {
			c.WarnIfForeignDirty()
		}
	}
	at := c.nowISO()
	ps.UpdatedAt = &at
	c.save()

	uCwd := c.unitCwd(packet)
	binding, err := fallback.ResolveBinding(c.Config, "execute", packet.Route)
	if err != nil {
		return execOutcome{}, err
	}
	channel := binding.Channel
	currentModel, currentHarness := binding.Model, binding.Harness
	visited := map[string]bool{}
	maxTransport := c.Config.Retry.MaxTransportRetries
	lastCode := errs.TRANSPORT

	// 出口契约必须裁决“这一次”的交付物——上次尝试遗留的陈旧 done-report 会把
	// 无产出的重试读成假成功。
	reportRel := filepath.ToSlash(filepath.Join("reports", id+".done.json"))
	_ = os.Remove(filepath.Join(c.TaskDir, reportRel))

	for t := 0; t <= maxTransport; t++ {
		if ferr := c.err(); ferr != nil {
			return execOutcome{}, ferr
		}
		if c.budgetGate() == "stop" {
			return execOutcome{Stopped: true}, nil
		}
		// 宁可 fail-loud 也不要把无 key 的 spawn 送进 pi 的 auth.json 陷阱（P1-12②）。
		if missingKey := c.missingKeyForRealDriver("execute", channel); missingKey != "" {
			c.recordRetry(ps, "transport", string(errs.AUTH),
				fmt.Sprintf("fail-loud: channel %s requires env %s (not set)", channel, missingKey))
			c.save()
			return execOutcome{ErrorCode: errs.AUTH}, nil
		}
		prompt := prompts.BuildExecutePrompt(packet, c.Spec, c.TaskDir, uCwd, ps.Attempts)
		visited[channel] = true
		apiKeys := c.apiKeysFor(channel)
		for k, v := range credentialEnv {
			apiKeys[k] = v
		}
		res := c.Drivers.Execute.Execute(drivers.CallArgs{
			Prompt:   prompt,
			PacketID: id,
			TaskDir:  c.TaskDir,
			Cwd:      uCwd,
			Model:    currentModel,
			Binding: config.LoopBinding{
				Harness: currentHarness, Model: currentModel,
				Channel: channel, Thinking: binding.Thinking,
			},
			APIKey:   c.channelApiKey(channel),
			APIKeys:  apiKeys,
			Watchdog: c.wdog(),
		})

		// 交付物 = done-report，存在且 schema 合法。
		reportAbs := filepath.Join(c.TaskDir, reportRel)
		raw, readErr := os.ReadFile(reportAbs)
		reportExists := readErr == nil
		rep := donereport.Result{Errors: []string{"missing"}}
		if reportExists {
			rep = donereport.Parse(raw, packet.DoneReportFormat)
		}
		// 事件流内的 provider 错误（pi 遇 401/403 照样 exit 0）优先于出口契约。
		streamMsg := ""
		if res.ErrorMessage != nil {
			streamMsg = *res.ErrorMessage
		}
		var contractRes exitcontract.Result
		if streamMsg != "" {
			streamCode := errs.ClassifyError(errs.SpawnResult{ExitCode: 1, Stderr: streamMsg})
			contractRes = exitcontract.Result{
				OK: false, ErrorCode: string(streamCode),
				Reasons: []string{fmt.Sprintf("stream error: %s", streamMsg)},
			}
		} else {
			detail := ""
			if !rep.OK {
				detail = strings.Join(rep.Errors, ",")
			}
			contractRes = exitcontract.Evaluate(toSpawnResult(res), []exitcontract.Deliverable{{
				Path: reportRel, Exists: reportExists, Valid: rep.OK, Detail: detail,
			}})
		}

		ps.Exit = &state.ExitInfo{
			ExitCode:     res.ExitCode,
			Signal:       strPtr(res.Signal),
			TimedOut:     res.TimedOut,
			IdleTimedOut: res.IdleTimedOut,
			WallTimedOut: res.WallTimedOut,
		}
		// exitcontract.Evaluate 不变量：!OK ⇒ ErrorCode 非空（进程失败→分类码、
		// 交付物缺失→EMPTY_RESPONSE、stderr 污染→分类码）——空码兜底链为死代码
		// （覆盖率审计 2026-08-22 删除）。
		code := contractRes.ErrorCode
		if code != "" {
			lastCode = errs.ErrorCode(code)
		}
		attemptNo := ps.Attempts
		c.recordSpawnMetrics(spawnMetricsArgs{
			role: "execute", packetID: id, model: currentModel,
			channel: channel, prompt: prompt, result: &res,
			errorCode: errs.ErrorCode(code), attempt: &attemptNo,
		})

		if contractRes.OK {
			ps.Status = "executed"
			ps.Interrupted = false
			ps.Evidence.Executor = map[string]any{
				"done-report":          reportRel,
				"requirements-covered": rep.Report["requirements-covered"],
				"changed-files":        rep.Report["changed-files"],
				"failures":             rep.Report["failures"],
				"at":                   c.nowISO(),
			}
			if !containsString(ps.Artifacts, reportRel) {
				ps.Artifacts = append(ps.Artifacts, reportRel)
			}
			at := c.nowISO()
			ps.UpdatedAt = &at
			c.touchProgress()
			c.save()
			return execOutcome{Success: true}, nil
		}

		// Evaluate 不变量：!OK ⇒ Reasons 至少一条——空串兜底为死代码（同上删除）。
		reasons := strings.Join(contractRes.Reasons, "; ")
		c.Log(fmt.Sprintf("execute %s attempt %d transport %d: %s (%s)", id, ps.Attempts, t, code, reasons))

		// 渠道降级：限流/配额（403）→ 下一配置渠道；AUTH 仅当订阅渠道有带 key 的
		// 兜底在等（P0-7）。
		if errs.ShouldFallback(errs.ErrorCode(code)) || fallback.AuthFallbackAllowed(c.Config, channel, errs.ErrorCode(code)) {
			next := fallback.NextChannel(c.Config, channel)
			var mapped *fallback.ChannelModel
			if next != "" {
				mapped = fallback.ModelForChannel(c.Config, next, "execute", packet.Route)
			}
			if next != "" && !visited[next] && mapped != nil {
				c.recordRetry(ps, "fallback", code, fmt.Sprintf("channel %s -> %s", channel, next))
				c.Log(fmt.Sprintf("fallback: %s -> %s (%s)", channel, next, code))
				channel = next
				currentModel = mapped.Model
				if mapped.Harness != "" {
					currentHarness = mapped.Harness
				}
				t-- // 降级不消耗传输重试
				c.save()
				continue
			}
			if next != "" && mapped == nil {
				c.Log(fmt.Sprintf("fallback chain ends: no model mapping for execute/%s on %s (P1-12①)", packet.Route, next))
			}
		}

		if !errs.IsRetryable(errs.ErrorCode(code)) {
			c.recordRetry(ps, "transport", code, "no-retry")
			c.save()
			return execOutcome{ErrorCode: errs.ErrorCode(code)}, nil
		}

		delay := errs.ComputeBackoff(t, errs.BackoffOptions{
			BaseDelayMs:  c.Config.Retry.BaseDelayMs,
			MaxDelayMs:   c.Config.Retry.MaxDelayMs,
			Jitter:       c.Config.Retry.Jitter,
			RetryAfterMs: float64(errs.ExtractRetryAfterMs(res.Stderr + "\n" + res.Stdout)),
		})
		c.recordRetry(ps, "transport", code, "backoff", delay)
		c.save()
		c.Sleep(time.Duration(delay) * time.Millisecond)
	}

	return execOutcome{ErrorCode: lastCode}, nil
}

// ---- L1 步骤（§4.4）----

type l1Outcome struct {
	Pass     bool
	Gate     verify.GateReport
	Boundary verify.BoundaryReport
}

// l1Step 编排器直接跑验收命令，再以 git diff 判定测试/边界完整性。
// 覆盖率闸门只是 packet 里的一条 L1 命令，此处无特判。worktree 模式下闸门与
// diff 在单元 worktree 内跑；主 checkout 另做契约防篡改扫描（executor 知道
// taskDir 绝对路径——它的账本脏文件被排除，任务目录外的用户 WIP 不理睬）。
func (c *Context) l1Step(packet *contract.Packet) l1Outcome {
	id := packet.PacketID
	ps := c.State.Packets[id]
	timeoutMs := c.Config.Verify.CommandTimeoutMs
	envAllowlist := c.Config.Verify.EnvAllowlist
	uCwd := c.unitCwd(packet)
	gate := verify.RunGateCommands(packet.Acceptance.L1, verify.GateOpts{
		Cwd: uCwd, TimeoutMS: timeoutMs, EnvAllowlist: envAllowlist,
	})
	changed, _ := verify.GetChangedFiles(uCwd, c.baseRefOf(ps))
	executorFiles, contractViolations := c.partitionChangedFiles(changed)
	if ps.Worktree != nil && ps.Worktree.Mode == "worktree" {
		prefix := c.taskDirPrefix()
		mainFiles, _ := verify.GetChangedFiles(c.Cwd, c.baseRefOf(ps))
		for _, f := range mainFiles {
			if strings.HasPrefix(f, prefix) && contractFileRe.MatchString(f[len(prefix):]) {
				contractViolations = append(contractViolations, boundaryViolationLite{Type: "contract-file", Path: f})
			}
		}
	}
	boundary := verify.CheckBoundaries(executorFiles, packet.Boundaries, c.Config.Verify.TestFilePatterns)
	if len(contractViolations) > 0 {
		// Executor 动了契约层本身（spec/packets/config）——直接判失败。
		for _, v := range contractViolations {
			boundary.Violations = append(boundary.Violations, verify.BoundaryViolation{Type: v.Type, Path: v.Path})
		}
		boundary.Pass = false
	}
	pass := gate.Pass && boundary.Pass
	ps.Evidence.L1 = map[string]any{
		"pass":     pass,
		"commands": gate.Results,
		"boundary": boundary,
		"at":       c.nowISO(),
	}
	if pass {
		ps.Status = "l1-passed"
	} else {
		ps.Status = "l1-failed"
	}
	at := c.nowISO()
	ps.UpdatedAt = &at
	if pass {
		c.touchProgress()
	}
	c.save()
	// 错误返回恒为 nil（函数体无任何产生路径）——覆盖率审计 2026-08-23 删除
	// 防御性 error 返回。
	return l1Outcome{Pass: pass, Gate: gate, Boundary: boundary}
}

// ---- LLM 裁决循环（review / judge 共用）----

type verdictArgs struct {
	role      string // "review" | "judge"
	binding   fallback.Binding
	prompt    string
	packetID  string
	attemptNo *int
	ps        *state.PacketState // 可空——里程碑级 judge 时重试只进 metrics
}

type verdictOutcome struct {
	OK        bool
	Verdict   *prompts.Verdict
	Stopped   bool
	ErrorCode errs.ErrorCode
}

// agentVerdictStep LLM 裁决角色的共用循环：传输重试 + 渠道降级 +
// 末行 JSON 裁决解析。事件流内的 provider 错误（pi 遇 401/403 exit 0）
// 优先于解析。ps 为空时重试记录只落 metrics。
func (c *Context) agentVerdictStep(a verdictArgs) (verdictOutcome, error) {
	channel := a.binding.Channel
	currentModel, currentHarness := a.binding.Model, a.binding.Harness
	visited := map[string]bool{}
	maxTransport := c.Config.Retry.MaxTransportRetries
	lastCode := errs.TRANSPORT

	call := func() drivers.Result {
		args := drivers.CallArgs{
			TaskDir: c.TaskDir,
			Cwd:     c.Cwd,
			Prompt:  a.prompt,
			Model:   currentModel,
			Binding: config.LoopBinding{
				Harness: currentHarness, Model: currentModel,
				Channel: channel, Thinking: a.binding.Thinking,
			},
			APIKey:   c.channelApiKey(channel),
			APIKeys:  c.apiKeysFor(channel),
			Watchdog: c.wdog(),
		}
		switch a.role {
		case "review":
			return c.Drivers.Review.Review(args)
		default: // judge（当前仅此两角色走裁决循环）
			return c.Drivers.Judge.Judge(args)
		}
	}

	record := func(scope, code, policy string, plannedDelayMS ...int64) {
		if a.ps != nil {
			c.recordRetry(a.ps, scope, code, policy, plannedDelayMS...)
		}
	}

	for t := 0; t <= maxTransport; t++ {
		if ferr := c.err(); ferr != nil {
			return verdictOutcome{}, ferr
		}
		if c.budgetGate() == "stop" {
			return verdictOutcome{Stopped: true}, nil
		}
		if missingKey := c.missingKeyForRealDriver(a.role, channel); missingKey != "" {
			record("transport", string(errs.AUTH),
				fmt.Sprintf("fail-loud: channel %s requires env %s (not set)", channel, missingKey))
			if a.ps != nil {
				c.save()
			}
			c.Log(fmt.Sprintf("%s: fail-loud — channel %s requires env %s (not set)", a.role, channel, missingKey))
			return verdictOutcome{ErrorCode: errs.AUTH}, nil
		}
		visited[channel] = true
		res := call()

		streamMsg := ""
		if res.ErrorMessage != nil {
			streamMsg = *res.ErrorMessage
		}
		var streamCode errs.ErrorCode
		if streamMsg != "" {
			streamCode = errs.ClassifyError(errs.SpawnResult{ExitCode: 1, Stderr: streamMsg})
		}
		var verdict *prompts.Verdict
		// 出口契约同款三重门（§6）：进程终态正常 + 无信号 + stderr 无错误模式
		// 才允许解析裁决——"超时/被 kill 被 trap 成 exit 0"不得读成绿灯。
		// 裁决文本取 Text ?? Stdout（Node 语义：harness 可能把裁决写 stdout）。
		text := res.Text
		if text == "" {
			text = res.Stdout
		}
		if streamCode == "" && res.ExitCode != nil && *res.ExitCode == 0 && !res.TimedOut &&
			res.Signal == "" && errs.ClassifyError(errs.SpawnResult{ExitCode: 0, Stderr: res.Stderr}) == "" {
			verdict, _ = prompts.ParseReviewVerdict(text)
		}
		code := ""
		if verdict == nil {
			if streamCode != "" {
				code = string(streamCode)
			} else if cls := errs.ClassifyError(toSpawnResult(res)); cls != "" {
				code = string(cls)
			} else {
				code = string(errs.EMPTY_RESPONSE)
			}
		}
		if code != "" {
			lastCode = errs.ErrorCode(code)
		}
		c.recordSpawnMetrics(spawnMetricsArgs{
			role: a.role, packetID: a.packetID, model: currentModel,
			channel: channel, prompt: a.prompt, result: &res,
			errorCode: errs.ErrorCode(code), attempt: a.attemptNo,
		})

		if verdict != nil {
			return verdictOutcome{OK: true, Verdict: verdict}, nil
		}

		who := a.role
		if a.packetID != "" {
			who = fmt.Sprintf("%s %s", a.role, a.packetID)
		}
		c.Log(fmt.Sprintf("%s transport %d: failed/unparseable (%s)", who, t, code))

		if errs.ShouldFallback(errs.ErrorCode(code)) || fallback.AuthFallbackAllowed(c.Config, channel, errs.ErrorCode(code)) {
			next := fallback.NextChannel(c.Config, channel)
			var mapped *fallback.ChannelModel
			if next != "" {
				mapped = fallback.ModelForChannel(c.Config, next, a.role, "")
			}
			if next != "" && !visited[next] && mapped != nil {
				record("fallback", code, fmt.Sprintf("channel %s -> %s", channel, next))
				channel = next
				currentModel = mapped.Model
				if mapped.Harness != "" {
					currentHarness = mapped.Harness
				}
				t--
				if a.ps != nil {
					c.save()
				}
				continue
			}
			if next != "" && mapped == nil {
				c.Log(fmt.Sprintf("%s fallback chain ends: no model mapping for %s on %s (P1-12①)", a.role, a.role, next))
			}
		}
		if !errs.IsRetryable(errs.ErrorCode(code)) {
			record("transport", code, "no-retry")
			if a.ps != nil {
				c.save()
			}
			return verdictOutcome{ErrorCode: errs.ErrorCode(code)}, nil
		}
		delay := errs.ComputeBackoff(t, errs.BackoffOptions{
			BaseDelayMs:  c.Config.Retry.BaseDelayMs,
			MaxDelayMs:   c.Config.Retry.MaxDelayMs,
			Jitter:       c.Config.Retry.Jitter,
			RetryAfterMs: float64(errs.ExtractRetryAfterMs(res.Stderr + "\n" + res.Stdout)),
		})
		record("transport", code, "backoff", delay)
		if a.ps != nil {
			c.save()
		}
		c.Sleep(time.Duration(delay) * time.Millisecond)
	}
	return verdictOutcome{ErrorCode: lastCode}, nil
}

// ---- Review 步骤（§4.3）----

type reviewOutcome struct {
	Pass      bool
	Stopped   bool
	Verdict   *prompts.Verdict
	ErrorCode errs.ErrorCode
}

// reviewStep 只在 L1 绿了之后运行（铁律 4）。评审者看到 packet + diff + L1
// 结果——绝看不到 executor transcript。
func (c *Context) reviewStep(packet *contract.Packet, defectInjection any) (reviewOutcome, error) {
	id := packet.PacketID
	ps := c.State.Packets[id]

	uCwd := c.unitCwd(packet)
	changed, _ := verify.GetChangedFiles(uCwd, c.baseRefOf(ps))
	executorFiles, _ := c.partitionChangedFiles(changed)
	var diffSummary string
	if executorFiles != nil {
		lines := make([]string, len(executorFiles))
		for i, f := range executorFiles {
			lines[i] = "- " + f
		}
		diffSummary = fmt.Sprintf("changed files (git diff %s, ledger excluded):\n%s",
			orHEAD(c.baseRefOf(ps)), strings.Join(lines, "\n"))
	} else {
		self := []any{}
		if ex, ok := ps.Evidence.Executor.(map[string]any); ok {
			if cf, ok := ex["changed-files"].([]any); ok {
				self = cf
			}
		}
		lines := make([]string, len(self))
		for i, f := range self {
			lines[i] = "- " + fmt.Sprintf("%v", f)
		}
		diffSummary = "changed files (executor-reported):\n" + strings.Join(lines, "\n")
	}
	// §4.3 向评审者承诺的是真实 git diff，不是文件清单（#11）。
	diffText, _ := verify.GetDiffText(uCwd, c.baseRefOf(ps), c.ledgerExcludes())
	prompt := prompts.BuildReviewPrompt(prompts.ReviewArgs{
		Packet:          packet,
		L1Evidence:      ps.Evidence.L1,
		DiffSummary:     diffSummary,
		DiffText:        diffText,
		DefectInjection: defectInjection,
	})

	binding, err := fallback.ResolveBinding(c.Config, "review", "")
	if err != nil {
		return reviewOutcome{}, err
	}
	attemptNo := ps.Attempts
	r, ferr := c.agentVerdictStep(verdictArgs{
		role: "review", binding: binding, prompt: prompt,
		packetID: id, attemptNo: &attemptNo, ps: ps,
	})
	if ferr != nil {
		return reviewOutcome{}, ferr
	}
	if r.Stopped {
		return reviewOutcome{Stopped: true}, nil
	}
	if !r.OK {
		return reviewOutcome{ErrorCode: r.ErrorCode}, nil
	}

	vb, _ := json.Marshal(r.Verdict)
	m := map[string]any{}
	_ = json.Unmarshal(vb, &m)
	m["at"] = c.nowISO()
	ps.Evidence.Review = m
	at := c.nowISO()
	ps.UpdatedAt = &at
	if r.Verdict.Verdict == "pass" {
		c.touchProgress()
	}
	c.save()
	return reviewOutcome{Pass: r.Verdict.Verdict == "pass", Verdict: r.Verdict}, nil
}

// ---- Judge 步骤（§7 里程碑终验）----

type judgeOutcome struct {
	Pass      bool
	Stopped   bool
	Split     bool // v0.18 A3：面板分歧 → 升级 inbox 由人裁决
	Verdict   *prompts.Verdict
	ErrorCode errs.ErrorCode
}

// loopToBinding config.LoopBinding → fallback.Binding（judge-panel 复用
// agentVerdictStep 的传输重试/降级骨架）。
func loopToBinding(b config.LoopBinding) fallback.Binding {
	return fallback.Binding{Harness: b.Harness, Model: b.Model, Channel: b.Channel, Thinking: b.Thinking}
}

// judgeStep 在 L2/L3 闸门通过后、里程碑关闭前，由全新会话对照 spec 的需求
// 矩阵复核整个里程碑。失败 → 升级 inbox。
// v0.18（A3 双 judge 面板）：config `judge-panel` 配置第二意见绑定（K3 主判
// + V4 Pro 异厂商复核）。语义：一致按共识；分歧 → split（Pass=false，inbox
// 叫人）；复核通道自身失败 → fail-loud 升级，绝不静默降级为单 judge
// （单 judge 偏差实证见 DESIGN §4.3/附录 A）。
func (c *Context) judgeStep(milestone contract.Milestone) (judgeOutcome, error) {
	ms := c.State.Milestones[milestone.ID]
	packets := []*contract.Packet{}
	for _, p := range c.PacketList {
		if m, err := contract.MilestoneOfPacket(p.PacketID); err == nil && m == milestone.ID {
			packets = append(packets, p)
		}
	}
	var gateLevels map[string]any
	if g, ok := ms.Gate.(map[string]any); ok {
		gateLevels, _ = g["levels"].(map[string]any)
	}
	prompt := prompts.BuildJudgePrompt(c.Spec, milestone, packets, c.State, gateLevels)

	binding, err := fallback.ResolveBinding(c.Config, "judge", "")
	if err != nil {
		return judgeOutcome{}, err
	}
	r, ferr := c.agentVerdictStep(verdictArgs{role: "judge", binding: binding, prompt: prompt})
	if ferr != nil {
		return judgeOutcome{}, ferr
	}

	var r2 *verdictOutcome
	if c.Config.JudgePanel != nil && r.OK && !r.Stopped {
		res, ferr := c.agentVerdictStep(verdictArgs{role: "judge", binding: loopToBinding(*c.Config.JudgePanel), prompt: prompt})
		if ferr != nil {
			return judgeOutcome{}, ferr
		}
		r2 = &res
	}

	if ms.Gate == nil {
		ms.Gate = map[string]any{"pass": true, "levels": map[string]any{}, "at": c.nowISO()}
		c.State.Milestones[milestone.ID] = ms
	}
	gmap, ok := ms.Gate.(map[string]any)
	if !ok {
		// C11（v0.18）：手工把 gate 写成非对象时，替换为对象并写回 state——
		// 原实现只改内存拷贝，judge 结论落进孤立 map 不落盘。
		gmap = map[string]any{}
		ms.Gate = gmap
		c.State.Milestones[milestone.ID] = ms
	}
	if r.Stopped {
		return judgeOutcome{Stopped: true}, nil
	}
	if !r.OK {
		gmap["judge"] = map[string]any{"verdict": "error", "error": string(r.ErrorCode), "at": c.nowISO()}
		c.save()
		return judgeOutcome{ErrorCode: r.ErrorCode}, nil
	}

	verdictRec := func(vo verdictOutcome) map[string]any {
		vb, _ := json.Marshal(vo.Verdict)
		m := map[string]any{}
		_ = json.Unmarshal(vb, &m)
		m["at"] = c.nowISO()
		return m
	}

	judge := verdictRec(r)
	pass := r.Verdict.Verdict == "pass"
	out := judgeOutcome{Pass: pass, Verdict: r.Verdict}
	if r2 != nil {
		switch {
		case !r2.OK:
			// 复核通道失败：fail-loud，与主判错误同路径升级。
			judge["panel"] = map[string]any{"agreement": "secondary-error",
				"error": string(r2.ErrorCode), "at": c.nowISO()}
			gmap["judge"] = judge
			c.save()
			return judgeOutcome{ErrorCode: r2.ErrorCode}, nil
		default:
			secPass := r2.Verdict.Verdict == "pass"
			split := secPass != pass
			rec := map[string]any{
				"agreement":     "agree",
				"binding-model": c.Config.JudgePanel.Model,
				"secondary":     verdictRec(*r2),
				"at":            c.nowISO(),
			}
			if split {
				rec["agreement"] = "split"
				merged := append(append([]string{}, r.Verdict.Reasons...), r2.Verdict.Reasons...)
				judge["verdict"] = "split"
				judge["score"] = nil
				judge["reasons"] = merged
				out.Split = true
				out.Pass = false
				out.Verdict = &prompts.Verdict{Verdict: "split", Reasons: merged}
			}
			judge["panel"] = rec
		}
	}
	gmap["judge"] = judge
	c.save()
	return out, nil
}

// ---- 里程碑闸门（§4.4）----

type gateOutcome struct {
	Pass   bool
	Levels map[string]any
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
	pass := true
	for _, level := range []string{"L2", "L3"} {
		var commands []contract.GateEntry
		for _, p := range packets {
			commands = append(commands, p.Acceptance.Level(level)...)
		}
		if len(commands) == 0 {
			levels[level] = map[string]any{"pass": true, "results": []any{}, "skipped": true}
			continue
		}
		gate := verify.RunGateCommands(commands, verify.GateOpts{
			Cwd: c.Cwd, TimeoutMS: timeoutMs, EnvAllowlist: envAllowlist,
		})
		levels[level] = gate
		if !gate.Pass {
			pass = false
		}
	}
	ms := c.State.Milestones[milestone.ID]
	ms.Gate = map[string]any{"pass": pass, "levels": levels, "at": c.nowISO()}
	c.State.Milestones[milestone.ID] = ms
	c.save()
	// 同 l1Step：无错误产生路径，删除防御性 error 返回（2026-08-23）。
	return gateOutcome{Pass: pass, Levels: levels}
}

// ---- Plan 步骤（§4.1）----

type planOutcome struct {
	OK        bool
	Stopped   bool
	ErrorCode errs.ErrorCode
	Packets   []*contract.Packet
	Result    *drivers.Result
}

// planStep 经长驻 kimi code 会话为里程碑签发/确认 packet 序列。会话纪律：
//   - 首调建会话，从流中捕获 id 存进 state.json（plan-session）；后续用
//     `kimi -r <id>` 续；
//   - 会话死亡（resume 失败/看门狗击杀挂起）触发冷启动协议：从契约层
//     （spec + state + retro 尾部）启动全新会话——契约层是唯一记忆；
//   - provider 失败走配置的渠道降级链；
//   - 所有渠道都失败则任务升级到 inbox（Plan 是定方向的，没了它继续跑没意义）。
func (c *Context) planStep(milestone contract.Milestone) (planOutcome, error) {
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

	attempt := func(sessionID string, coldStart bool, channel, model, harness string) (drivers.Result, errs.ErrorCode) {
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
		})
		res := c.Drivers.Plan.Plan(drivers.CallArgs{
			Prompt:      prompt,
			MilestoneID: milestone.ID,
			TaskDir:     c.TaskDir,
			Cwd:         c.Cwd,
			SessionID:   sessionID,
			Model:       model,
			Binding: config.LoopBinding{
				Harness: harness, Model: model, Channel: channel, Thinking: binding.Thinking,
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
		if msg != "" {
			code = errs.ClassifyError(errs.SpawnResult{ExitCode: 1, Stderr: msg})
		} else if res.ExitCode != nil && *res.ExitCode == 0 && !res.TimedOut && res.SpawnError == "" {
			code = ""
		} else {
			code = errs.ClassifyError(toSpawnResult(res))
		}
		return res, code
	}

	channel := binding.Channel
	model, harness := binding.Model, binding.Harness
	// 铁律 7：只有长驻会话诞生于与当前绑定相同的 model+channel 时才 resume；
	// 否则一律冷启动（#15）。
	var stored map[string]any
	if m, ok := c.State.PlanSession.(map[string]any); ok {
		stored = m
	}
	sessionID := ""
	if stored != nil {
		sc, _ := stored["channel"].(string)
		sm, _ := stored["model"].(string)
		if sc == channel && sm == model {
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
		result, code = attempt(sessionID, sessionID == "", channel, model, harness)
		if code == "" {
			// C5（v0.18）：调用成功却没拿到 session id——上游 resume_hint 格式漂移
			// 或缺失，长驻会话将无声退化为每轮冷启动。必须留观测痕迹。
			if result.SessionID == "" {
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
	if result.SessionID != "" {
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
			"session-id": result.SessionID,
			"channel":    channel,
			"model":      model,
			"created-at": createdAt,
			"updated-at": c.nowISO(),
			"resumes":    resumes,
		}
		c.save()
	}
	return planOutcome{OK: true, Packets: packets, Result: &result}, nil
}
