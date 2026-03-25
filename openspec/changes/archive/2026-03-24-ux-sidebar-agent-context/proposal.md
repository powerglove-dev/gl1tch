## Why

The sidebar is a rigid tmux pane that shows only a window list — it provides no insight into what agents are actually doing, cannot be hidden, and its ESC-capture global binding breaks CLI workflows (e.g. cancelling agents with Ctrl+C/ESC). The backtick leader key is ergonomically awkward and the status bar is completely unused real estate.

## What Changes

- **BREAKING** Replace the sidebar's window-list UI with an agent context panel showing per-session: provider name, current status (running/idle/error), tokens used, and estimated cost
- Add sidebar toggle: the panel can be shown or hidden per-user preference, defaulting to hidden
- Move "new session" and "prompt builder" launch controls from sidebar keys → tmux status bar display with chord-key hints
- Change leader key from `` ` `` to `ctrl+;` (`C-\;` in tmux notation)
- Remove the global `bind-key -n Escape select-pane -t .0` binding — ESC must pass through to running CLIs unintercepted
- Agent telemetry (tokens, cost) collected via event bus subscriptions in the panel process; agents publish `telemetry` events; panel subscribes and re-renders

## Capabilities

### New Capabilities

- `agent-context-panel`: Toggleable sidebar panel that displays per-session agent telemetry (provider, status, token count, cost estimate) sourced from event bus messages
- `status-bar-session-controls`: Tmux status bar shows new-session and prompt-builder chord hints alongside the existing clock

### Modified Capabilities

<!-- No existing specs affected — sidebar had no spec -->

## Impact

- `internal/bootstrap/bootstrap.go` — change leader key binding, remove ESC binding, update status bar format, add toggle chord binding
- `internal/sidebar/sidebar.go` — replace window-list model with agent context panel model; add show/hide toggle support
- `internal/bus/` — add `TelemetryEvent` message type for agents to publish token/cost data
- Agent integrations (e.g. `internal/chatui/`) — publish telemetry events on each response chunk
- No proto changes required (telemetry stays in-process via bus)
