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
	store    *chatui.ThreadStore
	registry *chatui.SlashRegistry
	loop     *research.Loop
	cwd      string
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
		loop := research.NewLoop(researchReg, llm).
			WithEventSink(research.NewFileEventSink("")).
			WithHintsProvider(research.NewFileEventHintsProvider("")).
			WithScoreOptions(opts)
		_ = registry.Register(chatui.ResearchSlashHandler(loop))
		host := &ThreadHost{store: store, registry: registry, loop: loop, cwd: cwd}
		h.hosts[workspaceID] = host
		return host
	}

	host := &ThreadHost{store: store, registry: registry, cwd: cwd}
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

	// Threads-first routing rules:
	//
	//   - In the main chat scope, freeform text and /research both
	//     produce a Slack-style parent summary card + an auto-spawned
	//     thread containing the full detail. There is no "legacy
	//     freeform → no reply" path; every question becomes a thread.
	//
	//   - In a thread scope, freeform text and /research run the same
	//     loop but the result is appended to the existing thread
	//     instead of spawning a new one. Follow-ups in a thread stay
	//     in that thread.
	//
	//   - All other /commands route through the slash registry as
	//     plain widgets (one-shot, no thread). /help, /status, etc.
	//     stay flat — chat-threads says only multi-step interactions
	//     become threads.
	if scope == chatui.SlashScopeMain {
		isResearch := line == "/research" || strings.HasPrefix(line, "/research ")
		isFreeform := !strings.HasPrefix(line, "/")
		if isResearch || isFreeform {
			question := line
			if isResearch {
				question = strings.TrimSpace(strings.TrimPrefix(line, "/research"))
			}
			// The user's typed line was already appended above as a
			// user message; remove it to avoid double-rendering,
			// since the parent card already echoes the question.
			host.store.RemoveLastMain()
			return host.runResearchAsParentThread(question)
		}
	} else if scope.IsThreadScope() {
		isResearch := line == "/research" || strings.HasPrefix(line, "/research ")
		isFreeform := !strings.HasPrefix(line, "/")
		if isResearch || isFreeform {
			question := line
			if isResearch {
				question = strings.TrimSpace(strings.TrimPrefix(line, "/research"))
			}
			return host.runResearchInExistingThread(scope.ThreadID(), question)
		}
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
// call gets, so the loop's shell-backed researchers (git-log,
// git-status, github-prs, github-issues) execute in the right repo. The
// canonical key is "cwd" — workflows read it as $GLITCH_CWD via the
// executor's GLITCH_<KEY> env-var promotion. Other context fields can
// be added here in the future without touching every researcher.
func (h *ThreadHost) queryContext() map[string]string {
	if h.cwd == "" {
		return nil
	}
	return map[string]string{"cwd": h.cwd}
}

// runResearchAsParentThread runs the research loop and emits one parent
// summary widget_card in main + an auto-spawned thread carrying the full
// detail (draft text, evidence bundle). The Slack-style UX hangs off this
// shape: the main scrollback shows the question + a one-line answer + a
// "X replies" link, and clicking the link opens the side pane with the
// thread messages.
//
// The function returns the JSON envelope DispatchSlash returns to the
// frontend; on success, the envelope's `detail` field is the spawned
// thread's ID so the frontend can immediately auto-open the side pane.
func (h *ThreadHost) runResearchAsParentThread(question string) string {
	if h.loop == nil {
		// No researchers configured. Surface an honest error in main.
		_, _ = h.store.Append(chatui.ChatMessage{
			Role:      chatui.RoleAssistant,
			Type:      chatui.MessageTypeText,
			Payload:   chatui.TextPayload{Body: "research is not configured for this workspace (no .glitch/workflows directory found)"},
			CreatedAt: time.Now(),
		})
		return jsonEnvelopeError("research not configured")
	}
	question = strings.TrimSpace(question)
	if question == "" {
		_, _ = h.store.Append(chatui.ChatMessage{
			Role:      chatui.RoleAssistant,
			Type:      chatui.MessageTypeText,
			Payload:   chatui.TextPayload{Body: "Usage: /research <question>"},
			CreatedAt: time.Now(),
		})
		return jsonEnvelopeError("missing question")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := h.loop.Run(ctx, research.ResearchQuery{
		Question: question,
		Context:  h.queryContext(),
	}, research.DefaultBudget())
	if err != nil {
		errMsg := chatui.ChatMessage{
			Role:      chatui.RoleAssistant,
			Type:      chatui.MessageTypeText,
			Payload:   chatui.TextPayload{Body: fmt.Sprintf("research loop failed: %v", err)},
			CreatedAt: time.Now(),
		}
		_, _ = h.store.Append(errMsg)
		return jsonEnvelopeError(err.Error())
	}

	// Build the one-line summary the parent card shows. We strip
	// newlines and clamp to ~140 chars so the card stays the same
	// height regardless of how chatty the model was. Full draft lives
	// in the spawned thread for users who want the long form.
	oneLine := strings.TrimSpace(result.Draft)
	oneLine = strings.ReplaceAll(oneLine, "\n", " ")
	if len(oneLine) > 140 {
		oneLine = oneLine[:137] + "…"
	}
	if oneLine == "" {
		oneLine = "(no answer)"
	}

	// Parent: a widget_card the renderer can show as one row in main.
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
	parent, err := h.store.Append(parentCard)
	if err != nil {
		return jsonEnvelopeError(err.Error())
	}

	// Spawn the thread under that parent and append the detail
	// messages (text answer + evidence bundle). The bundle items are
	// drillable from inside the thread; we don't render the score
	// card as a separate message because the bundle header already
	// shows the composite confidence.
	thread, err := h.store.Spawn(parent.ID, chatui.ExpandSidePane)
	if err != nil {
		return jsonEnvelopeError(err.Error())
	}

	detail := chatui.ResearchResultToMessages(result)
	for _, m := range detail {
		// Drop the score_card from the thread — it's redundant with
		// the bundle header which already carries the composite.
		if m.Type == chatui.MessageTypeScoreCard {
			continue
		}
		m.ThreadID = thread.ID
		if _, err := h.store.Append(m); err != nil {
			return jsonEnvelopeError(err.Error())
		}
	}
	// Return the thread ID in the envelope so the frontend can open
	// the side pane immediately without an extra ListThreads call.
	b, _ := json.Marshal(map[string]any{
		"ok":        true,
		"thread_id": thread.ID,
		"parent_id": parent.ID,
	})
	return string(b)
}

// runResearchInExistingThread runs the loop and appends the resulting
// detail messages to the existing thread. Used for follow-up questions
// inside an open thread — every follow-up stays in the same thread
// instead of spawning a new top-level summary card.
//
// The user's question is also appended (as a user role message) to the
// thread before the loop runs so the side-pane scrollback shows the
// turn-by-turn conversation.
func (h *ThreadHost) runResearchInExistingThread(threadID, question string) string {
	if h.loop == nil {
		return jsonEnvelopeError("research not configured")
	}
	question = strings.TrimSpace(question)
	if question == "" {
		return jsonEnvelopeError("missing question")
	}
	// User turn (the line we removed from main needs to land in the
	// thread instead — DispatchSlash already appended it to the
	// thread before delegating here, so we don't double-append).

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := h.loop.Run(ctx, research.ResearchQuery{
		Question: question,
		Context:  h.queryContext(),
	}, research.DefaultBudget())
	if err != nil {
		_, _ = h.store.Append(chatui.ChatMessage{
			ThreadID:  threadID,
			Role:      chatui.RoleAssistant,
			Type:      chatui.MessageTypeText,
			Payload:   chatui.TextPayload{Body: fmt.Sprintf("research loop failed: %v", err)},
			CreatedAt: time.Now(),
		})
		return jsonEnvelopeError(err.Error())
	}

	for _, m := range chatui.ResearchResultToMessages(result) {
		// Drop the score_card from the thread — the bundle header
		// already shows the composite confidence.
		if m.Type == chatui.MessageTypeScoreCard {
			continue
		}
		m.ThreadID = threadID
		if _, err := h.store.Append(m); err != nil {
			return jsonEnvelopeError(err.Error())
		}
	}
	b, _ := json.Marshal(map[string]any{
		"ok":        true,
		"thread_id": threadID,
	})
	return string(b)
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
