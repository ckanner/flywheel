package machine

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/imageinput"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/prompts"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func imageEditValue(t *testing.T, refs *imageinput.References) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(refs)
	require.NoError(t, err)
	return raw
}

func imageInputsEditFixture(t *testing.T, initial bool) (fixture, *Context, RunOpts, PlanningCommand, []byte) {
	t.Helper()
	fx, c := newReviewedFixture(t)
	original, err := os.ReadFile("../../plugins/dsh/test/fixtures/multimodal-reference.png")
	require.NoError(t, err)
	writeFile(t, filepath.Join(fx.root, "references/layout.png"), string(original))
	replacement := append(append([]byte{}, original...), []byte("replacement frozen input")...)
	writeFile(t, filepath.Join(fx.root, "references/replacement.png"), string(replacement))
	p := c.State.Planning
	proposal := p.Plans[0]
	proposal.Ref, proposal.Files = planning.Ref{Revision: len(p.Plans) + 1}, jsonClone(proposal.Files)
	proposal.PlannerInvocation = "initial-image-plan"
	proposal.Dependencies, err = c.planScopeIdentity()
	require.NoError(t, err)
	if initial {
		declaration := fmt.Sprintf("input-images: {version: 1, images: [{path: references/layout.png, sha256: %x, media-type: image/png}]}\n", sha256.Sum256(original))
		proposal.Files["task-packet-M01-T01.md"] = strings.Replace(proposal.Files["task-packet-M01-T01.md"], "packet-id:", declaration+"packet-id:", 1)
	}
	proposal, err = planning.SealPlan(proposal)
	require.NoError(t, err)
	require.NoError(t, planning.RecordPlan(p, proposal))
	require.NoError(t, planning.RecordReview(p, planning.Review{ID: "initial-image-plan-review", Kind: "plan", Target: proposal.Ref, Design: p.CurrentDesign,
		InputRevision: p.InputRevision, Invocation: "independent-image-plan-review", Role: "plan-review", Binding: "independent-reviewer", Verdict: "pass", ScopeVerdict: "within-envelope", Summary: "The declared input retains the approved implementation scope.", Findings: []planning.Finding{}}))
	require.NoError(t, c.checkPlanningExports(true))
	require.NoError(t, c.activatePlanningPlans())
	require.NoError(t, c.reloadPackets())
	c.save()
	require.NoError(t, c.err())
	node, err := ReadNode(c.State, "M01-T01", fx.taskDir, fx.root)
	require.NoError(t, err)
	v := node.ImageInputEditor
	require.True(t, v.Editable, v.Reason)
	path, bytes := "references/layout.png", original
	if initial {
		path, bytes = "references/replacement.png", replacement
	}
	refs := &imageinput.References{Version: 1, Images: []imageinput.Reference{{Path: path, SHA256: fmt.Sprintf("%x", sha256.Sum256(bytes)), MediaType: "image/png"}}}
	request := PlanningCommand{Action: "input-images", Revision: c.State.Revision, RequestID: "edit-image-inputs", Start: new(true),
		InputImages: &InputImagesEdit{Packet: "M01-T01", ContractFingerprint: v.ContractFingerprint, Plan: *v.Plan, Value: imageEditValue(t, refs)}}
	return fx, c, RunOpts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config}, request, original
}

func TestInputImagesEditsAddReplaceAndExplicitlyClearOnlyThroughIndependentReview(t *testing.T) {
	for _, action := range []string{"add", "replace", "clear"} {
		t.Run(action, func(t *testing.T) {
			fx, c, opts, request, original := imageInputsEditFixture(t, action != "add")
			if action == "clear" {
				request.InputImages.Value = json.RawMessage("null")
			}
			before := jsonClone(c.State)
			packetPath := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
			activePacket := mustReadFile(t, packetPath)
			saved, receipt, err := ControlPlanning(opts, request, "")
			require.NoError(t, err)
			require.Equal(t, "input-images", receipt.Action)
			require.Equal(t, "applied", receipt.Status)
			require.Equal(t, before.Planning.Approvals, saved.Planning.Approvals)
			require.Equal(t, before.Planning.Reviews, saved.Planning.Reviews)
			require.Equal(t, before.Planning.Active, saved.Planning.Active)
			require.Equal(t, before.Planning.StartAuthorized, saved.Planning.StartAuthorized)
			require.Equal(t, before.Packets, saved.Packets)
			require.Equal(t, activePacket, mustReadFile(t, packetPath))
			require.Equal(t, before.Planning.Plans, saved.Planning.Plans[:len(before.Planning.Plans)])
			latest := saved.Planning.Plans[len(saved.Planning.Plans)-1]
			expected, err := imageInputReferences(request.InputImages)
			require.NoError(t, err)
			packet, err := contract.ParsePacket([]byte(latest.Files["task-packet-M01-T01.md"]), "new image plan")
			require.NoError(t, err)
			require.Equal(t, expected, packet.InputImages)
			require.Equal(t, "control:input-images:"+sessionHash(request.RequestID), latest.PlannerInvocation)
			old := planning.FindPlan(before.Planning, request.InputImages.Plan)
			require.Equal(t, old.Files["spec.md"], latest.Files["spec.md"])
			require.Equal(t, old.Files["task-packet-M01-T02.md"], latest.Files["task-packet-M01-T02.md"])
			if action == "clear" {
				require.NotContains(t, latest.Files["task-packet-M01-T01.md"], "input-images:")
			}
			pending := saved.Planning.Requests[len(saved.Planning.Requests)-1]
			require.Equal(t, "input-images-edit", pending.Origin)
			require.Equal(t, "reviewer", pending.Stage)
			require.Equal(t, 1, pending.Rounds)
			require.True(t, pending.PlanningOnly)
			require.Nil(t, planning.LatestReview(saved.Planning, "plan", latest.Ref))
			require.Error(t, planning.Admit(saved.Planning, *c.Spec.PlanningRef))
			node, err := ReadNode(saved, "M01-T01", fx.taskDir, fx.root)
			require.NoError(t, err)
			require.False(t, node.ImageInputEditor.Editable)
			require.Equal(t, expected, node.ImageInputEditor.Value)
			require.Equal(t, latest.Ref, *node.ImageInputEditor.Plan)
			require.NotContains(t, string(mustReadFile(t, state.StateFile(fx.taskDir))), base64.StdEncoding.EncodeToString(original))
			replayed, duplicate, err := ControlPlanning(opts, request, "")
			require.NoError(t, err)
			require.Equal(t, "unchanged", duplicate.Status)
			require.Equal(t, saved.Revision, replayed.Revision)
			_, reconciled, err := ControlPlanning(opts, PlanningCommand{Action: "status", RequestID: request.RequestID}, "")
			require.NoError(t, err)
			require.Equal(t, "input-images", reconciled.Action)
			require.Equal(t, "applied", reconciled.Status)
			c.State, c.revision = saved, saved.Revision
			require.NoError(t, c.checkPlanningExports(true))
			reviews := 0
			c.Drivers.PlanReview = &fnDriver{role: "plan-review", fn: func(args drivers.CallArgs) drivers.Result {
				reviews++
				require.Contains(t, args.Prompt, "Manual image inputs revision")
				require.Empty(t, args.Images, "the independent contract review does not claim visual consumption")
				return planningJSONResult(t, prompts.DesignReviewOutput{Verdict: "pass", ScopeVerdict: "within-envelope", Summary: "The exact image-input change preserves the approved scope and acceptance.", Findings: []planning.Finding{}})
			}}
			c.Drivers.Plan = &fnDriver{role: "plan", fn: func(drivers.CallArgs) drivers.Result {
				t.Fatal("manual input edit must enter reviewer directly")
				return drivers.Result{}
			}}
			c.Drivers.Execute = &fnDriver{role: "execute", fn: func(drivers.CallArgs) drivers.Result {
				t.Fatal("input edit must not start implementation")
				return drivers.Result{}
			}}
			ok, err := c.processPlanningRequest(len(saved.Planning.Requests) - 1)
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t, 1, reviews)
			node, err = ReadNode(c.State, "M01-T01", fx.taskDir, fx.root)
			require.NoError(t, err)
			require.False(t, node.ImageInputEditor.Editable, "a reviewed but unactivated draft cannot be edited against the old active contract")
			require.Equal(t, "known", node.ImageInputEditor.Status)
			require.Equal(t, expected, node.ImageInputEditor.Value)
			require.NoError(t, c.activatePlanningPlans())
			require.NoError(t, c.reloadPackets())
			require.Equal(t, expected, c.Packets["M01-T01"].InputImages)
			require.Equal(t, before.Planning.Approvals, c.State.Planning.Approvals)
		})
	}
}

func TestInputImagesEditorReadsDeclarationsWithoutOpeningSourceImages(t *testing.T) {
	fx, c, _, request, _ := imageInputsEditFixture(t, true)
	require.NoError(t, os.Remove(filepath.Join(fx.root, "references/layout.png")))
	before := jsonClone(c.State)
	node, err := ReadNode(c.State, "M01-T01", fx.taskDir, fx.root)
	require.NoError(t, err)
	require.True(t, node.ImageInputEditor.Editable, node.ImageInputEditor.Reason)
	require.Equal(t, "known", node.ImageInputEditor.Status)
	require.Equal(t, c.Packets["M01-T01"].InputImages, node.ImageInputEditor.Value)
	require.Equal(t, before, c.State)
	packet := jsonClone(c.Packets["M01-T01"])
	packet.Schedule = nil
	require.True(t, c.imageInputEditor(packet, request.InputImages.ContractFingerprint).Editable, "image selection does not require a collaboration write split")
	require.False(t, c.collaborationEditor(packet, request.InputImages.ContractFingerprint).Editable)
	unknown, err := ReadNode(c.State, "M01-T01")
	require.NoError(t, err)
	raw, err := json.Marshal(unknown.ImageInputEditor)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"value":null`, "missing context must never silently look like a clear operation")
	require.False(t, unknown.ImageInputEditor.Editable)
	require.Equal(t, "unknown", unknown.ImageInputEditor.Status)
	copy := *c
	copy.State = jsonClone(c.State)
	copy.State.Planning.StartAuthorized = false
	for i := range copy.State.Planning.Approvals {
		copy.State.Planning.Approvals[i].Start = false
	}
	require.True(t, copy.imageInputEditor(packet, request.InputImages.ContractFingerprint).Editable)
	require.ErrorContains(t, planning.Admit(copy.State.Planning, *copy.Spec.PlanningRef), "authorize starting implementation")
	planning.FindPlan(copy.State.Planning, request.InputImages.Plan).Files["task-packet-M01-T01.md"] = "not a packet"
	view := copy.imageInputEditor(packet, request.InputImages.ContractFingerprint)
	require.False(t, view.Editable)
	require.Equal(t, "unknown", view.Status)
	require.Nil(t, view.Value, "an unreadable latest plan must not borrow active references under the latest plan identity")
	require.Contains(t, view.Reason, "cannot be decoded")
	copy.State = jsonClone(c.State)
	plan := planning.FindPlan(copy.State.Planning, request.InputImages.Plan)
	plan.Files["task-packet-M01-T01.md"] = strings.ReplaceAll(plan.Files["task-packet-M01-T01.md"], "M01-T01", "M01-T02")
	view = copy.imageInputEditor(packet, request.InputImages.ContractFingerprint)
	require.Equal(t, "unknown", view.Status)
	require.Nil(t, view.Value)
	require.False(t, view.Editable)
	require.Contains(t, view.Reason, "identity differs")
	copy.State.Planning = nil
	view = copy.imageInputEditor(packet, request.InputImages.ContractFingerprint)
	require.Equal(t, "known", view.Status)
	require.Equal(t, packet.InputImages, view.Value)
	require.Nil(t, view.Plan)
	require.False(t, view.Editable)
}

func TestInputImagesNestedSessionUsesTheSameProjectRootForPreviewAndFreeze(t *testing.T) {
	fx, c, opts, request, _ := imageInputsEditFixture(t, false)
	nested := filepath.Join(fx.root, "src", "nested-session")
	require.NoError(t, os.MkdirAll(nested, 0755))
	// Project configuration is discovered at the same root used by images and
	// the execution engine, even when the current session lives below it.
	require.NoError(t, os.Rename(filepath.Join(fx.taskDir, "flywheel.config.json"), filepath.Join(fx.root, "flywheel.config.json")))
	preview, err := imageinput.Inspect(ProjectRoot(nested), []string{"references/layout.png"})
	require.NoError(t, err)
	require.Len(t, preview.Previews, 1)
	request.InputImages.Value = imageEditValue(t, &preview.References)
	node, err := ReadNode(c.State, request.InputImages.Packet, fx.taskDir, nested)
	require.NoError(t, err)
	require.True(t, node.ImageInputEditor.Editable, node.ImageInputEditor.Reason)
	require.Equal(t, "known", node.ImageInputEditor.Status)
	opts.Cwd = nested
	saved, receipt, err := ControlPlanning(opts, request, "")
	require.NoError(t, err)
	require.Equal(t, "applied", receipt.Status)
	latest := saved.Planning.Plans[len(saved.Planning.Plans)-1]
	packet, err := contract.ParsePacket([]byte(latest.Files["task-packet-M01-T01.md"]), "root-relative image plan")
	require.NoError(t, err)
	require.Equal(t, &preview.References, packet.InputImages)
	dependency, err := c.planScopeIdentity()
	require.NoError(t, err)
	require.Equal(t, dependency, latest.Dependencies, "a nested session must freeze the complete repository dependency inventory")
	for _, location := range [][]string{{filepath.Join(fx.root, "missing-task"), nested}, {fx.taskDir, filepath.Join(fx.root, "missing-cwd")}} {
		_, _, _, err := packetControlContext(c.State, request.InputImages.Packet, location)
		require.Error(t, err)
	}
}

func TestInputImagesEditRejectsChangedBytesAndStaleAuthorityWithDurableReceipts(t *testing.T) {
	for _, scenario := range []string{"revision", "fingerprint", "plan", "changed-bytes", "invalid-raster", "wrong-mime", "symlink", "unchanged", "pending-invocation", "completed", "changed-sibling", "missing-exports"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c, opts, request, original := imageInputsEditFixture(t, false)
			refs, err := imageInputReferences(request.InputImages)
			require.NoError(t, err)
			switch scenario {
			case "revision":
				request.Revision--
			case "fingerprint":
				request.InputImages.ContractFingerprint = strings.Repeat("a", 64)
			case "plan":
				request.InputImages.Plan.Revision++
			case "changed-bytes":
				writeFile(t, filepath.Join(fx.root, refs.Images[0].Path), string(original)+"changed after preview")
			case "invalid-raster":
				writeFile(t, filepath.Join(fx.root, refs.Images[0].Path), "plain text")
				refs.Images[0].SHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte("plain text")))
				request.InputImages.Value = imageEditValue(t, refs)
			case "wrong-mime":
				refs.Images[0].MediaType = "image/jpeg"
				request.InputImages.Value = imageEditValue(t, refs)
			case "symlink":
				require.NoError(t, os.Remove(filepath.Join(fx.root, refs.Images[0].Path)))
				require.NoError(t, os.Symlink("replacement.png", filepath.Join(fx.root, refs.Images[0].Path)))
			case "unchanged":
				request.InputImages.Value = json.RawMessage("null")
			case "pending-invocation":
				c.State.PlanSession = map[string]any{"outcome-unknown": true}
				c.save()
				request.Revision = c.State.Revision
			case "completed":
				c.State.Packets["M01-T01"].Status = "completed"
				c.save()
				request.Revision = c.State.Revision
			case "changed-sibling":
				path := filepath.Join(fx.taskDir, "task-packet-M01-T02.md")
				writeFile(t, path, string(mustReadFile(t, path))+"\nUnreviewed sibling edit\n")
			case "missing-exports":
				require.NoError(t, os.RemoveAll(filepath.Join(fx.taskDir, "planning")))
			}
			require.NoError(t, c.err())
			before := jsonClone(c.State)
			active := mustReadFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"))
			saved, receipt, err := ControlPlanning(opts, request, "")
			require.NoError(t, err)
			require.Equal(t, "rejected", receipt.Status)
			require.Equal(t, "input-images", receipt.Action)
			require.Equal(t, before.Planning, saved.Planning)
			require.Equal(t, before.Packets, saved.Packets)
			require.Equal(t, active, mustReadFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md")))
			require.Len(t, saved.Inbox, len(before.Inbox)+1)
			_, replay, err := ControlPlanning(opts, request, "")
			require.NoError(t, err)
			require.Equal(t, receipt, replay)
			_, reconciled, err := ControlPlanning(opts, PlanningCommand{Action: "status", RequestID: request.RequestID}, "")
			require.NoError(t, err)
			require.Equal(t, receipt, reconciled)
			changed := *jsonClone(&request)
			changed.InputImages.Plan.Revision++
			_, _, err = ControlPlanning(opts, changed, "")
			require.ErrorContains(t, err, "reused for different content")
			if scenario == "pending-invocation" {
				require.Contains(t, receipt.Message, "original invocation")
			}
			if scenario == "missing-exports" {
				_, err := os.Stat(filepath.Join(fx.taskDir, "planning"))
				require.True(t, os.IsNotExist(err), "read-only admission cannot repair missing exports")
			}
		})
	}
}

func TestInputImagesEditRequiresExplicitBoundedReferencesAndRejectsCrossActionReuse(t *testing.T) {
	_, c, opts, request, _ := imageInputsEditFixture(t, false)
	for _, mutate := range []func(*PlanningCommand){
		func(r *PlanningCommand) { r.Action = "collaboration" },
		func(r *PlanningCommand) { r.InputImages = nil },
		func(r *PlanningCommand) { r.InputImages.Value = nil },
		func(r *PlanningCommand) { r.InputImages.Packet = "../outside" },
		func(r *PlanningCommand) { r.InputImages.ContractFingerprint = "changed" },
		func(r *PlanningCommand) { r.InputImages.Plan.Digest = "changed" },
		func(r *PlanningCommand) { r.InputImages.Plan.Revision = 0 },
		func(r *PlanningCommand) { r.InputImages.Value = json.RawMessage("{") },
		func(r *PlanningCommand) {
			r.InputImages.Value = json.RawMessage(`{"unknown":"` + strings.Repeat("x", 33<<10) + `"}`)
		},
		func(r *PlanningCommand) {
			r.InputImages.Value = json.RawMessage(`{"version":1,"images":[],"data":"secret bytes"}`)
		},
		func(r *PlanningCommand) { r.InputImages.Value = json.RawMessage(`{"version":1,"images":[]}`) },
		func(r *PlanningCommand) { r.InputImages.Value = json.RawMessage(`[]`) },
	} {
		invalid := *jsonClone(&request)
		mutate(&invalid)
		require.Error(t, ValidatePlanningCommand(invalid))
	}
	before := jsonClone(c.State)
	require.Error(t, applyInputImagesEdit(opts, c.State, PlanningCommand{}))
	badLocation := opts
	badLocation.Cwd = "relative"
	require.Error(t, applyInputImagesEdit(badLocation, c.State, request))
	require.Equal(t, before, c.State)
	saved, _, err := ControlPlanning(opts, request, "")
	require.NoError(t, err)
	impostor := PlanningCommand{Action: "plan", Revision: saved.Revision, RequestID: request.RequestID, Scope: "packet", ScopeID: "M01-T01", Text: inputImagesRequestIdentity(request.InputImages)}
	_, _, err = ControlPlanning(opts, impostor, "")
	require.ErrorContains(t, err, "already used for different")
	changed := *jsonClone(&request)
	changed.InputImages.Value = json.RawMessage("null")
	_, _, err = ControlPlanning(opts, changed, "")
	require.ErrorContains(t, err, "already used for different")
	// JSON member order and whitespace do not create a second semantic identity.
	refs, err := imageInputReferences(request.InputImages)
	require.NoError(t, err)
	request.InputImages.Value = json.RawMessage(fmt.Sprintf(`{ "images": [{ "media-type": %q, "sha256": %q, "path": %q }], "version": 1 }`, refs.Images[0].MediaType, refs.Images[0].SHA256, refs.Images[0].Path))
	_, receipt, err := ControlPlanning(opts, request, "")
	require.NoError(t, err)
	require.Equal(t, "unchanged", receipt.Status)
}
