## Why

The Switchboard activity feed floods the UI with unbounded output (no scrolling), tab navigation misaligns the agent runner panel, and there is no at-a-glance status board showing which agents are live, running, failed, or done. Debugging pipeline failures requires manual tmux archaeology because no worktree shell is preserved post-run.

## What Changes

- **Activity feed scrolling**: The feed panel becomes scroll-aware — output is capped visually and navigable with arrow keys without breaking layout.
- **Agent runner panel fix**: Tab through provider → model → prompt no longer expands or misaligns the outer bounding box; panel height is fixed and sections reflow within it.
- **Agent status board**: A new "SIGNAL BOARD" panel above the activity feed displays one LED row per agent job (running/done/failed/online) with a subtle blink animation on active jobs and keyboard-driven filter (all / running / done / failed).
- **Worktree debug popup**: Pressing enter/click on a signal board entry opens an 80%-width overlay popup. The popup embeds a live view of the agent's dedicated tmux window (already spawned by the pipeline runner).
- **Hidden pipeline tmux windows**: When the pipeline runner launches an agent job it creates a tmux window in the current session marked `hide-from-statusbar` so only the Switchboard window remains visible in the status bar. The window persists after the job finishes so the user can inspect it.
- **Popup shell integration**: The debug popup streams the captured content of the agent's tmux window and allows the user to attach/scroll, giving full post-mortem access to the agent's git worktree environment.

## Capabilities

### New Capabilities
- `activity-feed-scroll`: Bounded, scrollable activity feed panel that does not overflow the terminal.
- `agent-status-board`: Animated LED signal board above the activity feed; per-job status indicator with filter.
- `agent-debug-popup`: 80%-wide overlay showing the agent's tmux window content; keyboard dismiss.
- `pipeline-hidden-tmux-windows`: Pipeline runner creates per-job tmux windows hidden from the status bar; windows persist post-run for debugging.

### Modified Capabilities
- `agent-context-panel`: Agent runner tab navigation and outer panel sizing must not reflow the parent container when stepping through form steps.

## Impact

- `internal/switchboard/switchboard.go` — major: feed scroll state, signal board model, popup overlay, key routing
- `internal/switchboard/` — new files: `signal_board.go`, `debug_popup.go`
- `internal/pipeline/` — pipeline runner gets tmux window spawning via `mcp__tmux` or `os/exec tmux` calls
- `cmd/pipeline.go` — thread tmux session/window name through to pipeline runner
- No new external dependencies; tmux is already a hard runtime requirement
