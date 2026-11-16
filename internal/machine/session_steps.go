package machine

import "github.com/ckanner/flywheel/internal/contract"

// Keep the exact gate run observed by a received verdict. Its replay must not
// replace the original evidence with a new set of commands and timestamps.
func (c *Context) sessionMilestoneGate(milestone contract.Milestone) (gateOutcome, error) {
	if !c.sessionReplay("judge", milestone.ID) && !c.sessionReplay("judge-panel", milestone.ID) {
		if err := c.admitReviewedBoundary(); err != nil {
			return gateOutcome{}, err
		}
		return c.milestoneGateStep(milestone), nil
	}
	gate, ok := c.State.Milestones[milestone.ID].Gate.(map[string]any)
	identity := evidenceIdentity(gate)
	candidate, err := c.candidateFingerprint(c.Cwd)
	if !ok || gate["pass"] != true || identity == "" || err != nil || candidate != identity {
		return gateOutcome{}, c.sessionBlock(sessionKey("judge", milestone.ID), "recorded Judge result no longer has its original passing milestone gate")
	}
	return gateOutcome{Pass: true}, nil
}
