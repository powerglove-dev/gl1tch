## ADDED Requirements

### Requirement: Idle chat displays the attention feed
When a chat session has no in-flight assistant response and no user input within the idle threshold, the chat panel SHALL render the current attention feed as the default visible content.

#### Scenario: Fresh session with attention items
- **WHEN** a chat session is opened, has no prior messages, and the activity package reports three attention items
- **THEN** the chat panel SHALL render an `attention_feed` message containing those three items at the top of the visible area

#### Scenario: Empty attention feed shows a quiet state
- **WHEN** the chat session is idle and the activity package reports zero attention items
- **THEN** the chat panel SHALL render a brief quiet-state message (e.g. "Nothing needs you right now") and no feed widget

### Requirement: Compact on input
The expanded attention feed SHALL collapse to a one-line pinned strip the moment the user begins typing in the chat input.

#### Scenario: User types and feed compacts
- **WHEN** the attention feed is rendered as a full message and the user types one character into the chat input
- **THEN** the full feed SHALL be replaced by a one-line strip pinned to the top of the chat panel showing the count and the top item title

#### Scenario: Strip expands inline when clicked
- **WHEN** the user clicks the compact attention strip
- **THEN** an `attention_feed` message containing the current items SHALL be appended to the conversation history (not floating) and the strip SHALL remain pinned

### Requirement: Idle threshold is 30 seconds
The idle threshold for showing the full attention feed SHALL be 30 seconds since the last user input or assistant response.

#### Scenario: Recent activity suppresses the idle feed
- **WHEN** an assistant response completed less than 30 seconds ago
- **THEN** the chat panel SHALL NOT render the full attention feed and SHALL keep showing the conversation
