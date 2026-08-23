package metrics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/config"
)

func TestPromptHash(t *testing.T) {
	sum := sha256.Sum256([]byte("hello"))
	want := hex.EncodeToString(sum[:])[:12]
	assert.Equal(t, want, PromptHash("hello"))
	assert.Len(t, PromptHash("anything"), 12)
	assert.NotEqual(t, PromptHash("a"), PromptHash("b"))
}

func TestAppendCreatesDirsAndJSONL(t *testing.T) {
	file := filepath.Join(t.TempDir(), "nested", "dir", "metrics.jsonl")
	require.NoError(t, Append(file, map[string]any{"model": "v4-flash", "tokens": 42}))
	require.NoError(t, Append(file, map[string]any{"model": "k3", "tokens": 7}))

	data, err := os.ReadFile(file)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	require.Len(t, lines, 2)

	var rec map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &rec))
	assert.Equal(t, "v4-flash", rec["model"])
	assert.InDelta(t, 42, rec["tokens"], 1e-9)
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`, rec["ts"])
}

func TestAppendRejectsNonSerializable(t *testing.T) {
	err := Append(filepath.Join(t.TempDir(), "m.jsonl"), map[string]any{"bad": make(chan int)})
	assert.Error(t, err)
}

func TestPriceForModel(t *testing.T) {
	pricing := map[string]config.Price{
		"kimi-k3":         {Input: 15},
		"deepseek-v4-pro": {Input: 1},
	}
	p := PriceForModel(pricing, "kimi-code/k3")
	require.NotNil(t, p)
	assert.Equal(t, 15.0, p.Input, "短 id k3 → kimi-k3")

	p = PriceForModel(pricing, "kimi-coding/k3")
	require.NotNil(t, p)
	assert.Equal(t, 15.0, p.Input)

	// 全 id 直查优先。
	full := map[string]config.Price{
		"opencode-go/deepseek-v4-pro": {Input: 99},
		"deepseek-v4-pro":             {Input: 1},
	}
	p = PriceForModel(full, "opencode-go/deepseek-v4-pro")
	require.NotNil(t, p)
	assert.Equal(t, 99.0, p.Input)

	p = PriceForModel(pricing, "deepseek-v4-pro")
	require.NotNil(t, p)
	assert.Equal(t, 1.0, p.Input)

	assert.Nil(t, PriceForModel(pricing, ""))
	assert.Nil(t, PriceForModel(nil, "k3"))
	assert.Nil(t, PriceForModel(pricing, "unknown/model-x"))
}

func TestEstimateCostUSD(t *testing.T) {
	price := &config.Price{Input: 1, Output: 4, CacheRead: 0.1}
	cost := EstimateCostUSD(&Usage{Input: 1_000_000, Output: 500_000, CacheRead: 2_000_000, CacheWrite: 250_000}, price)
	require.NotNil(t, cost)
	// input 1 + output 2 + cacheRead 0.2 + cacheWrite(按 input 价) 0.25
	assert.InDelta(t, 3.45, *cost, 1e-9)

	assert.Nil(t, EstimateCostUSD(nil, price))
	assert.Nil(t, EstimateCostUSD(&Usage{}, nil))

	zero := EstimateCostUSD(&Usage{}, price)
	require.NotNil(t, zero)
	assert.InDelta(t, 0, *zero, 1e-9)
}

func TestAppendMkdirFailure(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "f")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	// 路径中间段是文件 → MkdirAll 失败。
	err := Append(filepath.Join(blocker, "sub", "m.jsonl"), map[string]any{})
	assert.Error(t, err)
}

func TestAppendOpenFileFailureOnDirectory(t *testing.T) {
	// 目标路径是目录 → OpenFile EISDIR。
	err := Append(t.TempDir(), map[string]any{})
	assert.Error(t, err)
}

func TestPriceForModelShortIDHit(t *testing.T) {
	pricing := map[string]config.Price{"v4-flash": {Input: 0.5}}
	p := PriceForModel(pricing, "opencode-go/v4-flash")
	require.NotNil(t, p)
	assert.InDelta(t, 0.5, p.Input, 1e-9)
}
