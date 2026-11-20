package machine

import (
	"encoding/json"
	"path/filepath"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
)

type CollaborationProjection struct {
	Status              string                        `json:"status"`
	Reason              string                        `json:"reason"`
	ContractFingerprint string                        `json:"contractFingerprint,omitempty"`
	Value               *contract.PacketCollaboration `json:"value,omitempty"`
}

// A matching stored fingerprint is necessary but does not manufacture review
// evidence for a legacy task. Nothing here grants execution authority or writes
// state; a changed/missing contract remains unavailable to the reviewed view.
func reviewedCollaboration(st *state.State, id string, location []string) *CollaborationProjection {
	view := &CollaborationProjection{Status: "unverified", Reason: "missing-frozen-contract"}
	if st.ContractFingerprints[id] == "" {
		return view
	}
	view.Reason = "legacy-review-not-recorded"
	if st.Planning == nil {
		return view
	}
	view.Reason = "contract-location-unavailable"
	if len(location) != 2 || !filepath.IsAbs(location[0]) || !filepath.IsAbs(location[1]) {
		return view
	}
	if _, err := contract.MilestoneOfPacket(id); err != nil {
		return view
	}
	taskDir, cwd := location[0], location[1]
	view.Reason = "configuration-unavailable"
	cfg, err := config.LoadReadOnly(taskDir, cwd)
	if err != nil {
		return view
	}
	view.Reason = "contract-unavailable"
	specRaw, specErr := readContractRevision(filepath.Join(taskDir, "spec.md"))
	packetRaw, packetErr := readContractRevision(filepath.Join(taskDir, "task-packet-"+id+".md"))
	if specErr != nil || packetErr != nil {
		return view
	}
	view.Reason = "contract-invalid"
	spec, specErr := contract.ParseSpec(specRaw, "spec.md")
	packet, packetErr := contract.ParsePacket(packetRaw, "task-packet-"+id+".md")
	if specErr != nil || packetErr != nil || packet.PacketID != id {
		return view
	}
	view.Reason = "contract-changed"
	ctx := &Context{Config: cfg, State: st, Spec: spec}
	fingerprint := ctx.packetContractFingerprint(packet, specRaw, packetRaw)
	if fingerprint != st.ContractFingerprints[id] {
		return view
	}
	view.Reason = "review-or-approval-unverified"
	if spec.PlanningRef == nil || planning.Admit(st.Planning, *spec.PlanningRef) != nil {
		return view
	}
	view.Status, view.Reason, view.ContractFingerprint = "absent", "no-collaboration-declared", fingerprint
	if packet.Collaboration != nil {
		raw, _ := json.Marshal(packet.Collaboration)
		if len(raw) > 32<<10 {
			view.Status, view.Reason = "unverified", "display-limit-exceeded"
			return view
		}
		view.Status, view.Reason, view.Value = "verified", "reviewed-contract-matched", packet.Collaboration
	}
	return view
}
