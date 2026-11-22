package drivers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/ckanner/flywheel/internal/watchdog"
)

// MachineLifecycle is the public, versioned evidence stream. Display progress
// is deliberately not part of this contract. A durable callback returning nil
// permits one correlated ACK; requestId still is not an idempotency key.
type MachineLifecycle struct {
	LifecycleVersion int             `json:"lifecycleVersion"`
	Type             string          `json:"type"`
	RequestID        string          `json:"requestId"`
	RunID            string          `json:"runId"`
	Sequence         int             `json:"sequence"`
	Event            string          `json:"event"`
	ThreadID         *string         `json:"threadId"`
	TurnID           *string         `json:"turnId"`
	Persistent       bool            `json:"persistent"`
	Configuration    json.RawMessage `json:"configuration"`
	Dispatch         string          `json:"dispatch"`
	Outcome          string          `json:"outcome"`
	CleanupConfirmed bool            `json:"cleanupConfirmed"`
}

type machineAdapter struct {
	Package  string `json:"package"`
	Version  string `json:"version"`
	Protocol int    `json:"protocol"`
}

type machineLifecycleFrame struct {
	Type      string          `json:"type"`
	Protocol  int             `json:"protocol"`
	RequestID string          `json:"requestId"`
	Adapter   machineAdapter  `json:"adapter"`
	Lifecycle json.RawMessage `json:"lifecycle"`
}

func machineAdapterVersion(harness string) string {
	switch harness {
	case "antigravity":
		return AntigravityPackageVersion
	case "claude-code-pro":
		return ClaudeCodeProPackageVersion
	default:
		return CodexProPackageVersion
	}
}
func machineAdapterMatches(a machineAdapter, harness string) bool {
	return a.Package == adapterPackageFor(harness) && a.Version == machineAdapterVersion(harness) && a.Protocol == 1
}

func machineOpaqueID(id string) bool {
	return strings.TrimSpace(id) != "" && len(id) <= 4096 && !strings.ContainsRune(id, 0)
}

func machineLifecycleProvesResult(event *MachineLifecycle, session *SessionOutcome) bool {
	return event != nil && session != nil && event.Event == "cleanup-confirmed" && event.CleanupConfirmed && event.Dispatch == "accepted" && event.Outcome == "completed" &&
		event.RequestID == session.RequestID && event.RunID == session.RunID && event.ThreadID != nil && *event.ThreadID == session.SessionID &&
		((event.TurnID == nil && session.TurnID == "") || (event.TurnID != nil && *event.TurnID == session.TurnID))
}

// Lifecycle uses the same requested settings as Codex's start frame; Claude's
// worker envelope has the identical field names. It does not contain secrets.
type machineLifecycleFlow struct {
	harness string
	request codexProStart
	last    *MachineLifecycle
}

func (f *machineLifecycleFlow) accept(raw json.RawMessage) (MachineLifecycle, error) {
	var e MachineLifecycle
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&e); err != nil || !json.Valid(raw) ||
		!receiptHasFields(raw, "lifecycleVersion", "type", "requestId", "runId", "sequence", "event", "threadId", "turnId", "persistent", "configuration", "dispatch", "outcome", "cleanupConfirmed") {
		return e, fmt.Errorf("machine lifecycle omitted or malformed a required fact")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields) // strict object decode above succeeded
	if string(fields["persistent"]) == "null" || string(fields["cleanupConfirmed"]) == "null" ||
		e.LifecycleVersion != 1 || e.Type != "lifecycle" || e.RequestID != f.request.RequestID || !machineOpaqueID(e.RunID) ||
		e.Persistent != (f.request.ThreadMode == "persistent" || f.harness == "antigravity") || e.Sequence < 1 || e.Sequence > 64 ||
		(e.ThreadID != nil && (!machineOpaqueID(*e.ThreadID) || (f.request.ThreadID != "" && *e.ThreadID != f.request.ThreadID))) ||
		(e.TurnID != nil && (!machineOpaqueID(*e.TurnID) || f.harness == "claude-code-pro" || f.harness == "antigravity")) ||
		(e.Dispatch != "not-dispatched" && e.Dispatch != "possible" && e.Dispatch != "accepted") ||
		(e.Outcome != "unknown" && e.Outcome != "completed" && e.Outcome != "failed" && e.Outcome != "cancelled") {
		return e, fmt.Errorf("machine lifecycle identity or facts contradict the invocation")
	}
	previous := ""
	sequence := 1
	if p := f.last; p != nil {
		previous, sequence = p.Event, p.Sequence+1
		if p.RunID != e.RunID || (p.ThreadID != nil && (e.ThreadID == nil || *p.ThreadID != *e.ThreadID)) ||
			(p.TurnID != nil && (e.TurnID == nil || *p.TurnID != *e.TurnID)) ||
			(p.Dispatch == "accepted" && e.Dispatch != "accepted") || (p.Dispatch == "possible" && e.Dispatch == "not-dispatched" && e.Event != "terminal") ||
			previous == "cleanup-confirmed" || (previous == "terminal" && e.Event != "cleanup-confirmed") {
			return e, fmt.Errorf("machine lifecycle identity or execution facts regressed")
		}
	}
	if e.Sequence != sequence {
		return e, fmt.Errorf("machine lifecycle sequence is missing, repeated or reordered")
	}
	terminal := e.Event == "terminal" || e.Event == "cleanup-confirmed"
	if !terminal && (e.Outcome != "unknown" || e.CleanupConfirmed) {
		return e, fmt.Errorf("machine lifecycle claims a premature terminal or cleanup")
	}
	switch e.Event {
	case "thread-ready":
		if f.harness != "codex-pro" || previous != "" || e.ThreadID == nil || e.TurnID != nil || e.Dispatch != "not-dispatched" {
			return e, fmt.Errorf("machine thread-ready is unsupported or out of order")
		}
		var configuration codexProConfiguration
		if err := json.Unmarshal(e.Configuration, &configuration); err != nil {
			return e, fmt.Errorf("machine thread-ready configuration is malformed")
		}
		if err := validateCodexProReadyConfiguration(&configuration, f.request); err != nil {
			return e, err
		}
	case "dispatch-intent":
		ready := f.harness == "codex-pro" && previous == "thread-ready"
		unobserved := (f.harness == "claude-code-pro" || f.harness == "antigravity") && previous == "" && string(e.Configuration) == "null"
		if (!ready && !unobserved) || e.Dispatch != "not-dispatched" || e.TurnID != nil {
			return e, fmt.Errorf("machine dispatch intent has no valid pre-dispatch checkpoint")
		}
	case "dispatch-possible":
		if previous != "dispatch-intent" || e.Dispatch != "possible" || e.TurnID != nil {
			return e, fmt.Errorf("machine possible dispatch is missing its acknowledged intent")
		}
	case "turn-accepted":
		if f.harness != "codex-pro" || previous != "dispatch-possible" || e.Dispatch != "accepted" || e.ThreadID == nil || e.TurnID == nil {
			return e, fmt.Errorf("machine acceptance is missing its dispatch or backend identity")
		}
	case "terminal":
		if e.Outcome == "completed" && (e.Dispatch != "accepted" || e.ThreadID == nil ||
			(f.harness == "codex-pro" && (previous != "turn-accepted" || e.TurnID == nil))) {
			return e, fmt.Errorf("machine terminal completion is missing accepted execution evidence")
		}
	case "cleanup-confirmed":
		if previous != "terminal" || !e.CleanupConfirmed || e.Dispatch != f.last.Dispatch || e.Outcome != f.last.Outcome {
			return e, fmt.Errorf("machine cleanup does not belong to its terminal execution")
		}
	default:
		return e, fmt.Errorf("machine lifecycle event is unsupported")
	}
	f.last = &e
	return e, nil
}

func (f *machineLifecycleFlow) frame(raw []byte) (MachineLifecycle, error) {
	var frame machineLifecycleFrame
	if err := json.Unmarshal(raw, &frame); err != nil || frame.Type != "lifecycle" || frame.Protocol != 3 ||
		frame.RequestID != f.request.RequestID || !machineAdapterMatches(frame.Adapter, f.harness) {
		return MachineLifecycle{}, fmt.Errorf("machine lifecycle envelope does not match the pinned adapter")
	}
	return f.accept(frame.Lifecycle)
}

// machineInteraction runs on the stdout-copy goroutine. The engine is waiting
// for this invocation and is the sole state writer. An ACK is written only after
// its callback fsyncs evidence and completes state CAS; any error starts normal
// watchdog cleanup, never a second dispatch.
type machineInteraction struct {
	mu               sync.Mutex
	stdin            io.Writer
	finish           func()
	err              error
	terminal         bool
	activitySequence int64
	flow             machineLifecycleFlow
	onLifecycle      func(MachineLifecycle) error
	onEvent          func(string)
	lines            jsonLines
}

func newMachineInteraction(harness string, request codexProStart, onLifecycle func(MachineLifecycle) error, onEvent func(string)) *machineInteraction {
	m := &machineInteraction{flow: machineLifecycleFlow{harness: harness, request: request}, onLifecycle: onLifecycle, onEvent: onEvent}
	m.lines = jsonLines{onLine: m.line, limit: watchdog.MaxBuffer}
	return m
}

func (m *machineInteraction) start(stdin io.WriteCloser, finish func(), payload []byte) {
	m.mu.Lock()
	m.stdin, m.finish = stdin, finish
	if m.err == nil {
		if _, err := stdin.Write(payload); err != nil {
			m.err = fmt.Errorf("machine worker request write failed: %w", err)
		}
	}
	failed := m.err != nil
	m.mu.Unlock()
	if failed {
		finish()
	}
}

func (m *machineInteraction) line(line string) {
	if m.onEvent != nil {
		m.onEvent(line)
	}
	m.mu.Lock()
	if m.err != nil {
		m.mu.Unlock()
		return
	}
	var header struct {
		Type      string          `json:"type"`
		Protocol  int             `json:"protocol"`
		RequestID string          `json:"requestId"`
		Text      *string         `json:"text"`
		Adapter   machineAdapter  `json:"adapter"`
		Native    json.RawMessage `json:"native"`
	}
	err := json.Unmarshal([]byte(line), &header)
	if err == nil && (m.stdin == nil || m.terminal || header.Protocol != 3 || header.RequestID != m.flow.request.RequestID) {
		err = fmt.Errorf("machine worker emitted unsolicited, mismatched or post-terminal data")
	}
	if err == nil {
		switch header.Type {
		case "activity":
			err = validateProActivityEvent(json.RawMessage(line), m.flow.harness, m.flow.request, &m.activitySequence)
		case "lifecycle":
			var event MachineLifecycle
			event, err = m.flow.frame([]byte(line))
			if err == nil {
				err = m.onLifecycle(event)
			}
			if err == nil {
				ack := struct {
					Type      string `json:"type"`
					Protocol  int    `json:"protocol"`
					RequestID string `json:"requestId"`
					RunID     string `json:"runId"`
					Sequence  int    `json:"sequence"`
				}{"ack", 3, event.RequestID, event.RunID, event.Sequence}
				err = json.NewEncoder(m.stdin).Encode(ack)
			}
		case "progress":
			if header.Text == nil {
				err = fmt.Errorf("machine progress omitted text")
			}
		case "native-event":
			// Observations are not durable lifecycle checkpoints and receive no
			// ACK. The original stream and final receipt retain their evidence.
			if len(m.flow.request.Native) == 0 || !machineAdapterMatches(header.Adapter, m.flow.harness) || len(header.Native) == 0 || string(header.Native) == "null" {
				err = fmt.Errorf("machine native event is unrequested or uncorrelated")
			}
		case "result":
			m.terminal = true
		default:
			err = fmt.Errorf("machine worker emitted an unsupported frame")
		}
	}
	if err != nil {
		m.err = fmt.Errorf("machine durable lifecycle acknowledgement failed: %w", err)
	}
	finish := m.finish
	m.mu.Unlock()
	if err != nil && finish != nil {
		finish()
	}
}

func (m *machineInteraction) failure() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.err
}

// The stdin pipe remains open for the caller's lifetime. Legacy runs retain the
// same single request behavior; protocol 3 additionally consumes bounded typed
// lifecycle frames and sends durable acknowledgements on that existing pipe.
func runMachineWorker(command, harness string, request codexProStart, payload []byte, opts watchdog.Opts,
	onLifecycle func(MachineLifecycle) error, onEvent func(string)) (watchdog.Result, error) {
	if opts.StdoutCapture == nil && (harness == "codex-pro" || harness == "claude-code-pro") {
		opts.StdoutCapture = newProActivityCapture(harness, request)
	}
	var interaction *machineInteraction
	events := jsonLines{onLine: onEvent, limit: watchdog.MaxBuffer}
	written := make(chan error, 1)
	if request.Protocol == 3 {
		interaction = newMachineInteraction(harness, request, onLifecycle, onEvent)
		opts.OnActivity = interaction.lines.feed
		opts.InteractStdin = func(stdin io.WriteCloser, finish func()) { interaction.start(stdin, finish, payload) }
	} else {
		if onEvent != nil {
			opts.OnActivity = events.feed
		}
		opts.InteractStdin = func(stdin io.WriteCloser, finish func()) {
			_, err := stdin.Write(payload)
			written <- err
			if err != nil {
				finish()
			}
		}
	}
	result := watchdog.Run(command, nil, opts)
	if interaction != nil {
		interaction.lines.flush()
		return result, interaction.failure()
	}
	if onEvent != nil {
		events.flush()
	}
	select {
	case err := <-written:
		if err != nil {
			return result, fmt.Errorf("worker request write failed: %w", err)
		}
		return result, err
	default: // spawn failure or cancellation before Start has no writer
		return result, nil
	}
}
