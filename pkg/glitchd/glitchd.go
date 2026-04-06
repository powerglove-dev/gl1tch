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
func RunBackend(ctx context.Context) error {
	return bootstrap.RunHeadless(ctx)
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
func StreamAnswerScoped(ctx context.Context, question string, repos []string, tokenCh chan<- string) error {
	qe, err := QueryEngine()
	if err != nil {
		return err
	}
	return qe.StreamScoped(ctx, question, repos, tokenCh)
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
type PromptInfo struct {
	ID        int64  `json:"ID"`
	Title     string `json:"Title"`
	Body      string `json:"Body"`
	ModelSlug string `json:"ModelSlug"`
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
		out[i] = PromptInfo{ID: p.ID, Title: p.Title, Body: p.Body, ModelSlug: p.ModelSlug, UpdatedAt: p.UpdatedAt}
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

// ── Chat workflows ────────────────────────────────────────────────────────

// ChatWorkflowInfo is a saved chat workflow exported for the desktop app.
type ChatWorkflowInfo struct {
	ID          int64  `json:"ID"`
	WorkspaceID string `json:"WorkspaceID"`
	Name        string `json:"Name"`
	StepsJSON   string `json:"StepsJSON"`
	UpdatedAt   int64  `json:"UpdatedAt"`
}

// ListChatWorkflows returns all saved workflows for a workspace as JSON.
func ListChatWorkflows(ctx context.Context, workspaceID string) string {
	st, err := OpenStore()
	if err != nil {
		return "[]"
	}
	wfs, err := st.ListChatWorkflows(ctx, workspaceID)
	if err != nil || wfs == nil {
		return "[]"
	}
	out := make([]ChatWorkflowInfo, len(wfs))
	for i, w := range wfs {
		out[i] = ChatWorkflowInfo{
			ID: w.ID, WorkspaceID: w.WorkspaceID, Name: w.Name,
			StepsJSON: w.StepsJSON, UpdatedAt: w.UpdatedAt,
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// SaveChatWorkflow inserts a new workflow and returns it as JSON.
func SaveChatWorkflow(ctx context.Context, workspaceID, name, stepsJSON string) string {
	st, err := OpenStore()
	if err != nil {
		return "{}"
	}
	id, err := st.InsertChatWorkflow(ctx, store.ChatWorkflow{
		WorkspaceID: workspaceID, Name: name, StepsJSON: stepsJSON,
	})
	if err != nil {
		return "{}"
	}
	w, err := st.GetChatWorkflow(ctx, id)
	if err != nil {
		return "{}"
	}
	info := ChatWorkflowInfo{
		ID: w.ID, WorkspaceID: w.WorkspaceID, Name: w.Name,
		StepsJSON: w.StepsJSON, UpdatedAt: w.UpdatedAt,
	}
	b, _ := json.Marshal(info)
	return string(b)
}

// UpdateChatWorkflow modifies an existing workflow.
func UpdateChatWorkflow(ctx context.Context, id int64, name, stepsJSON string) {
	st, err := OpenStore()
	if err != nil {
		return
	}
	_ = st.UpdateChatWorkflow(ctx, id, name, stepsJSON)
}

// DeleteChatWorkflow removes a workflow by ID.
func DeleteChatWorkflow(ctx context.Context, id int64) {
	st, err := OpenStore()
	if err != nil {
		return
	}
	_ = st.DeleteChatWorkflow(ctx, id)
}

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
