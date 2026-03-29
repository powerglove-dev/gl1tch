## Why

The Activity Feed has several UX rough edges that make it tedious to work with: marking lines requires hitting `m` once per line with no navigation, the cursor indicator causes a visible content shift and uses an off-theme color, completed steps with no output clutter the feed with empty badges, and step nesting lacks clear visual hierarchy.

## What Changes

- **Mark mode cycle**: pressing `m` enters mark mode; `j`/`k` in mark mode marks/unmarks lines while advancing the cursor; pressing `m` again pauses marking (j/k navigates without marking); pressing `m` again resumes marking
- **Cursor overlay**: the `> ` cursor indicator overlays on the line without shifting content right; its color uses the theme accent matching other panels
- **Step nesting**: steps render with tree connectors (├/└) and step output is indented beneath each step for clear visual hierarchy
- **Suppress empty done steps**: steps with status "done" and no output lines are omitted from the feed render — they add no information

## Capabilities

### New Capabilities
- `feed-mark-mode`: mark mode toggle cycle — `m` cycles through active/paused states; j/k marks/unmarks during active mark mode and navigates without marking during paused mode
- `feed-cursor-style`: cursor `>` overlays without layout shift; uses theme accent color consistent with other panels

### Modified Capabilities
- `feed-step-output`: suppress rendering of done steps that have no output lines; add tree-connector nesting (├/└) for steps and indented output beneath each step

## Impact

- `internal/switchboard/switchboard.go`: mark mode state machine, cursor rendering (`boxRowCursorColor`), feed step render loop
- `internal/switchboard/inbox_detail.go`: same mark mode changes for inbox detail panel
- `openspec/specs/feed-step-output/spec.md`: updated requirements for step suppression and nesting
