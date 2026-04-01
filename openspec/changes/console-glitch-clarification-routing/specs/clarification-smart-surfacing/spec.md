## ADDED Requirements

### Requirement: Urgency evaluated on clarification injection
When a `ClarificationMessage` is injected into the chat thread, the gl1tch panel SHALL immediately evaluate urgency based on elapsed time since `askedAt` relative to the 10-minute pipeline timeout. If less than 50% of the timeout has elapsed (< 5 minutes), the clarification SHALL be surfaced passively. If 50% or more has elapsed (≥ 5 minutes), the clarification SHALL be surfaced urgently.

#### Scenario: Fresh clarification surfaced passively
- **WHEN** a clarification arrives 1 minute after `askedAt`
- **THEN** the message is injected into the thread and a badge count is incremented, but the chat is NOT scrolled to bottom and no urgent indicator is shown

#### Scenario: Near-timeout clarification surfaced urgently
- **WHEN** a clarification arrives 6 minutes after `askedAt`
- **THEN** the chat panel scrolls to the injected message and an urgent badge indicator is set on the panel header

### Requirement: Periodic urgency re-evaluation promotes passive to urgent
The gl1tch panel SHALL re-evaluate urgency on all pending clarifications every 60 seconds. Any item that transitions from passive to urgent (elapsed time crosses the 50% threshold) SHALL trigger the urgent surfacing behavior.

#### Scenario: Passive clarification promoted after timeout threshold crossed
- **WHEN** a clarification was injected passively and 4 minutes pass such that elapsed time exceeds 5 minutes
- **THEN** on the next 60-second tick the panel SHALL scroll to that message and set the urgent badge

#### Scenario: No promotion when all pending are already urgent
- **WHEN** all pending clarifications are already in urgent state
- **THEN** the 60-second tick produces no additional scroll or badge update

### Requirement: Batch arrival summarized as grouped notification
When two or more clarifications arrive within a 3-second window, the gl1tch panel SHALL inject a single batch summary message listing all pipeline names before injecting the individual clarification messages.

#### Scenario: Three clarifications arrive in quick succession
- **WHEN** clarifications for `deploy-staging`, `run-tests`, and `build-docker` arrive within 3 seconds of each other
- **THEN** a batch summary message is injected: "3 pipelines need input: deploy-staging, run-tests, build-docker"
- **AND** the individual clarification messages follow in the thread

#### Scenario: Two clarifications arrive more than 3 seconds apart
- **WHEN** a clarification for `deploy-staging` arrives and 10 seconds later one for `run-tests` arrives
- **THEN** no batch summary is injected; each is injected individually
