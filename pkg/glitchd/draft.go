// draft.go implements the Phase 1 prompt-refinement loop. A "draft" is
// a work-in-progress prompt / workflow / skill / agent that the user is
// iterating on with gl1tch as a collaborator. Each turn:
//
//  1. The user types an instruction ("make it more concise", "add a
//     section about retries").
//  2. The orchestrator queries brainrag for workspace context relevant
//     to the instruction (and the current draft body).
//  3. It builds a kind-aware system prompt and asks the chosen provider
//     for a *replacement* draft body.
//  4. The new body and the turn are persisted to SQLite so the
//     conversation survives restarts.
//
// The streaming wire shape mirrors StreamPrompt: a chan<-string of token
// chunks. Callers (the desktop Wails layer) reframe those chunks as
// "draft:chunk" / "draft:done" events keyed by draft id.
package glitchd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/brainrag"
	"github.com/8op-org/gl1tch/internal/collector"
	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/store"
)

// DraftInfo is the wire shape exported to the desktop frontend. We
// re-export it here (instead of returning store.Draft directly) so the
// internal repository type can evolve without breaking the JS layer.
type DraftInfo struct {
	ID          int64             `json:"id"`
	WorkspaceID string            `json:"workspace_id"`
	Kind        string            `json:"kind"`
	Title       string            `json:"title"`
	Body        string            `json:"body"`
	Turns       []store.DraftTurn `json:"turns"`
	TargetID    int64             `json:"target_id,omitempty"`
	TargetPath  string            `json:"target_path,omitempty"`
	CreatedAt   int64             `json:"created_at"`
	UpdatedAt   int64             `json:"updated_at"`
}

// CreateDraft inserts a new empty (or seeded) draft and returns it as
// JSON. Body and title are optional — leaving them empty produces a
// brand-new draft the user can start refining from scratch.
func CreateDraft(ctx context.Context, workspaceID, kind, title, body string) string {
	st, err := OpenStore()
	if err != nil {
		return errorJSON(fmt.Errorf("open store: %w", err))
	}
	id, err := st.CreateDraft(ctx, store.Draft{
		WorkspaceID: workspaceID,
		Kind:        kind,
		Title:       title,
		Body:        body,
	})
	if err != nil {
		return errorJSON(err)
	}
	d, err := st.GetDraft(ctx, id)
	if err != nil {
		return errorJSON(err)
	}
	return draftJSON(d)
}

// GetDraft returns a single draft as JSON. Returns "{}" on error so
// the frontend can detect missing rows without an exception path.
func GetDraft(ctx context.Context, id int64) string {
	st, err := OpenStore()
	if err != nil {
		return "{}"
	}
	d, err := st.GetDraft(ctx, id)
	if err != nil {
		return "{}"
	}
	return draftJSON(d)
}

// ListDrafts returns all drafts for the given workspace, optionally
// filtered to a single kind. Empty kind returns all kinds.
func ListDrafts(ctx context.Context, workspaceID, kind string) string {
	st, err := OpenStore()
	if err != nil {
		return "[]"
	}
	drafts, err := st.ListDraftsByWorkspace(ctx, workspaceID, kind)
	if err != nil || drafts == nil {
		return "[]"
	}
	out := make([]DraftInfo, len(drafts))
	for i, d := range drafts {
		out[i] = toDraftInfo(d)
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// DeleteDraft removes a draft by ID. Idempotent at the API level —
// we swallow the not-found error so the desktop can call this without
// a confirmation roundtrip.
func DeleteDraft(ctx context.Context, id int64) {
	st, err := OpenStore()
	if err != nil {
		return
	}
	_ = st.DeleteDraft(ctx, id)
}

// RefineDraftOpts holds the inputs for one refinement turn.
type RefineDraftOpts struct {
	DraftID    int64
	UserTurn   string // the user's instruction this turn
	ProviderID string // optional override; "" means use observer default ollama
	Model      string
}

// RefineDraft runs one turn of the refinement loop. It loads the draft,
// queries brainrag for workspace context relevant to the instruction,
// builds a kind-aware system prompt, calls the provider, streams the
// new draft body to tokenCh, and on completion persists the turn +
// updated body to SQLite.
//
// Streaming semantics: tokenCh receives partial chunks of the *new
// body*, not assistant chatter. The model is instructed to emit only
// the body so the popup can stream directly into the editor without
// needing a parser. The full accumulated body is also persisted via
// AppendDraftTurn so refresh-after-stream works.
//
// On any error before the first token, RefineDraft closes tokenCh and
// returns the error. On a mid-stream provider failure, the partial
// body is still persisted (so the user can keep iterating from the
// best-effort result) and the error is returned.
func RefineDraft(ctx context.Context, opts RefineDraftOpts, tokenCh chan<- string) error {
	st, err := OpenStore()
	if err != nil {
		close(tokenCh)
		return fmt.Errorf("open store: %w", err)
	}
	draft, err := st.GetDraft(ctx, opts.DraftID)
	if err != nil {
		close(tokenCh)
		return fmt.Errorf("load draft %d: %w", opts.DraftID, err)
	}

	// Resolve provider/model. Empty provider means "use the observer
	// default" — for Phase 1 that's Ollama with the model the user has
	// set in observer.yaml. The desktop layer can pass through whatever
	// override the user picked in the popup picker.
	provider := opts.ProviderID
	model := opts.Model
	if provider == "" {
		provider = "ollama"
	}
	if provider == "ollama" && model == "" {
		if cfg, cerr := collector.LoadConfig(); cerr == nil && cfg.Model != "" {
			model = cfg.Model
		}
	}

	// Pull workspace context from brainrag. Best effort — if ES is down
	// or the embedder fails the loop still works, we just refine without
	// brain context. We never want a brain hiccup to block the user.
	brainContext := loadBrainContext(ctx, draft.WorkspaceID, opts.UserTurn, draft.Body)

	// Build the system prompt and the user-side instruction. Kept
	// separate so future kinds can override the system prompt without
	// reformatting the user turn.
	system := buildDraftSystemPrompt(draft.Kind, draft.Body, brainContext)
	userPrompt := opts.UserTurn

	// Stream into a buffer that we *also* tee to the caller's channel.
	// We need the full text on completion to persist the new body.
	var buf strings.Builder
	tee := make(chan string, 16)
	doneCh := make(chan error, 1)

	go func() {
		err := StreamPrompt(ctx, StreamPromptOpts{
			ProviderID: provider,
			Model:      model,
			Prompt:     userPrompt,
			SystemCtx:  system,
		}, tee)
		doneCh <- err
	}()

	for chunk := range tee {
		buf.WriteString(chunk)
		// Forward to caller. If the caller has gone away (ctx cancel),
		// drop the chunk and keep draining tee so the producer goroutine
		// doesn't block forever.
		select {
		case tokenCh <- chunk:
		case <-ctx.Done():
		}
	}
	streamErr := <-doneCh
	close(tokenCh)

	newBody := strings.TrimSpace(buf.String())
	// Persist whatever we got, even on partial failure — the user can
	// pick up from there next turn.
	if newBody != "" {
		turn := store.DraftTurn{
			Role:      "user",
			Text:      opts.UserTurn,
			Body:      newBody,
			Provider:  provider,
			Model:     model,
			Timestamp: time.Now().Unix(),
		}
		if perr := st.AppendDraftTurn(ctx, draft.ID, turn, newBody); perr != nil {
			// Don't mask the streaming error if there is one.
			if streamErr == nil {
				return fmt.Errorf("persist refinement: %w", perr)
			}
		}
	}
	return streamErr
}

// UpdateDraftBody persists local edits to a draft's title and body
// without running a refinement turn. This is what the editor popup
// calls when the user types directly into the CodeMirror surface
// rather than asking gl1tch to refine — manual edits need a path
// that doesn't go through the model.
//
// Returns "" on success or an error message. The desktop popup uses
// this right before PromoteDraft so the freshly-typed text is what
// gets written to the target.
func UpdateDraftBody(ctx context.Context, draftID int64, title, body string) string {
	st, err := OpenStore()
	if err != nil {
		return err.Error()
	}
	d, err := st.GetDraft(ctx, draftID)
	if err != nil {
		return err.Error()
	}
	d.Title = title
	d.Body = body
	if err := st.UpdateDraft(ctx, d); err != nil {
		return err.Error()
	}
	return ""
}

// PromoteDraft writes a draft's current body to its real target. For
// kind=prompt this means inserting (or updating) a prompts row. For
// kind=workflow it means writing the YAML body to target_path on disk
// (under <workspace>/.glitch/workflows/<title>.workflow.yaml when
// target_path is unset, i.e. brand-new draft).
//
// Returns a JSON {target_id, target_path} on success or {error: ...}
// on failure. The desktop popup uses this on its save button.
//
// makeGlobal only matters for kind=prompt — file-backed entities are
// always workspace-scoped (per the "everything is workspace-scoped
// unless noted" rule), so makeGlobal is silently ignored for them.
func PromoteDraft(ctx context.Context, draftID int64, makeGlobal bool) string {
	st, err := OpenStore()
	if err != nil {
		return errorJSON(err)
	}
	d, err := st.GetDraft(ctx, draftID)
	if err != nil {
		return errorJSON(err)
	}

	switch d.Kind {
	case store.DraftKindPrompt:
		return promotePromptDraft(ctx, st, d, makeGlobal)
	case store.DraftKindWorkflow:
		return promoteWorkflowDraft(ctx, st, d)
	default:
		return errorJSON(fmt.Errorf("promote: kind %q not yet supported", d.Kind))
	}
}

// promotePromptDraft handles kind=prompt promotion. CWD column on
// prompts doubles as the scope key: empty cwd = global, workspace cwd
// = project. Update-in-place when target_id is set, insert otherwise
// and backfill target_id so subsequent promotes update.
func promotePromptDraft(ctx context.Context, st *store.Store, d store.Draft, makeGlobal bool) string {
	cwd := ""
	if !makeGlobal {
		cwd = workspaceCWD(ctx, st, d.WorkspaceID)
	}

	if d.TargetID > 0 {
		if err := st.UpdatePrompt(ctx, store.Prompt{
			ID:    d.TargetID,
			Title: d.Title,
			Body:  d.Body,
			CWD:   cwd,
		}); err != nil {
			return errorJSON(err)
		}
		return promotedJSON(d.TargetID, "")
	}
	id, err := st.InsertPrompt(ctx, store.Prompt{
		Title: d.Title,
		Body:  d.Body,
		CWD:   cwd,
	})
	if err != nil {
		return errorJSON(err)
	}
	d.TargetID = id
	_ = st.UpdateDraft(ctx, d)
	return promotedJSON(id, "")
}

// promoteWorkflowDraft writes the draft body to disk. When the draft
// already has a target_path (it was opened from an existing file via
// CreateDraftFromTarget) we overwrite that file in place. When it
// doesn't (brand-new draft) we compute the path from the workspace's
// primary directory and the draft's title — same convention as the
// chain-bar save path so manually edited workflows land in the same
// .glitch/workflows tree as chain-bar workflows.
//
// On a brand-new draft, target_path is backfilled so the next promote
// updates in place instead of asking for a name again.
func promoteWorkflowDraft(ctx context.Context, st *store.Store, d store.Draft) string {
	if strings.TrimSpace(d.Body) == "" {
		return errorJSON(fmt.Errorf("workflow draft body is empty"))
	}

	path := d.TargetPath
	if path == "" {
		// Brand-new draft → derive a path from the title. We require
		// a title for new workflows because the filename has to come
		// from somewhere; the popup is responsible for surfacing the
		// "name your workflow" UI before letting the user hit save.
		title := strings.TrimSpace(d.Title)
		if title == "" {
			return errorJSON(fmt.Errorf("workflow draft requires a title"))
		}
		dir := primaryWorkspaceDir(ctx, st, d.WorkspaceID)
		if dir == "" {
			return errorJSON(errWorkspaceHasNoDirs)
		}
		// Reuse the same directory + filename convention as the chain
		// bar's save path so the two surfaces stay symmetric.
		title = strings.TrimSuffix(title, ".workflow.yaml")
		written, werr := SaveWorkflow(dir, title, d.Body)
		if werr != nil {
			return errorJSON(werr)
		}
		path = written
	} else {
		// Existing draft → overwrite the file at its declared path.
		// validateWorkflowPath enforces the same .glitch/workflows
		// safety gate as ReadWorkflowFile / WriteWorkflowFile.
		if errMsg := validateWorkflowPath(path); errMsg != "" {
			return errorJSON(fmt.Errorf("%s", errMsg))
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return errorJSON(err)
		}
		if err := os.WriteFile(path, []byte(d.Body), 0o644); err != nil {
			return errorJSON(err)
		}
	}

	// Backfill target_path so a second promote updates in place.
	if d.TargetPath != path {
		d.TargetPath = path
		_ = st.UpdateDraft(ctx, d)
	}
	return promotedJSON(0, path)
}

// loadBrainContext queries brainrag for workspace context relevant to
// the user's instruction. Returns "" on any failure — brain assistance
// is best-effort, never required for the loop to make progress.
//
// Strategy: combine the user's instruction and a head of the current
// draft body into a single query string so the kNN search lands on
// material that's relevant both to *what the user wants* and *what
// they're already drafting*.
func loadBrainContext(ctx context.Context, workspaceID, userTurn, currentBody string) string {
	cfg, err := collector.LoadConfig()
	if err != nil || cfg.Elasticsearch.Address == "" {
		return ""
	}
	es, err := esearch.New(cfg.Elasticsearch.Address)
	if err != nil {
		return ""
	}
	// Cheap reachability probe — avoids a multi-second timeout when ES
	// is down on a fresh install.
	pingCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	if err := es.Ping(pingCtx); err != nil {
		return ""
	}

	// Use the workspace id as the scope. brainrag.NewRAGStore takes a
	// raw scope string; we prefix with "workspace:" to match the
	// convention documented in NewRAGStore's doc comment.
	rs := brainrag.NewRAGStore(es, "workspace:"+workspaceID)

	// Default to the same Ollama embedder used elsewhere. Empty base
	// URL lets the embedder pick its own default.
	embedModel := cfg.Model
	if embedModel == "" {
		embedModel = "llama3.2"
	}
	embedder := brainrag.NewOllamaEmbedder("", embedModel)

	q := userTurn
	if head := strings.TrimSpace(currentBody); head != "" {
		// Cap the body slice we feed into the query so a giant draft
		// doesn't drown out the user's instruction.
		if len(head) > 600 {
			head = head[:600]
		}
		q = userTurn + "\n\n" + head
	}

	hits, err := rs.QueryWithText(ctx, embedder, q, 5)
	if err != nil || len(hits) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Relevant context from this workspace's brain:\n")
	for i, h := range hits {
		text := strings.TrimSpace(h.Text)
		if text == "" {
			continue
		}
		// Trim each hit so the system prompt stays bounded even with
		// large embedded chunks.
		if len(text) > 800 {
			text = text[:800] + "…"
		}
		fmt.Fprintf(&b, "[%d] %s\n", i+1, text)
	}
	return b.String()
}

// buildDraftSystemPrompt returns the system prompt sent to the
// underlying provider for a refinement turn. The prompt is intentionally
// strict about output format: the model must reply with *only* the new
// draft body, no preface or commentary, so the popup can stream tokens
// straight into the editor.
//
// Per-kind sections vary the framing (prompt vs workflow vs skill etc.)
// while sharing the same output contract.
func buildDraftSystemPrompt(kind, currentBody, brainContext string) string {
	var b strings.Builder
	b.WriteString("You are gl1tch's prompt designer. The user is iterating on a draft and your job is to return an improved version of it.\n\n")

	switch kind {
	case store.DraftKindPrompt:
		b.WriteString("The draft is an LLM PROMPT — instructions a future model will be given. Make it precise, structured, and free of filler. Use Markdown headings and lists when they help clarity.\n")
	case store.DraftKindWorkflow:
		b.WriteString("The draft is a gl1tch WORKFLOW YAML. Output must be valid YAML that conforms to the gl1tch workflow schema (top-level `name`, `version`, `steps`). Do not invent step types — only use ones the user already has.\n")
	case store.DraftKindSkill:
		b.WriteString("The draft is a SKILL definition (Markdown with frontmatter). Preserve the frontmatter format and keep the body actionable.\n")
	case store.DraftKindAgent:
		b.WriteString("The draft is an AGENT definition (Markdown). Keep persona and capabilities crisp; avoid generic AI boilerplate.\n")
	default:
		b.WriteString("The draft is a free-form text artifact. Improve it without changing its essential intent.\n")
	}

	b.WriteString("\nOUTPUT CONTRACT:\n")
	b.WriteString("- Reply with ONLY the new draft body.\n")
	b.WriteString("- No preface, no explanation, no code fences around the whole reply.\n")
	b.WriteString("- If the user's instruction is ambiguous, make the most reasonable improvement and proceed — do not ask questions.\n")

	if brainContext != "" {
		b.WriteString("\n")
		b.WriteString(brainContext)
	}

	b.WriteString("\nCURRENT DRAFT:\n")
	if strings.TrimSpace(currentBody) == "" {
		b.WriteString("(empty — produce an initial draft from the user's instruction)\n")
	} else {
		b.WriteString(currentBody)
		b.WriteString("\n")
	}
	return b.String()
}

// workspaceCWD returns the first directory associated with a workspace,
// which doubles as the workspace's "primary cwd" for prompt scoping.
// Returns "" if the workspace has no directories or doesn't exist.
func workspaceCWD(ctx context.Context, st *store.Store, workspaceID string) string {
	if workspaceID == "" {
		return ""
	}
	ws, err := st.GetWorkspace(ctx, workspaceID)
	if err != nil || len(ws.Directories) == 0 {
		return ""
	}
	return ws.Directories[0]
}

// toDraftInfo copies a store.Draft into the wire-friendly DraftInfo.
func toDraftInfo(d store.Draft) DraftInfo {
	turns := d.Turns
	if turns == nil {
		turns = []store.DraftTurn{}
	}
	return DraftInfo{
		ID:          d.ID,
		WorkspaceID: d.WorkspaceID,
		Kind:        d.Kind,
		Title:       d.Title,
		Body:        d.Body,
		Turns:       turns,
		TargetID:    d.TargetID,
		TargetPath:  d.TargetPath,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}

func draftJSON(d store.Draft) string {
	b, _ := json.Marshal(toDraftInfo(d))
	return string(b)
}

func promotedJSON(id int64, path string) string {
	b, _ := json.Marshal(map[string]any{"target_id": id, "target_path": path})
	return string(b)
}

func errorJSON(err error) string {
	b, _ := json.Marshal(map[string]string{"error": err.Error()})
	return string(b)
}
