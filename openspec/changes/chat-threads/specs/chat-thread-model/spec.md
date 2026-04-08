## ADDED Requirements

### Requirement: Thread is a first-class object
The chat data model SHALL represent each thread as a `Thread` record with at minimum: a `ID` (stable), a `ParentMessageID` (the top-level message that started it), `State` (`open` | `closed`), `Summary` (optional one-line, set on close), `LastActivityAt`, and `ExpandPref` (`inline` | `side_pane`).

#### Scenario: Spawning a thread creates a record
- **WHEN** a thread is spawned from a top-level message
- **THEN** a `Thread` record SHALL be created with `State = open`, `ParentMessageID` set, an empty `Summary`, and `LastActivityAt` set to the current time

#### Scenario: Threads are tied to a parent message
- **WHEN** any thread record is read from storage
- **THEN** its `ParentMessageID` SHALL reference an existing top-level message in the same chat session

### Requirement: Threads are flat
A thread parent SHALL be a top-level main-chat message; the dispatcher SHALL reject any attempt to spawn a thread from a message that already has a non-nil `ParentID`.

#### Scenario: Spawning a thread from inside a thread is rejected
- **WHEN** a user attempts to spawn a thread from a message whose `ParentID` is non-nil
- **THEN** the spawn SHALL fail with a "threads cannot be nested" error and no `Thread` record SHALL be created

### Requirement: ParentID is the routing key
Messages with a non-nil `ParentID` SHALL be routed into the corresponding thread by the chat renderer and SHALL NOT appear in the main chat stream.

#### Scenario: Reply is grouped under its thread
- **WHEN** a message with `ParentID = X` is appended to chat history
- **THEN** the renderer SHALL display it inside the thread whose `ParentMessageID = X` and SHALL NOT display it in the main chat stream

#### Scenario: Top-level message stays in main chat
- **WHEN** a message with `ParentID = nil` is appended
- **THEN** it SHALL appear in the main chat stream
