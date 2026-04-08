## ADDED Requirements

### Requirement: `/brain config` widget card
The system SHALL provide a `/brain config` slash command that returns a `widget_card` listing every editable brain and research-loop knob with its current value and an edit action.

#### Scenario: Card lists all editable knobs
- **WHEN** the user runs `/brain config`
- **THEN** the returned `widget_card` SHALL include rows for at minimum: confidence threshold, max iterations, max wallclock, max local tokens, max paid tokens, escalation policy, enabled researchers list

#### Scenario: Edit action opens an inline input
- **WHEN** the user clicks the edit action on the threshold row
- **THEN** the renderer SHALL present a constrained numeric input (range 0.0–1.0) and SHALL apply the new value on submit

### Requirement: Config edits emit events
Every successful config edit SHALL emit a `brain_config_change` event with the field name, old value, new value, and timestamp.

#### Scenario: Threshold change is logged
- **WHEN** the user changes the threshold from 0.85 to 0.80 via the widget
- **THEN** a `brain_config_change` event SHALL be written to the brain event store with `field=threshold, old=0.85, new=0.80`

#### Scenario: Failed validation does not emit an event
- **WHEN** the user attempts to set the threshold to a value outside [0,1]
- **THEN** the edit SHALL fail validation, no event SHALL be emitted, and the widget SHALL show a validation error

### Requirement: Constrained inputs for safety
The config widget SHALL use constrained inputs (numeric ranges, enum pickers, toggles) appropriate to each knob's type and SHALL validate every edit before emitting an event.

#### Scenario: Numeric range enforced
- **WHEN** a user attempts to enter a non-numeric value into a numeric field
- **THEN** the widget SHALL reject the input client-side and SHALL NOT submit it

### Requirement: Confirmation for high-impact edits
Edits to confidence threshold and budget caps SHALL require an explicit confirmation step before being applied; toggles and enable/disable edits SHALL apply immediately.

#### Scenario: Threshold edit shows preview and confirm
- **WHEN** the user submits a new threshold value
- **THEN** the widget SHALL display "change threshold from X to Y? [confirm] [cancel]" and SHALL only emit the event after confirm
