## MODIFIED Requirements

### Requirement: ClarificationRequested event consumed by gl1tch panel
The `ClarificationRequested` busd event (topic `agent.run.clarification`) SHALL be consumed by the gl1tch chat panel, not the console root model. The event payload schema is unchanged. `pipeline_bus.go` SHALL convert the event into a `ClarificationInjectMsg` Tea message and dispatch it to the root model, which SHALL forward it to the gl1tch panel.

#### Scenario: ClarificationRequested dispatched to glitch panel
- **WHEN** a `ClarificationRequested` event arrives on the busd topic `agent.run.clarification`
- **THEN** `pipeline_bus.go` converts it to a `ClarificationInjectMsg` Tea message
- **AND** the root console model forwards it to the gl1tch panel's `Update()` function
- **AND** no clarification state is set on the root console model

#### Scenario: ClarificationReply published by glitch panel
- **WHEN** the user submits an answer in the gl1tch chat and it resolves a pending clarification
- **THEN** the gl1tch panel publishes a `ClarificationReply` event on busd topic `agent.run.clarification.reply`
- **AND** the reply payload includes `RunID` and `Answer` unchanged from the existing schema
