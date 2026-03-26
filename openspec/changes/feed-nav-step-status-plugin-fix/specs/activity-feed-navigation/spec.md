## ADDED Requirements

### Requirement: Activity Feed is a focusable panel
The Activity Feed panel SHALL be included in the tab-cycle focus order alongside the Pipeline Launcher and Agent Runner panels. When focused, the feed SHALL display a visible selection cursor on the currently active line.

#### Scenario: Tab advances focus to feed
- **WHEN** the user presses Tab while the Agent Runner is focused
- **THEN** focus moves to the Activity Feed and `feedFocused` is `true`

#### Scenario: Tab wraps back to Pipeline Launcher
- **WHEN** the user presses Tab while the Activity Feed is focused
- **THEN** focus moves to the Pipeline Launcher and `feedFocused` is `false`

#### Scenario: Selection cursor is visible when focused
- **WHEN** the Activity Feed is focused
- **THEN** the currently selected line is highlighted with a distinct cursor indicator (e.g., `>` prefix or highlight colour)

### Requirement: Feed supports line-by-line navigation
When the Activity Feed is focused, the user SHALL be able to move the selection cursor up and down one line at a time using `j`/`k` or arrow keys.

#### Scenario: j moves cursor down
- **WHEN** the Activity Feed is focused and the user presses `j` or the down-arrow key
- **THEN** the `feedCursor` increments by 1, clamped to the last visible line

#### Scenario: k moves cursor up
- **WHEN** the Activity Feed is focused and the user presses `k` or the up-arrow key
- **THEN** the `feedCursor` decrements by 1, clamped to 0

#### Scenario: Navigation does not fire when feed is not focused
- **WHEN** the Activity Feed is not focused and the user presses `j` or `k`
- **THEN** the `feedCursor` does not change

### Requirement: Feed supports page navigation
When the Activity Feed is focused, the user SHALL be able to scroll through feed content a page at a time using PgDn / PgUp, and jump to the top or bottom using `g` / `G`.

#### Scenario: PgDn advances by one viewport height
- **WHEN** the Activity Feed is focused and the user presses PgDn
- **THEN** `feedCursor` advances by the visible feed panel height, clamped to the last line

#### Scenario: PgUp retreats by one viewport height
- **WHEN** the Activity Feed is focused and the user presses PgUp
- **THEN** `feedCursor` decrements by the visible feed panel height, clamped to 0

#### Scenario: g jumps to top
- **WHEN** the Activity Feed is focused and the user presses `g`
- **THEN** `feedCursor` is set to 0

#### Scenario: G jumps to bottom
- **WHEN** the Activity Feed is focused and the user presses `G`
- **THEN** `feedCursor` is set to the index of the last visible line

### Requirement: Status bar reflects feed navigation keys when focused
When the Activity Feed is focused, the status bar hint line SHALL show the available navigation keys (`↑↓ nav · PgUp/PgDn page · g/G top/bottom · enter open · tab focus`).

#### Scenario: Focused hint shown
- **WHEN** the Activity Feed is focused
- **THEN** the bottom status bar displays feed-specific navigation hints

#### Scenario: Default hint restored when focus leaves feed
- **WHEN** focus moves away from the Activity Feed
- **THEN** the status bar reverts to the default hint line
