package metrics

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func nativeMetric(mode string, count int) map[string]any {
	children := []any{}
	for i := range count {
		children = append(children, map[string]any{"id": fmt.Sprintf("00000000-0000-4000-8000-%012d", i), "state": "unknown", "model": nil, "logUri": "private-log-location", "role": "private-role-content"})
	}
	var command any
	if mode == "swarm" {
		command = "/swarm"
	}
	return map[string]any{"harness": "antigravity", "role": "execute", "invocation-id": "native", "receipt-sha256": "original",
		"antigravity-receipt": map[string]any{"ok": true, "output": "private-answer", "requested": map[string]any{"prompt": "private-prompt", "native": map[string]any{"version": 1, "mode": mode}},
			"native": map[string]any{"mode": mode, "dispatch": map[string]any{"method": map[string]string{"native": "prompt", "subagents": "subagent-instruction", "swarm": "slash-command"}[mode], "command": command},
				"observedAgents": count, "collaborationObserved": count > 0, "metadataTruncated": false, "limit": map[string]any{"maxObservedAgents": nil, "hard": false, "exceeded": false},
				"completion": map[string]any{"scope": "cli-invocation", "state": "completed", "source": "normal-result-and-cli-exit", "children": "unknown"},
				"children":   children, "aggregateUsageStatus": "unknown"}}}
}

func nativeObject(m map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		m = m[key].(map[string]any)
	}
	return m
}

func nativeLine(t *testing.T, m map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	return string(raw) + "\n"
}

func TestNativeMetricsExposeOnlyBoundedObservationFacts(t *testing.T) {
	for _, mode := range []string{"native", "subagents", "swarm"} {
		m := nativeMetric(mode, 1)
		report, err := parseReport(strings.NewReader(nativeLine(t, m)))
		require.NoError(t, err)
		observation := report.Calls[0].Native
		require.Equal(t, mode, observation.RequestedMode)
		require.Equal(t, "completed", observation.InvocationCompletion)
		require.Equal(t, "unknown", observation.ChildCompletion)
		require.Equal(t, "unknown", observation.AggregateUsageStatus)
		require.False(t, observation.HardLimit)
		require.Equal(t, 1, report.Groups[0].MissingUsage)
		raw, err := json.Marshal(report)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "private-")
	}
	m := nativeMetric("swarm", 25)
	r := nativeObject(m, "antigravity-receipt")
	r["ok"] = false
	nativeObject(r, "requested", "native")["maxObservedAgents"] = 4
	nativeObject(r, "native")["metadataTruncated"] = true
	nativeObject(r, "native", "limit")["maxObservedAgents"] = 4
	nativeObject(r, "native", "limit")["exceeded"] = true
	nativeObject(r, "native")["completion"] = map[string]any{"scope": "cli-invocation", "state": "unknown", "source": nil, "children": "unknown"}
	report, err := parseReport(strings.NewReader(nativeLine(t, m)))
	require.NoError(t, err)
	n := report.Calls[0].Native
	require.Equal(t, 25, n.ObservedAgents)
	require.Len(t, n.ChildIDs, 20)
	require.Equal(t, 5, n.OmittedChildren)
	require.True(t, n.GuardExceeded)
	require.True(t, n.MetadataTruncated)
	require.Equal(t, "unknown", n.InvocationCompletion)
	// A durable reconciliation can refine the projection without adding a call.
	correction := nativeMetric("swarm", 25)
	correction["antigravity-receipt"] = r
	correction["record-kind"], correction["reconciles-invocation"], correction["supersedes-receipt"], correction["receipt-sha256"] = "reconciliation", "native", "original", "refined"
	delete(correction, "harness")
	report, err = parseReport(strings.NewReader(nativeLine(t, m) + nativeLine(t, correction)))
	require.NoError(t, err)
	require.Len(t, report.Calls, 1)
	require.Equal(t, n, report.Calls[0].Native)
	for _, raw := range []string{`{"harness":"antigravity","role":"execute"}`, `{"harness":"antigravity","role":"execute","antigravity-receipt":{"native":null}}`, `{"harness":"antigravity","role":"execute","antigravity-receipt":{"native":{"children":[]}}}`} {
		report, err = parseReport(strings.NewReader(raw + "\n"))
		require.NoError(t, err)
		require.Nil(t, report.Calls[0].Native)
	}
	m = nativeMetric("native", 0)
	delete(nativeObject(m, "antigravity-receipt", "requested"), "native")
	report, err = parseReport(strings.NewReader(nativeLine(t, m)))
	require.NoError(t, err)
	require.Zero(t, report.Calls[0].Native.ObservedAgents)
}

func TestNativeMetricsRejectMalformedAndInventedEvidence(t *testing.T) {
	for name, edit := range map[string]func(map[string]any){
		"malformed receipt":      func(m map[string]any) { m["antigravity-receipt"] = 1 },
		"malformed native":       func(m map[string]any) { nativeObject(m, "antigravity-receipt")["native"] = "not-observations" },
		"bad requested contract": func(m map[string]any) { nativeObject(m, "antigravity-receipt", "requested", "native")["version"] = 2 },
		"missing count":          func(m map[string]any) { delete(nativeObject(m, "antigravity-receipt", "native"), "observedAgents") },
		"wrong count":            func(m map[string]any) { nativeObject(m, "antigravity-receipt", "native")["observedAgents"] = 5 },
		"hard limit":             func(m map[string]any) { nativeObject(m, "antigravity-receipt", "native", "limit")["hard"] = true },
		"whole tree completion": func(m map[string]any) {
			nativeObject(m, "antigravity-receipt", "native", "completion")["children"] = "completed"
		},
		"metadata truncation claimed successful": func(m map[string]any) { nativeObject(m, "antigravity-receipt", "native")["metadataTruncated"] = true },
		"unsafe identity": func(m map[string]any) {
			nativeObject(m, "antigravity-receipt", "native")["children"].([]any)[0].(map[string]any)["id"] = "not-an-observed-uuid"
		},
		"invented child completion": func(m map[string]any) {
			nativeObject(m, "antigravity-receipt", "native")["children"].([]any)[0].(map[string]any)["state"] = "completed"
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := nativeMetric("swarm", 1)
			edit(m)
			_, err := parseReport(strings.NewReader(nativeLine(t, m)))
			require.Error(t, err)
		})
	}
}

func TestNativeMetricsRetainLargeLegalReceiptsAndBoundMetadataOverflow(t *testing.T) {
	m := nativeMetric("swarm", 1)
	// Both the complete public receipt and native evidence are retained in the
	// invocation log. No private body or URI text may enter the UI projection.
	r := nativeObject(m, "antigravity-receipt")
	r["output"] = strings.Repeat("private-answer", 1<<16)
	child := nativeObject(r, "native")["children"].([]any)[0].(map[string]any)
	child["workspaceUris"] = make([]string, 64)
	for i := range child["workspaceUris"].([]string) {
		child["workspaceUris"].([]string)[i] = strings.Repeat("x", 16000)
	}
	m["native-receipt"] = r["native"]
	line := nativeLine(t, m)
	require.Greater(t, len(line), 2<<20)
	require.Less(t, len(line), 8<<20)
	report, err := parseReport(strings.NewReader(line))
	require.NoError(t, err)
	require.Len(t, report.Calls[0].Native.ChildIDs, 1)
	for _, count := range []int{1000, 1001, 1002} {
		m = nativeMetric("native", count)
		if count > 1000 {
			nativeObject(m, "antigravity-receipt")["ok"] = false
			nativeObject(m, "antigravity-receipt", "native")["metadataTruncated"] = true
		}
		report, err = parseReport(strings.NewReader(nativeLine(t, m)))
		if count > 1001 {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, count, report.Calls[0].Native.ObservedAgents)
		require.Len(t, report.Calls[0].Native.ChildIDs, 20)
		require.Equal(t, count-20, report.Calls[0].Native.OmittedChildren)
	}
}
