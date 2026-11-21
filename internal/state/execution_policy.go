package state

// ExecutionPolicyRecord is frozen per packet and copied with that packet by the
// parallel scheduler. Configuration changes cannot reinterpret a saved choice.
type ExecutionPolicyRecord struct {
	CurrentBinding        *BindingSnapshot  `json:"currentBinding,omitempty"`        // frozen display metadata, not selection authority
	CurrentBindingOmitted bool              `json:"currentBindingOmitted,omitempty"` // display projection only
	Version               int               `json:"version"`
	Identity              string            `json:"identity"`
	Route                 string            `json:"route"`
	Current               string            `json:"current"`
	Switches              int               `json:"switches"`
	StalledAttempts       int               `json:"stalled-attempts"`
	LastObservedAttempt   int               `json:"last-observed-attempt"`
	History               []ExecutionSwitch `json:"history,omitempty"`
}

type ExecutionSwitch struct {
	Kind    string               `json:"kind"`
	From    string               `json:"from"`
	To      string               `json:"to"`
	Before  SessionConfiguration `json:"before"`
	After   SessionConfiguration `json:"after"`
	Reason  string               `json:"reason"`
	At      string               `json:"at"`
	Attempt int                  `json:"attempt"`
}
