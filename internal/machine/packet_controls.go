package machine

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
)

type EligibilityCondition struct {
	Code    string `json:"code"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

type EligibleExecutionCandidate struct {
	ID         string                  `json:"id"`
	Binding    config.ExecutionBinding `json:"binding"`
	Eligible   bool                    `json:"eligible"`
	Conditions []EligibilityCondition  `json:"conditions"`
}

type PacketEligibility struct {
	Packet     string                       `json:"packet"`
	Revision   int64                        `json:"revision"`
	Status     string                       `json:"status"`
	Reason     string                       `json:"reason"`
	Source     string                       `json:"source"`
	Route      string                       `json:"route,omitempty"`
	Candidates []EligibleExecutionCandidate `json:"candidates"`
}

type CollaborationEditor struct {
	Packet              string                        `json:"packet"`
	Revision            int64                         `json:"revision"`
	ContractFingerprint string                        `json:"contractFingerprint,omitempty"`
	Plan                *planning.Ref                 `json:"plan,omitempty"`
	Editable            bool                          `json:"editable"`
	Reason              string                        `json:"reason"`
	Writes              []string                      `json:"writes"`
	Value               *contract.PacketCollaboration `json:"value,omitempty"`
}

// The read path uses a private state clone: executionPolicy may initialize a
// record while checking identity, but a console read never freezes a choice.
func packetControlContext(st *state.State, id string, location []string) (*Context, *contract.Packet, string, error) {
	if len(location) != 2 || !filepath.IsAbs(location[0]) || !filepath.IsAbs(location[1]) {
		return nil, nil, "", fmt.Errorf("contract-location-unavailable")
	}
	if _, err := contract.MilestoneOfPacket(id); err != nil {
		return nil, nil, "", err
	}
	taskDir, err := canonicalDirectory(location[0])
	if err != nil {
		return nil, nil, "", err
	}
	cwd, err := canonicalDirectory(ProjectRoot(location[1]))
	if err != nil {
		return nil, nil, "", err
	}
	cfg, err := config.LoadReadOnly(taskDir, cwd)
	if err != nil {
		return nil, nil, "", err
	}
	specRaw, err := readContractRevision(filepath.Join(taskDir, "spec.md"))
	if err != nil {
		return nil, nil, "", err
	}
	packetRaw, err := readContractRevision(filepath.Join(taskDir, "task-packet-"+id+".md"))
	if err != nil {
		return nil, nil, "", err
	}
	spec, err := contract.ParseSpec(specRaw, "spec.md")
	if err != nil {
		return nil, nil, "", err
	}
	packet, err := contract.ParsePacket(packetRaw, "task-packet-"+id+".md")
	if err != nil {
		return nil, nil, "", err
	}
	if packet.PacketID != id {
		return nil, nil, "", fmt.Errorf("packet identity differs from the selected node")
	}
	c := &Context{Config: cfg, State: jsonClone(st), Spec: spec, TaskDir: taskDir, Cwd: cwd}
	fingerprint := c.packetContractFingerprint(packet, specRaw, packetRaw)
	if fingerprint != st.ContractFingerprints[id] {
		return nil, nil, "", fmt.Errorf("contract-changed-or-unfrozen")
	}
	return c, packet, fingerprint, nil
}

func packetControlViews(st *state.State, id string, location []string) (*PacketEligibility, *CollaborationEditor, *ImageInputEditor) {
	eligibility := &PacketEligibility{Packet: id, Revision: st.Revision, Status: "unknown", Source: "configuration", Candidates: []EligibleExecutionCandidate{}}
	editor := &CollaborationEditor{Packet: id, Revision: st.Revision, Writes: []string{}}
	images := &ImageInputEditor{Packet: id, Revision: st.Revision, Status: "unknown"}
	defer func() {
		eligibility.Reason, editor.Reason = boundedDecisionText(eligibility.Reason, 1024), boundedDecisionText(editor.Reason, 1024)
		images.Reason = boundedDecisionText(images.Reason, 1024)
		if raw, _ := json.Marshal(eligibility); len(raw) > 48<<10 {
			eligibility.Status, eligibility.Reason, eligibility.Candidates = "unknown", "Execution candidate details exceed the console display limit; inspect the effective configuration.", []EligibleExecutionCandidate{}
		}
		if raw, _ := json.Marshal(editor); len(raw) > 32<<10 {
			editor.Editable, editor.Reason, editor.Writes, editor.Value = false, "Collaboration details exceed the console editor limit; inspect the complete planning contract.", []string{}, nil
		}
	}()
	c, packet, fingerprint, err := packetControlContext(st, id, location)
	if err != nil {
		eligibility.Reason, editor.Reason, images.Reason = err.Error(), err.Error(), err.Error()
		return eligibility, editor, images
	}
	editor = c.collaborationEditor(packet, fingerprint)
	images = c.imageInputEditor(packet, fingerprint)
	eligibility.Route = packet.Route
	if c.State.Packets[id].ExecutionPolicy != nil {
		eligibility.Source = "frozen"
	}
	if c.State.Planning != nil {
		if c.Spec.PlanningRef == nil {
			eligibility.Reason = "reviewed planning reference is missing"
			return eligibility, editor, images
		}
		if err := planning.Admit(c.State.Planning, *c.Spec.PlanningRef); err != nil {
			eligibility.Reason = err.Error()
			return eligibility, editor, images
		}
		if err := c.verifyPlanningProjection(c.State.Planning.Active); err != nil {
			eligibility.Reason = err.Error()
			return eligibility, editor, images
		}
		if err := c.verifyPlanningArtifacts(); err != nil {
			eligibility.Reason = err.Error()
			return eligibility, editor, images
		}
	}
	record, route, err := c.executionPolicy(packet)
	if err != nil {
		eligibility.Reason = err.Error()
		return eligibility, editor, images
	}
	bindings := map[string]config.ExecutionBinding{}
	ids := route.Allowed
	if record != nil {
		bindings = c.Config.ExecutionPolicy.Bindings
	} else {
		b, err := fallback.ResolveBinding(c.Config, "execute", packet.Route)
		if err != nil {
			eligibility.Reason = err.Error()
			return eligibility, editor, images
		}
		ids = []string{"route:" + packet.Route}
		bindings[ids[0]] = config.ExecutionBinding{Harness: b.Harness, Provider: b.Provider, Channel: b.Channel, Model: b.Model, Thinking: b.Thinking, NativeProfile: b.NativeProfile,
			Permission: c.Config.PermissionMode("execute", b.Harness, "routes."+packet.Route), SessionMode: c.Config.Sessions.ModeForRole("execute")}
	}
	for _, id := range ids {
		b := bindings[id]
		candidate := EligibleExecutionCandidate{ID: id, Binding: b, Eligible: true, Conditions: c.executionCandidateConditions(packet, b)}
		for _, check := range candidate.Conditions {
			candidate.Eligible = candidate.Eligible && check.Passed
		}
		eligibility.Candidates = append(eligibility.Candidates, candidate)
	}
	eligibility.Status, eligibility.Reason = "verified", "Current packet and effective execution configuration checked; account entitlement and model execution were not tested."
	return eligibility, editor, images
}

func (c *Context) collaborationEditor(packet *contract.Packet, fingerprint string) *CollaborationEditor {
	v := &CollaborationEditor{Packet: packet.PacketID, Revision: c.State.Revision, ContractFingerprint: fingerprint, Writes: []string{}, Value: packet.Collaboration}
	if packet.Schedule != nil {
		v.Writes = append(v.Writes, packet.Schedule.Writes...)
	}
	latest, draft, reason := c.packetEditPlan(packet)
	if latest != nil {
		v.Plan = &latest.Ref
	}
	if draft != nil {
		v.Value = draft.Collaboration
	}
	v.Editable, v.Reason = reason == "", reason
	if v.Editable && len(v.Writes) == 0 {
		v.Editable, v.Reason = false, "the reviewed packet must declare schedule.writes first"
	}
	if v.Editable {
		v.Reason = "Saving creates an immutable plan revision for independent review; implementation is not started."
	}
	return v
}

// Both packet editors use the same exact reviewed planning authority. This
// checks contracts and immutable exports only; it never opens image inputs.
func (c *Context) packetEditPlan(packet *contract.Packet) (*planning.ExecutionPlan, *contract.Packet, string) {
	p := c.State.Planning
	if p == nil {
		return nil, nil, "reviewed planning is required before editing a packet"
	}
	var latest *planning.ExecutionPlan
	for _, ref := range p.CurrentPlans {
		plan := planning.FindPlan(p, ref)
		if plan != nil && (latest == nil || plan.Ref.Revision > latest.Ref.Revision) {
			latest = plan
		}
	}
	if latest == nil {
		return nil, nil, "a current complete planning projection is required before editing a packet"
	}
	raw := latest.Files["task-packet-"+packet.PacketID+".md"]
	if raw == "" {
		return latest, nil, "the packet is absent from the current complete planning projection"
	}
	draft, err := contract.ParsePacket([]byte(raw), "packet edit draft")
	if err != nil {
		return latest, nil, "the current planning packet cannot be decoded: " + err.Error()
	}
	if draft.PacketID != packet.PacketID {
		return latest, nil, "the current planning packet identity differs from the selected packet"
	}
	if c.Spec.PlanningRef == nil {
		return latest, draft, "reviewed planning reference is missing"
	}
	if err := planning.AdmitPlanRevision(p, *c.Spec.PlanningRef); err != nil {
		return latest, draft, err.Error()
	}
	if err := c.verifyPlanningProjection(p.Active); err != nil {
		return latest, draft, err.Error()
	}
	if err := c.verifyPlanningArtifacts(); err != nil {
		return latest, draft, err.Error()
	}
	if c.State.Handoff != nil || planningHasPendingReceipt(c.State) {
		return latest, draft, "return human session control and settle outstanding invocations first"
	}
	for _, request := range p.Requests {
		if request.InputRevision == p.InputRevision && (request.Status == "pending" || request.Status == "running") {
			return latest, draft, "finish the outstanding planning request before editing the packet"
		}
	}
	if current := c.State.Packets[packet.PacketID]; current == nil || current.Status == "completed" {
		return latest, draft, "completed or untracked packets require an explicit new plan before changing their contract"
	}
	return latest, draft, ""
}
