package drivers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/imageinput"
	"github.com/stretchr/testify/require"
)

func TestAntigravityNativeReceiptsKeepInvocationAndChildEvidenceSeparate(t *testing.T) {
	for _, mode := range []string{"native", "subagents", "swarm"} {
		r := agyRequest(t)
		r.Native = json.RawMessage(`{"version":1,"mode":"` + mode + `","maxObservedAgents":2}`)
		r.Model, r.ReasoningEffort = "gemini-pro-orchestrator-neon", ""
		p := parseAgyFixture(r, agyReceipt(r))
		require.True(t, p.Settled, "%+v", p)
		require.NotEmpty(t, p.NativeReceipt)
		require.Nil(t, p.Usage)
		require.Contains(t, string(p.NativeReceipt), `"children":"unknown"`)
	}
	r := agyRequest(t)
	r.Native = json.RawMessage(`{"version":1,"mode":"swarm","maxObservedAgents":1}`)
	for name, edit := range map[string]func(map[string]any){
		"omitted requested native": func(m map[string]any) { delete(proObject(m, "requested"), "native") },
		"changed requested guard": func(m map[string]any) {
			proObject(m, "requested")["native"] = map[string]any{"version": 1, "mode": "swarm", "maxObservedAgents": 8}
		},
		"missing observations":           func(m map[string]any) { delete(m, "native") },
		"invalid observations":           func(m map[string]any) { m["native"] = "invented" },
		"invented whole tree completion": func(m map[string]any) { proObject(m, "native", "completion")["children"] = "completed" },
		"invented hard limit":            func(m map[string]any) { proObject(m, "native", "limit")["hard"] = true },
		"altered observed count":         func(m map[string]any) { proObject(m, "native")["observedAgents"] = 9 },
		"wrong dispatch":                 func(m map[string]any) { proObject(m, "native", "dispatch")["command"] = "/model" },
		"unknown source":                 func(m map[string]any) { proObject(m, "native", "completion")["source"] = "parent-summary" },
		"no native child observed": func(m map[string]any) {
			n := proObject(m, "native")
			n["children"], n["observedAgents"], n["collaborationObserved"] = []any{}, 0, false
		},
		"invented child completion": func(m map[string]any) {
			proObject(m, "native")["children"].([]any)[0].(map[string]any)["state"] = "completed"
		},
		"invented child model": func(m map[string]any) {
			proObject(m, "native")["children"].([]any)[0].(map[string]any)["model"] = "neon"
		},
		"observed limit exceeded": func(m map[string]any) {
			n := proObject(m, "native")
			n["children"] = append(n["children"].([]any), map[string]any{"id": "another-child", "state": "unknown", "model": nil})
			n["observedAgents"] = 2
			proObject(m, "native", "limit")["exceeded"] = true
		},
		"cutoff reported as success":     func(m map[string]any) { proObject(m, "execution")["finishedReason"] = "HARD_TURN_LIMIT" },
		"finished reason has wrong type": func(m map[string]any) { proObject(m, "execution")["finishedReason"] = 1 },
		"finished reason omitted":        func(m map[string]any) { delete(proObject(m, "execution"), "finishedReason") },
		"physical fact omitted":          func(m map[string]any) { delete(proObject(m, "execution"), "pid") },
		"root aliases child": func(m map[string]any) {
			proObject(m, "native")["children"].([]any)[0].(map[string]any)["id"] = proObject(m, "session")["id"]
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := agyReceipt(r)
			edit(m)
			p := parseAgyFixture(r, m)
			require.False(t, p.Settled)
			require.NotEmpty(t, p.Error)
		})
	}
	// A duplicate child cannot be disguised as additional observed workers.
	r.Native = json.RawMessage(`{"version":1,"mode":"subagents"}`)
	m := agyReceipt(r)
	n := proObject(m, "native")
	n["children"] = append(n["children"].([]any), n["children"].([]any)[0])
	n["observedAgents"] = 2
	require.Contains(t, parseAgyFixture(r, m).Error, "duplicated")
	r.Native = json.RawMessage(`{}`)
	m = agyReceipt(r)
	require.False(t, parseAgyFixture(r, m).Settled)
}

func TestAntigravityMetadataRetentionBoundaryPreservesFailureEvidence(t *testing.T) {
	for _, count := range []int{1000, 1001, 1002} {
		r := agyRequest(t)
		m := agyReceipt(r)
		children := []any{}
		for i := range count {
			children = append(children, map[string]any{"id": fmt.Sprintf("00000000-0000-4000-8000-%012d", i), "state": "unknown", "model": nil})
		}
		n := proObject(m, "native")
		n["children"], n["observedAgents"], n["collaborationObserved"] = children, count, true
		if count > 1000 {
			m["ok"] = false
			m["failure"] = map[string]any{"category": "protocol", "stage": "run", "detail": "native metadata exceeded retention bound"}
			n["metadataTruncated"] = true
			proObject(m, "execution")["outcome"] = "failed"
		}
		p := parseAgyFixture(r, m)
		if count == 1000 {
			require.True(t, p.Settled)
		} else if count == 1001 {
			require.False(t, p.Settled)
			require.NotEmpty(t, p.NativeReceipt)
			require.Contains(t, p.Error, "retention bound")
		} else {
			require.Empty(t, p.NativeReceipt)
		}
	}
}

func TestAntigravityNativeFailureClassificationPreservesIndependentOutcomeFacts(t *testing.T) {
	for _, category := range []string{"collaboration-not-observed", "observed-agent-limit", "native-cutoff"} {
		t.Run(category, func(t *testing.T) {
			r := agyRequest(t)
			r.Native = json.RawMessage(`{"version":1,"mode":"swarm","maxObservedAgents":1}`)
			m := agyReceipt(r)
			m["ok"], m["output"] = false, []any{}
			m["failure"] = map[string]any{"category": category, "stage": "run", "detail": "native policy observation"}
			e, n := proObject(m, "execution"), proObject(m, "native")
			e["outcome"] = "failed"
			expectedOutcome := "unknown"
			switch category {
			case "collaboration-not-observed":
				n["children"], n["observedAgents"], n["collaborationObserved"] = []any{}, 0, false
			case "observed-agent-limit":
				children := n["children"].([]any)
				n["children"] = append(children, map[string]any{"id": "44444444-4444-4444-8444-444444444444", "state": "unknown", "model": nil})
				n["observedAgents"] = 2
				proObject(m, "native", "limit")["exceeded"] = true
				proObject(m, "native", "completion")["state"] = "unknown"
				proObject(m, "native", "completion")["source"] = nil
				e["outcome"], e["terminalReceived"], e["exitCode"], e["signal"] = "unknown", false, nil, "SIGTERM"
				m["providerWork"] = map[string]any{"state": "unknown", "replaySafe": false}
			case "native-cutoff":
				e["finishedReason"] = "HARD_TURN_LIMIT"
				proObject(m, "providerWork")["state"] = "failed"
				proObject(m, "native", "completion")["state"] = "failed"
				proObject(m, "native", "completion")["source"] = nil
				expectedOutcome = "failed"
			}
			p := parseAgyFixture(r, m)
			require.Equal(t, errs.INVALID_REQUEST, p.ErrorCode)
			require.Contains(t, p.Error, category)
			require.Contains(t, p.Error, "native policy observation")
			require.Equal(t, expectedOutcome, p.Session.Outcome)
			require.True(t, p.CleanupConfirmed, "local cleanup remains a separate observed fact")
			require.NotEmpty(t, p.NativeReceipt)
			require.False(t, p.Settled)
			require.Nil(t, p.Usage)
		})
	}
}

func TestRunAntigravitySupportsNativeAttributionAndRejectsUnsafeRoles(t *testing.T) {
	a := agyRunArgs(t)
	a.RequestID = "native-run"
	a.Model, a.ReasoningEffort = "gemini-pro-orchestrator-neon", ""
	a.Native = json.RawMessage(`{"version":1,"mode":"subagents","maxObservedAgents":4}`)
	a.Managed = &NativeAdmission{Version: 1, RunID: "run", InvocationID: a.RequestID, Role: "execute", Scope: "packet:M01-T01", Channel: "google", TaskDir: a.Cwd}
	r := RunAntigravity(a)
	require.True(t, r.Settled, "%+v", r)
	require.NotEmpty(t, r.NativeReceipt)
	raw, err := os.ReadFile(filepath.Join(a.Cwd, "captured-agy.json"))
	require.NoError(t, err)
	var capture struct{ Request codexProStart }
	require.NoError(t, json.Unmarshal(raw, &capture))
	require.JSONEq(t, string(a.Native), string(capture.Request.Native))
	require.Equal(t, a.Managed, capture.Request.Managed)
	require.Empty(t, capture.Request.ReasoningEffort)
	for name, edit := range map[string]func(*AntigravityArgs){
		"evidence native": func(a *AntigravityArgs) {
			a.EvidenceOnly, a.PermissionMode = true, "read-only"
			a.Native = json.RawMessage(`{"version":1,"mode":"native"}`)
		},
		"evidence orchestrator": func(a *AntigravityArgs) {
			a.EvidenceOnly, a.PermissionMode = true, "read-only"
			a.Model, a.ReasoningEffort = "gemini-pro-orchestrator", ""
		},
		"planning swarm": func(a *AntigravityArgs) {
			a.Native = json.RawMessage(`{"version":1,"mode":"swarm"}`)
			a.Managed = &NativeAdmission{Role: "plan"}
		},
		"images": func(a *AntigravityArgs) { a.Images = []imageinput.Image{{}} },
	} {
		t.Run(name, func(t *testing.T) {
			a := agyRunArgs(t)
			edit(&a)
			r := RunAntigravity(a)
			require.False(t, r.Settled)
			require.Equal(t, "not-dispatched", r.Session.Dispatch)
			require.NoFileExists(t, filepath.Join(a.Cwd, "captured-agy.json"))
		})
	}
}
