package decision

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func subscriptionFixture() (Snapshot, Policy, Options, SubscriptionResult) {
	s, p, _ := fixture()
	b := &SubscriptionBinding{Profile: "hydrogen-local", Harness: "antigravity", Channel: "agy-native", Model: "gemini-4-hydrogen-low", Effort: "low"}
	o := Options{Backend: "subscription", Subscription: b, Model: b.Model, ExpectedModel: b.Model, Timeout: time.Second}
	r := SubscriptionResult{Model: b.Model, Text: `{"action-id":"inspect-config","applicability":0.98,"choice-confidence":0.83,"probabilities":{"inspect-config":0.96,"inspect-log":0.04}}`,
		Native: NativeExecution{RequestID: "jev-request", Dispatch: "accepted", Outcome: "completed", CleanupConfirmed: true, ReceiptHash: strings.Repeat("a", 64)}}
	o.SubscriptionCall = func(context.Context, string, string) SubscriptionResult { return r }
	return s, p, o, r
}

func TestSubscriptionChoiceIsToolFreeBoundedAndNeverUsesHTTP(t *testing.T) {
	s, p, o, reply := subscriptionFixture()
	calls := 0
	o.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("subscription attempted an API fallback")
		return nil, nil
	})
	o.SubscriptionCall = func(ctx context.Context, prompt, id string) SubscriptionResult {
		calls++
		deadline, bounded := ctx.Deadline()
		require.True(t, bounded)
		require.LessOrEqual(t, time.Until(deadline), o.Timeout)
		require.Contains(t, prompt, "You have no tools")
		require.Contains(t, prompt, "uncalibrated")
		require.Contains(t, prompt, `"inspect-config"`)
		require.True(t, strings.HasPrefix(id, "decision-"))
		return reply
	}
	dir := filepath.Join(t.TempDir(), "journal")
	r, err := Evaluate(context.Background(), dir, s, p, o)
	require.NoError(t, err)
	require.Equal(t, "ok", r.Status)
	require.Equal(t, "subscription", r.Backend)
	require.Zero(t, r.HTTPStatus)
	require.Nil(t, r.Usage.Input, "conversation totals must not be invented as per-call usage")
	require.NoError(t, ValidateReceiptIdentity(s, p, o, r))
	require.True(t, Select(s, r, p, o).Adopted)
	stored, found, err := Recover(dir, s, p, o)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, r, stored)
	stored, err = Evaluate(context.Background(), dir, s, p, o)
	require.NoError(t, err)
	require.Equal(t, r, stored)
	require.Equal(t, 1, calls)
	o.Subscription.Effort, o.Subscription.Model, o.Model, o.ExpectedModel = "high", "gemini-4-hydrogen-high", "gemini-4-hydrogen-high", "gemini-4-hydrogen-high"
	require.False(t, Select(s, r, p, o).Adopted, "same profile name cannot hide a changed model/effort")
	require.Error(t, ValidateReceiptIdentity(s, p, o, r))
}

func TestSubscriptionTerminalsNeverInventAUsableChoice(t *testing.T) {
	for _, test := range []struct {
		name, status, reason string
		change               func(*SubscriptionResult, *Policy)
		cleanupUnknown       bool
	}{
		{"not-dispatched", "skipped", "subscription-not-dispatched", func(r *SubscriptionResult, _ *Policy) {
			r.Native.Dispatch, r.Native.Outcome = "not-dispatched", "failed"
		}, false},
		{"cleanup", "unknown", "subscription-cleanup-unconfirmed", func(r *SubscriptionResult, _ *Policy) { r.Native.CleanupConfirmed = false }, true},
		{"unknown", "unknown", "subscription-outcome-unknown", func(r *SubscriptionResult, _ *Policy) { r.Native.Outcome = "unknown" }, false},
		{"failed", "error", "subscription-query-failed", func(r *SubscriptionResult, _ *Policy) { r.Native.Outcome = "failed" }, false},
		{"model", "error", "subscription-model-or-output-mismatch", func(r *SubscriptionResult, _ *Policy) { r.Model = "gemini-4-neon-high" }, false},
		{"oversize", "error", "subscription-model-or-output-mismatch", func(r *SubscriptionResult, _ *Policy) { r.Text = strings.Repeat("x", maxDocument+1) }, false},
		{"syntax", "error", "invalid-subscription-answer", func(r *SubscriptionResult, _ *Policy) { r.Text = "```json" }, false},
		{"missing-probability", "error", "invalid-subscription-answer", func(r *SubscriptionResult, _ *Policy) { r.Text = `{"action-id":"inspect-config"}` }, false},
		{"missing-proof", "error", "invalid-subscription-answer", func(r *SubscriptionResult, _ *Policy) { r.Native.ReceiptHash = "" }, false},
		{"applicability", "ok", "applicability-abstained", func(_ *SubscriptionResult, p *Policy) { p.MinApplicability = 1 }, false},
		{"choice", "ok", "choice-abstained", func(_ *SubscriptionResult, p *Policy) { p.MinChoice = 1 }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, p, o, reply := subscriptionFixture()
			test.change(&reply, &p)
			o.SubscriptionCall = func(context.Context, string, string) SubscriptionResult { return reply }
			r, err := Evaluate(context.Background(), filepath.Join(t.TempDir(), "journal"), s, p, o)
			require.NoError(t, err)
			require.Equal(t, test.status, r.Status)
			require.Equal(t, test.reason, r.Reason)
			require.Equal(t, test.cleanupUnknown, CleanupUnknown(r))
			require.False(t, Select(s, r, p, o).Adopted)
			require.NoError(t, ValidateReceiptIdentity(s, p, o, r))
		})
	}
}

func TestSubscriptionValidationAndLegacyHashCompatibility(t *testing.T) {
	s, p, o, _ := subscriptionFixture()
	require.NoError(t, ValidateSubscriptionBinding(o.Subscription))
	require.Error(t, ValidateSubscriptionBinding(nil))
	for _, b := range []SubscriptionBinding{
		{Profile: "bad/profile", Harness: "antigravity", Channel: "agy-native", Model: "gemini-4-hydrogen-low", Effort: "low"},
		{Profile: "hydrogen-local", Harness: "claude-code-pro", Channel: "agy-native", Model: "gemini-4-hydrogen-low", Effort: "low"},
		{Profile: "hydrogen-local", Harness: "antigravity", Channel: "agy-native", Model: "gemini-4-carbon-xhigh", Effort: "xhigh"},
		{Profile: "hydrogen-local", Harness: "antigravity", Channel: "agy-native", Model: "gemini-4-hydrogen-low", Effort: "high"},
	} {
		require.Error(t, ValidateSubscriptionBinding(&b))
	}
	o.SubscriptionCall = nil
	r, err := Evaluate(context.Background(), filepath.Join(t.TempDir(), "journal"), s, p, o)
	require.NoError(t, err)
	require.Equal(t, "missing-subscription-adapter", r.Reason)
	require.False(t, CleanupUnknown(r))
	for _, change := range []func(*Options){func(o *Options) { o.Backend = "api" }, func(o *Options) { o.APIKey = "secret" }, func(o *Options) { o.Endpoint = "https://example.test/v1/systemone" }, func(o *Options) { o.Model = "other" }} {
		_, _, bad, _ := subscriptionFixture()
		change(&bad)
		require.False(t, validTarget(bad))
	}
	_, _, old := fixture()
	before := targetHash(old)
	old.Backend = "http"
	require.Equal(t, before, targetHash(old), "old HTTP target hashes are a persisted contract")
	require.True(t, validTarget(old))
	r.Native = &NativeExecution{RequestID: "x", Dispatch: "not-dispatched", Outcome: "failed", CleanupConfirmed: true}
	require.Error(t, ValidateReceiptIdentity(s, p, o, r), "skipped cannot carry native dispatch claims")
	r.Native.RequestID = "bad/id"
	require.Error(t, ValidateReceiptIdentity(s, p, o, r))
}

func TestSubscriptionRecoveryAddsOnlyCleanupProofAndNeverReplays(t *testing.T) {
	s, p, o, reply := subscriptionFixture()
	reply.Native.CleanupConfirmed = false
	queries, dispatches := 0, 0
	o.SubscriptionCall = func(context.Context, string, string) SubscriptionResult { dispatches++; return reply }
	o.SubscriptionRecover = func(string) SubscriptionResult { queries++; r := reply; r.Native.CleanupConfirmed = true; return r }
	dir := filepath.Join(t.TempDir(), "journal")
	original, err := Evaluate(context.Background(), dir, s, p, o)
	require.NoError(t, err)
	require.True(t, CleanupUnknown(original))
	reconciled, found, err := Recover(dir, s, p, o)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "unknown", reconciled.Status)
	require.Equal(t, "subscription-cleanup-reconciled", reconciled.Reason)
	require.False(t, CleanupUnknown(reconciled))
	require.False(t, Select(s, reconciled, p, o).Adopted)
	require.Nil(t, reconciled.Usage.Input)
	stored, err := os.ReadFile(filepath.Join(dir, journalKey(s)+".json"))
	require.NoError(t, err)
	var unchanged Receipt
	require.NoError(t, json.Unmarshal(stored, &unchanged))
	require.Equal(t, original, unchanged)
	again, err := Evaluate(context.Background(), dir, s, p, o)
	require.NoError(t, err)
	require.Equal(t, reconciled, again)
	require.Equal(t, 1, queries)
	require.Equal(t, 1, dispatches)
	require.NoError(t, os.WriteFile(filepath.Join(dir, journalKey(s)+".reconciled.json"), []byte(`{}`), 0600))
	_, _, err = Recover(dir, s, p, o)
	require.ErrorContains(t, err, "reconciliation")
}

func TestSubscriptionRecoveryKeepsUnknownWithoutTrustworthyProof(t *testing.T) {
	for _, name := range []string{"no-query", "unsafe-target", "missing-proof", "different-request", "unconfirmed", "read-error", "write-error"} {
		t.Run(name, func(t *testing.T) {
			s, p, o, reply := subscriptionFixture()
			reply.Native.CleanupConfirmed = false
			o.SubscriptionCall = func(context.Context, string, string) SubscriptionResult { return reply }
			dir := filepath.Join(t.TempDir(), "journal")
			_, err := Evaluate(context.Background(), dir, s, p, o)
			require.NoError(t, err)
			o.SubscriptionRecover = func(string) SubscriptionResult {
				r := reply
				r.Native.CleanupConfirmed = true
				switch name {
				case "missing-proof":
					r.Native.ReceiptHash = ""
				case "different-request":
					r.Native.RequestID = "other"
				case "unconfirmed":
					r.Native.CleanupConfirmed = false
				}
				return r
			}
			switch name {
			case "no-query":
				o.SubscriptionRecover = nil
			case "unsafe-target":
				o.APIKey = "conflict"
			case "read-error":
				require.NoError(t, os.Mkdir(filepath.Join(dir, journalKey(s)+".reconciled.json"), 0700))
			case "write-error":
				saved := journalLink
				journalLink = func(*os.Root, string, string) error { return errors.New("write blocked") }
				t.Cleanup(func() { journalLink = saved })
			}
			r, found, err := Recover(dir, s, p, o)
			require.True(t, found)
			if name == "read-error" || name == "write-error" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.True(t, CleanupUnknown(r))
			}
		})
	}
}
