package config

// PlanningBaseRole shares only backend defaults. The caller must retain the
// requested role in its scope, history, admission and result contract.
func PlanningBaseRole(role string) string {
	switch role {
	case "design-plan":
		return "plan"
	case "design-review", "plan-review":
		return "review"
	default:
		return role
	}
}

// LoopBindingForRole resolves an explicit binding before the planning-role
// default. It does not materialize inherited values into user configuration.
// sourceRole identifies the binding to show in read-only diagnostics.
func (c *Config) LoopBindingForRole(role string) (binding LoopBinding, sourceRole string, ok bool) {
	if binding, ok = c.Loops[role]; ok {
		return binding, role, true
	}
	sourceRole = PlanningBaseRole(role)
	binding, ok = c.Loops[sourceRole]
	return binding, sourceRole, ok
}

// DriverKind follows the same explicit/default distinction as the model
// binding. An absent or empty kind retains the existing real-driver default.
func (c *Config) DriverKind(role string) string {
	if kind, present := c.Drivers[role]; present {
		return kind
	}
	return c.Drivers[PlanningBaseRole(role)]
}
