## 1. Feed Height Consistency

- [x] 1.1 Extract `feedPanelHeight(m model, contentH int) int` helper in `switchboard.go` that returns the feed panel height using the same formula as `View()`
- [x] 1.2 Update `clampFeedScroll()` to call `feedPanelHeight` instead of its own inline formula (removes the +6 vs +9 mismatch)
- [x] 1.3 Update `View()` feed height calculation to call `feedPanelHeight` so both are guaranteed to agree

## 2. Feed Cursor/Scroll Clamping

- [x] 2.1 Replace the cursor navigation handlers (j/k/pgdn/pgup in feed) with keep-cursor-in-view algorithm: if cursor < scrollOffset → scrollOffset = cursor; if cursor >= scrollOffset+visibleH → scrollOffset = cursor - visibleH + 1
- [x] 2.2 After updating scrollOffset, run the existing `clampFeedScroll` to enforce bounds against total visual lines
- [x] 2.3 Verify that scrolling reaches the last content line (no blank overscroll gap at the bottom)

## 3. Feed Logical-to-Visual Recalculation Tick

- [x] 3.1 Check whether an existing tick/timer is active in the switchboard model that can be reused for the recalc message
- [x] 3.2 Add a `feedRecalcMsg` message type; in its Update handler, rebuild the logical-to-visual map from `m.feed` and re-run `clampFeedScroll`
- [x] 3.3 Wire the recalc tick: emit `feedRecalcMsg` on a 200ms interval (or reuse existing ticker), returning the next tick command from Update

## 4. Inbox Detail Scroll Fix

- [x] 4.1 Replace `m.inboxDetailScroll = m.inboxDetailCursor` assignments in navigation handlers with keep-cursor-in-view scroll algorithm
- [x] 4.2 In `viewInboxDetail()`, after clamping `offset`, add a render-time clamp on `m.inboxDetailCursor` so that cursor highlights only draw within `[offset, offset+visibleH-1]`
- [x] 4.3 Test navigation to the last line of a long inbox entry — cursor SHALL be visible

## 5. Signal Board Fixed Height

- [x] 5.1 Define constant `sbFixedBodyRows = 20` in `signal_board.go`
- [x] 5.2 Replace the `min(len(m.feed)+9, maxSB)` formula in `signalBoardVisibleRows()` with `headerRows + sbFixedBodyRows + 1` (boxBot), retaining the `contentH - 3` safety cap
- [x] 5.3 Update the matching formula in `View()` (the height budgeting block) to use the same fixed calculation so feed height is computed correctly
- [x] 5.4 Verify on a normal terminal that the signal board no longer grows when feed entries are added
