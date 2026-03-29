## ADDED Requirements

### Requirement: Two-column layout below search input
The Jump Window SHALL render sysop entries in a left column and active job entries in a right column, side by side, below the search input row. Each column SHALL occupy approximately half the available terminal width. When the terminal width is less than 40 characters, the system SHALL fall back to the existing single-column layout.

#### Scenario: Two columns rendered side by side
- **WHEN** the Jump Window opens on a terminal at least 40 columns wide
- **THEN** the sysop list appears on the left half and the active jobs list appears on the right half of the panel body

#### Scenario: Narrow terminal fallback
- **WHEN** the terminal width is less than 40 columns
- **THEN** sysop and active jobs are rendered in a single vertical column as before

### Requirement: Independent column focus via Tab
The Jump Window SHALL maintain independent cursor positions for the sysop column (left) and active jobs column (right). Pressing `tab` SHALL cycle focus between the left column and the right column. `j`/`k` (or `up`/`down`) SHALL move the cursor only within the currently focused column. `enter` and `e` SHALL act on the currently focused column's selected item.

#### Scenario: Tab cycles focus left to right
- **WHEN** focus is on the left (sysop) column and the user presses `tab`
- **THEN** focus moves to the right (active jobs) column

#### Scenario: Tab cycles focus right to left
- **WHEN** focus is on the right (active jobs) column and the user presses `tab`
- **THEN** focus moves to the left (sysop) column

#### Scenario: Navigation stays within focused column
- **WHEN** focus is on the left column and the user presses `j`
- **THEN** the cursor moves down within the sysop list only; the active jobs cursor is unchanged

#### Scenario: Enter activates item in focused column
- **WHEN** focus is on the right column and the user presses `enter`
- **THEN** the selected active job window is activated

### Requirement: Search filters only active jobs
The search input SHALL filter only the active jobs list. Sysop entries SHALL always be fully visible regardless of the current search query.

#### Scenario: Query does not hide sysop entries
- **WHEN** the user types a search query that matches no sysop entry names
- **THEN** all sysop entries remain visible in the left column

#### Scenario: Query narrows active jobs list
- **WHEN** the user types a search query
- **THEN** only active job entries whose names contain the query (case-insensitive) are shown in the right column

### Requirement: Tab hint in hint bar
The hint bar SHALL include `tab` as a documented key with the description `switch col`, positioned between the navigation hint and the select hint.

#### Scenario: Tab hint displayed
- **WHEN** the Jump Window is open
- **THEN** the hint bar shows `tab switch col` alongside `j/k nav`, `enter select`, `e edit`, and `esc cancel`

### Requirement: Modal height bounded by column content
The panel height SHALL be determined by the taller of the two columns (`max(len(sysop), len(activeJobs))`), not their sum. Rows in the shorter column SHALL be padded with empty cells to keep the divider aligned.

#### Scenario: Height equals taller column
- **WHEN** the sysop list has 2 entries and active jobs has 5 entries
- **THEN** the panel body renders 5 content rows (not 7)

#### Scenario: Empty column pads to match height
- **WHEN** the sysop list has 3 entries and active jobs has 1 entry
- **THEN** the right column renders 1 item row followed by 2 blank rows to match the left column height
