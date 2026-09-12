// steps_verdict.go — LLM 裁决角色（Review / Judge / 双面板，DESIGN §4.3）：
// 传输重试 + 渠道降级 + 末行 JSON 裁决解析的共用循环与两个特化步骤。
package machine

import (
	"encoding/json"
	"fmt"
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
		case r2.Stopped:
			// 面板复核撞上预算硬停：原样透传 Stopped——吞成 secondary-error
			// 会把 budget-stopped 谎报成 judge 失败升级（PR#3 review 中项）。
			return judgeOutcome{Stopped: true}, nil
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
				merged := make([]string, 0, len(r.Verdict.Reasons)+len(r2.Verdict.Reasons))
				for _, rr := range r.Verdict.Reasons {
					merged = append(merged, "[primary] "+rr)
				}
				for _, rr := range r2.Verdict.Reasons {
					merged = append(merged, "[secondary] "+rr)
				}
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
