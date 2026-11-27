package machine

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
)

type planningFeedbackRequest struct {
	Version       int             `json:"version"`
	Workspace     string          `json:"workspace"`
	InputRevision int             `json:"input-revision"`
	At            string          `json:"at"`
	Command       PlanningCommand `json:"command"`
}

type savedPlanningControl struct {
	Request planningFeedbackRequest `json:"request"`
	Receipt *PlanningControlReceipt `json:"receipt"`
}

func planningControlRoot(taskDir string, create bool) (*os.Root, error) {
	root, err := openPlanningTask(taskDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	for _, name := range []string{".control", ".control/planning"} {
		if create {
			if err := mkdirPlanningDirectory(root, name, 0700); err != nil && !os.IsExist(err) {
				return nil, err
			}
		}
		info, err := statPlanningPath(root, name)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("planning control requires real directories")
		}
	}
	return root.OpenRoot(".control/planning")
}

func readPlanningFeedback(root *os.Root, name string) (planningFeedbackRequest, error) {
	var request planningFeedbackRequest
	raw, err := readPlanningBytes(root, name)
	if err != nil {
		return request, err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 192<<10 || !json.Valid(raw) || d.Decode(&request) != nil || request.Version != 1 || request.Command.Action != "feedback" ||
		request.InputRevision < 1 || request.At == "" || name != "feedback-"+request.Command.RequestID+".json" {
		return request, fmt.Errorf("invalid queued planning feedback")
	}
	return request, ValidatePlanningCommand(request.Command)
}

func planningFeedbackEntries(root *os.Root) ([]string, error) {
	entries, err := listPlanningDirectory(root)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "feedback-") && strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}
	if len(names) > 256 {
		return nil, fmt.Errorf("planning feedback queue exceeds its bounded capacity")
	}
	sort.Strings(names)
	return names, nil
}

func queuePlanningFeedback(opts RunOpts, command PlanningCommand) (*state.State, *PlanningControlReceipt, error) {
	st, err := state.Load(opts.TaskDir)
	if err != nil {
		return nil, nil, err
	}
	if err := planningControlWorkspace(opts, st); err != nil {
		return st, nil, err
	}
	if rejected := rejectedPlanningReceipt(st, command.RequestID); rejected != nil {
		return st, rejected, fmt.Errorf("planning request-id was already rejected; submit refreshed input with a new request-id")
	}
	already, err := planningCommandApplied(st, command, "")
	if err != nil || already {
		return st, planningControlResult(st, command, already, "This exact feedback is already recorded."), err
	}
	root, err := planningControlRoot(opts.TaskDir, true)
	if err != nil {
		return st, nil, err
	}
	defer root.Close()
	name := "feedback-" + command.RequestID + ".json"
	old, err := readPlanningFeedback(root, name)
	if err == nil {
		if !reflect.DeepEqual(old.Command, command) || old.Workspace != st.Project.Cwd {
			return st, nil, fmt.Errorf("queued request-id was already used for different feedback")
		}
	} else if !os.IsNotExist(err) {
		return st, nil, err
	} else {
		if st.Revision != command.Revision || st.Planning.PendingActivation != nil || st.Handoff != nil {
			return st, nil, fmt.Errorf("feedback requires the current revision outside activation or human session control")
		}
		// Validate against a clone before publication. Only the lease owner will
		// change the real input revision after settling the original call.
		copy := *st
		copy.Planning = planning.Clone(st.Planning)
		if err := applyPlanningCommand(opts, &copy, command, ""); err != nil {
			return st, nil, err
		}
		names, err := planningFeedbackEntries(root)
		if err != nil {
			return st, nil, err
		}
		if len(names) == 256 {
			return st, nil, fmt.Errorf("planning feedback queue is full")
		}
		entry := planningFeedbackRequest{Version: 1, Workspace: st.Project.Cwd, InputRevision: st.Planning.InputRevision, At: planningControlTime(opts), Command: command}
		raw, _ := json.Marshal(entry)
		tmp := ".feedback-" + rand.Text()
		defer root.Remove(tmp)
		if err := writePlanningBytes(root, tmp, raw); err != nil {
			return st, nil, err
		}
		if err := root.Link(tmp, name); err != nil {
			return st, nil, err // never overwrite another publisher's identity
		}
		dir, err := openPlanningDirectory(root)
		if err != nil {
			return st, nil, err
		}
		defer dir.Close()
		if err := syncPlanningHandle(dir); err != nil {
			return st, nil, err
		}
	}
	return st, &PlanningControlReceipt{RequestID: command.RequestID, Action: "feedback", Status: "queued", Revision: st.Revision,
		Message: "Feedback is queued; the owner will apply it only after consuming the original invocation receipt."}, nil
}

func (c *Context) planningQueuePending() (bool, error) {
	root, err := planningControlRoot(c.TaskDir, false)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer root.Close()
	names, err := planningFeedbackEntries(root)
	return len(names) > 0, err
}

func (c *Context) applyPlanningQueue() error {
	root, err := planningControlRoot(c.TaskDir, false)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	names, err := planningFeedbackEntries(root)
	if err != nil || len(names) == 0 {
		return err
	}
	if c.parallel != nil {
		return fmt.Errorf("queued planning input requires the owner boundary; parallel workers cannot adopt it")
	}
	if planningHasPendingReceipt(c.State) || c.State.Planning.PendingActivation != nil {
		return nil
	}
	for _, name := range names {
		entry, err := readPlanningFeedback(root, name)
		if err != nil {
			return err
		}
		already, err := planningCommandApplied(c.State, entry.Command, "")
		if err != nil {
			if err := c.finishPlanningFeedback(root, name, entry, false, err); err != nil {
				return err
			}
			continue
		}
		if !already {
			if c.State.Project == nil || entry.Workspace != c.State.Project.Cwd || entry.InputRevision != c.State.Planning.InputRevision {
				if err := c.finishPlanningFeedback(root, name, entry, false, fmt.Errorf("queued feedback no longer matches the recorded planning input; refresh before sending a new request")); err != nil {
					return err
				}
				continue
			}
			if err := applyPlanningCommand(RunOpts{TaskDir: c.TaskDir, Cwd: c.Cwd, Now: c.Now}, c.State, entry.Command, ""); err != nil {
				if err := c.finishPlanningFeedback(root, name, entry, false, err); err != nil {
					return err
				}
				continue
			}
			c.save()
			if err := c.err(); err != nil {
				return err
			}
		}
		if err := c.finishPlanningFeedback(root, name, entry, already, nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *Context) finishPlanningFeedback(root *os.Root, name string, entry planningFeedbackRequest, already bool, rejected error) error {
	receipt := planningControlResult(c.State, entry.Command, already, "Queued feedback was applied after the original invocation receipt was consumed.")
	if rejected != nil {
		receipt.Status, receipt.Message = "rejected", rejected.Error()
		if rejectedPlanningReceipt(c.State, entry.Command.RequestID) == nil {
			c.State.Inbox = append(c.State.Inbox, map[string]any{"kind": "planning-feedback-rejected", "request-id": entry.Command.RequestID, "message": receipt.Message})
			c.save()
			if err := c.err(); err != nil {
				return err
			}
		}
		c.planningPause = true
	} else if !entry.Command.StartRequested(false) {
		c.planningPause = true
	}
	receipt.Revision = c.State.Revision
	// Keep the original input with its terminal control receipt across restart.
	raw, _ := json.Marshal(savedPlanningControl{entry, receipt})
	if err := writePlanningBytes(root, "receipt-"+entry.Command.RequestID+".json", raw); err != nil {
		return err
	}
	return root.Remove(name)
}

func rejectedPlanningReceipt(st *state.State, id string) *PlanningControlReceipt {
	for _, value := range st.Inbox {
		if item, ok := value.(map[string]any); ok && (item["kind"] == "planning-feedback-rejected" || item["kind"] == "planning-collaboration-rejected" || item["kind"] == "planning-input-images-rejected") && item["request-id"] == id {
			message, _ := item["message"].(string)
			action := "feedback"
			if item["kind"] == "planning-collaboration-rejected" {
				action = "collaboration"
			} else if item["kind"] == "planning-input-images-rejected" {
				action = "input-images"
			}
			return &PlanningControlReceipt{RequestID: id, Action: action, Status: "rejected", Revision: st.Revision, Message: message}
		}
	}
	return nil
}

func readPlanningControlReceipt(taskDir, id string, st *state.State) (*PlanningControlReceipt, error) {
	if st.Planning == nil {
		return nil, fmt.Errorf("planning request receipts require a reviewed planning task")
	}
	root, err := planningControlRoot(taskDir, false)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if root != nil {
		defer root.Close()
		raw, err := readPlanningBytes(root, "receipt-"+id+".json")
		if err == nil {
			var saved savedPlanningControl
			d := json.NewDecoder(bytes.NewReader(raw))
			d.DisallowUnknownFields()
			if len(raw) > 192<<10 || !json.Valid(raw) || d.Decode(&saved) != nil || saved.Receipt == nil || saved.Request.Command.RequestID != id ||
				saved.Request.Version != 1 || saved.Request.InputRevision < 1 || saved.Request.At == "" || st.Project == nil || saved.Request.Workspace != st.Project.Cwd ||
				saved.Request.Command.Action != "feedback" || saved.Receipt.RequestID != id || saved.Receipt.Action != "feedback" ||
				saved.Receipt.Revision > st.Revision || ValidatePlanningCommand(saved.Request.Command) != nil {
				return nil, fmt.Errorf("planning control receipt does not match its original request")
			}
			if saved.Receipt.Status == "rejected" {
				if rejected := rejectedPlanningReceipt(st, id); rejected != nil && rejected.Message == saved.Receipt.Message {
					return saved.Receipt, nil
				}
			} else if saved.Receipt.Status == "applied" || saved.Receipt.Status == "unchanged" {
				if applied, err := planningCommandApplied(st, saved.Request.Command, ""); err == nil && applied {
					return saved.Receipt, nil
				}
			}
			return nil, fmt.Errorf("planning control receipt lacks its saved adoption or rejection evidence")
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		pending, err := readPlanningFeedback(root, "feedback-"+id+".json")
		if err == nil {
			if applied, err := planningCommandApplied(st, pending.Command, ""); err != nil {
				return nil, err
			} else if !applied && rejectedPlanningReceipt(st, id) == nil {
				return &PlanningControlReceipt{RequestID: id, Action: "feedback", Status: "queued", Revision: st.Revision, Message: "Feedback is waiting for its owner boundary."}, nil
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	if rejected := rejectedPlanningReceipt(st, id); rejected != nil {
		return rejected, nil
	}
	action := ""
	for _, request := range st.Planning.Requests {
		if request.ID == id {
			action = "plan"
			if request.Origin == "collaboration-edit" {
				action = "collaboration"
			} else if request.Origin == "input-images-edit" {
				action = "input-images"
			}
		}
	}
	for _, feedback := range st.Planning.Feedback {
		if feedback.RequestID == id {
			action = "feedback"
		}
	}
	for _, approval := range st.Planning.Approvals {
		if approval.RequestID == id {
			action = "approve"
		}
	}
	if action == "" {
		return nil, fmt.Errorf("planning request-id has no saved request or receipt")
	}
	return &PlanningControlReceipt{RequestID: id, Action: action, Status: "applied", Revision: st.Revision, Message: "The original action is recorded in the planning ledger."}, nil
}
