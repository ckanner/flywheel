package drivers

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// NativeAdmission is an orchestrator-owned identity, never model-authored tool
// arguments. The runtime must attach actual root/child IDs to this invocation.
type NativeAdmission struct {
	Version      int    `json:"version"`
	RunID        string `json:"runId"`
	InvocationID string `json:"invocationId"`
	Role         string `json:"role"`
	Scope        string `json:"scope"`
	CandidateID  string `json:"candidateId,omitempty"`
	Channel      string `json:"channel"`
	TaskDir      string `json:"taskDir"`
}

// The adapter must acknowledge this exact requested capability set. A known
// startup rejection may precede admission; successful dispatch never may.
func validateNativeReceipt(requested, raw json.RawMessage, success, cleanup bool) error {
	if len(requested) == 0 {
		if len(raw) != 0 && string(raw) != "null" {
			return fmt.Errorf("unrequested native capability receipt")
		}
		return nil
	}
	if len(raw) == 0 || string(raw) == "null" {
		if success {
			return fmt.Errorf("native invocation completed without capability evidence")
		}
		return nil
	}
	var request any
	var receipt struct {
		Version   int `json:"version"`
		Requested any `json:"requested"`
		Effective *struct {
			Matched bool `json:"matched"`
		} `json:"effective"`
		Cleanup *struct {
			Confirmed bool `json:"confirmed"`
		} `json:"cleanup"`
		Completion *struct {
			Confirmed  bool     `json:"confirmed"`
			PendingIDs []string `json:"pendingIds"`
		} `json:"completion"`
	}
	if json.Unmarshal(requested, &request) != nil || json.Unmarshal(raw, &receipt) != nil || receipt.Version != 1 ||
		!reflect.DeepEqual(request, receipt.Requested) || receipt.Cleanup == nil || receipt.Cleanup.Confirmed != cleanup ||
		(success && (receipt.Effective == nil || !receipt.Effective.Matched || receipt.Completion == nil || !receipt.Completion.Confirmed || len(receipt.Completion.PendingIDs) != 0)) {
		return fmt.Errorf("native receipt identity, effective configuration or cleanup mismatch")
	}
	return nil
}

// LCU's public close drains its bounded turn stop and native process exit. The
// outer watchdog must leave that grace available, without resetting wall time.
func dshCleanupGrace(a DSHArgs) int64 {
	var native struct {
		LCU struct {
			Enabled bool `json:"enabled"`
		} `json:"lcu"`
	}
	_ = json.Unmarshal(a.Native, &native)
	if native.LCU.Enabled {
		return max(a.Watchdog.KillGraceMs, 145000)
	}
	if len(a.Native) != 0 {
		return max(a.Watchdog.KillGraceMs, 20000)
	}
	return a.Watchdog.KillGraceMs
}
