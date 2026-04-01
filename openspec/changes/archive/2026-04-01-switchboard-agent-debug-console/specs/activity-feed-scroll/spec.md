## ADDED Requirements

### Requirement: Feed is bounded to terminal height
The activity feed SHALL render no more lines than the available vertical height. Lines that fall outside the visible window SHALL NOT be rendered, preventing layout overflow.

#### Scenario: Long output does not overflow
- **WHEN** an agent job emits more output lines than the feed panel height
- **THEN** only the lines that fit within the panel height are visible
- **THEN** the layout of the left column and right column remains aligned

### Requirement: Feed is scrollable
The Model SHALL track a `feedScrollOffset int` that can be incremented and decremented by the user. Pressing `↓` while the feed has focus SHALL increment the offset (scroll down into older entries). Pressing `↑` SHALL decrement the offset (scroll toward newest). The offset SHALL be clamped to `[0, max(0, totalLines - visibleHeight)]`.

#### Scenario: User scrolls down in feed
- **WHEN** the feed has focus and the user presses `↓`
- **THEN** `feedScrollOffset` increments by 1
- **THEN** the visible window shifts to show older content

#### Scenario: Offset clamped at bottom
- **WHEN** the user presses `↓` and the offset is already at the maximum
- **THEN** the offset does not change

#### Scenario: Offset clamped at top
- **WHEN** the user presses `↑` and the offset is already 0
- **THEN** the offset does not change

### Requirement: Feed auto-scrolls to top on new entry
When a new feed entry is appended the `feedScrollOffset` SHALL be reset to 0 (follow mode) so the newest entry is always visible.

#### Scenario: New job resets scroll
- **WHEN** a new agent job is submitted
- **THEN** `feedScrollOffset` is set to 0
- **THEN** the newest feed entry appears at the top of the feed panel
