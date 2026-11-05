package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func codexProRequest(t *testing.T) codexProStart {
	t.Helper()
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return codexProStart{Type: "start", Protocol: 1, RequestID: "request-one", Cwd: cwd,
		Model: "gpt-test", ReasoningEffort: "high", PermissionMode: "workspace-write", Prompt: "task", TimeoutMS: 3000}
}

func codexProFixture(r codexProStart) map[string]any {
	machinePermission := r.PermissionMode
	policy := map[string]any{"type": "workspaceWrite", "writableRoots": []string{r.Cwd}, "networkAccess": false,
		"excludeTmpdirEnvVar": false, "excludeSlashTmp": false}
	if r.PermissionMode == "read-only" {
		policy = map[string]any{"type": "readOnly", "networkAccess": false}
	}
	if r.PermissionMode == "danger-full-access" {
		machinePermission = "dangerously-bypass-approvals-and-sandbox"
		policy = map[string]any{"type": "dangerFullAccess"}
	}
	return map[string]any{
		"protocol": 1, "type": "result", "requestId": r.RequestID, "runId": "run-one", "ok": true,
		"requested": map[string]any{"cwd": r.Cwd, "model": r.Model, "effort": r.ReasoningEffort, "permissionMode": machinePermission, "threadId": nil},
		"costUSD":   nil, "costStatus": "unknown", "failure": nil,
		"outcome": map[string]any{
			"output": []any{map[string]any{"type": "text", "text": "verified response"}}, "stopReason": "completed", "threadId": "thread-one",
			"configuration": map[string]any{
				"source": "thread/start", "matched": true, "missing": []string{}, "mismatches": []string{},
				"requested": map[string]any{"cwd": r.Cwd, "model": r.Model, "approvalPolicy": "never", "sandbox": r.PermissionMode},
				"effective": map[string]any{"cwd": r.Cwd, "model": r.Model, "approvalPolicy": "never", "sandbox": r.PermissionMode, "sandboxPolicy": policy},
			},
			"turnConfiguration": map[string]any{"requestedEffort": r.ReasoningEffort, "effectiveEffort": nil, "evidence": "turn/start-request", "accepted": true},
			"cleanup":           map[string]any{"confirmed": true},
			"usage":             map[string]any{"status": "complete", "run": map[string]any{"totalTokens": 18, "inputTokens": 10, "cachedInputTokens": 6, "cacheWriteInputTokens": 2, "outputTokens": 8, "reasoningOutputTokens": 3}, "observations": 2, "replayedBaseline": false},
			"rateLimits":        map[string]any{"observations": 0},
			"execution": map[string]any{
				"runId": "run-one", "threadId": "thread-one", "turnId": "turn-one", "ownership": "supervised-group",
				"pid": 123, "supervisorPid": 122, "exitCode": nil, "signal": "SIGTERM", "supervisorExitCode": 0, "supervisorSignal": nil,
				"terminationRequested": true, "timedOut": false, "cancelled": false, "truncated": false, "spawnError": nil, "failure": nil,
				"startedAt": "2026-11-03T00:00:00.000Z", "finishedAt": "2026-11-03T00:00:01.000Z", "durationMs": 1000,
			},
		},
	}
}

func codexProFrame(r codexProStart, receipt map[string]any) map[string]any {
	return map[string]any{"protocol": 1, "type": "result", "requestId": r.RequestID,
		"adapter": map[string]any{"package": "dsh-subagent-codex-pro", "version": CodexProPackageVersion, "protocol": 1}, "receipt": receipt, "error": nil}
}

func proObject(m map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		m = m[key].(map[string]any)
	}
	return m
}

func TestCodexProReceiptRequiresIndependentCompletionEvidence(t *testing.T) {
	request := codexProRequest(t)
	receipt := codexProFixture(request)
	progress := map[string]any{"type": "progress", "protocol": 1, "requestId": request.RequestID, "text": "working"}
	parsed := parseCodexProStream("\n"+dshJSONL(progress, codexProFrame(request, receipt)), request)
	require.True(t, parsed.Settled, "%+v", parsed)
	assert.Equal(t, "verified response", parsed.Text)
	assert.Equal(t, "thread-one", parsed.SessionID)
	require.NotNil(t, parsed.Usage)
	assert.Equal(t, Usage{Input: 10, Output: 8, Reasoning: 3, CacheRead: 6, CacheWrite: 2, TotalTokens: 18}, *parsed.Usage)
	var kept map[string]any
	require.NoError(t, json.Unmarshal(parsed.Receipt, &kept))
	assert.Equal(t, "unknown", kept["costStatus"])
	assert.Nil(t, proObject(kept, "outcome", "turnConfiguration")["effectiveEffort"])

	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"version", func(r map[string]any) { r["protocol"] = 2 }},
		{"request identity", func(r map[string]any) { r["requestId"] = "foreign" }},
		{"run identity", func(r map[string]any) { r["runId"] = "foreign" }},
		{"missing success", func(r map[string]any) { delete(r, "ok") }},
		{"requested model", func(r map[string]any) { proObject(r, "requested")["model"] = "different" }},
		{"resume", func(r map[string]any) { proObject(r, "requested")["threadId"] = "old-thread" }},
		{"fabricated cost", func(r map[string]any) { r["costUSD"] = 0 }},
		{"missing outcome", func(r map[string]any) { r["outcome"] = nil }},
		{"failure without outcome or failure details", func(r map[string]any) { r["ok"], r["outcome"] = false, nil }},
		{"contradictory failure", func(r map[string]any) { r["failure"] = map[string]any{"category": "service"} }},
		{"unknown output block", func(r map[string]any) {
			proObject(r, "outcome")["output"] = []any{map[string]any{"type": "image", "text": ""}}
		}},
		{"not completed", func(r map[string]any) { proObject(r, "outcome")["stopReason"] = "aborted" }},
		{"no cleanup", func(r map[string]any) { delete(proObject(r, "outcome"), "cleanup") }},
		{"unconfirmed cleanup", func(r map[string]any) { proObject(r, "outcome", "cleanup")["confirmed"] = false }},
		{"missing physical execution", func(r map[string]any) { delete(proObject(r, "outcome"), "execution") }},
		{"foreign physical thread", func(r map[string]any) { proObject(r, "outcome", "execution")["threadId"] = "foreign" }},
		{"missing turn", func(r map[string]any) { delete(proObject(r, "outcome", "execution"), "turnId") }},
		{"unowned process", func(r map[string]any) { proObject(r, "outcome", "execution")["ownership"] = "owned-process" }},
		{"missing timeout fact", func(r map[string]any) { delete(proObject(r, "outcome", "execution"), "timedOut") }},
		{"timed out", func(r map[string]any) { proObject(r, "outcome", "execution")["timedOut"] = true }},
		{"cancelled", func(r map[string]any) { proObject(r, "outcome", "execution")["cancelled"] = true }},
		{"truncated", func(r map[string]any) { proObject(r, "outcome", "execution")["truncated"] = true }},
		{"spawn error", func(r map[string]any) { proObject(r, "outcome", "execution")["spawnError"] = "enoent" }},
		{"execution failure", func(r map[string]any) {
			proObject(r, "outcome", "execution")["failure"] = map[string]any{"category": "transport"}
		}},
		{"missing explicit exit", func(r map[string]any) { delete(proObject(r, "outcome", "execution"), "exitCode") }},
		{"missing explicit failure", func(r map[string]any) { delete(proObject(r, "outcome", "execution"), "failure") }},
		{"malformed execution", func(r map[string]any) { proObject(r, "outcome", "execution")["timedOut"] = "false" }},
		{"contradictory exit tuple", func(r map[string]any) { proObject(r, "outcome", "execution")["exitCode"] = 0 }},
		{"contradictory supervisor exit tuple", func(r map[string]any) { proObject(r, "outcome", "execution")["supervisorSignal"] = "SIGTERM" }},
		{"unexpected signal", func(r map[string]any) { proObject(r, "outcome", "execution")["signal"] = "SIGABRT" }},
		{"nonzero child exit", func(r map[string]any) {
			e := proObject(r, "outcome", "execution")
			e["exitCode"], e["signal"] = 1, nil
		}},
		{"unrequested child termination", func(r map[string]any) { proObject(r, "outcome", "execution")["terminationRequested"] = false }},
		{"missing configuration", func(r map[string]any) { delete(proObject(r, "outcome"), "configuration") }},
		{"resumed configuration", func(r map[string]any) { proObject(r, "outcome", "configuration")["source"] = "thread/resume" }},
		{"unmatched configuration", func(r map[string]any) { proObject(r, "outcome", "configuration")["matched"] = false }},
		{"missing policy evidence", func(r map[string]any) { proObject(r, "outcome", "configuration")["missing"] = []string{"sandbox"} }},
		{"wrong effective model", func(r map[string]any) { proObject(r, "outcome", "configuration", "effective")["model"] = "different" }},
		{"wrong cwd", func(r map[string]any) {
			proObject(r, "outcome", "configuration", "effective")["cwd"] = "/missing-workspace"
		}},
		{"relative cwd", func(r map[string]any) { proObject(r, "outcome", "configuration", "effective")["cwd"] = "." }},
		{"interactive approvals", func(r map[string]any) {
			proObject(r, "outcome", "configuration", "effective")["approvalPolicy"] = "on-request"
		}},
		{"missing effort evidence", func(r map[string]any) { delete(proObject(r, "outcome"), "turnConfiguration") }},
		{"fabricated effective effort", func(r map[string]any) { proObject(r, "outcome", "turnConfiguration")["effectiveEffort"] = "high" }},
		{"unaccepted turn", func(r map[string]any) { proObject(r, "outcome", "turnConfiguration")["accepted"] = false }},
		{"wrong effort", func(r map[string]any) { proObject(r, "outcome", "turnConfiguration")["requestedEffort"] = "low" }},
		{"malformed usage", func(r map[string]any) { proObject(r, "outcome")["usage"] = []string{"broken"} }},
		{"missing usage", func(r map[string]any) { delete(proObject(r, "outcome"), "usage") }},
		{"missing rate limits", func(r map[string]any) { delete(proObject(r, "outcome"), "rateLimits") }},
		{"empty output", func(r map[string]any) {
			proObject(r, "outcome")["output"] = []any{map[string]any{"type": "text", "text": " "}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := codexProFixture(request)
			tc.change(r)
			parsed := parseCodexProStream(dshJSONL(codexProFrame(request, r)), request)
			assert.False(t, parsed.Settled)
			assert.NotEmpty(t, parsed.Error)
			assert.NotEmpty(t, parsed.Receipt, "failed receipt must remain auditable")
		})
	}
	for _, change := range []func(map[string]any){
		func(e map[string]any) { e["signal"], e["exitCode"], e["terminationRequested"] = nil, 0, false },
		func(e map[string]any) { e["signal"], e["exitCode"], e["supervisorExitCode"] = nil, nil, nil },
		func(e map[string]any) {
			e["signal"], e["supervisorSignal"], e["supervisorExitCode"] = "SIGKILL", "SIGKILL", nil
		},
	} {
		r := codexProFixture(request)
		change(proObject(r, "outcome", "execution"))
		p := parseCodexProStream(dshJSONL(codexProFrame(request, r)), request)
		assert.True(t, p.Settled, "%+v", p)
	}
}

func TestCodexProStreamRejectsFramingAndWorkerErrors(t *testing.T) {
	request := codexProRequest(t)
	frame := codexProFrame(request, codexProFixture(request))
	progress := map[string]any{"type": "progress", "protocol": 1, "requestId": request.RequestID, "text": "working"}
	for name, stream := range map[string]string{
		"malformed": "{bad}\n", "empty": "\n", "missing terminal": dshJSONL(progress),
		"after terminal": dshJSONL(frame, progress), "duplicate terminal": dshJSONL(frame, frame),
	} {
		t.Run(name, func(t *testing.T) {
			p := parseCodexProStream(stream, request)
			assert.False(t, p.Settled)
			assert.NotEmpty(t, p.Error)
		})
	}
	for _, change := range []func(map[string]any){
		func(f map[string]any) { f["type"] = "ready" },
		func(f map[string]any) { f["protocol"] = 2 },
		func(f map[string]any) { f["requestId"] = "foreign" },
		func(f map[string]any) { f["type"] = "progress" },
		func(f map[string]any) { proObject(f, "adapter")["version"] = "0.7.0" },
		func(f map[string]any) { f["receipt"] = nil },
		func(f map[string]any) { delete(f, "error") },
		func(f map[string]any) {
			f["error"] = map[string]any{"code": "INVALID_REQUEST", "message": "worker admission failed"}
		},
	} {
		f := codexProFrame(request, codexProFixture(request))
		change(f)
		p := parseCodexProStream(dshJSONL(f), request)
		assert.False(t, p.Settled)
		assert.NotEmpty(t, p.Error)
	}
}

func TestCodexProCleanupEvidenceIsIndependentFromModelSuccess(t *testing.T) {
	request := codexProRequest(t)
	for _, tc := range []struct {
		name      string
		change    func(map[string]any)
		confirmed bool
	}{
		{"completed", func(map[string]any) {}, true},
		{"service error after cleanup", func(r map[string]any) {
			r["ok"] = false
			proObject(r, "outcome")["stopReason"] = "error"
			proObject(r, "outcome")["diagnostic"] = "503 service unavailable"
		}, true},
		{"failed initialization after cleanup", func(r map[string]any) {
			execution := proObject(r, "outcome", "execution")
			execution["threadId"], execution["turnId"] = nil, nil
			execution["pid"], execution["supervisorPid"] = nil, nil
			execution["signal"], execution["supervisorExitCode"] = nil, nil
			execution["terminationRequested"] = false
			r["ok"], r["outcome"] = false, nil
			r["failure"] = map[string]any{"category": "transport", "stage": "initialize", "diagnostic": "server closed",
				"cleanup": map[string]any{"confirmed": true}, "execution": execution, "usage": nil}
		}, true},
		{"execution omitted", func(r map[string]any) { delete(proObject(r, "outcome"), "execution") }, false},
		{"foreign execution", func(r map[string]any) { proObject(r, "outcome", "execution")["runId"] = "other-run" }, false},
		{"foreign thread", func(r map[string]any) { proObject(r, "outcome", "execution")["threadId"] = "other-thread" }, false},
		{"unowned execution", func(r map[string]any) { proObject(r, "outcome", "execution")["ownership"] = "unowned" }, false},
		{"cleanup omitted", func(r map[string]any) { delete(proObject(r, "outcome"), "cleanup") }, false},
		{"cleanup unconfirmed", func(r map[string]any) { proObject(r, "outcome", "cleanup")["confirmed"] = false }, false},
		{"foreign receipt", func(r map[string]any) { r["requestId"] = "another-run" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			receipt := codexProFixture(request)
			tc.change(receipt)
			parsed := parseCodexProStream(dshJSONL(codexProFrame(request, receipt)), request)
			assert.Equal(t, tc.confirmed, parsed.CleanupConfirmed)
		})
	}
	frame := codexProFrame(request, codexProFixture(request))
	for _, tail := range []string{"{bad}\n", dshJSONL(frame)} {
		parsed := parseCodexProStream(dshJSONL(frame)+tail, request)
		assert.False(t, parsed.CleanupConfirmed, "a damaged or duplicate terminal stream cannot attest quiescence")
	}
	proObject(frame, "adapter")["version"] = "0.7.0"
	assert.False(t, parseCodexProStream(dshJSONL(frame), request).CleanupConfirmed)
	failure := codexProFixture(request)
	failure["ok"] = false
	proObject(failure, "outcome")["stopReason"] = "error"
	proObject(failure, "outcome")["diagnostic"] = "503 service unavailable"
	frame = codexProFrame(request, failure)
	for _, tail := range []string{"{bad}\n", dshJSONL(frame)} {
		assert.False(t, parseCodexProStream(dshJSONL(frame)+tail, request).CleanupConfirmed,
			"failed results must also validate the entire stream before permitting retries")
	}
}

func TestCodexProFullPolicyAdmission(t *testing.T) {
	request := codexProRequest(t)
	inside := filepath.Join(request.Cwd, "inside")
	require.NoError(t, os.Mkdir(inside, 0o700))
	outside := t.TempDir()
	link := filepath.Join(request.Cwd, "escape")
	require.NoError(t, os.Symlink(outside, link))
	for _, tc := range []struct {
		name, permission string
		policy           any
		valid            bool
	}{
		{"readonly", "read-only", map[string]any{"type": "readOnly", "networkAccess": false}, true},
		{"full access", "danger-full-access", map[string]any{"type": "dangerFullAccess"}, true},
		{"full access constrained", "danger-full-access", map[string]any{"type": "readOnly", "networkAccess": false}, false},
		{"full access extra policy", "danger-full-access", map[string]any{"type": "dangerFullAccess", "networkAccess": false}, false},
		{"full access null policy", "danger-full-access", map[string]any{"type": "dangerFullAccess", "networkAccess": nil}, false},
		{"full access unknown field", "danger-full-access", map[string]any{"type": "dangerFullAccess", "future": true}, false},
		{"wrong readonly type", "read-only", map[string]any{"type": "workspaceWrite", "networkAccess": false}, false},
		{"readonly writable roots", "read-only", map[string]any{"type": "readOnly", "networkAccess": false, "writableRoots": []string{}}, false},
		{"network missing", "workspace-write", map[string]any{"type": "workspaceWrite"}, false},
		{"network enabled", "workspace-write", map[string]any{"type": "workspaceWrite", "networkAccess": true}, false},
		{"malformed policy", "workspace-write", "broken", false},
		{"incomplete workspace", "workspace-write", map[string]any{"type": "workspaceWrite", "networkAccess": false}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := request
			r.PermissionMode = tc.permission
			raw, err := json.Marshal(tc.policy)
			require.NoError(t, err)
			assert.Equal(t, tc.valid, validateCodexProPolicy(raw, r) == nil)
		})
	}
	for _, tc := range []struct {
		root  string
		valid bool
	}{{request.Cwd, true}, {inside, true}, {outside, false}, {".", false}, {link, false}, {filepath.Join(request.Cwd, "missing"), false}} {
		policy := map[string]any{"type": "workspaceWrite", "networkAccess": false, "writableRoots": []string{tc.root}, "excludeTmpdirEnvVar": false, "excludeSlashTmp": true}
		raw, err := json.Marshal(policy)
		require.NoError(t, err)
		assert.Equal(t, tc.valid, validateCodexProPolicy(raw, request) == nil, tc.root)
	}
	alias := filepath.Join(t.TempDir(), "workspace-alias")
	require.NoError(t, os.Symlink(request.Cwd, alias))
	assert.True(t, codexProSameDirectory(alias, request.Cwd))
}

func TestCodexProUsageAndFailureContracts(t *testing.T) {
	for _, raw := range []string{
		`{"status":"unknown","observations":0}`,
		`{"status":"partial","run":{"inputTokens":10},"reason":"interrupted"}`,
	} {
		u, err := parseCodexProUsage(json.RawMessage(raw))
		require.NoError(t, err)
		assert.Nil(t, u)
	}
	for _, raw := range []string{
		`[]`, `{}`, `{"status":"complete","run":{}}`,
		`{"status":"complete","run":{"inputTokens":-1}}`,
		`{"status":"complete","run":{"inputTokens":9007199254740992}}`,
		`{"status":"complete","run":{"inputTokens":10,"outputTokens":2,"reasoningOutputTokens":3,"cachedInputTokens":0,"cacheWriteInputTokens":0,"totalTokens":12}}`,
		`{"status":"complete","run":{"inputTokens":10,"outputTokens":2,"reasoningOutputTokens":0,"cachedInputTokens":8,"cacheWriteInputTokens":3,"totalTokens":12}}`,
	} {
		_, err := parseCodexProUsage(json.RawMessage(raw))
		assert.Error(t, err, raw)
	}
	request := codexProRequest(t)
	for _, category := range []string{"service", "transport", "limit", "access-policy", "INVALID_REQUEST", "invalid-request", "invalid-result", "RATE_LIMIT", "AUTH", "TIMEOUT", "runtime", "ADAPTER_UNAVAILABLE"} {
		r := codexProFixture(request)
		r["ok"], r["outcome"] = false, nil
		r["failure"] = map[string]any{"category": category, "stage": "initialize", "diagnostic": "service stopped", "execution": nil, "usage": nil, "cleanup": map[string]any{"confirmed": true}}
		p := parseCodexProStream(dshJSONL(codexProFrame(request, r)), request)
		assert.False(t, p.Settled)
		assert.NotEmpty(t, p.Error)
	}
	for _, tc := range []struct {
		category string
		status   int
		stop     string
		want     errs.ErrorCode
	}{{"service", 0, "error", errs.SERVER}, {"transport", 0, "error", errs.TRANSPORT}, {"limit", 0, "error", errs.QUOTA},
		{"access-policy", 0, "error", errs.INVALID_REQUEST}, {"limit", 0, "max-tokens", errs.CONTEXT_WINDOW_EXCEEDED},
		{"access-policy", 401, "error", errs.AUTH}, {"transport", 429, "error", errs.RATE_LIMIT}, {"invalid-result", 0, "error", errs.EMPTY_RESPONSE}} {
		e := &codexProExecution{Failure: &codexProFailureFacts{Category: tc.category, HTTPStatus: tc.status}}
		message := codexProFailureMessage("", "service stopped", tc.stop, e)
		assert.Equal(t, tc.want, errs.ClassifyError(errs.SpawnResult{ExitCode: 1, Stderr: message}), message)
	}
	r := codexProFixture(request)
	r["ok"] = false
	proObject(r, "outcome")["stopReason"] = "timeout"
	proObject(r, "outcome", "execution")["timedOut"] = true
	proObject(r, "outcome", "usage")["status"] = "partial"
	p := parseCodexProStream(dshJSONL(codexProFrame(request, r)), request)
	assert.True(t, p.TimedOut)
	assert.Nil(t, p.Usage)
	assert.NotEmpty(t, p.Receipt)
}

// A separate test process exercises actual pipes, environment, and signals.
func TestCodexProWorkerProcessFixture(t *testing.T) {
	scenario := os.Getenv("FLYWHEEL_TEST_CODEX_PRO_SCENARIO")
	if scenario == "" {
		return
	}
	if scenario == "closed-stdin" {
		_ = os.Stdin.Close()
		time.Sleep(5 * time.Second)
		os.Exit(0)
	}
	var request codexProStart
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if request.Type != "start" || request.Protocol != 1 || request.RequestID == "" {
		os.Exit(2)
	}
	if scenario == "record" {
		raw, _ := json.Marshal(map[string]any{"request": request, "env": os.Environ()})
		_ = os.WriteFile("worker-input.json", raw, 0o600)
		closed := make(chan struct{})
		go func() { var data [1]byte; _, _ = os.Stdin.Read(data[:]); close(closed) }()
		select {
		case <-closed:
			fmt.Fprintln(os.Stderr, "stdin lifetime connection closed before completion")
			os.Exit(3)
		case <-time.After(30 * time.Millisecond):
		}
	}
	receipt := codexProFixture(request)
	if scenario == "cancel-receipt" {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM)
		fmt.Print(dshJSONL(map[string]any{"type": "progress", "protocol": 1, "requestId": request.RequestID, "text": "ready"}))
		<-signals
		receipt["ok"] = false
		proObject(receipt, "outcome")["stopReason"] = "aborted"
		proObject(receipt, "outcome", "execution")["cancelled"] = true
		proObject(receipt, "outcome", "usage")["status"] = "partial"
	}
	if scenario == "hang" {
		time.Sleep(5 * time.Second)
	}
	if scenario == "empty" {
		os.Exit(0)
	}
	if scenario == "unknown-usage" {
		proObject(receipt, "outcome")["usage"] = map[string]any{"status": "unknown", "observations": 0}
	}
	if scenario == "cleanup-error" {
		proObject(receipt, "outcome", "cleanup")["confirmed"] = false
	}
	if scenario == "adapter-timeout" {
		receipt["ok"] = false
		proObject(receipt, "outcome", "execution")["timedOut"] = true
	}
	if scenario == "adapter-truncated" {
		receipt["ok"] = false
		proObject(receipt, "outcome", "execution")["truncated"] = true
	}
	fmt.Fprintln(os.Stderr, "worker diagnostic")
	fmt.Print(strings.TrimSpace(dshJSONL(codexProFrame(request, receipt))))
	if scenario == "exit-error" {
		os.Exit(7)
	}
	if scenario == "orphan-stdio" {
		child := exec.Command("/bin/sh", "-c", "sleep 30")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		_ = child.Start()
	}
	os.Exit(0)
}

func codexProRunArgs(t *testing.T, scenario string) CodexProArgs {
	t.Helper()
	return CodexProArgs{Command: kimiScript(t, "exec "+shQuote(os.Args[0])+" -test.run '^TestCodexProWorkerProcessFixture$'\n"),
		Cwd: t.TempDir(), Model: "gpt-test", Prompt: "task", PermissionMode: "workspace-write",
		APIKeys:  map[string]string{"FLYWHEEL_TEST_CODEX_PRO_SCENARIO": scenario},
		Watchdog: watchdog.Opts{IdleTimeoutMs: 3000, WallTimeoutMs: 3000, KillGraceMs: 30}}
}

func TestRunCodexProBindingEnvironmentAndLifetime(t *testing.T) {
	a := codexProRunArgs(t, "record")
	a.ReasoningEffort = "xhigh"
	a.PermissionMode = "read-only"
	a.Watchdog.WallTimeoutMs = 0
	var events []string
	a.OnEvent = func(line string) { events = append(events, line) }
	t.Setenv(CodexProWorkerEnv, a.Command)
	a.Command = ""
	t.Setenv(CodexProBinaryEnv, "/trusted/codex")
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "codex-home"))
	t.Setenv("UNRELATED_PRIVATE_KEY", "must-not-leak")
	r := RunCodexPro(a)
	require.True(t, r.Settled, "%+v", r)
	assert.Equal(t, "codex-pro", r.Harness)
	assert.Empty(t, r.Provider)
	assert.Equal(t, "gpt-test", r.Model)
	assert.Equal(t, "verified response", r.Text)
	assert.NotEmpty(t, r.CodexProReceipt)
	assert.Nil(t, r.CostUSD)
	assert.Len(t, events, 1, "the final unterminated stdout line is forwarded once")
	raw, err := os.ReadFile(filepath.Join(a.Cwd, "worker-input.json"))
	require.NoError(t, err)
	var input struct {
		Request codexProStart `json:"request"`
		Env     []string      `json:"env"`
	}
	require.NoError(t, json.Unmarshal(raw, &input))
	assert.Equal(t, "xhigh", input.Request.ReasoningEffort)
	assert.Equal(t, "read-only", input.Request.PermissionMode)
	assert.Equal(t, int64(watchdog.DefaultWallTimeoutMs), input.Request.TimeoutMS)
	assert.Contains(t, input.Env, CodexProBinaryEnv+"=/trusted/codex")
	assert.Contains(t, input.Env, "CODEX_HOME="+os.Getenv("CODEX_HOME"))
	assert.NotContains(t, strings.Join(input.Env, "\n"), "UNRELATED_PRIVATE_KEY")
	assert.Equal(t, "/trusted/codex", CodexProBinary())
	t.Setenv(CodexProWorkerEnv, "")
	t.Setenv(CodexProBinaryEnv, "")
	assert.Equal(t, "flywheel-codex-pro-worker", CodexProCommand())
	assert.Equal(t, "codex", CodexProBinary())
}

func TestRunCodexProFailuresAndFactory(t *testing.T) {
	for _, scenario := range []string{"unknown-usage", "empty", "exit-error", "cleanup-error", "adapter-timeout", "adapter-truncated"} {
		t.Run(scenario, func(t *testing.T) {
			r := RunCodexPro(codexProRunArgs(t, scenario))
			if scenario == "unknown-usage" {
				assert.True(t, r.Settled, "%+v", r)
				assert.Nil(t, r.Usage)
			} else {
				assert.False(t, r.Settled)
				require.NotNil(t, r.ErrorMessage)
			}
			if scenario == "empty" || scenario == "cleanup-error" {
				assert.NotEmpty(t, r.CleanupError, "missing cleanup proof must prevent another model call")
			} else {
				assert.Empty(t, r.CleanupError, "a known failure with cleanup proof remains eligible for normal error handling")
			}
		})
	}
	a := codexProRunArgs(t, "ok")
	t.Setenv(CodexProWorkerEnv, a.Command)
	for _, model := range []string{"", "gpt-override"} {
		r := InvokeHarness(CallArgs{Prompt: a.Prompt, Model: model, Cwd: a.Cwd, PermissionMode: "read-only", APIKeys: a.APIKeys,
			Binding: config.LoopBinding{Harness: "codex-pro", Model: "gpt-binding", Thinking: "high"}, Watchdog: a.Watchdog}, "ignored")
		require.True(t, r.Settled, "%+v", r)
		want := model
		if want == "" {
			want = "gpt-binding"
		}
		assert.Equal(t, want, r.Model)
	}
	for _, change := range []func(*CodexProArgs){
		func(a *CodexProArgs) { a.Model = "" },
		func(a *CodexProArgs) { a.Prompt = " " },
		func(a *CodexProArgs) { a.SessionID = "old" },
		func(a *CodexProArgs) { a.PermissionMode = "full-access" },
		func(a *CodexProArgs) { a.Cwd = filepath.Join(t.TempDir(), "missing") },
	} {
		bad := a
		change(&bad)
		r := RunCodexPro(bad)
		assert.False(t, r.Settled)
		require.NotNil(t, r.ErrorMessage)
		assert.Contains(t, *r.ErrorMessage, "invalid request")
	}
	a.Command = filepath.Join(t.TempDir(), "missing-worker")
	a.PermissionMode = ""
	r := RunCodexPro(a)
	assert.NotEmpty(t, r.SpawnError)
	assert.False(t, r.Settled)
	assert.Empty(t, r.CleanupError, "an unstarted worker cannot own an app-server")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.Watchdog.Context = ctx
	r = RunCodexPro(a)
	assert.True(t, r.Cancelled)
	assert.Empty(t, r.CleanupError, "pre-start cancellation does not create a cleanup obligation")
}

func TestRunCodexProPhysicalCancellationAndTimeout(t *testing.T) {
	t.Run("cancel receipt cannot turn cancel into success", func(t *testing.T) {
		a := codexProRunArgs(t, "cancel-receipt")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		a.Watchdog.Context = ctx
		a.OnEvent = func(line string) {
			if strings.Contains(line, "progress") {
				cancel()
			}
		}
		r := RunCodexPro(a)
		assert.True(t, r.Cancelled)
		assert.False(t, r.Settled)
		assert.NotEmpty(t, r.CodexProReceipt)
		assert.Nil(t, r.Usage, "interrupted usage is only a lower bound")
	})
	t.Run("idle timeout", func(t *testing.T) {
		a := codexProRunArgs(t, "hang")
		a.Watchdog.IdleTimeoutMs = 30
		r := RunCodexPro(a)
		assert.True(t, r.TimedOut)
		assert.True(t, r.IdleTimedOut)
		assert.False(t, r.Settled)
	})
	t.Run("stdin write failure", func(t *testing.T) {
		a := codexProRunArgs(t, "closed-stdin")
		a.Prompt = strings.Repeat("x", 1024*1024)
		r := RunCodexPro(a)
		assert.False(t, r.Settled)
		require.NotNil(t, r.ErrorMessage)
		assert.Contains(t, *r.ErrorMessage, "request write failed")
	})
	t.Run("inherited stdio cannot fake completion", func(t *testing.T) {
		a := codexProRunArgs(t, "orphan-stdio")
		a.Watchdog.WallTimeoutMs, a.Watchdog.IdleTimeoutMs = 20000, 20000
		r := RunCodexPro(a)
		assert.False(t, r.Settled)
		assert.NotEmpty(t, r.CleanupError)
		assert.NotEmpty(t, r.CodexProReceipt)
	})
}
