## ADDED Requirements

### Requirement: Switchboard top header bar has one padding row below it
The Switchboard SHALL render a single blank row between the top header bar and the panel body area.

#### Scenario: Blank padding row present in Switchboard layout
- **WHEN** the Switchboard is rendered at any terminal size
- **THEN** there is exactly one blank row separating the ORCAI title bar from the first panel row

### Requirement: Cron TUI displays top header bar
The Cron TUI SHALL render a full-width ORCAI title bar as the first row of its view, styled identically to the Switchboard top header bar.

#### Scenario: Cron TUI shows title bar
- **WHEN** the Cron TUI is rendered at any terminal size
- **THEN** the first visible row is the full-width ORCAI header bar with themed accent background

#### Scenario: Cron TUI header bar followed by padding row
- **WHEN** the Cron TUI is rendered
- **THEN** a single blank row separates the header bar from the jobs pane

### Requirement: Switchboard theme picker provides live preview
The Switchboard theme picker SHALL update all panel colors in real time as the user navigates between theme options, without requiring the user to press enter first.

#### Scenario: Panel colors change on theme cursor move
- **WHEN** the theme picker is open in Switchboard
- **AND** the user moves the cursor to a different theme
- **THEN** the Switchboard panels immediately re-render using the highlighted theme's colors

#### Scenario: Original theme restored on cancel
- **WHEN** the theme picker is open
- **AND** the user presses esc
- **THEN** the panels revert to the theme that was active before the picker was opened

### Requirement: Cron entry rename publishes busd event
When a cron entry is renamed via the Cron TUI edit overlay, the system SHALL publish a `cron.entry.updated` event on the busd with the old and new entry names.

#### Scenario: Rename triggers event
- **WHEN** the user saves an edited cron entry with a different name
- **THEN** a `cron.entry.updated` event is published containing `old_name` and `new_name`

#### Scenario: Non-rename edit does not publish update event
- **WHEN** the user saves an edited cron entry without changing the name
- **THEN** no `cron.entry.updated` event is published

### Requirement: Cron TUI cron jobs pane supports opening pipeline file
The Cron TUI cron jobs pane SHALL support pressing `p` to open the selected job's pipeline YAML file in `$EDITOR`.

#### Scenario: Press p opens pipeline file
- **WHEN** the cron jobs pane is focused
- **AND** a job entry of kind `pipeline` is selected
- **AND** the user presses `p`
- **THEN** the TUI suspends and opens the pipeline YAML file in `$EDITOR` (falling back to `vi`)

#### Scenario: Hint bar shows p pipeline
- **WHEN** the cron jobs pane is active and not filtering
- **THEN** the hint bar includes `p pipeline`

#### Scenario: p is a no-op for non-pipeline kinds
- **WHEN** a job entry of kind other than `pipeline` is selected
- **AND** the user presses `p`
- **THEN** nothing happens
