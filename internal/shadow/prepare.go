package shadow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
)

type snapshot struct {
	Spec     *contract.Spec
	Packets  []*contract.Packet
	Revision int64
	Status   string
	Workflow *state.WorkflowState
	State    *state.State `json:"-"`
}

func readSnapshot(directory string) (snapshot, error) {
	spec, err := contract.LoadSpec(directory)
	if err != nil {
		return snapshot{}, err
	}
	packets, err := contract.LoadPackets(directory, spec)
	if err != nil {
		return snapshot{}, err
	}
	s := snapshot{Spec: spec, Packets: packets}
	_, err = os.Stat(state.StateFile(directory))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return snapshot{}, err
	}
	s.State, err = state.Load(directory)
	if err != nil {
		return snapshot{}, err
	}
	s.Revision, s.Status, s.Workflow = s.State.Revision, s.State.Status, s.State.Workflow
	if s.Status == "running" {
		return snapshot{}, fmt.Errorf("capture a prepared or quiescent task, not a running task")
	}
	return s, nil
}

var snapshotRead = readSnapshot
var absolutePath = filepath.Abs
var homePath = regexp.MustCompile(`/(Users|home)/[^/\s"']+`)
var webAddress = regexp.MustCompile(`https?://[^\s"'<>]+`)

func deidentify(text string) string {
	return homePath.ReplaceAllString(webAddress.ReplaceAllString(text, "[url]"), "[home]")
}

// structuralBlockers probes a copy using the existing selector. Its semantic
// declarations are neutralized only for this read-only diagnostic projection.
func structuralBlockers(s snapshot, mode, effective string) []string {
	if s.State != nil && effective == "full" {
		return []string{"Persisted Full workflow cannot be downgraded"}
	}
	if mode == "full" {
		return []string{"Full explicitly requested"}
	}
	probe := *s.Spec
	if probe.Workflow != nil {
		w := *probe.Workflow
		w.ScopeClear, w.ImplementationKnown, w.VerificationComplete, w.ReviewSufficient = true, true, true, true
		w.Reason, w.Evidence = "Structural probe only", []string{"No semantic qualification is asserted by this probe"}
		probe.Workflow = &w
	}
	selection, _ := contract.SelectWorkflow(&probe, s.Packets, "quick") // validated version/mode; fixed valid override
	if selection.Effective == "full" {
		return []string{deidentify(selection.Reason)}
	}
	return []string{}
}

// Prepare reads two matching contract/state snapshots. It does not acquire a
// task writer lease or touch task files; old snapshots are not initial labels.
func Prepare(directory, mode string) (Dataset, error) {
	absolute, err := absolutePath(directory)
	if err != nil {
		return Dataset{}, err
	}
	physical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return Dataset{}, err
	}
	first, err := snapshotRead(physical)
	if err != nil {
		return Dataset{}, err
	}
	second, err := snapshotRead(physical)
	if err != nil {
		return Dataset{}, err
	}
	if digest(jsonBytes(first)) != digest(jsonBytes(second)) {
		return Dataset{}, fmt.Errorf("task changed while preparing shadow snapshot; capture again")
	}
	if first.State != nil && mode != "" {
		return Dataset{}, fmt.Errorf("an existing task uses its persisted mode; --mode is only for a new prepared task")
	}
	selection, err := state.ResolveWorkflow(first.State, first.Spec, first.Packets, mode)
	if err != nil {
		return Dataset{}, err
	}
	kind := "prepared"
	if first.State != nil {
		kind = "replay"
	}
	return prepareSnapshot(physical, first, selection, mode, kind)
}

// Capture uses already validated initial contracts and the existing selection.
// It neither reads nor writes state, and must be called only for a new task.
func Capture(directory string, spec *contract.Spec, packets []*contract.Packet, selection contract.WorkflowSelection) (Dataset, error) {
	return prepareSnapshot(directory, snapshot{Spec: spec, Packets: packets}, selection, selection.Requested, "decision-time")
}

func prepareSnapshot(physical string, first snapshot, selection contract.WorkflowSelection, mode, kind string) (Dataset, error) {
	ref := digest(append([]byte(physical+"\x00"), jsonBytes(first)...))
	c := Case{ID: "snapshot-" + ref[:16], Source: Source{kind, ref}, Task: deidentify(first.Spec.Goal),
		Evidence: []string{}, Baseline: Baseline{selection.Effective, deidentify(selection.Reason)},
		HardBlockers: structuralBlockers(first, mode, selection.Effective)}
	for _, r := range first.Spec.Requirements {
		c.Evidence = append(c.Evidence, deidentify(r.Title+"; verification described: "+r.Verification))
	}
	for _, p := range first.Packets {
		c.Evidence = append(c.Evidence, deidentify("Packet goal: "+p.Goal))
	}
	if w := first.Spec.Workflow; w != nil {
		for _, e := range append([]string{w.Reason}, w.Evidence...) {
			c.Evidence = append(c.Evidence, deidentify(e))
		}
	}
	return ParseDataset(jsonBytes(Dataset{Version: 1, Mode: "shadow", Cases: []Case{c}}))
}
