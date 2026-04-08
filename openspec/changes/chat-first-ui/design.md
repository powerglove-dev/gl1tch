## Context

The desktop chat surface (`internal/chatui` and the desktop layout) currently treats chat as a markdown stream. Around it sit a left sidebar (navigation, ambient state, configuration entry points) and a workspace switcher. The sidebar predates a lot of the work that's now landed: the attention classifier, the upcoming research loop, the brain stats — all of these have content that *belongs* in chat because they're conversational and contextual. Splitting them across two surfaces fragments the experience.

This change is the UI half of "glitch as daily driver." Thread 1 (research loop) and Thread 4 (brain observability) both need a place to render rich, interactive content. Forcing them into either the sidebar or a markdown blob in chat is the wrong answer. The right answer is: chat carries structured payloads, the renderer paints them natively, and the sidebar goes away.

Existing infrastructure to lean on: `internal/chatui` already has a message model and a renderer, the activity sidebar was retired in `bcf1eb4`, and the attention classifier is shipping items today. What's missing is (a) a structured message-type protocol, (b) a slash-command surface, and (c) the discipline to delete the sidebar instead of leaving it as a fallback.

## Goals / Non-Goals

**Goals:**
- One persistent navigation surface: the workspace switcher. Everything else lives in chat.
- Chat can render rich, interactive widgets (cards, action chips, evidence bundles, score cards, attention feeds) as first-class message types.
- Slash commands (`/status`, `/brain config`, `/researcher list`, `/sessions`, `/help`) replace every sidebar entry point and return widget messages where appropriate.
- The attention feed is the default content of an idle chat — when the user isn't typing, the panel shows what needs them.
- Widget buttons round-trip back into chat as structured messages so the user can keep their hands on the keyboard if they want.

**Non-Goals:**
- No theming work, no animation work, no color changes.
- No drag-and-drop widget rearrangement.
- No "classic mode" toggle. The sidebar is removed, not hidden.
- No new event bus. Widget actions reuse the chat input pipeline.
- No mobile or web responsive considerations — desktop only.
- No feature parity audit beyond "every removed sidebar entry has a slash command."

## Decisions

### Decision 1: Chat messages become a discriminated union, not just markdown blobs

```go
type Message struct {
    ID        string
    Role      Role        // user | assistant | system
    Timestamp time.Time
    Type      MessageType // text | widget_card | action_chips | evidence_bundle | score_card | attention_feed
    Payload   any         // type-specific struct
}
```

The renderer dispatches on `Type`. `text` is markdown (today's behavior). The other types are structured payloads with their own paint logic. Persistence (chat history) stores `Type` + `Payload` as JSON so reloading a session repaints widgets correctly.

**Why:** structured payloads beat embedding HTML in markdown by every metric — testability, persistence, theming consistency, accessibility. The discriminated union is the smallest change that unlocks everything else.

**Alternative considered:** keep markdown and embed widget shortcodes (`{{widget:brain-config}}`). Rejected — shortcodes are a parser, parsers have bugs, and the round-trip from widget click back to chat input gets ugly.

### Decision 2: Five widget types in v1, no more

`widget_card`, `action_chips`, `evidence_bundle`, `score_card`, `attention_feed`. That's it. Resist the temptation to add a generic `form` type or `chart` type until something concrete needs them.

**Why:** small, well-defined surface area is easier to test and easier to teach. Each of the five maps to a known consumer:
- `widget_card`: brain config, researcher list, session list — anything that's a key/value table with optional buttons.
- `action_chips`: the "next prompt" suggestions under assistant answers.
- `evidence_bundle`: the research loop's evidence + score breakdown.
- `score_card`: brain observability metrics over time (Thread 4 will add the chart variant later).
- `attention_feed`: the idle-state default content.

**Alternative considered:** ship a generic `widget` type with arbitrary JSX-like children. Rejected — it's a UI framework, not a feature, and we'd spend the next month bikeshedding the schema.

### Decision 3: Slash commands dispatch to widgets, not text replies

`/brain config` returns a `widget_card` message, not a markdown table. `/status` returns a `widget_card` summarizing health. `/researcher list` returns a `widget_card` with the table from `glitch researcher list`. The slash dispatcher lives in `internal/chatui` and calls into existing packages (brain, research, plugin manager) directly — it does not go through the assistant/router, because slash commands are deterministic, not natural language.

**Why:** slash commands are a keyboard accelerator for specific functions. Routing them through the LLM would be slow and error-prone. The LLM is for natural-language `glitch ask` calls.

### Decision 4: Widget buttons synthesize chat input messages

When a user clicks an `action_chip` button, the renderer sends a synthetic message through the same pipeline as keyboard input, with a `synthetic: true` marker so audit logs can distinguish them. The button's `action` field is either:
- A slash command string (e.g. `/brain config threshold 0.8`) — dispatched immediately by the slash handler.
- A natural-language prompt (e.g. `dig into PR #412`) — sent to the assistant like normal user input.

**Why:** one input pipeline, one history, one undo. No new event bus, no new state machine. The `synthetic: true` marker is the only new bit.

### Decision 5: Idle chat = attention feed

When a chat session has no in-flight assistant response and no recent user input (>30s), the panel renders an `attention_feed` message at the top spanning the visible area. The moment the user types a character, the feed compacts to a one-line strip pinned to the top of the chat panel ("3 things need you · expand"). Clicking the strip re-expands the feed inline as a regular message in the conversation history (not floating).

**Why:** the attention feed is the highest-value passive surface. Putting it in the sidebar made it ignorable; putting it in the chat panel makes it the thing the user sees when they're not actively typing. Compacting on type respects that the user is now in the middle of something.

**Alternative considered:** a separate "feed" tab. Rejected — tabs are sidebar in disguise.

### Decision 6: Workspace switcher is a top strip, not a sidebar

A 28px-tall strip across the top of the chat panel: workspace name, dropdown to switch, connection/health dot, brain status dot. Nothing else. Clicking either dot opens a `widget_card` in chat with details. Right-clicking the workspace name copies the path.

**Why:** persistent navigation deserves persistent space, but only the thing that's actually persistent (which workspace am I in). Everything else is summonable via slash command.

### Decision 7: One-time welcome message on first launch after the change

On the first chat session after this change lands, prepend a system message: "The sidebar is gone. Type `/help` to see the new commands, or just keep chatting." Not a modal, not a tutorial — a single chat message the user can dismiss by scrolling past.

**Why:** the change is visible enough that ignoring it would be hostile; a modal would be over-engineered.

### Decision 8: Persist widget messages so reloads repaint

Chat history serialization is extended to include `Type` and `Payload` for non-text messages. Reloading a session repaints widgets in their last state. Action button clicks are *not* replayed — buttons shown in history are inert by default, with a "this widget is from a past session" affordance if the user hovers.

**Why:** chat history that loses widgets is worse than no history. Inert buttons in past sessions avoid replaying actions accidentally.

## Risks / Trade-offs

- **[Risk] Users miss something the sidebar gave them and we don't notice** → Mitigation: the welcome message lists the slash commands explicitly; we audit the sidebar's current entries before deletion (Task 1.1) so nothing is silently dropped.
- **[Risk] Five widget types isn't enough and we keep adding more in v2** → Accepted. Adding a sixth type is cheap once the protocol exists; the discipline is to require a concrete consumer before adding one.
- **[Risk] Idle = attention feed feels noisy if there are too many items** → Mitigation: the attention classifier already has a confidence threshold; we only show items above it. If it's still noisy in practice, add a `/feed quiet` command.
- **[Risk] Persisted widget payloads grow chat history storage** → Mitigation: payloads are bounded (no images, no large blobs); we can compress at the storage layer if it bites.
- **[Risk] Slash command surface grows unbounded** → Mitigation: every slash command must map to a real package call with a real consumer; we don't add slash commands for hypothetical features.
- **[Risk] Removing the sidebar breaks user muscle memory** → Accepted. The pre-1.0 rule is "wipe and restart" — no fallback toggle, no classic mode.
- **[Trade-off] Slash commands are deterministic and bypass the LLM** → Accepted. They're a keyboard accelerator, not a chat surface. Natural-language entry points still go through the assistant.
- **[Trade-off] Widget actions replay through the input pipeline** → Accepted. Slightly indirect, but it gives us one history, one undo, one audit trail for free.

## Migration Plan

No data migration. Rollout:

1. Inventory the current sidebar — every button, label, link — and map each to either a slash command or a widget card. Anything that maps to "nothing" must be re-justified before deletion.
2. Land the chat message types and the renderer for `text` (no behavior change yet) + `widget_card`.
3. Land the slash dispatcher with `/help`, `/status`, `/sessions`.
4. Land `attention_feed` and the idle/compact rule.
5. Land `evidence_bundle` and `score_card` (consumed by Threads 1 and 4).
6. Land `action_chips` and the synthetic input pipeline.
7. Move the workspace switcher to the top strip.
8. Delete the sidebar code in one commit. Land the welcome message in the same commit.

**Rollback:** every step before #8 is additive. Step #8 is the irreversible one. Rollback after #8 is a git revert; we accept that.

## Forward-Compatibility With Threads

A separate change (`chat-threads`) will introduce Slack/Mattermost-style threads — sub-conversations spawned from any message, with their own scrollback, slash-command scope, and widget context. This change does not implement threads but it adds three small forward-compat hooks so retrofitting them later is additive, not a refactor:

1. **`ParentID` on every message** — nullable, always `nil` in v1, populated by `chat-threads` to point at the thread root.
2. **Reply-count affordance in the renderer** — when any message has `ParentID` pointing at a parent, the parent renders "💬 N replies"; clicking is a no-op in v1.
3. **`Scope` parameter on the slash dispatcher** — always `"main"` in v1; `chat-threads` will pass the active thread ID so commands like `/dir add` execute against the thread's local context.

These hooks are cheap to ship now and avoid a chat-first-ui v1 → threads v1 migration that would touch every message-handling code path.

## Open Questions

- **OQ1: Where does the chat *input* live after the sidebar is gone?** Bottom of the chat panel, presumably, but worth confirming the existing layout supports a full-width input.
- **OQ2: Do widget cards need a "minimize" affordance, or do they always render at full size?** Lean: minimize is a v2 problem; v1 cards are compact by default and expand to show details on click.
- **OQ3: How do we handle widget actions that take time (e.g. running a slow slash command)?** Probably reuse the existing assistant typing indicator; confirm the chat renderer can show a loading state on a widget message.
- **OQ4: Should `/help` be a static list or dynamically generated from the slash dispatcher?** Lean: dynamic, so adding a slash command automatically updates `/help`.
- **OQ5: What does the attention feed look like in compact mode beyond "3 things need you"?** Probably the top-1 item title plus a count badge; defer to a quick visual pass during implementation.
