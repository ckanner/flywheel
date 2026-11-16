package machine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

// A migrated fixture with a real approved root and independently reviewed child
// proposal, before activation. Its legacy Spec/packets are owned by exact bytes.
func newReviewedFixture(t *testing.T) (fixture, *Context) {
	t.Helper()
	fx := newGitFixture(t)
	c := unitTestContext(t, fx)
	p, err := planning.New(planning.Brief{Version: 1, ID: fx.slug, Goal: c.Spec.Goal,
		OriginalRequest: "Review the design before implementation", Constraints: []string{}, Evidence: []string{}, Policy: "review-first", Mode: "full"})
	require.NoError(t, err)
	c.State.Planning, c.State.Sessions = p, &state.SessionLedger{Version: 4, Records: map[string]*state.SessionRecord{}}
	c.State.Project = &state.ProjectIdentity{Cwd: fx.root}
	p.LegacyFiles = map[string]string{}
	files := map[string]string{"spec.md": fixtureSpec}
	for _, packet := range c.PacketList {
		name := "task-packet-" + packet.PacketID + ".md"
		raw := string(mustReadFile(t, filepath.Join(fx.taskDir, name)))
		p.LegacyFiles[name] = planning.DigestBytes([]byte(raw))
		files[name] = strings.Replace(raw, "route: default\n", "route: default\nschedule:\n  after: []\n  writes: [build/"+packet.PacketID+".txt]\n", 1)
	}
	p.LegacyFiles["spec.md"] = planning.DigestBytes([]byte(fixtureSpec))
	design := planning.Design{Ref: planning.Ref{Revision: 1}, InputRevision: 1,
		Document:     "# Artifact generation\nEach packet writes one artifact; test the real output before delivery.",
		Requirements: []planning.Requirement{}, Milestones: []planning.Milestone{}, Decisions: []planning.Decision{},
		Baseline: strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD")), PlannerInvocation: "design-proposal", ImplementationScope: []string{"build"}}
	for _, r := range c.Spec.Requirements {
		design.Requirements = append(design.Requirements, planning.Requirement{ID: r.ID, Title: r.Title, Milestone: r.Milestone, Verification: r.Verification})
	}
	for _, m := range c.Spec.Milestones {
		design.Milestones = append(design.Milestones, planning.Milestone{ID: m.ID, Title: m.Title, Gate: m.Gate})
	}
	design, err = planning.SealDesign(design)
	require.NoError(t, err)
	require.NoError(t, planning.RecordDesign(p, design))
	require.NoError(t, planning.RecordReview(p, planning.Review{ID: "root-review", Kind: "design", Target: design.Ref,
		InputRevision: 1, Invocation: "root-review-call", Role: "design-review", Binding: "independent-reviewer", Verdict: "pass", Summary: "Requirements have testable gates", Findings: []planning.Finding{}}))
	require.NoError(t, planning.Approve(p, planning.Approval{Kind: "human", Design: design.Ref, ReviewID: "root-review",
		RequestID: "root-approval", Source: "verified-user-action", At: c.nowISO(), Start: true}))
	require.NoError(t, c.checkPlanningExports(true))
	dependency, err := c.planScopeIdentity()
	require.NoError(t, err)
	plan, err := planning.SealPlan(planning.ExecutionPlan{Ref: planning.Ref{Revision: 1}, Design: design.Ref, InputRevision: 1,
		Scope: "milestone", ScopeID: "M-01", Files: files, Dependencies: dependency, ScopeVerdict: "within-envelope", PlannerInvocation: "child-proposal"})
	require.NoError(t, err)
	require.NoError(t, planning.RecordPlan(p, plan))
	require.NoError(t, planning.RecordReview(p, planning.Review{ID: "child-review", Kind: "plan", Target: plan.Ref, Design: &design.Ref,
		InputRevision: 1, Invocation: "child-review-call", Role: "plan-review", Binding: "independent-reviewer", Verdict: "pass", ScopeVerdict: "within-envelope", Summary: "Files and tests remain in scope", Findings: []planning.Finding{}}))
	c.save()
	require.NoError(t, c.err())
	require.NoError(t, c.checkPlanningExports(true))
	return fx, c
}

func TestReviewedActivationCommitsExactProjectionAndResumesIdempotently(t *testing.T) {
	fx, c := newReviewedFixture(t)
	require.NoError(t, c.validatePlanningFiles(c.State.Planning.Plans[0].Files, planning.Request{Scope: "milestone", ScopeID: "M-01"}))
	require.NoError(t, c.activatePlanningPlans())
	p := c.State.Planning
	require.Equal(t, "ready", p.Phase)
	require.Nil(t, p.PendingActivation)
	require.NotNil(t, p.Active)
	require.Equal(t, "active", p.Active.Stage)
	require.Equal(t, strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD")), p.Active.Head)
	require.NotEmpty(t, p.Active.OldFiles)
	for name, digest := range p.Active.Files {
		require.Equal(t, digest, planning.DigestBytes(mustReadFile(t, filepath.Join(fx.taskDir, name))))
		info, err := os.Stat(filepath.Join(fx.taskDir, name))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0644), info.Mode().Perm(), "committed projection modes match Git worktree checkout")
	}
	spec, err := contract.LoadSpec(fx.taskDir)
	require.NoError(t, err)
	require.Equal(t, planning.ActivationRefs(*p.Active), *spec.PlanningRef)
	require.NoError(t, c.admitReviewedExecution())
	before := p.Active.Head
	require.NoError(t, c.activatePlanningPlans())
	require.Equal(t, before, c.State.Planning.Active.Head)
	require.NoError(t, c.resumePlanningActivation())
	stored, err := state.Load(fx.taskDir)
	require.NoError(t, err)
	require.NoError(t, planning.Validate(stored.Planning))
	require.NotContains(t, stored.Planning.Plans[0].Files["spec.md"], "planning-ref:", "sealing avoids digest self-reference")
}

func TestPlanningAtomicWritesReportDurabilityFaultsAndPreserveOwnership(t *testing.T) {
	for _, fault := range []string{"chmod", "write", "sync file", "close", "open directory", "sync directory"} {
		t.Run(fault, func(t *testing.T) {
			rootDir := t.TempDir()
			root, err := os.OpenRoot(rootDir)
			require.NoError(t, err)
			defer root.Close()
			writeFile(t, filepath.Join(rootDir, "target"), "previous bytes")
			chmod, write, sync, close, directory := chmodPlanningHandle, writePlanningHandle, syncPlanningHandle, closePlanningHandle, openPlanningDirectory
			t.Cleanup(func() {
				chmodPlanningHandle, writePlanningHandle, syncPlanningHandle, closePlanningHandle, openPlanningDirectory = chmod, write, sync, close, directory
			})
			failure := errors.New("simulated persistence fault")
			switch fault {
			case "chmod":
				chmodPlanningHandle = func(*os.File, os.FileMode) error { return failure }
			case "write":
				writePlanningHandle = func(*os.File, []byte) (int, error) { return 0, failure }
			case "sync file":
				syncPlanningHandle = func(*os.File) error { return failure }
			case "close":
				closePlanningHandle = func(f *os.File) error { require.NoError(t, f.Close()); return failure }
			case "open directory":
				openPlanningDirectory = func(*os.Root) (*os.File, error) { return nil, failure }
			case "sync directory":
				calls := 0
				syncPlanningHandle = func(f *os.File) error {
					calls++
					if calls == 2 {
						return failure
					}
					return sync(f)
				}
			}
			require.ErrorIs(t, writePlanningBytes(root, "target", []byte("next bytes")), failure)
			if fault == "open directory" || fault == "sync directory" {
				require.Equal(t, "next bytes", string(mustReadFile(t, filepath.Join(rootDir, "target"))), "rename may have happened; callers still receive the failed durability result")
			} else {
				require.Equal(t, "previous bytes", string(mustReadFile(t, filepath.Join(rootDir, "target"))))
			}
			entries, err := os.ReadDir(rootDir)
			require.NoError(t, err)
			require.Len(t, entries, 1, "failed writes do not leave temporary files as candidate artifacts")
		})
	}
}

func TestPlanningDirectoryFailureCannotLookLikeCompleteVerification(t *testing.T) {
	_, c := newReviewedFixture(t)
	rootDir := t.TempDir()
	mkdir, stat, open, list := mkdirPlanningDirectory, statPlanningPath, openPlanningTask, listPlanningDirectory
	t.Cleanup(func() {
		mkdirPlanningDirectory, statPlanningPath, openPlanningTask, listPlanningDirectory = mkdir, stat, open, list
	})
	mkdirPlanningDirectory = func(*os.Root, string, os.FileMode) error { return os.ErrPermission }
	_, err := planningRoot(rootDir, true)
	require.ErrorIs(t, err, os.ErrPermission)
	mkdirPlanningDirectory = mkdir
	statPlanningPath = func(*os.Root, string) (os.FileInfo, error) { return nil, os.ErrPermission }
	_, err = planningRoot(rootDir, true)
	require.ErrorIs(t, err, os.ErrPermission)
	statPlanningPath = stat
	openPlanningTask = func(string) (*os.Root, error) { return nil, os.ErrPermission }
	require.ErrorIs(t, c.checkPlanningExports(false), os.ErrPermission)
	openPlanningTask = open
	listPlanningDirectory = func(*os.Root) ([]os.DirEntry, error) { return nil, os.ErrPermission }
	require.ErrorIs(t, c.verifyPlanningArtifacts(), os.ErrPermission)
	listPlanningDirectory = list
	require.NoError(t, c.activatePlanningPlans())
	listPlanningDirectory = func(*os.Root) ([]os.DirEntry, error) { return nil, os.ErrPermission }
	require.ErrorIs(t, c.verifyPlanningProjection(c.State.Planning.Active), os.ErrPermission)
}

func TestReviewedPrewarmPreservesVersionFourAuthorityAndCleanup(t *testing.T) {
	fx, c := newReviewedFixture(t)
	require.NoError(t, c.activatePlanningPlans())
	enableSessionPolicy(c)
	c.Config.Scheduler.Prewarm, c.prewarmReview = true, true
	prepared := &preparedReviewStub{matches: true}
	stubReviewPrewarm(t, func(args drivers.DSHArgs) drivers.PreparedDSH {
		prepared.id = args.PrewarmID
		durable := loadFixtureState(t, fx)
		require.Equal(t, 4, durable.Sessions.Version)
		require.NotNil(t, durable.Planning)
		require.Equal(t, args.PrewarmID, durable.WorkerPrewarm.ID)
		require.Empty(t, args.Prompt)
		return prepared
	})
	require.NoError(t, c.beginReviewPrewarm(c.PacketList[0]))
	require.NotNil(t, c.State.WorkerPrewarm)
	require.NoError(t, c.closeReviewPrewarm("verification requires another attempt"))
	require.Equal(t, 1, prepared.closes)
	durable := loadFixtureState(t, fx)
	require.Nil(t, durable.WorkerPrewarm)
	require.Equal(t, 4, durable.Sessions.Version)
	require.NoError(t, planning.Validate(durable.Planning))
}

func TestPlanningExportsAreRecreatedOnlyFromUnchangedAuthority(t *testing.T) {
	fx, c := newReviewedFixture(t)
	name := filepath.Join(fx.taskDir, "planning", "design-000001.md")
	want := string(mustReadFile(t, name))
	require.NoError(t, os.Remove(name))
	require.Error(t, c.verifyPlanningArtifacts())
	require.NoError(t, c.checkPlanningExports(true))
	require.Equal(t, want, string(mustReadFile(t, name)))
	writeFile(t, name, "A different design")
	require.ErrorContains(t, c.checkPlanningExports(true), "immutable planning export changed")
	require.Equal(t, "A different design", string(mustReadFile(t, name)), "external edits are preserved for human reconciliation")
	writeFile(t, name, want)
	extra := filepath.Join(fx.taskDir, "planning", "unowned.json")
	writeFile(t, extra, "{}")
	require.ErrorContains(t, c.verifyPlanningArtifacts(), "unowned planning export")
	require.NoError(t, os.Remove(extra))
	writeFile(t, filepath.Join(fx.taskDir, "planning", ".planning-tmp-interrupted"), "uncommitted write")
	require.ErrorContains(t, c.verifyPlanningArtifacts(), "unowned planning export")
	require.NoError(t, c.checkPlanningExports(true))
	require.NoFileExists(t, filepath.Join(fx.taskDir, "planning", ".planning-tmp-interrupted"))

	oldWrite := writePlanningBytes
	t.Cleanup(func() { writePlanningBytes = oldWrite })
	require.NoError(t, os.Remove(name))
	writePlanningBytes = func(*os.Root, string, []byte) error { return os.ErrPermission }
	require.ErrorIs(t, c.checkPlanningExports(true), os.ErrPermission)
}

func TestPlanningTemporaryRecoveryIsConfinedAndReportsFilesystemFaults(t *testing.T) {
	fx, c := newReviewedFixture(t)
	name := filepath.Join(fx.taskDir, ".planning-tmp-interrupted")
	writeFile(t, name, "partial projection")
	stat, remove, list, open := statPlanningPath, removePlanningFile, listPlanningDirectory, openPlanningTask
	t.Cleanup(func() {
		statPlanningPath, removePlanningFile, listPlanningDirectory, openPlanningTask = stat, remove, list, open
	})
	openPlanningTask = func(string) (*os.Root, error) { return nil, os.ErrPermission }
	require.ErrorIs(t, c.checkPlanningExports(true), os.ErrPermission)
	openPlanningTask = open
	listPlanningDirectory = func(*os.Root) ([]os.DirEntry, error) { return nil, os.ErrPermission }
	require.ErrorIs(t, c.checkPlanningExports(true), os.ErrPermission)
	listPlanningDirectory = list
	statPlanningPath = func(root *os.Root, name string) (os.FileInfo, error) {
		if strings.HasPrefix(name, ".planning-tmp-") {
			return nil, os.ErrPermission
		}
		return stat(root, name)
	}
	require.ErrorIs(t, c.checkPlanningExports(true), os.ErrPermission)
	statPlanningPath = stat
	removePlanningFile = func(*os.Root, string) error { return os.ErrPermission }
	require.ErrorIs(t, c.checkPlanningExports(true), os.ErrPermission)
	require.FileExists(t, name, "failed recovery preserves the bytes for a later attempt")
	removePlanningFile = remove
	require.NoError(t, os.Remove(name))
	external := filepath.Join(t.TempDir(), "evidence")
	writeFile(t, external, "retain external evidence")
	require.NoError(t, os.Symlink(external, name))
	require.ErrorContains(t, c.checkPlanningExports(true), "must be a regular file")
	require.Equal(t, "retain external evidence", string(mustReadFile(t, external)))
	require.NoError(t, os.Remove(name))
	require.NoError(t, os.Mkdir(name, 0700))
	require.ErrorContains(t, c.checkPlanningExports(true), "must be a regular file")
	require.DirExists(t, name)
	require.NoError(t, os.Remove(name))
	require.NoError(t, c.checkPlanningExports(true))
}

func TestReviewedPlanningRecoversReservedAtomicFilesBeforeExecution(t *testing.T) {
	fx, opts, _ := briefEngineFixture(t, true)
	rootTemp := filepath.Join(fx.taskDir, ".planning-tmp-projection-interrupted")
	exportTemp := filepath.Join(fx.taskDir, "planning", ".planning-tmp-export-interrupted")
	writeFile(t, rootTemp, "partial projected contract")
	writeFile(t, exportTemp, "partial exported design")
	summary, err := RunTask(opts)
	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	require.NoFileExists(t, rootTemp)
	require.NoFileExists(t, exportTemp)
	require.FileExists(t, filepath.Join(fx.root, "build", "M01-T01.txt"))
	require.FileExists(t, filepath.Join(fx.root, "build", "M01-T02.txt"))
	st := loadFixtureState(t, fx)
	require.NoError(t, planning.Admit(st.Planning, planning.ActivationRefs(*st.Planning.Active)))
	require.NotContains(t, gitCmd(t, fx.root, "ls-files"), ".planning-tmp-")
}

func TestPlanningFilesystemRejectsSymlinksSpecialFilesAndOversizedSnapshots(t *testing.T) {
	rootDir := t.TempDir()
	root, err := planningRoot(rootDir, false)
	require.NoError(t, err)
	t.Cleanup(func() { root.Close() })
	_, err = readPlanningBytes(root, "missing")
	require.True(t, os.IsNotExist(err))
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "external"), filepath.Join(rootDir, "link")))
	_, err = readPlanningBytes(root, "link")
	require.ErrorContains(t, err, "invalid planning file")
	require.NoError(t, os.Mkdir(filepath.Join(rootDir, "directory"), 0700))
	_, err = readPlanningBytes(root, "directory")
	require.ErrorContains(t, err, "invalid planning file")
	writeFile(t, filepath.Join(rootDir, "large"), strings.Repeat("x", (2<<20)+1))
	_, err = readPlanningBytes(root, "large")
	require.ErrorContains(t, err, "invalid planning file")
	require.NoError(t, writePlanningBytes(root, "normal", []byte("immutable snapshot")))
	raw, err := readPlanningBytes(root, "normal")
	require.NoError(t, err)
	require.Equal(t, "immutable snapshot", string(raw))
	require.Error(t, writePlanningBytes(root, "directory", []byte("cannot replace directory")))
	require.NoError(t, root.Close())
	_, err = planningEntries(root)
	require.Error(t, err)
	require.Error(t, writePlanningBytes(root, "closed", nil))
	_, err = planningRoot(filepath.Join(rootDir, "absent"), false)
	require.Error(t, err)
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(rootDir, "planning")))
	_, err = planningRoot(rootDir, true)
	require.ErrorContains(t, err, "real directory")
}

func TestChildPlanningValidationPreservesRootScopeAndAcceptanceFloor(t *testing.T) {
	_, c := newReviewedFixture(t)
	base := c.State.Planning.Plans[0].Files
	request := planning.Request{Scope: "milestone", ScopeID: "M-01"}
	for name, edit := range map[string]func(map[string]string){
		"missing spec": func(files map[string]string) { delete(files, "spec.md") },
		"changed goal": func(files map[string]string) {
			files["spec.md"] = strings.Replace(files["spec.md"], "Prove the orchestrator", "Replace the orchestrator", 1)
		},
		"changed requirement": func(files map[string]string) {
			files["spec.md"] = strings.Replace(files["spec.md"], "First thing works", "Skip the first thing", 1)
		},
		"changed milestone": func(files map[string]string) {
			files["spec.md"] = strings.Replace(files["spec.md"], "L1 green for all packets.", "Always pass", 1)
		},
		"unowned path":     func(files map[string]string) { files["source.go"] = "package changed" },
		"malformed packet": func(files map[string]string) { files["task-packet-M01-T01.md"] = "not a packet" },
		"missing scope":    func(files map[string]string) { files["task-packet-M01-T01.md"] = fixturePacket("M01-T01", "R-01") },
		"out of scope": func(files map[string]string) {
			files["task-packet-M01-T01.md"] = strings.Replace(files["task-packet-M01-T01.md"], "writes: [build/M01-T01.txt]", "writes: [private/]", 1)
		},
		"placeholder acceptance": func(files map[string]string) {
			files["task-packet-M01-T01.md"] = strings.Replace(files["task-packet-M01-T01.md"], "test -f build/M01-T01.txt", "true", 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			files := jsonClone(base)
			edit(files)
			require.Error(t, c.validatePlanningFiles(files, request))
		})
	}
	require.ErrorContains(t, c.validatePlanningFiles(base, planning.Request{Scope: "packet", ScopeID: "M99-T01"}), "no executable packet")
	ref := c.State.Planning.CurrentDesign
	c.State.Planning.CurrentDesign = nil
	require.ErrorContains(t, c.validatePlanningFiles(base, request), "root design")
	c.State.Planning.CurrentDesign = ref
	require.NoError(t, c.activatePlanningPlans())
	files := jsonClone(base)
	files["task-packet-M01-T02.md"] += "\nRewrite unrelated work\n"
	require.ErrorContains(t, c.validatePlanningFiles(files, planning.Request{Scope: "packet", ScopeID: "M01-T01"}), "unaffected/completed")
	c.State.Packets["M01-T02"].Status = "completed"
	require.ErrorContains(t, c.validatePlanningFiles(files, request), "unaffected/completed")
	require.NoError(t, c.validatePlanningFiles(base, request))
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing-parent"))
	require.Error(t, c.validatePlanningFiles(base, request))
}

func TestChildProjectionCoversEveryReadyRequirementInItsDeclaredMilestone(t *testing.T) {
	_, c := newReviewedFixture(t)
	files := jsonClone(c.State.Planning.Plans[0].Files)
	request := planning.Request{Scope: "milestone", ScopeID: "M-01"}
	delete(files, "task-packet-M01-T02.md")
	require.ErrorContains(t, c.validatePlanningFiles(files, request), "requirement R-02 has no packet")
	files = jsonClone(c.State.Planning.Plans[0].Files)
	design := c.State.Planning.Designs[0]
	design.Ref.Revision++
	design.Requirements = append(design.Requirements, planning.Requirement{ID: "R-03", Title: "Later output", Milestone: "M-02", Verification: "test -f build/M02-T01.txt"})
	design.Milestones = append(design.Milestones, planning.Milestone{ID: "M-02", Title: "Later milestone", Gate: "Later output verified"})
	var err error
	design, err = planning.SealDesign(design)
	require.NoError(t, err)
	require.NoError(t, planning.RecordDesign(c.State.Planning, design))
	files["spec.md"] = strings.Replace(fixtureSpec, "requirements:\n", "  - id: M-02\n    title: Later milestone\n    gate: Later output verified\n    planning: deferred\nrequirements:\n", 1)
	files["spec.md"] = strings.Replace(files["spec.md"], "---\n\n# Mini spec", "  - id: R-03\n    title: Later output\n    milestone: M-02\n    verification: test -f build/M02-T01.txt\n---\n\n# Mini spec", 1)
	require.NoError(t, c.validatePlanningFiles(files, request), "explicitly deferred future requirements need no fabricated packet")
	files["spec.md"] = strings.Replace(files["spec.md"], "    planning: deferred\n", "", 1)
	require.ErrorContains(t, c.validatePlanningFiles(files, request), "milestone M-02 has no packet")
	files["task-packet-M02-T01.md"] = strings.Replace(fixturePacket("M02-T01", "R-01"), "route: default", "route: default\nschedule: {after: [], writes: [build/M02-T01.txt]}", 1)
	require.ErrorContains(t, c.validatePlanningFiles(files, request), "different milestone")
	files["task-packet-M02-T01.md"] = strings.Replace(files["task-packet-M02-T01.md"], "requirements: [R-01]", "requirements: [R-03]", 1)
	require.NoError(t, c.validatePlanningFiles(files, request))
	files["spec.md"] = strings.Replace(files["spec.md"], "    gate: Later output verified\n", "    gate: Later output verified\n    planning: deferred\n", 1)
	require.ErrorContains(t, c.validatePlanningFiles(files, request), "deferred", "a deferred milestone cannot conceal existing executable packets")
}

func TestActivationStopsBeforeProjectionWhenAuthorityOrDependenciesChange(t *testing.T) {
	for name, change := range map[string]func(*testing.T, fixture, *Context){
		"no root":         func(_ *testing.T, _ fixture, c *Context) { c.State.Planning.CurrentDesign = nil },
		"no approval":     func(_ *testing.T, _ fixture, c *Context) { c.State.Planning.Approvals = []planning.Approval{} },
		"no child":        func(_ *testing.T, _ fixture, c *Context) { c.State.Planning.CurrentPlans = map[string]planning.Ref{} },
		"no child review": func(_ *testing.T, _ fixture, c *Context) { c.State.Planning.Reviews = c.State.Planning.Reviews[:1] },
		"changed source": func(t *testing.T, fx fixture, _ *Context) {
			writeFile(t, filepath.Join(fx.root, "external.go"), "package external\n")
		},
		"non Git": func(t *testing.T, fx fixture, _ *Context) {
			require.NoError(t, os.RemoveAll(filepath.Join(fx.root, ".git")))
		},
		"CAS conflict": func(t *testing.T, fx fixture, _ *Context) {
			s := loadFixtureState(t, fx)
			_, err := state.Save(fx.taskDir, s, nil)
			require.NoError(t, err)
		},
	} {
		t.Run(name, func(t *testing.T) {
			fx, c := newReviewedFixture(t)
			before := string(mustReadFile(t, filepath.Join(fx.taskDir, "spec.md")))
			change(t, fx, c)
			require.Error(t, c.activatePlanningPlans())
			require.Equal(t, before, string(mustReadFile(t, filepath.Join(fx.taskDir, "spec.md"))))
			require.Nil(t, loadFixtureState(t, fx).Planning.Active)
		})
	}
	_, c := newReviewedFixture(t)
	old := snapshotGitFiles
	t.Cleanup(func() { snapshotGitFiles = old })
	snapshotGitFiles = func(string) ([]byte, error) { return nil, errors.New("inventory unavailable") }
	require.ErrorContains(t, c.activatePlanningPlans(), "inventory unavailable")
}

func restartReviewedFixture(t *testing.T, fx fixture, c *Context) *Context {
	t.Helper()
	restarted, err := CreateContext(Opts{TaskDir: fx.taskDir, Cwd: fx.root, Config: c.Config, Drivers: c.Drivers, Log: c.Log, Now: c.Now, Sleep: c.Sleep})
	require.NoError(t, err)
	require.Nil(t, restarted.Spec, "loading state precedes any partial contract projection")
	return restarted
}

func addReviewedPlan(t *testing.T, c *Context, files map[string]string, scope, id string) planning.ExecutionPlan {
	t.Helper()
	p := c.State.Planning
	dependency, err := c.planScopeIdentity()
	require.NoError(t, err)
	revision := len(p.Plans) + 1
	plan, err := planning.SealPlan(planning.ExecutionPlan{Ref: planning.Ref{Revision: revision}, Design: *p.CurrentDesign,
		InputRevision: p.InputRevision, Scope: scope, ScopeID: id, Files: files, Dependencies: dependency,
		ScopeVerdict: "within-envelope", PlannerInvocation: fmt.Sprintf("child-proposal-%d", revision)})
	require.NoError(t, err)
	require.NoError(t, planning.RecordPlan(p, plan))
	require.NoError(t, planning.RecordReview(p, planning.Review{ID: fmt.Sprintf("child-review-%d", revision), Kind: "plan", Target: plan.Ref, Design: p.CurrentDesign,
		InputRevision: p.InputRevision, Invocation: fmt.Sprintf("child-review-call-%d", revision), Role: "plan-review", Binding: "independent-reviewer", Verdict: "pass", ScopeVerdict: "within-envelope", Summary: "The proposed projection remains in the root scope", Findings: []planning.Finding{}}))
	c.save()
	require.NoError(t, c.err())
	require.NoError(t, c.checkPlanningExports(true))
	return plan
}

func pendingReviewedFixture(t *testing.T) (fixture, *Context) {
	t.Helper()
	fx, c := newReviewedFixture(t)
	old := writePlanningBytes
	writePlanningBytes = func(*os.Root, string, []byte) error { return os.ErrPermission }
	err := c.activatePlanningPlans()
	writePlanningBytes = old
	require.ErrorIs(t, err, os.ErrPermission)
	require.NotNil(t, c.State.Planning.PendingActivation)
	return fx, c
}

func TestActivationRetiresOnlyOwnedUnchangedPacketsAndResumesRemoval(t *testing.T) {
	for _, scenario := range []string{"owned", "already absent", "changed", "special", "remove failure", "crash after remove"} {
		t.Run(scenario, func(t *testing.T) {
			fx, c := newReviewedFixture(t)
			require.NoError(t, c.activatePlanningPlans())
			files := jsonClone(c.State.Planning.Plans[0].Files)
			delete(files, "task-packet-M01-T02.md")
			files["task-packet-M01-T01.md"] = strings.Replace(files["task-packet-M01-T01.md"], "requirements: [R-01]", "requirements: [R-01, R-02]", 1)
			require.NoError(t, c.validatePlanningFiles(files, planning.Request{Scope: "milestone", ScopeID: "M-01"}))
			addReviewedPlan(t, c, files, "milestone", "M-01")
			retired := filepath.Join(fx.taskDir, "task-packet-M01-T02.md")
			remove := removePlanningFile
			t.Cleanup(func() { removePlanningFile = remove })
			switch scenario {
			case "already absent":
				require.NoError(t, os.Remove(retired))
			case "changed":
				writeFile(t, retired, "external packet revision")
			case "special":
				require.NoError(t, os.Remove(retired))
				require.NoError(t, os.Mkdir(retired, 0700))
			case "remove failure":
				removePlanningFile = func(*os.Root, string) error { return os.ErrPermission }
			case "crash after remove":
				removePlanningFile = func(root *os.Root, name string) error {
					require.NoError(t, remove(root, name))
					return errors.New("interrupted after retiring packet")
				}
			}
			err := c.activatePlanningPlans()
			if scenario == "owned" || scenario == "already absent" {
				require.NoError(t, err)
				require.NoFileExists(t, retired)
				require.NotContains(t, c.State.Planning.Active.Files, "task-packet-M01-T02.md")
				require.NoError(t, c.admitReviewedExecution())
			} else {
				require.Error(t, err)
				durable := loadFixtureState(t, fx)
				require.NotNil(t, durable.Planning.PendingActivation)
				require.Contains(t, durable.Planning.Active.Files, "task-packet-M01-T02.md", "failed transactions preserve the previous active identity")
				if scenario == "changed" {
					require.Equal(t, "external packet revision", string(mustReadFile(t, retired)))
				}
				if scenario == "crash after remove" {
					removePlanningFile = remove
					restarted := restartReviewedFixture(t, fx, c)
					require.NoError(t, restarted.resumePlanningActivation())
					require.NoFileExists(t, retired)
					require.NoError(t, restarted.admitReviewedExecution())
				}
			}
		})
	}
}

func TestActivationCombinesCurrentChildScopesWithoutLosingPriorRefs(t *testing.T) {
	_, c := newReviewedFixture(t)
	first := c.State.Planning.Plans[0]
	files := jsonClone(first.Files)
	files["task-packet-M01-T01.md"] += "\nKeep the existing artifact format.\n"
	second := addReviewedPlan(t, c, files, "packet", "M01-T01")
	require.NoError(t, c.activatePlanningPlans())
	require.Equal(t, []planning.Ref{first.Ref, second.Ref}, c.State.Planning.Active.Plans)
	require.NoError(t, c.admitReviewedExecution())
}

func TestPendingActivationRejectsDependencyReadAndFilesystemRaces(t *testing.T) {
	for _, fault := range []string{"dependency inventory", "dependency changed", "projection root", "special projection", "extra packet", "commit hook"} {
		t.Run(fault, func(t *testing.T) {
			fx, c := pendingReviewedFixture(t)
			inventory, open := snapshotGitFiles, openPlanningTask
			t.Cleanup(func() { snapshotGitFiles, openPlanningTask = inventory, open })
			switch fault {
			case "dependency inventory":
				snapshotGitFiles = func(string) ([]byte, error) { return nil, os.ErrPermission }
			case "dependency changed":
				writeFile(t, filepath.Join(fx.root, "unexpected-source.go"), "package unexpected\n")
			case "projection root":
				calls := 0
				openPlanningTask = func(path string) (*os.Root, error) {
					calls++
					if calls == 4 { // after two temporary-recovery roots and export verification
						return nil, os.ErrPermission
					}
					return open(path)
				}
			case "special projection":
				name := filepath.Join(fx.taskDir, "task-packet-M01-T01.md")
				require.NoError(t, os.Remove(name))
				require.NoError(t, os.Mkdir(name, 0700))
			case "extra packet":
				writeFile(t, filepath.Join(fx.taskDir, "task-packet-M99-T01.md"), fixturePacket("M99-T01", "R-01"))
			case "commit hook":
				installCommitHook(t, fx, "exit 1")
			}
			require.Error(t, c.resumePlanningActivation())
			require.Nil(t, loadFixtureState(t, fx).Planning.Active)
		})
	}
}

func TestActivationRechecksExportsAndProjectionAfterGitCheckpoint(t *testing.T) {
	for _, target := range []string{"planning/design-000001.md", "task-packet-M01-T01.md", "Git metadata"} {
		t.Run(target, func(t *testing.T) {
			fx, c := newReviewedFixture(t)
			old := inspectPlanDirtyFiles
			t.Cleanup(func() { inspectPlanDirtyFiles = old })
			inspectPlanDirtyFiles = func(cwd, head string) ([]string, bool) {
				files, ok := old(cwd, head)
				if target == "Git metadata" {
					require.NoError(t, os.Rename(filepath.Join(fx.root, ".git"), filepath.Join(fx.root, "detached-git")))
				} else {
					writeFile(t, filepath.Join(fx.taskDir, target), "late concurrent modification")
				}
				return files, ok
			}
			require.Error(t, c.activatePlanningPlans())
			durable := loadFixtureState(t, fx)
			require.Nil(t, durable.Planning.Active)
			require.NotNil(t, durable.Planning.PendingActivation)
		})
	}
}

func TestInvalidStoredPlanCannotProjectOrActivateEvenWithTypedReview(t *testing.T) {
	for _, body := range []string{"not frontmatter", "---\nunknown: true\n---\ninvalid spec\n"} {
		t.Run(body, func(t *testing.T) {
			_, c := newReviewedFixture(t)
			files := jsonClone(c.State.Planning.Plans[0].Files)
			files["spec.md"] = body
			plan := addReviewedPlan(t, c, files, "milestone", "M-01")
			if body == "not frontmatter" {
				// The pure persistence type stores raw documents. The activation
				// boundary must still reject malformed source snapshots after restart.
				require.Error(t, c.activatePlanningPlans())
				c.State.Planning.PendingActivation = &planning.Activation{Design: plan.Design, Plans: []planning.Ref{plan.Ref},
					Files: map[string]string{"spec.md": planning.DigestBytes([]byte(body))}, OldFiles: c.State.Planning.LegacyFiles, Stage: "intent", Base: "base"}
				c.State.Planning.Phase = "activating"
				require.NoError(t, planning.Validate(c.State.Planning))
				require.Error(t, c.resumePlanningActivation())
			} else {
				require.Error(t, c.activatePlanningPlans(), "schema-invalid projected contracts cannot become active")
			}
			require.Nil(t, c.State.Planning.Active)
		})
	}
}

func TestActivationResumesPartialProjectionWithoutRewritingHistory(t *testing.T) {
	fx, c := newReviewedFixture(t)
	old := writePlanningBytes
	t.Cleanup(func() { writePlanningBytes = old })
	written := ""
	writePlanningBytes = func(root *os.Root, name string, raw []byte) error {
		if err := old(root, name, raw); err != nil {
			return err
		}
		written = name
		return errors.New("interrupted after atomic projection write")
	}
	require.ErrorContains(t, c.activatePlanningPlans(), "interrupted after atomic projection write")
	require.NotEmpty(t, written)
	durable := loadFixtureState(t, fx)
	require.Nil(t, durable.Planning.Active)
	require.NotNil(t, durable.Planning.PendingActivation)
	require.Equal(t, "activating", durable.Planning.Phase)
	require.Equal(t, durable.Planning.PendingActivation.Files[written], planning.DigestBytes(mustReadFile(t, filepath.Join(fx.taskDir, written))))
	require.Len(t, durable.Planning.Plans, 1)
	writePlanningBytes = old
	restarted := restartReviewedFixture(t, fx, c)
	require.NoError(t, restarted.activatePlanningPlans())
	require.NoError(t, restarted.admitReviewedExecution())
	require.Len(t, restarted.State.Planning.Designs, 1)
	require.Len(t, restarted.State.Planning.Plans, 1)
	require.Len(t, restarted.State.Planning.Reviews, 2)
	require.Nil(t, restarted.State.Planning.PendingActivation)
}

func TestActivationRecoveryAfterCommitBeforeStatePublicationKeepsOneCheckpoint(t *testing.T) {
	fx, c := newReviewedFixture(t)
	old := inspectPlanDirtyFiles
	t.Cleanup(func() { inspectPlanDirtyFiles = old })
	bumped := false
	inspectPlanDirtyFiles = func(cwd, head string) ([]string, bool) {
		if !bumped {
			bumped = true
			durable := loadFixtureState(t, fx)
			require.NotNil(t, durable.Planning.PendingActivation)
			require.Nil(t, durable.Planning.Active)
			_, err := state.Save(fx.taskDir, durable, nil)
			require.NoError(t, err)
		}
		return old(cwd, head)
	}
	err := c.activatePlanningPlans()
	var conflict *state.CasConflictError
	require.ErrorAs(t, err, &conflict)
	require.True(t, bumped)
	head := strings.TrimSpace(gitCmd(t, fx.root, "rev-parse", "HEAD"))
	durable := loadFixtureState(t, fx)
	require.Nil(t, durable.Planning.Active)
	require.NotNil(t, durable.Planning.PendingActivation)
	inspectPlanDirtyFiles = old
	restarted := restartReviewedFixture(t, fx, c)
	require.NoError(t, restarted.resumePlanningActivation())
	require.Equal(t, head, restarted.State.Planning.Active.Head, "recovery reuses the committed exact projection")
	require.NoError(t, restarted.admitReviewedExecution())
}

func TestPendingActivationRefusesExternalContractEditsAndExportTampering(t *testing.T) {
	for _, target := range []string{"spec.md", "planning/design-000001.md", "task-packet-M01-T01.md"} {
		t.Run(target, func(t *testing.T) {
			fx, c := newReviewedFixture(t)
			old := writePlanningBytes
			t.Cleanup(func() { writePlanningBytes = old })
			writePlanningBytes = func(*os.Root, string, []byte) error { return errors.New("projection interrupted") }
			require.Error(t, c.activatePlanningPlans())
			writePlanningBytes = old
			name := filepath.Join(fx.taskDir, target)
			writeFile(t, name, "external revision to preserve")
			restarted := restartReviewedFixture(t, fx, c)
			require.Error(t, restarted.resumePlanningActivation())
			require.Equal(t, "external revision to preserve", string(mustReadFile(t, name)))
			require.Nil(t, loadFixtureState(t, fx).Planning.Active)
		})
	}
}

func TestProjectionVerificationRejectsMissingChangedAndUnownedFiles(t *testing.T) {
	fx, c := newReviewedFixture(t)
	require.ErrorContains(t, c.verifyPlanningProjection(nil), "not active")
	require.NoError(t, c.activatePlanningPlans())
	a := c.State.Planning.Active
	oldTaskDir := c.TaskDir
	c.TaskDir = filepath.Join(fx.root, "missing")
	require.Error(t, c.verifyPlanningProjection(a))
	c.TaskDir = oldTaskDir
	path := filepath.Join(fx.taskDir, "spec.md")
	original := mustReadFile(t, path)
	require.NoError(t, os.Remove(path))
	require.Error(t, c.verifyPlanningProjection(a))
	writeFile(t, path, "changed projection")
	require.ErrorContains(t, c.verifyPlanningProjection(a), "projected contract changed")
	writeFile(t, path, string(original))
	writeFile(t, filepath.Join(fx.taskDir, "task-packet-M99-T01.md"), fixturePacket("M99-T01", "R-01"))
	require.ErrorContains(t, c.verifyPlanningProjection(a), "unowned packet")
	require.Error(t, c.activatePlanningPlans(), "matching refs cannot hide changed projections")
}

func TestPlanProjectionUsesNewestCompleteVersionAndRejectsMissingSpec(t *testing.T) {
	_, c := newReviewedFixture(t)
	p := c.State.Planning
	first := p.Plans[0]
	second := first
	second.Ref.Revision++
	second.Files = jsonClone(first.Files)
	delete(second.Files, "task-packet-M01-T02.md")
	var err error
	second, err = planning.SealPlan(second)
	require.NoError(t, err)
	require.NoError(t, planning.RecordPlan(p, second))
	files := c.planFiles([]planning.Ref{second.Ref, first.Ref})
	require.NotContains(t, files, "task-packet-M01-T02.md", "older snapshots must not resurrect retired packets")
	files["spec.md"] = "caller mutation"
	require.Equal(t, fixtureSpec, p.Plans[1].Files["spec.md"])
	require.Empty(t, c.planFiles([]planning.Ref{{Revision: 99, Digest: strings.Repeat("e", 64)}}))
	_, err = c.projectedPlanningFiles(&planning.Activation{Design: first.Design, Plans: []planning.Ref{}})
	require.Error(t, err)
	files = jsonClone(first.Files)
	files["task-packet-"+strings.Repeat("x", 300)+".md"] = "too long for a filesystem entry"
	require.Error(t, c.validatePlanningFiles(files, planning.Request{Scope: "milestone", ScopeID: "M-01"}))
}
