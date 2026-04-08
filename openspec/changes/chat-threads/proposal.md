## Why

Chat-first-ui collapses everything into chat, but a flat chat stream is the wrong shape for any task that takes more than one round-trip — configuring directories, configuring a skill, triaging an attention item, drilling into one piece of evidence from a research result, reviewing a PR file by file. Today these have to either become long widget cards (unreadable), modal wizards (clunky, fight the chat metaphor), or chains of pollution-inducing main-chat messages (lose context). Slack and Mattermost solved this with threads: a sub-conversation that has its own scrollback, its own slash commands, its own widgets, and a parent message that summarizes it when it closes. Glitch needs the same primitive — once it exists, every multi-step interaction in the product becomes a thread, and the main chat stays clean.

## What Changes

- Introduce **threads** as a first-class chat construct: any message can spawn a thread; threads have their own scrollback, their own slash-command scope, and their own widget context.
- Wire the **`ParentID`** field added by `chat-first-ui` so messages with a parent are routed into the parent's thread instead of the main stream. The reply-count affordance becomes interactive: clicking it expands the thread.
- Wire the slash dispatcher's **`Scope`** parameter so commands inside a thread execute against that thread's local context (e.g. `/ignore` inside a directory-config thread ignores files in *that* directory, not globally).
- Add a **thread expand/collapse UX**: clicking the reply-count affordance opens the thread inline below its parent, scrollable independently of the main chat. A second click collapses it. Threads also support a "side pane" mode for long-running ones (configuration sessions).
- Add **thread lifecycle**: every thread has a state (`open` | `closed`). Closing a thread emits a `thread_closed` event, optionally captures a one-line summary on the parent message, and freezes further input. Re-opening is allowed; the parent's reply count and last-activity timestamp update accordingly.
- Add **canonical thread workflows** as the first consumers — the things this primitive exists for:
  - **Configure directories** thread (`/config dirs` → thread with directory picker widget, `/dir add`, `/dir scan`, `/dir ignore`, save-and-close).
  - **Configure a skill** thread (`/skills` → list, click → thread with the skill's config widget and a test-it action).
  - **Triage an attention item** thread (click an attention feed item → thread with the item context, `/why` to call the research loop, `/dismiss` or `/act` to close).
  - **Drill into evidence** thread (click a single evidence item from a research result → thread scoped to that evidence, with follow-up prompts that pass the evidence as context).
- Add a **thread-aware persistence layer**: chat history stores threads and their state, reloading a session re-creates the thread tree, closed threads are read-only just like the inert past-session widgets defined by `chat-first-ui`.
- Reframe **`/brain config`** (defined in `brain-observability`) so it opens a thread instead of a single widget card. **BREAKING** for that proposal's design.md but additive at the spec level — the widget is the same, it just lives inside a thread now.

## Capabilities

### New Capabilities
- `chat-thread-model`: the data model for a thread — its parent message, its messages, its open/closed state, its last-activity timestamp, its summary on close.
- `chat-thread-ui`: the renderer behaviors — expand/collapse, side-pane mode, reply-count affordance becoming interactive, scrolling independently of the main chat.
- `chat-thread-scope-routing`: the slash-dispatcher behavior that routes scoped commands to the active thread's context, including how widgets in a thread declare their thread-local scope.
- `chat-thread-lifecycle`: open/close/reopen semantics, summary capture on close, thread_closed/thread_opened events, freezing of closed threads except for re-open.
- `chat-thread-canonical-workflows`: the four shipped-in-v1 thread workflows: configure-directories, configure-skill, triage-attention-item, drill-into-evidence.

### Modified Capabilities
- `chat-message-types`: the `ParentID` field stops being always-`nil` and becomes the routing key threads use to group messages. The reply-count affordance becomes interactive.
- `chat-slash-commands`: the `Scope` parameter stops being always-`"main"`; handlers MAY now branch on it (and the four canonical workflows DO).

## Impact

- **Depends on**: `chat-first-ui` (must land first; its `ParentID` and `Scope` forward-compat hooks are the foundation this proposal builds on).
- **Touched packages**: `internal/chatui` (thread renderer, scope-aware dispatcher, persistence extension), `internal/activity` (attention item click → triage thread), `internal/research` (drill-into-evidence thread context), `internal/brain` (brain config thread workflow).
- **No new database**: threads live alongside existing chat history with the new `ParentID`/`Scope` columns; storage extension is additive.
- **Persistence growth**: threads are bounded — a closed thread freezes — so storage growth is proportional to user activity, not exponential.
- **Out of scope**: cross-thread search, thread permalinks, mentions/notifications inside threads, thread "follow" semantics, exporting a thread to markdown (could land as a follow-up).
- **Non-goals**: making the main chat *itself* a thread (it isn't — it's the root context); supporting nested threads (a thread cannot spawn a sub-thread in v1 — the parent-child relationship is flat); replacing widget cards with threads everywhere (single-screen widgets like `/status` stay as widget cards; only multi-step interactions become threads).
- **Breaking change for `brain-observability`**: that proposal currently puts brain config in a single widget card. With this change it should put it in a thread. The `brain-observability` design.md should be updated to reference this change as the rendering target for `/brain config`. The spec-level requirements do not change — the widget shape is the same, only its container.
