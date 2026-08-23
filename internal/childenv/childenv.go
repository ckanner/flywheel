// Package childenv — 子进程环境纪律（§9）：编排器 spawn 的一切
// （agent CLI、闸门命令、intake 源命令）都拿白名单环境。不注入任何 API key
// 或 token；渠道的 executor key 单独显式注入（drivers/pi --api-key）。
package childenv

import "os"

// ChildEnvKeys 白名单基础键。
var ChildEnvKeys = []string{
	"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL", "TERM", "SHELL", "USER", "LOGNAME",
	"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME",
}

// LookupEnv 环境变量读取（可注入，测试隔离宿主环境）。
type LookupEnv func(key string) (string, bool)

// OSLookupEnv 默认实现。
func OSLookupEnv(key string) (string, bool) { return os.LookupEnv(key) }

// BuildChildEnv 白名单环境 + 显式 apiKeys 注入（低价值 executor key 或
// 人批的 live 凭证）。
func BuildChildEnv(lookup LookupEnv, apiKeys map[string]string) map[string]string {
	env := map[string]string{}
	for _, k := range ChildEnvKeys {
		if v, ok := lookup(k); ok {
			env[k] = v
		}
	}
	for k, v := range apiKeys {
		if v != "" {
			env[k] = v
		}
	}
	return env
}

// GateEnv 确定性闸门/测试命令的白名单环境（P0-4）：L1/L2/L3 命令跑的是
// executor 写的代码，绝不能继承编排器的 key。`allow` 显式追加额外键
// （config verify.env-allowlist）。NODE_TEST_CONTEXT 恒清除：嵌套 node --test
// 继承 "child" 模式时变红也 exit 0（2026-08-20 实测）——oracle 需要真退出码。
func GateEnv(lookup LookupEnv, allow []string) map[string]string {
	out := map[string]string{}
	for _, k := range append(append([]string{}, ChildEnvKeys...), allow...) {
		if v, ok := lookup(k); ok {
			out[k] = v
		}
	}
	delete(out, "NODE_TEST_CONTEXT")
	return out
}

// EnvSlice map 环境转 exec.Cmd.Env 所需的 KEY=VALUE 切片。
func EnvSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
