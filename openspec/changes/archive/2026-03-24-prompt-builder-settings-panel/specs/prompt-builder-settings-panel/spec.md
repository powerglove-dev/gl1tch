## ADDED Requirements

### Requirement: Sidebar mode toggle
The prompt builder sidebar SHALL support two modes: `steps` (default) and `settings`. The user SHALL be able to toggle between modes by pressing `,`. The active mode SHALL be visually indicated in the sidebar header.

#### Scenario: Toggle from steps to settings
- **WHEN** the sidebar is in `steps` mode and the user presses `,`
- **THEN** the sidebar switches to `settings` mode and renders the settings panel

#### Scenario: Toggle from settings to steps
- **WHEN** the sidebar is in `settings` mode and the user presses `,`
- **THEN** the sidebar switches to `steps` mode and renders the steps list

#### Scenario: Default mode on open
- **WHEN** the prompt builder opens
- **THEN** the sidebar is in `steps` mode

### Requirement: Settings panel fields
The settings panel SHALL expose three editable fields: pipeline `name`, `description`, and `tags`. Each field SHALL be a `textinput` bubble. Focus SHALL cycle through the fields via `Tab` (forward) and `Shift+Tab` (backward) when the sidebar is in `settings` mode.

#### Scenario: Name field pre-populated
- **WHEN** the settings panel opens
- **THEN** the `name` input is pre-populated with the current pipeline name

#### Scenario: Tab cycles through settings fields
- **WHEN** the sidebar is in `settings` mode and the user presses `Tab`
- **THEN** focus advances to the next settings field, wrapping from `tags` back to `name`

#### Scenario: Shift+Tab cycles backwards
- **WHEN** the sidebar is in `settings` mode and the user presses `Shift+Tab`
- **THEN** focus moves to the previous settings field, wrapping from `name` back to `tags`

### Requirement: Settings fields update model
Changes to the settings inputs SHALL be reflected immediately in the `Model` state. The pipeline `name`, `description`, and `tags` SHALL be updated as the user types.

#### Scenario: Name input updates model
- **WHEN** the user edits the `name` input in the settings panel
- **THEN** `model.name` reflects the current input value

#### Scenario: Description input updates model
- **WHEN** the user edits the `description` input in the settings panel
- **THEN** `model.description` reflects the current input value

#### Scenario: Tags input updates model
- **WHEN** the user edits the `tags` input (comma-separated) in the settings panel
- **THEN** `model.tags` is updated by splitting the input on commas and trimming whitespace

### Requirement: Persist description and tags to pipeline YAML
When the pipeline is saved, the `description` and `tags` fields SHALL be written to the `.pipeline.yaml` file. Empty `description` SHALL be omitted (`omitempty`). Empty `tags` slice SHALL be omitted.

#### Scenario: Description saved to YAML
- **WHEN** the user saves a pipeline with a non-empty description
- **THEN** the `.pipeline.yaml` contains a `description:` field with the entered value

#### Scenario: Tags saved as YAML sequence
- **WHEN** the user saves a pipeline with tags `"go, tui, ai"`
- **THEN** the `.pipeline.yaml` contains a `tags:` field as a YAML sequence: `["go", "tui", "ai"]`

#### Scenario: Empty description omitted
- **WHEN** the user saves a pipeline with an empty description
- **THEN** the `.pipeline.yaml` does not contain a `description:` field

### Requirement: Sidebar mode does not affect right pane behavior
When the sidebar is in `settings` mode, the right config pane SHALL remain rendered but not receive keyboard input. Tab/Shift+Tab SHALL be consumed by the settings panel only.

#### Scenario: Right pane inactive during settings mode
- **WHEN** the sidebar is in `settings` mode and the user presses `Tab`
- **THEN** the right config pane field focus does not change
