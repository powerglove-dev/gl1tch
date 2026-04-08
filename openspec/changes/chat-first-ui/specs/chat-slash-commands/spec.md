## ADDED Requirements

### Requirement: Slash command dispatcher
The chat input SHALL recognize messages beginning with `/` as slash commands and SHALL dispatch them to a registered handler instead of sending them to the assistant.

#### Scenario: `/help` returns a help widget card
- **WHEN** the user types `/help` and submits
- **THEN** the dispatcher SHALL invoke the help handler, return a `widget_card` listing every registered slash command with its description, and SHALL NOT call the assistant

#### Scenario: Unknown slash command shows an error widget
- **WHEN** the user types `/nope`
- **THEN** the dispatcher SHALL render a `widget_card` titled "Unknown command" listing the closest matches, and SHALL NOT call the assistant

### Requirement: Required v1 slash commands
The dispatcher SHALL register at minimum the following commands: `/help`, `/status`, `/sessions`, `/brain`, `/researcher`, `/feed`.

#### Scenario: `/status` returns a status widget
- **WHEN** the user runs `/status`
- **THEN** a `widget_card` SHALL be returned containing workspace name, brain status, connection health, and active session count

#### Scenario: `/researcher list` returns the researcher table
- **WHEN** the user runs `/researcher list`
- **THEN** a `widget_card` SHALL be returned containing one row per registered researcher (name, source, version, schema, status)

#### Scenario: `/brain config` returns the brain config card
- **WHEN** the user runs `/brain config`
- **THEN** a `widget_card` SHALL be returned with the current threshold, budgets, and enabled researchers as key/value rows and editable action buttons

#### Scenario: `/sessions` returns the session history list
- **WHEN** the user runs `/sessions`
- **THEN** a `widget_card` SHALL be returned listing recent sessions with title, timestamp, and an action to reopen each

### Requirement: Slash commands bypass the assistant LLM
Slash commands SHALL be handled deterministically by Go code and SHALL NOT be routed through the assistant or any LLM.

#### Scenario: Slash command does not invoke the LLM
- **WHEN** the user runs any registered slash command
- **THEN** no call SHALL be made to qwen2.5:7b or any paid model as part of dispatching that command

### Requirement: Dynamic `/help` from registered commands
The `/help` widget content SHALL be generated from the live slash dispatcher registry, so adding a new slash command automatically appears in `/help`.

#### Scenario: New slash command appears in /help
- **WHEN** a new slash command is registered at startup
- **THEN** running `/help` SHALL list it without any change to the help handler itself

### Requirement: Dispatcher accepts a Scope parameter
The slash command dispatcher SHALL accept a `Scope` parameter on every dispatch call identifying the chat surface the command originated from. In v1 the scope is always `main` and SHALL be ignored by handlers; the field is reserved for the `chat-threads` change to route slash commands to the active thread context.

#### Scenario: Main-chat dispatch carries main scope
- **WHEN** a slash command is dispatched from the main chat input
- **THEN** the dispatcher SHALL set `Scope = "main"` and the handler SHALL behave identically to v1 with no scope-aware branching

#### Scenario: Handlers may inspect scope without changing behavior
- **WHEN** a handler reads the `Scope` parameter
- **THEN** it MAY log it for debugging but SHALL NOT change its v1 behavior based on its value
