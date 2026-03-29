## ADDED Requirements

### Requirement: BusAwarePlugin optional interface for Tier 1 plugins
The `internal/plugin` package SHALL define:
```go
type BusAwarePlugin interface {
    Plugin
    SetBusClient(c busd.Client)
}
```
This interface is optional — the base `Plugin` interface is unchanged. Existing plugins require no modification.

#### Scenario: Non-bus-aware plugin loads without error
- **WHEN** a Tier 1 plugin that does not implement `BusAwarePlugin` is registered
- **THEN** the plugin loads and executes normally; no `SetBusClient` call is made

#### Scenario: Bus-aware plugin receives client on load
- **WHEN** a Tier 1 plugin implements `BusAwarePlugin` and the bus is available
- **THEN** the plugin manager calls `SetBusClient` with a connected `busd.Client` after the plugin is registered

### Requirement: Plugin manager injects bus client into BusAwarePlugin instances
When a Tier 1 plugin is registered and it satisfies `BusAwarePlugin`, the plugin manager SHALL call `SetBusClient` before the plugin is first used. If the bus is unavailable, the manager SHALL pass a no-op client implementation that satisfies `busd.Client` but silently drops all operations.

#### Scenario: SetBusClient called before first Execute
- **WHEN** a `BusAwarePlugin` is registered and then `Execute` is called
- **THEN** `SetBusClient` has been called at least once before `Execute` runs

#### Scenario: No-op client passed when bus unavailable
- **WHEN** the bus daemon is not running at plugin registration time
- **THEN** `SetBusClient` is still called with a no-op client; the plugin can call Publish/Subscribe without panic

### Requirement: Bus-aware plugin can publish and subscribe to pipeline events
A `BusAwarePlugin` with a valid `busd.Client` SHALL be able to call `client.Publish(ctx, topic, payload)` and `client.Subscribe(ctx, pattern, handler)` to interact with the full orcai event bus, including `pipeline.*` topics.

#### Scenario: Plugin publishes custom event
- **WHEN** a `BusAwarePlugin` calls `client.Publish(ctx, "plugin.my-plugin.done", payload)`
- **THEN** subscribers to `"plugin.*"` receive the event

#### Scenario: Plugin subscribes to step events
- **WHEN** a `BusAwarePlugin` subscribes to `"pipeline.step.*"`
- **THEN** the plugin's handler is invoked when any pipeline step event is published during the same process lifetime
