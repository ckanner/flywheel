package state

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
)

// Each archived round is the exact settled state before the next input was
// accepted. Snapshots do not recursively duplicate the earlier history.
type PreparationRound struct {
	Version  int               `json:"version"`
	Number   int               `json:"number"`
	Digest   string            `json:"digest"`
	Snapshot *PreparationState `json:"snapshot"`
}

func PreparationRemaining(st *PreparationState) contract.InvestigationLimits {
	limits := st.Contract.Limits
	limits.MaxActions -= st.ActionsUsed
	limits.MaxOutputBytes -= st.OutputBytes
	limits.MaxSeconds = max(0, limits.MaxSeconds-int((st.ElapsedMS+999)/1000))
	return limits
}

func PreparationRounds(st *PreparationState) []*PreparationState {
	result := make([]*PreparationState, 0, len(st.History)+1)
	for _, round := range st.History {
		result = append(result, round.Snapshot)
	}
	return append(result, st)
}

func PreparationTotals(st *PreparationState) (actions int, output int, elapsed int64) {
	for _, round := range PreparationRounds(st) {
		actions += round.ActionsUsed
		output += round.OutputBytes
		elapsed += round.ElapsedMS
	}
	return
}

// A disabled intermediate round cannot replenish an earlier call allocation.
// Unknown dispatched calls remain spent; a trustworthy skipped receipt does not.
func PreparationDecisionRemaining(st *PreparationState, maximum int) int {
	for _, round := range PreparationRounds(st) {
		if round.Contract.Decision == nil {
			continue
		}
		used := 0
		for _, receipt := range round.Decisions {
			if receipt.Status != "skipped" {
				used++
			}
		}
		if round.DecisionPending != nil {
			used++
		}
		maximum = min(maximum, round.Contract.Decision.Policy.MaxCalls-used)
	}
	return max(0, maximum)
}

// Native advice may have left a process running even after its response was
// journaled as unknown. A report or another round cannot erase that fact.
func PreparationCleanupUnknown(st *PreparationState) bool {
	for _, round := range PreparationRounds(st) {
		for _, receipt := range round.Decisions {
			if decision.CleanupUnknown(receipt) {
				return true
			}
		}
	}
	return false
}

func preparationLimitsWithin(next, remaining contract.InvestigationLimits) bool {
	return next.MaxActions <= remaining.MaxActions && next.MaxSeconds <= remaining.MaxSeconds && next.MaxOutputBytes <= remaining.MaxOutputBytes && next.MaxFileBytes <= remaining.MaxFileBytes
}

// Additional evidence is not new implementation authority. Repairing a
// report-only task uses the separate exact planning approval, never Continue.
func PreparationAuthorityMatches(previous, next contract.Preparation) bool {
	return previous.Goal == next.Goal && previous.Intent == next.Intent && slices.Equal(previous.ImplementationScope, next.ImplementationScope)
}

func validatePreparationRounds(st *PreparationState) error {
	rounds := PreparationRounds(st)
	for i, previous := range st.History {
		snapshot := previous.Snapshot
		if snapshot == nil || previous.Version != 1 || previous.Number != i+1 || len(snapshot.History) != 0 || previous.Digest != PreparationHash(snapshot) {
			return fmt.Errorf("investigation round must retain its exact immutable snapshot")
		}
		if snapshot.RunID != st.RunID || snapshot.Cwd != st.Cwd || snapshot.Contract.ID != st.Contract.ID || snapshot.Report == nil || snapshot.Pending != nil || snapshot.DecisionPending != nil || snapshot.AnalysisPending != nil || rounds[i+1].Revision <= snapshot.Revision {
			return fmt.Errorf("investigation round has unsettled work or changed task identity")
		}
		if !PreparationAuthorityMatches(snapshot.Contract, rounds[i+1].Contract) {
			return fmt.Errorf("new investigation round cannot change the original goal, intent or implementation scope")
		}
		contextual := *snapshot
		contextual.History = st.History[:i]
		if err := validatePreparationCurrent(&contextual); err != nil {
			return err
		}
		if !preparationLimitsWithin(rounds[i+1].Contract.Limits, PreparationRemaining(snapshot)) {
			return fmt.Errorf("new investigation round cannot reset spent action, output, or time budgets")
		}
		if next := rounds[i+1].Contract.Decision; next != nil && next.Policy.MaxCalls > PreparationDecisionRemaining(&contextual, next.Policy.MaxCalls) {
			return fmt.Errorf("new investigation round cannot reset decision calls")
		}
	}
	seen := map[string]bool{}
	for _, round := range rounds {
		for _, observation := range round.Observations {
			if seen[observation.Invocation.ID] {
				return fmt.Errorf("investigation invocation identities cannot repeat across rounds")
			}
		}
		if round.Pending != nil && seen[round.Pending.ID] {
			return fmt.Errorf("investigation pending invocation repeats earlier evidence")
		}
		for _, observation := range round.Observations {
			seen[observation.Invocation.ID] = true
		}
	}
	return nil
}

func validatePreparationTransition(old, next *PreparationState) error {
	if old == nil {
		return nil
	}
	if old.RunID != next.RunID || old.Cwd != next.Cwd || old.Contract.ID != next.Contract.ID || len(next.History) < len(old.History) || len(next.History) > len(old.History)+1 {
		return fmt.Errorf("investigation identity and round history are immutable")
	}
	for i, round := range old.History {
		if PreparationHash(round) != PreparationHash(next.History[i]) {
			return fmt.Errorf("previous investigation reports cannot be rewritten")
		}
	}
	if len(next.History) > len(old.History) {
		snapshot := *old
		snapshot.History = nil
		if PreparationHash(&snapshot) != PreparationHash(next.History[len(old.History)].Snapshot) {
			return fmt.Errorf("new investigation round must archive the exact previous revision")
		}
		return nil
	}
	if old.ContractHash != next.ContractHash || old.StartedAt != next.StartedAt || old.Deadline != next.Deadline || old.ActionsUsed > next.ActionsUsed || old.OutputBytes > next.OutputBytes || old.ElapsedMS > next.ElapsedMS {
		return fmt.Errorf("investigation inputs and consumed budgets cannot change within a round")
	}
	if len(next.Observations) < len(old.Observations) || len(next.Decisions) < len(old.Decisions) || len(next.DecisionEffects) < len(old.DecisionEffects) ||
		(len(old.Observations) > 0 && !reflect.DeepEqual(old.Observations, next.Observations[:len(old.Observations)])) ||
		(len(old.Decisions) > 0 && !reflect.DeepEqual(old.Decisions, next.Decisions[:len(old.Decisions)])) ||
		(len(old.DecisionEffects) > 0 && !reflect.DeepEqual(old.DecisionEffects, next.DecisionEffects[:len(old.DecisionEffects)])) {
		return fmt.Errorf("investigation receipts and adopted decisions are append-only")
	}
	if old.Report != nil && !reflect.DeepEqual(old.Report, next.Report) {
		return fmt.Errorf("an investigation report is immutable; start a new round for additional evidence")
	}
	return validatePreparationAnalysisTransition(old, next)
}
