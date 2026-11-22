package machine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/imageinput"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
)

// InputImagesEdit carries references only. RawMessage distinguishes an explicit
// JSON null (remove the declaration for review) from an omitted value.
type InputImagesEdit struct {
	Packet              string          `json:"packet"`
	ContractFingerprint string          `json:"contractFingerprint"`
	Plan                planning.Ref    `json:"plan"`
	Value               json.RawMessage `json:"value"`
}

type ImageInputEditor struct {
	Packet              string                 `json:"packet"`
	Revision            int64                  `json:"revision"`
	ContractFingerprint string                 `json:"contractFingerprint,omitempty"`
	Plan                *planning.Ref          `json:"plan,omitempty"`
	Editable            bool                   `json:"editable"`
	Status              string                 `json:"status"` // known | unknown declaration; never proof of current source bytes
	Reason              string                 `json:"reason"`
	Value               *imageinput.References `json:"value"`
}

func imageInputReferences(edit *InputImagesEdit) (*imageinput.References, error) {
	if edit == nil || len(edit.Value) == 0 {
		return nil, fmt.Errorf("input-images requires the exact packet, contract fingerprint, plan and an explicit value; use null only to request removal")
	}
	if _, err := contract.MilestoneOfPacket(edit.Packet); err != nil {
		return nil, err
	}
	if !planningRefDigest.MatchString(edit.ContractFingerprint) || edit.Plan.Revision < 1 || !planningRefDigest.MatchString(edit.Plan.Digest) {
		return nil, fmt.Errorf("input-images requires an exact contract fingerprint and immutable plan reference")
	}
	raw, err := json.Marshal(edit)
	if err != nil || len(raw) > 32<<10 {
		return nil, fmt.Errorf("input-images editor request must be valid JSON within 32 KiB")
	}
	var refs *imageinput.References
	decoder := json.NewDecoder(bytes.NewReader(edit.Value))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&refs) != nil {
		return nil, fmt.Errorf("input-images value must be frozen references or explicit null, without unknown fields")
	}
	return refs, imageinput.Validate(refs)
}

func inputImagesRequestIdentity(edit *InputImagesEdit) string {
	refs, _ := imageInputReferences(edit) // the control boundary has validated the complete edit
	return "Manual image inputs revision " + sessionHash(struct {
		Packet              string
		ContractFingerprint string
		Plan                planning.Ref
		Value               *imageinput.References
	}{edit.Packet, edit.ContractFingerprint, edit.Plan, refs})
}

func (c *Context) imageInputEditor(packet *contract.Packet, fingerprint string) *ImageInputEditor {
	v := &ImageInputEditor{Packet: packet.PacketID, Revision: c.State.Revision, ContractFingerprint: fingerprint, Value: packet.InputImages, Status: "known"}
	latest, draft, reason := c.packetEditPlan(packet)
	if latest != nil {
		v.Plan = &latest.Ref
		if draft == nil {
			v.Status, v.Value = "unknown", nil
		}
	}
	if draft != nil {
		v.Value = draft.InputImages
	}
	v.Editable, v.Reason = reason == "", reason
	if v.Editable {
		v.Reason = "Saving revalidates the original image bytes and creates an immutable plan for independent review; implementation is not started."
	}
	return v
}

func applyInputImagesEdit(opts RunOpts, st *state.State, r PlanningCommand) error {
	edit := r.InputImages
	refs, err := imageInputReferences(edit)
	if err != nil {
		return err
	}
	c, packet, fingerprint, err := packetControlContext(st, edit.Packet, []string{opts.TaskDir, opts.Cwd})
	if err != nil {
		return err
	}
	view := c.imageInputEditor(packet, fingerprint)
	if !view.Editable {
		return fmt.Errorf("input-images cannot be edited: %s", view.Reason)
	}
	if edit.ContractFingerprint != fingerprint || view.Plan == nil || edit.Plan != *view.Plan {
		return fmt.Errorf("input-images snapshot changed; refresh the packet and exact current plan")
	}
	if reflect.DeepEqual(packet.InputImages, refs) {
		return fmt.Errorf("input-images is unchanged; no new review is needed")
	}
	if _, err := imageinput.Load(c.Cwd, refs); err != nil {
		return err
	}
	request := planning.Request{ID: r.RequestID, Origin: "input-images-edit", Scope: "packet", ScopeID: edit.Packet, InputRevision: c.State.Planning.InputRevision,
		PlanningOnly: true, Feedback: inputImagesRequestIdentity(edit)}
	return c.stagePacketFieldEdit(st, *planning.FindPlan(c.State.Planning, *view.Plan), edit.Packet, "input-images", refs, request,
		"control:input-images:", "Manual image input revision saved; the exact plan requires independent review. Implementation has not started.")
}
