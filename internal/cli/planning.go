package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/spf13/cobra"
)

var planningFields = map[string][]string{
	"prepare":  {"action", "request-id", "brief", "start"},
	"plan":     {"action", "revision", "request-id", "scope", "scope-id", "text", "start"},
	"feedback": {"action", "revision", "request-id", "design", "text", "policy", "start"},
	"approve":  {"action", "revision", "request-id", "design", "start"},
	"status":   {"action", "request-id"},
	"migrate":  {"action", "revision", "request-id", "policy", "text", "start"},
}

func readPlanningCommand(input io.Reader) (machine.PlanningCommand, error) {
	var command machine.PlanningCommand
	raw, err := io.ReadAll(io.LimitReader(input, (128<<10)+1))
	if err != nil {
		return command, err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 128<<10 || !json.Valid(raw) || d.Decode(&command) != nil {
		return command, fmt.Errorf("invalid bounded planning request")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	allowed, ok := planningFields[command.Action]
	if !ok {
		return command, fmt.Errorf("unsupported planning action")
	}
	for name, value := range fields {
		if !contains(allowed, name) {
			return command, fmt.Errorf("field %s is not valid for planning %s", name, command.Action)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return command, fmt.Errorf("planning field %s cannot be null", name)
		}
	}
	return command, machine.ValidatePlanningCommand(command)
}

func planningControlDirectory(path string) (string, error) {
	resolved, err := controlDirectory(path)
	if os.IsNotExist(err) && filepath.IsAbs(path) {
		return filepath.Clean(path), nil // only prepare may create it, inside the engine
	}
	return resolved, err
}

type planningUserAction struct {
	Kind      string       `json:"kind"`
	SessionID string       `json:"sessionId"`
	CommandID string       `json:"commandId"`
	RequestID string       `json:"request-id"`
	Revision  int64        `json:"revision"`
	Design    planning.Ref `json:"design"`
	Start     bool         `json:"start"`
}

var openPlanningTTY = func() (io.ReadWriteCloser, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

func planningSource(taskDir string, request machine.PlanningCommand, design *planning.Design, review *planning.Review) (string, error) {
	if raw, present := os.LookupEnv("FLYWHEEL_PLANNING_USER_ACTION"); present {
		var proof planningUserAction
		d := json.NewDecoder(strings.NewReader(raw))
		d.DisallowUnknownFields()
		if len(raw) > 4096 || !json.Valid([]byte(raw)) || d.Decode(&proof) != nil || proof.Kind != "dsh" ||
			proof.SessionID == "" || len(proof.SessionID) > 200 || proof.SessionID != os.Getenv("DSH_SESSION_ID") ||
			strings.TrimSpace(proof.CommandID) == "" || len(proof.CommandID) > 200 || strings.ContainsAny(proof.SessionID+proof.CommandID, "\x00\r\n") ||
			proof.RequestID != request.RequestID || proof.Revision != request.Revision || request.Design == nil || proof.Design != *request.Design || proof.Start != request.StartRequested(false) {
			return "", fmt.Errorf("human planning action does not match this exact host command")
		}
		return "dsh:" + proof.SessionID + ":" + proof.CommandID, nil
	}
	terminal, err := openPlanningTTY()
	if err != nil {
		return "", fmt.Errorf("approval requires a verified DSH user command or an interactive controlling terminal")
	}
	defer terminal.Close()
	// Display the exact authoritative records, not a possibly changed export.
	// These concrete structs contain only JSON-safe fields.
	designJSON, _ := json.MarshalIndent(design, "", "  ")
	reviewJSON, _ := json.MarshalIndent(review, "", "  ")
	if _, err := fmt.Fprintf(terminal, "Authoritative design:\n%s\nIndependent review:\n%s\nExport for comparison: %s/planning/design-%06d.md\nDesign digest: %s\nStart implementation: %t\nType 'approve %s' to approve this exact design: ",
		designJSON, reviewJSON, taskDir, request.Design.Revision, request.Design.Digest, request.StartRequested(false), request.Design.Digest); err != nil {
		return "", fmt.Errorf("cannot present the exact design for approval: %w", err)
	}
	line, err := bufio.NewReader(io.LimitReader(terminal, 256)).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "approve "+request.Design.Digest {
		return "", fmt.Errorf("the controlling terminal did not confirm this exact design")
	}
	return "tty:" + request.RequestID, nil
}

func controlPlanning(cmd *cobra.Command, taskDir, cwd string) (controlResponse, error) {
	reply := controlResponse{Protocol: 1, TaskDir: taskDir, Cwd: cwd}
	request, err := readPlanningCommand(cmd.InOrStdin())
	if err != nil {
		return reply, err
	}
	if request.Action == "status" {
		if request.RequestID != "" {
			_, receipt, receiptErr := machine.ControlPlanning(machine.RunOpts{TaskDir: taskDir, Cwd: cwd}, request, "")
			snapshot, snapshotErr := controlSnapshot(taskDir)
			snapshot.PlanningControl = receipt
			return snapshot, errors.Join(receiptErr, snapshotErr)
		}
		return controlSnapshot(taskDir)
	}
	source := ""
	if request.Action == "approve" {
		st, err := state.Load(taskDir)
		if err != nil {
			return reply, err
		}
		if st.Planning == nil || st.Revision != request.Revision || !reflect.DeepEqual(st.Planning.CurrentDesign, request.Design) {
			return reply, fmt.Errorf("approval target is stale; refresh and review the exact current design")
		}
		review := planning.LatestReview(st.Planning, "design", *request.Design)
		if review == nil || review.Verdict != "pass" {
			return reply, fmt.Errorf("approval requires a passing independent review of this design")
		}
		source, err = planningSource(taskDir, request, planning.FindDesign(st.Planning, *request.Design), review)
		if err != nil {
			return reply, err
		}
	}
	cfg, err := config.LoadReadOnly(taskDir, machine.ProjectRoot(cwd))
	if err != nil {
		return reply, err
	}
	st, receipt, err := machine.ControlPlanning(machine.RunOpts{TaskDir: taskDir, Cwd: cwd, Config: cfg, Context: cmd.Context()}, request, source)
	reply.State, reply.PlanningControl = st, receipt
	if state.Exists(taskDir) {
		snapshot, snapshotErr := controlSnapshot(taskDir)
		reply = snapshot
		reply.PlanningControl = receipt
		err = errors.Join(err, snapshotErr)
	}
	if err != nil {
		return reply, err
	}
	start := request.StartRequested(request.Action == "prepare" || request.Action == "plan" || request.Action == "migrate")
	if !start || receipt != nil && receipt.Status == "queued" {
		return reply, nil
	}
	extra := []string{}
	if request.Action == "plan" || request.Action == "migrate" || request.Action == "feedback" {
		extra = []string{"--planning-only"}
	}
	reply, err = startControlledRun(taskDir, cwd, "", extra)
	reply.PlanningControl = receipt
	return reply, err
}
