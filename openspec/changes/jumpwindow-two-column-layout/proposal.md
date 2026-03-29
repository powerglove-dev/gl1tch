## Why

The Jump Window lists sysop entries and active jobs in a single vertical column, which mixes navigation concerns, causes the modal to be unnecessarily tall, and makes search filter across everything instead of just the actionable jobs. A two-column layout with explicit focus control improves spatial clarity and makes the window more usable at a glance.

## What Changes

- Replace the single-column sysop + active-jobs list with a side-by-side two-column layout (sysop left, active jobs right)
- Add tab key to cycle keyboard focus between the two columns (left pane / right pane)
- Restrict search input to filter only active jobs; sysop entries are always fully visible
- Add `tab` navigation hint to the hint bar
- Fix modal height: eliminate the unnecessary blank rows that make the panel taller than its content requires

## Capabilities

### New Capabilities

- `jumpwindow-two-column`: Two-column sysop/active-jobs layout with tab focus cycling and scoped search

### Modified Capabilities

<!-- none -->

## Impact

- `internal/jumpwindow/jumpwindow.go` — model struct, Update(), View(), applyFilter()
- No external API or protocol changes; all changes are UI-only within the jump window program
