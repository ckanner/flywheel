package machine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/planning"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
)

// PlanningCommand contains data only. Human identity is supplied separately by
// the verified host command or a terminal confirmation, never by request JSON.
type PlanningCommand struct {
	Action    string          `json:"action"`
	Revision  int64           `json:"revision,omitempty"`
	RequestID string          `json:"request-id,omitempty"`
	Brief     *planning.Brief `json:"brief,omitempty"`
	Scope     string          `json:"scope,omitempty"`
	ScopeID   string          `json:"scope-id,omitempty"`
	Design    *planning.Ref   `json:"design,omitempty"`
	Text      string          `json:"text,omitempty"`
	Policy    string          `json:"policy,omitempty"`
	Start     *bool           `json:"start,omitempty"`
}

type PlanningControlReceipt struct {
	RequestID string `json:"request-id,omitempty"`
	Action    string `json:"action"`
	Status    string `json:"status"`
	Revision  int64  `json:"revision"`
	Message   string `json:"message"`
}

var planningControlID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,119}$`)
var planningRefDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

func normalizePlanningScope(r PlanningCommand) (PlanningCommand, error) {
	if r.Action != "plan" {
		return r, nil
	}
	if r.Scope == "design" {
		if r.ScopeID != "" && r.ScopeID != "root" {
			return r, fmt.Errorf("design scope only accepts the root identifier")
		}
	}
	r.Scope, r.ScopeID = planning.CanonicalScope(r.Scope, r.ScopeID)
	if r.Scope == "replan" {
		return r, fmt.Errorf("replan requires root, a milestone ID, or a packet ID")
	}
	return r, nil
}

func (r PlanningCommand) StartRequested(defaultValue bool) bool {
	if r.Start != nil {
		return *r.Start
	}
	return defaultValue
}

func ValidatePlanningCommand(r PlanningCommand) error {
	if r.Brief != nil && r.Action != "prepare" || (r.Scope != "" || r.ScopeID != "") && r.Action != "plan" ||
		r.Design != nil && r.Action != "approve" && r.Action != "feedback" ||
		r.Text != "" && r.Action != "feedback" && r.Action != "migrate" && r.Action != "plan" ||
		r.Policy != "" && r.Action != "feedback" && r.Action != "migrate" ||
		r.Action == "status" && (r.Revision != 0 || r.Start != nil) ||
		r.Action == "prepare" && r.Revision != 0 {
		return fmt.Errorf("planning fields do not belong to the requested action")
	}
	if r.RequestID != "" && !planningControlID.MatchString(r.RequestID) {
		return fmt.Errorf("planning request-id must be a stable bounded identifier")
	}
	if r.Action == "status" {
		return nil
	}
	if r.Action == "prepare" {
		if r.Brief == nil {
			return fmt.Errorf("prepare requires a thin brief retaining the original user request")
		}
		return nil
	}
	if r.Revision < 1 || r.RequestID == "" {
		return fmt.Errorf("planning mutation requires a positive observed revision and request-id")
	}
	if r.Design != nil && (r.Design.Revision < 1 || !planningRefDigest.MatchString(r.Design.Digest)) {
		return fmt.Errorf("planning design must identify an exact revision and SHA256 digest")
	}
	if r.Policy != "" && r.Policy != "review-first" && r.Policy != "delegated" {
		return fmt.Errorf("planning policy must be review-first or delegated")
	}
	if len(r.Text) > 8192 || strings.ContainsRune(r.Text, 0) {
		return fmt.Errorf("planning feedback must be at most 8192 bytes without NUL")
	}
	switch r.Action {
	case "plan":
		if r.Scope != "design" && r.Scope != "milestone" && r.Scope != "packet" && r.Scope != "replan" {
			return fmt.Errorf("planning scope must be design, milestone, packet, or replan")
		}
		if r.ScopeID != "" && !planningControlID.MatchString(r.ScopeID) || (r.Scope == "milestone" || r.Scope == "packet") && r.ScopeID == "" {
			return fmt.Errorf("scoped planning requires a bounded scope-id")
		}
		_, err := normalizePlanningScope(r)
		return err
	case "feedback":
		if strings.TrimSpace(r.Text) == "" {
			return fmt.Errorf("feedback requires nonempty text")
		}
	case "approve":
		if r.Design == nil {
			return fmt.Errorf("approval requires the exact reviewed design")
		}
	case "migrate":
	default:
		return fmt.Errorf("planning action must be prepare, plan, feedback, approve, status, or migrate")
	}
	return nil
}

// ControlPlanning performs no model calls. Only feedback can be queued while a
// run owns the lease; the owner applies it after its prior receipt is consumed.
func ControlPlanning(opts RunOpts, request PlanningCommand, trustedSource string) (*state.State, *PlanningControlReceipt, error) {
	if err := ValidatePlanningCommand(request); err != nil {
		return nil, nil, err
	}
	request, _ = normalizePlanningScope(request) // checked by ValidatePlanningCommand
	if request.Action == "approve" && ((!strings.HasPrefix(trustedSource, "dsh:") && !strings.HasPrefix(trustedSource, "tty:")) || len(trustedSource) <= 4 ||
		len(trustedSource) > 1024 || strings.ContainsAny(trustedSource, "\x00\r\n")) {
		return nil, nil, fmt.Errorf("design approval requires a verified human command or controlling terminal")
	}
	if request.Action != "approve" && trustedSource != "" {
		return nil, nil, fmt.Errorf("human approval proof is valid only for design approval")
	}
	if request.Action == "status" {
		st, err := state.Load(opts.TaskDir)
		if err == nil && request.RequestID != "" {
			receipt, err := readPlanningControlReceipt(opts.TaskDir, request.RequestID, st)
			return st, receipt, err
		}
		return st, nil, err
	}
	if request.Action == "prepare" {
		if err := os.MkdirAll(opts.TaskDir, 0700); err != nil {
			return nil, nil, err
		}
	}
	release, err := AcquireRunLease(opts.TaskDir)
	if err != nil {
		if request.Action == "feedback" && errors.Is(err, syscall.EWOULDBLOCK) {
			return queuePlanningFeedback(opts, request)
		}
		return nil, nil, err
	}
	defer release()
	if request.Action == "prepare" {
		st, unchanged, err := preparePlanning(opts, *request.Brief)
		return st, planningControlResult(st, request, unchanged, "Thin brief saved; design and review remain independent stages."), err
	}
	if request.Action == "migrate" {
		st, unchanged, err := migratePlanning(opts, request)
		return st, planningControlResult(st, request, unchanged, "Legacy contracts retained as source evidence; no review or approval was invented."), err
	}
	st, err := state.Load(opts.TaskDir)
	if err != nil {
		return nil, nil, err
	}
	if err := planningControlWorkspace(opts, st); err != nil {
		return st, nil, err
	}
	if rejected := rejectedPlanningReceipt(st, request.RequestID); rejected != nil {
		return st, rejected, fmt.Errorf("planning request-id was already rejected; submit refreshed input with a new request-id")
	}
	already, err := planningCommandApplied(st.Planning, request, trustedSource)
	if err != nil || already {
		return st, planningControlResult(st, request, already, "This exact planning action is already recorded."), err
	}
	if st.Revision != request.Revision {
		return st, nil, fmt.Errorf("planning state revision changed; refresh before submitting this action")
	}
	if st.Planning.PendingActivation != nil || st.Handoff != nil {
		return st, nil, fmt.Errorf("planning control requires completed activation and returned human session control")
	}
	if planningHasPendingReceipt(st) {
		if request.Action == "feedback" {
			return queuePlanningFeedback(opts, request)
		}
		return st, nil, fmt.Errorf("consume or reconcile the original invocation receipt before changing planning authority")
	}
	if err := applyPlanningCommand(opts, st, request, trustedSource); err != nil {
		return st, nil, err
	}
	st, err = state.Save(opts.TaskDir, st, &request.Revision)
	return st, planningControlResult(st, request, false, "Planning action recorded for the exact current input."), err
}

func planningControlResult(st *state.State, r PlanningCommand, unchanged bool, message string) *PlanningControlReceipt {
	if st == nil {
		return nil
	}
	status := "applied"
	if unchanged {
		status = "unchanged"
	}
	return &PlanningControlReceipt{RequestID: r.RequestID, Action: r.Action, Status: status, Revision: st.Revision, Message: message}
}

func planningControlWorkspace(opts RunOpts, st *state.State) error {
	if st.Planning == nil {
		return fmt.Errorf("legacy task requires an explicit planning migrate action")
	}
	cwd, err := canonicalDirectory(ProjectRoot(opts.Cwd))
	if err != nil {
		return err
	}
	if st.Project == nil || st.Project.Cwd != cwd {
		return fmt.Errorf("planning workspace differs from the recorded task workspace")
	}
	return nil
}

func planningHasPendingReceipt(st *state.State) bool {
	if st.WorkerPrewarm != nil {
		return true
	}
	if legacy, ok := st.PlanSession.(map[string]any); ok && legacy["outcome-unknown"] == true {
		return true
	}
	if st.Sessions != nil {
		for _, record := range st.Sessions.Records {
			// An explicit human abandonment retires the invocation without
			// applying a result. It is terminal for the planning boundary just
			// like a consumed receipt; the unknown facts remain in Last.
			if record.Last != nil && record.Last.Status != "consumed" && record.Last.Status != "abandoned" {
				return true
			}
		}
	}
	return false
}

func planningControlTime(opts RunOpts) string {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return now().UTC().Format(time.RFC3339Nano)
}

func planningCommandApplied(p *planning.State, r PlanningCommand, source string) (bool, error) {
	var err error
	r, err = normalizePlanningScope(r)
	if err != nil {
		return false, err
	}
	matched, same := false, true
	for _, old := range p.Requests {
		if old.ID == r.RequestID {
			matched, same = true, same && r.Action == "plan" && old.Scope == r.Scope && old.ScopeID == r.ScopeID && old.PlanningOnly && old.Feedback == r.Text
		}
	}
	for _, old := range p.Feedback {
		if old.RequestID == r.RequestID {
			matched, same = true, same && r.Action == "feedback" && reflect.DeepEqual(old.Target, r.Design) && old.Text == r.Text && old.Policy == r.Policy
		}
	}
	for _, old := range p.Approvals {
		if old.RequestID == r.RequestID {
			matched, same = true, same && r.Action == "approve" && r.Design != nil && old.Design == *r.Design && old.Start == r.StartRequested(false) && old.Source == source
		}
	}
	if matched && !same {
		return false, fmt.Errorf("planning request-id was already used for different content or authority")
	}
	return matched, nil
}

func applyPlanningCommand(opts RunOpts, st *state.State, r PlanningCommand, source string) error {
	p := st.Planning
	switch r.Action {
	case "plan":
		if err := planning.BeginRequest(p, planning.Request{ID: r.RequestID, Scope: r.Scope, ScopeID: r.ScopeID, InputRevision: p.InputRevision, PlanningOnly: true, Feedback: r.Text}); err != nil {
			return err
		}
		p.Phase, p.Reason = "drafting", "Planning requested without starting implementation."
	case "feedback":
		if err := planning.ApplyFeedback(p, planning.Feedback{RequestID: r.RequestID, Target: r.Design, Source: "control:" + r.RequestID, Text: r.Text, Policy: r.Policy, At: planningControlTime(opts)}); err != nil {
			return err
		}
	case "approve":
		review := planning.LatestReview(p, "design", *r.Design)
		if review == nil {
			return fmt.Errorf("design approval requires its independent review receipt")
		}
		reportHash, err := planningRepairAuthority(opts, st, r)
		if err != nil {
			return err
		}
		if err := planning.Approve(p, planning.Approval{Kind: "human", Design: *r.Design, ReviewID: review.ID, RequestID: r.RequestID, Source: source, Start: r.StartRequested(false), At: planningControlTime(opts), ReportHash: reportHash}); err != nil {
			return err
		}
	}
	st.Status = "paused"
	return nil
}

func preparePlanning(opts RunOpts, brief planning.Brief) (*state.State, bool, error) {
	if brief.Version == 0 {
		brief.Version = 1
	}
	if brief.Policy == "" {
		brief.Policy = "review-first"
	}
	if brief.Constraints == nil {
		brief.Constraints = []string{}
	}
	if brief.Evidence == nil {
		brief.Evidence = []string{}
	}
	cwd, err := canonicalDirectory(ProjectRoot(opts.Cwd))
	if err != nil {
		return nil, false, err
	}
	if _, ok := verify.GetBaseRef(cwd); !ok {
		return nil, false, fmt.Errorf("planning preparation requires a Git workspace with an existing baseline")
	}
	preparation, err := planningPreparation(opts.TaskDir, cwd)
	if err != nil {
		return nil, false, err
	}
	if preparation != nil && preparation.Contract.Intent == "investigate" && brief.ImplementationAuthorized {
		return nil, false, fmt.Errorf("report-only investigation requires an exact human design approval with start before implementation")
	}
	originalBrief := brief
	if preparation != nil && preparation.Contract.Intent == "investigate-and-fix" {
		brief.ImplementationAuthorized = true
		if brief.Authorization == "" {
			brief.Authorization = "investigation:" + preparation.ContractHash
		}
	}
	if state.Exists(opts.TaskDir) {
		st, err := state.Load(opts.TaskDir)
		if err != nil {
			return nil, false, err
		}
		if err := planningControlWorkspace(opts, st); err != nil {
			return st, false, err
		}
		if !reflect.DeepEqual(st.Planning.Brief, brief) && !reflect.DeepEqual(st.Planning.Brief, originalBrief) {
			return st, false, fmt.Errorf("task already has a different brief; submit version-bound feedback")
		}
		if preparation != nil {
			c := &Context{State: st}
			imported := st.PreparationSource == nil
			if err := c.importPreparation(preparation); err != nil {
				return st, false, err
			}
			if imported {
				st, err = state.Save(opts.TaskDir, st, &st.Revision)
				return st, false, err
			}
		}
		return st, true, nil
	}
	if _, err := os.Stat(filepath.Join(opts.TaskDir, "spec.md")); !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("existing implementation contracts require explicit migration")
	}
	cfg := opts.Config
	if cfg == nil {
		value := config.Default()
		cfg = &value
	}
	st, err := state.InitPlanning(brief, cfg)
	if err != nil {
		return nil, false, err
	}
	st.Project = &state.ProjectIdentity{Cwd: cwd, WorkspaceCwd: WorkspaceRoot(cwd)}
	if preparation != nil {
		c := &Context{State: st}
		_ = c.importPreparation(preparation) // fresh state has no conflicting import marker
	}
	st, err = state.Save(opts.TaskDir, st, nil)
	return st, false, err
}
