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

type SessionRecord struct {
	Task            string             `json:"task"`
	Role            string             `json:"role"`
	Scope           string             `json:"scope"`
	Cwd             string             `json:"cwd"`
	Binding         string             `json:"binding"`
	Goal            string             `json:"goal"`
	Contract        string             `json:"contract"`
	Generation      int                `json:"generation"`
	ThreadID        string             `json:"thread-id,omitempty"`
	Protocol        string             `json:"protocol,omitempty"`
	ResumeCandidate bool               `json:"resume-candidate"`
	Action          string             `json:"action"`
	Reason          string             `json:"reason"`
	Last            *SessionInvocation `json:"last"`
}

type SessionInvocation struct {
	ID               string `json:"id"`
	Status           string `json:"status"` // intent | received | consumed | blocked
	StartedAt        string `json:"started-at"`
	Attempt          int    `json:"attempt"`
	RequestedThread  string `json:"requested-thread,omitempty"`
	Persistent       bool   `json:"persistent,omitempty"`
	CandidateBefore  string `json:"candidate-before"`
	CandidateAfter   string `json:"candidate-after,omitempty"`
	GuardBefore      string `json:"guard-before,omitempty"`
	GuardAfter       string `json:"guard-after,omitempty"`
	ReceiptDigest    string `json:"receipt-digest,omitempty"`
	Accounted        bool   `json:"accounted"`
	Dispatch         string `json:"dispatch"`
	Outcome          string `json:"outcome"`
	CleanupConfirmed bool   `json:"cleanup-confirmed"`
	UsageStatus      string `json:"usage-status"`
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
		if key != record.Role+"/"+record.Scope {
			return fmt.Errorf("session key %q does not match its role and scope", key)
		}
		if record.Last.Status == "consumed" && !record.Last.Accounted || record.Last.Status == "intent" && record.Last.Accounted {
			return fmt.Errorf("session %s accounting contradicts invocation status", key)
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
