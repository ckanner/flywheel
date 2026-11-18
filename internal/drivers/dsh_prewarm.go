package drivers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// PreparedDSH is a one-use owned SDK process, not retained model history.
type PreparedDSH interface {
	ID() string
	Matches(DSHArgs) bool
	Run(DSHArgs) Result
	Close() error
}

const DSHPrewarmTTL = 60 * time.Second

var warmID = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,128}$`)

// Narrow filesystem seams cover I/O failures without weakening proof checks.
var dshWarmStat = os.Stat
var dshWarmFileStat = (*os.File).Stat
var dshWarmSync = (*os.File).Sync
var dshWarmCreate = os.CreateTemp
var dshWarmTTL = DSHPrewarmTTL
var dshWarmCloseGrace = 3 * time.Second
var dshWarmOpenDirectory = os.Open
var dshWarmProbe = syscall.Kill

type dshPrepare struct {
	Web              *DSHWebRequest  `json:"web,omitempty"`
	Native           json.RawMessage `json:"native,omitempty"`
	Type             string          `json:"type"`
	Protocol         int             `json:"protocol"`
	WarmID           string          `json:"warmId"`
	Binding          string          `json:"binding"`
	ReceiptDirectory string          `json:"receiptDirectory"`
	TTLMS            int64           `json:"ttlMs"`
	Cwd              string          `json:"cwd"`
	WorkspaceCwd     string          `json:"workspaceCwd,omitempty"`
	Provider         string          `json:"provider"`
	Model            string          `json:"model"`
	ReasoningEffort  string          `json:"reasoningEffort"`
	IdleTimeoutMS    int64           `json:"idleTimeoutMs,omitempty"`
	PermissionMode   string          `json:"permissionMode"`
}

type dshWarmReceipt struct {
	Protocol         int    `json:"protocol"`
	WarmID           string `json:"warmId"`
	Binding          string `json:"binding"`
	WorkerPID        int    `json:"workerPid"`
	NoProcess        bool   `json:"noProcess,omitempty"`
	Dispatched       *bool  `json:"dispatched"`
	CleanupConfirmed bool   `json:"cleanupConfirmed"`
	Error            string `json:"error,omitempty"`
}

// DSHPrewarmBinding freezes the actual child environment without exposing its
// contents. Session history and prompt are intentionally chosen at invocation.
func DSHPrewarmBinding(a DSHArgs) (string, error) {
	if a.Doctor {
		return "", fmt.Errorf("a doctor probe cannot consume prewarming")
	}
	a.Prompt, a.SessionID, a.RequestID, a.SessionTitle = "", "", "", ""
	a.PersistentSession = false
	a.Managed = nil
	if a.Web != nil {
		copy := *a.Web
		copy.ExpectedBinding = nil
		a.Web = &copy
	}
	r, command, _, env, err := dshCall(a)
	if err != nil {
		return "", err
	}
	r.RequestID = ""
	// All values are concrete JSON primitives; marshaling cannot fail.
	raw, _ := json.Marshal(struct {
		Request dshStart
		Command string
		Env     map[string]string
		Idle    int64
		Wall    int64
		Grace   int64
	}{r, command, env, a.Watchdog.IdleTimeoutMs, a.Watchdog.WallTimeoutMs, a.Watchdog.KillGraceMs})
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func dshWarmDirectory(path string) error {
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	info, err := dshWarmStat(path)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(path) || physical != path || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("prewarm evidence requires a private physical directory")
	}
	return nil
}

func readDSHWarmReceipt(id, directory, binding string) (dshWarmReceipt, error) {
	var receipt dshWarmReceipt
	if !warmID.MatchString(id) || len(binding) != 64 {
		return receipt, fmt.Errorf("invalid prewarm evidence identity")
	}
	if err := dshWarmDirectory(directory); err != nil {
		return receipt, err
	}
	fd, err := syscall.Open(filepath.Join(directory, "terminal.json"), syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return receipt, err
	}
	file := os.NewFile(uintptr(fd), "prewarm-terminal")
	defer file.Close()
	info, err := dshWarmFileStat(file)
	if err != nil {
		return receipt, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 8192 {
		return receipt, fmt.Errorf("prewarm terminal evidence is not a bounded private regular file")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 8193))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return receipt, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return receipt, fmt.Errorf("prewarm evidence has a trailing document")
	}
	if receipt.Protocol != 1 || receipt.WarmID != id || receipt.Binding != binding || receipt.Dispatched == nil ||
		*receipt.Dispatched || !receipt.CleanupConfirmed || (receipt.NoProcess && receipt.WorkerPID != 0) || (!receipt.NoProcess && receipt.WorkerPID <= 0) {
		return receipt, fmt.Errorf("prewarm evidence does not prove this unused SDK was cleaned up")
	}
	if err := dshWarmSync(file); err != nil {
		return receipt, err
	}
	if err := syncDSHWarmDirectory(directory); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func syncDSHWarmDirectory(directory string) error {
	file, err := dshWarmOpenDirectory(directory)
	if err != nil {
		return err
	}
	defer file.Close()
	return dshWarmSync(file)
}

// RecoverDSHPrewarm only reads the exact unused-worker receipt. A live or reused
// PID blocks recovery; elapsed time and EOF assumptions never prove quiescence.
func RecoverDSHPrewarm(id, directory, binding string) error {
	receipt, err := readDSHWarmReceipt(id, directory, binding)
	if err != nil {
		return err
	}
	if !receipt.NoProcess && (dshWarmProbe(receipt.WorkerPID, 0) != syscall.ESRCH || dshWarmProbe(-receipt.WorkerPID, 0) != syscall.ESRCH) {
		return fmt.Errorf("prewarm worker and owned process group have not been proven physically exited")
	}
	return nil
}

// DSHPrewarm owns exactly one process and one later formal invocation.
type DSHPrewarm struct {
	mu             sync.Mutex
	prepare        dshPrepare
	args           DSHArgs
	command        string
	env            map[string]string
	ctx            context.Context
	cancel         context.CancelFunc
	done           chan struct{}
	processDone    chan struct{}
	inputDone      chan struct{}
	formalDone     chan struct{}
	ready          chan struct{}
	controls       chan *dshStart
	consumed       bool
	closed         bool
	readySeen      bool
	warmTerminal   bool
	warmErrorCode  errs.ErrorCode
	warmErrorText  string
	formalOutput   bool
	formalRequest  dshStart
	startError     error
	writeError     error
	streamError    error
	result         watchdog.Result
	formal         *DSHArgs
	idleTimer      *time.Timer
	wallTimer      *time.Timer
	prepareTimer   *time.Timer
	closeTimer     *time.Timer
	prepareExpired bool
	formalSent     bool
	lastActivity   time.Time
	fallbackCancel context.CancelFunc
	fallbackResult *Result
	idleTimedOut   bool
	wallTimedOut   bool
}

// PrepareDSH starts asynchronously after its caller has persisted the warm
// identity. Only initialize is authorized until Run receives a formal request.
func PrepareDSH(a DSHArgs) *DSHPrewarm {
	ctx := a.Watchdog.Context
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	h := &DSHPrewarm{args: a, ctx: ctx, cancel: cancel, done: make(chan struct{}), processDone: make(chan struct{}), inputDone: make(chan struct{}), ready: make(chan struct{}), controls: make(chan *dshStart, 1)}
	h.args.APIKeys = maps.Clone(a.APIKeys)
	binding, err := DSHPrewarmBinding(a)
	if err == nil {
		err = dshWarmDirectory(a.PrewarmDirectory)
	}
	if err == nil {
		var entries []os.DirEntry
		entries, err = os.ReadDir(a.PrewarmDirectory)
		if err == nil && len(entries) != 0 {
			err = fmt.Errorf("prewarm evidence directory must be new and empty")
		}
	}
	if err == nil && !warmID.MatchString(a.PrewarmID) {
		err = fmt.Errorf("prewarm requires an owned random identity")
	}
	preparedArgs := a
	preparedArgs.Prompt, preparedArgs.SessionID, preparedArgs.RequestID, preparedArgs.SessionTitle = "", "", "", ""
	preparedArgs.PersistentSession = false
	preparedArgs.Managed = nil
	if preparedArgs.Web != nil {
		web := *preparedArgs.Web
		web.ExpectedBinding = nil
		preparedArgs.Web = &web
	}
	r, command, _, env, _ := dshCall(preparedArgs) // Binding validation already checked these arguments.
	h.command, h.env = command, env
	h.prepare = dshPrepare{Web: r.Web, Native: r.Native, Type: "prepare", Protocol: 1, WarmID: a.PrewarmID, Binding: binding,
		ReceiptDirectory: a.PrewarmDirectory, TTLMS: dshWarmTTL.Milliseconds(), Cwd: r.Cwd, WorkspaceCwd: r.WorkspaceCwd,
		Provider: r.Provider, Model: r.Model, ReasoningEffort: r.ReasoningEffort, PermissionMode: r.PermissionMode, IdleTimeoutMS: r.IdleTimeoutMS}
	if err != nil {
		h.startError = err
		cancel()
		close(h.done)
		return h
	}
	h.prepareTimer = time.AfterFunc(dshWarmTTL, h.expirePrepare)
	go h.runProcess()
	return h
}

func (h *DSHPrewarm) ID() string { return h.prepare.WarmID }

func (h *DSHPrewarm) Matches(a DSHArgs) bool {
	binding, err := DSHPrewarmBinding(a)
	return err == nil && binding == h.prepare.Binding
}

func (h *DSHPrewarm) expirePrepare() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.formalSent {
		return // a queued prepare timer cannot cancel an already handed-off call
	}
	select {
	case <-h.processDone:
		return // an exited preparation keeps its original terminal classification
	default:
	}
	if !h.warmTerminal {
		h.prepareExpired = true
	}
	// A terminal frame owns classification, but not physical process cleanup.
	// Retire admission at the TTL, then allow the SDK's own bounded shutdown.
	// Sending TERM to the inherited group immediately would kill the SDK before
	// its owning worker can publish a trustworthy no-dispatch cleanup receipt.
	if h.closeTimer == nil {
		h.closeTimer = time.AfterFunc(dshWarmCloseGrace, h.cancel)
	}
}

func (h *DSHPrewarm) activity(chunk, stream string) {
	h.mu.Lock()
	h.lastActivity = time.Now()
	if h.idleTimer != nil && !h.idleTimedOut {
		h.idleTimer.Reset(dshWebIdleTimeout(*h.formal))
	}
	h.mu.Unlock()
}

func (h *DSHPrewarm) line(line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var frame struct {
		Web              json.RawMessage `json:"web"`
		Protocol         int             `json:"protocol"`
		Type             string          `json:"type"`
		WarmID           string          `json:"warmId"`
		Binding          string          `json:"binding"`
		Level            string          `json:"level"`
		Dispatched       *bool           `json:"dispatched"`
		CleanupConfirmed *bool           `json:"cleanupConfirmed"`
		Error            *struct {
			Code    errs.ErrorCode `json:"code"`
			Message string         `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &frame); err != nil {
		h.streamError = fmt.Errorf("malformed SDK prewarm stream")
		h.cancel()
		return
	}
	if frame.Type == "warm-ready" || frame.Type == "warm-result" {
		if h.warmTerminal || h.formalOutput || frame.Protocol != 1 || frame.WarmID != h.prepare.WarmID || frame.Binding != h.prepare.Binding || frame.Dispatched == nil || *frame.Dispatched ||
			(frame.Type == "warm-ready" && (h.readySeen || frame.Level != "sdk-initialized")) ||
			(frame.Type == "warm-result" && (frame.CleanupConfirmed == nil || !*frame.CleanupConfirmed)) {
			h.streamError = fmt.Errorf("SDK prewarm identity or readiness mismatch")
			h.cancel()
			return
		}
		if frame.Type == "warm-ready" {
			if _, err := validateDSHWebReady(frame.Web, dshStart{Web: h.prepare.Web, Provider: h.prepare.Provider, Model: h.prepare.Model, ReasoningEffort: h.prepare.ReasoningEffort}); err != nil {
				h.streamError = err
				h.cancel()
				return
			}
			h.readySeen = true
			close(h.ready)
		} else {
			if frame.Error != nil && !errs.IsKnownCode(frame.Error.Code) {
				h.streamError = fmt.Errorf("SDK prewarm returned an invalid structured error receipt")
				h.cancel()
				return
			}
			h.warmTerminal = true
			if frame.Error != nil {
				h.warmErrorCode, h.warmErrorText = frame.Error.Code, frame.Error.Message
				if frame.Error.Code == errs.TIMEOUT {
					h.prepareExpired = true
				}
			}
		}
		return
	}
	if h.formal == nil || h.warmTerminal || !h.readySeen {
		h.streamError = fmt.Errorf("SDK worker emitted invocation output before a formal request")
		h.cancel()
		return
	}
	h.formalOutput = true
	if h.formal.OnEvent != nil {
		h.formal.OnEvent(line)
	}
}

func dshTimeout(ms, fallback int64) time.Duration {
	if ms <= 0 {
		ms = fallback
	}
	return time.Duration(ms) * time.Millisecond
}

func (h *DSHPrewarm) runProcess() {
	events := jsonLines{onLine: h.line, limit: watchdog.MaxBuffer}
	result := watchdog.Run(h.command, []string{"--prewarm"}, watchdog.Opts{
		Context: h.ctx, Cwd: h.prepare.Cwd, Env: childenv.EnvSlice(h.env), KillGraceMs: dshCleanupGrace(h.args),
		WallTimeoutMs:      DSHPrewarmTTL.Milliseconds() + dshTimeout(h.args.Watchdog.WallTimeoutMs, watchdog.DefaultWallTimeoutMs).Milliseconds(),
		IdleTimeoutMs:      DSHPrewarmTTL.Milliseconds() + dshTimeout(h.args.Watchdog.IdleTimeoutMs, watchdog.DefaultIdleTimeoutMs).Milliseconds(),
		OnActivity:         func(chunk, stream string) { h.activity(chunk, stream); events.feed(chunk, stream) },
		InteractStdin:      h.interact,
		DisableIdleTimeout: h.prepare.Web != nil,
		StdoutCapture: newDSHActivityCapture(func() dshStart {
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.formalRequest
		}),
	})
	close(h.processDone)
	if result.ExitCode != nil || result.Signal != "" {
		<-h.inputDone
	}
	events.flush()
	h.mu.Lock()
	h.prepareTimer.Stop()
	if h.closeTimer != nil {
		h.closeTimer.Stop()
	}
	if h.idleTimer != nil {
		h.idleTimer.Stop()
		h.wallTimer.Stop()
	}
	h.result = result
	h.mu.Unlock()
	if result.SpawnError != "" || (result.Cancelled && result.ExitCode == nil && result.Signal == "") {
		err := RecordUnstartedDSHPrewarm(h.ID(), h.prepare.ReceiptDirectory, h.prepare.Binding)
		h.mu.Lock()
		h.startError = err
		h.mu.Unlock()
	}
	close(h.done)
}

// RecordUnstartedDSHPrewarm records positive local knowledge that no process
// was started, including a failed state CAS before PrepareDSH was called.
func RecordUnstartedDSHPrewarm(id, directory, binding string) error {
	if !warmID.MatchString(id) || len(binding) != 64 {
		return fmt.Errorf("invalid unstarted prewarm identity")
	}
	if err := dshWarmDirectory(directory); err != nil {
		return err
	}
	no := false
	receipt := dshWarmReceipt{Protocol: 1, WarmID: id, Binding: binding,
		NoProcess: true, Dispatched: &no, CleanupConfirmed: true}
	data, _ := json.Marshal(receipt)
	file, err := dshWarmCreate(directory, ".terminal-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := dshWarmSync(file); err != nil {
		return err
	}
	if err := os.Link(file.Name(), filepath.Join(directory, "terminal.json")); err != nil {
		return err
	}
	return syncDSHWarmDirectory(directory)
}

func (h *DSHPrewarm) interact(stdin io.WriteCloser, finish func()) {
	defer close(h.inputDone)
	defer stdin.Close()
	encoder := json.NewEncoder(stdin)
	if err := encoder.Encode(h.prepare); err != nil {
		h.writeFailure(err, finish)
		return
	}
	var request *dshStart
	select {
	case request = <-h.controls:
	case <-h.processDone:
		return
	case <-h.ctx.Done():
		return
	}
	if request != nil {
		select {
		case <-h.ready:
		case <-h.processDone:
			return
		case <-h.ctx.Done():
			return
		}
	}
	frame := map[string]any{"protocol": 1, "warmId": h.prepare.WarmID, "type": "close"}
	if request != nil {
		frame["type"], frame["request"] = "start", request
		h.mu.Lock()
		if h.prepareExpired || h.closed || h.ctx.Err() != nil {
			h.mu.Unlock()
			return
		}
		h.formalSent = true
		h.prepareTimer.Stop()
		h.mu.Unlock()
	}
	if err := encoder.Encode(frame); err != nil {
		h.writeFailure(err, finish)
		return
	}
	// Unlike legacy v1/v2, EOF is always a lifeline failure for two-phase workers.
	select {
	case <-h.processDone:
	case <-h.ctx.Done():
	}
}

func (h *DSHPrewarm) writeFailure(err error, finish func()) {
	h.mu.Lock()
	h.writeError = err
	h.mu.Unlock()
	finish()
}

func (h *DSHPrewarm) expire(idle bool) {
	h.mu.Lock()
	select {
	case <-h.processDone:
		h.mu.Unlock()
		return // a queued callback cannot time out an already exited process
	default:
	}
	if idle {
		// Reset does not retract an AfterFunc callback that is already queued.
		// Fresh output owns the current idle window even if that callback runs.
		if time.Since(h.lastActivity) < dshWebIdleTimeout(*h.formal) {
			h.mu.Unlock()
			return
		}
		h.idleTimedOut = true
	} else {
		h.wallTimedOut = true
	}
	h.mu.Unlock()
	h.cancel()
}

func (h *DSHPrewarm) Run(a DSHArgs) Result {
	started := time.Now()
	request, _, _, _, err := dshCall(a)
	if err != nil || !h.Matches(a) {
		cleanup := h.Close()
		out := invalidDSHSession(a, "formal invocation does not match the prepared SDK binding")
		if cleanup != nil {
			out.CleanupError, out.Session.CleanupConfirmed = cleanup.Error(), false
		}
		return out
	}
	h.mu.Lock()
	if h.consumed || h.closed || h.startError != nil {
		h.mu.Unlock()
		return invalidDSHSession(a, "SDK prewarm can only be consumed once")
	}
	h.consumed = true
	h.formalDone = make(chan struct{})
	defer close(h.formalDone)
	h.formal = &a
	h.lastActivity = started
	h.formalRequest = request
	h.idleTimer = time.AfterFunc(dshWebIdleTimeout(a), func() { h.expire(true) })
	h.wallTimer = time.AfterFunc(dshTimeout(a.Watchdog.WallTimeoutMs, watchdog.DefaultWallTimeoutMs), func() { h.expire(false) })
	h.mu.Unlock()
	stop := func() bool { return false }
	if a.Watchdog.Context != nil {
		stop = context.AfterFunc(a.Watchdog.Context, h.cancel)
	}
	h.controls <- &request
	<-h.done
	stop()
	h.mu.Lock()
	h.idleTimer.Stop()
	h.wallTimer.Stop()
	result, writeError, streamError := h.result, h.writeError, h.streamError
	warmErrorCode, warmErrorText := h.warmErrorCode, h.warmErrorText
	result.DurationMS = time.Since(started).Milliseconds()
	if h.idleTimedOut || h.wallTimedOut {
		result.TimedOut, result.IdleTimedOut, result.WallTimedOut = true, h.idleTimedOut, h.wallTimedOut
		result.Cancelled = a.Watchdog.Context != nil && a.Watchdog.Context.Err() != nil
	}
	h.mu.Unlock()
	result.Stdout = dshFormalOutput(result.Stdout)
	if streamError != nil {
		writeError = streamError
		result.CleanupError = "SDK prewarm stream did not provide trustworthy cleanup: " + streamError.Error()
	}
	out := finishDSH(a, request, result, writeError)
	// A worker may expire before it receives the formal frame. Only the exact
	// durable no-dispatch receipt plus outer process quiescence permits certainty.
	if !out.Settled && result.CleanupError == "" && !result.Truncated && streamError == nil &&
		RecoverDSHPrewarm(h.ID(), h.prepare.ReceiptDirectory, h.prepare.Binding) == nil {
		out.CleanupError = ""
		out.Cancelled = (a.Watchdog.Context != nil && a.Watchdog.Context.Err() != nil) ||
			(h.args.Watchdog.Context != nil && h.args.Watchdog.Context.Err() != nil)
		out.Session = &SessionOutcome{Protocol: DSHFreshSessionProtocol, RequestID: request.RequestID,
			Dispatch: "not-dispatched", Outcome: "failed", CleanupConfirmed: true, UsageStatus: "complete"}
		if a.PersistentSession {
			out.Session.Protocol = DSHSessionProtocol
		}
		out.Usage, out.CostUSD = &Usage{}, new(float64)
		if request.Web != nil {
			out.Usage, out.CostUSD, out.Session.UsageStatus = nil, nil, "unknown"
		}
		// Initialization failures still own their structured retry/fallback
		// classification. Trust it only after the same receipt, stream and
		// physical error-exit checks used for a formal worker failure.
		if warmErrorCode != "" && writeError == nil && out.ExitCode != nil && *out.ExitCode == 1 && out.Signal == "" && !out.Cancelled && !out.TimedOut {
			out.ErrorCode = warmErrorCode
			message := string(warmErrorCode) + ": " + warmErrorText
			out.ErrorMessage = &message
		}
		if !out.Cancelled && !out.TimedOut {
			a.RequestID = request.RequestID
			return h.coldAfterExpiry(a, started, out)
		}
	}
	return out
}

// A safe unused expiration is an optimization miss, not a failed Review.
// Keep the same durable invocation and spend only its remaining time allowance.
func (h *DSHPrewarm) coldAfterExpiry(a DSHArgs, started time.Time, unused Result) Result {
	// Admit the cold cancellation handle atomically with Close; there is no
	// unowned window between deciding to fall back and publishing its lifeline.
	h.mu.Lock()
	// The local TTL can fire while a failed initialization is still shutting
	// down. Its later non-timeout terminal error must not become a cold call.
	if !h.prepareExpired || h.formalSent || h.closed || !h.Matches(a) || (h.warmErrorCode != "" && h.warmErrorCode != errs.TIMEOUT) {
		h.mu.Unlock()
		return unused
	}
	wall := dshTimeout(a.Watchdog.WallTimeoutMs, watchdog.DefaultWallTimeoutMs) - time.Since(started)
	idle := dshWebIdleTimeout(a) - time.Since(h.lastActivity)
	if wall < time.Millisecond || idle < time.Millisecond {
		h.mu.Unlock()
		unused.TimedOut, unused.WallTimedOut, unused.IdleTimedOut = true, wall < time.Millisecond, idle < time.Millisecond
		unused.ErrorCode = errs.TIMEOUT
		message := timeoutMessage("DSH worker", unused)
		unused.ErrorMessage = &message
		return unused
	}
	ctx := a.Watchdog.Context
	if ctx == nil {
		ctx = h.args.Watchdog.Context
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	h.fallbackCancel = cancel
	h.mu.Unlock()
	a.Prewarm, a.Watchdog.Context = nil, ctx
	a.Watchdog.WallTimeoutMs, a.Watchdog.IdleTimeoutMs = wall.Milliseconds(), idle.Milliseconds()
	result := RunDSH(a)
	result.DurationMS = time.Since(started).Milliseconds()
	h.mu.Lock()
	h.fallbackResult = &result
	h.mu.Unlock()
	return result
}

func dshFormalOutput(stdout string) string {
	var formal strings.Builder
	for _, line := range strings.Split(stdout, "\n") {
		var frame struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal([]byte(line), &frame)
		if frame.Type != "warm-ready" && frame.Type != "warm-result" {
			formal.WriteString(line + "\n")
		}
	}
	return formal.String()
}

// Close waits for physical and SDK cleanup. Repeated closes observe the same
// terminal proof; a consumed worker is cancelled rather than reused.
func (h *DSHPrewarm) Close() error {
	h.mu.Lock()
	if !h.closed {
		h.closed = true
		if h.consumed {
			h.cancel()
			if h.fallbackCancel != nil {
				h.fallbackCancel()
			}
		} else {
			h.controls <- nil
			if h.closeTimer == nil {
				h.closeTimer = time.AfterFunc(dshWarmCloseGrace, h.cancel)
			}
		}
	}
	closeTimer := h.closeTimer
	consumed := h.consumed
	h.mu.Unlock()
	<-h.done
	if closeTimer != nil {
		closeTimer.Stop()
	}
	if consumed {
		<-h.formalDone
	}
	h.cancel()
	if h.startError != nil {
		return h.startError
	}
	if h.result.CleanupError != "" || h.result.Truncated || h.streamError != nil || h.writeError != nil {
		return fmt.Errorf("prewarm transport or process cleanup could not be confirmed")
	}
	h.mu.Lock()
	fallback := h.fallbackResult
	h.mu.Unlock()
	if fallback != nil {
		if fallback.CleanupError != "" || fallback.Session == nil || !fallback.Session.CleanupConfirmed {
			return fmt.Errorf("cold invocation after expiry did not confirm cleanup")
		}
		return nil
	}
	if consumed && parseDSHStream(dshFormalOutput(h.result.Stdout), h.formalRequest).CleanupConfirmed {
		return nil
	}
	return RecoverDSHPrewarm(h.ID(), h.prepare.ReceiptDirectory, h.prepare.Binding)
}
