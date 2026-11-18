package machine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
)

// The durable intent preserves the original contracts, not invented planner or
// reviewer invocations. The fence refs are migration markers, never active refs.
type planningMigration struct {
	Version   int               `json:"version"`
	Workspace string            `json:"workspace"`
	Command   PlanningCommand   `json:"command"`
	Brief     planning.Brief    `json:"brief"`
	Original  map[string]string `json:"original"`
	Fenced    map[string]string `json:"fenced"`
}

func planningFence(raw []byte, ref planning.ActivationRef) ([]byte, error) {
	fields, body, err := contract.ParseFrontmatter(raw, "spec.md")
	if err != nil {
		return nil, err
	}
	fields["planning-ref"] = ref
	header, err := json.Marshal(fields) // JSON is also valid YAML frontmatter.
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(header) + "\n---\n" + body), nil
}

func newPlanningMigration(opts RunOpts, st *state.State, command PlanningCommand) (planningMigration, error) {
	intent := planningMigration{Version: 1, Workspace: ProjectRoot(opts.Cwd), Command: command, Original: map[string]string{}, Fenced: map[string]string{}}
	spec, err := contract.LoadSpec(opts.TaskDir)
	if err != nil {
		return intent, err
	}
	if spec.PlanningRef != nil {
		return intent, fmt.Errorf("a fenced legacy task requires its original migration intent")
	}
	packets, err := contract.LoadPackets(opts.TaskDir, spec)
	if err != nil {
		return intent, err
	}
	root, err := planningRoot(opts.TaskDir, false)
	if err != nil {
		return intent, err
	}
	defer root.Close()
	names := []string{"spec.md"}
	for _, p := range packets {
		names = append(names, "task-packet-"+p.PacketID+".md")
	}
	sort.Strings(names)
	evidence := []string{}
	for _, name := range names {
		raw, err := readPlanningBytes(root, name)
		if err != nil {
			return intent, err
		}
		intent.Original[name], intent.Fenced[name] = string(raw), string(raw)
		evidence = append(evidence, name+" SHA256 "+planning.DigestBytes(raw))
	}
	ref := planning.ActivationRef{Version: 1, Design: planning.Ref{Revision: 1, Digest: planning.DigestBytes([]byte(intent.Original["spec.md"]))},
		Plans: []planning.Ref{{Revision: 1, Digest: sessionHash(intent.Original)}}}
	fenced, err := planningFence([]byte(intent.Original["spec.md"]), ref)
	if err != nil {
		return intent, err
	}
	intent.Fenced["spec.md"] = string(fenced)
	policy := command.Policy
	if policy == "" {
		policy = "review-first"
	}
	request := command.Text
	if request == "" {
		request = spec.Goal
	}
	mode := "full"
	if st.Workflow != nil {
		mode = st.Workflow.Effective
	}
	intent.Brief = planning.Brief{Version: 1, ID: st.Slug, Goal: spec.Goal, OriginalRequest: request,
		Constraints: []string{"Retain existing implementation, costs and historical evidence; review any changed design before adoption."},
		Evidence:    evidence, Policy: policy, Mode: mode, ImplementationAuthorized: true,
		Authorization: "legacy-task:" + st.Slug + ":" + sessionHash(intent.Original)}
	return intent, planning.ValidateBrief(intent.Brief)
}

func migratePlanning(opts RunOpts, command PlanningCommand) (*state.State, bool, error) {
	st, err := state.Load(opts.TaskDir)
	if err != nil {
		return nil, false, err
	}
	cwd, err := canonicalDirectory(ProjectRoot(opts.Cwd))
	if err != nil {
		return st, false, err
	}
	if st.Project != nil && st.Project.Cwd != cwd || planningHasPendingReceipt(st) || st.Handoff != nil || st.WorkerPrewarm != nil {
		return st, false, fmt.Errorf("migration requires the original workspace and settled owned calls")
	}
	root, err := planningControlRoot(opts.TaskDir, true)
	if err != nil {
		return st, false, err
	}
	defer root.Close()
	var intent planningMigration
	raw, err := readPlanningBytes(root, "migration.json")
	if os.IsNotExist(err) {
		if st.Planning != nil || st.Revision != command.Revision {
			return st, false, fmt.Errorf("migration requires the observed legacy state revision")
		}
		intent, err = newPlanningMigration(opts, st, command)
		if err != nil {
			return st, false, err
		}
		intent.Workspace = cwd
		raw, _ = json.Marshal(intent)
		if len(raw) > 2<<20 {
			return st, false, fmt.Errorf("migration source snapshot exceeds 2 MiB")
		}
		if err := writePlanningBytes(root, "migration.json", raw); err != nil {
			return st, false, err
		}
	} else if err != nil {
		return st, false, err
	} else {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if len(raw) > 2<<20 || !json.Valid(raw) || d.Decode(&intent) != nil || intent.Version != 1 || intent.Workspace != cwd || !reflect.DeepEqual(intent.Command, command) {
			return st, false, fmt.Errorf("migration intent belongs to a different request or workspace")
		}
	}
	if intent.Original["spec.md"] == "" || len(intent.Original) != len(intent.Fenced) {
		return st, false, fmt.Errorf("migration intent is missing complete contract ownership")
	}
	marker := planning.ActivationRef{Version: 1, Design: planning.Ref{Revision: 1, Digest: planning.DigestBytes([]byte(intent.Original["spec.md"]))},
		Plans: []planning.Ref{{Revision: 1, Digest: sessionHash(intent.Original)}}}
	expected, err := planningFence([]byte(intent.Original["spec.md"]), marker)
	if err != nil {
		return st, false, err
	}
	for name, original := range intent.Original {
		fenced, present := intent.Fenced[name]
		if !present || name == "spec.md" && fenced != string(expected) || name != "spec.md" && fenced != original {
			return st, false, fmt.Errorf("migration intent changes unowned contract content: %s", name)
		}
	}
	p, err := planning.New(intent.Brief)
	if err != nil {
		return st, false, err
	}
	p.LegacyFiles = map[string]string{}
	for name, raw := range intent.Fenced {
		p.LegacyFiles[name] = planning.DigestBytes([]byte(raw))
	}
	if err := planning.Validate(p); err != nil {
		return st, false, err
	}
	if st.Planning != nil {
		if !reflect.DeepEqual(st.Planning.LegacyFiles, p.LegacyFiles) || !reflect.DeepEqual(st.Planning.Brief, p.Brief) {
			return st, false, fmt.Errorf("saved planning state differs from the original migration")
		}
		return st, true, nil
	}
	taskRoot, err := planningRoot(opts.TaskDir, false)
	if err != nil {
		return st, false, err
	}
	defer taskRoot.Close()
	for name, original := range intent.Original {
		current, err := readPlanningBytes(taskRoot, name)
		if err != nil {
			return st, false, err
		}
		if string(current) != original && string(current) != intent.Fenced[name] {
			return st, false, fmt.Errorf("legacy contract changed during migration: %s", name)
		}
	}
	if err := writePlanningBytes(taskRoot, "spec.md", []byte(intent.Fenced["spec.md"])); err != nil {
		return st, false, err
	}
	st.Planning = p
	if st.Sessions == nil {
		st.Sessions = &state.SessionLedger{Records: map[string]*state.SessionRecord{}}
	}
	st.Sessions.Version = max(4, st.Sessions.Version)
	if st.Project == nil {
		st.Project = &state.ProjectIdentity{Cwd: cwd, WorkspaceCwd: WorkspaceRoot(cwd)}
	}
	st.Status = "paused"
	st, err = state.Save(opts.TaskDir, st, nil)
	return st, false, err
}
