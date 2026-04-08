## Context

`chat-first-ui` flattens the desktop into a chat panel and a workspace switcher. That works for one-shot interactions: ask a question, get an answer, render it. It does not work for anything that takes more than one round-trip with state — configuring a directory ignore list, walking through a skill's settings, triaging an attention item by asking the loop why it fired, drilling into a single piece of evidence and asking follow-up questions about it. Today the design forces those into either long widget cards (no scrollback, no follow-up), modal wizards (fight the chat metaphor), or polluting the main chat with messages that aren't really part of the user's main task.

Slack and Mattermost solved this with threads. A thread is a sub-conversation that has its own scrollback, its own slash commands, its own widgets, and a parent message that summarizes it when it closes. The main timeline stays clean; the focused work stays focused.

This change introduces threads as a first-class chat construct in glitch. It depends on `chat-first-ui`, which already added the forward-compat hooks (`ParentID` on every message, `Scope` on the slash dispatcher, an inert reply-count affordance). The job here is to wire those hooks into a real thread model, real expand/collapse UX, real lifecycle events, and the four canonical workflows that prove the primitive is right.

## Goals / Non-Goals

**Goals:**
- A thread is a first-class object: parent message + ordered children + open/closed state + last-activity timestamp + optional close summary.
- Any message — assistant, user, widget — can spawn a thread. Threads are flat (a thread cannot spawn a sub-thread in v1).
- A thread has its own slash-command scope: `/ignore` inside a directory-config thread targets that directory; `/why` inside an attention triage thread runs the research loop scoped to that item.
- Closed threads are read-only; widgets and inputs inside them disable the same way `chat-first-ui` already disables widgets in reloaded historical sessions.
- Reloading a session reconstructs the thread tree exactly; closed threads stay closed; expand/collapse state is restored.
- Four canonical thread workflows ship in v1: configure-directories, configure-skill, triage-attention-item, drill-into-evidence. Each is the proof that the primitive fits its intended consumer.

**Non-Goals:**
- No nested threads. A thread parent must be a top-level message.
- No cross-thread search, no thread permalinks, no mentions, no "follow this thread" notifications.
- No thread export or sharing surface (could land as a follow-up).
- No moving the main chat into "the root thread" — the main chat is the root context, threads are scoped sub-contexts off of it.
- No retroactive threading of historical sessions. Threads exist for messages created after the change ships.
- No replacement of widget cards everywhere. Single-screen widgets (`/status`, `/researcher list`) stay as widget cards; threads are for *multi-step interactions with state*.

## Decisions

### Decision 1: Threads are flat, not nested

A thread parent must be a top-level main-chat message. Spawning a thread from inside a thread is rejected at the dispatcher level. This rules out infinite nesting, simplifies the renderer, and matches Slack/Mattermost behavior — both of which deliberately keep threads one level deep because nested threads turn into "where am I?" hellscapes.

**Alternative considered:** allow one level of nesting ("a sub-thread for a specific reply"). Rejected — cost is high, value is small, and the four canonical workflows don't need it.

### Decision 2: Two display modes — inline expand and side pane

Most threads display as **inline expand**: clicking the parent message's reply-count affordance reveals the thread directly below the parent in the main chat, scrollable independently. Clicking again collapses it. This is the default and matches Slack's "view in conversation" view.

Some threads — long-running configuration sessions, in particular — display as a **side pane**: the thread takes a right-hand pane next to the main chat, the main chat scrolls independently. The user opts into side-pane mode via a thread action button or by holding shift while clicking the affordance. Side pane is designed for "I'm tuning settings while still keeping an eye on the main feed."

The thread itself doesn't know which mode it's in; the renderer picks based on user action and remembers the choice per thread.

**Alternative considered:** modal overlay. Rejected — it breaks the chat metaphor and the user can't see the main chat while working in the thread.

### Decision 3: Slash-command scope is the thread ID

When the user runs a slash command from inside a thread, the dispatcher passes `Scope = thread:<id>` to the handler. Handlers in v1 either:
- **Ignore scope** (most slash commands like `/help`, `/status`, `/sessions`) — they behave the same in any context.
- **Read scope and act on the thread's local state** — `/dir add`, `/ignore`, `/why`, `/dismiss`, `/test`, `/save`. These are the canonical-workflow handlers.

The handler signature stays the same as `chat-first-ui` defined; the `Scope` parameter is the only addition.

### Decision 4: Thread lifecycle — open / closed, with reopen allowed

```
            ┌──────┐  user closes        ┌────────┐
   spawn → │ open │ ─────────────────→ │ closed │
            └──────┘                    └────────┘
                ↑                            │
                └────  user reopens  ────────┘
```

A new thread is `open`. The user closes it via a thread close action button (or by completing a workflow that auto-closes). Closing emits a `thread_closed` event, captures an optional one-line summary on the parent message ("ignored 14 files in /tmp/scratch"), and freezes input — text input and widget actions are disabled.

A closed thread can be reopened by clicking a "reopen" affordance on the parent. Reopening emits `thread_opened`, restores input, and updates the parent's last-activity timestamp.

**Why reopen at all?** Configuration and triage threads often need a second pass. Forcing the user to spawn a fresh thread loses the context. But reopen is a deliberate action — the default for a closed thread is "this is done."

### Decision 5: Auto-close when a workflow's terminal action runs

The four canonical workflows each declare a terminal action:
- configure-directories → `/save`
- configure-skill → `/save` (or `/cancel`)
- triage-attention-item → `/dismiss` or `/act`
- drill-into-evidence → no auto-close (drill threads stay open until the user closes them, since "I'm done thinking about this evidence" is harder to detect)

When a terminal action runs successfully, the dispatcher closes the thread automatically and captures the summary line. The user can override by re-opening.

### Decision 6: Persistence — threads are stored as a tree, not flattened

Chat history serialization extends to include a `Thread` record per thread (parent message ID, state, summary, last-activity, expand-state) and the per-message `ParentID` field already added by `chat-first-ui` does the rest. Reloading walks the messages and groups them by `ParentID` to reconstruct the tree. Closed threads round-trip with their summary intact.

**Why a separate `Thread` record at all?** Because the open/closed state, summary, and side-pane preference are properties of the thread, not of any individual message. Putting them on the parent message would conflate concerns.

### Decision 7: The four canonical workflows are the spec

This is the discipline that keeps the design honest: each canonical workflow has a small dedicated requirement set in `chat-thread-canonical-workflows/spec.md`, with scenarios that name the exact slash commands, the exact widgets, the exact terminal action. If a workflow can't be specified at that level, the threading primitive isn't ready for it yet.

The four:
1. **Configure directories** — `/config dirs` opens a thread containing a directory picker widget plus a chat where the user can type free-form requests like "ignore .DS_Store everywhere." Slash commands: `/dir add <path>`, `/dir scan`, `/dir ignore <pattern>`, `/save`. Auto-close on `/save`.
2. **Configure a skill** — `/skills` returns a `widget_card` listing skills; clicking one opens a thread containing the skill's config widget plus a "test it" action chip. Slash commands: `/edit <field>=<value>`, `/test`, `/save`, `/cancel`. Auto-close on `/save` or `/cancel`.
3. **Triage an attention item** — clicking an attention feed item opens a thread containing the item context plus an action chip set. Slash commands: `/why` (runs the research loop scoped to the item), `/dismiss`, `/act`. Auto-close on `/dismiss` or `/act`.
4. **Drill into evidence** — clicking a single item in an `evidence_bundle` opens a thread containing that evidence as a widget plus a chat where follow-ups automatically receive the evidence as context. No slash commands required; no auto-close.

## Risks / Trade-offs

- **[Risk] Threads make the chat panel feel like every other team chat product, losing what makes glitch distinctive** → Mitigation: threads are scoped to multi-step interactions, not used as a general organization tool. The main chat remains the primary surface; threads are summoned by specific actions, not by user habit.
- **[Risk] Inline expand makes the main chat scroll feel unpredictable** → Mitigation: an expanded thread has a fixed maximum height (5 messages visible) with internal scrolling; the parent's footprint in the main chat is bounded.
- **[Risk] Side-pane mode collides with the workspace switcher and chat input** → Mitigation: the side pane takes the right third of the chat panel; the input stays at the bottom of the main chat (left two-thirds) and the thread has its own input at the bottom of the side pane. Confirm during implementation that this layout is reachable in the existing desktop window.
- **[Risk] Auto-close on terminal action surprises the user** → Mitigation: the close is animated and reversible — the parent message shows "thread closed (reopen)" for 10 seconds with a clear undo affordance. After 10 seconds the parent settles into the standard closed-thread state.
- **[Risk] Slash command scope explodes if every workflow defines its own commands** → Mitigation: keep canonical workflows as the spec floor — if a fifth workflow needs threads, it lands as a follow-up change with its own requirements, not by scope creep here.
- **[Risk] Persistence layer rewrites are hard** → Mitigation: the schema extension is additive (`ParentID` is already in the message; the new `Thread` record table is a sibling); reloading falls back to flat-chat behavior if a `Thread` record is missing.
- **[Trade-off] No nested threads** → Accepted. The cost is "I want to drill into a drill," which is a self-correcting habit (just close the inner thread and open a new one off the main chat).
- **[Trade-off] Closed threads can be reopened** → Accepted. Forces handlers to think about idempotency but matches user intuition.
- **[Trade-off] Brain config moves into a thread** → Accepted. Better UX than a single widget card; the `brain-observability` proposal's design.md should be updated to reference this change as the rendering target.

## Migration Plan

No data migration. Rollout:

1. Land the `Thread` record type and the persistence extension in `internal/chatui`.
2. Land the inline-expand renderer for threads, wired to the existing reply-count affordance.
3. Land the scope-aware dispatcher (passing the thread ID to handlers; existing handlers ignore it).
4. Land the lifecycle events (`thread_opened`, `thread_closed`) and the auto-close + summary capture path.
5. Land each canonical workflow one at a time, starting with **configure-directories** as the smallest and most concrete.
6. Land side-pane mode as a follow-on once at least one workflow proves inline-expand isn't enough.
7. Update `brain-observability/design.md` to reference threads as the `/brain config` rendering target (no spec change required, just design.md).

**Rollback:** persistence is additive. If threads need to be ripped out, the renderer falls back to flat chat and the `Thread` records become dead rows. Every message still has a valid `ParentID` (always nil if threads are off).

## Open Questions

- **OQ1: What's the exact close animation duration before the parent settles?** Lean: 10 seconds, configurable via a hidden setting.
- **OQ2: Do threads support drag-to-resize for inline expand height?** Lean: no in v1; fixed height (5 messages visible) with internal scrolling. Drag-to-resize is a follow-up if the fixed height bites.
- **OQ3: How does the attention feed handle a triage thread that's still open after 24 hours?** Lean: the parent attention item shows a "still triaging" badge, doesn't disappear from the feed until the thread closes. This needs confirmation that the attention classifier can carry the thread state forward.
- **OQ4: Can a user spawn an empty thread (no parent message)?** No — every thread must have a parent. The dispatcher rejects empty-thread spawns.
- **OQ5: Should `drill into evidence` thread context include the *full* bundle or just the one evidence item?** Lean: just the one item. The user can spawn multiple drill threads for multiple items.
- **OQ6: When a closed thread is reopened, does its summary stay visible?** Lean: yes — the summary becomes a "previous outcome" line at the top of the reopened thread, so the user can see what they decided last time.
