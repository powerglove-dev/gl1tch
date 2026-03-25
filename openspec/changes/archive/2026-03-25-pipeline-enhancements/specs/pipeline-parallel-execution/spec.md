## ADDED Requirements

### Requirement: Steps declare dependencies with needs
A pipeline step MAY declare a `needs` list of step IDs. The runner SHALL not execute a step until all steps in its `needs` list have completed successfully. Steps with no `needs` (or an empty list) are eligible to run immediately when the pipeline starts.

#### Scenario: Independent steps run concurrently
- **WHEN** two steps have no `needs` dependency on each other
- **THEN** the runner starts both steps concurrently without waiting for either to finish first

#### Scenario: Dependent step waits for needs
- **WHEN** step `B` declares `needs: [A]`
- **THEN** step `B` does not start until step `A` has status `done`

#### Scenario: Fan-out from common dependency
- **WHEN** steps `B` and `C` both declare `needs: [A]`
- **THEN** both `B` and `C` start concurrently once `A` completes

#### Scenario: Fan-in to aggregation step
- **WHEN** step `D` declares `needs: [B, C]`
- **THEN** step `D` starts only after both `B` and `C` have completed

### Requirement: Pipeline load detects dependency cycles
The pipeline loader SHALL perform a topological sort of the step dependency graph at load time. If a cycle is detected, `Load` SHALL return an error describing the cycle. No execution SHALL begin on a pipeline with a detected cycle.

#### Scenario: Cycle detected at load
- **WHEN** step A needs B and step B needs A
- **THEN** `pipeline.Load` returns an error containing the cycle description

#### Scenario: Valid DAG loads without error
- **WHEN** the step dependency graph is a valid DAG
- **THEN** `pipeline.Load` returns nil error

### Requirement: Pipeline enforces a maximum parallel step limit
The pipeline MAY declare a `max_parallel` integer field. When set, the runner SHALL not execute more than `max_parallel` steps simultaneously. The default when unset is 8. This prevents goroutine explosion on large `for_each` expansions.

#### Scenario: Parallel cap respected
- **WHEN** 20 independent steps are ready and `max_parallel` is 4
- **THEN** at most 4 steps execute simultaneously at any point

#### Scenario: Default cap applied when unset
- **WHEN** `max_parallel` is not declared in the pipeline YAML
- **THEN** the runner uses a default cap of 8

### Requirement: Step failure blocks dependent steps
When a step fails and has no `on_failure` routing, all steps that transitively depend on it SHALL be marked `skipped`. Sibling steps that do not depend on the failed step SHALL continue executing.

#### Scenario: Dependent skipped on upstream failure
- **WHEN** step A fails with no `on_failure` and step B needs A
- **THEN** step B is marked `skipped`

#### Scenario: Sibling continues despite upstream failure
- **WHEN** step A fails and step C has no dependency on A
- **THEN** step C continues executing normally
