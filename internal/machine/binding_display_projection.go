package machine

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ckanner/flywheel/internal/state"
)

const displayBindingBudget = 64 << 10
const displayNativeOptionsLimit = 1 << 10

func compactCurrentExecutionBinding(policy *state.ExecutionPolicyRecord, budget *int) {
	if policy == nil || policy.CurrentBinding == nil {
		return
	}
	bindings, omitted := compactBindingSnapshots(map[string]state.BindingSnapshot{"currentBinding": *policy.CurrentBinding}, budget)
	policy.CurrentBinding, policy.CurrentBindingOmitted = nil, omitted > 0
	if binding, ok := bindings["currentBinding"]; ok {
		policy.CurrentBinding = &binding
	}
}

// These maps are display metadata, not the execution or recovery contract.
// Bound their combined contribution to one reply, including the 100-run list.
// Original frozen records remain intact on disk; no identity or scope string
// is shortened into something that could be mistaken for a complete binding.
func compactBindingSnapshots(bindings map[string]state.BindingSnapshot, budget *int) (map[string]state.BindingSnapshot, int) {
	result := map[string]state.BindingSnapshot{}
	keys := make([]string, 0, len(bindings))
	for key := range bindings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	omitted := 0
	for _, key := range keys {
		binding := bindings[key]
		if len(binding.Binding.NativeOptions) > displayNativeOptionsLimit {
			binding.NativeOptionsHash = fmt.Sprintf("%x", sha256.Sum256(binding.Binding.NativeOptions))
			binding.NativeOptionsOmitted = true
			binding.Binding.NativeOptions = nil
		}
		raw, _ := json.Marshal(map[string]state.BindingSnapshot{key: binding}) // validated JSON, finite scalar metadata
		if len(raw) > *budget {
			omitted++
			continue
		}
		*budget -= len(raw)
		result[key] = binding
	}
	return result, omitted
}
