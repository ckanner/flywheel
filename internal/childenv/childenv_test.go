package childenv

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeEnv 返回固定宿主环境。
func fakeEnv() LookupEnv {
	return func(key string) (string, bool) {
		switch key {
		case "PATH", "HOME", "SECRET_KEY", "EXTRA", "NODE_TEST_CONTEXT":
			return "v-" + key, true
		default:
			return "", false
		}
	}
}

func TestBuildChildEnvWhitelist(t *testing.T) {
	env := BuildChildEnv(fakeEnv(), map[string]string{"OPENCODE_API_KEY": "k1", "LIVE_KEY": "k2"})
	assert.Equal(t, "v-PATH", env["PATH"])
	assert.Equal(t, "v-HOME", env["HOME"])
	assert.Equal(t, "k1", env["OPENCODE_API_KEY"], "显式注入的 key 在")
	assert.Equal(t, "k2", env["LIVE_KEY"])
	_, ok := env["SECRET_KEY"]
	assert.False(t, ok, "白名单外的宿主 key 绝不透传")
}

func TestEnvSlice(t *testing.T) {
	got := EnvSlice(map[string]string{"A": "1", "B": "2"})
	assert.ElementsMatch(t, []string{"A=1", "B=2"}, got)
	assert.Empty(t, EnvSlice(nil))
}

func TestBuildChildEnvSkipsEmptyKeys(t *testing.T) {
	env := BuildChildEnv(fakeEnv(), map[string]string{"EMPTY": ""})
	_, ok := env["EMPTY"]
	assert.False(t, ok, "空值 key 不注入")
}

func TestGateEnv(t *testing.T) {
	env := GateEnv(fakeEnv(), []string{"EXTRA"})
	assert.Equal(t, "v-PATH", env["PATH"])
	assert.Equal(t, "v-EXTRA", env["EXTRA"], "allow 显式追加")
	_, ok := env["SECRET_KEY"]
	assert.False(t, ok, "闸门命令不得见 orchestrator 的 key")
	_, ok = env["NODE_TEST_CONTEXT"]
	assert.False(t, ok, "NODE_TEST_CONTEXT 恒清除（嵌套 node --test 假绿防护）")
}

func TestGateEnvDoesNotMutateChildEnvKeys(t *testing.T) {
	before := len(ChildEnvKeys)
	GateEnv(fakeEnv(), []string{"X", "Y"})
	assert.Equal(t, before, len(ChildEnvKeys), "allow 列表不得污染全局白名单")
}

func TestOSLookupEnv(t *testing.T) {
	v, ok := OSLookupEnv("PATH")
	assert.True(t, ok)
	assert.NotEmpty(t, v)
	_, ok = OSLookupEnv("FLYWHEEL_DEFINITELY_NOT_SET_XYZ")
	assert.False(t, ok)
}
