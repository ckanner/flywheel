package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func collaborationProjectionFixture(t *testing.T, split bool, transform ...func(string) string) (fixture, *Context) {
	t.Helper()
	fx, c := newReviewedFixture(t)
	if split {
		p := c.State.Planning
		next := p.Plans[0]
		next.Ref = planning.Ref{Revision: 2}
		next.Files = jsonClone(next.Files)
		next.PlannerInvocation = "collaboration-proposal"
		name := "task-packet-M01-T01.md"
		next.Files[name] = strings.Replace(next.Files[name], "writes: [build/M01-T01.txt]", "writes: [build/M01-T01.txt, build/M01-T01-extra.txt]\ncollaboration:\n  version: 1\n  tasks:\n    - goal: Produce the primary artifact\n      writes: [build/M01-T01.txt]\n    - goal: Produce the independent secondary artifact\n      writes: [build/M01-T01-extra.txt]", 1)
		for _, apply := range transform {
			next.Files[name] = apply(next.Files[name])
		}
		next, err := planning.SealPlan(next)
		require.NoError(t, err)
		require.NoError(t, planning.RecordPlan(p, next))
		require.NoError(t, planning.RecordReview(p, planning.Review{ID: "collaboration-review", Kind: "plan", Target: next.Ref, Design: p.CurrentDesign,
			InputRevision: 1, Invocation: "collaboration-independent-review", Role: "plan-review", Binding: "independent-reviewer", Verdict: "pass", ScopeVerdict: "within-envelope", Summary: "The two write scopes are independent and remain within the approved build scope", Findings: []planning.Finding{}}))
		require.NoError(t, c.checkPlanningExports(true))
	}
	require.NoError(t, c.activatePlanningPlans())
	require.NoError(t, c.reloadPackets())
	return fx, c
}

func TestNodeCollaborationRequiresMatchingReviewedContractWithoutStateMutation(t *testing.T) {
	fx, c := collaborationProjectionFixture(t, true)
	before, err := json.Marshal(c.State)
	require.NoError(t, err)
	node, err := ReadNode(c.State, "M01-T01", fx.taskDir, fx.root)
	require.NoError(t, err)
	require.Equal(t, "verified", node.Collaboration.Status)
	require.Equal(t, c.State.ContractFingerprints["M01-T01"], node.Collaboration.ContractFingerprint)
	require.Len(t, node.Collaboration.Value.Tasks, 2)
	require.Equal(t, []string{"build/M01-T01-extra.txt"}, node.Collaboration.Value.Tasks[1].Writes)
	after, err := json.Marshal(c.State)
	require.NoError(t, err)
	require.Equal(t, before, after)

	packetFile, specFile, configFile := filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), filepath.Join(fx.taskDir, "spec.md"), c.Config.File
	packetRaw, specRaw, configRaw := mustReadFile(t, packetFile), mustReadFile(t, specFile), mustReadFile(t, configFile)
	for _, scenario := range []string{"missing-fingerprint", "legacy", "missing-location", "relative-location", "invalid-id", "invalid-config", "missing-spec", "missing-packet", "invalid-spec", "invalid-packet", "changed-packet", "changed-verification", "missing-plan-ref", "approval-withdrawn"} {
		t.Run(scenario, func(t *testing.T) {
			st := jsonClone(c.State)
			id, location := "M01-T01", []string{fx.taskDir, fx.root}
			t.Cleanup(func() {
				require.NoError(t, os.WriteFile(packetFile, packetRaw, 0644))
				require.NoError(t, os.WriteFile(specFile, specRaw, 0644))
				require.NoError(t, os.WriteFile(configFile, configRaw, 0644))
			})
			switch scenario {
			case "missing-fingerprint":
				st.ContractFingerprints = nil
			case "legacy":
				st.Planning = nil
			case "missing-location":
				location = nil
			case "relative-location":
				location[0] = "relative"
			case "invalid-id":
				id = "../outside"
				st.Packets[id] = st.Packets["M01-T01"]
				st.ContractFingerprints[id] = st.ContractFingerprints["M01-T01"]
			case "invalid-config":
				writeFile(t, configFile, "{")
			case "missing-spec":
				require.NoError(t, os.Remove(specFile))
			case "missing-packet":
				require.NoError(t, os.Remove(packetFile))
			case "invalid-spec":
				writeFile(t, specFile, "broken")
			case "invalid-packet":
				writeFile(t, packetFile, "broken")
			case "changed-packet":
				writeFile(t, packetFile, string(packetRaw)+"\nUnreviewed extra instructions\n")
			case "changed-verification":
				changed := *c.Config
				changed.Verify.CommandTimeoutMs++
				raw, err := json.Marshal(changed)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(configFile, raw, 0644))
			case "missing-plan-ref":
				writeFile(t, specFile, regexp.MustCompile(`(?m)^planning-ref:.*\n`).ReplaceAllString(string(specRaw), ""))
			case "approval-withdrawn":
				st.Planning.StartAuthorized = false
			}
			node, err := ReadNode(st, id, location...)
			require.NoError(t, err)
			require.Equal(t, "unverified", node.Collaboration.Status)
			require.NotEmpty(t, node.Collaboration.Reason)
			require.Nil(t, node.Collaboration.Value, "unreviewed current data must not look like an approved split")
		})
	}
}

func TestNodeDistinguishesReviewedAbsenceFromMissingReviewEvidence(t *testing.T) {
	fx, c := collaborationProjectionFixture(t, false)
	node, err := ReadNode(c.State, "M01-T01", fx.taskDir, fx.root)
	require.NoError(t, err)
	require.Equal(t, "absent", node.Collaboration.Status)
	require.Nil(t, node.Collaboration.Value)
	require.NotEmpty(t, node.Collaboration.ContractFingerprint)
	st := &state.State{Packets: map[string]*state.PacketState{"M01-T01": {}}}
	node, err = ReadNode(st, "M01-T01")
	require.NoError(t, err)
	require.Equal(t, "unverified", node.Collaboration.Status)
}

func TestNodeCollaborationOmitsOversizedReviewedScopesWithoutTruncatingPaths(t *testing.T) {
	fx, c := collaborationProjectionFixture(t, true, func(packet string) string {
		packet = strings.Replace(packet, "writes: [build/M01-T01.txt, build/M01-T01-extra.txt]", "writes: [build/]", 1)
		packet = strings.ReplaceAll(packet, "build/M01-T01.txt", "build/"+strings.Repeat("scope/", 4000)+"primary.txt")
		return strings.ReplaceAll(packet, "build/M01-T01-extra.txt", "build/"+strings.Repeat("scope/", 4000)+"secondary.txt")
	})
	before, err := json.Marshal(c.State)
	require.NoError(t, err)
	node, err := ReadNode(c.State, "M01-T01", fx.taskDir, fx.root)
	require.NoError(t, err)
	require.Equal(t, "unverified", node.Collaboration.Status)
	require.Equal(t, "display-limit-exceeded", node.Collaboration.Reason)
	require.Nil(t, node.Collaboration.Value, "a shortened scope must never look like a complete reviewed split")
	raw, err := json.Marshal(node.Collaboration)
	require.NoError(t, err)
	require.Less(t, len(raw), 1024)
	after, err := json.Marshal(c.State)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
