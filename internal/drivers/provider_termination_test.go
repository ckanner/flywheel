package drivers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cancelledProFixture(harness string, request codexProStart) map[string]any {
	if harness == "codex-pro" {
		receipt := codexProFixture(request)
		receipt["ok"] = false
		proObject(receipt, "outcome")["stopReason"] = "aborted"
		proObject(receipt, "outcome", "execution")["cancelled"] = true
		proObject(receipt, "outcome", "usage")["status"] = "partial"
		return receipt
	}
	receipt := claudeCodeProFixture(claudeCodeProStart(request))
	receipt["ok"], receipt["output"], receipt["usage"] = false, []any{}, claudeCodeProUnknownUsage()
	receipt["failure"] = map[string]any{"stage": "run", "category": "cancelled", "detail": "cancelled"}
	proObject(receipt, "execution")["outcome"] = "unknown"
	proObject(receipt, "execution")["terminalReceived"] = false
	proObject(receipt, "execution")["cancelled"] = true
	proObject(receipt, "session")["resumable"] = false
	return receipt
}

func TestProTerminationCauseProcessFixture(t *testing.T) {
	harness := os.Getenv("FLYWHEEL_TEST_PRO_TERMINATION")
	if harness == "" {
		return
	}
	var request codexProStart
	if json.NewDecoder(os.Stdin).Decode(&request) != nil {
		os.Exit(91)
	}
	if request.Prompt == "caller-cancel" || request.Prompt == "ack-error" {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM)
		if request.Prompt == "ack-error" {
			events, _ := lifecycleFixture(request, harness)
			fmt.Println(lifecycleFrame(events[0], harness))
		} else {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "progress", "protocol": request.Protocol, "requestId": request.RequestID, "text": "ready"})
		}
		<-signals
	}
	receipt := cancelledProFixture(harness, request)
	frame := codexProFrame(request, receipt)
	if harness == "claude-code-pro" {
		frame = claudeCodeProFrame(claudeCodeProStart(request), receipt)
	}
	if request.Prompt != "provider-cancel" && request.Prompt != "ack-error" && request.Prompt != "damaged-tail" {
		frame["error"] = map[string]any{"code": "INVALID_ACTIVITY", "message": "invalid provider activity metadata"}
	}
	_ = json.NewEncoder(os.Stdout).Encode(frame)
	if request.Prompt == "damaged-tail" {
		fmt.Println("{damaged-stream}")
	}
	os.Exit(1)
}

func TestProWorkerFailureCannotBecomeAUserCancellation(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		for _, scenario := range []string{"invalid-activity", "ack-error", "damaged-tail", "provider-cancel", "caller-cancel"} {
			t.Run(harness+"/"+scenario, func(t *testing.T) {
				command := kimiScript(t, "FLYWHEEL_TEST_PRO_TERMINATION="+harness+" exec "+shQuote(os.Args[0])+" -test.run '^TestProTerminationCauseProcessFixture$'\n")
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				limits := watchdog.Opts{Context: ctx, IdleTimeoutMs: 5000, WallTimeoutMs: 10000}
				onEvent := func(line string) {
					if scenario == "caller-cancel" && strings.Contains(line, `"type":"progress"`) {
						cancel()
					}
				}
				cwd := t.TempDir()
				recoveryDirectory := ""
				var onLifecycle func(MachineLifecycle) error
				if scenario == "ack-error" {
					recoveryDirectory = filepath.Join(cwd, "journal")
					onLifecycle = func(MachineLifecycle) error { return errors.New("durable checkpoint failed") }
				}
				var result Result
				if harness == "codex-pro" {
					result = RunCodexPro(CodexProArgs{Command: command, Cwd: cwd, Model: "gpt-test", Prompt: scenario,
						PermissionMode: "read-only", RequestID: "codex-termination", Watchdog: limits, OnEvent: onEvent,
						RecoveryDirectory: recoveryDirectory, OnLifecycle: onLifecycle})
				} else {
					result = RunClaudeCodePro(ClaudeCodeProArgs{Command: command, Cwd: cwd, Model: "sonnet", Prompt: scenario,
						PermissionMode: "danger-full-access", RequestID: "claude-termination", Watchdog: limits, OnEvent: onEvent,
						RecoveryDirectory: recoveryDirectory, OnLifecycle: onLifecycle})
				}
				raw := result.CodexProReceipt
				if harness == "claude-code-pro" {
					raw = result.ClaudeCodeProReceipt
				}
				require.NotEmpty(t, raw, "retain the original physical cancellation receipt independently of the initiating cause")
				assert.Contains(t, string(raw), `"cancelled":true`)
				assert.False(t, result.Settled)
				assert.False(t, result.TimedOut)
				if scenario == "provider-cancel" || scenario == "caller-cancel" {
					assert.True(t, result.Cancelled, "%+v", result)
					assert.Nil(t, result.ErrorMessage)
				} else {
					assert.False(t, result.Cancelled, "%+v", result)
					require.NotNil(t, result.ErrorMessage)
					if scenario == "ack-error" {
						assert.Contains(t, *result.ErrorMessage, "durable checkpoint failed")
					} else if scenario == "invalid-activity" {
						assert.Contains(t, *result.ErrorMessage, "invalid provider activity metadata")
					} else {
						assert.Contains(t, *result.ErrorMessage, "malformed")
					}
				}
			})
		}
	}
}
