## ADDED Requirements

### Requirement: Switchboard supports multiple concurrently active jobs
The Switchboard model SHALL track all active jobs in a `map[string]*jobHandle` keyed by feed ID rather than a single `activeJob` pointer. Launching a new pipeline or agent SHALL NOT be blocked by an existing running job, up to the configured parallel cap (default 8).

#### Scenario: Second job launches while first is running
- **WHEN** job A is in `running` state and the user launches job B from the launcher
- **THEN** job B starts immediately and both A and B appear in the feed with status `running`

#### Scenario: Parallel job cap blocks additional launches
- **WHEN** 8 jobs are concurrently running (the default cap)
- **THEN** attempting to launch a 9th job displays a "max parallel jobs reached" warning
- **AND** no 9th job is started

#### Scenario: Completed job slot is freed
- **WHEN** one of the running jobs reaches `done` or `failed`
- **THEN** its slot is freed and a new job can be launched without hitting the cap

### Requirement: Each parallel job owns an independent background tmux window
The Switchboard SHALL create a separate background tmux window for each concurrently active job. Windows are named `orcai-<feedID>`. No two jobs SHALL share a window.

#### Scenario: Each job window is uniquely named
- **WHEN** jobs A and B run concurrently
- **THEN** each has a distinct tmux window target (`orcai-<feedID-A>` and `orcai-<feedID-B>`)

#### Scenario: Completed job window persists for review
- **WHEN** a job finishes
- **THEN** its background tmux window remains open so the user can navigate to it and review the output
