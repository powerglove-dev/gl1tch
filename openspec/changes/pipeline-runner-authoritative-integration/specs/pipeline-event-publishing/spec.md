## MODIFIED Requirements

### Requirement: Step output published to event bus when publish_to is set
When a pipeline step declares `publish_to: "<topic>"`, the runner SHALL publish the step's output map as a JSON payload to the named event bus topic after the step completes successfully. If the bus is unavailable, the publish SHALL fail silently and execution SHALL continue. This requirement was previously specced but unimplemented; this change fully wires it end-to-end using the injected `EventPublisher`.

#### Scenario: Output published on success
- **WHEN** step `fetch` succeeds with output `{"url": "..."}` and declares `publish_to: "orcai.fetch.done"`
- **THEN** a bus event with topic `"orcai.fetch.done"` and JSON payload `{"url": "..."}` is published via the injected `EventPublisher`

#### Scenario: Publish skipped on step failure
- **WHEN** a step with `publish_to` fails
- **THEN** no event is published to the topic

#### Scenario: Bus unavailable does not abort pipeline
- **WHEN** the event bus is not running and a step has `publish_to` set
- **THEN** the step executes normally; the publish error is logged at debug level but does not fail the step

### Requirement: Runner publishes pipeline step lifecycle events
The runner SHALL publish to `topics.StepStarted` when a step begins and to `topics.StepDone` or `topics.StepFailed` when it ends. Topic constants SHALL be imported from `internal/busd/topics`. The payload SHALL be JSON: `{"run_id": <int64>, "pipeline": "<name>", "step": "<id>", "status": "<status>", "duration_ms": <int>, "output": {<map or null>}}`.

#### Scenario: Step started event published
- **WHEN** the runner begins executing step `build`
- **THEN** an event on `topics.StepStarted` (`pipeline.step.started`) with `step: "build"` is published

#### Scenario: Step done event published with duration and output
- **WHEN** step `build` completes successfully with output `{"artifact": "bin/app"}`
- **THEN** an event on `topics.StepDone` with `status: "done"`, `duration_ms` set, and `output: {"artifact": "bin/app"}` is published

#### Scenario: Step failed event published
- **WHEN** step `build` fails after all retry attempts
- **THEN** an event on `topics.StepFailed` with `status: "failed"` is published

### Requirement: Runner connects to bus via injected EventPublisher, not bus.addr file
The pipeline runner SHALL NOT directly read `bus.addr` or manage bus connections. The publisher is fully injected at the cmd layer (see `pipeline-event-publisher` spec). The runner's only contract is: call `publisher.Publish(ctx, topic, payload)` and treat non-nil errors as debug-level logs.

#### Scenario: Runner uses injected publisher exclusively
- **WHEN** `WithEventPublisher(myPublisher)` is passed to `Run`
- **THEN** all bus publishing goes through `myPublisher`; no direct socket connections are made by the runner

#### Scenario: NoopPublisher means no bus IO
- **WHEN** no `WithEventPublisher` option is passed (defaults to NoopPublisher)
- **THEN** the runner makes no network or socket calls for event publishing
