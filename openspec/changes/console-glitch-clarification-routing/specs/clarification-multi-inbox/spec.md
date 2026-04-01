## ADDED Requirements

### Requirement: Multiple pending clarifications rendered as numbered inline list
When two or more clarifications are pending, the gl1tch panel SHALL render them as a numbered list within the chat thread. Each entry SHALL show its index, pipeline name, step ID, and question. The numbering SHALL reflect insertion order (oldest = 1).

#### Scenario: Two pending clarifications rendered as numbered list
- **WHEN** clarifications are pending for run 42 (`deploy-staging`) and run 43 (`run-tests`)
- **THEN** the thread shows:
  ```
  1. [deploy-staging › run-migration] Should I skip the DB migration?
  2. [run-tests › lint] Fix lint errors before continuing? (y/n)
  ```

#### Scenario: Single pending clarification not numbered
- **WHEN** exactly one clarification is pending
- **THEN** it is rendered without a numeric prefix

### Requirement: Plain answer routes to oldest pending clarification
When the user submits a message without an explicit index prefix and multiple clarifications are pending, the answer SHALL be routed to the oldest pending clarification (index 1).

#### Scenario: Plain answer matches oldest pending
- **WHEN** two clarifications are pending and user submits "yes" with no prefix
- **THEN** "yes" is written as the answer for `pendingClarifications[0]` (the oldest)
- **AND** that entry is removed from the pending list; the remaining item becomes index 1

#### Scenario: Queue advances after each answer
- **WHEN** three clarifications are pending and user answers them one at a time with plain messages
- **THEN** each answer resolves the current index 1; the list shrinks by one after each

### Requirement: Explicit index prefix routes answer out of order
When the user prefixes a message with `<N>:` (e.g., `2: yes`), the answer SHALL be routed to `pendingClarifications[N-1]`. If N is out of range, the input SHALL be treated as a plain answer to index 1 and an inline warning SHALL be shown.

#### Scenario: Explicit index routes to correct run
- **WHEN** two clarifications are pending and user submits `2: no`
- **THEN** "no" is written as the answer for the second pending clarification
- **AND** the first pending clarification remains unresolved at index 1

#### Scenario: Out-of-range index falls back to index 1 with warning
- **WHEN** one clarification is pending and user submits `3: yes`
- **THEN** "yes" is routed to `pendingClarifications[0]`
- **AND** an inline warning is shown: "Index 3 out of range — answered #1 instead"
