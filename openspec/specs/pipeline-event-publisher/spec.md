## Requirements

### Requirement: EventPublisher interface defined in pipeline package
The pipeline package SHALL define an `EventPublisher` interface in `internal/pipeline/event.go`:
```
EventPublisher.Publish(ctx context.Context, topic string, payload []byte) error
```
The `Run` function SHALL accept an `EventPublisher` parameter. When nil or a `NoopPublisher` is passed, no events are emitted and pipeline execution is unaffected.

#### Scenario: Run accepts nil publisher without panic
- **WHEN** `Run` is called with a nil-equivalent `NoopPublisher`
- **THEN** the pipeline executes normally and no events are emitted

#### Scenario: Publisher receives topic and payload
- **WHEN** a test `EventPublisher` is injected and a step completes
- **THEN** `Publish` is called with a non-empty topic string and a non-nil payload

### Requirement: NoopPublisher satisfies EventPublisher and is safe to use
A `NoopPublisher` struct SHALL be exported from the pipeline package. Its `Publish` method SHALL always return nil. It SHALL be used as the default when no bus is available.

#### Scenario: NoopPublisher.Publish returns nil
- **WHEN** `NoopPublisher{}.Publish(ctx, "any.topic", []byte("data"))` is called
- **THEN** it returns nil error
