## ADDED Requirements

### Requirement: Configure-directories workflow
The system SHALL provide a `/config dirs` slash command that opens a thread containing a directory picker widget and a chat input scoped to the thread. The thread SHALL accept the slash commands `/dir add <path>`, `/dir scan`, `/dir ignore <pattern>`, and `/save`. The terminal action `/save` SHALL auto-close the thread with a summary describing the directories added or ignored.

#### Scenario: /config dirs spawns a directory thread
- **WHEN** the user runs `/config dirs`
- **THEN** a new thread SHALL be spawned whose first message contains a directory picker widget; the thread SHALL be in `open` state

#### Scenario: /dir add inside the thread updates the picker
- **WHEN** the user runs `/dir add ./scratch` inside the directory thread
- **THEN** the thread's directory picker widget SHALL show `./scratch` as a pending addition

#### Scenario: /save auto-closes with summary
- **WHEN** the user runs `/save` after adding two directories and ignoring three patterns
- **THEN** the thread SHALL transition to `closed` with a summary like "added 2 directories, ignored 3 patterns" and the parent message SHALL display the summary as its subtitle

### Requirement: Configure-skill workflow
The system SHALL provide a `/skills` slash command that returns a `widget_card` listing skills; clicking a skill SHALL open a thread containing that skill's configuration widget and a "test it" action chip. The thread SHALL accept `/edit <field>=<value>`, `/test`, `/save`, and `/cancel`. Both `/save` and `/cancel` SHALL auto-close the thread.

#### Scenario: Clicking a skill in the list spawns a configure-skill thread
- **WHEN** the user clicks a skill row in the `/skills` widget card
- **THEN** a new thread SHALL be spawned whose first message contains the selected skill's configuration widget

#### Scenario: /test runs the skill within the thread without leaving it
- **WHEN** the user runs `/test` inside a configure-skill thread
- **THEN** the test result SHALL be appended to the thread (not the main chat) as an assistant message

### Requirement: Triage-attention-item workflow
Clicking an attention feed item SHALL open a thread containing the item's full context plus an action chip set with `/why`, `/dismiss`, and `/act`. `/why` SHALL invoke the research loop scoped to the item; `/dismiss` and `/act` SHALL auto-close the thread.

#### Scenario: Click on attention item opens triage thread
- **WHEN** the user clicks an item rendered inside an `attention_feed` message
- **THEN** a new thread SHALL be spawned whose first message contains the item's context (source, severity, timestamp, body) and an `action_chips` payload containing `/why`, `/dismiss`, `/act`

#### Scenario: /why runs the research loop scoped to the item
- **WHEN** the user runs `/why` inside a triage thread
- **THEN** the research loop SHALL be invoked with a question derived from the attention item's context, and the resulting evidence bundle SHALL be appended to the thread (not the main chat)

#### Scenario: /dismiss auto-closes the thread
- **WHEN** the user runs `/dismiss` inside a triage thread
- **THEN** the thread SHALL transition to `closed` with summary `dismissed`; the attention item SHALL be marked dismissed in the activity store

### Requirement: Drill-into-evidence workflow
Clicking a single evidence item rendered inside an `evidence_bundle` message SHALL open a thread containing that evidence as a widget. Free-form messages typed in the thread SHALL be sent to the assistant with the selected evidence as additional context. The drill thread SHALL NOT auto-close; the user closes it explicitly when done.

#### Scenario: Clicking one evidence item opens a drill thread
- **WHEN** the user clicks one item inside an `evidence_bundle`
- **THEN** a new thread SHALL be spawned whose first message contains that single evidence item as a widget

#### Scenario: Follow-up questions inherit the evidence context
- **WHEN** the user types a free-form question inside a drill thread
- **THEN** the question SHALL be sent to the assistant with the drill thread's evidence appended to the prompt as additional context, and the response SHALL be appended to the thread (not the main chat)
