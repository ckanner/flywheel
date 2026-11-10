package machine

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/shadow"
)

var shadowConfigPath = config.GlobalConfigPath

// Observation is deliberately outside workflow decisions. No observer result
// or error is passed into state, retries, sessions, delivery or permission gates.
func (c *Context) observeShadow() {
	j := c.Config.JEVShadow
	if !j.Enabled || !c.Fresh {
		return
	}
	if c.Config.Budget.Enforce == "hard" {
		c.Log("JEV shadow: skipped (hard budget; optional experiment is separately metered)")
		return
	}
	key, _ := c.EnvLookup(j.KeyEnv)
	if key == "" {
		c.Log("JEV shadow: skipped (missing API key environment variable); workflow unchanged")
		return
	}
	data, err := shadow.Capture(c.TaskDir, c.Spec, c.PacketList, c.State.Workflow.WorkflowSelection)
	if err != nil {
		c.Log("JEV shadow: skipped (summary validation); workflow unchanged")
		return
	}
	path, err := shadowConfigPath()
	if err != nil {
		c.Log("JEV shadow: skipped (result directory unavailable); workflow unchanged")
		return
	}
	options := shadow.Options{Endpoint: j.Endpoint, Model: j.Model, ExpectedModel: j.ExpectedModel, APIKey: key,
		Timeout: time.Duration(j.TimeoutMS) * time.Millisecond, MaxCalls: 1, SaveInput: true}
	directory := shadow.ObservationDirectory(filepath.Join(filepath.Dir(path), "shadow"), data, options)
	ctx, cancel := context.WithTimeout(c.RunContext, 5*time.Second)
	defer cancel()
	report, err := shadow.Run(ctx, data, directory, options)
	if err != nil {
		c.Log("JEV shadow: unavailable (target, input or record validation); workflow unchanged")
		return
	}
	c.Log(fmt.Sprintf("JEV shadow: quick=%d full=%d abstain=%d errors=%d unknown=%d; workflow unchanged; separate usage and results: %s", report.Summary.QuickCandidates, report.Summary.FullCandidates, report.Summary.Abstentions, report.Summary.Errors, report.Summary.Unknown, directory))
}
