package investigation

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/diagnostic"
	"github.com/ckanner/flywheel/internal/metrics"
	"github.com/ckanner/flywheel/internal/state"
)

type Options struct {
	Now              func() time.Time
	LookupEnv        func(string) (string, bool)
	ExpectedRevision int64
}

// Isolate OS-specific relative-directory resolution failures without changing
// the process-wide working directory in tests.
var absoluteInvestigationPath = filepath.Abs

// Keep cancellation at configuration-discovery boundaries reproducible without
// changing process-wide directories or invoking a real service in tests.
var loadInvestigationConfiguration = diagnostic.LoadConfigurationContext

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func Start(dir, cwd string, raw []byte, o Options) (*state.PreparationState, error) {
	p, err := contract.ValidatePreparation(raw)
	if err != nil {
		return nil, err
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return nil, err
	}
	cwd, err = absoluteInvestigationPath(cwd)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	release, err := state.AcquirePreparationLease(dir)
	if err != nil {
		return nil, err
	}
	defer release()
	if state.Exists(dir) {
		return nil, fmt.Errorf("implementation task already exists; do not replace it with a preparation")
	}
	global, err := loadInvestigationConfiguration(context.Background(), cwd, dir)
	if err != nil {
		return nil, err
	}
	// The master switch is the only runtime opt-in. Historical per-contract
	// modes cannot enable a service or turn the normal path back into shadow.
	p.Decision = nil
	jevStatus := "disabled"
	if j := global.JEV; j.Enabled {
		p.Decision = &contract.InvestigationDecision{Policy: j.DecisionPolicy(), Endpoint: j.Endpoint, Model: j.Model, KeyEnv: j.KeyEnv, TimeoutMS: j.TimeoutMS, HardBudget: global.Budget.Enforce == "hard"}
		jevStatus = "enabled"
	}
	now := o.now()
	st := &state.PreparationState{Version: 1, RunID: rand.Text(), Cwd: cwd, Contract: *p, ContractHash: state.PreparationHash(p), Status: "ready", Reason: "approved-investigation-actions", StartedAt: now.Format(time.RFC3339Nano), Deadline: now.Add(time.Duration(p.Limits.MaxSeconds) * time.Second).Format(time.RFC3339Nano), Observations: []state.InvestigationObservation{}, Decisions: []decision.Receipt{}, JEVStatus: jevStatus}
	return state.SavePreparation(dir, st)
}

func loadOwned(dir string, o Options) (*state.PreparationState, func(), error) {
	release, err := state.AcquirePreparationLease(dir)
	if err != nil {
		return nil, nil, err
	}
	st, err := state.LoadPreparation(dir)
	if err == nil && o.ExpectedRevision > 0 && st.Revision != o.ExpectedRevision {
		err = fmt.Errorf("stale preparation revision: expected %d, found %d", o.ExpectedRevision, st.Revision)
	}
	if err != nil {
		release()
		return nil, nil, err
	}
	return st, release, nil
}

func persist(dir string, st **state.PreparationState, o Options) error {
	start, _ := time.Parse(time.RFC3339Nano, (*st).StartedAt)
	(*st).ElapsedMS = max((*st).ElapsedMS, o.now().Sub(start).Milliseconds())
	next, err := state.SavePreparation(dir, *st)
	if err == nil {
		*st = next
	}
	return err
}

func terminal(st *state.PreparationState) bool { return st.Report != nil || st.Status == "cancelled" }

// Run owns a finite action loop. A persisted dispatch with no trustworthy
// receipt blocks; restarting never repeats an unknown action or resets budget.
func Run(ctx context.Context, dir string, o Options) (*state.PreparationState, error) {
	st, release, err := loadOwned(dir, o)
	if err != nil {
		return nil, err
	}
	defer release()
	if terminal(st) || (st.Status == "awaiting-report" && st.Reason == "jev-evidence-ready-for-report") {
		return st, nil
	}
	deadline, _ := time.Parse(time.RFC3339Nano, st.Deadline)
	ctx, cancel := context.WithTimeout(ctx, max(time.Duration(0), deadline.Sub(o.now())))
	defer cancel()
	activePolicy(ctx, dir, st)
	if st.Pending != nil {
		receipt, err := state.ReadPreparationReceipt(dir, *st.Pending)
		if err != nil {
			st.Status, st.Reason = "blocked", "action-outcome-unknown: "+err.Error()
			return st, persist(dir, &st, o)
		}
		if receipt.Status == "unknown" {
			st.Status, st.Reason = "blocked", "action-cleanup-unknown: "+receipt.Error+"; retained workspace: "+receipt.Execution.Workspace
			return st, persist(dir, &st, o)
		}
		consume(st, *receipt)
		if err := persist(dir, &st, o); err != nil {
			return st, err
		}
	}
	for {
		if ctx.Err() != nil {
			abandonDecision(dir, st)
			st.Status, st.Reason = "awaiting-report", "time-budget-exhausted"
			if ctx.Err() == context.Canceled {
				st.Status, st.Reason = "cancelled", "cancelled"
			}
			return st, persist(dir, &st, o)
		}
		legal := remainingActions(st)
		if len(legal) == 0 || st.ActionsUsed >= st.Contract.Limits.MaxActions || st.OutputBytes >= st.Contract.Limits.MaxOutputBytes {
			abandonDecision(dir, st)
			st.Status, st.Reason = "awaiting-report", "actions-complete-or-budget-exhausted"
			return st, persist(dir, &st, o)
		}
		identity, err := candidate(ctx, st)
		if err != nil {
			continue // candidate only fails when this owned context is cancelled
		}
		priorDecisions := len(st.Decisions)
		selected, selection, err := choose(ctx, dir, &st, legal, identity, o)
		if err != nil {
			return st, err
		}
		// A cancellation during optional advice cannot dispatch another action.
		if ctx.Err() != nil {
			continue
		}
		current, err := candidate(ctx, st)
		if err != nil {
			continue
		}
		if current != identity {
			selected, selection = legal[0], decision.Selection{ActionID: legal[0].ID, Reason: "candidate-changed-baseline"}
		}
		if selection.Adopted {
			active, hard := activePolicy(ctx, dir, st)
			if !active || hard {
				selected, selection = legal[0], decision.Selection{ActionID: legal[0].ID, Reason: "jev-disabled-before-adoption"}
			}
		}
		if ctx.Err() != nil {
			continue
		}
		if selection.Adopted && (selected.Kind == "finish" || selected.Kind == "defer") {
			for _, control := range state.InvestigationControls(st, current) {
				if control.ActionID == selected.ID {
					st.DecisionEffects = append(st.DecisionEffects, state.InvestigationDecisionEffect{InvestigationControl: control, DecisionID: st.Decisions[len(st.Decisions)-1].DecisionID, Observations: len(st.Observations), BindingHash: state.PreparationHash(control), CandidateHash: current, At: o.now().Format(time.RFC3339Nano)})
				}
			}
			st.Status, st.Reason = "ready", "jev-evidence-backed-defer"
			if selected.Kind == "finish" {
				st.Status, st.Reason = "awaiting-report", "jev-evidence-ready-for-report"
			}
			if err := persist(dir, &st, o); err != nil {
				return st, err
			}
			if selected.Kind == "finish" {
				return st, nil
			}
			continue
		}
		invocationID := selection.InvocationID
		if invocationID == "" {
			invocationID = state.PreparationHash([]any{st.RunID, st.ActionsUsed, st.Revision, selected})
		}
		st.Pending = &state.PreparationInvocation{ID: invocationID, ActionID: selected.ID, BindingHash: state.PreparationHash(selected), CandidateHash: current, StartedAt: o.now().Format(time.RFC3339Nano), Adopted: selection.Adopted, Reason: selection.Reason}
		if len(st.Decisions) > priorDecisions {
			st.Pending.DecisionID = st.Decisions[len(st.Decisions)-1].DecisionID
		}
		st.ActionsUsed++
		st.Status, st.Reason = "running", "action-intent-persisted"
		if err := persist(dir, &st, o); err != nil {
			return st, err
		}
		started := o.now()
		phase := metrics.Phase{ID: invocationID, Run: st.RunID, Name: "investigation-action", Scope: selected.ID, InvocationID: invocationID, StartedAt: started}
		metricsFile := filepath.Join(dir, "metrics.jsonl")
		if err := metrics.BeginPhase(metricsFile, phase); err != nil {
			return st, err
		}
		result := performAction(ctx, st.Cwd, selected, st.Contract.Limits, st.Contract.Limits.MaxOutputBytes-st.OutputBytes, o)
		if result.Execution != nil && ctx.Err() == nil {
			after, err := candidate(ctx, st)
			if err != nil || after != current {
				result.Err = fmt.Errorf("investigation inputs changed during the action")
			}
		}
		receipt := state.InvestigationObservation{Invocation: *st.Pending, Status: "observed", Output: result.Output, ContentHash: result.ContentHash, Truncated: result.Truncated, Execution: result.Execution, FinishedAt: o.now().Format(time.RFC3339Nano), DurationMS: max(int64(0), o.now().Sub(started).Milliseconds())}
		if result.Err != nil {
			receipt.Status, receipt.Error = "failed", result.Err.Error()
		}
		if ctx.Err() != nil {
			receipt.Status, receipt.Error = "cancelled", ctx.Err().Error()
		}
		if result.Unknown {
			receipt.Status, receipt.Error = "unknown", result.Err.Error()
		}
		if err := state.SavePreparationReceipt(dir, receipt); err != nil {
			return st, err
		}
		ended := o.now()
		phase.EndedAt = &ended
		if err := metrics.EndPhase(metricsFile, phase); err != nil {
			return st, err
		}
		if receipt.Status == "unknown" {
			st.Status, st.Reason = "blocked", "action-cleanup-unknown: "+receipt.Error+"; retained workspace: "+receipt.Execution.Workspace
			return st, persist(dir, &st, o)
		}
		consume(st, receipt)
		if err := persist(dir, &st, o); err != nil {
			return st, err
		}
	}
}

func consume(st *state.PreparationState, receipt state.InvestigationObservation) {
	st.Observations = append(st.Observations, receipt)
	st.OutputBytes += len(receipt.Output)
	st.Pending = nil
	st.Status, st.Reason = "ready", "observation-recorded"
}

func remainingActions(st *state.PreparationState) []contract.InvestigationAction {
	done := map[string]bool{}
	for _, receipt := range st.Observations {
		done[receipt.Invocation.ActionID] = true
	}
	for _, effect := range st.DecisionEffects {
		for _, id := range effect.Actions {
			done[id] = true
		}
	}
	legal := []contract.InvestigationAction{}
	for _, action := range st.Contract.Actions {
		if !done[action.ID] {
			legal = append(legal, action)
		}
	}
	return legal
}

func snapshot(st *state.PreparationState, legal []contract.InvestigationAction, candidateHash string) decision.Snapshot {
	s := decision.Snapshot{Version: 1, ID: fmt.Sprintf("investigate-%d-%d", st.ActionsUsed, st.Revision+1), Identity: decision.Identity{TaskID: st.Contract.ID, RunID: st.RunID, Revision: st.Revision + 1, ContractHash: st.ContractHash, CandidateHash: candidateHash, ObservationHash: state.PreparationHash(st.Observations)}, BaselineID: legal[0].ID, SemanticUncertainty: len(st.Contract.Hypotheses) > 1, Facts: []decision.Fact{{Category: decision.TaskSummary, Text: st.Contract.Goal}}}
	for _, h := range st.Contract.Hypotheses {
		s.Facts = append(s.Facts, decision.Fact{Category: decision.DiagnosticFacts, Text: "Hypothesis " + h.ID + ": " + h.Claim + "; falsifier: " + h.Falsifier})
	}
	for _, action := range legal {
		s.Actions = append(s.Actions, decision.Action{ID: action.ID, Kind: decision.Diagnostic, Description: action.Purpose, BindingHash: state.PreparationHash(action)})
	}
	for _, control := range state.InvestigationControls(st, candidateHash) {
		if len(s.Actions) < 16 {
			description := "Defer the listed remaining read because cited observations already address its hypotheses; this is not a contradicted finding."
			if control.Kind == "finish" {
				description = "Stop evidence collection and ask the analyst for a report if the cited observations sufficiently address the hypotheses. This does not prove a root cause or complete a repair."
			}
			description += " Actions: " + strings.Join(control.Actions, ",") + "; evidence: " + strings.Join(control.Evidence, ",")
			s.Actions = append(s.Actions, decision.Action{ID: control.ActionID, Kind: decision.Diagnostic, Description: excerpt(description, 1024), BindingHash: state.PreparationHash(control)})
			s.SemanticUncertainty = true
		}
	}
	for _, observation := range st.Observations {
		if e := observation.Execution; e != nil {
			result := "unknown"
			if e.ExitCode != nil {
				result = fmt.Sprintf("exit=%d", *e.ExitCode)
			}
			if e.Kind == "http_read" {
				result = fmt.Sprintf("http-status=%d", e.HTTPStatus)
			}
			s.Facts = append(s.Facts, decision.Fact{Category: decision.DiagnosticFacts, Text: fmt.Sprintf("Action %s: status=%s; %s; timed-out=%t; cancelled=%t; truncated=%t; cleanup=%s; duration-ms=%d. This is an execution fact, not a proved hypothesis.", observation.Invocation.ActionID, observation.Status, result, e.TimedOut, e.Cancelled, observation.Truncated, e.Cleanup, observation.DurationMS)})
			if st.Contract.Decision == nil || !slices.Contains(st.Contract.Decision.Policy.AllowedData, decision.Logs) {
				s.MissingFacts = append(s.MissingFacts, "Output for action "+observation.Invocation.ActionID+" is withheld because logs are not authorized.")
				continue
			}
		}
		text := excerpt(observation.Output, 1024)
		if text != "" {
			hypotheses := ""
			for _, action := range st.Contract.Actions {
				if action.ID == observation.Invocation.ActionID {
					hypotheses = strings.Join(action.Hypotheses, ",")
				}
			}
			category := decision.Source
			if observation.Execution != nil {
				category = decision.Logs
			}
			s.Facts = append(s.Facts, decision.Fact{Category: category, Text: excerpt("Observation "+observation.Invocation.ID+" (action "+observation.Invocation.ActionID+"; hypotheses "+hypotheses+"; status "+observation.Status+"): "+text, 2048)})
		}
	}
	return s
}

func excerpt(text string, limit int) string {
	if len(text) > limit {
		text = text[:limit]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	return text
}

// Read the current master switch before dispatch and adoption, while retaining
// frozen request identity for recovery. A switch never resets a run's journal.
func activePolicy(ctx context.Context, dir string, st *state.PreparationState) (bool, bool) {
	global, err := loadInvestigationConfiguration(ctx, st.Cwd, dir)
	if err != nil {
		st.JEVStatus = "config-unavailable"
		return false, true
	}
	st.JEVStatus = "disabled"
	if !global.JEV.Enabled {
		return false, global.Budget.Enforce == "hard"
	}
	st.JEVStatus = "not-configured"
	if st.Contract.Decision == nil || st.Contract.Decision.Policy.Mode != decision.Adopt {
		return false, global.Budget.Enforce == "hard"
	}
	frozen, current := st.Contract.Decision, global.JEV
	st.JEVStatus = "policy-changed"
	if frozen.Endpoint != current.Endpoint || frozen.Model != current.Model || frozen.KeyEnv != current.KeyEnv || frozen.TimeoutMS != current.TimeoutMS || frozen.Policy.MaxCalls > current.MaxCalls || frozen.Policy.MinApplicability < current.MinApplicability || frozen.Policy.MinChoice < current.MinChoice {
		return false, global.Budget.Enforce == "hard"
	}
	for _, category := range frozen.Policy.AllowedData {
		if !slices.Contains(current.AllowedData, category) {
			return false, global.Budget.Enforce == "hard"
		}
	}
	st.JEVStatus = "enabled"
	return true, global.Budget.Enforce == "hard"
}

func choose(ctx context.Context, dir string, st **state.PreparationState, legal []contract.InvestigationAction, candidateHash string, o Options) (contract.InvestigationAction, decision.Selection, error) {
	baseline := decision.Selection{ActionID: legal[0].ID, Reason: "deterministic-baseline"}
	cfg := (*st).Contract.Decision
	if cfg == nil {
		return legal[0], baseline, nil
	}
	active, hard := activePolicy(ctx, dir, *st)
	if !active && (*st).DecisionPending == nil {
		return legal[0], baseline, nil
	}
	charged := 0
	for _, receipt := range (*st).Decisions {
		if receipt.Status != "skipped" {
			charged++
		}
	}
	if charged >= cfg.Policy.MaxCalls && (*st).DecisionPending == nil {
		baseline.Reason = "decision-call-budget"
		return legal[0], baseline, nil
	}
	options := decision.Options{Endpoint: cfg.Endpoint, Model: cfg.Model, ExpectedModel: cfg.Model, Timeout: time.Duration(cfg.TimeoutMS) * time.Millisecond, HardBudget: cfg.HardBudget || hard, Now: o.Now}
	recovering := (*st).DecisionPending != nil
	journal := filepath.Join(dir, ".investigation", "decisions", (*st).RunID)
	var receipt decision.Receipt
	var err error
	if recovering {
		// Reconciliation uses the frozen target, independently of current config
		// or credentials. An unknown dispatch must never become a fresh request.
		var found bool
		receipt, found, err = decision.Recover(journal, *(*st).DecisionPending, cfg.Policy, options)
		if !found && err == nil {
			receipt = decision.Receipt{Version: 1, DecisionID: (*st).DecisionPending.ID, Status: "unknown", Reason: "reserved-without-terminal"}
		}
	}
	if !recovering {
		s := snapshot(*st, legal, candidateHash)
		if decision.Validate(s, cfg.Policy) != nil {
			// Preparation contracts may contain more context than this bounded
			// decision protocol permits. Optional advice cannot stop valid work.
			baseline.Reason = "decision-snapshot-unavailable"
			return legal[0], baseline, nil
		}
		lookup := o.LookupEnv
		if lookup == nil {
			lookup = os.LookupEnv
		}
		options.APIKey, _ = lookup(cfg.KeyEnv)
		(*st).DecisionPending = &s
		if (*st).DecisionStartedAt == nil {
			(*st).DecisionStartedAt = map[string]string{}
		}
		(*st).DecisionStartedAt[s.ID] = o.now().Format(time.RFC3339Nano)
		if err := persist(dir, st, o); err != nil {
			return legal[0], baseline, err
		}
		receipt, err = decision.Evaluate(ctx, journal, s, cfg.Policy, options)
	}
	s := *(*st).DecisionPending
	if err != nil {
		// Optional advice failure must not reset or repeat a dispatched request.
		receipt.Version, receipt.DecisionID, receipt.Status, receipt.Reason = 1, s.ID, "unknown", "decision-unavailable: "+err.Error()
	}
	current := s
	current.Identity.Revision = (*st).Revision
	current.Identity.CandidateHash = candidateHash
	current.Identity.ObservationHash = state.PreparationHash((*st).Observations)
	selected := baseline
	active, hard = activePolicy(ctx, dir, *st)
	options.HardBudget = options.HardBudget || hard
	if active {
		selected = decision.Select(current, receipt, cfg.Policy, options)
	}
	(*st).Decisions = append((*st).Decisions, receipt)
	(*st).DecisionPending = nil
	// The caller CAS-saves receipt consumption together with the unique action
	// binding. A crash before that CAS leaves the original decision recoverable.
	for _, action := range legal {
		if action.ID == selected.ActionID {
			return action, selected, nil
		}
	}
	if selected.Adopted {
		for _, control := range state.InvestigationControls(*st, candidateHash) {
			if control.ActionID == selected.ActionID {
				return contract.InvestigationAction{ID: control.ActionID, Kind: control.Kind}, selected, nil
			}
		}
	}
	return legal[0], baseline, nil
}

func Finish(dir string, report state.InvestigationReport, o Options) (*state.PreparationState, error) {
	st, release, err := loadOwned(dir, o)
	if err != nil {
		return nil, err
	}
	defer release()
	if terminal(st) || st.Pending != nil {
		return st, fmt.Errorf("terminal or unresolved investigation cannot accept a new report")
	}
	if err := validateReport(st, report); err != nil {
		return st, err
	}
	abandonDecision(dir, st)
	st.Report, st.Status, st.Reason = &report, report.Status, "analyst-report-with-checked-evidence-references"
	err = persist(dir, &st, o)
	return st, err
}

func validateReport(st *state.PreparationState, report state.InvestigationReport) error {
	if (report.Status != "supported" && report.Status != "inconclusive" && report.Status != "access-blocked") || strings.TrimSpace(report.Summary) == "" || len(report.Summary) > 8192 || report.CounterEvidence == "" || report.Recommendation == "" || report.Regression == "" || report.Rollback == "" || len(report.Findings) != len(st.Contract.Hypotheses) {
		return fmt.Errorf("report requires conclusion, all hypotheses, counter-evidence, limitations, recommendation, regression, and rollback")
	}
	observations := map[string]state.InvestigationObservation{}
	for _, observation := range st.Observations {
		observations[observation.Invocation.ID] = observation
	}
	hypotheses := map[string]bool{}
	for _, hypothesis := range st.Contract.Hypotheses {
		hypotheses[hypothesis.ID] = true
	}
	supported := false
	for _, finding := range report.Findings {
		if !hypotheses[finding.Hypothesis] || finding.Reason == "" || (finding.Status != "supported" && finding.Status != "contradicted" && finding.Status != "unknown") {
			return fmt.Errorf("invalid report hypothesis or finding")
		}
		delete(hypotheses, finding.Hypothesis)
		if finding.Status != "unknown" && len(finding.Evidence) == 0 {
			return fmt.Errorf("supported or contradicted hypothesis requires evidence")
		}
		for _, id := range finding.Evidence {
			observation, ok := observations[id]
			if !ok || observation.Status != "observed" {
				return fmt.Errorf("report cites an absent or unsuccessful observation")
			}
		}
		supported = supported || finding.Status == "supported"
	}
	if report.Status == "supported" && !supported || (report.Status != "supported" || len(st.DecisionEffects) > 0) && len(report.Limitations) == 0 {
		return fmt.Errorf("report conclusion lacks support or explicit limitations")
	}
	return nil
}

func Cancel(dir string, o Options) (*state.PreparationState, error) {
	st, release, err := loadOwned(dir, o)
	if err != nil {
		return nil, err
	}
	defer release()
	if st.Pending != nil {
		return st, fmt.Errorf("unresolved invocation must be reconciled before cancellation")
	}
	if !terminal(st) {
		abandonDecision(dir, st)
		st.Status, st.Reason = "cancelled", "user-cancelled"
		err = persist(dir, &st, o)
	}
	return st, err
}

// Cancelling, timing out, or exhausting the action budget still reconciles an
// optional request. A missing journal is unknown, never permission to resend.
func abandonDecision(dir string, st *state.PreparationState) {
	if st.DecisionPending == nil {
		return
	}
	snapshot := *st.DecisionPending
	cfg := st.Contract.Decision // validated whenever a pending decision is loaded
	options := decision.Options{Endpoint: cfg.Endpoint, Model: cfg.Model, ExpectedModel: cfg.Model, Timeout: time.Duration(cfg.TimeoutMS) * time.Millisecond, HardBudget: cfg.HardBudget}
	receipt, found, err := decision.Recover(filepath.Join(dir, ".investigation", "decisions", st.RunID), snapshot, cfg.Policy, options)
	if !found || err != nil {
		receipt = decision.Receipt{Version: 1, DecisionID: snapshot.ID, Status: "unknown", Reason: "abandoned-request-without-trustworthy-terminal"}
	}
	st.Decisions = append(st.Decisions, receipt)
	st.DecisionPending = nil
}
