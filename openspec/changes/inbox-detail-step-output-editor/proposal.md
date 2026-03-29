## Why

The inbox detail view shows steps with status badges but omits step output, requiring the user to cross-reference the activity feed to understand what each step produced. Additionally, there is no way to extract run content into an external editor for selection and injection into the agent runner prompt — the mark-and-dispatch workflow requires manual line-by-line marking inside the TUI.

## What Changes

- Step output (`output.value`) is rendered beneath each step badge in the inbox detail view, consistent with how the activity feed already displays step output.
- A new `e` keybinding in the inbox detail opens the full run content in `$EDITOR` via `tea.ExecProcess` (suspending the TUI). When the editor exits, the system clipboard is checked; if it contains text that was not present before the editor launched, that text is injected into the agent runner prompt and the agent modal is opened.
- If the clipboard is empty on return, the editor's saved file content (stripped of the original boilerplate) is used as the injected context instead.

## Capabilities

### New Capabilities
- `inbox-step-output`: Per-step output rendered beneath step badges in the inbox detail view, sourced from `StepRecord.Output["value"]`, last 5 lines, matching the feed-step-output display pattern.
- `inbox-editor-dispatch`: Open inbox detail content in `$EDITOR` (via `tea.ExecProcess`), capture yanked/clipboard text on return, and inject it into the agent runner prompt.

### Modified Capabilities

## Impact

- `internal/switchboard/inbox_detail.go` — `buildRunContent` extended to render step output lines beneath each step badge.
- `internal/switchboard/switchboard.go` — new `e` key handler in inbox detail state; `tea.ExecProcess` call; clipboard read on `ExecDoneMsg`; agent modal injection.
- No new dependencies for step output. Clipboard read on macOS uses `pbpaste`; Linux uses `xclip`/`xsel` via a small helper (already available via exec).
