package machine

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/ckanner/flywheel/internal/metrics"
)

func (c *Context) measurePhase(name, scope string) func() {
	return c.measurePhaseWithInvocation(name, scope, nil)
}

// This span includes managed admission, recovery, and durable receipt handling.
// Its duration is not a provider-only latency measurement.
func (c *Context) measureManagedInvocation(role, scope string) func() {
	return c.measurePhaseWithInvocation("managed-invocation", scope, func() string {
		if record := c.sessionRecord(role, scope); record != nil && record.Last != nil {
			return record.Last.ID
		}
		return ""
	})
}

func (c *Context) measurePhaseWithInvocation(name, scope string, invocation func() string) func() {
	if c.runID == "" || c.metricsFile == "" || c.fatal != nil {
		return func() {}
	}
	phase := metrics.Phase{ID: rand.Text(), Run: c.runID, Name: name, Scope: scope, ParentID: c.phaseParent, StartedAt: c.Now()}
	if err := metrics.BeginPhase(c.metricsFile, phase); err != nil {
		c.fatal = fmt.Errorf("phase start could not be persisted: %w", err)
		return func() {}
	}
	parent := c.phaseParent
	c.phaseParent = phase.ID
	finished := false
	return func() {
		if finished {
			return
		}
		finished = true
		ended := c.Now()
		phase.EndedAt = &ended
		if invocation != nil {
			phase.InvocationID = invocation()
		}
		c.phaseParent = parent
		if err := metrics.EndPhase(c.metricsFile, phase); err != nil && c.fatal == nil {
			c.fatal = fmt.Errorf("phase end could not be persisted: %w", err)
		}
	}
}

// Only transport-backoff call sites use this helper. Human-approval polling and
// other sleeps retain their own semantics and are not relabelled as retry time.
func (c *Context) waitRetry(delay time.Duration, scope string) {
	defer c.measurePhase("retry-wait", scope)()
	if c.err() == nil {
		c.Sleep(delay)
	}
}
