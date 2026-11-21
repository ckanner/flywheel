package machine

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"gopkg.in/yaml.v3"
)

// CollaborationEdit changes only a packet's task split. The active files and
// historical approvals remain immutable until a new independent plan review.
type CollaborationEdit struct {
	Packet              string                       `json:"packet"`
	ContractFingerprint string                       `json:"contractFingerprint"`
	Plan                planning.Ref                 `json:"plan"`
	Value               contract.PacketCollaboration `json:"value"`
}

func validateCollaborationEdit(edit *CollaborationEdit) error {
	if edit == nil {
		return fmt.Errorf("collaboration requires the exact packet, contract fingerprint, plan reference and task split")
	}
	if _, err := contract.MilestoneOfPacket(edit.Packet); err != nil {
		return err
	}
	if !planningRefDigest.MatchString(edit.ContractFingerprint) || edit.Plan.Revision < 1 || !planningRefDigest.MatchString(edit.Plan.Digest) {
		return fmt.Errorf("collaboration requires an exact contract fingerprint and immutable plan reference")
	}
	raw, _ := json.Marshal(edit)
	if len(raw) > 32<<10 {
		return fmt.Errorf("collaboration editor request exceeds 32 KiB")
	}
	// Parent scopes are rechecked under the lease against the actual packet.
	if edit.Value.Version != 1 || len(edit.Value.Tasks) < 2 || len(edit.Value.Tasks) > 8 {
		return fmt.Errorf("collaboration requires version 1 and 2–8 tasks")
	}
	return nil
}

func collaborationRequestIdentity(edit *CollaborationEdit) string {
	return "Manual collaboration revision " + sessionHash(edit)
}

func applyCollaborationEdit(opts RunOpts, st *state.State, r PlanningCommand) error {
	edit := r.Collaboration
	c, packet, fingerprint, err := packetControlContext(st, edit.Packet, []string{opts.TaskDir, opts.Cwd})
	if err != nil {
		return err
	}
	view := c.collaborationEditor(packet, fingerprint)
	if !view.Editable {
		return fmt.Errorf("collaboration cannot be edited: %s", view.Reason)
	}
	if edit.ContractFingerprint != fingerprint || view.Plan == nil || edit.Plan != *view.Plan {
		return fmt.Errorf("collaboration snapshot changed; refresh the packet and exact current plan")
	}
	if reflect.DeepEqual(packet.Collaboration, &edit.Value) {
		return fmt.Errorf("collaboration is unchanged; no new review is needed")
	}
	packet.Collaboration = &edit.Value
	if err := contract.ValidateCollaboration(packet); err != nil {
		return err
	}
	return c.stageCollaborationEdit(st, *planning.FindPlan(c.State.Planning, *view.Plan), edit, r.RequestID)
}

// stageCollaborationEdit validates the complete replacement projection on a
// private context before publishing the atomic planning transition to st.
func (c *Context) stageCollaborationEdit(st *state.State, previous planning.ExecutionPlan, edit *CollaborationEdit, requestID string) error {
	request := planning.Request{ID: requestID, Origin: "collaboration-edit", Scope: "packet", ScopeID: edit.Packet, InputRevision: c.State.Planning.InputRevision, PlanningOnly: true, Feedback: collaborationRequestIdentity(edit)}
	return c.stagePacketFieldEdit(st, previous, edit.Packet, "collaboration", edit.Value, request,
		"control:collaboration:", "Manual collaboration revision saved; the exact plan requires independent review. Implementation has not started.")
}

func (c *Context) stagePacketFieldEdit(st *state.State, previous planning.ExecutionPlan, packetID, field string, proposed any, request planning.Request, producer, reason string) error {
	// Work only on the private clone. A validation failure must not leak an
	// unpersisted plan through the control response or alter an approved artifact.
	p := c.State.Planning
	files := jsonClone(previous.Files)
	name := "task-packet-" + packetID + ".md"
	data, body, err := contract.ParseFrontmatter([]byte(files[name]), name)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(proposed) // only validated typed collaboration or image references reach this boundary
	var value any
	_ = json.Unmarshal(raw, &value)
	if value == nil {
		delete(data, field) // only an explicit image-input null requests a reviewed removal
	} else {
		data[field] = value
	}
	frontmatter, _ := yaml.Marshal(data) // parsed YAML plus JSON data has no custom marshalers
	files[name] = "---\n" + string(frontmatter) + "---\n" + body
	if err := c.validatePlanningFiles(files, request); err != nil {
		return err
	}
	dependencies, err := c.planScopeIdentity()
	if err != nil {
		return err
	}
	err = planning.RecordPlanForReview(p, planning.ExecutionPlan{Ref: planning.Ref{Revision: len(p.Plans) + 1}, Design: *p.CurrentDesign, InputRevision: p.InputRevision,
		Scope: request.Scope, ScopeID: request.ScopeID, Files: files, Dependencies: dependencies, ScopeVerdict: "within-envelope", PlannerInvocation: producer + sessionHash(request.ID)}, request)
	if err == nil {
		p.Reason = reason
		*st = *c.State
	}
	return err
}
