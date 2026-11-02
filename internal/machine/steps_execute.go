// steps_execute.go — Execute 步骤与 L1 闸门（DESIGN §4.2/§4.4）：worktree
// 单元执行循环 + 确定性验收 + 测试文件模式判定。
package machine

import (
	"crypto/sha256"
	"fmt"
	"io"
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
)

var writeCollectedReport = os.WriteFile
var removeCollectedReport = os.Remove
var openReportFile = func(r *os.Root, rel string) (io.ReadCloser, error) { return r.Open(rel) }

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

	if ps.Status == "completed" && ps.Worktree == nil {
		ps.BaseRef = nil // 手动重新执行已交付单元，从当前主分支开始新的候选。
		ps.ContractSyncPending = false
	}
	ms := c.State.Milestones[ps.Milestone]
	ms.Status, ms.Gate, ms.ClosedAt = "in-progress", nil, nil
	c.State.Milestones[ps.Milestone] = ms
	c.State.Status = "running"
	c.State.CurrentMilestone = strPtrAlways(ps.Milestone)
	ps.Status = "executing"
	ps.Attempts++
	invalidateEvidence(ps)
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
	// Executor 只能写自己的工作区。报告先写 worktree 内账本目录，进程退出
	// 且格式校验通过后由编排器收进主任务目录；无需开放主 checkout 写权限。
	executionTaskDir := filepath.Join(uCwd, c.taskDirPrefix())
	binding, err := fallback.ResolveBinding(c.Config, "execute", packet.Route)
	if err != nil {
		return execOutcome{}, err
	}
	channel := binding.Channel
	currentModel, currentHarness := binding.Model, binding.Harness
	currentProvider := fallback.ResolveProvider(c.Config, currentHarness, binding.Provider, channel)
	visited := map[string]bool{}
	maxTransport := c.Config.Retry.MaxTransportRetries
	lastCode := errs.TRANSPORT

	// 出口契约必须裁决“这一次”的交付物——上次尝试遗留的陈旧 done-report 会把
	// 无产出的重试读成假成功。
	reportRel := filepath.ToSlash(filepath.Join("reports", id+".done.json"))
	reportBaseline, _ := verify.GetBaseRef(uCwd)
	_ = os.Remove(filepath.Join(c.TaskDir, reportRel))

	for t := 0; t <= maxTransport; t++ {
		if ferr := c.err(); ferr != nil {
			return execOutcome{}, ferr
		}
		// 预算闸门由 runExecuteLoop 每轮统一把守（v0.24 单一所有者）。
		// 宁可 fail-loud 也不要把无 key 的 spawn 送进 pi 的 auth.json 陷阱（P1-12②）。
		if missingKey := c.missingKeyForRealDriver("execute", channel); missingKey != "" {
			c.recordRetry(ps, "transport", string(errs.AUTH),
				fmt.Sprintf("fail-loud: channel %s requires env %s (not set)", channel, missingKey))
			c.save()
			return execOutcome{ErrorCode: errs.AUTH}, nil
		}
		_ = os.Remove(filepath.Join(executionTaskDir, reportRel))
		prompt := prompts.BuildExecutePrompt(packet, c.Spec, executionTaskDir, uCwd, ps.Attempts, prompts.ExecuteOptions{
			Feedback: ps.LastFeedback, ProtectedTestPatterns: c.Config.Verify.ProtectedTestPatterns,
		})
		visited[channel] = true
		apiKeys := c.apiKeysFor(channel)
		for k, v := range credentialEnv {
			apiKeys[k] = v
		}
		res := c.Drivers.Execute.Execute(drivers.CallArgs{
			Prompt:         prompt,
			PacketID:       id,
			TaskDir:        executionTaskDir,
			Cwd:            uCwd,
			PermissionMode: "workspace-write",
			Model:          currentModel,
			Binding: config.LoopBinding{
				Harness: currentHarness, Provider: currentProvider, Model: currentModel,
				Channel: channel, Thinking: binding.Thinking,
			},
			APIKey:   c.channelApiKey(channel),
			APIKeys:  apiKeys,
			Watchdog: c.wdog(),
		})

		// 交付物 = done-report，存在且 schema 合法。
		raw, readErr := readUnitReport(uCwd, filepath.Join(c.taskDirPrefix(), reportRel))
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
		if streamMsg != "" && !res.TimedOut && !res.Cancelled && res.SpawnError == "" && res.CleanupError == "" {
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
			Settled: res.Settled, Cancelled: res.Cancelled,
			Truncated: res.Truncated, CleanupError: res.CleanupError,
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
			if err := os.MkdirAll(filepath.Join(c.TaskDir, "reports"), 0o755); err != nil {
				return execOutcome{}, err
			}
			if err := writeCollectedReport(filepath.Join(c.TaskDir, reportRel), raw, 0o644); err != nil {
				return execOutcome{}, err
			}
			if executionTaskDir != c.TaskDir {
				// 重开已交付packet时，HEAD可能已有旧报告。恢复调用前的该
				// tracked路径（含模式）；只删除本来untracked的本次报告。
				restored, err := verify.RestoreReservedReport(uCwd, reportBaseline, filepath.Join(c.taskDirPrefix(), reportRel))
				if err != nil {
					return execOutcome{}, err
				}
				if !restored {
					if err := removeCollectedReport(filepath.Join(executionTaskDir, reportRel)); err != nil {
						return execOutcome{}, err
					}
				}
			}
			ps.Status = "executed"
			ps.LastFeedback = nil
			ps.Interrupted = false
			ps.Evidence.Executor = map[string]any{
				"done-report":          reportRel,
				"report-sha256":        fmt.Sprintf("%x", sha256.Sum256(raw)),
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
		ps.LastFeedback = map[string]any{"error-code": code, "details": contractRes.Reasons, "at": c.nowISO()}
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
				currentProvider = fallback.ResolveProvider(c.Config, currentHarness, "", channel)
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

// readUnitReport 使用 os.Root 的穿越保护；不跟随指向工作区之外的链接，
// 不读 FIFO/设备，报告体量有界。报告是执行者提供的不可信数据。
func readUnitReport(cwd, rel string) ([]byte, error) {
	r, err := os.OpenRoot(cwd)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	info, err := r.Lstat(rel)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("done-report must be a regular file")
	}
	f, err := openReportFile(r, rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	const limit = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > limit {
		return nil, fmt.Errorf("done-report exceeds 1 MiB")
	}
	return raw, nil
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
	// 验收会执行仓库代码；命令或并发编辑若改变候选，旧测试结果不能为
	// 改后的内容背书。账本和 ignored 构建产物仍遵守统一快照排除规则。
	beforeIdentity, beforeErr := c.verificationIdentity(packet)
	gate := verify.GateReport{}
	if beforeErr == nil {
		gate = verify.RunGateCommands(packet.Acceptance.L1, verify.GateOpts{
			Context: c.RunContext,
			Cwd:     uCwd, TimeoutMS: timeoutMs, EnvAllowlist: envAllowlist,
		})
	}
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
	boundary := verify.CheckBoundaries(executorFiles, packet.Boundaries, c.effectiveTestFilePatterns())
	if len(contractViolations) > 0 {
		// Executor 动了契约层本身（spec/packets/config）——直接判失败。
		for _, v := range contractViolations {
			boundary.Violations = append(boundary.Violations, verify.BoundaryViolation{Type: v.Type, Path: v.Path})
		}
		boundary.Pass = false
	}
	pass := gate.Pass && boundary.Pass
	identity, identityErr := c.verificationIdentity(packet)
	if beforeErr != nil {
		identityErr = fmt.Errorf("before acceptance: %w", beforeErr)
	} else if identityErr != nil {
		identityErr = fmt.Errorf("after acceptance: %w", identityErr)
	} else if beforeIdentity != identity {
		identityErr = fmt.Errorf("candidate or verification inputs changed during acceptance; rerun verification on the resulting candidate")
	}
	if identityErr != nil {
		pass = false
		c.Log(fmt.Sprintf("%s: candidate snapshot failed: %v", id, identityErr))
	}
	ps.Evidence.L1 = map[string]any{
		"pass":            pass,
		"identity":        identity,
		"identity-before": beforeIdentity,
		"identity-error":  jsonNilable(errorText(identityErr)),
		"commands":        gate.Results,
		"boundary":        boundary,
		"at":              c.nowISO(),
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

func errorText(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

// effectiveTestFilePatterns 只保护显式冻结的验收文件。普通单测/回归测试
// 可随实现修改；test-file-patterns 仅用于缺陷抽样识别。
func (c *Context) effectiveTestFilePatterns() []string {
	return c.Config.Verify.ProtectedTestPatterns
}
