## ADDED Requirements

### Requirement: Open and closed states with reopen allowed
A thread SHALL be in exactly one of two states at any time: `open` or `closed`. A thread starts in `open`, can transition to `closed` via a user action or a workflow's terminal action, and can transition back to `open` via an explicit reopen action.

#### Scenario: New thread is open
- **WHEN** a thread is spawned
- **THEN** its `State` SHALL be `open`

#### Scenario: Closing freezes the thread
- **WHEN** a thread transitions to `closed`
- **THEN** its input field and any widget action buttons SHALL be inert until the thread is reopened

#### Scenario: Reopen restores input and updates timestamp
- **WHEN** the user clicks the reopen affordance on a closed thread
- **THEN** the thread's `State` SHALL become `open`, its input SHALL be re-enabled, its `LastActivityAt` SHALL be updated, and a `thread_opened` event SHALL be emitted

### Requirement: Lifecycle events
Every state transition SHALL emit a structured event: `thread_opened` on spawn or reopen, `thread_closed` on close. Events SHALL carry the thread ID, the parent message ID, the new state, and (for `thread_closed`) the captured summary if any.

#### Scenario: Closing emits a thread_closed event
- **WHEN** a thread transitions to `closed`
- **THEN** a `thread_closed` event SHALL be written to the brain event store with the thread ID, parent message ID, and summary

#### Scenario: Spawning emits a thread_opened event
- **WHEN** a thread is created from a parent message
- **THEN** a `thread_opened` event SHALL be written with the thread ID, parent message ID, and `reason = spawned`

### Requirement: Optional summary on close
Closing a thread SHALL accept an optional one-line summary; the summary SHALL be persisted on the `Thread` record and rendered as a subtitle on the parent message in the main chat.

#### Scenario: Auto-close captures workflow summary
- **WHEN** a canonical workflow's terminal action runs (e.g. `/save` for configure-directories)
- **THEN** the thread SHALL close with a summary describing the outcome (e.g. "ignored 14 files in /tmp/scratch")

#### Scenario: Manual close prompts for optional summary
- **WHEN** the user clicks the close action button without a workflow terminal action
- **THEN** the user MAY provide an optional summary; if omitted the summary SHALL be empty

### Requirement: 10-second undo on auto-close
After an auto-close, the parent message SHALL display a "thread closed (reopen)" affordance for 10 seconds before settling into the standard closed-thread state.

#### Scenario: Undo within 10 seconds reopens the thread
- **WHEN** the user clicks the reopen affordance within 10 seconds of an auto-close
- **THEN** the thread SHALL transition back to `open`, its input SHALL be re-enabled, and the `thread_closed` event SHALL be marked `superseded` in the audit log

### Requirement: Persistence of thread tree
Chat history serialization SHALL persist `Thread` records and SHALL reconstruct the thread tree on reload. Closed threads SHALL round-trip with their summary and last-activity timestamp intact.

#### Scenario: Reload reconstructs an expanded thread
- **WHEN** a session containing one open thread with three replies is closed and reloaded
- **THEN** the thread SHALL appear in its previous state (open or closed) with all three replies grouped under the parent

#### Scenario: Closed-with-summary thread reloads with summary
- **WHEN** a closed thread with a summary is reloaded
- **THEN** the parent message in the main chat SHALL show the summary as its subtitle
