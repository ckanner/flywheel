package contract

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func sessionSchemaValue() map[string]any {
	return map[string]any{"version": 1, "records": map[string]any{
		"execute/M01-T01": map[string]any{
			"task": "/repo/docs/task", "role": "execute", "scope": "M01-T01", "cwd": "/repo/worktree",
			"binding": strings.Repeat("a", 64), "goal": strings.Repeat("b", 64), "contract": strings.Repeat("c", 64),
			"generation": 1, "thread-id": "opaque thread ID", "protocol": "backend/v1", "resume-candidate": true,
			"action": "create", "reason": "first invocation",
			"last": map[string]any{
				"id": "call-one", "status": "received", "started-at": "2026-11-07T12:00:00Z", "attempt": 1,
				"persistent": true, "candidate-before": strings.Repeat("d", 64), "candidate-after": strings.Repeat("e", 64),
				"guard-before": strings.Repeat("a", 64), "guard-after": strings.Repeat("b", 64),
				"receipt-digest": strings.Repeat("f", 64), "accounted": false,
				"dispatch": "dispatched", "outcome": "completed", "cleanup-confirmed": true, "usage-status": "complete",
			},
		},
	}}
}

func TestSessionSchemaRejectsMalformedExtensionBeforeDispatch(t *testing.T) {
	require.NoError(t, ValidateSessionStateValue(sessionSchemaValue()))
	require.NoError(t, ValidateSessionStateValue(map[string]any{"version": 1, "records": map[string]any{}}))
	for _, tc := range []struct {
		name string
		edit func(map[string]any, map[string]any, map[string]any)
	}{
		{"unknown version", func(v, _, _ map[string]any) { v["version"] = 2 }},
		{"missing version", func(v, _, _ map[string]any) { delete(v, "version") }},
		{"null records", func(v, _, _ map[string]any) { v["records"] = nil }},
		{"unknown ledger field", func(v, _, _ map[string]any) { v["trust-me"] = true }},
		{"unknown record field", func(_, r, _ map[string]any) { r["trust-me"] = true }},
		{"unknown invocation field", func(_, _, i map[string]any) { i["trust-me"] = true }},
		{"missing task", func(_, r, _ map[string]any) { delete(r, "task") }},
		{"unknown role", func(_, r, _ map[string]any) { r["role"] = "executor-reviewer" }},
		{"empty scope", func(_, r, _ map[string]any) { r["scope"] = "" }},
		{"wrong binding", func(_, r, _ map[string]any) { r["binding"] = "not-a-digest" }},
		{"negative generation", func(_, r, _ map[string]any) { r["generation"] = -1 }},
		{"fractional generation", func(_, r, _ map[string]any) { r["generation"] = 1.5 }},
		{"missing resume handle", func(_, r, _ map[string]any) { delete(r, "thread-id") }},
		{"missing resume protocol", func(_, r, _ map[string]any) { delete(r, "protocol") }},
		{"unknown action", func(_, r, _ map[string]any) { r["action"] = "force-replay" }},
		{"nil invocation", func(_, r, _ map[string]any) { r["last"] = nil }},
		{"invalid invocation ID", func(_, _, i map[string]any) { i["id"] = "path/../call" }},
		{"unknown invocation status", func(_, _, i map[string]any) { i["status"] = "maybe-running" }},
		{"negative attempt", func(_, _, i map[string]any) { i["attempt"] = -1 }},
		{"nonboolean persistence", func(_, _, i map[string]any) { i["persistent"] = "true" }},
		{"invalid candidate digest", func(_, _, i map[string]any) { i["candidate-before"] = "candidate" }},
		{"invalid original guard digest", func(_, _, i map[string]any) { i["guard-before"] = "guard" }},
		{"invalid terminal guard digest", func(_, _, i map[string]any) { i["guard-after"] = "guard" }},
		{"invalid receipt digest", func(_, _, i map[string]any) { i["receipt-digest"] = "receipt" }},
		{"missing terminal candidate", func(_, _, i map[string]any) { delete(i, "candidate-after") }},
		{"missing terminal receipt", func(_, _, i map[string]any) { delete(i, "receipt-digest") }},
		{"terminal cleanup unknown", func(_, _, i map[string]any) { i["cleanup-confirmed"] = false }},
		{"terminal dispatch unknown", func(_, _, i map[string]any) { i["dispatch"] = "unknown" }},
		{"terminal outcome unknown", func(_, _, i map[string]any) { i["outcome"] = "unknown" }},
		{"unknown usage status", func(_, _, i map[string]any) { i["usage-status"] = "zero" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := sessionSchemaValue()
			record := value["records"].(map[string]any)["execute/M01-T01"].(map[string]any)
			tc.edit(value, record, record["last"].(map[string]any))
			require.Error(t, ValidateSessionStateValue(value))
		})
	}
	require.Error(t, ValidateSessionStateValue(nil))
	require.Error(t, ValidateSessionStateValue([]any{}))
}

func TestSessionSchemaPreservesUnknownFactsInUnconsumedStates(t *testing.T) {
	for _, status := range []string{"intent", "blocked"} {
		value := sessionSchemaValue()
		record := value["records"].(map[string]any)["execute/M01-T01"].(map[string]any)
		record["resume-candidate"] = false
		delete(record, "thread-id")
		delete(record, "protocol")
		invocation := record["last"].(map[string]any)
		invocation["status"], invocation["dispatch"], invocation["outcome"] = status, "unknown", "unknown"
		invocation["cleanup-confirmed"], invocation["usage-status"] = false, "unknown"
		for _, field := range []string{"candidate-after", "receipt-digest", "persistent", "guard-before", "guard-after"} {
			delete(invocation, field)
		}
		require.NoError(t, ValidateSessionStateValue(value), "unknown invocation must remain representable to block replay")
	}
}

func TestSessionSchemaCompileFailurePropagates(t *testing.T) {
	original := compileAllFn
	t.Cleanup(func() { compileAllFn = original; schemaOnce = sync.Once{} })
	want := errors.New("embedded session schema failed")
	compileAllFn = func(func(string) ([]byte, error)) (*jsonschema.Schema, *jsonschema.Schema, *jsonschema.Schema, error) {
		return nil, nil, nil, want
	}
	schemaOnce = sync.Once{}
	require.ErrorIs(t, ValidateSessionStateValue(sessionSchemaValue()), want)
}
