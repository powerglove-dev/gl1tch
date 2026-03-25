## Requirements

### Requirement: Step output published to event bus when publish_to is set
When a pipeline step declares `publish_to: "<topic>"`, the runner SHALL publish the step's output map as a JSON payload to the named event bus topic after the step completes successfully. If the bus is unavailable, the publish SHALL fail silently and execution SHALL continue.

#### Scenario: Output published on success
- **WHEN** step `fetch` succeeds with output `{"url": "..."}` and declares `publish_to: "orcai.fetch.done"`
- **THEN** a bus event with topic `"orcai.fetch.done"` and JSON payload `{"url": "..."}` is published

#### Scenario: Publish skipped on step failure
- **WHEN** a step with `publish_to` fails
- **THEN** no event is published to the topic

#### Scenario: Bus unavailable does not abort pipeline
- **WHEN** the event bus is not running and a step has `publish_to` set
- **THEN** the step executes normally; the publish error is logged but does not fail the step

### Requirement: Runner publishes pipeline step lifecycle events
The runner SHALL publish to `orcai.pipeline.step.started` when a step begins and to `orcai.pipeline.step.done` or `orcai.pipeline.step.failed` when it ends. The payload SHALL be JSON: `{"pipeline": "<name>", "step": "<id>", "status": "<status>", "duration_ms": <int>}`.

#### Scenario: Step started event published
- **WHEN** the runner begins executing step `build`
- **THEN** an event on `orcai.pipeline.step.started` with `step: "build"` is published

#### Scenario: Step done event published with duration
- **WHEN** step `build` completes successfully
- **THEN** an event on `orcai.pipeline.step.done` with `status: "done"` and `duration_ms` set is published

#### Scenario: Step failed event published
- **WHEN** step `build` fails after all retry attempts
- **THEN** an event on `orcai.pipeline.step.failed` with `status: "failed"` is published

### Requirement: Runner connects to bus from bus.addr file
The pipeline runner SHALL read `~/.config/orcai/bus.addr` to obtain the bus address. If the file is absent or the address is unreachable, lifecycle events and `publish_to` events SHALL be silently skipped. The runner SHALL NOT retry the bus connection during pipeline execution.

#### Scenario: Bus addr present and reachable
- **WHEN** `bus.addr` contains a valid address and the bus is running
- **THEN** lifecycle events are delivered to the bus

#### Scenario: Bus addr absent
- **WHEN** `bus.addr` does not exist
- **THEN** the runner skips all bus publishing and runs the pipeline normally
