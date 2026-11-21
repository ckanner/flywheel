package state

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/stretchr/testify/require"
)

func TestRuntimeSubscriptionLedgerBindsProfileAndAllowsUnknownPerCallUsage(t *testing.T) {
	st := runtimeLedgerFixture(t)
	ledger := st.RuntimeDecisions
	ledger.Config = &config.JEVConfig{Enabled: true, Backend: "subscription", Profile: "hydrogen-local", TimeoutMS: 30000, MaxCalls: 8, MinApplicability: .8, MinChoice: .6, AllowedData: []decision.Category{decision.DiagnosticFacts}}
	ledger.Subscription = &decision.SubscriptionBinding{Profile: "hydrogen-local", Harness: "antigravity", Channel: "agy-native", Model: "gemini-4-hydrogen-low", Effort: "low"}
	require.NoError(t, validateRuntimeDecisions(st))
	ledger.Subscription.Effort = "high"
	require.ErrorContains(t, validateRuntimeDecisions(st), "frozen subscription")
	ledger.Subscription.Effort = "low"
	ledger.Subscription.Profile = "other"
	require.ErrorContains(t, validateRuntimeDecisions(st), "profile contradicts")
	ledger.Subscription.Profile = "hydrogen-local"
	r := ledger.Records[0]
	o := decision.Options{Backend: "subscription", Subscription: ledger.Subscription, Model: ledger.Subscription.Model, ExpectedModel: ledger.Subscription.Model, Timeout: 30 * time.Second,
		AllowedKinds: []decision.ActionKind{decision.Diagnostic, decision.Repair, decision.Model, decision.Replan},
		SubscriptionCall: func(context.Context, string, string) decision.SubscriptionResult {
			return decision.SubscriptionResult{
				Model: ledger.Subscription.Model, Text: `{"action-id":"repair","applicability":1,"choice-confidence":1,"probabilities":{"repair":1,"model-1":0,"replan":0,"feedback-0":0,"recheck-0":0}}`,
				Native: decision.NativeExecution{RequestID: "jev-one", Dispatch: "accepted", Outcome: "completed", CleanupConfirmed: true, ReceiptHash: strings.Repeat("a", 64)}}
		}}
	receipt, err := decision.Evaluate(context.Background(), filepath.Join(t.TempDir(), "journal"), *r.Snapshot, ledger.Config.DecisionPolicy(), o)
	require.NoError(t, err)
	selection := decision.Select(*r.Snapshot, receipt, ledger.Config.DecisionPolicy(), o)
	require.True(t, selection.Adopted)
	r.Receipt, r.Selection, r.Accounted, r.Status = &receipt, &selection, true, "applied"
	st.Packets[r.Packet].PendingStrategy = ""
	require.NoError(t, validateRuntimeDecisions(st))
	dir := t.TempDir()
	_, err = Save(dir, st, nil)
	require.NoError(t, err)
	_, err = Load(dir)
	require.NoError(t, err)
	r.Receipt.Native.CleanupConfirmed = false
	require.Error(t, validateRuntimeDecisions(st))
}

func TestPreparationRejectsReceiptFromAnotherBackend(t *testing.T) {
	st := preparationFixture(t)
	addPreparationDecision(st)
	st.Decisions[0].Backend, st.Decisions[0].Status = "subscription", "unknown"
	require.ErrorContains(t, validatePreparationState(st), "backend contradicts")
}
