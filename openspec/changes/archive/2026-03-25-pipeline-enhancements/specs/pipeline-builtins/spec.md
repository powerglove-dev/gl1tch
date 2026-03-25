## ADDED Requirements

### Requirement: builtin.assert validates a condition
`builtin.assert` SHALL evaluate the `condition` arg as a boolean expression and return an error if it is false. Supported expressions: `"true"`, `"false"`, `"contains:<str>"` against `value` arg, `"matches:<regex>"` against `value` arg, `"len > <n>"` against `value` arg. On failure, the error message SHALL include the `message` arg if provided.

#### Scenario: Assert passes on true condition
- **WHEN** `builtin.assert` is called with `condition: "true"`
- **THEN** the step succeeds with no output

#### Scenario: Assert fails on false condition
- **WHEN** `builtin.assert` is called with `condition: "false"` and `message: "expected pass"`
- **THEN** the step returns an error containing `"expected pass"`

#### Scenario: Assert contains check
- **WHEN** `builtin.assert` is called with `condition: "contains:hello"` and `value: "hello world"`
- **THEN** the step succeeds

### Requirement: builtin.set_data injects variables into the execution context
`builtin.set_data` SHALL accept a `data` arg (map of string→string) and merge it into the step's output map. Subsequent steps can access these values via `step.<id>.data.<key>`.

#### Scenario: Variables accessible downstream
- **WHEN** `builtin.set_data` sets `{"env": "staging"}`
- **THEN** a subsequent step template `{{step.vars.data.env}}` resolves to `"staging"`

### Requirement: builtin.log emits a message to the pipeline output
`builtin.log` SHALL write the `message` arg (after template interpolation) to the pipeline's output writer at the time of execution. The step always succeeds and returns no output data.

#### Scenario: Message written to output
- **WHEN** `builtin.log` is called with `message: "deploying {{step.build.data.version}}"`
- **THEN** the interpolated message appears in the pipeline output stream

### Requirement: builtin.sleep pauses execution for a duration
`builtin.sleep` SHALL sleep for the duration specified in the `duration` arg (e.g. `"500ms"`, `"2s"`). If the pipeline context is cancelled during sleep, the step SHALL return a cancellation error immediately.

#### Scenario: Sleep completes normally
- **WHEN** `builtin.sleep` is called with `duration: "100ms"` and the context is not cancelled
- **THEN** execution resumes after at least 100ms

#### Scenario: Sleep interrupted by cancellation
- **WHEN** the pipeline context is cancelled while `builtin.sleep` is waiting
- **THEN** the step returns an error and does not block until the full duration

### Requirement: builtin.http_get performs an HTTP GET request
`builtin.http_get` SHALL perform an HTTP GET to the `url` arg. On a 2xx response it returns `{"status": <code>, "body": <text>}`. On a non-2xx response or network error it returns an error. Optional `timeout` arg (default `"10s"`) sets the request deadline.

#### Scenario: Successful GET
- **WHEN** `builtin.http_get` is called with a URL that returns 200
- **THEN** the step returns `status: 200` and the response body

#### Scenario: Non-2xx treated as error
- **WHEN** the server returns HTTP 404
- **THEN** `builtin.http_get` returns an error including the status code

#### Scenario: Network failure returns error
- **WHEN** the URL is unreachable
- **THEN** `builtin.http_get` returns an error after the timeout
