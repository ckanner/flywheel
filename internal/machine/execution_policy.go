package machine

import (
	"fmt"
	"slices"

	"github.com/ckanner/flywheel/internal/circuitbreaker"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/errs"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/state"
)

func (c *Context) executionPolicy(packet *contract.Packet) (*state.ExecutionPolicyRecord, config.ExecutionRoute, error) {
	record := c.State.Packets[packet.PacketID].ExecutionPolicy
	p := c.Config.ExecutionPolicy
	var route config.ExecutionRoute
	enabled := false
	if p != nil {
		route, enabled = p.Routes[packet.Route]
	}
	if !enabled {
		if record != nil {
			return nil, route, fmt.Errorf("packet %s has a frozen execution policy that is no longer configured", packet.PacketID)
		}
		return nil, route, nil
	}
	bindings := map[string]config.ExecutionBinding{}
	channels := map[string]config.Channel{}
	native := map[string]config.NativeProfile{}
	for _, id := range route.Allowed {
		binding := p.Bindings[id]
		bindings[id], channels[binding.Channel] = binding, c.Config.Channels[binding.Channel]
		if binding.NativeProfile != "" {
			native[binding.NativeProfile] = c.Config.NativeProfiles[binding.NativeProfile]
		}
	}
	identityInput := []any{p.Version, route, bindings, channels}
	if len(native) != 0 {
		identityInput = append(identityInput, native)
	}
	identity := sessionHash(identityInput)
	if record == nil {
		record = &state.ExecutionPolicyRecord{Version: 1, Identity: identity, Route: packet.Route, Current: route.Initial,
			CurrentBinding: &state.BindingSnapshot{BindingID: route.Initial, Binding: c.executionBindingSnapshot(p.Bindings[route.Initial])}}
		c.State.Packets[packet.PacketID].ExecutionPolicy = record
	}
	if record.Version != 1 || record.Identity != identity || record.Route != packet.Route || !slices.Contains(route.Allowed, record.Current) || record.Switches != len(record.History) {
		return nil, route, fmt.Errorf("packet %s execution policy identity changed; restore its frozen allowed bindings before continuing", packet.PacketID)
	}
	return record, route, nil
}

func (c *Context) executionBinding(packet *contract.Packet) (fallback.Binding, string, string, error) {
	record, _, err := c.executionPolicy(packet)
	if err != nil {
		return fallback.Binding{}, "", "", err
	}
	if record == nil {
		binding, err := fallback.ResolveBinding(c.Config, "execute", packet.Route)
		if err == nil {
			err = c.executionCandidateError(packet, config.ExecutionBinding{Harness: binding.Harness, NativeProfile: binding.NativeProfile})
		}
		return binding, c.Config.PermissionMode("execute", binding.Harness, "routes."+packet.Route), "", err
	}
	b := c.Config.ExecutionPolicy.Bindings[record.Current]
	return fallback.Binding{NativeProfile: b.NativeProfile, Harness: b.Harness, Provider: b.Provider, Channel: b.Channel, Model: b.Model, Thinking: b.Thinking}, b.Permission, b.SessionMode, c.executionCandidateError(packet, b)
}

func executionConfiguration(b config.ExecutionBinding) state.SessionConfiguration {
	return state.SessionConfiguration{NativeProfile: b.NativeProfile, Harness: b.Harness, Model: b.Model, Provider: b.Provider, Channel: b.Channel, Thinking: b.Thinking, Permission: b.Permission, SessionMode: b.SessionMode}
}

func (c *Context) switchExecutionPolicy(packet *contract.Packet, record *state.ExecutionPolicyRecord, route config.ExecutionRoute, next, kind, reason string) bool {
	if next == "" || next == record.Current || !slices.Contains(route.Allowed, next) || record.Switches >= route.MaxSwitches {
		return false
	}
	if c.executionCandidateError(packet, c.Config.ExecutionPolicy.Bindings[next]) != nil {
		return false
	}
	// A new model cannot bypass an unconsumed or unknown invocation. The normal
	// Execute entry reconciles all roles before it may select or dispatch again.
	if r := c.sessionRecord("execute", packet.PacketID); r != nil && r.Last.Status != "consumed" {
		return false
	}
	p := c.Config.ExecutionPolicy
	record.History = append(record.History, state.ExecutionSwitch{Kind: kind, From: record.Current, To: next,
		Before: executionConfiguration(p.Bindings[record.Current]), After: executionConfiguration(p.Bindings[next]),
		Reason: reason, At: c.nowISO(), Attempt: c.State.Packets[packet.PacketID].Attempts})
	record.Current, record.Switches, record.StalledAttempts = next, record.Switches+1, 0
	record.CurrentBinding = &state.BindingSnapshot{BindingID: next, Binding: c.executionBindingSnapshot(p.Bindings[next])}
	c.retireSession("execute", packet.PacketID, kind+": "+reason+"; selected binding requires a cold start")
	c.Log(fmt.Sprintf("execution %s: %s -> %s (%s)", packet.PacketID, kind, next, reason))
	return true
}

// This hook runs before an unchanged, verified L1 failure would trigger Plan.
// It does not reset attempts, authorize more tokens or reinterpret transport
// errors as poor reasoning. No remaining packet budget means no new selection.
func (c *Context) upgradeExecutionPolicy(packet *contract.Packet, code errs.ErrorCode, assessment circuitbreaker.ProgressAssessment) bool {
	ps := c.State.Packets[packet.PacketID]
	if code != errL1Failed || assessment.Status != "stalled" || ps.FailedAttempts >= packet.Escalation.MaxRetries {
		return false
	}
	record, route, err := c.executionPolicy(packet)
	if err != nil {
		c.fatal = err
		return false
	}
	if record == nil || record.LastObservedAttempt >= ps.Attempts {
		return false
	}
	if c.budgetGate() == "stop" {
		return false
	}
	record.LastObservedAttempt = ps.Attempts
	record.StalledAttempts++
	index := slices.Index(route.QualityOrder, record.Current)
	if index < 0 || index+1 >= len(route.QualityOrder) {
		return false
	}
	return c.switchExecutionPolicy(packet, record, route, route.QualityOrder[index+1], "quality-upgrade", assessment.Reason)
}

// Policy routes require a complete explicitly allowed destination even when the
// legacy channel chain is configured. Other routes retain their old behavior.
func (c *Context) executionChannelFallback(packet *contract.Packet, nextChannel, code string) (bool, error) {
	record, route, err := c.executionPolicy(packet)
	if err != nil {
		return false, err
	}
	if record == nil {
		return true, nil
	}
	next := route.ChannelFallbacks[record.Current]
	if next == "" || c.Config.ExecutionPolicy.Bindings[next].Channel != nextChannel {
		return false, nil
	}
	return c.switchExecutionPolicy(packet, record, route, next, "channel-fallback", "confirmed channel error "+code), nil
}
