## ADDED Requirements

### Requirement: Feed cursor always visible
The feed panel SHALL ensure the cursor line is always within the visible viewport after any navigation or content update.

#### Scenario: Cursor at last line is visible
- **WHEN** the user navigates to the last logical line in the feed
- **THEN** that line SHALL be visible within the rendered panel and highlighted

#### Scenario: Scrolling reaches last line
- **WHEN** the user scrolls down to the end of the feed
- **THEN** the last content line SHALL be the last visible line in the panel (no blank overscroll gap)

#### Scenario: Cursor clamped after new entries arrive
- **WHEN** new feed entries are appended while the cursor is near the bottom
- **THEN** the cursor and scroll offset SHALL be reclamped so the cursor remains visible

### Requirement: Feed height consistent between clamp and render
The height value used in `clampFeedScroll` SHALL match the height value used in `View()` for the feed panel. A shared helper SHALL be used to prevent drift.

#### Scenario: No off-by-N scroll escape
- **WHEN** the terminal has any height and the feed has any number of entries
- **THEN** the maximum scroll offset computed by clamp SHALL equal the maximum scroll offset that keeps the last line visible in the rendered output

### Requirement: Feed logical-to-visual map kept current
The logical-to-visual line map used for cursor positioning SHALL be recalculated on a regular tick interval so that stale bounds do not persist between renders.

#### Scenario: Recalculation after entry appended
- **WHEN** a new feed entry is appended and the next tick fires
- **THEN** the logical-to-visual map SHALL reflect the new entry and scroll bounds SHALL be updated
