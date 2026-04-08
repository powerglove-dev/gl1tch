package research

import "context"

// Researcher is anything the loop can ask for evidence.
//
// The interface is intentionally tiny: a stable Name() the planner can pick
// from, a natural-language Describe() the planner LLM sees in its prompt, and
// a Gather() that returns Evidence (or an error) for one query. Everything
// the loop does — capability adapters, pipeline-backed researchers, future
// plugin researchers — fits behind this surface.
//
// Implementations must be safe for concurrent calls: the gather stage runs
// independent researchers in parallel.
type Researcher interface {
	// Name is the kebab-case identifier the planner emits to ask for this
	// researcher's evidence. Must be unique within a registry.
	Name() string
	// Describe is the one-or-two-sentence description shown to the planner
	// LLM. It must answer "what kind of question would I pick this for?"
	// without exposing implementation details (no shell commands, no
	// internal package names).
	Describe() string
	// Gather runs one researcher invocation. The query carries the user's
	// question and any context the loop has accumulated; prior is the
	// evidence already gathered in this iteration so the researcher can
	// avoid duplicating work. Implementations should return the smallest
	// useful Evidence value — the loop's bundle is bounded.
	Gather(ctx context.Context, q ResearchQuery, prior EvidenceBundle) (Evidence, error)
}
