## ADDED Requirements

### Requirement: Inbox and Cron Jobs panels support scroll navigation
When the Inbox or Cron Jobs panel is focused and contains more items than fit in the visible area, the user SHALL be able to scroll the list using the up/down arrow keys or `j`/`k`. The visible window SHALL advance one row at a time. The selected cursor SHALL be visible at all times (scroll follows cursor).

#### Scenario: Scroll down reveals items below the fold
- **WHEN** the Inbox panel is focused and the user presses the down arrow or `j`
- **THEN** the selected index advances by one and the scroll offset adjusts so the selected item is visible

#### Scenario: Scroll up at top does not scroll past beginning
- **WHEN** the Inbox panel is focused, the selected index is 0, and the user presses the up arrow or `k`
- **THEN** the selected index remains 0 and the scroll offset remains 0

#### Scenario: Scroll wraps at bottom of list
- **WHEN** the Inbox panel is focused and the selected index is at the last item
- **THEN** pressing down does not advance past the last item

#### Scenario: Cron Jobs panel scrolls identically
- **WHEN** the Cron Jobs panel is focused and contains more items than visible rows
- **THEN** up/down/`j`/`k` scroll the Cron list in the same manner as the Inbox

### Requirement: Inbox and Cron Jobs panels support fuzzy text search
The user SHALL be able to activate an in-panel text search by pressing `/` while the panel is focused. While search is active, a one-line search prompt SHALL appear inside the panel box. Keystrokes SHALL update the query in real time, filtering items to those whose name contains the query (case-insensitive). Pressing `Esc` SHALL dismiss the search and restore the full unfiltered list.

#### Scenario: Pressing / activates search mode
- **WHEN** the Inbox panel is focused and the user presses `/`
- **THEN** a search prompt row appears inside the Inbox panel and subsequent keystrokes append to the query

#### Scenario: Filter narrows the visible list in real time
- **WHEN** the search query is "opencode"
- **THEN** only inbox items whose name contains "opencode" (case-insensitive) are rendered; non-matching items are hidden

#### Scenario: Empty query shows all items
- **WHEN** the search is active and the query is empty
- **THEN** all items are shown (no filtering applied)

#### Scenario: Esc dismisses search and resets filter
- **WHEN** the search is active and the user presses `Esc`
- **THEN** the search prompt disappears, the query is cleared, and the full unfiltered list is restored

#### Scenario: Scroll offset resets when filter changes
- **WHEN** the user types a character that changes the filtered result set
- **THEN** the scroll offset resets to 0 so the first matching item is visible

#### Scenario: Cron Jobs panel search works identically
- **WHEN** the Cron Jobs panel is focused and the user activates search with `/`
- **THEN** the same search UX applies, filtering cron entries by name
