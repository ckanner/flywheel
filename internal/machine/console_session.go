package machine

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/state"
)

type SessionCommand struct {
	Action   string `json:"action"`
	Key      string `json:"key"`
	Revision int64  `json:"revision"`
	Reason   string `json:"reason,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
}

func (r SessionCommand) Validate() error {
	if r.Revision < 1 || r.Key == "" || len(r.Key) > 256 {
		return fmt.Errorf("session command requires an exact key and positive revision")
	}
	switch r.Action {
	case "claim":
		if strings.TrimSpace(r.Reason) == "" || len(r.Reason) > 1000 || r.Prompt != "" {
			return fmt.Errorf("takeover requires a bounded reason and no prompt")
		}
	case "send":
		if strings.TrimSpace(r.Prompt) == "" || len(r.Prompt) > 65536 || r.Reason != "" {
			return fmt.Errorf("session prompt requires 1–65536 bytes and no takeover reason")
		}
	case "return", "reconcile", "release":
		if r.Prompt != "" || r.Reason != "" {
			return fmt.Errorf("return accepts no prompt or reason")
		}
	default:
		return fmt.Errorf("unknown session action")
	}
	return nil
}

type SessionHistory struct {
	Key        string `json:"key"`
	Thread     string `json:"threadId"`
	Cwd        string `json:"cwd"`
	Invocation string `json:"invocationId"`
	Text       string `json:"text"`
	Truncated  bool   `json:"truncated"`
	Harness    string `json:"harness"`
	Model      string `json:"model"`
}

// Last confirmed provider output is read from the digest-bound receipt, not an
// inferred native history path. It is explicitly not the full conversation.
func ReadSessionHistory(taskDir, key string) (*SessionHistory, error) {
	st, err := state.Load(taskDir)
	if err != nil {
		return nil, err
	}
	if st.Sessions == nil || st.Sessions.Records[key] == nil {
		return nil, fmt.Errorf("session key not found")
	}
	r := st.Sessions.Records[key]
	receipt, _, err := state.ReadSessionReceipt(taskDir, key, r.Last.ID, r.Last.ReceiptDigest)
	if err != nil {
		return nil, err
	}
	var result drivers.Result
	if err := json.Unmarshal(receipt.Result, &result); err != nil {
		return nil, err
	}
	text := result.Text
	truncated := len(text) > 65536
	if truncated {
		text = string([]rune(text)[:min(len([]rune(text)), 16384)])
	}
	return &SessionHistory{Key: key, Thread: r.ThreadID, Cwd: r.Cwd, Invocation: r.Last.ID, Text: text, Truncated: truncated, Harness: result.Harness, Model: result.Model}, nil
}

func (c *Context) handoffArgs(r *state.SessionRecord) (drivers.CallArgs, error) {
	route, id := "", "loops."+r.Role
	if packet := c.Packets[strings.TrimPrefix(r.Scope, "final:")]; packet != nil && r.Role == "execute" {
		route, id = packet.Route, "routes."+packet.Route
	}
	b, err := fallback.ResolveBinding(c.Config, r.Role, route)
	permission, sessionMode := c.Config.PermissionMode(r.Role, b.Harness, id), ""
	if packet := c.Packets[r.Scope]; r.Role == "execute" && packet != nil {
		b, permission, sessionMode, err = c.executionBinding(packet)
	}
	if err != nil {
		return drivers.CallArgs{}, err
	}
	a := drivers.CallArgs{Cwd: r.Cwd, WorkspaceCwd: r.WorkspaceCwd, CandidateScope: r.CandidateScope, TaskDir: c.TaskDir, PacketID: strings.TrimPrefix(r.Scope, "final:"), Model: b.Model,
		Binding:        config.LoopBinding{NativeProfile: b.NativeProfile, Harness: b.Harness, Provider: b.Provider, Channel: b.Channel, Model: b.Model, Thinking: b.Thinking},
		PermissionMode: permission, SessionMode: sessionMode, APIKey: c.channelApiKey(b.Channel), APIKeys: c.apiKeysFor(b.Channel), Watchdog: c.wdog(), ExpectedSessionID: r.ThreadID}
	if err := c.resolveInvocationNative(r.Role, &a); err != nil {
		return drivers.CallArgs{}, err
	}
	if len(r.CandidateScope) > 0 {
		packet := c.Packets[a.PacketID]
		if packet == nil || packet.Schedule == nil {
			return drivers.CallArgs{}, fmt.Errorf("original shared session scope is unavailable")
		}
		a.CandidateScope = slices.Clone(packet.Schedule.Writes)
		slices.Sort(a.CandidateScope)
	}
	binding, goal, _ := c.sessionIdentities(r.Scope, a)
	scoped := c.scopedSessions(r.Role, b.Harness)
	if a.SessionMode != "" {
		scoped = a.SessionMode == "scoped" && c.trackedSessions(r.Role, b.Harness)
	}
	if !scoped || r.ThreadID == "" || !r.ResumeCandidate || binding != r.Binding || goal != r.Goal {
		return drivers.CallArgs{}, fmt.Errorf("original session binding or history is unavailable; inspect configuration and recorded identity")
	}
	return a, nil
}

func (c *Context) runSessionCommand(request SessionCommand) (TaskSummary, error) {
	if err := request.Validate(); err != nil {
		return c.summary(), err
	}
	if c.State.Sessions == nil || c.State.Sessions.Records[request.Key] == nil {
		return c.summary(), fmt.Errorf("session key not found")
	}
	r := c.State.Sessions.Records[request.Key]
	if request.Action == "reconcile" {
		if c.State.Handoff == nil || c.State.Handoff.Key != request.Key {
			return c.summary(), fmt.Errorf("claim this session before reconciling manual work")
		}
		if r.Last.Status == "received" && r.Last.Accounted && r.Last.CleanupConfirmed {
			c.consumeSession(r.Role, r.Scope)
		}
		if r.Last.Status != "consumed" || !r.Last.CleanupConfirmed {
			return c.summary(), fmt.Errorf("manual invocation still requires terminal evidence")
		}
		c.State.Handoff.Pending = false
		c.save()
		summary := c.summary()
		summary.Step = &StepSum{Name: "session-reconcile", OK: c.err() == nil}
		return summary, c.err()
	}
	if r.Last.Status != "consumed" || !r.Last.CleanupConfirmed {
		return c.summary(), fmt.Errorf("session has pending or unknown effects; reconcile its original stage first")
	}
	handoff := c.State.Handoff
	if request.Action == "claim" {
		if handoff != nil {
			return c.summary(), fmt.Errorf("a session is already under human control")
		}
		if _, err := c.handoffArgs(r); err != nil {
			return c.summary(), err
		}
		if reason := sessionQualityReason(r.Role, r, c.Config.Sessions.Quality); reason != "" {
			return c.summary(), fmt.Errorf("session requires history rebuild before takeover: %s", reason)
		}
		candidate, err := c.candidateFingerprint(r.Cwd)
		if err != nil {
			return c.summary(), err
		}
		packetID := strings.TrimPrefix(r.Scope, "final:")
		if c.Packets[packetID] == nil {
			return c.summary(), fmt.Errorf("interactive takeover requires a packet-scoped session; use read-only history for milestone roles")
		}
		c.State.Sessions.Version = max(2, c.State.Sessions.Version) // preserve any newer lifecycle fence
		c.consoleRetryReason = request.Reason
		if err := c.reopenConsolePacket(packetID); err != nil {
			return c.summary(), err
		}
		c.State.Handoff = &state.Handoff{Key: request.Key, Thread: r.ThreadID, Cwd: r.Cwd, Packet: packetID, Candidate: candidate, StartedAt: c.nowISO(), Reason: request.Reason}
	} else {
		if handoff == nil || handoff.Key != request.Key || handoff.Thread != r.ThreadID {
			return c.summary(), fmt.Errorf("this exact session must be claimed before sending or returning")
		}
		if handoff.Pending {
			return c.summary(), fmt.Errorf("reconcile the preceding manual invocation before another operation")
		}
		if request.Action == "release" {
			c.State.Inbox = append(c.State.Inbox, map[string]any{"at": c.nowISO(), "action": "handoff-release", "session": request.Key, "reason": "human control released; verification remains outstanding"})
			c.State.Handoff = nil
			c.State.Status = "paused"
			c.save()
			result := c.summary()
			result.Step = &StepSum{Name: "session-release", OK: c.err() == nil}
			return result, c.err()
		}
		if request.Action == "send" {
			a, err := c.handoffArgs(r)
			if err != nil {
				return c.summary(), err
			}
			if c.budgetGate() == "stop" {
				return c.summary(), fmt.Errorf("budget blocks manual model dispatch")
			}
			a.Prompt = request.Prompt
			handoff.Pending, handoff.PreviousInvocation = true, r.Last.ID
			c.save()
			result, err := c.invokeScoped(r.Role, r.Scope, a, func(args drivers.CallArgs) drivers.Result { return consoleInvoke(args, args.Model) })
			if err != nil {
				return c.summary(), err
			}
			c.recordSpawnMetrics(spawnMetricsArgs{role: r.Role, packetID: handoff.Packet, model: a.Model, channel: a.Binding.Channel, prompt: a.Prompt, result: &result, errorCode: result.ErrorCode})
			current := c.sessionRecord(r.Role, r.Scope)
			// invokeScoped only returns nil error after durable, attributable terminal
			// facts and cleanup. Unknown receipts stay blocked there, before consumption.
			c.consumeSession(r.Role, r.Scope)
			c.State.Handoff.Pending = false
			if result.ErrorCode != "" || !result.Settled || result.ExitCode == nil || *result.ExitCode != 0 || result.Cancelled || result.TimedOut || result.Truncated || result.Signal != "" || result.Session.Outcome != "completed" {
				c.save()
				return c.summary(), fmt.Errorf("manual session call did not complete successfully (%s)", result.ErrorCode)
			}
			c.State.Inbox = append(c.State.Inbox, map[string]any{"at": c.nowISO(), "action": "human-prompt", "session": request.Key, "invocation": current.Last.ID})
		} else {
			packet := c.Packets[handoff.Packet]
			if packet == nil {
				return c.summary(), fmt.Errorf("takeover packet no longer exists")
			}
			if !c.l1Step(packet).Pass || c.quick() && !c.quickGateStep(packet) {
				return c.summary(), fmt.Errorf("handoff validation failed; human control retained")
			}
			c.State.Packets[handoff.Packet].Evidence.Executor = map[string]any{"source": "human-takeover", "identity": evidenceIdentity(c.State.Packets[handoff.Packet].Evidence.L1), "session": handoff.Key, "at": c.nowISO()}
			c.save()
			review, err := c.reviewStep(packet, nil)
			if err != nil {
				return c.summary(), err
			}
			if !review.Pass {
				return c.summary(), fmt.Errorf("independent review did not pass; human control retained")
			}
			c.State.Inbox = append(c.State.Inbox, map[string]any{"at": c.nowISO(), "action": "handoff-return", "session": request.Key, "candidate": evidenceIdentity(c.State.Packets[handoff.Packet].Evidence.L1)})
			c.State.Handoff = nil
		}
	}
	c.State.Status = "paused"
	c.save()
	summary := c.summary()
	summary.Step = &StepSum{Name: "session-" + request.Action, OK: c.err() == nil}
	return summary, c.err()
}

var consoleInvoke = drivers.InvokeHarness
