## Context

The inbox detail view (`inbox_detail.go`) renders run metadata, a step tree, and stdout/stderr. Steps are displayed with a status badge, ID, duration, and model — but `StepRecord.Output["value"]` is never shown. The activity feed already renders per-step output via `feed-step-output`, so the data is persisted; it just isn't surfaced in the detail view.

The agent runner modal accepts a pre-filled prompt via `m.agent.prompt.SetValue(...)`. The existing mark-and-dispatch workflow populates this prompt by requiring the user to mark individual lines with `m`. Opening the inbox content in `$EDITOR` would allow bulk selection using native editor tools (visual mode, search, etc.) and then inject the selection back without requiring line-by-line marking.

BubbleTea provides `tea.ExecProcess` to suspend the TUI, hand control to a child process (the editor), and resume when it exits — this is the standard pattern for editor integration (e.g., how `git commit` works in TUI shells).

## Goals / Non-Goals

**Goals:**
- Render `StepRecord.Output["value"]` beneath each step badge in the inbox detail view (last 5 lines, matching feed-step-output pattern).
- Add an `e` keybinding that opens the full run content in `$EDITOR` via `tea.ExecProcess`.
- On editor exit, read the system clipboard; if it contains new text, inject it into the agent runner prompt and open the agent modal.
- Fall back to reading the saved temp file if the clipboard is empty or unchanged.

**Non-Goals:**
- Modifying how step output is stored (already in `StepRecord.Output`).
- Supporting `$VISUAL` separately from `$EDITOR` (use `$EDITOR`, fallback to `vi`).
- Live clipboard polling or inter-process communication beyond reading clipboard once on exit.
- Changing the existing mark-and-dispatch workflow.

## Decisions

### 1. Source step output from `StepRecord.Output["value"]`
`StepRecord.Output` is `map[string]any`. The `"value"` key holds the string output. This mirrors the `feed-step-output` spec which reads `output.value` from the bus event that populated this field. Casting via `fmt.Sprintf("%v", v)` handles both `string` and other types safely.

**Alternative considered**: Storing output as a dedicated string field on `StepRecord`. Rejected — the existing map structure is already the contract; adding a field would require a migration.

### 2. Clipboard read via subprocess (`pbpaste` / `xclip`)
On macOS, `pbpaste` reads the clipboard. On Linux, `xclip -o -selection clipboard` or `xsel --clipboard --output`. A small `readClipboard() string` helper runs the appropriate command via `exec.Command`. No new Go dependency needed.

**Alternative considered**: A cross-platform clipboard library (e.g., `golang.design/x/clipboard`). Rejected — adds a CGo dependency with display server requirements; subprocess is simpler and already sufficient.

### 3. Clipboard snapshot before/after editor
Take a clipboard snapshot before launching the editor. After `ExecDoneMsg`, read the clipboard again. If it differs from the snapshot, the diff is the user's intended selection. If unchanged and the temp file was modified, use the temp file delta.

**Why snapshot**: Avoids injecting stale clipboard content the user yanked in a different context before opening the editor.

### 4. `tea.ExecProcess` for editor launch
`tea.ExecProcess(cmd, callback)` suspends the BubbleTea renderer, runs the editor in the terminal's full TTY, then fires an `ExecDoneMsg` when the process exits. This is the canonical BubbleTea approach and requires no manual terminal state management.

### 5. Temp file format
Write the run content as plain text (ANSI stripped) to a temp file named `orcai-inbox-<runID>-*.txt`. The file is deleted after the clipboard/content is read. The file uses the same `buildRunContent` output but with ANSI stripped so it is readable in any editor.

## Risks / Trade-offs

- **Clipboard approach assumes user uses system clipboard register** (`"+y` in vim, `M-w` in emacs). Users who only yank to internal registers will get the temp file fallback. → Mitigation: document the `e` hint to mention "yank to clipboard or save file".
- **Linux clipboard availability**: `xclip`/`xsel` may not be installed. → Mitigation: if the subprocess fails, silently fall back to temp file content.
- **Editor not set**: If `$EDITOR` is unset, fall back to `vi`. If `vi` is not found, show a flash message "set $EDITOR to use this feature".
- **Large run output**: Temp file could be large for runs with many steps and long output. → Mitigation: no truncation — editor handles large files fine; the existing 5-line-per-step display limit only applies to the TUI view.

## Open Questions

- Should the `e` keybinding be available only when the inbox detail is open, or also from the inbox list? (Proposed: detail only, consistent with existing `r` dispatch scope.)
- Should we strip ANSI from the temp file entirely, or preserve it? (Proposed: strip — editors display raw escape codes otherwise.)
