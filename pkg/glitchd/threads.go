// threads.go is the public glue for the chat-threads + chat-first-ui
// openspec changes. It owns one chatui.ThreadStore + chatui.SlashRegistry
// per workspace and exposes the methods the desktop App (or any other
// frontend) needs to drive a threaded chat: dispatch a slash line, list
// main scrollback, list thread messages, spawn a drill thread, close a
// thread, list threads.
//
// The store is in-memory for v1 — persistence will land in a follow-up
// alongside the existing SaveMessage path. Per the project memory
// "no migrations pre-1.0," the v1 desktop UI is happy to lose threaded
// state on restart; users who want durability use `glitch ask` from the
// CLI today.
//
// This file lives in pkg/glitchd (not internal/) so the standalone
// `glitch-desktop` Go module can import it. The ThreadHost type wraps the
// internal chatui types and exposes only the operations the frontend
// actually calls, keeping the import surface narrow.
package glitchd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/8op-org/gl1tch/internal/chatui"
	"github.com/8op-org/gl1tch/internal/research"
)

// ThreadHost wraps the per-workspace chat state. One host per workspace
// ID; the host registry constructs hosts lazily on first access.
//
// cwd is the workspace's primary directory (the first enabled directory
// in the workspace config). It is forwarded into every research call as
// ResearchQuery.Context["cwd"], which the executor promotes to the
// GLITCH_CWD env var, which the canonical workflow files use to scope
// git/gh commands to the right repo. Without this, every thread runs
// against the desktop binary's own cwd — which is the gl1tch repo
// regardless of which workspace the user is looking at.
type ThreadHost struct {
	store        *chatui.ThreadStore
	registry     *chatui.SlashRegistry
	loop         *research.Loop
	feedbackSink research.EventSink
	workspaceID  string
	cwd          string
}

// ThreadHosts is the lazy registry of per-workspace hosts. Held in its
// own struct so the App's growing field set stays readable.
type ThreadHosts struct {
	mu    sync.Mutex
	hosts map[string]*ThreadHost
}

// NewThreadHosts constructs an empty registry.
func NewThreadHosts() *ThreadHosts {
	return &ThreadHosts{hosts: make(map[string]*ThreadHost)}
}

// AdoptDirectoryAsWorkspace constructs (or returns) a ThreadHost
// keyed on a synthetic workspaceID with the supplied directory as
// its primary cwd. Used by `glitch smoke pack` to drive the loop
// against any local checkout without requiring a real workspace
// row in the SQLite store.
//
// The ThreadHost is otherwise identical to one constructed via
// EnsureHost: same research loop, same brain event sink, same
// hints provider. Idempotent — calling twice for the same
// workspaceID returns the same host so per-fixture invocations
// share state.
func (h *ThreadHosts) AdoptDirectoryAsWorkspace(workspaceID, dir string) *ThreadHost {
	h.mu.Lock()
	if existing, ok := h.hosts[workspaceID]; ok {
		h.mu.Unlock()
		return existing
	}
	h.mu.Unlock()

	store := chatui.NewThreadStore()
	registry := chatui.NewSlashRegistry()
	_ = registry.Register(chatui.HelpHandler(registry))
	_ = registry.LoadAliases()

	mgr := BuildExecutorManager()
	researchReg, _ := research.DefaultRegistry(mgr, "")
	if researchReg == nil || len(researchReg.Names()) == 0 {
		// No researchers — produce a host with just /help so the
		// smoke runner can still report a clean failure.
		h.mu.Lock()
		defer h.mu.Unlock()
		host := &ThreadHost{store: store, registry: registry, workspaceID: workspaceID, cwd: dir}
		h.hosts[workspaceID] = host
		return host
	}

	llm := research.NewOllamaLLM(mgr, research.DefaultLocalModel)
	opts := research.DefaultScoreOptions()
	opts.SkipSelfConsistency = true
	sink := research.NewFileEventSink("")
	loop := research.NewLoop(researchReg, llm).
		WithEventSink(sink).
		WithHintsProvider(research.NewFileEventHintsProvider("")).
		WithScoreOptions(opts)
	_ = registry.Register(chatui.ResearchSlashHandler(loop))

	h.mu.Lock()
	defer h.mu.Unlock()
	host := &ThreadHost{
		store:        store,
		registry:     registry,
		loop:         loop,
		feedbackSink: sink,
		workspaceID:  workspaceID,
		cwd:          dir,
	}
	h.hosts[workspaceID] = host
	return host
}

// EnsureHost returns the host for workspaceID, constructing it on first
// access. The constructor wires the canonical slash commands (/help,
// /research) so a fresh workspace immediately has a useful menu.
//
// Construction depends on the research loop, which depends on the
// research default registry, which depends on the workspace having a
// .glitch/workflows directory. When the directory is missing the host
// still constructs successfully — the /research handler is simply not
// registered, and the frontend's /help output reflects that.
func (h *ThreadHosts) EnsureHost(workspaceID string) *ThreadHost {
	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, ok := h.hosts[workspaceID]; ok {
		return existing
	}
	store := chatui.NewThreadStore()
	registry := chatui.NewSlashRegistry()
	_ = registry.Register(chatui.HelpHandler(registry))
	// Load user aliases after the built-ins so the built-ins win
	// on name collisions. Errors are non-fatal — a missing or
	// malformed slash.yaml just leaves the registry with the
	// built-in handlers.
	_ = registry.LoadAliases()

	// Look up the workspace's primary directory so the loop's shell
	// steps run inside the right repo. workspaceCWD returns "" when
	// the workspace doesn't exist or has no directories — the loop
	// then falls back to the desktop binary's own cwd, which is fine
	// for the unconfigured-workspace case.
	cwd := ""
	if workspaceID != "" {
		if st, err := OpenStore(); err == nil {
			cwd = workspaceCWD(context.Background(), st, workspaceID)
		}
	}

	mgr := BuildExecutorManager()
	researchReg, _ := research.DefaultRegistry(mgr, "")
	if researchReg != nil && len(researchReg.Names()) > 0 {
		llm := research.NewOllamaLLM(mgr, research.DefaultLocalModel)
		opts := research.DefaultScoreOptions()
		// SkipSelfConsistency keeps assistant calls under a few
		// seconds in the desktop UI; the brain stats engine can
		// re-enable it later.
		opts.SkipSelfConsistency = true
		// One sink for both attempt/score writes (loop) and
		// feedback writes (frontend 👍/👎). Sharing the file means
		// the hints reader sees a single coherent timeline.
		sink := research.NewFileEventSink("")
		loop := research.NewLoop(researchReg, llm).
			WithEventSink(sink).
			WithHintsProvider(research.NewFileEventHintsProvider("")).
			WithScoreOptions(opts)
		_ = registry.Register(chatui.ResearchSlashHandler(loop))
		host := &ThreadHost{
			store:        store,
			registry:     registry,
			loop:         loop,
			feedbackSink: sink,
			workspaceID:  workspaceID,
			cwd:          cwd,
		}
		h.hosts[workspaceID] = host
		return host
	}

	host := &ThreadHost{store: store, registry: registry, workspaceID: workspaceID, cwd: cwd}
	h.hosts[workspaceID] = host
	return host
}

// DispatchSlash runs a slash command line through the workspace's slash
// dispatcher and appends the resulting messages to the store. Returns a
// JSON envelope ({ok:true,detail} or {ok:false,error}) the frontend can
// branch on. Lines that don't start with `/` are appended as a free-form
// user message and the dispatcher is bypassed.
func (h *ThreadHosts) DispatchSlash(workspaceID, line, scopeRaw string) string {
	host := h.EnsureHost(workspaceID)
	scope := chatui.SlashScope(scopeRaw)
	if scope == "" {
		scope = chatui.SlashScopeMain
	}

	// host.cwd flows into every research call below via
	// queryContext() → ResearchQuery.Context → PipelineResearcher's
	// step.Vars injection → executor.cli_adapter's `cmd.Dir = vars["cwd"]`.
	// No process-env mutation, no race; the cwd is per-call and
	// scoped to the workspace the user is looking at.

	line = strings.TrimSpace(line)
	if line == "" {
		return jsonEnvelopeError("empty input")
	}

	userMsg := chatui.ChatMessage{
		Role:      chatui.RoleUser,
		Type:      chatui.MessageTypeText,
		Payload:   chatui.TextPayload{Body: line},
		CreatedAt: time.Now(),
	}
	if scope.IsThreadScope() {
		userMsg.ThreadID = scope.ThreadID()
	}
	if _, err := host.store.Append(userMsg); err != nil {
		return jsonEnvelopeError(err.Error())
	}

	// DispatchSlash is slash-commands-only. Freeform text and /research
	// are handled by Execute → RunResearch, which streams results and
	// wires the user's provider choice. If freeform text arrives here
	// (shouldn't happen, but defensive), return an error.
	if !strings.HasPrefix(line, "/") {
		return jsonEnvelopeError("freeform text should route through Execute, not DispatchSlash")
	}

	// /research is also handled by Execute now. Redirect with a hint.
	if line == "/research" || strings.HasPrefix(line, "/research ") {
		return jsonEnvelopeError("/research is handled by the research loop — type your question directly")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	msgs, err := host.registry.Dispatch(ctx, line, scope)
	if err != nil {
		errMsg := chatui.ChatMessage{
			Role:      chatui.RoleAssistant,
			Type:      chatui.MessageTypeText,
			Payload:   chatui.TextPayload{Body: fmt.Sprintf("error: %v", err)},
			CreatedAt: time.Now(),
		}
		if scope.IsThreadScope() {
			errMsg.ThreadID = scope.ThreadID()
		}
		_, _ = host.store.Append(errMsg)
		return jsonEnvelopeError(err.Error())
	}

	for _, m := range msgs {
		if scope.IsThreadScope() {
			m.ThreadID = scope.ThreadID()
		}
		if _, err := host.store.Append(m); err != nil {
			return jsonEnvelopeError(err.Error())
		}
	}
	return jsonEnvelopeOK("dispatched")
}

// queryContext returns the workspace-scoped context map every research
// call gets. Carries:
//
//   "cwd"          — primary directory for shell-backed researchers
//                    (git-log, git-status, github-prs, github-issues
//                    execute inside this directory via GLITCH_CWD)
//   "workspace_id" — id of the workspace the call belongs to. The
//                    brain hints reader filters events by it so a
//                    👍 in one workspace doesn't bias hints in
//                    another, and emitted events are stamped so the
//                    history stays workspace-attributable.
//
// Both keys may be empty when the workspace has no directories yet
// or when the host is unscoped (rare; tests).
func (h *ThreadHost) queryContext() map[string]string {
	out := map[string]string{}
	if h.cwd != "" {
		out["cwd"] = h.cwd
	}
	if h.workspaceID != "" {
		out["workspace_id"] = h.workspaceID
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ExtractParentContext pulls a text summary from the parent ChatMessage so
// thread follow-ups carry the context that spawned the thread. Handles all
// payload types the attention feed / chat surface produces: widget_card
// (attention events), text (user questions), evidence bundles, etc.
func ExtractParentContext(msg chatui.ChatMessage) string {
	var b strings.Builder
	switch p := msg.Payload.(type) {
	case chatui.TextPayload:
		b.WriteString(p.Body)
	case chatui.WidgetCardPayload:
		if p.Title != "" {
			b.WriteString(p.Title)
		}
		if p.Subtitle != "" {
			b.WriteString("\n")
			b.WriteString(p.Subtitle)
		}
		for _, row := range p.Rows {
			fmt.Fprintf(&b, "\n%s: %s", row.Key, row.Value)
		}
	case chatui.EvidenceBundlePayload:
		for _, item := range p.Items {
			fmt.Fprintf(&b, "[%s] %s: %s\n", item.Source, item.Title, item.Body)
		}
	case map[string]any:
		// Payloads round-tripped through JSON unmarshal into
		// map[string]any. Extract what we can.
		if title, ok := p["title"].(string); ok {
			b.WriteString(title)
		}
		if subtitle, ok := p["subtitle"].(string); ok {
			b.WriteString("\n")
			b.WriteString(subtitle)
		}
		if body, ok := p["body"].(string); ok {
			b.WriteString(body)
		}
		if rows, ok := p["rows"].([]any); ok {
			for _, r := range rows {
				if row, ok := r.(map[string]any); ok {
					k, _ := row["key"].(string)
					v, _ := row["value"].(string)
					if k != "" || v != "" {
						fmt.Fprintf(&b, "\n%s: %s", k, v)
					}
				}
			}
		}
	}
	// Include metadata keys that carry structured context (repo, event
	// type, workspace) so the research loop can scope its search.
	if msg.Metadata != nil {
		for _, key := range []string{"repo", "event_type", "pr_number", "url"} {
			if v, ok := msg.Metadata[key]; ok && v != "" {
				fmt.Fprintf(&b, "\n%s: %s", key, v)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// MainScrollback returns the workspace's main-chat messages as JSON.
func (h *ThreadHosts) MainScrollback(workspaceID string) string {
	host := h.EnsureHost(workspaceID)
	return mustJSONString(host.store.MainScrollback())
}

// ThreadMessages returns the messages inside one thread as JSON.
func (h *ThreadHosts) ThreadMessages(workspaceID, threadID string) string {
	host := h.EnsureHost(workspaceID)
	return mustJSONString(host.store.ThreadMessages(threadID))
}

// ListThreads returns every thread in a workspace.
func (h *ThreadHosts) ListThreads(workspaceID string) string {
	host := h.EnsureHost(workspaceID)
	return mustJSONString(host.store.AllThreads())
}

// SpawnThreadOnMessage spawns (or returns the existing) thread under any
// chat message identified by parentMessageID. The parent does NOT have
// to exist in the chatui store — chat messages live in their own
// persistence (glitchd.SaveMessage), and we want every chat row to be
// threadable without first re-importing it. The thread is returned as
// JSON; the frontend opens its side pane on the returned ID.
func (h *ThreadHosts) SpawnThreadOnMessage(workspaceID, parentMessageID string) string {
	host := h.EnsureHost(workspaceID)
	thread, err := host.store.Spawn(parentMessageID, chatui.ExpandSidePane)
	if err != nil {
		return jsonEnvelopeError(err.Error())
	}
	return mustJSONString(thread)
}

// SpawnDrillThreadFromEvidence spawns a drill thread under the parent
// message and seeds it with the supplied evidence (encoded as the JSON
// shape of chatui.EvidenceBundleItem).
func (h *ThreadHosts) SpawnDrillThreadFromEvidence(workspaceID, parentMessageID, evidenceJSON string) string {
	host := h.EnsureHost(workspaceID)
	var item chatui.EvidenceBundleItem
	if err := json.Unmarshal([]byte(evidenceJSON), &item); err != nil {
		return jsonEnvelopeError("bad evidence json: " + err.Error())
	}
	thread, _, err := chatui.SpawnDrillThread(host.store, parentMessageID, item)
	if err != nil {
		return jsonEnvelopeError(err.Error())
	}
	return mustJSONString(thread)
}

// CloseThread freezes a thread and stamps a one-line summary on it.
func (h *ThreadHosts) CloseThread(workspaceID, threadID, summary string) string {
	host := h.EnsureHost(workspaceID)
	if err := host.store.Close(threadID, summary); err != nil {
		return jsonEnvelopeError(err.Error())
	}
	return jsonEnvelopeOK("closed")
}

// ReopenThread transitions a closed thread back to open.
func (h *ThreadHosts) ReopenThread(workspaceID, threadID string) string {
	host := h.EnsureHost(workspaceID)
	if err := host.store.Reopen(threadID); err != nil {
		return jsonEnvelopeError(err.Error())
	}
	return jsonEnvelopeOK("reopened")
}

// RecordResearchFeedback writes one EventTypeFeedback record tagged
// to the supplied thread. accepted=true is a thumbs-up; false is a
// thumbs-down. The brain hints reader weights this above any
// composite proxy: explicit accepts boost the picks; explicit
// rejects filter the picks out of future hints entirely.
//
// queryID and question are looked up from the thread's most recent
// assistant message metadata when available; the desktop frontend
// passes them in directly so we don't have to round-trip through
// store reads.
func (h *ThreadHosts) RecordResearchFeedback(workspaceID, threadID, queryID, question string, accepted bool) string {
	host := h.EnsureHost(workspaceID)
	if host.feedbackSink == nil {
		// No sink wired (only happens when the host was constructed
		// without a research loop because no workflows exist).
		return jsonEnvelopeError("no event sink available — research loop not configured")
	}
	// Stamp the feedback event with workspaceID so the brain hints
	// reader's workspace filter matches it. queryID is the loop's
	// research call id (the desktop frontend reads it from the
	// assistant message metadata stamped by ResearchResultToMessages).
	research.EmitFeedback(host.feedbackSink, queryID, workspaceID, question, accepted, nil)
	verdict := "👎"
	if accepted {
		verdict = "👍"
	}
	return jsonEnvelopeOK("recorded " + verdict + " for thread " + threadID)
}

// ResearchResult is the result of a RunResearch call, carrying everything
// the caller needs to stream the answer and open the thread.
type ResearchResult struct {
	Draft    string // the answer text to stream as chat:chunk
	ThreadID string // the thread containing full evidence detail
	ParentID string // the parent card in main scrollback (empty for follow-ups)
	Error    string // non-empty on failure
}

// RunResearch runs the research loop directly and stores the result in the
// thread system. For main-scope questions (threadID empty) it creates a
// parent card + thread. For thread follow-ups it appends to the existing
// thread. Provider/model, when non-empty, override the draft stage LLM.
//
// This is the canonical research execution path for Execute. It replaces
// the old DispatchSlash→runResearchAsParentThread indirection.
func (h *ThreadHosts) RunResearch(workspaceID, question, threadID, provider, model string) ResearchResult {
	host := h.EnsureHost(workspaceID)
	loop := host.loop
	if loop == nil {
		return ResearchResult{Error: "research not configured (no .glitch/workflows found)"}
	}

	// Wire user's provider into the draft stage. Intelligence ops
	// (plan, score, critique) stay on local Ollama.
	if provider != "" {
		mgr := BuildExecutorManager()
		draftLLM := research.NewProviderLLM(mgr, provider, model)
		loop = loop.WithDraftLLM(draftLLM)
	}

	// Build query context, enriched with parent context for follow-ups.
	qCtx := host.queryContext()
	if threadID != "" {
		if thread, ok := host.store.LookupByID(threadID); ok {
			if parent, ok := host.store.LookupMessage(thread.ParentMessageID); ok {
				parentText := ExtractParentContext(parent)
				if parentText != "" {
					if qCtx == nil {
						qCtx = make(map[string]string)
					}
					qCtx["thread_parent_context"] = parentText
					question = "Context from the parent message in this thread:\n" + parentText + "\n\nUser question: " + question
				}
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := loop.Run(ctx, research.ResearchQuery{
		Question: question,
		Context:  qCtx,
	}, research.DefaultBudget())
	if err != nil {
		return ResearchResult{Error: "research loop failed: " + err.Error()}
	}

	// Thread follow-up: append to existing thread.
	if threadID != "" {
		for _, m := range chatui.ResearchResultToMessages(result) {
			if m.Type == chatui.MessageTypeScoreCard {
				continue
			}
			m.ThreadID = threadID
			_, _ = host.store.Append(m)
		}
		return ResearchResult{Draft: result.Draft, ThreadID: threadID}
	}

	// Main scope: create parent card + thread.
	oneLine := strings.TrimSpace(result.Draft)
	oneLine = strings.ReplaceAll(oneLine, "\n", " ")
	if len(oneLine) > 140 {
		oneLine = oneLine[:137] + "…"
	}
	if oneLine == "" {
		oneLine = "(no answer)"
	}
	parentCard := chatui.ChatMessage{
		Role: chatui.RoleAssistant,
		Type: chatui.MessageTypeWidgetCard,
		Payload: chatui.WidgetCardPayload{
			Title:    "research: " + question,
			Subtitle: fmt.Sprintf("%d evidence source(s) · confidence %.2f · click to open thread", result.Bundle.Len(), result.Score.Composite),
			Rows: []chatui.WidgetRow{
				{Key: "answer", Value: oneLine},
			},
		},
		CreatedAt: time.Now(),
	}
	parent, err := host.store.Append(parentCard)
	if err != nil {
		return ResearchResult{Draft: result.Draft, Error: err.Error()}
	}
	thread, err := host.store.Spawn(parent.ID, chatui.ExpandSidePane)
	if err != nil {
		return ResearchResult{Draft: result.Draft, Error: err.Error()}
	}
	for _, m := range chatui.ResearchResultToMessages(result) {
		if m.Type == chatui.MessageTypeScoreCard {
			continue
		}
		m.ThreadID = thread.ID
		_, _ = host.store.Append(m)
	}
	return ResearchResult{Draft: result.Draft, ThreadID: thread.ID, ParentID: parent.ID}
}

// ── helpers ─────────────────────────────────────────────────────────────────

func jsonEnvelopeOK(detail string) string {
	b, _ := json.Marshal(map[string]any{"ok": true, "detail": detail})
	return string(b)
}

func jsonEnvelopeError(msg string) string {
	b, _ := json.Marshal(map[string]any{"ok": false, "error": msg})
	return string(b)
}

func mustJSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return jsonEnvelopeError(err.Error())
	}
	return string(b)
}
