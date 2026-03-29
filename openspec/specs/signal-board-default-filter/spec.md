## ADDED Requirements

### Requirement: Signal board initialises with "running" filter
The `SignalBoard` struct SHALL initialise `activeFilter` to `"running"` rather than the empty string. The `buildSignalBoard` renderer SHALL treat `"running"` as the starting state, not a special override.

#### Scenario: Initial state shows only running entries
- **WHEN** the switchboard model is first constructed
- **THEN** `m.signalBoard.activeFilter` equals `"running"` and `filteredFeed` returns only `FeedRunning` entries
