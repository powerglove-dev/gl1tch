## ADDED Requirements

### Requirement: Signal board fixed minimum height
The signal board panel SHALL use a fixed height formula that accommodates at least 20 agent body rows, regardless of how many agents are currently running or how many feed entries exist.

#### Scenario: 20 agents visible without growing
- **WHEN** there are 20 or more running agents
- **THEN** the signal board body SHALL have at least 20 visible rows and SHALL NOT grow beyond its fixed height

#### Scenario: Height does not grow with feed entries
- **WHEN** new feed entries are appended
- **THEN** the signal board height SHALL NOT increase

#### Scenario: Safety cap on small terminals
- **WHEN** the terminal height is less than the fixed height plus minimum feed space
- **THEN** the signal board height SHALL be capped so the feed panel retains at least 3 rows
