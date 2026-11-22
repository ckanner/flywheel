package diagnostic

import (
	"context"
	"encoding/json"
	"github.com/ckanner/flywheel/internal/drivers"
	"slices"
)

func (r *Report) antigravity(ctx context.Context, b Binding, cache map[string]bool) {
	worker := drivers.AntigravityCommand()
	ready := r.command(ctx, "antigravity-worker", worker, cache, false)
	ready = r.command(ctx, "node", "node", cache, true) && ready
	ready = r.command(ctx, "agy", drivers.AntigravityBinary(), cache, true) && ready
	key := "antigravity-import:" + worker
	if _, checked := cache[key]; ready && !checked && ctx.Err() == nil {
		result := runCommand(worker, []string{"--doctor"}, commandOptions(ctx, r.Cwd))
		var probe struct {
			Protocol              int      `json:"protocol"`
			Type                  string   `json:"type"`
			Package               string   `json:"package"`
			Version               string   `json:"version"`
			MachineProtocol       int      `json:"machineProtocol"`
			Available             bool     `json:"available"`
			InferenceTested       *bool    `json:"inferenceTested"`
			EvidenceOnly          string   `json:"evidenceOnly"`
			NativeVersion         int      `json:"nativeVersion"`
			NativeOrchestrator    bool     `json:"nativeOrchestrator"`
			NativeModes           []string `json:"nativeModes"`
			NativeAgentLimit      string   `json:"nativeAgentLimit"`
			HardNativeAgentLimit  *bool    `json:"hardNativeAgentLimit"`
			NativeCompletionScope string   `json:"nativeCompletionScope"`
		}
		ok := cleanCommand(result) && json.Unmarshal([]byte(result.Stdout), &probe) == nil && probe.Protocol == 1 && probe.Type == "doctor" && probe.Package == "@ckanner/dsh-subagent-antigravity" && probe.Version == drivers.AntigravityPackageVersion && probe.MachineProtocol == 1 && probe.Available && probe.InferenceTested != nil && !*probe.InferenceTested && probe.EvidenceOnly == "verified-managed-agent" &&
			probe.NativeVersion == 1 && probe.NativeOrchestrator && slices.Equal(probe.NativeModes, []string{"native", "subagents", "swarm"}) && probe.NativeAgentLimit == "observed-total-children-guard" && probe.HardNativeAgentLimit != nil && !*probe.HardNativeAgentLimit && probe.NativeCompletionScope == "cli-invocation"
		cache[key] = ok
		if ok {
			r.add("antigravity.api", "pass", "Antigravity 官方 CLI 插件公开 machine/lifecycle 入口可用；未发送推理请求", "", "")
		} else {
			r.add("antigravity.api", "fail", "Antigravity 插件精确版本或公开入口检查失败", "INVALID_REQUEST", "安装与 Flywheel 匹配的插件；不会改走 API 或其他 CLI")
		}
	}
	r.add(b.ID+".subscription", "unknown", "调用前核对本机 API/ADC/customModels 与 credits 设置；账号权益和剩余额度未验证", "", "额度耗尽或配置冲突时停止，不自动启用额外计费")
	r.add(b.ID+".configuration", "unknown", "模型与权限由真实 init 回执核对；effort、execution mode 和 OS sandbox 没有独立生效回显", "", "调查分析要求受管 custom agent、真实 init 工具目录且无工具/子 Agent step；这不构成 OS 沙箱")
}
