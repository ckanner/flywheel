package machine

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
)

// MaxExecutionGraceRounds caps convergence-driven continuation rounds above a
// packet's base attempt limit. Strictly converging loops continue up to this
// explicit safety ceiling; stalled or oscillating loops stop immediately.
const MaxExecutionGraceRounds = 3

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

// ConvergenceInput is the unified input consumed by EvaluateConvergence across
// both Planning and Execution review loops.
type ConvergenceInput struct {
	History                []ConvergenceRound
	GraceUsed              int
	MaxGrace               int
	PriorNonReviewFailures int
}

// ConvergenceDecision is the deterministic verdict returned by the unified
// convergence kernel across both Planning and Execution loops.
type ConvergenceDecision struct {
	AllowGrace bool
	Stalled    bool
	Policy     string
	Reason     string
}

// EvaluateConvergence determines whether a review loop that reached its current
// attempt/round limit is strictly converging (eligible for bounded continuation
// up to MaxGrace) or stalled/oscillating (must circuit-break immediately).
func EvaluateConvergence(in ConvergenceInput) ConvergenceDecision {
	if in.GraceUsed >= in.MaxGrace {
		return ConvergenceDecision{Reason: fmt.Sprintf("safety ceiling reached (%d/%d)", in.GraceUsed, in.MaxGrace)}
	}
	if len(in.History) == 1 && in.PriorNonReviewFailures > 0 && in.History[0].openTotal() > 0 {
		return ConvergenceDecision{
			AllowGrace: true,
			Policy:     "first-review-grace",
			Reason:     fmt.Sprintf("first review after %d prior non-review retries (%d open findings)", in.PriorNonReviewFailures, in.History[0].openTotal()),
		}
	}
	if len(in.History) < 2 {
		return ConvergenceDecision{Reason: "insufficient review history to prove convergence"}
	}
	first := in.History[0]
	prev := in.History[len(in.History)-2]
	curr := in.History[len(in.History)-1]
	currOpen := curr.openTotal()
	prevOpen := prev.openTotal()
	firstOpen := first.openTotal()

	// Case A: strictly decreasing open blocker/major count (no arbitrary cliff;
	// bounded explicitly by MaxGrace safety ceiling).
	if currOpen > 0 && currOpen < prevOpen {
		return ConvergenceDecision{
			AllowGrace: true,
			Policy:     "convergence-grace",
			Reason:     fmt.Sprintf("monotonically converging (first=%d prev=%d now=%d)", firstOpen, prevOpen, currOpen),
		}
	}
	// Case B: all prior open IDs cleared and only 1 new localized finding remains,
	// strictly below the initial round's open count.
	if currOpen == 1 && currOpen < firstOpen && allPriorOpenIDsCleared(prev.OpenIDs, curr.OpenIDs) {
		return ConvergenceDecision{
			AllowGrace: true,
			Policy:     "convergence-grace",
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

func convergenceRoundFromReviewEvidence(ev any) (ConvergenceRound, bool) {
	m, ok := ev.(map[string]any)
	if !ok || m["verdict"] != "return" {
		return ConvergenceRound{}, false
	}
	raw, ok := m["findings"]
	if !ok || raw == nil {
		return ConvergenceRound{}, true
	}
	b, _ := json.Marshal(raw)
	var findings []prompts.ReviewFinding
	if json.Unmarshal(b, &findings) != nil {
		return ConvergenceRound{}, true
	}
	var round ConvergenceRound
	for _, f := range findings {
		if f.Status == "resolved" {
			round.ResolvedIDs = append(round.ResolvedIDs, f.ID)
			continue
		}
		if f.Severity == "blocker" {
			round.OpenBlockers++
			round.OpenIDs = append(round.OpenIDs, f.ID)
		} else if f.Severity != "minor" {
			round.OpenMajors++
			round.OpenIDs = append(round.OpenIDs, f.ID)
		}
	}
	sort.Strings(round.OpenIDs)
	sort.Strings(round.ResolvedIDs)
	return round, true
}

// executionConvergenceHistory collects ordered return-review rounds for the
// packet's current execution epoch (bounded to reviewReturnCount so human-triage
// or replan resets start a clean convergence trajectory).
func executionConvergenceHistory(ps *state.PacketState) []ConvergenceRound {
	var rounds []ConvergenceRound
	for _, rawEntry := range ps.EvidenceHistory {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			continue
		}
		var rev any
		switch ev := entry["evidence"].(type) {
		case state.Evidence:
			rev = ev.Review
		case map[string]any:
			rev = ev["review"]
		}
		if r, ok := convergenceRoundFromReviewEvidence(rev); ok {
			rounds = append(rounds, r)
		}
	}
	if r, ok := convergenceRoundFromReviewEvidence(ps.Evidence.Review); ok {
		rounds = append(rounds, r)
	}
	if revCount := reviewReturnCount(ps); revCount > 0 && len(rounds) > revCount {
		rounds = rounds[len(rounds)-revCount:]
	}
	return rounds
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
