package state

import (
	"fmt"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/planning"
)

// InitPlanning creates a real brief-only task; it does not fabricate a Spec,
// packet, workflow selection or evidence of past review/approval.
func InitPlanning(brief planning.Brief, cfg *config.Config) (*State, error) {
	p, err := planning.New(brief)
	if err != nil {
		return nil, err
	}
	enforce := "off"
	var limit *int64
	if cfg != nil {
		if cfg.Budget.Enforce != "" {
			enforce = cfg.Budget.Enforce
		}
		limit = cfg.Budget.DailyTokenLimit
	}
	return &State{SchemaVersion: 2, Slug: brief.ID, Status: "pending", Planning: p,
		Milestones: map[string]MilestoneState{}, Packets: map[string]*PacketState{}, Inbox: []any{},
		Sessions: &SessionLedger{Version: 4, Records: map[string]*SessionRecord{}},
		Budget: Budget{Enforce: enforce, DailyTokenLimit: limit, Daily: map[string]DailyBucket{},
			Cache: CacheStats{ByProvider: map[string]ProviderCache{}}, Warnings: []string{}}}, nil
}

func validatePlanningState(st *State) error {
	if st.PreparationSource != nil && len(st.PreparationSource.ImplementationScope) == 0 && st.Planning == nil {
		return fmt.Errorf("an investigation source without implementation scope requires reviewed planning authority")
	}
	if st.Planning == nil {
		if st.Sessions != nil && st.Sessions.Version == 4 {
			return fmt.Errorf("version 4 session ledger requires its planning authority")
		}
		return nil
	}
	if st.Sessions == nil || st.Sessions.Version != 4 || st.Slug != st.Planning.Brief.ID {
		return fmt.Errorf("planning task identity and version 4 session ledger are required")
	}
	return planning.Validate(st.Planning)
}
