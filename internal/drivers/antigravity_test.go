package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/imageinput"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func agyRequest(t *testing.T) codexProStart {
	r := claudeCodeProRequest(t)
	r.Model = "gemini-4-hydrogen-high"
	r.Subscription = &config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"}
	return r
}
func agyReceipt(r codexProStart) map[string]any {
	mode, id := "new", "11111111-1111-4111-8111-111111111111"
	var requestedSession any
	if r.ThreadID != "" {
		mode, id = "resume", r.ThreadID
		requestedSession = r.ThreadID
	}
	execution, permission, tools := "accept-edits", "always-proceed", []string{"run_command"}
	policy := "native"
	if r.EvidenceOnly {
		execution, permission, policy = "plan", "request-review", "evidence-only"
	}
	receipt := map[string]any{
		"protocol": 1, "type": "result", "requestId": r.RequestID, "runId": "22222222-2222-4222-8222-222222222222", "ok": true,
		"requested": map[string]any{"cwd": r.Cwd, "model": r.Model, "effort": r.ReasoningEffort, "executionMode": execution, "permissionMode": permission, "toolPolicy": policy, "sandbox": false, "sessionMode": mode, "sessionId": requestedSession, "subscriptionPolicy": r.Subscription},
		"execution": map[string]any{"pid": 123, "processGroupId": 123, "ownership": "process-group", "dispatch": "dispatched", "turnDispatch": "accepted", "outcome": "completed", "terminalReceived": true, "finishedReason": nil, "exitCode": 0, "signal": nil, "cancelled": false, "timedOut": false, "truncated": false},
		"cleanup":   map[string]any{"confirmed": true, "method": "process-group"}, "providerWork": map[string]any{"state": "completed", "replaySafe": true},
		"session":       map[string]any{"mode": mode, "id": id, "resumed": mode == "resume", "resumable": true},
		"configuration": map[string]any{"source": "cli-init", "matched": true, "observed": map[string]any{"cwd": r.Cwd, "model": r.Model, "permissionMode": permission, "tools": tools}},
		"subscription":  map[string]any{"mode": "subscription-only", "status": "verified-local-controls", "creditsFallback": false, "configFingerprint": "fixture-settings-sha256"},
		"output":        []any{map[string]any{"type": "text", "text": "verified answer"}}, "usage": map[string]any{"status": "complete", "scope": "conversation", "inputTokens": 100, "outputTokens": 20, "thinkingTokens": 3, "cacheReadTokens": 0, "totalTokens": 123},
		"costUSD": nil, "costStatus": "unknown", "failure": nil,
	}
	native := config.AntigravityNative{Version: 1, Mode: "native"}
	if len(r.Native) > 0 {
		_ = json.Unmarshal(r.Native, &native)
		proObject(receipt, "requested")["native"] = r.Native
	}
	if r.InputImageManifest != nil {
		proObject(receipt, "requested")["inputImages"] = r.InputImageManifest
	}
	var command any
	children := []any{}
	if native.Mode == "swarm" {
		command = "/swarm"
	}
	if native.Mode != "native" {
		children = append(children, map[string]any{"id": "33333333-3333-4333-8333-333333333333", "kind": "worker", "role": "worker", "state": "unknown", "model": nil, "logUri": nil, "workspaceUris": []string{}})
	}
	receipt["native"] = map[string]any{
		"mode": native.Mode, "dispatch": map[string]any{"method": map[string]string{"native": "prompt", "subagents": "subagent-instruction", "swarm": "slash-command"}[native.Mode], "command": command},
		"observedAgents": len(children), "collaborationObserved": len(children) > 0, "metadataTruncated": false, "limit": map[string]any{"maxObservedAgents": native.MaxObservedAgents, "hard": false, "exceeded": false},
		"completion": map[string]any{"scope": "cli-invocation", "state": "completed", "source": "normal-result-and-cli-exit", "children": "unknown"},
		"children":   children, "aggregateUsageStatus": "unknown", "artifacts": []any{},
	}
	if r.EvidenceOnly {
		runID := receipt["runId"].(string)
		name := "dsh-evidence-" + runID
		proObject(receipt, "configuration", "observed")["agent"] = name
		proObject(receipt, "native")["managedAgent"] = map[string]any{
			"name": name, "path": filepath.Join(r.Cwd, ".agents", "agents", name+".md"),
			"digest": strings.Repeat("a", 64), "ownerRunId": runID, "cleanup": "confirmed",
		}
	}
	return receipt
}
func agyFrame(r codexProStart, m map[string]any) map[string]any {
	return map[string]any{"type": "result", "protocol": r.Protocol, "requestId": r.RequestID, "adapter": map[string]any{"package": "@ckanner/dsh-subagent-antigravity", "version": AntigravityPackageVersion, "protocol": 1}, "receipt": m, "error": nil}
}
func parseAgyFixture(r codexProStart, m map[string]any) claudeCodeProParsed {
	return parseOwnedCLIStream(dshJSONL(agyFrame(r, m)), r, "antigravity", "Antigravity", parseAntigravityReceipt)
}
func TestAntigravityReceiptPreservesConversationAccountingAndExplicitHistory(t *testing.T) {
	for _, mode := range []string{"fresh", "scoped", "resume", "evidence"} {
		t.Run(mode, func(t *testing.T) {
			r := agyRequest(t)
			if mode == "scoped" || mode == "resume" {
				r.Protocol, r.ThreadMode = 2, "persistent"
			}
			if mode == "resume" {
				r.ThreadID = "33333333-3333-4333-8333-333333333333"
			}
			if mode == "evidence" {
				r.EvidenceOnly, r.PermissionMode = true, "read-only"
			}
			p := parseAgyFixture(r, agyReceipt(r))
			require.True(t, p.Settled, "%+v", p)
			assert.Equal(t, "verified answer", p.Text)
			assert.Nil(t, p.Usage, "conversation snapshots must never inflate per-invocation totals")
			assert.Equal(t, "unknown", p.Session.UsageStatus)
			assert.Empty(t, p.Session.TurnID)
			assert.Equal(t, r.ThreadMode == "persistent", p.Session.ResumeCandidate)
			assert.Equal(t, mode == "resume", p.Session.ResumeAccepted)
			assert.True(t, p.Session.CleanupConfirmed)
		})
	}
	t.Run("acp-split-model-effort", func(t *testing.T) {
		r := agyRequest(t)
		r.Model, r.ReasoningEffort = "gemini-4-carbon-high", "high"
		m := agyReceipt(r)
		proObject(m, "configuration")["source"] = "acp-init"
		proObject(m, "configuration", "observed")["model"] = "gemini-4-carbon"
		proObject(m, "configuration", "observed")["effort"] = "high"
		p := parseAgyFixture(r, m)
		require.True(t, p.Settled, "%+v", p)
		proObject(m, "configuration", "observed")["effort"] = "low"
		require.False(t, parseAgyFixture(r, m).Settled)
	})
}
func TestAntigravityRejectsUnprovenAndContradictoryReceipts(t *testing.T) {
	r := agyRequest(t)
	for name, edit := range map[string]func(map[string]any){
		"identity":               func(m map[string]any) { m["requestId"] = "other" },
		"missing lifecycle":      func(m map[string]any) { delete(m, "execution") },
		"wrong session":          func(m map[string]any) { proObject(m, "session")["mode"] = "resume" },
		"unsafe process":         func(m map[string]any) { proObject(m, "execution")["processGroupId"] = 124 },
		"no spawn contradiction": func(m map[string]any) { proObject(m, "execution")["pid"] = nil },
		"usage malformed":        func(m map[string]any) { m["usage"] = "not counters" },
		"usage negative":         func(m map[string]any) { proObject(m, "usage")["inputTokens"] = -1 },
		"usage absent complete":  func(m map[string]any) { proObject(m, "usage")["totalTokens"] = nil },
		"usage wrong scope":      func(m map[string]any) { proObject(m, "usage")["scope"] = "session" },
		"cleanup":                func(m map[string]any) { proObject(m, "cleanup")["confirmed"] = false },
		"unknown model":          func(m map[string]any) { proObject(m, "configuration", "observed")["model"] = "auto" },
		"credits":                func(m map[string]any) { proObject(m, "subscription")["creditsFallback"] = true },
		"output type":            func(m map[string]any) { m["output"] = []any{map[string]any{"type": "image"}} },
		"empty output":           func(m map[string]any) { m["output"] = []any{} },
	} {
		t.Run(name, func(t *testing.T) {
			m := agyReceipt(r)
			edit(m)
			p := parseAgyFixture(r, m)
			require.False(t, p.Settled)
			require.NotEmpty(t, p.Error)
			assert.Nil(t, p.Usage)
			assert.False(t, p.Session.ResumeCandidate)
		})
	}
	r.EvidenceOnly = true
	r.PermissionMode = "read-only"
	m := agyReceipt(r)
	delete(proObject(m, "configuration", "observed"), "tools")
	require.False(t, parseAgyFixture(r, m).Settled)
	for _, category := range []string{"quota", "rate-limit", "auth"} {
		m := agyReceipt(r)
		m["ok"] = false
		m["failure"] = map[string]any{"category": category, "stage": "run", "detail": "native rejected"}
		proObject(m, "execution")["outcome"] = "failed"
		proObject(m, "providerWork")["state"] = "failed"
		p := parseAgyFixture(r, m)
		assert.Equal(t, "failed", p.Session.Outcome)
		codes := map[string]errs.ErrorCode{"quota": errs.QUOTA, "rate-limit": errs.RATE_LIMIT, "auth": errs.AUTH}
		assert.Equal(t, codes[category], p.ErrorCode)
	}
	m = agyReceipt(r)
	m["ok"] = false
	m["failure"] = map[string]any{"category": "invalid-request", "stage": "start", "detail": "no accepted prompt"}
	m["execution"] = map[string]any{"pid": nil, "processGroupId": nil, "ownership": "none", "dispatch": "not-dispatched", "turnDispatch": "not-dispatched", "outcome": "not-started", "terminalReceived": false, "finishedReason": nil, "exitCode": nil, "signal": nil, "cancelled": false, "timedOut": false, "truncated": false}
	m["cleanup"] = map[string]any{"confirmed": true, "method": "not-started"}
	m["providerWork"] = map[string]any{"state": "not-dispatched", "replaySafe": true}
	proObject(m, "native")["completion"] = map[string]any{"scope": "cli-invocation", "state": "not-started", "source": nil, "children": "unknown"}
	p := parseAgyFixture(r, m)
	assert.Equal(t, "not-dispatched", p.Session.Dispatch)
	assert.Equal(t, &Usage{}, p.Usage)
	// The CLI can start and reject init before user input; physical PID alone is not turn dispatch.
	proObject(m, "execution")["pid"] = 123
	proObject(m, "execution")["processGroupId"] = 123
	proObject(m, "execution")["ownership"] = "process-group"
	proObject(m, "cleanup")["method"] = "process-group"
	p = parseAgyFixture(r, m)
	assert.Equal(t, "not-dispatched", p.Session.Dispatch)
	assert.Equal(t, &Usage{}, p.Usage)
}
func TestAntigravityWorkerProcessFixture(t *testing.T) {
	if os.Getenv("FLYWHEEL_AGY_FIXTURE") != "1" {
		return
	}
	var r codexProStart
	if json.NewDecoder(os.Stdin).Decode(&r) != nil {
		os.Exit(2)
	}
	r.InputImageManifest = imageinput.Describe(r.Images)
	raw, _ := json.Marshal(map[string]any{"request": r, "env": os.Environ()})
	_ = os.WriteFile(filepath.Join(r.Cwd, "captured-agy.json"), raw, 0o600)
	_ = json.NewEncoder(os.Stdout).Encode(agyFrame(r, agyReceipt(r)))
	os.Exit(0)
}
func agyRunArgs(t *testing.T) AntigravityArgs {
	cwd := t.TempDir()
	require.NoError(t, os.Chmod(cwd, 0o700))
	return AntigravityArgs{Command: kimiScript(t, "export FLYWHEEL_AGY_FIXTURE=1\nexec "+shQuote(os.Args[0])+" -test.run '^TestAntigravityWorkerProcessFixture$'\n"), Cwd: cwd, Prompt: "task", Model: "gemini-4-hydrogen-high", ReasoningEffort: "high", PermissionMode: "danger-full-access", Subscription: &config.SubscriptionPolicy{Version: 1, Mode: "subscription-only"}, Watchdog: watchdog.Opts{WallTimeoutMs: 3000, IdleTimeoutMs: 3000}}
}
func TestRunAntigravityAdmissionAndEnvironment(t *testing.T) {
	for name, edit := range map[string]func(*AntigravityArgs){
		"identity": func(a *AntigravityArgs) { a.RequestID = " " }, "prompt": func(a *AntigravityArgs) { a.Prompt = "" },
		"permission": func(a *AntigravityArgs) { a.PermissionMode = "workspace-write" }, "evidence cannot use full access": func(a *AntigravityArgs) { a.EvidenceOnly = true },
		"implicit resume": func(a *AntigravityArgs) { a.SessionID = "history" }, "native override": func(a *AntigravityArgs) { a.Native = json.RawMessage(`{}`) },
		"no policy": func(a *AntigravityArgs) { a.Subscription = nil }, "key": func(a *AntigravityArgs) { a.APIKeys = map[string]string{"ANY_KEY": "secret"} },
		"effort": func(a *AntigravityArgs) { a.ReasoningEffort = "medium" }, "cwd": func(a *AntigravityArgs) { a.Cwd = filepath.Join(a.Cwd, "missing") },
		"deadline": func(a *AntigravityArgs) { a.Watchdog.WallTimeoutMs = 2147483648 }, "size": func(a *AntigravityArgs) { a.Prompt = strings.Repeat("&", machineRequestBytes/6) },
		"lifecycle": func(a *AntigravityArgs) { a.RecoveryDirectory = "relative" },
		"invalid image": func(a *AntigravityArgs) {
			a.Images = []imageinput.Image{{MIMEType: "image/svg+xml", Data: []byte("<svg/>")}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			a := agyRunArgs(t)
			edit(&a)
			r := RunAntigravity(a)
			require.NotNil(t, r.Session)
			assert.Equal(t, "not-dispatched", r.Session.Dispatch)
			assert.Empty(t, r.AntigravityReceipt)
			assert.NoFileExists(t, filepath.Join(a.Cwd, "captured-agy.json"))
		})
	}
	a := agyRunArgs(t)
	t.Setenv(AntigravityWorkerEnv, a.Command)
	t.Setenv(AntigravityBinaryEnv, "/trusted/agy")
	t.Setenv(AntigravityTransportEnv, "acp")
	a.Command = ""
	a.Images = []imageinput.Image{{MIMEType: "image/png", Data: []byte("png-bytes")}}
	a.Watchdog.WallTimeoutMs = 0
	a.APIKeys = map[string]string{"EMPTY": ""}
	r := RunAntigravity(a)
	require.True(t, r.Settled, "%+v", r)
	require.NotEmpty(t, r.AntigravityReceipt)
	assert.Nil(t, r.ClaudeCodeProReceipt)
	raw, err := os.ReadFile(filepath.Join(a.Cwd, "captured-agy.json"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), AntigravityBinaryEnv+"=/trusted/agy")
	assert.Contains(t, string(raw), AntigravityTransportEnv+"=acp")
	assert.Equal(t, "/trusted/agy", AntigravityBinary())
	a.Images = nil
	a.PersistentSession = true
	r = RunAntigravity(a)
	require.True(t, r.Settled)
	a.SessionID = r.SessionID
	r = RunAntigravity(a)
	assert.True(t, r.Session.ResumeAccepted)
	t.Run("input image manifest mismatch rejected", func(t *testing.T) {
		imgReq := agyRequest(t)
		imgReq.Images = []imageinput.Image{{MIMEType: "image/png", Data: []byte("img")}}
		imgReq.InputImageManifest = imageinput.Describe(imgReq.Images)
		m := agyReceipt(imgReq)
		require.True(t, parseAgyFixture(imgReq, m).Settled)
		delete(proObject(m, "requested"), "inputImages")
		bad := parseAgyFixture(imgReq, m)
		require.False(t, bad.Settled)
		require.Contains(t, bad.Error, "input image manifest mismatch")
	})
	t.Setenv(AntigravityWorkerEnv, "")
	t.Setenv(AntigravityBinaryEnv, "")
	t.Setenv(AntigravityTransportEnv, "")
	assert.Equal(t, "flywheel-antigravity-worker", AntigravityCommand())
	assert.Equal(t, "agy", AntigravityBinary())
}
func TestAntigravityNodeWorkerOfflineBridge(t *testing.T) {
	if os.Getenv("FLYWHEEL_TEST_NODE_BRIDGE") != "1" {
		t.Skip("requires built public adapter")
	}
	_, err := exec.LookPath("node")
	require.NoError(t, err)
	worker, err := filepath.Abs("../../plugins/dsh/dist/antigravity-worker.js")
	require.NoError(t, err)
	source, err := os.ReadFile("testdata/fake-agy.js")
	require.NoError(t, err)
	source = []byte(strings.Replace(string(source), "const args=", `const cfgPath=process.env.HOME+'/agy-fixture.json'; const fixtureConfig=JSON.parse(readFileSync(cfgPath,'utf8')); process.env.FIXTURE_SCENARIO=fixtureConfig.scenario; process.env.FIXTURE_CALLS=fixtureConfig.callsFile; process.env.FIXTURE_INPUT=fixtureConfig.inputFile;
if(fixtureConfig.finishedReason)process.env.FIXTURE_FINISHED_REASON=fixtureConfig.finishedReason;
const args=`, 1))
	fixture := filepath.Join(t.TempDir(), "fake-agy.mjs")
	require.NoError(t, os.WriteFile(fixture, source, 0o755))
	t.Setenv(AntigravityBinaryEnv, fixture)
	t.Setenv(AntigravityWorkerEnv, worker)
	t.Setenv(AntigravityTransportEnv, "headless")
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("HOME", home)
	for _, scenario := range []string{"success", "tools-inherited", "wrong-model", "quota", "duplicate", "waiting", "missing-agent", "wrong-agent", "illegal-tool"} {
		t.Run(scenario, func(t *testing.T) {
			a := agyRunArgs(t)
			cwd, err := filepath.EvalSymlinks(a.Cwd)
			require.NoError(t, err)
			a.Cwd = cwd
			a.Command = worker
			a.RequestID = "bridge-" + scenario
			a.RecoveryDirectory = filepath.Join(a.Cwd, "recovery")
			a.PersistentSession = true
			a.Watchdog = watchdog.Opts{WallTimeoutMs: 10000, IdleTimeoutMs: 8000}
			a.OnLifecycle = func(MachineLifecycle) error { return nil }
			a.EvidenceOnly = scenario == "missing-agent" || scenario == "wrong-agent" || scenario == "illegal-tool" || scenario == "success" || scenario == "tools-inherited"
			if a.EvidenceOnly {
				a.PermissionMode = "read-only"
			}
			settings, _ := json.Marshal(map[string]string{"scenario": scenario, "callsFile": filepath.Join(a.Cwd, "agy-calls.jsonl"), "inputFile": filepath.Join(a.Cwd, "agy-input.jsonl")})
			require.NoError(t, os.WriteFile(filepath.Join(home, "agy-fixture.json"), settings, 0o600))
			r := RunAntigravity(a)
			assert.NoDirExists(t, filepath.Join(a.Cwd, ".agents"), "evidence-only must never write .agents inside user repo")
			success := scenario == "success" || scenario == "tools-inherited"
			require.Equal(t, success, r.Settled, "%+v", r)
			require.NotEmpty(t, r.AntigravityReceipt)
			if success {
				assert.Nil(t, r.Usage)
				assert.Equal(t, "unknown", r.Session.UsageStatus)
				assert.True(t, r.Session.ResumeCandidate)
				before, err := os.ReadFile(filepath.Join(a.Cwd, "agy-calls.jsonl"))
				require.NoError(t, err)
				recovered, err := RecoverMachine(MachineRecoveryArgs{Harness: "antigravity", RequestID: a.RequestID, RunID: r.Session.RunID, RecoveryDirectory: a.RecoveryDirectory, Cwd: a.Cwd, Model: a.Model, ReasoningEffort: a.ReasoningEffort, PermissionMode: a.PermissionMode, PersistentSession: true, Subscription: a.Subscription, EvidenceOnly: a.EvidenceOnly})
				require.NoError(t, err)
				require.True(t, recovered.Settled, "%+v", recovered)
				after, err := os.ReadFile(filepath.Join(a.Cwd, "agy-calls.jsonl"))
				require.NoError(t, err)
				assert.Equal(t, before, after, "recovery must not launch agy")
				a.SessionID = r.SessionID
				a.RequestID = "bridge-resume"
				a.RecoveryDirectory = filepath.Join(a.Cwd, "resume-recovery")
				r = RunAntigravity(a)
				require.True(t, r.Settled, "%+v", r)
				assert.True(t, r.Session.ResumeAccepted)
			}
			if scenario == "missing-agent" || scenario == "wrong-agent" {
				assert.NoFileExists(t, filepath.Join(a.Cwd, "agy-input.jsonl"))
				assert.Equal(t, "not-dispatched", r.Session.Dispatch)
			}
			if scenario == "illegal-tool" {
				assert.Equal(t, "unknown", r.Session.Outcome)
				assert.Empty(t, r.Text)
				assert.False(t, r.Session.ResumeCandidate)
			}
		})
	}
	for _, tc := range []struct {
		name, scenario, model, mode, finishedReason string
		limit                                       int
		success                                     bool
	}{
		{"orchestrator-native", "success", "gemini-pro-orchestrator-neon", "native", "", 4, true},
		{"subagents", "children", "gemini-4-neon-high", "subagents", "", 4, true},
		{"swarm", "swarm-children", "gemini-4-neon-high", "swarm", "", 4, true},
		{"collaboration-missing", "success", "gemini-4-neon-high", "swarm", "", 4, true},
		{"observed-limit", "child-limit", "gemini-4-neon-high", "swarm", "", 2, false},
		{"native-cutoff", "success", "gemini-pro-orchestrator", "native", "HARD_TURN_LIMIT", 4, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := agyRunArgs(t)
			a.Cwd, err = filepath.EvalSymlinks(a.Cwd)
			require.NoError(t, err)
			a.Command, a.RequestID, a.Model = worker, "bridge-"+tc.name, tc.model
			if config.AntigravityOrchestrator(tc.model) {
				a.ReasoningEffort = ""
			}
			a.Native = json.RawMessage(fmt.Sprintf(`{"version":1,"mode":%q,"maxObservedAgents":%d}`, tc.mode, tc.limit))
			a.Managed = &NativeAdmission{Version: 1, RunID: "bridge-run", InvocationID: a.RequestID, Role: "execute", Scope: "M01-T01", Channel: "google", TaskDir: a.Cwd}
			a.RecoveryDirectory = filepath.Join(a.Cwd, "native-recovery")
			a.PersistentSession = true
			a.OnLifecycle = func(MachineLifecycle) error { return nil }
			a.Watchdog = watchdog.Opts{WallTimeoutMs: 10000, IdleTimeoutMs: 8000}
			settings, _ := json.Marshal(map[string]string{"scenario": tc.scenario, "finishedReason": tc.finishedReason, "callsFile": filepath.Join(a.Cwd, "agy-calls.jsonl"), "inputFile": filepath.Join(a.Cwd, "agy-input.jsonl")})
			require.NoError(t, os.WriteFile(filepath.Join(home, "agy-fixture.json"), settings, 0600))
			r := RunAntigravity(a)
			require.Equal(t, tc.success, r.Settled, "%+v", r)
			require.NotEmpty(t, r.AntigravityReceipt)
			require.NotEmpty(t, r.NativeReceipt, "%+v", r)
			assert.Nil(t, r.Usage, "child/conversation counters are not exclusive invocation usage")
			assert.Contains(t, string(r.NativeReceipt), `"children":"unknown"`)
			if !tc.success {
				assert.Equal(t, errs.INVALID_REQUEST, r.ErrorCode)
				expected := "unknown"
				if tc.name == "native-cutoff" {
					expected = "failed"
				}
				assert.Equal(t, expected, r.Session.Outcome, "error classification cannot invent a terminal provider outcome")
			}
			calls, err := os.ReadFile(filepath.Join(a.Cwd, "agy-calls.jsonl"))
			require.NoError(t, err)
			assert.Equal(t, !config.AntigravityOrchestrator(tc.model), strings.Contains(string(calls), `"--effort"`), string(calls))
			if tc.name == "swarm" {
				input, err := os.ReadFile(filepath.Join(a.Cwd, "agy-input.jsonl"))
				require.NoError(t, err)
				assert.Contains(t, string(input), "/swarm ")
				recovered, err := RecoverMachine(MachineRecoveryArgs{Harness: "antigravity", RequestID: a.RequestID, RunID: r.Session.RunID, RecoveryDirectory: a.RecoveryDirectory, Cwd: a.Cwd, Model: a.Model, ReasoningEffort: a.ReasoningEffort, PermissionMode: a.PermissionMode, PersistentSession: true, Subscription: a.Subscription, Native: a.Native})
				require.NoError(t, err)
				require.True(t, recovered.Settled, "%+v", recovered)
				require.JSONEq(t, string(r.NativeReceipt), string(recovered.NativeReceipt))
				after, err := os.ReadFile(filepath.Join(a.Cwd, "agy-calls.jsonl"))
				require.NoError(t, err)
				require.Equal(t, calls, after, "native recovery reads the original journal without redispatch")
			}
		})
	}
	t.Run("acp default with images", func(t *testing.T) {
		acpFixture, err := filepath.Abs("../../../dsh-subagent-antigravity/test/fixtures/acp/fake-acp-machine.js")
		require.NoError(t, err)
		a := agyRunArgs(t)
		cwd, err := filepath.EvalSymlinks(a.Cwd)
		require.NoError(t, err)
		a.Cwd = cwd
		a.Command = worker
		a.RequestID = "bridge-acp-images"
		a.Model = "gemini-4-neon-high"
		a.RecoveryDirectory = filepath.Join(a.Cwd, "acp-recovery")
		a.PersistentSession = true
		a.Watchdog = watchdog.Opts{WallTimeoutMs: 10000, IdleTimeoutMs: 8000}
		a.OnLifecycle = func(MachineLifecycle) error { return nil }
		a.Images = []imageinput.Image{{MIMEType: "image/png", Data: []byte("png-bytes")}}
		t.Setenv(AntigravityBinaryEnv, acpFixture)
		t.Setenv(AntigravityTransportEnv, "")
		t.Setenv("FIXTURE_SCENARIO", "success")
		r := RunAntigravity(a)
		require.True(t, r.Settled, "%+v", r)
		require.Contains(t, r.Text, "images=1")
		require.NotEmpty(t, r.AntigravityReceipt)
		require.NotContains(t, string(r.AntigravityReceipt), "cG5nLWJ5dGVz", "raw image base64 must be stripped into manifest")
		recovered, err := RecoverMachine(MachineRecoveryArgs{Harness: "antigravity", InputImages: imageinput.Describe(a.Images), RequestID: a.RequestID, RunID: r.Session.RunID, RecoveryDirectory: a.RecoveryDirectory, Cwd: a.Cwd, Model: a.Model, ReasoningEffort: a.ReasoningEffort, PermissionMode: a.PermissionMode, PersistentSession: true, Subscription: a.Subscription})
		require.NoError(t, err)
		require.True(t, recovered.Settled, "%+v", recovered)
	})
	t.Run("cancellation waits for physical cleanup", func(t *testing.T) {
		a := agyRunArgs(t)
		cwd, err := filepath.EvalSymlinks(a.Cwd)
		require.NoError(t, err)
		a.Cwd = cwd
		a.Command = worker
		settings, _ := json.Marshal(map[string]string{"scenario": "hang", "callsFile": filepath.Join(a.Cwd, "agy-calls.jsonl"), "inputFile": filepath.Join(a.Cwd, "agy-input.jsonl")})
		require.NoError(t, os.WriteFile(filepath.Join(home, "agy-fixture.json"), settings, 0o600))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		a.Watchdog.Context = ctx
		a.OnEvent = func(line string) {
			if strings.Contains(line, `"type":"activity"`) {
				cancel()
			}
		}
		r := RunAntigravity(a)
		require.False(t, r.Settled)
		require.True(t, r.Cancelled, fmt.Sprintf("%+v", r))
		require.True(t, r.Session.CleanupConfirmed)
		assert.False(t, r.Session.ResumeCandidate)
	})
}

func TestAntigravityFactoryRecoveryAndEvidenceAdmission(t *testing.T) {
	a := agyRunArgs(t)
	t.Setenv(AntigravityWorkerEnv, a.Command)
	for _, model := range []string{"", a.Model} {
		out := InvokeHarness(CallArgs{Subscription: a.Subscription, Prompt: a.Prompt, Cwd: a.Cwd, Model: model, Binding: config.LoopBinding{Harness: "antigravity", Model: a.Model, Thinking: a.ReasoningEffort}, PermissionMode: a.PermissionMode, Watchdog: a.Watchdog}, "")
		require.True(t, out.Settled, "%+v", out)
	}
	denied := InvokeHarness(CallArgs{EvidenceOnly: true, Binding: config.LoopBinding{Harness: "codex-pro"}, PermissionMode: "read-only"}, "")
	require.Contains(t, *denied.ErrorMessage, "evidence-only")
	cfg := config.Default()
	d := CreateDrivers(&cfg)
	d.Investigate = nil
	assert.Equal(t, d.Plan, d.ForRole("investigate"))
	d.Investigate = d.Review
	assert.Equal(t, d.Review, d.ForRole("investigate"))
	r := agyRequest(t)
	raw, err := json.Marshal(agyReceipt(r))
	require.NoError(t, err)
	recovered := recoveredMachineResult("antigravity", raw, r)
	require.True(t, recovered.Settled)
	assert.NotEmpty(t, recovered.AntigravityReceipt)
	assert.Nil(t, recovered.ClaudeCodeProReceipt)
	t.Setenv(AntigravityWorkerEnv, filepath.Join(t.TempDir(), "missing-recovery-worker"))
	_, err = RecoverMachine(MachineRecoveryArgs{Harness: "antigravity", RequestID: r.RequestID, RunID: "recorded-run", RecoveryDirectory: filepath.Join(r.Cwd, "recovery"), Cwd: r.Cwd, Model: r.Model, ReasoningEffort: r.ReasoningEffort, PermissionMode: r.PermissionMode, Subscription: r.Subscription})
	require.ErrorContains(t, err, "did not complete cleanly")
}
