package contract

import (
	"testing"

	"github.com/ckanner/flywheel/internal/decision"
	"github.com/stretchr/testify/require"
)

func TestPreparationFreezesSubscriptionTargetAndRejectsContradictoryBinding(t *testing.T) {
	p := preparationContractFixture(t)
	p.Decision = &InvestigationDecision{Backend: "subscription", Model: "gemini-4-hydrogen-low", TimeoutMS: 30000,
		Policy:       decision.Policy{Version: "jev-v1", Mode: decision.Adopt, AllowedData: []decision.Category{decision.DiagnosticFacts}, MinApplicability: .8, MinChoice: .6, MaxCalls: 8},
		Subscription: &decision.SubscriptionBinding{Profile: "hydrogen-local", Harness: "antigravity", Channel: "agy-native", Model: "gemini-4-hydrogen-low", Effort: "low"}}
	_, err := ValidatePreparation(preparationJSON(t, p))
	require.NoError(t, err)
	p.Decision.Model = "gemini-4-neon-high"
	_, err = ValidatePreparation(preparationJSON(t, p))
	require.ErrorContains(t, err, "contradicts")
	p.Decision.Model = p.Decision.Subscription.Model
	p.Decision.Subscription.Effort = "high"
	_, err = ValidatePreparation(preparationJSON(t, p))
	require.ErrorContains(t, err, "frozen subscription")
	p.Decision.Subscription.Effort = "low"
	p.Decision.Endpoint = "https://api.invalid/v1/systemone"
	_, err = ValidatePreparation(preparationJSON(t, p))
	require.Error(t, err)
	p.Decision.Endpoint = ""
	p.Decision.Subscription = nil
	_, err = ValidatePreparation(preparationJSON(t, p))
	require.Error(t, err)
}
