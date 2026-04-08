## ADDED Requirements

### Requirement: Next-prompt chip generator
After every assistant answer, the system SHALL generate 2–3 short follow-up prompts via a single qwen2.5:7b call and SHALL render them as an `action_chips` message immediately under the answer.

#### Scenario: Chips appear under each answer
- **WHEN** the assistant produces an answer
- **THEN** within a short delay an `action_chips` message containing 2–3 follow-up prompts SHALL appear directly below the answer

#### Scenario: Chip generation does not block the answer
- **WHEN** the assistant produces an answer
- **THEN** the answer SHALL be rendered to chat before the chip generator completes; chips SHALL be appended asynchronously

### Requirement: Cheap second pass, no loop call
Chip generation SHALL be a single local-model call (qwen2.5:7b) and SHALL NOT invoke the research loop, any researcher, or any paid model.

#### Scenario: Chip generation invokes only the local model
- **WHEN** the chip generator runs
- **THEN** it SHALL make exactly one call to qwen2.5:7b and zero calls to any other component

### Requirement: Chips never propose destructive actions
The chip generator's system prompt SHALL forbid destructive actions, and the parser SHALL drop any chip whose text matches a destructive intent classification.

#### Scenario: Destructive suggestion is dropped
- **WHEN** the model emits a chip text whose intent classifier labels it as `destructive`
- **THEN** that chip SHALL be removed from the output and SHALL NOT be rendered

### Requirement: Chip click flows through the widget action protocol
A chip click SHALL be dispatched as a synthetic chat input message via the existing `widget-action-protocol`.

#### Scenario: Click sends synthetic input
- **WHEN** the user clicks a chip whose text is `dig into PR #412`
- **THEN** a synthetic chat input message with that text SHALL be dispatched and the assistant SHALL handle it as a normal user prompt
