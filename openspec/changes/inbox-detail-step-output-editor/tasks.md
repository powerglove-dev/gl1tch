## 1. Step Output in Inbox Detail

- [x] 1.1 In `buildRunContent`, after writing each step badge line, extract `StepRecord.Output["value"]` as a string, split on newlines, take the last 5, and write each as an indented dim line
- [x] 1.2 Write a unit test in `inbox_detail_test.go` (or existing test file) covering: step with output shows last 5 lines, step with empty output shows no lines, step with exactly 5 lines shows all 5

## 2. Clipboard Helper

- [x] 2.1 Add `readClipboard() string` helper in `internal/switchboard/` (or a shared internal package): run `pbpaste` on darwin, `xclip -o -selection clipboard` on linux (fallback `xsel --clipboard --output`); return empty string on any error
- [x] 2.2 Add `clipboardSnapshot() string` convenience alias used to capture pre-launch clipboard state

## 3. Editor Launch

- [x] 3.1 Add `editorCmd() (string, bool)` helper: return `$EDITOR` if set, else check `vi` on PATH; return `("", false)` if neither found
- [x] 3.2 In `switchboard.go`, handle `key.Matches(msg, key.NewBinding(key.WithKeys("e")))` when `m.inboxDetailOpen`: snapshot clipboard, write ANSI-stripped run content to a temp file (`os.CreateTemp("", "orcai-inbox-*.txt")`), store temp path and clipboard snapshot on model, launch via `tea.ExecProcess`
- [x] 3.3 If `editorCmd` returns false, show a flash/status message "set $EDITOR to use this feature" and do not launch editor
- [x] 3.4 Add `inboxEditorTempFile string` and `inboxEditorClipSnapshot string` fields to the `Model` struct

## 4. Post-Editor Injection

- [x] 4.1 Handle `tea.ExecMsg` (or `tea.ExecDoneMsg`) in the switchboard Update: read clipboard, compare to snapshot
- [x] 4.2 If clipboard changed and non-empty: set `m.agent.prompt.SetValue(clipboardContent)`, open agent modal, focus prompt
- [x] 4.3 Else if temp file content differs from original written content: read temp file, inject into agent prompt, open agent modal
- [x] 4.4 Else: restore inbox detail as-is (no modal)
- [x] 4.5 Delete the temp file (`os.Remove`) in all branches after reading

## 5. Hint Bar & UX

- [x] 5.1 Add `{Key: "e", Desc: "editor"}` to the inbox detail hint list in `viewInboxDetail`
- [x] 5.2 Verify the hint bar still fits at standard terminal widths (80, 120 cols) with the new hint added
