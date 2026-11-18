package drivers

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/watchdog"
)

const DSHWebModel = "gpt-6-pro-web"

// Web is an explicitly negotiated extension, not a provider-name convention.
// Old workers reject the unknown start field before sending any model input.
type DSHWebRequest struct {
	Version           int            `json:"version"`
	ExecutionDomainID string         `json:"executionDomainId"`
	RequireIsolation  bool           `json:"requireIsolation"`
	Probe             bool           `json:"probe,omitempty"`
	ExpectedBinding   *DSHWebBinding `json:"expectedBinding,omitempty"`
}

type DSHWebBinding struct {
	Protocol            int    `json:"protocol"`
	Transport           string `json:"transport"`
	Provider            string `json:"provider"`
	Model               string `json:"model"`
	ReasoningEffort     string `json:"reasoningEffort"`
	ConnectionID        string `json:"connectionId"`
	AccountFingerprint  string `json:"accountFingerprint"`
	PolicyVersion       string `json:"policyVersion"`
	ContextPolicy       string `json:"contextPolicy"`
	Observation         string `json:"observation"`
	Usage               string `json:"usage"`
	HostContractVersion int    `json:"hostContractVersion"`
	RuntimeVersion      string `json:"runtimeVersion"`
}

type DSHWebFacts struct {
	Version               int    `json:"version"`
	HostInvocationID      string `json:"hostInvocationId"`
	ExecutionDomainID     string `json:"executionDomainId"`
	SessionID             string `json:"sessionId"`
	Dispatch              string `json:"dispatch"`
	Outcome               string `json:"outcome"`
	RemoteGeneration      string `json:"remoteGeneration"`
	LocalCleanupConfirmed bool   `json:"localCleanupConfirmed"`
	UsageStatus           string `json:"usageStatus"`
}

type DSHWebReceipt struct {
	Version int           `json:"version"`
	Binding DSHWebBinding `json:"binding"`
	Facts   DSHWebFacts   `json:"facts"`
}

func dshWebString(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 512 && !strings.ContainsAny(value, "\x00\r\n")
}

func prepareDSHWeb(web *DSHWebRequest, request dshStart) (*DSHWebRequest, error) {
	if request.Model != DSHWebModel {
		if web != nil {
			return nil, fmt.Errorf("DSH Web extension requires its exact Web model")
		}
		return nil, nil
	}
	if request.ReasoningEffort != "pro" {
		return nil, fmt.Errorf("DSH Web Pro requires explicit thinking pro; no high fallback is supported")
	}
	if web == nil {
		// Direct CLI diagnostics have no task. Their stable physical workspace is
		// their conservative domain; managed tasks supply their persistent task ID.
		root := request.WorkspaceCwd
		if root == "" {
			root = request.Cwd
		}
		web = &DSHWebRequest{Version: 1, ExecutionDomainID: fmt.Sprintf("flywheel-%x", sha256.Sum256([]byte(root))), RequireIsolation: true}
	}
	if web.Version != 1 || !dshWebString(web.ExecutionDomainID) || !web.RequireIsolation {
		return nil, fmt.Errorf("DSH Web requires a stable execution domain and project-only isolation")
	}
	if web.ExpectedBinding != nil {
		if err := validateDSHWebBinding(*web.ExpectedBinding, request); err != nil {
			return nil, err
		}
	}
	detached := *web
	return &detached, nil
}

func validateDSHWebBinding(binding DSHWebBinding, request dshStart) error {
	if binding.Protocol != 1 || binding.Transport != "chatgpt-web" || binding.Provider != request.Provider ||
		binding.Model != DSHWebModel || binding.Model != request.Model || binding.ReasoningEffort != "pro" || binding.ReasoningEffort != request.ReasoningEffort ||
		binding.ContextPolicy != "project-only" || binding.Observation != "opaque" || binding.Usage != "unknown" ||
		binding.HostContractVersion != 1 || binding.RuntimeVersion != "web-runtime/1" ||
		!dshWebString(binding.ConnectionID) || !dshWebString(binding.AccountFingerprint) || !dshWebString(binding.PolicyVersion) {
		return fmt.Errorf("DSH Web binding does not attest the requested isolated model and public host contract")
	}
	return nil
}

func decodeDSHWeb(raw json.RawMessage, into any) error {
	if len(raw) == 0 || len(raw) > 16384 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("missing or oversized DSH Web receipt")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return fmt.Errorf("invalid DSH Web receipt: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("DSH Web receipt contains trailing data")
	}
	return nil
}

func validateDSHWebReady(raw json.RawMessage, request dshStart) (*DSHWebBinding, error) {
	if request.Web == nil {
		if len(raw) != 0 {
			return nil, fmt.Errorf("unrequested DSH Web binding")
		}
		return nil, nil
	}
	var binding DSHWebBinding
	if err := decodeDSHWeb(raw, &binding); err != nil {
		return nil, err
	}
	if err := validateDSHWebBinding(binding, request); err != nil {
		return nil, err
	}
	if request.Web.ExpectedBinding != nil && binding != *request.Web.ExpectedBinding {
		return nil, fmt.Errorf("DSH Web connection, account, or policy changed before resume")
	}
	return &binding, nil
}

func validateDSHWebResult(raw json.RawMessage, request dshStart, binding *DSHWebBinding, sessionID, status string, usage json.RawMessage, cost *float64, settled bool, lifecycle *SessionOutcome) error {
	if request.Web == nil {
		if len(raw) != 0 {
			return fmt.Errorf("unrequested DSH Web result")
		}
		return nil
	}
	if binding == nil && len(raw) == 0 && status != "ok" {
		return nil
	} // admission never granted
	var receipt DSHWebReceipt
	if err := decodeDSHWeb(raw, &receipt); err != nil {
		return err
	}
	if binding == nil || receipt.Version != 1 || receipt.Binding != *binding || len(usage) != 0 && string(usage) != "null" || cost != nil {
		return fmt.Errorf("DSH Web terminal binding or unknown usage mismatch")
	}
	facts := receipt.Facts
	// A missing/null boolean is not an explicit negative cleanup fact. Other
	// required fields have nonzero literals checked below.
	var shape struct {
		Facts map[string]json.RawMessage `json:"facts"`
	}
	_ = json.Unmarshal(raw, &shape)
	cleanup := string(shape.Facts["localCleanupConfirmed"])
	if (cleanup != "true" && cleanup != "false") || facts.Version != 1 || facts.HostInvocationID != request.RequestID || facts.ExecutionDomainID != request.Web.ExecutionDomainID || facts.SessionID != sessionID ||
		settled && !facts.LocalCleanupConfirmed || lifecycle != nil && lifecycle.Outcome != "unknown" && lifecycle.Outcome != facts.Outcome ||
		facts.UsageStatus != "unknown" || !slices.Contains([]string{"not-dispatched", "dispatched", "unknown"}, facts.Dispatch) ||
		!slices.Contains([]string{"completed", "failed", "cancelled", "unknown"}, facts.Outcome) ||
		!slices.Contains([]string{"idle", "running", "unknown"}, facts.RemoteGeneration) ||
		facts.Dispatch == "unknown" && facts.Outcome != "unknown" || facts.Dispatch == "not-dispatched" && facts.Outcome == "completed" ||
		facts.RemoteGeneration != "idle" && facts.Outcome != "unknown" ||
		status == "ok" && (!facts.LocalCleanupConfirmed || (!request.Web.Probe && (facts.Dispatch != "dispatched" || facts.Outcome != "completed")) || request.Web.Probe && (facts.Dispatch != "not-dispatched" || facts.Outcome != "failed" || facts.RemoteGeneration != "idle")) {
		return fmt.Errorf("DSH Web terminal facts are incomplete, mismatched, or contradictory")
	}
	return nil
}

func dshWebIdleTimeout(a DSHArgs) time.Duration {
	if a.Model == DSHWebModel {
		return dshTimeout(a.Watchdog.WallTimeoutMs, watchdog.DefaultWallTimeoutMs) + time.Second
	}
	return dshTimeout(a.Watchdog.IdleTimeoutMs, watchdog.DefaultIdleTimeoutMs)
}
