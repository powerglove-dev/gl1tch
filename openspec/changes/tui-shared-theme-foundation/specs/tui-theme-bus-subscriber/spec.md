## MODIFIED Requirements

### Requirement: busd subscriber cmd handles daemon unavailability gracefully
If the busd daemon is not running when a TUI process attempts to subscribe, the subscription SHALL schedule a retry after a backoff delay rather than silently dropping the subscription. The TUI SHALL continue to function with its last-known theme while retrying.

#### Scenario: busd not running at TUI startup — retry scheduled
- **WHEN** a sub-TUI initializes and the busd socket does not exist
- **THEN** the TUI starts successfully and displays its default or last-known theme
- **AND** a retry is scheduled so the subscription reconnects once busd becomes available

#### Scenario: busd connection lost mid-session — retry reconnects
- **WHEN** the busd daemon stops unexpectedly while a sub-TUI is running
- **THEN** the sub-TUI continues to function with the theme active at disconnect
- **AND** the subscription automatically reconnects when busd restarts

#### Scenario: Retry does not impact TUI responsiveness
- **WHEN** busd is unavailable and retries are pending
- **THEN** the TUI remains fully interactive and responsive to user input
