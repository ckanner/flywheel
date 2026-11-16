package drivers

import (
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestPlanningDriversUseExplicitSelectionAndCompatibleInjection(t *testing.T) {
	cfg := config.Default()
	cfg.Drivers = map[string]string{"plan": "mock", "review": "real", "design-review": "mock", "plan-review": "real"}
	d := CreateDrivers(&cfg)
	assert.Equal(t, "mock", d.ForRole("design-plan").Name())
	assert.Equal(t, "mock", d.ForRole("design-review").Name())
	assert.Equal(t, "real", d.ForRole("plan-review").Name())
	assert.Equal(t, "real", d.ForRole("review").Name())
	assert.Same(t, d.DesignPlan, d.ForRole("design-plan"))
	assert.Same(t, d.DesignReview, d.ForRole("design-review"))
	assert.Equal(t, d.PlanReview, d.ForRole("plan-review"))

	plan, review := NewMockDriver(nil), NewMockDriver(nil)
	injected := &Drivers{Plan: plan, Review: review, Execute: plan, Judge: review, Intake: plan, Garden: review}
	for _, role := range []string{"plan", "design-plan", "execute", "intake"} {
		assert.Same(t, plan, injected.ForRole(role), role)
	}
	for _, role := range []string{"review", "design-review", "plan-review", "judge", "judge-panel", "garden"} {
		assert.Same(t, review, injected.ForRole(role), role)
	}
	assert.Nil(t, injected.ForRole("unknown"))
	assert.Nil(t, (&Drivers{}).ForRole("design-plan"))
}
