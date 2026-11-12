package machine

import (
	"encoding/json"

	"github.com/ckanner/flywheel/internal/circuitbreaker"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/verify"
)

// Only Go-owned complete observations are eligible for early spin detection.
// A model's changing self-report, a larger diff, or fewer log lines is not a
// progress signal. Unknown observations retain the normal attempt/budget bounds.
func (c *Context) failureProgress(packet *contract.Packet, code errs.ErrorCode) circuitbreaker.ProgressEvidence {
	observation := circuitbreaker.ProgressEvidence{Version: 1}
	if code != errL1Failed && code != errReviewReturned {
		return observation
	}
	ps := c.State.Packets[packet.PacketID]
	raw, err := json.Marshal(ps.Evidence.L1)
	if err != nil {
		return observation
	}
	var l1 struct {
		Pass           bool                  `json:"pass"`
		Candidate      string                `json:"candidate"`
		CheckSet       string                `json:"check-set"`
		Environment    string                `json:"environment"`
		Identity       string                `json:"identity"`
		IdentityBefore string                `json:"identity-before"`
		IdentityError  *string               `json:"identity-error"`
		Commands       []verify.GateResult   `json:"commands"`
		Boundary       verify.BoundaryReport `json:"boundary"`
	}
	if json.Unmarshal(raw, &l1) != nil {
		return observation
	}
	observation.Candidate, observation.CheckSet, observation.Environment = l1.Candidate, l1.CheckSet, l1.Environment
	observation.Complete = l1.Identity != "" && l1.Identity == l1.IdentityBefore && l1.IdentityError == nil &&
		!l1.Boundary.Skipped && len(l1.Commands) == len(packet.Acceptance.L1)
	for _, command := range l1.Commands {
		observation.Complete = observation.Complete && command.ObservationComplete
		observation.Checks = append(observation.Checks, circuitbreaker.CheckObservation{
			ID: command.CheckID, Pass: command.Pass, ExitCode: command.ExitCode, Failure: command.OutputDigest,
		})
	}
	// Boundary is a deterministic checker with an explicit ID, not a test case
	// inferred from arbitrary output. Its policy is included in the check set.
	boundary := circuitbreaker.CheckObservation{ID: "boundary/v1", Pass: l1.Boundary.Pass}
	if !boundary.Pass {
		boundary.Failure = sessionHash(l1.Boundary.Violations)
	}
	observation.Checks = append(observation.Checks, boundary)
	if code == errReviewReturned {
		review, ok := ps.Evidence.Review.(map[string]any)
		if !ok || !l1.Pass || review["verdict"] != "return" || review["identity"] != l1.Identity {
			observation.Complete = false
			return observation
		}
		verdict := make(map[string]any, len(review))
		for key, value := range review {
			if key != "at" && key != "identity" {
				verdict[key] = value
			}
		}
		observation.CheckSet = sessionHash([]any{l1.CheckSet, "review/v1", c.Config.Loops["review"]})
		observation.Checks = append(observation.Checks, circuitbreaker.CheckObservation{
			ID: "review/v1", Failure: sessionHash(verdict),
		})
	}
	return observation
}
