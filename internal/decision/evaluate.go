package decision

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"
)

func receiptFor(s Snapshot, p Policy, o Options) Receipt {
	return Receipt{Version: 1, DecisionID: s.ID, SnapshotHash: hash(s), PolicyHash: hash(p), TargetHash: targetHash(o), QuestionHash: hash(questions(s)), Status: "unknown", Reason: "reserved-without-terminal"}
}

func skipped(r Receipt, reason string) Receipt {
	zero := int64(0)
	r.Status, r.Reason, r.Usage = "skipped", reason, Usage{Input: &zero, Output: &zero}
	return r
}

func skipReason(s Snapshot, p Policy, o Options) string {
	switch {
	case p.Mode == Off:
		return "disabled"
	case p.Mode == Offline:
		return "offline-no-network"
	case s.ExecutorUnknown:
		return "executor-outcome-unknown"
	case len(s.Actions) < 2:
		return "one-legal-action"
	case !s.SemanticUncertainty:
		return "deterministic-choice"
	case !dataAllowed(s, p):
		return "data-not-authorized"
	case o.HardBudget:
		// System One exposes no enforceable per-request usage ceiling. A byte
		// limit or estimated token count is not an enforceable cost reservation.
		return "hard-budget-without-enforceable-bound"
	}
	return ""
}

// Evaluate records a bounded snapshot and intent before its single HTTP call.
// directory is a stable journal for this run, shared by its decision points:
// exclusive call slots count every dispatched or unknown request toward MaxCalls.
// Off/offline never contact the service; shadow only captures input. A later live
// evaluation must be explicit and use advisory/adopt with separate data opt-in.
func Evaluate(ctx context.Context, directory string, s Snapshot, p Policy, o Options) (Receipt, error) {
	if err := Validate(s, p); err != nil {
		return Receipt{}, err
	}
	r := receiptFor(s, p, o)
	if reason := skipReason(s, p, o); reason != "" {
		return skipped(r, reason), nil
	}
	if ctx == nil || !validTarget(o) || len(request(s, o.Model)) > maxDocument {
		return r, fmt.Errorf("invalid decision context, target, or request bounds")
	}
	input := raw(struct {
		Snapshot Snapshot `json:"snapshot"`
		Policy   Policy   `json:"policy"`
		Target   string   `json:"target-hash"`
	}{s, p, r.TargetHash})
	if o.APIKey != "" && bytes.Contains(input, []byte(o.APIKey)) {
		return r, fmt.Errorf("decision input contains the configured API key")
	}
	if err := ctx.Err(); err != nil {
		return skipped(r, "cancelled-before-dispatch"), nil
	}
	root, err := openJournal(directory)
	if err != nil {
		return r, err
	}
	defer root.Close()
	key := hash(struct{ Task, Run, Decision string }{s.Identity.TaskID, s.Identity.RunID, s.ID})
	if p.Mode == Shadow {
		err := install(root, key+".snapshot.json", input)
		return skipped(r, "snapshot-captured"), err
	}
	if err := install(root, key+".input.json", input); err != nil {
		return r, err
	}
	if previous, exists, err := load(root, key, r); exists || err != nil {
		return previous, err
	}
	if o.APIKey == "" {
		return skipped(r, "missing-api-key"), nil
	}
	if err := exclusive(root, key+".pending", raw(r)); os.IsExist(err) {
		return r, nil
	} else if err != nil {
		return r, err
	}
	if err := syncJournal(root); err != nil {
		return r, err
	}
	owned, err := reserveSlot(root, key, p.MaxCalls)
	if err != nil {
		return r, err
	}
	if !owned {
		r = skipped(r, "decision-call-budget")
	} else {
		now := o.Now
		if now == nil {
			now = time.Now
		}
		started := now()
		call(ctx, s, p, o, &r)
		r.DurationMS = now().Sub(started).Milliseconds()
	}
	return r, publish(root, key+".json", raw(r))
}

// Recover reads only the frozen request's existing terminal or intent. It never
// creates the journal, reserves a call, or sends a request. If an outer engine
// decision intent exists but this returns found=false, the owner must retain
// that uncertainty and continue its baseline; recovery must not call Evaluate
// to guess that dispatch never happened.
func Recover(directory string, s Snapshot, p Policy, o Options) (Receipt, bool, error) {
	if err := Validate(s, p); err != nil {
		return Receipt{}, false, err
	}
	expected := receiptFor(s, p, o)
	root, err := existingJournal(directory)
	if os.IsNotExist(err) {
		return expected, false, nil
	}
	if err != nil {
		return expected, false, err
	}
	defer root.Close()
	key := hash(struct{ Task, Run, Decision string }{s.Identity.TaskID, s.Identity.RunID, s.ID})
	return load(root, key, expected)
}

// Replay evaluates a caller-supplied recorded/fixture native response without
// contacting the service or writing state. It cannot authorize online adoption.
func Replay(s Snapshot, p Policy, o Options, nativeResponse []byte) (Receipt, error) {
	if err := Validate(s, p); err != nil {
		return Receipt{}, err
	}
	if p.Mode != Offline || !validTarget(o) {
		return Receipt{}, fmt.Errorf("replay requires offline mode and a fixed target")
	}
	r := receiptFor(s, p, o)
	if err := parseResponse(nativeResponse, s, o.ExpectedModel, &r); err != nil {
		return r, err
	}
	r.Status, r.Reason = "ok", "offline-recorded-response"
	return r, nil
}
