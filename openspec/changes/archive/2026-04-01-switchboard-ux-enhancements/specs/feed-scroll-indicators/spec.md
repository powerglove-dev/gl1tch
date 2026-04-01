## ADDED Requirements

### Requirement: Activity feed header shows scroll indicators
The activity feed box title SHALL include scroll indicator glyphs (`↑`, `↓`, or `↕`) when there is hidden content above, below, or both, relative to the current viewport. When all content fits within the viewport, no indicator is shown.

#### Scenario: No indicator when all content visible
- **WHEN** all feed entries fit within the visible feed viewport and feedScrollOffset is 0
- **THEN** the feed box title is `ACTIVITY FEED` with no scroll glyph

#### Scenario: Down indicator when content below
- **WHEN** there are feed entries below the current viewport bottom and feedScrollOffset is 0
- **THEN** the feed box title includes `↓`

#### Scenario: Up indicator when scrolled down
- **WHEN** feedScrollOffset is greater than 0 and no content exists below the viewport
- **THEN** the feed box title includes `↑`

#### Scenario: Both indicators when content above and below
- **WHEN** feedScrollOffset is greater than 0 and there is also content below the viewport
- **THEN** the feed box title includes `↕` (or both `↑` and `↓`)
