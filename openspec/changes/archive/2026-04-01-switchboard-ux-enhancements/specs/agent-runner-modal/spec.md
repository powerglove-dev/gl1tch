## ADDED Requirements

### Requirement: Agent runner opens a centred overlay modal
When the agent runner panel is focused and `enter` is pressed, the switchboard SHALL display a centred overlay modal that covers the main panels. The modal SHALL provide a multi-line textarea for prompt entry, a selection list for provider, and a selection list for model. The inline multi-step form (`formStep` 0/1/2) SHALL be replaced entirely by this modal flow.

#### Scenario: enter opens agent modal
- **WHEN** the agent runner panel is focused and the user presses `enter`
- **THEN** a centred overlay modal is displayed over the switchboard panels

#### Scenario: Modal contains prompt textarea and provider/model selectors
- **WHEN** the agent modal is displayed
- **THEN** it SHALL show a multi-line textarea for the prompt, a provider selection list, and a model selection list within the same modal surface

#### Scenario: Modal is at least 60 columns wide
- **WHEN** the terminal is 62 or more columns wide
- **THEN** the modal SHALL render at no less than 60 columns wide

#### Scenario: Modal degrades on narrow terminals
- **WHEN** the terminal is fewer than 62 columns wide
- **THEN** the inline form is used instead of the overlay modal

### Requirement: Agent modal captures all key events while open
While the agent modal is open, ALL key events SHALL be routed to the modal handler. No key event SHALL reach the underlying panel handlers.

#### Scenario: Keys inside modal do not affect panels
- **WHEN** the agent modal is open and the user presses any navigation key (arrow, tab, etc.)
- **THEN** those keys affect only the modal's internal state and do not change panel focus or selection

#### Scenario: ESC closes the modal without submitting
- **WHEN** the agent modal is open and the user presses `ESC`
- **THEN** the modal is dismissed and no agent job is started

### Requirement: ctrl+s submits the agent job from the modal
Within the agent modal, pressing `ctrl+s` SHALL validate that a provider, model, and non-empty prompt are selected/entered and then submit the agent job exactly as the existing inline form does. The modal SHALL close after submission.

#### Scenario: ctrl+s submits the job
- **WHEN** the agent modal is open, a provider and model are selected, and the prompt textarea is non-empty, and the user presses `ctrl+s`
- **THEN** the agent job is started and the modal is dismissed

#### Scenario: ctrl+s does nothing when prompt is empty
- **WHEN** the agent modal is open and the prompt textarea is empty and the user presses `ctrl+s`
- **THEN** no job is started and the modal remains open

### Requirement: Modal retains provider/model selection between opens
The last-selected provider and model SHALL be remembered on the model across modal open/close cycles within a single TUI session.

#### Scenario: Provider/model persist after close
- **WHEN** the user opens the modal, selects a provider and model, closes without submitting, then reopens the modal
- **THEN** the previously selected provider and model are pre-selected
