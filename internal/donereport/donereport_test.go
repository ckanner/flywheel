package donereport

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseValid(t *testing.T) {
	raw := mustJSON(t, map[string]any{
		"requirements-covered": []string{"R-01"},
		"changed-files":        []string{"src/a.go"},
		"commands-run":         []string{"go test ./..."},
		"failures":             []string{},
	})
	r := Parse(raw, nil)
	assert.True(t, r.OK)
	assert.Empty(t, r.Errors)
	assert.Equal(t, "R-01", r.Report["requirements-covered"].([]any)[0])
}

func TestParseInvalidJSON(t *testing.T) {
	r := Parse([]byte("{nope"), nil)
	assert.False(t, r.OK)
	assert.Equal(t, []string{"invalid-json"}, r.Errors)
	assert.Nil(t, r.Report)
}

func TestParseNotAnObject(t *testing.T) {
	for _, raw := range [][]byte{[]byte("null"), []byte("[1,2]"), []byte(`"str"`), []byte("42")} {
		r := Parse(raw, nil)
		assert.False(t, r.OK)
		assert.Equal(t, []string{"not-an-object"}, r.Errors)
		assert.Nil(t, r.Report)
	}
}

func TestParseDefaultKeysMissing(t *testing.T) {
	raw := mustJSON(t, map[string]any{"requirements-covered": []string{}})
	r := Parse(raw, nil)
	assert.False(t, r.OK)
	assert.ElementsMatch(t, []string{
		"missing-key: changed-files",
		"missing-key: commands-run",
		"missing-key: failures",
	}, r.Errors)
}

func TestParseKeyViolations(t *testing.T) {
	format := map[string][]string{
		"requirements-covered": {},
		"changed-files":        {},
	}
	// 三种违规各一：缺键 / 非数组 / 非字符串元素。
	raw := mustJSON(t, map[string]any{
		"requirements-covered": "not-array",
		"changed-files":        []any{"ok", 42},
	})
	r := Parse(raw, format)
	assert.False(t, r.OK)
	// 键按字典序遍历：changed-files < requirements-covered。
	assert.Equal(t, []string{
		"non-string-element: changed-files",
		"not-an-array: requirements-covered",
	}, r.Errors)
}

// TestParseUnexpectedKeyRejected（C8 回归）：format 声明键之外的多余键拒绝。
func TestParseUnexpectedKeyRejected(t *testing.T) {
	format := map[string][]string{"requirements-covered": {}}
	raw := mustJSON(t, map[string]any{
		"requirements-covered": []string{"R-01"},
		"smuggled":             []string{"x"}, // executor 私自夹带
	})
	r := Parse(raw, format)
	assert.False(t, r.OK)
	assert.Equal(t, []string{"unexpected-key: smuggled"}, r.Errors)
}

func TestParseEmptyFormatChecksNothing(t *testing.T) {
	// 忠实 Node：format 为非 nil 空对象 → Object.keys = [] → 零键检查。
	// （schema 已禁 done-report-format:{}，此处仅锁定代码语义。）
	r := Parse([]byte(`{"anything":1}`), map[string][]string{})
	assert.True(t, r.OK)
}

func TestParseErrorsDefaultKeyOrder(t *testing.T) {
	// nil format：按 DEFAULT_FORMAT_KEYS 常量序（Node 插入序）遍历，确定性锁定。
	raw := mustJSON(t, map[string]any{})
	r := Parse(raw, nil)
	assert.Equal(t, []string{
		"missing-key: requirements-covered",
		"missing-key: changed-files",
		"missing-key: commands-run",
		"missing-key: failures",
	}, r.Errors)
}
