## ADDED Requirements

### Requirement: Runner emits structured step-status log lines
The pipeline runner SHALL print a structured log line to stdout at each major step lifecycle transition. The format SHALL be `[step:<id>] status:<state>` where `<id>` is the step ID and `<state>` is one of `running`, `done`, or `failed`.

#### Scenario: Running line emitted before execute
- **WHEN** the DAG runner is about to call a step's `Execute` method
- **THEN** it prints `[step:<id>] status:running` to stdout before execution begins

#### Scenario: Done line emitted on success
- **WHEN** a step's `Execute` returns without error
- **THEN** the runner prints `[step:<id>] status:done` to stdout

#### Scenario: Failed line emitted on error
- **WHEN** a step's `Execute` returns an error (after exhausting retries)
- **THEN** the runner prints `[step:<id>] status:failed` to stdout

#### Scenario: Input and output steps do not emit status lines
- **WHEN** the runner processes a step of type `input` or `output`
- **THEN** no `[step:*] status:*` line is printed for that step
