## Context

The Jump Window (`internal/jumpwindow/jumpwindow.go`) is a self-contained BubbleTea program launched as a full-screen alt-screen popup via `jumpwindow.Run()`. It currently renders a single vertical column: search input at top, sysop entries below, then active job entries. The list grows vertically with content, causing the modal to be unnecessarily tall when there are few items. A two-column layout will put sysop on the left and active jobs on the right, keeping the modal height bounded and the two concerns visually separated.

## Goals / Non-Goals

**Goals:**
- Render sysop and active jobs in side-by-side columns below the search input
- Tab key cycles focus: left column (sysop) → right column (active jobs) → back
- Search input filters only active jobs; sysop list is always fully shown
- Hint bar includes `tab` as a navigation hint
- Modal height is determined by `max(len(sysop), len(filtered))` rows, not their sum

**Non-Goals:**
- No changes to tmux interaction logic (switch-client, select-window)
- No changes to how windows are listed or labelled
- No scrolling within columns; all items must fit within the visible column height

## Decisions

### Focus tracking via `focusCol int` field

Add `focusCol int` (0 = left/sysop, 1 = right/active jobs) and `selectedSysop int` / `selectedJob int` to replace the single `selected int`. Each column tracks its own cursor independently.

**Alternatives considered:**
- Keep a single `selected int` with an offset: rejected because split rendering makes the unified index confusing and error-prone when columns have different heights.

### Column width split

Split available width 50/50 minus 3 chars for the centre divider glyph (`│`). If the terminal is very narrow (< 40 cols), fall back to the existing single-column layout.

**Alternatives considered:**
- Fixed left-column width (e.g., 20 cols): rejected because sysop entry names vary and a fixed width would clip them on wider terminals.
- 40/60 split: considered but 50/50 matches the symmetry of the two equal-importance sections.

### Search scope

`applyFilter()` continues to filter `m.windows → m.filtered` as before. Sysop entries (`m.sysop`) are never filtered. This is a behavioural clarification, not a structural change.

### Modal height fix

Currently `View()` emits one `BoxRow` per list item for both sections sequentially, so height = `len(sysop) + len(filtered) + overhead`. With two columns the height = `max(len(sysop), len(filtered)) + overhead`. The rendering loop iterates `rowCount = max(len(m.sysop), len(m.filtered))` times and prints a blank cell for the shorter column.

### Hint bar addition

Add `hint("tab", "switch col")` to the hint bar between `j/k nav` and `enter select`.

## Risks / Trade-offs

- [ANSI width accounting] Lipgloss `.Width()` counts runes, not ANSI escape bytes. Column cells must be padded to fixed rune-width to keep the divider column aligned. Mitigation: use `panelrender.BoxRow` only for full-width rows; build column cells with explicit rune-width padding via `lipgloss.NewStyle().Width(colW).Render(...)` or manual space padding.
- [Very narrow terminals] A terminal narrower than 40 cols breaks the 50/50 layout. Mitigation: single-column fallback when `w < 40`.
- [No scrolling] If either list is very long the bottom items will be clipped. Mitigation: out of scope for this change; acceptable given typical sysop list size (1–5 items).
