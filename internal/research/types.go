package research

import (
	"encoding/json"
	"time"
)

// ResearchQuery is the input to one research call.
//
// Question is the natural-language ask. Context carries any caller-supplied
// scoping (workspace, file paths, prior conversation excerpt) the planner
// should consider when picking researchers and writing the draft. ID is a
// stable hash the loop emits in events so that scores, escalations, and
// follow-ups can be correlated to one logical call.
type ResearchQuery struct {
	ID       string            `json:"id"`
	Question string            `json:"question"`
	Context  map[string]string `json:"context,omitempty"`
}

// Evidence is one piece of information returned by a Researcher.
//
// Source is the researcher's Name(). Title is a short human-readable label
// the draft can cite. Body is the actual content (free text or a small
// JSON-marshallable payload). Refs are optional pointers (file paths, URLs,
// commit SHAs) the draft can include verbatim. Tags are free-form labels the
// scorer uses for cross-capability agreement.
//
// Evidence values are designed to round-trip through JSON so that
// PipelineResearcher can produce them from a YAML pipeline whose final step
// prints JSON, and so that the brain event store can persist a bundle for
// later inspection.
type Evidence struct {
	Source    string            `json:"source"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	Refs      []string          `json:"refs,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
}

// EvidenceBundle is the accumulated set of Evidence values gathered across
// one research call. Items are appended in the order they were produced; the
// scorer and renderer may sort or filter as needed.
//
// Bundles are bounded: callers may set MaxItems and MaxBytes on the loop
// budget to truncate runaway researchers. Truncated evidence is kept with
// Truncated=true on each affected item so the user can see what was clipped.
type EvidenceBundle struct {
	Items []Evidence `json:"items"`
}

// Add appends an Evidence value to the bundle. Nil-safe.
func (b *EvidenceBundle) Add(e Evidence) {
	if b == nil {
		return
	}
	b.Items = append(b.Items, e)
}

// Len returns the number of items currently in the bundle.
func (b *EvidenceBundle) Len() int {
	if b == nil {
		return 0
	}
	return len(b.Items)
}

// Sources returns the unique researcher names that contributed to the bundle,
// in the order they first appeared. Used by the scorer for cross-capability
// agreement.
func (b *EvidenceBundle) Sources() []string {
	if b == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(b.Items))
	out := make([]string, 0, len(b.Items))
	for _, it := range b.Items {
		if _, ok := seen[it.Source]; ok {
			continue
		}
		seen[it.Source] = struct{}{}
		out = append(out, it.Source)
	}
	return out
}

// Budget caps how much work one research call may do. The loop checks
// budgets before each stage and exits cleanly with Reason ReasonBudgetExceeded
// when any cap is hit. All zero values mean "unlimited" only when explicitly
// constructed via NoBudget(); the default constructor returns conservative
// caps so the loop cannot silently runaway.
type Budget struct {
	MaxIterations  int
	MaxWallclock   time.Duration
	MaxLocalTokens int
	MaxPaidTokens  int
}

// DefaultBudget returns the conservative defaults the loop uses when a caller
// does not supply its own. Five iterations, sixty seconds wallclock, fifty
// thousand local tokens, zero paid tokens (escalation off by default).
func DefaultBudget() Budget {
	return Budget{
		MaxIterations:  5,
		MaxWallclock:   60 * time.Second,
		MaxLocalTokens: 50_000,
		MaxPaidTokens:  0,
	}
}

// NoBudget returns a sentinel Budget with no caps. Tests use this to drive
// the loop deterministically; production code should always pass a real
// budget so that runaway calls are impossible.
func NoBudget() Budget {
	return Budget{
		MaxIterations:  1<<31 - 1,
		MaxWallclock:   24 * time.Hour,
		MaxLocalTokens: 1<<31 - 1,
		MaxPaidTokens:  1<<31 - 1,
	}
}

// Reason explains why the loop returned its Result. The renderer uses it to
// label evidence_bundle widgets and the brain audit uses it to bucket events.
type Reason string

const (
	// ReasonAccepted means the composite confidence cleared the threshold
	// and the draft is the loop's recommended answer.
	ReasonAccepted Reason = "accepted"
	// ReasonBudgetExceeded means the loop hit one of its caps before
	// reaching the threshold. The Draft on the Result is the highest-
	// confidence draft seen so far.
	ReasonBudgetExceeded Reason = "budget_exceeded"
	// ReasonEscalated means the loop could not clear the threshold locally
	// and the caller had escalation enabled, so the Result reflects the
	// paid verifier's verdict on the local draft.
	ReasonEscalated Reason = "escalated"
	// ReasonUnscored means the planner short-circuited (empty plan) and the
	// draft was returned without running gather/critique/score. The Score
	// field on the Result is zero-valued.
	ReasonUnscored Reason = "unscored"
)

// Result is the output of one research call.
//
// Draft is the answer text. Bundle is the evidence the loop gathered. Score
// is the per-signal breakdown plus the composite confidence. Reason explains
// the loop's exit condition. Iterations is the count of plan→score cycles
// actually run. ConfigHash is the hash of the brain/research-loop config in
// effect at the time of the call so the brain stats engine can attribute
// outcomes to a specific configuration without ambiguity.
type Result struct {
	Query      ResearchQuery  `json:"query"`
	Draft      string         `json:"draft"`
	Bundle     EvidenceBundle `json:"bundle"`
	Score      Score          `json:"score"`
	Reason     Reason         `json:"reason"`
	Iterations int            `json:"iterations"`
	ConfigHash string         `json:"config_hash,omitempty"`
}

// Score is the per-signal breakdown the loop produces during the score stage.
// Each Signal field is a pointer so we can distinguish "not computed for this
// iteration" (nil) from "computed and equal to zero" (non-nil zero), which
// matters when learning weights from logged events.
//
// Composite is the final scalar in [0,1]. The loop's accept/refine/escalate
// decision is made on Composite alone; the breakdown is for logging and
// rendering only.
type Score struct {
	Composite              float64  `json:"composite"`
	SelfConsistency        *float64 `json:"self_consistency,omitempty"`
	EvidenceCoverage       *float64 `json:"evidence_coverage,omitempty"`
	CrossCapabilityAgree   *float64 `json:"cross_capability_agreement,omitempty"`
	JudgeScore             *float64 `json:"judge_score,omitempty"`
}

// MarshalJSON makes Score round-trip cleanly through pipeline outputs and the
// brain event store. The custom marshaller is here so that future changes to
// the signal set do not silently break older logged events.
func (s Score) MarshalJSON() ([]byte, error) {
	type alias Score
	return json.Marshal(alias(s))
}

// Float returns p when non-nil, or the zero value otherwise. Used by the
// composite calculator to treat missing signals as 0 in the average without
// losing the "missing" bit in the per-signal log.
func Float(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// Ptr returns a pointer to v. Convenience for Score field assignments.
func Ptr(v float64) *float64 {
	return &v
}
