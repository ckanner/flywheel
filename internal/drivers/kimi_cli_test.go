package drivers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const kimiComplete = `{"role":"meta","type":"session.resume_hint","session_id":"session-one"}`

func kimiScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kimi code")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755))
	return path
}

func TestKimiPrintCommandContract(t *testing.T) {
	// Exercise the same logical/physical path distinction as macOS /var versus
	// /private/var on every platform, without changing the requested directory.
	cwd := filepath.Join(t.TempDir(), "workspace alias")
	require.NoError(t, os.Symlink(t.TempDir(), cwd))
	physicalCwd, err := filepath.EvalSymlinks(cwd)
	require.NoError(t, err)
	command := kimiScript(t, `printf '%s\0' "$@" > args.bin
pwd > cwd.txt
env > env.txt
printf '%s\n' 'diagnostic only' >&2
printf '%s\n' '{"role":"meta","type":"system.version","version":"2.4.1"}'
printf '%s\n' '{"role":"assistant","content":"reviewable plan","usage":{"input":9,"output":2,"totalTokens":11}}'
printf '%s' '`+kimiComplete+`'
`)
	t.Setenv("UNRELATED_SECRET", "not-for-the-cli")
	t.Setenv("KIMI_MODEL_THINKING_EFFORT", "max")
	var events []string
	r := RunKimi(KimiArgs{
		Command: command, Cwd: cwd, Prompt: "plan\nwith literal $(text)",
		Model: "kimi-code/k3.1-pro", Thinking: "low", SessionID: "session-one",
		APIKeys: map[string]string{"KIMI_MODEL_THINKING_EFFORT": "max", "KIMI_API_KEY": "test-key"},
		OnEvent: func(line string) { events = append(events, line) },
	})
	require.True(t, r.Settled, "%+v", r)
	assert.Equal(t, "session-one", r.SessionID)
	assert.Equal(t, "reviewable plan", r.Text)
	require.NotNil(t, r.Usage)
	assert.Equal(t, int64(9), r.Usage.Input)
	assert.Equal(t, int64(2), r.Usage.Output)
	assert.Nil(t, r.CostUSD, "subscription use has no invented monetary price")
	assert.Nil(t, r.ErrorMessage)
	assert.Len(t, events, 3, "stderr must not enter JSONL events; final line needs no newline")
	assert.Equal(t, kimiComplete, events[len(events)-1])

	rawArgs, err := os.ReadFile(filepath.Join(cwd, "args.bin"))
	require.NoError(t, err)
	assert.Equal(t, []string{"--session", "session-one", "-m", "kimi-code/k3.1-pro", "-p", "plan\nwith literal $(text)", "--output-format", "stream-json"}, strings.Split(strings.TrimSuffix(string(rawArgs), "\x00"), "\x00"))
	actualCwd, err := os.ReadFile(filepath.Join(cwd, "cwd.txt"))
	require.NoError(t, err)
	assert.Equal(t, physicalCwd, strings.TrimSpace(string(actualCwd)))
	env, err := os.ReadFile(filepath.Join(cwd, "env.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(env), "KIMI_MODEL_THINKING_EFFORT=low\n")
	assert.NotContains(t, string(env), "KIMI_MODEL_THINKING_EFFORT=max")
	assert.Contains(t, string(env), "KIMI_API_KEY=test-key")
	assert.NotContains(t, string(env), "UNRELATED_SECRET")
}

func TestKimiCommandCanBeConfiguredForDesktopPATH(t *testing.T) {
	t.Setenv(KimiCommandEnv, kimiScript(t, "printf '%s\\n' "+shQuote(kimiComplete)+"\n"))
	r := RunKimi(KimiArgs{Prompt: "p"})
	assert.True(t, r.Settled)
	assert.Equal(t, "session-one", r.SessionID)
}

func TestKimiParsingPreservesTextAndHonestUsage(t *testing.T) {
	p := ParseKimiStreamJson(`
not-json
{"role":"meta","type":"system.version"}
{"role":"assistant","content":"initial"}
{"role":"assistant","content":[{"type":"text","text":"plan "},{"type":"thinking","text":"hidden"},{"text":"complete"}],"usage":{"input":9,"output":2,"reasoning":3,"cacheRead":4,"cacheWrite":5,"totalTokens":11}}
{"role":"assistant","tool_calls":[{"id":"tool-one"}]}
{"role":"assistant","content":null}
{"role":"assistant","content":""}
{"role":"assistant","content":[]}
{"role":"assistant","content":{"future":"shape"}}
{"role":"meta","type":"turn.step.retrying","error_message":"temporarily unavailable"}
{"usage":{"new_unknown_tokens":99}}
` + kimiComplete)
	assert.Equal(t, "plan complete", p.Text)
	assert.Empty(t, p.ErrorMessage)
	assert.Equal(t, "session-one", p.SessionID)
	require.NotNil(t, p.Usage)
	assert.Equal(t, Usage{Input: 9, Output: 2, Reasoning: 3, CacheRead: 4, CacheWrite: 5, TotalTokens: 11}, *p.Usage)

	for _, usage := range []string{`null`, `{}`, `{"input":null}`, `{"input_tokens":10}`, `{"input":9}`, `{"input":9,"output":2}`, `{"input":"bad","output":0,"totalTokens":1}`, `{"input":-1,"output":0,"totalTokens":1}`, `{"input":1.5,"output":0,"totalTokens":1}`, `{"input":1,"output":-2,"totalTokens":1}`, `[]`, `"unknown"`} {
		t.Run(usage, func(t *testing.T) {
			parsed := ParseKimiStreamJson(`{"role":"assistant","content":"answer","usage":` + usage + `}`)
			assert.Nil(t, parsed.Usage, "invalid or unsupported usage is unknown, not zero")
			assert.Equal(t, "answer", parsed.Text)
		})
	}
	zero := ParseKimiStreamJson(`{"role":"assistant","usage":{"input":0,"output":0,"totalTokens":0,"cacheRead":null}}`)
	require.NotNil(t, zero.Usage, "explicit zeros remain distinguishable from absent usage")
	assert.Equal(t, Usage{}, *zero.Usage)
	latest := ParseKimiStreamJson("{\"usage\":{\"input\":9,\"output\":1,\"totalTokens\":10}}\n{\"usage\":{\"input\":11,\"output\":1,\"totalTokens\":12}}")
	require.NotNil(t, latest.Usage)
	assert.Equal(t, int64(11), latest.Usage.Input, "usage snapshots must not be double-counted")
}

func TestKimiErrorsRemainErrorsAfterLaterOutput(t *testing.T) {
	p := ParseKimiStreamJson(`{"type":"error","message":{"message":"401 unauthorized"}}
{"role":"error","error":{"message":"session occupied"}}
{"role":"error","message":" ","error":"session occupied"}
{"type":"error","message":null,"error":{"code":"unknown"}}
{"role":"assistant","content":"text after error"}
` + kimiComplete)
	assert.Equal(t, `401 unauthorized; session occupied; {"type":"error","message":null,"error":{"code":"unknown"}}`, p.ErrorMessage)
	assert.Equal(t, "text after error", p.Text)
}

func TestKimiCompletionNeedsSuccessfulPrintBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, body, errorContains string
		wantCode                  int
	}{
		{"missing hint", `printf '%s\n' '{"role":"assistant","content":"partial"}'`, "session.resume_hint", 0},
		{"stream error despite exit zero", `printf '%s\n' '{"type":"error","error":{"message":"429 rate limit"}}' '` + kimiComplete + `'`, "429 rate limit", 0},
		{"wrong resume cwd", `printf '%s\n' 'Session belongs to a different working directory' >&2; exit 1`, "different working directory", 1},
		{"failure after hint", `printf '%s\n' '` + kimiComplete + `'; printf '%s\n' 'cleanup failed' >&2; exit 4`, "cleanup failed", 4},
		{"silent failure", `exit 2`, "", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := RunKimi(KimiArgs{Command: kimiScript(t, tc.body), Cwd: t.TempDir(), Prompt: "p", SessionID: "existing"})
			require.NotNil(t, r.ExitCode)
			assert.Equal(t, tc.wantCode, *r.ExitCode)
			assert.False(t, r.Settled)
			assert.Empty(t, r.SessionID, "failed runs do not promise a newly resumable session")
			assert.Nil(t, r.Usage)
			if tc.errorContains != "" {
				require.NotNil(t, r.ErrorMessage)
				assert.Contains(t, *r.ErrorMessage, tc.errorContains)
			} else {
				assert.Nil(t, r.ErrorMessage)
			}
		})
	}
	missing := RunKimi(KimiArgs{Command: filepath.Join(t.TempDir(), "no-kimi"), Cwd: t.TempDir(), Prompt: "p"})
	assert.NotEmpty(t, missing.SpawnError)
	assert.False(t, missing.Settled)
	assert.Empty(t, missing.SessionID)
}

func TestKimiTruncatedOutputCannotConfirmCompletion(t *testing.T) {
	oldLimit := watchdog.MaxBuffer
	watchdog.MaxBuffer = 128
	t.Cleanup(func() { watchdog.MaxBuffer = oldLimit })
	// Even if a hint fits in the retained prefix, later lost output might be an error.
	r := RunKimi(KimiArgs{Command: kimiScript(t, `printf '%s\n' '`+kimiComplete+`'
printf '%s\n' '`+strings.Repeat("x", 512)+`'
`), Prompt: "p"})
	assert.True(t, r.Truncated)
	assert.False(t, r.Settled)
	assert.Empty(t, r.SessionID)
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "truncated")

	// A long unterminated JSONL line cannot grow the callback buffer indefinitely.
	// Force distinct writes; a terminal newline then allows subsequent events again.
	var events []string
	r = RunKimi(KimiArgs{Command: kimiScript(t, `printf '%s' '`+strings.Repeat("x", 256)+`'
sleep 0.03
printf '%s\n' 'end'
printf '%s\n' '`+strings.Repeat("y", 256)+`'
printf '%s' '`+kimiComplete+`'
`), Prompt: "p", OnEvent: func(line string) { events = append(events, line) }})
	assert.True(t, r.Truncated)
	assert.False(t, r.Settled)
	assert.Equal(t, []string{kimiComplete}, events)
}

func TestKimiWatchdogTimeoutCannotBecomeSuccessfulHint(t *testing.T) {
	command := kimiScript(t, `finish() { printf '%s\n' '`+kimiComplete+`'; exit 0; }
trap finish TERM
printf '%s\n' '{"role":"assistant","content":"working"}'
while :; do sleep 0.01; done
`)
	r := RunKimi(KimiArgs{Command: command, Prompt: "p", Watchdog: watchdog.Opts{IdleTimeoutMs: 1000, WallTimeoutMs: 50, KillGraceMs: 100}})
	assert.True(t, r.TimedOut)
	assert.True(t, r.WallTimedOut)
	assert.False(t, r.Settled)
	assert.Empty(t, r.SessionID)
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "timed out")
}

func TestKimiCancellationUsesOwnedWatchdogProcess(t *testing.T) {
	ready := make(chan struct{})
	var once sync.Once
	completed := make(chan Result, 1)
	command := kimiScript(t, `printf '%s\n' '{"role":"assistant","content":"ready"}'
while :; do sleep 0.01; done
`)
	go func() {
		completed <- RunKimi(KimiArgs{Command: command, Prompt: "p", OnEvent: func(line string) { once.Do(func() { close(ready) }) }, Watchdog: watchdog.Opts{WallTimeoutMs: 1000, KillGraceMs: 20}})
	}()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("fake CLI did not start")
	}
	watchdog.KillActiveChildren(20 * time.Millisecond)
	select {
	case r := <-completed:
		assert.False(t, r.Settled)
		assert.True(t, r.Cancelled)
		assert.False(t, r.TimedOut)
		assert.NotEmpty(t, r.Signal)
		assert.Empty(t, r.SessionID)
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled CLI did not exit")
	}
}

func TestKimiCancelledContextRejectsSuccessfulExitAndHint(t *testing.T) {
	command := kimiScript(t, `finish() { printf '%s\n' '`+kimiComplete+`'; exit 0; }
trap finish TERM
printf '%s\n' '{"role":"assistant","content":"ready"}'
while :; do sleep 0.01; done
`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := RunKimi(KimiArgs{Command: command, Prompt: "p", OnEvent: func(string) { cancel() },
		Watchdog: watchdog.Opts{Context: ctx, WallTimeoutMs: 1000, KillGraceMs: 100}})
	require.NotNil(t, r.ExitCode)
	assert.Zero(t, *r.ExitCode)
	assert.True(t, r.Cancelled)
	assert.False(t, r.Settled)
	assert.Empty(t, r.SessionID)
}
