package investigation

import (
	"context"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/decision"
	"github.com/ckanner/flywheel/internal/decisionexec"
	"github.com/ckanner/flywheel/internal/state"
)

var subscriptionOptions = decisionexec.Options

func freezeDecision(global *config.Config, policy decision.Policy) *contract.InvestigationDecision {
	j := global.JEV
	d := &contract.InvestigationDecision{Policy: policy, Endpoint: j.Endpoint, Model: j.Model, KeyEnv: j.KeyEnv, TimeoutMS: j.TimeoutMS, HardBudget: global.Budget.Enforce == "hard"}
	if j.Backend == "subscription" {
		d.Backend = j.Backend
		d.Subscription, _ = global.ResolveJEVSubscription() // Configuration validation already established this binding.
		if d.Subscription != nil {
			d.Model = d.Subscription.Model
		}
	}
	return d
}

func investigationDecisionOptions(ctx context.Context, st *state.PreparationState, dir string) decision.Options {
	d := st.Contract.Decision
	if d.Backend == "subscription" {
		return subscriptionOptions(ctx, d.Subscription, st.Cwd, dir, time.Duration(d.TimeoutMS)*time.Millisecond)
	}
	return decision.Options{Endpoint: d.Endpoint, Model: d.Model, ExpectedModel: d.Model, Timeout: time.Duration(d.TimeoutMS) * time.Millisecond}
}

func subscriptionDecisionCurrent(ctx context.Context, global *config.Config, frozen *contract.InvestigationDecision) bool {
	if frozen.Backend != "subscription" {
		return global.JEV.Backend != "subscription"
	}
	if ctx.Err() != nil || global.JEV.Backend != "subscription" {
		return false
	}
	current, err := global.ResolveJEVSubscription()
	return err == nil && state.PreparationHash(current) == state.PreparationHash(frozen.Subscription)
}
