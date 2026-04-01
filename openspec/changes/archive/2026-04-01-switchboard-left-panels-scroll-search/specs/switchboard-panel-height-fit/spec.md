## ADDED Requirements

### Requirement: Left column panels fit within terminal height
The Switchboard left column SHALL distribute its available height across all four panels (Pipelines, Agent Runner, Inbox, Cron Jobs) such that the total rendered line count never exceeds the column height passed to `viewLeftColumn`. No panel SHALL overflow the terminal.

#### Scenario: Panels fit exactly when content is short
- **WHEN** all panels have fewer items than their allocated height
- **THEN** the left column renders exactly `height` lines with no overflow

#### Scenario: Panels are clamped when content overflows
- **WHEN** the Inbox contains more items than its allocated row budget
- **THEN** only the items that fit within the allocated rows are rendered; remaining items are accessible via scroll

#### Scenario: Cron panel dropped when budget is insufficient
- **WHEN** the remaining height after Inbox minimum (4 rows) cannot also satisfy the Cron minimum (4 rows)
- **THEN** the Cron Jobs panel is omitted from rendering and the Inbox receives the full remaining budget

### Requirement: Height budget is split 60/40 between Inbox and Cron Jobs
After subtracting the fixed-height panels (banner, Pipelines, Agent Runner) and separator rows, the Switchboard SHALL allocate 60% of the remaining rows to the Inbox panel and 40% to the Cron Jobs panel, each with a minimum of 4 rows.

#### Scenario: Standard terminal (24 rows) distributes correctly
- **WHEN** terminal height is 24 rows and fixed panels consume 10 rows
- **THEN** Inbox receives at least 8 rows and Cron receives at least 4 rows

#### Scenario: Large terminal gives both panels generous allocations
- **WHEN** terminal height is 50 rows and fixed panels consume 10 rows
- **THEN** Inbox and Cron together consume the remaining 40 rows at approximately 60/40 split
