## ADDED Requirements

### Requirement: Slash dispatcher passes thread scope
When a slash command is dispatched from inside a thread, the dispatcher SHALL set the `Scope` parameter (added by `chat-first-ui`) to `thread:<thread-id>` so handlers may branch on it.

#### Scenario: Command from inside a thread carries thread scope
- **WHEN** the user runs `/ignore foo` from inside a thread with ID `t-42`
- **THEN** the dispatcher SHALL invoke the handler with `Scope = "thread:t-42"`

#### Scenario: Command from main chat still carries main scope
- **WHEN** the user runs `/help` from the main chat input
- **THEN** the dispatcher SHALL invoke the handler with `Scope = "main"` even when one or more threads are currently expanded

### Requirement: Scope-aware handlers branch on Scope
Handlers for the four canonical workflows SHALL inspect the `Scope` parameter and operate on the thread's local state when `Scope` begins with `thread:`. Handlers MUST refuse to run thread-scoped commands when `Scope = "main"`.

#### Scenario: /dir add inside a directory thread targets that directory
- **WHEN** the user runs `/dir add ./scratch` inside a `configure-directories` thread
- **THEN** the handler SHALL add `./scratch` to the directory list local to that thread, not to any global config

#### Scenario: /dir add from main chat is rejected
- **WHEN** the user runs `/dir add ./scratch` in the main chat
- **THEN** the handler SHALL refuse with a clear "this command is only valid inside a directory configuration thread" message

### Requirement: Widgets inherit their thread scope
A widget rendered inside a thread SHALL declare its scope as the enclosing thread; clicks on its action chips SHALL be dispatched with that thread's scope.

#### Scenario: Action chip in a thread carries thread scope
- **WHEN** the user clicks an action chip rendered inside thread `t-7`
- **THEN** the synthetic chat input message produced by the click SHALL be dispatched with `Scope = "thread:t-7"`
