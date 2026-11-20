// Package planning defines versioned design contracts and deterministic adoption
// rules. It never invokes models, writes task state, or authenticates a caller.
package planning

// Ref identifies exact immutable bytes, not a mutable latest-version label.
type Ref struct {
	Revision int    `json:"revision"`
	Digest   string `json:"digest"`
}

type Brief struct {
	DesignGranularity        string   `json:"design-granularity,omitempty"`
	GranularityReason        string   `json:"granularity-reason,omitempty"`
	Version                  int      `json:"version"`
	ID                       string   `json:"id"`
	Goal                     string   `json:"goal"`
	OriginalRequest          string   `json:"original-request"`
	Constraints              []string `json:"constraints"`
	Evidence                 []string `json:"evidence"`
	Policy                   string   `json:"policy"`
	Authorization            string   `json:"authorization,omitempty"`
	ImplementationAuthorized bool     `json:"implementation-authorized"`
	Mode                     string   `json:"mode,omitempty"`
}

type Requirement struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Milestone    string `json:"milestone"`
	Verification string `json:"verification"`
}

type Decision struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // fixed | delegated | unresolved
	Statement string `json:"statement"`
	Rationale string `json:"rationale"`
}

type Milestone struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Gate  string `json:"gate"`
}

type Design struct {
	Ref                 Ref           `json:"ref"`
	InputRevision       int           `json:"input-revision"`
	Document            string        `json:"document"`
	Requirements        []Requirement `json:"requirements"`
	Decisions           []Decision    `json:"decisions"`
	Milestones          []Milestone   `json:"milestones"`
	Baseline            string        `json:"baseline"`
	PlannerInvocation   string        `json:"planner-invocation"`
	ImplementationScope []string      `json:"implementation-scope"`
}

// Files holds the unprojected contracts. The engine adds planning-ref only after
// sealing, avoiding a digest that recursively contains itself.
type ExecutionPlan struct {
	Preparation       string            `json:"preparation,omitempty"`
	Ref               Ref               `json:"ref"`
	Design            Ref               `json:"design"`
	InputRevision     int               `json:"input-revision"`
	Scope             string            `json:"scope"`
	ScopeID           string            `json:"scope-id"`
	Files             map[string]string `json:"files"`
	Dependencies      string            `json:"dependencies"`
	ScopeVerdict      string            `json:"scope-verdict"`
	PlannerInvocation string            `json:"planner-invocation"`
}

type Finding struct {
	ID         string `json:"id"`
	Severity   string `json:"severity"`
	Target     string `json:"target"`
	Evidence   string `json:"evidence"`
	Impact     string `json:"impact"`
	Resolution string `json:"resolution,omitempty"`
	Status     string `json:"status"`
}

// Invocation, Role and Binding are supplied by the engine from a settled
// independent call, never trusted from a model's output.
type Review struct {
	DesignVerdict   string    `json:"design-verdict,omitempty"`
	CoverageVerdict string    `json:"coverage-verdict,omitempty"`
	Plan            *Ref      `json:"plan,omitempty"`
	ID              string    `json:"id"`
	Kind            string    `json:"kind"` // design | plan
	Target          Ref       `json:"target"`
	Design          *Ref      `json:"design,omitempty"`
	InputRevision   int       `json:"input-revision"`
	Invocation      string    `json:"invocation"`
	Role            string    `json:"role"`
	Binding         string    `json:"binding"`
	Verdict         string    `json:"verdict"`
	ScopeVerdict    string    `json:"scope-verdict,omitempty"`
	Summary         string    `json:"summary"`
	Findings        []Finding `json:"findings"`
}

// Source is a receipt from the trusted user control boundary. The engine must
// authenticate it before calling Approve; a model-provided string is not proof.
type Approval struct {
	Kind       string `json:"kind"` // human | delegated
	Design     Ref    `json:"design"`
	ReviewID   string `json:"review-id"`
	RequestID  string `json:"request-id"`
	Source     string `json:"source"`
	At         string `json:"at"`
	Start      bool   `json:"start"`
	ReportHash string `json:"report-hash,omitempty"`
}

type Feedback struct {
	RequestID string `json:"request-id"`
	Target    *Ref   `json:"target,omitempty"`
	Source    string `json:"source"`
	Text      string `json:"text"`
	Policy    string `json:"policy,omitempty"`
	At        string `json:"at"`
}

// Request bounds a single planning scope, while State.Rounds is cumulative.
type Request struct {
	DesignGranularity  string `json:"design-granularity,omitempty"`
	ID                 string `json:"id"`
	Scope              string `json:"scope"`
	ScopeID            string `json:"scope-id"`
	InputRevision      int    `json:"input-revision"`
	Rounds             int    `json:"rounds"`
	Retries            int    `json:"retries"`
	MaxRounds          int    `json:"max-rounds"`
	Status             string `json:"status"`
	PlanningOnly       bool   `json:"planning-only"`
	Feedback           string `json:"feedback,omitempty"`
	Stage              string `json:"stage,omitempty"`
	Role               string `json:"role,omitempty"`
	Channel            string `json:"channel,omitempty"`
	BindingSource      string `json:"binding-source,omitempty"`
	BindingGranularity string `json:"binding-granularity,omitempty"`
}

type ActivationRef struct {
	Version int   `json:"version"`
	Design  Ref   `json:"design"`
	Plans   []Ref `json:"plans"`
}

// Activation records the complete projected file set and original ownership
// hashes. A pending transaction always blocks implementation, including when
// its previous active refs remain present.
type Activation struct {
	Design   Ref               `json:"design"`
	Plans    []Ref             `json:"plans"`
	Files    map[string]string `json:"files"`
	OldFiles map[string]string `json:"old-files"`
	Stage    string            `json:"stage"`
	Base     string            `json:"base"`
	Head     string            `json:"head,omitempty"`
}

type State struct {
	// Projection exists only in a read-only console copy. The authoritative
	// schema rejects it, so an excerpt cannot be saved as task state.
	Projection        *Projection       `json:"projection,omitempty"`
	Version           int               `json:"version"`
	Phase             string            `json:"phase"`
	Brief             Brief             `json:"brief"`
	Policy            string            `json:"policy"`
	InputRevision     int               `json:"input-revision"`
	MaxRounds         int               `json:"max-rounds"`
	Rounds            int               `json:"rounds"`
	Requests          []Request         `json:"requests"`
	Designs           []Design          `json:"designs"`
	Plans             []ExecutionPlan   `json:"plans"`
	Reviews           []Review          `json:"reviews"`
	Approvals         []Approval        `json:"approvals"`
	Feedback          []Feedback        `json:"feedback"`
	CurrentDesign     *Ref              `json:"current-design,omitempty"`
	CurrentPlan       *Ref              `json:"current-plan,omitempty"`
	CurrentPlans      map[string]Ref    `json:"current-plans"`
	Active            *Activation       `json:"active,omitempty"`
	PendingActivation *Activation       `json:"pending-activation,omitempty"`
	StartAuthorized   bool              `json:"start-authorized"`
	Reason            string            `json:"reason,omitempty"`
	LegacyFiles       map[string]string `json:"legacy-files,omitempty"`
}

type Projection struct {
	Kind             string         `json:"kind"`
	ApprovalComplete bool           `json:"approval-complete"`
	Ledger           string         `json:"ledger"`
	Omitted          map[string]int `json:"omitted"`
}
