package diagnostic

import (
	"context"
	"encoding/json"
	"os"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
)

// Native profiles select a different execution contract. A legacy SDK probe
// does not cover that contract. This check imports local interfaces only; real
// effective capability admission remains an invocation-owned runtime operation.
func (r *Report) nativeProfile(ctx context.Context, cfg *config.Config, b Binding, cache map[string]bool) {
	if b.NativeProfile == "" {
		return
	}
	available := false
	switch b.Harness {
	case "dsh":
		available = r.dshNative(ctx, cache)
		var options struct {
			LCU struct {
				Enabled bool `json:"enabled"`
			} `json:"lcu"`
		}
		if err := json.Unmarshal(cfg.NativeProfiles[b.NativeProfile].Options, &options); err != nil {
			available = false
		} else if options.LCU.Enabled {
			if cache["dsh-native-import:"+drivers.DSHCommand()+":lcu"] {
				r.add(b.ID+".native.lcu", "pass", "已导入 LCU 0.5.0 managed 接口；未启动桌面、获取资源租约或执行 GUI 操作", "", "")
			} else {
				r.add(b.ID+".native.lcu", "fail", "原生 profile 要求 LCU，但固定版本的 managed 接口不可用", "INVALID_REQUEST", "安装与当前 Flywheel 配套的 LCU 0.5.0；安装成功仍须运行时租约与桌面能力准入")
				available = false
			}
		}
	case "kimi":
		available = r.kimiNative(ctx, cache)
	case "codex-pro":
		available = cache["codex-pro-import:"+drivers.CodexProCommand()+":native"]
	case "claude-code-pro":
		available = cache["claude-code-pro-import:"+drivers.ClaudeCodeProCommand()+":native"]
	}
	if available {
		r.add(b.ID+".native.manifest", "pass", "已检查原生 profile "+b.NativeProfile+" 的本地 worker 与版本化接口（未推理）", "", "")
	} else {
		r.add(b.ID+".native.manifest", "fail", "原生 profile 的本地 worker、固定版本依赖或版本化接口未通过检查", "INVALID_REQUEST", "重新安装当前 Flywheel 与对应原生依赖；不会降级为 legacy 调用或替换 profile")
	}
	r.add(b.ID+".native.admission", "unknown", "仅检查本地接口；本次 profile 的实际能力注册、有效配置、子任务归属与资源上限尚未验证", "", "正式调用须执行实际运行时准入并核对回执；--probe 不替代原生能力验收")
}

func (r *Report) dshNative(ctx context.Context, cache map[string]bool) bool {
	worker := drivers.DSHCommand()
	key := "dsh-native-import:" + worker
	if available, seen := cache[key]; seen {
		return available
	}
	cache[key] = false
	ready := r.command(ctx, "dsh-worker", worker, cache, false)
	ready = r.command(ctx, "node", "node", cache, true) && ready
	if !ready || ctx.Err() != nil {
		return false
	}
	result := runCommand(worker, []string{"--native-doctor"}, commandOptions(ctx, r.Cwd))
	var probe struct {
		Protocol        int    `json:"protocol"`
		Type            string `json:"type"`
		Harness         string `json:"harness"`
		NativeVersion   int    `json:"nativeVersion"`
		Available       bool   `json:"available"`
		InferenceTested *bool  `json:"inferenceTested"`
		AdmissionTested *bool  `json:"admissionTested"`
		NativeAdmission string `json:"nativeAdmission"`
		Composition     struct {
			Package            string   `json:"package"`
			Version            string   `json:"version"`
			Protocol           int      `json:"protocol"`
			Available          bool     `json:"available"`
			Presets            []string `json:"presets"`
			Executor           string   `json:"executor"`
			NamedPresetMounted *bool    `json:"namedPresetMounted"`
		} `json:"composition"`
		LCU struct {
			Package         string `json:"package"`
			Version         string `json:"version"`
			ManagedProtocol int    `json:"managedProtocol"`
			Available       bool   `json:"available"`
		} `json:"lcu"`
	}
	valid := cleanCommand(result) && json.Unmarshal([]byte(result.Stdout), &probe) == nil &&
		probe.Protocol == 1 && probe.Type == "native-doctor" && probe.Harness == "dsh" && probe.NativeVersion == 1 && probe.Available &&
		probe.InferenceTested != nil && !*probe.InferenceTested && probe.AdmissionTested != nil && !*probe.AdmissionTested &&
		probe.NativeAdmission == "requires-selected-sdk-runtime-and-effective-capability-probe" &&
		probe.Composition.Package == "dsh-preset-generator" && probe.Composition.Version == "0.4.0" && probe.Composition.Protocol == 1 &&
		probe.Composition.Available && probe.Composition.Executor == "ptc" && probe.Composition.NamedPresetMounted != nil && !*probe.Composition.NamedPresetMounted &&
		len(probe.Composition.Presets) == 2 && probe.Composition.Presets[0] == "daily" && probe.Composition.Presets[1] == "heavy"
	cache[key] = valid
	cache[key+":lcu"] = valid && probe.LCU.Package == "dsh-plugin-lcu" && probe.LCU.Version == "0.5.0" && probe.LCU.ManagedProtocol == 1 && probe.LCU.Available
	return valid
}

func (r *Report) kimiNative(ctx context.Context, cache map[string]bool) bool {
	worker := drivers.KimiNativeCommand()
	key := "kimi-native-import:" + worker
	if available, seen := cache[key]; seen {
		return available
	}
	cache[key] = false
	ready := r.command(ctx, "kimi-native-worker", worker, cache, false)
	ready = r.command(ctx, "node", "node", cache, true) && ready
	if !ready || ctx.Err() != nil {
		return false
	}
	opts := commandOptions(ctx, r.Cwd)
	if binary := os.Getenv(drivers.KimiCommandEnv); binary != "" {
		opts.Env = append(opts.Env, drivers.KimiCommandEnv+"="+binary)
	}
	result := runCommand(worker, []string{"--doctor"}, opts)
	var probe struct {
		Protocol          int    `json:"protocol"`
		Type              string `json:"type"`
		Available         bool   `json:"available"`
		CLIVersion        string `json:"cliVersion"`
		BackendProtocol   string `json:"backendProtocol"`
		InferenceTested   *bool  `json:"inferenceTested"`
		NativeAdmission   string `json:"nativeAdmission"`
		RecoverySupported *bool  `json:"recoverySupported"`
	}
	valid := cleanCommand(result) && json.Unmarshal([]byte(result.Stdout), &probe) == nil &&
		probe.Protocol == 1 && probe.Type == "doctor" && probe.Available && probe.CLIVersion != "" &&
		versionPattern.FindString(probe.CLIVersion) == probe.CLIVersion && probe.BackendProtocol == "kimi-server/v1" &&
		probe.InferenceTested != nil && !*probe.InferenceTested && probe.NativeAdmission == "requires-actual-server-schema-and-effective-session-probe" &&
		probe.RecoverySupported != nil && !*probe.RecoverySupported
	cache[key] = valid
	return valid
}
