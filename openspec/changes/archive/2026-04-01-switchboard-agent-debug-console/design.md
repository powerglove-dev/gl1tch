## Context

The Switchboard (`internal/switchboard/switchboard.go`) is a single BubbleTea model with three visual regions: left column (banner + launcher + agent runner), right column (activity feed), and a bottom bar. All rendering is manual ANSI string concatenation with no scroll state. The activity feed appends lines unconditionally so long-running agents produce hundreds of rows that blow past the terminal height. The agent runner panel (`buildAgentSection`) changes height as the user steps through form steps (provider → model → prompt), causing the left column to grow taller than the right column and misalign the join line. tmux is already a hard runtime requirement via the session launch flow.

## Goals / Non-Goals

**Goals:**
- Activity feed is bounded to the available height and scrollable up/down without layout breakage
- Agent runner outer box is fixed height regardless of which form step is active; content reflows within it
- New "SIGNAL BOARD" panel in the right column above the activity feed: one LED row per job, animated blink on running jobs, keyboard filter (all/running/done/failed)
- Pressing `enter` on a signal board row opens an 80%-wide debug popup overlay showing the agent's tmux window content
- Pipeline runner creates a dedicated tmux window per job, hidden from the status bar; window survives job completion
- Debug popup embeds a scrollable view of that tmux window's captured pane output

**Non-Goals:**
- Interactive typing inside the popup (read-only capture; user can separately attach via `tmux select-window`)
- Persistent worktree management (worktrees are already handled by the agent itself if needed)
- Mouse click support (keyboard-only navigation for now)
- Rewriting the feed as a `viewport` component (ANSI string approach is preserved for consistency)

## Decisions

### D1 — Feed scroll via offset integer, not `viewport`
The feed is already rendered as plain ANSI strings. Introducing `charmbracelet/bubbles/viewport` would require refactoring all ANSI box-drawing helpers. Instead, `Model` gains `feedScrollOffset int` clamped to `[0, max(0, totalFeedLines - visibleH)]`. Arrow-down/up in feed-focus mode nudge the offset; new entries added at the top auto-scroll to 0 (follow mode). This is consistent with how the rest of the Switchboard is drawn.

**Alternative considered**: `viewport.Model` — cleaner API but requires switching rendering to lipgloss styles throughout; deferred.

### D2 — Agent runner fixed height via `agentSectionHeight() int`
A new method computes the maximum height the agent section could ever occupy (banner + launcher rows + fixed agent box height) and clamps the left column render to that. The agent box itself gets a fixed inner height budget: when the provider list is longer than the budget it scrolls internally (same offset approach as the feed). This prevents the box from growing when switching form steps.

### D3 — Signal board as a new model in `signal_board.go`
The signal board is a self-contained struct (`SignalBoard`) embedded in `Model`. It holds a `[]jobRecord` slice mirrored from `m.feed` entries and an `activeFilter` field (all/running/done/failed). A `tick` drives the blink state (reuse the existing 3-second tick or add a 500ms blink tick). Rendering is a fixed-height compact strip of LED rows above the activity feed in the right column.

**LED row format** (one line per job):
```
  [●] 09:28:02  agent: github-copilot/claude-sonnet-4.6   running
```
`●` blinks between bright and dim on the running tick for active jobs.

### D4 — Tmux window per job, hidden from status bar
When the switchboard launches an agent job via `runAgentCmdCh` it now also calls:
```
tmux new-window -d -t <current-session> -n "orcai-<feedID>"
tmux set-window-option -t <session>:orcai-<feedID> hide-from-statusbar on
```
The window starts a shell in the agent's working directory (CWD). Pipeline output lines are also echoed into the window via `tmux send-keys`. After job completion the window remains open. The `jobHandle` struct gains `tmuxWindow string`.

**Alternative considered**: Embedding a `pty` inside the BubbleTea process — too complex, tmux is already required.

### D5 — Debug popup as an overlay layer in `View()`
`Model` gains `debugPopupOpen bool` and `debugPopupJobID string`. When open, `View()` renders the normal layout at full width/height, then overlays an 80%-wide bordered box centered horizontally. Inside: the captured pane from `tmux capture-pane -t <window> -p`. The popup is static (no live streaming) and refreshes on the next tick. Dismiss with `esc` or `q`.

### D6 — Signal board focus and keybinding
A new focus state `focusSignalBoard` is added to the existing focus rotation (`launcher → agent → signalBoard → back to launcher`). When `focusSignalBoard` is active, `↑`/`↓` navigate rows, `f` cycles the filter, `enter` opens the debug popup.

## Risks / Trade-offs

- **`tmux capture-pane` latency**: Refreshing on every tick (3s) may feel slow. Mitigation: reduce tick to 1s for popup mode; keep 3s for normal mode.
- **`hide-from-statusbar` availability**: Requires tmux ≥ 3.2. Mitigation: wrap in error check; if unsupported, window is visible but functional.
- **Fixed agent runner height may clip long provider lists**: Use internal scroll offset so all items remain reachable.
- **Signal board mirroring feed**: Two sources of truth. Mitigation: signal board derives from `m.feed` at render time rather than maintaining its own slice — no duplication.

## Migration Plan

1. Add `feedScrollOffset int` to `Model`, wire `↑`/`↓` in feed-focus mode
2. Fix `buildAgentSection` height budget
3. Add `signal_board.go` with `SignalBoard` type and render logic
4. Wire signal board into `View()` right column
5. Add tmux window creation to `handleEnter()`
6. Add `debug_popup.go` with overlay render
7. Wire popup open/close into `Update()`
8. Add `focusSignalBoard` to focus rotation

No migration required for existing data; all state is in-memory per session.

## Open Questions

- Should the debug popup auto-attach (drop into the tmux window) on a second `enter` press, or always remain a read-only overlay? (Leaning: second `enter` runs `tmux select-window` in a new os/exec call to surface the window.)
- Should job windows be cleaned up on orcai exit, or left for manual inspection? (Leaning: leave them; they're hidden from the status bar so they don't clutter.)
