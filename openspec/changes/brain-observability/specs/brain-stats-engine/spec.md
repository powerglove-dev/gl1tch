## ADDED Requirements

### Requirement: Event-driven incremental computation
The stats engine SHALL update metric time-series incrementally on each relevant brain event (`research_score`, `research_escalation`, `brain_feedback`, `brain_config_change`) and SHALL NOT re-scan the full event store on read.

#### Scenario: New score event updates the bucket
- **WHEN** a `research_score` event is emitted
- **THEN** the engine SHALL update the daily bucket for the affected metrics in O(1) time

#### Scenario: Read does not rescan raw events
- **WHEN** `/brain stats` requests the time-series for `accept_rate`
- **THEN** the engine SHALL serve it from cached buckets without iterating raw events

### Requirement: Bucketing — daily for 30 days, weekly beyond
Each metric time-series SHALL be stored as daily buckets for the most recent 30 days and weekly buckets for older history.

#### Scenario: 30-day window uses daily buckets
- **WHEN** the renderer requests the last 30 days of `accept_rate`
- **THEN** the engine SHALL return 30 daily values

#### Scenario: Older history uses weekly buckets
- **WHEN** the renderer requests data older than 30 days
- **THEN** the engine SHALL return weekly buckets

### Requirement: Stats query API
The engine SHALL expose `Stats.Get(metric, range) (Series, error)` and `Stats.Update(event) error` as its only public methods.

#### Scenario: Get returns a series for a known metric
- **WHEN** `Stats.Get("accept_rate", Last30Days)` is called
- **THEN** it SHALL return a `Series` value containing the bucketed values and the timestamp range

#### Scenario: Get for an unknown metric returns an error
- **WHEN** `Stats.Get("does_not_exist", anything)` is called
- **THEN** it SHALL return a non-nil error and an empty series

### Requirement: Config-change annotations
The stats engine SHALL annotate time-series with `brain_config_change` events so that the rendering layer can mark when a knob was edited.

#### Scenario: Config change appears as an annotation on the series
- **WHEN** the user edits the confidence threshold via `/brain config` and the stats engine processes the resulting `brain_config_change` event
- **THEN** subsequent `Get()` calls SHALL include that change as an annotation on the affected metrics' series

### Requirement: Storage colocated with brain events
The bucketed series SHALL be stored alongside the existing brain event store with no new database, schema migration, or external service.

#### Scenario: No new database file is created
- **WHEN** the stats engine starts for the first time
- **THEN** it SHALL initialize its bucket storage in the existing brain store path
