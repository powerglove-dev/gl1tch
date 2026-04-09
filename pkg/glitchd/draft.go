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
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/8op-org/gl1tch/internal/brainrag"
	"github.com/8op-org/gl1tch/internal/capability"
	"github.com/8op-org/gl1tch/internal/esearch"
	"github.com/8op-org/gl1tch/internal/store"
	"github.com/8op-org/gl1tch/pkg/glitchproto"
)

// DraftInfo is the wire shape exported to the desktop frontend. We
// re-export it here (instead of returning store.Draft directly) so the
// internal repository type can evolve without breaking the JS layer.
type DraftInfo struct {
	ID           int64             `json:"id"`
	WorkspaceID  string            `json:"workspace_id"`
	Kind         string            `json:"kind"`
	Title        string            `json:"title"`
	Body         string            `json:"body"`
	InputFormat  string            `json:"input_format"`
	OutputFormat string            `json:"output_format"`
	Turns        []store.DraftTurn `json:"turns"`
	TargetID     int64             `json:"target_id,omitempty"`
	TargetPath   string            `json:"target_path,omitempty"`
	CreatedAt    int64             `json:"created_at"`
	UpdatedAt    int64             `json:"updated_at"`
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
		if cfg, cerr := capability.LoadConfig(); cerr == nil && cfg.Model != "" {
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
	//
	// Output cleanup — why we filter:
	//   Provider plugins layer several sidecar protocols into their
	//   stdout: the `<<GLITCH_*>>` output-formatting markers, brain
	//   capture blocks (`<brain …>…</brain>`), end-of-run stats
	//   telemetry JSON (`{"type":"gl1tch-stats", …}`), GLITCH_WRITE
	//   acknowledgements (`[wrote: path]`), and leading `<>` arrow
	//   markers. The desktop chat strips all of these in its TS
	//   render path (see frontend/src/lib/parseAgentOutput.ts), but
	//   the draft editor streams tokens straight into CodeMirror,
	//   so without a filter the user sees every layer of
	//   scaffolding in the editor and gets it pasted into their
	//   saved prompt.
	//
	//   glitchproto.NewContentOnlyWriter is the "I just want the
	//   model's actual content, nothing else" primitive — it
	//   composes the GLITCH marker splitter with a line-buffered
	//   scrubber for the other four protocols. Any surface that
	//   wants clean provider output (workflow step capture, copy
	//   targets, future refinement surfaces) should use it so we
	//   don't end up with five copies of this stripping logic
	//   drifting out of sync.
	//
	// TODO(plain-output): the real fix is producer-side — plumb an
	// output-mode flag through StreamPromptOpts and the plugin
	// invocation boundary so plain-text callers can tell the
	// plugin NOT to inject OutputProtocolInstructions at all (and
	// to suppress stats/brain capture for that call). Until then
	// we strip on receive, which works but wastes tokens and
	// nudges the model toward an unnatural reply shape.
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

	// tokenChWriter adapts the caller's token channel to an
	// io.Writer so the cleaner's downstream sink can be an
	// io.MultiWriter that tees into both buf and the live channel.
	// Declared inline because it's tightly coupled to this
	// function's ctx + tokenCh — lifting it out would just add an
	// indirection for no gain.
	liveWriter := &tokenChWriter{ch: tokenCh, ctx: ctx}
	cleaner := glitchproto.NewContentOnlyWriter(io.MultiWriter(&buf, liveWriter))
	for chunk := range tee {
		// Write into the cleaner. If the caller has gone away
		// (ctx cancel), liveWriter drops silently and we keep
		// draining tee so the producer goroutine doesn't block
		// forever — buf still accumulates a persistable copy of
		// whatever content the stream produced before the cancel.
		_, _ = cleaner.Write([]byte(chunk))
	}
	_ = cleaner.Close()
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

// UpdateDraftBody persists local edits to a draft's title, body, and
// optional input/output format hints — all without running a refinement
// turn. This is what the editor popup calls when the user types
// directly into the CodeMirror surface rather than asking gl1tch to
// refine; manual edits need a path that doesn't go through the model.
//
// inputFormat and outputFormat are optional. Empty string means
// "free-form text" — the default for any prompt that hasn't opted into
// a structured shape. They're persisted to the draft row so the
// builder can carry the user's intent through to PromoteDraft, which
// in turn writes them onto the prompts table.
//
// Returns "" on success or an error message. The desktop popup uses
// this right before PromoteDraft so the freshly-typed text is what
// gets written to the target.
func UpdateDraftBody(ctx context.Context, draftID int64, title, body, inputFormat, outputFormat string) string {
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
	d.InputFormat = inputFormat
	d.OutputFormat = outputFormat
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
	case store.DraftKindSkill:
		return promoteSkillDraft(ctx, st, d)
	case store.DraftKindAgent:
		return promoteAgentDraft(ctx, st, d)
	case store.DraftKindCollectors:
		return promoteCollectorsDraft(ctx, d)
	default:
		return errorJSON(fmt.Errorf("promote: kind %q not yet supported", d.Kind))
	}
}

// promoteCollectorsDraft writes the draft body back to the workspace's
// collectors.yaml via WriteWorkspaceCollectorConfig, which validates
// the YAML before writing AND restarts the workspace's collector pod
// so the new config takes effect immediately.
//
// The target_path on the draft is informational only — the actual
// path is recomputed from the workspace id so a malformed draft
// can't redirect the write to somewhere outside the workspace's
// config dir.
func promoteCollectorsDraft(_ context.Context, d store.Draft) string {
	if strings.TrimSpace(d.WorkspaceID) == "" {
		return errorJSON(fmt.Errorf("collectors draft requires workspace_id"))
	}
	if err := WriteWorkspaceCollectorConfig(d.WorkspaceID, d.Body); err != nil {
		return errorJSON(err)
	}
	// Recompute the path so the response includes the canonical
	// location even if the draft's target_path was empty.
	path, _ := WorkspaceCollectorConfigPath(d.WorkspaceID)
	return promotedJSON(0, path)
}

// PromoteDraftAs is the "save as new" path. It detaches the draft
// from any existing target (clearing target_id and target_path),
// renames it to newName, and promotes. Always lands in the
// workspace — used when the user wants to fork a global entity into
// a workspace copy without overwriting the original.
//
// Returns the same {target_id, target_path} JSON as PromoteDraft.
func PromoteDraftAs(ctx context.Context, draftID int64, newName string) string {
	st, err := OpenStore()
	if err != nil {
		return errorJSON(err)
	}
	d, err := st.GetDraft(ctx, draftID)
	if err != nil {
		return errorJSON(err)
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return errorJSON(fmt.Errorf("save as: name is required"))
	}
	// Detach from the original target so the next promote treats this
	// as a brand-new entity. We persist the change before promoting
	// so the existing promote codepath sees the cleared fields.
	d.Title = newName
	d.TargetID = 0
	d.TargetPath = ""
	if err := st.UpdateDraft(ctx, d); err != nil {
		return errorJSON(err)
	}
	return PromoteDraft(ctx, draftID, false)
}

// promoteSkillDraft writes a skill draft to disk. Brand-new drafts
// (no target_path) land at <workspace>/.claude/skills/<title>/SKILL.md.
// Existing drafts overwrite their target_path, but only if it lives
// inside one of the workspace's directories — global skills are
// read-only and the popup is responsible for routing the user
// through PromoteDraftAs instead.
func promoteSkillDraft(ctx context.Context, st *store.Store, d store.Draft) string {
	if strings.TrimSpace(d.Body) == "" {
		return errorJSON(fmt.Errorf("skill draft body is empty"))
	}

	path := d.TargetPath
	if path == "" {
		title := strings.TrimSpace(d.Title)
		if title == "" {
			return errorJSON(fmt.Errorf("skill draft requires a title"))
		}
		path = SkillPathForName(ctx, d.WorkspaceID, title)
		if path == "" {
			return errorJSON(errWorkspaceHasNoDirs)
		}
	} else {
		// Existing draft → must be writable (i.e. inside the workspace).
		if !isWorkspaceWritablePath(ctx, st, d.WorkspaceID, path) {
			return errorJSON(fmt.Errorf("skill at %q is read-only — use save as new to fork", path))
		}
	}

	if err := writeSkillFile(path, d.Body); err != nil {
		return errorJSON(err)
	}
	if d.TargetPath != path {
		d.TargetPath = path
		_ = st.UpdateDraft(ctx, d)
	}
	return promotedJSON(0, path)
}

// promoteAgentDraft is the agent counterpart to promoteSkillDraft.
// New drafts land at <workspace>/.claude/commands/<title>.md;
// existing workspace drafts overwrite in place; global drafts are
// rejected (the popup must route through PromoteDraftAs).
func promoteAgentDraft(ctx context.Context, st *store.Store, d store.Draft) string {
	if strings.TrimSpace(d.Body) == "" {
		return errorJSON(fmt.Errorf("agent draft body is empty"))
	}

	path := d.TargetPath
	if path == "" {
		title := strings.TrimSpace(d.Title)
		if title == "" {
			return errorJSON(fmt.Errorf("agent draft requires a title"))
		}
		path = AgentPathForName(ctx, d.WorkspaceID, title)
		if path == "" {
			return errorJSON(errWorkspaceHasNoDirs)
		}
	} else {
		if !isWorkspaceWritablePath(ctx, st, d.WorkspaceID, path) {
			return errorJSON(fmt.Errorf("agent at %q is read-only — use save as new to fork", path))
		}
	}

	if err := writeAgentFile(path, d.Body); err != nil {
		return errorJSON(err)
	}
	if d.TargetPath != path {
		d.TargetPath = path
		_ = st.UpdateDraft(ctx, d)
	}
	return promotedJSON(0, path)
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
			ID:           d.TargetID,
			Title:        d.Title,
			Body:         d.Body,
			CWD:          cwd,
			InputFormat:  d.InputFormat,
			OutputFormat: d.OutputFormat,
		}); err != nil {
			return errorJSON(err)
		}
		return promotedJSON(d.TargetID, "")
	}
	id, err := st.InsertPrompt(ctx, store.Prompt{
		Title:        d.Title,
		Body:         d.Body,
		CWD:          cwd,
		InputFormat:  d.InputFormat,
		OutputFormat: d.OutputFormat,
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
	cfg, err := capability.LoadConfig()
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
		embedModel = "qwen2.5:7b"
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
	case store.DraftKindCollectors:
		b.WriteString("The draft is a gl1tch COLLECTORS YAML — it configures which sources (git, github, claude, copilot, directories) the workspace's collector pod indexes. Output must be valid YAML matching the existing schema. Preserve unknown sections rather than dropping them. Default state for any newly added section should be opt-in (enabled flags off, repo lists empty) unless the user explicitly asked to enable something.\n")
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

// workspaceCWD returns the workspace's primary directory — the one
// git/gh/shell tooling targets when the user runs `glitch ask` or
// drives a thread from the desktop. This is now an explicit field on
// the Workspace (set by AddWorkspaceDirectory on first insert,
// re-elected by SetWorkspacePrimaryDirectory) rather than the
// implicit "Directories[0] wins" pattern it used to be.
//
// Returns "" when the workspace has no directories or doesn't exist;
// callers fall back to the process cwd in that case.
func workspaceCWD(ctx context.Context, st *store.Store, workspaceID string) string {
	if workspaceID == "" {
		return ""
	}
	ws, err := st.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return ""
	}
	if ws.PrimaryDirectory != "" {
		return ws.PrimaryDirectory
	}
	// Defensive fallback for the (impossible-after-migration) case
	// where a workspace has rows but none flagged primary. The
	// migration backfills, AddWorkspaceDirectory enforces it, and
	// RemoveWorkspaceDirectory re-elects on demotion — but reading
	// Directories[0] here keeps the call site safe regardless.
	if len(ws.Directories) > 0 {
		return ws.Directories[0]
	}
	return ""
}

// tokenChWriter adapts a chan<-string (the refinement popup's live
// stream sink) to io.Writer so we can compose it behind an
// io.MultiWriter alongside the buffer that accumulates bytes for
// persistence. On ctx cancel it drops the chunk rather than
// blocking, so the producer goroutine can finish draining the
// upstream tee without deadlocking.
//
// We intentionally report len(p) back even when the send is
// dropped — the contract for NewTextOnlyWriter is that every byte
// is "consumed" once the splitter has accepted it, and surfacing a
// short-write here would confuse io.MultiWriter's bookkeeping with
// no upside (the caller has already canceled, the bytes aren't
// going anywhere useful anyway).
type tokenChWriter struct {
	ch  chan<- string
	ctx context.Context
}

func (w *tokenChWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	select {
	case w.ch <- string(p):
	case <-w.ctx.Done():
	}
	return len(p), nil
}

var _ io.Writer = (*tokenChWriter)(nil)

// toDraftInfo copies a store.Draft into the wire-friendly DraftInfo.
func toDraftInfo(d store.Draft) DraftInfo {
	turns := d.Turns
	if turns == nil {
		turns = []store.DraftTurn{}
	}
	return DraftInfo{
		ID:           d.ID,
		WorkspaceID:  d.WorkspaceID,
		Kind:         d.Kind,
		Title:        d.Title,
		Body:         d.Body,
		InputFormat:  d.InputFormat,
		OutputFormat: d.OutputFormat,
		Turns:        turns,
		TargetID:     d.TargetID,
		TargetPath:   d.TargetPath,
		CreatedAt:    d.CreatedAt,
		UpdatedAt:    d.UpdatedAt,
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
