package config

import "path/filepath"

// ConfigurationScope identifies the single selected file. It does not merge
// the inspected configuration with the file currently open in an editor.
func (c *Config) ConfigurationScope(cwd, taskDir string) string {
	if c.File == "(embedded legal table)" {
		return "embedded"
	}
	if taskDir != "" && c.File == filepath.Join(taskDir, "flywheel.config.json") {
		return "task"
	}
	if c.File == filepath.Join(cwd, "flywheel.config.json") {
		return "project"
	}
	return "global"
}

// InitialExecutionBinding is the configured first executor, before any task's
// recorded adaptive choices. Callers use a validated configuration and route.
// The original Routes declaration remains available for compatibility.
func (c *Config) InitialExecutionBinding(route string) (ExecutionBinding, string) {
	if c.ExecutionPolicy != nil {
		if policy, ok := c.ExecutionPolicy.Routes[route]; ok {
			return c.ExecutionPolicy.Bindings[policy.Initial], policy.Initial
		}
	}
	b := c.Routes[route]
	return ExecutionBinding{NativeProfile: b.NativeProfile, Harness: b.Harness, Provider: b.Provider,
		Channel: b.Channel, Model: b.Model, Thinking: b.Thinking,
		Permission: c.PermissionMode("execute", b.Harness, "routes."+route), SessionMode: c.Sessions.ModeForRole("execute")}, ""
}
