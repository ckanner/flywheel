package drivers

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/watchdog"
)

const DSHWebRecoveryVersion = "chatgpt-web/read-only-recovery/1"

type DSHWebRecoveryArgs struct {
	RequestID, RecoveryDirectory, Cwd, WorkspaceCwd, Provider, PermissionMode, RequestedSessionID, ExecutionDomainID string
	Native                                                                                                           json.RawMessage
	ExpectedBinding                                                                                                  *DSHWebBinding
	Watchdog                                                                                                         watchdog.Opts
}

type dshWebRecoveryOriginal struct {
	Cwd                string          `json:"cwd"`
	WorkspaceCwd       string          `json:"workspaceCwd"`
	Provider           string          `json:"provider"`
	Model              string          `json:"model"`
	ReasoningEffort    string          `json:"reasoningEffort"`
	PermissionMode     string          `json:"permissionMode"`
	Persistent         bool            `json:"persistent"`
	RequestedSessionID string          `json:"requestedSessionId"`
	ExecutionDomainID  string          `json:"executionDomainId"`
	Native             json.RawMessage `json:"native"`
}

type dshWebRecoveryRequest struct {
	Type              string                 `json:"type"`
	Protocol          int                    `json:"protocol"`
	RequestID         string                 `json:"requestId"`
	RecoveryDirectory string                 `json:"recoveryDirectory"`
	Original          dshWebRecoveryOriginal `json:"original"`
	ExpectedBinding   *DSHWebBinding         `json:"expectedBinding,omitempty"`
}

var encodeDSHWebRecovery = (*json.Encoder).Encode

func parseDSHWebRecovery(raw string, request dshWebRecoveryRequest) (Result, error) {
	var envelope struct {
		Type      string            `json:"type"`
		Protocol  int               `json:"protocol"`
		RequestID string            `json:"requestId"`
		Revision  string            `json:"revision"`
		Frames    []json.RawMessage `json:"frames"`
	}
	decodeErr := json.Unmarshal([]byte(raw), &envelope)
	_, digestErr := hex.DecodeString(envelope.Revision)
	if decodeErr != nil || envelope.Type != "web-recovery" || envelope.Protocol != 1 ||
		envelope.RequestID != request.RequestID || digestErr != nil || len(envelope.Revision) != 64 || len(envelope.Frames) != 2 {
		return Result{}, fmt.Errorf("Web read-only recovery envelope or revision is invalid")
	}
	a := request.Original
	original := dshStart{Type: "start", Protocol: 2, RequestID: request.RequestID, Cwd: a.Cwd, WorkspaceCwd: a.WorkspaceCwd,
		Provider: a.Provider, Model: a.Model, ReasoningEffort: a.ReasoningEffort, PermissionMode: a.PermissionMode, Persistent: true,
		SessionID: a.RequestedSessionID, Native: a.Native, Web: &DSHWebRequest{Version: 1, ExecutionDomainID: a.ExecutionDomainID, RequireIsolation: true, ExpectedBinding: request.ExpectedBinding}}
	parsed := parseDSHStream(string(envelope.Frames[0])+"\n"+string(envelope.Frames[1])+"\n", original)
	if parsed.Session == nil || !parsed.CleanupConfirmed || parsed.Session.Outcome == "unknown" || len(parsed.WebReceipt) == 0 {
		return Result{}, fmt.Errorf("Web read-only recovery lacks a complete original terminal: %s", parsed.Error)
	}
	exit := 0
	out := Result{Harness: "dsh", Provider: a.Provider, Model: a.Model, Recovered: true, ExitCode: &exit,
		Session: parsed.Session, SessionID: parsed.SessionID, Text: parsed.Text, WebReceipt: parsed.WebReceipt,
		NativeReceipt: parsed.NativeReceipt, Settled: true, Cancelled: parsed.Cancelled, ErrorCode: parsed.ErrorCode}
	if parsed.Error != "" {
		exit = 1
		out.ErrorMessage = &parsed.Error
	}
	return out, nil
}

// RecoverDSHWeb starts only a public file-evidence query. It cannot initialize
// an SDK, connect a browser, adopt a response or send model/tool work.
func RecoverDSHWeb(a DSHWebRecoveryArgs) (Result, error) {
	if !machineOpaqueID(a.RequestID) || !filepath.IsAbs(a.RecoveryDirectory) || strings.ContainsRune(a.RecoveryDirectory, 0) ||
		!filepath.IsAbs(a.Cwd) || !dshWebString(a.Provider) || !dshWebString(a.ExecutionDomainID) ||
		(a.WorkspaceCwd != "" && !filepath.IsAbs(a.WorkspaceCwd)) || (a.RequestedSessionID != "" && !machineOpaqueID(a.RequestedSessionID)) ||
		(a.PermissionMode != "read-only" && a.PermissionMode != "workspace-write" && a.PermissionMode != "danger-full-access") {
		return Result{}, fmt.Errorf("invalid original Web invocation for read-only recovery")
	}
	request := dshWebRecoveryRequest{Type: "web-recover", Protocol: 1, RequestID: a.RequestID, RecoveryDirectory: a.RecoveryDirectory, ExpectedBinding: a.ExpectedBinding,
		Original: dshWebRecoveryOriginal{Cwd: a.Cwd, WorkspaceCwd: a.WorkspaceCwd, Provider: a.Provider, Model: DSHWebModel, ReasoningEffort: "pro",
			PermissionMode: a.PermissionMode, Persistent: true, RequestedSessionID: a.RequestedSessionID, ExecutionDomainID: a.ExecutionDomainID, Native: a.Native}}
	env := childenv.BuildChildEnv(childenv.OSLookupEnv, nil)
	for _, key := range []string{"FLYWHEEL_DSH_BIN", "FLYWHEEL_DSH_PROFILE", "DSH_HOME"} {
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}
	written := make(chan error, 1)
	result := watchdog.Run(DSHCommand(), nil, watchdog.Opts{Context: a.Watchdog.Context,
		Env: childenv.EnvSlice(env), IdleTimeoutMs: 10_000, WallTimeoutMs: 10_000, KillGraceMs: 1_000,
		InteractStdin: func(stdin io.WriteCloser, finish func()) {
			defer stdin.Close() // The DSH worker requires one line followed by EOF.
			err := encodeDSHWebRecovery(json.NewEncoder(stdin), request)
			written <- err
			if err != nil {
				finish()
			}
		},
	})
	select {
	case err := <-written:
		if err != nil {
			return Result{}, fmt.Errorf("Web recovery request write failed: %w", err)
		}
	default:
	}
	if result.ExitCode == nil || *result.ExitCode != 0 || result.Signal != "" || result.TimedOut || result.Cancelled || result.Truncated || result.SpawnError != "" || result.CleanupError != "" {
		return Result{}, fmt.Errorf("Web read-only recovery query did not complete cleanly")
	}
	return parseDSHWebRecovery(result.Stdout, request)
}
