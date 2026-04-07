package esearch

import "time"

// Event is the universal observation document indexed into glitch-events.
//
// WorkspaceID is set by the collector pod that produced the event so
// brain queries can filter by workspace. Empty WorkspaceID is allowed
// and means "global / unattributed" — used for events from collectors
// running outside any workspace pod (e.g. legacy collectors before the
// per-workspace split, or always-on global watchers).
type Event struct {
	Type         string         `json:"type"`          // e.g. "git.commit", "git.push", "pr.opened"
	Source       string         `json:"source"`         // collector name: "git", "github", "pipeline"
	WorkspaceID  string         `json:"workspace_id,omitempty"`
	Repo         string         `json:"repo,omitempty"`
	Branch       string         `json:"branch,omitempty"`
	Author       string         `json:"author,omitempty"`
	SHA          string         `json:"sha,omitempty"`
	Message      string         `json:"message,omitempty"`
	Body         string         `json:"body,omitempty"`
	FilesChanged []string       `json:"files_changed,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Timestamp    time.Time      `json:"timestamp"`
}

// PipelineRun is indexed into glitch-pipelines when a pipeline completes.
//
// WorkspaceID identifies the workspace whose collector pod observed
// the run. Same semantics as Event.WorkspaceID — empty means global.
type PipelineRun struct {
	Name        string         `json:"name"`
	Status      string         `json:"status"` // "success", "failure"
	WorkspaceID string         `json:"workspace_id,omitempty"`
	ExitCode    int            `json:"exit_code"`
	Steps       map[string]any `json:"steps,omitempty"`
	Stdout      string         `json:"stdout,omitempty"`
	Stderr      string         `json:"stderr,omitempty"`
	DurationMs  int64          `json:"duration_ms"`
	Model       string         `json:"model,omitempty"`
	Provider    string         `json:"provider,omitempty"`
	TokensIn    int64          `json:"tokens_in"`
	TokensOut   int64          `json:"tokens_out"`
	CostUSD     float64        `json:"cost_usd"`
	Timestamp   time.Time      `json:"timestamp"`
}

// Summary is an LLM-generated digest indexed into glitch-summaries.
type Summary struct {
	Scope        string    `json:"scope"` // "daily", "weekly"
	Date         string    `json:"date"`  // YYYY-MM-DD
	SummaryText  string    `json:"summary"`
	KeyDecisions []string  `json:"key_decisions,omitempty"`
	Repos        []string  `json:"repos,omitempty"`
	GeneratedBy  string    `json:"generated_by"`
	Timestamp    time.Time `json:"timestamp"`
}

// Insight is a pattern or anomaly detected by the reasoning engine.
type Insight struct {
	Type           string    `json:"type"` // "pattern", "anomaly", "recommendation"
	Pattern        string    `json:"pattern"`
	Confidence     float64   `json:"confidence"`
	EvidenceCount  int       `json:"evidence_count"`
	Evidence       []string  `json:"evidence,omitempty"`
	Recommendation string    `json:"recommendation,omitempty"`
	Repos          []string  `json:"repos,omitempty"`
	FirstSeen      time.Time `json:"first_seen"`
	LastSeen       time.Time `json:"last_seen"`
	Timestamp      time.Time `json:"timestamp"`
}
