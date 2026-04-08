## 1. Thread data model

- [ ] 1.1 Add `Thread` struct in `internal/chatui` with fields: `ID`, `ParentMessageID`, `State`, `Summary`, `LastActivityAt`, `ExpandPref`
- [ ] 1.2 Add a thread store backed by the existing chat history persistence
- [ ] 1.3 Implement `Spawn(parentID) (Thread, error)` rejecting parents whose own `ParentID` is non-nil (no nesting)
- [ ] 1.4 Implement `LookupByParent(parentID) (Thread, ok)` and `LookupByID(id) (Thread, ok)`
- [ ] 1.5 Add unit tests covering spawn, nesting rejection, and lookups

## 2. Persistence and reload

- [ ] 2.1 Extend chat history serialization to write `Thread` records alongside messages
- [ ] 2.2 Implement reload that walks messages, groups by `ParentID`, and reconstructs the tree
- [ ] 2.3 Round-trip closed-thread `Summary` and `LastActivityAt`
- [ ] 2.4 Tolerate sessions with no `Thread` records (legacy / unthreaded sessions reload as flat chat)
- [ ] 2.5 Test: write a session with two threads (one open, one closed-with-summary), reload, assert tree is intact

## 3. Inline expand renderer

- [ ] 3.1 Wire the reply-count affordance from `chat-first-ui` to dispatch a thread expand/collapse action
- [ ] 3.2 Render an expanded thread inline directly below its parent in the main chat stream
- [ ] 3.3 Cap visible height at 5 messages with internal scrolling for longer threads
- [ ] 3.4 Persist per-thread expand state in the thread record so reloads remember it
- [ ] 3.5 Snapshot test for expanded vs collapsed rendering

## 4. Side-pane mode

- [ ] 4.1 Detect shift-click on the reply-count affordance
- [ ] 4.2 Render the thread in a right-hand pane (right third of the chat panel) with its own scroll and input
- [ ] 4.3 Persist `ExpandPref` per thread so re-opening uses the same mode
- [ ] 4.4 Confirm the layout doesn't collide with the workspace switcher or main chat input
- [ ] 4.5 Snapshot test for side-pane layout

## 5. Scope-aware slash dispatcher

- [ ] 5.1 Pass `Scope = "thread:<id>"` to handlers for any slash command originating inside a thread
- [ ] 5.2 Reject thread-scoped commands (e.g. `/dir add`) when invoked from the main chat with a clear error
- [ ] 5.3 Extend widget action chip rendering to inherit the enclosing thread's scope
- [ ] 5.4 Update existing slash handlers to ignore Scope (no behavior change for v1 main-chat commands)
- [ ] 5.5 Test: run `/help` from main and from inside a thread, both succeed; run `/dir add` from main, fails

## 6. Lifecycle and events

- [ ] 6.1 Implement `Close(threadID, summary)` that transitions to `closed`, persists the summary, freezes input
- [ ] 6.2 Implement `Reopen(threadID)` that transitions back to `open`, updates `LastActivityAt`
- [ ] 6.3 Emit `thread_opened` and `thread_closed` events to the brain event store
- [ ] 6.4 Implement the 10-second undo affordance after auto-close
- [ ] 6.5 Test: close → reopen round trip; auto-close → undo; auto-close → settle after 10s

## 7. Configure-directories canonical workflow

- [ ] 7.1 Register `/config dirs` slash command (main chat) that spawns the thread and renders the directory picker widget
- [ ] 7.2 Register thread-scoped `/dir add`, `/dir scan`, `/dir ignore`, `/save`
- [ ] 7.3 Wire `/save` as the terminal action with summary capture
- [ ] 7.4 Smoke test: run `/config dirs`, add two directories, ignore three patterns, `/save`, verify thread closes with the right summary

## 8. Configure-skill canonical workflow

- [ ] 8.1 Register `/skills` slash command returning a `widget_card` listing skills
- [ ] 8.2 Wire skill-row click to spawn a configure-skill thread with the skill's config widget
- [ ] 8.3 Register thread-scoped `/edit`, `/test`, `/save`, `/cancel`
- [ ] 8.4 Wire `/test` to run the skill and append the result to the thread (not main chat)
- [ ] 8.5 Smoke test for save and cancel paths

## 9. Triage-attention-item canonical workflow

- [ ] 9.1 Wire attention feed item click to spawn a triage thread containing the item context and `action_chips` for `/why`, `/dismiss`, `/act`
- [ ] 9.2 Wire `/why` to invoke the research loop scoped to the item; render the resulting `evidence_bundle` inside the thread
- [ ] 9.3 Wire `/dismiss` and `/act` as terminal actions; mark the activity store accordingly
- [ ] 9.4 Smoke test: trigger an attention item, click it, run `/why`, see grounded evidence, `/dismiss`, verify thread closes

## 10. Drill-into-evidence canonical workflow

- [ ] 10.1 Wire single-evidence-item clicks inside an `evidence_bundle` to spawn a drill thread containing that one item
- [ ] 10.2 In drill threads, prepend the evidence to the assistant prompt for any free-form follow-up
- [ ] 10.3 No auto-close — drill threads stay open until the user closes them
- [ ] 10.4 Smoke test: open a research result, click an evidence item, ask a follow-up, verify the response references the evidence

## 11. Brain-config relocation

- [ ] 11.1 Update `brain-observability/design.md` to reference threads as the rendering target for `/brain config` (no spec change required)
- [ ] 11.2 Once `brain-observability` lands, port `/brain config` to spawn a thread instead of returning a flat widget card

## 12. Validation and rollout

- [ ] 12.1 Run `openspec validate chat-threads --strict` and fix findings
- [ ] 12.2 Manual smoke covering all four canonical workflows end to end
- [ ] 12.3 Document threads in the `/help` widget output (auto-generated from registered commands)
