## ADDED Requirements

### Requirement: Widget buttons synthesize chat input messages
A widget action button click SHALL be translated into a synthetic message routed through the same chat input pipeline as keyboard input, marked with a `synthetic: true` flag for audit purposes.

#### Scenario: Action chip click runs a slash command
- **WHEN** a user clicks an `action_chips` button whose `action` is `/brain config threshold 0.8`
- **THEN** the chat input pipeline SHALL receive a synthetic message with that text and the slash dispatcher SHALL handle it as if the user had typed it

#### Scenario: Action chip click sends a natural-language prompt
- **WHEN** a user clicks an `action_chips` button whose `action` is `dig into PR #412`
- **THEN** the chat input pipeline SHALL receive a synthetic message with that text and the assistant SHALL handle it as a normal user prompt

#### Scenario: Synthetic flag is preserved in the audit log
- **WHEN** any synthetic message is dispatched
- **THEN** the brain audit log SHALL record that the message originated from a widget action and SHALL identify the source widget message ID

### Requirement: No separate event bus for widget actions
The widget action protocol SHALL reuse the existing chat input pipeline and SHALL NOT introduce a separate event bus, RPC channel, or callback registry.

#### Scenario: Implementation uses chat input pipeline
- **WHEN** a developer adds a new action button type
- **THEN** wiring the button SHALL only require constructing a synthetic chat input message; it SHALL NOT require registering a new event handler

### Requirement: Inert past-session buttons
Widget buttons rendered as part of a reloaded historical session SHALL be visibly disabled and SHALL NOT trigger actions when clicked.

#### Scenario: Click on past-session button is ignored
- **WHEN** a user clicks an action button on a widget message from a session loaded from history
- **THEN** no synthetic message SHALL be dispatched and a tooltip SHALL explain that the widget is from a past session
