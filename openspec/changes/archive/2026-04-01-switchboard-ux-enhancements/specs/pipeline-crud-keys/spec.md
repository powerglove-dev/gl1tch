## ADDED Requirements

### Requirement: [n] creates a new pipeline file in $EDITOR
When the pipelines panel is focused, pressing `n` SHALL open `$EDITOR` (falling back to `vi` if unset) in a new tmux window with a pre-populated pipeline YAML template at a new file path derived from a timestamp (e.g. `~/.config/orcai/pipelines/new-pipeline-<timestamp>.yaml`). The TUI SHALL remain running while the editor is open. When the editor window closes, the pipeline list SHALL refresh automatically.

#### Scenario: n opens editor with template
- **WHEN** the pipelines panel is focused and the user presses `n`
- **THEN** a new tmux window is created running `$EDITOR <new-file-path>` and the new file contains a YAML pipeline template

#### Scenario: Pipeline list refreshes after editor exits
- **WHEN** the tmux editor window for a new pipeline closes
- **THEN** the pipelines panel list is refreshed and the new pipeline appears if the file was saved

#### Scenario: $EDITOR fallback
- **WHEN** `$EDITOR` is not set and the user presses `n`
- **THEN** `vi` is used as the editor

### Requirement: [e] edits the selected pipeline in $EDITOR
When the pipelines panel is focused and a pipeline is selected, pressing `e` SHALL open the selected pipeline's YAML file in `$EDITOR` in a new tmux window. The TUI SHALL remain running. When the editor window closes, the pipeline list SHALL refresh.

#### Scenario: e opens selected pipeline in editor
- **WHEN** the pipelines panel is focused with a pipeline selected and the user presses `e`
- **THEN** a new tmux window is created running `$EDITOR <selected-pipeline-path>`

#### Scenario: e is accessible from jump window
- **WHEN** the jump window is open and shows a pipeline entry
- **THEN** pressing `e` on that entry SHALL open the pipeline file in `$EDITOR` via a new tmux window

#### Scenario: e does nothing when no pipeline selected
- **WHEN** the pipelines panel is focused but the pipeline list is empty
- **THEN** pressing `e` has no effect

### Requirement: [d] deletes the selected pipeline with confirmation
When the pipelines panel is focused and a pipeline is selected, pressing `d` SHALL display a centred confirmation modal showing the pipeline name and file path, with `[y]es / [n]o` options. Pressing `y` SHALL delete the file from disk and refresh the pipeline list. Any other key SHALL dismiss the modal without deleting.

#### Scenario: d shows confirmation modal
- **WHEN** the pipelines panel is focused with a pipeline selected and the user presses `d`
- **THEN** a confirmation modal is displayed showing the pipeline name and its full file path

#### Scenario: y confirms deletion
- **WHEN** the delete confirmation modal is displayed and the user presses `y`
- **THEN** the pipeline YAML file is deleted from disk and the pipeline list is refreshed

#### Scenario: n or ESC cancels deletion
- **WHEN** the delete confirmation modal is displayed and the user presses any key other than `y`
- **THEN** the modal is dismissed and no file is deleted

#### Scenario: d does nothing when no pipeline selected
- **WHEN** the pipelines panel is focused but the pipeline list is empty
- **THEN** pressing `d` has no effect

### Requirement: [p] focuses the pipelines panel globally
Pressing `p` from anywhere in the switchboard (regardless of which panel currently has focus) SHALL immediately move focus to the pipelines panel. This mirrors the existing `a` / `f` / `s` focus shortcuts.

#### Scenario: p focuses pipelines from agent runner
- **WHEN** the agent runner panel is focused and the user presses `p`
- **THEN** the pipelines panel receives focus and the agent runner loses focus

#### Scenario: p focuses pipelines from activity feed
- **WHEN** the activity feed is focused and the user presses `p`
- **THEN** the pipelines panel receives focus and the activity feed loses focus

#### Scenario: p is shown in the status bar hint line
- **WHEN** the switchboard renders the bottom hint/status bar
- **THEN** `p pipelines` SHALL appear as a shortcut hint alongside `a agent`, `s signals`, `f feed`

### Requirement: Pipelines panel body has no trailing blank rows
The pipelines panel inner body SHALL render exactly as many rows as there are pipeline entries (plus one row for each rendered item). No blank padding rows SHALL appear below the last pipeline entry.

#### Scenario: Panel body ends at last entry
- **WHEN** the pipelines panel renders fewer entries than the panel's maximum inner height
- **THEN** the rendered body contains no blank rows below the last pipeline entry
