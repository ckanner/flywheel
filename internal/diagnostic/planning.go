package diagnostic

import (
	"fmt"
	"sort"

	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
)

// planningScope is read-only. A partially projected activation is deliberately
// not loaded as an implementation contract or repaired by diagnostics.
func (r *Report) planningScope(st *state.State) (map[string]bool, bool) {
	p := st.Planning
	required := map[string]bool{}
	r.add("planning", "pass", fmt.Sprintf("规划状态已验证：%s，输入版本 %d", p.Phase, p.InputRevision), "", "")
	if p.PendingActivation != nil {
		r.add("planning.activation", "unknown", "存在尚未完成的激活事务；实现调用仍被阻止，未读取可能处于半投影状态的契约", "", "通过规划恢复入口核对原事务；doctor 不恢复、不批准或启动实现")
		return required, false
	}
	if p.Phase != "ready" || p.Active == nil {
		planner, reviewer := "design-plan", "design-review"
		currentPlan := false
		if p.CurrentPlan != nil {
			plan := planning.FindPlan(p, *p.CurrentPlan)
			currentPlan = plan != nil && plan.InputRevision == p.InputRevision
		}
		if p.Phase == "approved" || currentPlan {
			planner, reviewer = "plan", "plan-review"
		}
		if len(p.Requests) > 0 {
			request := p.Requests[len(p.Requests)-1]
			if request.InputRevision == p.InputRevision {
				scope, _ := planning.CanonicalScope(request.Scope, request.ScopeID)
				if scope == "design" {
					planner, reviewer = "design-plan", "design-review"
				} else if scope == "milestone" || scope == "packet" {
					planner, reviewer = "plan", "plan-review"
				}
			}
		}
		required["loops."+planner], required["loops."+reviewer] = true, true
	}
	if p.Active == nil {
		r.add("contracts", "skipped", "规划任务尚无已激活的 Spec / packets；当前阶段只检查规划绑定", "", "")
		return required, false
	}
	return required, true
}

func (r *Report) requireBindings(required map[string]bool) {
	present := map[string]bool{}
	for _, b := range r.Configuration.Bindings {
		present[b.ID] = true
	}
	ids := make([]string, 0, len(required))
	for id := range required {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if !present[id] {
			r.add(id, "fail", "缺少任务所需的模型绑定", "INVALID_REQUEST", "在 loops 或 routes 中补齐此绑定")
		}
	}
}
