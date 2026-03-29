## ADDED Requirements

### Requirement: User can archive a signal board entry
The signal board SHALL allow the user to archive the selected entry by pressing `d`. Archiving sets the entry's `archived` flag to `true` and removes it from all filter views except `"archived"`. Archived entries SHALL persist in-memory for the session but are not saved to disk.

#### Scenario: Archive any-status entry
- **WHEN** the user selects any entry and presses `d`
- **THEN** the entry's `archived` flag is set to `true` and it disappears from the current view

#### Scenario: Archived entries hidden from non-archived filters
- **WHEN** the active filter is `"running"`, `"all"`, `"done"`, or `"failed"`
- **THEN** entries with `archived == true` are NOT shown

#### Scenario: Archived filter shows only archived entries
- **WHEN** the user cycles the filter to `"archived"`
- **THEN** only entries with `archived == true` are displayed, regardless of their `FeedStatus`

#### Scenario: Archive hint shown in hint bar
- **WHEN** the signal board is focused and at least one entry is visible
- **THEN** the hint bar includes a `d: archive` hint

### Requirement: Signal board default filter is "running"
The signal board SHALL initialise with `"running"` as its active filter so the board opens showing only active pipelines. Cycling through filters with `f` SHALL follow the order: `running → all → done → failed → archived → running`.

#### Scenario: Board opens with running filter
- **WHEN** the switchboard is first rendered
- **THEN** the signal board's active filter is `"running"` and only `FeedRunning` entries are visible

#### Scenario: Filter cycle includes archived and wraps
- **WHEN** the user presses `f` repeatedly from `"running"`
- **THEN** the filter advances: `running → all → done → failed → archived → running`
