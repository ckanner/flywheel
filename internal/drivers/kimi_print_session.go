package drivers

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
)

var kimiPrintSessionPreparer = prepareKimiPrintSession

// Kimi preparation runs before any inference. Deterministic compatibility,
// schema or identity failures must not be retried as transport faults; only
// startup/registry races remain transport-retryable.
func kimiPrintPreparationErrorCode(code string) errs.ErrorCode {
	switch code {
	case string(errs.TIMEOUT):
		return errs.TIMEOUT
	case "RUNTIME_AUTH":
		return errs.AUTH
	case "RUNTIME_IDENTITY", "RUNTIME_PROTOCOL", "UNSUPPORTED_SCHEMA", "INVALID_REQUEST":
		return errs.INVALID_REQUEST
	default:
		return errs.TRANSPORT
	}
}

// Directory removal does not make Getwd fail consistently across platforms.
// Keep the resolution boundary injectable so its failure can be tested directly.
var kimiPrintSessionAbs = filepath.Abs

// Creating title metadata is deliberately separate from sending a business
// prompt. A failed/unknown preparation can never silently run an unnamed task.
func prepareKimiPrintSession(a KimiArgs) Result {
	id := a.RequestID
	if id == "" {
		id = "print-title-" + rand.Text()
	}
	model, effort := a.Model, a.Thinking
	if model == "" {
		model = KimiDefaultModel
	}
	if effort == "" {
		effort = "high"
	}
	wall := a.Watchdog.WallTimeoutMs
	if wall <= 0 {
		wall = watchdog.DefaultWallTimeoutMs
	}
	cwd, err := kimiPrintSessionAbs(a.Cwd)
	if err != nil {
		out := invalidHarnessResult("kimi", model, "Kimi print-session workspace cannot be resolved")
		out.Session = &SessionOutcome{Protocol: "kimi-print/v1", RequestID: a.RequestID, Dispatch: "not-dispatched", Outcome: "failed", CleanupConfirmed: true, UsageStatus: "complete"}
		out.Usage, out.CostUSD = &Usage{}, new(float64)
		return out
	}
	request := struct {
		Type            string `json:"type"`
		Protocol        int    `json:"protocol"`
		RequestID       string `json:"requestId"`
		Cwd             string `json:"cwd"`
		Model           string `json:"model"`
		ReasoningEffort string `json:"reasoningEffort"`
		SessionTitle    string `json:"sessionTitle"`
		TimeoutMS       int64  `json:"timeoutMs"`
	}{"prepare-print", 1, id, cwd, model, effort, a.SessionTitle, wall}
	payload, _ := json.Marshal(request) // this struct has only JSON scalar fields
	env := childenv.BuildChildEnv(childenv.OSLookupEnv, a.APIKeys)
	for _, key := range []string{KimiCommandEnv, "KIMI_CODE_HOME", "FLYWHEEL_RESOURCE_DIR"} {
		if value, exists := os.LookupEnv(key); exists {
			env[key] = value
		}
	}
	if a.Command != "" {
		env[KimiCommandEnv] = a.Command
	}
	env["KIMI_MODEL_THINKING_EFFORT"] = effort
	physical, writeErr := runMachineWorker(KimiNativeCommand(), "kimi", codexProStart{Protocol: 1}, append(payload, '\n'), watchdog.Opts{
		Context: a.Watchdog.Context, Cwd: a.Cwd, Env: childenv.EnvSlice(env), DisableIdleTimeout: true,
		WallTimeoutMs: wall, KillGraceMs: max(a.Watchdog.KillGraceMs, 15000),
	}, nil, nil)
	var receipt struct {
		Protocol            int    `json:"protocol"`
		Type                string `json:"type"`
		RequestID           string `json:"requestId"`
		Status              string `json:"status"`
		SessionID           string `json:"sessionId"`
		SessionTitle        string `json:"sessionTitle"`
		CleanupConfirmed    *bool  `json:"cleanupConfirmed"`
		InferenceDispatched *bool  `json:"inferenceDispatched"`
		Error               *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	out := fromWatchdog(physical, "kimi", "kimi", model)
	trusted := json.Unmarshal([]byte(physical.Stdout), &receipt) == nil && writeErr == nil && !out.Truncated &&
		receipt.Protocol == 1 && receipt.Type == "prepared-print" && receipt.RequestID == id &&
		(receipt.Status == "ok" || receipt.Status == "error" || receipt.Status == "cancelled") &&
		receipt.CleanupConfirmed != nil && receipt.InferenceDispatched != nil && !*receipt.InferenceDispatched
	cleanup := trusted && *receipt.CleanupConfirmed && out.CleanupError == "" && (out.ExitCode != nil || out.Signal != "")
	if out.SpawnError != "" || (out.Cancelled && out.ExitCode == nil && out.Signal == "") {
		cleanup = true
	}
	out.Session = &SessionOutcome{Protocol: "kimi-print/v1", RequestID: a.RequestID, Dispatch: "not-dispatched",
		Outcome: "failed", CleanupConfirmed: cleanup, UsageStatus: "complete"}
	out.Usage, out.CostUSD = &Usage{}, new(float64)
	validID := strings.TrimSpace(receipt.SessionID) != "" && len(receipt.SessionID) <= 512 && !strings.ContainsAny(receipt.SessionID, "\x00\r\n")
	if trusted {
		if validID {
			out.Session.SessionID = receipt.SessionID
		}
		mergeWorkerTermination(&out, receipt.Status == "cancelled", receipt.Error != nil && receipt.Error.Code == "TIMEOUT", false)
	}
	if !cleanup && out.CleanupError == "" {
		out.CleanupError = "Kimi print-session preparation cleanup unconfirmed"
	}
	out.Settled = trusted && cleanup && receipt.Status == "ok" && receipt.Error == nil && receipt.SessionTitle == a.SessionTitle &&
		validID &&
		!out.Cancelled && !out.TimedOut && out.Signal == "" && out.SpawnError == "" && out.ExitCode != nil && *out.ExitCode == 0
	if out.Settled {
		out.SessionID = receipt.SessionID
	} else {
		failureCode := ""
		if trusted && receipt.Error != nil {
			failureCode = receipt.Error.Code
		}
		message := "Kimi could not prepare a named print session; the task prompt was not sent"
		if failureCode != "" {
			message = "Kimi print-session preparation failed (" + failureCode + "); the task prompt was not sent"
		}
		out.ErrorCode = kimiPrintPreparationErrorCode(failureCode)
		if out.TimedOut {
			out.ErrorCode = errs.TIMEOUT
			message = timeoutMessage("Kimi print-session preparation", out)
		}
		out.ErrorMessage = &message
	}
	return out
}
