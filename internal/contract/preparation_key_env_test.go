package contract

import (
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/decision"
	"github.com/stretchr/testify/require"
)

func TestPreparationJEVKeyNamesPreserveLegacyLengthAndRejectInvalidNames(t *testing.T) {
	for _, test := range []struct {
		name   string
		keyEnv string
		valid  bool
	}{
		{"legacy-121-characters", strings.Repeat("K", 121), true},
		{"legacy-128-characters", strings.Repeat("K", 128), true},
		{"over-128-characters", strings.Repeat("K", 129), false},
		{"empty", "", false},
		{"leading-digit", "1KEY", false},
		{"assignment", "KEY=secret", false},
		{"hyphen", "KEY-NAME", false},
		{"space", "KEY NAME", false},
		{"newline", "KEY\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := preparationContractFixture(t)
			p.Decision = &InvestigationDecision{
				Policy: decision.Policy{
					Version: "jev-v1", Mode: decision.Adopt,
					AllowedData:      []decision.Category{decision.TaskSummary},
					MinApplicability: .9, MinChoice: .8, MaxCalls: 1,
				},
				Endpoint: "https://example.org/v1/systemone", Model: "jev-1.14.0",
				KeyEnv: test.keyEnv, TimeoutMS: 1000,
			}
			loaded, err := ValidatePreparation(preparationJSON(t, p))
			if test.valid {
				require.NoError(t, err)
				require.Equal(t, test.keyEnv, loaded.Decision.KeyEnv)
			} else {
				require.ErrorContains(t, err, "key-env")
				require.Nil(t, loaded)
			}
		})
	}
}
