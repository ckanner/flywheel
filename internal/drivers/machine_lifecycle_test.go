package drivers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lifecycleFixture(request codexProStart, harness string) ([]MachineLifecycle, map[string]any) {
	receipt := codexProFixture(request)
	configuration := proObject(receipt, "outcome", "configuration")
	thread, run := "thread-one", "run-one"
	if request.ThreadID != "" {
		thread = request.ThreadID
	}
	names := []string{"thread-ready", "dispatch-intent", "dispatch-possible", "turn-accepted", "terminal", "cleanup-confirmed"}
	if harness == "claude-code-pro" {
		receipt = claudeCodeProFixture(claudeCodeProStart(request))
		configuration = proObject(receipt, "configuration")
		thread, run = proObject(receipt, "session")["id"].(string), receipt["runId"].(string)
		names = []string{"dispatch-intent", "dispatch-possible", "terminal", "cleanup-confirmed"}
	}
	configRaw, _ := json.Marshal(configuration)
	e := MachineLifecycle{LifecycleVersion: 1, Type: "lifecycle", RequestID: request.RequestID, RunID: run,
		Persistent: request.ThreadMode == "persistent", Dispatch: "not-dispatched", Outcome: "unknown", Configuration: configRaw, ThreadID: &thread}
	if harness == "claude-code-pro" {
		e.Configuration, e.ThreadID = json.RawMessage("null"), nil
	}
	var events []MachineLifecycle
	for i, name := range names {
		e.Event, e.Sequence = name, i+1
		if name == "dispatch-possible" {
			e.Dispatch = "possible"
		}
		if name == "turn-accepted" {
			turn := "turn-one"
			e.Dispatch, e.TurnID = "accepted", &turn
		}
		if name == "terminal" {
			e.Dispatch, e.Outcome, e.CleanupConfirmed, e.Configuration, e.ThreadID = "accepted", "completed", true, configRaw, &thread
		}
		events = append(events, e)
	}
	return events, receipt
}

func lifecycleFrame(event MachineLifecycle, harness string) string {
	version := CodexProPackageVersion
	if harness == "claude-code-pro" {
		version = ClaudeCodeProPackageVersion
	}
	return strings.TrimSpace(dshJSONL(map[string]any{"type": "lifecycle", "protocol": 3, "requestId": event.RequestID,
		"adapter": machineAdapter{Package: "dsh-subagent-" + harness, Version: version, Protocol: 1}, "lifecycle": event}))
}

func TestMachineLifecycleCorrelatesConfigurationBeforeDurableAcknowledgement(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		for _, persistent := range []bool{false, true} {
			request := codexProRequest(t)
			request.Protocol, request.ThreadMode = 3, "ephemeral"
			if harness == "claude-code-pro" {
				request.PermissionMode, request.Model = "danger-full-access", "sonnet"
			}
			if persistent {
				request.ThreadMode, request.ThreadID = "persistent", "11111111-1111-4111-8111-111111111111"
			}
			events, receipt := lifecycleFixture(request, harness)
			flow := machineLifecycleFlow{harness: harness, request: request}
			stream := ""
			for _, event := range events {
				frame := lifecycleFrame(event, harness)
				got, err := flow.frame([]byte(frame))
				require.NoError(t, err)
				assert.Equal(t, event.Sequence, got.Sequence)
				stream += frame + "\n"
			}
			if harness == "codex-pro" {
				parsed := parseCodexProStream(stream+dshJSONL(codexProFrame(request, receipt)), request)
				require.True(t, parsed.Settled, parsed.Error)
				assert.False(t, parseCodexProStream(dshJSONL(codexProFrame(request, receipt)), request).Settled)
			} else {
				r := claudeCodeProStart(request)
				parsed := parseClaudeCodeProStream(stream+dshJSONL(claudeCodeProFrame(r, receipt)), r)
				require.True(t, parsed.Settled, parsed.Error)
				assert.False(t, parseClaudeCodeProStream(dshJSONL(claudeCodeProFrame(r, receipt)), r).Settled)
			}
		}
	}
}

func TestMachineLifecycleRejectsUnknownOmittedAndReorderedFacts(t *testing.T) {
	request := codexProRequest(t)
	request.Protocol, request.ThreadMode = 3, "persistent"
	events, _ := lifecycleFixture(request, "codex-pro")
	for _, test := range []struct {
		name string
		at   int
		edit func(*MachineLifecycle)
	}{
		{"version", 0, func(e *MachineLifecycle) { e.LifecycleVersion = 2 }},
		{"run", 0, func(e *MachineLifecycle) { e.RunID = " " }},
		{"request", 0, func(e *MachineLifecycle) { e.RequestID = "wrong" }},
		{"persistence", 0, func(e *MachineLifecycle) { e.Persistent = false }},
		{"sequence", 0, func(e *MachineLifecycle) { e.Sequence = 2 }},
		{"thread", 0, func(e *MachineLifecycle) { x := "bad\x00id"; e.ThreadID = &x }},
		{"turn", 0, func(e *MachineLifecycle) { x := ""; e.TurnID = &x }},
		{"dispatch", 0, func(e *MachineLifecycle) { e.Dispatch = "dispatched" }},
		{"outcome", 0, func(e *MachineLifecycle) { e.Outcome = "invented" }},
		{"early terminal", 0, func(e *MachineLifecycle) { e.Outcome = "completed" }},
		{"unsupported", 0, func(e *MachineLifecycle) { e.Event = "busy" }},
		{"ready shape", 0, func(e *MachineLifecycle) { e.ThreadID = nil }},
		{"bad configuration", 0, func(e *MachineLifecycle) { e.Configuration = json.RawMessage(`[]`) }},
		{"unproved configuration", 0, func(e *MachineLifecycle) { e.Configuration = json.RawMessage(`{}`) }},
		{"foreign run", 1, func(e *MachineLifecycle) { e.RunID = "other" }},
		{"lost thread", 1, func(e *MachineLifecycle) { e.ThreadID = nil }},
		{"bad intent", 1, func(e *MachineLifecycle) { e.Dispatch = "possible" }},
		{"bad possible", 2, func(e *MachineLifecycle) { e.Dispatch = "not-dispatched" }},
		{"bad acceptance", 3, func(e *MachineLifecycle) { e.TurnID = nil }},
		{"lost turn", 4, func(e *MachineLifecycle) { e.TurnID = nil }},
		{"dispatch regression", 4, func(e *MachineLifecycle) { e.Dispatch = "possible" }},
		{"bad cleanup", 5, func(e *MachineLifecycle) { e.CleanupConfirmed = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			flow := machineLifecycleFlow{harness: "codex-pro", request: request}
			for _, e := range events[:test.at] {
				_, err := flow.frame([]byte(lifecycleFrame(e, "codex-pro")))
				require.NoError(t, err)
			}
			e := events[test.at]
			test.edit(&e)
			_, err := flow.frame([]byte(lifecycleFrame(e, "codex-pro")))
			require.Error(t, err)
		})
	}
	for _, raw := range []string{`{`, `null`, `[]`, `{ "lifecycleVersion":1 }`} {
		flow := machineLifecycleFlow{harness: "codex-pro", request: request}
		_, err := flow.accept([]byte(raw))
		require.Error(t, err)
	}
	for _, field := range []string{"persistent", "cleanupConfirmed", "threadId"} {
		raw, _ := json.Marshal(events[0])
		var value map[string]any
		require.NoError(t, json.Unmarshal(raw, &value))
		if field == "threadId" {
			delete(value, field)
		} else {
			value[field] = nil
		}
		raw, _ = json.Marshal(value)
		flow := machineLifecycleFlow{harness: "codex-pro", request: request}
		_, err := flow.accept(raw)
		require.Error(t, err)
	}
	for _, edit := range []func(map[string]any){func(m map[string]any) { m["protocol"] = 2 }, func(m map[string]any) { m["requestId"] = "other" }, func(m map[string]any) { m["adapter"] = nil }} {
		var value map[string]any
		require.NoError(t, json.Unmarshal([]byte(lifecycleFrame(events[0], "codex-pro")), &value))
		edit(value)
		raw, _ := json.Marshal(value)
		flow := machineLifecycleFlow{harness: "codex-pro", request: request}
		_, err := flow.frame(raw)
		require.Error(t, err)
	}
	flow := machineLifecycleFlow{harness: "codex-pro", request: request}
	for _, e := range events {
		_, err := flow.frame([]byte(lifecycleFrame(e, "codex-pro")))
		require.NoError(t, err)
	}
	extra := events[len(events)-1]
	extra.Sequence++
	_, err := flow.frame([]byte(lifecycleFrame(extra, "codex-pro")))
	require.Error(t, err)
	flow = machineLifecycleFlow{harness: "claude-code-pro", request: request}
	_, err = flow.frame([]byte(lifecycleFrame(events[0], "claude-code-pro")))
	require.Error(t, err)
	// A failed call can end before a thread exists; no ready is fabricated.
	failure := events[0]
	failure.Event, failure.ThreadID, failure.Configuration, failure.Outcome, failure.CleanupConfirmed = "terminal", nil, json.RawMessage("null"), "failed", true
	flow = machineLifecycleFlow{harness: "codex-pro", request: request}
	_, err = flow.frame([]byte(lifecycleFrame(failure, "codex-pro")))
	require.NoError(t, err)
	// A purported completion without backend acceptance must not advance.
	failure.Outcome, failure.Dispatch = "completed", "accepted"
	flow = machineLifecycleFlow{harness: "codex-pro", request: request}
	_, err = flow.frame([]byte(lifecycleFrame(failure, "codex-pro")))
	require.Error(t, err)
}

func TestClaudeLifecycleRejectsUnsupportedAcceptanceAndTurnIdentity(t *testing.T) {
	request := codexProStart(claudeCodeProRequest(t))
	request.Protocol, request.ThreadMode = 3, "ephemeral"
	events, _ := lifecycleFixture(request, "claude-code-pro")
	for _, unsupported := range []string{"acceptance", "turn-identity"} {
		t.Run(unsupported, func(t *testing.T) {
			flow := machineLifecycleFlow{harness: "claude-code-pro", request: request}
			for _, event := range events[:2] {
				_, err := flow.frame([]byte(lifecycleFrame(event, "claude-code-pro")))
				require.NoError(t, err)
			}
			event := events[2]
			if unsupported == "acceptance" {
				event.Event, event.Outcome, event.CleanupConfirmed = "turn-accepted", "unknown", false
			} else {
				event.TurnID = new("invented-native-turn")
			}
			_, err := flow.frame([]byte(lifecycleFrame(event, "claude-code-pro")))
			require.Error(t, err, "Claude CLI has neither a public turn acceptance event nor a turn identifier")
		})
	}
}

func TestMachineLifecycleReceiptMustMatchAcknowledgedNativeIdentity(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		for _, identity := range []string{"run", "thread", "turn"} {
			if harness == "claude-code-pro" && identity == "turn" {
				continue // The CLI has no public turn identifier to correlate.
			}
			t.Run(harness+"/"+identity, func(t *testing.T) {
				request := codexProRequest(t)
				if harness == "claude-code-pro" {
					request = codexProStart(claudeCodeProRequest(t))
				}
				request.Protocol, request.ThreadMode = 3, "ephemeral"
				events, receipt := lifecycleFixture(request, harness)
				stream := ""
				for _, event := range events {
					stream += lifecycleFrame(event, harness) + "\n"
				}
				if harness == "codex-pro" {
					execution := proObject(receipt, "outcome", "execution")
					switch identity {
					case "run":
						receipt["runId"], execution["runId"] = "another-run", "another-run"
					case "thread":
						proObject(receipt, "outcome")["threadId"], execution["threadId"] = "another-thread", "another-thread"
					case "turn":
						execution["turnId"] = "another-turn"
					}
					parsed := parseCodexProStream(stream+dshJSONL(codexProFrame(request, receipt)), request)
					assert.False(t, parsed.Settled)
					assert.Contains(t, parsed.Error, "durable lifecycle evidence")
				} else {
					if identity == "run" {
						receipt["runId"] = "33333333-3333-4333-8333-333333333333"
					} else {
						proObject(receipt, "session")["id"] = "33333333-3333-4333-8333-333333333333"
					}
					parsed := parseClaudeCodeProStream(stream+dshJSONL(claudeCodeProFrame(claudeCodeProStart(request), receipt)), claudeCodeProStart(request))
					assert.False(t, parsed.Settled)
					assert.Contains(t, parsed.Error, "durable lifecycle evidence")
				}
			})
		}
	}
}

type lifecycleWriter struct {
	bytes.Buffer
	fail bool
}

func (w *lifecycleWriter) Write(p []byte) (int, error) {
	if w.fail {
		return 0, errors.New("closed pipe")
	}
	return w.Buffer.Write(p)
}
func (w *lifecycleWriter) Close() error { return nil }

func TestMachineAcknowledgementRequiresSuccessfulDurableCallback(t *testing.T) {
	request := codexProRequest(t)
	request.Protocol, request.ThreadMode = 3, "persistent"
	events, _ := lifecycleFixture(request, "codex-pro")
	for _, scenario := range []string{"ok", "callback-failed", "ack-failed", "start-failed", "unsolicited", "malformed", "mismatch", "progress", "unsupported", "missing-progress-text"} {
		t.Run(scenario, func(t *testing.T) {
			w := &lifecycleWriter{}
			stops, calls := 0, 0
			m := newMachineInteraction("codex-pro", request, func(e MachineLifecycle) error {
				calls++
				require.NotContains(t, w.String(), `"type":"ack"`)
				if scenario == "callback-failed" {
					return errors.New("fsync failed")
				}
				if scenario == "ack-failed" {
					w.fail = true
				}
				return nil
			}, func(string) {})
			if scenario == "unsolicited" {
				m.line(lifecycleFrame(events[0], "codex-pro"))
			}
			if scenario == "start-failed" {
				w.fail = true
			}
			m.start(w, func() { stops++ }, []byte("request\n"))
			line := lifecycleFrame(events[0], "codex-pro")
			switch scenario {
			case "malformed":
				line = "{"
			case "mismatch":
				line = strings.Replace(line, request.RequestID, "other", 1)
			case "progress":
				line = dshJSONL(map[string]any{"type": "progress", "protocol": 3, "requestId": request.RequestID, "text": "working"})
			case "unsupported":
				line = dshJSONL(map[string]any{"type": "other", "protocol": 3, "requestId": request.RequestID})
			case "missing-progress-text":
				line = dshJSONL(map[string]any{"type": "progress", "protocol": 3, "requestId": request.RequestID})
			}
			m.line(line)
			if scenario == "ok" {
				require.NoError(t, m.failure())
				assert.Equal(t, 1, calls)
				assert.Contains(t, w.String(), `"type":"ack"`)
				m.line(dshJSONL(map[string]any{"type": "result", "protocol": 3, "requestId": request.RequestID}))
				m.line(line)
				require.Error(t, m.failure())
				assert.Equal(t, 1, calls)
			} else if scenario == "progress" {
				require.NoError(t, m.failure())
				assert.Zero(t, calls)
			} else {
				require.Error(t, m.failure())
				assert.Greater(t, stops, 0)
			}
		})
	}
}

func TestMachineLifecycleProcessFixture(t *testing.T) {
	harness := os.Getenv("FLYWHEEL_TEST_MACHINE_LIFECYCLE")
	if harness == "" {
		return
	}
	decoder := json.NewDecoder(os.Stdin)
	var request codexProStart
	if err := decoder.Decode(&request); err != nil {
		os.Exit(2)
	}
	events, receipt := lifecycleFixture(request, harness)
	for _, event := range events {
		fmt.Println(lifecycleFrame(event, harness))
		var ack struct {
			Type      string `json:"type"`
			Protocol  int    `json:"protocol"`
			RequestID string `json:"requestId"`
			RunID     string `json:"runId"`
			Sequence  int    `json:"sequence"`
		}
		if decoder.Decode(&ack) != nil || ack.Type != "ack" || ack.Protocol != 3 || ack.RequestID != request.RequestID || ack.RunID != event.RunID || ack.Sequence != event.Sequence {
			os.Exit(3)
		}
		if event.Event == "dispatch-possible" {
			_ = os.WriteFile(filepath.Join(request.Cwd, "dispatched"), []byte("one"), 0o600)
		}
	}
	if harness == "codex-pro" {
		fmt.Print(dshJSONL(codexProFrame(request, receipt)))
	} else {
		fmt.Print(dshJSONL(claudeCodeProFrame(claudeCodeProStart(request), receipt)))
	}
	os.Exit(0)
}

func TestMachineLifecycleRoundTripNeverDispatchesBeforeAcknowledgement(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		for _, reject := range []bool{false, true} {
			command := kimiScript(t, "exec "+shQuote(os.Args[0])+" -test.run '^TestMachineLifecycleProcessFixture$'\n")
			cwd := t.TempDir()
			count := 0
			if harness == "codex-pro" {
				t.Setenv(CodexProWorkerEnv, command)
			} else {
				t.Setenv(ClaudeCodeProWorkerEnv, command)
			}
			a := CallArgs{Cwd: cwd, Prompt: "task", Model: "gpt-test", RequestID: "call-lifecycle", Binding: config.LoopBinding{Harness: harness}, PermissionMode: "danger-full-access",
				RecoveryDirectory: filepath.Join(cwd, "recovery"), APIKeys: map[string]string{"FLYWHEEL_TEST_MACHINE_LIFECYCLE": harness}}
			if harness == "claude-code-pro" {
				a.APIKeys = nil
				t.Setenv("FLYWHEEL_TEST_MACHINE_LIFECYCLE", harness)
				a.Model = "sonnet"
				// Native credential filtering intentionally removes arbitrary env;
				// the fixture command itself supplies its non-secret test selector.
				command = kimiScript(t, "FLYWHEEL_TEST_MACHINE_LIFECYCLE=claude-code-pro exec "+shQuote(os.Args[0])+" -test.run '^TestMachineLifecycleProcessFixture$'\n")
				t.Setenv(ClaudeCodeProWorkerEnv, command)
			}
			a.OnLifecycle = func(e MachineLifecycle) error {
				count++
				if e.Event == "dispatch-possible" {
					_, err := os.Stat(filepath.Join(cwd, "dispatched"))
					require.True(t, os.IsNotExist(err))
					if reject {
						return errors.New("durable save unavailable")
					}
				}
				return nil
			}
			r := InvokeHarness(a, "")
			assert.Equal(t, !reject, r.Settled, "%+v", r)
			assert.Greater(t, count, 0)
			_, err := os.Stat(filepath.Join(cwd, "dispatched"))
			if reject {
				require.True(t, os.IsNotExist(err))
				assert.Equal(t, "unknown", r.Session.Outcome)
			} else {
				require.NoError(t, err)
			}
		}
	}
}

func TestDurableDriverAdmissionAndInvalidLifecycleNeverGrantReplay(t *testing.T) {
	ca := codexProRunArgs(t, "ok")
	ca.RecoveryDirectory = filepath.Join(ca.Cwd, "private")
	r := RunCodexPro(ca)
	require.NotNil(t, r.ErrorMessage)
	assert.Contains(t, *r.ErrorMessage, "acknowledgement callback")
	assert.Equal(t, "not-dispatched", r.Session.Dispatch)
	ca.OnLifecycle = func(MachineLifecycle) error { return nil }
	ca.RecoveryDirectory = "relative"
	assert.Equal(t, "not-dispatched", RunCodexPro(ca).Session.Dispatch)
	ca.RecoveryDirectory, ca.OnLifecycle = "", nil
	ca.Prompt = strings.Repeat("x", 1<<20)
	r = RunCodexPro(ca)
	assert.Equal(t, "not-dispatched", r.Session.Dispatch)
	assert.Contains(t, *r.ErrorMessage, "1 MiB")
	claude := claudeCodeProRunArgs(t, "success")
	claude.RecoveryDirectory = filepath.Join(claude.Cwd, "private")
	r = RunClaudeCodePro(claude)
	assert.Equal(t, "not-dispatched", r.Session.Dispatch)
	assert.Contains(t, *r.ErrorMessage, "acknowledgement callback")
	r = InvokeHarness(CallArgs{Model: "test", RequestID: "call-rejected", RecoveryDirectory: ca.Cwd,
		Binding: config.LoopBinding{Harness: "codex-pro", Provider: "unsupported"}}, "")
	assert.Contains(t, r.Session.Protocol, "worker-v3")
	request := codexProRequest(t)
	request.Protocol, request.ThreadMode = 3, "persistent"
	events, _ := lifecycleFixture(request, "codex-pro")
	events[0].Sequence = 2
	assert.Equal(t, "unknown", parseCodexProStream(lifecycleFrame(events[0], "codex-pro"), request).Session.Outcome)
	cr := claudeCodeProRequest(t)
	cr.Protocol, cr.ThreadMode = 3, "ephemeral"
	ce, _ := lifecycleFixture(codexProStart(cr), "claude-code-pro")
	ce[0].Sequence = 2
	assert.Equal(t, "unknown", parseClaudeCodeProStream(lifecycleFrame(ce[0], "claude-code-pro"), cr).Session.Outcome)
}
