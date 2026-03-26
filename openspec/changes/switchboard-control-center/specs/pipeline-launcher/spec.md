## ADDED Requirements

### Requirement: Pipeline launcher lists saved pipelines
The left column's Pipeline Launcher section SHALL list all `*.pipeline.yaml` files found in `~/.config/orcai/pipelines/`. The list SHALL update when the user presses the refresh key (`r`). If the directory is empty or does not exist, the section SHALL show an "no pipelines saved" placeholder.

#### Scenario: Pipelines listed from config dir
- **WHEN** `~/.config/orcai/pipelines/` contains one or more `*.pipeline.yaml` files
- **THEN** each pipeline name (filename minus `.pipeline.yaml`) appears as a selectable row

#### Scenario: Empty state shown when no pipelines
- **WHEN** `~/.config/orcai/pipelines/` is empty or does not exist
- **THEN** the launcher shows "no pipelines saved"

#### Scenario: Refresh reloads pipeline list
- **WHEN** the user presses `r`
- **THEN** the launcher re-scans `~/.config/orcai/pipelines/` and updates the list

### Requirement: Enter on a pipeline row launches it
Pressing `Enter` on a selected pipeline row SHALL start the pipeline via `pipeline.Run(...)` in a background goroutine. Output SHALL be streamed to the Activity Feed as it arrives. Only one pipeline or agent job SHALL run at a time; the launcher SHALL be disabled while a job is active.

#### Scenario: Pipeline launches on Enter
- **WHEN** the user selects a pipeline row and presses `Enter`
- **THEN** the pipeline starts running and output appears in the activity feed

#### Scenario: Launcher disabled while job is active
- **WHEN** a pipeline is currently running
- **THEN** pressing `Enter` on any row has no effect and the launcher shows a `[running]` badge

#### Scenario: Pipeline completion updates activity feed
- **WHEN** a pipeline finishes successfully
- **THEN** the activity feed entry for that run shows a `✓ done` badge

#### Scenario: Pipeline failure updates activity feed
- **WHEN** a pipeline exits with an error
- **THEN** the activity feed entry shows a `✗ failed` badge and the error message

### Requirement: Pipeline launcher is keyboard-navigable
The user SHALL navigate the pipeline list with `↑`/`↓` or `j`/`k`. The selected row SHALL be highlighted with Dracula selection colors.

#### Scenario: Arrow keys move selection
- **WHEN** the pipeline list has multiple entries and the user presses `↓`
- **THEN** selection moves to the next row

#### Scenario: Selected row highlighted
- **WHEN** a row is selected
- **THEN** it renders with the Dracula selection background color
