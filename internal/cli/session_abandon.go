package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ckanner/flywheel/internal/machine"
)

const sessionUserActionEnv = "FLYWHEEL_SESSION_USER_ACTION"

func sessionUserActionFromEnv(request *machine.SessionCommand) (*machine.SessionUserAction, error) {
	raw, present := os.LookupEnv(sessionUserActionEnv)
	if !present {
		return nil, fmt.Errorf("session abandonment requires a verified host command or controlling terminal")
	}
	if len(raw) > 4096 || !json.Valid([]byte(raw)) {
		return nil, fmt.Errorf("session abandonment proof is malformed or oversized")
	}
	var proof machine.SessionUserAction
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proof); err != nil || proof.Kind != "dsh" ||
		proof.SessionID == "" || len(proof.SessionID) > 200 || proof.SessionID != os.Getenv("DSH_SESSION_ID") ||
		proof.CommandID == "" || len(proof.CommandID) > 200 || strings.ContainsAny(proof.SessionID+proof.CommandID, "\x00\r\n") ||
		proof.Action != "abandon" || proof.Key != request.Key || proof.Revision != request.Revision || proof.Reason != request.Reason {
		return nil, fmt.Errorf("session abandonment proof does not match this exact host command")
	}
	return &proof, nil
}

func sessionUserActionFromTTY(request *machine.SessionCommand, record *machine.SessionHistory) (*machine.SessionUserAction, error) {
	terminal, err := openPlanningTTY()
	if err != nil {
		return nil, fmt.Errorf("session abandonment requires a verified host command or an interactive controlling terminal")
	}
	defer terminal.Close()
	invocation := record.Invocation
	if invocation == "" {
		return nil, fmt.Errorf("the selected session has no recorded invocation to abandon")
	}
	if _, err := fmt.Fprintf(terminal, "Session: %s\nInvocation: %s\nRecorded status: blocked (dispatch/outcome unknown)\nReason: %s\nType 'abandon %s' to retire this exact invocation without replay: ",
		request.Key, invocation, request.Reason, invocation); err != nil {
		return nil, fmt.Errorf("cannot present the exact invocation for abandonment: %w", err)
	}
	line, err := bufio.NewReader(io.LimitReader(terminal, 512)).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "abandon "+invocation {
		return nil, fmt.Errorf("the controlling terminal did not confirm this exact invocation")
	}
	return &machine.SessionUserAction{Kind: "tty", Key: request.Key, Revision: request.Revision, Action: "abandon", Reason: request.Reason}, nil
}

// sessionActionProof attaches the proof to an abandon command. The control
// boundary uses only the host-generated environment proof; the human CLI adds
// an interactive confirmation on the controlling terminal.
func sessionActionProof(request *machine.SessionCommand, record *machine.SessionHistory, allowTTY bool) error {
	if request.Action != "abandon" {
		return nil
	}
	proof, err := sessionUserActionFromEnv(request)
	if err == nil {
		request.UserAction = proof
		return nil
	}
	if !allowTTY {
		return err
	}
	proof, err = sessionUserActionFromTTY(request, record)
	if err != nil {
		return err
	}
	request.UserAction = proof
	return nil
}
