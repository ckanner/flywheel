package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/templates"
	"github.com/stretchr/testify/require"
)

func subscriptionDecisionCLI(t *testing.T) (string, []string) {
	t.Helper()
	_, input, _ := decisionCLIFixture(t)
	raw, err := templates.FS.ReadFile("flywheel.config.example.json")
	require.NoError(t, err)
	var values map[string]any
	require.NoError(t, json.Unmarshal(raw, &values))
	values["jev"].(map[string]any)["enabled"] = true
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(input), "flywheel.config.json"), []byte(mustJSONT(t, values)), 0600))
	return input, []string{"decision", "evaluate", "--input", input, "--cwd", filepath.Dir(input), "--directory", filepath.Join(t.TempDir(), "evaluation"), "--max-calls", "1"}
}

func TestDecisionCLIEvaluatesSubscribedNativeWithoutAPIFlagsOrKeys(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "configured-timeout", true: "shorter-timeout"}[explicit], func(t *testing.T) {
			_, args := subscriptionDecisionCLI(t)
			want := 30 * time.Second
			if explicit {
				args = append(args, "--timeout", "1s")
				want = time.Second
			}
			calls := 0
			saved := subscriptionDecisionOptions
			t.Cleanup(func() { subscriptionDecisionOptions = saved })
			subscriptionDecisionOptions = func(_ context.Context, b *decision.SubscriptionBinding, cwd, dir string, timeout time.Duration) decision.Options {
				require.Equal(t, want, timeout)
				require.True(t, filepath.IsAbs(cwd))
				require.Equal(t, "antigravity", b.Harness)
				return decision.Options{Backend: "subscription", Subscription: b, Model: b.Model, ExpectedModel: b.Model, Timeout: timeout, SubscriptionCall: func(context.Context, string, string) decision.SubscriptionResult {
					calls++
					return decision.SubscriptionResult{
						Model: b.Model, Text: `{"action-id":"inspect-config","applicability":1,"choice-confidence":1,"probabilities":{"inspect-config":1,"inspect-log":0}}`,
						Native: decision.NativeExecution{RequestID: "jev-evaluation", Dispatch: "accepted", Outcome: "completed", CleanupConfirmed: true, ReceiptHash: strings.Repeat("a", 64)}}
				}}
			}
			code, out := runCLI(t, args...)
			require.Zero(t, code, out)
			require.Equal(t, 1, calls)
			var report decisionCLIReport
			require.NoError(t, json.Unmarshal([]byte(out), &report))
			require.False(t, report.Adopted)
			require.Equal(t, decision.Advisory, report.Mode)
			require.Equal(t, "subscription", report.Receipt.Backend)
			require.Nil(t, report.Receipt.Usage.Input)
		})
	}
}

func TestDecisionCLISubscriptionRejectsConflictingFlagsAndUnavailableCwd(t *testing.T) {
	_, args := subscriptionDecisionCLI(t)
	for _, flags := range [][]string{{"--endpoint", "https://api.invalid/v1/systemone"}, {"--key-env", "API_KEY"}, {"--model", "other"}, {"--expected-model", "other"}} {
		code, out := runCLI(t, append(append([]string{}, args...), flags...)...)
		require.Equal(t, 1, code, out)
		require.Contains(t, out, "conflict")
	}
	for _, pathError := range []bool{false, true} {
		t.Run(map[bool]string{false: "absolute", true: "physical"}[pathError], func(t *testing.T) {
			savedAbs, savedPhysical := decisionAbsolutePath, decisionPhysicalPath
			t.Cleanup(func() { decisionAbsolutePath, decisionPhysicalPath = savedAbs, savedPhysical })
			failure := func(string) (string, error) { return "", errors.New("cwd changed during evaluation") }
			if pathError {
				decisionPhysicalPath = failure
			} else {
				decisionAbsolutePath = failure
			}
			code, out := runCLI(t, args...)
			require.Equal(t, 1, code, out)
			require.Contains(t, out, "cwd changed")
		})
	}
	saved := loadDecisionConfiguration
	t.Cleanup(func() { loadDecisionConfiguration = saved })
	loadDecisionConfiguration = func(context.Context, string, string) (*config.Config, error) {
		c := config.Default()
		c.JEV = config.JEVConfig{Enabled: true, Backend: "subscription", Profile: "missing"}
		return &c, nil
	}
	key := ""
	o := decision.Options{Timeout: time.Second}
	_, err := configureDecisionEvaluation(context.Background(), "", "", &key, &o)
	require.ErrorContains(t, err, "tool-free Antigravity")
}
