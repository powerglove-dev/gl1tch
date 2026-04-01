## MODIFIED Requirements

### Requirement: Activity Feed displays per-step output beneath step badge
The Activity Feed SHALL render each step as a single line showing only the step name and status badge. Raw output lines (from `output.value` / `step.lines`) SHALL NOT be rendered in the feed panel — the feed is a social-style status timeline, not a log tail. Step connector characters SHALL use Unicode box-drawing (`├─ ` for non-final, `└─ ` for final). Entry timestamps SHALL be formatted as 12-hour time with lowercase am/pm (e.g., `2:34 pm`), with no zero-padding on the hour.

#### Scenario: Step output is suppressed in the feed
- **WHEN** a pipeline step emits a `pipeline.step.done` event with `output.value: "line1\nline2\nline3"`
- **THEN** the Activity Feed shows only the step name and status badge — no output lines beneath it

#### Scenario: Empty output shows no lines
- **WHEN** a step's `output.value` is empty or the output map has no value key
- **THEN** no output lines appear beneath the step badge

#### Scenario: Step survives scroll
- **WHEN** the user scrolls up in the Activity Feed
- **THEN** step lines scroll together with their parent entry card

#### Scenario: Non-final step connector is tee
- **WHEN** a step is not the last visible step in its feed entry
- **THEN** its rendered line starts with `├─ `

#### Scenario: Final step connector is corner
- **WHEN** a step is the last visible step in its feed entry
- **THEN** its rendered line starts with `└─ `

#### Scenario: Timestamp uses 12hr format
- **WHEN** an entry's timestamp is 14:05:30
- **THEN** the rendered feed line shows `2:05 pm` (not `14:05:30`)
