## 1. Column Width Math

- [x] 1.1 Add `rightColWidth()` helper: `w * 25 / 100`, min 20
- [x] 1.2 Update `leftColWidth()` docstring to note three-column context (no logic change)
- [x] 1.3 Add `midColWidth()` helper: `w - leftW - rightW - 4` (two 2-char gutters), min 10
- [x] 1.4 Update `feedPanelWidth()` to return `rightColWidth()` instead of the two-column formula

## 2. Remove Agent Runner from Left Column

- [x] 2.1 Remove `buildAgentSection()` call from `viewLeftColumn()`
- [x] 2.2 Verify `viewLeftColumn()` still fills to `height` correctly after removal

## 3. Center Column Render Function

- [x] 3.1 Create `viewCenterColumn(height, width int) []string` that renders agents grid (top) then a blank line then agent runner section (bottom)
- [x] 3.2 Clamp combined height so center column does not exceed `contentH`

## 4. Agents Grid Panel

- [x] 4.1 Add `agentsGridRow int` and `agentsGridCol int` fields to `Model` struct
- [x] 4.2 Add `agentsCenterFocused bool` field to track focus for the center panel
- [x] 4.3 Create `buildAgentsGrid(height, width int) []string` that renders agent cards in a grid; derive `gridCols = max(1, width/24)`
- [x] 4.4 Render each card with agent name + status badge on one line; highlight selected card with accent color
- [x] 4.5 Update panel header key from `"signal_board"` to `"agents"` in the center panel render
- [x] 4.6 Wire `h`/`j`/`k`/`l` keys to move `agentsGridRow`/`agentsGridCol` when center panel is focused
- [x] 4.7 Clamp `agentsGridRow`/`agentsGridCol` whenever the agent list length changes

## 5. Activity Feed Right Column

- [x] 5.1 Update `View()` to assemble three columns: zip-join left+center with first gutter, then zip-join result+right with second gutter
- [x] 5.2 Pass `rightColWidth()` as width to `viewActivityFeed()`
- [x] 5.3 Redesign `viewActivityFeed()` to render each entry as a centered card (`min(rightW-4, 36)` wide) — no raw output lines, only agent name, status badge, pipeline name, and step names
- [x] 5.4 Compute card indent: `max(0, (rightW - cardWidth) / 2)` using `stripANSI` widths; prepend `strings.Repeat(" ", indent)` to each card line
- [x] 5.5 Remove all `step.lines` rendering from `feedRawLines()` and `viewActivityFeed()`

## 6. ANSI Timeline Connectors

- [x] 6.1 Replace `"├ "` with `"├─ "` in feed step rendering
- [x] 6.2 Replace `"└ "` with `"└─ "` in feed step rendering

## 7. 12-Hour Timestamps

- [x] 7.1 Change `entry.ts.Format("15:04:05")` to `strings.ToLower(entry.ts.Format("3:04 PM"))` in `feedRawLines()`
- [x] 7.2 Apply the same format change to any other timestamp render sites in `viewActivityFeed()`

## 8. Narrow Terminal Fallback

- [x] 8.1 Add width guard in `View()`: if `w < 80`, skip right column and use two-column layout (left + center)

## 9. Tests and Review

- [x] 9.1 Update existing `View()`-level tests that assert on column widths or line structure
- [x] 9.2 Add test for `rightColWidth()` and `midColWidth()` helpers
- [x] 9.3 Add test: agents grid renders expected number of columns for given width
- [x] 9.4 Add test: `h`/`j`/`k`/`l` cursor movement and clamping in agents grid
- [x] 9.5 Add test: timestamp renders as `"2:34 pm"` format
- [x] 9.6 Add test: step connectors use `├─` / `└─`; no raw output lines present
- [x] 9.7 Add test: feed card is horizontally centered within right column width
- [x] 9.7 Run `go test ./internal/switchboard/...` — all tests pass
- [x] 9.8 Principal engineer review before merge to main
