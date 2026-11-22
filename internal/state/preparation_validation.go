package state

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
)

func validatePreparationState(st *PreparationState) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	var value any
	_ = json.Unmarshal(raw, &value) // typed marshal above produced valid JSON
	if err := contract.ValidatePreparationStateValue(value); err != nil {
		return err
	}
	if err := validatePreparationRounds(st); err != nil {
		return err
	}
	return validatePreparationCurrent(st)
}

// Schema and flat history are checked once by validatePreparationState. The
// same semantic checks apply to each archived round with its prior evidence.
func validatePreparationCurrent(st *PreparationState) error {
	if !filepath.IsAbs(st.Cwd) || st.ActionsUsed > st.Contract.Limits.MaxActions || st.OutputBytes > st.Contract.Limits.MaxOutputBytes {
		return fmt.Errorf("invalid preparation identity or budget")
	}
	contractJSON, _ := json.Marshal(st.Contract)
	if _, err := contract.ValidatePreparation(contractJSON); err != nil {
		return err
	}
	started, err := time.Parse(time.RFC3339Nano, st.StartedAt)
	if err != nil {
		return err
	}
	deadline, err := time.Parse(time.RFC3339Nano, st.Deadline)
	if err != nil {
		return err
	}
	if !deadline.Equal(started.Add(time.Duration(st.Contract.Limits.MaxSeconds)*time.Second)) || st.ContractHash != PreparationHash(st.Contract) {
		return fmt.Errorf("preparation contract or deadline mismatch")
	}
	seen, actions := map[string]bool{}, map[string]bool{}
	outputBytes := 0
	for _, observation := range st.Observations {
		if observation.Status == "unknown" {
			return fmt.Errorf("unknown investigation execution must remain pending")
		}
		if seen[observation.Invocation.ID] || actions[observation.Invocation.ActionID] {
			return fmt.Errorf("duplicate investigation receipt or action")
		}
		if err := validatePreparationInvocation(st, observation.Invocation, started); err != nil {
			return err
		}
		if err := validatePreparationObservation(observation); err != nil {
			return err
		}
		for _, action := range st.Contract.Actions {
			if action.ID != observation.Invocation.ActionID {
				continue
			}
			e := observation.Execution
			if action.Command != nil || action.HTTP != nil {
				if e == nil || e.Kind != action.Kind || (observation.Status == "observed" && ((action.Command != nil && (e.ExitCode == nil || *e.ExitCode != action.Command.ExpectExit)) || (action.HTTP != nil && e.HTTPStatus != action.HTTP.ExpectStatus))) {
					return fmt.Errorf("execution observation does not match approved action")
				}
			} else if e != nil {
				return fmt.Errorf("file observation cannot carry an external execution result")
			}
		}
		finished, _ := time.Parse(time.RFC3339Nano, observation.FinishedAt)
		if finished.Sub(started).Milliseconds() > st.ElapsedMS || observation.DurationMS > st.ElapsedMS {
			return fmt.Errorf("observation exceeds recorded elapsed time")
		}
		seen[observation.Invocation.ID], actions[observation.Invocation.ActionID] = true, true
		outputBytes += len(observation.Output)
	}
	used := len(st.Observations)
	if st.Pending != nil {
		if seen[st.Pending.ID] || actions[st.Pending.ActionID] {
			return fmt.Errorf("pending invocation repeats a recorded action")
		}
		if err := validatePreparationInvocation(st, *st.Pending, started); err != nil {
			return err
		}
		used++
	}
	if used != st.ActionsUsed || outputBytes != st.OutputBytes {
		return fmt.Errorf("preparation action or output budget does not match durable history")
	}
	if err := validatePreparationDecisions(st, started, actions); err != nil {
		return err
	}
	if err := validatePreparationEffects(st, started); err != nil {
		return err
	}
	if err := validatePreparationAnalysis(st); err != nil {
		return err
	}
	terminalReport := st.Status == "supported" || st.Status == "inconclusive" || st.Status == "access-blocked"
	if terminalReport != (st.Report != nil) || (st.Report != nil && st.Report.Status != st.Status) ||
		((terminalReport || st.Status == "cancelled" || st.Status == "awaiting-report") && (st.Pending != nil || st.DecisionPending != nil)) ||
		(st.Status == "running" && st.Pending == nil) || (st.Pending != nil && st.Status != "running" && st.Status != "blocked") {
		return fmt.Errorf("preparation status does not match its pending work or report")
	}
	if st.Report != nil {
		return validatePreparationReport(st)
	}
	return nil
}

func validatePreparationInvocation(st *PreparationState, invocation PreparationInvocation, started time.Time) error {
	at, err := time.Parse(time.RFC3339Nano, invocation.StartedAt)
	if err != nil || at.Before(started) {
		return fmt.Errorf("invalid investigation invocation time")
	}
	for _, action := range st.Contract.Actions {
		if action.ID == invocation.ActionID && invocation.BindingHash == PreparationHash(action) {
			if invocation.DecisionID != "" {
				for _, receipt := range st.Decisions {
					if receipt.DecisionID == invocation.DecisionID {
						if invocation.Adopted && (receipt.Status != "ok" || receipt.ActionID != action.ID) {
							return fmt.Errorf("adopted action lacks a matching successful decision")
						}
						return nil
					}
				}
				return fmt.Errorf("invocation refers to an absent decision")
			}
			if invocation.Adopted {
				return fmt.Errorf("adopted action requires its decision identity")
			}
			return nil
		}
	}
	return fmt.Errorf("invocation is not bound to an approved investigation action")
}

func validatePreparationObservation(observation InvestigationObservation) error {
	started, err := time.Parse(time.RFC3339Nano, observation.Invocation.StartedAt)
	if err != nil {
		return err
	}
	finished, err := time.Parse(time.RFC3339Nano, observation.FinishedAt)
	if err != nil || finished.Before(started) || observation.DurationMS < 0 {
		return fmt.Errorf("invalid investigation observation time")
	}
	if observation.Status == "observed" {
		if len(observation.ContentHash) != 64 || observation.Error != "" {
			return fmt.Errorf("successful observation requires content identity and no error")
		}
	} else if (observation.Status != "failed" && observation.Status != "cancelled" && observation.Status != "unknown") || observation.Error == "" {
		return fmt.Errorf("unsuccessful observation requires a failure status and reason")
	}
	if e := observation.Execution; e != nil {
		if e.Kind == "http_read" {
			if e.Workspace != "" || e.ExitCode != nil || e.Signal != "" || e.InputHash != "" || e.Cleanup != "not-created" || observation.Status == "unknown" {
				return fmt.Errorf("inconsistent HTTP observation")
			}
		} else if e.Kind != "run_command" || (e.Workspace != "" && !filepath.IsAbs(e.Workspace)) {
			return fmt.Errorf("invalid command observation")
		}
		if observation.Status == "observed" && (e.TimedOut || e.Cancelled || e.Signal != "" || (e.Kind == "run_command" && (e.Cleanup != "removed" || e.ExitCode == nil || e.InputHash == "")) || (e.Kind == "http_read" && (e.HTTPStatus < 200 || e.HTTPStatus > 299))) {
			return fmt.Errorf("successful observation lacks a reliable execution result")
		}
		if observation.Status == "unknown" && (e.Kind != "run_command" || e.Cleanup != "retained" || e.Workspace == "") {
			return fmt.Errorf("unknown command must retain its workspace")
		}
	} else if observation.Status == "unknown" {
		return fmt.Errorf("unknown observation requires its execution record")
	}
	return nil
}

func validatePreparationDecisions(st *PreparationState, started time.Time, usedActions map[string]bool) error {
	seen := map[string]bool{}
	dispatched := 0
	for _, receipt := range st.Decisions {
		if seen[receipt.DecisionID] {
			return fmt.Errorf("duplicate investigation decision")
		}
		seen[receipt.DecisionID] = true
		if st.Contract.Decision != nil && (receipt.Backend == "subscription") != (st.Contract.Decision.Backend == "subscription") {
			return fmt.Errorf("investigation receipt backend contradicts its frozen target")
		}
		if receipt.Status == "ok" {
			if st.Contract.Decision == nil || receipt.Model != st.Contract.Decision.Model ||
				receipt.PolicyHash != PreparationHash(st.Contract.Decision.Policy) || receipt.SnapshotHash == "" || receipt.TargetHash == "" || receipt.QuestionHash == "" {
				return fmt.Errorf("successful decision lacks its frozen policy, target or request identity")
			}
			found := knownCollectionControl(st, receipt.ActionID)
			for _, action := range st.Contract.Actions {
				found = found || action.ID == receipt.ActionID
			}
			if !found {
				return fmt.Errorf("decision selected an unapproved action")
			}
		}
		if receipt.DurationMS > st.ElapsedMS {
			return fmt.Errorf("decision duration exceeds recorded elapsed time")
		}
		if receipt.Status != "skipped" {
			dispatched++
		}
	}
	if st.DecisionPending != nil {
		pending := st.DecisionPending
		if st.Pending != nil || seen[pending.ID] || st.Contract.Decision == nil ||
			pending.Identity.TaskID != st.Contract.ID || pending.Identity.RunID != st.RunID ||
			pending.Identity.Revision > st.Revision || pending.Identity.ContractHash != st.ContractHash ||
			pending.Identity.ObservationHash != PreparationHash(st.Observations) {
			return fmt.Errorf("pending decision does not match the current preparation")
		}
		if err := decision.Validate(*pending, st.Contract.Decision.Policy); err != nil {
			return err
		}
		for _, action := range pending.Actions {
			found := false
			for _, allowed := range st.Contract.Actions {
				if action.ID == allowed.ID && !usedActions[action.ID] && action.BindingHash == PreparationHash(allowed) {
					found = true
				}
			}
			for _, control := range InvestigationControls(st, pending.Identity.CandidateHash) {
				if action.ID == control.ActionID && action.Kind == decision.Diagnostic && action.BindingHash == PreparationHash(control) {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("pending decision contains an unavailable action")
			}
		}
		seen[pending.ID] = true
		// A persisted decision intent may already have reached the service.
		// Reserve its worst-case call until a trustworthy skipped receipt exists.
		dispatched++
	}
	if (len(seen) > 0 && st.Contract.Decision == nil) || (st.Contract.Decision != nil && dispatched > st.Contract.Decision.Policy.MaxCalls) || len(seen) != len(st.DecisionStartedAt) {
		return fmt.Errorf("investigation decision budget or dispatch times do not match history")
	}
	for id, at := range st.DecisionStartedAt {
		when, err := time.Parse(time.RFC3339Nano, at)
		if !seen[id] || err != nil || when.Before(started) {
			return fmt.Errorf("invalid investigation decision dispatch time")
		}
	}
	return nil
}

func validatePreparationReport(st *PreparationState) error {
	if PreparationCleanupUnknown(st) {
		return fmt.Errorf("report cannot discard unknown native decision cleanup")
	}
	report := st.Report
	if strings.TrimSpace(report.Summary) == "" || len(report.Findings) != len(st.Contract.Hypotheses) {
		return fmt.Errorf("report must cover the declared hypotheses")
	}
	hypotheses, observations := map[string]bool{}, map[string]InvestigationObservation{}
	for _, h := range st.Contract.Hypotheses {
		hypotheses[h.ID] = true
	}
	for _, round := range PreparationRounds(st) {
		for _, o := range round.Observations {
			observations[o.Invocation.ID] = o
		}
	}
	supported := false
	for _, finding := range report.Findings {
		if !hypotheses[finding.Hypothesis] || (finding.Status != "unknown" && len(finding.Evidence) == 0) {
			return fmt.Errorf("report finding lacks its hypothesis or evidence")
		}
		delete(hypotheses, finding.Hypothesis)
		for _, id := range finding.Evidence {
			if observation, ok := observations[id]; !ok || observation.Status != "observed" {
				return fmt.Errorf("report cites an absent or unsuccessful observation")
			}
		}
		supported = supported || finding.Status == "supported"
	}
	if (report.Status == "supported" && !supported) || ((report.Status != "supported" || len(st.DecisionEffects) > 0) && len(report.Limitations) == 0) {
		return fmt.Errorf("report conclusion lacks support or explicit limitations")
	}
	return nil
}
