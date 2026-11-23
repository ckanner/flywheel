package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/investigation"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/spf13/cobra"
)

type investigationRequest struct {
	Action    string                     `json:"action"`
	Run       *bool                      `json:"run,omitempty"`
	Revision  int64                      `json:"revision,omitempty"`
	Contract  json.RawMessage            `json:"contract,omitempty"`
	Goal      string                     `json:"goal,omitempty"`
	Intent    string                     `json:"intent,omitempty"`
	ReadPaths []string                   `json:"read-paths,omitempty"`
	Report    *state.InvestigationReport `json:"report,omitempty"`
}

func sanitizeInvestigationID(taskDir string) string {
	base := filepath.Base(taskDir)
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && (r == '_' || r == '.' || r == '-'):
			b.WriteRune(r)
		case b.Len() > 0:
			b.WriteByte('-')
		}
		if b.Len() >= 120 {
			break
		}
	}
	if b.Len() == 0 {
		return "investigation"
	}
	return b.String()
}

func synthesizeInvestigationContract(taskDir, goal, intent string, readPaths []string) ([]byte, error) {
	goal = strings.TrimSpace(goal)
	if len(goal) > 2000 {
		return nil, fmt.Errorf("investigation goal exceeds 2000 bytes")
	}
	if intent != "" && intent != "investigate" {
		return nil, fmt.Errorf("shorthand goal supports read-only investigate intent only")
	}
	if len(readPaths) == 0 {
		readPaths = []string{"."}
	}
	p := contract.Preparation{
		Version:             1,
		ID:                  sanitizeInvestigationID(taskDir),
		Goal:                goal,
		Expected:            "Root cause and evidence-backed findings for: " + goal,
		Intent:              "investigate",
		Environment:         "local workspace",
		Window:              "current checkout",
		KnownEvidence:       []string{},
		ReadPaths:           readPaths,
		ImplementationScope: []string{},
		Hypotheses: []contract.Hypothesis{{
			ID:        "H-01",
			Claim:     "Primary hypothesis for: " + goal,
			Falsifier: "Direct repository evidence disproving this hypothesis.",
		}},
		Actions: []contract.InvestigationAction{{
			ID:         "A-01",
			Kind:       "list_files",
			Path:       readPaths[0],
			Purpose:    "Survey initial repository scope before deeper read-only checks.",
			Hypotheses: []string{"H-01"},
		}},
		Limits: contract.InvestigationLimits{
			MaxActions:     8,
			MaxSeconds:     120,
			MaxFileBytes:   262144,
			MaxOutputBytes: 16384,
		},
	}
	raw, _ := json.Marshal(p)
	return raw, nil
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
	hasContract := len(request.Contract) > 0
	hasGoal := strings.TrimSpace(request.Goal) != ""
	hasShorthand := hasGoal || request.Intent != "" || len(request.ReadPaths) > 0
	if hasShorthand && (request.Action != "start" || hasContract || !hasGoal) {
		return reply, fmt.Errorf("goal shorthand is only valid for start without an explicit contract")
	}
	contractMissing := request.Action == "start" && !hasContract && !hasGoal || request.Action == "continue" && !hasContract
	contractExtra := request.Action != "start" && request.Action != "continue" && hasContract
	if contractMissing || contractExtra || (request.Action == "report") != (request.Report != nil) {
		return reply, fmt.Errorf("contract or goal is required only for start, contract for continue, and report only for report")
	}
	if request.Run != nil && request.Action != "start" {
		return reply, fmt.Errorf("run is only valid when creating an investigation")
	}
	contractRaw := request.Contract
	if request.Action == "start" && hasGoal {
		contractRaw, err = synthesizeInvestigationContract(taskDir, request.Goal, request.Intent, request.ReadPaths)
		if err != nil {
			return reply, err
		}
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
		reply.Preparation, err = investigation.Start(taskDir, cwd, contractRaw, opts)
		if err == nil && request.Run != nil && *request.Run {
			release()
			release = nil
			return startControlledRun(taskDir, cwd, "", []string{"--investigate", "--expect-revision", fmt.Sprint(reply.Preparation.Revision)})
		}
	case "continue":
		reply.Preparation, err = investigation.Continue(taskDir, contractRaw, opts)
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
