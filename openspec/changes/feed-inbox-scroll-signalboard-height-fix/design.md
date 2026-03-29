## Context

The activity feed, inbox detail, and signal board panels all render inside a fixed vertical space determined by the BubbleTea window height. Three separate bugs exist:

1. **Activity feed** — `clampFeedScroll()` calculates `sbHeight` with `len(m.feed)+6`, but `View()` uses `len(m.feed)+9`. The 3-row mismatch makes `feedH` (and therefore `visibleH`) larger in `View()` than in clamp, so the cursor can be clamped to a position that is actually off the bottom of the rendered panel. Additionally, the logical-to-visual map is only rebuilt during rendering; when new feed entries arrive between renders, stale bounds allow the cursor to escape the viewport.

2. **Inbox detail** — Navigation sets `m.inboxDetailScroll = m.inboxDetailCursor` directly (scroll follows cursor 1:1). When the cursor is near the last lines and `visibleH` is smaller than the distance from the scroll anchor to the cursor, the cursor renders below the visible window. The clamp in `viewInboxDetail()` corrects `offset` but the cursor highlight is drawn at `absIdx = offset + i` so it still falls outside `0..visibleH-1`.

3. **Signal board** — Height formula `min(len(m.feed)+9, maxSB)` grows the panel with every new feed entry. At 20+ agents the panel can consume most of the screen. It should be a fixed value that is large enough for 20 body rows plus headers/footer.

## Goals / Non-Goals

**Goals:**
- Fix feed cursor/scroll so the highlighted line is always visible and scrolling reaches the last content line.
- Add a tick-driven recalculation so the feed's logical-to-visual map stays current when new entries arrive.
- Fix inbox detail so the cursor is always inside the rendered viewport.
- Fix signal board height to a constant that fits ≥20 body rows.

**Non-Goals:**
- Smooth animated scrolling.
- Independent horizontal scrolling.
- Changing the mark/cursor visual style.
- Altering signal board scroll or selection behavior.

## Decisions

### D1 — Unify the height constant in clampFeedScroll and View

**Decision:** Extract a single `feedPanelHeight(m, contentH)` helper that both `View()` and `clampFeedScroll()` call. The helper returns the same `feedH` value regardless of call site.

**Alternatives considered:**
- Inline-fix only `clampFeedScroll` to use +9: fragile, will drift again.
- Store `feedH` on the model and update it on resize: adds model state; the helper is simpler and stateless.

### D2 — Cursor-aware scroll clamping

**Decision:** After any cursor move, run `clampFeedScroll` which ensures:
```
if cursor < scrollOffset → scrollOffset = cursor
if cursor >= scrollOffset + visibleH → scrollOffset = cursor - visibleH + 1
scrollOffset = clamp(scrollOffset, 0, max(0, totalVisualLines - visibleH))
```
This is the standard "keep cursor in view" algorithm used by text editors.

**Alternatives considered:**
- `scroll = cursor` (current): loses independent scrolling.
- Page-based: only jumps; doesn't allow smooth navigation to last line.

### D3 — Tick-driven logical-to-visual recalculation

**Decision:** Send a lightweight `feedRecalcMsg` on each `tea.Tick` (reuse existing tick if present, otherwise add a 200ms ticker). The Update handler for this message rebuilds the logicalToVisual map from `m.feed` and re-runs clamp. This keeps bounds fresh without blocking the render path.

**Alternatives considered:**
- Rebuild on every render: already happens, but clamp runs before render so stale bounds persist for one frame.
- Rebuild on every `feedAppend`: requires threading msg through all append call sites.

### D4 — Inbox detail: clamp cursor before drawing

**Decision:** In `viewInboxDetail()`, after clamping `offset`, also clamp `m.inboxDetailCursor` so it is always in `[offset, offset+visibleH-1]`. The cursor clamp is applied to a local copy for rendering; the model cursor is clamped in the navigation handlers instead.

**Decision:** In navigation handlers (`j`/`k`/pgdn/pgup`), use the standard keep-cursor-in-view scroll algorithm (D2) instead of `scroll = cursor`.

### D5 — Signal board fixed height

**Decision:** Replace `min(len(m.feed)+9, maxSB)` with a constant `sbFixedBodyRows = 20` plus the header/footer rows count. The formula becomes:
```
sbHeight = headerRows + sbFixedBodyRows + 1  // +1 for boxBot
```
Cap at `contentH - 3` as a safety guard so the feed panel always has at least 3 rows.

**Alternatives considered:**
- Keep the dynamic formula but with a higher cap: still unpredictable; fixed is simpler.
- Make it configurable: YAGNI, 20 rows covers the stated requirement.

## Risks / Trade-offs

- **D3 tick overhead**: A 200ms ticker adds minimal CPU overhead. If the feed already has a ticker, reuse it to avoid the extra goroutine.
- **D5 screen space**: A fixed 20-row signal board on a small terminal (< 30 rows) may leave very little feed space. The `contentH - 3` safety cap prevents the panel from fully displacing the feed. On typical 40+ row terminals this is fine.
- **D4 local cursor clamp**: Clamping only for render means the model cursor can temporarily be out-of-viewport until the next navigation. This is acceptable — the cursor will snap back on the next keypress.

## Open Questions

- Should the feed ticker be a dedicated `feedHeightRecalcTicker` or reuse an existing app-wide ticker? Prefer reuse to avoid goroutine proliferation.
