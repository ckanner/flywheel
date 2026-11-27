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

const packetUserActionEnv = "FLYWHEEL_PACKET_USER_ACTION"

// PacketControlReceipt reports the verified semantic transition of a packet
// triage command (`retry` | `replan` | `accept`).
type PacketControlReceipt struct {
	PacketID string `json:"packet-id"`
	Action   string `json:"action"`
	Applied  bool   `json:"applied"`
	Revision int64  `json:"revision"`
	Status   string `json:"status,omitempty"`
	Message  string `json:"message,omitempty"`
}

func readPacketCommand(reader io.Reader) (*machine.PacketCommand, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, 131073))
	if err != nil {
		return nil, err
	}
	if len(raw) > 131072 || !json.Valid(raw) {
		return nil, fmt.Errorf("invalid or oversized packet command")
	}
	var request machine.PacketCommand
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return nil, err
	}
	return &request, request.Validate()
}

func packetUserActionFromEnv(request *machine.PacketCommand, taskDir string) (*machine.PacketUserAction, error) {
	raw, present := os.LookupEnv(packetUserActionEnv)
	if !present {
		return nil, fmt.Errorf("packet triage requires a verified host command or controlling terminal")
	}
	if len(raw) > 8192 || !json.Valid([]byte(raw)) {
		return nil, fmt.Errorf("packet triage proof is malformed or oversized")
	}
	var proof machine.PacketUserAction
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proof); err != nil || proof.Kind != "dsh" ||
		proof.SessionID == "" || len(proof.SessionID) > 200 || proof.SessionID != os.Getenv("DSH_SESSION_ID") ||
		proof.CommandID == "" || len(proof.CommandID) > 200 || strings.ContainsAny(proof.SessionID+proof.CommandID, "\x00\r\n") ||
		proof.Action != request.Action || proof.PacketID != request.PacketID || proof.Revision != request.Revision ||
		proof.Reason != request.Reason || proof.Guidance != request.Guidance || proof.TaskDir != taskDir {
		return nil, fmt.Errorf("packet triage proof does not match this exact host command")
	}
	return &proof, nil
}

func packetUserActionFromTTY(request *machine.PacketCommand, taskDir string) (*machine.PacketUserAction, error) {
	terminal, err := openPlanningTTY()
	if err != nil {
		return nil, fmt.Errorf("packet triage requires a verified host command or an interactive controlling terminal")
	}
	defer terminal.Close()
	if _, err := fmt.Fprintf(terminal, "Packet: %s\nAction: %s\nReason: %s\nType '%s %s' to confirm this exact triage decision: ",
		request.PacketID, request.Action, request.Reason, request.Action, request.PacketID); err != nil {
		return nil, fmt.Errorf("cannot present the exact packet triage confirmation: %w", err)
	}
	line, err := bufio.NewReader(io.LimitReader(terminal, 512)).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != request.Action+" "+request.PacketID {
		return nil, fmt.Errorf("the controlling terminal did not confirm this exact packet triage decision")
	}
	return &machine.PacketUserAction{
		Kind:     "tty",
		TaskDir:  taskDir,
		PacketID: request.PacketID,
		Revision: request.Revision,
		Action:   request.Action,
		Reason:   request.Reason,
		Guidance: request.Guidance,
	}, nil
}

func packetActionProof(request *machine.PacketCommand, taskDir string, allowTTY bool) error {
	proof, err := packetUserActionFromEnv(request, taskDir)
	if err == nil {
		request.UserAction = proof
		return nil
	}
	if !allowTTY {
		return err
	}
	proof, err = packetUserActionFromTTY(request, taskDir)
	if err != nil {
		return err
	}
	request.UserAction = proof
	return nil
}

func packetControlReceipt(st, before *state.State, request *machine.PacketCommand) *PacketControlReceipt {
	receipt := &PacketControlReceipt{PacketID: request.PacketID, Action: request.Action}
	if st == nil {
		receipt.Message = "task state is unavailable"
		return receipt
	}
	receipt.Revision = st.Revision
	ps := st.Packets[request.PacketID]
	if ps == nil {
		receipt.Message = "packet not found"
		return receipt
	}
	receipt.Status = ps.Status
	var beforePs *state.PacketState
	if before != nil {
		beforePs = before.Packets[request.PacketID]
	}
	if beforePs == nil {
		receipt.Message = "the packet command's effect could not be verified because pre-command state is unavailable; refresh status"
		return receipt
	}
	switch request.Action {
	case "retry":
		receipt.Applied = beforePs.Status == "inbox" && ps.Status == "pending" && st.Revision > before.Revision
		if !receipt.Applied {
			receipt.Message = "the inbox packet was not re-armed for retry"
		}
	case "replan":
		receipt.Applied = beforePs.Status == "inbox" && ps.Status == "pending" && ps.PlanRevisions > beforePs.PlanRevisions
		if !receipt.Applied {
			receipt.Message = "the inbox packet replan did not complete"
		}
	default: // accept
		receipt.Applied = beforePs.Status == "inbox" && ps.Status == "completed" &&
			ps.HumanSignoff != nil && ps.HumanSignoff.Reason == request.Reason
		if !receipt.Applied {
			receipt.Message = "the inbox packet was not signed off and completed"
		}
	}
	return receipt
}

func startPacketControl(cmd *cobra.Command, taskDir, cwd string) (controlResponse, error) {
	request, err := readPacketCommand(cmd.InOrStdin())
	if err != nil {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, err
	}
	if err := packetActionProof(request, taskDir, false); err != nil {
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
	file, err := sessionInputTemp(directory, "packet-request-*")
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
	quick := request.Action == "retry" || request.Action == "accept"
	before, _ := controlSnapshotFn(taskDir)
	reply, err := startControlledRunWait(taskDir, cwd, "", []string{
		"--only", "packet",
		"--packet-request", file.Name(),
		"--packet-request-digest", fmt.Sprintf("%x", sha256.Sum256(raw)),
		"--expect-revision", fmt.Sprint(request.Revision),
	}, quick)
	if !quick && err == nil && reply.Running {
		rev := request.Revision
		if reply.State != nil {
			rev = reply.State.Revision
		}
		reply.PacketControl = &PacketControlReceipt{
			PacketID: request.PacketID,
			Action:   request.Action,
			Applied:  true,
			Revision: rev,
			Status:   "running",
			Message:  "packet replan started in background",
		}
		return reply, nil
	}
	reply.PacketControl = packetControlReceipt(reply.State, before.State, request)
	if err != nil && reply.PacketControl != nil {
		message := err.Error()
		if reply.State != nil && reply.State.Control != nil && strings.TrimSpace(reply.State.Control.Message) != "" {
			message = reply.State.Control.Message
		}
		reply.PacketControl.Applied = false
		reply.PacketControl.Message = message
	}
	return reply, err
}

func loadPacketInput(taskDir, path, digest string) (*machine.PacketCommand, error) {
	root, err := filepath.EvalSymlinks(taskDir)
	if err != nil {
		return nil, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(root, ".control")
	if parent != directory || !strings.HasPrefix(filepath.Base(path), "packet-request-") {
		return nil, fmt.Errorf("packet input must belong to this task's control directory")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("packet input must be a private ordinary file")
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
		return nil, fmt.Errorf("packet input identity changed")
	}
	raw, err := sessionInputRead(io.LimitReader(file, 131073))
	if err != nil {
		return nil, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != digest {
		return nil, fmt.Errorf("packet input content changed")
	}
	request, err := readPacketCommand(strings.NewReader(string(raw)))
	if err == nil {
		err = os.Remove(path)
	}
	return request, err
}
