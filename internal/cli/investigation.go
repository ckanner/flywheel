package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/ckanner/flywheel/internal/investigation"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/spf13/cobra"
)

type investigationRequest struct {
	Action   string                     `json:"action"`
	Run      *bool                      `json:"run,omitempty"`
	Revision int64                      `json:"revision,omitempty"`
	Contract json.RawMessage            `json:"contract,omitempty"`
	Report   *state.InvestigationReport `json:"report,omitempty"`
}

func controlInvestigation(cmd *cobra.Command, taskDir, cwd string) (controlResponse, error) {
	reply := controlResponse{Protocol: 1, TaskDir: taskDir, Cwd: cwd}
	raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), (128<<10)+1))
	if err != nil {
		return reply, err
	}
	var request investigationRequest
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 128<<10 || d.Decode(&request) != nil || !json.Valid(raw) {
		return reply, fmt.Errorf("invalid bounded investigation request")
	}
	if request.Action != "start" && request.Action != "status" && request.Revision < 1 {
		return reply, fmt.Errorf("investigation mutation requires the observed revision")
	}
	if (request.Action == "start" || request.Action == "continue") != (len(request.Contract) > 0) || (request.Action == "report") != (request.Report != nil) {
		return reply, fmt.Errorf("contract is required only for start/continue; report only for report")
	}
	if request.Run != nil && request.Action != "start" {
		return reply, fmt.Errorf("run is only valid when creating an investigation")
	}
	if request.Action != "start" {
		if _, err := controlDirectory(taskDir); err != nil {
			return reply, err
		}
	}
	opts := investigation.Options{ExpectedRevision: request.Revision}
	var release func()
	if request.Action != "status" && request.Action != "run" {
		if request.Action == "start" {
			if err := os.MkdirAll(taskDir, 0o700); err != nil {
				return reply, err
			}
		}
		var leaseErr error
		release, leaseErr = machine.AcquireRunLease(taskDir)
		if leaseErr != nil {
			return reply, leaseErr
		}
		defer func() {
			if release != nil {
				release()
			}
		}()
	}
	switch request.Action {
	case "start":
		reply.Preparation, err = investigation.Start(taskDir, cwd, request.Contract, opts)
		if err == nil && request.Run != nil && *request.Run {
			release()
			release = nil
			return startControlledRun(taskDir, cwd, "", []string{"--investigate", "--expect-revision", fmt.Sprint(reply.Preparation.Revision)})
		}
	case "continue":
		reply.Preparation, err = investigation.Continue(taskDir, request.Contract, opts)
	case "status":
		return controlSnapshot(taskDir)
	case "run":
		return startControlledRun(taskDir, cwd, "", []string{"--investigate", "--expect-revision", fmt.Sprint(request.Revision)})
	case "report":
		reply.Preparation, err = investigation.Finish(taskDir, *request.Report, opts)
	case "cancel":
		reply.Preparation, err = investigation.Cancel(taskDir, opts)
	default:
		err = fmt.Errorf("investigation action must be start, continue, run, status, report, or cancel")
	}
	return reply, err
}
