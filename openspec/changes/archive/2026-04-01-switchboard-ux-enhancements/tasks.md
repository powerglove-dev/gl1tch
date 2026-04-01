## 1. Model & State

- [x] 1.1 Add `confirmDelete bool` and `pendingDeletePipeline string` fields to `Model` for delete confirmation state
- [x] 1.2 Add `agentModalOpen bool` field to `Model` to track overlay modal visibility
- [x] 1.3 Remove `formStep` multi-step wizard state from `agentSection`; retain `selectedProvider`, `selectedModel`, and `prompt` textarea
- [x] 1.4 Ensure `selectedProvider` and `selectedModel` survive modal close/reopen (no reset on close)

## 2. [p] Pipelines Focus Shortcut

- [x] 2.1 Add `case "p"` at the top of the `Update` key switch (before panel-local handlers) that sets `launcher.focused = true` and unfocuses all other panels
- [x] 2.2 Add `p pipelines` to the status bar hint line string in `viewStatusBar` (or equivalent render helper)

## 3. [n] New Pipeline

- [x] 3.1 Add `case "n"` in the `Update` key switch when `launcher.focused` is true: generate a timestamped new-file path under `~/.config/orcai/pipelines/`
- [x] 3.2 Write a minimal pipeline YAML template to the new file path before opening the editor
- [x] 3.3 Resolve `$EDITOR` env var; fall back to `vi` if unset
- [x] 3.4 Execute `tmux new-window -d -n orcai-edit "$EDITOR <path>"` via `os/exec`
- [x] 3.5 Trigger a pipeline list refresh (re-discover pipelines from disk) after the window returns — use the existing pipeline discovery helper

## 4. [e] Edit Pipeline

- [x] 4.1 Add `case "e"` in the `Update` key switch when `launcher.focused` is true and a pipeline is selected: resolve the pipeline's full file path
- [x] 4.2 Execute `tmux new-window -d -n orcai-edit "$EDITOR <path>"` (reuse the editor-launch helper from 3.3–3.4)
- [x] 4.3 Trigger a pipeline list refresh after the window returns
- [x] 4.4 Wire `e` in the jump window (`jumpwindow.go`) to open the selected pipeline entry in the editor

## 5. [d] Delete Pipeline with Confirmation Modal

- [x] 5.1 Add `case "d"` in the `Update` key switch when `launcher.focused` is true and a pipeline is selected: set `confirmDelete = true` and `pendingDeletePipeline = <name>`
- [x] 5.2 When `confirmDelete` is true, route all key events to the delete-modal handler before any other handler
- [x] 5.3 In the delete-modal handler: `y` → `os.Remove` the file, refresh pipeline list, clear `confirmDelete`; any other key → clear `confirmDelete` without deleting
- [x] 5.4 Render the delete confirmation modal in `View()` as a centred overlay showing the pipeline name and full file path with `[y]es / [n]o` prompt

## 6. Agent Runner Overlay Modal

- [x] 6.1 Add an `openAgentModal()` helper that sets `agentModalOpen = true` and focuses the prompt textarea
- [x] 6.2 In `Update`, when `agentModalOpen` is true, route ALL key events to the modal handler and return early (no panel key events leak through)
- [x] 6.3 Modal key handler: arrow keys navigate provider/model lists; `tab` cycles focus between prompt textarea and selection lists; `ESC` closes modal without submitting; `ctrl+s` submits if provider, model, and non-empty prompt are set
- [x] 6.4 Remove the `enter` handling that advanced `formStep` in the old inline wizard; replace with `enter` on the agent runner panel opening the modal
- [x] 6.5 Render the agent modal in `View()` as a centred overlay (min 60 cols) with labelled sections: PROVIDER list, MODEL list, PROMPT textarea
- [x] 6.6 Add terminal-width guard: if `m.width < 62`, skip the modal and show a status message instead

## 7. Feed Scroll Indicators

- [x] 7.1 In `viewActivityFeed` (or the feed box title builder), compute `hasAbove = feedScrollOffset > 0` and `hasBelow` based on total rendered lines vs. visible height
- [x] 7.2 Append the appropriate glyph to the feed box title: `↑` when only above, `↓` when only below, `↕` when both, nothing when neither

## 8. Remove Trailing Blank Rows in Pipelines Panel

- [x] 8.1 In the pipelines panel body renderer, remove any blank-row padding loop that fills rows up to a fixed height; render only as many rows as there are entries

## 9. Tests

- [x] 9.1 Add a test asserting `p` keypress focuses the launcher regardless of prior focus state
- [x] 9.2 Add tests for `d` keypress: modal shown, `y` deletes (mock file), other key cancels
- [x] 9.3 Add a test asserting `agentModalOpen` is set on `enter` from agent runner and `ESC` clears it
- [x] 9.4 Add a test for scroll indicator logic: `hasAbove`/`hasBelow` computed correctly from `feedScrollOffset` and feed entry count
