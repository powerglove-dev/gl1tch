## ADDED Requirements

### Requirement: Inbox detail cursor always visible
The inbox detail panel SHALL ensure the highlighted cursor line is always within the visible viewport after any navigation action.

#### Scenario: Cursor at last line is visible
- **WHEN** the user navigates to the last line of inbox detail content
- **THEN** that line SHALL be visible and highlighted within the rendered panel

#### Scenario: Page-down lands cursor in view
- **WHEN** the user presses page-down
- **THEN** the cursor SHALL move to a line that is visible in the new viewport position

#### Scenario: Navigation does not scroll cursor off bottom
- **WHEN** the user presses j/k and the cursor would move outside the visible area
- **THEN** the scroll offset SHALL adjust so the cursor remains within `[offset, offset+visibleH-1]`

### Requirement: Inbox detail scroll decoupled from cursor
The inbox detail scroll offset SHALL be updated using the keep-cursor-in-view algorithm rather than direct `scroll = cursor` assignment. Scroll SHALL only change enough to keep the cursor visible.

#### Scenario: Scrolling not farther than needed
- **WHEN** the cursor moves one line down and was already visible
- **THEN** the scroll offset SHALL NOT change
