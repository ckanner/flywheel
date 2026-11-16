package drivers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"

	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dshCancelledFrames(request dshStart) (map[string]any, map[string]any) {
	ready, terminal := dshFixtureFrames(request)
	terminal["status"], terminal["text"] = "cancelled", ""
	if request.Protocol == 2 {
		lifecycle := terminal["lifecycle"].(map[string]any)
		lifecycle["outcome"], lifecycle["resumeCandidate"] = "cancelled", false
	}
	return ready, terminal
}

func kimiCancelledFrames(request kimiNativeStart) (map[string]any, map[string]any) {
	ready, terminal := kimiNativeFrames(request)
	terminal["status"], terminal["text"] = "cancelled", ""
	terminal["native"] = nativeReceiptFixture(request.Native, false, true)
	lifecycle := terminal["lifecycle"].(map[string]any)
	lifecycle["outcome"], lifecycle["resumeCandidate"] = "cancelled", false
	return ready, terminal
}

func TestDSHKimiCancelledReceiptCannotHideTransportFailure(t *testing.T) {
	dsh := dshStart{Protocol: 1, RequestID: "dsh-termination", Cwd: "/repo", Provider: "deepseek-account",
		Model: "deepseek-v4-flash", ReasoningEffort: "high", PermissionMode: "read-only"}
	kimi := kimiNativeStart{RequestID: "kimi-termination", Native: json.RawMessage(`{"version":1,"mode":"agent"}`)}
	for _, harness := range []string{"dsh", "kimi"} {
		for _, scenario := range []string{"valid-provider-cancel", "damaged-tail", "write-error", "truncated", "outer-cancel-with-damaged-tail"} {
			t.Run(harness+"/"+scenario, func(t *testing.T) {
				ready, terminal := dshCancelledFrames(dsh)
				if harness == "kimi" {
					ready, terminal = kimiCancelledFrames(kimi)
				}
				exit := 1
				physical := watchdog.Result{Stdout: dshJSONL(ready, terminal), ExitCode: &exit}
				var writeErr error
				switch scenario {
				case "damaged-tail":
					physical.Stdout += "{damaged-tail}\n"
				case "write-error":
					writeErr = errors.New("request pipe failed")
				case "truncated":
					physical.Truncated = true
				case "outer-cancel-with-damaged-tail":
					physical.Cancelled = true
					physical.Stdout += "{damaged-tail}\n"
				}
				var out Result
				if harness == "dsh" {
					out = finishDSH(DSHArgs{Provider: dsh.Provider, Model: dsh.Model}, dsh, physical, writeErr)
				} else {
					out = finishKimiNative(kimi, physical, writeErr)
				}
				wantCancelled := scenario == "valid-provider-cancel" || physical.Cancelled
				assert.Equal(t, wantCancelled, out.Cancelled, "%+v", out)
				assert.False(t, out.Settled)
				assert.False(t, out.TimedOut)
				if !wantCancelled {
					require.NotNil(t, out.ErrorMessage)
					assert.Equal(t, errs.TRANSPORT, out.ErrorCode)
					if scenario == "write-error" {
						assert.Contains(t, *out.ErrorMessage, "request pipe failed")
					} else if scenario == "damaged-tail" {
						assert.False(t, out.Session.CleanupConfirmed)
						assert.Contains(t, physical.Stdout, `"status":"cancelled"`, "the prior receipt remains in the raw stream")
					}
				}
			})
		}
	}
	// A provider failure is not a malformed stream. Its original typed error
	// and diagnostic must not be erased by the cancellation correction.
	exit := 1
	failed := finishDSH(DSHArgs{}, dsh, watchdog.Result{Stdout: dshJSONL(dshErrorFrame(dsh, errs.AUTH)), ExitCode: &exit}, nil)
	assert.False(t, failed.Cancelled)
	assert.Equal(t, errs.AUTH, failed.ErrorCode)
	require.NotNil(t, failed.ErrorMessage)
	assert.Contains(t, *failed.ErrorMessage, "AUTH")
	ready, terminal := kimiCancelledFrames(kimi)
	terminal["status"] = "error"
	terminal["error"] = map[string]any{"code": "AUTH", "message": "original provider failure"}
	terminal["lifecycle"].(map[string]any)["outcome"] = "failed"
	failed = finishKimiNative(kimi, watchdog.Result{Stdout: dshJSONL(ready, terminal), ExitCode: &exit}, nil)
	assert.False(t, failed.Cancelled)
	require.NotNil(t, failed.ErrorMessage)
	assert.Contains(t, *failed.ErrorMessage, "AUTH: original provider failure")
}

func TestDSHKimiOuterCancellationProcessFixture(t *testing.T) {
	harness := os.Getenv("FLYWHEEL_TEST_DSH_KIMI_TERMINATION")
	if harness == "" {
		return
	}
	var ready, terminal map[string]any
	if harness == "dsh" {
		var request dshStart
		if json.NewDecoder(os.Stdin).Decode(&request) != nil {
			os.Exit(91)
		}
		ready, terminal = dshCancelledFrames(request)
	} else {
		var request kimiNativeStart
		if json.NewDecoder(os.Stdin).Decode(&request) != nil {
			os.Exit(92)
		}
		ready, terminal = kimiCancelledFrames(request)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	fmt.Print(dshJSONL(ready))
	<-signals
	fmt.Print(dshJSONL(terminal))
	fmt.Println("{damaged-after-caller-cancel}")
	os.Exit(1)
}

func TestDSHKimiRealContextCancellationSurvivesDamagedCleanupStream(t *testing.T) {
	for _, harness := range []string{"dsh", "kimi"} {
		t.Run(harness, func(t *testing.T) {
			command := kimiScript(t, "FLYWHEEL_TEST_DSH_KIMI_TERMINATION="+harness+" exec "+shQuote(os.Args[0])+" -test.run '^TestDSHKimiOuterCancellationProcessFixture$'\n")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			observedReady := false
			onEvent := func(line string) {
				if strings.Contains(line, `"type":"ready"`) {
					observedReady = true
					cancel()
				}
			}
			limits := watchdog.Opts{Context: ctx, IdleTimeoutMs: 5000, WallTimeoutMs: 10000}
			var out Result
			if harness == "dsh" {
				out = RunDSH(DSHArgs{Command: command, Cwd: t.TempDir(), Provider: "deepseek-account", Model: "deepseek-v4-flash",
					Prompt: "offline cancellation", Watchdog: limits, OnEvent: onEvent})
			} else {
				t.Setenv(KimiNativeWorkerEnv, command)
				out = runKimiNative(KimiArgs{Cwd: t.TempDir(), Prompt: "offline cancellation", PermissionMode: "danger-full-access",
					Native: json.RawMessage(`{"version":1,"mode":"agent"}`), Watchdog: limits, OnEvent: onEvent})
			}
			assert.True(t, observedReady)
			assert.True(t, out.Cancelled, "%+v", out)
			assert.False(t, out.TimedOut)
			assert.False(t, out.Settled)
			assert.Contains(t, out.Stdout, `"status":"cancelled"`)
			assert.Contains(t, out.Stdout, "damaged-after-caller-cancel")
		})
	}
}

func kimiTimeoutFrames(request kimiNativeStart) (map[string]any, map[string]any) {
	ready, terminal := kimiCancelledFrames(request)
	terminal["status"] = "error"
	terminal["error"] = map[string]any{"code": "TIMEOUT", "message": "provider invocation deadline elapsed"}
	terminal["lifecycle"].(map[string]any)["outcome"] = "failed"
	return ready, terminal
}

func TestKimiNativeProviderTimeoutRequiresTrustworthyStructuredCode(t *testing.T) {
	request := kimiNativeStart{RequestID: "kimi-provider-timeout", Native: json.RawMessage(`{"version":1,"mode":"agent"}`)}
	for _, scenario := range []string{"valid-timeout", "damaged-tail", "write-error", "truncated", "message-only", "lowercase-code", "other-timeout-code", "outer-cancel-with-timeout"} {
		t.Run(scenario, func(t *testing.T) {
			ready, terminal := kimiTimeoutFrames(request)
			switch scenario {
			case "message-only":
				terminal["error"] = map[string]any{"code": "TRANSPORT", "message": "TIMEOUT: the provider may have timed out"}
			case "lowercase-code":
				terminal["error"].(map[string]any)["code"] = "timeout"
			case "other-timeout-code":
				terminal["error"].(map[string]any)["code"] = "CLEANUP_TIMEOUT"
			}
			exit := 1
			physical := watchdog.Result{Stdout: dshJSONL(ready, terminal), ExitCode: &exit}
			var writeErr error
			switch scenario {
			case "damaged-tail":
				physical.Stdout += "{damaged-after-provider-timeout}\n"
			case "write-error":
				writeErr = errors.New("request pipe failed")
			case "truncated":
				physical.Truncated = true
			case "outer-cancel-with-timeout":
				physical.Cancelled = true
			}
			out := finishKimiNative(request, physical, writeErr)
			wantTimeout := scenario == "valid-timeout" || physical.Cancelled
			assert.Equal(t, wantTimeout, out.TimedOut, "%+v", out)
			assert.Equal(t, physical.Cancelled, out.Cancelled, "the actual outer cancellation remains authoritative")
			assert.False(t, out.IdleTimedOut)
			assert.False(t, out.WallTimedOut)
			assert.False(t, out.Settled)
			require.NotNil(t, out.ErrorMessage)
			switch {
			case physical.Cancelled:
				assert.Empty(t, out.ErrorCode, "provider timeout must not replace the caller cancellation classification")
			case wantTimeout:
				assert.Equal(t, errs.TIMEOUT, out.ErrorCode)
				assert.Contains(t, *out.ErrorMessage, "timed out in the provider runtime")
				assert.True(t, out.Session.CleanupConfirmed)
				assert.Equal(t, "unknown", out.Session.Outcome)
			default:
				assert.Equal(t, errs.TRANSPORT, out.ErrorCode)
				if scenario == "damaged-tail" || scenario == "write-error" || scenario == "truncated" {
					assert.Contains(t, *out.ErrorMessage, "transport was not trustworthy")
					assert.False(t, out.Session.CleanupConfirmed)
				}
			}
		})
	}
}

func TestKimiNativeProviderTimeoutProcessFixture(t *testing.T) {
	scenario := os.Getenv("FLYWHEEL_TEST_KIMI_PROVIDER_TIMEOUT")
	if scenario == "" {
		return
	}
	var request kimiNativeStart
	if json.NewDecoder(os.Stdin).Decode(&request) != nil {
		os.Exit(93)
	}
	ready, terminal := kimiTimeoutFrames(request)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	fmt.Print(dshJSONL(ready))
	if scenario == "outer-cancel" {
		<-signals
	}
	fmt.Print(dshJSONL(terminal))
	if scenario == "damaged-tail" {
		fmt.Println("{damaged-after-provider-timeout}")
	}
	os.Exit(1)
}

func TestKimiNativePublicDriverPreservesProviderTimeoutAndCallerCancellation(t *testing.T) {
	for _, scenario := range []string{"valid-timeout", "damaged-tail", "outer-cancel"} {
		t.Run(scenario, func(t *testing.T) {
			command := kimiScript(t, "FLYWHEEL_TEST_KIMI_PROVIDER_TIMEOUT="+scenario+" exec "+shQuote(os.Args[0])+" -test.run '^TestKimiNativeProviderTimeoutProcessFixture$'\n")
			t.Setenv(KimiNativeWorkerEnv, command)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			observedReady := false
			out := RunKimi(KimiArgs{Cwd: t.TempDir(), Prompt: "offline provider timeout", PermissionMode: "danger-full-access",
				Native:   json.RawMessage(`{"version":1,"mode":"agent"}`),
				Watchdog: watchdog.Opts{Context: ctx, IdleTimeoutMs: 5000, WallTimeoutMs: 10000},
				OnEvent: func(line string) {
					if strings.Contains(line, `"type":"ready"`) {
						observedReady = true
						if scenario == "outer-cancel" {
							cancel()
						}
					}
				}})
			assert.True(t, observedReady)
			assert.Equal(t, scenario != "damaged-tail", out.TimedOut, "%+v", out)
			assert.Equal(t, scenario == "outer-cancel", out.Cancelled)
			assert.False(t, out.IdleTimedOut)
			assert.False(t, out.WallTimedOut)
			assert.False(t, out.Settled)
			assert.Contains(t, out.Stdout, `"code":"TIMEOUT"`, "the original provider receipt remains available")
			switch scenario {
			case "valid-timeout":
				assert.Equal(t, errs.TIMEOUT, out.ErrorCode)
				assert.True(t, out.Session.CleanupConfirmed)
			case "damaged-tail":
				assert.Equal(t, errs.TRANSPORT, out.ErrorCode)
				assert.False(t, out.Session.CleanupConfirmed)
			default:
				assert.Empty(t, out.ErrorCode)
			}
		})
	}
}
