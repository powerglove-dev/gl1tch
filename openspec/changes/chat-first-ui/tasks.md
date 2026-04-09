## 1. Sidebar inventory and audit

- [ ] 1.1 Enumerate every entry in the current left sidebar (buttons, labels, status indicators, links) and write the list to `openspec/changes/chat-first-ui/sidebar-inventory.md`
- [ ] 1.2 Map each entry to one of: slash command (which?), widget card (which?), workspace strip element, or "delete (justified)"
- [ ] 1.3 Review the mapping before writing any code so nothing is silently dropped

## 2. Chat message-type protocol

- [ ] 2.1 Refactor the existing chat `Message` struct to include `Type` (enum) and `Payload` (any)
- [ ] 2.2 Define payload structs for `text`, `widget_card`, `action_chips`, `evidence_bundle`, `score_card`, `attention_feed`
- [ ] 2.3 Update chat history serialization to round-trip `Type` + `Payload` as JSON
- [ ] 2.4 Add a fallback renderer for unknown types ("[unsupported widget]")
- [ ] 2.5 Add unit tests for serialization round-trip and fallback rendering

## 3. Renderers for v1 widget types

- [ ] 3.1 Implement `widget_card` renderer (title, subtitle, key/value rows, action buttons)
- [ ] 3.2 Implement `action_chips` renderer (inline button row)
- [ ] 3.3 Implement `evidence_bundle` renderer (compact summary + expand affordance for full breakdown)
- [ ] 3.4 Implement `score_card` renderer (metric name + current value + sparkline)
- [ ] 3.5 Implement `attention_feed` renderer (ordered item list with per-item action)
- [ ] 3.6 Snapshot tests for each renderer with representative payloads

## 4. Slash command dispatcher

- [ ] 4.1 Add a `slash.Registry` in `internal/chatui` with `Register(name, describe, handler)` and `Dispatch(line)`
- [ ] 4.2 Wire chat input to detect leading `/` and route to the dispatcher instead of the assistant
- [ ] 4.3 Ensure dispatcher path makes no LLM calls
- [ ] 4.4 Implement the unknown-command response with closest-match suggestions

## 5. Required v1 slash commands

- [ ] 5.1 `/help` — dynamically generated from the live registry, returns `widget_card`
- [ ] 5.2 `/status` — workspace name, brain status, connection health, active session count
- [ ] 5.3 `/sessions` — recent sessions with reopen action buttons
- [ ] 5.4 `/brain config` — current config as a `widget_card` with editable action buttons
- [ ] 5.5 `/researcher list` — registered researchers table from `glitch-researcher-extensibility`
- [ ] 5.6 `/feed` — show, quiet, or threshold the attention feed
- [ ] 5.7 Smoke test invoking each command end-to-end

## 6. Attention feed as idle default

- [ ] 6.1 Adapt `internal/activity` output to the `attention_feed` payload struct
- [ ] 6.2 Implement the idle detector (30s since last user input or assistant response)
- [ ] 6.3 Render full feed when idle, compact strip when typing
- [ ] 6.4 Implement the click-to-expand behavior that appends an `attention_feed` message to history
- [ ] 6.5 Implement the empty-feed quiet state
- [ ] 6.6 Manual smoke: leave the chat idle for 30s with attention items present, type a character, click the strip

## 7. Widget action protocol

- [ ] 7.1 Define the `synthetic: true` flag on chat input messages
- [ ] 7.2 Wire button clicks to construct a synthetic chat input message and route it through the existing pipeline
- [ ] 7.3 Add brain audit logging that records widget origin (source widget message ID) for synthetic inputs
- [ ] 7.4 Implement inert button rendering for messages loaded from history
- [ ] 7.5 Test: click a chip running a slash command, click a chip running a natural-language prompt, click an inert button (no-op)

## 8. Workspace switcher relocation

- [ ] 8.1 Build the 28px top strip with workspace name, dropdown, connection dot, brain status dot
- [ ] 8.2 Wire the dots to open `widget_card` messages with details on click
- [ ] 8.3 Wire right-click on workspace name to copy the path to clipboard
- [ ] 8.4 Visual smoke: switch workspaces, click both dots

## 9. Sidebar deletion and welcome message

- [ ] 9.1 Delete the sidebar layout component and any code that referenced it
- [ ] 9.2 Add the one-time welcome message ("The sidebar is gone. Type `/help` …") shown on first session after the change lands
- [ ] 9.3 Verify no dangling sidebar references in `internal/chatui` or the desktop layout
- [ ] 9.4 Run `openspec validate chat-first-ui --strict` and fix findings
- [ ] 9.5 Manual smoke: fresh launch shows top strip + chat panel + welcome message + idle attention feed

## 10. Workspace primary-directory affordance (frontend follow-up)

The store + Wails layer for workspace primary-directory landed in
commit-after-bf9e71d (`store.SetWorkspacePrimaryDirectory`,
`App.SetWorkspacePrimaryDirectory`, `WorkspaceDirectory.Primary` flag
on `ListWorkspaceDirectoriesDetailed`). The frontend half is still
TODO and lands as part of chat-first-ui because the directory list
moves out of the dying left sidebar into a `/config dirs` widget /
thread workflow under chat-first-ui's slash-command surface.

- [ ] 10.1 Render a star/badge on the primary row in
  `ListWorkspaceDirectoriesDetailed` output (drives the existing
  Sidebar.tsx directory list while it still exists; same component
  re-used by the `/config dirs` widget when chat-first-ui lands the
  thread workflow)
- [ ] 10.2 Right-click action ("set as primary") on every non-primary
  directory row, calling `App.SetWorkspacePrimaryDirectory` and
  re-rendering on the `workspace:updated` event the backend already
  emits
- [ ] 10.3 Add a "primary" / "additional" group header to the
  directory list so the user can see at a glance which one anchors
  the research loop's cwd vs which ones are scanned for reference
- [ ] 10.4 Surface `Workspace.PrimaryDirectory` in the workspace
  switcher tooltip so the user can confirm which repo a workspace
  targets without opening the directory list
- [ ] 10.5 Smoke: in the desktop, add a second directory to a
  workspace, set it as primary, run a thread on a chat row, confirm
  the research loop's `git -C` lands in the new primary repo
  (mirrors `glitch threads smoke` from the CLI side)
