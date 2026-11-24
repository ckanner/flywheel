package decision

import "fmt"

// ValidateReceiptIdentity verifies a stored terminal against its frozen request
// without network or journal access. It does not authorize adoption: the owner
// must still use Select and recheck its current candidate and action binding.
func ValidateReceiptIdentity(s Snapshot, p Policy, o Options, r Receipt) error {
	if err := Validate(s, p); err != nil {
		return err
	}
	if !validTarget(o) {
		return fmt.Errorf("invalid frozen decision target")
	}
	if err := validateStoredReceipt(r, receiptFor(s, p, o)); err != nil {
		return err
	}
	if !validText(r.Reason, 512) || r.HTTPStatus < 0 || r.HTTPStatus > 599 || r.Native != nil && (r.Backend != "subscription" || !validNative(r.Native)) {
		return fmt.Errorf("invalid decision terminal metadata")
	}
	if r.Status == "ok" && (!successfulTransport(r, o) || !matchesExpectedModel(r.Model, o.ExpectedModel) || (o.Backend != "subscription" && (r.Usage.Input == nil || r.Usage.Output == nil)) || !validAnswer(s, r)) {
		return fmt.Errorf("successful decision receipt lacks a valid answer")
	}
	if r.Status == "skipped" && (r.Native != nil || r.HTTPStatus != 0 || r.Model != "" || r.ActionID != "" || r.Usage.Input == nil || r.Usage.Output == nil || *r.Usage.Input != 0 || *r.Usage.Output != 0) {
		return fmt.Errorf("skipped decision receipt cannot claim dispatch or usage")
	}
	return nil
}

func validateStoredReceipt(actual, expected Receipt) error {
	if actual.Backend != expected.Backend {
		return fmt.Errorf("decision backend changed")
	}
	if actual.Version != expected.Version || actual.DecisionID != expected.DecisionID || actual.SnapshotHash != expected.SnapshotHash || actual.PolicyHash != expected.PolicyHash || actual.TargetHash != expected.TargetHash || actual.QuestionHash != expected.QuestionHash || actual.DurationMS < 0 || (actual.Usage.Input != nil && *actual.Usage.Input < 0) || (actual.Usage.Output != nil && *actual.Usage.Output < 0) || (actual.Status != "ok" && actual.Status != "error" && actual.Status != "unknown" && actual.Status != "skipped") {
		return fmt.Errorf("invalid decision receipt identity or terminal")
	}
	return nil
}
