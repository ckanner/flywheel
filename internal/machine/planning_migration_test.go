package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func migrationFixture(t *testing.T) (fixture, RunOpts, *state.State, PlanningCommand) {
	t.Helper()
	fx := newGitFixture(t)
	st := loadFixtureState(t, fx)
	state.RecordUsage(st, state.Usage{Input: 123, Output: 45}, state.UsageOpts{Day: "2026-11-16", Provider: "legacy"})
	st.Inbox = append(st.Inbox, map[string]any{"kind": "legacy-audit", "message": "Keep original history"})
	st, err := state.Save(fx.taskDir, st, nil)
	require.NoError(t, err)
	return fx, RunOpts{TaskDir: fx.taskDir, Cwd: fx.root, Config: mustConfig(t, fx)}, st,
		PlanningCommand{Action: "migrate", Revision: st.Revision, RequestID: "migrate-once", Start: new(false)}
}

func TestPlanningMigrationKeepsLegacyEvidenceWithoutInventingReview(t *testing.T) {
	fx, opts, before, command := migrationFixture(t)
	packet := mustReadFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"))
	st, receipt, err := ControlPlanning(opts, command, "")
	require.NoError(t, err)
	require.Equal(t, "applied", receipt.Status)
	require.Equal(t, before.Budget, st.Budget)
	require.Equal(t, before.Inbox, st.Inbox)
	require.Equal(t, before.Packets, st.Packets)
	require.Equal(t, before.Milestones, st.Milestones)
	require.Equal(t, 4, st.Sessions.Version)
	require.Equal(t, fx.root, st.Project.Cwd)
	require.Len(t, st.Planning.LegacyFiles, 3)
	require.Nil(t, st.Planning.CurrentDesign)
	require.Nil(t, st.Planning.CurrentPlan)
	require.Nil(t, st.Planning.Active)
	require.Empty(t, st.Planning.Designs)
	require.Empty(t, st.Planning.Plans)
	require.Empty(t, st.Planning.Reviews)
	require.Empty(t, st.Planning.Approvals)
	require.True(t, st.Planning.StartAuthorized, "migration retains the prior implementation grant but creates no design approval")
	require.True(t, st.Planning.Brief.ImplementationAuthorized, "the original implementation grant remains visible; review-first still pauses")
	require.Equal(t, packet, mustReadFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md")))
	spec, err := contract.LoadSpec(fx.taskDir)
	require.NoError(t, err)
	require.NotNil(t, spec.PlanningRef, "legacy binaries see the explicit downgrade fence")
	for name, hash := range st.Planning.LegacyFiles {
		require.Equal(t, planning.DigestBytes(mustReadFile(t, filepath.Join(fx.taskDir, name))), hash)
	}
	var intent planningMigration
	require.NoError(t, json.Unmarshal(mustReadFile(t, filepath.Join(fx.taskDir, ".control", "planning", "migration.json")), &intent))
	require.Equal(t, fixtureSpec, intent.Original["spec.md"])
	require.Equal(t, string(packet), intent.Original["task-packet-M01-T01.md"])
	again, receipt, err := ControlPlanning(opts, command, "")
	require.NoError(t, err)
	require.Equal(t, "unchanged", receipt.Status)
	require.Equal(t, st.Revision, again.Revision)
}

func TestPlanningMigrationRecoversBeforeAndAfterDowngradeFence(t *testing.T) {
	for _, afterFence := range []bool{false, true} {
		t.Run(map[bool]string{false: "fence-write-failed", true: "state-CAS-failed"}[afterFence], func(t *testing.T) {
			fx, opts, before, command := migrationFixture(t)
			original := writePlanningBytes
			t.Cleanup(func() { writePlanningBytes = original })
			writePlanningBytes = func(root *os.Root, name string, raw []byte) error {
				if name != "spec.md" {
					return original(root, name, raw)
				}
				if !afterFence {
					return os.ErrPermission
				}
				if err := original(root, name, raw); err != nil {
					return err
				}
				require.NoError(t, os.Remove(filepath.Join(fx.taskDir, ".state.lock")))
				return os.Mkdir(filepath.Join(fx.taskDir, ".state.lock"), 0700)
			}
			_, _, err := ControlPlanning(opts, command, "")
			require.Error(t, err)
			require.FileExists(t, filepath.Join(fx.taskDir, ".control", "planning", "migration.json"))
			unchanged := loadFixtureState(t, fx)
			require.Nil(t, unchanged.Planning)
			require.Equal(t, before.Budget, unchanged.Budget)
			spec, err := contract.LoadSpec(fx.taskDir)
			require.NoError(t, err)
			require.Equal(t, afterFence, spec.PlanningRef != nil)
			writePlanningBytes = original
			if afterFence {
				require.NoError(t, os.Remove(filepath.Join(fx.taskDir, ".state.lock")))
			}
			st, _, err := ControlPlanning(opts, command, "")
			require.NoError(t, err)
			require.Equal(t, before.Budget, st.Budget)
			require.Empty(t, st.Planning.Designs)
			require.Empty(t, st.Planning.Approvals)
		})
	}
}

func TestPlanningMigrationRefusesChangedOrIncompleteSourceOwnership(t *testing.T) {
	for _, scenario := range []string{"changed-contract", "different-request", "invalid-json", "invalid-brief", "missing-spec", "invalid-fence", "foreign-key", "invalid-path", "bad-frontmatter", "lost-source"} {
		t.Run(scenario, func(t *testing.T) {
			fx, opts, st, command := migrationFixture(t)
			intent, err := newPlanningMigration(opts, st, command)
			require.NoError(t, err)
			switch scenario {
			case "changed-contract":
				writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), fixturePacket("M01-T01", "R-01")+"\nchanged concurrently\n")
			case "different-request":
				intent.Command.RequestID = "other"
			case "invalid-brief":
				intent.Brief.Goal = ""
			case "missing-spec":
				delete(intent.Original, "spec.md")
			case "invalid-fence":
				intent.Fenced["spec.md"] += "\nextra content\n"
			case "foreign-key":
				delete(intent.Fenced, "task-packet-M01-T01.md")
				intent.Fenced["task-packet-M99-T99.md"] = "unowned"
			case "invalid-path":
				intent.Original["../escape"], intent.Fenced["../escape"] = "escape", "escape"
				ref := planning.ActivationRef{Version: 1, Design: planning.Ref{Revision: 1, Digest: planning.DigestBytes([]byte(intent.Original["spec.md"]))}, Plans: []planning.Ref{{Revision: 1, Digest: sessionHash(intent.Original)}}}
				raw, err := planningFence([]byte(intent.Original["spec.md"]), ref)
				require.NoError(t, err)
				intent.Fenced["spec.md"] = string(raw)
			case "bad-frontmatter":
				intent.Original["spec.md"] = "not a spec"
			case "lost-source":
				require.NoError(t, os.Remove(filepath.Join(fx.taskDir, "task-packet-M01-T01.md")))
			}
			raw, err := json.Marshal(intent)
			require.NoError(t, err)
			if scenario == "invalid-json" {
				raw = []byte("{broken")
			}
			writeFile(t, filepath.Join(fx.taskDir, ".control", "planning", "migration.json"), string(raw))
			_, _, err = ControlPlanning(opts, command, "")
			require.Error(t, err)
			require.Nil(t, loadFixtureState(t, fx).Planning)
		})
	}
}

func TestPlanningMigrationAdmissionAndFactoryFailures(t *testing.T) {
	for _, scenario := range []string{"missing-state", "missing-cwd", "foreign-cwd", "unsettled", "control-path", "stale", "bad-spec", "bad-packet", "already-fenced", "legacy-mode"} {
		t.Run(scenario, func(t *testing.T) {
			fx, opts, st, command := migrationFixture(t)
			switch scenario {
			case "missing-state":
				require.NoError(t, os.Remove(state.StateFile(fx.taskDir)))
			case "missing-cwd":
				opts.Cwd = filepath.Join(fx.root, "missing")
			case "foreign-cwd":
				st.Project = &state.ProjectIdentity{Cwd: t.TempDir(), WorkspaceCwd: fx.root}
				_, err := state.Save(fx.taskDir, st, nil)
				require.NoError(t, err)
			case "unsettled":
				st.PlanSession = map[string]any{"outcome-unknown": true}
				_, err := state.Save(fx.taskDir, st, nil)
				require.NoError(t, err)
			case "control-path":
				writeFile(t, filepath.Join(fx.taskDir, ".control"), "file")
			case "stale":
				command.Revision++
			case "bad-spec":
				writeFile(t, filepath.Join(fx.taskDir, "spec.md"), "invalid")
			case "bad-packet":
				writeFile(t, filepath.Join(fx.taskDir, "task-packet-M01-T01.md"), "invalid")
			case "already-fenced":
				raw, err := planningFence([]byte(fixtureSpec), planning.ActivationRef{Version: 1, Design: planning.Ref{Revision: 1, Digest: strings.Repeat("a", 64)}, Plans: []planning.Ref{{Revision: 1, Digest: strings.Repeat("b", 64)}}})
				require.NoError(t, err)
				writeFile(t, filepath.Join(fx.taskDir, "spec.md"), string(raw))
			case "legacy-mode":
				st.Workflow = &state.WorkflowState{Requested: "full", Effective: "full", Reason: "existing", SelectedAt: state.NowISO()}
				intent, err := newPlanningMigration(opts, st, command)
				require.NoError(t, err)
				require.Equal(t, "full", intent.Brief.Mode)
				return
			}
			_, _, err := ControlPlanning(opts, command, "")
			require.Error(t, err)
		})
	}
	_, err := planningFence([]byte("invalid"), planning.ActivationRef{})
	require.Error(t, err)
	_, err = planningFence([]byte("---\nnested:\n  ? [one, two]\n  : value\n---\n"), planning.ActivationRef{})
	require.Error(t, err)
	// A preserved intent cannot silently be attached to unrelated planning state.
	fx, opts, st, command := migrationFixture(t)
	intent, err := newPlanningMigration(opts, st, command)
	require.NoError(t, err)
	raw, err := json.Marshal(intent)
	require.NoError(t, err)
	writeFile(t, filepath.Join(fx.taskDir, ".control", "planning", "migration.json"), string(raw))
	p, err := planning.New(intent.Brief)
	require.NoError(t, err)
	p.LegacyFiles = map[string]string{"spec.md": strings.Repeat("a", 64)}
	st.Planning, st.Sessions = p, &state.SessionLedger{Version: 4, Records: map[string]*state.SessionRecord{}}
	_, err = state.Save(fx.taskDir, st, nil)
	require.NoError(t, err)
	_, _, err = ControlPlanning(opts, command, "")
	require.ErrorContains(t, err, "differs from the original migration")
}
