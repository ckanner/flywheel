package state

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ckanner/flywheel/internal/contract"
)

// SessionLedger is owned by the task's run lease. History is role scoped; a
// received invocation is retained until its stage and accounting are applied
// together in state.json. Receipts are evidence, never a second state machine.
type SessionLedger struct {
	Version int                       `json:"version"`
	Records map[string]*SessionRecord `json:"records"`
}

type SessionConfiguration struct {
	NativeProfile string          `json:"nativeProfile,omitempty"`
	NativeOptions json.RawMessage `json:"nativeOptions,omitempty"`
	Harness       string          `json:"harness"`
	Model         string          `json:"model"`
	Provider      string          `json:"provider"`
	Channel       string          `json:"channel"`
	Thinking      string          `json:"thinking"`
	Permission    string          `json:"permission"`
	SessionMode   string          `json:"sessionMode"`
}

type SessionRecord struct {
	WebBinding      json.RawMessage       `json:"web-binding,omitempty"`
	Context         *SessionContext       `json:"context,omitempty"`
	Configuration   *SessionConfiguration `json:"configuration,omitempty"`
	Task            string                `json:"task"`
	Role            string                `json:"role"`
	Scope           string                `json:"scope"`
	Cwd             string                `json:"cwd"`
	WorkspaceCwd    string                `json:"workspace-cwd,omitempty"`
	CandidateScope  []string              `json:"candidate-scope,omitempty"`
	Binding         string                `json:"binding"`
	Goal            string                `json:"goal"`
	Contract        string                `json:"contract"`
	Generation      int                   `json:"generation"`
	ThreadID        string                `json:"thread-id,omitempty"`
	Protocol        string                `json:"protocol,omitempty"`
	ResumeCandidate bool                  `json:"resume-candidate"`
	Action          string                `json:"action"`
	Reason          string                `json:"reason"`
	Quality         *SessionQuality       `json:"quality,omitempty"`
	Last            *SessionInvocation    `json:"last"`
}

// Context is an input checkpoint, not a summary of private model reasoning.
type SessionContext struct {
	Version  int    `json:"version"`
	Identity string `json:"identity,omitempty"`
	Mode     string `json:"mode"`
	Reason   string `json:"reason"`
}

// Quality counters describe confirmed work in one role's current generation.
// They never grant permission to replay an unknown or unconsumed invocation.
type SessionQuality struct {
	ConfirmedTurns    int    `json:"confirmed-turns"`
	UnproductiveTurns int    `json:"unproductive-turns"`
	LastInputTokens   *int64 `json:"last-input-tokens,omitempty"`
	LastUsageStatus   string `json:"last-usage-status"`
	LastInvocationID  string `json:"last-invocation-id,omitempty"`
	Rebuilds          int    `json:"rebuilds"`
	RebuildReason     string `json:"rebuild-reason,omitempty"`
}

// Recovery retains non-secret original settings. The public read-only API uses
// these settings to validate a saved receipt; current credentials are not needed.
type SessionRecovery struct {
	Version          int    `json:"version"`
	Harness          string `json:"harness"`
	AdapterVersion   string `json:"adapter-version"`
	Directory        string `json:"directory"`
	Model            string `json:"model"`
	Effort           string `json:"effort"`
	PermissionMode   string `json:"permission-mode"`
	RunID            string `json:"run-id,omitempty"`
	Sequence         int    `json:"sequence"`
	Event            string `json:"event,omitempty"`
	CheckpointDigest string `json:"checkpoint-digest,omitempty"`
}

// Keep the exact accounted increment so later read-only evidence can fill an
// unknown counter once without rebilling an already-known native run.
type SessionAccounting struct {
	Day           string        `json:"day"`
	Provider      string        `json:"provider"`
	ReceiptDigest string        `json:"receipt-digest"`
	Usage         *SessionUsage `json:"usage"`
	CostUSD       *float64      `json:"cost-usd"`
}

type SessionUsage struct {
	Input       int64 `json:"input"`
	Output      int64 `json:"output"`
	Reasoning   int64 `json:"reasoning"`
	CacheRead   int64 `json:"cache-read"`
	CacheWrite  int64 `json:"cache-write"`
	TotalTokens int64 `json:"total-tokens"`
}

type SessionInvocation struct {
	EngineRunID      string             `json:"engine-run-id,omitempty"`
	ID               string             `json:"id"`
	Status           string             `json:"status"` // intent | received | consumed | blocked | abandoned
	StartedAt        string             `json:"started-at"`
	Attempt          int                `json:"attempt"`
	RequestedThread  string             `json:"requested-thread,omitempty"`
	Persistent       bool               `json:"persistent,omitempty"`
	CandidateBefore  string             `json:"candidate-before"`
	CandidateAfter   string             `json:"candidate-after,omitempty"`
	GuardBefore      string             `json:"guard-before,omitempty"`
	GuardAfter       string             `json:"guard-after,omitempty"`
	ReceiptDigest    string             `json:"receipt-digest,omitempty"`
	Accounted        bool               `json:"accounted"`
	Dispatch         string             `json:"dispatch"`
	Outcome          string             `json:"outcome"`
	CleanupConfirmed bool               `json:"cleanup-confirmed"`
	UsageStatus      string             `json:"usage-status"`
	Recovery         *SessionRecovery   `json:"recovery,omitempty"`
	Accounting       *SessionAccounting `json:"accounting,omitempty"`
	Abandon          *SessionAbandon    `json:"abandon,omitempty"`
}

// TerminalSessionStatus reports whether an invocation has a settled effect
// boundary: consumed applied a trusted result; abandoned retired an unknown one.
func TerminalSessionStatus(status string) bool {
	return status == "consumed" || status == "abandoned"
}

// SessionAbandon records the explicit human decision to retire an unresolved
// planning invocation. It preserves the unknown dispatch/outcome facts; no
// stage result is applied and the old thread is never resumed.
type SessionAbandon struct {
	At         string `json:"at"`
	Reason     string `json:"reason"`
	Authority  string `json:"authority"`
	RequestID  string `json:"request-id,omitempty"`
	Invocation string `json:"invocation,omitempty"`
	ThreadID   string `json:"thread-id,omitempty"`
	Source     string `json:"source,omitempty"`
}

// SessionReceipt stores the exact normalized result (including the raw public
// backend receipt), the observed candidate and the executor report captured at
// terminal time. One atomic slot per scope bounds recovery storage.
type SessionReceipt struct {
	Version        int             `json:"version"`
	InvocationID   string          `json:"invocation-id"`
	Key            string          `json:"key"`
	Task           string          `json:"task"`
	Cwd            string          `json:"cwd"`
	Binding        string          `json:"binding"`
	CandidateAfter string          `json:"candidate-after"`
	GuardBefore    string          `json:"guard-before,omitempty"`
	GuardAfter     string          `json:"guard-after,omitempty"`
	Report         []byte          `json:"report,omitempty"`
	Result         json.RawMessage `json:"result"`
}

func sessionReceiptPath(taskDir, key string) string {
	return filepath.Join(taskDir, ".sessions", fmt.Sprintf("%x.json", sha256.Sum256([]byte(key))))
}

// A backend owns exactly one directory for one invocation. Derivation from the
// engine ID prevents state or a model-controlled opaque run ID choosing a path.
func SessionRecoveryDirectory(taskDir, invocationID string) string {
	return filepath.Join(taskDir, ".sessions", fmt.Sprintf("recovery-%x", sha256.Sum256([]byte(invocationID))))
}

func PrepareSessionRecovery(taskDir, invocationID string) (string, error) {
	dir, err := sessionReceiptDirectory(taskDir, true)
	if err != nil {
		return "", err
	}
	dir.Close()
	// The adapter creates this leaf exclusively. Go must not precreate it or
	// reuse another invocation's journal, even for a known-not-dispatched call.
	return SessionRecoveryDirectory(taskDir, invocationID), nil
}

const maxSessionReceiptSize = 32 << 20

// File operations are injected at I/O boundaries so interruption and storage
// failures can be tested without weakening confinement or atomic replacement.
var openSessionReceiptDirectory = (*os.Root).OpenRoot
var mkdirSessionReceiptDirectory = (*os.Root).Mkdir
var openSessionReceiptFile = (*os.Root).OpenFile
var readSessionReceipt = io.ReadAll
var writeSessionReceipt = (*os.File).Write
var syncSessionReceipt = (*os.File).Sync
var closeSessionReceipt = (*os.File).Close
var renameSessionReceipt = (*os.Root).Rename

func syncSessionDirectory(root *os.Root) error {
	f, err := root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	return syncSessionReceipt(f)
}

// Open the real receipt directory, anchored to the task directory. Reject a
// symlink even when it targets another directory inside the task; identity
// checking also detects replacement between Lstat and OpenRoot.
func sessionReceiptDirectory(taskDir string, create bool) (*os.Root, error) {
	root, err := os.OpenRoot(taskDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if create {
		if err := mkdirSessionReceiptDirectory(root, ".sessions", 0o700); err != nil && !os.IsExist(err) {
			return nil, err
		}
		if err := syncSessionDirectory(root); err != nil {
			return nil, err
		}
	}
	info, err := root.Lstat(".sessions")
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("session receipts must use a real directory")
	}
	dir, err := openSessionReceiptDirectory(root, ".sessions")
	if err != nil {
		return nil, err
	}
	current, err := dir.Stat(".")
	if err != nil || !os.SameFile(info, current) {
		dir.Close()
		return nil, fmt.Errorf("session receipt directory changed during open")
	}
	return dir, nil
}

func WriteSessionReceipt(taskDir, key string, receipt SessionReceipt) (string, error) {
	dir, err := sessionReceiptDirectory(taskDir, true)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	raw, err := json.Marshal(receipt)
	if err != nil {
		return "", err
	}
	if len(raw) > maxSessionReceiptSize {
		return "", fmt.Errorf("session receipt exceeds 32 MiB")
	}
	name := filepath.Base(sessionReceiptPath(taskDir, key))
	tmp := name + ".tmp-" + rand.Text()
	f, err := openSessionReceiptFile(dir, tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	defer dir.Remove(tmp)
	if _, err = writeSessionReceipt(f, raw); err != nil {
		return "", err
	}
	if err = syncSessionReceipt(f); err != nil {
		return "", err
	}
	if err = closeSessionReceipt(f); err != nil {
		return "", err
	}
	if err = renameSessionReceipt(dir, tmp, name); err != nil {
		return "", err
	}
	if err = syncSessionDirectory(dir); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

func ReadSessionReceipt(taskDir, key, invocationID, digest string) (*SessionReceipt, string, error) {
	dir, err := sessionReceiptDirectory(taskDir, false)
	if err != nil {
		return nil, "", err
	}
	defer dir.Close()
	name := filepath.Base(sessionReceiptPath(taskDir, key))
	// O_NONBLOCK prevents an attacker-controlled FIFO from hanging before the
	// descriptor's own type check. O_NOFOLLOW rejects leaf symlinks atomically.
	f, err := openSessionReceiptFile(dir, name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSessionReceiptSize {
		return nil, "", fmt.Errorf("invalid session receipt file")
	}
	raw, err := readSessionReceipt(io.LimitReader(f, maxSessionReceiptSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(raw) > maxSessionReceiptSize {
		return nil, "", fmt.Errorf("session receipt exceeds 32 MiB")
	}
	actual := fmt.Sprintf("%x", sha256.Sum256(raw))
	if digest != "" && digest != actual {
		return nil, "", fmt.Errorf("session receipt digest mismatch")
	}
	var receipt SessionReceipt
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return nil, "", err
	}
	result := bytes.TrimSpace(receipt.Result)
	if receipt.Version != 1 || receipt.Key != key || receipt.InvocationID != invocationID || receipt.Task != taskDir ||
		receipt.InvocationID == "" || receipt.Cwd == "" || receipt.Binding == "" || len(result) == 0 || result[0] != '{' || !json.Valid(raw) {
		return nil, "", fmt.Errorf("session receipt identity mismatch")
	}
	for _, proof := range []string{receipt.CandidateAfter, receipt.GuardBefore, receipt.GuardAfter} {
		// An absent post-call proof is a recoverable accounting fact, but a
		// malformed value must never poison the schema-validated state ledger.
		if proof != "" && (len(proof) != sha256.Size*2 || strings.Trim(proof, "0123456789abcdef") != "") {
			return nil, "", fmt.Errorf("invalid session receipt checkpoint")
		}
	}
	return &receipt, actual, nil
}

func validateSessions(raw json.RawMessage) error {
	var value any
	_ = json.Unmarshal(raw, &value) // outer state JSON was already checked
	if err := contract.ValidateSessionStateValue(value); err != nil {
		return err
	}
	var ledger SessionLedger
	if err := json.Unmarshal(raw, &ledger); err != nil {
		return err // schema integers can still exceed the Go integer range
	}
	threads := map[[2]string]string{}
	for key, record := range ledger.Records {
		if len(record.WebBinding) != 0 && ledger.Version < 5 {
			return fmt.Errorf("Web binding requires version 5 session ledger")
		}
		if key != record.Role+"/"+record.Scope {
			return fmt.Errorf("session key %q does not match its role and scope", key)
		}
		if record.Last.Status == "consumed" && !record.Last.Accounted || record.Last.Status == "intent" && record.Last.Accounted {
			return fmt.Errorf("session %s accounting contradicts invocation status", key)
		}
		if record.Last.Status == "abandoned" {
			abandon := record.Last.Abandon
			if !record.Last.Accounted || !record.Last.CleanupConfirmed || abandon == nil ||
				strings.TrimSpace(abandon.At) == "" || strings.TrimSpace(abandon.Reason) == "" ||
				len(abandon.Reason) > 1000 || abandon.Authority != "human" || strings.ContainsRune(abandon.Reason, 0) ||
				len(abandon.Source) > 1024 || strings.ContainsAny(abandon.Source, "\x00\r\n") {
				return fmt.Errorf("session %s abandonment facts are invalid", key)
			}
			// state.schema.json enforces at least one unknown fact (and
			// resume-candidate:false); the machine engine also rejects a known
			// dispatch/outcome before it reaches persistence.
		}
		if recovery := record.Last.Recovery; recovery != nil {
			if !filepath.IsAbs(record.Task) || recovery.Directory != SessionRecoveryDirectory(record.Task, record.Last.ID) ||
				(recovery.Sequence == 0 && (recovery.RunID != "" || recovery.Event != "" || recovery.CheckpointDigest != "")) ||
				(recovery.Sequence > 0 && (strings.TrimSpace(recovery.RunID) == "" || recovery.Event == "" || recovery.CheckpointDigest == "")) {
				return fmt.Errorf("session %s recovery directory or checkpoint identity is invalid", key)
			}
		}
		if record.ThreadID != "" {
			identity := [2]string{record.Protocol, record.ThreadID}
			if other, found := threads[identity]; found {
				return fmt.Errorf("sessions %s and %s share a backend thread", other, key)
			}
			threads[identity] = key
		}
	}
	return nil
}
