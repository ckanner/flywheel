package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

type moduleHistoryDriver struct {
	drivers.Driver
	calls  []drivers.CallArgs
	mutate func(*drivers.Result)
}

func (d *moduleHistoryDriver) call(a drivers.CallArgs) drivers.Result {
	d.calls = append(d.calls, a)
	id := fmt.Sprintf("observed-history-%d", len(d.calls))
	return drivers.MockResult(func(r *drivers.Result) {
		r.Harness, r.SessionID = a.Binding.Harness, id
		r.Session = &drivers.SessionOutcome{RequestID: id, SessionID: id, Protocol: a.Binding.Harness + "/fixture",
			Dispatch: "dispatched", Outcome: "completed", CleanupConfirmed: true, ResumeCandidate: true, UsageStatus: "complete"}
		if d.mutate != nil {
			d.mutate(r)
		}
	})
}

func (d *moduleHistoryDriver) Intake(a drivers.CallArgs) drivers.Result { return d.call(a) }
func (d *moduleHistoryDriver) Garden(a drivers.CallArgs) drivers.Result { return d.call(a) }

func TestAdviceNativeHistoryIsRetainedWithoutReusingAnotherCall(t *testing.T) {
	for _, role := range []string{"intake", "garden"} {
		for _, harness := range []string{"codex-pro", "claude-code-pro", "dsh", "kimi", "codex", "pi"} {
			t.Run(role+"/"+harness, func(t *testing.T) {
				taskDir := filepath.Join(t.TempDir(), "alert-repair")
				cfg := testConfigT(t, taskDir)
				cfg.Permissions.Mode = "full-access"
				driver := &moduleHistoryDriver{}
				m := NewContext(CtxOpts{TaskDir: taskDir, Cwd: taskDir, Config: cfg,
					Drivers: &drivers.Drivers{Intake: driver, Garden: driver}})
				binding := fallback.Binding{Harness: harness, Channel: "subscription", Model: "chosen", Thinking: "high"}
				options := CallOptions{Role: role, Binding: binding, Prompt: "PRIVATE_PROMPT_DOES_NOT_BECOME_A_TITLE"}
				for range 2 {
					require.True(t, CallAgent(m, options).OK)
				}
				require.Len(t, driver.calls, 2)
				for _, call := range driver.calls {
					require.Empty(t, call.SessionID, "saved provider IDs must never become module input history")
					require.Equal(t, harness == "codex-pro" || harness == "claude-code-pro", call.PersistentSession)
					if config.SupportsScopedSessions(harness) {
						require.Equal(t, "Flywheel · "+role+" · alert-repair", call.SessionTitle)
					} else {
						require.Empty(t, call.SessionTitle, "do not invent naming support for legacy adapters")
					}
					require.NotContains(t, call.SessionTitle, "PRIVATE_PROMPT")
					require.Equal(t, options.Prompt, call.Prompt)
					require.Equal(t, cfg.PermissionMode(role, harness), call.PermissionMode)
					require.Equal(t, ModuleWatchdogOpts(cfg), call.Watchdog)
					require.Nil(t, call.OnLifecycle)
					require.Empty(t, call.RecoveryDirectory)
				}
				require.Len(t, readMetricsLinesT(t, m.MetricsFile), 2)
				require.NoFileExists(t, filepath.Join(taskDir, "state.json"), "history retention must not create a role ledger")
			})
		}
	}
}

func TestRetainedAdviceHistoryKeepsUnknownAndCancellationGates(t *testing.T) {
	for _, role := range []string{"intake", "garden"} {
		for _, harness := range []string{"codex-pro", "claude-code-pro"} {
			for _, failure := range []string{"unknown", "cleanup", "cancelled"} {
				t.Run(role+"/"+harness+"/"+failure, func(t *testing.T) {
					taskDir := t.TempDir()
					cfg := testConfigT(t, taskDir)
					writeFileT(t, filepath.Join(taskDir, "state.json"), `{"revision":0,"status":"pending","packets":{},"inbox":[],"budget":{"daily":{},"cache":{"by-provider":{}}}}`)
					driver := &moduleHistoryDriver{mutate: func(r *drivers.Result) {
						r.Usage = &drivers.Usage{Input: 11, Output: 7, TotalTokens: 18}
						switch failure {
						case "unknown":
							r.Session.Outcome = "unknown"
						case "cleanup":
							r.CleanupError = "owned group remains"
						case "cancelled":
							r.Cancelled = true
						}
					}}
					m := NewContext(CtxOpts{TaskDir: taskDir, Cwd: taskDir, Config: cfg,
						Drivers: &drivers.Drivers{Intake: driver, Garden: driver},
						Sleep:   func(int64) { t.Fatal("retained history must not enable automatic retry") }})
					options := CallOptions{Role: role, Prompt: "private advice", Binding: fallback.Binding{Harness: harness, Channel: "subscription", Model: "chosen"}}
					result := CallAgent(m, options)
					require.False(t, result.OK)
					require.Equal(t, errs.TRANSPORT, result.ErrorCode)
					require.False(t, CallAgent(m, options).OK)
					require.Len(t, driver.calls, 1)
					require.True(t, driver.calls[0].PersistentSession)
					require.Empty(t, driver.calls[0].SessionID)
					saved, err := state.Load(taskDir)
					require.NoError(t, err)
					require.Equal(t, int64(18), saved.Budget.Totals.Tokens)
					require.Nil(t, saved.Sessions, "native persistence is independent of managed role history")
					require.Len(t, readMetricsLinesT(t, m.MetricsFile), 1)
				})
			}
		}
	}
}

func TestRetainedAdviceRetryStartsFreshNativeHistory(t *testing.T) {
	for _, role := range []string{"intake", "garden"} {
		for _, harness := range []string{"codex-pro", "claude-code-pro"} {
			t.Run(role+"/"+harness, func(t *testing.T) {
				taskDir := t.TempDir()
				cfg := testConfigT(t, taskDir)
				driver := &moduleHistoryDriver{}
				driver.mutate = func(r *drivers.Result) {
					if len(driver.calls) == 1 {
						r.ExitCode, r.ErrorCode, r.Settled = new(1), errs.SERVER, false
						r.Session.Dispatch, r.Session.Outcome = "not-dispatched", "failed"
					}
				}
				waits := 0
				m := NewContext(CtxOpts{TaskDir: taskDir, Cwd: taskDir, Config: cfg,
					Drivers: &drivers.Drivers{Intake: driver, Garden: driver}, Sleep: func(int64) { waits++ }})
				result := CallAgent(m, CallOptions{Role: role, Prompt: "advice", Binding: fallback.Binding{
					Harness: harness, Channel: "subscription", Model: "chosen"}})
				require.True(t, result.OK)
				require.Equal(t, 1, waits)
				require.Len(t, driver.calls, 2)
				for _, call := range driver.calls {
					require.True(t, call.PersistentSession)
					require.Empty(t, call.SessionID, "a bounded safe retry must still start its own native history")
				}
				entries := readMetricsLinesT(t, m.MetricsFile)
				require.Len(t, entries, 2)
				require.NotEqual(t, entries[0]["invocation"].(map[string]any)["sessionId"], entries[1]["invocation"].(map[string]any)["sessionId"])
			})
		}
	}
}

func TestModuleSessionTitleIsReadableAndBoundedWithoutPromptText(t *testing.T) {
	for _, sample := range []struct{ task, cwd, want string }{
		{"/tasks/alert\n\u202erepair", "/workspace", "Flywheel · garden · alert repair"},
		{"", "/workspace/alerts", "Flywheel · garden · alerts"},
		{string(os.PathSeparator), "/workspace/alerts", "Flywheel · garden · alerts"},
		{"/tasks/\n\u200d", "/workspace", "Flywheel · garden"},
	} {
		m := &Ctx{TaskDir: sample.task, Cwd: sample.cwd}
		require.Equal(t, sample.want, m.sessionTitle("garden"))
	}
	m := &Ctx{TaskDir: "/tasks/" + strings.Repeat("𠮷", 200)}
	title := m.sessionTitle("intake")
	require.True(t, utf8.ValidString(title))
	require.LessOrEqual(t, utf8.RuneCountInString(title), 120)
	require.LessOrEqual(t, len(title), 512)
	require.True(t, strings.HasSuffix(title, "…"))
	for _, r := range title {
		require.False(t, unicode.IsControl(r) || unicode.In(r, unicode.Cf))
	}
}
