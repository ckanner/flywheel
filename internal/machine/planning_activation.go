package machine

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"gopkg.in/yaml.v3"
)

const maxOracleBytes = 64 * 1024

func isOwnedOraclePath(name string) bool {
	if !strings.HasPrefix(name, "oracles/verify-") {
		return false
	}
	base := strings.TrimPrefix(name, "oracles/")
	if base == "" || strings.ContainsAny(base, `/\`) || strings.Contains(base, "..") {
		return false
	}
	return strings.HasSuffix(base, ".mjs") || strings.HasSuffix(base, ".sh")
}

var lookPathNode = exec.LookPath
var checkOracleSyntax = defaultCheckOracleSyntax

func defaultCheckOracleSyntax(name, absPath string) error {
	var cmd *exec.Cmd
	if strings.HasSuffix(name, ".sh") {
		cmd = exec.Command("sh", "-n", absPath)
	} else {
		node, err := lookPathNode("node")
		if err != nil {
			return nil
		}
		cmd = exec.Command(node, "--check", absPath)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("oracle %s failed syntax check: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}

func renderProjectContext(st *state.State, prepareCmd string) []byte {
	var lines []string
	lines = append(lines, "# Project Context Memory (Stable Authority)")
	milestoneTitles := map[string]string{}
	if st != nil && st.Planning != nil {
		p := st.Planning
		lines = append(lines, "", "Goal: "+p.Brief.Goal)
		if len(p.Brief.Constraints) > 0 {
			lines = append(lines, "Constraints: "+strings.Join(p.Brief.Constraints, "; "))
		}
		if len(p.Brief.QualityGates) > 0 {
			lines = append(lines, "Quality gates: "+strings.Join(p.Brief.QualityGates, "; "))
		}
		if p.Clarification != nil && len(p.Clarification.Answers) > 0 {
			keys := make([]string, 0, len(p.Clarification.Answers))
			for k := range p.Clarification.Answers {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			lines = append(lines, "", "## Confirmed Clarifications")
			for _, k := range keys {
				lines = append(lines, fmt.Sprintf("- %s: %s", k, p.Clarification.Answers[k]))
			}
		}
		if p.CurrentDesign != nil {
			if d := planning.FindDesign(p, *p.CurrentDesign); d != nil {
				if len(d.ImplementationScope) > 0 {
					lines = append(lines, "", "## Implementation Scope", strings.Join(d.ImplementationScope, ", "))
				}
				if len(d.Decisions) > 0 {
					lines = append(lines, "", "## Architecture Decisions")
					for _, dec := range d.Decisions {
						lines = append(lines, fmt.Sprintf("- [%s/%s] %s", dec.ID, dec.Kind, dec.Statement))
					}
				}
				if len(d.Milestones) > 0 {
					lines = append(lines, "", "## Milestones")
					for _, m := range d.Milestones {
						milestoneTitles[m.ID] = m.Title
						lines = append(lines, fmt.Sprintf("- %s %s (gate: %s)", m.ID, m.Title, m.Gate))
					}
				}
			}
		}
	}
	if st != nil {
		if delivered := deliveredMilestoneLines(st, milestoneTitles); len(delivered) > 0 {
			lines = append(lines, "", "## Delivered Milestones")
			lines = append(lines, delivered...)
		}
		if facts := verifiedToolchainFactLines(st, prepareCmd); len(facts) > 0 {
			lines = append(lines, "", "## Verified Workspace & Toolchain Facts")
			lines = append(lines, facts...)
		}
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func deliveredMilestoneLines(st *state.State, titles map[string]string) []string {
	ids := make([]string, 0, len(st.Milestones))
	for id, ms := range st.Milestones {
		if ms.Status == "closed" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		ms := st.Milestones[id]
		var packets []string
		for pid, ps := range st.Packets {
			if ps == nil || ps.Status != "completed" {
				continue
			}
			if m, err := contract.MilestoneOfPacket(pid); err == nil && m == id {
				packets = append(packets, pid)
			}
		}
		sort.Strings(packets)
		title := titles[id]
		if title == "" {
			title = id
		}
		outcome := milestoneDeliveryOutcome(ms)
		lines = append(lines, fmt.Sprintf("- %s (%s): packets=[%s], outcome=%s", id, title, strings.Join(packets, ", "), outcome))
	}
	return lines
}

func milestoneDeliveryOutcome(ms state.MilestoneState) string {
	if ms.HumanSignoff != nil {
		return fmt.Sprintf("gate pass, human-accepted (%s)", ms.HumanSignoff.Reason)
	}
	if gmap, ok := ms.Gate.(map[string]any); ok {
		if jmap, ok := gmap["judge"].(map[string]any); ok && jmap["score"] != nil {
			return fmt.Sprintf("gate pass, judge %v", jmap["score"])
		}
	}
	return "closed"
}

func verifiedToolchainFactLines(st *state.State, prepareCmd string) []string {
	seen := map[string]bool{}
	var cmds []string
	addCmd := func(raw string) {
		c := strings.TrimSpace(raw)
		if c != "" && !seen[c] {
			seen[c] = true
			cmds = append(cmds, c)
		}
	}
	for _, ps := range st.Packets {
		if ps == nil || ps.Status != "completed" {
			continue
		}
		if l1, ok := ps.Evidence.L1.(map[string]any); ok && l1["pass"] == true {
			if results, ok := l1["commands"].([]any); ok {
				for _, item := range results {
					if m, ok := item.(map[string]any); ok && m["pass"] == true {
						if cmd, ok := m["cmd"].(string); ok {
							addCmd(cmd)
						}
					}
				}
			}
		}
	}
	sort.Strings(cmds)
	var lines []string
	if strings.TrimSpace(prepareCmd) != "" {
		lines = append(lines, "- verify.prepare-cmd: "+strings.TrimSpace(prepareCmd))
	}
	if len(cmds) > 0 {
		if len(cmds) > 12 {
			cmds = cmds[:12]
		}
		lines = append(lines, "- Verified passing gate commands: "+strings.Join(cmds, " | "))
	}
	return lines
}

func (c *Context) verifyPrepareCmd() string {
	if c.Config == nil {
		return ""
	}
	return strings.TrimSpace(c.Config.Verify.PrepareCmd)
}

func (c *Context) syncProjectContext() error {
	taskRoot, err := openPlanningTask(c.TaskDir)
	if err != nil {
		return err
	}
	defer taskRoot.Close()
	raw := renderProjectContext(c.State, c.verifyPrepareCmd())
	actual, readErr := readPlanningBytes(taskRoot, "project-context.md")
	if readErr != nil || !bytes.Equal(raw, actual) {
		return writePlanningBytes(taskRoot, "project-context.md", raw)
	}
	return nil
}

// Exports are reviewable copies, never an alternative source of authority.
// Saving the state/consumption transaction precedes their creation, so a crash
// can regenerate a missing export without replaying a model invocation.
func planningExports(p *planning.State) map[string][]byte {
	out := map[string][]byte{}
	add := func(name string, value any) {
		raw, _ := json.MarshalIndent(value, "", "  ")
		out[name] = append(raw, '\n')
	}
	add("brief.json", p.Brief)
	for _, d := range p.Designs {
		add(fmt.Sprintf("design-%06d.json", d.Ref.Revision), d)
		out[fmt.Sprintf("design-%06d.md", d.Ref.Revision)] = []byte(d.Document)
	}
	for _, p := range p.Plans {
		add(fmt.Sprintf("plan-%06d.json", p.Ref.Revision), p)
	}
	for i, r := range p.Reviews {
		add(fmt.Sprintf("review-%06d.json", i+1), r)
	}
	return out
}

// Narrow filesystem seams exercise disk-full, durability and concurrent path
// failures without weakening the same production OpenRoot boundary.
var writePlanningHandle = (*os.File).Write
var syncPlanningHandle = (*os.File).Sync
var closePlanningHandle = (*os.File).Close
var chmodPlanningHandle = (*os.File).Chmod
var mkdirPlanningDirectory = (*os.Root).Mkdir
var statPlanningPath = (*os.Root).Lstat
var openPlanningTask = os.OpenRoot
var removePlanningFile = (*os.Root).Remove
var listPlanningDirectory = planningEntries
var openPlanningDirectory = func(root *os.Root) (*os.File, error) { return root.Open(".") }

var writePlanningBytes = func(root *os.Root, name string, raw []byte) error {
	tmp := ".planning-tmp-" + rand.Text()
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	// Git recreates ordinary contract files as 0644 in execution worktrees.
	// Normalize explicitly despite a restrictive umask so the same committed
	// bytes have the same candidate identity before and after integration.
	if err = chmodPlanningHandle(f, 0644); err != nil {
		f.Close()
		return err
	}
	if _, err = writePlanningHandle(f, raw); err != nil {
		f.Close()
		return err
	}
	if err = syncPlanningHandle(f); err != nil {
		f.Close()
		return err
	}
	if err = closePlanningHandle(f); err != nil {
		return err
	}
	if err := root.Rename(tmp, name); err != nil {
		return err
	}
	dir, err := openPlanningDirectory(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	return syncPlanningHandle(dir)
}

func planningEntries(root *os.Root) ([]os.DirEntry, error) {
	dir, err := openPlanningDirectory(root)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return dir.ReadDir(-1)
}

func planningRoot(taskDir string, exports bool) (*os.Root, error) {
	root, err := openPlanningTask(taskDir)
	if err != nil {
		return nil, err
	}
	if !exports {
		return root, nil
	}
	defer root.Close()
	if err := mkdirPlanningDirectory(root, "planning", 0700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	info, err := statPlanningPath(root, "planning")
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("planning exports must use a real directory")
	}
	return root.OpenRoot("planning")
}

// Reject links and special files before a bounded read. OpenRoot confines any
// concurrent path replacement to the recorded task directory.
func readPlanningBytes(root *os.Root, name string) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 2<<20 {
		return nil, fmt.Errorf("invalid planning file %s", name)
	}
	return root.ReadFile(name)
}

// Only the leased orchestrator calls this recovery boundary. Atomic publication
// may leave ordinary files under the reserved prefix after a process crash;
// their bytes are never authoritative and state regenerates the intended file.
// Other file kinds are rejected instead of followed or recursively removed.
func cleanPlanningTemps(root *os.Root) error {
	entries, err := listPlanningDirectory(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".planning-tmp-") {
			continue
		}
		info, err := statPlanningPath(root, entry.Name())
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("reserved planning temporary path must be a regular file: %s", entry.Name())
		}
		if err := removePlanningFile(root, entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

func (c *Context) recoverPlanningTemps() error {
	for _, exports := range []bool{false, true} {
		root, err := planningRoot(c.TaskDir, exports)
		if err != nil {
			return err
		}
		err = cleanPlanningTemps(root)
		root.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Context) checkPlanningExports(create bool) error {
	if create {
		if err := c.recoverPlanningTemps(); err != nil {
			return err
		}
	}
	root, err := planningRoot(c.TaskDir, create)
	if err != nil {
		return err
	}
	if !create {
		parent := root
		root, err = parent.OpenRoot("planning")
		parent.Close()
		if err != nil {
			return err
		}
	}
	defer root.Close()
	expected := planningExports(c.State.Planning)
	for name, raw := range expected {
		actual, err := readPlanningBytes(root, name)
		if os.IsNotExist(err) && create {
			if err := writePlanningBytes(root, name, raw); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !bytes.Equal(raw, actual) {
			return fmt.Errorf("immutable planning export changed: %s", name)
		}
	}
	entries, err := listPlanningDirectory(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if expected[entry.Name()] == nil {
			return fmt.Errorf("unowned planning export: %s", entry.Name())
		}
	}
	if create && (c.State.Planning.CurrentDesign != nil || c.State.Planning.Clarification != nil) {
		if err := c.syncProjectContext(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Context) verifyPlanningArtifacts() error { return c.checkPlanningExports(false) }

// Investigation evidence does not widen its original implementation grant.
// Report-only work may propose a scope for review, but cannot project executable
// contracts until a human starts this exact design against the same report.
func (c *Context) checkPlanningRepairScope(design *planning.Design, activating bool) error {
	preparation, err := planningPreparation(c.TaskDir, c.Cwd)
	if err != nil {
		return err
	}
	if preparation == nil {
		if c.State.PreparationSource != nil {
			return fmt.Errorf("the recorded investigation report is no longer available")
		}
		return nil
	}
	if c.State.PreparationSource == nil {
		return fmt.Errorf("investigation evidence must be imported before planning its implementation")
	}
	if err := c.importPreparation(preparation); err != nil {
		return err // an existing source is checked without changing usage or state
	}
	if preparation.Contract.Intent == "investigate" {
		if !activating {
			return nil
		}
		if !planning.StartAllowed(c.State.Planning, design.Ref) || !c.planningRepairApproved(preparation) {
			return fmt.Errorf("report-only activation requires exact human Start and its supported report grant")
		}
	}
	if !planningScopeWithin(design.ImplementationScope, c.effectivePreparationScope()) {
		return fmt.Errorf("execution plan exceeds the original investigation implementation scope")
	}
	return nil
}

// A complete projection includes previous unchanged packets. Scope restriction
// is enforced mechanically before the independent semantic envelope review.
func (c *Context) validatePlanningFiles(files map[string]string, request planning.Request) error {
	if c.State.Planning.CurrentDesign == nil {
		return fmt.Errorf("child plan requires a root design")
	}
	design := planning.FindDesign(c.State.Planning, *c.State.Planning.CurrentDesign)
	if err := c.checkPlanningRepairScope(design, false); err != nil {
		return err
	}
	spec, err := contract.ParseSpec([]byte(files["spec.md"]), "spec.md")
	if err != nil {
		return err
	}
	if spec.PlanningRef != nil || spec.Goal != c.State.Planning.Brief.Goal || len(spec.Requirements) != len(design.Requirements) || len(spec.Milestones) != len(design.Milestones) {
		return fmt.Errorf("child plan cannot rewrite the approved goal, root coverage or planning authority")
	}
	for i, r := range design.Requirements {
		if !reflect.DeepEqual(spec.Requirements[i], contract.Requirement{ID: r.ID, Title: r.Title, Milestone: r.Milestone, Verification: r.Verification}) {
			return fmt.Errorf("child plan changes approved requirement %s or its verification floor", r.ID)
		}
	}
	for i, m := range design.Milestones {
		actual := spec.Milestones[i]
		if actual.ID != m.ID || actual.Title != m.Title || actual.Gate != m.Gate {
			return fmt.Errorf("child plan changes approved milestone %s", m.ID)
		}
	}
	dir, err := os.MkdirTemp("", "flywheel-planning-validation-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for name, raw := range files {
		if isOwnedOraclePath(name) {
			if len(raw) > maxOracleBytes {
				return fmt.Errorf("oracle %s exceeds %d byte limit", name, maxOracleBytes)
			}
			_ = os.MkdirAll(filepath.Join(dir, "oracles"), 0700)
			target := filepath.Join(dir, name)
			if err := os.WriteFile(target, []byte(raw), 0600); err != nil {
				return err
			}
			if err := checkOracleSyntax(name, target); err != nil {
				return err
			}
			continue
		}
		if name != "spec.md" && (!strings.HasPrefix(name, "task-packet-") || !strings.HasSuffix(name, ".md") || filepath.Base(name) != name) {
			return fmt.Errorf("unowned projected path %q", name)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(raw), 0600); err != nil {
			return err
		}
	}
	packets, err := contract.LoadPackets(dir, spec)
	if err != nil {
		return err
	}
	requirementMilestones := map[string]string{}
	deferred := map[string]bool{}
	for _, r := range design.Requirements {
		requirementMilestones[r.ID] = r.Milestone
	}
	for _, m := range spec.Milestones {
		deferred[m.ID] = m.Planning == "deferred"
	}
	covered := map[string]bool{}
	milestonePackets := map[string]int{}
	found := false
	for _, p := range packets {
		milestone, _ := contract.MilestoneOfPacket(p.PacketID)
		milestonePackets[milestone]++
		for _, id := range p.Requirements {
			if requirementMilestones[id] != milestone {
				return fmt.Errorf("packet %s assigns requirement %s to a different milestone than the approved design", p.PacketID, id)
			}
			covered[id] = true
		}
		if request.ScopeID == p.PacketID || request.ScopeID == milestone {
			found = true
		}
		if p.Schedule == nil || len(p.Schedule.Writes) == 0 {
			return fmt.Errorf("reviewed packet %s requires explicit schedule.writes", p.PacketID)
		}
		for _, path := range p.Schedule.Writes {
			inScope := false
			for _, allowed := range design.ImplementationScope {
				if allowed == "." || strings.TrimSuffix(path, "/") == allowed || strings.HasPrefix(path, strings.TrimSuffix(allowed, "/")+"/") {
					inScope = true
				}
			}
			if !inScope {
				return fmt.Errorf("packet %s writes outside the approved design: %s", p.PacketID, path)
			}
		}
		for _, level := range []string{"L1", "L2", "L3"} {
			for _, gate := range p.Acceptance.Level(level) {
				cmd := strings.TrimSpace(gate.Command())
				if cmd == "true" || cmd == ":" || cmd == "exit 0" {
					return fmt.Errorf("packet %s has a placeholder acceptance command", p.PacketID)
				}
			}
		}
	}
	if !found {
		return fmt.Errorf("requested planning scope %s has no executable packet", request.ScopeID)
	}
	for _, m := range spec.Milestones {
		if !deferred[m.ID] && milestonePackets[m.ID] == 0 {
			return fmt.Errorf("milestone %s has no packet; future unplanned work must remain explicitly deferred", m.ID)
		}
	}
	for _, r := range design.Requirements {
		if !deferred[r.Milestone] && !covered[r.ID] {
			return fmt.Errorf("approved requirement %s has no packet in its ready milestone", r.ID)
		}
	}
	if active := c.State.Planning.Active; active != nil {
		previous := c.planFiles(active.Plans)
		for name, raw := range previous {
			if name == "spec.md" || isOwnedOraclePath(name) {
				continue
			}
			id := strings.TrimSuffix(strings.TrimPrefix(name, "task-packet-"), ".md")
			milestone, _ := contract.MilestoneOfPacket(id)
			ps := c.State.Packets[id]
			if (request.ScopeID != id && request.ScopeID != milestone || ps != nil && ps.Status == "completed") && files[name] != raw {
				return fmt.Errorf("child plan cannot retire or rewrite unaffected/completed packet %s", id)
			}
		}
	}
	return nil
}

// Each child version contains a complete projection; the most recent version
// is authoritative. Merging older maps would resurrect retired packets.
func (c *Context) planFiles(refs []planning.Ref) map[string]string {
	var latest *planning.ExecutionPlan
	for _, ref := range refs {
		p := planning.FindPlan(c.State.Planning, ref)
		if p != nil && (latest == nil || p.Ref.Revision > latest.Ref.Revision) {
			latest = p
		}
	}
	if latest == nil {
		return map[string]string{}
	}
	out := map[string]string{}
	for name, raw := range latest.Files {
		out[name] = raw
	}
	return out
}

func (c *Context) projectedPlanningFiles(a *planning.Activation) (map[string]string, error) {
	files := c.planFiles(a.Plans)
	data, body, err := contract.ParseFrontmatter([]byte(files["spec.md"]), "spec.md")
	if err != nil {
		return nil, err
	}
	refRaw, _ := json.Marshal(planning.ActivationRefs(*a))
	var ref any
	_ = json.Unmarshal(refRaw, &ref)
	data["planning-ref"] = ref
	// A tree parsed by yaml.Unmarshal plus concrete JSON-safe references contains
	// only values representable by YAML; there are no custom marshalers here.
	fm, _ := yaml.Marshal(data)
	files["spec.md"] = "---\n" + string(fm) + "---\n" + body
	return files, nil
}

func (c *Context) verifyPlanningProjection(a *planning.Activation) error {
	if a == nil {
		return fmt.Errorf("execution projection is not active")
	}
	root, err := planningRoot(c.TaskDir, false)
	if err != nil {
		return err
	}
	defer root.Close()
	for name, digest := range a.Files {
		raw, err := readPlanningBytes(root, name)
		if err != nil {
			return err
		}
		if planning.DigestBytes(raw) != digest {
			return fmt.Errorf("projected contract changed: %s", name)
		}
	}
	entries, err := listPlanningDirectory(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "task-packet-") && strings.HasSuffix(entry.Name(), ".md") && a.Files[entry.Name()] == "" {
			return fmt.Errorf("unowned packet outside active projection: %s", entry.Name())
		}
	}
	return nil
}

func (c *Context) activatePlanningPlans() error {
	p := c.State.Planning
	if p.PendingActivation != nil {
		return c.resumePlanningActivation()
	}
	if p.CurrentDesign == nil || !planning.Authorized(p, *p.CurrentDesign) {
		return fmt.Errorf("activation requires the exact approved design")
	}
	if err := c.checkPlanningRepairScope(planning.FindDesign(p, *p.CurrentDesign), true); err != nil {
		return err
	}
	refs := []planning.Ref{}
	for _, ref := range p.CurrentPlans {
		r := planning.LatestReview(p, "plan", ref)
		if r == nil || r.Verdict != "pass" || r.ScopeVerdict != "within-envelope" {
			return fmt.Errorf("execution plan requires independent passing review")
		}
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Revision < refs[j].Revision })
	if len(refs) == 0 {
		return fmt.Errorf("activation requires an execution plan")
	}
	if p.Active != nil && p.Active.Design == *p.CurrentDesign && reflect.DeepEqual(p.Active.Plans, refs) {
		if err := c.verifyPlanningProjection(p.Active); err != nil {
			return err
		}
		p.Phase = "ready"
		c.save()
		return c.err()
	}
	head, ok := verify.GetBaseRef(c.Cwd)
	if !ok {
		return fmt.Errorf("reviewed execution requires a Git checkout")
	}
	latest := planning.FindPlan(p, refs[len(refs)-1])
	dependency, err := c.planScopeIdentity()
	if err != nil {
		return err
	}
	if dependency != latest.Dependencies {
		return fmt.Errorf("repository dependencies changed after planning; request a new child plan")
	}
	a := &planning.Activation{Design: *p.CurrentDesign, Plans: refs, Files: map[string]string{}, OldFiles: map[string]string{}, Stage: "intent", Base: head}
	for name, digest := range p.LegacyFiles {
		a.OldFiles[name] = digest
	}
	if p.Active != nil {
		for name, digest := range p.Active.Files {
			a.OldFiles[name] = digest
		}
	}
	files, err := c.projectedPlanningFiles(a)
	if err != nil {
		return err
	}
	for name, raw := range files {
		a.Files[name] = planning.DigestBytes([]byte(raw))
	}
	p.PendingActivation, p.Phase = a, "activating"
	c.save() // Durable intent BEFORE touching any executable contract or Git.
	if err := c.err(); err != nil {
		return err
	}
	return c.resumePlanningActivation()
}

func (c *Context) resumePlanningActivation() error {
	a := c.State.Planning.PendingActivation
	if a == nil {
		return nil
	}
	if err := c.checkPlanningRepairScope(planning.FindDesign(c.State.Planning, a.Design), true); err != nil {
		return err
	}
	if err := c.checkPlanningExports(true); err != nil {
		return err
	}
	latest := planning.FindPlan(c.State.Planning, a.Plans[len(a.Plans)-1])
	dependency, err := c.planScopeIdentity()
	if err != nil {
		return err
	}
	if dependency != latest.Dependencies {
		return fmt.Errorf("repository dependencies changed during activation; preserve the pending transaction for explicit recovery")
	}
	files, err := c.projectedPlanningFiles(a)
	if err != nil {
		return err
	}
	root, err := planningRoot(c.TaskDir, false)
	if err != nil {
		return err
	}
	defer root.Close()
	for name, raw := range files {
		actual, err := readPlanningBytes(root, name)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		digest := planning.DigestBytes(actual)
		if err == nil && digest != a.Files[name] && digest != a.OldFiles[name] {
			return fmt.Errorf("activation refuses an externally changed contract: %s", name)
		}
		if err != nil || digest != a.Files[name] {
			if isOwnedOraclePath(name) {
				if err := mkdirPlanningDirectory(root, "oracles", 0755); err != nil && !os.IsExist(err) {
					return err
				}
			}
			if err := writePlanningBytes(root, name, []byte(raw)); err != nil {
				return err
			}
		}
	}
	for name, digest := range a.OldFiles {
		if a.Files[name] != "" {
			continue
		}
		raw, err := readPlanningBytes(root, name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if planning.DigestBytes(raw) != digest {
			return fmt.Errorf("retired packet changed externally: %s", name)
		}
		if err := removePlanningFile(root, name); err != nil {
			return err
		}
	}
	if err := c.verifyPlanningProjection(a); err != nil {
		return err
	}
	if err := c.loadPlanningContracts(); err != nil {
		return err
	}
	if err := c.persistPlanContracts(fmt.Sprintf("docs(%s): activate reviewed design %d", c.State.Slug, a.Design.Revision)); err != nil {
		return err
	}
	head, ok := verify.GetBaseRef(c.Cwd)
	if !ok || head == verify.EmptyTree {
		return fmt.Errorf("activation has no committed Git checkpoint")
	}
	if err := c.verifyPlanningArtifacts(); err != nil {
		return err
	}
	if err := c.verifyPlanningProjection(a); err != nil {
		return err
	}
	a.Stage, a.Head = "synced", head
	c.save()
	if err := c.err(); err != nil {
		return err
	}
	a.Stage = "active"
	c.State.Planning.Active = a
	c.State.Planning.PendingActivation = nil
	c.State.Planning.Phase = "ready"
	for _, milestone := range c.Spec.Milestones {
		checkpoint := c.reviewedPlanCheckpoint(milestone.ID)
		if checkpoint != nil {
			ms := c.State.Milestones[milestone.ID]
			ms.Plan = &state.PlanProgress{Status: "passed", StartedAt: c.nowISO(), FinishedAt: c.nowISO(), Contracts: sessionHash(c.State.ContractFingerprints), Reason: "独立实施计划评审通过并完成激活", Checkpoint: checkpoint}
			c.State.Milestones[milestone.ID] = ms
		}
	}
	c.save() // Only the completed projection/commit/sync transaction becomes live.
	return c.err()
}
