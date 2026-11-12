package machine

import (
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
	if c.RunContext != nil && c.RunContext.Err() != nil {
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
	options := shadow.Options{Endpoint: j.Endpoint, Model: j.Model, ExpectedModel: j.ExpectedModel,
		Timeout: time.Duration(j.TimeoutMS) * time.Millisecond, MaxCalls: 1, SaveInput: true}
	directory := shadow.ObservationDirectory(filepath.Join(filepath.Dir(path), "shadow"), data, options)
	if err := shadow.Enqueue(data, directory, options); err != nil {
		c.Log("JEV shadow: unavailable (target, input or record validation); workflow unchanged")
		return
	}
	c.Log("JEV shadow: snapshot queued; workflow unchanged; review cases.json and explicitly run a separately budgeted shadow experiment: " + directory)
}
