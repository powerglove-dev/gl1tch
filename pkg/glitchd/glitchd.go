// Package glitchd exposes gl1tch backend services for embedding in the
// desktop GUI. This is the public API boundary — the desktop app imports
// this instead of internal/ packages directly.
package glitchd

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/8op-org/gl1tch/internal/bootstrap"
	"github.com/8op-org/gl1tch/internal/collector"
	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/observer"
	"github.com/8op-org/gl1tch/internal/store"
)

// RunBackend starts all background services (busd, supervisor, collectors,
// brain, cron) as goroutines. Blocks until ctx is cancelled.
//
// Equivalent to RunBackendWithOptions(ctx, BackendOptions{}). Kept as
// the zero-arg name for the rare caller that wants the legacy headless
// behavior (every collector registered as a global supervisor service).
func RunBackend(ctx context.Context) error {
	return bootstrap.RunHeadless(ctx)
}

// BackendOptions configures RunBackendWithOptions. Mirrors the
// internal bootstrap.Options struct but lives in pkg/glitchd so the
// desktop app and other public callers don't have to import internal/.
type BackendOptions struct {
	// SkipGlobalCollectors disables the global supervisor's collector
	// services. Set to true when the caller is going to manage
	// collectors via the per-workspace pod manager (i.e. the desktop
	// app calls InitPodManager + StartAllWorkspacePods after this
	// returns). See the doc comment on bootstrap.Options for the full
	// rationale — short version: running both paths makes every
	// indexed doc end up with workspace_id="".
	SkipGlobalCollectors bool
}

// RunBackendWithOptions is the option-taking variant of RunBackend.
// The desktop app's app.go uses this with SkipGlobalCollectors:true.
func RunBackendWithOptions(ctx context.Context, opts BackendOptions) error {
	return bootstrap.RunHeadlessWithOptions(ctx, bootstrap.Options{
		SkipGlobalCollectors: opts.SkipGlobalCollectors,
	})
}

// QueryEngine creates an observer query engine connected to Elasticsearch.
func QueryEngine() (*observer.QueryEngine, error) {
	cfg, err := collector.LoadConfig()
	if err != nil {
		return nil, err
	}

	es, err := esearch.New(cfg.Elasticsearch.Address)
	if err != nil {
		return nil, err
	}

	if err := es.Ping(context.Background()); err != nil {
		return nil, err
	}

	return observer.NewQueryEngine(es, cfg.Model), nil
}

// StreamAnswer queries the observer and streams tokens to the channel.
func StreamAnswer(ctx context.Context, question string, tokenCh chan<- string) error {
	qe, err := QueryEngine()
	if err != nil {
		return err
	}
	return qe.Stream(ctx, question, tokenCh)
}

// StreamAnswerScoped queries the observer scoped to specific repo names.
// repos should be directory basenames (e.g. ["gl1tch", "ensemble"]).
//
// Equivalent to StreamAnswerScopedWorkspace with workspaceID="" —
// kept as a convenience wrapper for callers that don't yet pass a
// workspace id.
func StreamAnswerScoped(ctx context.Context, question string, repos []string, tokenCh chan<- string) error {
	return StreamAnswerScopedWorkspace(ctx, question, repos, "", tokenCh)
}

// StreamAnswerScopedWorkspace is the workspace-aware variant of
// StreamAnswerScoped. When workspaceID is non-empty, the underlying
// observer query filters on the workspace_id field so the answer
// only draws from documents that workspace's collector pod produced.
//
// Use this for the desktop chat answer path so workspace A's brain
// can never accidentally surface events from workspace B even if
// they share repo names.
func StreamAnswerScopedWorkspace(ctx context.Context, question string, repos []string, workspaceID string, tokenCh chan<- string) error {
	qe, err := QueryEngine()
	if err != nil {
		return err
	}
	return qe.StreamScopedWorkspace(ctx, question, repos, workspaceID, tokenCh)
}

// SaveMessage persists a workspace message via the store.
func SaveMessage(ctx context.Context, id, workspaceID, role, blocksJSON string, timestamp int64) error {
	st, err := OpenStore()
	if err != nil {
		return err
	}
	return st.SaveWorkspaceMessage(ctx, store.WorkspaceMessage{
		ID:          id,
		WorkspaceID: workspaceID,
		Role:        role,
		BlocksJSON:  blocksJSON,
		Timestamp:   timestamp,
	})
}

// ── Prompts ───────────────────────────────────────────────────────────────

// PromptInfo is a prompt exported for the desktop app.
//
// CWD is the workspace directory the prompt is scoped to (empty
// string when the prompt is global). The sidebar uses this to bucket
// prompts into workspace vs global views without an extra round trip.
type PromptInfo struct {
	ID        int64  `json:"ID"`
	Title     string `json:"Title"`
	Body      string `json:"Body"`
	ModelSlug string `json:"ModelSlug"`
	CWD       string `json:"CWD"`
	UpdatedAt int64  `json:"UpdatedAt"`
}

// ListAllPrompts returns all saved prompts as JSON.
func ListAllPrompts(ctx context.Context) string {
	st, err := OpenStore()
	if err != nil {
		return "[]"
	}
	prompts, err := st.ListPrompts(ctx)
	if err != nil || prompts == nil {
		return "[]"
	}
	out := make([]PromptInfo, len(prompts))
	for i, p := range prompts {
		out[i] = PromptInfo{
			ID:        p.ID,
			Title:     p.Title,
			Body:      p.Body,
			ModelSlug: p.ModelSlug,
			CWD:       p.CWD,
			UpdatedAt: p.UpdatedAt,
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// CreatePrompt saves a new prompt and returns it as JSON.
func CreatePrompt(ctx context.Context, title, body, modelSlug string) string {
	st, err := OpenStore()
	if err != nil {
		return "{}"
	}
	id, err := st.InsertPrompt(ctx, store.Prompt{Title: title, Body: body, ModelSlug: modelSlug})
	if err != nil {
		return "{}"
	}
	p, err := st.GetPrompt(ctx, id)
	if err != nil {
		return "{}"
	}
	info := PromptInfo{ID: p.ID, Title: p.Title, Body: p.Body, ModelSlug: p.ModelSlug, UpdatedAt: p.UpdatedAt}
	b, _ := json.Marshal(info)
	return string(b)
}

// DeletePromptByID removes a prompt by ID.
func DeletePromptByID(ctx context.Context, id int64) {
	st, err := OpenStore()
	if err != nil {
		return
	}
	_ = st.DeletePrompt(ctx, id)
}

// Chat workflows used to live in a SQLite table here. As of the YAML
// unification (Phase 3 of the editor-popup work) workflows are real
// .workflow.yaml files under <workspace>/.glitch/workflows/. The save
// path moved to SaveChainAsWorkflow (workflow_files.go) and the legacy
// rows are migrated to disk by MigrateChatWorkflowsToYAML on startup.

// ── Clarification ─────────────────────────────────────────────────────────

// AnswerClarification records the user's answer for a pending clarification.
func AnswerClarification(runID, answer string) {
	st, err := OpenStore()
	if err != nil {
		return
	}
	_ = st.AnswerClarification(runID, answer)
}

// ClarificationRequest is a pending clarification exported for the desktop app.
type ClarificationRequest struct {
	RunID    string `json:"run_id"`
	StepID   string `json:"step_id"`
	Question string `json:"question"`
}

// LoadPendingClarifications returns unanswered clarification requests.
func LoadPendingClarifications() ([]ClarificationRequest, error) {
	st, err := OpenStore()
	if err != nil {
		return nil, err
	}
	reqs, err := st.LoadPendingClarifications()
	if err != nil {
		return nil, err
	}
	out := make([]ClarificationRequest, len(reqs))
	for i, r := range reqs {
		out[i] = ClarificationRequest{RunID: r.RunID, StepID: r.StepID, Question: r.Question}
	}
	return out, nil
}

// ── Store singleton ────────────────────────────────────────────────────────

var (
	storeOnce     sync.Once
	storeInstance *store.Store
	storeErr      error
)

// OpenStore returns a singleton handle to the SQLite store.
func OpenStore() (*store.Store, error) {
	storeOnce.Do(func() {
		storeInstance, storeErr = store.Open()
	})
	return storeInstance, storeErr
}
