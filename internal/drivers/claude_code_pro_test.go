package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func claudeCodeProRequest(t *testing.T) claudeCodeProStart {
	t.Helper()
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return claudeCodeProStart{Type: "start", Protocol: 1, RequestID: "request-claude-one", Cwd: cwd,
		Model: "sonnet", ReasoningEffort: "high", PermissionMode: "danger-full-access", Prompt: "task", TimeoutMS: 3000}
}

func claudeCodeProFixture(r claudeCodeProStart) map[string]any {
	mode, session := "ephemeral", "11111111-1111-4111-8111-111111111111"
	observedModel := r.Model
	if r.Model == "sonnet" || r.Model == "opus" || r.Model == "haiku" {
		observedModel = "claude-" + r.Model + "-test"
	}
	var requestedSession any
	if r.ThreadMode == "persistent" {
		mode = "persistent"
	}
	if r.ThreadID != "" {
		requestedSession, session = r.ThreadID, r.ThreadID
	}
	return map[string]any{
		"protocol": 1, "type": "result", "requestId": r.RequestID, "runId": "22222222-2222-4222-8222-222222222222", "ok": true,
		"requested": map[string]any{"cwd": r.Cwd, "model": r.Model, "effort": r.ReasoningEffort,
			"permissionMode": "bypassPermissions", "sessionMode": mode, "sessionId": requestedSession, "settingSources": []string{"user", "project", "local"}},
		"execution": map[string]any{"pid": 123, "processGroupId": 123, "ownership": "process-group", "dispatch": "dispatched", "turnDispatch": "accepted",
			"outcome": "completed", "terminalReceived": true, "exitCode": 0, "signal": nil,
			"cancelled": false, "timedOut": false, "truncated": false},
		"cleanup": map[string]any{"confirmed": true, "method": "process-group"},
		"session": map[string]any{"mode": mode, "id": session, "resumed": r.ThreadID != "", "resumable": mode == "persistent"},
		"configuration": map[string]any{"source": "cli-init", "matched": true,
			"observed": map[string]any{"cwd": r.Cwd, "model": observedModel, "permissionMode": "bypassPermissions", "cliVersion": "test"}},
		"output":          []any{map[string]any{"type": "text", "text": "verified response"}},
		"usage":           map[string]any{"status": "complete", "inputTokens": 10, "outputTokens": 5, "cacheReadTokens": 4, "cacheCreationTokens": 3, "totalTokens": 22},
		"reportedCostUSD": 0.42, "costUSD": nil, "costStatus": "unknown", "failure": nil,
	}
}

func claudeCodeProFrame(r claudeCodeProStart, receipt map[string]any) map[string]any {
	return map[string]any{"protocol": r.Protocol, "type": "result", "requestId": r.RequestID,
		"adapter": map[string]any{"package": "dsh-subagent-claude-code-pro", "version": ClaudeCodeProPackageVersion, "protocol": 1},
		"receipt": receipt, "error": nil}
}

func claudeCodeProUnknownUsage() map[string]any {
	return map[string]any{"status": "unknown", "inputTokens": nil, "outputTokens": nil, "cacheReadTokens": nil, "cacheCreationTokens": nil, "totalTokens": nil}
}

func TestClaudeCodeProReceiptSeparatesNativePermissionHistoryAndCost(t *testing.T) {
	for _, scope := range []string{"fresh", "start", "resume"} {
		t.Run(scope, func(t *testing.T) {
			r := claudeCodeProRequest(t)
			if scope != "fresh" {
				r.Protocol, r.ThreadMode = 2, "persistent"
			}
			if scope == "resume" {
				r.ThreadID = "33333333-3333-4333-8333-333333333333"
			}
			frame := claudeCodeProFrame(r, claudeCodeProFixture(r))
			progress := map[string]any{"protocol": r.Protocol, "type": "progress", "requestId": r.RequestID, "text": "working"}
			p := parseClaudeCodeProStream("\n"+dshJSONL(progress, frame), r)
			require.True(t, p.Settled, "%+v", p)
			assert.True(t, p.CleanupConfirmed)
			assert.Equal(t, "verified response", p.Text)
			require.NotNil(t, p.Usage)
			assert.Equal(t, Usage{Input: 17, Output: 5, CacheRead: 4, CacheWrite: 3, TotalTokens: 22}, *p.Usage)
			assert.Equal(t, "completed", p.Session.Outcome)
			assert.Equal(t, "dispatched", p.Session.Dispatch)
			assert.Equal(t, "complete", p.Session.UsageStatus)
			assert.Equal(t, scope != "fresh", p.Session.ResumeCandidate)
			assert.Equal(t, scope == "resume", p.Session.ResumeAccepted)
			assert.Equal(t, fmt.Sprintf("claude-code-pro/0.4.0/worker-v%d", r.Protocol), p.Session.Protocol)
			assert.Empty(t, p.Session.TurnID, "the CLI has no public turn identifier; never fabricate one")
			var raw map[string]any
			require.NoError(t, json.Unmarshal(p.Receipt, &raw))
			assert.Equal(t, 0.42, raw["reportedCostUSD"], "native estimate remains auditable")
			assert.Nil(t, raw["costUSD"], "the API-equivalent estimate is not subscription cost")
		})
	}
}

func TestClaudeCodeProReceiptRejectsUnprovenIdentityAndLifecycle(t *testing.T) {
	r := claudeCodeProRequest(t)
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"protocol", func(m map[string]any) { m["protocol"] = 2 }},
		{"type", func(m map[string]any) { m["type"] = "notice" }},
		{"request", func(m map[string]any) { m["requestId"] = "another" }},
		{"run", func(m map[string]any) { m["runId"] = " " }},
		{"null run", func(m map[string]any) { m["runId"] = "bad\x00run" }},
		{"missing success", func(m map[string]any) { delete(m, "ok") }},
		{"missing failure field", func(m map[string]any) { delete(m, "failure") }},
		{"success plus failure", func(m map[string]any) { m["failure"] = map[string]any{"category": "auth"} }},
		{"failure without details", func(m map[string]any) { m["ok"] = false }},
		{"model", func(m map[string]any) { proObject(m, "requested")["model"] = "another" }},
		{"cwd", func(m map[string]any) { proObject(m, "requested")["cwd"] = "/another" }},
		{"effort", func(m map[string]any) { proObject(m, "requested")["effort"] = "max" }},
		{"approval", func(m map[string]any) { proObject(m, "requested")["permissionMode"] = "acceptEdits" }},
		{"history mode", func(m map[string]any) { proObject(m, "requested")["sessionMode"] = "persistent" }},
		{"implicit recent history", func(m map[string]any) { proObject(m, "requested")["sessionId"] = "recent" }},
		{"omitted fresh identity", func(m map[string]any) { delete(proObject(m, "requested"), "sessionId") }},
		{"settings sources", func(m map[string]any) { proObject(m, "requested")["settingSources"] = []string{"user"} }},
		{"fabricated subscription bill", func(m map[string]any) { m["costUSD"] = 0 }},
		{"fabricated cost status", func(m map[string]any) { m["costStatus"] = "known" }},
		{"negative reported estimate", func(m map[string]any) { m["reportedCostUSD"] = -1 }},
		{"missing native estimate fact", func(m map[string]any) { delete(m, "reportedCostUSD") }},
		{"no execution", func(m map[string]any) { delete(m, "execution") }},
		{"execution wrong shape", func(m map[string]any) { m["execution"] = []string{"a"} }},
		{"physical fact missing", func(m map[string]any) { delete(proObject(m, "execution"), "exitCode") }},
		{"physical fact null", func(m map[string]any) { proObject(m, "execution")["cancelled"] = nil }},
		{"no cleanup", func(m map[string]any) { delete(m, "cleanup") }},
		{"null cleanup", func(m map[string]any) { proObject(m, "cleanup")["confirmed"] = nil }},
		{"no session", func(m map[string]any) { delete(m, "session") }},
		{"wrong session mode", func(m map[string]any) { proObject(m, "session")["mode"] = "persistent" }},
		{"no session id", func(m map[string]any) { delete(proObject(m, "session"), "id") }},
		{"nonstr session id", func(m map[string]any) { proObject(m, "session")["id"] = 5 }},
		{"empty session id", func(m map[string]any) { proObject(m, "session")["id"] = " " }},
		{"nul session id", func(m map[string]any) { proObject(m, "session")["id"] = "bad\x00id" }},
		{"unrequested resume", func(m map[string]any) { proObject(m, "session")["resumed"] = true }},
		{"both exit and signal", func(m map[string]any) { proObject(m, "execution")["signal"] = "SIGTERM" }},
		{"wrong process ownership", func(m map[string]any) { proObject(m, "execution")["ownership"] = "supervised-group" }},
		{"no pid", func(m map[string]any) { proObject(m, "execution")["pid"] = nil }},
		{"pid is not group leader", func(m map[string]any) { proObject(m, "execution")["processGroupId"] = 456 }},
		{"unknown dispatch", func(m map[string]any) { proObject(m, "execution")["dispatch"] = "unknown" }},
		{"unknown outcome", func(m map[string]any) { proObject(m, "execution")["outcome"] = "invented" }},
		{"wrong cleanup method", func(m map[string]any) { proObject(m, "cleanup")["method"] = "wait" }},
		{"no configuration", func(m map[string]any) { delete(m, "configuration") }},
		{"unmatched configuration", func(m map[string]any) { proObject(m, "configuration")["matched"] = false }},
		{"no observed configuration", func(m map[string]any) { proObject(m, "configuration")["observed"] = nil }},
		{"requested instead of observed", func(m map[string]any) { proObject(m, "configuration")["source"] = "request" }},
		{"wrong observed cwd", func(m map[string]any) { proObject(m, "configuration", "observed")["cwd"] = "/wrong" }},
		{"wrong observed permission", func(m map[string]any) { proObject(m, "configuration", "observed")["permissionMode"] = "plan" }},
		{"missing observed model", func(m map[string]any) { proObject(m, "configuration", "observed")["model"] = "" }},
		{"missing output", func(m map[string]any) { delete(m, "output") }},
		{"unknown output block", func(m map[string]any) { m["output"] = []any{map[string]any{"type": "image", "text": "unsupported"}} }},
		{"missing usage", func(m map[string]any) { delete(m, "usage") }},
		{"usage wrong type", func(m map[string]any) { m["usage"] = "not counters" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := claudeCodeProFixture(r)
			tc.edit(m)
			p := parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(r, m)), r)
			assert.False(t, p.Settled)
			assert.NotEmpty(t, p.Error)
			assert.Equal(t, "unknown", p.Session.Outcome)
			assert.Nil(t, p.Usage)
			assert.NotEmpty(t, p.Receipt)
		})
	}
	r.Protocol, r.ThreadMode, r.ThreadID = 2, "persistent", "33333333-3333-4333-8333-333333333333"
	m := claudeCodeProFixture(r)
	proObject(m, "session")["id"] = "another-session"
	p := parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(r, m)), r)
	assert.Equal(t, "unknown", p.Session.Outcome)
	assert.Nil(t, p.Usage)
}

func TestClaudeCodeProStreamRejectsCorruptionAndDistinguishesAdmission(t *testing.T) {
	r := claudeCodeProRequest(t)
	f := claudeCodeProFrame(r, claudeCodeProFixture(r))
	progress := map[string]any{"protocol": 1, "type": "progress", "requestId": r.RequestID, "text": "activity"}
	for name, stream := range map[string]string{
		"empty": "\n", "malformed": "{", "progress only": dshJSONL(progress),
		"duplicate": dshJSONL(f, f), "late progress": dshJSONL(f, progress), "bad tail": dshJSONL(f) + "{broken}\n",
	} {
		t.Run(name, func(t *testing.T) {
			p := parseClaudeCodeProStream(stream, r)
			assert.False(t, p.Settled)
			assert.Equal(t, "unknown", p.Session.Outcome)
			assert.Nil(t, p.Usage)
			assert.False(t, p.CleanupConfirmed)
		})
	}
	for _, edit := range []func(map[string]any){
		func(f map[string]any) { f["protocol"] = 2 },
		func(f map[string]any) { f["requestId"] = "another" },
		func(f map[string]any) { f["type"] = "unknown" },
		func(f map[string]any) { f["type"] = "progress" },
		func(f map[string]any) { delete(f, "error") },
		func(f map[string]any) { proObject(f, "adapter")["package"] = "dsh-subagent-codex-pro" },
		func(f map[string]any) { proObject(f, "adapter")["version"] = "0.3.1" },
		func(f map[string]any) { proObject(f, "adapter")["protocol"] = 2 },
		func(f map[string]any) { f["error"] = map[string]any{"code": "WORKER_ERROR", "message": "lost receipt"} },
	} {
		f := claudeCodeProFrame(r, claudeCodeProFixture(r))
		edit(f)
		p := parseClaudeCodeProStream(dshJSONL(f), r)
		assert.False(t, p.Settled)
		assert.Equal(t, "unknown", p.Session.Outcome)
		assert.Nil(t, p.Usage)
		assert.False(t, p.CleanupConfirmed)
	}
	admission := func() map[string]any {
		f := claudeCodeProFrame(r, nil)
		f["error"] = map[string]any{"code": "ADAPTER_UNAVAILABLE", "message": "Install the explicit optional peer"}
		f["admission"] = map[string]any{"dispatched": false, "cleanupConfirmed": true}
		return f
	}
	p := parseClaudeCodeProStream(dshJSONL(admission()), r)
	assert.Equal(t, "not-dispatched", p.Session.Dispatch)
	assert.Equal(t, "failed", p.Session.Outcome)
	assert.True(t, p.CleanupConfirmed)
	assert.Equal(t, errs.INVALID_REQUEST, p.ErrorCode)
	require.NotNil(t, p.Usage)
	assert.Zero(t, p.Usage.TotalTokens)
	for _, edit := range []func(map[string]any){
		func(f map[string]any) { proObject(f, "admission")["dispatched"] = true },
		func(f map[string]any) { proObject(f, "admission")["cleanupConfirmed"] = false },
		func(f map[string]any) { proObject(f, "admission")["dispatched"] = nil },
		func(f map[string]any) { proObject(f, "admission")["cleanupConfirmed"] = nil },
		func(f map[string]any) { f["receipt"] = claudeCodeProFixture(r) },
		func(f map[string]any) { f["error"] = nil },
		func(f map[string]any) { proObject(f, "error")["code"] = "WORKER_ERROR" },
	} {
		f := admission()
		edit(f)
		p := parseClaudeCodeProStream(dshJSONL(f), r)
		assert.Equal(t, "unknown", p.Session.Outcome)
		assert.False(t, p.CleanupConfirmed)
		assert.Nil(t, p.Usage)
	}
}

func TestClaudeCodeProUsageAndTerminalFailures(t *testing.T) {
	for _, status := range []string{"complete", "partial", "unknown"} {
		r := claudeCodeProRequest(t)
		m := claudeCodeProFixture(r)
		usage := proObject(m, "usage")
		usage["status"] = status
		if status != "complete" {
			usage["cacheReadTokens"], usage["totalTokens"] = nil, nil
		}
		p := parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(r, m)), r)
		assert.True(t, p.Settled)
		assert.Equal(t, status, p.Session.UsageStatus)
		assert.Equal(t, status == "complete", p.Usage != nil)
	}
	for _, raw := range []string{
		`{"status":"complete","inputTokens":10,"outputTokens":5,"cacheReadTokens":4,"cacheCreationTokens":3,"totalTokens":22}`,
		`{"status":"unknown","inputTokens":null,"outputTokens":null,"cacheReadTokens":null,"cacheCreationTokens":null,"totalTokens":null}`,
	} {
		_, _, err := parseClaudeCodeProUsage(json.RawMessage(raw))
		require.NoError(t, err)
	}
	for _, raw := range []string{
		`{broken}`, `null`, `[]`, `{"status":"complete"}`,
		`{"status":"invalid","inputTokens":0,"outputTokens":0,"cacheReadTokens":0,"cacheCreationTokens":0,"totalTokens":0}`,
		`{"status":"partial","inputTokens":-1,"outputTokens":null,"cacheReadTokens":null,"cacheCreationTokens":null,"totalTokens":null}`,
		`{"status":"complete","inputTokens":9007199254740992,"outputTokens":0,"cacheReadTokens":0,"cacheCreationTokens":0,"totalTokens":0}`,
		`{"status":"complete","inputTokens":0,"outputTokens":0,"cacheReadTokens":null,"cacheCreationTokens":0,"totalTokens":0}`,
		`{"status":"complete","inputTokens":10,"outputTokens":5,"cacheReadTokens":4,"cacheCreationTokens":3,"totalTokens":15}`,
	} {
		usage, _, err := parseClaudeCodeProUsage(json.RawMessage(raw))
		require.Error(t, err)
		assert.Nil(t, usage)
	}
	r := claudeCodeProRequest(t)
	m := claudeCodeProFixture(r)
	m["output"] = []any{map[string]any{"type": "text", "text": " "}}
	p := parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(r, m)), r)
	assert.False(t, p.Settled)
	assert.Equal(t, "completed", p.Session.Outcome, "model output failure is separate from execution certainty")
	assert.NotNil(t, p.Usage)
	assert.Contains(t, p.Error, "empty response")
	for category, code := range map[string]errs.ErrorCode{
		"auth": errs.AUTH, "timeout": errs.TIMEOUT, "invalid-result": errs.EMPTY_RESPONSE,
		"invalid-request": errs.INVALID_REQUEST, "unsupported-runtime": errs.INVALID_REQUEST, "configuration-mismatch": errs.INVALID_REQUEST,
		"limit": errs.TRANSPORT, "unknown": errs.TRANSPORT,
	} {
		m = claudeCodeProFixture(r)
		m["ok"], m["output"], m["failure"] = false, []any{}, map[string]any{"stage": "query-run", "category": category, "detail": "provider declined"}
		proObject(m, "execution")["outcome"], proObject(m, "execution")["exitCode"] = "failed", 1
		p = parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(r, m)), r)
		assert.False(t, p.Settled)
		assert.Equal(t, code, p.ErrorCode)
		assert.True(t, p.CleanupConfirmed)
	}
}

func TestClaudeCodeProNotStartedProofPreservesAResumeCheckpoint(t *testing.T) {
	r := claudeCodeProRequest(t)
	r.Protocol, r.ThreadMode, r.ThreadID = 2, "persistent", "33333333-3333-4333-8333-333333333333"
	notStarted := func() map[string]any {
		m := claudeCodeProFixture(r)
		m["ok"], m["output"], m["usage"] = false, []any{}, claudeCodeProUnknownUsage()
		m["failure"] = map[string]any{"stage": "start", "category": "spawn", "detail": "CLI unavailable"}
		m["execution"] = map[string]any{"pid": nil, "processGroupId": nil, "ownership": "none", "dispatch": "not-dispatched",
			"outcome": "not-started", "terminalReceived": false, "exitCode": nil, "signal": nil,
			"cancelled": false, "timedOut": false, "truncated": false}
		m["cleanup"] = map[string]any{"confirmed": true, "method": "not-started"}
		proObject(m, "session")["id"], proObject(m, "session")["resumed"], proObject(m, "session")["resumable"] = nil, false, false
		m["configuration"] = map[string]any{"source": nil, "observed": nil, "matched": false}
		return m
	}
	p := parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(r, notStarted())), r)
	assert.Equal(t, "not-dispatched", p.Session.Dispatch)
	assert.Equal(t, "failed", p.Session.Outcome)
	assert.True(t, p.CleanupConfirmed)
	assert.Equal(t, r.ThreadID, p.Session.SessionID)
	assert.False(t, p.Session.ResumeAccepted)
	require.NotNil(t, p.Usage)
	assert.Zero(t, p.Usage.TotalTokens)
	m := notStarted()
	proObject(m, "execution")["pid"] = 123
	p = parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(r, m)), r)
	assert.Equal(t, "unknown", p.Session.Outcome)
	assert.False(t, p.CleanupConfirmed)
	assert.Nil(t, p.Usage)
}

func TestClaudeCodeProWorkerProcessFixture(t *testing.T) {
	if os.Getenv("FLYWHEEL_CLAUDE_FIXTURE") != "1" {
		return
	}
	var r claudeCodeProStart
	if json.NewDecoder(os.Stdin).Decode(&r) != nil {
		os.Exit(4)
	}
	encoded, _ := json.Marshal(map[string]any{"request": r, "env": os.Environ(), "args": os.Args})
	_ = os.WriteFile(filepath.Join(r.Cwd, "captured-claude.json"), encoded, 0o600)
	if r.Prompt == "no-receipt" {
		os.Exit(0)
	}
	if r.Prompt == "wait" {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"protocol": r.Protocol, "type": "progress", "requestId": r.RequestID, "text": "ready"})
		<-signals
		m := claudeCodeProFixture(r)
		m["ok"], m["output"], m["usage"] = false, []any{}, claudeCodeProUnknownUsage()
		m["failure"] = map[string]any{"stage": "run", "category": "cancelled", "detail": "cancelled"}
		proObject(m, "execution")["outcome"], proObject(m, "execution")["terminalReceived"], proObject(m, "execution")["cancelled"] = "unknown", false, true
		proObject(m, "session")["resumable"] = false
		_ = json.NewEncoder(os.Stdout).Encode(claudeCodeProFrame(r, m))
		os.Exit(1)
	}
	m := claudeCodeProFixture(r)
	f, exit := claudeCodeProFrame(r, m), 0
	switch r.Prompt {
	case "error":
		m["ok"], m["output"], m["failure"] = false, []any{}, map[string]any{"stage": "query-run", "category": "auth", "detail": "account login required"}
		proObject(m, "execution")["outcome"], proObject(m, "execution")["exitCode"] = "failed", 1
		proObject(m, "session")["resumable"] = false
		exit = 1
	case "admission":
		f["receipt"], f["error"], f["admission"] = nil,
			map[string]any{"code": "ADAPTER_UNAVAILABLE", "message": "optional peer is not installed"}, map[string]any{"dispatched": false, "cleanupConfirmed": true}
		exit = 1
	case "cleanup":
		m["ok"], m["output"], m["failure"] = false, []any{}, map[string]any{"stage": "cleanup", "category": "cleanup-unconfirmed", "detail": "owned group remains"}
		proObject(m, "cleanup")["confirmed"] = false
		proObject(m, "session")["resumable"] = false
		exit = 1
	case "native-truncated":
		m["ok"], m["output"], m["failure"] = false, []any{}, map[string]any{"stage": "run", "category": "output-limit", "detail": "native output exceeded boundary"}
		proObject(m, "execution")["truncated"] = true
		proObject(m, "session")["resumable"] = false
		exit = 1
	case "native-timeout":
		m["ok"], m["output"], m["failure"] = false, []any{}, map[string]any{"stage": "run", "category": "timeout", "detail": "native turn deadline expired"}
		proObject(m, "execution")["timedOut"] = true
		proObject(m, "session")["resumable"] = false
		exit = 1
	case "bad-exit":
		exit = 9
	case "no-success-exit":
		exit = 1
	}
	_ = json.NewEncoder(os.Stdout).Encode(f)
	if r.Prompt == "garbage-tail" {
		_, _ = fmt.Fprintln(os.Stdout, "{bad json}")
	}
	if r.Prompt == "outer-truncated" {
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat(" ", int(watchdog.MaxBuffer+1)))
	}
	if r.Prompt == "signal" {
		_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		select {}
	}
	os.Exit(exit)
}

func claudeCodeProRunArgs(t *testing.T, scenario string) ClaudeCodeProArgs {
	t.Helper()
	return ClaudeCodeProArgs{Command: kimiScript(t, "export FLYWHEEL_CLAUDE_FIXTURE=1\nexec "+shQuote(os.Args[0])+" -test.run '^TestClaudeCodeProWorkerProcessFixture$'\n"),
		Cwd: t.TempDir(), Prompt: scenario, Model: "sonnet", PermissionMode: "danger-full-access",
		Watchdog: watchdog.Opts{WallTimeoutMs: 3000, IdleTimeoutMs: 3000, KillGraceMs: 20}}
}

func TestRunClaudeCodeProBindingsEnvironmentAndFactory(t *testing.T) {
	a := claudeCodeProRunArgs(t, "success")
	t.Setenv(ClaudeCodeProWorkerEnv, a.Command)
	a.Command = ""
	t.Setenv(ClaudeCodeProBinaryEnv, "/trusted/claude")
	t.Setenv("CLAUDE_CONFIG_DIR", "/trusted/claude-settings")
	t.Setenv("ANTHROPIC_API_KEY", "ambient-should-not-leak")
	t.Setenv("UNRELATED_SECRET", "other-should-not-leak")
	a.APIKeys = map[string]string{"ANTHROPIC_API_KEY": "explicit-test-key"}
	var events []string
	a.OnEvent = func(line string) { events = append(events, line) }
	first := RunClaudeCodePro(a)
	require.True(t, first.Settled, "%+v", first)
	assert.Equal(t, "claude-code-pro", first.Harness)
	assert.Empty(t, first.Provider, "the native harness is not a DSH LLM provider")
	assert.Nil(t, first.CostUSD)
	assert.NotEmpty(t, first.ClaudeCodeProReceipt)
	assert.NotEmpty(t, events)
	var capture struct {
		Request claudeCodeProStart `json:"request"`
		Env     []string           `json:"env"`
	}
	raw, err := os.ReadFile(filepath.Join(a.Cwd, "captured-claude.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &capture))
	assert.Equal(t, "high", capture.Request.ReasoningEffort)
	assert.Equal(t, "danger-full-access", capture.Request.PermissionMode)
	assert.Contains(t, capture.Env, "ANTHROPIC_API_KEY=explicit-test-key")
	assert.Contains(t, capture.Env, "CLAUDE_CONFIG_DIR=/trusted/claude-settings")
	assert.Contains(t, capture.Env, ClaudeCodeProBinaryEnv+"=/trusted/claude")
	assert.NotContains(t, strings.Join(capture.Env, "\n"), "ambient-should-not-leak")
	assert.NotContains(t, strings.Join(capture.Env, "\n"), "other-should-not-leak")
	a.PersistentSession, a.RequestID = true, "engine-first"
	first = RunClaudeCodePro(a)
	require.True(t, first.Settled, "%+v", first)
	assert.True(t, first.Session.ResumeCandidate)
	a.SessionID, a.RequestID = first.SessionID, "engine-repair"
	second := RunClaudeCodePro(a)
	require.True(t, second.Settled, "%+v", second)
	assert.Equal(t, first.SessionID, second.SessionID)
	assert.True(t, second.Session.ResumeAccepted)
	for _, model := range []string{"", "opus"} {
		r := InvokeHarness(CallArgs{Prompt: "success", Cwd: a.Cwd, Model: model,
			Binding: config.LoopBinding{Harness: "claude-code-pro", Model: "sonnet", Thinking: "medium"}, PermissionMode: "danger-full-access", Watchdog: a.Watchdog}, "ignored")
		require.True(t, r.Settled, "%+v", r)
		if model == "" {
			assert.Equal(t, "sonnet", r.Model)
		} else {
			assert.Equal(t, model, r.Model)
		}
	}
	assert.Equal(t, "/trusted/claude", ClaudeCodeProBinary())
	t.Setenv(ClaudeCodeProWorkerEnv, "")
	t.Setenv(ClaudeCodeProBinaryEnv, "")
	assert.Equal(t, "flywheel-claude-code-pro-worker", ClaudeCodeProCommand())
	assert.Equal(t, "claude", ClaudeCodeProBinary())
}

func TestRunClaudeCodeProRejectsBeforeDispatch(t *testing.T) {
	for _, edit := range []func(*ClaudeCodeProArgs){
		func(a *ClaudeCodeProArgs) { a.Model = " " },
		func(a *ClaudeCodeProArgs) { a.Prompt = "" },
		func(a *ClaudeCodeProArgs) { a.PermissionMode = "workspace-write" },
		func(a *ClaudeCodeProArgs) { a.SessionID = "unrequested" },
		func(a *ClaudeCodeProArgs) { a.SessionID, a.PersistentSession = "\x00", true },
		func(a *ClaudeCodeProArgs) { a.SessionID, a.PersistentSession = " ", true },
		func(a *ClaudeCodeProArgs) { a.RequestID = " " },
		func(a *ClaudeCodeProArgs) { a.RequestID = strings.Repeat("a", 257) },
		func(a *ClaudeCodeProArgs) { a.RequestID = "\x00" },
		func(a *ClaudeCodeProArgs) { a.ReasoningEffort = "ultra" },
		func(a *ClaudeCodeProArgs) { a.Watchdog.WallTimeoutMs = 2147483648 },
		func(a *ClaudeCodeProArgs) { a.Cwd = filepath.Join(a.Cwd, "missing") },
		func(a *ClaudeCodeProArgs) { a.APIKeys = map[string]string{"WRONG_API_KEY": "secret"} },
		func(a *ClaudeCodeProArgs) { a.Prompt = strings.Repeat("x", claudeCodeProRequestBytes) },
		func(a *ClaudeCodeProArgs) { a.Prompt = strings.Repeat("&", claudeCodeProRequestBytes/6) },
	} {
		a := claudeCodeProRunArgs(t, "success")
		edit(&a)
		r := RunClaudeCodePro(a)
		require.NotNil(t, r.Session)
		assert.False(t, r.Settled)
		assert.Equal(t, "not-dispatched", r.Session.Dispatch)
		assert.True(t, r.Session.CleanupConfirmed)
		assert.Empty(t, r.ClaudeCodeProReceipt)
		assert.NotNil(t, r.Usage)
		assert.NotNil(t, r.CostUSD)
		assert.Contains(t, *r.ErrorMessage, "invalid request")
	}
	a := claudeCodeProRunArgs(t, "success")
	a.Watchdog.WallTimeoutMs = 0
	a.APIKeys = map[string]string{"IGNORED_EMPTY": ""}
	r := RunClaudeCodePro(a)
	require.True(t, r.Settled, "%+v", r)
}

func TestClaudeCodeProRequestByteLimitIncludesEscapingAndNewline(t *testing.T) {
	a := claudeCodeProRunArgs(t, "")
	a.RequestID = "owned-byte-boundary"
	cwd, err := filepath.EvalSymlinks(a.Cwd)
	require.NoError(t, err)
	request := claudeCodeProStart{Type: "start", Protocol: 1, RequestID: a.RequestID, Cwd: cwd,
		Model: a.Model, ReasoningEffort: "high", PermissionMode: a.PermissionMode, TimeoutMS: a.Watchdog.WallTimeoutMs}
	empty, err := json.Marshal(request)
	require.NoError(t, err)
	available := claudeCodeProRequestBytes - len(empty) - 1
	a.Prompt = strings.Repeat("&", available/6) + strings.Repeat("a", available%6)
	request.Prompt = a.Prompt
	exact, err := json.Marshal(request)
	require.NoError(t, err)
	require.Equal(t, claudeCodeProRequestBytes, len(exact)+1)
	r := RunClaudeCodePro(a)
	require.True(t, r.Settled, "%+v", r)

	// One more encoded byte must be rejected before a worker can parse an
	// uncorrelated INPUT_LIMIT error and leave the saved invocation unknown.
	a.Prompt += "a"
	a.Command = "/must-not-start-for-oversized-request"
	r = RunClaudeCodePro(a)
	assert.False(t, r.Settled)
	assert.Equal(t, "not-dispatched", r.Session.Dispatch)
	assert.Equal(t, "failed", r.Session.Outcome)
	assert.True(t, r.Session.CleanupConfirmed)
	assert.Equal(t, &Usage{}, r.Usage)
	require.NotNil(t, r.CostUSD)
	assert.Zero(t, *r.CostUSD)
	assert.Empty(t, r.SpawnError)
	assert.Empty(t, r.ClaudeCodeProReceipt)
	assert.Contains(t, *r.ErrorMessage, "1 MiB JSONL limit")
}

func TestRunClaudeCodeProPhysicalFailuresAndNonDispatch(t *testing.T) {
	for _, scenario := range []string{"no-receipt", "garbage-tail", "bad-exit", "no-success-exit", "signal", "cleanup", "outer-truncated", "native-truncated", "native-timeout", "error", "admission"} {
		t.Run(scenario, func(t *testing.T) {
			r := RunClaudeCodePro(claudeCodeProRunArgs(t, scenario))
			assert.False(t, r.Settled)
			require.NotNil(t, r.ErrorMessage)
			if scenario == "error" {
				assert.Equal(t, errs.AUTH, r.ErrorCode)
				assert.Equal(t, "failed", r.Session.Outcome)
			} else if scenario == "admission" {
				assert.Equal(t, errs.INVALID_REQUEST, r.ErrorCode)
				assert.Equal(t, "not-dispatched", r.Session.Dispatch)
				assert.Zero(t, *r.CostUSD)
			} else if scenario == "cleanup" {
				assert.NotEmpty(t, r.CleanupError)
				assert.False(t, r.Session.CleanupConfirmed)
				assert.NotNil(t, r.Usage, "a trustworthy terminal can still carry attributable consumption")
			} else {
				assert.Equal(t, "unknown", r.Session.Outcome)
			}
			if scenario == "native-timeout" {
				assert.True(t, r.TimedOut)
				assert.False(t, r.Cancelled)
				assert.Contains(t, *r.ErrorMessage, "timed out")
			}
		})
	}
	a := claudeCodeProRunArgs(t, "success")
	a.Command = filepath.Join(t.TempDir(), "missing-worker")
	r := RunClaudeCodePro(a)
	assert.Equal(t, "not-dispatched", r.Session.Dispatch)
	assert.True(t, r.Session.CleanupConfirmed)
	require.NotNil(t, r.Usage)
	assert.Zero(t, r.Usage.TotalTokens)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.Watchdog.Context = ctx
	r = RunClaudeCodePro(a)
	assert.True(t, r.Cancelled)
	assert.Equal(t, "not-dispatched", r.Session.Dispatch)
	assert.True(t, r.Session.CleanupConfirmed)
	assert.Empty(t, r.ClaudeCodeProReceipt)
	a = claudeCodeProRunArgs(t, "success")
	a.Command = kimiScript(t, "exec 0<&-\nsleep 5\n")
	a.Prompt = strings.Repeat("large request", 1<<13)
	r = RunClaudeCodePro(a)
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "request write failed")
	assert.Equal(t, "unknown", r.Session.Outcome)
}

func TestRunClaudeCodeProCancellationAndTimeout(t *testing.T) {
	for _, kind := range []string{"cancel", "timeout"} {
		t.Run(kind, func(t *testing.T) {
			a := claudeCodeProRunArgs(t, "wait")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a.Watchdog.Context = ctx
			if kind == "timeout" {
				a.Watchdog.WallTimeoutMs = 500
			} else {
				a.OnEvent = func(line string) {
					if strings.Contains(line, `"type":"progress"`) {
						cancel()
					}
				}
			}
			r := RunClaudeCodePro(a)
			assert.False(t, r.Settled)
			assert.Equal(t, "unknown", r.Session.Outcome)
			assert.True(t, r.Session.CleanupConfirmed)
			assert.NotEmpty(t, r.ClaudeCodeProReceipt)
			if kind == "timeout" {
				assert.True(t, r.TimedOut)
			} else {
				assert.True(t, r.Cancelled)
			}
		})
	}
}

func TestHarnessAdmissionKeepsKnownZeroWithoutStartingAnAdapter(t *testing.T) {
	for _, harness := range []string{"dsh", "kimi", "codex-pro", "claude-code-pro"} {
		for _, persistent := range []bool{false, true} {
			r := InvokeHarness(CallArgs{RequestID: "engine-admission", SessionID: "saved-history", PersistentSession: persistent,
				Binding: config.LoopBinding{Harness: harness}, PermissionMode: "unsupported"}, "")
			require.NotNil(t, r.Session)
			assert.Equal(t, "not-dispatched", r.Session.Dispatch)
			assert.Equal(t, "failed", r.Session.Outcome)
			assert.True(t, r.Session.CleanupConfirmed)
			assert.Equal(t, "saved-history", r.Session.SessionID)
			assert.NotEmpty(t, r.Session.Protocol)
			require.NotNil(t, r.Usage)
			assert.Zero(t, r.Usage.TotalTokens)
			assert.Zero(t, *r.CostUSD)
		}
	}
	for _, permission := range []string{"", "read-only", "workspace-write"} {
		r := InvokeHarness(CallArgs{RequestID: "engine-admission", Binding: config.LoopBinding{Harness: "claude-code-pro"}, PermissionMode: permission}, "")
		assert.Equal(t, "not-dispatched", r.Session.Dispatch)
		assert.Contains(t, *r.ErrorMessage, "full-access")
	}
	r := InvokeHarness(CallArgs{RequestID: "untracked", Binding: config.LoopBinding{Harness: "typo"}}, "")
	assert.Nil(t, r.Session)
}

func TestTrackedAdaptersPreStartCancellationIsNotAnUnknownInvocation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, persistent := range []bool{false, true} {
		r := RunDSH(DSHArgs{Cwd: t.TempDir(), Command: filepath.Join(t.TempDir(), "never-started"),
			Provider: "p", Model: "m", Prompt: "task", RequestID: "pre-cancel", PersistentSession: persistent,
			Watchdog: watchdog.Opts{Context: ctx}})
		assert.True(t, r.Cancelled)
		require.NotNil(t, r.Session)
		assert.Equal(t, "not-dispatched", r.Session.Dispatch)
		assert.True(t, r.Session.CleanupConfirmed)
		assert.NotNil(t, r.Usage)
		assert.NotNil(t, r.CostUSD)
	}
	r := RunKimi(KimiArgs{Cwd: t.TempDir(), Command: filepath.Join(t.TempDir(), "never-started"), Prompt: "task", RequestID: "pre-cancel",
		Watchdog: watchdog.Opts{Context: ctx}})
	assert.True(t, r.Cancelled)
	assert.Equal(t, "not-dispatched", r.Session.Dispatch)
	assert.True(t, r.Session.CleanupConfirmed)
	assert.NotNil(t, r.Usage)
	assert.NotNil(t, r.CostUSD)
}
