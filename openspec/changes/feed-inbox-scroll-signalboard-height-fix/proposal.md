## Why

Scrolling in the activity feed and inbox detail is broken: the cursor/line selector moves off-screen and scrolling stops before reaching the last line of content. Additionally, the signal board grows unboundedly with the number of running agents, stealing vertical space from the feed. These issues make the UI unusable when monitoring many agents or long step output.

## What Changes

- **Activity feed scroll fix**: Correct the `clampFeedScroll` height constant (+6 vs +9 mismatch with `View()`) so scroll bounds match the actual rendered panel height; ensure the cursor is always clamped within the visible viewport after navigation.
- **Activity feed live recalculation**: Add a mechanism (tick/subscription) so the logical-to-visual line map is recalculated whenever new feed entries arrive, keeping scroll bounds accurate as content grows.
- **Inbox detail scroll fix**: Fix scroll/cursor coupling so navigating to the last line does not push the cursor past the visible viewport; visible area must always include the cursor line.
- **Signal board fixed height**: Replace the dynamic `len(m.feed)+9` growth formula with a fixed height that accommodates at least 20 agent rows regardless of how many agents are running.

## Capabilities

### New Capabilities
- `feed-scroll-clamp`: Correct, consistent viewport clamping for the activity feed so the cursor always stays within the visible area and scrolling reaches the last content line.
- `inbox-detail-scroll-clamp`: Correct viewport clamping for inbox detail so the cursor is never rendered outside the visible panel.
- `signal-board-fixed-height`: Signal board uses a fixed minimum height (≥20 body rows) rather than growing with feed length.

### Modified Capabilities
- `feed-step-output`: Scroll and viewport behavior for feed step output changes (cursor clamping, live height recalculation).

## Impact

- `internal/switchboard/switchboard.go` — `clampFeedScroll()`, `View()`, feed scroll/cursor navigation handlers
- `internal/switchboard/inbox_detail.go` — scroll/cursor navigation, viewport clamp
- `internal/switchboard/signal_board.go` — `signalBoardVisibleRows()`, height formula
