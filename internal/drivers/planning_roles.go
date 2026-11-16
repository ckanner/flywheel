package drivers

// ForRole selects the configured driver without changing the invocation role.
// Existing injected drivers need not define new fields: their Plan and Review
// implementations remain usable with independent planning role inputs.
func (d *Drivers) ForRole(role string) Driver {
	switch role {
	case "design-plan":
		if d.DesignPlan != nil {
			return d.DesignPlan
		}
		return d.Plan
	case "design-review":
		if d.DesignReview != nil {
			return d.DesignReview
		}
		return d.Review
	case "plan-review":
		if d.PlanReview != nil {
			return d.PlanReview
		}
		return d.Review
	case "plan":
		return d.Plan
	case "execute":
		return d.Execute
	case "review":
		return d.Review
	case "judge", "judge-panel":
		return d.Judge
	case "intake":
		return d.Intake
	case "garden":
		return d.Garden
	default:
		return nil
	}
}
