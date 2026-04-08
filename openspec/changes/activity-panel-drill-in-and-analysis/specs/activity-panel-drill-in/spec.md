## ADDED Requirements

### Requirement: Indexing activity rows SHALL carry a document preview

Every activity event emitted for a collector indexing delta (indexing-kind / check-in event with a non-zero new-doc count) SHALL include a bounded preview list of the actual newly-indexed documents so the user can see what changed without leaving the activity sidebar. The preview SHALL contain at most 5 items and each item SHALL carry at minimum a document identifier, source label, timestamp, and a short human-readable title or summary derived from the underlying `Event` record.

#### Scenario: Collector indexes new documents and emits a preview

- **WHEN** the brain's periodic collector refresh detects N new documents (N ≥ 1) for a source
- **THEN** the emitted `brain:activity` event for that indexing delta carries an `items` array of up to 5 entries, each with `id`, `source`, `timestamp`, and `title`

#### Scenario: Indexing delta exceeds preview cap

- **WHEN** a collector refresh detects more than 5 new documents for a source
- **THEN** the emitted event's `items` array contains exactly 5 entries (the most recent by timestamp) and the count field still reflects the true delta

#### Scenario: No new documents

- **WHEN** a collector refresh detects zero new documents for a source
- **THEN** no indexing activity event is emitted and no preview is attached

### Requirement: Activity sidebar SHALL render the preview inline under indexing rows

The desktop activity sidebar SHALL render the preview items inline beneath the corresponding indexing row using the existing expandable-row pattern. The row SHALL expose a "View all" affordance that opens the drill-in modal.

#### Scenario: User expands an indexing row with preview items

- **WHEN** the user clicks an indexing activity row that carries a non-empty `items` array
- **THEN** the row expands to show each preview item with its source badge, timestamp, and title

#### Scenario: User clicks "View all" on an indexing row

- **WHEN** the user clicks the "View all" affordance on an expanded indexing row
- **THEN** the indexed-docs modal opens scoped to the row's source and time window

### Requirement: Drill-in modal SHALL list all indexed documents for a source/window

The system SHALL provide a full-size modal (the Indexed Docs Modal) that lists every indexed document for a given source and time window, not just the preview subset. The modal SHALL expose, per document, its source, timestamp, title/summary, and a mechanism to view the full raw document record.

#### Scenario: Modal loads the full set of indexed documents

- **WHEN** the modal is opened for a source and time window
- **THEN** it calls the `ListIndexedDocs` backend binding and renders every returned document in a scrollable list

#### Scenario: User inspects a raw document

- **WHEN** the user expands a document row in the modal
- **THEN** the row reveals the full raw `Event` record (all fields and metadata) rendered as formatted JSON

#### Scenario: Source has no documents in the requested window

- **WHEN** the modal is opened for a source/window that has no indexed documents
- **THEN** the list pane shows an empty-state message and the analysis pane is disabled until docs are present

### Requirement: Drill-in modal SHALL support multi-selection of documents

The modal SHALL allow the user to select an arbitrary subset of listed documents via checkboxes and SHALL display a running count of the current selection. The selection state is what drives the "Analyze selected" action defined by the analysis capability.

#### Scenario: User selects a subset of documents

- **WHEN** the user toggles checkboxes on multiple document rows
- **THEN** the modal updates the selection count and enables the "Analyze selected (N)" control

#### Scenario: User clears the selection

- **WHEN** the user clears all checkboxes
- **THEN** the "Analyze selected" control is disabled and the "Analyze all" control remains available
