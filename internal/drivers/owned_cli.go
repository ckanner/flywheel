package drivers

import (
	"encoding/json"
	"fmt"
	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
	"strings"
)

func unknownOwnedCLISession(harness string, request codexProStart) *SessionOutcome {
	return &SessionOutcome{Protocol: fmt.Sprintf("%s/%s/worker-v%d", harness, machineAdapterVersion(harness), request.Protocol), RequestID: request.RequestID, Dispatch: "unknown", Outcome: "unknown", UsageStatus: "unknown"}
}
func parseOwnedCLIStream(stdout string, request codexProStart, harness, label string, parseReceipt func(json.RawMessage, codexProStart) claudeCodeProParsed) claudeCodeProParsed {
	parsed := claudeCodeProParsed{Session: unknownOwnedCLISession(harness, request)}
	lifecycle := machineLifecycleFlow{harness: harness, request: codexProStart(request)}
	terminal, trusted := false, true
	var nativeSequence, activitySequence int64
	for _, line := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var frame struct {
			Protocol  int     `json:"protocol"`
			Type      string  `json:"type"`
			RequestID string  `json:"requestId"`
			Text      *string `json:"text"`
			Adapter   struct {
				Package  string `json:"package"`
				Version  string `json:"version"`
				Protocol int    `json:"protocol"`
			} `json:"adapter"`
			Receipt json.RawMessage `json:"receipt"`
			Error   *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			Admission *struct {
				Dispatched       *bool `json:"dispatched"`
				CleanupConfirmed *bool `json:"cleanupConfirmed"`
			} `json:"admission"`
		}
		if err := json.Unmarshal([]byte(line), &frame); err != nil || frame.Protocol != request.Protocol ||
			frame.RequestID != request.RequestID || terminal {
			trusted = false
			parsed.Error = "invalid request: " + label + " worker emitted malformed, mismatched or post-terminal data"
			break
		}
		switch frame.Type {
		case "activity":
			if err := validateProActivityEvent(json.RawMessage(line), harness, request, &activitySequence); err != nil {
				trusted = false
				parsed.Error = err.Error()
			}
		case "native-event":
			if err := validateProNativeEvent(json.RawMessage(line), harness, request, &nativeSequence); err != nil {
				trusted = false
				parsed.Error = err.Error()
			}
		case "lifecycle":
			if _, err := lifecycle.frame([]byte(line)); err != nil || request.Protocol != 3 {
				trusted = false
				parsed.Error = "invalid request: " + label + " lifecycle evidence is invalid"
			}
		case "progress":
			if frame.Text == nil {
				trusted = false
				parsed.Error = "invalid request: " + label + " progress omitted text"
			}
		case "result":
			terminal = true
			parsed = parseReceipt(frame.Receipt, request)
			if request.Protocol == 3 && parsed.Settled && !machineLifecycleProvesResult(lifecycle.last, parsed.Session) {
				trusted = false
				parsed.Error = "invalid request: " + label + " result omitted durable lifecycle evidence"
			}
			if !receiptHasFields(json.RawMessage(line), "receipt", "error") || frame.Adapter.Package != adapterPackageFor(harness) ||
				frame.Adapter.Version != machineAdapterVersion(harness) || frame.Adapter.Protocol != 1 {
				trusted = false
				parsed.Error = "invalid request: " + label + " terminal envelope does not match the pinned adapter contract"
			} else if frame.Admission != nil {
				if string(frame.Receipt) != "null" || frame.Error == nil || frame.Admission.Dispatched == nil || *frame.Admission.Dispatched ||
					frame.Admission.CleanupConfirmed == nil || !*frame.Admission.CleanupConfirmed || !claudeCodeProAdmissionError(frame.Error.Code) {
					trusted = false
					parsed.Error = "invalid request: " + label + " admission contradicts its terminal envelope"
				} else {
					parsed.CleanupConfirmed = true
					parsed.Session.Dispatch, parsed.Session.Outcome, parsed.Session.UsageStatus = "not-dispatched", "failed", "complete"
					parsed.Session.SessionID, parsed.Usage = request.ThreadID, &Usage{}
					parsed.Error, parsed.ErrorCode = label+" worker admission failed: "+frame.Error.Message, errs.INVALID_REQUEST
				}
			} else if frame.Error != nil {
				if !machineWorkerObservationError(frame.Error.Code) {
					trusted = false
				}
				// The error report and the independently validated cleanup
				// receipt answer different questions. Neither grants a checkpoint.
				parsed.Cancelled, parsed.TimedOut, parsed.Truncated, parsed.Settled = false, false, false, false
				parsed.Session.Outcome, parsed.Session.ResumeCandidate = "unknown", false
				parsed.ErrorCode = errs.TRANSPORT
				parsed.Error = label + " worker failed without a trusted invocation result: " + frame.Error.Message
			}
		default:
			trusted = false
			parsed.Error = "invalid request: " + label + " worker emitted an unknown frame"
		}
		if !trusted {
			break
		}
	}
	if !terminal && parsed.Error == "" {
		parsed.Error = label + " worker ended without a terminal receipt"
	}
	if !trusted {
		parsed.StreamFailed = true
		parsed.CleanupConfirmed, parsed.Settled, parsed.Usage, parsed.ErrorCode = false, false, nil, ""
		parsed.Cancelled, parsed.TimedOut, parsed.Truncated = false, false, false
		parsed.Session = unknownOwnedCLISession(harness, request)
	}
	parsed.Session.CleanupConfirmed = parsed.CleanupConfirmed
	return parsed
}

func runOwnedCLIWorker(a ClaudeCodeProArgs, request codexProStart, command string, env map[string]string, harness, label string, parseReceipt func(json.RawMessage, codexProStart) claudeCodeProParsed) Result {
	payload, _ := json.Marshal(request)
	payload = append(payload, '\n')
	result, interactionErr := runMachineWorker(command, harness, request, payload, watchdog.Opts{
		Context: a.Watchdog.Context, Cwd: request.Cwd, Env: childenv.EnvSlice(env),
		IdleTimeoutMs: a.Watchdog.IdleTimeoutMs, WallTimeoutMs: request.TimeoutMS,
		KillGraceMs: max(a.Watchdog.KillGraceMs, claudeCodeProCleanupMS),
	}, a.OnLifecycle, a.OnEvent)
	parsed := parseOwnedCLIStream(result.Stdout, request, harness, label, parseReceipt)
	if interactionErr != nil {
		parsed.StreamFailed = true
		parsed.Error = label + " worker transport failed: " + interactionErr.Error()
		parsed.Session, parsed.Usage, parsed.CleanupConfirmed = unknownOwnedCLISession(harness, request), nil, false
	}
	out := fromWatchdog(result, harness, "", a.Model)
	out.NativeReceipt = parsed.NativeReceipt
	out.Session, out.SessionID, out.Text, out.Usage = parsed.Session, parsed.Session.SessionID, parsed.Text, parsed.Usage
	if harness == "antigravity" {
		out.AntigravityReceipt = parsed.Receipt
	} else {
		out.ClaudeCodeProReceipt = parsed.Receipt
	}
	// Preserve a real caller cancellation, but do not relabel a known worker
	// protocol/ACK failure as a user stop just because cleanup used abort.
	providerFactsTrusted := !parsed.StreamFailed && !result.Truncated
	parsed.CleanupConfirmed = parsed.CleanupConfirmed && providerFactsTrusted
	mergeWorkerTermination(&out, parsed.Cancelled && providerFactsTrusted, parsed.TimedOut && providerFactsTrusted, parsed.Truncated && providerFactsTrusted)
	if result.Truncated {
		out.Usage, out.Session.UsageStatus = nil, "unknown"
	}
	if (result.ExitCode != nil || result.Signal != "") && !parsed.CleanupConfirmed && out.CleanupError == "" {
		out.CleanupError = label + " process cleanup unconfirmed: no trustworthy owned-group receipt"
	}
	out.Settled = parsed.Settled && parsed.Error == "" && !out.Cancelled && !out.TimedOut && !out.Truncated && out.Signal == "" &&
		out.SpawnError == "" && out.CleanupError == "" && out.ExitCode != nil && *out.ExitCode == 0
	if result.SpawnError != "" || (result.Cancelled && result.ExitCode == nil && result.Signal == "") {
		out.Session.Dispatch, out.Session.Outcome, out.Session.CleanupConfirmed, out.Session.UsageStatus = "not-dispatched", "failed", true, "complete"
		out.Usage = &Usage{}
	} else {
		out.Session.CleanupConfirmed = parsed.CleanupConfirmed && out.CleanupError == ""
		if result.Cancelled || result.TimedOut || result.Truncated || result.Signal != "" || result.CleanupError != "" ||
			result.ExitCode == nil || (*result.ExitCode != 0 && out.Session.Outcome == "completed") {
			out.Session.Outcome, out.Session.ResumeCandidate = "unknown", false
		}
	}
	if out.Session.Dispatch == "not-dispatched" && out.Session.CleanupConfirmed {
		out.CostUSD = new(float64) // positive proof no backend process accepted work
	}
	if !out.Settled && !out.Cancelled {
		message := parsed.Error
		switch {
		case out.TimedOut:
			message = timeoutMessage(label+" worker", out)
			out.ErrorCode = errs.TIMEOUT
		case out.SpawnError != "":
			message = out.SpawnError
		case out.CleanupError != "":
			message = strings.TrimPrefix(message+"; "+out.CleanupError, "; ")
		case out.Truncated:
			message = label + " worker output was truncated"
		case message == "":
			message = label + " worker exited without successful process completion"
		}
		out.ErrorMessage = &message
		if !out.TimedOut && !out.Truncated && out.SpawnError == "" && out.CleanupError == "" && out.Signal == "" && out.ExitCode != nil && *out.ExitCode == 1 {
			out.ErrorCode = parsed.ErrorCode
		}
	}
	return out
}
