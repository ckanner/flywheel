package machine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ckanner/flywheel/internal/planning"
)

// ConvergenceRound captures objective, machine-checkable open/resolved blocking
// finding counts for a single review iteration.
type ConvergenceRound struct {
	OpenBlockers int
	OpenMajors   int
	OpenIDs      []string
	ResolvedIDs  []string
}

func (r ConvergenceRound) openTotal() int {
	return r.OpenBlockers + r.OpenMajors
}

// ConvergenceDecision is the deterministic verdict returned by the unified
// convergence kernel across both Planning and Execution loops.
type ConvergenceDecision struct {
	AllowGrace bool
	Stalled    bool
	Reason     string
}

// EvaluateConvergence determines whether a review loop that reached its base
// round budget is strictly converging (eligible for bounded +1 grace) or
// stalled/oscillating (must stop immediately).
func EvaluateConvergence(history []ConvergenceRound, graceUsed, maxGrace int) ConvergenceDecision {
	if graceUsed >= maxGrace {
		return ConvergenceDecision{Reason: fmt.Sprintf("grace budget exhausted (%d/%d)", graceUsed, maxGrace)}
	}
	if len(history) < 2 {
		return ConvergenceDecision{Reason: "insufficient review history to prove convergence"}
	}
	first := history[0]
	prev := history[len(history)-2]
	curr := history[len(history)-1]
	currOpen := curr.openTotal()
	prevOpen := prev.openTotal()
	firstOpen := first.openTotal()

	// Case A: strictly decreasing open blocker/major count and at most 2 remaining.
	if currOpen > 0 && currOpen < prevOpen && currOpen <= 2 {
		return ConvergenceDecision{
			AllowGrace: true,
			Reason:     fmt.Sprintf("monotonically converging (first=%d prev=%d now=%d)", firstOpen, prevOpen, currOpen),
		}
	}
	// Case B: all prior open IDs cleared and only 1 new localized finding remains,
	// strictly below the initial round's open count.
	if currOpen == 1 && currOpen < firstOpen && allPriorOpenIDsCleared(prev.OpenIDs, curr.OpenIDs) {
		return ConvergenceDecision{
			AllowGrace: true,
			Reason:     fmt.Sprintf("all prior open findings resolved with 1 remaining (first=%d prev=%d now=%d)", firstOpen, prevOpen, currOpen),
		}
	}
	return ConvergenceDecision{
		Stalled: true,
		Reason:  fmt.Sprintf("not converging (first=%d prev=%d now=%d)", firstOpen, prevOpen, currOpen),
	}
}

func allPriorOpenIDsCleared(prevIDs, currIDs []string) bool {
	if len(prevIDs) == 0 {
		return false
	}
	currSet := make(map[string]bool, len(currIDs))
	for _, id := range currIDs {
		currSet[strings.ToLower(strings.TrimSpace(id))] = true
	}
	for _, id := range prevIDs {
		if currSet[strings.ToLower(strings.TrimSpace(id))] {
			return false
		}
	}
	return true
}

func convergenceRoundFromFindings(findings []planning.Finding) ConvergenceRound {
	var round ConvergenceRound
	for _, f := range findings {
		if f.Status == "resolved" {
			round.ResolvedIDs = append(round.ResolvedIDs, f.ID)
			continue
		}
		if f.Severity == "blocker" {
			round.OpenBlockers++
			round.OpenIDs = append(round.OpenIDs, f.ID)
		} else if f.Severity == "major" {
			round.OpenMajors++
			round.OpenIDs = append(round.OpenIDs, f.ID)
		}
	}
	sort.Strings(round.OpenIDs)
	sort.Strings(round.ResolvedIDs)
	return round
}

// planningConvergenceHistory collects ordered revise-review rounds belonging to
// the active planning request's canonical scope and input revision.
func planningConvergenceHistory(p *planning.State, req planning.Request) ([]ConvergenceRound, []string) {
	scope, scopeID := planning.CanonicalScope(req.Scope, req.ScopeID)
	var history []ConvergenceRound
	var lastOpenLabels []string
	var reviews []planning.Review
	if scope == "design" {
		for _, rev := range p.Reviews {
			if (rev.Kind == "design" || rev.Kind == "compact-v1") && rev.InputRevision == req.InputRevision {
				reviews = append(reviews, rev)
			}
		}
	} else {
		planKey := planning.PlanKey(scope, scopeID)
		matchingRefs := map[planning.Ref]bool{}
		for _, plan := range p.Plans {
			if plan.InputRevision == req.InputRevision && planning.PlanKey(plan.Scope, plan.ScopeID) == planKey {
				matchingRefs[plan.Ref] = true
			}
		}
		for _, rev := range p.Reviews {
			if rev.Kind == "plan" && rev.InputRevision == req.InputRevision && matchingRefs[rev.Target] {
				reviews = append(reviews, rev)
			}
		}
	}
	if len(reviews) == 0 {
		return nil, nil
	}
	// Limit to at most the last req.Rounds reviews so earlier completed requests
	// on the same scope do not skew the active request's trajectory.
	if req.Rounds > 0 && len(reviews) > req.Rounds {
		reviews = reviews[len(reviews)-req.Rounds:]
	}
	for _, rev := range reviews {
		if rev.Verdict != "revise" {
			continue
		}
		history = append(history, convergenceRoundFromFindings(rev.Findings))
	}
	latest := reviews[len(reviews)-1]
	for _, f := range latest.Findings {
		if f.Status != "resolved" && (f.Severity == "blocker" || f.Severity == "major") {
			lastOpenLabels = append(lastOpenLabels, fmt.Sprintf("%s(%s): %s", f.ID, f.Severity, f.Target))
		}
	}
	return history, lastOpenLabels
}

// evaluateExecutionConvergenceGrace shares the bounded convergence guard for
// code-review returns in the execution loop.
func evaluateExecutionConvergenceGrace(converging bool, openFindings, failedAttempts, baseMax int, graceUsed bool) bool {
	return converging && openFindings > 0 && openFindings <= 2 && failedAttempts == baseMax && !graceUsed
}
