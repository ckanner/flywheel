package cli

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/spf13/cobra"
)

const milestoneUserActionEnv = "FLYWHEEL_MILESTONE_USER_ACTION"

// MilestoneControlReceipt reports the verified semantic transition of a
// milestone triage command (`retry` | `accept`).
type MilestoneControlReceipt struct {
	MilestoneID string `json:"milestone-id"`
	Action      string `json:"action"`
	Applied     bool   `json:"applied"`
	Revision    int64  `json:"revision"`
	Status      string `json:"status,omitempty"`
	Message     string `json:"message,omitempty"`
}

func readMilestoneCommand(reader io.Reader) (*machine.MilestoneCommand, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, 131073))
	if err != nil {
		return nil, err
	}
	if len(raw) > 131072 || !json.Valid(raw) {
		return nil, fmt.Errorf("invalid or oversized milestone command")
	}
	var request machine.MilestoneCommand
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return nil, err
	}
	return &request, request.Validate()
}

func milestoneUserActionFromEnv(request *machine.MilestoneCommand, taskDir string) (*machine.MilestoneUserAction, error) {
	raw, present := os.LookupEnv(milestoneUserActionEnv)
	if !present {
		return nil, fmt.Errorf("milestone triage requires a verified host command or controlling terminal")
	}
	if len(raw) > 8192 || !json.Valid([]byte(raw)) {
		return nil, fmt.Errorf("milestone triage proof is malformed or oversized")
	}
	var proof machine.MilestoneUserAction
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proof); err != nil || proof.Kind != "dsh" ||
		proof.SessionID == "" || len(proof.SessionID) > 200 || proof.SessionID != os.Getenv("DSH_SESSION_ID") ||
		proof.CommandID == "" || len(proof.CommandID) > 200 || strings.ContainsAny(proof.SessionID+proof.CommandID, "\x00\r\n") ||
		proof.Action != request.Action || proof.MilestoneID != request.MilestoneID || proof.Revision != request.Revision ||
		proof.Reason != request.Reason || proof.Guidance != request.Guidance || proof.TaskDir != taskDir {
		return nil, fmt.Errorf("milestone triage proof does not match this exact host command")
	}
	return &proof, nil
}

func milestoneUserActionFromTTY(request *machine.MilestoneCommand, taskDir string) (*machine.MilestoneUserAction, error) {
	terminal, err := openPlanningTTY()
	if err != nil {
		return nil, fmt.Errorf("milestone triage requires a verified host command or an interactive controlling terminal")
	}
	defer terminal.Close()
	if _, err := fmt.Fprintf(terminal, "Milestone: %s\nAction: %s\nReason: %s\nType '%s %s' to confirm this exact milestone decision: ",
		request.MilestoneID, request.Action, request.Reason, request.Action, request.MilestoneID); err != nil {
		return nil, fmt.Errorf("cannot present the exact milestone triage confirmation: %w", err)
	}
	line, err := bufio.NewReader(io.LimitReader(terminal, 512)).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != request.Action+" "+request.MilestoneID {
		return nil, fmt.Errorf("the controlling terminal did not confirm this exact milestone triage decision")
	}
	return &machine.MilestoneUserAction{
		Kind:        "tty",
		TaskDir:     taskDir,
		MilestoneID: request.MilestoneID,
		Revision:    request.Revision,
		Action:      request.Action,
		Reason:      request.Reason,
		Guidance:    request.Guidance,
	}, nil
}

func milestoneActionProof(request *machine.MilestoneCommand, taskDir string, allowTTY bool) error {
	proof, err := milestoneUserActionFromEnv(request, taskDir)
	if err == nil {
		request.UserAction = proof
		return nil
	}
	if !allowTTY {
		return err
	}
	proof, err = milestoneUserActionFromTTY(request, taskDir)
	if err != nil {
		return err
	}
	request.UserAction = proof
	return nil
}

func milestoneControlReceipt(st, before *state.State, request *machine.MilestoneCommand) *MilestoneControlReceipt {
	receipt := &MilestoneControlReceipt{MilestoneID: request.MilestoneID, Action: request.Action}
	if st == nil {
		receipt.Message = "task state is unavailable"
		return receipt
	}
	receipt.Revision = st.Revision
	ms, ok := st.Milestones[request.MilestoneID]
	if !ok {
		receipt.Message = "milestone not found"
		return receipt
	}
	receipt.Status = ms.Status
	if before == nil {
		receipt.Message = "the milestone command's effect could not be verified because pre-command state is unavailable; refresh status"
		return receipt
	}
	beforeMs, beforeOK := before.Milestones[request.MilestoneID]
	if !beforeOK {
		receipt.Message = "the milestone command's effect could not be verified because pre-command state is unavailable; refresh status"
		return receipt
	}
	switch request.Action {
	case "retry":
		receipt.Applied = beforeMs.Status != "closed" && st.Status == "paused" && st.Revision > before.Revision
		if !receipt.Applied {
			receipt.Message = "the milestone was not re-armed for re-verification"
		}
	default: // accept
		receipt.Applied = beforeMs.Status != "closed" && ms.Status == "closed" &&
			ms.HumanSignoff != nil && ms.HumanSignoff.Reason == request.Reason
		if !receipt.Applied {
			receipt.Message = "the milestone was not signed off and closed"
		}
	}
	return receipt
}

func startMilestoneControl(cmd *cobra.Command, taskDir, cwd string) (controlResponse, error) {
	request, err := readMilestoneCommand(cmd.InOrStdin())
	if err != nil {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, err
	}
	if err := milestoneActionProof(request, taskDir, false); err != nil {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, err
	}
	directory := filepath.Join(taskDir, ".control")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, err
	}
	info, err := sessionDirectoryStat(directory)
	if err != nil {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, fmt.Errorf("control directory must not be a symlink")
	}
	file, err := sessionInputTemp(directory, "milestone-request-*")
	if err != nil {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, err
	}
	defer file.Close()
	raw, _ := json.Marshal(request)
	if _, err := sessionInputWrite(file, raw); err != nil {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, err
	}
	if err := sessionInputSync(file); err != nil {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, err
	}
	before, _ := controlSnapshotFn(taskDir)
	reply, err := startControlledRunWait(taskDir, cwd, "", []string{
		"--only", "milestone",
		"--milestone-request", file.Name(),
		"--milestone-request-digest", fmt.Sprintf("%x", sha256.Sum256(raw)),
		"--expect-revision", fmt.Sprint(request.Revision),
	}, true)
	reply.MilestoneControl = milestoneControlReceipt(reply.State, before.State, request)
	if err != nil && reply.MilestoneControl != nil {
		message := err.Error()
		if reply.State != nil && reply.State.Control != nil && strings.TrimSpace(reply.State.Control.Message) != "" {
			message = reply.State.Control.Message
		}
		reply.MilestoneControl.Applied = false
		reply.MilestoneControl.Message = message
	}
	return reply, err
}

func loadMilestoneInput(taskDir, path, digest string) (*machine.MilestoneCommand, error) {
	root, err := filepath.EvalSymlinks(taskDir)
	if err != nil {
		return nil, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(root, ".control")
	if parent != directory || !strings.HasPrefix(filepath.Base(path), "milestone-request-") {
		return nil, fmt.Errorf("milestone input must belong to this task's control directory")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("milestone input must be a private ordinary file")
	}
	file, err := sessionInputOpen(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, opened) {
		return nil, fmt.Errorf("milestone input identity changed")
	}
	raw, err := sessionInputRead(io.LimitReader(file, 131073))
	if err != nil {
		return nil, err
	}
	if digest == "" || fmt.Sprintf("%x", sha256.Sum256(raw)) != digest {
		return nil, fmt.Errorf("milestone input content changed")
	}
	request, err := readMilestoneCommand(strings.NewReader(string(raw)))
	if err == nil {
		_ = os.Remove(path)
	}
	return request, err
}
