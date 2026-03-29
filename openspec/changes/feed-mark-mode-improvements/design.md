## Context

The Activity Feed (`viewActivityFeed`) and Inbox Detail panel (`viewInboxDetail`) share a line-marking feature. Currently pressing `m` toggles a mark on the cursor line in place — the cursor doesn't move and the user must press `m` + `j` alternately to mark multiple lines. The cursor indicator (`> `) is implemented by prepending two characters and reducing available content width by 2, which is correct for width but creates a visual shift relative to unmarked lines since the content origin moves right. The cursor color is hardcoded to `aBrC` (bright cyan) which does not follow the active theme. Steps in the feed are rendered with fixed-string indentation (`"  "` / `"    "`) with no tree connectors. Done steps with no output still render their badge line.

## Goals / Non-Goals

**Goals:**
- Mark mode state machine: `m` cycles active → paused, paused → active; `j`/`k` in active mode marks/unmarks the line before moving the cursor
- Cursor overlay: `>` occupies the same visual column as regular content with no width change to content area; color from `ANSIPalette.Accent`
- Tree-connector nesting: `├`/`└` glyphs for steps, content indented beneath
- Suppress done steps with no output lines from feed render

**Non-Goals:**
- Changing dispatch (`r`) or any other mark-mode behavior
- Applying mark mode to the Signal Board or other panels
- Infinite mark stack / undo — mark state is cleared on dispatch as today

## Decisions

### 1. Mark mode as a tri-state on the Model

Add `feedMarkMode` (and `inboxMarkMode`) as an int or iota type cycling: `markModeOff → markModeActive → markModePaused → markModeActive → …`. When `markModeActive`, `j`/`k` handlers mark/unmark the current line then move the cursor. When `markModePaused`, `j`/`k` move normally. When `markModeOff`, `j`/`k` always move normally and `m` enters `markModeActive`.

**Alternatives considered:**
- Two booleans (`isMarking`, `isPaused`): equivalent but less clear as a state machine. iota enum is self-documenting.
- Keeping `m` as a single-line toggle and adding a separate key for range select: breaks existing muscle memory and the user explicitly requested the cycle.

### 2. Cursor overlay without width change

Replace the `cursorMark` prepend approach with an in-place overlay: measure the first 2 visible characters of `content`, replace them with `"> "`, and keep the content width unchanged. If content is shorter than 2 chars, pad with spaces then overlay. This means no adjustment to `availForContent` — the content occupies the same width either way.

**Alternatives considered:**
- Keep prepend but shift the inner width calculation: already done, but the visual effect is that the content "slides" right relative to non-cursor rows since the leading spaces that indent step text are eaten. The overlay approach keeps column alignment stable.

### 3. Tree connectors for steps

Replace `stepIndent + col + stepGlyph + " " + step.id` with a connector-aware prefix. Use `├ ` for all steps except the last visible step, which uses `└ `. Output lines beneath each step use `│ ` on the left to form a visual tree.

**Alternatives considered:**
- Pure indentation (current): works but does not visually separate entries from steps.
- Box-drawing with colors: over-engineered; single-char connectors at `pal.Dim` suffice.

### 4. Suppress done steps with no output

In the step render loop, skip `appendRow` for the step badge when `step.status == "done"` and `len(step.lines) == 0`. Running, failed, and pending steps always render their badge regardless of output.

**Alternatives considered:**
- Hide all done steps (even with output): loses useful information about what ran.
- Only hide the last step if named "done": fragile — depends on pipeline convention.

## Risks / Trade-offs

- [Overlay cursor may clip ANSI-heavy content at column 0] → Mitigation: strip ANSI before slicing, then overlay plain `"> "` with color wrap. Already done by the existing `lipgloss.Width` path.
- [Tree connectors require knowing which step is "last"] → Mitigation: pre-scan `entry.steps` to find the last non-suppressed step index before entering the render loop.
- [Mark mode state persists across focus changes] → Mitigation: reset `feedMarkMode` to `Off` when feed loses focus, same as clearing selection on panel switch.
