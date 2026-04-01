## MODIFIED Requirements

### Requirement: Agent runner panel has fixed outer height
The agent runner panel (AGENT RUNNER box) SHALL occupy a fixed number of lines regardless of which form step (provider / model / prompt) is currently active. When the list of providers or models exceeds the inner height budget, the list SHALL scroll internally using an offset, preserving the outer box dimensions.

#### Scenario: Panel height unchanged when switching form steps
- **WHEN** the user presses `tab` to advance from the provider step to the model step
- **THEN** the total number of lines rendered by `buildAgentSection` does not change
- **THEN** the bottom border of the AGENT RUNNER box remains on the same row

#### Scenario: Long provider list scrolls inside box
- **WHEN** the number of providers exceeds the inner height budget
- **THEN** only the providers that fit within the budget are rendered
- **THEN** the outer box borders are not pushed down

#### Scenario: Long model list scrolls inside box
- **WHEN** the selected provider has more models than the inner height budget
- **THEN** only the models that fit within the budget are rendered
- **THEN** the outer box borders are not pushed down

### Requirement: Left column total height is stable
The left column (banner + launcher + agent runner) SHALL produce exactly `contentH` lines on every render. Extra space SHALL be padded with empty lines; content that would exceed `contentH` SHALL be clipped.

#### Scenario: Left column height equals right column height
- **WHEN** the terminal window size is received
- **THEN** `viewLeftColumn(contentH, leftW)` returns exactly `contentH` lines
- **THEN** the side-by-side join in `View()` produces no ragged rows
