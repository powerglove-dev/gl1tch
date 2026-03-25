## Why

The current sidebar panel is functional but visually minimal — it resembles a basic list widget, not the BBS aesthetic the rest of orcai is building toward. More critically, the toggle (`ctrl+space t`) always spawns the panel on window 0, meaning it is only visible there. Users who are actively working in a session window (window 1, 2, etc.) cannot bring up a monitoring panel without leaving their current window.

The result is that the telemetry and session data — the most useful live information — is stranded on a window the user has left.

## What Changes

- The sidebar/panel is redesigned to look like a **BBS sysop node monitor**: full box-drawing borders, block-character header with `▒▒▒ ORCAI SYSOP MONITOR ▒▒▒`, per-node rows labelled `NODE 01`, `NODE 02`, etc., status badges (`BUSY` / `IDLE` / `WAIT`), and a scrolling activity log at the bottom.
- The toggle (`ctrl+space t`) is changed from spawning on `orcai:0` to spawning **on the current window** — `tmux split-window` without `-f` targets the active window. The panel appears as a side pane wherever the user is.
- The toggle is **per-window**: each window tracks its own panel visibility independently (marker file keyed by window index).
- The panel is **narrow by design** (25–30% width) so it does not disturb the active session pane.
- The activity log scrolls the last N events (streaming start, done, cost) in chronological order, BBS scrollback style.
- Colour palette remains Dracula 256-colour: purple borders, pink headers, green ● BUSY, dim ○ IDLE, yellow metrics.

## Capabilities

### Modified Capabilities

- `agent-context-panel`: Redesigned as a BBS sysop monitor — new visual language (box-art, node rows, activity log), per-window toggle, current-window spawn.

## Impact

- `internal/sidebar/sidebar.go` — visual rewrite of `View()` + `RunToggle()` spawn target change.
- `internal/sidebar/sidebar_test.go` — update visual assertions to match new layout.
- No changes to bootstrap, chatui, welcome, or picker.
- No new dependencies.
