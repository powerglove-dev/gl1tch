## 1. Mark Mode State Machine

- [x] 1.1 Add `feedMarkMode` iota type (`markModeOff`, `markModeActive`, `markModePaused`) to Model in switchboard.go
- [x] 1.2 Add `inboxMarkMode` field (same type) to Model for inbox detail panel
- [x] 1.3 Update `m` key handler in feed: cycle `Off → Active → Paused → Off` (third press exits and clears marks)
- [x] 1.4 Update `j` key handler in feed: when `markModeActive`, toggle mark on current line before moving cursor
- [x] 1.5 Update `k` key handler in feed: when `markModeActive`, toggle mark on current line before moving cursor
- [x] 1.6 Update `m` key handler in inbox detail panel with the same cycle logic
- [x] 1.7 Update `j`/`k` handlers in inbox detail panel for mark-on-navigate behavior
- [x] 1.8 Reset `feedMarkMode` to `Off` and clear marks when feed loses focus (panel switch)
- [x] 1.9 Update hint bar to reflect mark mode state (e.g., `m mark` → `m pause` when active, `m resume` when paused)

## 2. Cursor Overlay

- [x] 2.1 Rewrite `boxRowCursorColor` to overlay `> ` on the first 2 visible columns of content instead of prepending
- [x] 2.2 Change cursor color from hardcoded `aBrC` to `borderColor` parameter (which is already `pal.Accent` at call site)
- [x] 2.3 Verify cursor row visible width equals non-cursor row width at same position

## 3. Step Nesting with Tree Connectors

- [x] 3.1 Pre-scan `entry.steps` before the step render loop to identify the index of the last visible (non-suppressed) step
- [x] 3.2 Replace `stepIndent + glyph` prefix with `├ ` for non-final steps and `└ ` for the final step
- [x] 3.3 Prefix step output lines with `│   ` for output under non-final steps
- [x] 3.4 Prefix step output lines with `    ` (plain indent) for output under the final step

## 4. Suppress Empty Done Steps

- [x] 4.1 In the step render loop, skip `appendRow` for the badge when `step.status == "done"` and `len(step.lines) == 0`
- [x] 4.2 Ensure the last-step index computed in 3.1 accounts for suppressed steps (skips them when finding the last visible step)

## 6. Polish

- [x] 6.1 Update `[`/`]` page-up/down handlers: when in `MarkModeActive`, mark/unmark the current line before page-jumping (consistent with j/k)
- [x] 6.2 Update feed `appendRow` to use `●` prefix + dark background for marked rows, matching inbox detail style
- [x] 6.3 Update feed hint bar `r` hint to show mark count: `run (N)` instead of `run`

## 7. Exit, Mark All, Clear All

- [x] 7.1 `m` third press exits mark mode (Off→Active→Paused→Off) in both feed and inbox detail
- [x] 7.2 Add `A` key: mark all lines in feed when in mark mode
- [x] 7.3 Add `D` key: clear all marks in feed when in mark mode
- [x] 7.4 Add `A` key: mark all lines in inbox detail when in mark mode
- [x] 7.5 Add `D` key: clear all marks in inbox detail when in mark mode
- [x] 7.6 Show `A mark all` and `D clear` hints in feed hint bar when in mark mode
- [x] 7.7 Show `A mark all` and `D clear` hints in inbox detail hint bar when in mark mode
- [x] 7.8 Fix feed marked row: separate `●` (success color, no bg) from content (green bg), matching inbox detail

## 5. Tests

- [x] 5.1 Add test: mark mode cycles Off → Active → Paused → Off on repeated `m` presses
- [x] 5.2 Add test: `j` in active mark mode toggles mark on current line and advances cursor
- [x] 5.3 Add test: `j` in paused mark mode advances cursor without marking
- [x] 5.4 Add test: focus loss resets mark mode to Off
- [x] 5.5 Add test: done step with no output lines is not rendered in feed
- [x] 5.6 Add test: done step with output lines is rendered in feed
- [x] 5.7 Add test: cursor row visible width equals non-cursor row visible width
