## ADDED Requirements

### Requirement: Three-column layout
The switchboard SHALL render three columns: left (30%, min 28), center (remainder after left and right), and right (25%, min 20). Two 2-character gutters separate the columns. Below a total width of 80 characters the right column SHALL be hidden and the layout SHALL fall back to two columns.

#### Scenario: Normal width renders three columns
- **WHEN** terminal width is ≥ 80 characters
- **THEN** left column occupies 30% of width, right column occupies 25%, center column occupies the remainder

#### Scenario: Narrow terminal hides right column
- **WHEN** terminal width is < 80 characters
- **THEN** layout renders two columns (left and center) without the activity feed column

### Requirement: Left column contents
The left column SHALL contain the pipeline launcher, inbox, and cron sections. The agent runner section SHALL NOT appear in the left column.

#### Scenario: Left column does not contain agent runner
- **WHEN** the switchboard View() is called
- **THEN** the left column output does not include the agent runner section

### Requirement: Center column stacks agents grid above agent runner
The center column SHALL render the agents grid panel in the upper portion and the agent runner panel directly below it, separated by a blank line.

#### Scenario: Center column layout order
- **WHEN** the switchboard View() is called
- **THEN** agents grid lines appear before agent runner lines in the center column

### Requirement: Right column contains activity feed
The right column SHALL exclusively contain the activity feed panel at the full column height.

#### Scenario: Activity feed fills right column
- **WHEN** the switchboard View() is called with terminal width ≥ 80
- **THEN** the right column contains only the activity feed panel spanning contentH rows
