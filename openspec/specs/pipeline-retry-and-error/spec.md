## Requirements

### Requirement: Steps support a retry policy
A pipeline step MAY declare a `retry` block with fields `max_attempts` (integer, ≥2) and `interval` (duration string, e.g. `"2s"`). When set, the runner SHALL re-execute the step on failure up to `max_attempts` total attempts, sleeping `interval` between each attempt. If all attempts fail, the step is marked failed.

#### Scenario: Step succeeds on second attempt
- **WHEN** a step fails on attempt 1 and succeeds on attempt 2, with `max_attempts: 3`
- **THEN** the step is marked done and only 2 executions occur

#### Scenario: Step exhausts all retries
- **WHEN** a step fails on every attempt with `max_attempts: 3`
- **THEN** after 3 total attempts the step is marked failed

#### Scenario: Interval respected between retries
- **WHEN** a step fails with `retry.interval: "500ms"`
- **THEN** the runner waits at least 500ms before the next attempt

#### Scenario: Step without retry block fails immediately
- **WHEN** a step has no `retry` block and its `Execute` returns an error
- **THEN** the step is marked failed after a single attempt

### Requirement: Steps support on_failure routing
A pipeline step MAY declare an `on_failure` field containing the ID of another step to execute when this step fails (after exhausting retries). The runner SHALL execute the `on_failure` step instead of propagating failure to dependent steps. The `on_failure` step's output SHALL NOT be treated as input to the original step's dependents.

#### Scenario: on_failure step executed after failure
- **WHEN** step A fails and declares `on_failure: cleanup`
- **THEN** step `cleanup` is executed after step A is marked failed

#### Scenario: Dependents of failed step still skipped
- **WHEN** step A fails and declares `on_failure: cleanup`, and step B needs A
- **THEN** step B is still marked `skipped` (on_failure does not unblock dependents)

#### Scenario: on_failure step can access failure context
- **WHEN** the `on_failure` step runs
- **THEN** it can access `{{step.<failed_id>.state}}` = `"failed"` in its templates

#### Scenario: No on_failure means failure propagates
- **WHEN** a step fails and has no `on_failure` declaration
- **THEN** dependent steps are marked `skipped` as normal

### Requirement: retry.on controls when retries apply
The `retry` block MAY include an `on` field. Supported values are `"always"` (default, retry on any error) and `"on_failure"` (retry only on non-nil error, not on context cancellation). When omitted, `"always"` is assumed.

#### Scenario: Default retry on any error
- **WHEN** `retry.on` is not set and the step returns any error
- **THEN** the runner retries up to max_attempts

#### Scenario: Context cancellation stops retries
- **WHEN** the pipeline context is cancelled mid-retry
- **THEN** the runner stops retrying and marks the step as failed immediately
