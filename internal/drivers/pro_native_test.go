package drivers

import (
	"encoding/json"
	"testing"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func proNativeFixture(t *testing.T, request codexProStart, receipt map[string]any, harness string, child bool) map[string]any {
	t.Helper()
	var requested any
	require.NoError(t, json.Unmarshal(request.Native, &requested))
	native := map[string]any{
		"version": 1, "requested": requested,
		"effective": map[string]any{"source": "config/read", "matched": true, "config": map[string]any{"agents.enabled": true}},
		"observed":  map[string]any{"events": []any{}, "truncated": false}, "children": []any{},
		"completion": map[string]any{"confirmed": true, "pendingIds": []string{}},
		"cleanup":    map[string]any{"confirmed": true, "scope": "local-process-group", "pendingIds": []string{}},
	}
	if harness == "codex-pro" {
		usage := map[string]any{"strategy": "per-thread", "scope": "observed-children", "status": "unknown", "children": map[string]any{}, "totalTokens": nil}
		if child {
			report := proObject(receipt, "outcome", "usage")
			native["children"] = []any{map[string]any{"id": "child", "parentId": "thread-one", "kind": "agent", "state": "completed", "turnId": "child-turn", "usage": report}}
			usage["status"], usage["children"], usage["totalTokens"] = "complete", map[string]any{"child": report}, int64(18)
		}
		native["usage"] = usage
	} else {
		native["effective"] = map[string]any{"source": "cli-init+invocation", "matched": true, "verification": "tools-only", "config": map[string]any{"tools": []string{"Agent"}}}
		usage := map[string]any{"strategy": "root-result-only", "status": "complete", "childSnapshotsAreAdditive": false,
			"aggregateFamilyStatus": "complete", "root": receipt["usage"], "rootResults": []any{}}
		if child {
			native["children"] = []any{map[string]any{"id": "child", "parentId": nil, "ownerId": proObject(receipt, "session")["id"], "kind": "local_agent", "state": "completed", "turnId": "run-1",
				"usage": map[string]any{"status": "observed", "scope": "task-run", "total_tokens": 1000, "duration_ms": 10, "tool_uses": 2}}}
			usage["aggregateFamilyStatus"] = "unknown"
		}
		native["usage"] = usage
	}
	receipt["native"] = native
	return native
}

func TestProNativeCodexAccountingUsesOneExclusiveDeltaPerOwnedThread(t *testing.T) {
	r := codexProRequest(t)
	r.Native = json.RawMessage(`{"version":1,"agents":{"enabled":true}}`)
	m := codexProFixture(r)
	n := proNativeFixture(t, r, m, "codex-pro", true)
	p := parseCodexProStream(dshJSONL(codexProFrame(r, m)), r)
	require.True(t, p.Settled, "%+v", p)
	require.NotNil(t, p.Usage)
	assert.Equal(t, int64(36), p.Usage.TotalTokens)
	assert.Equal(t, int64(20), p.Usage.Input)
	assert.Equal(t, int64(16), p.Usage.Output)
	assert.Equal(t, "complete", p.Session.UsageStatus)
	assert.NotEmpty(t, p.NativeReceipt)
	assert.Equal(t, 18, proObject(m, "outcome", "usage", "run")["totalTokens"], "root usage is never overwritten with the family total")

	for _, status := range []string{"partial", "unknown"} {
		report := map[string]any{"status": status, "reason": "historical baseline or missing native event"}
		proObject(n, "usage")["children"] = map[string]any{"child": report}
		proObject(n, "usage")["status"], proObject(n, "usage")["totalTokens"] = "partial", nil
		n["children"].([]any)[0].(map[string]any)["usage"] = report
		p = parseCodexProStream(dshJSONL(codexProFrame(r, m)), r)
		require.True(t, p.Settled, "%+v", p)
		assert.Nil(t, p.Usage)
		assert.Equal(t, "partial", p.Session.UsageStatus)
	}
}

func TestProNativeCodexNoObservationIsNotAZeroChildBill(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		r := codexProRequest(t)
		r.Native, _ = json.Marshal(map[string]any{"version": 1, "agents": map[string]any{"enabled": enabled}})
		m := codexProFixture(r)
		proNativeFixture(t, r, m, "codex-pro", false)
		p := parseCodexProStream(dshJSONL(codexProFrame(r, m)), r)
		require.True(t, p.Settled, "%+v", p)
		if enabled {
			assert.Nil(t, p.Usage)
			assert.Equal(t, "unknown", p.Session.UsageStatus)
		} else {
			require.NotNil(t, p.Usage)
			assert.Equal(t, int64(18), p.Usage.TotalTokens)
		}
	}
}

func TestProNativeClaudeSnapshotsAndAdvisorNeverBecomeAnAdditiveBill(t *testing.T) {
	for _, mode := range []string{"root-only", "child", "advisor"} {
		r := claudeCodeProRequest(t)
		r.Native = json.RawMessage(`{"version":1,"agents":{"enabled":true}}`)
		if mode == "advisor" {
			r.Native = json.RawMessage(`{"version":1,"advisor":{"enabled":true,"model":"opus"}}`)
		}
		m := claudeCodeProFixture(r)
		n := proNativeFixture(t, codexProStart(r), m, "claude-code-pro", mode == "child")
		if mode == "child" {
			// Another run of the same task is a different ledger entry; neither snapshot is additive.
			first := n["children"].([]any)[0].(map[string]any)
			second := map[string]any{}
			for key, value := range first {
				second[key] = value
			}
			second["turnId"] = "run-2"
			n["children"] = []any{first, second}
		}
		p := parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(r, m)), r)
		require.True(t, p.Settled, "%+v", p)
		if mode == "root-only" {
			require.NotNil(t, p.Usage)
			assert.Equal(t, int64(22), p.Usage.TotalTokens)
		} else {
			assert.Nil(t, p.Usage)
			assert.Equal(t, "unknown", p.Session.UsageStatus)
		}
		assert.NotEmpty(t, p.NativeReceipt)
		assert.Equal(t, 22, proObject(m, "usage")["totalTokens"])
	}
}

func TestProNativeMalformedEvidenceCannotAuthorizeCompletion(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		for _, tc := range []struct {
			name string
			edit func(map[string]any)
		}{
			{"version", func(n map[string]any) { n["version"] = 2 }},
			{"requested", func(n map[string]any) { n["requested"] = map[string]any{"version": 1} }},
			{"effective", func(n map[string]any) { proObject(n, "effective")["matched"] = false }},
			{"effective source", func(n map[string]any) { proObject(n, "effective")["source"] = "prompt" }},
			{"missing config", func(n map[string]any) { delete(proObject(n, "effective"), "config") }},
			{"missing observed", func(n map[string]any) { delete(n, "observed") }},
			{"truncated", func(n map[string]any) { proObject(n, "observed")["truncated"] = true }},
			{"children missing", func(n map[string]any) { delete(n, "children") }},
			{"completion missing", func(n map[string]any) { delete(n, "completion") }},
			{"completion pending", func(n map[string]any) { proObject(n, "completion")["confirmed"] = false }},
			{"cleanup missing", func(n map[string]any) { delete(n, "cleanup") }},
			{"cleanup pending", func(n map[string]any) { proObject(n, "cleanup")["confirmed"] = false }},
			{"unknown cleanup scope", func(n map[string]any) { proObject(n, "cleanup")["scope"] = "global" }},
			{"unlisted pending child", func(n map[string]any) { proObject(n, "cleanup")["pendingIds"] = []string{"foreign"} }},
			{"child still running", func(n map[string]any) { n["children"].([]any)[0].(map[string]any)["state"] = "running" }},
			{"duplicate child run", func(n map[string]any) { n["children"] = append(n["children"].([]any), n["children"].([]any)[0]) }},
			{"usage strategy", func(n map[string]any) { proObject(n, "usage")["strategy"] = "sum-snapshots" }},
		} {
			t.Run(harness+"/"+tc.name, func(t *testing.T) {
				r := codexProRequest(t)
				if harness == "claude-code-pro" {
					r = codexProStart(claudeCodeProRequest(t))
				}
				r.Native = json.RawMessage(`{"version":1,"agents":{"enabled":true}}`)
				var m map[string]any
				if harness == "codex-pro" {
					m = codexProFixture(r)
				} else {
					m = claudeCodeProFixture(claudeCodeProStart(r))
				}
				tc.edit(proNativeFixture(t, r, m, harness, true))
				if harness == "codex-pro" {
					p := parseCodexProStream(dshJSONL(codexProFrame(r, m)), r)
					assert.False(t, p.Settled)
					assert.Nil(t, p.Usage)
					assert.NotEmpty(t, p.Error)
					assert.NotEmpty(t, p.NativeReceipt)
				} else {
					p := parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(claudeCodeProStart(r), m)), claudeCodeProStart(r))
					assert.False(t, p.Settled)
					assert.Nil(t, p.Usage)
					assert.NotEmpty(t, p.Error)
					assert.NotEmpty(t, p.NativeReceipt)
				}
			})
		}
	}
}

func TestProNativeClaudeRemoteCancellationKeepsLocalCleanupSeparate(t *testing.T) {
	r := claudeCodeProRequest(t)
	r.Native = json.RawMessage(`{"version":1,"agents":{"enabled":true},"workflows":{"enabled":true}}`)
	m := claudeCodeProFixture(r)
	n := proNativeFixture(t, codexProStart(r), m, "claude-code-pro", true)
	m["ok"], m["output"] = false, []any{}
	m["failure"] = map[string]any{"stage": "cleanup", "category": "cleanup-unconfirmed", "detail": "native remote work remains unconfirmed after local cleanup"}
	proObject(m, "execution")["cancelled"] = true
	proObject(n, "cleanup")["confirmed"], proObject(n, "cleanup")["scope"], proObject(n, "cleanup")["pendingIds"] = false, "remote-job", []string{"child"}
	proObject(n, "completion")["confirmed"], proObject(n, "completion")["pendingIds"] = false, []string{"child"}
	n["children"].([]any)[0].(map[string]any)["state"] = "running"
	n["children"].([]any)[0].(map[string]any)["kind"] = "remote_agent"
	p := parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(r, m)), r)
	assert.False(t, p.Settled)
	assert.False(t, p.CleanupConfirmed)
	assert.True(t, p.Cancelled)
	assert.Equal(t, "unknown", p.Session.Outcome)
	assert.Nil(t, p.Usage)
	assert.Contains(t, p.Error, "native remote work remains unconfirmed")
	assert.Equal(t, true, proObject(m, "cleanup")["confirmed"])
}

func TestProNativeCodexCancellationDoesNotTurnStoppedProcessesIntoFinishedChildren(t *testing.T) {
	r := codexProRequest(t)
	r.Native = json.RawMessage(`{"version":1,"agents":{"enabled":true}}`)
	m := codexProFixture(r)
	n := proNativeFixture(t, r, m, "codex-pro", true)
	m["ok"] = false
	proObject(m, "outcome")["stopReason"] = "aborted"
	proObject(m, "outcome", "execution")["cancelled"] = true
	proObject(n, "completion")["confirmed"], proObject(n, "completion")["pendingIds"] = false, []string{"child"}
	n["children"].([]any)[0].(map[string]any)["state"] = "running"
	p := parseCodexProStream(dshJSONL(codexProFrame(r, m)), r)
	assert.False(t, p.Settled)
	assert.True(t, p.CleanupConfirmed)
	assert.True(t, p.Cancelled)
	assert.Equal(t, "unknown", p.Session.Outcome)
	assert.Nil(t, p.Usage, "the last child snapshot is only a lower bound when completion is missing")
	assert.Equal(t, "partial", p.Session.UsageStatus)
	assert.NotEmpty(t, p.NativeReceipt)
}

func TestProNativeRecoveryPreservesOriginalReceiptAndAccounting(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		r := codexProRequest(t)
		if harness == "claude-code-pro" {
			r = codexProStart(claudeCodeProRequest(t))
		}
		r.Native = json.RawMessage(`{"version":1,"agents":{"enabled":true}}`)
		m := codexProFixture(r)
		if harness == "claude-code-pro" {
			m = claudeCodeProFixture(claudeCodeProStart(r))
		}
		proNativeFixture(t, r, m, harness, true)
		raw, err := json.Marshal(m)
		require.NoError(t, err)
		out := recoveredMachineResult(harness, raw, r)
		require.True(t, out.Settled, "%+v", out)
		assert.NotEmpty(t, out.NativeReceipt)
		assert.True(t, out.Recovered)
		if harness == "codex-pro" {
			require.NotNil(t, out.Usage)
			assert.Equal(t, int64(36), out.Usage.TotalTokens)
		} else {
			assert.Nil(t, out.Usage)
		}
	}
}

func TestProNativeEventsAreCorrelatedMetadataAndCannotSupplyTheAnswer(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		r := codexProRequest(t)
		if harness == "claude-code-pro" {
			r = codexProStart(claudeCodeProRequest(t))
		}
		r.Native = json.RawMessage(`{"version":1,"agents":{"enabled":true}}`)
		r.Managed = &NativeAdmission{Version: 1, RunID: "run", InvocationID: r.RequestID, Role: "Executor", Scope: "scope", Channel: "c", TaskDir: r.Cwd}
		m := codexProFixture(r)
		if harness == "claude-code-pro" {
			m = claudeCodeProFixture(claudeCodeProStart(r))
		}
		proNativeFixture(t, r, m, harness, true)
		frame := codexProFrame(r, m)
		if harness == "claude-code-pro" {
			frame = claudeCodeProFrame(claudeCodeProStart(r), m)
		}
		event := map[string]any{"type": "native-event", "protocol": r.Protocol, "requestId": r.RequestID, "adapter": frame["adapter"], "managed": r.Managed,
			"native": map[string]any{"version": 1, "sequence": 1, "kind": "child-completed", "id": "child", "parentId": nil, "state": "completed"}}
		if harness == "codex-pro" {
			p := parseCodexProStream(dshJSONL(event, frame), r)
			require.True(t, p.Settled, "%+v", p)
			assert.Equal(t, "verified response", p.Text)
			assert.False(t, parseCodexProStream(dshJSONL(event, event, frame), r).Settled)
		} else {
			p := parseClaudeCodeProStream(dshJSONL(event, frame), claudeCodeProStart(r))
			require.True(t, p.Settled, "%+v", p)
			assert.Equal(t, "verified response", p.Text)
			assert.False(t, parseClaudeCodeProStream(dshJSONL(event, event, frame), claudeCodeProStart(r)).Settled)
		}
		for _, edit := range []func(map[string]any){
			func(e map[string]any) { proObject(e, "native")["version"] = 2 },
			func(e map[string]any) { e["managed"] = nil },
			func(e map[string]any) { proObject(e, "adapter")["version"] = "old" },
		} {
			raw, err := json.Marshal(event)
			require.NoError(t, err)
			var copied map[string]any
			require.NoError(t, json.Unmarshal(raw, &copied))
			edit(copied)
			raw, err = json.Marshal(copied)
			require.NoError(t, err)
			var seq int64
			assert.Error(t, validateProNativeEvent(raw, harness, r, &seq))
			assert.Zero(t, seq)
		}
	}
}

func TestProNativeInvalidRequestsAndFormalReviewRejectBeforeWorker(t *testing.T) {
	for _, native := range []json.RawMessage{json.RawMessage(`{bad`), json.RawMessage(`null`), json.RawMessage(`{"version":2}`)} {
		r := RunCodexPro(CodexProArgs{Native: native, Model: "m", Prompt: "p", Cwd: t.TempDir(), Command: "/no-worker-may-start", Watchdog: watchdog.Opts{}})
		assert.Equal(t, "not-dispatched", r.Session.Dispatch)
		assert.Empty(t, r.SpawnError)
		assert.NotNil(t, r.ErrorMessage)
		c := RunClaudeCodePro(ClaudeCodeProArgs{Native: native, Model: "sonnet", Prompt: "p", Cwd: t.TempDir(), PermissionMode: "danger-full-access", Command: "/no-worker-may-start"})
		assert.Equal(t, "not-dispatched", c.Session.Dispatch)
		assert.Empty(t, c.SpawnError)
		assert.NotNil(t, c.ErrorMessage)
	}
	r := RunCodexPro(CodexProArgs{Native: json.RawMessage(`{"version":1,"review":{"target":{"type":"uncommittedChanges"}}}`),
		Model: "m", Prompt: "mandatory formal verdict contract", Cwd: t.TempDir(), Command: "/no-worker-may-start"})
	assert.Equal(t, "not-dispatched", r.Session.Dispatch)
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "independent job")
}

func TestProNativeMissingEvidenceAndNonDispatchRemainDistinct(t *testing.T) {
	r := codexProRequest(t)
	r.Native = json.RawMessage(`{"version":1,"agents":{"enabled":true}}`)
	require.NoError(t, validateProNativeRequest("codex-pro", r.Native))
	root := &Usage{Input: 10, Output: 8, TotalTokens: 18}
	m := codexProFixture(r)
	n := proNativeFixture(t, r, m, "codex-pro", false)
	raw, err := json.Marshal(n)
	require.NoError(t, err)
	_, err = foldProNative("codex-pro", nil, raw, true, true, false, "thread-one", root, "complete")
	assert.ErrorContains(t, err, "unexpected native")
	_, err = foldProNative("codex-pro", r.Native, nil, true, true, false, "thread-one", root, "complete")
	assert.ErrorContains(t, err, "omitted")
	for _, notDispatched := range []bool{false, true} {
		out, err := foldProNative("codex-pro", r.Native, nil, false, true, notDispatched, "thread-one", root, "complete")
		require.NoError(t, err)
		assert.Equal(t, notDispatched, out.Cleanup)
		assert.Equal(t, notDispatched, out.Completion)
		if notDispatched {
			assert.Equal(t, root, out.Usage)
		} else {
			assert.Nil(t, out.Usage)
		}
	}
	pre, err := foldProNative("codex-pro", r.Native, raw, false, true, true, "", &Usage{}, "complete")
	require.NoError(t, err)
	assert.Equal(t, &Usage{}, pre.Usage)
	n = proNativeFixture(t, r, m, "codex-pro", true)
	raw, err = json.Marshal(n)
	require.NoError(t, err)
	_, err = foldProNative("codex-pro", r.Native, raw, false, true, true, "thread-one", root, "complete")
	assert.ErrorContains(t, err, "non-dispatch")
	proObject(n, "completion")["confirmed"] = false
	proObject(n, "completion")["pendingIds"] = []string{"not-in-ledger"}
	raw, err = json.Marshal(n)
	require.NoError(t, err)
	_, err = foldProNative("codex-pro", r.Native, raw, false, true, false, "thread-one", root, "complete")
	assert.ErrorContains(t, err, "pending identity")
}

func TestProNativeCodexCounterAndAncestryGuards(t *testing.T) {
	for _, tc := range []struct {
		name    string
		edit    func(map[string]any)
		invalid bool
	}{
		{"child without dispatch", func(n map[string]any) { proObject(n, "requested", "agents")["enabled"] = false }, true},
		{"invented empty bill", func(n map[string]any) {
			n["children"] = []any{}
			proObject(n, "usage")["children"], proObject(n, "usage")["status"], proObject(n, "usage")["totalTokens"] = map[string]any{}, "unknown", 0
		}, true},
		{"unknown parent", func(n map[string]any) { n["children"].([]any)[0].(map[string]any)["parentId"] = "unowned" }, true},
		{"missing parent", func(n map[string]any) { n["children"].([]any)[0].(map[string]any)["parentId"] = nil }, true},
		{"cyclic parent", func(n map[string]any) { n["children"].([]any)[0].(map[string]any)["parentId"] = "child" }, true},
		{"map differs from ledger", func(n map[string]any) {
			proObject(n, "usage", "children")["child"] = map[string]any{"status": "unknown"}
		}, true},
		{"invalid counters", func(n map[string]any) {
			proObject(n["children"].([]any)[0].(map[string]any), "usage", "run")["totalTokens"] = -1
		}, true},
		{"false total", func(n map[string]any) { proObject(n, "usage")["totalTokens"] = 17 }, true},
		{"partial fabricated total", func(n map[string]any) { proObject(n, "usage")["status"] = "partial" }, true},
		{"nested independent child", func(n map[string]any) {
			first := n["children"].([]any)[0].(map[string]any)
			second := map[string]any{"id": "grandchild", "parentId": "child", "kind": "agent", "state": "completed", "turnId": "grand-turn", "usage": first["usage"]}
			n["children"] = []any{first, second}
			proObject(n, "usage", "children")["grandchild"] = first["usage"]
			proObject(n, "usage")["totalTokens"] = 36
		}, false},
		{"family counter overflow", func(n map[string]any) {
			large := map[string]any{"status": "complete", "run": map[string]any{"inputTokens": int64(1<<53 - 1), "outputTokens": 0,
				"cachedInputTokens": 0, "cacheWriteInputTokens": 0, "reasoningOutputTokens": 0, "totalTokens": int64(1<<53 - 1)}}
			n["children"].([]any)[0].(map[string]any)["usage"] = large
			proObject(n, "usage", "children")["child"], proObject(n, "usage")["totalTokens"] = large, int64(1<<53-1)
		}, true},
		{"child aggregate counter overflow", func(n map[string]any) {
			large := map[string]any{"status": "complete", "run": map[string]any{"inputTokens": int64(1<<53 - 1), "outputTokens": 0,
				"cachedInputTokens": 0, "cacheWriteInputTokens": 0, "reasoningOutputTokens": 0, "totalTokens": int64(1<<53 - 1)}}
			first := n["children"].([]any)[0].(map[string]any)
			first["usage"] = large
			n["children"] = []any{first, map[string]any{"id": "sibling", "parentId": "thread-one", "kind": "agent", "state": "completed", "turnId": "sibling-turn", "usage": large}}
			proObject(n, "usage")["children"] = map[string]any{"child": large, "sibling": large}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := codexProRequest(t)
			r.Native = json.RawMessage(`{"version":1,"agents":{"enabled":true}}`)
			m := codexProFixture(r)
			n := proNativeFixture(t, r, m, "codex-pro", true)
			tc.edit(n)
			requested, err := json.Marshal(n["requested"])
			require.NoError(t, err)
			raw, err := json.Marshal(n)
			require.NoError(t, err)
			out, err := foldProNative("codex-pro", requested, raw, true, true, false, "thread-one", &Usage{Input: 10, Output: 8, TotalTokens: 18}, "complete")
			if tc.invalid {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, out.Usage)
				assert.Equal(t, int64(54), out.Usage.TotalTokens)
			}
		})
	}
}

func TestProNativeClaudeRootProofCannotInventACompleteTotal(t *testing.T) {
	r := claudeCodeProRequest(t)
	r.Native = json.RawMessage(`{"version":1}`)
	m := claudeCodeProFixture(r)
	n := proNativeFixture(t, codexProStart(r), m, "claude-code-pro", false)
	raw, err := json.Marshal(n)
	require.NoError(t, err)
	_, err = foldProNative("claude-code-pro", r.Native, raw, true, true, false, proObject(m, "session")["id"].(string), &Usage{}, "complete")
	assert.ErrorContains(t, err, "root usage contradicts")
}
