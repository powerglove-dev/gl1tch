## Requirements

### Requirement: Inbox items display a "needs attention" marker for failed runs
When an inbox item represents a run that completed with a non-zero exit status, the item's description line SHALL include a `⚠` attention marker rendered in the theme's error color. Items with a zero exit status or still-running items SHALL NOT display the marker.

#### Scenario: Failed run shows attention marker
- **WHEN** a run completes with exit status != 0
- **THEN** the inbox item's description shows a `⚠` marker in the error color

#### Scenario: Successful run has no attention marker
- **WHEN** a run completes with exit status 0
- **THEN** the inbox item's description does NOT show a `⚠` marker

#### Scenario: In-flight run has no attention marker
- **WHEN** a run has not yet completed (exit status is nil)
- **THEN** the inbox item's description does NOT show a `⚠` marker

#### Scenario: Marker color matches theme error color
- **WHEN** the active theme has a custom error color
- **THEN** the `⚠` marker is rendered in that color
