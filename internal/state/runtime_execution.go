package state

import (
	"fmt"
	"strings"
	"time"
)

// The link is not inferred from a model name or a clock. New invocations name
// their decision explicitly; retained terminal summaries must stay in its next
// attempt. Older records without this optional metadata remain valid.
func validateRuntimeExecutionLinks(st *State, records map[string]*RuntimeDecisionRecord) error {
	seen := map[string]string{}
	for id, r := range records {
		if r.Execution == nil || r.Execution.Invocation == nil {
			continue
		}
		call := r.Execution.Invocation
		_, err := time.Parse(time.RFC3339Nano, call.StartedAt)
		if err != nil || call.Attempt != r.Attempt+1 ||
			(r.Status != "applied" && r.Status != "baseline") || !strings.HasPrefix(r.Action, "model-") || seen[call.ID] != "" {
			return fmt.Errorf("runtime execution summary does not match its settled model decision")
		}
		if _, ok := r.ModelBindings[r.Action]; !ok {
			return fmt.Errorf("runtime execution summary lacks its frozen model binding")
		}
		seen[call.ID] = id
	}
	if st.Sessions == nil {
		return nil
	}
	for _, s := range st.Sessions.Records {
		if s == nil || s.Last == nil || s.Last.DecisionID == "" {
			continue
		}
		r := records[s.Last.DecisionID]
		if r == nil || r.Execution == nil || s.Role != "execute" || s.Scope != r.Packet ||
			s.Last.Attempt != r.Attempt+1 || (r.Status != "applied" && r.Status != "baseline") || !strings.HasPrefix(r.Action, "model-") {
			return fmt.Errorf("session decision link differs from its exact execution scope")
		}
		if _, ok := r.ModelBindings[r.Action]; !ok {
			return fmt.Errorf("session decision link lacks its frozen model binding")
		}
	}
	return nil
}
