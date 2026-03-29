## MODIFIED Requirements

### Requirement: EventPublisher interface defined in pipeline package
The pipeline package SHALL define an `EventPublisher` interface in `internal/pipeline/event.go`:
```
EventPublisher.Publish(ctx context.Context, topic string, payload []byte) error
```
The `Run` function SHALL accept an `EventPublisher` via the `WithEventPublisher(p EventPublisher)` run option. When nil or a `NoopPublisher` is passed, no events are emitted and pipeline execution is unaffected. The `busd` package SHALL NOT be imported by the pipeline package — the publisher is injected at the cmd layer.

#### Scenario: Run accepts nil publisher without panic
- **WHEN** `Run` is called with a nil-equivalent `NoopPublisher`
- **THEN** the pipeline executes normally and no events are emitted

#### Scenario: Publisher receives topic and payload
- **WHEN** a test `EventPublisher` is injected and a step completes
- **THEN** `Publish` is called with a non-empty topic string and a non-nil payload

#### Scenario: WithEventPublisher option wires publisher into runner
- **WHEN** `Run` is called with `WithEventPublisher(myPublisher)`
- **THEN** `myPublisher.Publish` is called for run and step lifecycle events

### Requirement: NoopPublisher satisfies EventPublisher and is safe to use
A `NoopPublisher` struct SHALL be exported from the pipeline package. Its `Publish` method SHALL always return nil. It SHALL be used as the default when no bus is available.

#### Scenario: NoopPublisher.Publish returns nil
- **WHEN** `NoopPublisher{}.Publish(ctx, "any.topic", []byte("data"))` is called
- **THEN** it returns nil error

### Requirement: busd client resolved at cmd layer and injected as EventPublisher
At `cmd/pipeline.go` and `cmd/sysop.go`, the runner entry points SHALL attempt to connect to the busd daemon using the address in `~/.config/orcai/bus.addr`. On success, a `busd.Client` SHALL be wrapped as an `EventPublisher` and passed via `WithEventPublisher`. On failure or absence of `bus.addr`, `NoopPublisher` SHALL be used.

#### Scenario: cmd/pipeline uses busd publisher when bus is available
- **WHEN** `bus.addr` is present and the bus is reachable
- **THEN** pipeline lifecycle events are delivered to the bus during `orcai pipeline run`

#### Scenario: cmd/pipeline falls back to NoopPublisher when bus absent
- **WHEN** `bus.addr` does not exist
- **THEN** `orcai pipeline run` completes normally with no bus publishing
